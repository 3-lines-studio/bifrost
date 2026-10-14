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

Flags precede the package path. `init` runs `bun install` automatically (`--no-install` skips it). `build` accepts `--sourcemaps`, `--static-workers`, `--output`, `--vite-config`, and `--render-concurrency` and `--render-queue` (the App Router takes its renderer worker count and its queue depth here). `dev` accepts `--poll`, `--vite-port` (zero picks a free port), and `--vite-config`. `routes` prints the route table (views, API handlers, and middleware) without building the app (`-C`, `--vite-config`, and `--json` for the route table a code generator can read, including the parameter names of dynamic patterns).

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

A server route also answers `Accept: application/vnd.bifrost.data+json` with the props its loader produced, and nothing else:

```json
{"pattern":"/docs/{slug...}","props":{"slug":"intro"},"document":{"lang":"en"},"status":200}
```

A native client renders the same view itself, so it needs the loader's data and not a rendered document. The response never reaches the renderer, so it costs no render and no JavaScript runtime. It is `Cache-Control: no-store`, a loader error becomes JSON with its own status, and a loader `Redirect` keeps its `Location` with no body. Server routes carry `Vary: Accept`, because the same URL has two representations.

```go
request, _ := http.NewRequest(http.MethodGet, "/docs/intro", nil)
request.Header.Set("Accept", "application/vnd.bifrost.data+json")
```

Use `/{$}` for an exact root page; the standard `/` pattern is a subtree fallback. Bifrost does not add router-specific adapters.

Rendered documents are `Cache-Control: private, no-cache, no-store, max-age=0, must-revalidate` by default, because their props usually come from the request. Declare a route as cacheable with `Route.WithCache` in Go or with a `Cache` export next to `Load` in the App Router, for example `public, s-maxage=60` to let a CDN hold it. The document keeps `Vary: Accept`, and the client-side navigation response stays `no-store` and never reaches a shared cache.

## Build and runtime boundary

Build phases execute the application to collect immutable declarations, so code that constructs `Config`, routes, loaders, and generators must be side-effect free. Check `bifrost.Building()` immediately after `New`, before opening listeners, databases, queues, or background workers. When declarations live in an internal package, pass the generated package-local `bifrostAssets` from `main` into that package; `example/structured` shows the layout that keeps one generated embedded tree instead of a second copied embed.

## SSR concurrency contract

Bifrost uses two isolated Bun renderer processes by default, about 40 MB of RSS each; `RenderConcurrency: N` starts N production renderers, each handling one render at a time, and `RenderConcurrency: 1` is enough for a single-core box. Development always serializes SSR through its one Vite module graph. This keeps simultaneous requests from racing through one JavaScript module graph while allowing explicit production scaling.

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
- `page.go` exports `Load`, the Go loader for `page.tsx`, and can export `Cache`, the `Cache-Control` of the rendered document.
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

## Native target

`bifrost native [dir]` writes an Expo Router project to `.bifrost/native`: one screen per route, each a stub that calls the same `renderPage` the web entry hydrates and reads its props from the data endpoint. Both routers come from one route table, so `/docs/{slug}` becomes `app/docs/[slug].tsx` and nothing is written twice. `--origin` sets the base URL the client talks to; by default it is the host alias of each platform, `10.0.2.2` on Android and `localhost` elsewhere.

The generated project is a build artifact. Every run rewrites `app/` and `bifrost/`, so a screen the route table no longer has disappears, and nothing written there is worth keeping.

An app keeps its own files in a directory next to `app/`: `native/` by default, `--from` to move it. They are copied over the generated project after everything else, so they win, and `bifrost native` never writes them. A session store, a login screen, the client the app talks to the server with: all of it goes there and survives every regeneration. When the directory exists, the command prints where it read them from.

A resolver belongs to that directory too. The two seams a native target needs, the navigation module and the jsx runtime, live in `bifrost/metro.js` as `withBifrost`, and the generated `metro.config.js` is the three lines that install them. An app that needs a resolver of its own replaces `metro.config.js` whole and wraps the helper, so nothing has to be written twice:

```js
const { getDefaultConfig } = require("expo/metro-config");
const { withBifrost, withBifrostWarnings } = require("./bifrost/metro");
const { withNativewind } = require("nativewind/metro");

const seams = withBifrost(getDefaultConfig(__dirname));
const withCSS = withNativewind(seams, { inlineRem: 16 });

module.exports = withBifrostWarnings(withCSS);
```

The order of the resolver does not matter. Metro hands the last resolver only the default one, so a resolver that reached for that would drop whoever was there before; `withBifrost` keeps the one the config already had and hands it everything it does not own. The order of `withBifrostWarnings` does: it wraps the transformer, and the last wrapper wins. Installed anywhere but outermost, the wrapper that runs after it puts its own transformer back and the diagnostics simply never run -- which is indistinguishable from a stylesheet with nothing to report. `bifrost native` reads the `metro.config.js` an overlay ships with and says so when it does not find it.

It is there because the two most expensive failures of a native target are the quiet ones.

A class the compiler cannot express compiles to nothing and looks exactly like one that works. `h-[100dvh]` is a viewport on the web and no height at all here, `min-h-svh` is no height either, and either one closes a tree to zero without a word. The compiler collects the values it dropped and the transformer throws them away, so the wrapper reads them back and names the class that asked for each one, once, in development:

```
bifrost: global.css: .h-[100dvh]: height: 100dvh has no native counterpart
```

A property the target lacks outright -- `cursor`, `resize`, `snap` -- is not reported: it was cosmetic in every case measured, and the reset Tailwind ships is full of them. A build never pays for the second look, and neither does a stylesheet under `node_modules`.

The other quiet failure is a prop whose whole meaning is the document. A renderer drops `dangerouslySetInnerHTML` without a word, so a paragraph compiled from markdown paints nothing at all and the page comes out missing it, and the runtime names it in development as well:

```
bifrost: a native target drops dangerouslySetInnerHTML on <div>
```

Three things a client of a web app needs are installed here, because none of them is the app's business.

TypeScript paths are read from the app's `tsconfig.json`, so an import spelled `@/components/x` resolves the moment the web one does and an app never has to write its imports twice. Metro has never read them, because the web bundler gets them from a config of its own; the tsconfig is the one place they are written down. A pattern with no wildcard names a single module and is left to the resolver that would have found it anyway. A comment and a trailing comma are read too, because an editor leaves both behind and a JSON parser refuses them.

`React.createElement` reaches the tag map as well. A library built before the automatic runtime builds its elements by hand and the jsx runtime alias never sees them, so a tag written for the web would paint a tag for the web; measured, this is what `@base-ui` does. The module object is patched in place rather than copied, because a copy holds the own enumerable keys only and a library that imports React as a namespace would get `undefined` for everything else.

A relative request gets the origin in front of it. A page asks its server for `/api/...` and the browser puts the origin back; a native client has none, so the request goes looking for a host that is not the one the app talks to. The shim is the browser's rule and nothing else, and it runs before the first screen does, because the generated layout imports it.

Its `package.json` contributes `dependencies`, `devDependencies` and `overrides`, which are merged into the generated manifest; every other key still comes from the generator, so the shell keeps owning the modules it needs. `overrides` is there because a dependency that an app adds is not necessarily the one that gets installed: a package the generator already pins with a range that a newer release also satisfies lets the manager hoist that release instead, and a pin is the only way to say which copy wins.

Its `app/` may restyle a screen the route table already has, but it may not add one. An overlay screen the server does not serve is an error, because a route the server does not know would take the two routers apart. Symlinks are refused as well, so the project can be rebuilt anywhere.

The layout is the one file an app is most tempted to replace, to add a line to the shell the generator writes. Two files it can ship instead are imported for it. One is `global.css` at the root of the overlay, which is where the utilities come from and which is not the one the web app has, because Tailwind needs different `@source` paths and none of the font packages. The other is a module of its own, `bifrost/setup.ts`, for whatever a native target has to install before the first screen renders. Both are imported before the network shim, so the app can install what the shim wraps.

A page written for the browser reaches for a document from places the tag map cannot cover -- a measurement, a media query, somewhere to keep a preference -- and none of them has a native counterpart. The generator writes that shim to `bifrost/dom.js` and **imports none of it**: a module nothing imports is never bundled, so a page that never touches a document pays nothing. A page that does turns it on with one line of its own:

```ts
import "./dom";
```

It answers what it can and answers as absent everything else: a query returns null, an attribute is not there, a measurement is zero, and reading a cookie says there is none rather than inventing one. `localStorage` really stores, `innerWidth` and `innerHeight` come from the window dimensions, and the clipboard borrows `expo-clipboard` when the app has it instead of pretending the copy happened. Every assignment is a fallback, so anything the app or the target already installed stays.

Sessions need nothing extra from the client: an Expo app sends its cookie jar with every request, so a cookie a login route sets reaches the next data request. A route a session middleware protects answers with a redirect instead of props, and the screen moves the router to it rather than rendering the destination under the path that was asked for.

A status is part of what a loader returns, so it travels with the props. When a loader answers `bifrost.NotFound()` or `bifrost.Status`, the props already carry `__bifrostNotFound` or `__bifrostError`, and the view the generator wrote picks the app's `not-found.tsx` or `error.tsx` out of them without the screen knowing anything about it.

A page that throws while it renders is the other half. The server catches it and re-renders into the nearest `error.tsx`; a native client is the one rendering, so the screen puts a boundary around the view and walks the same ladder, deepest `error.tsx` first and one level up on every further throw. How deep the ladder goes is a number the server counts, so the data endpoint sends it as `errorFallbacks`, and the message is the one the page threw, because it was thrown here and not on a server that would have replaced it with a status text.

The root document attributes travel with every page too, because they were the attributes of the root element: `class` becomes the class of a view around the page, which is how a loader hands the screen a theme for the whole tree, and `dir` becomes the direction. `lang` has no counterpart on a native target, because a text node there inherits no language to read from, so it is dropped rather than faked.

```go
func Load(r *http.Request) (any, error) {
	return bifrost.PageData{Props: map[string]any{}, Document: bifrost.Document{Class: "dark"}}, nil
}
```

### Styling

The stylesheet is a file of the overlay and the generated layout imports it, which is what makes it part of the bundle. Nothing else does: `withNativewind` takes an `input` option and NativeWind 5 ignores it. Measured through the whole transformer, the same stylesheet compiles to the same 592 bytes with `input: "./global.css"`, with `input: "./otro.css"` pointing at a file that does not exist, and with no options at all.

The option worth setting is `inlineRem`, the size a root `rem` is worth. It defaults to 14 and the browser default is 16, so an app that wants the two targets to agree sets it; measured, `gap-2` compiles to 7 without it and to 8 with it.

`postcss.config.mjs` is not optional, and nothing says so when it is missing:

```js
export default { plugins: { "@tailwindcss/postcss": {} } };
```

Without it Tailwind never runs, and what is left is the theme -- the `:root` variables, the `@theme` mappings -- and not one utility. Measured on the same stylesheet and probe: 958 bytes with the file, 393 without, and the utilities are the 565 that go missing. What comes out without it still paints the right colours, so it looks exactly like a project where NativeWind was never installed, which is why it is worth saying twice.

#### What the stylesheet is scanned for

Tailwind 4 finds its own candidates in every file it can reach, and the generated project is inside the directory it scans. Its own `bifrost/host.tsx` is written in prose, and prose contains class names: measured in a tree whose only sources are that file and one page, its comment about blocks and its tag list put `.block`, `.inline` and `.table` into the stylesheet with nothing asking for them, and a native target then drops them again.

`source(none)` turns that off, and the stylesheet then holds what the app declares and nothing else:

```css
@import "tailwindcss" source(none);

@source "../../app";
@source "../../components";
@source "./app-components";
@source "./components";
```

The scan is the only thing that ever finds a class, and a class that is not scanned is not reported anywhere: it compiles to nothing and the view is quietly missing its styles. So the list has to be complete, and that is the trap. `../../app` and `../../components` reach the web app's tree; every file the overlay ships is under `./`, including the generated `app/` and whatever else the overlay puts beside it. Measured on a real overlay: with the first two lines alone, a class written only in `app-components/` is absent from the stylesheet; with all four it is there.

The gain is small enough that leaving `source(none)` out is the better default. Measured on that same overlay, the whole stylesheet: 25560 bytes and eight dropped-value warnings with automatic detection, 24947 and six with `source(none)`, and the two that go are the ones the generated file produced. A couple hundred bytes is not worth a list that silently costs a class the day somebody adds a directory.

### One page, two targets

A page is written once, for the browser, and the tags in it are what the native screen paints. Metro resolves `react/jsx-runtime` to a wrapper that turns a lowercase tag into the component that draws it: `div` and the other blocks become a `View` that groups any text inside it into a `Text`, `p`, `h1`, `span` and the rest become `Text`, `a` becomes a `Pressable` that navigates a path and opens a url, `button`, `input`, `textarea` and `img` become their controls. Sheet links, metadata and scripts have nothing to paint, so they render nothing.

The vocabulary is the set of tags a native target supports. A lowercase tag outside it is one the map is missing: it warns in development with its own name, and the element is left exactly as it was, so the failure is loud instead of a paragraph that quietly disappears. Anything that is not a lowercase tag is untouched too, because React Native builds its own views out of strings and replacing one would tear them apart.

An app always has a tag the generator does not know, or one it wants painted its own way, so it ships `bifrost/host-extra.tsx` in its overlay and the map takes it:

```tsx
export const extraTags: Record<string, unknown> = {
  svg: Svg,
  path: Path,
  div: Box,
};
```

It is merged over the generated table, so a name that is already there is replaced and one that is not is added, and it is merged last so the app wins. Everything else stays generated, including the props the target drops and the warning for a tag nobody knows.

The seam is there because replacing the file is what an app had to do, and it is not a cost you pay once. Measured: an app replaced this file, the generator later added a function the runtime calls, the replaced copy did not have it, and **every element of the app threw in development** -- `loaded.droppedProps is not a function` -- with nothing saying why. Through the seam the same app runs.

A tag that is behaviour and not presentation is left out on purpose. `select` needs a picker, so claiming it would be a lie: it stays unsupported, and the app gives it a native counterpart next to the screen. A form's `method` and `action` are the same kind of thing, and a `button type="submit"` does nothing on its own.

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
make throughput
```

The integration scripts serve on `127.0.0.1:8080`, `:8081` and `:8082`. Set `BIFROST_TEST_PORT` to move the whole range, for example `BIFROST_TEST_PORT=8090 make integration` when 8080 is taken.

See [DESIGN.md](DESIGN.md), [QUESTIONNAIRE.md](QUESTIONNAIRE.md), and [IMPLEMENTATION.md](IMPLEMENTATION.md) for the model, decisions, completed scope, and measured limits.
