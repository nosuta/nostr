package nip19

import (
	"github.com/nosuta/nostr"
)

func NeventFromRelayEvent(ie nostr.RelayEvent) string {
	return EncodeNevent(ie.ID, []string{ie.Relay.URL}, ie.PubKey)
}
