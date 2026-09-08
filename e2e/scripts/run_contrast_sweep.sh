#!/usr/bin/env bash
# run_contrast_sweep.sh — render every candidate combination (#2770 Phase 2).
#
#   e2e/scripts/run_contrast_sweep.sh [--scope per-pr|full] [--slug sweep]
#
# Sweeps palette x glass-ramp from design/candidates/, rendering each
# combination twice (baseline + sentinel) so the analyzer can attribute pixels
# to tokens, then builds the comparison sheet.
#
# Cost is dominated by builds, not renders: token values are compile-time
# constants, so each combination needs its own bundle at ~2.6 min. 3 palettes x
# 3 ramps x 2 passes = 18 builds ~= 47 min. Run it unattended.
#
# Only palette x ramp needs a build — theme and backdrop are runtime, which is
# why those two axes stay inside a single capture pass.

set -euo pipefail

SCOPE="full"
SLUG="sweep"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --scope) SCOPE="$2"; shift 2 ;;
    --slug)  SLUG="$2";  shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

cd "$(dirname "${BASH_SOURCE[0]}")/../.."
ROOT="$PWD"
CAND="$ROOT/design/candidates"

# Plain globs rather than `mapfile`: macOS ships bash 3.2, which has no
# mapfile, and this script's shebang resolves to whatever bash is first on PATH.
PALETTES=("$CAND"/palette-*.json)
RAMPS=("$CAND"/glass-*.json)
if [ ! -e "${PALETTES[0]}" ] || [ ! -e "${RAMPS[0]}" ]; then
  echo "no candidates found in $CAND" >&2
  exit 1
fi
echo "==> ${#PALETTES[@]} palettes x ${#RAMPS[@]} ramps = $(( ${#PALETTES[@]} * ${#RAMPS[@]} * 2 )) builds"

# Purge previous captures and manifests before starting.
#
# Without this, a killed or differently-scoped earlier run leaves manifests
# behind and the analyzer happily folds them in — producing a sheet that
# compares candidates rendered from DIFFERENT CODE STATES. That is worse than no
# sheet: it looks like a decision artifact and every conclusion from it is an
# artifact of which run each variant came from. Observed once, hence this.
echo "==> clearing previous sweep state"
rm -rf "e2e/investigations/$SLUG/captures" "e2e/investigations/$SLUG"/manifest-*.json

ENV_START_PID=""

# See run_contrast_gate.sh: env:start does not exit when it boots the Auth
# Emulator itself, so wait on observable readiness instead.
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

# The sentinel pass rewrites checked-in generated files. Restore on ANY exit.
#
# This trap does NOT survive SIGKILL, so a hard-killed run leaves sentinel hues
# in the tree. That is caught by `npm run lint:design-tokens` (regenerate and
# diff), which names the fix in its failure message — the drift gate is the real
# backstop here, the trap is just the tidy path. If a run was killed, run
# `npm run generate:design-tokens` before trusting anything.
cleanup() {
  echo "==> restoring real design tokens"
  node "$ROOT/scripts/gen_design_tokens.js" >/dev/null
  stop_env
}
trap cleanup EXIT INT TERM

capture() {  # variant, pass
  local variant="$1" pass="$2"
  npm run build:e2e >/dev/null
  start_env
  (cd e2e && npx tsx scripts/contrast_capture.mts \
    --slug "$SLUG" --pass "$pass" --scope "$SCOPE" --variant "$variant")
  stop_env
}

for p in "${PALETTES[@]}"; do
  for r in "${RAMPS[@]}"; do
    pid=$(node -e "process.stdout.write(require('$p').id)")
    rid=$(node -e "process.stdout.write(require('$r').id)")
    variant="${pid}__${rid}"
    echo "==> [$variant]"

    # Sentinel FIRST, baseline SECOND, so the tree AND the build products are
    # left holding real values — see run_contrast_gate.sh for why that matters.
    node "$ROOT/scripts/gen_design_tokens.js" --sentinel --candidate "$p" --candidate "$r" >/dev/null
    capture "$variant" sentinel

    node "$ROOT/scripts/gen_design_tokens.js" --candidate "$p" --candidate "$r" >/dev/null
    capture "$variant" baseline
  done
done

cleanup
trap - EXIT

echo "==> analysing and building the sheet"
(cd e2e && node scripts/analyze_sweep.mjs --slug "$SLUG" \
  --out "$ROOT/docs/concepts/theme/contrast-eval.html")
