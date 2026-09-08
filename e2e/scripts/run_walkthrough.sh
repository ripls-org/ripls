#!/usr/bin/env bash
#
# run_walkthrough.sh [reel-slug] — render one user journey walkthrough (#2684)
# HERMETICALLY: provision a throwaway Postgres testcontainer, run the reel's
# spec against a clean schema, export the deliverables, and reap the container.
# Expects nothing at startup (no pre-existing DB state), leaves nothing behind.
# Mirrors the CI Web-E2E DB lifecycle (server/cmd/web-smoke-db + an always-run
# `docker rm` reaper) rather than mutating the shared local ripls_e2e_proof DB.
#
# A reel is a spec at e2e/tests/walkthroughs/<reel-slug>.spec.ts whose tests are
# ordered SCENES (see e2e/lib/walkthrough.ts). Available reels = the files in
# that directory. Default: onboarding.
#
# Prereq (built once; rebuild when the Flutter app or server changes):
#   npm run build:e2e        # = build:web:e2e + go build -o tmp/server ./server
# The web bundle is EMBEDDED in the server binary at Go build time, so a
# rebuilt bundle changes nothing served until tmp/server is rebuilt — the
# check below (and e2e/lib/server.ts) fails fast on a stale binary.
#
# Then, from anywhere:
#   e2e/scripts/run_walkthrough.sh [reel-slug]
#
# Desktop-aspect evaluation render (#2912) — same reel at a 16:9 laptop
# viewport (1440×810), output to e2e/videos-walkthroughs/<reel>-desktop/,
# never deployed to the website:
#   WALKTHROUGH_VIEWPORT=desktop e2e/scripts/run_walkthrough.sh [reel-slug]
#
# Output (gitignored) under e2e/videos-walkthroughs/<reel-slug>/:
#   scene-NN-<name>.webm   per-scene native masters (780×1688)
#   scene-NN-<name>.mp4    per-scene H.264 clips
#   <reel-slug>.mp4        the assembled reel (H.264, scene concat)
#   frames/                QC stills from the assembled reel
set -euo pipefail
cd "$(dirname "$0")/../.." # repo root

REEL="${1:-onboarding}"
SPEC="tests/walkthroughs/${REEL}.spec.ts"

if [[ ! -f "e2e/${SPEC}" ]]; then
  echo "unknown reel '${REEL}' — no e2e/${SPEC}. Available reels:" >&2
  ls e2e/tests/walkthroughs/*.spec.ts 2>/dev/null |
    sed -E 's|.*/([^/]+)\.spec\.ts|  \1|' >&2
  exit 1
fi

if [[ ! -x tmp/server ]]; then
  echo "tmp/server not found — build it first:  npm run build:e2e" >&2
  exit 1
fi
# The Flutter Web bundle is embedded in the server binary at Go build time,
# so a binary older than the bundle serves a STALE app. Fail fast.
if [[ -n "$(find server/services/web/app_assets -newer tmp/server -print -quit 2>/dev/null)" ]]; then
  echo "tmp/server is OLDER than the web bundle (the bundle is embedded at Go build time)." >&2
  echo "Rebuild:  npm run build:e2e   (or just: go build -o tmp/server ./server)" >&2
  exit 1
fi

# One reel at a time: the stack binds a single fixed port, so a second
# concurrent run silently drives its scenes against the FIRST run's server
# and DB. That doesn't error — it renders plausible-looking clips of the
# wrong world and flakes at random locators. Fail loudly instead.
WALKTHROUGH_PORT="${E2E_PORT:-8090}"
if lsof -ti:"${WALKTHROUGH_PORT}" >/dev/null 2>&1; then
  echo "port ${WALKTHROUGH_PORT} is already in use — another reel render (or a" >&2
  echo "leftover server) is running. Reels are NOT parallel-safe: wait for it" >&2
  echo "to finish, or set E2E_PORT to a free port for this run." >&2
  exit 1
fi

# Unique container name so the reaper targets exactly this run's container.
SMOKE_DB_NAME="ripls-walkthrough-$$"
export SMOKE_DB_NAME
cleanup() { docker rm -fv "${SMOKE_DB_NAME}" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "provisioning throwaway Postgres (container ${SMOKE_DB_NAME})…"
E2E_DB_URL=$(go run ./server/cmd/web-smoke-db)
echo "db ready: ${E2E_DB_URL%%\?*}"

cd e2e
# E2E_INVITE_LINK_HOSTNAME=ripls.app: share links/QRs on camera read as the
# real product, not localhost. Reel specs navigate by short code against the
# local server, never via the display URL.
E2E_DB_URL="${E2E_DB_URL}" E2E_PORT="${E2E_PORT:-8090}" \
  E2E_INVITE_LINK_HOSTNAME="ripls.app" \
  npx playwright test "${SPEC}" --project=walkthrough

scripts/export_walkthrough.sh "${REEL}"
