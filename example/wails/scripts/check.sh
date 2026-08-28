#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
port="${WAILS_TEST_PORT:-18081}"
binary="$(mktemp)"
stdout="$(mktemp)"
stderr="$(mktemp)"

cleanup() {
  kill "${pid:-}" 2>/dev/null || true
  wait "${pid:-}" 2>/dev/null || true
  if [ -n "${vite_pid:-}" ]; then
    kill "$vite_pid" 2>/dev/null || true
    wait "$vite_pid" 2>/dev/null || true
  fi
  rm -f "$binary" "$stdout" "$stderr"
}
trap cleanup EXIT

go build -tags server -o "$binary" .
if [ "${BIFROST_CHECK_DEV:-}" = "1" ]; then
  bun run --cwd frontend dev -- --host 127.0.0.1 --port 9245 --strictPort >"$stdout" 2>"$stderr" &
  vite_pid=$!
  BIFROST_DEV_DIR="$PWD/.bifrost" BIFROST_VITE_PORT=9245 BIFROST_EXTERNAL_VITE=1 WAILS_SERVER_HOST=127.0.0.1 WAILS_SERVER_PORT="$port" "$binary" >>"$stdout" 2>>"$stderr" &
else
  WAILS_SERVER_HOST=127.0.0.1 WAILS_SERVER_PORT="$port" "$binary" >"$stdout" 2>"$stderr" &
fi
pid=$!

for _ in $(seq 1 240); do
  if curl -fsS "http://127.0.0.1:$port/health" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$pid" 2>/dev/null; then
    cat "$stdout"
    cat "$stderr" >&2
    exit 1
  fi
  sleep 0.25
done

curl -fsS "http://127.0.0.1:$port/health" >/dev/null
if [ "${BIFROST_CHECK_DEV:-}" = "1" ]; then
  for _ in $(seq 1 240); do
    if curl -fsS http://127.0.0.1:9245/@vite/client >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  curl -fsS http://127.0.0.1:9245/@vite/client >/dev/null
fi
WAILS_TEST_URL="http://127.0.0.1:$port" bun frontend/browser-check.mjs
