# Bifrost Example

This application exercises SSR, client-only pages, static prerendering, React hydration, CSS, and Tailwind.

## Prerequisites

- Go 1.26.0+
- JavaScript packages installed in `node_modules`
- Bun for the Bun backend, or npm/pnpm for package installation when testing Sobek without Bun

## Bun backend

```bash
cd example
make dev
```

Production:

```bash
cd example
make start
```

## Sobek backend

Development:

```bash
cd example
make dev-sobek
```

Production build and start:

```bash
cd example
make build-sobek
./bifrost-sobek
```

The Sobek build records `"runtime": "sobek"` in `.bifrost/manifest.json`, so the production binary does not need `BIFROST_JS_RUNTIME` set. `BIFROST_SOBEK_WORKERS` defaults to `min(GOMAXPROCS, 4)` and can be overridden when starting the app.

Open <http://localhost:8080>.

### Prove Bun is not used

Use a `PATH` that does not contain Bun:

```bash
cd ..
go build -o /tmp/bifrost-sobek ./cmd/bifrost
cd example
PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin \
  BIFROST_JS_RUNTIME=sobek \
  /tmp/bifrost-sobek build ./cmd/full/main.go --go-build=./bifrost-sobek

PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin ./bifrost-sobek
```

Checks:

```bash
curl -fsS http://localhost:8080/ | grep "Bifrost Examples"
test ! -d cmd/full/.bifrost/runtime
grep '"runtime": "sobek"' cmd/full/.bifrost/manifest.json
```

## Available routes

- `/` — React SSR and Tailwind
- `/about` — client-only page
- `/nested` — nested SSR component
- `/api-demo` — loader data and hydration
- `/product` — static prerender
- `/blog/hello-world` — static route with data
- `/dashboard?demo=true` — authenticated SSR example
- `/error-render` — render error handling
