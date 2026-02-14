# Bifrost Documentation

Server-side rendering for React components in Go.

## Overview

Bifrost bridges Go backends with React frontends using Bun for SSR. It handles server-side rendering, static site generation, and static asset embedding for production. Hot reload during development is handled by [Air](https://github.com/cosmtrek/air) with proxy configuration.

## Installation

```bash
go get github.com/3-lines-studio/bifrost
```

Requires [Bun](https://bun.sh) and [Air](https://github.com/cosmtrek/air) to be installed:

```bash
go install github.com/cosmtrek/air@latest
```

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
    r, err := bifrost.New(
        bifrost.WithAssetsFS(bifrostFS),
        bifrost.WithTiming(),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer r.Stop()
    
    // Create page handlers
    home := r.NewPage("./pages/home.tsx", func(req *http.Request) (map[string]interface{}, error) {
        return map[string]interface{}{
            "name": "World",
        }, nil
    })
    
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

## API Reference

### Creating a Renderer

```go
func New(opts ...Option) (*Renderer, error)
```

Creates a new Bifrost renderer. Must be stopped with `renderer.Stop()` when done.

**Options:**

| Option | Description |
|--------|-------------|
| `WithAssetsFS(fs embed.FS)` | Embed built assets for production |
| `WithTiming()` | Enable debug timing logs |

### Creating Pages

```go
func (r *Renderer) NewPage(componentPath string, opts ...interface{}) *Page
```

Creates an HTTP handler for a React component.

**Parameters:**

- `componentPath` - Path to the .tsx file (e.g., `./pages/home.tsx`)
- `opts` - Optional props loader function or page options

**Props Loader:**

```go
func(*http.Request) (map[string]interface{}, error)
```

A function that receives the HTTP request and returns props to pass to the React component.

**Page Options:**

| Option | Description |
|--------|-------------|
| `WithClientOnly()` | Clien only js (no SSR at runtime) |

### Registering Routes

```go
func RegisterAssetRoutes(r Router, renderer *Renderer, appRouter http.Handler)
```

Sets up asset serving for `/dist/*` and public files. Must be called after setting up page routes.

## Page Types

### SSR Pages (Server-Side Rendering)

Render React components on each request with dynamic data:

```go
page := r.NewPage("./pages/user.tsx", func(req *http.Request) (map[string]interface{}, error) {
    userID := chi.URLParam(req, "id")
    user, err := db.GetUser(userID)
    if err != nil {
        return nil, err
    }
    
    return map[string]interface{}{
        "user": user,
    }, nil
})
```

### Static Pages (MPA with Hydration)

Pre-rendered at build time with client-side JavaScript hydration. Static pages behave like a Multi-Page Application (MPA) where each page is pre-rendered to HTML at build time, then hydrated with React on the client.

```go
// Create a static page
page := r.NewPage("./pages/about.tsx", bifrost.WithClientOnly())
```

**Key characteristics:**

- HTML is generated at build time, not on each request
- Still includes JavaScript bundles for client-side hydration
- No Bun runtime required when serving (HTML is pre-built)
- Interactive React components work after hydration

**Build Process:**

```bash
# Build static pages
bifrost-build main.go

# This generates:
# - .bifrost/pages/[page]/index.html - Pre-rendered HTML
# - .bifrost/dist/ - JS/CSS bundles for hydration
# - .bifrost/manifest.json - Asset manifest
```

When embedded with `embed.FS`, static pages serve the pre-built HTML directly without invoking Bun:

```go
//go:embed all:.bifrost
var bifrostFS embed.FS

r, err := bifrost.New(bifrost.WithAssetsFS(bifrostFS))
```

**Note:** Zero-JavaScript static pages are not currently supported. All pages include React hydration.

**Use cases for static pages:**

- Documentation sites
- Marketing landing pages
- Blogs and content pages
- Any page that doesn't need server-side data fetching on each request

## Props and Data Flow

Go passes data to React components via the props loader:

```go
// Go
home := r.NewPage("./pages/home.tsx", func(req *http.Request) (map[string]interface{}, error) {
    return map[string]interface{}{
        "message": "Hello from Go!",
        "count":   42,
    }, nil
})
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

Return a redirect error from props loader:

```go
page := r.NewPage("./pages/protected.tsx", func(req *http.Request) (map[string]interface{}, error) {
    if !isAuthenticated(req) {
        return nil, &RedirectError{
            URL:    "/login",
            Status: http.StatusFound,
        }
    }
    // ...
})
```

Implement the `RedirectError` interface:

```go
type RedirectError interface {
    RedirectURL() string
    RedirectStatusCode() int
}
```

### Build Errors

Bifrost provides detailed error information for build failures:

```go
if err := r.Build(entrypoints, outdir); err != nil {
    if bifrostErr, ok := err.(*bifrost.BifrostError); ok {
        // Access detailed error info
        for _, e := range bifrostErr.Errors {
            fmt.Printf("Error in %s:%d:%d: %s\n", e.File, e.Line, e.Column, e.Message)
        }
    }
}
```

## Development vs Production

### Development Mode

Development uses [Air](https://github.com/cosmtrek/air) for hot reload with proxy configuration. Create `.air.toml` in your project root:

```toml
[build]
  cmd = "go build -o ./tmp/main main.go"
  bin = "./tmp/main"
  delay = 0
  exclude_dir = ["node_modules", ".bifrost", "tmp"]
  include_ext = ["go", "tsx", "ts", "css"]

[proxy]
  app_port = 8080
  proxy_port = 3000
  enabled = true
```

Run with:

```bash
air
```

Air watches Go and TypeScript files, rebuilds on changes, and proxies requests through port 3000 to your app on port 8080.

Development features:

- Hot reload on file changes
- Source maps
- Detailed error pages
- Assets served from disk

### Production Mode

- Assets embedded via `embed.FS`
- Optimized builds
- Render caching
- SSR bundles for production rendering
- Minimal error details

```bash
# Build (generates both client and SSR bundles)
bifrost-build main.go

# Run
./myapp
```

**SSR Bundles:**

For SSR pages, `bifrost-build` generates server-side bundles in `.bifrost/ssr/`:

- Pre-built for Bun runtime target
- Automatically extracted from `embed.FS` at runtime
- Used instead of source TSX files in production
- Allows single-binary deployment with embedded SSR

When `WithAssetsFS()` is provided:

1. SSR bundles are extracted to temp directory on first render
2. Subsequent renders use the cached temp files
3. Temp directory cleaned up on `renderer.Stop()`

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
│   ├── ssr/          # SSR bundles (production)
│   ├── pages/        # Static HTML files
│   └── manifest.json # Asset manifest
└── go.mod
```

## Complete Example

See the [example/](../example/) directory for a working implementation with:

- Multiple page types (SSR and static)
- Dynamic routes with URL parameters
- Error handling demos
- Asset embedding

## Best Practices

1. **Always defer Stop()**: `defer r.Stop()` after creating the renderer
2. **Use WithClientOnly() for marketing pages**: No runtime Bun dependency
3. **Keep props minimal**: Pass only necessary data to React
4. **Handle errors in props loaders**: Return proper errors or redirects
5. **Use embed.FS in production**: Ensures single binary deployment
