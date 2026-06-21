# Bifrost Documentation

Server-side rendering for React and Svelte components in Go.

## Overview

Bifrost bridges Go backends with React and Svelte frontends using Bun for SSR. It features a clean architecture with strict separation between development and production modes.

## Installation

```bash
go get github.com/3-lines-studio/bifrost
```

Requires [Bun](https://bun.sh) to be installed.

**Runtime Requirements:**
- **Development**: Always requires Bun
- **Production with SSR**: Bun runtime is embedded automatically (no system Bun required)
- **Production static-only**: No Bun runtime included

## Architecture

Bifrost is organized into focused internal packages:

- `internal/core` — Shared types (`PageConfig`, `PropsLoader`, `RedirectError`), manifest and HTML shell, page routing decisions, critical CSS, MIME/path helpers
- `internal/usecase` — Build and page-serve orchestration (wiring core to adapters)
- `internal/adapters/http` — Page and asset HTTP handlers
- `internal/adapters/process` — Bun renderer process and bundle IPC
- `internal/adapters/runtime` — Renderer host lifecycle and embedded runtime
- `internal/adapters/fs` — Filesystem and embed abstractions
- `internal/adapters/framework` — Framework entry templates (e.g. React and Svelte)
- `internal/adapters/cli` — Terminal output and build reports
- Root `bifrost` package — Public `App` API (`New`, `Page`, `Wrap`, …)

### Go vs TypeScript Boundary

TypeScript is intentionally minimal in Bifrost.

- TypeScript is only for Bun-specific capabilities that Go does not provide directly:
  - rendering React and Svelte components inside the Bun runtime
  - invoking `Bun.build`
  - serializing raw render/build results back to Go
- Everything else belongs in Go:
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
    "embed"
    "log"
    "net/http"
    
    "github.com/3-lines-studio/bifrost"
)

//go:embed all:.bifrost
var bifrostFS embed.FS

func main() {
    // Create Bifrost app
    app := bifrost.New(
        bifrostFS,
        bifrost.Page("/", "./pages/home.tsx", 
            bifrost.WithLoader(func(req *http.Request) (any, error) {
                return map[string]any{
                    "name": "World",
                }, nil
            }),
        ),
        bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
    )
    defer app.Stop()
    
    // Setup API routes
    api := http.NewServeMux()
    
    // Start server
    log.Fatal(http.ListenAndServe(":8080", app.Wrap(api)))
}
```

### Svelte

```go
package main

import (
    "embed"
    "log"
    "net/http"

    "github.com/3-lines-studio/bifrost"
)

//go:embed all:.bifrost
var bifrostFS embed.FS

func main() {
    app := bifrost.New(
        bifrostFS,
        bifrost.Page("/", "./pages/home.svelte",
            bifrost.WithLoader(func(req *http.Request) (any, error) {
                return map[string]any{
                    "name": "World",
                }, nil
            }),
        ),
        bifrost.Page("/about", "./pages/about.svelte", bifrost.WithClient()),
    )
    defer app.Stop()

    api := http.NewServeMux()
    log.Fatal(http.ListenAndServe(":8080", app.Wrap(api)))
}
```

## Mode Detection

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
# Build assets (from module root; path is your main package entrypoint)
go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest build ./main.go

# Build Go binary
go build -o myapp main.go

# Run (no BIFROST_DEV set)
./myapp
```

`go install github.com/3-lines-studio/bifrost/cmd/bifrost@latest` installs a binary named `bifrost`; run `bifrost init`, `bifrost dev`, `bifrost build`, or `bifrost doctor`.

Requirements:
- `embed.FS` is **mandatory** - panics at startup if missing
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

Strict validation causes panic on:
- Missing `embed.FS` in production
- Missing manifest.json in embedded assets

## Framework Support

Bifrost supports React and Svelte with the same Go API surface.

### React

React is the default framework. Use `.tsx` components with `bifrost.New()`:

```go
app := bifrost.New(bifrostFS,
    bifrost.Page("/", "./pages/home.tsx",
        bifrost.WithLoader(func(req *http.Request) (any, error) {
            return map[string]any{"name": "World"}, nil
        }),
    ),
)
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

### Svelte

Svelte is auto-detected from `.svelte` file extensions. Use `bifrost.New()` — no special constructor needed:

```go
app := bifrost.New(bifrostFS,
    bifrost.Page("/", "./pages/home.svelte",
        bifrost.WithLoader(func(req *http.Request) (any, error) {
            return map[string]any{"name": "World"}, nil
        }),
    ),
)
```

Svelte 5 components use `$props()` and `<svelte:head>`:

```svelte
<script lang="ts">
  let { name }: { name: string } = $props();
</script>

<svelte:head>
  <title>{name}</title>
</svelte:head>

<h1>{name}</h1>
```

**Scoped styles** work automatically. Svelte adds `svelte-*` class hashes to elements and CSS selectors, and Bifrost's critical CSS extraction correctly identifies which scoped rules apply to each page.

**Auto-detection:** Pages with `.svelte` path suffixes use the Svelte adapters; `.tsx` paths use React. Mixed-framework apps are supported within the same Go process.

## API Reference

### Creating an App

```go
func New(assetsFS embed.FS, pages ...Route) *App

func NewWithFramework(assetsFS embed.FS, fw Framework, pages ...Route) *App

func NewWithOptions(assetsFS embed.FS, opts []ConfigOption, pages ...Route) *App
```

Creates a new Bifrost application. Must be stopped with `app.Stop()` when done. `New` defaults to React but auto-detects Svelte from `.svelte` component paths. Use `NewWithFramework` when selecting a non-default framework constant (e.g. `bifrost.React` or `bifrost.Svelte`). Use `NewWithOptions` for app-wide settings such as `WithDefaultHTMLLang` or `WithFramework` inside the options slice.

**Parameters:**

- `assetsFS` - Embedded assets (required in production)
- `pages` - Route configurations

### Creating Pages

```go
func Page(pattern string, componentPath string, opts ...PageOption) Route
```

Creates a route configuration for a React or Svelte component.

**Parameters:**

- `pattern` - URL pattern (e.g., "/", "/about", "/blog/*")
- `componentPath` - Path to the component file (e.g., "./pages/home.tsx" or "./pages/home.svelte")
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

**App options** (use `NewWithOptions(assets, []bifrost.ConfigOption{...}, pages...)`):

```go
func WithDefaultHTMLLang(lang string) ConfigOption

func WithFramework(fw Framework) ConfigOption
```

**Document language:** precedence is loader/static-data field `bifrost.PropHTMLLang` (`"__bifrost_html_lang"`) → `WithHTMLLang` → `WithDefaultHTMLLang` → `"en"`. The reserved key is stripped before props reach the component.

**Document class:** precedence is loader/static-data field `bifrost.PropHTMLClass` (`"__bifrost_html_class"`) → `WithHTMLClass` → empty class. The reserved key is stripped before props reach the component.

**Props Loader:**

```go
type PropsLoader func(*http.Request) (any, error)
```

A function that receives the HTTP request and returns props to pass to the component.

### Registering Routes

Bifrost provides two methods to get an http.Handler:

**With API router:**

```go
func (app *App) Wrap(api Router) http.Handler
```

Registers all Bifrost routes into your existing router and returns a wrapped http.Handler that serves assets and delegates to your router. Panics if router is nil.

```go
api := chi.NewRouter()
// ... add API routes

handler := app.Wrap(api)
http.ListenAndServe(":8080", handler)
```

**Without API router:**

```go
func (app *App) Handler() http.Handler
```

Returns an http.Handler that serves only Bifrost pages and assets (no custom API routes).

```go
http.ListenAndServe(":8080", app.Handler())
```

## Route Table

On startup, Bifrost prints a route table listing all registered routes, their component paths, and render modes:

```
Bifrost routes:
  PATTERN             COMPONENT             MODE
  /{$}                ./pages/home.tsx      ssr
  /about              ./pages/about.tsx     client
  /product            ./pages/product.tsx   static
  /shop               ./pages/shop.svelte   ssr
```

The table prints only when stdout is a terminal. Control it with environment variables:

| Variable | Effect |
|----------|--------|
| `BIFROST_ROUTE_TABLE=1` | Always print, even when stdout is not a terminal |
| `BIFROST_NO_ROUTE_TABLE=1` | Never print, even when stdout is a terminal |

## Page Types

### SSR Pages (Server-Side Rendering)

Render React or Svelte components on each request with dynamic data:

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

#### Streaming HTML and First Contentful Paint

For SSR pages, Bifrost streams the HTML response in two phases: the document head (including output from your `Head` component, critical CSS, stylesheets, and `modulepreload` links) is written and flushed as soon as it is ready, then the server-rendered body and trailing scripts follow. That lets the browser start downloading JavaScript and CSS while the main page tree is still being rendered in Bun.

**Reverse proxies:** If you use nginx, Caddy, or another reverse proxy in front of your Go server, turn off response buffering for HTML routes (for example, in nginx, `proxy_buffering off` in the relevant `location`). Otherwise the proxy may wait for the full response and you will not see a better time to first byte or First Contentful Paint.

**Streaming behavior by framework:**

- **React:** SSR pages use `renderToReadableStream` for the page body; Bun forwards byte chunks after the usual head flush. **Suspense** (or other deferred server work) makes progressive HTML visible; synchronous trees still work but gain little. If streaming fails, Bifrost falls back to `renderToString` for that request.
- **Svelte:** SSR pages use `render` from `svelte/server`. The render is synchronous and produces the full HTML string.

Errors that occur after bytes have been sent cannot be turned into an HTTP 500.

**LCP-focused routing:** For marketing or landing routes where Largest Contentful Paint matters most, prefer **static prerender** (`WithStatic`) so HTML is served from prebuilt files with no Bun work per request. Pair that with hero images that use explicit dimensions and `fetchPriority="high"` where appropriate.

### Static Pages

There are two static page modes:

#### Client-Only (`WithClient`)

Empty HTML shell with client-side JavaScript rendering:

```go
bifrost.Page("/admin", "./pages/admin.tsx", bifrost.WithClient())
```

Characteristics:
- Empty `<div id="app"></div>` shell HTML
- JavaScript bundles for client-side rendering
- No Bun runtime needed to serve
- Component renders entirely on client

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
go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest build ./main.go
```

Generates:
- `.bifrost/pages/[page]/index.html`:
  - Client-only: empty shell HTML
  - Static prerender: full HTML with rendered body
- `.bifrost/dist/` - JS/CSS bundles for hydration
- `.bifrost/manifest.json` - Asset manifest with mode info

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
bifrost.Page("/", "./pages/home.tsx", 
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

bifrost.Page("/", "./pages/home.tsx",
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
    RedirectURL() string
    RedirectStatusCode() int
}
```

Implementations should also satisfy `error` (typically via an `Error()` method) because loaders return `(any, error)`.

### Production Errors

Bifrost **panics** on initialization errors in production:

- Missing `embed.FS` in production
- Missing manifest.json in embedded assets
- Missing embedded Bun runtime (for SSR pages)

This ensures fast failure at startup rather than runtime errors.

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
- **Next steps** — Context-specific guidance (e.g. run `bun install`, verify import paths)

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
go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest init myapp
```

This scaffolds a complete working project with all required files.

Options:
- `--template <name>`: Choose from `minimal` (default), `spa`, `svelte`

Examples:
```bash
go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest init myapp
go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest init --template spa myspa
go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest init --template svelte mysvelteapp
```

### Repair .bifrost Directory

If the `.bifrost` directory is missing:

```bash
go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest doctor .
```

### Build for Production

```bash
go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest build ./main.go
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

**Build Pipeline:**

1. AST scan discovers all `Page()` calls
2. Detects `WithClient()` for mode classification
3. Generates client entry files for each page
4. Builds client bundles (JS/CSS) to `.bifrost/dist/`
5. Builds SSR bundles to `.bifrost/ssr/` (for SSR pages)
6. Generates manifest.json with asset mapping
7. Pre-renders static HTML for client-only pages
8. Copies public/ assets

### SSR Bundles

For SSR pages, production builds include server bundles:

- Located in `.bifrost/ssr/`
- Pre-built for Bun runtime target
- Extracted from `embed.FS` at runtime
- Used instead of source files in production

## Project Structure

```
myapp/
├── main.go           # Go server
├── pages/            # Page components
│   ├── home.tsx
│   └── about.tsx
├── components/       # Shared components
├── public/           # Static assets
├── .bifrost/         # Build output (gitignore)
│   ├── dist/         # Client bundles
│   ├── ssr/          # SSR bundles (production only)
│   ├── pages/        # Static HTML files
│   └── manifest.json # Asset manifest
└── go.mod
```

## Best Practices

1. **Always defer Stop()**: `defer app.Stop()` after creating the app
2. **Use typed options**: `WithLoader()`, `WithClient()`, `WithStatic()`
3. **Test mode behavior**: Set `BIFROST_DEV=1` explicitly in tests that render components
4. **Strict production**: Always embed `.bifrost` and run the build CLI (`go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest build ./main.go`, or an installed `bifrost` binary from `cmd/bifrost`)
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

See the [example/](../example/) directory for a working implementation with:

- SSR and static pages
- Dynamic routes with URL parameters
- Error handling demos
- Asset embedding
- Chi router integration
