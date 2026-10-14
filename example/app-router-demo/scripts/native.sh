#!/usr/bin/env bash
# Generates the native project for this app and bundles it.
#
# The unit tests compare the text the generator writes. This is the other half:
# the generated project is a real Expo app that has to resolve, transform and
# bundle, and nothing else in the repository ever builds one.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
native="$root/.bifrost/native"

step() { printf '  \033[32m>\033[0m %s\n' "$1"; }

step "bifrost native"
(cd "$root" && go run ../../cmd/bifrost native .)

step "bun install"
(cd "$native" && bun install)

# A bundle is the check: every route the generator wrote is compiled, so a
# screen that does not compile fails here, and so does a resolver that cannot
# find what the generated code imports.
step "expo export --platform android"
rm -rf "$native/export"
(cd "$native" && bunx expo export --platform android --output-dir "$native/export")

bundle=$(find "$native/export" -name '*.hbc' -o -name '*.js' | head -1)
if [[ -z "$bundle" ]]; then
  printf '  \033[31mFAIL\033[0m no bundle was written\n'
  exit 1
fi

# A bundle that is a few hundred bytes is a bundle that resolved nothing.
size=$(wc -c < "$bundle")
if (( size < 100000 )); then
  printf '  \033[31mFAIL\033[0m the bundle is %s bytes, which is not an app\n' "$size"
  exit 1
fi

printf '  \033[32mok\033[0m   bundled %s (%s bytes)\n' "${bundle#"$native"/}" "$size"
