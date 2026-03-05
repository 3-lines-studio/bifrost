---
title: Framework Support
description: Use React or Svelte with the same Go API.
order: 3
---

## React

React is the default framework. Create an app with `bifrost.New()`:

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

## Svelte

Use `bifrost.NewWithFramework()` with `bifrost.Svelte`:

```go
app := bifrost.NewWithFramework(bifrostFS, bifrost.Svelte,
    bifrost.Page("/", "./pages/home.svelte",
        bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
            return map[string]any{"message": "Hello from Svelte!"}, nil
        }),
    ),
)
```

Svelte components use `.svelte` files with Svelte 5 runes:

```svelte
<script lang="ts">
  let { message }: { message: string } = $props();
</script>

<h1>{message}</h1>
```

## Identical APIs

Every feature works the same across both frameworks:

- `WithLoader()` — SSR with data loading
- `WithClient()` — Client-only rendering
- `WithStatic()` — Static prerendering
- `WithStaticData()` — Static with dynamic paths
- Error handling and redirects
- Asset embedding and production builds

The only difference is the constructor (`New` vs `NewWithFramework`) and the component file format.

## Choosing a Framework

Both frameworks are fully supported. Choose based on your team's preference:

- **React** — Larger ecosystem, more third-party components
- **Svelte** — Smaller bundle sizes, less boilerplate, built-in reactivity

You can't mix frameworks within a single app instance. Each app is either React or Svelte.
