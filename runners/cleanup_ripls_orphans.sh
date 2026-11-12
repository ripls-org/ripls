#!/bin/bash
#
# Hourly sweep of leaked CI test infrastructure on mimir (self-hosted runner host).
#
# Removes:
#   1. ripls-test-postgres-* containers older than POSTGRES_THRESHOLD_HOURS.
#      Source: pgvector containers spun up by server/storage/testing.go for
#      `go test`. The CI workflows have a `cleanup-test-postgres` post-step,
#      but it doesn't fire when the runner container restarts mid-job, the
#      job is cancelled, or mimir reboots. The 6h threshold is well past any
#      legitimate test timeout (longest is 20m).
#
#   2. buildx_buildkit_builder-* containers that aren't referenced by any
#      currently-registered buildx instance. setup-buildx-action@v4 creates
#      these with restart=unless-stopped, so abandoned ones survive every
#      docker daemon restart and accumulate forever.
#
#   3. Dangling buildx_buildkit_*_state volumes. Each builder container owns a
#      *named* `<container>_state` volume; the `docker rm -fv` in step 2 only
#      deletes *anonymous* volumes, so the named one is orphaned (referenced by
#      no container) and survives forever. These had accumulated to ~15GB.
#
#   4. Buildkit build cache above BUILD_CACHE_MAX_GB. The cache is unbounded by
#      default and had grown to ~97GB of CI image-build layers, filling the
#      pool Docker's data-root sits on. We keep the most-recently-used cache up
#      to the cap and prune the rest; buildkit never evicts cache an in-flight
#      build is using.
#
# Steps 3 and 4 are why /var/lib/docker grows without bound: see
# the host runbook ("Host reliability & operations" / the cache layout; kept
# privately).
#
# Run hourly by the ripls-cleanup-orphans systemd timer (owned by the mimir
# repo) from /opt/runners/cleanup_ripls_orphans.sh. Source of truth lives in
# this repo at runners/cleanup_ripls_orphans.sh; mimir's copy is just a deploy.

set -uo pipefail

POSTGRES_THRESHOLD_HOURS=6
BUILDX_THRESHOLD_HOURS=1
# Soft cap on retained buildkit build cache. Bounds /var/lib/docker growth on
# the `fast` mirror; large enough to keep CI image builds warm across branches.
BUILD_CACHE_MAX_GB=30
LOG_FILE=/var/log/cleanup_ripls_orphans.log

log() {
    printf '%s %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$*" | tee -a "$LOG_FILE"
}

now_epoch=$(date +%s)
postgres_cutoff=$((now_epoch - POSTGRES_THRESHOLD_HOURS * 3600))
buildx_cutoff=$((now_epoch - BUILDX_THRESHOLD_HOURS * 3600))

# 1. Stale ripls-test-postgres-* containers (any state).
removed_pg=0
while IFS=$'\t' read -r id name created_iso; do
    [ -z "$id" ] && continue
    # docker {{.CreatedAt}} is e.g. "2026-04-27 18:57:04 -0500 CDT". GNU date
    # on Slackware can't parse the trailing tz abbreviation; the numeric offset
    # alone is enough.
    created_epoch=$(date -d "${created_iso% [A-Z][A-Z][A-Z]}" +%s 2>/dev/null) || {
        log "WARN could not parse created_at='$created_iso' for $name; skipping"
        continue
    }
    if [ "$created_epoch" -lt "$postgres_cutoff" ]; then
        if docker rm -fv "$id" >/dev/null 2>&1; then
            log "removed stale postgres testcontainer name=$name created=$created_iso"
            removed_pg=$((removed_pg + 1))
        else
            log "FAILED to remove postgres testcontainer name=$name id=$id"
        fi
    fi
done < <(docker ps -a \
    --filter 'name=ripls-test-postgres-' \
    --format '{{.ID}}	{{.Names}}	{{.CreatedAt}}')

# 2. Orphan buildx builders.
# `docker buildx ls --format '{{.Name}}'` lists active builder instance names
# (one per line, with node lines indented). Container names follow the pattern
# buildx_buildkit_<instance><node-index>, where <instance> is e.g.
# 'builder-8ced0690-575c-4b9e-979d-834c638f60e4' and <node-index> is a single
# digit (typically 0). A buildx container with an instance name not present in
# the active set is an orphan.
removed_bx=0
if active_raw=$(docker buildx ls --format '{{.Name}}' 2>/dev/null); then
    # Strip leading whitespace, '*' suffix on the active builder, blank lines,
    # and the literal "default" / "NAME" header.
    active_instances=$(printf '%s\n' "$active_raw" \
        | sed 's/^[[:space:]]*//; s/\*$//' \
        | grep -vE '^(NAME|default|)$' \
        | sort -u)

    while IFS=$'\t' read -r id name created_iso; do
        [ -z "$id" ] && continue
        # docker {{.CreatedAt}} is e.g. "2026-04-27 18:57:04 -0500 CDT". GNU date
    # on Slackware can't parse the trailing tz abbreviation; the numeric offset
    # alone is enough.
    created_epoch=$(date -d "${created_iso% [A-Z][A-Z][A-Z]}" +%s 2>/dev/null) || {
        log "WARN could not parse created_at='$created_iso' for $name; skipping"
        continue
    }
        if [ "$created_epoch" -ge "$buildx_cutoff" ]; then
            continue  # too young — could be an active build's builder
        fi
        # Container name → instance name: strip prefix, strip trailing single digit.
        instance="${name#buildx_buildkit_}"
        instance="${instance%[0-9]}"
        if printf '%s\n' "$active_instances" | grep -qx "$instance"; then
            continue  # instance is currently registered with buildx
        fi
        if docker rm -fv "$id" >/dev/null 2>&1; then
            log "removed orphan buildx builder name=$name created=$created_iso"
            removed_bx=$((removed_bx + 1))
        else
            log "FAILED to remove buildx builder name=$name id=$id"
        fi
    done < <(docker ps -a \
        --filter 'name=buildx_buildkit_builder-' \
        --format '{{.ID}}	{{.Names}}	{{.CreatedAt}}')
else
    log "WARN docker buildx ls failed; skipping buildx sweep"
fi

# 3. Orphan buildkit state volumes.
# `docker rm -fv` in step 2 only removes *anonymous* volumes, so each builder's
# named `<container>_state` volume is left dangling (no container references it).
# Sweep any buildx_buildkit_* volume that no container uses. Both filters are
# required: dangling=true is the orphan signal, name= scopes it to buildkit so a
# co-tenant's idle volume is never touched.
removed_vol=0
while IFS= read -r vol; do
    [ -z "$vol" ] && continue
    if docker volume rm "$vol" >/dev/null 2>&1; then
        log "removed orphan buildkit state volume name=$vol"
        removed_vol=$((removed_vol + 1))
    else
        log "FAILED to remove buildkit state volume name=$vol"
    fi
done < <(docker volume ls \
    --filter 'dangling=true' \
    --filter 'name=buildx_buildkit_' \
    --format '{{.Name}}')

# 4. Cap buildkit build cache at BUILD_CACHE_MAX_GB.
# --max-used-space keeps the most-recently-used cache up to the cap and prunes
# the rest; a no-op (fast) when already under the cap, and buildkit never evicts
# cache an in-flight build is using, so this is safe to run mid-build. Bytes
# (not "30GB") to sidestep any unit-parsing differences across docker versions.
build_cache_max_bytes=$((BUILD_CACHE_MAX_GB * 1024 * 1024 * 1024))
if prune_out=$(docker builder prune -f --max-used-space "$build_cache_max_bytes" 2>&1); then
    reclaimed=$(printf '%s\n' "$prune_out" | grep -iE '^Total:' | head -1 | tr -s '[:space:]' ' ')
    log "capped build cache at ${BUILD_CACHE_MAX_GB}GB${reclaimed:+ ($reclaimed)}"
else
    log "WARN docker builder prune failed: $(printf '%s' "$prune_out" | tail -1)"
fi

log "done removed_postgres=$removed_pg removed_buildx=$removed_bx removed_volumes=$removed_vol"
