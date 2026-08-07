# Bifrost Example

This application exercises SSR, client-only pages, static prerendering, React hydration, CSS, and Tailwind.

## Prerequisites

- Go 1.26.0+
- A C toolchain (QuickJS backend)
- JavaScript packages installed in `node_modules`

## Development

```bash
cd example
make dev
```

## Production build and start

```bash
cd example
make start
```

The build records `"runtime": "quickjs"` in `.bifrost/manifest.json`. `BIFROST_QUICKJS_WORKERS` defaults to `min(GOMAXPROCS, 8)` and can be overridden when starting the app.

## Available routes

- `/` — React SSR and Tailwind
- `/about` — client-only page
- `/nested` — nested SSR component
- `/api-demo` — loader data and hydration
- `/product` — static prerender
- `/blog/hello-world` — static route with data
- `/dashboard?demo=true` — authenticated SSR example
- `/error-render` — render error handling
