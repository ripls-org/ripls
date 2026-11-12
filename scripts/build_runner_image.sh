#!/usr/bin/env bash
# build_runner_image.sh — build the self-hosted-linux runner image, supplying
# every toolchain version the runner Dockerfile declares from versions.env.
#
#   bash scripts/build_runner_image.sh <image-tag>
#
# Used by both builders of that image so they cannot drift apart:
#   - build_runner_image.yaml     (merge to main -> ripls-runner:latest)
#   - validate_runner_candidate.yaml (per-PR -> ripls-runner:pr-<N>)
#
# The build-arg list is DERIVED from the Dockerfile's bare `ARG NAME` lines
# rather than hardcoded here. Those bare ARGs are exactly the values that must
# come from versions.env (check_versions.js enforces "bare ARG, no literal" for
# each baked tool), so adding a tool to the Dockerfile automatically feeds it
# from versions.env in every builder. Hardcoded lists are what broke #2883: the
# lint toolchain added in #1611 was wired into one workflow but not the other,
# and the candidate build died on `golangci-lint@v` — an empty version.
#
# An ARG with no value in versions.env is a hard error here, not an empty
# --build-arg: Docker would happily bake `@v` or an empty install path and fail
# far from the cause (or, worse, succeed with a broken tool).

set -euo pipefail

tag="${1:-}"
if [[ -z "$tag" ]]; then
  echo "usage: $0 <image-tag>" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

set -a
# shellcheck disable=SC1091
. versions.env
set +a

build_args=()
resolved=()
missing=""
while read -r name; do
  [[ -n "$name" ]] || continue
  value="${!name:-}"
  if [[ -z "$value" ]]; then
    missing="$missing $name"
    continue
  fi
  build_args+=(--build-arg "$name=$value")
  resolved+=("$name=$value")
done < <(sed -n 's/^ARG[[:space:]]\{1,\}\([A-Z0-9_]\{1,\}\)[[:space:]]*$/\1/p' runners/Dockerfile)

if [[ -n "$missing" ]]; then
  echo "build_runner_image FAILED: runners/Dockerfile declares build arg(s) with no value in versions.env:$missing" >&2
  echo "Add them to versions.env (with a '# renovate:' annotation) or give the ARG a default in the Dockerfile." >&2
  exit 1
fi

if [[ ${#build_args[@]} -eq 0 ]]; then
  echo "build_runner_image FAILED: no bare 'ARG NAME' lines found in runners/Dockerfile — did the file change shape?" >&2
  exit 1
fi

echo "Building $tag from versions.env with:"
printf '  %s\n' "${resolved[@]}"

docker build \
  -f runners/Dockerfile \
  -t "$tag" \
  "${build_args[@]}" \
  runners
