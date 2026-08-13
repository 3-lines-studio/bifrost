# Releasing Bifrost

## Required checks

Run from a clean checkout with Go 1.25.0 or newer and Bun (used for frontend installs and the SSR renderer):

```bash
make check
make bench
git status --short
```

`git status --short` must print nothing. CI runs `make check` on Linux and macOS before accepting the tag.

## Tag

After all checks pass and the tree is clean:

```bash
git tag -a v1.1.0 -m "bifrost v1.1.0"
git push origin v1.1.0
```

The release workflow reruns `make check` for the tag.
