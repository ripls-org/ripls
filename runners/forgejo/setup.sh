#!/usr/bin/env bash
# setup.sh — runs ON THE HOST, from /opt/forgejo-runners. sync.sh ships it and
# invokes it; run it by hand only to repair a runner.
#
#   setup.sh --pending       print the numbers of runners not yet registered
#   setup.sh                 render configs, register (token on stdin), start
#
# A registration token is read from stdin only when some runner still needs
# one, is handed straight to `forgejo-runner register`, and is never written
# to disk or printed.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
STATE="${STATE:-/var/cache/nas/forgejo-runner}"
INSTANCE="${INSTANCE:-https://code.overt.org}"
COMPOSE="$HERE/docker-compose.yaml"
JOB_IMAGE="ripls-runner:latest"

# Labels per runner. `ubuntu-latest` is here because Forgejo has no hosted
# runners: the workflows that ask for it land on this fleet too.
# `android-builder` caps concurrent Android release builds at two, as on the
# GitHub fleet (see ../docker-compose.yml, runner-4).
labels_for() {
  case "$1" in
    1) echo "self-hosted-linux ubuntu-latest" ;;
    *) echo "self-hosted-linux ubuntu-latest android-builder" ;;
  esac
}

runner_numbers() {
  sed -n 's/^  forgejo-runner-\([0-9][0-9]*\):$/\1/p' "$COMPOSE"
}

runner_image() {
  sed -n 's/^  image: \(code\.forgejo\.org\/forgejo\/runner:.*\)$/\1/p' "$COMPOSE" | head -1
}

pending() {
  local n
  for n in $(runner_numbers); do
    [ -f "$STATE/runner-$n/data/.runner" ] || echo "$n"
  done
}

render_config() {
  local n="$1" label list=""
  for label in $(labels_for "$n"); do
    list="${list:+$list, }\"$label:docker://$JOB_IMAGE\""
  done
  mkdir -p "$STATE/runner-$n/data" "$STATE/runner-$n/gradle"
  sed -e "s|@N@|$n|g" -e "s|@LABELS@|$list|" "$HERE/config.template.yaml" \
    >"$STATE/runner-$n/data/config.yaml"
}

if [ "${1:-}" = "--pending" ]; then
  pending
  exit 0
fi

for n in $(runner_numbers); do
  render_config "$n"
done

todo="$(pending)"
if [ -n "$todo" ]; then
  IFS= read -r token || true
  if [ -z "${token:-}" ]; then
    echo "setup.sh: runners $(echo "$todo" | tr '\n' ' ')need registering, but no token arrived on stdin" >&2
    exit 1
  fi
  image="$(runner_image)"
  for n in $todo; do
    # The token travels in the environment of a short-lived container, not on
    # a command line that `ps` or the shell history could show.
    REGISTRATION_TOKEN="$token" docker run --rm --user 0:0 \
      -e REGISTRATION_TOKEN -v "$STATE/runner-$n/data:/data" -w /data \
      --entrypoint sh "$image" -c \
      'forgejo-runner register --no-interactive --config /data/config.yaml \
         --instance "$0" --name "$1" --token "$REGISTRATION_TOKEN" >/dev/null' \
      "$INSTANCE" "mimir-forgejo-$n"
    echo "registered mimir-forgejo-$n"
  done
  unset token
fi

docker compose -f "$COMPOSE" up -d --remove-orphans
