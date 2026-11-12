#!/usr/bin/env bash
# deploy.sh — roll one environment's server onto an image already in the local
# Docker daemon, and roll back if it does not become healthy.
#
#   deploy.sh <env> <tag>        e.g. deploy.sh dev dev-3482c27
#
# CI loads ripls-server:<env>-<sha> into this host's daemon, then runs this over
# SSH with a key whose authorized_keys entry forces this script; the requested
# command arrives in SSH_ORIGINAL_COMMAND. It runs on the host rather than in the
# runner because compose resolves env files and bind mounts on the host.
#
# Steps:
#   1. Point ripls-server:<env> (what the stack runs) at the requested tag,
#      keeping the image it replaces as ripls-server:<env>-previous.
#   2. Recreate the server and wait for its /readyz healthcheck.
#   3. If it never becomes healthy, point ripls-server:<env> back at the
#      previous image, recreate again, and exit non-zero.
#   4. Keep the newest KEEP <env>-<sha> tags and drop older ones.
#
# One deploy per environment at a time (flock); dev and prod do not block each
# other.
set -euo pipefail

BASE=/opt/ripls/server
IMAGE=ripls-server
KEEP=${KEEP:-10}
WAIT_TIMEOUT=${WAIT_TIMEOUT:-240}

log() { echo "deploy: $*"; }
die() { echo "deploy: $*" >&2; exit 1; }

# Arguments come from the command line, or from SSH_ORIGINAL_COMMAND when the
# script is the forced command of an SSH key. Either way they are validated
# before anything uses them.
if [ $# -eq 0 ] && [ -n "${SSH_ORIGINAL_COMMAND:-}" ]; then
  # Deliberately split on whitespace: the command is two plain words.
  # shellcheck disable=SC2086
  set -- $SSH_ORIGINAL_COMMAND
fi
[ $# -eq 2 ] || die "usage: deploy.sh <dev|prod> <env>-<git-sha>"
ENV=$1
TAG=$2
case "$ENV" in dev|prod) ;; *) die "unknown environment '$ENV'" ;; esac
[[ "$TAG" =~ ^${ENV}-[0-9a-f]{7,40}$ ]] || die "tag '$TAG' is not ${ENV}-<git-sha>"

PROJECT_DIR="$BASE/ripls_server_$ENV"
[ -f "$PROJECT_DIR/docker-compose.yaml" ] || die "no stack at $PROJECT_DIR"
cd "$PROJECT_DIR"

exec 9>"/run/lock/ripls-deploy-$ENV.lock"
flock -w 600 9 || die "another $ENV deploy held the lock for 10 minutes"

docker image inspect "$IMAGE:$TAG" >/dev/null 2>&1 \
  || die "$IMAGE:$TAG is not in the local daemon (CI loads it before calling this)"

previous=$(docker image inspect "$IMAGE:$ENV" --format '{{.Id}}' 2>/dev/null || true)
wanted=$(docker image inspect "$IMAGE:$TAG" --format '{{.Id}}')
if [ -n "$previous" ] && [ "$previous" != "$wanted" ]; then
  docker tag "$previous" "$IMAGE:$ENV-previous"
fi

log "$ENV: $IMAGE:$ENV -> $TAG"
docker tag "$IMAGE:$TAG" "$IMAGE:$ENV"

if docker compose up -d --wait --wait-timeout "$WAIT_TIMEOUT" server; then
  log "$ENV: healthy on $TAG"
else
  log "$ENV: $TAG did not become healthy; last server log lines:"
  docker compose logs --no-color --tail 40 server || true
  if [ -n "$previous" ] && [ "$previous" != "$wanted" ]; then
    log "$ENV: rolling back to the previous image"
    docker tag "$previous" "$IMAGE:$ENV"
    if docker compose up -d --wait --wait-timeout "$WAIT_TIMEOUT" server; then
      log "$ENV: rolled back and healthy"
    else
      log "$ENV: ROLLBACK ALSO UNHEALTHY — needs a person"
    fi
  else
    log "$ENV: no earlier image to roll back to"
  fi
  exit 1
fi

# Prune: newest KEEP <env>-<sha> tags survive. Untagging an image a container
# still uses is refused without -f, so the running image can never go.
docker images "$IMAGE" --format '{{.CreatedAt}}\t{{.Tag}}' \
  | grep -P "\t${ENV}-[0-9a-f]{7,40}$" | sort -r | tail -n +"$((KEEP + 1))" | cut -f2 \
  | while read -r old; do
      docker rmi "$IMAGE:$old" >/dev/null 2>&1 && log "$ENV: pruned $IMAGE:$old" || true
    done
