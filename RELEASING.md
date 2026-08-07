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

## Tag

After all checks pass and the tree is clean:

```bash
git tag -s v1.0.0 -m "bifrost v1.0.0"
git push origin v1.0.0
```

The release workflow reruns `make check` for the tag.
