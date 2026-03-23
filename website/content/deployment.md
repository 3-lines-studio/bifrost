---
title: Deployment
description: Build and deploy Bifrost apps as single binaries.
order: 5
---

## Build Pipeline

The build command prepares all frontend assets for production:

```bash
go run github.com/3-lines-studio/bifrost/cmd/build@latest ./main.go
```

The build pipeline:

1. Scans your Go source for `Page()` calls
2. Generates client entry files for each page
3. Builds client bundles (JS/CSS) to `.bifrost/dist/`
4. Builds SSR bundles to `.bifrost/ssr/` (for SSR pages only)
5. Pre-renders static HTML for static pages
6. Copies `public/` assets
7. Generates `manifest.json`

## Embedding Assets

Use `embed.FS` to include all build output in the binary:

```go
//go:embed all:.bifrost
var bifrostFS embed.FS
```

If you serve files from `public/`, embed that too:

```go
//go:embed all:.bifrost all:public
var bifrostFS embed.FS
```

## Binary Size

- **Static-only apps** — No Bun runtime included. Smallest binary size.
- **SSR apps** — Bun runtime is embedded automatically for server-side rendering.

## SSR performance

For HTML responses, disable reverse-proxy buffering (for example nginx `proxy_buffering off` on page routes) so streaming and early flush reach the browser. For the fastest first paint on marketing-style routes, use static prerender (`WithStatic`) so pages are served as prebuilt HTML without invoking Bun per request.

## Routing with an API

Use `app.Wrap()` to combine Bifrost pages with your API routes:

```go
api := http.NewServeMux()
api.HandleFunc("/api/users", handleUsers)

handler := app.Wrap(api)
log.Fatal(http.ListenAndServe(":8080", handler))
```

Use `app.Handler()` if you only serve Bifrost pages:

```go
log.Fatal(http.ListenAndServe(":8080", app.Handler()))
```

## Repair

If the `.bifrost` directory is missing or corrupted:

```bash
go run github.com/3-lines-studio/bifrost/cmd/doctor@latest .
```

This repairs the build directory without scaffolding missing app files.
