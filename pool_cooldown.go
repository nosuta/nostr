package nostr

import "time"

// relayCooldownTTL is how long a relay that failed to connect is skipped before
// it is tried again. It keeps one dead relay (an outage, or one this network
// cannot reach) from paying the dial timeout on every fetch and publish.
const relayCooldownTTL = 60 * time.Second

// CooldownStore persists relay connect-failure cooldowns across runs. A relay
// URL maps to the unix second its cooldown ends.
type CooldownStore interface {
	// LoadCooldowns returns the persisted cooldowns (possibly empty).
	LoadCooldowns() map[string]int64
	// SaveCooldowns replaces the persisted set. It is called when a cooldown is
	// set or cleared, so implementations should be cheap.
	SaveCooldowns(map[string]int64)
}

// usableRelays drops relays still serving a connect-failure cooldown. When
// every relay is cooling down the full set is returned, so an operation never
// silently targets nothing.
func (pool *Pool) usableRelays(urls []string) []string {
	pool.penMu.Lock()
	defer pool.penMu.Unlock()
	if len(pool.pen) == 0 {
		return urls
	}
	now := time.Now()
	out := make([]string, 0, len(urls))
	for _, url := range urls {
		nm := NormalizeURL(url)
		if until, ok := pool.pen[nm]; ok {
			if until.After(now) {
				continue
			}
			delete(pool.pen, nm)
		}
		out = append(out, url)
	}
	if len(out) == 0 {
		return urls
	}
	return out
}

// penalize cools a relay down after it failed to connect.
func (pool *Pool) penalize(url string) {
	if url == "" {
		return
	}
	pool.penMu.Lock()
	nm := NormalizeURL(url)
	until := time.Now().Add(pool.cooldownTTL)
	// Never shorten a cooldown that is already running (a persisted one may
	// outlast this attempt).
	if existing, ok := pool.pen[nm]; ok && existing.After(until) {
		until = existing
	}
	pool.pen[nm] = until
	pool.saveCooldownsLocked()
	pool.penMu.Unlock()
}

// clearPenalty forgets a relay's cooldown after a successful connection.
func (pool *Pool) clearPenalty(url string) {
	if url == "" {
		return
	}
	pool.penMu.Lock()
	nm := NormalizeURL(url)
	if _, ok := pool.pen[nm]; ok {
		delete(pool.pen, nm)
		pool.saveCooldownsLocked()
	}
	pool.penMu.Unlock()
}

// saveCooldownsLocked persists the currently-active cooldowns. Callers hold
// penMu.
func (pool *Pool) saveCooldownsLocked() {
	if pool.cooldowns == nil {
		return
	}
	now := time.Now()
	out := make(map[string]int64, len(pool.pen))
	for url, until := range pool.pen {
		if until.After(now) {
			out[url] = until.Unix()
		}
	}
	pool.cooldowns.SaveCooldowns(out)
}

// settle updates the cooldown after a one-shot fetch: a relay that is not
// connected once the fetch ended failed to be reached; a connected one is
// cleared.
func (pool *Pool) settle(urls []string) {
	if len(urls) == 0 {
		return
	}
	connected := pool.ConnectedRelays(urls)
	for _, url := range urls {
		if connected[url] {
			pool.clearPenalty(url)
			continue
		}
		pool.penalize(url)
	}
}

// Warm dials the given relays in the background so the first user request does
// not pay the connection cost. It never blocks and ignores per-relay errors; a
// relay that fails is cooled down like any other connect failure.
func (pool *Pool) Warm(urls []string) {
	for _, url := range pool.usableRelays(urls) {
		if url == "" {
			continue
		}
		go func(url string) {
			if _, err := pool.EnsureRelay(url); err != nil {
				pool.penalize(url)
				return
			}
			pool.clearPenalty(url)
		}(url)
	}
}

// ConnectedRelays reports, for each URL, whether the pool currently holds a
// live connection to it. URLs the pool has never dialled (or has since
// dropped) report false. It never dials: a status query must not trigger
// network I/O, so it only inspects the pool's existing relay table.
func (pool *Pool) ConnectedRelays(urls []string) map[string]bool {
	out := make(map[string]bool, len(urls))
	for _, url := range urls {
		relay, ok := pool.Relays.Load(NormalizeURL(url))
		out[url] = ok && relay != nil && relay.IsConnected()
	}
	return out
}
