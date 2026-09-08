# known_for

Shared library that derives "is known for" topic tags from items a
user or community has shared into scope. Three sources contribute
to a single shared counts map keyed on the (normalized) category
string, so a category that surfaces from more than one source
collapses into one chip whose weight is the sum across sources:

| Source | Base weight | Completion bonus |
|--------|-------------|------------------|
| Gear listed in scope | +1 per shared gear (`community_gear` pivot) | +1 per completed loan (`Transfer.State == COMPLETED && TransferType == LOAN`) |
| Experiences shared into scope | +1 per shared experience (`community_experience` pivot) | +1 when `Experience.State == COMPLETED` |
| Requests shared into scope | +1 per shared request (`community_request` pivot) | +1 when `Request.State == FULFILLED` |

A category surfaces as a chip when its combined weight clears
[`CategoryThreshold`] (currently 2). Chips render with the bare
category label ("Power Tools", "Cooking") — no role suffix — so
the visual surface stays focused on topics rather than mechanics.
The final list is sorted by weight descending (alphabetic
tie-break) and capped at [`MaxTags`] (currently 20). The client
widget surfaces the first two rows and reveals the rest behind an
expansion affordance.

## Modes

| Mode | Caller | What it answers |
|------|--------|-----------------|
| `ModePerUser` | User profile (`server/services/profile/`) | What is **this user** known for, across the viewer ↔ target community intersection? |
| `ModePerCommunity` | Workshop community screen (`server/services/workshop/`) | What is **this community** (or aggregate of communities) known for, counting any owner / host / requester? |

## Key files

| File | Role |
|------|------|
| `known_for.go` | `Derive(ctx, storage, opts)` entrypoint, `Mode` / `Options`, shared category-tally + normalization helpers. |
| `loans.go` | Gear-share + completed-loan tally; gear category extraction. |
| `experiences.go` | Shared-experience tally with `EXPERIENCE_STATE_COMPLETED` bonus; community-experience pivot for per-user scope. |
| `requests.go` | Shared-request tally with `REQUEST_STATE_FULFILLED` bonus; community-request pivot for per-user scope. |
| `known_for_test.go` | Table-driven coverage of every source plus the cross-source merge. |

## When to add code here

- A **new source** (e.g. completed giveaways) — add a sibling file
  alongside `loans.go` / `experiences.go` / `requests.go`. Each
  file exposes a single `add…Counts(ctx, s, opts, sharedSet, counts)`
  that writes directly into the shared `categoryCounts` (base
  weight per shared item, plus the completion bonus). Wire it into
  `Derive` next to the existing `addLoanCounts` / `addHostCounts` /
  `addAskerCounts` calls.
- A **new aggregation mode** — extend `Mode` and add a new query
  branch to every `load…` helper.

Service-specific glue (`connect.Internal` wrapping, payload
plumbing) stays in the calling service package.

## Conventions

- Errors wrap via `fmt.Errorf("...: %w", err)`. Callers wrap the
  outermost error with `connecterr.Internal(ctx, op, err, kv...)` at
  the RPC boundary.
- Logging via `logging.LoggerWithContext(ctx)` with
  `operation="DeriveKnownFor"`, `mode`, `community_id_count`, plus
  per-source counts. Gear / experience / request categories are
  AI-detected, low-stakes strings — no PII concerns.
- Query budget: **7 reads** per call (3 for gear/loans which needs
  both the community_gear pivot and the transfer query, 2 for
  experiences, 2 for requests). Enforce via
  `storage.AssertMaxQueries(t, ctx, 7, fn)` in any new test.
