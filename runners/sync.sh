#!/usr/bin/env bash
# sync.sh — push the canonical runner config from this repo to mimir.
# Syncs the compose project files, the host-side scripts, and versions.env
# into /opt/runners.
#
# Usage:
#   ./sync.sh                # sync files
#   ./sync.sh --rebuild      # sync, then rebuild image and recreate containers
#
# The host is fixed to `mimir`. Override with HOST=...
#
# The systemd units that RUN those scripts (ripls-runner-recreate,
# ripls-runner-recreate-idle, ripls-cleanup-orphans) are owned by the mimir
# repo under deploy/systemd/ and deployed from there — this script only ships
# the things they invoke. Before the Ubuntu migration the schedules lived in
# the Unraid User Scripts plugin, which is why this used to install each script
# into its own plugin directory.
set -euo pipefail

HOST="${HOST:-mimir}"
DEST="/opt/runners"
HERE="$(cd "$(dirname "$0")" && pwd)"

# Sync only the files we manage. Avoid --delete: the host-only .env holds
# ACCESS_TOKEN and must not be touched.
rsync -rltv \
  --include='Dockerfile' \
  --include='docker-compose.yml' \
  --include='clean-entrypoint.sh' \
  --include='recreate_runners.sh' \
  --include='cleanup_ripls_orphans.sh' \
  --include='seed_agent_workspaces.sh' \
  --exclude='*' \
  "$HERE"/ "root@$HOST:$DEST"/

# versions.env lives at the repo root, not in runners/. Ship it alongside the
# compose file so the local-rebuild fallback can read the build args from it.
rsync -ltv "$HERE/../versions.env" "root@$HOST:$DEST/versions.env"

ssh "root@$HOST" "chmod +x $DEST/recreate_runners.sh $DEST/cleanup_ripls_orphans.sh $DEST/seed_agent_workspaces.sh"

echo "Synced $HERE -> root@$HOST:$DEST"

# Warm any runner added by this compose file before --rebuild starts it. The
# recreate timers also run this, so a sync without --rebuild is covered too.
ssh "root@$HOST" "$DEST/seed_agent_workspaces.sh"

if [ "${1:-}" = "--rebuild" ]; then
  # Manual rebuild. The automated path is the build_runner_image CI workflow
  # (builds ripls-runner:latest into mimir's daemon on merge) + the scheduled
  # recreate_runners.sh; this is the on-demand equivalent. Source versions.env
  # so docker compose interpolates the build args the Dockerfile requires
  # (GO_VERSION, FLUTTER_VERSION, …).
  ssh "root@$HOST" "cd $DEST && set -a && . ./versions.env && set +a && docker compose build && docker compose up -d --force-recreate"
  echo "Rebuilt and recreated runners on $HOST."
fi
