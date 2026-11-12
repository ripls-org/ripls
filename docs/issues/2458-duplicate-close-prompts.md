# Issue #2458: Duplicate event completion reminder notifications

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2458
**Filed:** 2026-06-15 | **Last activity:** 2026-06-15
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/server/scheduled_notifications.md` (canonical
design), `docs/issues/625-scheduled-notifications.md` (history),
`docs/server/architecture.md`, `docs/server/observability.md`,
`docs/server/conventions.md`, `docs/proto_conventions.md`,
`docs/reviews/error_handling_review_prompt.md`,
`docs/reviews/observability_review_prompt.md`,
`docs/reviews/code_quality_review_prompt.md`,
`docs/reviews/testing_review_prompt.md` (engaged dimensions).

> **Audit note (post-`ripls-audit`):** Reviewed against
> `docs/server/architecture.md`, `docs/server/conventions.md`,
> `docs/server/observability.md`, `docs/server/scheduled_notifications.md`,
> and the error_handling / observability / code_quality / testing review
> prompts. Audit refinements applied below:
> 1. **Migration safety** — `CREATE UNIQUE INDEX IF NOT EXISTS` will fail
>    on a table with pre-existing duplicates. Phase 2 now precedes the
>    index creation with an idempotent `DELETE`-of-duplicates statement
>    so first-deploy CI converges without a hand-rolled hotfix.
> 2. **Field-name conventions** — observability logs for a request-anchor
>    reap use `target_request_id` (not `request_id`, which is reserved
>    for the HTTP correlation key — see
>    `docs/server/conventions.md` Structured Logging).
> 3. **Transient-error policy** — the helper's real-DB-error path composes
>    with the existing `errs.TransientStreak` machinery; no new error
>    escalation surface.
> 4. **Goroutine safety** — the reaper runs inside the existing
>    `logging.GoSafe("scheduled-notifications-reconciler", …)` goroutine
>    (`server/main.go:1316`); no `cmd/check-goroutines` allowlist edit
>    needed.
> 5. **Doc-context-map gate** — Phase 4 must regenerate `docs/llms.txt`
>    via `npm run generate:context-map` after the
>    `docs/server/scheduled_notifications.md` edit so `npm run lint`
>    stays green.
> 6. **Schema-comment style** — the new index comments must not include
>    phase markers (CLAUDE.md / `conventions.md` rule); reference the
>    issue (`#2458`) and the invariant only.

## Problem Statement

A host on app 0.24.0 (Android, PROD) received **two push notifications
asking them to mark their event completed**, ~2 minutes apart. They
expected one.

The notification copy in the screenshot matches the Experience
**close-prompt** (`server/jobs/scheduled_notifications/experience_reminders.go:552`):
title `Wrap up your event`, body `Mark <name> as completed when
you're done`. There is **no other code path** that produces that exact
copy — the duplicates come from the scheduled-notification system, not
from the reactive `EXPERIENCE_COMPLETED` notification or any
chat/system message.

The close-prompt fires at `experience.start_unix_sec + 24h` to the
host only and is gated by `UserNotificationPreferences.notify_event_close_prompts`.
A correctly-functioning reconciler/dispatcher pair should produce
**exactly one** push per close-prompt; the user got two.

The ~2-minute gap rules out simultaneous send (same `FindDueScheduledNotifications`
batch). It is consistent with **two separate dispatcher ticks**
(1-minute interval) each firing a different row that shared the same
uniqueness key.

## Context from Comments

- Single `@claude` ping from `maintainer` (2026-06-15) — no
  textual content. No reproduction steps beyond the screenshot.

## Root-Cause Analysis (code)

### How the system is supposed to work

Per `docs/server/scheduled_notifications.md`:

- The **reconciler** (every 10 min,
  `server/jobs/scheduled_notifications/reconcile.go`) loads the
  desired-state tuple set from current entity state, loads the
  existing rows from storage for that item type, and applies
  `INSERT` / `UPDATE fire_at` / `DELETE` per the diff.
- The **uniqueness key** is
  `(recipient_user_id, item-variant-id, purpose, offset_seconds_from_anchor)`
  (`framework.go:UniquenessKey`).
- The **dispatcher** (every 1 min, `dispatch.go`) pulls due rows,
  routes by `CanHandle`, atomically claims via
  `ClaimAndDeleteScheduledNotification` (delete-by-id), and renders +
  sends. Delete-on-fire is the lifecycle.
- **Re-fire protection** rests on two rules: `Desired()` drops tuples
  whose `fire_at < now - DesiredFireAtGrace` (5 min); and the reconciler
  leaves past-`fire_at` rows alone, since the dispatcher owns them.

### Where the invariant breaks

The uniqueness key is enforced **only in application code** — it is
the diff key, not a database constraint. Two things can produce
duplicate rows with the same key:

1. **Multi-replica reconciler race (most likely).** Prod is configured
   with `min_instances = 1, max_instances = 2`
   (`terraform/environments/prod/main.tf:334`). Each instance runs its
   own `scheduled-notifications-reconciler` goroutine (`server/main.go:1316`).
   When both replicas' reconcilers tick close in time:
   - A: `Existing()` returns 0 rows for this key.
   - B: `Existing()` also returns 0 rows (before A commits).
   - A: `INSERT` row₁ with auto-generated UUID id₁.
   - B: `INSERT` row₂ with auto-generated UUID id₂.

   Both rows have the same `(recipient_user_id, experience_experience_id,
   experience_purpose, experience_offset_seconds_from_anchor)` and the
   same `fire_at`. **The schema has no UNIQUE constraint on this key**
   (`server/storage/protosql_schema.go:424-425` — only the
   `fire_at_unix_sec` and `recipient_user_id` indexes exist).

   On subsequent reconciler ticks, `existingByKey` (`reconcile.go:62-75`)
   collapses both rows to one (last-write-wins map). The diff sees one
   key → one desired tuple → "Unchanged". Both rows persist.

   At `fire_at`, the dispatcher's `FindDueScheduledNotifications` batch
   includes both rows. They get claimed (delete-by-id, different ids)
   and dispatched independently — to the same recipient.

   The ~2-minute gap matches the dispatcher's 1-min tick interval and
   the staggered tick offsets between replicas.

2. **Re-fire within grace, after a dispatcher fire.** The design
   protects against this only because `DesiredFireAtGrace = 5min` is
   smaller than the **single-replica** reconciler interval of 10 min.
   With two replicas the effective system reconciler cadence is closer
   to 5 min (staggered ticks), which is **right at the boundary**.
   A single late reconciler tick landing within 5 minutes of the
   dispatcher's claim would re-emit the tuple, the next dispatcher
   tick would re-fire — duplicate with the same ~2-min gap.

Both root causes share the same fix surface: **insertion must be
idempotent against the reconciler's uniqueness key**, and any
duplicates that slip in must be reaped before they fire. The current
codebase enforces uniqueness only in the diff map, which is too late.

### Why prior fixes don't cover this

Issue #2088's dedup (`server/notifications/community_subscriber/dedup.go`)
keys on the propagated HTTP `request_id` of an originating RPC. The
close prompt is **not** RPC-originated — it's emitted by a background
ticker — so `request_id` is empty and the dedup gate is bypassed by
design (no-op fallback). #2088's fix is orthogonal and correct; it
just doesn't apply here.

## Potential Solutions

### Option A: Database-level uniqueness via partial UNIQUE indexes

- **Approach:** Add three partial UNIQUE indexes on `scheduled_notification`
  — one per `item` oneof variant — keying on the uniqueness tuple:

  ```sql
  CREATE UNIQUE INDEX idx_scheduled_notification_unique_experience
    ON "scheduled_notification" (
      recipient_user_id,
      experience_experience_id,
      experience_purpose,
      experience_offset_seconds_from_anchor
    )
    WHERE experience_experience_id IS NOT NULL
      AND experience_experience_id <> '';
  -- same shape for loan_transfer_id and request_request_id
  ```

  Add a `ScheduledNotificationInsertOrIgnore` storage helper that runs
  `INSERT ... ON CONFLICT DO NOTHING` against the new indexes and
  reports whether the row was actually inserted (via `RowsAffected`).
  The reconciler insert path calls this helper instead of the generic
  `Insert`. A conflict counts as `Unchanged` (the other replica
  already wrote the row) — not as `Errored`, so the logs stay clean.

  Add a one-time **reaper** at process start (or as part of the
  reconciler's first tick) that detects and deletes pre-existing
  duplicates whose `fire_at` is still in the future. Prefer keeping
  the row with the most recent `updated_at` (or smallest `id` —
  either is fine, the rows are equivalent).

- **Pros:** Exact, race-free uniqueness enforced where state lives.
  The reconciler can't insert duplicates regardless of how many
  replicas race or how the diff is timed. Idiomatic for this codebase
  — the same partial-unique-on-oneof-variant pattern is already used
  for `share_link` (`protosql_schema.go:456-460`). Per-purpose unique
  indexes also speed up the reconciler's `existingByKey` lookups at
  scale (negligible win today; free at the index level).

- **Cons:** Schema migration (additive index creation — additive
  schema changes are routine here; the existing
  `CREATE UNIQUE INDEX IF NOT EXISTS` calls in `protosql_schema.go`
  run on startup). Insert path forks: `Insert` for everything else,
  `InsertOrIgnore`-style helper for `ScheduledNotification`. Need a
  reaper for pre-existing duplicates.

- **Effort:** Small-to-medium.

### Option B: Application-level idempotency via a "check-then-insert" transaction

- **Approach:** Wrap the reconciler's INSERT step in a serializable
  transaction that re-reads `Existing()` for the affected key inside
  the transaction and only inserts if still missing. The reconciler's
  current `Existing()` → diff → `Insert()` becomes
  `BEGIN SERIALIZABLE ... SELECT FOR UPDATE ... INSERT ... COMMIT`.

- **Pros:** No schema change.

- **Cons:** Materially worse than Option A on every dimension:
  - Postgres SERIALIZABLE transactions on a hot table = retries and
    contention; the diff loop runs over potentially hundreds of
    desired tuples per tick.
  - The "check-then-insert" pattern is famous for getting subtle
    aspects wrong (predicate locking semantics, what counts as a
    conflict).
  - Doesn't repair pre-existing duplicate rows in prod.
  - The protosql `Insert` shape would need to take an optional pre-check
    callback or a SQL prelude — both invasive.

- **Effort:** Medium-large.

### Option C: Single-writer reconciler via leader election

- **Approach:** Run the reconciler on only one instance at a time —
  e.g. a periodic claim row in a dedicated `reconciler_leadership`
  table updated with a TTL by whichever replica owns it. Dispatchers
  can still run on both replicas (they're already safe — claim is
  atomic delete-by-id).

- **Pros:** Conceptually eliminates the cross-replica race entirely.

- **Cons:** New durable claim primitive (a smaller version of what
  #1659 / #1924 already build). Doesn't help with the within-replica
  re-fire-within-grace edge case (which a strict reading of the design
  also leaves open). Largest blast radius — changes the deployment
  story for the reconciler. Doesn't fix the underlying invariant gap;
  the day someone scales the reconciler to two leaders for any
  reason, the bug returns.

- **Effort:** Medium.

## Recommendation

**Option A.** Lift the application-level uniqueness invariant into the
database as a partial unique index per `item` variant, switch the
reconciler insert path to `ON CONFLICT DO NOTHING`, and reap
pre-existing duplicates whose `fire_at` is still in the future.

Why this is the right shape:

- The uniqueness key was **always** the source of truth — it just
  wasn't enforced where the writes happen. Option A closes that gap
  with the smallest, most idiomatic change in this codebase
  (`share_link` already uses this exact pattern).
- It is timing-independent and replica-independent. Any future
  scaling change (more replicas, faster reconciler tick, manual
  re-run for a backfill) stays correct.
- It also strengthens the design's re-fire-within-grace assumption —
  even if the grace window's discipline lapses, the DB rejects the
  duplicate before it can fire.
- Option B is more invasive and weaker (no defense in depth, no
  prod-data repair). Option C trades a real-world bug for a
  new claim primitive and a deployment-shape change.

The reaper runs once at the start of each reconciler tick (cheap:
indexed scan by the new unique key, looking for groups of size > 1
restricted to `fire_at > now`). It survives a partial migration where
some duplicates predate the index. After the first clean reconciler
tick, it stays no-op (every group has exactly one row).

## Open Questions

- **Should the duplicate reaper hard-delete or soft-delete?** —
  Recommendation: **hard-delete.** The lifecycle is delete-on-fire;
  there is no soft-delete pathway in this table. The reaper logs an
  `INFO` line with the discarded row id, the kept row id, and the key
  — for forensics — but doesn't preserve the row.

- **Pick "winner" by smallest id, latest `updated_at`, or earliest
  `created_at`?** — Recommendation: **smallest id (UUID v4
  lexicographic)** for determinism. The rows are functionally
  equivalent — same recipient, same purpose, same offset, same
  fire_at, same anchor. The choice only matters for which row's id
  shows up in subsequent log lines.

- **Should the insert helper be generic (`InsertOrIgnore`) or
  scheduled-notification-specific?** — Recommendation:
  **scheduled-notification-specific.** Naming the helper
  `InsertScheduledNotificationIfAbsent` (or
  `ClaimScheduledNotificationKey`) keeps the protosql Insert surface
  narrow and avoids tempting other callers to bypass uniqueness
  semantics they haven't designed for. Matches the pattern of the
  existing `ClaimAndDeleteScheduledNotification` and
  `FindXScheduledNotifications` helpers in
  `server/storage/scheduled_notification.go`.

- **What does the conflict look like to the reconciler stats?** —
  Recommendation: count `ON CONFLICT DO NOTHING` (zero
  `RowsAffected`) as `Unchanged`, with a `DEBUG` log noting the key
  and which replica we believe wrote first. Counting it as `Errored`
  would noise up alerts on the legitimate two-replica race; counting
  it as `Inserted` would lie. `Unchanged` is the truthful summary —
  the row exists in the desired shape, we just didn't write it.

- **Should we also add a `loan` and `request` partial index even
  though only `experience` is reported broken?** — Recommendation:
  **yes.** They're trivially symmetric and prevent the same bug from
  silently shipping to loan reminders and request follow-ups; the
  schema change is one migration; adding two more partial indexes is
  free. Cover all three `item` oneof variants.

- **Should the reaper run on every reconciler tick or just once at
  process start?** — Recommendation: **every tick, but only over rows
  whose `fire_at` is in the future.** Per-tick cost is bounded by the
  index size (small at our scale). The reaper is naturally idempotent
  — after the index lands, no new duplicates appear, so steady-state
  cost is zero.

- **Observability for the duplicate detection itself?** — Recommendation:
  emit per-duplicate `INFO` lines with
  `operation = ScheduledNotificationReconcile`, the standard entity
  fields (`recipient_user_id`, the anchor id from the `item` oneof —
  `experience_id` / `transfer_id` / **`target_request_id`** depending on
  the variant; the `target_request_id` spelling avoids colliding with
  the HTTP correlation `request_id` per
  `docs/server/conventions.md` *Structured Logging*), `purpose`,
  `offset_seconds_from_anchor`, the kept and discarded ids, and
  `outcome = "reaped_duplicate"`. This piggy-backs on the existing
  observability schema (`docs/server/observability.md`) and lets us
  watch the reaper's rate fall to zero after rollout — a clean
  before/after signal.

- **How does the new helper interact with the background-job transient
  error policy?** — Recommendation: no new path. The helper returns a
  conflict outcome as `(inserted=false, nil)` (not an error), so the
  fail-open `errs.TransientStreak` plumbing in `server/main.go:1317-1325`
  sees only **real** DB errors (connection reset, etc.) and the
  existing `errs.LogJobError` already routes them through
  `errs.TransientPageAfter` (per
  `docs/server/observability.md` *Background-job transient errors*).
  The fix doesn't move that policy.

## Implementation Plan

### Phase 1 — Lock the bug down with a failing test
**Goal:** A regression test that reproduces the duplicate close-prompt
end-to-end in the existing in-process test harness.

**Tasks:**
- [ ] Add a unit test in
      `server/jobs/scheduled_notifications/reconcile_test.go` (or a
      new file alongside it — match what's already there) that:
  - Builds a `fakeReconcileStorage` pre-populated with **two
    duplicate rows** for the same uniqueness key (same recipient,
    same experience, same `CLOSE_PROMPT` purpose, same
    `OffsetClosePromptAfterStart` offset, same `fire_at`, different
    ids). This simulates the post-race steady state.
  - Runs `Reconcile` with a `stubReconciler` whose `Desired` emits
    one tuple for that key.
  - Asserts that after the reconcile **only one row remains** in
    storage for that key (the duplicate was reaped). Currently this
    fails: the diff sees one key → "Unchanged" → both rows persist.
- [ ] Add a sibling test that pre-populates the fake `ReconcileStorage`
      with **no rows**, runs two reconciles back-to-back **without
      committing the first's INSERT before the second reads** (use the
      existing `fakeReconcileStorage.mu` to model the race; the
      existing `TestReconcile_ConcurrentDispatcherDeletedRow` shape is
      the model). Assert the second `Insert` is a no-op once the
      uniqueness check is in place. Failing today.
- [ ] Confirm both tests **fail** today. They are the red→green target
      for Phases 2 and 3.

**Validates:** The bug is reproducible without testcontainers and
without prod data, and the fix has an explicit acceptance shape.

### Phase 2 — DB schema + storage helper
**Goal:** Make duplicate inserts impossible at the storage layer.

**Tasks:**
- [ ] **Pre-index cleanup (migration safety).** Before the new unique
      indexes can be created, any duplicate rows already in prod must
      be reaped — otherwise `CREATE UNIQUE INDEX IF NOT EXISTS` fails
      with `duplicate key value violates unique constraint` on first
      deploy. In `server/storage/protosql_schema.go`, run an
      idempotent `DELETE` immediately before the `indexStatements`
      block, scoped to **future-fire-at** rows only (`fire_at_unix_sec > <now>`)
      so the dispatcher's ownership of past-fire-at rows
      (`scheduled_notifications.md` *Re-fire protection*) is respected.
      Shape (one per `item` variant; example for experience):

      ```sql
      DELETE FROM "scheduled_notification" a
      USING "scheduled_notification" b
      WHERE a.id > b.id
        AND a.recipient_user_id = b.recipient_user_id
        AND a.experience_experience_id = b.experience_experience_id
        AND a.experience_purpose = b.experience_purpose
        AND a.experience_offset_seconds_from_anchor = b.experience_offset_seconds_from_anchor
        AND a.experience_experience_id IS NOT NULL
        AND a.experience_experience_id <> ''
        AND a.fire_at_unix_sec > EXTRACT(EPOCH FROM NOW())::bigint;
      ```

      Idempotent (no-op once there are no duplicates), and safe under
      multi-replica deploy (both replicas DELETE the same rows by id,
      result is convergent).

- [ ] In `server/storage/protosql_schema.go`, add three partial
      `CREATE UNIQUE INDEX IF NOT EXISTS` statements to the
      `indexStatements` list (next to the existing
      `idx_scheduled_notification_due` and
      `idx_scheduled_notification_user`):
  - `idx_scheduled_notification_unique_experience` on
    `(recipient_user_id, experience_experience_id, experience_purpose,
    experience_offset_seconds_from_anchor)`
    `WHERE experience_experience_id IS NOT NULL AND experience_experience_id <> ''`.
  - `idx_scheduled_notification_unique_loan` on
    `(recipient_user_id, loan_transfer_id, loan_purpose,
    loan_offset_seconds_from_anchor)` with the symmetric predicate.
  - `idx_scheduled_notification_unique_request` on
    `(recipient_user_id, request_request_id, request_purpose,
    request_offset_seconds_from_anchor)` with the symmetric predicate.

      Match the comment style of the surrounding indexes; explain that
      these enforce the reconciler's uniqueness key
      (`framework.go:UniquenessKey`). Reference the issue (`#2458`),
      **not** any phase number — phase markers in source comments fail
      the `scripts/check_ephemeral_comments.js` gate (`server/conventions.md`).
- [ ] In `server/storage/scheduled_notification.go`, add a new helper
      `InsertScheduledNotificationIfAbsent(ctx, row) (inserted bool, id string, err error)`
      that runs `INSERT ... ON CONFLICT DO NOTHING` and reports
      whether `RowsAffected() > 0`. Mirror the construction style of
      `Insert` in `protosql.go` for the column extraction; do **not**
      generalize the change into the generic `Insert` path.

      Note: the helper must mint a UUID for `id` before the insert
      (the generic `Insert` does this in `protosql.go:55`) and return
      the *attempted* id even on `inserted=false` — the reconciler
      doesn't depend on the returned id for the conflict case, but
      logging it (alongside the existing winner's id, looked up via
      a follow-up SELECT if useful) helps forensics.
- [ ] Unit-test the helper in
      `server/storage/scheduled_notification_test.go` using the existing
      `storage.SetupTestStorage(t)` testcontainer pattern (per
      `server/conventions.md` *Server Testing* — tests must hit a real
      postgres so the partial-index predicates and `ON CONFLICT` are
      genuinely exercised, not mocked away):
  - Insert succeeds when the table is empty.
  - Second insert with the same uniqueness tuple but different id
    succeeds with `inserted=false` and does **not** create a second
    row.
  - Distinct purposes / offsets / recipients all coexist.
  - Different `item` variants with the "same" coincidental column
    values (e.g. `experience.experience_id = X` and `loan.transfer_id = X`)
    do **not** collide (the partial-index predicates ensure this).
  - Pre-index cleanup `DELETE` purges duplicates with future fire_at
    only; past-fire-at duplicates survive (dispatcher's territory).
- [ ] `npm run build` and `go test ./server/storage/...` green.

**Validates:** The uniqueness invariant is now enforced at the DB
level; new inserts can't produce duplicate rows regardless of
reconciler timing.

### Phase 3 — Wire the helper into the reconciler + reap pre-existing duplicates
**Goal:** Reconciler uses the new helper for inserts and clears any
duplicates already in storage.

**Tasks:**
- [ ] In `server/jobs/scheduled_notifications/reconcile.go`, switch
      the reconciler's INSERT branch (around line 84) from
      `store.Insert(ctx, d)` to the new
      `InsertScheduledNotificationIfAbsent` helper through the
      `ReconcileStorage` interface (extend the interface). On
      `inserted=false`, increment `stats.Unchanged` (not `Inserted`),
      and emit a `DEBUG` line — the row already exists by another
      writer's hand.
- [ ] Add a duplicate-reaper pass that runs at the top of `Reconcile`
      (or extract it into its own driver step the reconciler calls
      once per tick). Lives inside the existing
      `logging.GoSafe("scheduled-notifications-reconciler", …)`
      goroutine (`server/main.go:1316`) — no new goroutine spawn, so
      no edit to `server/cmd/check-goroutines` allowlist needed.
      Implementation:
  - Group `existing` rows by `UniquenessKey`.
  - For each key with `len(rows) > 1` **and** all rows' `fire_at >
    now` (future-only, to respect the
    "past-fire-at belongs to the dispatcher" invariant from
    `scheduled_notifications.md` *Re-fire protection*), pick the
    smallest-id row as the keeper, and `store.Delete()` the rest.
  - Emit an `INFO` line per discarded row via
    `logging.LoggerWithContext(ctx)` with the standard fields
    (`operation = ScheduledNotificationReconcile`, `recipient_user_id`,
    the anchor id from the `item` oneof — using **`target_request_id`**
    for the request variant per the `server/conventions.md`
    *Structured Logging* field-name rule, **not** `request_id`, which
    is reserved for the HTTP correlation key), `purpose`,
    `offset_seconds_from_anchor`, `outcome = "reaped_duplicate"`,
    `kept_scheduled_notification_id`, and
    `discarded_scheduled_notification_id`. Count discards into a new
    `stats.ReapedDuplicates` field on `ReconcileStats` and log it in
    the tick summary.
- [ ] Update `framework_test.go` /
      `reconcile_test.go` for any new interface methods on
      `ReconcileStorage` and the new stat field. Make the Phase 1
      tests green.
- [ ] Add a focused reaper test:
  - Future-fire duplicates → both detected, one kept, one
    deleted, `ReapedDuplicates = 1`.
  - Past-fire-at "duplicates" → reaper leaves both alone (dispatcher
    owns them), `ReapedDuplicates = 0`.
  - Triplicates → two of three deleted, `ReapedDuplicates = 2`.
- [ ] `npm run build` and
      `go test ./server/jobs/scheduled_notifications/...`.
      Re-run `go test -tags=integration -timeout 20m ./server/...` to
      confirm no integration regressions (the integration tests use
      the real protosql schema, so the new unique indexes get
      exercised).

**Validates:** End-to-end behavior matches the design doc's invariant
("at most one row per uniqueness key, dispatcher fires it exactly
once"). The bug from the report cannot recur. Prod data with
pre-existing duplicates self-heals on the first reconciler tick after
deploy.

### Phase 4 — Doc + observability polish
**Goal:** Make the new behavior discoverable and the metric watchable.

**Tasks:**
- [ ] Update `docs/server/scheduled_notifications.md`:
  - In *Re-fire protection*, add a third rule: "DB-level partial unique
    indexes on the per-`item`-variant uniqueness tuple guarantee at
    most one row per key, regardless of reconciler concurrency."
  - In *Storage*, document the three new partial unique indexes (one
    line each).
  - Add a one-paragraph *Idempotent insert* subsection pointing to
    `InsertScheduledNotificationIfAbsent` and the reaper.
  - Refresh the `freshness:` `verified_commit` and `verified_on`
    stamps per `docs/context_map.md`.
- [ ] **Regenerate the context-map index.** Because
      `docs/server/scheduled_notifications.md` has its `context:`
      frontmatter rewritten (description / freshness), run
      `npm run generate:context-map` to refresh `docs/llms.txt`. The
      `npm run lint:context-map` gate (part of `npm run lint`) will
      fail CI otherwise.
- [ ] Update the package `README.md` in
      `server/jobs/scheduled_notifications/` with a one-line pointer
      to the new helper, matching the existing READMEs-list-the-files
      style.
- [ ] (Stretch) Add a Metabase chart on the
      `outcome = "reaped_duplicate"` log line for the first 14 days
      after deploy to confirm the rate falls to zero. Follow
      `ripls-analytics` skill conventions if pursuing this.

**Validates:** A future maintainer reading the design doc understands
both the application invariant and the DB enforcement that backs it;
oncall has a signal that the fix took.

## Related Issues

- **#625** — Scheduled notifications design and original implementation;
  this fix tightens the uniqueness story without changing the design
  contract.
- **#2088** — Duplicate notifications via the community-event bus;
  orthogonal mechanism (RPC fan-out) with its own dedup gate. Worth
  reading for the "where dedup belongs" debate; the resolution there
  was "at the closest layer to the invariant," which is the same
  reasoning leading us to DB-level uniqueness here.
- **#1622** — Notification dispatcher deleted-guard. Same family of
  concerns (dispatcher must handle race with concurrent state
  changes); reinforces that the dispatcher's responsibility is *fire
  what's claimed*, not *re-validate uniqueness*.
