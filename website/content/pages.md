---
title: Pages & Routing
description: Define routes and choose rendering modes for each page.
order: 2
---

## Defining Pages

Every page maps a URL pattern to a frontend component:

```go
bifrost.Page("/about", "./pages/about.tsx")
```

Pages are registered when creating the app:

```go
app := bifrost.New(bifrostFS,
    bifrost.Page("/", "./pages/home.tsx", bifrost.WithLoader(homeLoader)),
    bifrost.Page("/about", "./pages/about.tsx", bifrost.WithStatic()),
)
```

## Page Modes

Each page can operate in one of four modes:

### SSR (Server-Side Rendering)

Render the component on every request with fresh data from a loader function:

```go
bifrost.Page("/user/{id}", "./pages/user.tsx",
    bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
        user, err := db.GetUser(req.PathValue("id"))
        if err != nil {
            return nil, err
        }
        return map[string]any{"user": user}, nil
    }),
)
```

The loader receives the full `*http.Request`, so you have access to path parameters, query strings, headers, and cookies.

### Client-Only

Serve an empty HTML shell. The component renders entirely on the client:

```go
bifrost.Page("/admin", "./pages/admin.tsx", bifrost.WithClient())
```

Good for interactive dashboards and pages that don't need SEO.

### Static Prerender

Render full HTML at build time. The page hydrates on the client for interactivity:

```go
bifrost.Page("/about", "./pages/about.tsx", bifrost.WithStatic())
```

Best for marketing pages, landing pages, and content that rarely changes.

### Static with Data

Prerender multiple pages from a data source. Each entry becomes a separate static route:

```go
bifrost.Page("/blog/{slug...}", "./pages/blog.tsx",
    bifrost.WithStaticData(func(ctx context.Context) ([]bifrost.StaticPathData, error) {
        posts := getAllPosts()
        paths := make([]bifrost.StaticPathData, len(posts))
        for i, post := range posts {
            paths[i] = bifrost.StaticPathData{
                Path:  "/blog/" + post.Slug,
                Props: map[string]any{"title": post.Title, "body": post.Body},
            }
        }
        return paths, nil
    }),
)
```

## URL Patterns

Bifrost uses Go's standard `net/http` pattern syntax:

| Pattern | Matches |
|---------|---------|
| `/` | Only the root path |
| `/{$}` | Exact root match |
| `/about` | Exact path |
| `/user/{id}` | Single path parameter |
| `/docs/{slug...}` | Catch-all wildcard |

Access path parameters in loaders with `req.PathValue("id")`.

## Props Flow

Data flows from Go to the frontend component as props:

```go
// Go loader
return map[string]any{
    "title": "Hello",
    "count": 42,
    "items": []string{"a", "b"},
}, nil
```

Props are serialized as JSON. Keep them minimal — pass only what the component needs.
