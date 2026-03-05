# Bifrost Website

The official Bifrost documentation site, built with Bifrost + Svelte.

## Development

```bash
bun install
make dev
```

Open [http://localhost:3000](http://localhost:3000).

## Production Build

```bash
make build
make start
```

## Writing Content

Documentation pages live in `content/` as markdown files with YAML frontmatter:

```markdown
---
title: Page Title
description: A short description for SEO and the docs header.
order: 1
---

Your markdown content here.
```

### Frontmatter Fields

| Field | Required | Description |
|-------|----------|-------------|
| `title` | Yes | Page title shown in nav and heading |
| `description` | No | Short description for meta tags and subtitle |
| `order` | Yes | Sort order in the sidebar navigation |

### File Naming

The filename (without `.md`) becomes the URL slug:

- `getting-started.md` → `/docs/getting-started`
- `api.md` → `/docs/api`

### Adding a New Page

1. Create a new `.md` file in `content/`
2. Add frontmatter with `title` and `order`
3. Write your content in markdown
4. Rebuild — the page appears automatically in the sidebar nav
