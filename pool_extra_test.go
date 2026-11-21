package nostr

import (
	"context"
	stdjson "encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// These mirror the narrow pool shapes downstream consumers rely on, so this
// package stays compile-time compatible without importing those consumers.
type streamPool interface {
	SubscribeMany(ctx context.Context, urls []string, filter Filter, opts SubscriptionOptions) chan RelayEvent
	PublishMany(ctx context.Context, urls []string, evt Event) chan PublishResult
}

type batchFetcher interface {
	FetchManyFilters(ctx context.Context, urls []string, filters []Filter, opts SubscriptionOptions) chan RelayEvent
}

var _ streamPool = (*Pool)(nil)
var _ batchFetcher = (*Pool)(nil)

// fakeCooldowns is a CooldownStore that records saves.
type fakeCooldowns struct {
	m     map[string]int64
	saves int
}

func (f *fakeCooldowns) LoadCooldowns() map[string]int64 { return f.m }
func (f *fakeCooldowns) SaveCooldowns(m map[string]int64) {
	f.m = m
	f.saves++
}

func TestPersistedCooldownSkipsRelayOnStart(t *testing.T) {
	cooldowns := &fakeCooldowns{m: map[string]int64{
		"wss://dead.example": time.Now().Add(time.Minute).Unix(),
	}}
	p := NewPool(PoolOptions{Context: context.Background(), Cooldowns: cooldowns, CooldownTTL: time.Minute})
	defer p.Close("test closed")

	got := p.usableRelays([]string{"wss://dead.example", "wss://alive.example"})
	if len(got) != 1 || got[0] != "wss://alive.example" {
		t.Fatalf("usableRelays = %v, want [wss://alive.example]", got)
	}
}

func TestPenalizePersistsAndClearsCooldown(t *testing.T) {
	cooldowns := &fakeCooldowns{}
	p := NewPool(PoolOptions{Context: context.Background(), Cooldowns: cooldowns, CooldownTTL: time.Minute})
	defer p.Close("test closed")

	p.penalize("wss://dead.example")
	if _, ok := cooldowns.m[NormalizeURL("wss://dead.example")]; !ok || cooldowns.saves == 0 {
		t.Fatalf("penalize did not persist (saves=%d, m=%v)", cooldowns.saves, cooldowns.m)
	}
	p.clearPenalty("wss://dead.example")
	if len(cooldowns.m) != 0 {
		t.Fatalf("clearPenalty left %v", cooldowns.m)
	}
}

func TestMergeFiltersCoalescesCompatibleAuthors(t *testing.T) {
	a := SecretKey{1}.Public()
	b := SecretKey{2}.Public()
	c := SecretKey{3}.Public()
	groups := mergeFilters([]Filter{
		{Kinds: []Kind{KindGiftWrap}, Authors: []PubKey{a}},
		{Kinds: []Kind{KindGiftWrap}, Authors: []PubKey{b}},
		{Kinds: []Kind{KindGiftWrap}, Authors: []PubKey{c}},
	})
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if len(groups[0].Authors) != 3 {
		t.Fatalf("merged authors = %d, want 3", len(groups[0].Authors))
	}
}

func TestMergeFiltersKeepsIncompatibleSeparate(t *testing.T) {
	a := SecretKey{1}.Public()
	b := SecretKey{2}.Public()
	groups := mergeFilters([]Filter{
		{Kinds: []Kind{KindGiftWrap}, Authors: []PubKey{a}},
		{Kinds: []Kind{KindTextNote}, Authors: []PubKey{b}},
		{Kinds: []Kind{KindGiftWrap}, Authors: []PubKey{b}, Tags: TagMap{"d": {"x"}}},
	})
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3 (kind and tag mismatches must not merge)", len(groups))
	}
}

func TestLocalRelayPublishFetchAndCancel(t *testing.T) {
	key := SecretKey{1}
	event := Event{Kind: 1, CreatedAt: 1, Content: "local transport test", Tags: Tags{}}
	if err := event.Sign(key); err != nil {
		t.Fatal(err)
	}
	published := make(chan Event, 1)
	requests := make(chan struct{}, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		send := func(values ...any) error {
			data, err := stdjson.Marshal(values)
			if err != nil {
				return err
			}
			return conn.Write(ctx, websocket.MessageText, data)
		}
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var message []stdjson.RawMessage
			if stdjson.Unmarshal(data, &message) != nil || len(message) < 2 {
				return
			}
			var command string
			if stdjson.Unmarshal(message[0], &command) != nil {
				return
			}
			switch command {
			case "EVENT":
				var received Event
				if stdjson.Unmarshal(message[1], &received) != nil {
					return
				}
				select {
				case published <- received:
				case <-ctx.Done():
					return
				}
				if send("OK", received.ID.Hex(), true, "") != nil {
					return
				}
			case "REQ":
				var id string
				if stdjson.Unmarshal(message[1], &id) != nil {
					return
				}
				if send("EVENT", id, event) != nil || send("EOSE", id) != nil {
					return
				}
				select {
				case requests <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}
		}
	}))
	defer server.Close()
	pool := NewPool(PoolOptions{Context: ctx})
	defer pool.Close("test closed")
	urls := []string{"ws" + strings.TrimPrefix(server.URL, "http")}
	results := pool.PublishMany(ctx, urls, event)
	select {
	case result, ok := <-results:
		if !ok || result.Error != nil {
			t.Fatalf("publish failed: %v", result.Error)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case received := <-published:
		if received.ID != event.ID || !received.VerifySignature() {
			t.Fatal("published event changed")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	filter := Filter{Kinds: []Kind{1}, Authors: []PubKey{key.Public()}}
	fetched := pool.FetchMany(ctx, urls, filter, SubscriptionOptions{})
	count := 0
	for received := range fetched {
		if received.Event.ID != event.ID {
			t.Fatal("wrong fetched event")
		}
		count++
	}
	if count != 1 || ctx.Err() != nil {
		t.Fatalf("fetch did not finish at EOSE: count=%d, err=%v", count, ctx.Err())
	}
	<-requests
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	subscribed := pool.SubscribeMany(subCtx, urls, filter, SubscriptionOptions{})
	select {
	case <-requests:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	subCancel()
	for range subscribed {
	}
	if ctx.Err() != nil {
		t.Fatal("subscription did not cancel promptly")
	}
	pool.Close("test closed")
	pool.Close("test closed")
}

// secretSigner adapts a raw secret to Signer so tests can exercise the
// AuthSigner path the way an external (NIP-07) account would.
type secretSigner struct{ sk SecretKey }

func (s secretSigner) GetPublicKey(context.Context) (PubKey, error) {
	return s.sk.Public(), nil
}

func (s secretSigner) SignEvent(_ context.Context, ev *Event) error {
	return ev.Sign(s.sk)
}

func TestLocalRelayAuthRequired(t *testing.T) {
	key := SecretKey{7}
	runRelayAuthRequired(t, key, authKeyOptions(key))
}

// TestAuthSignerRelayAuthRequired covers NIP-07/NIP-46 accounts: a pool with
// only AuthSigner (no raw key) must still answer a relay's NIP-42 challenge.
func TestAuthSignerRelayAuthRequired(t *testing.T) {
	key := SecretKey{7}
	runRelayAuthRequired(t, key, authSignerOptions(secretSigner{sk: key}))
}

// TestProactiveAuthAnswered covers a relay that sends AUTH on connect and
// serves nothing until answered (relay.damus.io): a reactive CLOSED is never
// offered, so only a proactive AUTH response can authenticate.
func TestProactiveAuthAnswered(t *testing.T) {
	key := SecretKey{7}
	runProactiveAuth(t, key, authKeyOptions(key))
	runProactiveAuth(t, key, authSignerOptions(secretSigner{sk: key}))
}

func runRelayAuthRequired(t *testing.T, key SecretKey, cfg PoolOptions) {
	t.Helper()
	event := Event{Kind: 1, CreatedAt: 1, Content: "authed", Tags: Tags{}}
	if err := event.Sign(key); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var authed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		challenge, _ := stdjson.Marshal([]any{"AUTH", "test-challenge"})
		if conn.Write(ctx, websocket.MessageText, challenge) != nil {
			return
		}
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var message []stdjson.RawMessage
			if stdjson.Unmarshal(data, &message) != nil || len(message) < 2 {
				return
			}
			var command string
			if stdjson.Unmarshal(message[0], &command) != nil {
				return
			}
			switch command {
			case "AUTH":
				var ev Event
				if stdjson.Unmarshal(message[1], &ev) != nil {
					return
				}
				if ev.Kind != KindClientAuthentication || !ev.VerifySignature() {
					return
				}
				if ev.Tags.FindWithValue("challenge", "test-challenge") == nil {
					return
				}
				authed.Store(true)
				ok, _ := stdjson.Marshal([]any{"OK", ev.ID.Hex(), true, ""})
				if conn.Write(ctx, websocket.MessageText, ok) != nil {
					return
				}
			case "REQ":
				var id string
				if stdjson.Unmarshal(message[1], &id) != nil {
					return
				}
				if !authed.Load() {
					closed, _ := stdjson.Marshal([]any{"CLOSED", id, "auth-required: authenticate"})
					if conn.Write(ctx, websocket.MessageText, closed) != nil {
						return
					}
					continue
				}
				ev, _ := stdjson.Marshal([]any{"EVENT", id, event})
				eose, _ := stdjson.Marshal([]any{"EOSE", id})
				if conn.Write(ctx, websocket.MessageText, ev) != nil || conn.Write(ctx, websocket.MessageText, eose) != nil {
					return
				}
			}
		}
	}))
	defer server.Close()
	pool := NewPool(withContext(ctx, cfg))
	defer pool.Close("test closed")
	urls := []string{"ws" + strings.TrimPrefix(server.URL, "http")}
	fetched := pool.FetchMany(ctx, urls, Filter{Kinds: []Kind{1}}, SubscriptionOptions{})
	count := 0
	for received := range fetched {
		if received.Event.ID == event.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("fetched %d events after NIP-42 auth, want 1", count)
	}
}

// runProactiveAuth drives a relay that sends AUTH on connect and withholds the
// requested events until the challenge is answered. It never sends a
// "auth-required:" CLOSED, so the reactive pool path cannot help: only a
// proactive AUTH response authenticates.
func runProactiveAuth(t *testing.T, key SecretKey, cfg PoolOptions) {
	t.Helper()
	event := Event{Kind: 1, CreatedAt: 1, Content: "authed", Tags: Tags{}}
	if err := event.Sign(key); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		challenge, _ := stdjson.Marshal([]any{"AUTH", "test-challenge"})
		if conn.Write(ctx, websocket.MessageText, challenge) != nil {
			return
		}
		authed := false
		pending := ""
		serve := func(id string) {
			ev, _ := stdjson.Marshal([]any{"EVENT", id, event})
			eose, _ := stdjson.Marshal([]any{"EOSE", id})
			_ = conn.Write(ctx, websocket.MessageText, ev)
			_ = conn.Write(ctx, websocket.MessageText, eose)
		}
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var message []stdjson.RawMessage
			if stdjson.Unmarshal(data, &message) != nil || len(message) < 2 {
				return
			}
			var command string
			if stdjson.Unmarshal(message[0], &command) != nil {
				return
			}
			switch command {
			case "AUTH":
				var ev Event
				if stdjson.Unmarshal(message[1], &ev) != nil {
					return
				}
				if ev.Kind != KindClientAuthentication || !ev.VerifySignature() {
					return
				}
				if ev.PubKey != key.Public() {
					return
				}
				if ev.Tags.FindWithValue("challenge", "test-challenge") == nil {
					return
				}
				authed = true
				ok, _ := stdjson.Marshal([]any{"OK", ev.ID.Hex(), true, ""})
				if conn.Write(ctx, websocket.MessageText, ok) != nil {
					return
				}
				if pending != "" {
					serve(pending)
					pending = ""
				}
			case "REQ":
				var id string
				if stdjson.Unmarshal(message[1], &id) != nil {
					return
				}
				if authed {
					serve(id)
				} else {
					// Hold the request until the challenge is answered.
					pending = id
				}
			}
		}
	}))
	defer server.Close()
	pool := NewPool(withContext(ctx, cfg))
	defer pool.Close("test closed")
	urls := []string{"ws" + strings.TrimPrefix(server.URL, "http")}
	fetched := pool.FetchMany(ctx, urls, Filter{Kinds: []Kind{1}}, SubscriptionOptions{})
	count := 0
	for received := range fetched {
		if received.Event.ID == event.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("fetched %d events after a proactive AUTH, want 1", count)
	}
}

// TestFailedRelayCooledDown covers the resilience default: a relay that fails
// to connect is cooled down after a one-shot fetch, so later operations skip it
// while a reachable relay is still used.
func TestFailedRelayCooledDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := "ws://" + ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// A relay that answers EOSE so it counts as reachable.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var message []stdjson.RawMessage
			if stdjson.Unmarshal(data, &message) != nil || len(message) < 2 {
				return
			}
			var command string
			if stdjson.Unmarshal(message[0], &command) != nil {
				return
			}
			if command == "REQ" {
				var id string
				_ = stdjson.Unmarshal(message[1], &id)
				eose, _ := stdjson.Marshal([]any{"EOSE", id})
				if conn.Write(ctx, websocket.MessageText, eose) != nil {
					return
				}
			}
		}
	}))
	defer server.Close()
	alive := "ws" + strings.TrimPrefix(server.URL, "http")

	pool := NewPool(PoolOptions{Context: ctx})
	defer pool.Close("test closed")

	for range pool.FetchMany(ctx, []string{dead, alive}, Filter{Kinds: []Kind{1}}, SubscriptionOptions{}) {
	}

	usable := pool.usableRelays([]string{dead, alive})
	if len(usable) != 1 || usable[0] != alive {
		t.Fatalf("usable relays = %v, want only %q", usable, alive)
	}
}

func TestConnectedRelays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var message []stdjson.RawMessage
			if stdjson.Unmarshal(data, &message) != nil || len(message) < 2 {
				return
			}
			var command string
			if stdjson.Unmarshal(message[0], &command) != nil {
				return
			}
			if command == "REQ" {
				var id string
				_ = stdjson.Unmarshal(message[1], &id)
				eose, _ := stdjson.Marshal([]any{"EOSE", id})
				if conn.Write(ctx, websocket.MessageText, eose) != nil {
					return
				}
			}
		}
	}))
	defer server.Close()
	pool := NewPool(PoolOptions{Context: ctx})
	defer pool.Close("test closed")
	url := "ws" + strings.TrimPrefix(server.URL, "http")

	// A relay the pool has never dialled is not connected.
	if states := pool.ConnectedRelays([]string{"wss://never.example", url}); states["wss://never.example"] || states[url] {
		t.Fatalf("unexpected connected state before dial: %v", states)
	}
	for range pool.FetchMany(ctx, []string{url}, Filter{Kinds: []Kind{1}}, SubscriptionOptions{}) {
	}
	if states := pool.ConnectedRelays([]string{url}); !states[url] {
		t.Fatalf("relay not connected after fetch: %v", states)
	}
}

func TestCloseCancelsBlockedForwarder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := make(chan int, 1)
	source <- 1
	out := forward(ctx, cancel, source)
	cancel()
	select {
	case <-out:
	case <-time.After(time.Second):
		t.Fatal("forwarder did not cancel")
	}
}
