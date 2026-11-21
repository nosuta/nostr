package nostr

import (
	"context"
	"sync"
	"time"
)

// authCacheTTL and authCacheMax bound the NIP-42 AUTH signature cache. A
// challenge is per-connection, so a short TTL is enough to absorb a relay's
// repeated asks (or the pool's own retries) without growing without bound.
const (
	authCacheTTL = 5 * time.Minute
	authCacheMax = 128
)

// authSigned is one cached NIP-42 AUTH signature.
type authSigned struct {
	event Event
	at    time.Time
}

// authCall is one in-flight AUTH signing, shared by concurrent callers that
// asked for the same challenge.
type authCall struct {
	done  chan struct{}
	event Event
	err   error
}

// authDeduper collapses repeated NIP-42 AUTH signings for the same
// (relay, challenge). A relay — or the pool's own retries and multiple
// subscriptions — may ask for an identical challenge several times. With an
// external signer (NIP-07, NIP-46) every signing is a round trip that can prompt
// or stall, so an identical challenge is signed once and the response reused
// while it is fresh. Concurrent duplicates share one signing (singleflight).
type authDeduper struct {
	sign func(context.Context, *Event) error

	mu       sync.Mutex
	seen     map[string]authSigned
	inflight map[string]*authCall
}

func newAuthDeduper(sign func(context.Context, *Event) error) *authDeduper {
	return &authDeduper{
		sign:     sign,
		seen:     make(map[string]authSigned),
		inflight: make(map[string]*authCall),
	}
}

// authKey identifies the challenge a NIP-42 AUTH event answers: the relay tag
// plus the challenge tag. Events without a challenge are never cached.
func authKey(ev *Event) (string, bool) {
	var relay, challenge string
	for _, tag := range ev.Tags {
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case "relay":
			if relay == "" {
				relay = tag[1]
			}
		case "challenge":
			if challenge == "" {
				challenge = tag[1]
			}
		}
	}
	if challenge == "" {
		return "", false
	}
	return relay + "\x00" + challenge, true
}

// handle signs ev, reusing an identical recent signature when one exists.
func (d *authDeduper) handle(ctx context.Context, ev *Event) error {
	key, ok := authKey(ev)
	if !ok {
		return d.sign(ctx, ev)
	}

	d.mu.Lock()
	if s, hit := d.seen[key]; hit && time.Since(s.at) < authCacheTTL {
		d.mu.Unlock()
		*ev = s.event
		return nil
	}
	if c, ok := d.inflight[key]; ok {
		d.mu.Unlock()
		select {
		case <-c.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		if c.err != nil {
			return c.err
		}
		*ev = c.event
		return nil
	}
	c := &authCall{done: make(chan struct{})}
	d.inflight[key] = c
	d.mu.Unlock()

	err := d.sign(ctx, ev)

	d.mu.Lock()
	if err == nil {
		c.event = *ev
		if len(d.seen) >= authCacheMax {
			cutoff := time.Now().Add(-authCacheTTL)
			for k, v := range d.seen {
				if v.at.Before(cutoff) {
					delete(d.seen, k)
				}
			}
			if len(d.seen) >= authCacheMax {
				d.seen = make(map[string]authSigned)
			}
		}
		d.seen[key] = authSigned{event: *ev, at: time.Now()}
	}
	c.err = err
	delete(d.inflight, key)
	d.mu.Unlock()
	close(c.done)
	return err
}
