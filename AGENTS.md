# Bifrost Agent Configuration

Bifrost is a Go library for server-side rendering React components using Bun. It bridges Go backends with React frontends.

## Build/Test Commands

```bash
# Run all checks
make check
```

## Development Notes

- Sobek is the default SSR backend and does not require Bun
- Bun is an optional backend selected with `BIFROST_JS_RUNTIME=bun`
- The Bun backend uses Unix sockets for Go-Bun communication
- Supports hot reload in development mode via bifrost dev
- Production builds embed assets using `embed.FS`
