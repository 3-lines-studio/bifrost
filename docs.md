# Bifrost Documentation

Server-side rendering for React components in Go.

## Overview

Bifrost bridges Go backends with React frontends. Sobek is the default in-process JavaScript backend; Bun is an optional higher-throughput backend.

## Installation

```bash
go get github.com/3-lines-studio/bifrost
go install github.com/3-lines-studio/bifrost/cmd/bifrost@latest
```

Install JavaScript packages in `node_modules` with npm, pnpm, Bun, or another package manager.

**Runtime requirements:**
- **Default development and builds**: Pure Go with Sobek; Bun is not required
- **Default production SSR**: Sobek runs inside the Go process
- **Optional Bun backend**: Bun is required during development and builds; its runtime is embedded in production binaries
- **Production static-only**: No JavaScript runtime is active after export

## Architecture

Bifrost is organized into focused internal packages:

- `internal/core` — Shared types (`PageConfig`, `PropsLoader`, `RedirectError`), manifest and HTML shell, page routing decisions, critical CSS, MIME/path helpers
- `internal/usecase` — Build and page-serve orchestration (wiring core to adapters)
- `internal/adapters/http` — Page and asset HTTP handlers
- `internal/adapters/process` — Optional Bun renderer process and bundle IPC
- `internal/adapters/esbuild` — Default esbuild-Go and Tailwind build pipeline
- `internal/adapters/sobek` — Default in-process JavaScript renderer and worker pool
- `internal/adapters/runtime` — Backend selection and renderer lifecycle
- `internal/adapters/fs` — Filesystem and embed abstractions
- `internal/adapters/react` — React entry templates and runtime source
- `internal/adapters/cli` — Terminal output and build reports
- Root `bifrost` package — Public `App` API (`New`, `Page`, `Wrap`, …)

### Go vs TypeScript Boundary

TypeScript is intentionally minimal in Bifrost. The default Sobek backend uses Go for build orchestration, esbuild, Tailwind scanning, runtime pooling, and IPC-free rendering. TypeScript remains only in generated React entries and the optional Bun adapter.

- Behavior shared by both backends belongs in Go:
  - build orchestration and fallback behavior
  - manifest generation
  - artifact path normalization
  - filesystem layout decisions and cleanup
  - validation and user-facing build errors

If a behavior can be implemented in Go with equivalent correctness, it must live in Go, not in TypeScript.

## Quick Start

```go
package main

import (
    "context"
    "embed"
    "errors"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/3-lines-studio/bifrost"
)

//go:embed all:.bifrost
var bifrostFS embed.FS

func main() {
    // Create Bifrost app
    app, err := bifrost.New(
        bifrostFS,
        bifrost.Page("/{$}", "./pages/home.tsx",
            bifrost.WithLoader(func(req *http.Request) (any, error) {
                return map[string]any{
                    "name": "World",
                }, nil
            }),
        ),
        bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
    )
    if err != nil {
        log.Fatalf("create app: %v", err)
    }
    defer app.Stop()

    api := http.NewServeMux()
    server := &http.Server{
        Addr:              ":8080",
        Handler:           app.Wrap(api),
        ReadHeaderTimeout: 5 * time.Second,
    }
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()
    go func() {
        <-ctx.Done()
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        if err := server.Shutdown(shutdownCtx); err != nil {
            log.Printf("server shutdown: %v", err)
        }
    }()

    if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        log.Printf("server stopped: %v", err)
    }
}
```

### Mode Detection

Bifrost determines mode by checking the `BIFROST_DEV` environment variable:

| Variable | Mode | Behavior |
|----------|------|----------|
| `BIFROST_DEV=1` | Development | Source component files, hot reload, no embed required |
| Unset or other | Production | SSR bundles from embed.FS, strict validation |

### Development Mode

```bash
bifrost dev ./main.go
```

`bifrost dev` sets `BIFROST_DEV=1`, builds and restarts on `.go` changes, reverse-proxies `:<port>` → `:8080`, and relies on Bun's per-request module re-import for frontend hot reload.

### Production Mode

```bash
# Build assets — output goes to dir(main.go)/.bifrost (the "app root")
bifrost build ./main.go

# Build Go binary
go build -o myapp main.go

# Run (no BIFROST_DEV set)
./myapp
```

`go install github.com/3-lines-studio/bifrost/cmd/bifrost@latest` installs a binary named `bifrost`; run `bifrost init`, `bifrost dev`, `bifrost build`, or `bifrost doctor`.

Requirements:
- `embed.FS` is **mandatory** — `New` returns an error if it is missing
- `.bifrost/manifest.json` must exist in embedded assets
- SSR bundles extracted from `.bifrost/ssr/` in embed.FS (SSR pages only)
- Embedded Bun runtime included only for SSR pages
- Source component files are **never** used

**Static-only apps** (WithClient or WithStatic only):
- No Bun runtime embedded
- Smaller binary size
- Cannot serve SSR pages

**SSR apps** (with at least one SSR page):
- Bun runtime automatically embedded
- Required for server-side rendering

`New` returns an error on:
- Missing `embed.FS` in production
- Missing or invalid `manifest.json` in embedded assets
- Missing Bun or embedded runtime when SSR needs it
- Conflicting page modes for one shared component

## React Support

Bifrost v1 supports React. Use `.tsx` components with `bifrost.New()`:

```go
app, err := bifrost.New(bifrostFS,
    bifrost.Page("/{$}", "./pages/home.tsx",
        bifrost.WithLoader(func(req *http.Request) (any, error) {
            return map[string]any{"name": "World"}, nil
        }),
    ),
)
if err != nil {
    log.Fatalf("create app: %v", err)
}
```

React components follow standard conventions:

```tsx
export function Head({ name }: { name: string }) {
  return <title>{name}</title>;
}

export function Page({ name }: { name: string }) {
  return <h1>{name}</h1>;
}
```

Only `.tsx` page components are supported.

## API Reference

### Creating an App

```go
func New(assetsFS embed.FS, pages ...Route) (*App, error)

func NewWithOptions(assetsFS embed.FS, opts []ConfigOption, pages ...Route) (*App, error)
```

Creates a new Bifrost application. Setup failures return a nil app and a wrapped error. Call `app.Stop()` when done. Use `NewWithOptions` only for app-wide settings such as `WithDefaultHTMLLang`.

**Parameters:**

- `assetsFS` - Embedded assets (required in production)
- `pages` - Route configurations

### Creating Pages

```go
func Page(pattern string, componentPath string, opts ...PageOption) Route
```

Creates a route configuration for a React component.

**Parameters:**

- `pattern` - URL pattern (e.g., "/", "/about", "/blog/*")
- `componentPath` - Path to the component file (e.g., "./pages/home.tsx")
- `opts` - Page options (variadic, typed)

**Page Options:**

```go
// Props loader - function to load data from request
func WithLoader(loader PropsLoader) PageOption

// Client-only mode - static page with empty shell + client render
func WithClient() PageOption

// Static prerender mode - full HTML at build time + hydration
func WithStatic() PageOption

// Static prerender with dynamic paths
func WithStaticData(loader StaticDataLoader) PageOption

// Document <html lang> for this route (overridden by loader key below)
func WithHTMLLang(lang string) PageOption

// Document <html class> for this route (overridden by loader key below)
func WithHTMLClass(class string) PageOption
```

`WithLoader` is valid only for SSR pages. Bifrost rejects loaders on client-only or static pages, nil loaders, and combining `WithStatic` with `WithStaticData`.

**App options** (use `NewWithOptions(assets, []bifrost.ConfigOption{...}, pages...)`):

```go
func WithDefaultHTMLLang(lang string) ConfigOption
```

**Document language:** precedence is loader/static-data field `bifrost.PropHTMLLang` (`"__bifrost_html_lang"`) → `WithHTMLLang` → `WithDefaultHTMLLang` → `"en"`. The reserved key is stripped before props reach the component.

**Document class:** precedence is loader/static-data field `bifrost.PropHTMLClass` (`"__bifrost_html_class"`) → `WithHTMLClass` → empty class. The reserved key is stripped before props reach the component.

**Props Loader:**

```go
type PropsLoader func(*http.Request) (any, error)
```

A function that receives the HTTP request and returns props to pass to the component.

### Registering Routes

Routes normally go to `New`. To add routes later, call `Handle` before `Wrap` or `Handler`:

```go
func (app *App) Handle(routes ...Route) error
```

`Handle` returns an error for invalid or duplicate routes, build-entry name collisions, conflicting shared-component modes, stale production assets, or route registration after `Wrap`/`Handler`.

Bifrost provides two methods to get an `http.Handler`:

**With API router:**

```go
func (app *App) Wrap(api Router) http.Handler
```

Registers all Bifrost routes into your existing router and returns a wrapped http.Handler that serves assets and delegates to your router. Panics if router is nil.

```go
api := chi.NewRouter()
// ... add API routes

handler := app.Wrap(api)
// Use handler with an http.Server that handles graceful signal shutdown.
```

**Without API router:**

```go
func (app *App) Handler() http.Handler
```

Returns an http.Handler that serves only Bifrost pages and assets (no custom API routes).

```go
handler := app.Handler()
// Use handler with an http.Server that handles graceful signal shutdown.
```

## Route Table

On startup, Bifrost prints a route table listing all registered routes, their component paths, and render modes:

```
Bifrost routes:
  PATTERN             COMPONENT             MODE
  /{$}                ./pages/home.tsx      ssr
  /about              ./pages/about.tsx     client
  /product            ./pages/product.tsx   static
```

The table prints only when stdout is a terminal. Control it with environment variables:

| Variable | Effect |
|----------|--------|
| `BIFROST_ROUTE_TABLE=1` | Always print, even when stdout is not a terminal |
| `BIFROST_NO_ROUTE_TABLE=1` | Never print, even when stdout is a terminal |

## Page Types

### SSR Pages (Server-Side Rendering)

Render React components on each request with dynamic data:

```go
bifrost.Page("/user/{id}", "./pages/user.tsx",
    bifrost.WithLoader(func(req *http.Request) (any, error) {
        userID := chi.URLParam(req, "id")
        user, err := db.GetUser(userID)
        if err != nil {
            return nil, err
        }

        return map[string]any{
            "user": user,
        }, nil
    }),
)
```

#### SSR Performance

React SSR uses `renderToString`. Bifrost buffers the rendered page before writing the HTTP response so render failures can return a clean HTTP 500. Request cancellation propagates to the Go-to-Bun HTTP request.

For routes where latency, throughput, or Largest Contentful Paint matters most, prefer static prerender (`WithStatic`). Those routes serve prebuilt HTML without invoking Bun per request.

### Static Pages

There are two static page modes:

#### Client-Only (`WithClient`)

Empty HTML shell with client-side JavaScript rendering:

```go
bifrost.Page("/admin", "./pages/admin.tsx", bifrost.WithClient())
```

Characteristics:
- Empty `<div id="app"></div>` shell HTML in development and production
- JavaScript bundles for client-side rendering
- No Bun runtime or server-rendered `Head` output
- Component renders entirely on the client

Use SSR or static prerender when a page needs server-rendered metadata or SEO.

**Use cases:**
- Admin dashboards
- Interactive apps without SEO needs
- Pages with heavy client-side interactivity

#### Static Prerender (`WithStatic`)

Full HTML prerendered at build time with hydration:

```go
bifrost.Page("/about", "./pages/about.tsx", bifrost.WithStatic())
```

Characteristics:
- Full HTML with rendered body at build time
- JavaScript bundles for client hydration
- No Bun runtime needed to serve
- Better initial load performance and SEO
- Component hydrates on client for interactivity

**Use cases:**
- Marketing pages
- Landing pages
- Content pages
- Any page that benefits from fast initial render

**Build Process:**

```bash
bifrost build ./main.go
```

Generates:
- `.bifrost/pages/<entry>.html` — client-only shell HTML
- `.bifrost/pages/routes/<url>/index.html` — static-prerender HTML
- `.bifrost/dist/` — JS/CSS bundles for rendering or hydration
- `.bifrost/manifest.json` — asset and route manifest

#### Static with Data (`WithStaticData`)

Prerender multiple pages from a data source at build time:

```go
type PostProps struct {
    Title string `json:"title"`
    Body  string `json:"body"`
}

bifrost.Page("/blog/{slug...}", "./pages/blog.tsx",
    bifrost.WithStaticData(func(ctx context.Context) ([]bifrost.StaticPathData, error) {
        posts := getAllPosts()
        paths := make([]bifrost.StaticPathData, len(posts))
        for i, post := range posts {
            paths[i] = bifrost.StaticPathData{
                Path:  "/blog/" + post.Slug,
                Props: PostProps{Title: post.Title, Body: post.Body},
            }
        }
        return paths, nil
    }),
)
```

`StaticPathData.Props` accepts both `map[string]any` and structs. Structs are JSON-serialized and passed to the component as props.

When embedded with `embed.FS`, static pages serve the pre-built HTML directly.

## Props and Data Flow

Go passes data to components via the props loader:

```go
// Go
bifrost.Page("/{$}", "./pages/home.tsx",
    bifrost.WithLoader(func(req *http.Request) (any, error) {
        return map[string]any{
            "message": "Hello from Go!",
            "count":   42,
        }, nil
    }),
)
```

```tsx
// React
export default function Home({ message, count }) {
    return (
        <div>
            <h1>{message}</h1>
            <p>Count: {count}</p>
        </div>
    );
}
```

### Struct Props

Loaders can return structs instead of maps. Structs are serialized via JSON marshal/unmarshal when merging is needed, and passed through directly otherwise:

```go
type HomeProps struct {
    Message string `json:"message"`
    Count   int    `json:"count"`
}

bifrost.Page("/{$}", "./pages/home.tsx",
    bifrost.WithLoader(func(req *http.Request) (any, error) {
        return HomeProps{
            Message: "Hello from Go!",
            Count:   42,
        }, nil
    }),
)
```

Structs are supported everywhere maps are: props loaders and static data loaders. Reserved keys `__bifrost_html_lang` and `__bifrost_html_class` work via struct JSON tags. When reserved keys are present in a struct, they are extracted and stripped before the struct reaches the component.

## Error Handling

### Redirects

Return an error from the props loader that implements `bifrost.RedirectError` (an interface type alias). Example:

```go
type loginRedirect struct {
    url    string
    status int
}

func (e *loginRedirect) Error() string              { return "redirect" }
func (e *loginRedirect) RedirectURL() string       { return e.url }
func (e *loginRedirect) RedirectStatusCode() int   { return e.status }

bifrost.Page("/protected", "./pages/protected.tsx",
    bifrost.WithLoader(func(req *http.Request) (any, error) {
        if !isAuthenticated(req) {
            return nil, &loginRedirect{url: "/login", status: http.StatusFound}
        }
        // ...
    }),
)
```

The interface is:

```go
type RedirectError interface {
    error
    RedirectURL() string
    RedirectStatusCode() int
}
```

### Production Errors

Bifrost returns initialization errors from `New` in production:

- Missing `embed.FS`
- Missing or invalid `manifest.json`
- Missing, stale, or mode-mismatched page entries
- Missing client scripts, client HTML shells, or SSR bundles
- Missing embedded Bun runtime for SSR pages

Handle the error before starting the HTTP server. This keeps setup failures out of request handling.

**Note:** Runtime-related errors only occur when the app has SSR pages. Static-only apps don't include or require the Bun runtime.

### Development Error Pages

In development mode (`BIFROST_DEV=1`), Bifrost renders rich, structured error pages when page compilation or rendering fails. These pages are designed to help developers quickly diagnose and fix issues.

**Features:**

- **Error type badges** — Visual classification into Build Error, Render Error, or Import Error
- **File location** — Precise `file:line:column` when Bun provides position data
- **Code snippets** — The offending line of source code, highlighted inline
- **Stack traces** — Full JavaScript stack trace in an expandable details section
- **Sub-errors** — Individual error details from `Bun.build` logs, each with its own file location
- **Import info** — Specifier and referrer for module resolution failures
- **Next steps** — Context-specific guidance such as installing JavaScript packages or verifying import paths

**Error flow:**

1. Bun returns errors as JSON with nested `position`, `specifier`, and `referrer` fields
2. Go deserializes into `*core.StructuredError` via `bunErrorJSON` types
3. `errors.As` extracts the structured error at any depth in the error chain
4. The dev error template renders the structured data

**Fallback behavior:**

- Errors without structured data (e.g. Go loader errors) show the raw message in a `<pre>` block
- Production mode always shows a generic error page with no internals exposed
- If the error template itself fails to render, a minimal HTML fallback is served

## Build System

### Initialize Project

Create a new project with one command:

```bash
bifrost init myapp
```

This scaffolds a complete working project with all required files.

Options:
- `--template <name>`: Choose from `minimal` (default), `spa`

Examples:
```bash
bifrost init myapp
bifrost init --template spa myspa
```

### Repair .bifrost Directory

If the `.bifrost` directory is missing:

```bash
bifrost doctor .
```

### Build for Production

```bash
bifrost build ./main.go
```

**Flags:**

- `--go-build[=path]` — After building assets, run `go build -o <path> <main.go>`. Default output path is `./tmp/app`. The main file argument must come **before** the flag.

```bash
# Build assets only
bifrost build ./main.go

# Build assets, then compile Go binary to ./tmp/app
bifrost build ./main.go --go-build

# Build assets, then compile Go binary to a custom path
bifrost build ./main.go --go-build=./myapp
```

**Build declaration rules:**

The build scan accepts direct `bifrost.Page()` calls with:

- a string-literal component path, such as `"./pages/home.tsx"`
- direct Bifrost option calls, such as `bifrost.WithClient()`

Indirect component paths (`Page("/{$}", homePath)`) and expanded option slices (`opts...`) fail with a clear build error. This prevents the build from silently omitting or misclassifying a page. A component may back several routes, but every route using that component must use the same page mode. Shared client-only routes must also use the same document language and class because they share one HTML shell.

**Build Pipeline:**

1. Scan the main module import graph for direct `bifrost.Page()` declarations
2. Validate component paths, options, and shared-component modes
3. Generate client and SSR entry files
4. Build SSR bundles and client JS/CSS
5. Generate client-only HTML shells
6. Compile the embedded Bun runtime when Bun SSR or static export needs it; Sobek builds omit this step
7. Export static-prerender routes and remove their build-only SSR bundles
8. Copy `public/` assets and write `manifest.json`
9. Exit non-zero if any required page or bundle fails

Production `/dist/` assets are content-hashed and served with `Cache-Control: public, max-age=31536000, immutable`.

### SSR Bundles

For SSR pages, production builds include server bundles:

- Located in `.bifrost/ssr/`
- Built for the selected Bun or Sobek runtime
- Extracted from `embed.FS` at runtime
- Used instead of source files in production

## Project Structure

```
myapp/
├── main.go           # Go server
├── embed.go          # //go:embed all:.bifrost (same package as main.go)
├── pages/            # Page components (relative to main.go's directory)
│   ├── home.tsx
│   └── about.tsx
├── components/       # Shared components
├── public/           # Static assets
├── .bifrost/         # Build output (gitignored; .gitkeep for embed)
│   ├── dist/         # Client bundles
│   ├── ssr/          # SSR bundles (production only)
│   ├── pages/        # Static HTML files
│   └── manifest.json # Asset manifest
└── go.mod
```

`.bifrost/` and component paths (`./pages/...`) resolve relative to `dir(main.go)` — the directory containing your main package. For single-binary projects where `main.go` is at the module root, this is the module root. `public/` resolves to `dir(main.go)/public` when it exists, otherwise to `<module root>/public` (matching how component paths fall back to the module root). `embed.go` must be in the same directory as `main.go` because `//go:embed` cannot reach parent directories.

### Multi-binary Projects

For monorepos with multiple binaries (`cmd/web/`, `cmd/admin/`), each binary owns its own `.bifrost/`, `public/`, and `embed.go`:

```
myrepo/
├── go.mod
├── cmd/
│   ├── web/
│   │   ├── main.go
│   │   ├── embed.go      # //go:embed all:.bifrost
│   │   ├── pages/
│   │   ├── public/
│   │   └── .bifrost/     # web's build output
│   └── admin/
│       ├── main.go
│       ├── embed.go
│       ├── pages/
│       ├── public/
│       └── .bifrost/     # admin's build output
└── internal/
    └── shared/           # shared Go code (scanned by both binaries)
        └── routes.go
```

```bash
bifrost build ./cmd/web/main.go     # → cmd/web/.bifrost/
bifrost build ./cmd/admin/main.go   # → cmd/admin/.bifrost/
```

Each build writes only to its own `dir(main.go)/.bifrost/`. No collision — even within a single Go module. `bifrost.Page()` calls in imported packages (e.g. `internal/shared/routes.go`) are discovered automatically via import-graph traversal.

### Runtime concurrency

Sobek is the default and uses an in-process worker pool:

```bash
bifrost build ./main.go
BIFROST_SOBEK_WORKERS=4 ./app
```

Select Bun explicitly when maximum SSR throughput matters more than binary size and memory use:

```bash
BIFROST_JS_RUNTIME=bun bifrost build ./main.go
BIFROST_JS_RUNTIME=bun ./app
```

Sobek uses esbuild's Go API for React SSR bundles, hydration bundles, code splitting, CSS imports, source maps, and production asset hashes. Production pages share one lazily initialized SSR registry, which deduplicates React and shared modules while keeping import failures isolated to the selected page. Tailwind's official JavaScript compiler runs inside Sobek; Bifrost scans the esbuild source graph in Go instead of loading Tailwind's native Node scanner. Bun is not started or required by a Sobek build. JavaScript packages must already exist in `node_modules`; use npm, pnpm, or another package installer if Bun is unavailable.

Sobek removes the production child process and embedded Bun executable. It uses a pool of isolated JavaScript runtimes because one Sobek runtime is not goroutine-safe. The default worker count is `min(GOMAXPROCS, 4)`; set `BIFROST_SOBEK_WORKERS` only after load testing. Each worker loads its own copy of each used SSR bundle, trading memory for throughput.

The manifest records the selected runtime, so a production binary built from Sobek assets selects Sobek when `BIFROST_JS_RUNTIME` is unset. An explicit runtime environment value overrides the manifest and must match the built assets. `bifrost build --go-build` applies the measured Sobek PGO profile by default; set `BIFROST_SOBEK_PGO=off` to disable it or set the variable to a profile path to replace it. React Compiler transforms currently run only in Bun builds; Sobek preserves React behavior but omits that optional optimization. Asynchronous JavaScript SSR that leaves a Promise pending is unsupported; load data in Go and pass it as props. Prefer static prerender for high-volume pages.

### Route patterns

Patterns follow the wrapped router. With the built-in `http.ServeMux`, `"/"` is a subtree route and catches unmatched paths. Use `"/{$}"` for the exact root path. Static export supports literal segments, Go `{name}` and `{name...}` wildcards, Chi-style `:name`, `*`, and trailing-slash subtrees.

## Best Practices

1. **Always stop the app**: `defer app.Stop()` after creation, return from `main` on server errors instead of calling `log.Fatal`, and call `Stop` during graceful signal shutdown
2. **Use typed options**: `WithLoader()`, `WithClient()`, `WithStatic()`
3. **Test mode behavior**: Set `BIFROST_DEV=1` explicitly in tests that render components
4. **Strict production**: Always embed `.bifrost` and run `bifrost build ./main.go`
5. **Handle errors in props loaders**: Return proper errors or redirects
6. **Keep props minimal**: Pass only necessary data to components
7. **Keep TypeScript minimal**: Treat TS as a thin Bun adapter; keep policy, validation, and artifact logic in Go

## Extension Points

The architecture supports future extensions:

- **New page modes**: extend `PageMode` in [`internal/core`](internal/core/types.go)
- **New page options**: implement the `PageOption` function type in `internal/core`
- **Build pipeline**: extend [`internal/usecase`](internal/usecase/build_project.go) and related build steps
- **Renderer / Bun host**: extend [`internal/adapters/runtime`](internal/adapters/runtime/host.go) and [`internal/adapters/process`](internal/adapters/process/)

## Complete Example

See the [example/](example/) directory for a working implementation with:

- SSR and static pages
- Dynamic routes with URL parameters
- Error handling demos
- Asset embedding
- Chi router integration
