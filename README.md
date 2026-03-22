<p align="center">
  <img src="assets/bifrost.png" alt="Bifrost logo" width="200">
</p>

# Bifrost

Server-side rendering for React pages from Go: register routes, embed build output, and serve HTML through `net/http`.

## Requirements

- [Go](https://go.dev/dl/) 1.25 or newer
- [Bun](https://bun.sh) on the machine where you develop and where you run production builds; SSR production binaries embed the Bun runtime (static-only apps do not)

## Install

```bash
go get github.com/3-lines-studio/bifrost
```

## New project

```bash
go run github.com/3-lines-studio/bifrost/cmd/init@latest myapp
```

Templates: `minimal` (default), `spa`, `desktop` — e.g. `go run github.com/3-lines-studio/bifrost/cmd/init@latest --template spa myapp`.

## Production build

1. In your `main` package, embed the build tree: `//go:embed all:.bifrost`
2. Generate assets (from your module root, pointing at the same `main` you will build):

   ```bash
   go run github.com/3-lines-studio/bifrost/cmd/build@latest ./main.go
   ```

3. `go build` your app and run it **without** `BIFROST_DEV=1`.

`go install github.com/3-lines-studio/bifrost/cmd/build@latest` installs a binary named `build` (the directory name); rename or alias if you want `bifrost-build` on your PATH.

## `.bifrost` directory

If `.bifrost` is missing for `go:embed`, repair the placeholder tree:

```bash
go run github.com/3-lines-studio/bifrost/cmd/doctor@latest .
```

## Documentation

API, page modes (`WithLoader`, `WithClient`, `WithStatic`, …), redirects, and behavior details: [docs.md](docs.md).

## Developing this repository

```bash
make check
```

## License

MIT
