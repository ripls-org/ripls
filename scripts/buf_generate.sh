#!/usr/bin/env bash
# Wrapper around `buf generate` for templates that pull REMOTE plugins from
# buf.build (the BSR). Those plugin fetches are rate-limited when unauthenticated,
# and a CI job that forgets to provide BUF_TOKEN otherwise fails with a confusing
# "BSR rate limiting" error deep in the run.
#
# Anti-drift guard: in CI, fail fast with a clear, actionable message if BUF_TOKEN
# is missing — so a workflow that didn't thread the token through (a plain job
# that forgot `env: BUF_TOKEN`, or a reusable-workflow caller that didn't pass it,
# since reusable workflows don't inherit secrets) breaks loudly at the point of
# use instead of silently hitting a rate limit.
#
# Local dev is unaffected: developers authenticate `buf` via `buf registry login`
# (stored credentials), so the guard only triggers when CI is set AND BUF_TOKEN is
# empty. The Go template (buf.gen.go.yaml) uses local plugins and does not go
# through this wrapper.
set -euo pipefail

if [ -n "${CI:-}" ] && [ -z "${BUF_TOKEN:-}" ]; then
  echo "ERROR: BUF_TOKEN is empty in CI." >&2
  echo "  This 'buf generate' pulls remote plugins from buf.build and will hit BSR" >&2
  echo "  rate limits unauthenticated. Provide BUF_TOKEN on the job, and for a" >&2
  echo "  reusable workflow declare it under workflow_call.secrets and pass it from" >&2
  echo "  every caller (secrets are not inherited)." >&2
  exit 1
fi

exec buf generate "$@"
