package nostr

import (
	"context"
	"sync"
)

// FetchManyFilters opens one one-shot fetch covering several filters. Filters
// that differ only by their author list are merged into a single request, so a
// caller syncing N single-author scopes pays one relay round-trip instead of
// N; filters that cannot be merged (different kinds or tags) each get their
// own request and the results are fanned into the returned channel.
func (pool *Pool) FetchManyFilters(ctx context.Context, urls []string, filters []Filter, opts SubscriptionOptions) chan RelayEvent {
	switch len(filters) {
	case 0:
		out := make(chan RelayEvent)
		close(out)
		return out
	case 1:
		return pool.FetchMany(ctx, urls, filters[0], opts)
	}

	groups := mergeFilters(filters)
	if len(groups) == 1 {
		return pool.FetchMany(ctx, urls, groups[0], opts)
	}

	out := make(chan RelayEvent)
	go func() {
		defer close(out)
		var wg sync.WaitGroup
		for _, group := range groups {
			wg.Add(1)
			go func(filter Filter) {
				defer wg.Done()
				for evt := range pool.FetchMany(ctx, urls, filter, opts) {
					select {
					case out <- evt:
					case <-ctx.Done():
						return
					}
				}
			}(group)
		}
		wg.Wait()
	}()
	return out
}

// mergeFilters folds filters that are identical except for their author list
// into one filter whose authors are the union. Each returned filter needs at
// most one relay request.
func mergeFilters(filters []Filter) []Filter {
	groups := make([]Filter, 0, len(filters))
	for _, filter := range filters {
		merged := false
		for i := range groups {
			if sameFilterExceptAuthors(groups[i], filter) {
				groups[i].Authors = appendUniquePubKeys(groups[i].Authors, filter.Authors)
				merged = true
				break
			}
		}
		if !merged {
			groups = append(groups, filter)
		}
	}
	return groups
}

// sameFilterExceptAuthors reports whether two filters select the same events
// apart from the authors they name. Limit is compared explicitly because the
// upstream FilterEqual ignores it.
func sameFilterExceptAuthors(a, b Filter) bool {
	if a.Limit != b.Limit {
		return false
	}
	a.Authors = nil
	b.Authors = nil
	return FilterEqual(a, b)
}

func appendUniquePubKeys(dst, src []PubKey) []PubKey {
	// Build a fresh slice so the caller's backing array is never written past
	// its length by a merge.
	out := make([]PubKey, 0, len(dst)+len(src))
	out = append(out, dst...)
	for _, pk := range src {
		seen := false
		for _, existing := range out {
			if existing == pk {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, pk)
		}
	}
	return out
}
