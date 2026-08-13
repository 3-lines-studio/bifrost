# Bifrost decision questionnaire

Status: accepted decisions implemented; Vite owns frontend builds and plugins, while typed Go AppPlugins own server extensions

## How to answer

Each question has an ID and a recommended default.

You can answer in any of these forms:

```text
Accept all recommended defaults except P03=B, R05=C, and B08=custom: ...
```

```text
P01: A
P02: C
P03: custom: ...
```

```text
Accept section P. I want to discuss R04 and E07.
```

Priorities:

- **Blocker**: changes the core data model or public API. Resolve before implementation.
- **V1**: resolve before the first usable release.
- **Later**: record intent now; implementation can wait.

Recommendations favor a small, explicit, fast Go library over matching every Next.js feature.

---

# P. Product boundary

## P01 — What is Bifrost? **[Blocker]**

- A. A Go library with an optional build CLI.
- B. A framework that owns project layout, development, build, and serving.
- C. A CLI-first framework with a small Go integration package.

**Recommended: A.** Keep the runtime usable as normal Go code. Use the CLI only where Go alone cannot build frontend assets.

**Answer:** A — Go library with an optional build CLI.

## P02 — Which part of Next.js is the target? **[Blocker]**

- A. Routing, SSR, static generation, client hydration, assets, and development tooling.
- B. A close Next.js clone, including file routing, nested layouts, server components, actions, middleware, and caching.
- C. Only React SSR from Go.
- D. Custom scope.

**Recommended: A.**

**Answer:** A — Routing, SSR, static generation, hydration, assets, and development tooling.

## P03 — Who owns the HTTP server? **[Blocker]**

- A. The user. Bifrost returns or registers an `http.Handler`.
- B. Bifrost. The user configures hooks and API routes through Bifrost.
- C. Support both as equal APIs.

**Recommended: A.**

**Answer:** A — The user owns the HTTP server.

## P04 — Must production be one deployable Go binary? **[Blocker]**

- A. Yes. Embed frontend assets and any required renderer runtime.
- B. Assets may be external, but the renderer must be included.
- C. Bun may be an external production dependency.
- D. Support both embedded and external deployment.

**Recommended: A for the default, D only if it stays a thin configuration layer.**

**Answer:** A — One deployable Go binary by default.

## P05 — Supported deployment targets **[Blocker]**

Select all required for V1:

- A. Linux amd64.
- B. Linux arm64.
- C. macOS development only.
- D. macOS production.
- E. Windows development.
- F. Windows production.
- G. Containers.
- H. Serverless or function platforms.

**Recommended: A, B, C, G.** Windows support changes the renderer process and IPC design.

**Answer:** A, B, C, and G — Linux amd64/arm64 production, macOS development, and containers. No Windows support.

## P06 — Frontend scope **[Blocker]**

- A. React only.
- B. React first, but keep a small private renderer boundary.
- C. Public multi-framework adapter API from V1.

**Recommended: B.** Do not expose a framework abstraction until a second implementation exists.

**Answer:** B — React first with a small private renderer boundary.

## P07 — Compatibility with Bifrost v1 **[Blocker]**

- A. No API compatibility. Provide a migration guide later.
- B. Preserve the current API where it does not damage the model.
- C. Full source compatibility.

**Recommended: A.** This is a from-scratch design.

**Answer:** A — No source compatibility requirement.

## P08 — Main success measure **[Blocker]**

Rank these:

- Simplicity of user code.
- Request latency.
- Throughput.
- Build speed.
- Development reload speed.
- Small binary size.
- Low memory use.
- Feature breadth.

**Recommended order:** simplicity, correctness, request latency, throughput, development reload speed, build speed, memory, binary size, feature breadth.

**Answer:** Simplicity, correctness, request latency, throughput, development reload speed, build speed, memory, binary size, then feature breadth.

---

# A. Public Go API

## A01 — Route declaration style **[Blocker]**

- A. Three constructors: `Server`, `Static`, and `Client`.
- B. One `Page` constructor with functional options.
- C. Public `Route` struct literals.
- D. Builder methods.

**Recommended: A.** Constructors make invalid mode combinations impossible.

**Answer:** A — `Server`, `Static`, and `Client` constructors.

## A02 — Application construction errors **[Blocker]**

- A. `New(...) (*App, error)`.
- B. `New(...) *App` and panic on invalid declarations, like `http.ServeMux` conflicts.
- C. Provide both `New` and `MustNew`.

**Recommended: C**, with `New` returning an error and `MustNew` kept as optional shorthand.

**Answer:** C — `New` returns an error; `MustNew` is optional shorthand.

## A03 — Server loader argument **[Blocker]**

- A. `func(*http.Request) (any, error)`.
- B. `func(context.Context, *http.Request) (any, error)`.
- C. A custom Bifrost request context.
- D. Generic typed loaders.

**Recommended: A.** `http.Request` already carries context and route path values.

**Answer:** A — `func(*http.Request) (any, error)`.

## A04 — Loader output control **[Blocker]**

Should loaders return only props and errors, or control HTTP output?

- A. Props/errors only. Use typed errors for redirects and not-found.
- B. Return a result containing props, status, and headers.
- C. Receive `http.ResponseWriter`.
- D. Support A first and add an advanced result later.

**Recommended: D.** Never pass `ResponseWriter` to a loader; it breaks rendering and error guarantees.

**Answer:** D — Props/errors first; add a typed advanced result later. Never expose `http.ResponseWriter` to loaders.

## A05 — Props type **[Blocker]**

- A. `any`, encoded once by Bifrost.
- B. `json.RawMessage`; users own encoding.
- C. Generic route types.
- D. Support `any` and an advanced raw JSON path.

**Recommended: D**, with `any` as the normal API.

**Answer:** D — `any` by default with an advanced raw JSON path.

## A06 — Static generator API **[Blocker]**

- A. Return `[]StaticPage`.
- B. Return an iterator.
- C. Receive a yield callback.
- D. Support slices first and add streaming only if builds need it.

**Recommended: D.**

**Answer:** D — Slice API first; add streaming only after measured need.

## A07 — Registration with `net/http` **[Blocker]**

- A. `app.Handler()` only.
- B. `app.Register(*http.ServeMux)` plus `app.Handler()`.
- C. A custom router interface for third-party routers.
- D. Middleware wrapping an arbitrary next handler.

**Recommended: B.** Standard `net/http` is the contract. Other routers can mount the returned handler.

**Answer:** B — `Register(*http.ServeMux)` and `Handler()`.

## A08 — App lifecycle **[V1]**

- A. Explicit `Start` and `Close`.
- B. Lazy start on first request and `Close`.
- C. Start during `New`, return errors there, and expose `Close`.

**Recommended: C.** Production should fail before accepting traffic.

**Answer:** C — Start during `New`, fail before traffic, and expose `Close`.

## A09 — Configuration style **[V1]**

- A. `Config` struct passed to `New`.
- B. App-level functional options.
- C. Environment variables.
- D. Config file.

**Recommended: A**, with environment variables used by the CLI, not as hidden runtime policy.

**Answer:** A — Explicit `Config` struct.

## A10 — API stability target **[V1]**

- A. Experiment freely until v1.0.0.
- B. Freeze the public API after the first prototype.
- C. Keep most new types under `internal` until two full examples work.

**Recommended: C, then A until v1.0.0.**

**Answer:** C — Keep types internal until two examples work; API remains experimental until v1.0.0.

---

# R. Routing

## R01 — Route source **[Blocker]**

- A. Explicit Go declarations only.
- B. File-based routes only.
- C. Explicit routes in V1; file routing may compile into the same model later.
- D. Both in V1.

**Recommended: C.**

**Answer:** C — Explicit routes in V1; file routing may compile into the same model later.

## R02 — Pattern syntax **[Blocker]**

- A. Go 1.22+ `http.ServeMux` path patterns.
- B. Bifrost syntax such as `/blog/:slug`.
- C. Third-party router syntax.

**Recommended: A.** Do not maintain a second router.

**Answer:** A — Go `http.ServeMux` patterns.

## R03 — HTTP methods **[Blocker]**

- A. Pages own GET; HEAD follows GET. APIs stay outside Bifrost.
- B. Page declarations may include any method.
- C. Bifrost also models API routes.

**Recommended: A.**

**Answer:** A — Pages own GET and HEAD only.

## R04 — Route identity **[Blocker]**

- A. Exact URL pattern.
- B. User-supplied route ID.
- C. Component path.
- D. Generated numeric ID.

**Recommended: A.** A component may serve several routes.

**Answer:** A — Exact URL pattern.

## R05 — Trailing slashes **[Blocker]**

- A. Preserve standard `ServeMux` behavior.
- B. Always redirect to no trailing slash.
- C. Always redirect to a trailing slash.
- D. Configurable global policy.

**Recommended: A initially.**

**Answer:** A — Standard `ServeMux` behavior.

## R06 — URL normalization **[Blocker]**

Choose required behavior:

- Clean duplicate slashes.
- Reject `..` path segments.
- Ignore query strings for route identity.
- Preserve percent-encoded path segments.
- Case-sensitive matching.

**Recommended:** follow `net/http` and `ServeMux`; add only strict static-output validation.

**Answer:** Follow `net/http` and `ServeMux`; add strict static-output validation only.

## R07 — Static generated path validation **[Blocker]**

- A. Every generated path must match its declared route or the build fails.
- B. Allow a generator to emit paths for any static route.
- C. Warn and skip mismatches.

**Recommended: A.**

**Answer:** A — A generated path must match its route or the build fails.

## R08 — Duplicate patterns **[Blocker]**

- A. Startup/build error.
- B. Last declaration wins.
- C. Let `ServeMux` panic.

**Recommended: A**, before registration.

**Answer:** A — Return a validation error before registration.

## R09 — Route groups and nested routing **[Later]**

- A. Not planned.
- B. Frontend/file-routing concern only.
- C. Core Go model in a later release.
- D. Required in V1.

**Recommended: B.**

**Answer:** B — Keep route groups and nesting in a future frontend/file-routing layer.

## R10 — Client-side navigation **[Blocker]**

- A. Full document navigation only.
- B. Bifrost intercepts links and fetches page data for SPA-like navigation.
- C. React Router owns client navigation on client-only pages.
- D. A in V1, consider B later.

**Recommended: D.** Client navigation adds a second routing and data protocol.

**Answer:** D — Full document navigation in V1; reconsider Bifrost client navigation later.

---

# E. Rendering and data semantics

## E01 — Initial page kinds **[Blocker]**

- A. Server, Static, Client.
- B. Add server-only HTML with no browser JavaScript.
- C. Add incremental static regeneration.
- D. Add streaming server components.

**Recommended: A.**

**Answer:** A — Server, Static, and Client.

## E02 — Server hydration **[Blocker]**

- A. All Server pages hydrate.
- B. Hydration is optional per route.
- C. Server pages never hydrate unless enabled.

**Recommended: A for V1.** Optional hydration creates another build variant.

**Answer:** A — Server pages hydrate.

## E03 — Static hydration **[Blocker]**

- A. All Static pages hydrate.
- B. Hydration is optional per route.
- C. Static pages default to no JavaScript.

**Recommended: A for V1.**

**Answer:** A — Static pages hydrate.

## E04 — SSR streaming **[Blocker]**

- A. Required from the first renderer implementation.
- B. Start buffered, then add streaming without changing the public API.
- C. Buffered rendering only.

**Recommended: A** if Suspense streaming is a core promise; otherwise B is the smaller implementation.

**Answer:** A — Streaming is part of the renderer contract from the first implementation.

## E05 — Head rendering **[Blocker]**

- A. A named `Head` export in the page module.
- B. React-native metadata API designed by Bifrost.
- C. Go route metadata.
- D. Static HTML template only.

**Recommended: A initially.**

**Answer:** A — Named `Head` export.

## E06 — Layouts **[Blocker]**

- A. Pages import layouts themselves.
- B. Route declarations list layouts.
- C. File routing discovers nested layouts.
- D. Required Next.js-like nested layouts in V1.

**Recommended: A.** Keep layout composition out of the Go runtime model.

**Answer:** A — Pages compose their own layouts.

## E07 — Loader errors **[Blocker]**

Which outcomes must be first-class?

- Ordinary 500 error.
- Redirect with URL and status.
- Not found.
- Unauthorized/forbidden.
- Custom status and headers.
- Retryable renderer failure.

**Recommended for V1:** 500, redirect, and not-found. Treat auth as ordinary status/error helpers later.

**Answer:** First-class 500, redirect, and not-found outcomes in V1.

## E08 — Loader execution **[V1]**

- A. One call per request, with no built-in cache.
- B. Built-in request deduplication.
- C. Built-in cross-request cache.
- D. User owns all caching.

**Recommended: A and D.** Bifrost should not hide application cache policy.

**Answer:** A and D — One loader call per request; users own caching.

## E09 — Request cancellation **[V1]**

- A. Cancel loader and renderer when `Request.Context()` ends.
- B. Let rendering finish for possible cache use.
- C. Configurable.

**Recommended: A.**

**Answer:** A — Cancel loader and renderer with the request context.

## E10 — Render timeout **[V1]**

- A. No Bifrost timeout; server context controls it.
- B. Global Bifrost timeout.
- C. Per-route timeout.
- D. Global default with per-route override.

**Recommended: A initially.** Avoid overlapping timeout policy with `http.Server`.

**Answer:** A — HTTP server context owns timeouts initially.

## E11 — Props JSON contract **[Blocker]**

- A. Standard `encoding/json` values only.
- B. Add tagged support for dates, big integers, maps, and custom JS values.
- C. User-defined codec.

**Recommended: A.** Document exact escaping and unsupported values.

**Answer:** A — Standard `encoding/json` values.

## E12 — Props exposure **[Blocker]**

- A. Hydrated props appear in page HTML as escaped JSON.
- B. Fetch props from a separate endpoint after load.
- C. Encrypt or sign embedded props.

**Recommended: A.** State clearly that secrets must never be returned as props.

**Answer:** A — Embed correctly escaped props in HTML; props must contain no secrets.

## E13 — Markdown responses **[Later]**

- A. Remove.
- B. Keep content negotiation in V1.
- C. Add later as middleware outside the core.

**Recommended: C.**

**Answer:** C — Add Markdown later as middleware or a plugin, outside core routing.

## E14 — Incremental static regeneration **[Later]**

- A. Not planned.
- B. Add later with explicit stale/revalidate rules.
- C. Required in V1.

**Recommended: B.** Do not let ISR affect the first static model.

**Answer:** B — Add ISR later with explicit semantics.

---

# F. Frontend module contract

## F01 — Page export **[Blocker]**

- A. Named `Page` export.
- B. Default export.
- C. Support both.

**Recommended: A** if preserving the current clear contract; C is friendlier but adds ambiguity.

**Answer:** A — Named `Page` export.

## F02 — Head export **[Blocker]**

- A. Named `Head` component receiving page props.
- B. Metadata object/function.
- C. Both.

**Recommended: A.**

**Answer:** A — Named `Head` component receiving props.

## F03 — Error boundary export **[Later]**

- A. Use normal React error boundaries inside the page.
- B. Support a named route-level error export.
- C. Global Bifrost error page only.

**Recommended: A and C initially.**

**Answer:** A and C — Normal React boundaries plus a global Bifrost error page initially.

## F04 — Loading UI and Suspense **[V1]**

- A. Normal React Suspense only.
- B. Named loading module/export managed by Bifrost.
- C. No streaming Suspense guarantee.

**Recommended: A if E04 chooses streaming.**

**Answer:** A — Normal React Suspense under the streaming contract.

## F05 — CSS handling **[Blocker]**

Select required V1 inputs:

- Plain CSS imports.
- CSS Modules.
- Tailwind or PostCSS.
- CSS-in-JS.
- Sass/Less.

**Recommended:** plain CSS imports and CSS Modules first. Treat other tools as external build plugins only after demand.

**Answer:** Vite owns CSS. Plain CSS and CSS Modules work directly; Tailwind, PostCSS, and other processors use normal Vite plugins in `vite.config.ts`.

## F06 — Critical CSS **[V1]**

- A. Remove from V1.
- B. Keep automatic extraction.
- C. Optional build optimization.

**Recommended: A or C.** It must never affect correctness.

**Answer:** A — Automatic critical CSS is removed from V1. It may return as an optional build plugin and can never affect correctness.

## F07 — Asset imports **[V1]**

Select required behavior:

- Images imported from TS/TSX.
- Fonts imported from CSS.
- JSON imports.
- SVG as URL.
- SVG as React component.

**Recommended:** image/font URLs, JSON, and SVG URLs. Defer SVG component transforms.

**Answer:** Image and font URLs, JSON, and SVG URLs. Defer SVG component transforms.

## F08 — Browser targets **[V1]**

- A. Modern evergreen browsers only.
- B. Configurable target list.
- C. Legacy browser support.

**Recommended: A initially.**

**Answer:** A — Modern evergreen browsers.

## F09 — TypeScript **[Blocker]**

- A. TS/TSX required and first-class; JS/JSX also accepted.
- B. TypeScript only.
- C. JavaScript only.

**Recommended: A.**

**Answer:** A — TS/TSX first-class; JS/JSX accepted.

## F10 — React version ownership **[Blocker]**

- A. User's project owns React versions.
- B. Bifrost embeds fixed React versions.
- C. Bifrost declares a supported version range and validates it.

**Recommended: C**, while resolving React from the user project.

**Answer:** C — User project supplies React within a validated supported range. Vite and Bun versions are also pinned to supported majors and recorded in the manifest.

---

# B. Build and artifacts

## B01 — Route discovery **[Blocker]**

- A. Run the app in a describe phase over a dedicated protocol FD.
- B. Parse Go source.
- C. Require a separate static route config file.
- D. Generate route metadata through `go generate`.

**Recommended: A.** It uses the same declarations as runtime.

**Answer:** A — Run the app in a describe phase over a dedicated protocol file descriptor.

## B02 — Describe-phase side effects **[Blocker]**

- A. The user must construct routes without connecting to databases or starting services.
- B. Bifrost must discover routes without executing normal `main` setup.
- C. Provide a separate exported build function.
- D. Custom approach.

**Recommended: B**, enabled by checking build phase before application services start. This needs a clear entrypoint contract.

**Answer:** B — Detect build phase before application services start.

## B03 — Build entrypoint **[Blocker]**

- A. CLI takes a main Go file, as today.
- B. CLI takes a package path such as `./cmd/web`.
- C. CLI takes a built binary.
- D. Support package path and built binary.

**Recommended: B**, with C as an internal optimization.

**Answer:** B — CLI accepts a Go package path.

## B04 — Production artifact location **[V1]**

- A. `.bifrost/` under the app root.
- B. `dist/`.
- C. Configurable output directory.
- D. Go generated package instead of files.

**Recommended: A with a CLI override.**

**Answer:** A — `.bifrost/` by default with a CLI override.

## B05 — Embedding **[Blocker]**

- A. User writes `//go:embed all:.bifrost`.
- B. CLI generates a Go package that embeds output.
- C. Runtime reads files from disk.
- D. Support generated embedding and disk mode.

**Recommended: B for the default, D if implementation stays small.** Generated embedding removes manual placeholder problems.

**Answer:** B — Generate a Go package that embeds output.

## B06 — Build determinism **[V1]**

- A. Same source and tool versions must produce byte-identical manifest and assets where the bundler permits it.
- B. Stable behavior matters, but byte-identical output does not.

**Recommended: A.** Sort all model output and avoid timestamps in artifacts.

**Answer:** A — Deterministic output where tools permit it.

## B07 — Stale artifact behavior **[Blocker]**

- A. Production startup fails if app spec and manifest digests differ.
- B. Warn and continue.
- C. Rebuild automatically in production.

**Recommended: A.**

**Answer:** A — Fail startup on stale artifacts.

## B08 — Build failure policy **[Blocker]**

- A. Any required route failure fails the whole build.
- B. Emit partial output with warnings.
- C. Configurable.

**Recommended: A.** Optional optimizations may fall back, but required output may not.

**Answer:** A — Any required route failure fails the build.

## B09 — Static generation isolation **[V1]**

- A. Run all generators in the app process.
- B. Run each route generator in a fresh process.
- C. Worker pool of app processes.

**Recommended: A initially.** Add isolation only if leaks or crashes become a real problem.

**Answer:** A — Run generators in the app process initially.

## B10 — Static generation concurrency **[V1]**

- A. Sequential.
- B. Fixed worker count.
- C. Configurable worker count with a conservative default.

**Recommended: C.** Generator execution may hit databases or APIs.

**Answer:** C — Configurable worker count with a conservative default.

## B11 — Public directory **[V1]**

- A. Keep a root `public/` copied as-is.
- B. Require explicit asset registration.
- C. Bundler imports only; no public directory.

**Recommended: A.**

**Answer:** A — Keep root `public/` copied as-is.

## B12 — Manifest format **[V1]**

- A. Versioned JSON for inspection.
- B. Binary format for startup speed.
- C. Generated Go data.

**Recommended: A.** Parse once at startup; readability is more useful than small parse savings.

**Answer:** A — Versioned JSON manifest.

## B13 — Source maps **[V1]**

- A. Development only.
- B. Development and production.
- C. Configurable production generation.

**Recommended: C, default off in production.**

**Answer:** C — Production source maps configurable and off by default.

## B14 — Package installation **[V1]**

- A. Require Bun and an existing `package.json`.
- B. Bifrost manages a hidden frontend package.
- C. Bifrost initializes files, then the user owns them.

**Recommended: C.** Hidden package ownership becomes hard to debug.

**Answer:** C — Bifrost initializes package files; users own them.

## B15 — Frontend build owner **[V1 revision]**

- A. Maintain a custom Bun bundler integration and custom frontend plugin API.
- B. Use Vite for client/SSR builds, CSS, assets, manifests, plugins, and development; use Bun only to run Vite and execute SSR.

**Answer:** B — Vite is the sole frontend build and development authority. Bifrost generates entries, enforces output invariants, validates Vite manifests, and never renames Vite output. Go remains responsible for server declarations, HTTP, props, static generation, embedding, and runtime process control.

---

# D. Development experience

## D01 — Development command **[V1]**

- A. `bifrost dev ./cmd/web` owns Go rebuild and proxying.
- B. User runs Go and Bun separately.
- C. Support both, with A as convenience.

**Recommended: C.**

**Answer:** C — Support a convenient `bifrost dev` command and separate user-run processes.

## D02 — Go changes **[V1]**

- A. Rebuild and restart the child process.
- B. Plugin-based hot replacement.
- C. Leave restart tooling to the user.

**Recommended: A.**

**Answer:** A — Rebuild and restart the Go child.

## D03 — Frontend changes **[V1]**

- A. Full browser reload.
- B. React Fast Refresh.
- C. Server recompiles on next request with no browser reload protocol.
- D. A first, B later.

**Recommended: D.**

**Answer:** Vite now owns frontend updates, HMR, CSS updates, React Fast Refresh, and its browser error overlay. Full reload remains only for successful Go child replacement.

## D04 — Failed frontend rebuild **[V1]**

- A. Keep the last good handler and show an error overlay.
- B. Replace the route with an error handler.
- C. Return the build error only on the affected request.

**Recommended: A.**

**Answer:** A — Keep the last good handler and report the new error.

## D05 — Compile timing **[V1]**

- A. Compile every view at dev startup.
- B. Compile on first request.
- C. Compile changed views eagerly after file events.
- D. B initially, then C after first use.

**Recommended: D.**

**Answer:** Compile all declared views once in a validated bootstrap build, then let Vite compile and invalidate frontend modules on demand through its development module graph. Go changes repeat the atomic bootstrap build.

## D06 — Browser error overlay **[V1]**

- A. Required.
- B. Terminal errors are enough initially.
- C. Add after core runtime works.

**Recommended: C.**

**Answer:** Use Vite's browser error overlay rather than implementing a Bifrost-specific overlay.

## D07 — Development asset behavior **[V1]**

- A. Compiler returns the same artifact model as production.
- B. Use inferred `/dist/{name}` paths.
- C. Serve directly from Bun's development server.

**Recommended: A.**

**Answer:** Route and generated-entry contracts stay shared. Production consumes strict Vite manifests; development intentionally consumes Vite's live module graph and SSR loader.

## D08 — Route table and diagnostics **[V1]**

- A. Print by default on a terminal.
- B. Log only when requested.
- C. Expose a diagnostics API and let the CLI print it.

**Recommended: C**, with the dev CLI printing a concise table.

**Answer:** C — Expose diagnostics; the dev CLI prints a concise route table.

---

# O. Runtime, operations, and security

## O01 — Renderer process model **[Blocker]**

- A. One long-lived Bun process.
- B. Fixed worker pool.
- C. One process per request.
- D. One process initially, pool later behind the same interface.

**Recommended: D.** Measure before adding pool policy.

**Answer:** D — One long-lived Bun process first; preserve an interface for a measured worker pool later.

## O02 — Renderer IPC **[Blocker]**

- A. Unix socket.
- B. stdin/stdout pipes.
- C. TCP loopback.
- D. Platform-specific best transport behind one private interface.

**Recommended: B for simplicity if it supports streaming and restart cleanly; otherwise D.** Unix sockets block Windows.

**Answer:** A — Unix sockets. Windows is out of scope, so use the simple existing Unix model.

## O03 — Renderer wire format **[V1]**

- A. NDJSON.
- B. Length-prefixed binary frames with JSON payloads where useful.
- C. HTTP.
- D. Benchmark A and B with realistic page sizes before choosing.

**Recommended: D.**

**Answer:** B — Benchmarks chose length-prefixed binary response frames. Local decoder results: NDJSON ~126 µs and 110 allocations versus binary ~0.49 µs and 35 allocations for a 32 KiB streamed response. Render requests remain JSON because there is one request frame per render.

## O04 — Renderer crash behavior **[V1]**

- A. Fail current requests, restart automatically, and apply a restart limit.
- B. Terminate the Go app.
- C. Stay down until manually restarted.

**Recommended: A.**

**Answer:** A — Fail active requests, restart with a limit, and report the failure.

## O05 — Overload behavior **[V1]**

- A. Unbounded renderer queue.
- B. Bounded queue, then return 503.
- C. Block until request context ends.
- D. Configurable bounded concurrency and queue.

**Recommended: D.** Unbounded queues cause memory failure.

**Answer:** D — Bounded configurable concurrency and queue.

## O06 — Logging **[V1]**

- A. Use `log/slog`.
- B. No library logs; return all events to callbacks.
- C. `slog` with an optional injected logger.

**Recommended: C.** Do not log every request by default.

**Answer:** C — `slog` with an optional injected logger; no request log by default.

## O07 — Metrics and tracing **[Later]**

- A. Expose hooks around load, queue, render, and write phases.
- B. Built-in Prometheus metrics.
- C. Built-in OpenTelemetry.
- D. No observability API.

**Recommended: A first.** Keep vendor choices outside core.

**Answer:** A — Typed hooks around load, queue, render, and write phases.

## O08 — Asset cache headers **[V1]**

- A. Hashed build assets get immutable long cache; public files get conservative cache.
- B. User configures every header.
- C. No cache headers.

**Recommended: A**, with an override hook.

**Answer:** A — Immutable caching for hashed assets with an override hook.

## O09 — Compression **[V1]**

- A. Leave compression to the HTTP server or reverse proxy.
- B. Built-in gzip.
- C. Prebuild gzip and Brotli variants.

**Recommended: A.**

**Answer:** A — Compression belongs to the server or reverse proxy.

## O10 — Content Security Policy **[V1]**

- A. Correctly escape inline props, but leave CSP to the user.
- B. Built-in per-request nonces.
- C. Avoid inline scripts and styles entirely.
- D. A first, design hooks so B can be added.

**Recommended: D.**

**Answer:** D — Escape correctly now and preserve typed hooks for nonce support.

## O11 — Subresource integrity **[Later]**

- A. Not needed for same-origin embedded assets.
- B. Add integrity hashes to scripts and styles.
- C. Required in V1.

**Recommended: A.** Manifest hashes still serve build validation and ETags.

**Answer:** A — No SRI requirement for same-origin embedded assets.

## O12 — Graceful shutdown **[V1]**

- A. `App.Close(ctx)` drains renderer work and stops children.
- B. `App.Close()` stops immediately.
- C. User kills child processes through the CLI.

**Recommended: A.**

**Answer:** A — Context-aware graceful close.

## O13 — HTML and props size limits **[V1]**

- A. No Bifrost limits.
- B. Configurable maximum props, head, and buffered frame sizes.
- C. Fixed safe limits.

**Recommended: B.** Streaming body size should not need a fixed limit.

**Answer:** B — Configurable props, head, and frame limits.

## O14 — Public file safety **[V1]**

- A. Reject all manifest paths that escape the build root and serve only known roots.
- B. Trust generated manifests.
- C. Allow arbitrary configured filesystem paths.

**Recommended: A.**

**Answer:** A — Serve only validated files under known roots.

---

# C. Compatibility, quality, and release

## C01 — Minimum Go version **[Blocker]**

- A. Go 1.25.
- B. Go 1.26.
- C. Oldest version that supports required `ServeMux` behavior.
- D. Latest two stable Go releases.

**Recommended: D**, expressed as one concrete minimum in `go.mod` after checking dependencies.

**Answer:** D — Support the latest two stable Go releases. Start with Go 1.25 as the concrete minimum unless dependencies force newer.

## C02 — Bun version policy **[Blocker]**

- A. Pin one exact embedded Bun version.
- B. Support a tested version range.
- C. Always use the user's installed Bun.
- D. Pin production builds; allow a compatible local version in development.

**Recommended: D.** Record exact tool versions in the manifest.

**Answer:** D — Pin production Bun; permit a validated compatible local Bun in development.

## C03 — Performance acceptance **[Blocker]**

Choose concrete priorities or limits:

- Client route overhead versus plain `net/http`.
- Static route overhead versus `http.FileServer`.
- SSR Go overhead excluding loader and JS renderer.
- Maximum startup time for 100/1,000 routes.
- Maximum idle renderer memory.
- Maximum binary growth from embedded runtime.

**Recommended:** first collect baselines, then set limits. Do not optimize only a shell microbenchmark.

**Answer:** Baselines collected and recorded in IMPLEMENTATION.md. Regression limits: client handler under 250 ns and 8 allocations, 1,000-route model startup under 5 ms, static lookup under 1 µs at 10,000 paths, and no unbounded renderer queue. Binary and idle memory are tracked but not hard-failed because the pinned Bun runtime dominates them.

## C04 — Required test levels **[V1]**

Select all:

- Unit tests for model validation.
- Fuzz tests for paths, manifests, and HTML/JSON escaping.
- Race tests.
- Benchmarks.
- Go-to-renderer integration tests.
- Browser end-to-end tests.
- Cross-platform tests.
- Reproducible build tests.

**Recommended:** all except unsupported production platforms.

**Answer:** All listed test levels on supported platforms; no Windows jobs.

## C05 — Correctness policy **[Blocker]**

- A. Fail fast on invalid declarations, stale builds, missing files, and invalid static paths.
- B. Warn and use fallbacks where possible.
- C. Strict production, forgiving development.

**Recommended: C**, but development may keep the last good build rather than invent missing output.

**Answer:** C — Strict production; development may retain the last good build but must not invent output.

## C06 — Repository shape **[V1]**

- A. One Go module containing library and CLI.
- B. Separate modules for runtime, CLI, and frontend package.
- C. Monorepo with one Go module and one internal JS package.

**Recommended: C initially.** Split only when versioning needs differ.

**Answer:** C — Monorepo with one Go module and one internal JS package.

## C07 — License and package path **[V1]**

Confirm:

- Module path.
- Package name.
- License.
- Whether the module uses a `/v2` suffix.

**Recommended:** settle these before external imports or generated files depend on them.

**Answer:** Module `github.com/3-lines-studio/bifrost`, package `bifrost`, MIT license, no `/v2` suffix; v1 module path from the start.

## C08 — First milestone **[Blocker]**

- A. Pure model and fake-renderer prototype.
- B. One working SSR page immediately.
- C. Full replacement for current Bifrost.
- D. Model prototype, then one vertical SSR/client/static example.

**Recommended: D.** The model prototype should stay small and reversible.

**Answer:** D — Model prototype, then one vertical Server/Static/Client example.

---

# Suggested answer order

Resolve these first because they change the model:

```text
P01 P02 P03 P04 P05 P06 P07 P08
A01 A02 A03 A04 A05 A06 A07
R01 R02 R03 R04 R05 R07 R10
E01 E02 E03 E04 E05 E06 E07 E11 E12
F01 F02 F05 F09 F10
B01 B02 B03 B05 B07 B08
O01 O02
C01 C02 C03 C05 C08
```

After those answers, rewrite `DESIGN.md` as agreed decisions rather than recommendations. Then implement the smallest model slice and benchmark it before adding Bun or CLI code.
