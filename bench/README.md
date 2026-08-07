# Bifrost browser test bench

A real-browser performance bench for Bifrost SSR tuning. It drives Chromium
(via go-rod, headless by default) against a heavy fixture page and measures
Navigation Timing + LCP, plus the server-side QuickJS render time reported in
the `X-Bifrost-Render-Ms` response header.

## Fixture

`bench/main.go` serves `/heavy`:

- 2000 x 20 table (≈4.6 MB SSR HTML) rendered through QuickJS
- `PropsLoader` with 50 ms simulated data-fetch latency and a ≈600 KB props
  payload (exercises the loader path and the props `JSON.parse` fast path)
- tunable per run: `-rows`, `-cols`, `-latency`

The bench page weights are query-param overridable (`/heavy?rows=500&latency=0`)
for manual exploration.

## Scenarios

| scenario | what it measures |
|---|---|
| `solo` | one cold-cache navigation, no throttle: TTFB + render time |
| `burst` | 8 concurrent cold-cache navigations: worker-pool saturation (TTFB/render p95) |
| `throttled` | solo with CDP `Network.emulateNetworkConditions` Fast-4G (150 ms RTT, 9.3 Mbps): full browser pipeline TTFB/load/LCP |
| `soak` | `-soak N` replaces the scenarios with N sequential renders on one page — long enough to trigger render-boundary GCs, so GC strategies can be compared at steady state |

`make bench-sweep` also runs a short soak (30 renders) per config so the table
shows GC steady-state render times next to the saturation numbers; override
with `-sweep-soak N` (`0` = off).

The solo/burst scenarios run too few renders per worker to trigger the
default render-boundary GC, so GC comparisons must use `-soak` (or the
sweep's built-in soak).

## Headed mode

Add `-headed` to run Chromium with a real window (needs a display) instead of
headless. Solo/throttled LCP numbers come from actual paint either way; burst
LCP is unreliable in both modes because background tabs are not painted —
use burst for server saturation (TTFB/render), not LCP.

Metrics per navigation: `ttfb_ms`, `dcl_ms`, `load_ms`, `lcp_ms`
(largest-contentful-paint via buffered observer), `render_ms`, transfer and
document sizes. Aggregated as p50/p95 per scenario; every sample is dumped to
`../results/browser-<timestamp>.json`.

## Run

```bash
cd bench && bun i        # once
make bench-browser       # baseline, one config
make bench-sweep         # one-at-a-time sweep of the env knobs
```

The harness builds the bifrost CLI, the bench assets, and the server binary on
first use (see `ensureBuilt` in `bench/harness/main.go`).

`make bench-sweep` sweeps, one knob at a time around the defaults:

- `BIFROST_QUICKJS_WORKERS` ∈ {1, 2, 4, 6, 8, 12, 16}
- `BIFROST_QUICKJS_GC_THRESHOLD` ∈ {4, 8, 16, 32, 64} MiB
- `BIFROST_QUICKJS_GC_INTERVAL` ∈ {0, 1, 5, 10, 25, 50} renders (`0` = threshold mode)

Each config restarts the server subprocess with the new env; the throttled
spot-check runs only for the baseline and the winning config to keep runtime
bounded. The recommended config minimizes burst p95 TTFB (the
saturation-sensitive metric), tie-broken by solo p95.

## Notes

- Requires a Chromium binary: default `/usr/bin/chromium`, override with
  `-browser` or `BIFROST_BENCH_BROWSER`.
- The bench is a tuning tool, not a correctness gate: it is not part of
  `make check`.
