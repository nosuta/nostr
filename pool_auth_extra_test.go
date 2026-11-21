package nostr

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingAuthSigner counts how many times it was asked to sign, so a test can
// prove the deduper reused a signature instead of calling the signer again.
type countingAuthSigner struct {
	sk    SecretKey
	calls int32
}

func (s *countingAuthSigner) sign(_ context.Context, ev *Event) error {
	atomic.AddInt32(&s.calls, 1)
	return ev.Sign(s.sk)
}

func authEvent(challenge string) *Event {
	return &Event{
		Kind:      KindClientAuthentication,
		CreatedAt: Timestamp(time.Now().Unix()),
		Tags: Tags{
			{"relay", "wss://relay.example"},
			{"challenge", challenge},
		},
	}
}

func TestAuthDeduperReusesSameChallenge(t *testing.T) {
	signer := &countingAuthSigner{sk: SecretKey{7}}
	d := newAuthDeduper(signer.sign)

	for i := 0; i < 3; i++ {
		ev := authEvent("abc")
		if err := d.handle(context.Background(), ev); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if ev.ID == (ID{}) || !ev.VerifySignature() {
			t.Fatalf("call %d produced an invalid signature", i)
		}
	}
	if got := atomic.LoadInt32(&signer.calls); got != 1 {
		t.Fatalf("signer calls = %d, want 1 (deduped)", got)
	}
}

func TestAuthDeduperDistinctChallengesSignSeparately(t *testing.T) {
	signer := &countingAuthSigner{sk: SecretKey{7}}
	d := newAuthDeduper(signer.sign)

	for _, challenge := range []string{"a", "b", "a"} {
		if err := d.handle(context.Background(), authEvent(challenge)); err != nil {
			t.Fatalf("handle: %v", err)
		}
	}
	if got := atomic.LoadInt32(&signer.calls); got != 2 {
		t.Fatalf("signer calls = %d, want 2 (a and b)", got)
	}
}

func TestAuthDeduperNoChallengeSignsEveryTime(t *testing.T) {
	signer := &countingAuthSigner{sk: SecretKey{7}}
	d := newAuthDeduper(signer.sign)

	for i := 0; i < 2; i++ {
		ev := &Event{
			Kind:      KindClientAuthentication,
			CreatedAt: Timestamp(time.Now().Unix()),
			Tags:      Tags{{"relay", "wss://relay.example"}},
		}
		if err := d.handle(context.Background(), ev); err != nil {
			t.Fatalf("handle: %v", err)
		}
	}
	if got := atomic.LoadInt32(&signer.calls); got != 2 {
		t.Fatalf("signer calls = %d, want 2 (no challenge to key on)", got)
	}
}

func TestAuthDeduperConcurrentSameChallengeSignsOnce(t *testing.T) {
	signer := &countingAuthSigner{sk: SecretKey{7}}
	d := newAuthDeduper(signer.sign)

	const n = 8
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev := authEvent("abc")
			if err := d.handle(context.Background(), ev); err != nil {
				t.Errorf("handle: %v", err)
				return
			}
			if !ev.VerifySignature() {
				t.Errorf("concurrent call produced an invalid signature")
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&signer.calls); got != 1 {
		t.Fatalf("signer calls = %d, want 1 (singleflight)", got)
	}
}
