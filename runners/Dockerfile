# Custom GitHub Actions runner image with all build tools pre-installed.
# Based on myoung34/github-runner which handles runner registration/lifecycle.
#
# Tools included: Go, Node.js, Flutter, JDK (Temurin), Android SDK + NDK,
#   ONNX Runtime, protoc-gen-go, protoc-gen-connect-go, golangci-lint, gofumpt,
#   buf (via npm), Git LFS, Ruby + Bundler (for Fastlane),
#   Google Chrome (web-smoke), Firebase CLI 13 (cloud-functions deploy)
#
# Tool versions are NOT hardcoded here — they are supplied as build args
# sourced from /versions.env (the single source of truth) by the build workflow
# and by runners/docker-compose.yml + sync.sh. Rebuild + republish the image
# (see #2287) after bumping a version in versions.env.

FROM myoung34/github-runner:ubuntu-noble

# Replace upstream entrypoint with a wrapper that also cleans .credentials*
# files. Without this, an in-place runner self-update leaves stale credentials
# in the writable layer; on container restart config.sh refuses with
# already configured / Value cannot be null. (configuredSettings).
COPY clean-entrypoint.sh /clean-entrypoint.sh
RUN mv /entrypoint.sh /entrypoint.upstream.sh \
    && mv /clean-entrypoint.sh /entrypoint.sh \
    && chmod +x /entrypoint.sh /entrypoint.upstream.sh

# Tool versions — supplied as build args from versions.env (no defaults on
# purpose: a build that omits them fails fast rather than baking a stale
# version). Declared after FROM so they are in scope for the RUN steps below.
ARG GO_VERSION
ARG NODE_MAJOR
ARG FLUTTER_VERSION
ARG JAVA_VERSION
ARG NDK_VERSION
ARG ONNXRUNTIME_VERSION
ARG GOLANGCI_LINT_VERSION
ARG GOFUMPT_VERSION

# Avoid interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# ─── System packages ─────────────────────────────────────────────────────────
# postgresql-client provides `psql`, which the Flutter Web e2e harness
# (e2e/lib/ephemeral-db.ts) shells out to in order to CREATE a per-worker
# ephemeral database. Without it the harness silently falls back to one shared
# DB, so the parallel Playwright workers (playwright.config.ts workers=4) start
# their servers against the same database and race on startup schema creation —
# the "server exited before /health" flake. Baking the client in restores the
# intended per-worker DB isolation.
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    git \
    git-lfs \
    unzip \
    xz-utils \
    zip \
    clang \
    cmake \
    ninja-build \
    pkg-config \
    libgtk-3-dev \
    postgresql-client \
    ruby-full \
    wget \
    ca-certificates \
    gnupg \
    sudo \
    && git lfs install \
    && rm -rf /var/lib/apt/lists/*

# ─── Go ──────────────────────────────────────────────────────────────────────
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/root/go/bin:${PATH}" \
    GOPATH="/root/go"

# Go tools used by CI
# golangci-lint and gofumpt are pinned from versions.env: `npm run lint:go` is a
# required check (#1611), so an unpinned linter lets a runner rebuild turn a
# green branch red with no repo change. protoc-gen-* stay at @latest for now —
# their output is regenerated every build, so a drift shows up as a diff rather
# than a mystery lint failure.
# NOTE: golangci-lint's module path carries its major (`/v2/cmd/...`), so a
# future major bump means editing this line too — the version alone is not
# enough. setup-toolchain's `go-lint` probe covers the window between a
# versions.env bump and this image being rebuilt and rolled (#3008).
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
    && go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest \
    && go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v${GOLANGCI_LINT_VERSION}" \
    && go install "mvdan.cc/gofumpt@v${GOFUMPT_VERSION}"

# ─── Node.js ─────────────────────────────────────────────────────────────────
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# ─── JDK (Temurin) ──────────────────────────────────────────────────────────
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       wget apt-transport-https \
    && wget -qO - https://packages.adoptium.net/artifactory/api/gpg/key/public \
       | gpg --dearmor -o /usr/share/keyrings/adoptium.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/adoptium.gpg] https://packages.adoptium.net/artifactory/deb noble main" \
       > /etc/apt/sources.list.d/adoptium.list \
    && apt-get update \
    # Install the real libasound2 (libasound2t64) explicitly alongside Temurin.
    # temurin-N-jdk only Depends on the *virtual* `libasound2`; left to apt,
    # that gets satisfied by the liboss4-salsa-asound2 OSS shim. Jobs that
    # install Chrome must drop that shim (it conflicts with Chrome's real
    # libasound2), and removing it cascades into removing the JDK — silently
    # breaking every later Android build on the reused runner (#2246). Binding
    # Temurin to the real libasound2t64 up front means the shim is never the
    # JDK's provider, so dropping it can't take the JDK with it.
    && apt-get install -y --no-install-recommends temurin-${JAVA_VERSION}-jdk libasound2t64 \
    && rm -rf /var/lib/apt/lists/* \
    # Pin a stable symlink to whatever path the package actually installed the
    # JDK at, derived from the binary itself. Hardcoding /usr/lib/jvm/temurin-N-
    # jdk-<arch> drifts when the arch suffix or package layout changes, and the
    # mismatch only surfaces at release time when Gradle validates JAVA_HOME.
    && ln -sfn "$(dirname "$(dirname "$(readlink -f "$(command -v javac)")")")" /usr/lib/jvm/temurin-current \
    && test -x /usr/lib/jvm/temurin-current/bin/java
ENV JAVA_HOME="/usr/lib/jvm/temurin-current"
ENV PATH="${JAVA_HOME}/bin:${PATH}"

# ─── Google Chrome (web-smoke browser automation) ───────────────────────────
# Baked in so test_web_smoke.yaml needs no runtime apt install. Chrome's real
# libasound2 dependency is already satisfied by the libasound2t64 installed
# with the JDK above, so no OSS-shim juggling is needed (#2246). Installs the
# `google-chrome` binary on PATH for chromedp.
RUN wget -qO - https://dl.google.com/linux/linux_signing_key.pub \
       | gpg --dearmor -o /usr/share/keyrings/google-chrome.gpg \
    && echo "deb [arch=amd64 signed-by=/usr/share/keyrings/google-chrome.gpg] https://dl.google.com/linux/chrome/deb/ stable main" \
       > /etc/apt/sources.list.d/google-chrome.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends google-chrome-stable \
    && rm -rf /var/lib/apt/lists/* \
    && google-chrome --version

# ─── Firebase CLI (cloud-functions deploy) ──────────────────────────────────
# Baked globally (npm prefix /usr → /usr/bin/firebase) so it stays on PATH even
# after deploy_cloud_functions.yaml's actions/setup-node step, which would
# shadow a Node-version-scoped global. Removes the runtime `npm install -g`.
#
# Pinned to @15 (>=15.22.3): on Node 24 the WIF/ADC credential exchange at
# sts.googleapis.com trips the Node 24.17.0 `Premature close` bug
# (nodejs/node#63989). firebase-tools <=14 is node-fetch based and surfaces it
# as the misleading "Failed to authenticate, have you run firebase login?",
# breaking prod deploys (firebase/firebase-tools#10726). firebase-tools 15.22.3
# dropped node-fetch (firebase/firebase-tools#10717), so it deploys cleanly
# regardless of the runner's Node 24 patch level. Do NOT downgrade below 15.
RUN npm install -g firebase-tools@15 \
    && firebase --version

# ─── Android SDK + NDK ──────────────────────────────────────────────────────
# Only the SDK bootstrap (cmdline-tools), platform-tools, and the NDK are baked
# here. The Android Gradle Plugin auto-downloads the build-tools, the compileSdk
# platform, and cmake a build needs at build time — driven by
# flutter.compileSdkVersion and the plugins' native (CMake) requirements — so
# those versions are NOT pinned in the image; they track the build config
# automatically (licenses are accepted below so auto-download is non-interactive).
# NDK stays pre-baked from versions.env (build-arg) to avoid a large per-build
# download; cmdline-tools is just the package-manager bootstrap, not a build
# toolchain version.
ENV ANDROID_HOME="/opt/android-sdk" \
    ANDROID_SDK_ROOT="/opt/android-sdk"
ENV PATH="${ANDROID_HOME}/cmdline-tools/latest/bin:${ANDROID_HOME}/platform-tools:${PATH}"

RUN mkdir -p ${ANDROID_HOME}/cmdline-tools \
    && cd /tmp \
    && wget -q "https://dl.google.com/android/repository/commandlinetools-linux-11076708_latest.zip" \
       -O cmdline-tools.zip \
    && unzip -q cmdline-tools.zip -d ${ANDROID_HOME}/cmdline-tools \
    && mv ${ANDROID_HOME}/cmdline-tools/cmdline-tools ${ANDROID_HOME}/cmdline-tools/latest \
    && rm cmdline-tools.zip \
    && (yes || true) | sdkmanager --licenses > /dev/null 2>&1 \
    && sdkmanager "platform-tools" \
    && sdkmanager "ndk;${NDK_VERSION}"

# ─── Flutter ─────────────────────────────────────────────────────────────────
ENV FLUTTER_HOME="/opt/flutter"
ENV PATH="${FLUTTER_HOME}/bin:${FLUTTER_HOME}/bin/cache/dart-sdk/bin:${PATH}"

RUN git clone --depth 1 --branch ${FLUTTER_VERSION} \
      https://github.com/flutter/flutter.git ${FLUTTER_HOME} \
    && flutter precache --linux --android \
    && flutter config --no-analytics \
    && dart --disable-analytics

# ─── ONNX Runtime ────────────────────────────────────────────────────────────
RUN wget -q "https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}.tgz" \
      -O /tmp/onnxruntime.tgz \
    && tar -xzf /tmp/onnxruntime.tgz -C /tmp \
    && cp /tmp/onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}/lib/libonnxruntime.so.${ONNXRUNTIME_VERSION} /usr/lib/ \
    && ln -sf /usr/lib/libonnxruntime.so.${ONNXRUNTIME_VERSION} /usr/lib/libonnxruntime.so \
    && ldconfig \
    && rm -rf /tmp/onnxruntime*

# ─── Ruby / Bundler (for Fastlane) ──────────────────────────────────────────
RUN gem install bundler --no-document

# ─── Smoke test ──────────────────────────────────────────────────────────────
RUN go version \
    && node --version \
    && flutter --version \
    && test -x "${JAVA_HOME}/bin/java" \
    && "${JAVA_HOME}/bin/java" -version \
    && sdkmanager --version \
    && ruby --version \
    && golangci-lint --version \
    && protoc-gen-go --version \
    && google-chrome --version \
    && firebase --version \
    && echo "All tools installed successfully"
