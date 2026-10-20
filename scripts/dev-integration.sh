#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
port=${BIFROST_TEST_PORT:-8080}
base=http://127.0.0.1:$port
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
  if [[ -n "${demo_pid:-}" ]]; then
    kill -TERM -"$demo_pid" 2>/dev/null || kill "$demo_pid" 2>/dev/null || true
    wait "$demo_pid" 2>/dev/null || true
  fi
  cp "$backup" "$source_file"
  cp "$go_backup" "$go_file"
  rm -rf "$root/example/app-router-demo/app/tempmedicion"
  rm -f "$cli" "$backup" "$go_backup"
}
trap cleanup EXIT

cd "$root"
go build -o "$cli" ./cmd/bifrost
BIFROST_ADDR=127.0.0.1:$port "$cli" dev ./example/basic >/tmp/bifrost-dev.out 2>/tmp/bifrost-dev.err &
pid=$!
for _ in $(seq 1 600); do
  if curl -sS $base/_bifrost/build-id >/dev/null 2>&1; then
    break
  fi
  sleep 0.05
done
if "$cli" dev ./example/basic >/tmp/bifrost-dev-second.out 2>/tmp/bifrost-dev-second.err; then
  exit 1
fi
grep -q "development server is already running" /tmp/bifrost-dev-second.err
BIFROST_TEST_URL=$base bun ./scripts/dev-browser.mjs
bridge_pid_before=$(pgrep -f "entries/vite-dev.ts" | head -1)
test -n "$bridge_pid_before"
before=$(curl -fsS $base/_bifrost/build-id)
touch "$go_file"
restarted=0
for _ in $(seq 1 1200); do
  after=$(curl -fsS $base/_bifrost/build-id 2>/dev/null || true)
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

wait_for_answer() {
  for _ in $(seq 1 900); do
    if curl -fsS "$1" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.05
  done
  echo "no answer from $1" >&2
  exit 1
}

await_rebuild() {
  local now
  for _ in $(seq 1 1200); do
    now=$(curl -fsS "$1/_bifrost/build-id" 2>/dev/null || true)
    if [[ -n "$now" && "$now" != "$2" ]]; then
      return 0
    fi
    sleep 0.05
  done
  echo "no rebuild of $1" >&2
  exit 1
}

answered() {
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' "$2")
  if [[ "$code" != "$1" ]]; then
    echo "$3 answered $code, want $1: $2" >&2
    exit 1
  fi
}

demo="$root/example/app-router-demo"
demo_route="$demo/app/tempmedicion"
demo_port=$((port + 2))
demo_base=http://127.0.0.1:$demo_port
rm -rf "$demo_route"
BIFROST_ADDR=127.0.0.1:$demo_port "$cli" dev ./example/app-router-demo >/tmp/bifrost-dev-demo.out 2>/tmp/bifrost-dev-demo.err &
demo_pid=$!
wait_for_answer "$demo_base/_bifrost/build-id"
before=$(curl -fsS "$demo_base/_bifrost/build-id")

mkdir -p "$demo_route"
printf 'export default function Page() { return <p>bifrost</p>; }\n' > "$demo_route/page.tsx"
await_rebuild "$demo_base" "$before"
answered 200 "$demo_base/tempmedicion" "a page that arrived"

before=$(curl -fsS "$demo_base/_bifrost/build-id")
rm -rf "$demo_route"
await_rebuild "$demo_base" "$before"
answered 404 "$demo_base/tempmedicion" "a page that left"
answered 200 "$demo_base/" "a page whose view moved"
echo "A route that arrives and one that leaves both landed"
