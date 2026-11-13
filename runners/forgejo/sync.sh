#!/usr/bin/env bash
# sync.sh — push the Forgejo runner fleet from this repo to mimir and start it.
#
# Usage:
#   ./sync.sh
#
# The host is fixed to `mimir`; override with HOST=... . Runners register at
# the organization named by FORGEJO_ORG (default ripls-org), so every repo in
# it shares the fleet.
#
# Registering a new runner needs a one-time token from the Forgejo API. For
# that this script reads FORGEJO_URL and FORGEJO_TOKEN from the environment,
# or from the file FORGEJO_ENV_FILE (default ~/.forgejo.env) when they are not
# set; the token needs `write:organization`. With every runner already
# registered it needs neither.
set -euo pipefail

ENV_FILE="${FORGEJO_ENV_FILE:-$HOME/.forgejo.env}"
if [ -z "${FORGEJO_TOKEN:-}" ] && [ -f "$ENV_FILE" ]; then
  # shellcheck disable=SC1090
  . "$ENV_FILE"
fi

HOST="${HOST:-mimir}"
DEST="/opt/forgejo-runners"
ORG="${FORGEJO_ORG:-ripls-org}"
HERE="$(cd "$(dirname "$0")" && pwd)"

ssh "root@$HOST" "mkdir -p $DEST"
rsync -rltv \
  --include='docker-compose.yaml' \
  --include='config.template.yaml' \
  --include='setup.sh' \
  --exclude='*' \
  "$HERE"/ "root@$HOST:$DEST"/
ssh "root@$HOST" "chmod +x $DEST/setup.sh"

pending="$(ssh "root@$HOST" "$DEST/setup.sh --pending")"
if [ -z "$pending" ]; then
  ssh "root@$HOST" "$DEST/setup.sh" </dev/null
else
  : "${FORGEJO_URL:?set FORGEJO_URL to register new runners}"
  : "${FORGEJO_TOKEN:?set FORGEJO_TOKEN to register new runners}"
  # Straight from the API into the host's stdin: the token is never in a
  # variable that could be echoed, an argument list or a file.
  curl -fsS -H "Authorization: token $FORGEJO_TOKEN" \
    "$FORGEJO_URL/api/v1/orgs/$ORG/actions/runners/registration-token" |
    sed -n 's/.*"token":"\([^"]*\)".*/\1/p' |
    ssh "root@$HOST" "INSTANCE='$FORGEJO_URL' $DEST/setup.sh"
fi

echo "Synced $HERE -> root@$HOST:$DEST"
