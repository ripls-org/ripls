#!/bin/sh
# entrypoint.sh — Container startup wrapper for the Ripls server.
#
# Collects the server's credentials into a private directory, one file per
# credential, and execs the server with a -<name>-file flag for each. This
# keeps the server binary hermetic: it never talks to a secret store, and any
# operator can supply the same files by any means (see docs/secrets.md).
#
# Credentials come from one of two places:
#
#   Secret Manager (default). /app/fetch-secrets reads each secret in
#   $GOOGLE_CLOUD_PROJECT with Application Default Credentials: the metadata
#   server on Cloud Run, or a service-account key named by
#   $GOOGLE_APPLICATION_CREDENTIALS on any other host.
#
#   A directory ($SECRETS_FROM_DIR). Each regular file in it is one credential,
#   named after its flag — a file called jwt-signing-secret becomes
#   -jwt-signing-secret-file. Nothing is fetched and no cloud account is needed;
#   a credential with no file leaves its feature unconfigured.
#
# Fail-fast: any failure aborts under `set -e`, with no retries. On Cloud Run
# the revision is marked unhealthy and rolled back; under Docker the restart
# policy tries again.
#
# Arguments given to the container (docker `command:`, Cloud Run `args`) are
# passed to the server after everything derived here, so they win: Go's flag
# package keeps the last value of a repeated flag.

set -eu

# Rebuilt on every start. Cloud Run gives each instance a fresh /tmp, but a
# restarted Docker container keeps its filesystem, and the 0400 files left by
# the previous start could not be rewritten in place. On Cloud Run /tmp is an
# in-memory tmpfs; elsewhere, mount a tmpfs at /tmp to keep these off disk.
SECRETS_DIR=/tmp/secrets
rm -rf "$SECRETS_DIR"
umask 077
mkdir -m 0700 "$SECRETS_DIR"

if [ -n "${SECRETS_FROM_DIR:-}" ]; then
  if [ ! -d "$SECRETS_FROM_DIR" ]; then
    echo "entrypoint: SECRETS_FROM_DIR=$SECRETS_FROM_DIR is not a directory" >&2
    exit 1
  fi
  # Dotfiles are skipped by the glob, which also skips the ..data bookkeeping
  # of a Kubernetes secret mount; cp follows its symlinks to the real files.
  for f in "$SECRETS_FROM_DIR"/*; do
    [ -f "$f" ] || continue
    cp "$f" "$SECRETS_DIR/$(basename "$f")"
  done
else
  if [ -z "${GOOGLE_CLOUD_PROJECT:-}" ]; then
    cat >&2 <<EOF
entrypoint: no credential source configured. Set either
  GOOGLE_CLOUD_PROJECT  to fetch credentials from Secret Manager in that project
                        (with GOOGLE_APPLICATION_CREDENTIALS outside Google Cloud), or
  SECRETS_FROM_DIR      to read them from a directory of files named after their
                        flags, e.g. jwt-signing-secret.
See docs/secrets.md.
EOF
    exit 1
  fi
  # Optional secrets degrade instead of failing startup; absent, each leaves
  # one feature unwired:
  #   mailgun-webhook-signing-key  /email/status rejects every delivery webhook,
  #                                losing delivery metrics (#2862)
  #   google-maps-api-key-server   the Google Maps client is not constructed
  #   twilio-*                     the SMS channel is never constructed (#2492)
  #   feedback-signer-key          feedback screenshot URLs fall back to
  #                                short-lived signing (#2549)
  /app/fetch-secrets -project "$GOOGLE_CLOUD_PROJECT" -dir "$SECRETS_DIR" \
    -optional mailgun-webhook-signing-key \
    -optional google-maps-api-key-server \
    -optional twilio-account-sid \
    -optional twilio-auth-token \
    -optional twilio-messaging-service-sid \
    -optional feedback-signer-key \
    openai-api-key \
    anthropic-api-key \
    mailgun-api-key \
    unsplash-access-key \
    pexels-api-key \
    pixabay-api-key \
    mapbox-access-token \
    jwt-signing-secret \
    github-app-private-key=github-app-private-key-base64
fi

# The database password may arrive as an environment variable (Cloud Run's
# secret_key_ref, a compose env_file); it takes precedence over a db-password
# file. It is written to the directory and dropped from the environment, so it
# reaches the server as a file and not through /proc/<pid>/environ.
if [ -n "${DB_PASSWORD:-}" ]; then
  printf '%s' "$DB_PASSWORD" > "$SECRETS_DIR/db-password"
fi
unset DB_PASSWORD

: "${DB_HOST:?entrypoint: DB_HOST must be set}"
: "${DB_USER:?entrypoint: DB_USER must be set}"
: "${DB_NAME:?entrypoint: DB_NAME must be set}"

# The container's own arguments are at the front of "$@". Everything below is
# appended after them, and the loop at the end moves them back to the end. The
# list is built in positional parameters rather than a string because values
# may contain spaces (MAILGUN_FROM_ADDRESS="Ripls <noreply@...>"); splitting
# one into two tokens would end Go's flag parsing and drop every later flag.
container_args=$#

set -- "$@" \
  "-db=postgres://$DB_USER@$DB_HOST:${DB_PORT:-5432}/$DB_NAME?sslmode=${DB_SSLMODE:-require}" \
  "-port=${PORT:-8080}" \
  "-notification-provider=${NOTIFICATION_PROVIDER:-fcm}" \
  -embedding-model-path=/app/models/ripls_embedding.onnx \
  -embedding-vocab-path=/app/models/vocab.txt

[ "${ENABLE_DEV_MODE:-}" = "true" ]      && set -- "$@" -dev-mode
[ -n "${GOOGLE_CLOUD_PROJECT:-}" ]       && set -- "$@" "-vertex-ai-project=$GOOGLE_CLOUD_PROJECT"
[ -n "${GOOGLE_CLOUD_LOCATION:-}" ]      && set -- "$@" "-vertex-ai-location=$GOOGLE_CLOUD_LOCATION"
[ -n "${GCS_MEDIA_BUCKET:-}" ]           && set -- "$@" "-gcs-bucket=$GCS_MEDIA_BUCKET"
[ -n "${LOCAL_MEDIA_STORAGE:-}" ]        && set -- "$@" "-local-media-storage=$LOCAL_MEDIA_STORAGE"
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
# secrets. Only the API key is a credential.
[ -n "${MAILGUN_DOMAIN:-}" ]             && set -- "$@" "-mailgun-domain=$MAILGUN_DOMAIN"
[ -n "${MAILGUN_FROM_ADDRESS:-}" ]       && set -- "$@" "-mailgun-from=$MAILGUN_FROM_ADDRESS"
[ -n "${MAILGUN_POSTAL_ADDRESS:-}" ]     && set -- "$@" "-mailgun-postal-address=$MAILGUN_POSTAL_ADDRESS"

# Branding (server/branding). The deployment's identity (legal entity, support
# address, policy and store URLs, Android package) defaults to empty, and
# consumers then render nothing: no store badge, no legal footer. Unset here
# means the flag's default.
[ -n "${BRAND_NAME:-}" ]                 && set -- "$@" "-brand-name=$BRAND_NAME"
[ -n "${LEGAL_ENTITY_NAME:-}" ]          && set -- "$@" "-legal-entity-name=$LEGAL_ENTITY_NAME"
[ -n "${SUPPORT_EMAIL:-}" ]              && set -- "$@" "-support-email=$SUPPORT_EMAIL"
[ -n "${PRIVACY_POLICY_URL:-}" ]         && set -- "$@" "-privacy-policy-url=$PRIVACY_POLICY_URL"
[ -n "${TERMS_URL:-}" ]                  && set -- "$@" "-terms-url=$TERMS_URL"
[ -n "${APP_STORE_URL:-}" ]              && set -- "$@" "-app-store-url=$APP_STORE_URL"
[ -n "${PLAY_STORE_URL:-}" ]             && set -- "$@" "-play-store-url=$PLAY_STORE_URL"
[ -n "${ANDROID_PACKAGE_ID:-}" ]         && set -- "$@" "-android-package-id=$ANDROID_PACKAGE_ID"
[ -n "${DEEP_LINK_SCHEME:-}" ]           && set -- "$@" "-deep-link-scheme=$DEEP_LINK_SCHEME"
[ -n "${BOT_USER_AGENT:-}" ]             && set -- "$@" "-bot-user-agent=$BOT_USER_AGENT"

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

# One -<name>-file flag per credential file. A name that is not a flag stem is
# refused here rather than left for the server's "flag provided but not
# defined", which would not say where the stray file came from.
for f in "$SECRETS_DIR"/*; do
  [ -f "$f" ] || continue
  name=$(basename "$f")
  case "$name" in
    -*|*[!a-z0-9-]*)
      echo "entrypoint: $name is not a credential name (lowercase letters, digits and dashes, named after its flag)" >&2
      exit 1
      ;;
  esac
  chmod 0400 "$f"
  set -- "$@" "-$name-file=$f"
done

# Move the container's own arguments from the front to the end.
while [ "$container_args" -gt 0 ]; do
  set -- "$@" "$1"
  shift
  container_args=$((container_args - 1))
done

exec ./server "$@"
