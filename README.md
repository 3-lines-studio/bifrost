<p align="center">
  <img src="assets/bifrost.png" alt="Bifrost logo" width="200">
</p>

# Bifrost

Server-side rendering for React pages from Go: register routes, embed build output, and serve HTML through `net/http`.

## Requirements

- Linux or macOS (Bifrost uses Unix sockets; Windows is not supported)
- [Go](https://go.dev/dl/) 1.26.0 or newer
- [Bun](https://bun.sh) on the machine where you develop and where you run production builds; SSR production binaries embed the Bun runtime (static-only apps do not)

## Install

```bash
go get github.com/3-lines-studio/bifrost
go install github.com/3-lines-studio/bifrost/cmd/bifrost@latest
```

## New project

```bash
bifrost init myapp
```

Templates: `minimal` (default), `spa` — e.g. `bifrost init --template spa myapp`.

## Development

```bash
bifrost dev ./main.go
```

Hot reload on `.go` file changes with reverse proxy on `:3000` → `:8080`. Frontend changes (`.tsx`, `.ts`, `.css`) are picked up live via Bun per-request re-import.

## Production build

1. In your `main` package, embed the build tree: `//go:embed all:.bifrost`
2. Generate assets (output goes to `dir(main.go)/.bifrost` — the app root, same directory as your main package):

   ```bash
   bifrost build ./main.go
   ```

3. `go build` your app and run it **without** `BIFROST_DEV=1`.

`bifrost build` exits non-zero if a required page or bundle fails. Build-scanned `Page` declarations must use string-literal component paths and direct Bifrost option calls; unsupported indirect forms fail with a clear error.

Production `/dist/` assets are content-hashed and served with a one-year immutable cache policy. Bifrost v1 uses one Bun renderer process per app.

Use `http.Server` with graceful `SIGINT`/`SIGTERM` shutdown so deferred `app.Stop()` cleanup runs. Generated projects include this setup. The Bun child also watches its parent and exits if the Go process stops abruptly.

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
```

See [RELEASING.md](RELEASING.md) for the v1 checklist and performance baseline.

## License

MIT
