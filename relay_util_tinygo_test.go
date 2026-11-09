//go:build tinygo

package nostr

import (
	"context"
	"testing"
)

func mustRelayConnect(t *testing.T, url string) *Relay {
	t.Helper()
	rl, err := RelayConnect(context.Background(), url, RelayOptions{})
	if err != nil {
		// these tests target public relays; skip instead of failing when one
		// is unreachable or the network is unavailable.
		t.Skipf("relay %s unavailable: %v", url, err)
	}

	return rl
}
