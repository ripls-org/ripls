# category

Keyword-based categorizer for user-authored content (experiences and
requests). The output is the `Category` string stored on each row,
which the `known_for` derivation reads to produce "host" and
"asker" chips.

The AI pipeline sets `Category` for gear but not for experiences or
requests, so this categorizer is how those rows get one.

## Key files

| File | Role |
|------|------|
| `category.go` | `Categorize(title, description)` — substring matcher over an ordered keyword dictionary. Empty string when no rule matches. |
| `category_test.go` | Table-driven coverage for matching, ordering, case-insensitivity, and the no-match path. |

## Usage

- **At save time** — the experience and request services call
  `Categorize` when `Category` is empty
  ([`services/experience/save.go`](../services/experience/save.go),
  [`services/request/lifecycle.go`](../services/request/lifecycle.go)).
- **For existing rows** — the one-shot
  [`cmd/recategorize`](../cmd/recategorize) dev tool re-derives
  categories after a dictionary change.

## Tuning the dictionary

`rules` in `category.go` is intentionally short and editable.
Order matters — the first matching rule wins, so specific buckets
(`"Hiking"`) sit above general ones (`"Outdoors"`). Keywords are
lowercased substrings, not word-boundary matches, so `"drill"` also
catches `"drilling"` — a feature, not a bug.

## Known limitations

1. **The dictionary is in code.** A keyword change needs a server
   redeploy.
2. **English-only.** The matcher does lowercased ASCII substring
   contains. Non-English content returns the empty string, so it
   gets no chip.

## Conventions

- No PII concerns — experience/request title and description are
  user-public.
