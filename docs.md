# Bifrost Documentation

Server-side rendering for React components in Go.

## Overview

Bifrost bridges Go backends with React frontends using Bun for SSR. It features a clean architecture with strict separation between development and production modes.

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
- `internal/adapters/framework` — Framework entry templates (e.g. React)
- `internal/adapters/cli` — Terminal output and build reports
- Root `bifrost` package — Public `App` API (`New`, `Page`, `Wrap`, …)

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
            bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
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

## Mode Detection

Bifrost determines mode by checking the `BIFROST_DEV` environment variable:

| Variable | Mode | Behavior |
|----------|------|----------|
| `BIFROST_DEV=1` | Development | Source TSX files, hot reload, no embed required |
| Unset or other | Production | SSR bundles from embed.FS, strict validation |

### Development Mode

```bash
export BIFROST_DEV=1
go run main.go
```

Features:
- Renders source TSX files directly
- Hot reload on file changes
- No embedded assets required
- Detailed error pages

### Production Mode

```bash
# Build assets (from module root; path is your main package entrypoint)
go run github.com/3-lines-studio/bifrost/cmd/build@latest ./main.go

# Build Go binary
go build -o myapp main.go

# Run (no BIFROST_DEV set)
./myapp
```

`go install github.com/3-lines-studio/bifrost/cmd/build@latest` installs a binary named `build` (the directory name); rename it or add a shell alias if you want a `bifrost-build` command on your PATH.

Requirements:
- `embed.FS` is **mandatory** - panics at startup if missing
- `.bifrost/manifest.json` must exist in embedded assets
- SSR bundles extracted from `.bifrost/ssr/` in embed.FS (SSR pages only)
- Embedded Bun runtime included only for SSR pages
- Source TSX files are **never** used

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

## API Reference

### Creating an App

```go
func New(assetsFS embed.FS, pages ...Route) *App

func NewWithFramework(assetsFS embed.FS, fw Framework, pages ...Route) *App

func NewWithOptions(assetsFS embed.FS, opts []ConfigOption, pages ...Route) *App
```

Creates a new Bifrost application. Must be stopped with `app.Stop()` when done. `New` defaults to React. Use `NewWithFramework` when selecting a non-default framework constant (today only `bifrost.React` exists). Use `NewWithOptions` for app-wide settings such as `WithDefaultHTMLLang` or `WithFramework` inside the options slice.

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
- `componentPath` - Path to the .tsx file (e.g., "./pages/home.tsx")
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

**Document language:** precedence is loader/static-data field `bifrost.PropHTMLLang` (`"__bifrost_html_lang"`) → `WithHTMLLang` → `WithDefaultHTMLLang` → `"en"`. The reserved key is stripped before props reach React.

**Document class:** precedence is loader/static-data field `bifrost.PropHTMLClass` (`"__bifrost_html_class"`) → `WithHTMLClass` → empty class. The reserved key is stripped before props reach React.

**Props Loader:**

```go
type PropsLoader func(*http.Request) (map[string]any, error)
```

A function that receives the HTTP request and returns props to pass to the React component.

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

## Page Types

### SSR Pages (Server-Side Rendering)

Render React components on each request with dynamic data:

```go
bifrost.Page("/user/{id}", "./pages/user.tsx", 
    bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
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

**React body streaming (`renderToReadableStream`):** All SSR pages use `renderToReadableStream` for the page body; Bun forwards byte chunks after the usual head flush. **Suspense** (or other deferred server work) makes progressive HTML visible; synchronous trees still work but gain little. If streaming fails, Bifrost falls back to `renderToString` for that request. Errors that occur after bytes have been sent cannot be turned into an HTTP 500.

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
go run github.com/3-lines-studio/bifrost/cmd/build@latest ./main.go
```

Generates:
- `.bifrost/pages/[page]/index.html`:
  - Client-only: empty shell HTML
  - Static prerender: full HTML with rendered body
- `.bifrost/dist/` - JS/CSS bundles for hydration
- `.bifrost/manifest.json` - Asset manifest with mode info

When embedded with `embed.FS`, static pages serve the pre-built HTML directly.

## Props and Data Flow

Go passes data to React components via the props loader:

```go
// Go
bifrost.Page("/", "./pages/home.tsx", 
    bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
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
    bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
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

Implementations should also satisfy `error` (typically via an `Error()` method) because loaders return `(map[string]any, error)`.

### Production Errors

Bifrost **panics** on initialization errors in production:

- Missing `embed.FS` in production
- Missing manifest.json in embedded assets
- Missing embedded Bun runtime (for SSR pages)

This ensures fast failure at startup rather than runtime errors.

**Note:** Runtime-related errors only occur when the app has SSR pages. Static-only apps don't include or require the Bun runtime.

## Build System

### Initialize Project

Create a new project with one command:

```bash
go run github.com/3-lines-studio/bifrost/cmd/init@latest myapp
```

This scaffolds a complete working project with all required files.

Options:
- `--template <name>`: Choose from `minimal` (default), `spa`, `desktop`

Examples:
```bash
go run github.com/3-lines-studio/bifrost/cmd/init@latest myapp
go run github.com/3-lines-studio/bifrost/cmd/init@latest --template spa myspa
```

### Repair .bifrost Directory

If the `.bifrost` directory is missing:

```bash
go run github.com/3-lines-studio/bifrost/cmd/doctor@latest .
```

### Build for Production

```bash
go run github.com/3-lines-studio/bifrost/cmd/build@latest ./main.go
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
- Used instead of source TSX files in production

## Project Structure

```
myapp/
├── main.go           # Go server
├── pages/            # React page components
│   ├── home.tsx
│   └── about.tsx
├── components/       # Shared React components
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
3. **Test mode behavior**: Set `BIFROST_DEV=1` explicitly in tests that render TSX
4. **Strict production**: Always embed `.bifrost` and run the build CLI (`go run github.com/3-lines-studio/bifrost/cmd/build@latest ./main.go`, or an installed `build` binary from `cmd/build`)
5. **Handle errors in props loaders**: Return proper errors or redirects
6. **Keep props minimal**: Pass only necessary data to React

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
