//go:build tinygo

package nip59

import (
	"encoding/json"

	"github.com/nosuta/nostr"
)

func unmarshalEvent(data []byte, evt *nostr.Event) error {
	return json.Unmarshal(data, evt)
}
