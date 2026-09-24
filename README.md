# Bifrost

Bifrost serves Vite-built React pages from an ordinary Go `net/http` application. Vite owns frontend builds, plugins, assets, and development HMR. Bun executes streaming React SSR. Go owns routing, loaders, HTTP, validation, and deployment.

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
bun install
go run github.com/3-lines-studio/bifrost/cmd/bifrost init ./myapp
go run github.com/3-lines-studio/bifrost/cmd/bifrost build ./cmd/web
go run github.com/3-lines-studio/bifrost/cmd/bifrost dev ./cmd/web
go run github.com/3-lines-studio/bifrost/cmd/bifrost routes ./cmd/web
go run github.com/3-lines-studio/bifrost/cmd/bifrost version
```

Flags precede the package path. `init` runs `bun install` automatically (`--no-install` skips it). `build` accepts `--sourcemaps`, `--static-workers`, `--output`, and `--vite-config`. `dev` accepts `--poll`, `--vite-port` (zero picks a free port), and `--vite-config`. `routes` prints the route table (views, API handlers, and middleware) without building the app (`-C` and `--vite-config`).

`dev` runs one validated bootstrap build, then owns a Bun process hosting Vite's development server and the SSR bridge. The bridge outlives Go child restarts, so a Go edit never discards frontend module state. Browser assets are proxied through your Go origin at `/_bifrost/dev/`, so HMR, module loads, and page requests share one origin and the Vite server never has to be reachable from the browser. Server and Static pages include stylesheet links from Vite's live SSR module graph, so development pages keep their styles instead of flashing unstyled content. Go replacement is detected by a long-poll to `/_bifrost/build-id` that holds until the process is replaced, so idle development generates almost no requests.

`build` runs the app through describe and static-generation phases, asks Vite to build each unique client and SSR view, prerenders Static routes through Bun, compiles a pinned standalone Bun renderer, validates Vite's manifests, writes a strict Bifrost manifest, and atomically replaces `.bifrost`. The generated `zz_bifrost_gen.go` embeds `.bifrost` and provides the package-local `bifrostAssets` value used by `Config`.

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

## Build and runtime boundary

Build phases execute the application to collect immutable declarations, so code that constructs `Config`, routes, loaders, and generators must be side-effect free. Check `bifrost.Building()` immediately after `New`, before opening listeners, databases, queues, or background workers. When declarations live in an internal package, pass the generated package-local `bifrostAssets` from `main` into that package; `example/structured` shows the layout that keeps one generated embedded tree instead of a second copied embed.

## SSR concurrency contract

Bifrost uses one isolated Bun renderer process by default; `RenderConcurrency: N` starts N production renderers, each handling one render at a time. Development always serializes SSR through its one Vite module graph. This keeps simultaneous requests from racing through one JavaScript module graph while allowing explicit production scaling.

Module globals persist between sequential requests handled by the same worker, so keep locale, user, authentication, and request data out of module-level variables: derive them from props or request-local React context.

Expose renderer readiness through the user-owned health endpoint:

```go
if err := app.Ready(request.Context()); err != nil {
    http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
    return
}
writer.WriteHeader(http.StatusNoContent)
```

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
- `page.go` exports `Load`, the Go loader for `page.tsx`.
- `route.go` exports any of `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, and `Options`.
- `middleware.go` exports `Middleware`, which wraps every route below it.
- `server.go` exports `Serve` and is only valid at the route root.
- `public/` holds files served as they are.

Every view accepts a default export instead of the named one, so `export default function Page` and `export default function Layout({ children })` work.

Every page receives `params`, `searchParams`, and `pathname` merged into its loader props, so a page reads `params.slug` and `searchParams.tab` without asking the loader. `params` keys by folder name, a catch-all parameter is an array of segments, and a repeated query key is an array too. Because Bifrost merges them into the props, a loader must return a map or a `bifrost.PageData` whose `Props` is a map.

A layout or a page can export `metadata`, and Bifrost merges the objects from the outer layouts down to the page, so a page overrides one key and inherits the rest. A page that exports `Head` cannot also export `metadata`. It renders `title`, `description`, `keywords`, `alternates.canonical`, `robots.index`, `robots.follow`, and `openGraph` (`title`, `description`, `url`, `images`). `generateMetadata(props)` covers what depends on the request: it receives the page props, may be async, and merges the same way.

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
- Static and client requests do no render work; SSR streams head and body frames.
- Isolated renderer workers with bounded concurrency and queue, readiness checks, and restart after transport failure.
- The extracted renderer runtime is cached per build and reused across starts, and every start removes the leftovers nothing is using.
- End-to-end request cancellation through Go, Bun, and React streams.
- Required build failures fail the whole build.

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
```

See [DESIGN.md](DESIGN.md), [QUESTIONNAIRE.md](QUESTIONNAIRE.md), and [IMPLEMENTATION.md](IMPLEMENTATION.md) for the model, decisions, completed scope, and measured limits.
