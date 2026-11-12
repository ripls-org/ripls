#!/bin/bash
# seed_agent_workspaces.sh — give every runner a full clone of ripls in its
# ripls-agent workspace, so no agent job pays a from-scratch clone.
#
# Agent jobs (Type2Fun-Foundation/ripls-agent, #2970) keep ripls checked out at
# the root of <work>/ripls-agent/ripls-agent with full history. Without a clone
# there, actions/checkout fetches all ~800 MB of it from GitHub, which takes
# about an hour over mimir's uplink. The work dirs are host bind mounts, so a
# clone survives container rebuilds and recreates; this only matters for a new
# runner or a wiped work dir.
#
# Two steps, neither needing a GitHub credential:
#   1. Refresh the bare seed's main from the runners' existing agent workspaces,
#      which agent jobs keep current. A stale seed is still correct, just a
#      bigger first fetch.
#   2. For each runner in docker-compose.yml with no agent workspace, clone the
#      seed there. Existing workspaces are never touched.
#
# On a fresh host the seed starts empty and step 2 does nothing; the first agent
# job's clone becomes the source for the next run. Runs from
# recreate_runners.sh (both passes) and sync.sh, before containers come up.

set -euo pipefail

BASE="${BASE:-/var/cache/nas/github-runner}"
SEED="${SEED:-$BASE/seed/ripls.git}"
COMPOSE="${COMPOSE:-/opt/runners/docker-compose.yml}"
WS_REL="ripls-agent/ripls-agent"
ORIGIN="https://github.com/Type2Fun-Foundation/ripls"

# The seed and workspaces are owned by various uids; this runs as root.
g() { git -c safe.directory='*' "$@"; }
log() { echo "[$(date '+%F %T')] seed: $*"; }

[ -d "$SEED" ] || g init -q --bare "$SEED"

# Runner work dirs, from the compose bind mounts (…/runner-N/work:/tmp/runner/work).
mapfile -t work_dirs < <(grep -oE "$BASE/runner-[0-9]+/work:" "$COMPOSE" | tr -d ':' | sort -u)

refreshed=0
for w in "${work_dirs[@]}"; do
  ws="$w/$WS_REL"
  [ -d "$ws/.git" ] || continue
  # Shallow or partial clones can't serve as a source.
  [ ! -f "$ws/.git/shallow" ] || continue
  [ -z "$(g -C "$ws" config --get remote.origin.promisor || true)" ] || continue
  g -C "$ws" rev-parse -q --verify refs/remotes/origin/main >/dev/null || continue
  if g -C "$SEED" fetch -q "$ws" '+refs/remotes/origin/main:refs/heads/main'; then
    refreshed=$((refreshed + 1))
  else
    log "could not fetch from $ws"
  fi
done

if ! g -C "$SEED" rev-parse -q --verify refs/heads/main >/dev/null; then
  log "seed has no main yet; nothing to seed (refreshed from $refreshed)"
  exit 0
fi

seeded=0
for w in "${work_dirs[@]}"; do
  ws="$w/$WS_REL"
  # The runner creates the dir at job start; an empty one is a failed first
  # job, safe to replace. Anything else belongs to a job or an earlier seed.
  if [ -d "$ws" ]; then
    rmdir "$ws" 2>/dev/null || continue
  fi
  mkdir -p "$(dirname "$ws")"
  tmp="$ws.seeding.$$"
  rm -rf "$tmp"
  if g clone -q --no-checkout "$SEED" "$tmp" &&
    g -C "$tmp" remote set-url origin "$ORIGIN" &&
    mv -T "$tmp" "$ws"; then
    log "seeded $ws"
    seeded=$((seeded + 1))
  else
    log "failed to seed $ws"
    rm -rf "$tmp"
  fi
done

log "done: refreshed from $refreshed, seeded $seeded"
