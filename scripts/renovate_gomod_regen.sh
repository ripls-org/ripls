#!/usr/bin/env bash
# Regenerate the Go protobuf packages, then tidy go.sum — run by self-hosted
# Renovate as a postUpgradeTask on gomod updates (see renovate.json).
#
# Why: generated code (server/gen/**) is NOT checked in, so Renovate's built-in
# go.sum refresh (`go get`/`go mod tidy`) cannot resolve the in-module imports
# of the generated packages and fails ("cannot find module providing package
# go.ripls.org/ripls/server/gen/..."). That left go.mod-only PRs with a stale
# go.sum that broke every Go build. Regenerating the Go packages first (local
# buf plugins, no network to buf.build) lets `go mod tidy` resolve everything
# and write a correct go.sum.
#
# Only go.mod + go.sum are committed back (postUpgradeTasks fileFilters); the
# regenerated server/gen/** is gitignored and intentionally discarded.
#
# Must run from the repo root. The image that provides buf + the protoc plugins
# + Go is renovate/Dockerfile; the command allowlist lives in renovate/config.js.
set -euo pipefail

echo "renovate_gomod_regen: regenerating Go proto packages for go.sum tidy"

for tool in buf go protoc-gen-go protoc-gen-connect-go; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "renovate_gomod_regen: required tool '$tool' not found on PATH" >&2
    exit 1
  fi
done

# Local plugins, source_relative output into server/gen — no remote plugins, so
# no network dependency on buf.build.
buf generate --template buf.gen.go.yaml

# Now that the generated packages exist, resolve imports and rewrite go.sum.
go mod tidy

echo "renovate_gomod_regen: done (go.mod + go.sum updated)"
