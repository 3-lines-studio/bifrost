# Bifrost Agent Configuration

Bifrost is a Go library for server-side rendering React components using Bun. It bridges Go backends with React frontends.

## Build/Test Commands

```bash
# Run all tests
go test ./...

# Run specific test (e.g., TestRenderer)
go test -run TestRenderer ./...

# Run tests with verbose output
go test -v ./...

# Build the package
go build

# Clean build artifacts
go clean

# Run all checks (test + build)
make check
```

## Code Style Guidelines

### Go Conventions

- **Naming**: Use `snake_case` for Go files (e.g., `renderer.go`, `page_handler.go`)
- **Package names**: Short, lowercase, no underscores (e.g., `bifrost`, not `bifrost_lib`)
- **Types**: PascalCase for exported types (e.g., `Renderer`, `Page`, `RedirectError`)
- **Functions**: PascalCase for exported, camelCase for internal
- **Constants**: PascalCase for exported (e.g., `PublicDir`, `BifrostDir`)
- **Interfaces**: Single-method interfaces use `-er` suffix (e.g., `RedirectError`)

## Architecture

### Key Patterns

1. **Renderer**: Manages Bun process for SSR
2. **Page**: HTTP handler for component routes
3. **Options**: Functional options pattern for configuration
4. **Embed**: Static assets embedded for production

## Development Notes

- Bun is required for SSR functionality
- Uses Unix sockets for Go-Bun communication
- Supports hot reload in development mode
- Production builds embed assets using `embed.FS`
