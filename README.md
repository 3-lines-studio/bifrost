# Bifrost

Bifrost is a file-based App Router served by Go. Pages are React components or Go `html/template` files. Go owns routing, loaders, and HTTP handlers; Vite builds client scripts and Bun renders React pages on the server.

## Requirements

Go 1.25+, Linux or macOS. React pages and pages with a client script also need Bun and Vite.

## Create and run

```sh
go install github.com/3-lines-studio/bifrost/cmd/bifrost@latest
bifrost init myapp
cd myapp
bifrost dev .
```

`init` writes an App Router project and an `AGENTS.md`, then runs `go mod tidy` and `bun install`. `--no-install` skips those installs.

| Command | Does |
| --- | --- |
| `bifrost dev [dir]` | Development server on `:8080` (`BIFROST_ADDR` overrides it); HMR for React pages and page scripts, rebuild and restart for templates. |
| `bifrost build [dir]` | Builds `.bifrost/bifrost-app` and prints the route table. |
| `bifrost routes [dir]` | Prints the route table; `--json` is the machine-readable form. |

Run a build with `./.bifrost/bifrost-app` (or `BIFROST_ADDR=:3000 ./.bifrost/bifrost-app`). The binary embeds the frontend.

## Routes

Routes live in `app/`, or in the project root when there is no `app/`. A directory becomes a route when it contains `page.tsx` or `page.html`; its path is the URL path.

| Path | URL |
| --- | --- |
| `app/page.tsx` | `/` |
| `app/posts/page.tsx` | `/posts` |
| `app/posts/post-id_/page.tsx` | `/posts/{post-id}` |
| `app/docs/slug__/page.tsx` | `/docs/{slug...}` |
| `app/marketing~/about/page.tsx` | `/about` |
| `app/_components/Card.tsx` | never routed |

One trailing `_` is a one-segment parameter. Two are a catch-all, which must be the last segment. A trailing `~` groups files without changing the URL, and a leading `_` keeps a directory out of routing. Bracketed names such as `[slug]` fail the build.

## Files

- `page.tsx` exports `Page` (or a default export) and optionally `Head`.
- `page.html` is a Go template rendered by `html/template`; no React or hydration.
- `page.js` or `page.ts` is the optional client entry for `page.html`; Vite builds it.
- `page.go` exports `Load(r *http.Request) (any, error)` and optionally `Cache() string`.
- `layout.tsx` exports `Layout`, wraps the routes below it, and stays mounted across navigation.
- `layout.html` wraps `page.html` routes and receives `.Content`.
- `template.tsx` exports `Template`, wraps like a layout, and remounts on navigation.
- `loading.tsx` exports `Loading`, shown while a navigation to that subtree runs.
- `error.tsx` exports `Error({ error, reset })`; `reset` re-fetches the route. `error.html` is its Go template equivalent.
- `not-found.tsx` exports `NotFound`; `not-found.html` and `not-found.js`/`not-found.ts` are its Go template and client entry.
- `route.go` exports any of `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options` with `http.ResponseWriter` and `*http.Request`. Missing methods answer 405.
- `middleware.go` exports `Middleware(next http.Handler) http.Handler` and wraps the subtree.
- `server.go` exports `Serve(ctx context.Context, handler http.Handler) error`; it is only valid at the route root.
- `public/` is served as-is.

## Page and loader

```tsx
export default function Page({ params, title }: { params: { slug: string }; title: string }) {
  return <article><h1>{title}</h1><p>{params.slug}</p></article>;
}
```

```go
package slug

import "net/http"

func Load(r *http.Request) (any, error) {
	return map[string]any{"title": "Post " + r.PathValue("slug")}, nil
}
```

Pages receive `params`, `searchParams`, and `pathname` merged into the loader props. `params` keys by folder name, a catch-all parameter is an array of segments, and a repeated query key is an array. A loader must return a map or a `bifrost.PageData` whose `Props` is a map. Return `bifrost.NotFound()` for a 404; a loader redirect keeps its `Location`.

A loader may also return `bifrost.PageData{Props: props, Document: bifrost.Document{Lang: "pt-BR", Class: "dark", Dir: "ltr"}}` to set the root document attributes.

## Go templates

A directory with `page.html` is rendered by Go:

```html
{{define "head"}}<title>{{.Props.title}}</title>{{end}}
<h1>{{.Props.title}}</h1>
```

```go
func Load(r *http.Request) (any, error) {
	return map[string]any{"title": "Post " + r.PathValue("slug")}, nil
}
```

Templates receive `.Props`, `.Params`, `.SearchParams`, and `.Pathname`. `layout.html` wraps the page and receives `.Content`; `{{define "head"}}` blocks are collected from the outside in. Loaders, middleware, caching, `?_data`, and Markdown work the same as for React pages.

`error.html` is inherited like `layout.html`; the nearest one renders loader errors inside the route layouts with `__bifrostError` and `__bifrostErrorLevel` in props. `not-found.html` renders unmatched URLs and a loader `bifrost.NotFound()`, with `pathname` in props. A sibling `not-found.js` or `not-found.ts` is the not-found client entry.

A sibling `page.js` or `page.ts` becomes the client entry. Vite builds it, so imports, TypeScript, and CSS imports work:

```js
import "./page.css";

const props = JSON.parse(document.getElementById("__BIFROST_PROPS__").textContent);
document.querySelector("h1").textContent = props.title;
```

The shell injects the props JSON and the built module; do not add your own entry script to `page.html`.

A build with only `page.html` routes and no `page.js` skips Bun and Vite. `bifrost dev` works for pure HTML apps too: it rebuilds and restarts the Go app on `.html`, Go, and `public/` changes, without Vite.

### Turbo and Alpine

Turbo Drive adds client navigation, history, scroll and a progress bar to a template app without a client entry:

```html
<script defer src="https://cdn.jsdelivr.net/npm/@hotwired/turbo@8/dist/turbo.es2017-umd.js"></script>
<script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3/dist/cdn.min.js"></script>
```

Keep global setup in the layout head and guard it so it runs once; use `turbo:load` for work that must run after each visit. Register `Alpine.data` on `alpine:init` before Alpine starts. `example/vanilla` is a complete app with Turbo, Alpine, a loader error, not-found, cache, Markdown, an API route and a `page.ts` island.

## Route handlers

```go
package search

import "net/http"

func Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"hits":[]}`))
}
```

Pages and handlers share the same tree and middleware.

## Metadata

```tsx
export const metadata = { title: "Posts", description: "All posts" };
```

Layouts and pages merge `metadata` from the outside in, so a page overrides one key and inherits the rest. `generateMetadata(props)` may be async and receives the page props. Supported keys include `title`, `description`, `keywords`, `alternates.canonical`, `robots.index`, `robots.follow`, and `openGraph`.

`Head` is optional and only valid in `page.tsx`:

```tsx
export function Head() {
  return <title>Posts</title>;
}
```

A page that exports `Head` cannot also export `metadata`. Go templates use `{{define "head"}}` instead; there is no metadata merge between a `layout.html` and its pages.

## Navigation

After the first render, internal `<a href>` links navigate in place without reloading the document. Layouts stay mounted; page state resets when the pathname changes.

```tsx
import { Link, navigate, refresh, useParams, usePathname, useSearchParams, useRouter } from "virtual:bifrost/navigation";

export function Actions() {
  const router = useRouter();
  const tab = useSearchParams().get("tab");
  const pathname = usePathname();
  const params = useParams();
  return (
    <nav>
      <Link href="/posts">Posts</Link>
      <button onClick={() => navigate("/posts")}>Go</button>
      <button onClick={() => router.back()}>Back</button>
      <button onClick={async () => { await refresh(); }}>Refresh</button>
    </nav>
  );
}
```

`navigate`, `replace`, and `refresh` return `Promise<void>` and are called from events or effects. They cannot be called during rendering. `useRouter()` exposes `push`, `replace`, `refresh`, `back`, and `forward`.

`bifrost init` writes the `virtual:bifrost/navigation` types to `bifrost.d.ts`.

## Styles and assets

Import a stylesheet from a layout:

```tsx
import type { ReactNode } from "react";
import "./style.css";

export function Layout({ children }: { children: ReactNode }) {
  return <main>{children}</main>;
}
```

`bifrost init` sets up Tailwind in `app/style.css`; Vite configuration lives in `vite.config.ts`.

Go templates reference public files through `asset`, which returns a content-hashed URL:

```html
<link rel="stylesheet" href="{{asset "/styles.css"}}">
```

Hashed URLs are served with `Cache-Control: public, max-age=31536000, immutable`; the original `/styles.css` still works with a one-hour cache.
