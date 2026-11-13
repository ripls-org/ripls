#!/usr/bin/env bash
# Fails when a lint allowlist has more entries than it has on the base ref.
#
# Several lints carry an allowlist of known violations that may shrink as files
# are fixed but must never grow. CI runs this after each such lint.
#
# Usage:
#   check_allowlist_ratchet.sh <allowlist-file> <base-ref> <label> <advice>
#
#   <base-ref>   branch to compare against, fetched from origin
#   <label>      names the allowlist in the error, e.g. "Color"
#   <advice>     what to do instead of adding an entry
#
# An entry is any line that is neither blank nor a `#` comment. A base ref that
# cannot be fetched, or that has no such file, counts as zero entries.
#
# Entries are counted with awk rather than `grep -v | wc -l`: grep exits 1 when
# nothing is left, which an empty allowlist makes the normal case, and that
# kills the step on any runner whose shell has `pipefail` set (Forgejo's
# default; #3293).
set -euo pipefail

if [ "$#" -ne 4 ]; then
  echo "usage: $0 <allowlist-file> <base-ref> <label> <advice>" >&2
  exit 2
fi

file="$1"
base_ref="$2"
label="$3"
advice="$4"

count_entries() {
  awk '!/^#/ && !/^[[:space:]]*$/ { n++ } END { print n + 0 }'
}

# A scratch ref, so the fetch never moves a branch the checkout is using.
scratch="refs/ratchet/base"
git fetch --quiet origin "+$base_ref:$scratch" 2>/dev/null || true

base_count=0
if git rev-parse --verify --quiet "$scratch" >/dev/null; then
  base_count=$({ git show "$scratch:$file" 2>/dev/null || true; } | count_entries)
fi
head_count=$(count_entries <"$file")

echo "base=$base_count head=$head_count"
if [ "$head_count" -gt "$base_count" ]; then
  echo "::error::$label allowlist grew from $base_count to $head_count entries. $advice"
  exit 1
fi
