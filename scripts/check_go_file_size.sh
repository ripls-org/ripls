#!/usr/bin/env bash
# check_go_file_size.sh — fail CI when any non-generated Go file under server/
# exceeds the size threshold. Mirrors scripts/check_dart_file_size.sh (issue
# #1424, the Go counterpart to the Dart sweep in #1324 / gate in #1293).
#
# A file is skipped when it is generated (*.pb.go, *_connect.go, or under a
# gen/ directory) or contains the literal `// go-line-count-allow` escape-hatch
# comment. If you add the hatch you must also add a tracking-issue link nearby
# explaining the deferred split.
#
# Usage: ./scripts/check_go_file_size.sh [threshold]
#        Default threshold is 1000 lines.

set -euo pipefail

THRESHOLD=${1:-1000}
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="$ROOT/server"

if [ ! -d "$SERVER" ]; then
  echo "error: $SERVER does not exist" >&2
  exit 2
fi

failed=0
while IFS= read -r -d '' file; do
  case "$file" in
    */gen/*|*.pb.go|*_connect.go) continue ;;
  esac
  if grep -q '// go-line-count-allow' "$file"; then
    continue
  fi
  lines=$(wc -l < "$file" | tr -d ' ')
  if [ "$lines" -gt "$THRESHOLD" ]; then
    rel="${file#"$ROOT/"}"
    echo "FAIL: $rel has $lines lines (limit $THRESHOLD)"
    failed=1
  fi
done < <(find "$SERVER" -name '*.go' -print0)

if [ "$failed" -ne 0 ]; then
  echo
  echo "Some non-generated Go files under server/ exceed $THRESHOLD lines."
  echo "Split the file into sibling files in the same package (see"
  echo "docs/server/architecture.md §3), or, if a split is genuinely deferred,"
  echo "add the comment '// go-line-count-allow' near the top of the file with a"
  echo "tracking-issue reference explaining why."
  exit 1
fi

echo "All non-generated Go files under server/ are <= $THRESHOLD lines."
