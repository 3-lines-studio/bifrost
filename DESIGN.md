# Bifrost: core data model

Status: V1 baseline implemented; detailed decisions are recorded in [QUESTIONNAIRE.md](QUESTIONNAIRE.md)

## Goal

Build a small, idiomatic Go library that serves React pages through `net/http`. A thin CLI handles frontend development and production builds.

Priority order:

1. Simple public API.
2. Correct builds and request behavior.
3. Fast request path.
4. Good development loop.

Bifrost should borrow the useful part of Next.js: one system for routing, data loading, rendering, client assets, and static output. It should not copy React Server Components, cache layers, middleware runtimes, or Node server behavior until a real use case requires them.

The user owns the HTTP server. Production defaults to one Go binary containing build assets and the pinned renderer runtime. V1 supports Linux amd64 and arm64, containers, and macOS development. Windows is not supported.

React is the only V1 frontend. The renderer boundary stays private until a second implementation proves a public abstraction is useful.

## Smallest supported system

Version 2 starts with three page kinds:

| Kind | Go work | JS server work | Browser work |
|---|---|---|---|
| Server | Load props on each request | Render on each request | Hydrate |
| Static | Generate props at build time | Render at build time | Hydrate |
| Client | None | None | Mount |

Every route has exactly one kind. These are presets, not flags. This avoids invalid combinations such as client-only plus a server loader.

Baseline public API:

```go
app, err := bifrost.New(bifrost.Config{
    Assets: bifrostAssets,
    Routes: []bifrost.Route{
        bifrost.Server("/", "pages/home.tsx", loadHome),
        bifrost.Server("/simple", "pages/home.tsx", nil),
        bifrost.Static("/blog/{slug}", "pages/blog.tsx", generatePosts),
        bifrost.Client("/app", "pages/app.tsx"),
    },
    AppPlugins: []bifrost.AppPlugin{
        observability,
    },
})
```

The three constructors return one opaque `Route` type. Users cannot mutate a route into an invalid state. `nil` means no loader for `Server` and one page at the route's own path for `Static`.

Initial callback shapes:

```go
type Loader func(*http.Request) (any, error)

type Document struct {
    Lang  string
    Class string
    Dir   string
}

type PageData struct {
    Props    any
    Document Document
}

type StaticPage struct {
    Path     string
    Props    any
    Document Document
}

type Generator func(context.Context) ([]StaticPage, error)
```

Returning `any` is less type-safe than generics, but it keeps heterogeneous routes in one app and makes the call site small. Bifrost converts the value to JSON once at the boundary. A generic API would still need type erasure inside the app.

## The key split

Do not use one `PageConfig` in every phase. There are four models.

### 1. Declaration model

The declaration model contains Go functions and source paths. It exists in the user's process.

Conceptually:

```go
type routeDef struct {
    pattern  string
    view     string
    kind     routeKind
    load     Loader
    generate Generator
}
```

`routeKind` is private. Constructors create valid combinations. Startup validation still checks paths, duplicate patterns, and nil or missing values.

A route is identified by its exact URL pattern, not by its component path. Several routes may use the same component with different loaders or kinds.

### 2. Build specification

The app emits a serializable description for the build command. The command must not parse Go source code to discover pages.

```go
type BuildSpec struct {
    Schema uint32
    Routes []BuildRoute
}

type BuildRoute struct {
    Pattern string
    View    string
    Kind    string
}
```

The actual format may add fields, but it should stay declarative. Go functions do not enter this file. For static generation, the build command invokes the app in a separate generate phase and receives path/props records.

The normalized `BuildSpec` gets a digest. Production startup rejects a manifest whose digest does not match the running app. This catches stale embedded output.

### 3. Build manifest

The manifest describes immutable files. It contains no request policy and no Go callbacks.

```go
type Manifest struct {
    Schema      uint32
    SpecHash    string
    Views       map[string]BuiltView
    Routes      map[string]BuiltRoute
    ClientFiles []FileRef
}

type BuiltView struct {
    Client AssetSet
    Server *ServerAssets
}

type ServerAssets struct {
    Entry   FileRef
    Imports []FileRef
}

type AssetSet struct {
    Entry   FileRef
    Styles  []FileRef
    Imports []FileRef
}

type BuiltRoute struct {
    Kind      string
    ViewID    string
    Documents map[string]FileRef
}

type FileRef struct {
    Path string
    Hash string
    Size int64
}
```

A **route** owns URL output. A **view** owns compiled React code. This allows `/shared-a` and `/shared-b` to reuse one built view while keeping separate Go loaders.

`ViewID` is generated from the normalized view input and browser behavior (`hydrate` or `mount`). It is not derived by replacing slashes in a path. The exact hash algorithm is an internal build detail.

`Documents` exists only for static routes. Each document records its normalized request path, built HTML file, safely encoded props, and validated root HTML attributes used by Vite's live SSR module graph in development. Server routes derive the same attributes from request-scoped `PageData`. Vite owns CSS extraction; Bifrost does not implement critical-CSS rewriting.

`ClientFiles` is the complete Vite client output set. View asset sets select initial scripts, styles, and preloads from Vite's manifest, while `ClientFiles` also makes dynamic chunks and emitted assets available to the HTTP asset handler.

`FileRef.Hash` supports immutable cache headers and stale/corrupt artifact checks. It is not used for request routing.

The JSON representation can use arrays instead of maps for stable output. The in-memory shape above shows ownership, not the required wire encoding.

### 4. Runtime model

At startup, Bifrost joins declarations with the manifest, validates all references, prebuilds HTML shell fragments, and registers a specialized handler for each route.

There is no universal request-time `PageDecision` object.

```go
type serverHandler struct {
    load     Loader
    renderer Renderer
    entry    serverEntry
    shell    documentShell
}

type staticHandler struct {
    files map[string]FileRef
    fs    fs.FS
}

type clientHandler struct {
    html []byte
}
```

An exact static route can hold one `FileRef` rather than a map. These types are private and immutable after startup.

The standard `http.ServeMux` performs route matching. Bifrost registers page patterns as GET routes and lets `net/http` provide path values. Bifrost should not add a second router.

## Extension model

Frontend and server extensions use different systems.

Frontend extensions are normal Vite plugins declared in `vite.config.ts`. Vite owns module resolution, loading, transforms, CSS, assets, source maps, virtual modules, build hooks, file watching, HMR, React Fast Refresh, and its browser error overlay. Bifrost injects generated client and SSR entries and enforces output roots, the asset base, SSR bundling, and final artifact validation. Go never renames Vite output.

Server extensions use a small typed Go API:

```go
type AppPlugin interface {
    Name() string
    Register(*AppRegistry) error
}

type Config struct {
    AppPlugins []AppPlugin
}
```

`AppRegistry` supports ordinary Go middleware, typed error and asset-header policy, runtime observation hooks, and route sources that emit normal validated `Route` values.

Rules:

1. Vite is the sole frontend builder and development module graph.
2. Vite plugins are trusted build code but cannot bypass Bifrost's final path, manifest, and hash validation.
3. App plugins register once during `New`; there is no global Go registry.
4. App plugin names are unique and appear in diagnostics.
5. Registration order is deterministic and preserved.
6. Hooks use concrete types and functions, not reflection or a generic event bus.
7. With no registered hook, the request path pays no hook allocation or dynamic lookup.
8. Observation hooks cannot mutate route identity, props, artifacts, or renderer output.
9. New route semantics such as ISR or React Server Components require a versioned core change rather than a plugin escape hatch.

## Request paths

### Server

1. `http.ServeMux` calls one `serverHandler`.
2. Run the loader, if any.
3. Marshal props to JSON once.
4. Send those same JSON bytes to the renderer.
5. Write the prebuilt document prefix and dynamic head.
6. Stream body chunks.
7. Write the same props bytes and the prebuilt suffix.

The hot path does not:

- look up a manifest entry;
- derive an entry name;
- decide a page mode;
- rebuild asset lists;
- marshal props twice;
- assemble the whole HTML document in a string.

### Static

1. `http.ServeMux` calls one `staticHandler`.
2. For a dynamic pattern, normalize the path and perform one map lookup.
3. Open and serve the immutable HTML file.

An exact static pattern skips step 2.

### Client

1. `http.ServeMux` calls one `clientHandler`.
2. Write a prebuilt HTML shell.

## Renderer boundary

The internal renderer receives bytes, not arbitrary Go values:

```go
type RenderRequest struct {
    Entry string
    Props json.RawMessage
}
```

Its output is ordered:

1. one head frame;
2. zero or more body frames;
3. one end frame or one error frame.

The Go side streams without a channel or `RenderedPage{Body string, Head string}` in the core model. The renderer uses one JSON request followed by length-prefixed binary head, body, done, or error frames. A local 32 KiB decoder benchmark measured about 126 µs and 110 allocations for NDJSON versus 0.49 µs and 35 allocations for binary framing. The renderer interface exposes no Bun or React types.

If rendering fails before the first body byte, return a normal error page. If it fails after the response is committed, stop the stream and log the request and renderer error. No model can replace HTTP status after bytes have been sent.

Production defaults to one Bun worker process and one active render. `RenderConcurrency` creates an explicit pool of isolated worker processes; each worker still handles one render at a time. An idle-worker channel provides bounded assignment without a shared mutable scheduler. Request cancellation propagates through the Go transport, Bun request signal, React stream, and body reader. `App.Ready` probes every worker.

## Build flow

1. Build the user's Go app or invoke it through `go run`.
2. Run it with `BIFROST_PHASE=describe` and read `BuildSpec` from a dedicated inherited file descriptor.
3. Generate one client entry for every unique mount/hydrate view and one SSR entry for every hydrate view.
4. Run Vite under Bun with the user's `vite.config.ts` for client and SSR builds.
5. Read Vite's manifests as the authoritative entry, chunk, CSS, and asset graph. Compute hashes without renaming output.
6. Run the app with `BIFROST_PHASE=generate` for static records.
7. Render every static document through the Vite SSR output running under Bun.
8. Remove build-only Static SSR entries and keep the transitive imports needed by Server views.
9. Validate all paths, references, versions, sizes, and SHA-256 hashes.
10. Write the Bifrost manifest last and atomically replace the output.
11. Fail the whole build if any required declaration, Vite plugin, output, render, or file fails.

Do not use AST scanning. It misses dynamic declarations, aliases, helper functions, and values from other packages. It also creates a second interpretation of the user's route model.

Do not produce a partial successful build. Optional optimizations may warn and fall back, but missing required JS, server bundles, or HTML is fatal.

## Required invariants

Validation happens before handlers become visible.

1. Every pattern starts with `/` and is accepted by `http.ServeMux`.
2. Page patterns contain no HTTP method or host. Bifrost owns GET and HEAD behavior.
3. Every pattern is unique.
4. Every source view path is clean and within one configured source root.
5. Server routes have no generator.
6. Static routes have no request loader. A dynamic static pattern must have a generator.
7. Client routes have neither loader nor generator.
8. Every manifest route matches one declared route with the same kind and view.
9. Every manifest reference points to a clean, existing file under the build root.
10. Every static output path is absolute, has no query or fragment, is unique, and matches its declared route pattern.
11. Every props value is valid JSON before it reaches the renderer.
12. The manifest schema and spec digest match the running binary.
13. Runtime state is immutable after startup. Development updates replace a whole compiled route atomically.

## Development mode

`bifrost dev` performs one validated bootstrap build, then starts a Bun process containing Vite's development server and the SSR bridge.

- Browser entries load from Vite and use its HMR, React Fast Refresh, CSS updates, plugin watching, and error overlay.
- Server and Static development renders use `vite.ssrLoadModule`, so frontend SSR updates do not rebuild Go.
- Go source or module changes perform a new atomic bootstrap build and restart only the Go child and its Vite bridge.
- Build failures leave the previous Go process active.
- A build-ID poll remains only to reload browsers after a successful Go restart; frontend changes use Vite HMR.

Production and development share route declarations and generated entry contracts. Vite intentionally supplies development modules instead of pretending hashed production artifacts are a development server.

## Layouts and file routing

Do not put layouts in the Go runtime model in the first version. A view is an opaque frontend entry. A React page can import and compose its layout directly.

If file-based routing and nested layouts are added later, a frontend compiler can turn a page plus its layouts into one normalized view and one `ViewID`. The route, manifest, and runtime models above do not need to change.

This is deliberate pushback against copying Next.js too early. Nested layouts, React Server Components, route groups, parallel routes, and intercepting routes would make the core model much larger before Bifrost has evidence that users need them.

## Features left out of the core

These may be added around the model without changing route identity:

- Markdown negotiation.
- Critical CSS extraction.
- Custom error pages.
- Configurable app-wide document defaults beyond the validated per-page attributes.
- File-based route discovery.

A framework enum is also omitted. React is the only implementation. Add a renderer registration point when a second real frontend implementation exists.

## Performance checks

Use the old Bifrost implementation as a comparison, not as a fixed target.

Current local reference for prebuilt-shell string assembly:

```text
~1.0 us/op
2627 B/op
15 allocs/op
```

Required benchmarks:

1. Client handler versus a plain `net/http` handler serving the same bytes.
2. Exact static route versus `http.ServeFile` or embedded `fs.File` serving.
3. Dynamic static route lookup with 1, 100, and 10,000 paths.
4. Server route with no loader, excluding renderer time.
5. Props encoding and shell streaming for small and large props.
6. Shared view across many routes to confirm one build and no request-time view lookup.

Initial success rules:

- Client handling adds no data-model allocation before the `ResponseWriter` work.
- Exact static handling performs no manifest or mode lookup.
- Server props are marshaled once.
- Shell writing does not create a full-document string.
- A shared view compiles once.
- Startup rejects every invalid-state fixture.

Do not set a broad requests-per-second claim until the renderer process and end-to-end response are benchmarked.

## First implementation slice

This vertical slice is implemented:

1. Opaque `Route` plus `Server`, `Static`, and `Client` constructors.
2. Declaration and route-source validation.
3. Minimal `AppPlugin` and typed `AppRegistry` registration for server behavior.
4. Deterministic route `BuildSpec` and digest; frontend extension belongs to Vite.
5. Manifest parsing and strict validation.
6. Runtime compilation into the three specialized handlers.
7. A fake renderer for tests.
8. Benchmarks for client and static handlers with and without hooks.

The completed slices include Vite client/SSR builds, normal Vite plugins, Tailwind, Vite HMR and Fast Refresh, a Bun-hosted Vite SSR bridge, generated embedding, standalone production Bun renderer, static generation, public assets, strict manifests, and browser-level examples. Critical CSS, Markdown, ISR, and public framework adapters remain optional Vite plugins, Go middleware, or versioned core work as appropriate.

## Fixed baseline decisions

- Explicit Go routes in V1; file routing can be a route-source plugin later.
- React only in V1, with a private renderer boundary.
- Server and Static pages hydrate.
- Static generation starts with a slice API.
- SSR streaming is part of the renderer contract.
- The latest two stable Go releases are supported, starting with Go 1.25 as the minimum.
- Production uses a pinned Bun runtime and Unix sockets. Windows is out of scope.
- Module path: `github.com/3-lines-studio/bifrost`; package: `bifrost`; license: MIT.
