package nostr

import "context"

// withContext pins a pool option set to a test's lifetime.
func withContext(ctx context.Context, opts PoolOptions) PoolOptions {
	opts.Context = ctx
	return opts
}

// authKeyOptions builds the AUTH handler a local key would provide.
func authKeyOptions(key SecretKey) PoolOptions {
	return PoolOptions{AuthRequiredHandler: func(_ context.Context, ev *Event) error {
		return ev.Sign(key)
	}}
}

// authSignerOptions builds the AUTH handler an external signer would provide.
func authSignerOptions(signer Signer) PoolOptions {
	return PoolOptions{AuthRequiredHandler: signer.SignEvent}
}
