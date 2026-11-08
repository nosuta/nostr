package checks

import (
	"github.com/nosuta/nostr/eventstore"
	"github.com/nosuta/nostr/eventstore/bleve"
	"github.com/nosuta/nostr/eventstore/boltdb"
	"github.com/nosuta/nostr/eventstore/lmdb"
	"github.com/nosuta/nostr/eventstore/mmm"
)

// compile-time checks to ensure all backends implement Store
var (
	_ eventstore.Store = (*lmdb.LMDBBackend)(nil)
	_ eventstore.Store = (*mmm.IndexingLayer)(nil)
	_ eventstore.Store = (*boltdb.BoltBackend)(nil)
	_ eventstore.Store = (*bleve.BleveBackend)(nil)
)
