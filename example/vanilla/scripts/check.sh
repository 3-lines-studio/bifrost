#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
addr=127.0.0.1:18702
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
BIFROST_ADDR="$addr" ./.bifrost/bifrost-app >/tmp/bifrost-vanilla-example.log 2>&1 &
pid=$!
for _ in $(seq 50); do
  if curl -sS --max-time 1 -o /dev/null "$base/"; then break; fi
  sleep 0.1
done

echo "server"
expect_status "home" 200 "/"
expect_body "home renders Go" "Bifrost vanilla" "/"
expect_body "ssr active nav" 'aria-current="page"' "/"
expect_body "ssr alpine fallback" "loading" "/"
expect_header "root middleware" "X-Vanilla" "1" "/"
expect_status "about group" 200 "/about"
expect_body "about group body" "marketing~/about" "/about"
expect_status "island" 200 "/island"

echo "components"
expect_status "components page" 200 "/components-demo"
cards=$(curl -sS --max-time 10 "$base/components-demo" | grep -o '<div data-bifrost-component="card">' | wc -l | tr -d ' ')
if [[ "$cards" == "20" ]]; then
  ok "20 component instances"
else
  bad "20 component instances" "got $cards"
fi
styles=$(curl -sS --max-time 10 "$base/components-demo" | grep -o '<style data-bifrost-components>' | wc -l | tr -d ' ')
if [[ "$styles" == "1" ]]; then
  ok "component css once"
else
  bad "component css once" "got $styles"
fi
expect_body "component scope" '@scope ([data-bifrost-component="card"])' "/components-demo"
expect_body "component dropdown" 'data-bifrost-component="dropdown"' "/components-demo"
css=$(curl -sS --max-time 10 "$base/" | grep -oE '/styles\.[a-f0-9]{12}\.css' | head -1 || true)
if [[ -n "$css" ]]; then
  ok "hashed css"
  expect_status "hashed css response" 200 "$css"
  expect_header "hashed css cache" "Cache-Control" "immutable" "$css"
else
  bad "hashed css" "no hashed URL in the page"
fi
js=$(curl -sS --max-time 10 "$base/" | grep -oE '/site\.[a-f0-9]{12}\.js' | head -1 || true)
if [[ -n "$js" ]]; then
  ok "hashed js"
  expect_status "hashed js response" 200 "$js"
else
  bad "hashed js" "no hashed URL in the page"
fi

echo "posts"
expect_status "posts" 200 "/posts"
expect_header "cache header" "Cache-Control" "s-maxage=60" "/posts"
expect_body "posts loader" "Hello, vanilla" "/posts"
expect_status "post" 200 "/posts/hello"
expect_body "post params" "slug: hello" "/posts/hello"
expect_status "loader not found" 404 "/posts/missing"
expect_body "not found view" "Not found" "/posts/missing"
expect_status "loader error" 503 "/posts/boom"
expect_body "nearest error view" "Posts error" "/posts/boom"

echo "docs"
expect_status "docs" 200 "/docs"
expect_status "docs catch-all" 200 "/docs/guides/start"
expect_body "docs catch-all body" "guides/start" "/docs/guides/start"
expect_header "markdown by suffix" "Content-Type" "text/markdown" "/docs/guides/start.md"

echo "api"
expect_header "api json" "Content-Type" "application/json" "/api/time"
expect_body "api time" '"time"' "/api/time"

echo "data and 404"
data=$(curl -sS --max-time 10 -H 'Accept: application/vnd.bifrost.data+json' "$base/posts/hello" || true)
if grep -q '"props"' <<<"$data"; then ok "data request"; else bad "data request" "$data"; fi
expect_status "unknown route" 404 "/nope"

echo
printf '%d passed, %d failed\n' "$passed" "$failed"
[[ "$failed" == "0" ]]
