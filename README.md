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
- `layout.go` exports `Layout(content g.Node) g.Node` or `LayoutContext(content g.Node, r *http.Request) g.Node` and wraps Go pages below it.
- `template.tsx` exports `Template`, wraps like a layout, and remounts on navigation.
- `loading.tsx` exports `Loading`, shown while a navigation to that subtree runs.
- `error.tsx` exports `Error({ error, reset })`; `reset` re-fetches the route.
- `not-found.tsx` exports `NotFound`.
- `error.go` exports `Error(err error, r *http.Request) g.Node` for Go pages below it.
- `not-found.go` exports `NotFound(r *http.Request) g.Node` for Go pages below it. The root file also handles unmatched URLs.
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

`Page` returns a `g.Node`. Components are plain Go functions in the same file and receive typed arguments. Bifrost writes the doctype.

A `layout.go` in the same directory or an ancestor wraps the page:

```go
func Layout(content g.Node) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Dashboard"))),
		h.Body(h.Main(content)),
	)
}
```

Inherited layouts apply from the outside in. With a layout, `Page` returns the content node; without one, `Page` returns the document. Use `LayoutContext(content, r)` when the layout needs params, query values, or the pathname. A page can also export `Layout(content, props) g.Node` in `page.go`; that page-specific layout takes precedence and skips inherited `layout.go` files.

Optional exports:

- `Error(err error, r *http.Request) g.Node` and `NotFound(r *http.Request) g.Node` in `page.go` render loader errors. Otherwise Bifrost uses the nearest `error.go` and `not-found.go`, then falls back to the root not-found. `bifrost.Status(...)`, `bifrost.NotFound()`, and `bifrost.Redirect(...)` set the status.
- A root `not-found.go` also renders unmatched URLs with 404.
- `Cache() string` sets `Cache-Control`.
- `Load` receives the request, so params, query values, and the pathname come from `r.PathValue`, `r.URL.Query`, and `r.URL.Path`. A catch-all parameter is a string with slash-separated segments.

App Router `middleware.go` wraps Go pages as well. A Go-only app does not need Bun or Vite: `bifrost build` and `bifrost dev` compile and restart the Go app, and dev rebuilds reload the page. Bifrost load hooks observe Go page `Load` calls; response hooks and middleware wrap the page handler. The `example/gomponents-app` app exercises typed props, nested layouts with request context, nested error and not-found views, Alpine, Tailwind, dynamic params, middleware, cache, and a root not-found page.

Go pages are full HTML. Add Turbo Drive to the layout for client navigation; the example does. Without it, links do normal page loads. The React `?_data` protocol and client router do not apply. Go pages are server-rendered only: there is no static export and no `loading.tsx` equivalent; `Load` runs synchronously before `Page`.

Go pages and React pages can coexist. A `page.go` next to `page.tsx` is still its loader. Shared Go layouts, errors, and not-found views live in `layout.go`, `error.go`, and `not-found.go`; page-specific layout/error functions can stay in `page.go`.

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

After the first render, internal `<a href>` links navigate in place without reloading the document for React pages. Go pages can use the same idea by including Turbo Drive in their layout; the gomponents example does. Layouts stay mounted; page state resets when the pathname changes.

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
