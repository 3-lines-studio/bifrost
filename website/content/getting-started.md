---
title: Getting Started
description: Install Bifrost and create your first project in minutes.
order: 1
---

## Installation

```bash
go get github.com/3-lines-studio/bifrost
```

[Bun](https://bun.sh) is required for development and building. Production binaries with SSR pages include the Bun runtime automatically — no system-level Bun installation needed on the target machine.

Static-only apps (no SSR) do not include the Bun runtime at all.

## Create a Project

The fastest way to get started is with the init command:

```bash
go run github.com/3-lines-studio/bifrost/cmd/init@latest myapp
```

This scaffolds a complete project and starts the dev server on `http://localhost:8080`.

### Templates

Choose a template that fits your use case:

- **minimal** (default) — React SSR app with a single page
- **spa** — Single-page application with client-only rendering
- **desktop** — Desktop application template

## Project Structure

A Bifrost project follows a simple layout:

```
myapp/
├── main.go           # Go server with route definitions
├── pages/            # Page components (.tsx)
│   ├── home.tsx
│   └── about.tsx
├── components/       # Shared components
├── public/           # Static assets (images, fonts)
├── style.css         # Global styles
├── .bifrost/         # Build output (gitignored)
└── go.mod
```

## Development Mode

Set `BIFROST_DEV=1` to enable development mode with hot reload:

```bash
BIFROST_DEV=1 go run main.go
```

In dev mode, source files are rendered directly and the browser refreshes on every change.

## Build for Production

```bash
# Build frontend assets and SSR bundles
go run github.com/3-lines-studio/bifrost/cmd/build@latest main.go

# Build the Go binary with embedded assets
go build -o myapp main.go

# Run in production
./myapp
```

The production binary is self-contained. All assets are embedded via `embed.FS`.
