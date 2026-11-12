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
cat >"$tmp/client.ts" <<'EOF'
const socket = process.env.BIFROST_SOCKET!;
const mode = process.argv[2];
const body = mode === "health" ? new Uint8Array(0) : new TextEncoder().encode(process.argv[3]);
const request = new Uint8Array(4 + body.length);
new DataView(request.buffer).setUint32(0, body.length, false);
request.set(body, 4);

let finished = false;
let finish = () => { finished = true; };

const conn = await Bun.connect({
  unix: socket,
  socket: {
    open(ws) { ws.write(request); },
    data(_ws, chunk) { if (chunk[0] === 3) finish(); },
    drain() {},
    close() { finish(); },
    error() { finish(); },
  },
});

if (mode === "abort") {
  await Bun.sleep(50);
  conn.end();
  await Bun.sleep(50);
  process.exit(0);
}

await Bun.sleep(500);
conn.end();
process.exit(finished ? 0 : 1);
EOF

BIFROST_SOCKET="$socket" bun run "$root/internal/renderproc/runtime.ts" >"$tmp/out" 2>"$tmp/err" &
pid=$!
for _ in $(seq 1 300); do
  if BIFROST_SOCKET="$socket" bun "$tmp/client.ts" health >/dev/null 2>&1; then break; fi
  if ! kill -0 "$pid" 2>/dev/null; then cat "$tmp/err" >&2; exit 1; fi
  sleep 0.01
done
BIFROST_SOCKET="$socket" bun "$tmp/client.ts" health >/dev/null

payload=$(printf '{"entry":"%s","props":{}}' "$tmp/entry.mjs")
BIFROST_SOCKET="$socket" bun "$tmp/client.ts" abort "$payload" >/dev/null 2>&1 || true
for _ in $(seq 1 200); do
  if [[ -f "$marker" ]]; then break; fi
  sleep 0.01
done
test -f "$marker"
echo 'renderer cancellation integration passed'
