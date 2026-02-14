# Bifrost Documentation

Server-side rendering for React components in Go.

## Overview

Bifrost bridges Go backends with React frontends using Bun for SSR. It features a clean architecture with strict separation between development and production modes.

## Installation

```bash
go get github.com/3-lines-studio/bifrost
```

Requires [Bun](https://bun.sh) to be installed.

## Architecture

Bifrost is organized into focused internal packages:

- `internal/types` - Shared types (PropsLoader, PageOption, RedirectError)
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
    // Create renderer
    // Production: requires WithAssetsFS
    // Development: works without it
    r, err := bifrost.New(bifrost.WithAssetsFS(bifrostFS))
    if err != nil {
        log.Fatal(err)
    }
    defer r.Stop()
    
    // Create page handlers with typed options
    home := r.NewPage("./pages/home.tsx", 
        bifrost.WithPropsLoader(func(req *http.Request) (map[string]any, error) {
            return map[string]any{
                "name": "World",
            }, nil
        }),
    )
    
    about := r.NewPage("./pages/about.tsx", bifrost.WithClientOnly())
    
    // Setup routes
    router := http.NewServeMux()
    router.Handle("/", home)
    router.Handle("/about", about)
    
    // Register asset routes
    assetRouter := http.NewServeMux()
    bifrost.RegisterAssetRoutes(assetRouter, r, router)
    
    log.Fatal(http.ListenAndServe(":8080", assetRouter))
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
- `WithAssetsFS()` is **mandatory** - fails fast at startup if missing
- `.bifrost/manifest.json` must exist in embedded assets
- SSR bundles extracted from `.bifrost/ssr/` in embed.FS
- Source TSX files are **never** used

Strict validation errors:
- `ErrAssetsFSRequiredInProd` - Missing `WithAssetsFS()` in production
- `ErrManifestMissingInAssetsFS` - Manifest not found in embedded assets

## API Reference

### Creating a Renderer

```go
func New(opts ...Option) (*Renderer, error)
```

Creates a new Bifrost renderer. Must be stopped with `renderer.Stop()` when done.

**Options:**

| Option | Description |
|--------|-------------|
| `WithAssetsFS(fs embed.FS)` | Embed built assets (required in production) |

### Creating Pages

```go
func (r *Renderer) NewPage(componentPath string, opts ...PageOption) http.Handler
```

Creates an HTTP handler for a React component. Returns `http.Handler` interface.

**Parameters:**

- `componentPath` - Path to the .tsx file (e.g., `./pages/home.tsx`)
- `opts` - Page options (variadic, typed)

**Page Options:**

```go
// Props loader - function to load data from request
func WithPropsLoader(loader PropsLoader) PageOption

// Client-only mode - static page with empty shell + client render
func WithClientOnly() PageOption

// Static prerender mode - full HTML at build time + hydration
func WithStaticPrerender() PageOption
```

**Props Loader:**

```go
type PropsLoader func(*http.Request) (map[string]any, error)
```

A function that receives the HTTP request and returns props to pass to the React component.

### Registering Routes

```go
func RegisterAssetRoutes(r Router, renderer *Renderer, appRouter http.Handler)
```

Sets up asset serving for `/dist/*` and public files.

## Page Types

### SSR Pages (Server-Side Rendering)

Render React components on each request with dynamic data:

```go
page := r.NewPage("./pages/user.tsx", 
    bifrost.WithPropsLoader(func(req *http.Request) (map[string]any, error) {
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

#### Client-Only (`WithClientOnly`)

Empty HTML shell with client-side JavaScript rendering:

```go
page := r.NewPage("./pages/about.tsx", bifrost.WithClientOnly())
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

#### Static Prerender (`WithStaticPrerender`)

Full HTML prerendered at build time with hydration:

```go
page := r.NewPage("./pages/blog.tsx", bifrost.WithStaticPrerender())
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
home := r.NewPage("./pages/home.tsx", 
    bifrost.WithPropsLoader(func(req *http.Request) (map[string]any, error) {
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
page := r.NewPage("./pages/protected.tsx", 
    bifrost.WithPropsLoader(func(req *http.Request) (map[string]any, error) {
        if !isAuthenticated(req) {
            return nil, &RedirectError{
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

Bifrost provides specific errors for production misconfiguration:

```go
import "github.com/3-lines-studio/bifrost/internal/runtime"

r, err := bifrost.New()
if err != nil {
    if errors.Is(err, runtime.ErrAssetsFSRequiredInProd) {
        // Missing WithAssetsFS() in production
    }
    if errors.Is(err, runtime.ErrManifestMissingInAssetsFS) {
        // Manifest not found in embedded assets
    }
    if errors.Is(err, runtime.ErrEmbeddedRuntimeNotFound) {
        // Run 'bifrost-build' to generate embedded Bun runtime
    }
    if errors.Is(err, runtime.ErrEmbeddedRuntimeExtraction) {
        // Failed to extract embedded runtime from binary
    }
    if errors.Is(err, runtime.ErrEmbeddedRuntimeStart) {
        // Embedded runtime failed to start
    }
}
```

## Build System

### Initialize Project

```bash
go run github.com/3-lines-studio/bifrost/cmd/init@latest .
```

Creates `.bifrost/` directory with `.gitkeep` for go:embed compatibility.

### Build for Production

```bash
bifrost-build main.go
```

**Build Pipeline:**

1. AST scan discovers all `NewPage()` calls
2. Detects `WithClientOnly()` for mode classification
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

1. **Always defer Stop()**: `defer r.Stop()` after creating the renderer
2. **Use typed options**: `WithPropsLoader()` and `WithClientOnly()`
3. **Test mode behavior**: Set `BIFROST_DEV=1` explicitly in tests that render TSX
4. **Strict production**: Always use `WithAssetsFS()` and run `bifrost-build`
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
