# Deploy

Bifrost deploys as one Go binary plus a Bun process that renders Server pages. The build machine needs Bun, Vite and `node_modules`; the host needs the binary and, unless the app embeds Bun, a `bun` executable.

## What you ship

| App | Binary | Bun on the host |
| --- | --- | --- |
| App Router, default build | 9.4 MB | yes |
| App Router, `--embed-bun` | 118 MB | no |
| Classic router, Server routes, `--embed-bun` | 118 MB | no |
| Classic router, only Static and Client routes | 9 MB | no |

Every App Router page renders on the server, so an App Router deploy always carries the renderer: either a `bun` on the host or the embedded copy. A classic-router app with only Static and Client routes prerenders at build time and never starts Bun.

## Build

```sh
go tool bifrost build .
```

For the App Router this writes `.bifrost/bifrost-app`: the deployable binary, built with `-trimpath -buildvcs=false -ldflags=-s -w`. For the classic router it writes `.bifrost` and the generated `zz_bifrost_gen.go`, and you run `CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o app .` yourself.

`--embed-bun` compiles Bun into the binary and removes the Bun dependency. Keep the default for hosts where `bee host setup` (or any Bun install) is available: the artifact stays 9.4 MB instead of 118 MB.

The build needs Go 1.27 or newer and `bun` plus `node_modules`, because Vite and the Static prerender run there. Run `bun install --frozen-lockfile` first in CI. Those are build-only: the runtime image needs neither.

## Run

```sh
BIFROST_BUN=/usr/local/bin/bun .bifrost/bifrost-app
```

| Variable | Use |
| --- | --- |
| `BIFROST_ADDR` | Listen address, `:8080` by default. |
| `BIFROST_BUN` | `bun` executable for the renderer. Falls back to `PATH`. |
| `BIFROST_IDLE_TIMEOUT` | Exit after this long without connections, for socket-activated stacks. |
| `LISTEN_FDS` / `LISTEN_PID` | systemd socket activation. `bifrost.Serve` serves the inherited socket. |
| `BIFROST_RENDERER_SOCKET` | Attach to a renderer owned by a supervisor instead of starting one. |
| `BIFROST_RUNTIME_SCRIPT` | Publish the extracted renderer script at this path for that supervisor. |

`bifrost.Serve` installs the idle shutdown and the inherited socket, and generated App Router mains use it.

## Health and readiness

`GET /_bifrost/ready` answers without rendering a page: 204 when the renderer is reachable, 503 when it is not. Use it as the health path; a static route exercises less.

## Saturation

A Server page waits up to two seconds for renderer capacity. Past that wait the app answers 503 with `Retry-After: 1`, so a burst degrades instead of failing at the first full queue.

A small page renders in about 45 us and one renderer worker holds about 40 MB of RSS. Two workers served 41k requests per second on a Ryzen 5 9600X with p99 at 130 us idle; the process that renders is CPU-bound, so raise workers with cores and memory, not with traffic alone. `RenderConcurrency` (App Router: `bifrost build --render-concurrency N`) sets the count.

## Renderer runtime cache

The build artifacts extract to `$XDG_CACHE_HOME/bifrost/bifrost-runtime-<build id>` (or the OS cache directory) on first start and are reused afterwards; stale directories are pruned after seven days. The directory must be writable: on a read-only filesystem, point `XDG_CACHE_HOME` at a writable path. With the host Bun the extraction is under 1 MB; with `--embed-bun` it is about 105 MB, once per build id.

## Containers

```dockerfile
FROM oven/bun:1.4.4-slim
COPY .bifrost/bifrost-app /app/app
ENV BIFROST_ADDR=:8080 XDG_CACHE_HOME=/tmp
EXPOSE 8080
ENTRYPOINT ["/app/app"]
```

Pin the image tag to the Bun version the app was built against, and build in an earlier stage where Vite and `node_modules` exist.

## bee

[bee](https://github.com/3-lines-studio/bee) is the deployment target Bifrost is tuned for: a native Go binary on systemd, with no Dockerfile and no registry.

```yaml
name: site
host: vultr-1
runtime: go
main: .
domains:
  - site.example.com
web:
  health_path: /_bifrost/ready
  scale_to_zero: 2m
```

- bee runs `go tool bifrost build`, needs `bun` and `node_modules`, and installs Bun on the host with `bee host setup`.
- The build defaults to the host Bun, so the artifact stays around 9 MB.
- bee sets `BIFROST_ADDR`, `BIFROST_BUN`, `BIFROST_RENDERER_SOCKET` and `BIFROST_RUNTIME_SCRIPT` per slot, and keeps one warm renderer alive while the app sleeps, so a wake skips the Bun boot.
- `web.scale_to_zero` stops the app after the idle window; a systemd socket holds the port and wakes it on the next request.
- `bifrost.render_concurrency` sets the renderer worker count; it defaults to one when `scale_to_zero` is set.

## Limits

- Linux amd64 and arm64. Windows is not supported; the renderer process model needs Unix sockets.
- The renderer is a subprocess. Platforms that cannot spawn one, like edge runtimes, cannot run App Router pages.
- Build on Linux against a libc the host has, or older.
