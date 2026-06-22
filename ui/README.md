# bifrost-ui

Svelte 5 component library for the Bifrost SSR framework. Built on COSS UI design tokens with Tailwind CSS v4.

## Installation

```bash
npm install bifrost-ui
```

## Usage

Import the global CSS in your app:

```css
@import "bifrost-ui/styles/globals.css";
```

Import components:

```svelte
<script lang="ts">
  import { Button, Card, Badge } from "bifrost-ui";
</script>

<Button variant="default">Click me</Button>
```

## Requirements

- Svelte 5
- Tailwind CSS v4
