package nostr

import "context"

// operation derives a per-call context that is also cancelled when the pool
// closes, so no forwarding goroutine outlives its pool.
func (pool *Pool) operation(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(pool.Context, cancel)
	if pool.Context.Err() != nil {
		cancel()
	}
	return ctx, func() {
		stop()
		cancel()
	}
}

// forward copies a relay-event channel, stopping when either the source ends
// or the derived context is cancelled.
func forward[T any](ctx context.Context, cancel context.CancelFunc, source <-chan T) chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case value, ok := <-source:
				if !ok {
					return
				}
				select {
				case out <- value:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

// forwardFetch forwards a one-shot fetch and records which relays could not be
// reached once it ends, so they are skipped for the cooldown.
func (pool *Pool) forwardFetch(ctx context.Context, cancel context.CancelFunc, urls []string, source <-chan RelayEvent) chan RelayEvent {
	out := make(chan RelayEvent)
	go func() {
		defer close(out)
		defer cancel()
		defer pool.settle(urls)
		for {
			select {
			case <-ctx.Done():
				return
			case value, ok := <-source:
				if !ok {
					return
				}
				select {
				case out <- value:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

// forwardPublishes forwards publish results, cooling a relay down when it could
// not be reached (Relay is nil) and clearing it when the relay responded.
func (pool *Pool) forwardPublishes(ctx context.Context, cancel context.CancelFunc, source <-chan PublishResult) chan PublishResult {
	out := make(chan PublishResult)
	go func() {
		defer close(out)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case value, ok := <-source:
				if !ok {
					return
				}
				if value.Relay == nil {
					pool.penalize(value.RelayURL)
				} else {
					pool.clearPenalty(value.RelayURL)
				}
				select {
				case out <- value:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}
