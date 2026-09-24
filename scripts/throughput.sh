#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
port=${BIFROST_TEST_PORT:-8080}
base=http://127.0.0.1:$port
binary=$(mktemp)
stdout=$(mktemp)
stderr=$(mktemp)
pid=""
cleanup() {
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -f "$binary" "$stdout" "$stderr"
}
trap cleanup EXIT

if curl -fsS "$base/" >/dev/null 2>&1; then
  echo "$base is already serving; stop the stray process before running this script" >&2
  exit 1
fi

cd "$root"
go run ./cmd/bifrost build ./example/basic
go build -o "$binary" ./example/basic
BIFROST_ADDR=127.0.0.1:$port "$binary" >"$stdout" 2>"$stderr" &
pid=$!

ready=0
for _ in $(seq 1 300); do
  if curl -sS "$base/" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.05
done
if [[ "$ready" != 1 ]]; then
  cat "$stderr" >&2
  exit 1
fi

BIFROST_TEST_URL=$base bun ./scripts/throughput.mjs
