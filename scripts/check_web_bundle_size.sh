#!/usr/bin/env bash
#
# check_web_bundle_size.sh — verifies that the Flutter Web bundle
# stays within budget on two axes:
#
#   1. INITIAL-CHUNK gzipped size — what the browser parses
#      synchronously before any deferred loads. Tolerated growth:
#      +10% over baseline.
#   2. TOTAL bundle gzipped size — every file under app/build/web/
#      that the server serves. Catches unbounded deferred-route
#      growth that the initial-chunk gate misses. Tolerated
#      growth: +20% (looser because deferred chunks naturally grow
#      as new screens land).
#
# Renderer: WASM with a JS fallback (#2119). `flutter build web --wasm`
# emits main.dart.wasm + main.dart.mjs (what a modern browser loads)
# alongside main.dart.js (the fallback older browsers load). The
# realistic bundle floor is a few MB gzipped, so both gates use measured
# baselines + headroom rather than hard caps. The bytes are served
# brotli/gzip-precompressed in production (see app_handler.go); this gate
# still measures gzipped build artifacts as a stable size *budget*.
#
# Baselines live in app/web/bundle_baseline.txt — first non-comment
# line is the initial-chunk baseline (bytes, gzipped), second is the
# total-bundle baseline. Setting either to 0 disables that gate; the
# script prints the current measurement so a PR can pick a number.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_DIR="${REPO_ROOT}/app"
BUILD_DIR="${APP_DIR}/build/web"
BASELINE_FILE="${APP_DIR}/web/bundle_baseline.txt"

# Initial-chunk tolerance: +10%. Tight because every byte here hits
# time-to-interactive on every cold visit.
INITIAL_TOLERANCE_NUMERATOR=110
INITIAL_TOLERANCE_DENOMINATOR=100

# Total-bundle tolerance: +20%. Looser because deferred chunks grow
# naturally as new screens land, and the cost is per-route on demand
# rather than per-cold-visit.
TOTAL_TOLERANCE_NUMERATOR=120
TOTAL_TOLERANCE_DENOMINATOR=100

if [[ ! -d "${APP_DIR}" ]]; then
  echo "::error::app/ directory not found at ${APP_DIR}" >&2
  exit 2
fi

if ! command -v flutter >/dev/null 2>&1; then
  echo "::error::flutter command not on PATH" >&2
  exit 2
fi

if ! command -v gzip >/dev/null 2>&1; then
  echo "::error::gzip command not on PATH" >&2
  exit 2
fi

echo ">>> running flutter build web --wasm --release"
(cd "${APP_DIR}" && flutter build web --wasm --release) >/dev/null

if [[ ! -d "${BUILD_DIR}" ]]; then
  echo "::error::flutter build web did not produce ${BUILD_DIR}" >&2
  exit 2
fi

# Identify the initial-chunk files. The Flutter Web initial chunk is
# whatever the browser parses synchronously before any deferred loads.
# flutter_bootstrap.js (tiny loader) is always present; the entry then
# differs by renderer target:
#   * --wasm build → main.dart.mjs (loader) + main.dart.wasm (module) is
#     what a WASM-capable browser downloads. main.dart.js is the fallback
#     older browsers load *instead*, so it is NOT part of this path.
#   * JS build     → main.dart.js (compiled Dart entry).
# We detect the WASM build by main.dart.wasm and measure that path;
# otherwise fall back to the JS entry. Deferred-route chunks (*.part.js,
# canvaskit/*, skwasm*) load on demand and are excluded here.
initial_files=()
[[ -f "${BUILD_DIR}/flutter_bootstrap.js" ]] && initial_files+=("${BUILD_DIR}/flutter_bootstrap.js")
if [[ -f "${BUILD_DIR}/main.dart.wasm" ]]; then
  [[ -f "${BUILD_DIR}/main.dart.mjs" ]] && initial_files+=("${BUILD_DIR}/main.dart.mjs")
  initial_files+=("${BUILD_DIR}/main.dart.wasm")
elif [[ -f "${BUILD_DIR}/main.dart.js" ]]; then
  initial_files+=("${BUILD_DIR}/main.dart.js")
fi

if [[ ${#initial_files[@]} -eq 0 ]]; then
  echo "::error::no initial-chunk entry found in ${BUILD_DIR}" >&2
  exit 2
fi

echo ""
echo "=== initial chunk ==="
initial_bytes=0
for f in "${initial_files[@]}"; do
  bytes=$(gzip -c "$f" | wc -c | tr -d ' ')
  rel="${f#${BUILD_DIR}/}"
  printf "  %-40s %10d bytes (gzipped)\n" "$rel" "$bytes"
  initial_bytes=$((initial_bytes + bytes))
done
echo "initial chunk total (gzipped): ${initial_bytes} bytes"

echo ""
echo "=== total bundle ==="
# Gzip-and-sum every file under build/web. We intentionally include
# canvaskit/, skwasm*, and every *.part.js because the gate is about
# disk + CDN cost, not initial-chunk cost. Excluded: precompressed
# *.br/*.gz siblings — this gate runs a plain `flutter build web` (not
# the npm build:web that precompresses), so they normally aren't here,
# but the filter keeps the measurement stable (no gzipping a .gz) if a
# precompressed tree is ever measured.
total_bytes=0
while IFS= read -r -d '' f; do
  bytes=$(gzip -c "$f" | wc -c | tr -d ' ')
  total_bytes=$((total_bytes + bytes))
done < <(find "${BUILD_DIR}" -type f ! -name '*.br' ! -name '*.gz' -print0)
echo "total bundle (gzipped sum of all files): ${total_bytes} bytes"

# Read the baselines. Layout:
#   <comment lines>
#   <initial-chunk baseline>
#   <total-bundle baseline>
# If only one number is present, treat it as the initial-chunk
# baseline and leave the total-bundle gate disabled (informational).
baselines=()
if [[ -f "${BASELINE_FILE}" ]]; then
  while IFS= read -r line; do
    [[ "${line}" =~ ^[[:space:]]*# ]] && continue
    [[ -z "${line// /}" ]] && continue
    baselines+=("$(echo "${line}" | tr -d ' ')")
  done < "${BASELINE_FILE}"
fi
initial_baseline=${baselines[0]:-0}
total_baseline=${baselines[1]:-0}

echo ""
echo "=== gate results ==="

gate_failed=0
gate_check() {
  local label="$1"
  local measured="$2"
  local baseline="$3"
  local num="$4"
  local den="$5"

  if [[ "${baseline}" -eq 0 ]]; then
    cat <<EOF

${label}: baseline is 0 (gate disabled). To enable, commit the
current measurement to ${BASELINE_FILE#${REPO_ROOT}/}:

  ${measured}

EOF
    return 0
  fi

  local budget=$(( baseline * num / den ))
  local delta=$(( measured - baseline ))
  local pct=$(( delta * 100 / baseline ))

  echo "${label}: measured=${measured} baseline=${baseline} budget=${budget} delta=${delta} (${pct}%)"

  if [[ "${measured}" -gt "${budget}" ]]; then
    echo "::error::${label} grew above the budget (measured=${measured} baseline=${baseline} budget=${budget})"
    gate_failed=1
  fi
}

gate_check "initial-chunk" "${initial_bytes}" "${initial_baseline}" "${INITIAL_TOLERANCE_NUMERATOR}" "${INITIAL_TOLERANCE_DENOMINATOR}"
gate_check "total-bundle"  "${total_bytes}"   "${total_baseline}"   "${TOTAL_TOLERANCE_NUMERATOR}"   "${TOTAL_TOLERANCE_DENOMINATOR}"

if [[ "${gate_failed}" -eq 1 ]]; then
  exit 1
fi

echo ""
echo "OK: bundle within budget."
