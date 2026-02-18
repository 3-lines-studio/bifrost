<p align="center">
  <img src="assets/bifrost.png" alt="Bifrost logo" width="200">
</p>

# Bifrost

Server-side rendering for React components in Go. Bridge your Go backend with React frontends.

## Installation

```bash
go get github.com/3-lines-studio/bifrost
```

Requires [Bun](https://bun.sh) to be installed for development and building.
Production binaries with SSR pages include the Bun runtime and do not require Bun to be installed on the target system.
Static-only apps (no SSR) do not include the Bun runtime.

## Features

- **SSR** - Server-side render React components
- **Hot Reload** - Auto-refresh in development
- **Props Loading** - Pass data from Go to React via typed options
- **File-based Routing** - Simple page organization
- **Production Ready** - Strict embedded assets for deployment
- **Self-Contained** - Single binary with embedded Bun runtime only when SSR pages are used
- **Static Site Generation** - Build static sites without runtime Bun dependency

## Development vs Production

Bifrost has two distinct modes with strict separation:

**Development** (`BIFROST_DEV=1`):

- Hot reload on file changes
- Source TSX files rendered directly
- Assets served from disk
- Requires Bun installed on system

**Production** (absence of `BIFROST_DEV`):

- **Requires** embedded assets via `embed.FS`
- **Requires** pre-built artifacts from `bifrost-build`
- Manifest-driven asset resolution
- Uses embedded Bun runtime for SSR pages (no system Bun required)
- Static-only apps do not include the Bun runtime
- Strict fail-fast on missing assets or runtime for SSR pages

## Quick Start

Copy and paste this command to create a new project:

```bash
go run github.com/3-lines-studio/bifrost/cmd/init@latest myapp
```

This creates a complete Bifrost project with a working SSR page and starts the dev server on http://localhost:8080.

Use another template:
- `go run github.com/3-lines-studio/bifrost/cmd/init@latest --template spa myapp`
- `go run github.com/3-lines-studio/bifrost/cmd/init@latest --template desktop myapp`

### Build for Production

```bash
# Build assets and SSR bundles
bifrost-build main.go

# Build Go binary with embedded assets
go build -o myapp main.go

# Run in production (no BIFROST_DEV set)
./myapp
```

### Repair Existing Project

If your `.bifrost` directory is missing or corrupted:

```bash
go run github.com/3-lines-studio/bifrost/cmd/doctor@latest .
```

`doctor` only repairs the `.bifrost` directory (does not scaffold missing app files).

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
    // Create Bifrost app with pages
    app := bifrost.New(
        bifrostFS,
        bifrost.Page("/", "./pages/home.tsx", 
            bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
                return map[string]any{
                    "name": "World",
                }, nil
            }),
        ),
        bifrost.Page("/about", "./pages/about.tsx", 
            bifrost.WithClient(),
        ),
        bifrost.Page("/blog", "./pages/blog.tsx",
            bifrost.WithStatic(),
        ),
    )
    defer app.Stop()
    
    // Setup your API routes
    api := http.NewServeMux()
    // ... add API endpoints
    
    // Start server (Wrap for custom API routes, use app.Handler() for Bifrost-only)
    log.Fatal(http.ListenAndServe(":8080", app.Wrap(api)))
}
```

### Page Options

- `WithLoader(fn)` - Provide a function to load props from the HTTP request
- `WithClient()` - Create a static page with client-side hydration only (empty shell)
- `WithStatic()` - Prerender full HTML at build time + client hydration
- `WithStaticData(fn)` - Prerender with dynamic paths from a data loader

### Mode Detection

Bifrost determines mode by checking the `BIFROST_DEV` environment variable:

- `BIFROST_DEV=1` -> Development mode
- Any other value or unset -> Production mode

In production mode:
- `embed.FS` is **required** and validated at startup
- SSR bundles are extracted from embedded assets
- Source TSX files are **not** used
- Missing assets cause immediate errors

## Static Site Generation

Build static sites that don't require Bun at runtime:

```go
// Client-only page (empty shell + client render)
bifrost.Page("/admin", "./pages/admin.tsx", bifrost.WithClient())

// Static prerender page (full HTML at build time + hydration)
bifrost.Page("/about", "./pages/about.tsx", bifrost.WithStatic())

// SSR page (server-side render on each request)
bifrost.Page("/dashboard", "./pages/dashboard.tsx", 
    bifrost.WithLoader(loader),
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

Bifrost enforces strict production requirements and **panics** on initialization errors:

- Missing `embed.FS` in production
- Missing manifest.json in embedded assets  
- Missing embedded Bun runtime (run `bifrost-build` to generate it)

These errors cause immediate panic at startup to fail fast.

## License

MIT
