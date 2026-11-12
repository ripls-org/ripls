#!/bin/bash
# reload-on-change.sh — reload the static-site nginx containers when their
# mounted config or content changes.
#
# Why this exists: the website deploy pipeline rsyncs files into place but
# cannot reload nginx. The deploy SSH endpoint is a restricted `rsync-server`
# container with no docker socket, so it can only write files. Without a reload,
# an nginx container keeps serving its in-memory config — which is exactly how a
# deploy that changed config (or deleted a file the running config depended on)
# can take the site down until someone reloads by hand.
#
# This host-side watcher closes that gap: it polls the config + content dirs of
# each managed site and reloads the matching container within INTERVAL seconds
# of any change. It reloads ONLY after `nginx -t` passes, so a broken config is
# never applied — the container keeps serving its last-good config and the
# failure is logged.
#
# Install: runs on mimir (the host, which has docker) as the mimir repo's
# ripls-nginx-reload unit, from /opt/ripls/web-nginx-reload/ where
# infra/sync.sh ships it. The canonical copy lives here. See README.md in this
# directory.

INTERVAL="${INTERVAL:-15}"
STAMPDIR=/var/run/nginx-reload
mkdir -p "$STAMPDIR"

CONTAINERS="static_web_type2fun static_web_type2fun_staging"

# Host dirs (config + content) to watch per container.
watch_dirs() {
  case "$1" in
    static_web_type2fun)
      echo "/srv/sites/type2fun_static_web/config /srv/sites/type2fun_static_web/content" ;;
    static_web_type2fun_staging)
      echo "/srv/sites/type2fun_static_web_staging/config /srv/sites/type2fun_static_web_staging/sites" ;;
  esac
}

log() { echo "$(date -Is) [nginx-reload] $*"; }

log "watcher started (interval=${INTERVAL}s)"
while true; do
  for c in $CONTAINERS; do
    stamp="$STAMPDIR/$c"
    dirs=$(watch_dirs "$c")

    if [ ! -f "$stamp" ]; then
      changed="init"
    else
      changed=$(find $dirs -type f -newer "$stamp" 2>/dev/null | head -n1)
    fi
    [ -z "$changed" ] && continue

    # Skip silently if the container isn't up yet (e.g. early boot).
    docker exec "$c" true 2>/dev/null || continue

    if docker exec "$c" nginx -t >/dev/null 2>&1; then
      if docker exec "$c" nginx -s reload >/dev/null 2>&1; then
        log "reloaded $c (trigger: ${changed})"
      else
        log "ERROR: reload failed for $c"
      fi
    else
      log "SKIP $c: nginx -t failed — keeping last-good config (trigger: ${changed})"
    fi
    # Stamp in both cases: a later fix rewrites the file (newer mtime) and is
    # re-detected, so we converge without re-logging the same failure each tick.
    touch "$stamp"
  done
  sleep "$INTERVAL"
done
