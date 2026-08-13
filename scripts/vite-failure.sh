#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
config="$root/vite.config.ts"
backup=$(mktemp)
cp "$config" "$backup"
cleanup() {
  cp "$backup" "$config"
  rm -f "$backup"
}
trap cleanup EXIT

before=$(sha256sum "$root/example/basic/.bifrost/manifest.json" | cut -d' ' -f1)
printf '\nthis is not valid TypeScript !!!\n' >> "$config"
if (cd "$root" && go run ./cmd/bifrost build ./example/basic >/tmp/bifrost-vite-failure.out 2>/tmp/bifrost-vite-failure.err); then
  echo "Vite configuration failure unexpectedly succeeded" >&2
  exit 1
fi
after=$(sha256sum "$root/example/basic/.bifrost/manifest.json" | cut -d' ' -f1)
test "$before" = "$after"
echo "Vite failure preserved the last good build"
