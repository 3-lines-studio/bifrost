# App Router demo

An app that exercises every App Router feature on purpose, and a script that asserts each one with `curl`.

```
make build     # go run ../../cmd/bifrost build .
make dev       # development server with HMR
make serve     # build and serve on 127.0.0.1:18700
make check     # build, then run scripts/check.sh (71 assertions)
```

## The tree

```
app/
  layout.tsx  template.tsx  loading.tsx  error.tsx  not-found.tsx  page.tsx  page.go
  middleware.go  server.go
  _components/            private: shared components, never routed
  store/                  private: Go data shared by the loaders
  docs/
    layout.tsx  loading.tsx  error.tsx  page.tsx  page.go
    slug__/               catch-all, with generateMetadata per doc
  projects/
    layout.tsx  page.tsx  page.go  middleware.go
    slug_/                dynamic, plus route.go for POST, PUT, PATCH, DELETE, OPTIONS
      log/                nested dynamic + static segment
    new/                  static beats dynamic
  admin/
    middleware.go         guards the whole subtree, including route.go
    login/route.go        sets the session cookie, no JavaScript needed
    api/state/route.go
  api/
    search/route.go       GET, and a 405 for the rest
    echo/route.go         all seven methods
    export/route.go       a hand-written text/markdown route
  labs/                   one experiment per behaviour, each one self-explaining
public/                   styles.css and a download target
```

## What it proves

Routing: markers, route groups, private folders, catch-all and dynamic params, static over dynamic, nested layouts, templates, per-segment middleware and `server.go`.

Data: loaders returning maps or `bifrost.PageData`, `params`, `searchParams` with repeated keys, `pathname`, merged metadata with a page override, `generateMetadata`, and per-request root document attributes.

HTTP: pages and `route.go` in the same tree, all methods, redirects from a loader and from `server.go`, a 404 with a status the loader chooses, a 405, JSON, Markdown by suffix and by `Accept`, and public files.

Client: in-place navigation, loading views, error views with `reset`, request-scoped state under concurrency, `data-bifrost-reload`, downloads, hash-only navigation, and a plain form that works with JavaScript off.

## Two framework bugs this demo found

Both are reported and not fixed here. The demo works around them.

1. **A layout that exports `Head` and `metadata` silently drops the `Head`.** The builder rejects the combination in a page (`exports both Head and metadata; keep one`) but accepts it in a layout, where the `Head` component is then ignored: `app/layout.tsx` linked the stylesheet and the link never reached the HTML. Workaround here: the stylesheet is a `<link precedence="high">` inside the layout body, which React 19 hoists.

2. **`bifrost.NotFound()` from a loader returns a 500 when the route has no `not-found.tsx` of its own.** With a catch-all page in `app/docs/slug__` and the only not-found view at the root, the response is a plain 500 instead of the nearest not-found view with a 404, which is what Next does. The generated loader returns the right props (`__bifrostNotFound`, `ErrorFallbacks: 2`) and the generated view renders `<NotFound/>` inside the docs layout, so the failure is in the render. One clue: that branch does not go through `requestPage`, so its props carry no `params`, `searchParams` or `pathname`, unlike every other path. The 503 branch (a loader that fails) does work. Workaround here: the docs loader answers 404 with a page of its own.

## Notes for whoever writes the next one

- Go shared code lives in a normal folder (`app/store`), not in a private one: Go tools skip directories that start with `_` or `.`, so `_store` cannot be imported.
- A section `not-found.tsx` registers `<section>/{path...}`, so a catch-all page in the same section collides with it and the build fails loudly. Pick one.
- A page cannot export `Head` while a layout above it exports `metadata`.
