//go:build !tinygo

package nip59

import (
	"github.com/mailru/easyjson"
	"github.com/nosuta/nostr"
)

func unmarshalEvent(data []byte, evt *nostr.Event) error {
	return easyjson.Unmarshal(data, evt)
}
