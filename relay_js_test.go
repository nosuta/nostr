//go:build js

package nostr

import (
	"strings"
	"testing"
	"unicode/utf8"

	ws "github.com/coder/websocket"
)

// Browsers reject close codes outside 1000 and 3000-4999. The native relay
// uses 1006 for abnormal closures, so the js transport has to translate it
// before calling WebSocket.close().
func TestBrowserCloseCode(t *testing.T) {
	tests := []struct {
		in   ws.StatusCode
		want ws.StatusCode
	}{
		{0, ws.StatusNormalClosure},
		{ws.StatusNormalClosure, ws.StatusNormalClosure},
		{ws.StatusGoingAway, ws.StatusNormalClosure},
		{ws.StatusAbnormalClosure, ws.StatusNormalClosure},
		{ws.StatusInternalError, ws.StatusNormalClosure},
		{2999, ws.StatusNormalClosure},
		{3000, 3000},
		{3999, 3999},
		{4999, 4999},
		{5000, ws.StatusNormalClosure},
	}

	for _, tt := range tests {
		if got := browserCloseCode(tt.in); got != tt.want {
			t.Errorf("browserCloseCode(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// A close reason longer than 123 bytes throws InvalidAccessError in browsers.
// Truncation must not split a UTF-8 sequence.
func TestBrowserCloseReason(t *testing.T) {
	if got := browserCloseReason(""); got != "" {
		t.Errorf("empty reason = %q", got)
	}

	exact := strings.Repeat("a", 123)
	if got := browserCloseReason(exact); got != exact {
		t.Errorf("123-byte reason changed: got %d bytes", len(got))
	}

	long := browserCloseReason(strings.Repeat("a", 200))
	if len(long) != 123 {
		t.Errorf("long reason = %d bytes, want 123", len(long))
	}

	multibyte := browserCloseReason(strings.Repeat("あ", 50))
	if len(multibyte) > 123 || !utf8.ValidString(multibyte) {
		t.Errorf("multibyte reason = %d bytes, valid utf8 = %v", len(multibyte), utf8.ValidString(multibyte))
	}
}
