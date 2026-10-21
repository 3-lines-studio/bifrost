#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
addr=127.0.0.1:18910
base="http://$addr"
pid=""
passed=0
failed=0

cleanup() {
  if [[ -n "$pid" ]]; then kill -TERM "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
}
trap cleanup EXIT

ok() { passed=$((passed + 1)); printf '  \033[32mok\033[0m   %s\n' "$1"; }
bad() { failed=$((failed + 1)); printf '  \033[31mFAIL\033[0m %s\n' "$1"; if [[ -n "${2:-}" ]]; then printf '       %s\n' "$2"; fi; }

expect_status() {
  local name=$1 want=$2 url=$3
  local got
  got=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$base$url" || echo 000)
  if [[ "$got" == "$want" ]]; then ok "$name ($got)"; else bad "$name" "wanted $want, got $got"; fi
}

expect_body() {
  local name=$1 pattern=$2 url=$3
  local body
  body=$(curl -sS --max-time 10 "$base$url" || true)
  if grep -qF -- "$pattern" <<<"$body"; then ok "$name"; else bad "$name" "no match for [$pattern]"; fi
}

expect_header() {
  local name=$1 header=$2 want=$3 url=$4
  local got
  got=$(curl -sS --max-time 10 -D - -o /dev/null "$base$url" | tr -d '\r' | grep -i "^$header:" | head -1 || true)
  if grep -qiF -- "$want" <<<"$got"; then ok "$name"; else bad "$name" "wanted [$want], got [$got]"; fi
}

cd "$root"
BIFROST_ADDR="$addr" ./.bifrost/bifrost-app >/tmp/bifrost-gomponents-app.log 2>&1 &
pid=$!
for _ in $(seq 50); do
  if curl -sS --max-time 1 -o /dev/null "$base/"; then break; fi
  sleep 0.1
done

echo "go pages"
expect_status "home" 200 "/"
expect_body "typed page" "Gomponents + Alpine + vanilla CSS" "/"
expect_body "root layout" "<title>Gomponents App Router</title>" "/"
expect_body "turbo navigation" "turbo.es2017-umd.js" "/"
expect_body "vanilla css link" "/styles.css" "/"
expect_status "vanilla css" 200 "/styles.css"
expect_body "parent passes props" "Parent passes variables" "/"
expect_header "middleware" "X-Go-Page" "1" "/"
expect_header "vary" "Vary" "Accept" "/"
expect_header "document cache" "Cache-Control" "private, no-cache, no-store" "/"
expect_header "cache" "Cache-Control" "public, max-age=60" "/error-demo"
expect_status "dynamic param" 200 "/hello/world"
expect_body "dynamic param body" "Hello, world" "/hello/world"
expect_body "nested layout" "Nested layout" "/hello/world"
expect_body "layout context" "Nested layout · world" "/hello/world"
data=$(curl -sS --max-time 10 -H "Accept: application/vnd.bifrost.data+json" "$base/hello/world")
if grep -qF '"Name":"world"' <<<"$data"; then
  ok "data response"
else
  bad "data response" "no props in $data"
fi
data_headers=$(curl -sS --max-time 10 -D - -o /dev/null -H "Accept: application/vnd.bifrost.data+json" "$base/hello/world" | tr -d '\r')
if grep -qi '^Cache-Control: no-store' <<<"$data_headers" && grep -qi '^Vary: Accept' <<<"$data_headers"; then
  ok "data cache and vary"
else
  bad "data cache and vary" "$data_headers"
fi
expect_status "nested error" 503 "/hello/world?state=error"
expect_body "nested error body" "Hello error" "/hello/world?state=error"
expect_status "nested not found" 404 "/hello/world?state=missing"
expect_body "nested not found body" "Hello not found" "/hello/world?state=missing"
expect_status "custom error" 503 "/error-demo?state=error"
expect_body "custom error body" "Custom error" "/error-demo?state=error"
expect_status "custom not found" 404 "/error-demo?state=missing"
expect_body "custom not found body" "Custom not found" "/error-demo?state=missing"
expect_status "root not found" 404 "/nope"
expect_body "root not found body" "Not found" "/nope"
expect_status "section not found" 404 "/hello/nope/extra"
expect_body "section not found body" "Hello not found" "/hello/nope/extra"

echo
if [[ "$failed" -eq 0 ]]; then
  printf '%d passed, 0 failed\n' "$passed"
else
  printf '%d passed, %d failed\n' "$passed" "$failed"
  exit 1
fi
