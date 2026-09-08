#!/usr/bin/env bash
# fetch_secret.sh — Print a Secret Manager secret value to stdout.
#
# Used by Flutter build wrappers (`npm run start:app:*`,
# `npm run build:app:*`) to inject the Mapbox token via --dart-define,
# and by scripts/run_server_with_sm_secrets.sh to inject server-side
# credentials. Designed for the same single-secret fetch in any context —
# keep it dumb and composable.
#
# Usage:
#   ./scripts/fetch_secret.sh <project-id|dev|prod> <secret-name>
#   ./scripts/fetch_secret.sh dev mapbox-access-token
#   ./scripts/fetch_secret.sh my-project-prod mapbox-access-token
#
# "dev" and "prod" resolve through scripts/gcp_project.sh, so callers name the
# environment rather than this deployment's project ids (#2953). A literal
# project id still works unchanged.
#
# Auth: uses whichever credentials gcloud is currently configured with —
# typically `gcloud auth application-default login` locally, or
# google-github-actions/auth@v3 in CI. The caller's identity must have
# roles/secretmanager.secretAccessor on the named secret.

set -euo pipefail
shopt -s extglob

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <project-id|dev|prod> <secret-name>" >&2
  exit 2
fi

# Resolve the environment aliases to this deployment's project ids. Anything
# else is passed through as a literal project id.
project="$1"
case "$project" in
  dev|prod) project="$("$(dirname "${BASH_SOURCE[0]}")/gcp_project.sh" "$project")" ;;
esac

# Fetch the secret, then strip any trailing whitespace before emitting it.
# Secret Manager values frequently pick up an unintended trailing newline (or
# CRLF, or stray spaces/tabs) on upload, which silently corrupts exact-match
# credentials — API keys, tokens, OAuth client IDs. ${var%%+([[:space:]])}
# removes only the trailing run, so leading whitespace and newlines embedded in
# multi-line payloads (PEM keys) survive. printf '%s' adds no trailing newline.
#
# This mirrors server/secretsflag.Resolve, which strips the same way when the
# server reads the value back — the two layers are belt-and-suspenders. Callers
# that capture via $(...) (the Flutter --dart-define wrappers) also benefit,
# since $(...) only strips trailing newlines, not trailing spaces/tabs/CR.
secret="$(gcloud secrets versions access latest --project="$project" --secret="$2")"
trimmed="${secret%%+([[:space:]])}"

# Refuse to emit an empty value. Callers typically wrap us in a $(...)
# substitution inside a longer command line; if we returned empty quietly,
# the surrounding build would proceed with a blank --dart-define and ship
# a broken binary (e.g. Firebase rejects an empty apiKey only at runtime
# in the browser). Failing here turns that class of bug into a CI failure.
if [[ -z "$trimmed" ]]; then
  echo "fetch_secret.sh: secret '$2' in project '$project' is empty after trimming whitespace" >&2
  exit 1
fi

printf '%s' "$trimmed"
