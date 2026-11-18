# Bifrost + MyGo example

One Go source for three targets, on the App Router: the home and about pages
the server renders for search engines, the notes screen only the app draws,
the JSON API both read, and the browser build the server serves.

```
main.go              MyGo: the window, the router over appPages, the notes state
app/page.go          home: View, Metadata, Screen, Cache
app/about/page.go    about: the same, a second public page
app/notes/page.go    notes: Screen(deps) only, noindex, no HTML on the server
app/api/notes/route.go   GET /api/notes
api/api.go           the client: fetch in a browser, net/http elsewhere
store/store.go       the notes the demo serves
```

`main.go` and the `zz_bifrost_routes.go` table that `bifrost build` generates
are the app. The server is generated too: there is no `server/` package to
write.

## Run it

```sh
bifrost build .                    # the server, .bifrost/bifrost-app
BIFROST_ADDR=127.0.0.1:8080 ./.bifrost/bifrost-app
```

`bifrost build` also compiles the browser build to `dist/web`, copies it inside
the binary, and serves it from there: `http://127.0.0.1:8080/` is the HTML, then
the app over it, and the binary deploys alone. `BIFROST_WASM_DIR` points it at a
directory instead, which is what `bifrost dev` does.

```sh
bifrost dev .                      # rebuild and restart on a Go change
go run .                           # the desktop app, BIFROST_EXAMPLE_API points it at a server
mygo build -platform darwin/universal   # or the signed app for each platform
```

The desktop app defaults to `http://localhost:8080`; in a browser it uses the
page's origin, so it works from any host.

## Verified

- `GET /` answers a document with `<title>Bifrost + MyGo</title>`, the `<h1>`
  and `<a href="/notes">` rendered by `seo.Body` from the MyGo view,
  `Cache-Control: public, s-maxage=60` from `Cache()`, `#mygo` and the script
  that boots `/app.wasm`, with no "Loading" label in it.
- `GET /about` is the second public page, in the sitemap.
- `GET /notes` answers the shell alone with `<meta name="robots"
  content="noindex">`: the app draws it, no crawler indexes it.
- `GET /api/notes` answers the JSON the app reads with the same client on
  desktop and in the browser.
- `GET /app.wasm` answers `application/wasm`, brotli when the client accepts it
  (3.1 MB from 13.8 MB), gzip otherwise, an ETag for a 304 on reload, and the
  new file after a rebuild without restarting the server.
- `GET /nope` answers 404 with the not-found page.
- `go vet ./...` and `GOOS=js GOARCH=wasm go build .` pass.

## Sizes

| | |
|---|---|
| `app.wasm` | 13.8 MB plain, 3.7 MB gzip, 3.1 MB brotli |
| server binary | 13.3 MB, and 30.9 MB carrying the browser build |
| minimal MyGo window | 13.3 MB of wasm, the UI tester alone 11.2 MB |

`client` keeps `net/http` out of the browser build: with it, the wasm grows
about 7 MB before compression.
