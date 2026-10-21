#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
addr=127.0.0.1:18931
base="http://$addr"
pid=""

cleanup() {
  if [[ -n "$pid" ]]; then kill -TERM "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
}
trap cleanup EXIT

cd "$root"
BIFROST_ADDR="$addr" ./.bifrost/bifrost-app >/tmp/bifrost-alpine-ui.log 2>&1 &
pid=$!
for _ in $(seq 50); do
  if curl -sS --max-time 1 -o /dev/null "$base/"; then break; fi
  sleep 0.1
done
node scripts/browser.mjs
