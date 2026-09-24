# Changelog

Notable changes. Releases are tagged `vMAJOR.MINOR.PATCH`; this file covers what landed after the last tag.

## Unreleased

### App Router

- File-based routing under `app/`, with the `page`, `layout`, `template`, `loading`, `error` and `not-found` conventions, route groups, and nested layout discovery.
- Generated routes inject `params`, `searchParams` and `pathname`, render `generateMetadata`, and accept default exports in views.
- Client-side navigation through `Route.WithNavigation()`: `<Link>`, `navigate`, `replace`, `refresh` and the navigation hooks from `virtual:bifrost/navigation`, with shared layouts and scroll, hash, focus, head and document-attribute handling.
- `Head` is only valid in `page.tsx`. Exporting it from a layout, template, error, not-found or loading view is a build error.
- A route can declare the `Cache-Control` of its document: `Route.WithCache` in Go, or a `Cache` export next to `Load` for an App Router page. Rendered documents stay `no-store` by default and the navigation response is never cacheable.
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

### Development server

- The Vite bridge survives Go rebuilds.
- Client entries are warmed before the bridge reports healthy.
- Stale development locks, runtime directories and sockets are pruned on start.

### Checks

- `make check` also runs the App Router demo.
- `make integration` runs the navigation browser checks, and the integration scripts take their ports from `BIFROST_TEST_PORT`.
- `make throughput` drives an app through the real renderer and reports throughput, p50, p95, p99 and the 503 count per concurrency level.
