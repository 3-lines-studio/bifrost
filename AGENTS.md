# Bifrost Agent Configuration

Bifrost is a Go library for server-side rendering React components using Bun. It bridges Go backends with React frontends.

## Build/Test Commands

```bash
# Run all checks
make check
```

## Development Notes

- Bun is required for SSR functionality
- Uses Unix sockets for Go-Bun communication
- Supports hot reload in development mode via air
- Production builds embed assets using `embed.FS`
