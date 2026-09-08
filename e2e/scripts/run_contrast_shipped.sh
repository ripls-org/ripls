#!/usr/bin/env bash
# run_contrast_shipped.sh — capture the SHIPPED tokens only (#2770).
#
#   e2e/scripts/run_contrast_shipped.sh [--slug after] [--scope photos]
#
# The candidate sweep renders 3 palettes x 4 ramps = 24 builds, ~63 minutes.
# That is the right tool for choosing between options and the wrong one for
# checking whether a fix landed: it rebuilds 11 combinations nobody asked about.
# This runs the two builds that actually answer "what does the app look like
# now" — sentinel first, then baseline.
#
# Sentinel FIRST and baseline SECOND is load-bearing: the sentinel pass rewrites
# the generated token files, so ending on baseline leaves both the tree and the
# build products holding real values. A run killed mid-sentinel leaves marker
# hues behind; `npm run lint:design-tokens` is the backstop that catches it.

set -euo pipefail

SLUG="after"
SCOPE="photos"
SURFACES=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --slug)     SLUG="$2";     shift 2 ;;
    --scope)    SCOPE="$2";    shift 2 ;;
    --surfaces) SURFACES="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

cd "$(dirname "${BASH_SOURCE[0]}")/../.."
ROOT="$PWD"

echo "==> clearing previous state for slug '$SLUG'"
rm -rf "e2e/investigations/$SLUG/captures" "e2e/investigations/$SLUG"/manifest-*.json

ENV_START_PID=""

start_env() {
  local state="e2e/investigations/$SLUG/env.json"
  mkdir -p "e2e/investigations/$SLUG"
  rm -f "$state"
  (cd e2e && npm run env:start -- --slug "$SLUG") \
    >"e2e/investigations/$SLUG/env-start.log" 2>&1 &
  ENV_START_PID=$!
  for _ in $(seq 1 120); do
    if [ -f "$state" ]; then
      local url
      url=$(node -e "process.stdout.write(require('$PWD/$state').baseUrl||'')" 2>/dev/null || true)
      if [ -n "$url" ] && curl -sf -o /dev/null "$url" 2>/dev/null; then return 0; fi
    fi
    sleep 2
  done
  echo "env did not become ready within 240s" >&2
  return 1
}

stop_env() {
  (cd e2e && npm run env:stop -- --slug "$SLUG" >/dev/null 2>&1) || true
  if [ -n "$ENV_START_PID" ]; then kill "$ENV_START_PID" 2>/dev/null || true; ENV_START_PID=""; fi
}

cleanup() {
  echo "==> restoring real design tokens"
  node "$ROOT/scripts/gen_design_tokens.js" >/dev/null
  stop_env
}
trap cleanup EXIT INT TERM

capture() {  # pass
  npm run build:e2e >/dev/null
  start_env
  if [ -n "$SURFACES" ]; then
    (cd e2e && npx tsx scripts/contrast_capture.mts \
      --slug "$SLUG" --pass "$1" --scope "$SCOPE" --variant shipped \
      --surfaces "$SURFACES")
  else
    (cd e2e && npx tsx scripts/contrast_capture.mts \
      --slug "$SLUG" --pass "$1" --scope "$SCOPE" --variant shipped)
  fi
  stop_env
}

echo "==> sentinel pass"
node "$ROOT/scripts/gen_design_tokens.js" --sentinel >/dev/null
capture sentinel

echo "==> baseline pass"
node "$ROOT/scripts/gen_design_tokens.js" >/dev/null
capture baseline

cleanup
trap - EXIT

echo "==> building the findings page"
(cd e2e && node scripts/build_findings_page.mjs --slug "$SLUG" --variant shipped \
  --out "$ROOT/docs/concepts/theme/contrast-findings-$SLUG.html")
