#!/usr/bin/env bash
# sync.sh — push this repo's host-side infra to mimir, under /opt/ripls.
#
#   infra/sync.sh
#
# Ships only the files listed below, at the same relative paths. The host runs
# them from there:
#   web-nginx-reload/  run by the mimir repo's ripls-nginx-reload unit
#   server/            the server stacks; CI runs server/deploy.sh over SSH
#
# Each server stack's .env is a symlink to its host-only secret file in
# /etc/nas/stack-env/, mimir's convention; this creates the link if missing.
#
# What this does NOT ship, and where it lives instead:
#   - systemd units and Caddy site entries: the mimir repo (/opt/nas), which owns
#     everything the host's own tooling discovers
#   - staging-web/ and go-vanity/: copied into the mimir repo's stacks/, the
#     pattern those two already follow
#   - secrets: host-only, under /etc/nas; never written from here
#
# No --delete, so a file dropped from the list stays on the host until removed
# by hand. The host is fixed to `mimir`; override with HOST=...
set -euo pipefail

HOST="${HOST:-mimir}"
DEST=/opt/ripls
HERE="$(cd "$(dirname "$0")" && pwd)"

FILES=(
  web-nginx-reload/reload-on-change.sh
  server/common.yaml
  server/container-events.sh
  server/create_reader.sh
  server/deploy.sh
  server/migrate_db.sh
  server/fluent-bit/fluent-bit.yaml
  server/ripls_server_dev/docker-compose.yaml
  server/ripls_server_prod/docker-compose.yaml
)
EXECUTABLES=(
  web-nginx-reload/reload-on-change.sh
  server/container-events.sh
  server/create_reader.sh
  server/deploy.sh
  server/migrate_db.sh
)
STACKS=(ripls_server_dev ripls_server_prod)

ssh "root@$HOST" "mkdir -p $DEST"
(cd "$HERE" && rsync -Rltv "${FILES[@]}" "root@$HOST:$DEST/")
ssh "root@$HOST" "cd $DEST && chmod 0755 ${EXECUTABLES[*]}"

for stack in "${STACKS[@]}"; do
  ssh "root@$HOST" "ln -sfn /etc/nas/stack-env/$stack.env $DEST/server/$stack/.env"
done

echo "Synced to root@$HOST:$DEST"
