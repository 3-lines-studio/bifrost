# Bifrost Agent Configuration

Bifrost is a Go library for server-side rendering React components using QuickJS. It bridges Go backends with React frontends.

## Build/Test Commands

```bash
# Run all checks
make check
```

## Development Notes

- QuickJS is the SSR backend (cgo, vendored quickjs-ng); there is no other runtime
- QuickJS runs in-process with a bounded worker pool; `BIFROST_QUICKJS_WORKERS` overrides `min(GOMAXPROCS, 8)`
- Supports hot reload in development mode via bifrost dev
- Production builds embed assets using `embed.FS`
