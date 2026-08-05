# Releasing Bifrost

## Required checks

Run from a clean checkout with Go 1.26.0 or newer and Bun 1.3.14:

```bash
make release-check
make bench
git status --short
```

`git status --short` must print nothing. CI runs `make check` on Linux and macOS before accepting the tag.

## Behavior checks

Confirm these contracts before a v1 tag:

- Client-only builds contain no Bun runtime or SSR bundles.
- Static-prerender bundles are removed after export.
- SSR apps stop their Bun child and remove temp files on graceful shutdown.
- The Bun child also exits and cleans temp files if its Go parent exits abruptly.
- `go doc -all github.com/3-lines-studio/bifrost` lists every public `App` method.
- Invalid option combinations and static output collisions fail the build.

These cases are covered by `make check`. The parent-exit test requires Bun and runs in `internal/adapters/process`.

## Local v1 baseline

Measured on Linux/amd64 with an AMD Ryzen 5 9600X and Bun 1.3.14. These values are reference points, not API guarantees.

- Renderer IPC with a prebuilt render function: 31–33 µs p50, 57–64 µs p95.
- Renderer startup: about 21 ms.
- Example React SSR load, 2,000 requests at concurrency 20: 7,458 requests/s, 2.31 ms p50, 5.01 ms p95, no failures.
- RSS after that load: about 124 MiB for Go and 152 MiB for the embedded Bun process.
- Mixed SSR example binary: 113 MiB; cold readiness: 144 ms.
- Client-only example binary: 15 MiB and no `.bifrost/runtime`, `.bifrost/ssr`, or `.bifrost/entries` directory.

Re-run the load and size checks when Bun, React, the Go toolchain, or runtime packaging changes.

## Tag

After all checks pass and the tree is clean:

```bash
git tag -s v1.0.0 -m "bifrost v1.0.0"
git push origin v1.0.0
```

The release workflow reruns `make check` for the tag.
