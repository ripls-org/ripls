#!/usr/bin/env bash
# check_viewmodel_tests.sh — fail CI when any *_view_model.dart under
# app/lib/presentation/viewmodels/ lacks a corresponding test file under
# app/test/presentation/viewmodels/.
#
# This enforces the contract from issue #1362: every ViewModel must have a
# paired unit-test file before it can be merged to main.
#
# Usage: ./scripts/check_viewmodel_tests.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VM_DIR="$ROOT/app/lib/presentation/viewmodels"
TEST_DIR="$ROOT/app/test/presentation/viewmodels"

if [ ! -d "$VM_DIR" ]; then
  echo "error: $VM_DIR does not exist" >&2
  exit 2
fi

failed=0
while IFS= read -r -d '' vm_file; do
  basename="${vm_file##*/}"
  test_file="$TEST_DIR/${basename%.dart}_test.dart"
  if [ ! -f "$test_file" ]; then
    rel="${vm_file#"$ROOT/"}"
    echo "MISSING TEST: $rel has no paired test at ${test_file#"$ROOT/"}"
    failed=1
  fi
done < <(find "$VM_DIR" -maxdepth 1 -name '*_view_model.dart' -print0)

if [ "$failed" -ne 0 ]; then
  echo
  echo "Every *_view_model.dart must have a paired test in app/test/presentation/viewmodels/."
  echo "Add the missing test file(s) listed above before merging."
  exit 1
fi

echo "All ViewModel files have paired tests."
