#!/usr/bin/env bash
# gcp_project.sh — Resolve an environment name to this deployment's GCP project id.
#
# Every npm script that reaches GCP used to name the project inline
# (`fetch_secret.sh my-project-dev …`), which put one deployment's identifiers in
# ~56 places in package.json and made the repo unusable by anyone else (#2953).
# Scripts now say *which environment* they mean and this resolves *whose*.
#
# Usage:
#   scripts/gcp_project.sh dev     # -> the dev project id
#   scripts/gcp_project.sh prod    # -> the prod project id
#
# Resolution order, first match wins:
#   1. GCP_PROJECT_DEV / GCP_PROJECT_PROD in the environment (what CI sets).
#   2. The same names in .env.local at the repo root (what a developer sets).
#
# .env.local is gitignored: project ids are not secrets, but they are the
# deployment's identity, and the point of this indirection is that they live
# outside version control.
#
# Exits non-zero with an actionable message when unresolved. That is
# deliberate — a caller that substituted an empty project id would produce a
# confusing gcloud error at best, and at worst read from the wrong project.

set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: scripts/gcp_project.sh <dev|prod>

Set the project ids once, either as environment variables or in .env.local at
the repo root:

    GCP_PROJECT_DEV=my-project-dev
    GCP_PROJECT_PROD=my-project-prod
EOF
  exit 2
}

[[ $# -eq 1 ]] || usage

case "$1" in
  dev)  var="GCP_PROJECT_DEV" ;;
  prod) var="GCP_PROJECT_PROD" ;;
  *)    usage ;;
esac

# Already in the environment? Use it and skip the file entirely.
value="${!var:-}"

if [[ -z "$value" ]]; then
  env_file="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.env.local"
  if [[ -f "$env_file" ]]; then
    # Read the assignment rather than sourcing the file: sourcing would execute
    # whatever else it contains, and this runs inside command substitution in
    # dozens of npm scripts.
    value="$(grep -E "^[[:space:]]*(export[[:space:]]+)?${var}=" "$env_file" \
      | tail -n 1 \
      | sed -E "s/^[[:space:]]*(export[[:space:]]+)?${var}=//" \
      | sed -E 's/^["'"'"']//; s/["'"'"']$//' \
      | tr -d '\r')"
  fi
fi

if [[ -z "$value" ]]; then
  echo "gcp_project.sh: $var is not set." >&2
  echo "  Set it in your environment or in .env.local at the repo root." >&2
  echo "  See scripts/gcp_project.sh for the resolution order." >&2
  exit 1
fi

printf '%s' "$value"
