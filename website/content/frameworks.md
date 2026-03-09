---
title: Framework Support
description: Use React with Bifrost for server-side rendering.
order: 3
---

## React

React is the default and only supported framework. Create an app with `bifrost.New()`:

```go
app := bifrost.New(bifrostFS,
    bifrost.Page("/", "./pages/home.tsx",
        bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
            return map[string]any{"message": "Hello from React!"}, nil
        }),
    ),
)
```

React components use `.tsx` files and standard React conventions:

```tsx
export default function Home({ message }: { message: string }) {
    return <h1>{message}</h1>;
}
```

## Features

All Bifrost features work with React:

- `WithLoader()` — SSR with data loading
- `WithClient()` — Client-only rendering
- `WithStatic()` — Static prerendering
- `WithStaticData()` — Static with dynamic paths
- Error handling and redirects
- Asset embedding and production builds
