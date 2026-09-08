#!/usr/bin/env bash
# run_contrast_gate.sh — composited-contrast gate, end to end (#2770).
#
#   e2e/scripts/run_contrast_gate.sh [--scope per-pr|full] [--slug contrast]
#
# Renders every surface in the evaluation set twice — once from the real tokens,
# once from a sentinel-hued bundle — and measures the contrast of what was
# actually painted. This catches what design/tokens.json's generation-time gate
# structurally cannot: a token validated against flat `background`/`surface` but
# rendered as a foreground on translucent glass over arbitrary media (#2764).
#
# Token values are compile-time constants, so each pass needs its own bundle.
# A rebuild is ~2.6 min and `flutter build web --wasm` has no meaningful
# incremental reuse for a constant change, so the two builds dominate runtime.
#
# Run from the repo root or from e2e/ — both work.

set -euo pipefail

SCOPE="full"
SLUG="contrast"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --scope) SCOPE="$2"; shift 2 ;;
    --slug)  SLUG="$2";  shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

cd "$(dirname "${BASH_SOURCE[0]}")/../.."   # repo root
ROOT="$PWD"

# The sentinel pass rewrites checked-in generated files. Restore on ANY exit —
# an interrupted run must never leave sentinel hues staged.
#
# This does NOT survive SIGKILL. A hard-killed run leaves sentinel values in the
# tree; `npm run lint:design-tokens` catches that and names the fix, so the
# drift gate is the real backstop and this trap is only the tidy path.
cleanup() {
  echo "==> restoring real design tokens"
  node "$ROOT/scripts/gen_design_tokens.js" >/dev/null
  stop_env
}
trap cleanup EXIT INT TERM

# `env:start` does NOT exit when it has to boot the Auth Emulator itself — the
# emulator is a non-detached child, so Node's event loop never drains and the
# command hangs with everything already healthy. (It returns promptly only when
# an emulator is already live and it can just attach.) So run it in the
# background and wait on observable readiness: env.json written, server serving.
ENV_START_PID=""

start_env() {
  local state="e2e/investigations/$SLUG/env.json"
  mkdir -p "e2e/investigations/$SLUG"
  rm -f "$state"   # never mistake a previous run's state for this one's

  (cd e2e && npm run env:start -- --slug "$SLUG") \
    >"e2e/investigations/$SLUG/env-start.log" 2>&1 &
  ENV_START_PID=$!

  for _ in $(seq 1 120); do
    if [ -f "$state" ]; then
      local url
      url=$(node -e "process.stdout.write(require('$PWD/$state').baseUrl||'')" 2>/dev/null || true)
      if [ -n "$url" ] && curl -sf -o /dev/null "$url" 2>/dev/null; then
        echo "    env ready at $url"
        return 0
      fi
    fi
    if ! kill -0 "$ENV_START_PID" 2>/dev/null && [ ! -f "$state" ]; then
      echo "env:start exited before writing state — see e2e/investigations/$SLUG/env-start.log" >&2
      return 1
    fi
    sleep 2
  done
  echo "env did not become ready within 240s" >&2
  return 1
}

stop_env() {
  (cd e2e && npm run env:stop -- --slug "$SLUG" >/dev/null 2>&1) || true
  if [ -n "$ENV_START_PID" ]; then
    kill "$ENV_START_PID" 2>/dev/null || true
    ENV_START_PID=""
  fi
}

capture_pass() {
  local pass="$1"
  echo "==> [$pass] building bundle"
  npm run build:e2e >/dev/null

  echo "==> [$pass] starting hermetic env (slug: $SLUG)"
  start_env

  echo "==> [$pass] capturing"
  (cd e2e && npx tsx scripts/contrast_capture.mts --slug "$SLUG" --pass "$pass" --scope "$SCOPE")

  echo "==> [$pass] stopping env"
  stop_env
}

# Sentinel FIRST, baseline SECOND. Restoring tokens at the end fixes the source
# tree but not the build products: the embedded web bundle and the server binary
# would still carry sentinel hues, so the next `npm run start:server` would serve
# a sentinel-coloured app with nothing to explain why. Ending on the baseline
# pass leaves both the tree and the artifacts correct, and costs nothing.
echo "==> generating sentinel tokens"
node "$ROOT/scripts/gen_design_tokens.js" --sentinel >/dev/null
capture_pass sentinel

echo "==> generating real tokens"
node "$ROOT/scripts/gen_design_tokens.js" >/dev/null
capture_pass baseline

# Restore before analysing so a failing gate still leaves a clean tree.
cleanup
trap - EXIT

echo "==> analysing"
(cd e2e && node scripts/analyze_contrast.mjs --slug "$SLUG" --json "investigations/$SLUG/report.json")
