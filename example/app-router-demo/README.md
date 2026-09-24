# App Router demo

An app that exercises every App Router feature on purpose, and a script that asserts each one with `curl`.

```
make build     # go run ../../cmd/bifrost build .
make dev       # development server with HMR
make serve     # build and serve on 127.0.0.1:18700
make check     # build, then run scripts/check.sh (74 assertions)
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

HTTP: pages and `route.go` in the same tree, all methods, redirects from a loader and from `server.go`, a 404 that falls back to the nearest not-found view, a 405, JSON, Markdown by suffix and by `Accept`, and public files.

Client: in-place navigation, loading views, error views with `reset`, request-scoped state under concurrency, `data-bifrost-reload`, downloads, hash-only navigation, and a plain form that works with JavaScript off.

## Notes for whoever writes the next one

- Go shared code lives in a normal folder (`app/store`), not in a private one: Go tools skip directories that start with `_` or `.`, so `_store` cannot be imported.
- A section `not-found.tsx` registers `<section>/{path...}`, so a catch-all page in the same section collides with it and the build fails loudly. Pick one.
- `Head` only lives in `page.tsx`: any other view file that exports it fails the build.
