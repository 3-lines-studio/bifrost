# Bifrost

Server-side rendering for React components in Go. Bridge your Go backend with React frontends.

## Installation

```bash
go get github.com/3-lines-studio/bifrost
```

Requires [Bun](https://bun.sh) to be installed.

## Features

- **SSR** - Server-side render React components
- **Hot Reload** - Auto-refresh in development
- **Props Loading** - Pass data from Go to React via typed options
- **File-based Routing** - Simple page organization
- **Production Ready** - Strict embedded assets for deployment
- **Static Site Generation** - Build static sites without runtime Bun dependency

## Development vs Production

Bifrost has two distinct modes with strict separation:

**Development** (`BIFROST_DEV=1`):

- Hot reload on file changes
- Source TSX files rendered directly
- Assets served from disk
- No embedded assets required

**Production** (absence of `BIFROST_DEV`):

- **Requires** embedded assets via `WithAssetsFS()`
- **Requires** pre-built SSR bundles from `bifrost-build`
- Manifest-driven asset resolution
- Strict fail-fast on missing assets

## Quick Start

### Initialize Project

```bash
# Create .bifrost directory for build artifacts
go run github.com/3-lines-studio/bifrost/cmd/init@latest .
```

### Development

```bash
# Set development mode
export BIFROST_DEV=1

# Run your app
go run main.go
```

### Build for Production

```bash
# Build assets and SSR bundles
bifrost-build main.go

# Build Go binary with embedded assets
go build -o myapp main.go

# Run in production (no BIFROST_DEV set)
./myapp
```

## API

### Creating Pages

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
    // Production: requires WithAssetsFS
    // Development: works without it
    r, err := bifrost.New(bifrost.WithAssetsFS(bifrostFS))
    if err != nil {
        log.Fatal(err)
    }
    defer r.Stop()
    
    // SSR page with props loader
    home := r.NewPage("./pages/home.tsx", 
        bifrost.WithPropsLoader(func(req *http.Request) (map[string]any, error) {
            return map[string]any{
                "name": "World",
            }, nil
        }),
    )
    
    // Static page (client-side only, no SSR)
    about := r.NewPage("./pages/about.tsx", 
        bifrost.WithClientOnly(),
    )
    
    // Static prerender page (full HTML at build time + hydration)
    blog := r.NewPage("./pages/blog.tsx",
        bifrost.WithStaticPrerender(),
    )
    
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

### Page Options

- `WithPropsLoader(fn)` - Provide a function to load props from the HTTP request
- `WithClientOnly()` - Create a static page with client-side hydration only (empty shell)
- `WithStaticPrerender()` - Prerender full HTML at build time + client hydration

### Mode Detection

Bifrost determines mode by checking the `BIFROST_DEV` environment variable:

- `BIFROST_DEV=1` → Development mode
- Any other value or unset → Production mode

In production mode:
- `WithAssetsFS()` is **required** and validated at startup
- SSR bundles are extracted from embedded assets
- Source TSX files are **not** used
- Missing assets cause immediate errors

## Static Site Generation

Build static sites that don't require Bun at runtime:

```go
// Client-only page (empty shell + client render)
home := r.NewPage("./pages/home.tsx", bifrost.WithClientOnly())

// Static prerender page (full HTML at build time + hydration)
blog := r.NewPage("./pages/blog.tsx", bifrost.WithStaticPrerender())

// SSR page (server-side render on each request)
dynamic := r.NewPage("./pages/dynamic.tsx", 
    bifrost.WithPropsLoader(loader),
)
```

Build with:

```bash
bifrost-build main.go
```

This generates:

- `.bifrost/dist/` - Client JS/CSS bundles
- `.bifrost/ssr/` - Server bundles for SSR pages only
- `.bifrost/pages/` - Static HTML files:
  - Client-only: empty shell HTML
  - Static prerender: full HTML with rendered body
- `.bifrost/manifest.json` - Asset manifest with mode info

## Architecture

Bifrost is organized into focused internal packages:

- `internal/types` - Shared types and contracts
- `internal/runtime` - Bun process management and IPC
- `internal/page` - HTTP handler orchestration
- `internal/assets` - Manifest resolution and embedded FS
- `internal/build` - Build pipeline and AST discovery
- `internal/cli` - Terminal output helpers

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

Bifrost enforces strict production requirements:

- `ErrAssetsFSRequiredInProd` - Returned when `WithAssetsFS()` is missing in production
- `ErrManifestMissingInAssetsFS` - Returned when manifest.json is not found in embedded assets

These errors are returned from `bifrost.New()` to fail fast at startup.

## License

MIT
