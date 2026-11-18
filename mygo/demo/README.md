# Bifrost + MyGo demo

One Go source for three targets: the server renders the pages to HTML a search
engine reads, the browser boots the same Go as WebAssembly over that HTML, and
the window of a MyGo app draws the same views.

It is also the test of the framework: `BIFROST_E2E=1 go test ./cmd/bifrost -run
TestEndToEndDemoApp` builds it and reads every route below.

## Run it

```sh
bifrost dev                    # the server and the browser build, on :8080
go run .                       # the desktop app, a client of that server
bifrost build                  # dist/web and a server that carries it
./.bifrost/bifrost-app         # one binary: server, pages and browser build
```

`bifrost build` writes the server to `.bifrost/bifrost-app`, the browser build
to `dist/web`, and a copy of it inside the server, so a deploy is one file. The
server listens where `BIFROST_ADDR` says, and names the site in the sitemap
where `BIFROST_SITE_URL` says:

```sh
BIFROST_ADDR=:8080 BIFROST_SITE_URL=https://demo.bifrost.run ./.bifrost/bifrost-app
```

The app reads the server it is a client of from `BIFROST_DEMO_SERVER`,
`http://localhost:8080` by default.

## What it shows

| Route | What it is |
|---|---|
| `/` | A page with no loader: the server renders it once and keeps the HTML. |
| `/pricing` | A loader that returns props the view and the metadata read, cached for a shared cache. |
| `/about` | A view that draws alone: `Screen` is the same `View`. |
| `/blog` | A layout of its own under the layout of the app. |
| `/blog/{slug}` | A parameter, a loader that answers 404, and a `Sitemap` that lists the paths of the pattern. |
| `/blog/boom` | A loader that fails: the error page, with a 500 and `noindex`. |
| `/dashboard` | A screen the app draws alone: the server answers a shell nobody indexes. |
| `/login` | A form the app owns; the server issues the token it keeps. |
| `/api/notes` | `GET` and `POST` of the notes the dashboard loads. |
| `/api/login` | Credentials for a token. Any password of four letters or more. |
| `/api/refresh` | A token for a token: what the app calls when the server rejects one. |
| `/api/me` | A `Middleware` that refuses a request without a token. |
| `/sitemap.xml`, `/robots.txt` | The pages a crawler reads, and not the screens. |

The page of `/blog/one-source-three-targets` reads its canonical, its social
card, its `article` card type and its `BlogPosting` schema from the props of its
loader; `/` writes two JSON-LD blocks, its keywords and its alternate languages.

## The pieces

- `app/page.go` — a page: `Load`, `View`, `Metadata`, `Screen`, `Cache`, and
  `Sitemap` when the pattern has a parameter.
- `app/layout.go`, `app/blog/layout.go` — `Layout` wraps the pages of its
  directory and the ones below it, on the server and in the window.
- `app/error.go`, `app/not-found.go` — what a failed load and an unknown path
  answer.
- `app/api/*/route.go` — `Get`, `Post` and `Middleware` beside the pages.
- `store/` — the data, standing in for a database.
- `main.go` — the app: the window, the router over `appPages`, the session and
  the calls to the API.

A screen reads the page it shows through the interface it declares, and the app
satisfies it: `Screen(deps Deps)`. The compiler checks the app has the methods,
so no casting and no `any`.
