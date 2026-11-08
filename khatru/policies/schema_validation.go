package policies

import (
	"context"

	"github.com/nosuta/nostr"
	"github.com/nosuta/nostr/schema"
)

func ValidateAgainstSchema(v schema.Validator) func(ctx context.Context, evt nostr.Event) (bool, string) {
	return func(ctx context.Context, evt nostr.Event) (bool, string) {
		err := v.ValidateEvent(evt)
		if err != nil {
			return true, err.Error()
		}
		return false, ""
	}
}
