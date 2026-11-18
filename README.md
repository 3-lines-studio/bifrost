# Bifrost

Bifrost serves Vite-built React pages from an ordinary Go `net/http` application. Vite owns frontend builds, plugins, assets, and development HMR. Bun executes streaming React SSR. Go owns routing, loaders, HTTP, validation, and deployment. A route can also be rendered by a Go function with `bifrost.Native`, with no frontend build at all.

The App Router under `app/` is the default and recommended router: `bifrost init` scaffolds it. The classic router below is the explicit, opt-in alternative for apps that declare routes in Go.

```go
app, err := bifrost.New(bifrost.Config{
    Assets: bifrostAssets,
    Routes: []bifrost.Route{
        bifrost.Server("/{$}", "pages/home.tsx", loadHome),
        bifrost.Static("/about", "pages/about.tsx", nil),
        bifrost.Client("/app", "pages/app.tsx"),
    },
})
if err != nil {
    log.Fatal(err)
}
if bifrost.Building() {
    return
}
defer app.Close(context.Background())
if err := http.ListenAndServe(":8080", app.Handler()); err != nil {
    log.Print(err)
}
```

## Commands

```sh
go run github.com/3-lines-studio/bifrost/cmd/bifrost init ./myapp
cd myapp
go tool bifrost dev .
go tool bifrost build .
go tool bifrost routes .
go tool bifrost version
```

`init` scaffolds the App Router, resolves the module with `go get -tool`, and runs `bun install` (`--no-install` skips both); `--classic` scaffolds the classic router instead, and `--native` a MyGo app of Go pages, with no Bun, no Vite, and no React. The tool directive makes `go tool bifrost` runnable offline from the module graph.

Flags precede the package path. `build` accepts `--sourcemaps`, `--static-workers`, `--output`, `--embed-bun`, `--vite-config`, and `--render-concurrency` and `--render-queue` (the App Router takes its renderer worker count and its queue depth here). `dev` accepts `--poll`, `--vite-port` (zero picks a free port), and `--vite-config`. `routes` prints the route table (views, API handlers, and middleware) without building the app (`-C` and `--vite-config`).

`dev` runs one validated bootstrap build, then owns a Bun process hosting Vite's development server and the SSR bridge. The bridge outlives Go child restarts, so a Go edit never discards frontend module state. Browser assets are proxied through your Go origin at `/_bifrost/dev/`, so HMR, module loads, and page requests share one origin and the Vite server never has to be reachable from the browser. Server and Static pages include stylesheet links from Vite's live SSR module graph, so development pages keep their styles instead of flashing unstyled content. Go replacement is detected by a long-poll to `/_bifrost/build-id` that holds until the process is replaced, so idle development generates almost no requests.

`build` runs the app through describe and static-generation phases, asks Vite to build each unique client and SSR view, prerenders Static routes through Bun, compiles the Bun renderer only when the app embeds it, validates Vite's manifests, writes a strict Bifrost manifest, and atomically replaces `.bifrost`. The generated `zz_bifrost_gen.go` embeds `.bifrost` and provides the package-local `bifrostAssets` value used by `Config`.

An app with Go pages skips the frontend phases it does not need: a tree of only Go pages builds with `go build` alone, and then `build` also compiles the browser build of the app to `dist/web` (`app.wasm`, its gzip, and the `wasm_exec.js` of the toolchain) and copies it inside the server, so a build deploys as one file: the server answers `/app.wasm` from the copy, or from `BIFROST_WASM_DIR` when set. `dev` rebuilds that wasm beside the server when a Go file changes, serves it from disk, and makes the open page follow the server: a restart reloads the tab.

## React module contract

```tsx
export function Head(props) {
  return <title>{props.title}</title>;
}

export function Page(props) {
  return <main>{props.title}</main>;
}
```

`Page` is required and `Head` is optional. Only `page.tsx` can export `Head`: every other view file rejects it, because the renderer keeps a single head per route. Server and Static pages hydrate; client pages mount into an empty shell. Loader and generator props reach the browser, so never return secrets as props. Hydrated pages follow React Client Component rules: page components cannot be async, and streamed deferred UI uses `React.lazy` with `Suspense`.

A Server loader may return request-scoped root document attributes without putting them in React props:

```go
return bifrost.PageData{
    Props: pageProps,
    Document: bifrost.Document{Lang: "pt-BR", Class: "dark", Dir: "ltr"},
}, nil
```

`StaticPage.Document` provides the same attributes for generated pages. Bifrost validates the language, class, and direction before writing the response.

`PageData.Headers` sends headers with the page, as the cookies of a session or a `Vary`. Bifrost keeps the ones that describe the answer itself — `Content-Type`, `Content-Length`, `Cache-Control`, the framing headers of HTTP and of a proxy — to itself.

## Native pages

A route rendered by a Go function needs no React, Vite, Bun, or embedded assets. Declare it with `Native` and build the app without `Assets`:

```go
app, err := bifrost.New(bifrost.Config{
    Routes: []bifrost.Route{
        bifrost.Native("/docs/{slug}", renderDoc, loadDoc),
    },
})

func renderDoc(ctx context.Context, props json.RawMessage) (bifrost.NativePage, error) {
    return bifrost.NativePage{
        Title:       "Docs",
        Description: "What the page is about",
        Canonical:   "https://example.com/docs",
        Body:        []byte("<h1>Docs</h1>"),
    }, nil
}
```

`NativePage.Body` is HTML placed inside the document Bifrost composes: doctype, `html` attributes from `PageData.Document`, charset, viewport, title, description, and canonical link. The loader receives the request, so `request.PathValue` reads dynamic segments. Props are encoded once exactly as for Server routes, so `RawProps` and `Limits.MaxPropsBytes` apply; the body is bounded by `Limits.MaxFrameBytes` and the composed head by `Limits.MaxHeadBytes`. The document keeps the same default `Cache-Control`; `WithCache` overrides it. `ResolveMarkdown` leaves native pages unchanged.

`WithError` gives a Native route a page for a loader that fails: Bifrost renders it with the status the error wraps, or 500, and never caches it. Redirects still redirect and a response hook still answers first.

`NativePage` also carries what a search engine and a social card read: `Image` becomes `og:image` with a `summary_large_image` card, `Robots` sets the crawler directives, `Keywords` the subject, `Type` the `og:type` of the card, `Alternates` the same page in other languages, `Schema` a JSON-LD value written with its `<` escaped (an array of them becomes one script each), and `Lang` and `Dir` override the document's attributes. `WithNoIndex` keeps a route out of the sitemap.

Native routes render in process: they start no renderer and read no assets. They share an app with Server, Static, and Client routes, so a page moves from React to Go one route at a time; the JS routes still need `Assets`, and an app of only native routes runs without them. `bifrost.Building()` is never true for one. `example/native` is a complete native-only app.

For a MyGo native UI view, the `mygo` module renders it to this HTML from the accessibility tree of MyGo's headless tester: `seo.Body(view, seo.Options{Links: links})`, and `web.Shell` wraps it with a loading overlay and the script that boots the WebAssembly build over it. It is a module of its own, so Bifrost's dependencies do not change. The [App Router](#the-app-router) generates all of it from `app/page.go`.

[`mygo/demo`](mygo/demo) is the app that exercises all of it: public pages with canonical, social card, keywords, alternates and JSON-LD, a blog whose posts a loader names and a sitemap lists, layouts under layouts, an error page, a not-found page, screens the app draws alone, a sign-in flow on `auth`, and a binary that carries the browser build.

## Sitemap and robots

Set `Config.SiteURL` to the public address, as `https://example.com`, and `Register` serves `/sitemap.xml` with every route whose pattern has no parameter, and `/robots.txt` pointing at it. `WithSitemap(paths...)` lists the pages a pattern cannot name, as every post of a blog, which is what a Go page's `Sitemap() []string` returns. `WithSitemapEntries(entries...)` adds when each page last changed and the languages it comes in: a `bifrost.SitemapEntry` carries `LastMod` and `Alternates`, which become `<lastmod>` and `xhtml:link rel="alternate" hreflang` of the sitemap, and a Go page returns them from `Sitemap() []bifrost.SitemapEntry`. Both carry an ETag and an hour of cache. Routes declared `WithNoIndex` stay out of the sitemap, which is how the app's screens stay out of a search engine. The generated App Router main reads the address from `BIFROST_SITE_URL`.

## HTTP composition

Register Bifrost and ordinary handlers on one user-owned `http.ServeMux`, then wrap that mux with shared middleware:

```go
mux := http.NewServeMux()
if err := app.Register(mux); err != nil {
    log.Fatal(err)
}
mux.Handle("/", apiRouter)
handler := sharedMiddleware(app.ResolveMarkdown(mux))
```

`ResolveMarkdown` serves server-rendered routes as Markdown for a `.md` path suffix or a preferred `Accept: text/markdown` media type. It leaves static pages, client pages, public files, and other mux handlers unchanged. `Handler` applies it automatically, and so do generated App Router apps.

Use `/{$}` for an exact root page; the standard `/` pattern is a subtree fallback. Bifrost does not add router-specific adapters.

Rendered documents are `Cache-Control: private, no-cache, no-store, max-age=0, must-revalidate` by default, because their props usually come from the request. Declare a route as cacheable with `Route.WithCache` in Go or with a `Cache` export next to `Load` in the App Router, for example `public, s-maxage=60` to let a CDN hold it. The document keeps `Vary: Accept`, and the client-side navigation response stays `no-store` and never reaches a shared cache.

## Build and runtime boundary

Build phases execute the application to collect immutable declarations, so code that constructs `Config`, routes, loaders, and generators must be side-effect free. Check `bifrost.Building()` immediately after `New`, before opening listeners, databases, queues, or background workers. When declarations live in an internal package, pass the generated package-local `bifrostAssets` from `main` into that package; `example/structured` shows the layout that keeps one generated embedded tree instead of a second copied embed.

## SSR concurrency contract

Bifrost uses two isolated Bun renderer processes by default, about 40 MB of RSS each; `RenderConcurrency: N` starts N production renderers, each handling one render at a time, and `RenderConcurrency: 1` is enough for a single-core box. Development always serializes SSR through its one Vite module graph. This keeps simultaneous requests from racing through one JavaScript module graph while allowing explicit production scaling.

Module globals persist between sequential requests handled by the same worker, so keep locale, user, authentication, and request data out of module-level variables: derive them from props or request-local React context.

The App Router serves `GET /_bifrost/ready` with `app.Ready`; point health checks there instead of rendering a page. Classic apps can expose the same check themselves:

```go
if err := app.Ready(request.Context()); err != nil {
    http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
    return
}
writer.WriteHeader(http.StatusNoContent)
```

`build` uses the Bun on the host by default and writes the renderer script instead of compiling Bun into the app, which keeps the binary about 105 MB smaller; at runtime the app runs the script with `bun`, from `BIFROST_BUN` or `PATH`. `build --embed-bun` compiles Bun into the app for a self-contained binary. A supervisor can keep one renderer process alive and have app instances attach through `BIFROST_RENDERER_SOCKET`; set `BIFROST_RUNTIME_SCRIPT` so every app start publishes the script for the supervisor. One shared renderer serves one worker at a time; `render_concurrency` above 1 is capped and logged.

`bifrost.Serve` listens on `BIFROST_ADDR`, or on the systemd socket when `LISTEN_FDS=1`, and exits cleanly after `BIFROST_IDLE_TIMEOUT` with no connections. The App Router uses it automatically.

## Frontend plugins

Use normal Vite module and build plugins in `vite.config.ts`. Bifrost enforces its entry points, output roots, SSR bundling, and asset base while preserving user plugins and transforms. Bifrost owns dynamic HTML streaming, so plugins that require an HTML entry or `transformIndexHtml` are outside the current contract.

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
});
```

Tailwind then uses its normal CSS entry:

```css
@import "tailwindcss";
```

## Frontend route metadata

Bifrost resolves `virtual:bifrost/routes` to the Go route table in production builds and development, and invalidates it when routes change:

```tsx
import { href, routes } from "virtual:bifrost/routes";

export function Nav() {
  return <nav>{routes.map((route) => route.pattern)}</nav>;
}
```

`href(pattern, params)` interpolates `http.ServeMux` patterns such as `/post/{slug}` and `/files/{path...}`. `bifrost init` writes the matching `bifrost.d.ts` declarations; add them manually to other projects.

The module is read-only metadata. Explicitly wired apps own their client-side navigation. App Router apps include it automatically.

## App Router files

Bifrost follows the Next.js App Router file conventions.

The project root is the route root, or the `app` directory when it exists. A directory becomes a route when it contains `page.tsx`, and its path is the URL path.

Go cannot compile an import path containing brackets, parentheses, or `@`, so directory markers replace the bracketed forms other frameworks use:

```sh
app/posts/page.tsx            # /posts
app/posts/post-id_/page.tsx   # /posts/{post-id}
app/docs/slug__/page.tsx      # /docs/{slug...}
app/marketing~/about/page.tsx # /about
app/_components/Card.tsx      # never routed
```

One trailing `_` marks a one-segment parameter; two mark a parameter that captures the rest of the path, which must be last. A trailing `~` marks a route group, which organizes files without appearing in the URL. A leading `_` keeps a directory out of routing, and a `page.tsx` or `route.go` inside one is a build error. The bracketed forms fail with an error that names the folder to use instead.

Go needs a legal identifier for every path value, so Bifrost replaces the characters one cannot contain: `post-id_` registers `/posts/{post_id}` while `r.PathValue("post-id")` still returns the captured segment.

The other files in a route directory are optional:

- `page.tsx` exports `Page` and, optionally, `Head`. It is the only file that can export `Head`.
- `layout.tsx` exports `Layout`, wraps every route below it, and stays mounted across navigation.
- `template.tsx` exports `Template`, wraps everything below it like a layout, and remounts on every navigation.
- `loading.tsx` exports `Loading`, and shows while a navigation to that route runs. It covers a route and everything below it, like `layout.tsx`, and the deepest one wins.
- `error.tsx` exports `Error`, and `not-found.tsx` exports `NotFound`. `Error` receives an `error` object and a `reset` function that re-fetches the route.
- `page.go` exports `Load`, the Go loader for the page, and can export `Cache`, the `Cache-Control` of the rendered document. It exports `View` instead of a `page.tsx`: a page is React or Go, never both. A tree of only Go pages builds without Bun or Vite. `bifrost dev` rebuilds and restarts it when a file under `app/` changes, and `bifrost routes` lists its pages beside the JS routes.

A Go page declares `func View(props Props) func(*ui.Context)` or `func View() func(*ui.Context)`, and `func Metadata(props Props) seo.Head` for what a crawler reads. `seo.Head` is the metadata type of the `mygo` module. Bifrost generates the server that composes `seo.Body` and `web.Shell`, and a page whose view takes no props renders once: the generated server keeps the HTML for the requests that follow.

`seo.Link(c, "Pricing", "/pricing")` draws the link `ui.Link` draws and records where it goes, because the accessibility tree carries a link's name but not its URL. A view that calls `ui.Link` directly still needs the page's `var Links map[string]string`.

A Go page can also export `Screen`, its view for the MyGo app: `func Screen(notes Notes) func(*ui.Context)`, where `Notes` is an interface the page declares and the app satisfies; a page that needs nothing declares `Screen()`. When the project root has a `main.go` with `type app`, `bifrost build` writes `zz_bifrost_routes.go` beside it with `func appPages(app *app) []appPage`, one entry per `Screen`. The compiler checks each `Screen` call; there is no `any` and no type assertion. The app's router is built from the same `app/` tree as the server.

A `page.go` with `Screen` and no `View` is a screen of the app alone: the server answers it with the shell and `noindex`, and the sitemap leaves it out. A `layout.go` exports `Layout(page func(*ui.Context)) func(*ui.Context)` and wraps the pages of its directory and the ones below it, on the server and in the app. A `not-found.go` exports `NotFound` and, optionally, `NotFoundMetadata`, and answers the paths no page holds with a 404. An `error.go` exports `Error(err error) func(*ui.Context)` and, optionally, `ErrorMetadata`, and answers a page whose `Load` failed, with the status the error wraps or a 500: it is the nearest one above the page, wrapped in the layouts of its directory, `noindex`, and never cached. A load that answers 404 shows the `not-found.go` above the page instead.

`func Sitemap() []string` lists the paths of a route whose pattern has a parameter, and `func Sitemap() []bifrost.SitemapEntry` adds when each last changed and the languages it comes in. `func Cache() string` sets the `Cache-Control` of the page. The scaffold of all of it is `bifrost init --native <directory>`.

`bifrost build` also compiles the browser build to `dist/web`, copies it inside the generated server, and serves it from there, or from `BIFROST_WASM_DIR` when set, so one command produces the pages, the server, and the app the server hands to the browser. `bifrost dev` sets `BIFROST_DEV`, which makes `web.Shell` carry the script that reloads a page when the server restarts.
- `route.go` exports any of `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, and `Options`.
- `middleware.go` exports `Middleware`, which wraps every route below it.
- `server.go` exports `Serve` and is only valid at the route root.
- `commands/` in the project root exports `Run` and turns the binary into a
  multi-command one (see [One-off commands](#one-off-commands)).
- `public/` holds files served as they are.

Every view accepts a default export instead of the named one, so `export default function Page` and `export default function Layout({ children })` work.

Every page receives `params`, `searchParams`, and `pathname` merged into its loader props, so a page reads `params.slug` and `searchParams.tab` without asking the loader. `params` keys by folder name, a catch-all parameter is an array of segments, and a repeated query key is an array too. Because Bifrost merges them into the props, a loader must return a map or a `bifrost.PageData` whose `Props` is a map.

A layout or a page can export `metadata`, and Bifrost merges the objects from the outer layouts down to the page, so a page overrides one key and inherits the rest. A page that exports `Head` cannot also export `metadata`. It renders `title`, `description`, `keywords`, `alternates.canonical`, `robots.index`, `robots.follow`, and `openGraph` (`title`, `description`, `url`, `images`). `generateMetadata(props)` covers what depends on the request: it receives the page props, may be async, and merges the same way.

## One-off commands

A project with a `commands` package in the root of the module answers its own
arguments. The generated main calls it before anything else: no renderer, no
Bun, no port, no health check.

```go
// commands/ops.go
package commands

import (
	"context"
	"errors"
)

func Run(ctx context.Context, args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil // no command: start the server
	}
	switch args[0] {
	case "cleanup":
		return true, cleanup(ctx)
	case "migrate":
		return true, migrate(ctx, args[1:])
	}
	return false, errors.New("unknown command " + args[0])
}
```

```sh
./app migrate          # runs the command and exits
./app                  # starts the server
```

The first return value says whether `Run` handled the arguments: `false` falls
through to the server, so an app without arguments behaves as it always did.
An error exits 1 with the message, as any startup failure does. This is what a
cron job or a migrator needs: one artifact to ship instead of a second binary,
and the same code and modules the server already has.

`bifrost build` adds the import only when the package exists and exports `Run`,
so every other project builds exactly as before.

## App Router navigation

Use normal `<a href="/posts/hello">` links. After the first server render, an App Router app fetches the next route through the same Go middleware and loader, lazy-loads its Vite module, and updates one React root. Shared layouts stay mounted, and back/forward, scroll, hash links, focus, page head, and root document attributes follow the route.

Page-local state resets when the pathname changes, dynamic parameters included (`/posts/one` → `/posts/two`). Query changes keep page state but reload props; hash-only changes keep state without running the loader. Layouts keep state while they stay in the tree. Back/forward restores scroll, not unmounted page state.

The current page stays visible with `aria-busy="true"` on `#app` while a navigation runs. When a `loading.tsx` covers the target route, the client renders the target tree with the loading view instead of the page and swaps in the page when the props arrive; a navigation that keeps the pathname, such as a query or hash change, never shows it, so page state survives. A newer navigation or refresh cancels the previous request. There is no prefetch or route-data cache: loaders run on route navigation and refresh, including back/forward between paths or queries.

External links, downloads, new tabs, and modified clicks keep browser behavior; `data-bifrost-reload` on a link forces a document load. Unsupported responses, incompatible builds, and heads with scripts, base tags, or HTTP-equivalent metadata fall back to document navigation. Direct visits and links without JavaScript still use SSR.

Navigation responses carry props and server-rendered head metadata, not page HTML. Bifrost still runs SSR to preserve render-error boundaries, discarding body chunks without buffering them; this saves document reloads, not SSR work. Custom middleware must preserve the navigation `Accept` header and `Vary: Accept`, and must not cache these responses.

Generated App Router routes use `Route.WithNavigation()`. Its view must export a hook-free `renderPage(props, pageKey?)` tree factory, an SSR `Page` component that renders the same tree, and, when a `loading.tsx` covers the route, a `renderPending(props, pageKey?)` factory that renders the same tree with the loading view instead of the page. The factory keys the page branch by `pageKey`, keeps layout keys stable, and opts out of React Compiler memoization; hooks belong in the page and layout components inside it. Ordinary `Server`, `Static`, and `Client` declarations keep their behavior.

### Programmatic navigation and refresh

App Router apps can import `navigate`, `replace`, `refresh`, `Link`, and the navigation hooks from `virtual:bifrost/navigation`. Call `navigate`, `replace`, and `refresh` from browser event handlers or effects, not during rendering:

```tsx
import { navigate, refresh } from "virtual:bifrost/navigation";

export function Actions() {
  return <>
    <button onClick={() => navigate("/posts")}>Posts</button>
    <button onClick={() => refresh()}>Refresh</button>
  </>;
}
```

- `navigate(href)` follows the same path as an internal link and adds a history entry. Relative URLs resolve against the current browser URL; external HTTP(S) URLs use a document load, and other URL schemes reject with `TypeError`.
- `replace(href)` does the same but replaces the current history entry.
- `refresh()` reruns the current URL's middleware and loader without adding a history entry, updating props and head metadata while keeping page/layout state, focus, and scroll. Call `await refresh()` after a successful mutation to display fresh server data. If the server redirects, Bifrost replaces the current history entry and applies normal route state and focus rules.
- They return `Promise<void>`, resolving after the client update, a superseding request, or initiation of a document fallback; they do not wait for a fallback document to load. Importing them during SSR is safe, but calling them without a mounted client router rejects.

The hooks read the current route. Bifrost provides the values on the server and on the client, so a component that renders them hydrates without a mismatch:

```tsx
import { Link, useParams, usePathname, useSearchParams, useRouter } from "virtual:bifrost/navigation";

export function Nav() {
  const pathname = usePathname();
  const params = useParams();
  const tab = useSearchParams().get("tab");
  const router = useRouter();
  return <nav>
    <Link href="/posts" aria-current={pathname === "/posts" ? "page" : undefined}>{tab}</Link>
    <button onClick={() => router.push("/posts")}>Posts</button>
  </nav>;
}
```

- `usePathname()` returns the current pathname, percent-encoded as the browser reports it.
- `useParams()` returns the route parameters, the same object the page receives; in a layout they are the parameters of the page below it.
- `useSearchParams()` returns a `URLSearchParams` built from the query string.
- `useRouter()` returns `push`, `replace`, `refresh`, `back`, and `forward`; `back` and `forward` use browser history.
- `Link` renders an anchor that navigates in place on click, exactly like any other internal link, and loads a document when JavaScript is off.

`bifrost init` includes the types. Existing App Router apps can add this to `bifrost.d.ts`:

```ts
declare module "virtual:bifrost/navigation" {
  export function navigate(href: string): Promise<void>;
  export function replace(href: string): Promise<void>;
  export function refresh(): Promise<void>;
  export function Link(props: { href: string; children?: unknown } & Record<string, unknown>): any;
  export function usePathname(): string;
  export function useParams(): Record<string, string | string[]>;
  export function useSearchParams(): URLSearchParams;
  export function useRouter(): {
    push(href: string): Promise<void>;
    replace(href: string): Promise<void>;
    refresh(): Promise<void>;
    back(): void;
    forward(): void;
  };
}
```

Run `bash scripts/navigation-integration.sh` for production and development browser checks (requires Chromium).

## Browser performance

Bifrost emits render-blocking styles first, preloads every static client import, and gives module preloads low fetch priority so they do not compete with high-priority LCP images. Vite owns tree shaking and chunking; hashed build assets use one-year immutable caching.

Serve production responses through Brotli or gzip: that belongs to the HTTP server, CDN, or reverse proxy, which owns content negotiation and caching. Import long-lived assets through Vite when possible so they receive hashed immutable URLs; files copied from `public/` keep stable URLs and revalidate by default.

Track compressed transfer bytes, request count, LCP, CLS, and hydration time under network and CPU throttling; local uncompressed load time is not a useful production browser metric.

## Go application plugins

```go
type AppPlugin interface {
    Name() string
    Register(*bifrost.AppRegistry) error
}
```

`AppPlugin`s register once during `New`. They add validated page routes, standard Go middleware, typed error handling, asset headers, and runtime observation hooks. Frontend transforms belong to Vite. There is no global Go registry or generic event bus.

## Guarantees

- Standard `http.ServeMux` patterns and path values.
- Props are encoded once and safely embedded for hydration.
- Immutable startup model and strict stale-manifest checks; Vite manifests are authoritative, and Go hashes but never renames Vite output.
- Tailwind, React Compiler, Vite aliases, linked workspace packages, virtual modules, CSS Modules, assets, and shared client/SSR chunks are covered by integration tests.
- Static and client requests do no render work; SSR streams head and body frames over a Unix socket, without HTTP between Go and Bun.
- Isolated renderer workers with bounded concurrency and queue, readiness checks, and restart after transport failure.
- The extracted renderer runtime is cached and reused across starts; stale builds are pruned after a week.
- Queueing is bounded by time, not by an instant reject: a request waits up to two seconds for renderer capacity and then returns 503 with `Retry-After`.
- `--host-bun` (the build default) keeps the renderer out of the app binary, and `BIFROST_RENDERER_SOCKET` lets several app processes share one warm renderer.
- End-to-end request cancellation through Go, Bun, and React streams.
- Required build failures fail the whole build.

## Deploy

See [DEPLOY.md](DEPLOY.md) for artifact sizes per router mix, environment variables, readiness, saturation, containers, and the bee setup.

## Platforms

Linux amd64 and arm64 production, containers, and macOS development. Windows is not supported.

## Checks

```sh
make check # test race vet, the App Router release gate, and the App Router demo
make gate # only the App Router release gate
make integration
make dev-integration
make reproducible
make bench
make throughput
```

The integration scripts serve on `127.0.0.1:8080`, `:8081` and `:8082`. Set `BIFROST_TEST_PORT` to move the whole range, for example `BIFROST_TEST_PORT=8090 make integration` when 8080 is taken.

See [DESIGN.md](DESIGN.md), [QUESTIONNAIRE.md](QUESTIONNAIRE.md), and [IMPLEMENTATION.md](IMPLEMENTATION.md) for the model, decisions, completed scope, and measured limits.
