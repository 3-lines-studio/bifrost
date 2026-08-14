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
go run github.com/3-lines-studio/bifrost/cmd/bifrost version
```

Call `bifrost.Building()` immediately after `New` and return before opening databases or listeners. `build` runs the app through dedicated describe and static-generation phases, asks Vite to build each unique client and SSR view, prerenders Static routes through Bun, compiles a pinned standalone Bun renderer, validates Vite's manifests, writes a strict Bifrost manifest, and atomically replaces `.bifrost`.

The generated `zz_bifrost_gen.go` embeds `.bifrost` and provides the package-local `bifrostAssets` value used by `Config`.

## React module contract

```tsx
export function Head(props) {
  return <title>{props.title}</title>;
}

export function Page(props) {
  return <main>{props.title}</main>;
}
```

`Page` is required. `Head` is optional. Server and Static pages hydrate. Client pages mount into an empty shell. Loader and generator props are sent to the browser; never return secrets as props. Hydrated pages use React Client Component rules, so page components cannot themselves be async. Use `React.lazy` with `Suspense` for streamed deferred UI.

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

`ResolveMarkdown` serves server-rendered routes as Markdown for a `.md` path suffix or a preferred `Accept: text/markdown` media type. It leaves static pages, client pages, public files, and other mux handlers unchanged. `Handler` applies it automatically.

Use `/{$}` for an exact root page. The standard `/` pattern is a subtree fallback. Bifrost does not add router-specific adapters.

## Build and runtime boundary

Build phases execute the application to collect immutable declarations. Code needed to construct `Config`, routes, loaders, and generators must be side-effect free. Check `bifrost.Building()` immediately after `New`, before opening listeners, databases, queues, or background workers.

When declarations live in an internal package, pass the generated package-local `bifrostAssets` from `main` into that package. See `example/structured` for this layout. This keeps one generated embedded tree and avoids a second copied embed.

## SSR concurrency contract

Bifrost uses one isolated Bun renderer process by default. `RenderConcurrency: N` starts N production renderer processes, and each process handles one render at a time. Development always serializes SSR through its one Vite module graph. This prevents simultaneous requests from racing through one JavaScript module graph while allowing explicit production scaling.

Mutable JavaScript module globals still persist between sequential requests handled by the same worker. Do not store locale, user, authentication, or request data in module-level variables. Derive them from props or request-local React context.

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

## Browser performance

Bifrost emits render-blocking styles first, preloads every static client import, and gives module preloads low fetch priority so they do not compete with high-priority LCP images. Vite remains responsible for tree shaking and chunking. Hashed build assets use one-year immutable caching.

Serve production responses through Brotli or gzip compression. Compression remains the HTTP server, CDN, or reverse proxy's job because that layer owns content negotiation and caching. Import long-lived assets through Vite when possible so they receive hashed immutable URLs; files copied from `public/` keep stable URLs and revalidate by default.

Track compressed transfer bytes, request count, LCP, CLS, and hydration time under network and CPU throttling. Local uncompressed load time is not a useful production browser metric.

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
- Request-scoped root document attributes are validated and kept out of React props.
- Immutable startup model and strict stale-manifest checks.
- Vite manifests are authoritative; Go hashes but never renames Vite output.
- Tailwind, React Compiler, Vite aliases, linked workspace packages, virtual modules, CSS Modules, assets, and shared client/SSR chunks are covered by integration tests.
- Static and client requests do no render work.
- SSR streams head and body frames.
- Isolated renderer workers with bounded concurrency and queue.
- End-to-end request cancellation through Go, Bun, and React streams.
- Renderer readiness checks and process restart after transport failure.
- Hashed assets use immutable cache headers.
- Required build failures fail the whole build.

## Platforms

Linux amd64 and arm64 production, containers, and macOS development. Windows is not supported.

## Checks

```sh
make check
make integration
make dev-integration
make reproducible
make bench
```

See [DESIGN.md](DESIGN.md), [QUESTIONNAIRE.md](QUESTIONNAIRE.md), and [IMPLEMENTATION.md](IMPLEMENTATION.md) for the model, decisions, completed scope, and measured limits.
