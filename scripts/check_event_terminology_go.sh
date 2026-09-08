#!/usr/bin/env bash
# check_event_terminology_go.sh — fail CI when a user-visible Go string literal
# contains "Experience" or "experience" in a file known to produce user-facing text.
#
# Background: CLAUDE.md requires that every string a user can read or hear says
# "Event", while code identifiers, proto names, and internal logs stay
# "Experience". This script enforces that rule on the narrow set of Go files
# whose job is to construct user-visible strings (chat system messages, story
# cards, push notifications, feed nudges, impact copy).
#
# Escape hatch: add
#   // experience-string-allow: <reason>
# on the line immediately above the offending literal. The lint will skip that
# line. Use sparingly — the reason field is auditable via grep.
#
# Excluded automatically:
#   - Lines whose first non-whitespace token is a logger/error call (operator-
#     facing log strings are permitted to say "experience" per docs/server/observability.md).
#   - Lines containing connecterr.Internal( anywhere (operation names and error
#     details in those calls are operator-facing, not user-visible).
#   - Literals that contain NO SPACES — these are identifier-shaped values
#     (RPC names, field keys, analytics event names, enum strings) rather than
#     human-readable text.
#   - Comments.
#
# Usage: ./scripts/check_event_terminology_go.sh
# Exit:  0 = clean, 1 = violations found, 2 = setup error

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Files known to produce user-visible strings. Extend this list when a new file
# is added that constructs strings rendered in the app, push notifications,
# emails, or chat.
# AI prompt files count: an exemplar CTA label handed to the model is copied
# verbatim into user-facing copy, so the violation is authored here even though
# no user-visible string is assembled here (#2803).
SCANNED_FILES=(
  "server/services/experience/share.go"
  "server/services/experience/lifecycle.go"
  "server/community/notifications.go"
  "server/community/events.go"
  "server/story/templates.go"
  "server/services/feed/nudges.go"
  "server/impact_metrics/metric_detail_factors.go"
  "server/ai/prompts.go"
  "server/ai/prompts_hero_card.go"
  "server/ai/prompts_impact_row.go"
  "server/ai/prompts_story.go"
  "server/ai/prompts_summaries.go"
  "server/ai/prompts_unified.go"
)

# Also scan email template files.
EMAIL_GLOB="server/email"

failed=0

check_file() {
  local rel="$1"
  local file="$ROOT/$rel"

  if [ ! -f "$file" ]; then
    return
  fi

  local prev_line=""
  local lineno=0
  while IFS= read -r line; do
    lineno=$((lineno + 1))

    # Skip blank lines and comment-only lines.
    if [[ "$line" =~ ^[[:space:]]*(//|/\*|\*) ]] || [[ -z "${line// }" ]]; then
      prev_line="$line"
      continue
    fi

    # If the previous non-blank line is the allow hatch, skip this line.
    if echo "$prev_line" | grep -q '// experience-string-allow:'; then
      prev_line="$line"
      continue
    fi

    # Skip logger/slog/error call sites (operator-facing strings).
    if echo "$line" | grep -qE '^\s*(logger\.|slog\.|log\.Printf|log\.Println|fmt\.Errorf)'; then
      prev_line="$line"
      continue
    fi

    # Skip any line containing connecterr.Internal( — operation names and detail
    # strings in those calls are operator-facing and never reach users.
    if echo "$line" | grep -q 'connecterr\.Internal('; then
      prev_line="$line"
      continue
    fi

    # Check if the line contains a string literal with Experience/experience.
    # We look for double-quoted or backtick strings.
    if echo "$line" | grep -qE '"[^"]*[Ee]xperience[^"]*"|`[^`]*[Ee]xperience[^`]*`'; then
      # Extract all matching literals and check each one.
      local found_violation=0
      # For each "..." literal containing experience, check if it has spaces.
      # Literals with NO SPACES are identifier-shaped (RPC names, field keys,
      # analytics event names) and are safe to ignore.
      while IFS= read -r literal; do
        # Strip surrounding quotes.
        local inner="${literal#\"}"
        inner="${inner%\"}"
        # If no spaces, it's an identifier — skip.
        if [[ "$inner" != *" "* && "$inner" != *$'\t'* ]]; then
          continue
        fi
        found_violation=1
        break
      done < <(echo "$line" | grep -oE '"[^"]*[Ee]xperience[^"]*"')

      if [ "$found_violation" -eq 1 ]; then
        echo "FAIL: $rel:$lineno: user-visible 'experience' string literal"
        echo "      $line"
        failed=1
      fi
    fi

    prev_line="$line"
  done < "$file"
}

# Check the fixed list of Go files.
for rel in "${SCANNED_FILES[@]}"; do
  check_file "$rel"
done

# Check email templates (HTML and plain-text).
if [ -d "$ROOT/$EMAIL_GLOB" ]; then
  while IFS= read -r -d '' file; do
    rel="${file#"$ROOT/"}"
    check_file "$rel"
  done < <(find "$ROOT/$EMAIL_GLOB" \( -name '*.html' -o -name '*.txt' \) -print0 2>/dev/null)
fi

if [ "$failed" -ne 0 ]; then
  echo ""
  echo "User-visible Go strings must say 'Event', not 'Experience' (CLAUDE.md UI Terminology)."
  echo "Fix the strings above, or add '// experience-string-allow: <reason>' on the line"
  echo "immediately before the literal if 'experience' is genuinely the right word."
  exit 1
fi

echo "check_event_terminology_go passed: no user-visible 'experience' strings found."
