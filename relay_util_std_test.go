//go:build !tinygo

package nostr

import (
	"testing"
)

func mustRelayConnect(t *testing.T, url string) *Relay {
	t.Helper()
	rl, err := RelayConnect(t.Context(), url, RelayOptions{})
	if err != nil {
		// these tests target public relays; skip instead of failing when one
		// is unreachable or the network is unavailable.
		t.Skipf("relay %s unavailable: %v", url, err)
	}

	return rl
}
