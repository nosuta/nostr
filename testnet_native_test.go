//go:build !js

package nostr

import "testing"

// skipIfNoLocalRelay is a no-op outside wasm: native runners can use real
// loopback listeners for the local relay tests.
func skipIfNoLocalRelay(t *testing.T) {}
