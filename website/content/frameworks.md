---
title: Framework Support
description: Use React or Svelte with Bifrost for server-side rendering.
order: 3
---

React is the default framework. Svelte is also supported.

## React

React is the default framework. Create an app with `bifrost.New()`:

```go
app := bifrost.New(bifrostFS,
    bifrost.Page("/", "./pages/home.tsx",
        bifrost.WithLoader(func(req *http.Request) (any, error) {
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

## Svelte

Svelte is auto-detected from `.svelte` file extensions. Use the same `bifrost.New()` constructor:

```go
app := bifrost.New(bifrostFS,
    bifrost.Page("/", "./pages/home.svelte",
        bifrost.WithLoader(func(req *http.Request) (any, error) {
            return map[string]any{"message": "Hello from Svelte!"}, nil
        }),
    ),
)
```

Svelte 5 components use `$props()` and `<svelte:head>`:

```svelte
<script lang="ts">
  let { message }: { message: string } = $props();
</script>

<svelte:head>
  <title>Bifrost + Svelte</title>
</svelte:head>

<h1>{message}</h1>
```

**Scoped styles** are fully supported. Svelte automatically adds unique `svelte-*` class hashes to elements and CSS selectors. Bifrost's critical CSS extraction correctly identifies which scoped rules apply to each page, keeping only the CSS needed for the rendered HTML.

## Features

All Bifrost features work with both React and Svelte:

- `WithLoader()` — SSR with data loading
- `WithClient()` — Client-only rendering
- `WithStatic()` — Static prerendering
- `WithStaticData()` — Static with dynamic paths
- Error handling and redirects
- Asset embedding and production builds
