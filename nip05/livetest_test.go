package nip05

import (
	"os"
	"testing"
)

// skipUnlessLive skips tests that depend on external DNS/HTTP data. Set
// NOSTR_LIVE_TESTS=1 to run them.
func skipUnlessLive(t *testing.T) {
	t.Helper()
	if os.Getenv("NOSTR_LIVE_TESTS") == "" {
		t.Skip("set NOSTR_LIVE_TESTS=1 to run tests that require external nip05 endpoints")
	}
}
