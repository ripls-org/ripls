# activity_digest

Daily/weekly activity digest assembly. Reads from `server/storage`,
classifies `CommunityEvent` rows through the digest registry, and
produces an `email.ActivityDigestInput` for
`email.Service.SendActivityDigest`.

## Key files

- `doc.go` — package comment.
- `builder.go` — `Builder` type plus `Build(ctx, localDate)` /
  `BuildRange(ctx, start, end, period)` entry points. Owns the parallel
  storage fan-out, foreign-key batch resolution, and the per-community
  grouping.
- `registry.go` — the digest classification of every
  `CommunityEventType`: reporting `Category` + `ActivityTotals` counter
  for counted actions, or an explicit exclusion (UNDONE retractions,
  bookkeeping signals). `TestEveryDigestEventTypeClassified` in
  `registry_test.go` fails CI when a new enum value lands without an
  entry — the digest-side sibling of `server/undo/registry.go`.
- `auth.go` — `AuthMethod` enum → human label mapping.
- `builder_test.go` and friends — unit tests including
  `AssertMaxQueries` checks on the storage call count and the
  reconciliation invariant (counted + excluded == fetched).

## When to add code here vs. elsewhere

- Pure query helpers belong in `server/storage` (e.g.
  `FindEventsInWindow`). The `Builder` only orchestrates and
  formats — it does not write raw SQL.
- A new `CommunityEventType` needs a `registry.go` entry (CI-enforced)
  in addition to its `server/undo/registry.go` classification.
- The email send itself lives in `server/email/SendActivityDigest`.
- Scheduling (the daily/weekly tick + claim primitive call) lives in
  `server/jobs/ActivityDigestJob` and `ActivityWeeklyDigestJob`. This
  package is unaware of scheduling.
