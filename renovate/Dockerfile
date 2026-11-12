# Self-hosted Renovate image with just enough Go toolchain to regenerate the
# protobuf Go packages before `go mod tidy`.
#
# Why this exists: generated code (server/gen/**) is NOT checked in, so a bare
# `go mod tidy` / `go get` cannot resolve the in-module imports of the generated
# packages and fails ("cannot find module providing package
# go.ripls.org/ripls/server/gen/..."). The hosted Mend app hit exactly this and
# left go.mod-only PRs with a stale go.sum. Self-hosting lets a postUpgradeTask
# run `buf generate --template buf.gen.go.yaml` (local plugins, no network to
# buf.build) and then `go mod tidy`, producing a correct go.sum. See
# scripts/renovate_gomod_regen.sh and docs/toolchain_automation.md.
#
# Only the GO toolchain is baked: the Go buf template uses the local
# protoc-gen-go / protoc-gen-connect-go plugins, so Dart/TS/Flutter generation
# (which need remote plugins and the Flutter SDK) is never invoked here.
#
# Built into mimir's local daemon by .github/workflows/build_renovate_image.yaml
# (no registry, same pattern as the runner image). Renovate itself is pinned via
# the FROM tag below and bumped by the dockerfile manager (manual, like other
# base images).

FROM renovate/renovate:43

USER root

# Go version is the single-source-of-truth bootstrap from versions.env (build
# arg, no default so a build that omits it fails fast). GOTOOLCHAIN=auto means
# `go mod tidy` downloads the exact toolchain go.mod pins, so this only needs to
# be close enough to bootstrap.
ARG GO_VERSION
# buf is pinned to the repo's @bufbuild/buf (package.json) so generated code
# matches CI. Only go.sum is committed from the regen, so buf version drift is
# harmless to the committed output, but pinning keeps behaviour predictable.
ARG BUF_VERSION=1.70.0

# Go toolchain (system-wide, readable by the unprivileged runtime user).
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:/usr/local/bin:${PATH}" \
    GOTOOLCHAIN=auto

# Local protoc plugins for buf.gen.go.yaml, plus `mod` (marwan-at-work/mod) which
# Renovate's postUpdateOptions:["gomodUpdateImportPaths"] spawns to rewrite Go
# import paths on a major bump of a path-versioned module (go-github/v75 -> /v88).
# Without it Renovate logs `spawn mod ENOENT`, silently skips the import rewrite,
# and the major stays a cosmetic go.mod-only change that `go mod tidy` reverts
# (the go-github/mailgun thrash loop). Installed to /usr/local/bin (GOBIN) so they
# are on PATH for the unprivileged runtime user. @latest matches the runner image;
# tool version does not affect the committed go.sum.
RUN GOBIN=/usr/local/bin GOPATH=/tmp/gobuild GOCACHE=/tmp/gocache \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
    && GOBIN=/usr/local/bin GOPATH=/tmp/gobuild GOCACHE=/tmp/gocache \
    go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest \
    && GOBIN=/usr/local/bin GOPATH=/tmp/gobuild GOCACHE=/tmp/gocache \
    go install github.com/marwan-at-work/mod/cmd/mod@latest \
    && rm -rf /tmp/gobuild /tmp/gocache

# buf static binary (prebuilt release — far faster than building from source).
RUN curl -fsSL "https://github.com/bufbuild/buf/releases/download/v${BUF_VERSION}/buf-Linux-x86_64" \
      -o /usr/local/bin/buf \
    && chmod +x /usr/local/bin/buf

# Per-run Go caches under the renovate user's home (writable at runtime). The
# module cache is sized for repeated runs but the container is --rm each cron
# fire, so it is effectively cold each time; that is acceptable for a weekly job.
ENV GOPATH=/home/ubuntu/go \
    GOCACHE=/home/ubuntu/.cache/go-build \
    GOFLAGS=-mod=mod

# Smoke test (fails the build if any tool is missing or off PATH).
RUN go version \
    && buf --version \
    && protoc-gen-go --version \
    && protoc-gen-connect-go --version \
    && command -v mod \
    && echo "renovate image tools OK"

# Drop back to the unprivileged runtime user the base image ships (uid 12021).
USER ubuntu
