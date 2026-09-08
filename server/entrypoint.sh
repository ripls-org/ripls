#!/bin/sh
# entrypoint.sh — Container startup wrapper for the Ripls server.
#
# Fetches credentials from Google Cloud Secret Manager using the Cloud Run
# runtime service account's identity (via the GCE metadata server) and
# execs the server with the values as plain --flag arguments. This keeps
# the server binary hermetic: it never speaks to Secret Manager, and any
# operator can supply the same flags by any means (k8s secret mount,
# systemd EnvironmentFile, local shell wrapper, …).
#
# Fail-fast: any HTTP failure on metadata-server token issuance or on a
# secret fetch causes `set -e` to abort and the container to crash. No
# retries — if SM is unreachable the right move is for Cloud Run to mark
# the revision unhealthy and roll back.
#
# Cloud Run / GKE auth: the metadata server provides a short-lived OAuth
# access token bound to the runtime SA. That SA must have
# roles/secretmanager.secretAccessor on each named secret in $GOOGLE_CLOUD_PROJECT
# (see terraform/environments/{dev,prod}/main.tf).
#
# Local dev does NOT use this script — see scripts/run_server_with_sm_secrets.sh.

set -eu

if [ -z "${GOOGLE_CLOUD_PROJECT:-}" ]; then
  cat >&2 <<EOF
entrypoint: GOOGLE_CLOUD_PROJECT must be set.

This is set by Terraform on every Cloud Run revision
(terraform/modules/containers/main.tf). If you are running this image
outside of Cloud Run / GKE you must either set GOOGLE_CLOUD_PROJECT
yourself OR use scripts/run_server_with_sm_secrets.sh on the host
instead (it fetches via gcloud ADC and execs the server binary with
the same flags).
EOF
  exit 1
fi

META="http://metadata.google.internal/computeMetadata/v1"

# Probe the metadata server. The metadata service is only reachable from
# inside Google's compute fabric (Cloud Run, GKE, GCE) and is the source
# of the runtime service account's OAuth token. A failed probe almost
# always means we are running outside Cloud Run.
if ! curl -sf --max-time 3 -o /dev/null -H "Metadata-Flavor: Google" "$META/instance/id"; then
  cat >&2 <<EOF
entrypoint: cannot reach the GCE metadata server at $META.

This script is designed to run inside Google Cloud Run (or another
GCE-hosted environment) where the metadata server is reachable and
serves short-lived OAuth tokens for the runtime service account.

Likely causes:
  1. You are running the image outside of Cloud Run (e.g. \`docker run\`
     on your laptop, or in another cloud). For local dev, use
     scripts/run_server_with_sm_secrets.sh instead — it fetches the
     same Secret Manager values via your gcloud ADC and execs the
     server binary with the same flags.
  2. Egress from the container to 169.254.169.254 is blocked by a VPC
     egress rule (rare for Cloud Run; possible for GKE with private
     networking misconfigured).
  3. DNS resolution for metadata.google.internal is broken (very rare
     inside Cloud Run).
EOF
  exit 1
fi

# Now fetch the actual access token. A failure here is unusual — it
# means the metadata server is reachable but the runtime SA can't be
# resolved or is missing.
TOKEN_RESPONSE=$(
  curl -sf -H "Metadata-Flavor: Google" \
    "$META/instance/service-accounts/default/token"
) || {
  cat >&2 <<EOF
entrypoint: metadata server is reachable but the access-token endpoint
failed at $META/instance/service-accounts/default/token.

Likely causes:
  1. The Cloud Run service has no runtime service account configured
     (very rare — Cloud Run falls back to the project's default Compute
     SA when none is set in Terraform).
  2. The runtime SA was deleted or disabled.
  3. The metadata server is rate-limiting us (extremely rare).

Inspect the service spec:
  gcloud run services describe \$SERVICE_NAME --project=\$GOOGLE_CLOUD_PROJECT \\
    --format='value(spec.template.spec.serviceAccountName)'
EOF
  exit 1
}

TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r .access_token)
if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  echo "entrypoint: metadata server returned a response but no access_token field" >&2
  echo "entrypoint: raw response (first 200 chars): $(echo "$TOKEN_RESPONSE" | head -c 200)" >&2
  exit 1
fi

# Secret tmpfs. /tmp is an in-memory tmpfs in Cloud Run by contract; the
# 0700 dir + 0400 files keep the secrets readable only by the runtime user.
# umask 077 guarantees the file creates restrictively before chmod.
SECRETS_DIR=/tmp/secrets
mkdir -p "$SECRETS_DIR"
chmod 0700 "$SECRETS_DIR"
umask 077

# fetch_secret_to_file <secret-name> <out-path> writes the decoded secret
# bytes directly to <out-path> at mode 0400. The value never lives in a
# shell variable in this script, so it never reaches the parent process's
# environment or `argv`.
#
# Returns non-zero on any fetch/decode failure; under `set -e` that aborts
# the script and the container exits unhealthy — Cloud Run rolls back.
fetch_secret_to_file() {
  fetch_secret_to_file_impl required "$1" "$2"
}

# fetch_secret_to_file_optional behaves like fetch_secret_to_file but
# writes an empty file (not an error) when the secret doesn't exist or
# the runtime SA can't access it. Use for credentials whose absence is
# tolerated by the server — the cfg field stays empty, and the
# corresponding feature is disabled.
fetch_secret_to_file_optional() {
  fetch_secret_to_file_impl optional "$1" "$2"
}

fetch_secret_to_file_impl() {
  REQUIRED="$1"
  SECRET_NAME="$2"
  OUT_PATH="$3"
  RESPONSE=$(
    curl -sf -H "Authorization: Bearer $TOKEN" \
      "https://secretmanager.googleapis.com/v1/projects/$GOOGLE_CLOUD_PROJECT/secrets/$SECRET_NAME/versions/latest:access"
  ) || {
    if [ "$REQUIRED" = "optional" ]; then
      : > "$OUT_PATH"
      chmod 0400 "$OUT_PATH"
      echo "entrypoint: optional secret '$SECRET_NAME' not available; feature disabled" >&2
      return 0
    fi
    cat >&2 <<EOF
entrypoint: failed to fetch secret '$SECRET_NAME' from project
'$GOOGLE_CLOUD_PROJECT' via Secret Manager.

Likely causes:
  1. The runtime SA does not have roles/secretmanager.secretAccessor
     on this specific secret. The for_each grant in
     terraform/environments/\$ENV/main.tf must include '$SECRET_NAME'
     in local.server_runtime_secrets — verify with:
       gcloud secrets get-iam-policy $SECRET_NAME \\
         --project=$GOOGLE_CLOUD_PROJECT
  2. The secret does not exist in $GOOGLE_CLOUD_PROJECT — verify with:
       gcloud secrets describe $SECRET_NAME --project=$GOOGLE_CLOUD_PROJECT
  3. The secret exists but has no enabled versions — verify with:
       gcloud secrets versions list $SECRET_NAME --project=$GOOGLE_CLOUD_PROJECT
EOF
    return 1
  }
  # Decode, then strip trailing whitespace before writing. Secret Manager
  # uploads frequently include an unintended trailing newline (or CRLF / stray
  # spaces) that silently corrupts exact-match credentials. The strip runs as a
  # pipe stage so the decoded value never lands in a shell variable. perl's
  # -0 slurps the whole payload as one record; s/\s+\z// removes only the
  # trailing whitespace run, so embedded newlines (multi-line PEM payloads) and
  # leading whitespace survive. Mirrors server/secretsflag.Resolve on the read
  # side. perl-base is a Debian Essential package (see Dockerfile).
  echo "$RESPONSE" | jq -r .payload.data | base64 -d | perl -0pe 's/\s+\z//' > "$OUT_PATH"
  chmod 0400 "$OUT_PATH"
  if [ ! -s "$OUT_PATH" ]; then
    if [ "$REQUIRED" = "optional" ]; then
      echo "entrypoint: optional secret '$SECRET_NAME' decoded to empty; feature disabled" >&2
      return 0
    fi
    echo "entrypoint: secret '$SECRET_NAME' decoded to an empty value (unexpected — check SM payload)." >&2
    return 1
  fi
}

fetch_secret_to_file openai-api-key                  "$SECRETS_DIR/openai-api-key"
fetch_secret_to_file anthropic-api-key               "$SECRETS_DIR/anthropic-api-key"
fetch_secret_to_file mailgun-api-key                 "$SECRETS_DIR/mailgun-api-key"
# Optional: verifies delivery-event webhooks (#2862). Absent secret → empty
# file → the server logs a startup warning and /email/status rejects every
# webhook, which loses delivery metrics but breaks nothing else. Optional so a
# deploy that lands before the secret exists still boots.
fetch_secret_to_file_optional mailgun-webhook-signing-key "$SECRETS_DIR/mailgun-webhook-signing-key"
fetch_secret_to_file unsplash-access-key             "$SECRETS_DIR/unsplash-access-key"
fetch_secret_to_file pexels-api-key                  "$SECRETS_DIR/pexels-api-key"
fetch_secret_to_file pixabay-api-key                 "$SECRETS_DIR/pixabay-api-key"
fetch_secret_to_file mapbox-access-token             "$SECRETS_DIR/mapbox-access-token"
# google-maps-api-key-server is optional until Phase 3 ships
# server/location/google.go (#2188). Absent secret → empty value → cfg
# sees empty string → Google client is not constructed. Matches the
# nil-handling pattern that mapbox-access-token follows on the dev path.
fetch_secret_to_file_optional google-maps-api-key-server "$SECRETS_DIR/google-maps-api-key-server"
# Twilio platform SMS (#2492). Optional: absent secrets → empty values → the
# SMS channel is never constructed and inbound webhook signatures are not
# verified. Create the secrets and uncomment the Terraform grant to enable.
fetch_secret_to_file_optional twilio-account-sid           "$SECRETS_DIR/twilio-account-sid"
fetch_secret_to_file_optional twilio-auth-token            "$SECRETS_DIR/twilio-auth-token"
fetch_secret_to_file_optional twilio-messaging-service-sid "$SECRETS_DIR/twilio-messaging-service-sid"
fetch_secret_to_file jwt-signing-secret              "$SECRETS_DIR/jwt-signing-secret"
fetch_secret_to_file github-app-private-key-base64   "$SECRETS_DIR/github-app-private-key"
# feedback-signer-key is optional: a GCP service-account JSON key used to sign
# durable feedback-screenshot URLs (#2549). Absent ⇒ empty file ⇒ server leaves
# the signer unset and feedback URLs fall back to short-lived signBlob signing.
fetch_secret_to_file_optional feedback-signer-key    "$SECRETS_DIR/feedback-signer-key"

# Serialize the DB password (provided as a Cloud Run secret env var) to a
# tmpfs file so it can be passed via -db-password-file instead of being
# embedded in the -db URL on the command line. The env var stays set —
# /proc/<pid>/environ is a separate surface tracked outside #1885.
if [ -n "${DB_PASSWORD:-}" ]; then
  # Strip trailing whitespace for the same reason as the SM fetches above —
  # the backing secret may carry an unintended trailing newline.
  printf '%s' "$DB_PASSWORD" | perl -0pe 's/\s+\z//' > "$SECRETS_DIR/db-password"
  chmod 0400 "$SECRETS_DIR/db-password"
fi

# Build the non-credential flag list in positional parameters (set --). We
# can't store them as a single string and splat with $FLAGS because env-var
# values may contain spaces (e.g. MAILGUN_FROM_ADDRESS="Ripls <noreply@...>").
# Word-splitting a quoted-looking string at exec time would break the value
# into two argv tokens; the second token wouldn't start with "-", which
# terminates Go's flag.Parse() and silently drops every flag after it
# (including the credential flags).
set -- -notification-provider=fcm \
       -embedding-model-path=/app/models/ripls_embedding.onnx \
       -embedding-vocab-path=/app/models/vocab.txt

[ "${ENABLE_DEV_MODE:-}" = "true" ]      && set -- "$@" -dev-mode
[ -n "${GOOGLE_CLOUD_PROJECT:-}" ]       && set -- "$@" "-vertex-ai-project=$GOOGLE_CLOUD_PROJECT"
[ -n "${GOOGLE_CLOUD_LOCATION:-}" ]      && set -- "$@" "-vertex-ai-location=$GOOGLE_CLOUD_LOCATION"
[ -n "${GCS_MEDIA_BUCKET:-}" ]           && set -- "$@" "-gcs-bucket=$GCS_MEDIA_BUCKET"
[ -n "${GITHUB_APP_ID:-}" ]              && set -- "$@" "-github-app-id=$GITHUB_APP_ID"
[ -n "${GITHUB_INSTALLATION_ID:-}" ]     && set -- "$@" "-github-installation-id=$GITHUB_INSTALLATION_ID"
# Repo the feedback bot files issues against. Required alongside the App
# credentials — without them the client can't be built and feedback silently
# stops creating issues.
[ -n "${GITHUB_REPO_OWNER:-}" ]          && set -- "$@" "-github-repo-owner=$GITHUB_REPO_OWNER"
[ -n "${GITHUB_REPO_NAME:-}" ]           && set -- "$@" "-github-repo-name=$GITHUB_REPO_NAME"
[ -n "${WAITLIST_NOTIFY_EMAIL:-}" ]      && set -- "$@" "-waitlist-notify-email=$WAITLIST_NOTIFY_EMAIL"
[ -n "${GOOGLE_CLIENT_ID:-}" ]           && set -- "$@" "-google-client-id=$GOOGLE_CLIENT_ID"
[ "${LOG_SOURCE_LOCATION:-}" = "true" ]  && set -- "$@" -log-source-location
[ -n "${INVITE_LINK_HOSTNAME:-}" ]       && set -- "$@" "-invite-link-hostname=$INVITE_LINK_HOSTNAME"
[ -n "${CORS_ALLOWED_ORIGINS:-}" ]       && set -- "$@" "-cors-allowed-origins=$CORS_ALLOWED_ORIGINS"
[ -n "${MAP_PROVIDER:-}" ]               && set -- "$@" "-map-provider=$MAP_PROVIDER"

# Mailgun routing config (domain + from) stays in env vars — they are not
# secrets. Only the API key comes from Secret Manager.
[ -n "${MAILGUN_DOMAIN:-}" ]             && set -- "$@" "-mailgun-domain=$MAILGUN_DOMAIN"
[ -n "${MAILGUN_FROM_ADDRESS:-}" ]       && set -- "$@" "-mailgun-from=$MAILGUN_FROM_ADDRESS"
[ -n "${MAILGUN_POSTAL_ADDRESS:-}" ]     && set -- "$@" "-mailgun-postal-address=$MAILGUN_POSTAL_ADDRESS"

# Off-app email channel (#2492) is gated by an env flag like the digest
# toggles. Stays false until the CAN-SPAM postal address + deliverability are
# signed off.
[ "${OFF_APP_EMAIL_ENABLED:-}" = "true" ] && set -- "$@" -off-app-email-enabled

# Platform SMS (#2492) is gated by an env flag like the digest toggles. Stays
# false until A2P registration + counsel sign-off; the Twilio credential files
# are still passed below (empty when the optional secrets are absent).
[ "${PLATFORM_SMS_ENABLED:-}" = "true" ] && set -- "$@" -platform-sms-enabled

# Activity digest (#1924). Daily and weekly cadences toggle
# independently. The recipient and timezone are shared.
[ "${ACTIVITY_DIGEST_ENABLED:-}" = "true" ]         && set -- "$@" -activity-digest-enabled
[ "${ACTIVITY_DIGEST_WEEKLY_ENABLED:-}" = "true" ]  && set -- "$@" -activity-digest-weekly-enabled
[ -n "${ACTIVITY_DIGEST_NOTIFY_EMAIL:-}" ]          && set -- "$@" "-activity-digest-notify-email=$ACTIVITY_DIGEST_NOTIFY_EMAIL"
[ -n "${ACTIVITY_DIGEST_TIMEZONE:-}" ]              && set -- "$@" "-activity-digest-timezone=$ACTIVITY_DIGEST_TIMEZONE"
[ -n "${ACTIVITY_DIGEST_SEND_HOUR:-}" ]             && set -- "$@" "-activity-digest-send-hour=$ACTIVITY_DIGEST_SEND_HOUR"
[ -n "${ACTIVITY_DIGEST_WEEKLY_WEEKDAY:-}" ]        && set -- "$@" "-activity-digest-weekly-weekday=$ACTIVITY_DIGEST_WEEKLY_WEEKDAY"
[ -n "${ACTIVITY_DIGEST_WEEKLY_SEND_HOUR:-}" ]      && set -- "$@" "-activity-digest-weekly-send-hour=$ACTIVITY_DIGEST_WEEKLY_SEND_HOUR"

exec ./server \
  "-db=postgres://$DB_USER@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=require" \
  "-db-password-file=$SECRETS_DIR/db-password" \
  "-port=$PORT" \
  "$@" \
  "-openai-api-key-file=$SECRETS_DIR/openai-api-key" \
  "-anthropic-api-key-file=$SECRETS_DIR/anthropic-api-key" \
  "-mailgun-api-key-file=$SECRETS_DIR/mailgun-api-key" \
  "-mailgun-webhook-signing-key-file=$SECRETS_DIR/mailgun-webhook-signing-key" \
  "-unsplash-access-key-file=$SECRETS_DIR/unsplash-access-key" \
  "-pexels-api-key-file=$SECRETS_DIR/pexels-api-key" \
  "-pixabay-api-key-file=$SECRETS_DIR/pixabay-api-key" \
  "-mapbox-access-token-file=$SECRETS_DIR/mapbox-access-token" \
  "-google-maps-api-key-server-file=$SECRETS_DIR/google-maps-api-key-server" \
  "-jwt-signing-secret-file=$SECRETS_DIR/jwt-signing-secret" \
  "-github-app-private-key-file=$SECRETS_DIR/github-app-private-key" \
  "-twilio-account-sid-file=$SECRETS_DIR/twilio-account-sid" \
  "-twilio-auth-token-file=$SECRETS_DIR/twilio-auth-token" \
  "-twilio-messaging-service-sid-file=$SECRETS_DIR/twilio-messaging-service-sid" \
  "-feedback-signer-key-file=$SECRETS_DIR/feedback-signer-key"
