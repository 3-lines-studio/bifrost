#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
addr=127.0.0.1:18701
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
  local name=$1 want=$2 url=$3 method=${4:-GET}
  local got
  if [[ "$method" == "HEAD" ]]; then
    got=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -I "$base$url" || echo 000)
  else
    got=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X "$method" "$base$url" || echo 000)
  fi
  if [[ "$got" == "$want" ]]; then ok "$name ($got)"; else bad "$name" "wanted $want, got $got"; fi
}

expect_body() {
  local name=$1 pattern=$2 url=$3
  local body
  body=$(curl -sS --max-time 10 "$base$url" || true)
  if grep -qF -- "$pattern" <<<"$body"; then ok "$name"; else bad "$name" "no match for [$pattern]"; fi
}

expect_absent() {
  local name=$1 pattern=$2 url=$3
  local body
  body=$(curl -sS --max-time 10 "$base$url" || true)
  if grep -qF -- "$pattern" <<<"$body"; then bad "$name" "unexpected [$pattern]"; else ok "$name"; fi
}

expect_header() {
  local name=$1 pattern=$2 url=$3 method=${4:-GET}
  local headers
  headers=$(curl -sS --max-time 10 -D - -o /dev/null -X "$method" "$base$url" || true)
  if grep -qiF -- "$pattern" <<<"$headers"; then ok "$name"; else bad "$name" "missing header [$pattern]"; fi
}

cd "$root"
BIFROST_ADDR="$addr" ./.bifrost/bifrost-app >/tmp/app-router-demo.log 2>&1 &
pid=$!
ready=0
for _ in $(seq 1 200); do
  if curl -fsS -o /dev/null "$base/healthz" 2>/dev/null; then ready=1; break; fi
  sleep 0.05
done
if [[ "$ready" != "1" ]]; then echo "the demo app never became ready"; exit 1; fi

echo "shell and server"
expect_status "home renders" 200 "/"
expect_body "home shows the map" "Everything the App Router does" "/"
expect_header "root middleware header" "X-Demo-Root: 1" "/"
expect_body "root template wraps the tree" 'data-template="root"' "/"
expect_body "root metadata is applied" "Bifrost App Router demo" "/"
expect_body "stylesheet is linked" "styles.css" "/"
expect_status "public file" 200 "/styles.css"
expect_status "public download file" 200 "/report.txt"
expect_status "custom server health route" 200 "/healthz"
expect_header "server redirect for old docs" "HTTP/1.1 301" "/old-docs/markers"
expect_status "unknown path is a 404" 404 "/nope"
expect_body "root not-found view" 'data-not-found="root"' "/nope"
expect_header "trailing slash redirects" "HTTP/1.1 308" "/docs/markers/"

echo "docs: catch-all, nesting and markdown"
expect_status "docs index" 200 "/docs"
expect_body "docs index lists pages" "docs/markers" "/docs"
expect_status "catch-all page" 200 "/docs/markers"
expect_body "catch-all segments are an array" '[&quot;markers&quot;]' "/docs/markers"
expect_body "section middleware composes" "root → docs" "/docs/markers"
expect_header "section middleware header" "X-Demo-Docs: 1" "/docs/markers"
expect_body "section layout wraps the page" 'data-layout="docs"' "/docs/markers"
expect_status "missing doc is a 404" 404 "/docs/nope"
expect_body "missing doc renders its own view" 'data-missing="nope"' "/docs/nope"
expect_body "generateMetadata runs per request" "Documentation for markers." "/docs/markers"
expect_header "markdown by suffix" "text/markdown" "/docs/markers.md"
expect_body "markdown body" "# " "/docs/markers.md"
accept=$(curl -sS --max-time 10 -H 'Accept: text/markdown' -D - -o /dev/null "$base/docs/markers" | tr -d '\r' | grep -i '^content-type' || true)
if grep -qi 'text/markdown' <<<"$accept"; then ok "markdown by accept header"; else bad "markdown by accept header" "$accept"; fi

echo "projects: dynamic, static precedence and mutations"
expect_status "project list" 200 "/projects"
expect_status "dynamic page" 200 "/projects/bifrost"
expect_body "params.slug arrives" 'data-project="bifrost"' "/projects/bifrost"
expect_body "static beats dynamic" "Static beats dynamic" "/projects/new"
expect_status "unknown project is a 404" 404 "/projects/nope"
expect_body "nested log page" 'data-log="wax"' "/projects/wax/log"
entry="log entry from check $$"
curl -sS --max-time 10 -o /dev/null -X POST --data-urlencode "text=$entry" "$base/projects/wax" || true
expect_body "route.go mutation is visible" "$entry" "/projects/wax/log"
put=$(curl -sS --max-time 10 -X PUT "$base/projects/wax")
if grep -qF '"method":"PUT"' <<<"$put"; then ok "route.go PUT"; else bad "route.go PUT" "$put"; fi

echo "admin: one middleware over pages and route.go"
expect_header "guarded page redirects" "HTTP/1.1 302" "/admin"
expect_header "guarded api redirects" "HTTP/1.1 302" "/admin/api/state"
cookie=$(curl -sS --max-time 10 -D - -o /dev/null -X POST "$base/admin/login?next=/admin" | tr -d '\r' | awk '/^[Ss]et-[Cc]ookie:/{print $2}')
if [[ "$cookie" == demo-admin=ok* ]]; then ok "login sets the cookie"; else bad "login sets the cookie" "$cookie"; fi
admin=$(curl -sS --max-time 10 -H "Cookie: $cookie" "$base/admin")
if grep -qF "root → admin" <<<"$admin"; then ok "guarded page renders with the cookie"; else bad "guarded page renders with the cookie" "$admin"; fi
state=$(curl -sS --max-time 10 -H "Cookie: $cookie" "$base/admin/api/state")
if grep -qF '"method":"GET"' <<<"$state" && grep -qF "admin" <<<"$state"; then ok "route.go behind the same middleware"; else bad "route.go behind the same middleware" "$state"; fi
expect_status "settings page is guarded too" 302 "/admin/settings"

echo "api: methods, 405 and hand-written markdown"
expect_body "search returns json" '"hits"' "/api/search?q=marker"
expect_status "unsupported method is a 405" 405 "/api/search" TRACE
expect_header "405 carries an allow header" "Allow:" "/api/search" TRACE
for method in GET POST PUT PATCH DELETE; do
  body=$(curl -sS --max-time 10 -X "$method" "$base/api/echo" || true)
  if grep -qF "\"method\":\"$method\"" <<<"$body"; then ok "echo $method"; else bad "echo $method" "$body"; fi
done
expect_status "echo HEAD" 200 "/api/echo" HEAD
expect_status "echo OPTIONS" 204 "/api/echo" OPTIONS
expect_header "hand-written markdown route" "text/markdown" "/api/export"

echo "labs: the behaviours, one by one"
expect_status "slow loader" 200 "/labs/loading"
expect_body "slow loader ran" "700ms" "/labs/loading"
expect_status "render error keeps its status" 500 "/labs/error?boom=1"
expect_body "nearest error view" 'data-error="labs"' "/labs/error?boom=1"
expect_status "loader error status" 503 "/labs/loader-error"
expect_body "loader error view" 'data-error="labs"' "/labs/loader-error"
expect_status "loader not-found" 404 "/labs/notfound"
expect_body "section not-found view" 'data-not-found="labs"' "/labs/notfound"
expect_status "the same lab with data" 200 "/labs/notfound?ok=1"
expect_body "repeated query keys become an array" '["one","two"]' "/labs/query?tag=one&tag=two"
expect_header "labs middleware header" "X-Demo-Labs: 1" "/labs/middleware"
expect_body "metadata title override" "The page overrides the layout title" "/labs/metadata"
expect_body "layout metadata survives" "openGraph" "/labs/metadata"
expect_body "directory lang on the document" '<html lang="pt-BR" class="lab-dark" dir="ltr"' "/labs/document"
expect_body "lang from the query" '<html lang="en"' "/labs/document?lang=en"
expect_absent "invalid lang is rejected" 'lang="not a language"' "/labs/document?lang=not%20a%20language"
expect_body "no-js form round trip" 'data-nojs-echo="hola"' "/labs/nojs?message=hola"
expect_body "hash lab loader count" 'data-loads=' "/labs/hash"

echo "labs: request scope under concurrency"
ids=$(for _ in $(seq 12); do curl -sS --max-time 10 "$base/labs/request" & done | grep -o 'data-request="[a-f0-9]*"' | sort -u | wc -l)
if [[ "$ids" == "12" ]]; then ok "twelve concurrent requests, twelve ids"; else bad "twelve concurrent requests, twelve ids" "got $ids"; fi
loads=$(curl -sS --max-time 10 "$base/labs/request" | grep -c 'data-request=' || true)
if [[ "$loads" == "1" ]]; then ok "request lab counts its calls"; else bad "request lab counts its calls" "counter=$loads"; fi

echo
printf '%d passed, %d failed\n' "$passed" "$failed"
[[ "$failed" == "0" ]]
