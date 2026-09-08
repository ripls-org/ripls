# Build stage for Go server.
# GO_VERSION is sourced from versions.env by the build workflow
# (build_and_deploy_container.yaml passes --build-arg). The default here is a
# guarded mirror of versions.env (scripts/check_versions.js enforces it) so a
# plain `docker build` (e.g. test_docker_build) still uses the pinned version.
ARG GO_VERSION=1.26.6
FROM golang:${GO_VERSION} AS builder

# Install build dependencies including nodejs for buf, and curl for downloading buf
RUN apt-get update && apt-get install -y --no-install-recommends \
    git nodejs npm curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /app

# Install buf CLI
RUN curl -sSL "https://github.com/bufbuild/buf/releases/latest/download/buf-$(uname -s)-$(uname -m)" -o "/usr/local/bin/buf" && \
    chmod +x "/usr/local/bin/buf"

# Copy package files for npm dependencies
COPY package*.json ./

# Install npm dependencies (for buf)
RUN npm install

# Copy go module files
COPY go.mod go.sum ./
RUN go mod download

# Install protoc plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

# Copy proto files and buf configuration
COPY proto ./proto
COPY buf.yaml buf.gen.go.yaml ./

# Generate Go code from protos (server only needs Go, not Dart)
RUN npm run generate:proto:go

# Copy rest of source code. `website/` is deliberately NOT copied: the static
# assets the server serves live under server/services/web/static/ and are
# compiled in via //go:embed, which cannot reach outside its own package
# directory. The build stage carried `COPY website ./website` for a while after
# that move — dead weight here, and a hard build failure in the open-source
# repo, where website/ is private-bound (#2953).
COPY server ./server

# Build the server binary with CGO enabled (required for ONNX Runtime)
RUN CGO_ENABLED=1 GOOS=linux go build -a -o server ./server

# Runtime stage - use Debian Trixie for glibc 2.38 compatibility (required by Go 1.25 and ONNX Runtime)
FROM debian:trixie-slim

# ONNX Runtime version. Must stay in lockstep with github.com/yalue/onnxruntime_go
# in go.mod: that binding requests an ORT C-API version the native library must
# support (1.22.x supports API <=22; binding v1.24.0 requests API 22).
ARG ONNXRUNTIME_VERSION=1.29.0

# Install runtime dependencies.
# curl + jq are required by /app/entrypoint.sh to fetch secrets from
# Google Cloud Secret Manager via the GCE metadata server at startup
# (#1768/#1770). perl strips trailing whitespace from each decoded secret in
# that same script; perl-base is a Debian Essential package, so listing it is a
# no-op install that documents the dependency.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates wget ffmpeg curl jq perl-base \
    && rm -rf /var/lib/apt/lists/*

# Download and install ONNX Runtime
RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then \
        ONNX_ARCH="x64"; \
    elif [ "$ARCH" = "aarch64" ]; then \
        ONNX_ARCH="aarch64"; \
    else \
        echo "Unsupported architecture: $ARCH" && exit 1; \
    fi && \
    wget -q "https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/onnxruntime-linux-${ONNX_ARCH}-${ONNXRUNTIME_VERSION}.tgz" -O /tmp/onnxruntime.tgz && \
    tar -xzf /tmp/onnxruntime.tgz -C /tmp && \
    cp /tmp/onnxruntime-linux-${ONNX_ARCH}-${ONNXRUNTIME_VERSION}/lib/libonnxruntime.so.${ONNXRUNTIME_VERSION} /usr/lib/ && \
    ln -s /usr/lib/libonnxruntime.so.${ONNXRUNTIME_VERSION} /usr/lib/libonnxruntime.so && \
    rm -rf /tmp/onnxruntime*

# Create app directory and user
RUN useradd -m -s /bin/bash appuser
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/server .

# Copy startup wrapper that fetches credentials from Secret Manager and
# execs the server with --flag args (#1768/#1770). Keeps the server
# binary itself cloud-agnostic.
COPY server/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Copy embedding model files
COPY model_tuning/ripls_embedding.onnx /app/models/
COPY model_tuning/ripls_embedding_tokenizer/vocab.txt /app/models/

# Set ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Readiness check (PORT will be set by Cloud Run, default to 8080 for local testing).
# Probes /readyz so the container is marked unhealthy when critical dependencies
# (database, storage) fail, letting the orchestrator route traffic away.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --quiet --tries=1 http://localhost:${PORT:-8080}/readyz -O /dev/null || exit 1

# Environment variables consumed by /app/entrypoint.sh (and forwarded to
# the server as flags). Secrets are NOT in env vars — entrypoint.sh
# fetches each one from Secret Manager at startup using the Cloud Run
# runtime service account's metadata-server token.
#
# Required:
#   PORT - Port to listen on (set automatically by Cloud Run)
#   DB_USER, DB_PASSWORD, DB_HOST, DB_PORT, DB_NAME - Database connection details
#   GOOGLE_CLOUD_PROJECT - GCP project ID; entrypoint.sh fetches secrets
#     from Secret Manager in this project. Also forwarded to the server
#     as --vertex-ai-project.
#
# Optional:
#   ENABLE_DEV_MODE - "true" enables --dev-mode
#   GOOGLE_CLOUD_LOCATION - --vertex-ai-location (defaults to 'global')
#   GCS_MEDIA_BUCKET - --gcs-bucket
#   GITHUB_APP_ID - --github-app-id (non-secret; private key fetched from SM)
#   GITHUB_INSTALLATION_ID - --github-installation-id
#   GOOGLE_CLIENT_ID - --google-client-id (OAuth client id, not a secret)
#   LOG_SOURCE_LOCATION - "true" enables --log-source-location
#   INVITE_LINK_HOSTNAME - --invite-link-hostname
#   CORS_ALLOWED_ORIGINS - --cors-allowed-origins
#   MAILGUN_DOMAIN, MAILGUN_FROM_ADDRESS - --mailgun-domain / --mailgun-from
#   ACTIVITY_DIGEST_ENABLED ("true") - --activity-digest-enabled (daily)
#   ACTIVITY_DIGEST_WEEKLY_ENABLED ("true") - --activity-digest-weekly-enabled
#   ACTIVITY_DIGEST_NOTIFY_EMAIL - shared recipient for both digests
#   ACTIVITY_DIGEST_TIMEZONE - IANA tz the window is computed in
#   ACTIVITY_DIGEST_SEND_HOUR - 0-23, local-hour the daily fires
#   ACTIVITY_DIGEST_WEEKLY_WEEKDAY - English day name ("Monday")
#   ACTIVITY_DIGEST_WEEKLY_SEND_HOUR - 0-23, local-hour the weekly fires
#
# Secret Manager secrets fetched by entrypoint.sh (require the runtime SA
# to hold roles/secretmanager.secretAccessor on each in
# $GOOGLE_CLOUD_PROJECT):
#   openai-api-key, anthropic-api-key, mailgun-api-key,
#   unsplash-access-key, pexels-api-key, pixabay-api-key,
#   mapbox-access-token, jwt-signing-secret,
#   github-app-private-key-base64.
#
# Note: Uses FCM with Application Default Credentials (from Cloud Run service account)
# Note: Embedding model is bundled in the image at /app/models/
CMD ["/app/entrypoint.sh"]
