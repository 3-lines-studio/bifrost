# Implementation status

This file separates the accepted scope from ideas explicitly deferred by the questionnaire.

## Implemented scope

### Public API and model

- `Config`, `New`, `MustNew`, `App`, `Handler`, `Register`, `Close`, and `Diagnostics`.
- Opaque `Route` with `Server`, `Static`, and `Client` constructors.
- Standard `http.ServeMux` patterns, GET/HEAD ownership, path values, and conflict checks.
- Typed loader, static generator, `RawProps`, redirect, and not-found APIs.
- Strict declaration, source path, generated path, props, and route ownership validation.
- Immutable declaration, build-spec, manifest, and specialized runtime models.

### Plugins and hooks

- Normal Vite frontend plugins through `vite.config.ts`, including Tailwind, React Fast Refresh, virtual modules, CSS transforms, assets, and development HMR.
- Named Go `AppPlugin`s with no global registry for server routes, middleware, error and asset-header policy, and runtime observation.
- Typed load, renderer-queue, render, and response hooks.
- `AppRegistry` sealing, unique plugin names, deterministic order, and startup failure on registration errors.
- Frontend build metadata was removed from Go; Vite is the sole frontend extension system.

### Build

- Package-based `bifrost build` command.
- Dedicated describe and static-generation protocol file descriptor.
- `bifrost.Building()` guard so user startup side effects do not run during builds.
- No Go AST route scanning.
- Unique hydrate/mount view planning and shared-view deduplication.
- Vite 8 client and SSR builds running under Bun, with the user's standard Vite configuration and plugins.
- Vite manifests as the authoritative entry, shared-chunk, CSS, dynamic-import, and asset graph.
- Shared client and SSR chunks, CSS Modules, Tailwind, image/font/SVG/JSON assets, and virtual modules through Vite.
- Go computes hashes and validates Vite output but never renames it.
- Configurable inline source maps.
- Concurrent static rendering with a configurable worker count.
- Strict all-or-nothing build failures.
- Public directory copying with route ownership checks.
- Deterministic, versioned JSON manifest written last.
- Stale app/manifest digest rejection.
- Generated, formatted Go embedding file.
- Atomic output replacement.
- Reproducible production output test.

### Runtime

- One default deployable Go binary.
- Gzip-compressed embedded standalone Bun renderer.
- Linux Unix-socket renderer transport.
- JSON request followed by length-prefixed binary head/body/done/error frames.
- SSR head and body streaming.
- Request-scoped props and validated root document language, class, and direction.
- One isolated renderer process by default, with an explicit multi-process worker pool for production concurrency; JavaScript module globals persist only between sequential requests on the same worker.
- End-to-end request cancellation through the Go transport, Bun request signal, React render signal, and body reader.
- Bounded renderer concurrency and queue with 503 overload response.
- Public renderer readiness checks across every worker.
- Renderer restart after transport/process failure, capped at five attempts per minute.
- Context-aware shutdown that blocks new renderer work, drains active work, and then stops children; parent-death handling on Linux.
- Props encoded once, validated, size-limited, and safely embedded for hydration.
- Configurable props, head, and frame limits.
- Server errors before commit and logging after commit.
- Static and Client handlers with no renderer work.
- Strict artifact path, size, and SHA-256 verification.
- Hashed immutable assets, ETags, conditional requests, public assets, and header hooks.
- Low-priority module preloads prevent the client graph from competing with high-priority LCP resources.
- Compression remains the HTTP server, CDN, or reverse proxy's job.

### Development and CLI

- `bifrost init`, `build`, `dev`, and `version`.
- One validated bootstrap build followed by a Bun-hosted Vite development server and SSR bridge; development binaries embed neither production SSR output nor the standalone renderer.
- Vite HMR, React Fast Refresh, CSS updates, plugin watching, development source maps, and browser overlay.
- Server and Static development renders use Vite's live SSR module graph.
- Frontend changes do not rebuild Go; Go/module changes atomically rebuild and replace the Go child.
- Last good server remains active when a Go rebuild fails.
- Build-ID polling performs full reload only after Go replacement.
- Route table output.
- Three complete examples: mixed Server/Static/Client/Tailwind, static/client with Go app plugins, and a structured monorepo app.
- The structured example covers internal app packages, generated-asset injection, standard `ServeMux` fallback composition, shared middleware, Vite aliases, a linked workspace package, Tailwind workspace scanning, and React Compiler.
- Fresh scaffolds include Vite, React, and Tailwind configuration and compile before their first build.

### Quality checks

- Unit tests for declarations, plugins, manifests, paths, build phases, renderer framing, handlers, hooks, errors, assets, and static generation.
- Race tests and `go vet`.
- Fuzz targets for manifests, static paths, and raw props.
- Request and startup benchmarks.
- Vite/Bun/React end-to-end builds and real Chromium checks for Tailwind, virtual-module plugins, CSS Modules, assets, hydration, Suspense, renderer-kill recovery, and HTTP behavior.
- Development browser tests covering Vite HMR, live SSR invalidation, Go process replacement, build-ID polling, and full reload.
- Static-only build check confirming no production renderer is embedded.
- Linux arm64 and macOS arm64 compile checks.
- Reproducible build check.

## Measured local reference

AMD Ryzen 5 9600X, Go 1.26.5 toolchain, Bun 1.3.14:

- Client handler: about 105 ns/op, 48 B/op, 3 allocs/op; the equivalent plain handler is about 104 ns/op with the same headers.
- Client handler with a response hook: about 185 ns/op, 112 B/op, 4 allocs/op.
- Dynamic static lookup and serve: about 333–353 ns/op for 1–10,000 paths, 139–140 B/op, 7 allocs/op.
- Server Go-side handler overhead with fake renderer and document-metadata handling: about 246 ns/op.
- Model startup: about 132 µs for 100 routes and 1.35 ms for 1,000 routes.
- 32 KiB response frame decode: NDJSON about 140 µs/110 allocs; binary about 0.52 µs/35 allocs.
- Mixed Vite SSR binary: about 45.0 MB after renderer compression and removal of build-only Static entries.
- Static/client-only Vite binary: about 7.4 MB; it embeds neither the renderer nor SSR output.
- Idle local RSS: about 55 MB for Go plus 40 MB for the renderer process.

These are regression references, not broad throughput claims.

## Explicitly deferred, not missing work

The questionnaire marked these as later work, optional plugins, or features requiring evidence:

- File-based routing, route groups, and nested layout discovery.
- Bifrost-owned client-side navigation.
- React Server Components, server actions, and new route kinds.
- Incremental static regeneration.
- Markdown negotiation.
- Automatic critical CSS.
- Built-in Prometheus or OpenTelemetry adapters.
- Built-in CSP nonce rewriting and SRI.
- A public multi-framework renderer API.
- Serverless runtime ownership.
- Windows support.

Vite plugins own frontend extensions. The typed Go `AppRegistry` can gain additive server hooks. ISR, RSC, and new route or render kinds still require a versioned core decision rather than an unrestricted server escape hatch.
