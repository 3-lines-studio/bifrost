#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
socket="$tmp/renderer.sock"
marker="$tmp/aborted"
pid=""
cleanup() {
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT

cat >"$tmp/entry.mjs" <<EOF
import { writeFileSync } from "node:fs";
export async function render(_props, signal) {
  await new Promise((resolve) => {
    signal.addEventListener("abort", () => {
      writeFileSync(${marker@Q}, "aborted");
      resolve();
    }, { once: true });
  });
  return { head: "", body: new ReadableStream({ start(controller) { controller.close(); } }) };
}
EOF

BIFROST_SOCKET="$socket" bun run "$root/internal/renderproc/runtime.ts" >"$tmp/out" 2>"$tmp/err" &
pid=$!
for _ in $(seq 1 300); do
  if curl -fsS --unix-socket "$socket" http://bifrost/health >/dev/null 2>&1; then break; fi
  if ! kill -0 "$pid" 2>/dev/null; then cat "$tmp/err" >&2; exit 1; fi
  sleep 0.01
done
curl -fsS --unix-socket "$socket" http://bifrost/health >/dev/null
payload=$(printf '{"entry":"%s","props":{}}' "$tmp/entry.mjs")
curl -sS --max-time 0.1 --unix-socket "$socket" -H 'content-type: application/json' -d "$payload" http://bifrost/render >/dev/null 2>&1 || true
for _ in $(seq 1 200); do
  if [[ -f "$marker" ]]; then break; fi
  sleep 0.01
done
test -f "$marker"
echo 'renderer cancellation integration passed'
