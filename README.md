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
- **Props Loading** - Pass data from Go to React
- **File-based Routing** - Simple page organization
- **Production Ready** - Embedded assets for deployment
- **Static Site Generation** - Build static sites without runtime Bun dependency

## Development vs Production

**Development** (`BIFROST_DEV=1`):

- Hot reload on file changes
- Source maps enabled
- Assets served from disk

**Production**:

- Embedded assets using `embed.FS`
- Optimized builds
- Cached renders

## Quick Start

### Development

```bash
# From the project root
make dev

# Or from the example directory
cd example
make dev
```

Then open http://localhost:8080

### Build for Production

```bash
cd example
make build
./bifrost
```

## Static Site Generation

Bifrost now supports building static sites that don't require Bun at runtime:

```go
// Static page (no SSR)
home := r.NewPage("./pages/home.tsx", bifrost.WithStatic())

// SSR page (with server-side rendering)
dynamic := r.NewPage("./pages/dynamic.tsx", propsLoader)
```

Build with:
```bash
bifrost-build main.go
```

This generates:
- Static HTML files for pages marked with `WithStatic()`
- JS bundles for client-side hydration
- Manifest for asset mapping

Perfect for CLI tools with embedded web UIs!
