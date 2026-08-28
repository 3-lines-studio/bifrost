#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cli=$(mktemp)
pid=""
source_file="$root/example/basic/pages/home.tsx"
go_file="$root/example/basic/main.go"
backup=$(mktemp)
go_backup=$(mktemp)
cp "$source_file" "$backup"
cp "$go_file" "$go_backup"
cleanup() {
  if [[ -n "$pid" ]]; then
    kill -TERM -"$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  cp "$backup" "$source_file"
  cp "$go_backup" "$go_file"
  rm -f "$cli" "$backup" "$go_backup"
}
trap cleanup EXIT

cd "$root"
go build -o "$cli" ./cmd/bifrost
"$cli" dev ./example/basic >/tmp/bifrost-dev.out 2>/tmp/bifrost-dev.err &
pid=$!
for _ in $(seq 1 600); do
  if curl -sS http://127.0.0.1:8080/_bifrost/build-id >/dev/null 2>&1; then
    break
  fi
  sleep 0.05
done
bun ./scripts/dev-browser.mjs
bridge_pid_before=$(pgrep -f "entries/vite-dev.ts" | head -1)
test -n "$bridge_pid_before"
before=$(curl -fsS http://127.0.0.1:8080/_bifrost/build-id)
touch "$go_file"
restarted=0
for _ in $(seq 1 1200); do
  after=$(curl -fsS http://127.0.0.1:8080/_bifrost/build-id 2>/dev/null || true)
  if [[ -n "$after" && "$after" != "$before" ]]; then
    restarted=1
    break
  fi
  sleep 0.05
done
test "$restarted" = 1
echo "Go development restart passed"
bridge_pid_after=$(pgrep -f "entries/vite-dev.ts" | head -1)
if [[ "$bridge_pid_before" != "$bridge_pid_after" ]]; then
  echo "Vite bridge was replaced during Go restart" >&2
  exit 1
fi
echo "Vite bridge survived Go restart"
