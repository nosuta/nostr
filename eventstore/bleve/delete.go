package bleve

import (
	"github.com/nosuta/nostr"
)

func (b *BleveBackend) DeleteEvent(id nostr.ID) error {
	return b.index.Delete(id.Hex())
}
