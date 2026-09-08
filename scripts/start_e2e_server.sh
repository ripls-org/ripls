#!/usr/bin/env bash
#
# start_e2e_server.sh — start the Ripls server for the e2e harness
# (#2162). Owns the flag set so e2e/lib/server.ts doesn't duplicate
# it in TypeScript; when the server adds a required flag, change
# happens here and nothing in TS drifts.
#
# Usage: scripts/start_e2e_server.sh [--log-format=json|text]
#
# Required env vars:
#   E2E_DB_URL    Postgres connection URL (testcontainer or local).
#
# Optional env vars:
#   E2E_PORT          Port to bind (default 8080).
#   E2E_SERVER_BIN    Path to the server binary (default tmp/server).
#                     Caller is responsible for building it.
#   E2E_LOG_LEVEL     Default info; set to debug for verbose triage.
#
# Mirrors the hermetic startup pattern used by Go integration tests
# (server/integration_tests/test_helpers.go § startTestServer) — the
# same surface Phase 2 of #2162 will reuse for multi-user scenarios.
# Specifically:
#
#   - No Secret Manager fetches. The integration-test pattern proves
#     that --dev-mode + --notification-provider=test (the default) +
#     a permissive map-provider config is enough to exercise every
#     RPC path the e2e specs hit (registration, share-link mint,
#     experience save/share, RSVP, etc.).
#   - No live third parties. --mock-ai-provider and
#     --mock-weather-provider keep every run hermetic and repeatable;
#     the weather one also frees a run to render dates the real
#     forecast horizon doesn't cover.
#   - --jwt-signing-secret is supplied inline as a fixed 64-byte
#     test string. Tokens minted under it are only valid for this
#     run; production fetches a real value via run_server_with_sm_secrets.sh.
#   - --invite-link-hostname follows the bound port so generated
#     share URLs are reachable from the same loopback the Playwright
#     browser dials (integration tests use a fixture host because
#     they assert URL strings, not navigability).
#   - --log-format defaults to text locally; CI uses json (per the
#     Phase-0 audit decision) so triagers can grep structured fields
#     like request_id when correlating a Playwright failure to its
#     server slog line.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

LOG_FORMAT="text"
for arg in "$@"; do
  case "${arg}" in
    --log-format=*) LOG_FORMAT="${arg#--log-format=}" ;;
    *)
      echo "start_e2e_server.sh: unknown arg ${arg}" >&2
      exit 2
      ;;
  esac
done

: "${E2E_DB_URL:?E2E_DB_URL must be set (Postgres connection URL)}"
PORT="${E2E_PORT:-8080}"
SERVER_BIN="${E2E_SERVER_BIN:-tmp/server}"
LOG_LEVEL="${E2E_LOG_LEVEL:-info}"
# Hostname baked into generated share links. Defaults to the loopback the
# Playwright browser dials so links stay navigable; the walkthrough reels
# override it to the real app host so links/QRs LOOK real on camera (their specs
# navigate to local paths by short code, never via the display URL).
INVITE_LINK_HOSTNAME="${E2E_INVITE_LINK_HOSTNAME:-localhost:${PORT}}"

if [[ ! -x "${SERVER_BIN}" ]]; then
  echo "start_e2e_server.sh: server binary not found at ${SERVER_BIN}" >&2
  echo "  build it first: go build -o ${SERVER_BIN} ./server" >&2
  exit 1
fi

# Phone auth runs against the Firebase Auth Emulator in e2e (WEB-2 milestone).
# When the harness boots the emulator it exports FIREBASE_AUTH_EMULATOR_HOST;
# the Firebase Admin SDK auto-detects it and validates emulator-issued ID tokens
# with no real credentials (proven by server/auth/firebase_emulator_test.go). The
# demo project id keeps the SDK in emulator/demo mode. Without the emulator env
# this is harmless: NewApp is lazy and token verification simply isn't exercised.
FIREBASE_PROJECT="${E2E_FIREBASE_PROJECT:-demo-ripls}"
if [[ -n "${FIREBASE_AUTH_EMULATOR_HOST:-}" ]]; then
  echo "start_e2e_server.sh: phone auth via Auth Emulator at ${FIREBASE_AUTH_EMULATOR_HOST} (project ${FIREBASE_PROJECT})" >&2
fi

exec "${SERVER_BIN}" \
  --log-level="${LOG_LEVEL}" \
  --log-format="${LOG_FORMAT}" \
  --port="${PORT}" \
  --db="${E2E_DB_URL}" \
  --local-media-storage=./media_storage \
  --dev-mode \
  --mock-ai-provider \
  --mock-weather-provider \
  --firebase-project="${FIREBASE_PROJECT}" \
  --embedding-model-path=./model_tuning/ripls_embedding.onnx \
  --embedding-vocab-path=./model_tuning/ripls_embedding_tokenizer/vocab.txt \
  --invite-link-hostname="${INVITE_LINK_HOSTNAME}" \
  --jwt-signing-secret="ripls-e2e-jwt-signing-secret-do-not-use-in-production-padding"
