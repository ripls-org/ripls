#!/usr/bin/env bash
# build_number.sh — print the store build number for HEAD: the git commit count
# plus BUILD_NUMBER_OFFSET from versions.env.
#
# Both stores require each upload's build number to be higher than the last.
# The commit count gives that for free within one history, and the offset
# carries it across a move to a new history (#3228): set it once, at the move,
# to more than the last uploaded build. release_app.yaml and the Fastfile both
# call this, so the number they label and the number they upload agree.
#
#   scripts/build_number.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

OFFSET="$(sed -n 's/^BUILD_NUMBER_OFFSET=//p' "$ROOT/versions.env")"
if [[ ! "$OFFSET" =~ ^[0-9]+$ ]]; then
  echo "build_number.sh: versions.env must set BUILD_NUMBER_OFFSET to a non-negative integer, got '${OFFSET}'" >&2
  exit 1
fi

COUNT="$(git -C "$ROOT" rev-list --count HEAD)"
echo $((COUNT + OFFSET))
