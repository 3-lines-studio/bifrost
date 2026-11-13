# Changelog

Notable changes, newest first. Releases are tagged `vMAJOR.MINOR.PATCH`.

## Unreleased

### App Router

- A `commands` package in the module root turns the app binary into a
  multi-command one: `./app migrate` runs the command and exits before the app
  is built, and no arguments start the server. One artifact for the server, the
  cron jobs and the migrator, instead of a second binary that ships the same
  modules again.

### Build

- `bifrost build` uses the host Bun by default: the App Router binary is 9.4 MB instead of 118 MB, and the runtime runs the renderer script with `bun` from `BIFROST_BUN` or `PATH`. `--embed-bun` compiles Bun into the binary for self-contained deploys; `--host-bun` stays accepted and defaults to true.
- App Router binaries build with `-trimpath -buildvcs=false -ldflags=-s -w`, which drops the artifact from 13.5 MB to 9.4 MB.

### Runtime

- Renderer admission is bounded by time instead of an instant reject: a request waits up to two seconds for capacity and then fails with 503 and `Retry-After`. Under a burst the old queue returned 503 for 98% of requests while workers were free microseconds later.
- The missing-Bun error names `BIFROST_BUN`, the Bun install, and `--embed-bun`.

### Performance

- The renderer no longer speaks HTTP. Each worker serves length-prefixed frames over its Unix socket: one length-prefixed JSON request, then head, body, done and error frames. The transport costs 1.7 us instead of 10.4 us per render in Go, and Bun answers in 3.2 us instead of 11.6 us. Two workers went from 41k to 60k requests per second, p99 at one connection from 130 us to 116 us, and p50 from 42 us to 32 us. Connections are pooled, so static prerender keeps its parallel workers.
- Props skip the second escaping pass: `json.Marshal` already escapes `<`, `>` and the JavaScript line separators, so only `RawProps` needs the three `bytes.ReplaceAll` scans. `marshalProps` drops from 1279 ns, 937 B and 24 allocs to 1141 ns, 456 B and 21 allocs.
- The renderer request frames the encoded props instead of re-marshaling them, so a 4 KB props page spends 0.6 us instead of 4.4 us on the wire payload.
- Request throughput with 4 KB props: 21.6k to 32.0k requests per second at 32 connections, and p50 at one connection drops from 75.7 us to 49.8 us.
- Go 1.27 is the minimum. Its toolchain brings the Green Tea GC, size-specialized small allocations, and `encoding/json` backed by v2 to every build.

### CLI

- `init` resolves the CLI with `go get -tool`, so `go tool bifrost` runs offline from the module graph instead of `go run` fetching it every time.
- The classic scaffold uses `bifrost.Serve`, so it gets `BIFROST_ADDR`, systemd socket activation, and `BIFROST_IDLE_TIMEOUT` like generated App Router mains.
- New `DEPLOY.md`: artifact sizes, environment variables, readiness, saturation, containers, and the bee setup.

## v1.3.11

### App Router

- The generated view re-exports the whole page module, so a page that re-exports `Head` from another file keeps its head instead of rendering an empty one.

### CLI

- `build --render-queue N` sets the queue depth of the generated App Router main, the same knob `Config.RenderQueue` has in the classic router. Zero keeps the default of 64.
- `bifrost build` uses the module in the working tree when the CLI reports a commit pseudo-version as its own version, instead of asking the module proxy for a commit that may not be published yet.
- The CLI reports the error when an app package does not load, instead of silently ignoring `Load`, `Cache`, `Serve`, `Middleware` and the HTTP methods of that package. Two packages in the same directory, which is easy to hit with a `page.go` next to `server.go` in `app/`, used to disable all of them without a word.

## v1.3.10

### App Router

- File-based routing under `app/`, with the `page`, `layout`, `template`, `loading`, `error` and `not-found` conventions, route groups, and nested layout discovery.
- Generated routes inject `params`, `searchParams` and `pathname`, render `generateMetadata`, and accept default exports in views.
- Client-side navigation through `Route.WithNavigation()`: `<Link>`, `navigate`, `replace`, `refresh` and the navigation hooks from `virtual:bifrost/navigation`, with shared layouts and scroll, hash, focus, head and document-attribute handling.
- `Head` is only valid in `page.tsx`. Exporting it from a layout, template, error, not-found or loading view is a build error.
- A route can declare the `Cache-Control` of its document: `Route.WithCache` in Go, or a `Cache` export next to `Load` for an App Router page. Rendered documents stay uncacheable by default and the navigation response is never cacheable.
- `bifrost.NotFound()` from a loader works without a `not-found.tsx` next to the route: the loader not-found and error branches keep the request props.
- `example/app-router-demo` is a self-checking demo app.

### CLI

- `bifrost routes` lists the API handlers and middleware.
- Trailing slashes redirect, matching the App Router.
- Apps with routes but no pages build.
- `init` scaffolds an App Router app by default; `--classic` keeps the classic router.
- `init` resolves the latest release with `go get`, runs `go mod tidy`, and leaves the port to `BIFROST_ADDR`.
- `build --render-concurrency N` sets the renderer worker count of the generated App Router main.

### Runtime and build

- Two renderer workers by default, about 40 MB of RSS each: one capped a Server page at half the throughput and doubled the tail latency.
- The renderer runtime ships uncompressed, and gzip support is gone.
- The runtime directory is cached per build id, shared across processes with a lock, and pruned when nothing is using it: runs no longer pile directories and sockets into `/tmp`, and a warm start is a fraction of what it was.
- The `describe` and `generate` phases no longer embed the previous build's output, so the Go build cache hits them and rebuilds skip recompiling and relinking the embedded output.
- Build temp directories are removed instead of leaking on every build.
- The production renderer caches its view modules instead of importing them per request: about 0.022 ms less work per render, measured at 2000 requests per variant.

### Development server

- The Vite bridge survives Go rebuilds.
- Client entries are warmed before the bridge reports healthy.
- Stale development locks, runtime directories and sockets are pruned on start.

### Checks

- `make check` also runs the App Router demo.
- `make integration` runs the navigation browser checks, and the integration scripts take their ports from `BIFROST_TEST_PORT`.
- `make throughput` drives an app through the real renderer and reports throughput, p50, p95, p99 and the 503 count per concurrency level.
