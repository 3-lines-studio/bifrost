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
| `Load(*http.Request) (any, error)` | server props, as a `Props` value or `bifrost.PageData` (status, document attributes, headers) |
| `Cache() string` | the `Cache-Control` of the rendered page |
| `Sitemap() []string` or `[]bifrost.SitemapEntry` | the paths a pattern with a parameter serves, with when each changed and its languages |
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

A request sends a `client.Blob` where it is: the bytes of memory, or a range of
a file the disk or the browser holds, without reading it into Go first.
`OnUpload` reports the bytes as they go out, and in a browser the request goes
with `XMLHttpRequest`, whose upload reports and which takes a `File` without a
copy.

```go
client.Do(ctx, client.Request{
	Method:   "PUT",
	URL:      signed,
	Blob:     picture.Blob(0, picture.Size()),
	OnUpload: func(sent int64) { ... },
})
```

`client.Copy` writes an answer to a writer as it arrives, which is a download.

## Files, pictures and uploads

`file` hands the app the files of the person: `file.Pick` in a window's dialog
or in the browser's picker, `file.Open` by path, and `file.Save` to download
one. A `File` reads in ranges, so a picture of 40 MiB goes out in parts:

```go
picture, err := file.Open(path)
head, err := picture.Head(ctx, 256<<10)          // what a decoder needs
metadata, err := exif.Read(ctx, picture, picture.Size())
```

`exif` reads the size, the orientation, the camera and lens, the exposure, the
time and the place of a JPEG, PNG or WebP from its headers, without decoding
the picture: what an upload sends to the server before the file.

`upload` sends the file in parts to a server that signs them, as S3 takes them:
concurrency per file, retries with backoff, the part size, progress, and a
`Store` that resumes an upload a reload interrupted.

```go
result, err := upload.Run(ctx, picture, signer, upload.Options{
	Store:      upload.NewMemory(),
	OnProgress: func(p upload.Progress) { win.Update(func() { show(p) }) },
})
```

The app implements `upload.Signer` on its own API: `Start` opens the upload,
`Sign` gives the URL of each part, `Complete` joins them and `Abort` drops one
nobody finishes. Everything else is the package's.

A `Store` keeps the parts the server already has: `upload.NewMemory()` while the
app runs, `upload.Disk(dir)` in a window, so a resume survives the app closing,
and `upload.Browser()` in a page, which keeps it in `localStorage`.

## State of a screen

`data.Cache` holds what a screen reads, loads it on a goroutine of its own, and
tells what changed, so the view reads a value without waiting for it:

```go
notes := data.New(loadNotes)                       // func(ctx, key) ([]Note, error)

switch entry := notes.Get(0); entry.State {
case data.Loading: ui.Spinner(c)
case data.Failed:  ui.Text(c, entry.Err.Error())
default:           list(c, entry.Value)
}

notes.OnChange(func(int) { win.Update(screen.refresh) })
```

`Get` starts one load per key, `Invalidate` marks a value stale, `Refresh`
loads it now, `Set` and `Update` write a value the app knows, and `Forget`
drops one. A failure stays until the app asks again.

## Signing people in

`auth.Flow` builds the authorization code flow of OAuth 2.0 with PKCE: `Start`
returns the page to open and the secrets that check the answer, and `Verify`
the code. `auth.Open` opens the browser of the system. In a window,
`auth.Listen` waits for the browser to come back on the loopback address; in a
browser the provider redirects to the app's server, which sets the session
cookie, and nothing listens there.

```go
flow := auth.Flow{AuthorizeURL: ..., ClientID: ..., RedirectURI: callback.URL(), Scopes: []string{"openid"}}
request, err := flow.Start()
auth.Open(request.URL)
answer, err := callback.Wait(ctx)
code, err := flow.Verify(answer, request)
```

The same pair opens what a payment needs: `auth.Open(gateway.URL)` sends the
person to the checkout page of the provider, and `mygo.App.OnOpenURL` gets them
back when the provider redirects to the scheme the app registered, as
`picsel://paid/...` in `urlSchemes` of `mygo.config.ts`. An app waits for the
order by polling the state of it in `data.Cache`, not by blocking the screen.

## Codes and files

`qr.Encode` makes a QR code of a text, `qr.Element` draws it in a view, and
`Code.PNG` and `Code.SVG` write it for a page or an email. It carries no
dependency, and the codes it makes read back in `zbar` at every level, from
version 1 to 36.

`web.Static` serves a directory of files, as the favicons, the manifest and the
pictures of a page, with the cache header a deploy wants.

`seo.Options.Locale` renders a page in the language of the request, as its
messages and its text take it, so one page serves `es`, `en` and `pt-BR` with
the words of each.

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
- `file.Pick`, `File.Head` and `auth.Listen` wait for the person and for the
  browser: call them from a goroutine. On the goroutine that draws, a browser
  answers `file.ErrUIThread`, which is `client.ErrUIThread`.
- In a browser a download is the browser's: `file.Save` hands it the URL, so
  the URL must authorize itself, as a signed one does, or ride the cookies of
  the page. Progress is a window's.
- `auth.Listen` is a window's: a browser signs in through the server that
  answers the redirect.
- A QR code chooses its mask by the score of the specification, and correct
  encoders differ in how they break a tie, as `python-qrcode` and `qrencode` do.
  Encode a code once and draw the `Code`: `Element` converts its bitmap once.
- The app's own font goes in with `ui.RegisterFont(data, family)` and an
  `//go:embed` of the file, and a text takes it with `.Font(family, size)`. The
  HTML `web.Shell` serves is styled with the system stack, so a page reads in
  the system font until the app draws.

## Size

Measured on this machine with Go 1.27 and `-ldflags=-s -w`: a window and one
text are 13.3 MB of wasm, the UI tester alone 11.2 MB. The
[example](example) is 13.8 MB plain, 3.7 MB gzipped and 3.1 MB brotli, and its
server is a single 13.3 MB binary. The [demo](demo) is 16.1 MB plain, 4.3 MB
gzipped and 3.6 MB brotli, and its server carries the build: one 33.8 MB file.
