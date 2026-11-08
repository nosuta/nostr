package test

import (
	"testing"

	"github.com/nosuta/nostr/sdk/hints/memoryh"
)

func TestMemoryHints(t *testing.T) {
	runTestWith(t, memoryh.NewHintDB())
}
