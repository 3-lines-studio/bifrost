# Bifrost

Server-side rendering for React components in Go. Bridge your Go backend with React frontends.

## Installation

```bash
go get github.com/3-lines-studio/bifrost
```

Requires [Bun](https://bun.sh) to be installed.

## Features

- **SSR** - Server-side render React components
- **Hot Reload** - Auto-refresh in development
- **Props Loading** - Pass data from Go to React
- **File-based Routing** - Simple page organization
- **Production Ready** - Embedded assets for deployment

## Development vs Production

**Development** (`BIFROST_DEV=1`):

- Hot reload on file changes
- Source maps enabled
- Assets served from disk

**Production**:

- Embedded assets using `embed.FS`
- Optimized builds
- Cached renders
