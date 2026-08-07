<p align="center">
  <img src="assets/bifrost.png" alt="Bifrost logo" width="200">
</p>

# Bifrost

Server-side rendering for React pages from Go: register routes, embed build output, and serve HTML through `net/http`.

## Requirements

- Linux or macOS (QuickJS is a cgo backend, so a POSIX toolchain is required; Windows is not supported)
- [Go](https://go.dev/dl/) 1.26.0 or newer
- JavaScript packages installed in `node_modules` with npm, pnpm, Bun, or another package manager
- A C toolchain (`gcc`/`clang`) — required by the QuickJS backend

## Install

```bash
go get github.com/3-lines-studio/bifrost
go install github.com/3-lines-studio/bifrost/cmd/bifrost@latest
```

## New project

```bash
bifrost init myapp
cd myapp
npm install --legacy-peer-deps
```

Templates: `minimal` (default), `spa` — e.g. `bifrost init --template spa myapp`.

## Development

```bash
bifrost dev ./main.go
```

Hot reload on `.go` file changes with reverse proxy on `:3000` → `:8080`. Frontend changes (`.tsx`, `.ts`, `.css`) are rebuilt and reloaded by the default QuickJS backend.

## Production build

1. In your `main` package, embed the build tree: `//go:embed all:.bifrost`
2. Generate assets (output goes to `dir(main.go)/.bifrost` — the app root, same directory as your main package):

   ```bash
   bifrost build ./main.go
   ```

3. `go build` your app and run it **without** `BIFROST_DEV=1`.

`bifrost build` exits non-zero if a required page or bundle fails. Build-scanned `Page` declarations must use string-literal component paths and direct Bifrost option calls; unsupported indirect forms fail with a clear error.

Production `/dist/` assets are content-hashed and served with a one-year immutable cache policy. QuickJS is the JavaScript backend: it runs in-process (cgo, vendored quickjs-ng) with a bounded worker pool, so the production binary has no child process and no external runtime.

Use `http.Server` with graceful `SIGINT`/`SIGTERM` shutdown so deferred `app.Stop()` cleanup runs. Generated projects include this setup.

`go install github.com/3-lines-studio/bifrost/cmd/bifrost@latest` installs a binary named `bifrost`; run `bifrost init`, `bifrost dev`, `bifrost build`, or `bifrost doctor`.

## `.bifrost` directory

If `.bifrost` is missing for `go:embed`, repair the placeholder tree:

```bash
bifrost doctor .
```

## Documentation

API, page modes (`WithLoader`, `WithClient`, `WithStatic`, …), redirects, and behavior details: [docs.md](docs.md).

## Developing this repository

```bash
make check
make bench
make bench-browser   # real-browser baseline (bench/, needs Chromium)
make bench-sweep     # one-at-a-time knob sweep + soak
```

See [RELEASING.md](RELEASING.md) for the v1 checklist and performance baseline.

## License

MIT
