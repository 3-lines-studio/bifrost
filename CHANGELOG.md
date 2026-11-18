# Changelog

Notable changes, newest first. Releases are tagged `vMAJOR.MINOR.PATCH`.

## Unreleased

### Native pages

- A page whose view takes no props renders once: the generated server keeps the HTML of `seo.Body` and `web.Shell` in a `sync.OnceValue`, so a static Go page costs no renderer time after the first request.
- `NativePage` carries `Keywords`, `Type`, and `Alternates`: the subject a search engine reads, the `og:type` of the card ("website" when the page has a title), and the same page in other languages as `link rel=alternate` elements. A `Schema` that is a JSON array becomes one script per value.
- `Route.WithSitemap` lists the paths a pattern cannot name, which is how a blog lists its posts. A Go page returns them from `Sitemap()`, and the generated server passes them on.
- A load that answers 404 shows the page of `not-found.go` above it, instead of the error page, and keeps `Error()` for everything else.
- `Route.WithError` gives a Native route a page for a loader that fails, with the status the error wraps or a 500, never cached. A redirect still redirects and a response hook still answers first.
- `NativePage` carries `Image`, `Robots`, `Schema`, `Lang`, and `Dir`: `og:image` with a `summary_large_image` card, the crawler directives, a JSON-LD value written with its `<` escaped, and document attributes the page sets for itself.
- `Config.SiteURL` makes `Register` serve `/sitemap.xml` with every route whose pattern has no parameter, and the paths a route adds with `WithSitemap`, and `/robots.txt` pointing at it, both with an ETag. `WithNoIndex` keeps a route out of the sitemap; the generated App Router reads the address from `BIFROST_SITE_URL`.

- A Go page can export `Screen`, its view for the MyGo app, typed by an interface the page declares: `bifrost build` writes `zz_bifrost_routes.go` at the root with `func appPages(app *app) []appPage`, so the app's router comes from the same tree as the server, checked by the compiler.
- The App Router reads `View` in `page.go` as a Go page, next to `page.tsx` for React: a directory has one or the other, and both is an error. The generated server composes `seo.Body` and `web.Shell` from `View`, `Metadata`, `Links`, and `Props`, so the page imports only `ui` and the WebAssembly build carries no server code. It keeps `Load` and `Cache`, and a tree of only Go pages compiles without Bun or Vite. `bifrost dev` rebuilds and restarts a tree of only Go pages when `app/` changes, without Bun or Vite, and `bifrost routes` lists them.

- `bifrost.Native` declares a page rendered in process by a Go function: no view file, no Vite, no Bun, no assets. Bifrost composes the document and keeps the loader, props, limits, middleware, hooks, error handling, and `WithCache` behavior of server routes. An app of only Native routes builds with `go build` and never runs `bifrost build`. Native routes share an app with JS routes, and the runtime merges their handlers, so a page moves from React to Go one route at a time.

### MyGo

- `mygo/client` sends a `Blob` where it is (memory, a range of a file of the disk or a `File` of the browser) and reports the bytes as they go out with `OnUpload`; `client.Copy` writes an answer to a writer as it arrives. In a browser a request with a blob or with progress goes with `XMLHttpRequest`, which takes a `File` without a copy and reports its upload.
- `mygo/file` picks files in a window's dialog or in the browser's picker, opens one by path, reads a range as a request body, and saves a download. `mygo/exif` reads the size, orientation, camera, lens, exposure, time and place of a JPEG, PNG or WebP from its headers, without decoding the picture.
- `mygo/data` caches the values a screen reads: one load per key on a goroutine of its own, `Loading`, `Ready` and `Failed` states, invalidation, refresh, optimistic writes, and changes to apply in `Window.Update`.
- `mygo/upload` sends a file in parts to a server that signs them: concurrency, retries with backoff, part size, progress, and resume from a `Store` after a failure or a reload. An app implements `Signer` on its own API.
- `mygo/qr` encodes QR codes at four levels and every version, draws them in a view, and writes them as PNG and SVG, with no dependency. It reads back in `zbar` at every level, from version 1 to 36.
- `mygo/auth` builds the authorization code flow of OAuth 2.0 with PKCE (`Flow.Start`, `Flow.Verify`), opens the browser, and waits for the callback on a loopback address in a window.
- `mygo/web.Static` serves a directory of files with the cache header a deploy wants, and `seo.Options.Locale` renders a page in the language of the request.
- The `mygo` module renders a MyGo native UI view to HTML for `Native` pages: `seo.Body` reads the accessibility tree of MyGo's headless tester, and `web.Shell` boots the WebAssembly build over that HTML at the page the URL names. It is a module of its own, so Bifrost keeps no MyGo dependency.
- `mygo/demo` is the app that exercises all of it, and the end-to-end test of `bifrost`: SEO metadata, a blog of pages a loader names, a layout tree, an error page, a not-found page, a sign-in flow on `auth.Session`, and a server binary that carries the browser build.
- `mygo/client` calls the server with `fetch` in a browser and `net/http` elsewhere: the wasm build drops about 6.9 MB, from 21.0 MB to 14.1 MB. A call made on the goroutine the app draws on would wait for an answer that arrives on that same goroutine, so it answers `client.ErrUIThread` instead of deadlocking the build.
- `mygo/auth` keeps the bearer token in the platform's secret store, or in localStorage in a browser. `auth.Session` holds it, stamps it on every request, and renews it once when the server answers 401 through a `Refresher`, signing out when it cannot.
- `seo.Link` draws the link `ui.Link` draws and records where it goes, so the HTML a crawler reads carries its URL: the accessibility tree holds a link's name, not its target.
- `web.Shell` dresses the content it serves until the app draws: a readable measure, system fonts and the page's headings, instead of a column of unstyled text.
- `web.Reload` makes an open page follow a dev server: an `EventSource` on `web.ReloadPath`, answered by `web.ReloadHandler`, reloads the tab when another process answers. A build leaves it out.
- `web.FileServerFS` serves a browser build from any `fs.FS`, embedded with `//go:embed` or on disk, and serves the compressed file a build leaves beside it. The compression of a build no request has asked for yet happens in the background at startup.
- `web.FileServer` prefers brotli over gzip, 3.1 MB from 13.8 MB, and reads the file again when it changes, so a development rebuild needs no restart.
- The spinner shows 400 ms after the navigation starts, so a cached build never flashes it, and it has no text to translate.

### App Router

- The Go page metadata is `seo.Head`, a type instead of a struct the page had to name, with `Title`, `Description`, `Canonical`, `Image`, `Robots`, `Schema`, `Lang`, and `Dir`.
- `layout.go` wraps the pages of its directory and the ones below it on the server and in the app; `not-found.go` answers the paths no page holds with a 404; `error.go` answers a page whose `Load` failed, with the status the error wraps or a 500, `noindex` and uncached. A `page.go` with `Screen` and no `View` is a screen of the app: the server answers the shell with `noindex` and leaves it out of the sitemap.
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
- `init --native` scaffolds a MyGo app of Go pages: no Bun, no Vite, no React. `app/page.go`, `layout.go`, `not-found.go`, and a second public page, with the `mygo.json` the app needs.
- `bifrost build` compiles the browser build to `dist/web` beside the server, and the generated main serves it from `BIFROST_WASM_DIR`, or `dist/web`. `bifrost dev` rebuilds the wasm beside the server when a Go file changes.
- `bifrost.Serve` defines no flags: it listens on `BIFROST_ADDR`, so a program parses its own flags without a late `-addr` registration failing the parse.

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
