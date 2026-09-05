#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
pid=""
cleanup() {
  if [[ -n "$pid" ]]; then
    kill -TERM "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT
trap 'cat "$tmp/server.log" "$tmp/build.log" 2>/dev/null' ERR

cd "$root"
go build -buildvcs=false -o "$tmp/bifrost" ./cmd/bifrost
app="$tmp/app"
mkdir -p "$app/posts/slug_" "$app/public"
ln -s "$root/node_modules" "$app/node_modules"
cat >"$app/go.mod" <<EOF
module navigation.test/app

go 1.25.0

require github.com/3-lines-studio/bifrost v0.0.0
replace github.com/3-lines-studio/bifrost => $root
EOF
cat >"$app/vite.config.ts" <<'EOF'
import babel from "@rolldown/plugin-babel";
import { defineConfig } from "vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react(), babel({ presets: [reactCompilerPreset()] })],
});
EOF
cat >"$app/layout.tsx" <<'EOF'
import { useEffect, useState } from "react";
import { navigate, refresh } from "virtual:bifrost/navigation";
export function Layout({ children }) {
  useEffect(() => {
    window.navigationAPI = { navigate, refresh };
  }, []);
  const [count, setCount] = useState(0);
  return <><nav><button onClick={() => setCount(count + 1)}>Root {count}</button><a href="/">Home</a><a href="/posts/one">One</a><a href="/posts/two?q=yes">Two</a><a href="/posts/slow">Slow</a><a href="/posts/redirect">Redirect</a><a href="/posts/missing">Missing</a><a href="/posts/broken">Broken</a><a href="/unknown">Unknown</a><a href="/posts/one#anchor">Anchor</a><a href="/posts/one" data-bifrost-reload>Reload</a><a href="/file.txt">File</a><a href="/posts/two" target="_blank">New tab</a><a href="/file.txt" download>Download</a></nav>{children}</>;
}
EOF
cat >"$app/page.tsx" <<'EOF'
const theme = `(function(){document.documentElement.classList.toggle('dark',true)})()`;
export function Head() {
  return <><title>Home</title><meta name="description" content="home" /><script dangerouslySetInnerHTML={{ __html: theme }} /></>;
}
export function Page() {
  return <main><h1>Home</h1></main>;
}
EOF
cat >"$app/not-found.tsx" <<'EOF'
export function NotFound() {
  return <main><h1>Not found</h1></main>;
}
EOF
cat >"$app/error.tsx" <<'EOF'
export function Error({ error }) {
  return <main><h1>Failed</h1><p>{error}</p></main>;
}
EOF
cat >"$app/posts/layout.tsx" <<'EOF'
import { useState } from "react";
export function Layout({ children }) {
  const [count, setCount] = useState(0);
  return <section><button onClick={() => setCount(count + 1)}>Posts {count}</button>{children}</section>;
}
EOF
cat >"$app/posts/slug_/page.tsx" <<'EOF'
import { useState } from "react";
import { navigate, refresh } from "virtual:bifrost/navigation";
import "./page.css";
const theme = `(function(){document.documentElement.classList.toggle('dark',true)})()`;
export function Head({ slug }) {
  return <><title>{slug}</title><meta name="description" content={slug} /><script dangerouslySetInnerHTML={{ __html: theme }} /></>;
}
export function Page({ slug, query, middleware, revision }) {
  const [count, setCount] = useState(0);
  const [draft, setDraft] = useState("");
  async function save(event) {
    event.preventDefault();
    const response = await fetch(location.pathname, { method: "POST" });
    if (!response.ok) {
      throw new Error("Save failed");
    }
    await refresh();
  }
  return <main><h1>{slug}</h1><p id="query">{query}</p><p id="middleware">{middleware}</p><p id="revision">{revision}</p><button onClick={() => setCount(count + 1)}>Page {count}</button><form onSubmit={save}><input id="draft" value={draft} onChange={event => setDraft(event.target.value)} /><button>Save</button></form><button onClick={() => navigate("/posts/two?from=action")}>Go to two</button><div style={{ height: 1800 }} /><h2 id="anchor">Anchor target</h2><div style={{ height: 1000 }} /></main>;
}
EOF
printf 'h1 { color: rgb(120, 30, 40); }\n' >"$app/posts/slug_/page.css"
cat >"$app/posts/slug_/page.go" <<'EOF'
package post

import (
  "errors"
  "net/http"
  "strconv"
  "sync/atomic"
  "time"

  "github.com/3-lines-studio/bifrost"
)

var revision atomic.Int64

func Load(r *http.Request) (any, error) {
  slug := r.PathValue("slug")
  delay, _ := r.Cookie("navigation-delay")
  if slug == "slow" || delay != nil {
    select {
    case <-r.Context().Done():
      return nil, r.Context().Err()
    case <-time.After(2 * time.Second):
    }
  }
  if slug == "redirect" {
    return nil, bifrost.Redirect("/posts/two?q=redirect")
  }
  if slug == "missing" {
    return nil, bifrost.NotFound()
  }
  if slug == "broken" {
    return nil, bifrost.Status(http.StatusInternalServerError, errors.New("broken page"))
  }
  return bifrost.PageData{Props: map[string]string{"slug": slug, "query": r.URL.Query().Get("q"), "middleware": r.Header.Get("X-Navigation-Test"), "revision": strconv.FormatInt(revision.Load(), 10)}, Document: bifrost.Document{Lang: "es", Class: "post", Dir: "ltr"}}, nil
}
EOF
cat >"$app/posts/slug_/route.go" <<'EOF'
package post

import "net/http"

func Post(w http.ResponseWriter, r *http.Request) {
  revision.Add(1)
  w.WriteHeader(http.StatusNoContent)
}
EOF
cat >"$app/middleware.go" <<'EOF'
package app

import "net/http"

func Middleware(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    r.Header.Set("X-Navigation-Test", "passed")
    next.ServeHTTP(w, r)
  })
}
EOF
printf 'file body\n' >"$app/public/file.txt"

for mode in production development; do
  if [[ "$mode" = production ]]; then
    "$tmp/bifrost" build "$app" >"$tmp/build.log" 2>&1
    BIFROST_ADDR=127.0.0.1:18109 "$app/.bifrost/bifrost-app" >"$tmp/server.log" 2>&1 &
  else
    BIFROST_ADDR=127.0.0.1:18109 "$tmp/bifrost" dev "$app" >"$tmp/server.log" 2>&1 &
  fi
  pid=$!
  ready=0
  for _ in $(seq 1 600); do
    if curl -fsS http://127.0.0.1:18109/ >/dev/null 2>&1; then
      ready=1
      break
    fi
    sleep 0.1
  done
  test "$ready" = 1
  bun "$root/scripts/navigation-browser.mjs"
  kill -TERM "$pid"
  wait "$pid"
  pid=""
  echo "$mode navigation passed"
done
