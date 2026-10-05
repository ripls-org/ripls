# Build stage for Go server.
# GO_VERSION is sourced from versions.env by the build workflow
# (build_and_deploy_container.yaml passes --build-arg). The default here is a
# guarded mirror of versions.env (scripts/check_versions.js enforces it) so a
# plain `docker build` (e.g. test_docker_build) still uses the pinned version.
ARG GO_VERSION=1.27.1
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

# The entrypoint's Secret Manager reader (server/cmd/fetch-secrets). Pure Go.
RUN CGO_ENABLED=0 GOOS=linux go build -o fetch-secrets ./server/cmd/fetch-secrets

# Runtime stage - use Debian Trixie for glibc 2.38 compatibility (required by Go 1.25 and ONNX Runtime)
FROM debian:trixie-slim

# ONNX Runtime version. Must stay in lockstep with github.com/yalue/onnxruntime_go
# in go.mod: that binding requests an ORT C-API version the native library must
# support (1.22.x supports API <=22; binding v1.24.0 requests API 22).
ARG ONNXRUNTIME_VERSION=1.29.0

# Install runtime dependencies. ca-certificates for every outbound TLS call
# (including /app/fetch-secrets), wget for the HEALTHCHECK below, ffmpeg for
# media processing.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates wget ffmpeg \
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

# Copy the startup wrapper and its Secret Manager reader. The wrapper collects
# credentials and execs the server with -<name>-file flags, keeping the server
# binary itself cloud-agnostic (docs/secrets.md).
COPY --from=builder /app/fetch-secrets .
COPY server/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Copy embedding model files
COPY model_tuning/ripls_embedding.onnx /app/models/
COPY model_tuning/ripls_embedding_tokenizer/vocab.txt /app/models/

# /var/lib/ripls/media is where LOCAL_MEDIA_STORAGE points in
# docker-compose.yaml. It exists in the image so a named volume mounted there
# inherits appuser's ownership instead of root's.
RUN mkdir -p /var/lib/ripls/media && chown -R appuser:appuser /var/lib/ripls

# Set ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Readiness check (PORT will be set by Cloud Run, default to 8080 for local testing).
# Probes /readyz so the container is marked unhealthy when critical dependencies
# (database, storage) fail, letting the orchestrator route traffic away.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --quiet --tries=1 http://localhost:${PORT:-8080}/readyz -O /dev/null || exit 1

# Environment variables consumed by /app/entrypoint.sh (and forwarded to the
# server as flags). Credentials are not env vars, with the one exception of
# DB_PASSWORD; the entrypoint turns each into a file. server/entrypoint.sh is
# the authoritative list.
#
# Credential source — set exactly one:
#   GOOGLE_CLOUD_PROJECT - fetch credentials from Secret Manager in this project
#     with Application Default Credentials (the metadata server on Cloud Run;
#     GOOGLE_APPLICATION_CREDENTIALS naming a service-account key elsewhere).
#     Also forwarded to the server as --vertex-ai-project.
#   SECRETS_FROM_DIR - read credentials from a directory of files, each named
#     after its flag (jwt-signing-secret -> --jwt-signing-secret-file).
#
# Database:
#   DB_HOST, DB_USER, DB_NAME - required
#   DB_PORT - default 5432
#   DB_PASSWORD - optional; wins over a db-password credential file
#   DB_SSLMODE - libpq sslmode, default require
#
# Optional:
#   PORT - port to listen on, default 8080 (set automatically by Cloud Run)
#   NOTIFICATION_PROVIDER - --notification-provider, default fcm
#   ENABLE_DEV_MODE - "true" enables --dev-mode
#   GOOGLE_CLOUD_LOCATION - --vertex-ai-location (defaults to 'global')
#   GCS_MEDIA_BUCKET - --gcs-bucket
#   LOCAL_MEDIA_STORAGE - --local-media-storage (instead of GCS_MEDIA_BUCKET)
#   GITHUB_APP_ID, GITHUB_INSTALLATION_ID - feedback bot App (non-secret)
#   GITHUB_REPO_OWNER, GITHUB_REPO_NAME - repo the feedback bot files into
#   GOOGLE_CLIENT_ID - --google-client-id (OAuth client id, not a secret)
#   LOG_SOURCE_LOCATION - "true" enables --log-source-location
#   INVITE_LINK_HOSTNAME - --invite-link-hostname
#   CORS_ALLOWED_ORIGINS - --cors-allowed-origins
#   MAP_PROVIDER - --map-provider
#   MAILGUN_DOMAIN, MAILGUN_FROM_ADDRESS, MAILGUN_POSTAL_ADDRESS
#   OFF_APP_EMAIL_ENABLED, PLATFORM_SMS_ENABLED ("true")
#   ACTIVITY_DIGEST_* - daily and weekly digest schedule and recipient
#
# Arguments given to the container are passed to the server after all of the
# above, so any server flag can be set or overridden that way.
#
# Note: Embedding model is bundled in the image at /app/models/
ENTRYPOINT ["/app/entrypoint.sh"]
