# Fork maintenance

This repository is a client-only fork of `go-nostr`. It is published as
`github.com/nosuta/nostr` and consumed by `concord-go` (root, `av`, and
`gui/go` modules). The fork intentionally diverges from upstream in a few
places; this document records what those divergences are and how to re-apply
them after merging upstream.

## Intentional divergences

| Area | Change | Why |
| --- | --- | --- |
| Module path | `fiatjaf.com/nostr` / `github.com/nbd-wtf/go-nostr` -> `github.com/nosuta/nostr` | All imports in this fork and its consumers use the new path. |
| Relay/server code | Removed `khatru`, `eventstore`, `schema`, `sdk`, `nip86` | Client-only library; less code for TinyGo to compile. |
| Relay pool | `nostrx` (AUTH dedup, cooldowns, warm-up, batched fetch) folded into the core `nostr` package | Single frontend entry point; no separate wrapper module. |
| Blossom | I/O-free types/BUD-11 helpers only; fasthttp client removed | fasthttp does not compile under TinyGo/wasm. |
| JSON/crypto/HTTP | `tinygo` and `js` build-tag variants for easyjson-free JSON, key handling, and `wasm-fetch` | Upstream targets standard Go/native only. |
| Relay transport | `relay_js.go` (`//go:build js`) for browser WebSockets | `relay.go` is `//go:build !js`. |

Concord-side code that used to live in `pkg/nostrx` and `pkg/blossom` now lives
here. Do not reintroduce those packages in `concord-go`.

## Build-tag map

| Tag | Files | Purpose |
| --- | --- | --- |
| `tinygo` | `*_tinygo.go`, `nip05/*_tinygo.go`, `nip11/*`, `nip46/*`, `nip59/*`, `nip77/*` | TinyGo-safe JSON, crypto, fetch, and keyer implementations. |
| `!tinygo` | `*_easyjson.go`, `*_std.go`, `helpers.go`, ... | Standard Go implementations using easyjson/net/http. |
| `js` | `relay_js.go`, `relay_js_test.go`, `testnet_js_test.go` | Browser WebSocket transport and wasm test skips. |
| `!js` | `relay.go`, `relay_test.go`, `testnet_native_test.go` | Native network transport and local-relay tests. |
| `libsecp256k1` | `signature_libsecp256k1.go` | Optional native CGo signing backend; never enabled on Web. |

When adding a new feature, check whether each build configuration still
compiles: native, Go js/wasm, and TinyGo js/wasm. Prefer a new build-tagged file
over editing an untagged upstream file: it keeps upstream merges small.

## Merging upstream

1. Work in a topic branch. Fetch upstream (`git fetch upstream` or the
   `gitnostr.com` git mirror; the `nostr://` remote is not a git transport for
   `git fetch`) and merge/rebase onto it.
2. Resolve conflicts by keeping upstream code for untagged files unless the
   change is one of the intentional divergences above. Re-apply the divergences
   as small, tagged patches where possible.
3. Re-run the import rewrite: every Go import of `fiatjaf.com/nostr` or
   `github.com/nbd-wtf/go-nostr` must become `github.com/nosuta/nostr`.
   `git grep -n 'fiatjaf.com/nostr\|github.com/nbd-wtf/go-nostr' -- '*.go'`
   must be empty. Upstream also rewrites its own imports, so repeat this after
   every merge.
4. Verify (all four must pass):

   ```sh
   gofmt -l .                         # must print nothing
   go build ./... && go vet ./...
   go test ./...
   # Go js/wasm, using wasmbrowsertest as go_js_wasm_exec + Chrome:
   GOOS=js GOARCH=wasm go test -short ./...
   # TinyGo js/wasm: full compile of every package plus runtime tests:
   GOOS=js GOARCH=wasm tinygo test -run '^$' ./...
   GOOS=js GOARCH=wasm tinygo test -short ./...
   ```

   A locally installed `go_js_wasm_exec` compiled against an older Go may look
   for a stale `GOROOT`; set `GOROOT=$(go env GOROOT)` when running the Go
   js/wasm command in that case.
5. Tag the result (`git tag -a vX.Y.Z -m ...`), push the commit and tag to
   `origin`, then bump the dependency in all three `concord-go` modules with
   `go get github.com/nosuta/nostr@vX.Y.Z` and commit the updated `go.work.sum`.

## Web/TinyGo runtime constraints

- TinyGo's wasm target cannot recover panics. Any `syscall/js` call that throws
  traps the whole web worker, so browser APIs must be called only with valid
  arguments. `relay_js.go` clamps close codes to 1000/3000-4999 and truncates
  close reasons to 123 bytes for this reason.
- `coder/websocket` itself calls `WebSocket.close(1011, ...)` from inside its
  JavaScript error listener and waits there for the close event. The
  `WebSocket.prototype.close` wrapper in godash's `worker.js` template must stay:
  it both avoids the invalid-code exception and delivers a synthetic close
  event so the Go side cannot deadlock. The nostr-side clamp removes our own
  1006 call sites but cannot remove that dependency-side path.
- Live network tests are skipped in `-short` mode (`nip11.TestFetch`). Local
  relay tests are skipped under `//go:build js`; browsers cannot provide the
  raw loopback WebSocket listeners they need.
- TinyGo/wasm schedules cooperatively. A tight CPU loop never yields, so
  timer-based cancellation cannot fire and `nip13.TestDoWorkTimeout` spins
  forever; it is skipped under `//go:build tinygo` only. Standard Go wasm keeps
  running it.
- Node-based `tinygo test` runner issues seen with Node 26 (undici
  `markResourceTiming`, `net.Listen` on `::1`) are runner limitations, not
  library failures; CI runs the same commands on Ubuntu.
