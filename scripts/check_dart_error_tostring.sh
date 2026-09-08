#!/usr/bin/env bash
# check_dart_error_tostring.sh — fail CI when viewmodel error state is set
# from raw `e.toString()`, or when a viewmodel state class re-introduces a
# resolved-string `errorMessage` field. See:
#   - app/lib/core/errors/README.md (typed UserError pattern)
#   - docs/client/i18n.md (viewmodels never resolve strings)
#
# Either pattern leaks technical exception text or English-only strings to
# users. Use `RpcErrorHandler.classify(e)` and store a typed `UserError?` on
# state; widgets call `RpcErrorHandler.localize(state.error!, context.l10n)`
# at render time.
#
# Escape hatch: append `// dart-error-tostring-allow` on the same line, with
# a tracking-issue link nearby explaining why the migration is deferred.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VM_DIR="$ROOT/app/lib/presentation/viewmodels"

if [ ! -d "$VM_DIR" ]; then
  echo "error: $VM_DIR does not exist" >&2
  exit 2
fi

failed=0

# Pattern 1: viewmodel call site sets errorMessage (or any *Error / mutationError
# field) directly from e.toString() / err.toString() / etc.
pattern1='(errorMessage|mutationError|[a-zA-Z]+Error):\s*(['"'"'"][^'"'"'"\n]*\$\{?\s*[a-zA-Z_]+\.toString\(\)|[a-zA-Z_]+\.toString\(\))'
hits1=$(grep -rEn "$pattern1" "$VM_DIR" --include='*.dart' \
  | grep -v 'dart-error-tostring-allow' \
  | grep -v '\.freezed\.dart' || true)

if [ -n "$hits1" ]; then
  echo "FAIL: viewmodel error state set from raw .toString() — use RpcErrorHandler.classify(e):"
  while IFS= read -r line; do
    echo "  $line"
  done <<< "$hits1"
  failed=1
fi

# Pattern 2: a viewmodel state class re-introduces a `String? errorMessage` /
# `String? mutationError` / `String? <foo>Error` field. The migration removed
# these in favor of `UserError? error` (and typed multi-error fields).
pattern2='^\s*String\?\s+(errorMessage|mutationError|[a-zA-Z]+Error)\b'
hits2=$(grep -rEn "$pattern2" "$VM_DIR" --include='*.dart' \
  | grep -v 'dart-error-tostring-allow' \
  | grep -v '\.freezed\.dart' || true)

if [ -n "$hits2" ]; then
  echo "FAIL: resolved-string error field on viewmodel state — use UserError? instead:"
  while IFS= read -r line; do
    echo "  $line"
  done <<< "$hits2"
  failed=1
fi

if [ "$failed" -ne 0 ]; then
  echo
  echo "Either migrate to the typed UserError pattern (see"
  echo "app/lib/core/errors/README.md) or, if the migration is genuinely"
  echo "deferred, append '// dart-error-tostring-allow' on the offending"
  echo "line with a tracking-issue reference."
  exit 1
fi

echo "No e.toString() leaks or String? errorMessage fields under viewmodels/."
