#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
binary=$(mktemp)
binary2=$(mktemp)
stdout=$(mktemp)
stderr=$(mktemp)
pid=""
pid2=""
cleanup() {
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  if [[ -n "$pid2" ]]; then
    kill "$pid2" 2>/dev/null || true
    wait "$pid2" 2>/dev/null || true
  fi
  rm -f "$binary" "$binary2" "$stdout" "$stderr"
}
trap cleanup EXIT

cd "$root"
go build -o "$binary" ./example/basic
"$binary" >"$stdout" 2>"$stderr" &
pid=$!

ready=0
for _ in $(seq 1 300); do
  if curl -sS http://127.0.0.1:8080/about >/tmp/bifrost-about.html 2>/dev/null; then
    ready=1
    break
  fi
  sleep 0.05
done
if [[ "$ready" != 1 ]]; then
  cat "$stderr" >&2
  exit 1
fi

test "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/ready)" = 204
curl -fsS 'http://127.0.0.1:8080/?name=Don' >/tmp/bifrost-home.html
curl -fsS http://127.0.0.1:8080/app >/tmp/bifrost-app.html
curl -fsS http://127.0.0.1:8080/post/first >/tmp/bifrost-post-first.html
curl -fsS http://127.0.0.1:8080/post/second >/tmp/bifrost-post.html
curl -fsS http://127.0.0.1:8080/robots.txt >/tmp/bifrost-robots.txt
asset=$(grep -o '/_bifrost/dist/[^" ]*\.js' /tmp/bifrost-home.html | head -1)
curl -fsSI "http://127.0.0.1:8080$asset" >/tmp/bifrost-asset.headers

grep -q '<html lang="es" class="theme-dark" dir="ltr">' /tmp/bifrost-home.html
grep -q '<title>Hello Don</title>' /tmp/bifrost-home.html
grep -q 'Hello.*Don' /tmp/bifrost-home.html
grep -q '<h1>About</h1>' /tmp/bifrost-about.html
grep -q '__BIFROST_PROPS__' /tmp/bifrost-app.html
grep -q '<html lang="pt-BR" dir="ltr">' /tmp/bifrost-post-first.html
grep -q '<h1>Second</h1>' /tmp/bifrost-post.html
grep -q 'User-agent' /tmp/bifrost-robots.txt
grep -qi 'cache-control: public, max-age=31536000, immutable' /tmp/bifrost-asset.headers

test "$(pgrep -P "$pid" | wc -l)" = 2
renderer_pid=$(pgrep -P "$pid" | head -1)
kill -KILL "$renderer_pid"
for _ in $(seq 1 100); do
  if [[ "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/ready)" = 503 ]]; then break; fi
  sleep 0.01
done
test "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/ready)" = 503
curl -sS 'http://127.0.0.1:8080/?name=Restart' >/dev/null || true
restarted=0
for _ in $(seq 1 200); do
  if curl -sS 'http://127.0.0.1:8080/?name=Restart' 2>/dev/null | grep -q 'Hello.*Restart'; then
    restarted=1
    break
  fi
  sleep 0.05
done
test "$restarted" = 1
for _ in $(seq 1 100); do
  if [[ "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/ready)" = 204 ]]; then break; fi
  curl -sS 'http://127.0.0.1:8080/?name=Restart' >/dev/null || true
  sleep 0.01
done
test "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/ready)" = 204

bun ./scripts/browser.mjs

test ! -e example/plugin/.bifrost/runtime/bifrost-renderer
test ! -e example/plugin/.bifrost/ssr
go build -o "$binary2" ./example/plugin
"$binary2" >/tmp/bifrost-plugin.out 2>/tmp/bifrost-plugin.err &
pid2=$!
for _ in $(seq 1 200); do
  if curl -sS http://127.0.0.1:8081/ >/tmp/bifrost-plugin.html 2>/dev/null; then break; fi
  sleep 0.05
done
curl -fsSI http://127.0.0.1:8081/dashboard >/tmp/bifrost-plugin.headers
curl -fsSI http://127.0.0.1:8081/robots.txt >/tmp/bifrost-plugin-asset.headers
grep -q '<h1>Plugin example</h1>' /tmp/bifrost-plugin.html
grep -qi 'x-example-plugin: active' /tmp/bifrost-plugin.headers
grep -qi 'x-asset-plugin: active' /tmp/bifrost-plugin-asset.headers

echo 'integration passed'
