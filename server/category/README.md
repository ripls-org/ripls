# category

Keyword-based categorizer for user-authored content (experiences and
requests). The output is the `Category` string stored on each row,
which the `known_for` derivation reads to produce "host" and
"asker" chips.

This package is a stopgap until the experience- and request-side
`TODO(#2013)` markers in
[`server/services/experience/gen_ai.go`](../services/experience/gen_ai.go)
and [`server/services/request/gen.go`](../services/request/gen.go)
are filled in by a proper LLM pass.

## Key files

| File | Role |
|------|------|
| `category.go` | `Categorize(title, description)` — substring matcher over an ordered keyword dictionary. Empty string when no rule matches. |
| `backfill.go` | `Backfill(ctx, storage)` — one-shot startup migration that scans every Experience and Request with empty `Category` and persists a derived label. Idempotent. |
| `category_test.go` | Table-driven coverage for matching, ordering, case-insensitivity, and the no-match path. |

## Usage

- **At creation time** — service code calls `Categorize` after
  consuming any AI-derived metadata. The AI's category wins when
  set; the keyword categorizer fills the gap otherwise.
- **For existing rows** — `Backfill` runs once at server startup
  (wired in [`server/main.go`](../main.go)) and persists categories
  on legacy rows. Call site carries a comment to remove the wiring
  after the migration has run successfully against the deployed
  database.

## Tuning the dictionary

`rules` in `category.go` is intentionally short and editable.
Order matters — the first matching rule wins, so specific buckets
(`"Hiking"`) sit above general ones (`"Outdoors"`). Keywords are
lowercased substrings, not word-boundary matches, so `"drill"` also
catches `"drilling"` — a feature, not a bug.

## Known limitations (stopgap intent)

This package is a deliberate stopgap. Three things stay TODO until
the proper AI categorization pipeline lands — they're documented
above `rules` in [`category.go`](./category.go) and tracked at #2013:

1. **The dictionary belongs in configuration, not code.** Keyword
   tweaks shouldn't need a server redeploy — migrate `rules` to an
   embedded `embed.FS` text-proto / YAML / JSON file or to a
   Secret-Manager-fetched config.
2. **LLM detection is the real home.** When the `TODO(#2013)`
   markers in `services/experience/gen_ai.go` and
   `services/request/gen.go` are resolved, this package becomes a
   fallback and eventually retires.
3. **English-only.** The matcher does lowercased ASCII substring
   contains. Non-English content silently returns the empty string
   (no chip). The LLM migration fixes this for free.

## Conventions

- Errors are logged but never returned from `Backfill` — startup
  must not block on a categorization miss.
- Logging via `logging.LoggerWithContext(ctx)` with
  `operation="CategoryBackfill"`, `scanned`, `updated`,
  `no_keyword_match` fields.
- No PII concerns — experience/request title and description are
  user-public.
