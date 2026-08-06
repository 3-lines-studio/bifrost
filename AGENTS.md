# Bifrost Agent Configuration

Bifrost is a Go library for server-side rendering React components using Bun. It bridges Go backends with React frontends.

## Build/Test Commands

```bash
# Run all checks
make check
```

## Development Notes

- QuickJS is the default SSR backend (cgo, vendored quickjs-ng); it does not require Bun
- Sobek is the pure-Go backend selected with `BIFROST_JS_RUNTIME=sobek`
- Bun is an optional backend selected with `BIFROST_JS_RUNTIME=bun`
- The Bun backend uses Unix sockets for Go-Bun communication
- Supports hot reload in development mode via bifrost dev
- Production builds embed assets using `embed.FS`
