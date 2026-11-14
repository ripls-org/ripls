#!/usr/bin/env bash
# install_hook.sh — install pre-receive-gitleaks for one repository on the
# Forgejo instance.
#
# Usage:
#   FORGEJO_HOST=root@host ./install_hook.sh <owner>/<repo>
#   FORGEJO_HOST=root@host ./install_hook.sh --remove <owner>/<repo>
#
# Forgejo runs in a container on that host (CONTAINER, default `forgejo`, as
# the user CONTAINER_USER, default `git`). Two files go into its data volume,
# so neither needs a new image or a restart, and both are in the instance's
# backups:
#
#   <data>/custom/bin/gitleaks                         the scanner, one copy
#                                                      for every repository
#   <data>/git/repositories/<owner>/<repo>.git/hooks/pre-receive.d/gitleaks
#                                                      the hook
#
# Forgejo's own pre-receive (in its global hooks directory) runs every
# executable in a repository's hooks/pre-receive.d, so this hook applies to
# that one repository only.
#
# The gitleaks release is the one the Actions scan pins
# (.github/actions/gitleaks-scan); its archive is checked against the
# release's published checksums before it is unpacked. Running this again
# updates both files.
set -euo pipefail

: "${FORGEJO_HOST:?set FORGEJO_HOST to the machine running Forgejo, as user@host}"
CONTAINER="${CONTAINER:-forgejo}"
CONTAINER_USER="${CONTAINER_USER:-git}"
DATA="${FORGEJO_DATA:-/var/lib/gitea}"
VERSION="${GITLEAKS_VERSION:-8.30.1}"
HERE="$(cd "$(dirname "$0")" && pwd)"

remove=false
if [ "${1:-}" = "--remove" ]; then
  remove=true
  shift
fi
REPO="${1:?usage: install_hook.sh [--remove] <owner>/<repo>}"
case "$REPO" in
  */*/* | /* | */ | *..* | *[!A-Za-z0-9._/-]*) echo "install_hook.sh: '$REPO' is not <owner>/<repo>" >&2; exit 2 ;;
  */*) ;;
  *) echo "install_hook.sh: '$REPO' is not <owner>/<repo>" >&2; exit 2 ;;
esac

# The repository directory is lower-case on disk whatever the display name.
repo_dir="$DATA/git/repositories/$(printf '%s' "$REPO" | tr '[:upper:]' '[:lower:]').git"
hook="$repo_dir/hooks/pre-receive.d/gitleaks"
bin="$DATA/custom/bin/gitleaks"

# in_container <script>: run a shell script inside the container; stdin passes
# through.
in_container() {
  # shellcheck disable=SC2029  # the script is meant to expand here
  ssh "$FORGEJO_HOST" "docker exec -i -u '$CONTAINER_USER' '$CONTAINER' sh -c $(printf '%q' "$1")"
}

in_container "test -d '$repo_dir'" </dev/null ||
  { echo "install_hook.sh: no repository at $repo_dir in container $CONTAINER" >&2; exit 1; }

if $remove; then
  in_container "rm -f '$hook'" </dev/null
  echo "Removed the hook from $REPO. The gitleaks binary stays at $bin."
  exit 0
fi

arch="$(in_container 'uname -m' </dev/null)"
case "$arch" in
  x86_64) asset="gitleaks_${VERSION}_linux_x64.tar.gz" ;;
  aarch64) asset="gitleaks_${VERSION}_linux_arm64.tar.gz" ;;
  *) echo "install_hook.sh: unsupported architecture $arch" >&2; exit 1 ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
base="https://github.com/gitleaks/gitleaks/releases/download/v${VERSION}"
curl -fsSL --retry 3 -o "$tmp/$asset" "$base/$asset"
curl -fsSL --retry 3 -o "$tmp/checksums.txt" "$base/gitleaks_${VERSION}_checksums.txt"
want="$(awk -v f="$asset" '$2 == f { print $1 }' "$tmp/checksums.txt")"
got="$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')"
if [ -z "$want" ] || [ "$want" != "$got" ]; then
  echo "install_hook.sh: checksum mismatch for $asset; not installing" >&2
  exit 1
fi
tar -xzf "$tmp/$asset" -C "$tmp" gitleaks

# Each file is written beside its destination and renamed into place, so a
# push never meets a half-written scanner or hook. The scanner goes first: the
# hook refuses every push while its scanner is missing.
in_container "mkdir -p '$(dirname "$bin")' && cat >'$bin.new' && chmod 755 '$bin.new' && mv '$bin.new' '$bin' && '$bin' version" <"$tmp/gitleaks"
in_container "mkdir -p '$(dirname "$hook")' && cat >'$hook.new' && chmod 755 '$hook.new' && mv '$hook.new' '$hook'" <"$HERE/pre-receive-gitleaks"

echo "Installed gitleaks $VERSION and the pre-receive hook for $REPO."
