# Scripts

## Event-Terminology Lint (`check_event_terminology_go.sh`)

`scripts/check_event_terminology_go.sh` enforces the CLAUDE.md UI Terminology rule for Go: user-visible string literals in files known to produce user-facing text (chat system messages, story cards, push notifications, feed nudges, impact copy) must say "Event", not "Experience". The script scans a hardcoded list of source files, skips logger/error call sites and identifier-shaped keys, and fails CI on any remaining violation. To exempt a specific literal, add `// experience-string-allow: <reason>` on the line immediately above it. Wired into `npm run lint:go` and therefore into `npm run lint`.
