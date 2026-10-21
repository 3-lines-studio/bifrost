#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
addr=127.0.0.1:18932
base="http://$addr"
pid=""
tmp=""
passed=0
failed=0

cleanup() {
  if [[ -n "$pid" ]]; then kill -TERM "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  if [[ -n "$tmp" ]]; then rm -rf "$tmp"; fi
}
trap cleanup EXIT

ok() { passed=$((passed + 1)); printf '  \033[32mok\033[0m   %s\n' "$1"; }
bad() { failed=$((failed + 1)); printf '  \033[31mFAIL\033[0m %s\n' "$1"; if [[ -n "${2:-}" ]]; then printf '       %s\n' "$2"; fi; }

cd "$root"
BIFROST_ADDR="$addr" ./.bifrost/bifrost-app >/tmp/bifrost-alpine-ui-check.log 2>&1 &
pid=$!
for _ in $(seq 50); do
  if curl -sS --max-time 1 -o /dev/null "$base/"; then break; fi
  sleep 0.1
done

body=$(curl -sS --max-time 10 "$base/")
for expected in 'ui-button' 'ui-dialog__panel' 'ui-dropdown-menu__panel' 'ui-tabs__list' 'ui-alert' 'ui-table' 'ui-pagination' 'ui-toasts' 'ui-pricing' '/ui/ui.js' '/ui/ui.css'; do
  if grep -qF -- "$expected" <<<"$body"; then ok "$expected"; else bad "$expected"; fi
done
for path in /ui/ui.css /ui/ui.js /app.css; do
  status=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$base$path")
  if [[ "$status" == "200" ]]; then ok "$path ($status)"; else bad "$path" "got $status"; fi
done

echo "registry"
if go run ./cmd/ui list | grep -q "dialog"; then ok "ui list"; else bad "ui list"; fi
tmp=$(mktemp -d)
if go run ./cmd/ui add dialog -to "$tmp" >/dev/null; then ok "ui add"; else bad "ui add"; fi
for file in ui/ui.go ui/dialog.go public/ui/ui.css public/ui/ui.js public/ui/dialog.css; do
  if [[ -f "$tmp/$file" ]]; then ok "copied $file"; else bad "copied $file"; fi
done
if grep -q './dialog.css' "$tmp/public/ui/ui.css"; then ok "ui css imports"; else bad "ui css imports"; fi

if [[ "$failed" -eq 0 ]]; then printf '%d passed, 0 failed\n' "$passed"; else printf '%d passed, %d failed\n' "$passed" "$failed"; exit 1; fi
