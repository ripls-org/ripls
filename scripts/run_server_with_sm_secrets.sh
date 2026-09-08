#!/usr/bin/env bash
# run_server_with_sm_secrets.sh — Local-dev wrapper that fetches the
# 10 server-side credentials from Google Cloud Secret Manager and
# execs the server binary with each value passed as a --flag argument.
#
# Mirrors what server/entrypoint.sh does in Cloud Run, but using gcloud
# ADC (the developer's `gcloud auth application-default login` identity)
# instead of the metadata server. Keeps the server binary hermetic on
# the local-dev side too.
#
# Usage:
#   scripts/run_server_with_sm_secrets.sh <project-id> <server-binary> [server-args...]
#   scripts/run_server_with_sm_secrets.sh dev tmp/server --log-level=debug --dev-mode ...
#
# Prerequisites:
#   - gcloud auth application-default login
#   - The signed-in user must have roles/secretmanager.secretAccessor on
#     each of the 10 secrets in the named project.
#
# Fail-fast: any fetch failure aborts before the server starts.

set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <project-id> <server-binary> [server-args...]" >&2
  exit 2
fi

PROJECT="$1"
SERVER_BIN="$2"
shift 2

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Preflight: both gcloud auth contexts are required. They're noop-fast when
# fresh (print-access-token refreshes silently within the token's validity
# window) and emit a clear error otherwise. See CLAUDE.md § Running the
# Local Server for the symptom-to-command mapping.
#
# - `gcloud auth login` → CLI auth, used by `gcloud secrets versions access`
#   for the secret fetches below.
# - `gcloud auth application-default login` → ADC, used by the server
#   process for Vertex AI / Gemini and Firebase Admin SDK calls.
if ! gcloud auth print-access-token >/dev/null 2>&1; then
  echo "preflight: gcloud CLI auth is missing or expired." >&2
  echo "  Fix: gcloud auth login" >&2
  echo "  Reason: needed by 'gcloud secrets versions access' for the Secret Manager fetches below." >&2
  exit 1
fi
if ! gcloud auth application-default print-access-token >/dev/null 2>&1; then
  echo "preflight: gcloud Application Default Credentials are missing or expired." >&2
  echo "  Fix: gcloud auth application-default login" >&2
  echo "  Reason: needed by the running server for Vertex AI / Gemini and Firebase Admin SDK." >&2
  exit 1
fi

# Fetch credentials in parallel — Secret Manager has no batch-access
# RPC, so we launch one gcloud per secret and wait. Sequentially this takes
# ~1–3s on cold ADC; parallel collapses to the slowest single fetch.
# Required secrets fail the script if missing; optional secrets fall
# back to an empty file (feature disabled).
#
# Each fetched value lands in $TMPDIR/<name> and is passed to the server
# via -<name>-file=... — the bytes never appear in the server's argv. See
# server/secretsflag/README.md.
#
# Cleanup: the EXIT trap covers early-exit paths (set -e, fetch failure).
# On the successful path we exec the server, which replaces this shell —
# the trap does not fire, so the $TMPDIR contents survive until the server
# exits. macOS clears /tmp on reboot so the leak is bounded; for any
# longer-lived host wipe the dir manually if the server is killed.
TMPDIR=$(mktemp -d)
chmod 0700 "$TMPDIR"
trap 'rm -rf "$TMPDIR"' EXIT

REQUIRED_SECRETS=(
  openai-api-key
  anthropic-api-key
  mailgun-api-key
  unsplash-access-key
  pexels-api-key
  pixabay-api-key
  mapbox-access-token
  jwt-signing-secret
  github-app-private-key-base64
)

# OPTIONAL_SECRETS may be missing in dev (no GSM entry yet); the
# corresponding feature stays disabled in the server. Fetch failure
# writes an empty file rather than exiting non-zero.
OPTIONAL_SECRETS=(
  google-maps-api-key-server
)

pids=()
for name in "${REQUIRED_SECRETS[@]}"; do
  "$SCRIPT_DIR/fetch_secret.sh" "$PROJECT" "$name" > "$TMPDIR/$name" &
  pids+=("$!")
done

failed=0
for pid in "${pids[@]}"; do
  wait "$pid" || failed=1
done
if [[ $failed -ne 0 ]]; then
  echo "run_server_with_sm_secrets: one or more required secret fetches failed (see stderr above)" >&2
  exit 1
fi

# Optional secrets run after the required pass so a missing one doesn't
# tie up the parallel batch. Each missing optional just writes an
# empty file.
for name in "${OPTIONAL_SECRETS[@]}"; do
  if ! "$SCRIPT_DIR/fetch_secret.sh" "$PROJECT" "$name" > "$TMPDIR/$name" 2>/dev/null; then
    : > "$TMPDIR/$name"
    echo "run_server_with_sm_secrets: optional secret '$name' not present in $PROJECT; feature disabled" >&2
  fi
done

# Lock down perms before exec; the server only needs read access.
chmod 0400 "$TMPDIR"/*

# Drop the EXIT trap so the dir survives the exec — the server reads each
# file at startup. Files share the lifetime of the parent shell session.
trap - EXIT

exec "$SERVER_BIN" \
  "$@" \
  --openai-api-key-file="$TMPDIR/openai-api-key" \
  --anthropic-api-key-file="$TMPDIR/anthropic-api-key" \
  --mailgun-api-key-file="$TMPDIR/mailgun-api-key" \
  --unsplash-access-key-file="$TMPDIR/unsplash-access-key" \
  --pexels-api-key-file="$TMPDIR/pexels-api-key" \
  --pixabay-api-key-file="$TMPDIR/pixabay-api-key" \
  --mapbox-access-token-file="$TMPDIR/mapbox-access-token" \
  --google-maps-api-key-server-file="$TMPDIR/google-maps-api-key-server" \
  --jwt-signing-secret-file="$TMPDIR/jwt-signing-secret" \
  --github-app-private-key-file="$TMPDIR/github-app-private-key-base64"
