# Bifrost + MyGo

One Go source for three targets: the server that search engines read, the
browser build, and the desktop or mobile app MyGo draws. This module is the
bridge: Bifrost renders the public pages on the server, MyGo draws the app, and
the app boots over the HTML.

It is a module of its own, so Bifrost keeps its dependencies to the standard
library and MyGo is downloaded only by apps that use it.

[example](example) is the smallest app: a page, a screen and an API.
[demo](demo) is all of it: SEO metadata, a blog with a dynamic sitemap, a
layout tree, an error page, a not-found page, a sign-in flow and a single-binary
deploy. `BIFROST_E2E=1 go test ./cmd/bifrost -run TestEndToEndDemoApp` builds
the demo and reads every route it serves.

## The App Router contract

A directory with `page.go` is a page. `bifrost build` reads it and generates
the server and the app's route table; nothing is wired by hand.

```go
// app/pricing/page.go
package pricing

import (
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

func View() func(*ui.Context) {           // the page: HTML on the server, UI in the app
	return func(c *ui.Context) {
		ui.Column(c).Padding(24).Children(func() {
			ui.Text(c, "Pricing").FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			seo.Link(c, "About", "/about")
		})
	}
}

func Metadata() seo.Head {                 // what a crawler reads
	return seo.Head{Title: "Pricing", Canonical: "https://example.com/pricing"}
}

func Screen() func(*ui.Context) { return View() } // the page in the app, made by appPages
```

| In `page.go` | |
|---|---|
| `View()` or `View(props Props) func(*ui.Context)` | the page. `props` come from `Load` |
| `Metadata()` or `Metadata(props Props) seo.Head` | title, description, canonical, image, robots, keywords, card type, alternates, JSON-LD, `lang`, `dir` |
| `Screen()` or `Screen(deps D) func(*ui.Context)` | the page in the app, typed: no `any`, no assertions |
| `Load(*http.Request) (any, error)` | server props, as a `Props` value or `bifrost.PageData` |
| `Cache() string` | the `Cache-Control` of the rendered page |
| `Sitemap() []string` | the paths a pattern with a parameter serves |
| `var Links map[string]string` | where a link drawn with `ui.Link` goes |

A `page.go` with `Screen` and no `View` is a screen of the app: the server
answers it with the shell alone, `Robots: "noindex"`, and leaves it out of the
sitemap. A page that sells or is searched has a `View`.

`layout.go` in a directory wraps the pages of that directory and the ones
below it, on the server and in the app:

```go
func Layout(page func(*ui.Context)) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Children(func() {
			nav(c)
			page(c)
		})
	}
}
```

`not-found.go` serves a path no page holds, with a 404:

```go
func NotFound() func(*ui.Context)          { ... }
func NotFoundMetadata() seo.Head           { ... }
```

`error.go` serves a page whose `Load` failed, with the status the error wraps
or a 500, `noindex` and never cached. The nearest one above the page wins, and
it gets the layouts of its own directory. A load that answers
`bifrost.NotFound()` shows the `not-found.go` above the page instead:

```go
func Error(err error) func(*ui.Context)    { ... } // the app draws it, and the server for crawlers
func ErrorMetadata() seo.Head              { ... }
```

`route.go` in a directory answers HTTP methods, as `Get(w, r)` for `GET /api`.

## Loading and serving the app

Bifrost composes each document: `seo.Body` renders the view in MyGo's headless
tester from the accessibility tree of its first frame, and `web.Shell` wraps it
in `<div id="seo">`, adds the empty `#mygo` element, and boots `/app.wasm` over
it at the URL's path. The bootstrap happens in the fragment, because MyGo's
first router follows the tab's history: `/pricing` becomes `#/pricing`. The
spinner waits 400 ms, so a cached build never flashes it, and a build that
fails to load leaves the HTML visible.

`bifrost build` compiles the browser build to `dist/web` (`app.wasm` with
`-ldflags=-s -w`, its gzip, and the `wasm_exec.js` of the toolchain), copies it
inside the generated server, and serves `/app.wasm` and `/wasm_exec.js` from
there: pages and app from one binary. `BIFROST_WASM_DIR` points it at a
directory instead, which is what `bifrost dev` does. `web.FileServer` is what it
mounts, over a directory or over a build embedded with `//go:embed`:

```go
files := web.FileServerFS(wasmFS())   // or web.FileServer("dist/web")
mux.Handle("GET /app.wasm", files)
mux.Handle("GET /wasm_exec.js", files)
```

It serves the compressed build beside the file (`app.wasm.gz`) when a build
wrote one, compresses the wasm itself in the background otherwise, prefers
brotli (`br`) over gzip, answers `304` to a matching ETag, and reads the file
again when it changes, so development needs no restart.

`bifrost dev` sets `BIFROST_DEV`, and the shell then carries the script that
follows the server: `web.Reload()` keeps an `EventSource` on `web.ReloadPath`,
and a page that reads the id of another process reloads itself.
`web.ReloadHandler()` answers that stream. A build leaves the script out, so a
production page never follows anything.

## Calling the server

`client` calls an HTTP server from every target: `fetch` in a browser, so the
wasm carries no HTTP stack and the browser does TLS; `net/http` elsewhere.

```go
notes, err := client.GetJSON[[]Note](ctx, base+"/api/notes", nil)
answer, err := client.SendJSON[Reply](ctx, "POST", base+"/api/notes", header, input)
```

`auth` keeps the bearer token: there is no cookie jar in a native window, so
the app holds the token and sends it in the `Authorization` header.

```go
session := auth.NewSession(auth.System()).WithRefresh(renew) // keychain, or localStorage
if err := session.Login(token); err != nil { ... }
response, err := session.Do(ctx, client.Request{URL: base + "/api/me"})
```

`Do` adds the header to every request, and one the server rejects with a 401
renews the token once through `renew` before it gives up; a session without a
refresher, or one whose refresher fails, signs out and answers the 401.

A backend without a secret store answers `errors.ErrUnsupported`; the app can
fall back to a file of its own or to asking for the token again.

## Limits

- A call to the server blocks until the answer arrives, and in a browser the
  answer arrives on the goroutine the app draws on: call from a goroutine, and
  apply what it answered in `Window.Update`. `client.Do` answers
  `client.ErrUIThread` instead of waiting forever.
- Links carry no URL in the accessibility tree, so `seo.Link` records where each
  one goes while the view renders. A view that calls `ui.Link` directly needs
  the page's `Links` map, keyed by accessible name.
- A `ui.Text` with a `Label` renders the label, not its text, as screen readers
  read it. Give content texts no label.
- Lists and grids build only the rows in view: the snapshot height decides how
  much of a long list reaches the HTML.
- A composite heading with child texts renders its name once; the children are
  dropped by their bounds.
- `seo.Options` defaults to a mobile viewport, 390×844, as crawlers read.
- No images, no CSS beyond what `web.Shell` carries for the content, no
  hydration: the server draws text and links, and the app is the UI that boots
  over it.
- Assistive technology in browsers still does not see MyGo's canvas. This adds
  no accessibility to the app, only text for crawlers.
- Server code in a page package (`Load` with `net/http`) is linked into the
  wasm: the linker drops what nothing calls, but `net/http` keeps about 1.6 MB
  of its own. `client` keeps it out of the app.

## Size

Measured on this machine with Go 1.27 and `-ldflags=-s -w`: a window and one
text are 13.3 MB of wasm, the UI tester alone 11.2 MB. The
[example](example) is 13.8 MB plain, 3.7 MB gzipped and 3.1 MB brotli, and its
server is a single 13.3 MB binary. The [demo](demo) is 16.1 MB plain, 4.3 MB
gzipped and 3.6 MB brotli, and its server carries the build: one 33.8 MB file.
