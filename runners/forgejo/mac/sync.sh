#!/usr/bin/env bash
# sync.sh — push the Forgejo runner setup to the Mac that builds iOS, and
# start it.
#
# Usage:
#   MAC_HOST=user@host ./sync.sh
#
# The runner registers at the organization named by FORGEJO_ORG (default
# ripls-org), next to the Linux fleet (see ../sync.sh).
#
# Registering needs a one-time token from the Forgejo API. For that this
# script reads FORGEJO_URL and FORGEJO_TOKEN from the environment, or from the
# file FORGEJO_ENV_FILE (default ~/.forgejo.env) when they are not set; the
# token needs `write:organization`. With the runner already registered it
# needs neither.
set -euo pipefail

ENV_FILE="${FORGEJO_ENV_FILE:-$HOME/.forgejo.env}"
if [ -z "${FORGEJO_TOKEN:-}" ] && [ -f "$ENV_FILE" ]; then
  # shellcheck disable=SC1090
  . "$ENV_FILE"
fi

: "${MAC_HOST:?set MAC_HOST to the build Mac, as user@host}"
DEST="forgejo-runner/setup"
ORG="${FORGEJO_ORG:-ripls-org}"
HERE="$(cd "$(dirname "$0")" && pwd)"

ssh "$MAC_HOST" "mkdir -p $DEST"
rsync -rltv \
  --include='config.template.yaml' \
  --include='org.ripls.forgejo-runner.plist' \
  --include='org.ripls.runner-prune.plist' \
  --include='setup.sh' \
  --include='prune.sh' \
  --exclude='*' \
  "$HERE"/ "$MAC_HOST:$DEST"/
ssh "$MAC_HOST" "chmod +x $DEST/setup.sh $DEST/prune.sh"

if ssh "$MAC_HOST" "$DEST/setup.sh --pending"; then
  : "${FORGEJO_URL:?set FORGEJO_URL to register the runner}"
  : "${FORGEJO_TOKEN:?set FORGEJO_TOKEN to register the runner}"
  # Straight from the API into the Mac's stdin: the token is never in a
  # variable that could be echoed, an argument list here or a file.
  curl -fsS -H "Authorization: token $FORGEJO_TOKEN" \
    "$FORGEJO_URL/api/v1/orgs/$ORG/actions/runners/registration-token" |
    sed -n 's/.*"token":"\([^"]*\)".*/\1/p' |
    ssh "$MAC_HOST" "INSTANCE='$FORGEJO_URL' $DEST/setup.sh"
else
  ssh "$MAC_HOST" "$DEST/setup.sh" </dev/null
fi

echo "Synced $HERE -> $MAC_HOST:$DEST"
