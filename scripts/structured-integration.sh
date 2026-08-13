#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

go run ./cmd/bifrost build -C ./example/structured ./cmd/server
go build -o /tmp/bifrost-structured ./example/structured/cmd/server
/tmp/bifrost-structured >/tmp/bifrost-structured.out 2>/tmp/bifrost-structured.err &
pid=$!
cleanup() {
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  rm -f /tmp/bifrost-structured
}
trap cleanup EXIT

for _ in $(seq 1 100); do
  if curl -fsS http://127.0.0.1:8082/api/health >/dev/null 2>&1; then break; fi
  sleep 0.05
done

root=$(curl -fsS http://127.0.0.1:8082/)
hello_headers=$(mktemp)
hello=$(curl -fsS -D "$hello_headers" http://127.0.0.1:8082/hello/Don)
health_headers=$(mktemp)
health=$(curl -fsS -D "$health_headers" http://127.0.0.1:8082/api/health)
trap 'rm -f "$hello_headers" "$health_headers"; cleanup' EXIT

grep -q '<h1>Hello.*Home</h1>' <<<"$root"
grep -q '<span data-workspace="resolved"' <<<"$root"
grep -q '<html lang="es" dir="ltr">' <<<"$hello"
grep -q '<h1>Hello.*Don</h1>' <<<"$hello"
grep -qi '^X-Structured-Example: active' "$hello_headers"
grep -qi '^X-Structured-Example: active' "$health_headers"
test "$health" = "ok"
test "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:8082/missing)" = "404"
grep -R -q -- '--font-weight-bold:700' example/structured/cmd/server/.bifrost/dist
grep -R -q 'react.memo_cache_sentinel' example/structured/cmd/server/.bifrost/dist

concurrent_dir=$(mktemp -d)
request_pids=()
for index in $(seq 1 32); do
  curl -fsS "http://127.0.0.1:8082/hello/User$index" >"$concurrent_dir/$index.html" &
  request_pids+=("$!")
done
for request_pid in "${request_pids[@]}"; do
  wait "$request_pid"
done
for index in $(seq 1 32); do
  grep -q '<html lang="es" dir="ltr">' "$concurrent_dir/$index.html"
  grep -q "Hello.*User$index" "$concurrent_dir/$index.html"
done
rm -rf "$concurrent_dir"

echo "structured composition integration passed"
