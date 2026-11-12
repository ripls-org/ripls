#!/usr/bin/env bash
# sync.sh — push the self-hosted Renovate runtime files from this repo to mimir.
#
# Deploys:
#   - config.js         -> $APPDIR (admin config, no secrets)
#   - mint_app_token.js -> $APPDIR (App-token minter)
#
# It deploys NO entrypoint. The script that actually fires Renovate is
# /opt/nas/scripts/run-renovate.sh, owned by the mimir repo and run by the
# run-renovate systemd timer. This repo used to ship its own run_renovate.sh
# into the Unraid User Scripts plugin; that copy diverged from the live one and
# was deleted with the Ubuntu migration.
#
# It does NOT touch the host-only credentials, which live in /etc/nas/ rather
# than beside these files — $APPDIR sits inside the restic backup set and the
# secrets deliberately do not. The image itself is built by the
# build_renovate_image CI workflow into mimir's daemon; this script only ships
# the host-side glue. See renovate/README.md and the host runbook (kept privately).
#
# Usage: ./sync.sh           (host defaults to mimir; override with HOST=...)
set -euo pipefail

HOST="${HOST:-mimir}"
APPDIR="${RENOVATE_APPDIR:-/var/lib/nas/ripls_renovate}"
HERE="$(cd "$(dirname "$0")" && pwd)"

# Admin config + token minter live on persistent appdata next to the host-only
# credentials the cron reads.
ssh "root@$HOST" "mkdir -p '$APPDIR/logs'"
rsync -ltv "$HERE/config.js"         "root@$HOST:$APPDIR/config.js"
rsync -ltv "$HERE/mint_app_token.js" "root@$HOST:$APPDIR/mint_app_token.js"

echo "Synced Renovate runtime -> root@$HOST:$APPDIR"
echo "Reminder: the credentials in /etc/nas/ are host-only and must exist."
