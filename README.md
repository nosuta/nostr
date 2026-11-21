# nostr

A Go **client** library for the Nostr protocol.

This is a fork of [go-nostr](https://github.com/nbd-wtf/go-nostr) trimmed to the
client side: relay/server components (the Khatru relay framework, eventstore
backends, relay schema and management APIs) have been removed. The module path
is `github.com/nosuta/nostr`.

## Installation

```sh
go get github.com/nosuta/nostr
```

## Components

- **keyer**: Key and bunker management utilities
- NIP-specific libraries with helpers and other things for many NIPs and related stuff, including blossom, negentropy and cashu mini-libraries.

## Development

See `FORK_MAINTENANCE.md` for the intentional divergences from upstream, the
build-tag map, and the procedure for merging upstream and publishing a release.
