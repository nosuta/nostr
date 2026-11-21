//go:build js

package nostr

import "testing"

// skipIfNoLocalRelay skips tests that spin up a local TCP/WebSocket relay.
// A wasm test runner cannot listen on or dial arbitrary local sockets the way
// these tests need, even when it proxies HTTP through the host browser.
func skipIfNoLocalRelay(t *testing.T) {
	t.Helper()
	t.Skip("local TCP relay tests require a native network stack")
}
