# Releasing Bifrost

## Required checks

Run from a clean checkout with Go 1.25.0 or newer and Bun (used for frontend installs and the SSR renderer):

```bash
make check
make integration
make dev-integration
make reproducible
make bench
make fuzz
git status --short
```

`git status --short` must print nothing. Nothing above runs on CI; `make integration` and `make dev-integration` need Chromium.

## Tag

Move the entries in `CHANGELOG.md` from the unreleased section under the new version, commit that, and after all checks pass with a clean tree:

```bash
git tag -a v1.1.0 -m "bifrost v1.1.0"
git push origin v1.1.0
```

The release workflow only waits for the tag to appear on proxy.golang.org.
