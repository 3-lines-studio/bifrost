# Bifrost Example

This is an example application demonstrating Bifrost's SSR capabilities.

## Prerequisites

- Go 1.21+
- Bun
- Air (`go install github.com/cosmtrek/air@latest`)

## Development

Run the development server with hot reload:

```bash
make dev
```

Then open http://localhost:8080

**Note:** The `.bifrost` directory is created automatically for the embed directive to work.

## Production Build

```bash
make build
./bifrost
```

## Available Routes

- `/` - Home page with SSR
- `/about` - About page
- `/nested` - Nested page example
- `/message/{message}` - Dynamic route example
- `/error` - Error handling demo
