#!/usr/bin/env bash
# prune.sh — runs ON THE MAC, nightly, from the LaunchAgent
# org.ripls.runner-prune (setup.sh installs it). Keeps the disk from filling
# with things the runners download and never remove.
#
#   prune.sh              prune
#   prune.sh --dry-run    print what would go, remove nothing
#
# Always, while no job is running:
#   - tool caches: each tool keeps its KEEP_VERSIONS newest versions. A
#     Flutter bump otherwise leaves a ~4 GB SDK behind for good. Covers the
#     Forgejo runner's cache and, until it is retired, the GitHub runner's.
#   - GitHub runner self-updates: every bin.<v> / externals.<v> directory the
#     `bin` and `externals` links no longer point at, and the update staging.
#
# Only when less than LOW_WATER_GB is free afterwards:
#   - package download caches (Go modules, pub, npm, SwiftPM, CocoaPods,
#     Homebrew). The next build downloads what it needs again.
#
# Everything removed here is fetched again on demand. Xcode archives,
# simulator runtimes and DerivedData are deliberately left alone.
#
# Written for the bash 3.2 that macOS ships.
set -euo pipefail

KEEP_VERSIONS="${KEEP_VERSIONS:-2}"
LOW_WATER_GB="${LOW_WATER_GB:-30}"
GITHUB_RUNNER="${GITHUB_RUNNER:-$HOME/actions-runner}"
FORGEJO_STATE="${STATE:-$HOME/forgejo-runner}"

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*"; }

# Free space on the volume holding $HOME, in whole GB. PRUNE_FREE_GB overrides
# the measurement (tests).
free_gb() {
  if [ -n "${PRUNE_FREE_GB:-}" ]; then
    echo "$PRUNE_FREE_GB"
  else
    df -Pk "$HOME" | awk 'NR==2 { print int($4 / 1048576) }'
  fi
}

# A job is running if the GitHub runner has a worker, the Forgejo daemon has
# any child process (host mode: each step is one), or a build tool is up.
# PRUNE_BUSY=0|1 overrides the check (tests).
busy() {
  if [ -n "${PRUNE_BUSY:-}" ]; then
    [ "$PRUNE_BUSY" = "1" ]
    return
  fi
  pgrep -f 'Runner\.Worker|xcodebuild|flutter_tools' >/dev/null && return 0
  local pid
  for pid in $(pgrep -f 'forgejo-runner daemon' || true); do
    pgrep -P "$pid" >/dev/null && return 0
  done
  return 1
}

remove() {
  local path="$1"
  [ -e "$path" ] || [ -L "$path" ] || return 0
  if [ "$DRY_RUN" = 1 ]; then
    log "would remove $path"
    return 0
  fi
  # The Go module cache is read-only on disk.
  chmod -R u+w "$path" 2>/dev/null || true
  rm -rf "$path"
  log "removed $path"
}

# Keep the $2 most recently installed entries of directory $1.
prune_versions() {
  local dir="$1" keep="$2" n=0 name
  [ -d "$dir" ] || return 0
  # Version directory names have no whitespace; mtime order is what we want.
  # shellcheck disable=SC2012
  ls -1t "$dir" | while IFS= read -r name; do
    [ -d "$dir/$name" ] || continue
    n=$((n + 1))
    [ "$n" -le "$keep" ] || remove "$dir/$name"
  done
}

prune_tool_cache() {
  local cache="$1" tool
  [ -d "$cache" ] || return 0
  for tool in "$cache"/*/; do
    [ -d "$tool" ] || continue
    prune_versions "${tool%/}" "$KEEP_VERSIONS"
  done
}

# Remove $GITHUB_RUNNER/$1.<version> for every version but the one the $1
# link points at. Without the link there is no telling which is live.
prune_runner_versions() {
  local kind="$1" live dir
  [ -L "$GITHUB_RUNNER/$kind" ] || return 0
  live="$(basename "$(readlink "$GITHUB_RUNNER/$kind")")"
  for dir in "$GITHUB_RUNNER/$kind".*; do
    [ -d "$dir" ] || continue
    [ "$(basename "$dir")" = "$live" ] || remove "$dir"
  done
}

if busy; then
  log "a job is running; skipped"
  exit 0
fi

log "start: $(free_gb) GB free"

prune_tool_cache "$FORGEJO_STATE/work/tool_cache"
prune_tool_cache "$GITHUB_RUNNER/_work/_tool"
prune_runner_versions bin
prune_runner_versions externals
remove "$GITHUB_RUNNER/_work/_update"

if [ "$(free_gb)" -lt "$LOW_WATER_GB" ]; then
  log "under $LOW_WATER_GB GB free; clearing download caches"
  for cache in \
    "$HOME/go/pkg/mod" \
    "$HOME/.pub-cache/hosted" \
    "$HOME/.pub-cache/hosted-hashes" \
    "$HOME/.npm/_cacache" \
    "$HOME/Library/Caches/go-build" \
    "$HOME/Library/Caches/org.swift.swiftpm" \
    "$HOME/Library/Caches/CocoaPods" \
    "$HOME/Library/Caches/Homebrew"; do
    remove "$cache"
  done
fi

free="$(free_gb)"
log "done: $free GB free"
if [ "$free" -lt "$LOW_WATER_GB" ]; then
  log "WARNING: still under $LOW_WATER_GB GB free; what is left needs a person"
fi
