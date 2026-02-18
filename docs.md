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

- `internal/types` - Shared types (PropsLoader, PageConfig, RedirectError)
- `internal/runtime` - Bun process management and IPC protocol
- `internal/page` - HTTP handler orchestration
- `internal/assets` - Manifest resolution and embedded FS handling
- `internal/build` - Build pipeline and AST-based page discovery
- `internal/cli` - Terminal output helpers

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
# Build first
bifrost-build main.go

# Build Go binary
go build -o myapp main.go

# Run (no BIFROST_DEV set)
./myapp
```

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
```

Creates a new Bifrost application. Must be stopped with `app.Stop()` when done.

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
```

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
bifrost-build main.go
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

Return a redirect from props loader:

```go
bifrost.Page("/protected", "./pages/protected.tsx", 
    bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
        if !isAuthenticated(req) {
            return nil, &bifrost.RedirectError{
                URL:    "/login",
                Status: http.StatusFound,
            }
        }
        // ...
    }),
)
```

Implement the `RedirectError` interface:

```go
type RedirectError interface {
    RedirectURL() string
    RedirectStatusCode() int
}
```

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
bifrost-build main.go
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
4. **Strict production**: Always embed `.bifrost` and run `bifrost-build`
5. **Handle errors in props loaders**: Return proper errors or redirects
6. **Keep props minimal**: Pass only necessary data to React

## Extension Points

The architecture supports future extensions:

- **New Page Modes**: Add to `PageMode` enum in `internal/types`
- **New Page Options**: Implement `PageOption` function type
- **Custom Build Targets**: Extend build pipeline in `internal/build`
- **Alternative Runtimes**: Runtime interface abstraction in `internal/runtime`

## Complete Example

See the [example/](../example/) directory for a working implementation with:

- SSR and static pages
- Dynamic routes with URL parameters
- Error handling demos
- Asset embedding
- Chi router integration
