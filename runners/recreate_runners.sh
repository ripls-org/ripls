#!/bin/bash
# recreate_runners.sh — recreate the self-hosted-linux runner fleet onto the
# current ripls-runner:latest image.
#
# The image is built + tagged in mimir's local Docker daemon by the
# build_runner_image CI workflow (on merge to main, using GitHub Actions' auth —
# no host credential, no registry). This script only RECREATES: no git, no
# build, no auth. It runs on mimir from a systemd timer
# (source-of-truth in repo; deployed by sync.sh to /opt/runners).
#
# Two modes:
#
#   recreate_runners.sh                 # full fleet (DAILY BACKSTOP)
#       `docker compose up -d --no-build` recreates every container whose image
#       changed (i.e. after CI rebuilt ripls-runner:latest). Running as host
#       cron — NOT inside a runner — it can roll the whole fleet without killing
#       its own job. Caveat: a recreate while a runner is busy (image changed
#       mid-job) kills that job; low-risk at the scheduled hour, and a killed job
#       reruns. This is the guaranteed catch-all for runners that were busy when
#       the prompt pass ran.
#
#   recreate_runners.sh --idle-only     # prompt pass (SHORT INTERVAL)
#       Rolls only runners that are (a) on a stale image and (b) IDLE (no live
#       Runner.Worker), one at a time with `up -d --no-build --no-deps`. Busy
#       runners are skipped and picked up on a later pass (or the daily backstop)
#       once idle. This lets idle capacity pick up a new toolchain within minutes
#       of a merge without killing in-flight jobs — and it self-skips the
#       build_runner_image job's own runner, which is busy building. It is cheap
#       and idempotent when nothing is stale (just image-id inspects), so it is
#       safe to schedule frequently as the "image changed" watcher (#2395).
#
# Schedules live in systemd timers owned by the mimir repo
# (deploy/systemd/ripls-runner-recreate{,-idle}.timer), not here: a daily full
# pass at 04:40 plus a short-interval `--idle-only` pass. Those units pass the
# flag directly, which is why there is no separate idle wrapper script — the
# Unraid User Scripts plugin could not pass arguments, and needed one. See
# docs/infra/dependency_automation.md and runners/README.md.

set -euo pipefail

# The compose project name MUST equal the deploy directory's name: `docker
# compose -p` matches on that, and a mismatched -p makes `down` a silent no-op.
PROJECT_DIR="${PROJECT_DIR:-/opt/runners}"
PROJECT_NAME="${PROJECT_NAME:-runners}"
LOG="${LOG:-/var/log/ripls-runner-recreate.log}"

exec >>"$LOG" 2>&1

cd "$PROJECT_DIR"

# Before any container comes up, so a new runner's first agent job finds a full
# clone instead of fetching ripls from scratch. Best-effort: a seeding failure
# must not stop the fleet from rolling.
./seed_agent_workspaces.sh || echo "[$(date '+%F %T')] seed_agent_workspaces.sh failed; continuing"

# Roll only stale + idle runners, one service at a time. Skips busy runners
# (live Runner.Worker) so in-flight jobs aren't killed, and skips runners
# already on the latest image so a frequent schedule is a cheap no-op.
recreate_idle_only() {
  local latest_id
  latest_id=$(docker image inspect -f '{{.Id}}' ripls-runner:latest 2>/dev/null) || {
    echo "[$(date '+%F %T')] idle pass: ripls-runner:latest not found; nothing to do"
    return 0
  }

  local rolled=0 busy=0 uptodate=0 created=0
  local svc cid cur_id
  while read -r svc; do
    [ -z "$svc" ] && continue
    cid=$(docker compose -p "$PROJECT_NAME" ps -q "$svc" 2>/dev/null || true)
    if [ -z "$cid" ]; then
      # Not running — bring it up on the current image.
      docker compose -p "$PROJECT_NAME" up -d --no-build --no-deps "$svc"
      created=$((created + 1))
      continue
    fi
    cur_id=$(docker inspect -f '{{.Image}}' "$cid" 2>/dev/null || true)
    if [ "$cur_id" = "$latest_id" ]; then
      uptodate=$((uptodate + 1))
      continue
    fi
    # Stale image. Recreate only if idle (no Runner.Worker process).
    if docker exec "$cid" pgrep -f Runner.Worker >/dev/null 2>&1; then
      echo "[$(date '+%F %T')] idle pass: skip busy $svc"
      busy=$((busy + 1))
      continue
    fi
    echo "[$(date '+%F %T')] idle pass: rolling idle stale $svc"
    docker compose -p "$PROJECT_NAME" up -d --no-build --no-deps "$svc"
    rolled=$((rolled + 1))
  done < <(docker compose -p "$PROJECT_NAME" config --services | grep '^github-runner-')

  echo "[$(date '+%F %T')] idle pass done: rolled=$rolled created=$created busy=$busy uptodate=$uptodate"
}

case "${1:-}" in
  --idle-only)
    recreate_idle_only
    ;;
  "")
    echo "[$(date '+%F %T')] full recreate start"
    # --no-build: use the CI-built image; error loudly if it's somehow absent
    # rather than trying to build without the versions.env build-args.
    docker compose -p "$PROJECT_NAME" up -d --no-build
    echo "[$(date '+%F %T')] full recreate done"
    ;;
  *)
    echo "usage: $0 [--idle-only]" >&2
    exit 2
    ;;
esac

# Remove dangling images freed by the recreate (old ripls-runner layers, pruned
# pr-<N> candidate tags). Only unreferenced images go; runners still on the old
# image (skipped as busy) keep it pinned until they roll.
docker image prune -f
