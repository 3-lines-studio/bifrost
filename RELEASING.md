# Releasing Bifrost

## Required checks

Run from a clean checkout with Go 1.26.0 or newer and Bun 1.3.14 (used for JS dependency install and audit):

```bash
make release-check
make bench
git status --short
```

`git status --short` must print nothing. CI runs `make check` on Linux and macOS before accepting the tag.

## Behavior checks

Confirm these contracts before a v1 tag:

- Client-only builds contain no SSR bundles.
- Static-prerender bundles are removed after export.
- SSR apps stop their QuickJS workers and remove temp files on graceful shutdown.
- `go doc -all github.com/3-lines-studio/bifrost` lists every public `App` method.
- Invalid option combinations and static output collisions fail the build.

These cases are covered by `make check`.

## Performance baseline

Measured 2026-08-07 on an AMD Ryzen 5 9600X (12 cores, 32 GB), Go 1.26,
Chromium 150 headless, via `make bench-browser` in `bench/`. Fixture: 2000x20
table (≈4.6 MB SSR HTML), `PropsLoader` with 50 ms latency and ≈600 KB props,
worker count at the `min(GOMAXPROCS, 8)` default. Numbers are a 60-render
soak on one page (`-soak 60`) — long enough for every GC strategy to show its
steady-state pauses; the solo/burst scenarios run too few renders per worker
to trigger render-boundary collections.

| GC config | soak render p50 (ms) | peak RSS |
|---|---|---|
| threshold 16 MiB (previous default) | 469 | ~0.9 GB |
| cadence 25 renders (current default) | 433-461 | 1.6-2.4 GB |

The render-boundary cadence removes mid-render automatic-GC pauses (render
p50 ~5-8% faster on this fixture; p95 within noise). RSS grows roughly
linearly with the interval because garbage accumulates between boundary
collections, and scales with page weight — the fixture is deliberately heavy
(4.6 MB output), so real pages accumulate proportionally less. Worker count
(1-16) was within noise. Memory-bound deployments restore threshold mode
with `BIFROST_QUICKJS_GC_INTERVAL=0`. Re-run with `make bench-browser`
(writes `results/browser-*.json`).

## Tag

After all checks pass and the tree is clean:

```bash
git tag -s v1.0.0 -m "bifrost v1.0.0"
git push origin v1.0.0
```

The release workflow reruns `make check` for the tag.
