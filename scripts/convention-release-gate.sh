#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
cli="$tmp/bifrost"
dev_pid=""
server_pid=""
cleanup() {
  if [[ -n "$dev_pid" ]]; then kill -TERM "$dev_pid" 2>/dev/null || true; wait "$dev_pid" 2>/dev/null || true; fi
  if [[ -n "$server_pid" ]]; then kill -TERM "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi
  rm -rf "$tmp"
}
trap cleanup EXIT

descendants() {
  local pending="$1" found="" next parent pid
  while [[ -n "$pending" ]]; do
    next=""
    while read -r pid parent; do
      for target in $pending; do
        if [[ "$parent" = "$target" ]]; then found="$found $pid"; next="$next $pid"; fi
      done
    done < <(ps -eo pid=,ppid=)
    pending="$next"
  done
  echo "$found"
}

deps="$tmp/deps"
mkdir "$deps"
if [[ -n "${BIFROST_RELEASE_BINARY:-}" ]]; then
  cp "$BIFROST_RELEASE_BINARY" "$cli"
  chmod +x "$cli"
  cat >"$deps/package.json" <<'EOF'
{"private":true,"dependencies":{"react":"19.2.4","react-dom":"19.2.4","vite":"^8.2.1"}}
EOF
  (cd "$deps" && bun install --frozen-lockfile=false >/dev/null)
  module_requirement="require github.com/3-lines-studio/bifrost ${BIFROST_RELEASE_VERSION:?BIFROST_RELEASE_VERSION is required}"
else
  cd "$root"
  go build -o "$cli" ./cmd/bifrost
  cp -a --reflink=auto "$root/node_modules" "$deps/node_modules"
  module_requirement=$'require github.com/3-lines-studio/bifrost v0.0.0\nreplace github.com/3-lines-studio/bifrost => '"$root"
fi

pure="$tmp/pure"
mkdir -p "$pure/public"
ln -s "$deps/node_modules" "$pure/node_modules"
printf 'export function Page() { return <main>pure-page</main> }\n' >"$pure/page.tsx"
printf 'public-ok\n' >"$pure/public/status.txt"
"$cli" build "$pure" >/dev/null 2>&1
BIFROST_ADDR=127.0.0.1:18101 "$pure/.bifrost/bifrost-app" >"$tmp/pure.log" 2>&1 &
server_pid=$!
for _ in $(seq 1 200); do curl -fsS http://127.0.0.1:18101/ >"$tmp/pure.html" 2>/dev/null && break; sleep 0.05; done
grep -q pure-page "$tmp/pure.html"
test "$(curl -fsS http://127.0.0.1:18101/status.txt)" = public-ok
kill -TERM "$server_pid"
wait "$server_pid"
server_pid=""

app="$tmp/full"
mkdir -p "$app/posts/slug_" "$app/posts/static" "$app/posts/api/slug_" "$app/broken/slug_" "$app/late" "$app/public"
ln -s "$deps/node_modules" "$app/node_modules"
cat >"$app/go.mod" <<EOF
module release.test/app

go 1.25.0

$module_requirement
EOF
cat >"$app/page.tsx" <<'EOF'
export function Page() { return <main>root-page</main> }
EOF
cat >"$app/layout.tsx" <<'EOF'
import type { ReactNode } from "react";
export function Layout({ children }: { children: ReactNode }) { return <section>root-layout{children}</section> }
EOF
cat >"$app/error.tsx" <<'EOF'
export function Error({ error }: { error: string }) { return <main>root-error:{error}</main> }
EOF
cat >"$app/not-found.tsx" <<'EOF'
export function NotFound() { return <main>root-not-found</main> }
EOF
cat >"$app/middleware.go" <<'EOF'
package app
import (
  "context"
  "net/http"
)
type orderKey struct{}
func Middleware(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    next.ServeHTTP(w, WithOrder(r, []string{"root"}))
  })
}
func WithOrder(r *http.Request, order []string) *http.Request { return r.WithContext(context.WithValue(r.Context(), orderKey{}, order)) }
func Order(r *http.Request) []string { return r.Context().Value(orderKey{}).([]string) }
EOF
cat >"$app/server.go" <<'EOF'
package app
import (
  "context"
  "errors"
  "net"
  "net/http"
  "os"
  "time"
)
func Serve(ctx context.Context, handler http.Handler) error {
  wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("X-Custom-Server", "yes")
    handler.ServeHTTP(w, r)
  })
  server := &http.Server{Addr: os.Getenv("CUSTOM_ADDR"), Handler: wrapped, ReadHeaderTimeout: 5 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
  done := make(chan error, 1)
  go func() { done <- server.ListenAndServe() }()
  select {
  case err := <-done:
    if errors.Is(err, http.ErrServerClosed) { return nil }
    return err
  case <-ctx.Done():
  }
  shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
  return server.Shutdown(shutdown)
}
EOF
cat >"$app/posts/layout.tsx" <<'EOF'
import type { ReactNode } from "react";
export function Layout({ children }: { children: ReactNode }) { return <article>posts-layout{children}</article> }
EOF
cat >"$app/posts/error.tsx" <<'EOF'
export function Error({ error }: { error: string }) { if (globalThis.location?.search === "?fail-boundary") throw new Error("boundary failed"); return <main>posts-error:{error}</main> }
EOF
cat >"$app/posts/not-found.tsx" <<'EOF'
export function NotFound() { return <main>posts-not-found</main> }
EOF
cat >"$app/posts/middleware.go" <<'EOF'
package posts
import (
  "net/http"
  app "release.test/app"
)
func Middleware(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    order := append(app.Order(r), "posts")
    next.ServeHTTP(w, app.WithOrder(r, order))
  })
}
EOF
cat >"$app/posts/slug_/page.go" <<'EOF'
package post
import (
  "errors"
  "net/http"
  "os"
  "github.com/3-lines-studio/bifrost"
  app "release.test/app"
)
func Load(r *http.Request) (any, error) {
  slug := r.PathValue("slug")
  switch slug {
  case "missing": return nil, bifrost.NotFound()
  case "loader-error": return nil, errors.New("private-loader-error")
  case "slow":
    _ = os.WriteFile(os.Getenv("ACTIVE_MARKER"), []byte("started"), 0600)
    <-r.Context().Done()
    return nil, r.Context().Err()
  }
  return map[string]any{"slug": slug, "order": app.Order(r)}, nil
}
EOF
cat >"$app/posts/slug_/page.tsx" <<'EOF'
export function Page({ slug, order }: { slug: string; order: string[] }) {
  if (slug === "render-error") throw new Error("private-js-error");
  return <main>post:{slug}:{order.join(",")}:VITE_MARKER</main>
}
EOF
cat >"$app/late/page.tsx" <<'EOF'
import { Suspense } from "react";
let failed = false;
const pending = new Promise<void>((resolve) => setTimeout(() => { failed = true; resolve() }, 200));
function Late() { if (!failed) throw pending; throw new Error("private-late-error") }
export function Page() { return <Suspense fallback={<main>stream-started</main>}><Late /></Suspense> }
EOF
cat >"$app/posts/static/page.tsx" <<'EOF'
export function Page() { return <main>static-page</main> }
EOF
cat >"$app/broken/error.tsx" <<'EOF'
export function Error() { throw new Error("private-boundary-error") }
EOF
cat >"$app/broken/slug_/page.tsx" <<'EOF'
export function Page() { throw new Error("private-broken-page") }
EOF
cat >"$app/posts/api/slug_/route.go" <<'EOF'
package api
import (
  "encoding/json"
  "net/http"
  app "release.test/app"
)
func reply(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(map[string]any{"method": r.Method, "slug": r.PathValue("slug"), "order": app.Order(r)}) }
func Get(w http.ResponseWriter, r *http.Request) { reply(w, r) }
func Post(w http.ResponseWriter, r *http.Request) { reply(w, r) }
func Put(w http.ResponseWriter, r *http.Request) { reply(w, r) }
func Patch(w http.ResponseWriter, r *http.Request) { reply(w, r) }
func Delete(w http.ResponseWriter, r *http.Request) { reply(w, r) }
func Head(w http.ResponseWriter, r *http.Request) { w.Header().Set("X-Method", r.Method) }
func Options(w http.ResponseWriter, r *http.Request) { reply(w, r) }
EOF
cat >"$app/vite.config.ts" <<'EOF'
import { defineConfig } from "vite";
export default defineConfig({ plugins: [{ name: "release-gate", transform(code, id) { return id.endsWith("page.tsx") ? code.replaceAll("VITE_MARKER", "vite-ok") : null } }] });
EOF
printf 'asset-ok\n' >"$app/public/asset.txt"
(cd "$app" && go mod tidy)

"$cli" build "$app" >/dev/null 2>&1
CUSTOM_ADDR=127.0.0.1:18102 ACTIVE_MARKER="$tmp/active" "$app/.bifrost/bifrost-app" >"$tmp/full.log" 2>&1 &
server_pid=$!
for _ in $(seq 1 200); do curl -fsS http://127.0.0.1:18102/ >"$tmp/root.html" 2>/dev/null && break; sleep 0.05; done
grep -q root-layout "$tmp/root.html"
test "$(curl -sSI http://127.0.0.1:18102/ | awk -F': ' 'tolower($1)=="x-custom-server" {gsub("\r", "", $2); print $2}')" = yes
post=$(curl -fsS http://127.0.0.1:18102/posts/hello)
grep -q 'posts-layout' <<<"$post"
grep -q 'hello' <<<"$post"
grep -q 'root,posts' <<<"$post"
grep -q 'vite-ok' <<<"$post"
for method in GET POST PUT PATCH DELETE OPTIONS; do
  body=$(curl -fsS -X "$method" http://127.0.0.1:18102/posts/api/value)
  grep -q "\"method\":\"$method\"" <<<"$body"
  grep -q '"slug":"value"' <<<"$body"
done
grep -q '"order":\["root","posts"\]' <<<"$body"
test "$(curl -sS -o /dev/null -w '%{http_code}' -I http://127.0.0.1:18102/posts/api/value)" = 200
grep -q static-page < <(curl -fsS http://127.0.0.1:18102/posts/static)
test "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:18102/posts/hello/)" = 404
test "$(curl -fsS http://127.0.0.1:18102/asset.txt)" = asset-ok
traversal=$(curl --path-as-is -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:18102/../asset.txt)
test "$traversal" != 200
hostile=$(curl --path-as-is -sS http://127.0.0.1:18102/posts/%2e%2e%2Fetc 2>/dev/null || true)
! grep -q 'root:x:' <<<"$hostile"
test "$(curl -sS -o "$tmp/missing.html" -w '%{http_code}' http://127.0.0.1:18102/posts/missing)" = 404
grep -q posts-not-found "$tmp/missing.html"
test "$(curl -sS -o "$tmp/root-missing.html" -w '%{http_code}' http://127.0.0.1:18102/unknown)" = 404
grep -q root-not-found "$tmp/root-missing.html"
test "$(curl -sS -o "$tmp/loader-error.html" -w '%{http_code}' http://127.0.0.1:18102/posts/loader-error)" = 500
grep -q posts-error "$tmp/loader-error.html"
! grep -q private-loader-error "$tmp/loader-error.html"
test "$(curl -sS -o "$tmp/render-error.html" -w '%{http_code}' http://127.0.0.1:18102/posts/render-error)" = 500
grep -q posts-error "$tmp/render-error.html"
grep -q __bifrostErrorLevel "$tmp/render-error.html"
grep -q __BIFROST_PROPS__ "$tmp/render-error.html"
! grep -q private-js-error "$tmp/render-error.html"
! grep -Eq '(/home/|node:internal|render-error frame|goroutine [0-9])' "$tmp/render-error.html"
test "$(curl -sS -o "$tmp/outward-error.html" -w '%{http_code}' http://127.0.0.1:18102/broken/value)" = 500
grep -q root-error "$tmp/outward-error.html"
! grep -Eq 'private-(boundary|broken)' "$tmp/outward-error.html"
late_code=$(curl -sS -o "$tmp/late.html" -w '%{http_code}' http://127.0.0.1:18102/late || true)
test "$late_code" = 200
grep -q stream-started "$tmp/late.html"
! grep -q root-error "$tmp/late.html"
! grep -q private-late-error "$tmp/late.html"
curl -fsS http://127.0.0.1:18102/posts/slow >/dev/null 2>&1 &
active_curl=$!
for _ in $(seq 1 100); do [[ -f "$tmp/active" ]] && break; sleep 0.05; done
test -f "$tmp/active"
started=$(date +%s)
kill -TERM "$server_pid"
wait "$server_pid"
server_pid=""
wait "$active_curl" 2>/dev/null || true
test "$(( $(date +%s) - started ))" -lt 10

CUSTOM_ADDR=127.0.0.1:18103 ACTIVE_MARKER="$tmp/dev-active" "$cli" dev -poll 100ms "$app" >"$tmp/dev.log" 2>&1 &
dev_pid=$!
for _ in $(seq 1 600); do curl -fsS http://127.0.0.1:18103/posts/dev >"$tmp/dev.html" 2>/dev/null && break; sleep 0.05; done
grep -q dev "$tmp/dev.html"
grep -q 'root,posts' "$tmp/dev.html"
baseline=$(descendants "$dev_pid")
baseline_count=$(wc -w <<<"$baseline")
for value in one two three; do
  before=$(curl -fsS http://127.0.0.1:18103/_bifrost/build-id)
  printf '\nvar rebuild%s = "%s"\n' "$value" "$value" >>"$app/posts/api/slug_/route.go"
  changed=""
  for _ in $(seq 1 600); do
    after=$(curl -fsS http://127.0.0.1:18103/_bifrost/build-id 2>/dev/null || true)
    if [[ -n "$after" && "$after" != "$before" ]]; then changed=yes; break; fi
    sleep 0.05
  done
  test "$changed" = yes
  current_count=$(wc -w <<<"$(descendants "$dev_pid")")
  test "$current_count" -le "$((baseline_count + 2))"
done
children=$(descendants "$dev_pid")
kill -TERM "$dev_pid"
wait "$dev_pid"
dev_pid=""
for pid in $children; do
  if kill -0 "$pid" 2>/dev/null; then echo "orphan process $pid" >&2; exit 1; fi
done

if [[ "${BIFROST_EMPTY_GOMODCACHE:-}" = 1 ]]; then
  empty="$tmp/gomodcache"
  mkdir "$empty"
  GOMODCACHE="$empty" "$cli" build "$pure" >/dev/null 2>&1
fi

echo "convention release gate passed"
