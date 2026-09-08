#!/usr/bin/env bash
# runner_candidate_checks.sh — validate that a candidate runner image's baked
# toolchain (Flutter / JDK / NDK / Node / Go) can build and test this repo,
# before a baked-tool ("needs runner rebuild") Renovate PR merges.
#
# Runs INSIDE a candidate runner image, against the repo mounted at the working
# directory:
#
#   docker run --rm --volumes-from "$(hostname)" -w "$PWD" \
#     ripls-runner:pr-<N> bash scripts/runner_candidate_checks.sh
#
# Why this exists: a baked-tool bump changes the runner image, but the PR's own
# CI runs on the OLD image, so green CI does NOT prove the NEW toolchain works.
# validate_runner_candidate.yaml builds ripls-runner:pr-<N> from the PR's
# versions.env and runs this inside it to close that gap (#2395). On green it
# becomes the required check that lets these PRs auto-merge + rebuild the fleet.
#
# Anti-drift (#2395 audit finding D): where the candidate is the SOLE validator
# of a baked tool — Flutter / JDK / NDK / Node — the checks below run the SAME
# commands as the production required checks (test_flutter.yaml,
# release_build_android.yaml), so the candidate can't green-light a bump a
# production check would fail. Each block names the workflow it mirrors; keep
# them in step when those workflows change.
#
# Go is deliberately compile-only here. GOTOOLCHAIN=auto means the go.mod-pinned
# toolchain (not the baked GO_VERSION bootstrap) compiles + tests the code, so
# the production Go Tests required check already validates a Go bump on the old
# image. A full `go test` in this nested container would additionally need
# postgres testcontainers and GCP/AI secrets, which must NOT enter this
# PR-controlled, secret-free validation container (#2395 audit finding A). So we
# compile the whole tree (cheap, secret-free) and leave the runtime suite to the
# production Go Tests check.

set -euo pipefail

# flutter/build_runner/go shell out to git for versioning; the mounted workspace
# is owned by a different uid, so mark it safe to avoid "dubious ownership".
git config --global --add safe.directory '*' 2>/dev/null || true

step() { printf '\n=== [candidate-check] %s ===\n' "$*"; }

step "Baked toolchain versions in this candidate image"
go version
node --version
flutter --version
java -version
ls "${ANDROID_HOME:-/opt/android-sdk}/ndk" 2>/dev/null || echo "NDK dir not found"

# ── Code generation: Node + buf + Dart codegen ──────────────────────────────
# Shared prelude of test_flutter.yaml / release_build_android.yaml / test_go.yaml.
step "Install npm dependencies"
npm install

step "Generate code from protos"
npm run generate:proto

step "Install Flutter dependencies"
(cd app && flutter pub get)

step "Generate freezed and mockito code"
(cd app && flutter pub run build_runner build --delete-conflicting-outputs)

# ── Flutter analyze + test: the baked Flutter / Dart SDK ─────────────────────
# Mirrors test_flutter.yaml's analyze + test steps, including the profiler
# workaround: without it this gate inherits the same ~60%-per-run flutter_tester
# crash rate (#2933) and would block every future baked-tool bump. Parity with
# test_flutter.yaml is preserved — both sides run the shim.
step "Analyze Flutter code"
(cd app && flutter analyze)

step "Disable the flutter_tester Dart profiler (issue #2933)"
bash scripts/disable_flutter_tester_profiler.sh

step "Run Flutter tests"
(cd app && flutter test --reporter expanded)

# ── Android release build: JDK + NDK + Gradle + R8 ───────────────────────────
# Mirrors release_build_android.yaml — the toolchain-sensitive gate the PR's own
# CI cannot exercise, since that build runs on the old image.
step "Create local environment config for the release build"
# The client id only has to be well-formed here: this build is a toolchain
# gate (does R8/ProGuard succeed), never a signed artifact anyone signs into.
# Set GOOGLE_CLIENT_ID to exercise a real one.
printf '{"ENVIRONMENT":"local","LOG_LEVEL":"INFO","ENABLE_NOTIFICATIONS":"true","GOOGLE_CLIENT_ID":"%s","CACHE_TTL_MINUTES":"5"}\n' \
  "${GOOGLE_CLIENT_ID:-000000000000-placeholder.apps.googleusercontent.com}" > app/env.local.json

step "Verify Android release build (R8/ProGuard)"
(cd app && flutter build apk --release --dart-define-from-file=env.local.json)

# ── Go compile: the baked Go bootstrap fetches + runs the go.mod toolchain ───
# Compile-only by design (see header). Build the whole tree, including
# integration-tagged files, so a toolchain regression that only the integration
# build surfaces is still caught (CLAUDE.md: build with -tags=integration on
# cross-package changes).
step "Compile Go (all packages)"
go build ./...

step "Compile Go (integration build tag)"
go build -tags=integration ./...

step "All candidate-image checks passed"
