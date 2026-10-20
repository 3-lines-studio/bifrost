# Bifrost

Bifrost is a file-based React App Router served by Go. Vite builds the client, Bun renders React on the server, and Go owns routing, loaders, and HTTP handlers.

## Requirements

Go 1.25+, Bun, Linux or macOS.

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
| `bifrost dev [dir]` | Development server with HMR on `:8080` (`BIFROST_ADDR` overrides it). |
| `bifrost build [dir]` | Builds `.bifrost/bifrost-app` and prints the route table. |
| `bifrost routes [dir]` | Prints the route table; `--json` is the machine-readable form. |

Run a build with `./.bifrost/bifrost-app` (or `BIFROST_ADDR=:3000 ./.bifrost/bifrost-app`). The binary embeds the frontend.

## Routes

Routes live in `app/`, or in the project root when there is no `app/`. A directory becomes a route when it contains `page.tsx`; its path is the URL path.

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
- `page.go` next to `page.tsx` exports `Load(r *http.Request) (any, error)` and optionally `Cache() string`.
- `page.go` without `page.tsx` is a Go page: it exports typed `Load` and `Page`, and optionally `Layout`, `Error`, `NotFound`, and `Cache`.
- `layout.tsx` exports `Layout`, wraps the routes below it, and stays mounted across navigation.
- `template.tsx` exports `Template`, wraps like a layout, and remounts on navigation.
- `loading.tsx` exports `Loading`, shown while a navigation to that subtree runs.
- `error.tsx` exports `Error({ error, reset })`; `reset` re-fetches the route.
- `not-found.tsx` exports `NotFound`.
- `not-found.go` at the app root exports `NotFound(r *http.Request) g.Node` and handles unmatched URLs for Go pages.
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

## Go pages

A directory with only `page.go` renders a Go page. The file contains the loader, the page, and its components:

```go
package dashboard

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type Props struct {
	Title string
}

func Load(r *http.Request) (Props, error) {
	return Props{Title: r.URL.Query().Get("title")}, nil
}

func Page(props Props) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text(props.Title))),
		h.Body(h.H1(g.Text(props.Title))),
	)
}
```

`Page` returns a `g.Node` with the full document. Bifrost writes the doctype. Components are plain Go functions in the same file and receive typed arguments. Add an optional layout to separate the document shell from the content:

```go
func Layout(content g.Node, props Props) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text(props.Title))),
		h.Body(h.Main(content)),
	)
}
```

With `Layout`, `Page` returns the content node. Without it, `Page` returns the document.

Optional exports:

- `Error(err error, r *http.Request) g.Node` and `NotFound(r *http.Request) g.Node` render loader errors. `bifrost.Status(...)`, `bifrost.NotFound()`, and `bifrost.Redirect(...)` set the status.
- A root `not-found.go` exports `NotFound(r *http.Request) g.Node`; it renders unmatched URLs with 404.
- `Cache() string` sets `Cache-Control`.
- `Load` receives the request, so params, query values, and the pathname come from `r.PathValue`, `r.URL.Query`, and `r.URL.Path`. A catch-all parameter is a string with slash-separated segments.

App Router `middleware.go` wraps Go pages as well. A Go-only app does not need Bun or Vite: `bifrost build` and `bifrost dev` compile and restart the Go app, and dev rebuilds reload the page. The `example/gomponents-app` app exercises typed props, nested components, Alpine, Tailwind, dynamic params, middleware, cache, custom error, and not-found pages.

Go pages and React pages can coexist. A `page.go` next to `page.tsx` is still its loader. Go pages use functions for layouts, errors, and not-found handling instead of separate `.tsx` files; share them by importing a Go package.

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

A page that exports `Head` cannot also export `metadata`.

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
