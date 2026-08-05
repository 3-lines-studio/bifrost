# Sobek optimization benchmark

Reference machine: AMD Ryzen 5 9600X, 6 cores / 12 threads, Linux amd64, Go 1.26.

Run benchmarks sequentially. Concurrent benchmark processes distort Sobek and GC results.

```bash
make bench-sobek
make profile-sobek
```

Use `benchstat` for controlled before/after comparisons:

```bash
go test ./internal/adapters/process -run '^$' \
  -bench '^BenchmarkRuntimeRealPageSerial/Sobek$' \
  -benchmem -benchtime=3s -count=7 > before.txt

# Apply one change and rebuild the Sobek example.
make -C example build-sobek

go test ./internal/adapters/process -run '^$' \
  -bench '^BenchmarkRuntimeRealPageSerial/Sobek$' \
  -benchmem -benchtime=3s -count=7 > after.txt

benchstat before.txt after.txt
```

## Workload

The real-page fixture is `example/pages/home.tsx`. It exercises React 19, hooks, context, `useId`, shared components, class variance utilities, Tailwind class merging, Unicode, escaping, and a large HTML tree.

Every experimental benchmark checks output parity before starting its timer. A result with `exact-parity=0` is not eligible for production.

## Results

### Kept

| Change | Main result | Tradeoff |
|---|---|---|
| React legacy string renderer only | SSR bundle 242 KB → 143 KB; startup + first render 47.2 ms → 32.0 ms | Warm latency unchanged |
| Build-time Sobek IIFE | startup + first render about 32.0 ms → 17.5 ms; preparation allocations roughly halved | Keeps ESM fallback for old bundles |
| Lazy production page registry | Four-page worker warmup 18.5 ms → 11.6–12.5 ms; allocation 16.3 MB → 9.0 MB; bundle 495 KB → 148 KB | A single-page first load is slightly larger; lazy loaders preserve import-failure isolation |
| Go PGO | Four-worker HTTP throughput improved about 3–5%; warm microbenchmark improved about 5% | Profile must be refreshed after major Sobek or renderer changes |
| Four default workers | Best throughput per MB on the reference machine | Eight workers maximize throughput when memory is less important |

### Rejected

| Experiment | Result | Reason rejected |
|---|---|---|
| `context.AfterFunc` cancellation | Slightly slower; three more allocations/render | Existing watcher is better |
| Go `strings.Builder` output host | About 2–3% slower | Saved 68 KB and 2.3k allocations, but host calls cost more |
| Direct Go props | Less than 1% faster | Weakens JSON boundary semantics for negligible gain |
| Preact compatibility | About 8% faster, but 50% more allocated bytes | React output parity failed |
| Replace `tailwind-merge` with join | About 1–2% faster | Package-specific and not generally correct |
| More than eight workers | Throughput nearly flat | Higher RSS and worse tail latency |

## Pool load result

64 HTTP clients, six-second runs, real home page:

| Workers | Requests/s | p50 | p95 | RSS |
|---:|---:|---:|---:|---:|
| 1 | 307 | 207 ms | 212 ms | 43 MB |
| 2 | 539 | 118 ms | 122 ms | 49 MB |
| 4 | 804 | 79 ms | 83 ms | 65 MB |
| 6 | 936 | 68 ms | 72 ms | 78 MB |
| 8 | 1,012 | 63 ms | 67 ms | 100 MB |
| 12 | 1,018 | 62 ms | 73 ms | 157 MB |

Four workers give the best throughput per MB. Eight workers are the current throughput-oriented setting.

## PGO

`bifrost build --go-build` uses the embedded Sobek PGO profile for Sobek builds. Configure it with:

```bash
BIFROST_SOBEK_PGO=off   # disable PGO
BIFROST_SOBEK_PGO=/path/to/profile.pprof  # use a replacement profile
```

Regenerate the reference profile after a major runtime change:

```bash
make profile-sobek
cp /tmp/bifrost-sobek-cpu.pprof cmd/bifrost/sobek-default.pgo
```
