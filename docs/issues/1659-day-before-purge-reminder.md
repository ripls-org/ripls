# Issue #1659: Phase 2 of #1619: notification copy + day-before-purge reminder for community soft-delete

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1659
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§§ 8, 6.6)

## Problem Statement

The §8 notification table promises five user-visible pushes for the
community soft-delete lifecycle:

| Event | Recipients | Status |
|---|---|---|
| `COMMUNITY_DELETED` | snapshot members minus deleter | ✅ shipped (#1654) |
| `COMMUNITY_RESTORED` | snapshot members minus restorer | ✅ shipped (#1655) |
| `OWNERSHIP_TRANSFERRED` | new owner only | ✅ shipped (#1656) |
| `MEMBER_LEFT` | community owner only | ✅ unchanged (pre-existing) |
| **Day-before-purge** | **all snapshot members** | ⬜ **this issue** |

Four of the five copy paths already exist in
`server/community/notifications.go` after the Phase 2 work. The
remaining piece is the day-before-purge reminder push: *"<name> will
be permanently deleted tomorrow. Tap to restore."* §6.6 step 3 of the
design doc explicitly calls this out as not deferrable — it's the
user-visible promise that the deletion is reversible up to the very
last moment.

This issue ships:

1. The reminder push itself (server-side push title/body in
   `notifications.go`-adjacent code, dispatched directly to
   snapshot members).
2. A daily job in `server/jobs/` that finds communities crossing
   the 29-day mark today and dispatches the reminder.
3. Idempotency tracking so a job re-run doesn't double-notify.
4. The actual hard-delete (§6.6 steps 1 & 2) is **out of scope** —
   that's #1620 (Phase 4). If the reminder fires today and #1620
   hasn't shipped yet, the community survives past day 30 and the
   user can keep restoring it. Acceptable wart documented at the
   PR level.

## Context from Comments

The issue has **no comments**. The body is the spec. Two specific
shape decisions are surfaced as open questions:

- **Idempotency tracking:** `deleted.purge_reminder_sent_at` field on
  `Community.Deleted` proto, OR a separate table. Body says "propose
  in PR." Field is simpler — recommend that.
- **Tests:** unit tests for copy + integration test for the daily
  job (seed 29-day-old community, run, assert pushes; re-run, assert
  no double-dispatch). l10n round-trip via the existing
  `app/test/core/l10n/` test catches missing keys automatically.

## Codebase audit

What already exists this PR will lean on:

- **Notification dispatch.** `notifications.Service.NotifyUser(ctx,
  userID, *Notification)` is the per-user push primitive. Used by
  the existing `notifyTargetedUsers` flow inside
  `server/community/notifications.go`. The day-before-purge push
  doesn't go through `RecordCommunityEventAndNotify` because there's
  no corresponding `CommunityEvent` row — it's a transient reminder,
  not a state transition. We call `NotifyUser` directly per
  recipient.
- **Job framework.** `server/jobs/` holds startup-and-periodic jobs.
  Each is a struct with a `Run(ctx) error` method. Wiring is in
  `server/main.go` ~line 870, where `go func() { ... }()` blocks
  spawn each job under `recover()` panic protection. Existing
  example of a periodic ticker:
  `server/main.go:895` (db pool stats publisher).
- **Always-on infra.** `terraform/environments/{dev,prod}/main.tf`
  both pin `min_instances = 1` ("Always-on for realtime and
  background work"), so an in-process daily ticker is a viable
  scheduling strategy — no external Cloud Scheduler needed.
- **Constants.** `server/community/constants.go` already has
  `RejoinWindowDays = 30` and `RejoinWindowSeconds`. Reuse those
  for the 29-day boundary so the day-before-reminder window stays
  in lockstep with the rejoin/purge window. Extend with a
  `PurgeReminderLeadDays = 1` constant if that helps readability.
- **§8 copy precedent.** Title/body for `COMMUNITY_DELETED`,
  `COMMUNITY_RESTORED`, `OWNERSHIP_TRANSFERRED` live in
  `notifications.go:buildNotification`. The day-before-purge
  notification doesn't go through `buildNotification` (no event row),
  so build the `*models.Notification` inline in the job. Match the
  existing copy style ("<name> will be permanently deleted
  tomorrow. Tap to restore.").
- **Snapshot recipient list.** `Community.DeletedSnapshot.MemberUserIds`
  is the source of truth — populated at delete time by #1654, never
  cleared, used by both `COMMUNITY_DELETED` dispatch and now by this
  job. Same set of recipients minus nobody (the original deleter is
  also a snapshot member and should get the reminder).
- **Push suppression.** `streamChecker` skips users with active
  community-event streams to avoid duplicate notifications. The
  community is soft-deleted by reminder time, so streams should
  already be closed via #1658. The check is still cheap and
  defensive — apply it.
- **Notification preferences.** The day-before-purge reminder is
  *not* user-toggleable per design (§8 lists no category). Skip
  category gating — the reminder always fires for snapshot members.
  Document the rationale in the job comment so a future contributor
  doesn't add a "disable purge reminders" toggle.

What's missing and must be built:

- **Proto field.** Add `optional int64 purge_reminder_sent_at_unix_sec = 22`
  to `Community` for idempotency. Server-only field; not exposed on
  the API. Wire-compatible with all existing rows (default 0 = unset).
- **Storage helper.** `ClaimCommunityPurgeReminder(ctx, communityID,
  nowUnixSec) (claimed bool, err error)` — conditional UPDATE that
  sets `purge_reminder_sent_at_unix_sec = $now` only if the row is
  currently soft-deleted AND the field is currently unset/zero. Same
  shape as `ClaimCommunityRestore` / `ClaimCommunityOwnerHandoff` /
  `ClaimCommunityUserRejoin`. Returns true exactly once per
  community across concurrent job runs.
- **Job:** `server/jobs/community_purge_reminder.go`,
  `CommunityPurgeReminderJob` struct with `Run(ctx) error`. Queries
  for soft-deleted communities crossing the 29-day mark, claims
  each, sends pushes, logs results.
- **Daily ticker wiring** in `server/main.go`: a goroutine that
  triggers `Run` once at startup (catch-up) and then on a 24h
  ticker. Mirrors the db pool stats publisher pattern but at a
  much longer cadence.
- **Job test:** integration test in
  `server/jobs/community_purge_reminder_test.go`: seed a community
  with `Deleted.DeletedAtUnixSec = now - 29.5 days`, run the job,
  assert the push lands and `purge_reminder_sent_at_unix_sec` is
  populated. Re-run the job, assert no second push.
- **ARB strings.** No client UI text changes needed — the push is
  server-rendered into `*models.Notification.Title/Body`, same as
  the existing four community-lifecycle pushes. **(Updated from
  the issue body's mention of ARB strings: server-rendered push
  copy isn't localised today, matching the precedent set by
  COMMUNITY_DELETED / COMMUNITY_RESTORED / OWNERSHIP_TRANSFERRED in
  `notifications.go:buildNotification`. Filing a follow-up to
  localise all five together is the right move; doing it
  inconsistently for just the new fifth one would be confusing.)**

## Potential Solutions

### Option A: In-process daily ticker + Community proto field (recommended)

- **Approach:** add `Community.purge_reminder_sent_at_unix_sec`,
  `ClaimCommunityPurgeReminder` storage helper,
  `CommunityPurgeReminderJob` in `server/jobs/`, a 24h ticker
  goroutine in `server/main.go` that runs the job. The push is
  built inline in the job; no `CommunityEvent` row, no copy added
  to `notifications.go:buildNotification`.
- **Pros:**
  - Matches the codebase's existing job + ticker pattern (db pool
    stats, impact backfill, community cascade backfill).
  - `min_instances = 1` makes the in-process ticker reliable; the
    server is always running, the goroutine survives.
  - The proto field is idempotent across job runs and across server
    restarts — set-once semantics via the conditional UPDATE.
  - Race-safe by the same pattern used everywhere else in this
    codebase.
- **Cons:**
  - Adds one new proto field (wire-compatible).
  - Daily ticker requires server uptime to fire; if the server is
    down for 24+ hours the reminder window can be missed for some
    communities (mitigated below).
- **Effort:** medium. ~400 lines including tests.

### Option B: External cron (Cloud Scheduler → endpoint)

- **Approach:** add an admin/cron HTTP endpoint that triggers the
  job; configure Cloud Scheduler in terraform to hit it daily.
- **Pros:**
  - Independent of in-process state; survives instance restarts
    cleanly.
- **Cons:**
  - Adds Cloud Scheduler resource + IAM + auth on the cron endpoint.
  - Diverges from the existing job pattern in this codebase.
  - Heavier to ship, debug, and revert.
- **Effort:** medium-large. Rejected as over-engineering for a
  daily-cadence job under `min_instances = 1`.

### Option C: Catch-up on startup, no ticker

- **Approach:** the job runs once on every server start; if the
  server restarts more than once per day the job runs more than
  once that day, but the storage idempotency covers it. No 24h
  ticker.
- **Pros:** minimal scaffolding.
- **Cons:**
  - Servers don't restart daily in steady state. A reminder due
    today won't fire if no deploy happens today.
  - The whole point of the reminder is that the user needs to know
    *the day before purge*, so missed days are user-visible.
- **Effort:** smallest. Rejected as insufficient for the §8 promise.

## Recommendation

**Option A.** Matches the codebase pattern, leverages the always-on
deployment, and the conditional-UPDATE primitive matches every
other Phase 2 race-safety primitive. Catch-up at startup + 24h
ticker covers the deploy-day case and the steady-state daily
cadence in one mechanism.

## Open Questions

- **`purge_reminder_sent_at_unix_sec` on `Community` vs. on
  `Community.Deleted` (DeletedMetadata)?** —
  *Recommendation:* directly on `Community` (field 22), not nested
  inside `DeletedMetadata`. The metadata struct is shared across
  many entity types and is the wrong place for a community-specific
  field. Cleared on restore alongside `Deleted` and
  `DeletedSnapshot`.
- **What about communities deleted >30 days ago that never got the
  reminder (e.g. server was down for 2+ days)?** —
  *Recommendation:* the job's WHERE clause should find any
  soft-deleted community with `purge_reminder_sent_at_unix_sec = 0`
  AND `deleted_deleted_at_unix_sec <= now - 29 days`. This catches
  both today's-29-day-mark communities AND any stragglers from a
  longer outage. Cap at, say, 1000 per run to avoid runaway batches.
  Send the reminder; the user can still restore until #1620's
  hard-delete fires.
- **Should we record a `CommunityEvent` for audit when the reminder
  fires?** —
  *Recommendation:* no. The proto field
  `purge_reminder_sent_at_unix_sec` IS the audit record. Adding an
  event type would require an enum value, undo classification,
  copy in `buildNotification` (which we'd never use because we
  dispatch directly), and an extra row per community per purge
  cycle. Logs already capture per-community per-recipient
  dispatches; that's sufficient for support.
- **Should we suppress the reminder for users with active streams
  on the (deleted) community?** —
  *Recommendation:* no, fire unconditionally. The community is
  soft-deleted, so per #1658 any active streams already terminated.
  Even if a stream was somehow still live, the reminder is the
  user-visible promise — pushing is the right behavior.
- **Should we run the job on every server start?** —
  *Recommendation:* yes, plus the 24h ticker. The conditional UPDATE
  guarantees no double-dispatch. Catching up on startup covers the
  deploy-day window without complicating the scheduler.
- **What if `DeletedSnapshot.MemberUserIds` is empty (snapshot was
  taken on a 0-member community)?** —
  *Recommendation:* send no pushes (nothing to do), but still set
  `purge_reminder_sent_at_unix_sec` so the job doesn't re-process
  it tomorrow. Log at INFO with `recipient_count=0` for
  observability.
- **L10n: localize the day-before-purge string?** —
  *Recommendation:* no — for consistency with the four other
  community-lifecycle pushes which are not localised today. File a
  follow-up to localise all five together if it becomes a priority.

## Implementation Plan

### Phase 1: Proto field + storage primitive

**Goal:** the data model and race-safe primitive land first with
their own coverage.

**Tasks:**
- [ ] Add `optional int64 purge_reminder_sent_at_unix_sec = 22` to
  `Community` in `proto/ripls/models/community.proto`. Documented
  as server-only, set by the daily reminder job, cleared on
  restore.
- [ ] In `server/services/community/lifecycle.go:RestoreCommunity`,
  clear `community.PurgeReminderSentAtUnixSec = 0` alongside
  `community.Deleted = nil` and
  `community.DeletedSnapshot = nil` so a delete→restore→delete
  cycle gets a fresh reminder window.
- [ ] Add `ClaimCommunityPurgeReminder(ctx, communityID,
  nowUnixSec) (claimed bool, err error)` in
  `server/storage/community.go`. Conditional UPDATE that succeeds
  iff (a) community is currently soft-deleted, (b)
  `purge_reminder_sent_at_unix_sec` is currently 0/NULL. Sets the
  flat column to `nowUnixSec`. Mirrors the
  `ClaimCommunityRestore` / `ClaimCommunityUserRejoin` pattern.
- [ ] Storage helper `FindCommunitiesNeedingPurgeReminder(ctx,
  nowUnixSec, leadSeconds, limit) ([]*models.Community, error)`
  — query for soft-deleted communities where
  `now - deleted_at >= leadSeconds` AND
  `purge_reminder_sent_at_unix_sec = 0`. Bounded by `limit` to cap
  per-run batch size.
- [ ] Constants in `server/community/constants.go`:
  `PurgeReminderLeadDays = RejoinWindowDays - 1` (i.e. fire at
  29 days),
  `PurgeReminderLeadSeconds = PurgeReminderLeadDays * 24 * 60 * 60`,
  `PurgeReminderBatchSize = 1000`.
- [ ] Storage unit tests:
  - `ClaimCommunityPurgeReminder` succeeds on a soft-deleted
    community with unset reminder field.
  - Idempotent: second claim returns false.
  - Fails on active community (not soft-deleted).
  - Fails when reminder field is already set.
  - `FindCommunitiesNeedingPurgeReminder` finds rows in the right
    deleted-at window, skips active communities, skips
    already-reminded communities, respects limit.

**Validates:** the data model and primitive give clean
exactly-once semantics across concurrent job runs.

### Phase 2: Daily reminder job

**Goal:** the job that consumes the primitives and dispatches
pushes.

**Tasks:**
- [ ] Add `server/jobs/community_purge_reminder.go` with
  `CommunityPurgeReminderJob` struct holding storage +
  `notifications.Service`. Constructor
  `NewCommunityPurgeReminderJob`.
- [ ] `Run(ctx)` method:
  1. Logger setup with `operation=CommunityPurgeReminderJob`,
     `now`. Emit start INFO with batch size.
  2. Call `FindCommunitiesNeedingPurgeReminder` for the current
     timestamp.
  3. For each candidate:
     - `ClaimCommunityPurgeReminder(ctx, communityID, now)`. On
       loss, log DEBUG and skip (another runner won the race).
     - Build `*models.Notification` with title
       `"<name> will be deleted tomorrow"` and body
       `"<name> will be permanently deleted tomorrow. Tap to
       restore."` Match the existing copy style.
     - For each user in `community.DeletedSnapshot.MemberUserIds`,
       call `notificationService.NotifyUser`. Log per-user
       failures at WARN with `community_id` and `user_id`; do not
       fail the whole job on individual push failures.
  4. Emit a summary INFO with
     `communities_processed`, `pushes_attempted`,
     `pushes_failed`.
- [ ] Use `connecterr.Internal` semantics not strictly needed (no
  RPC), but wrap top-level storage errors with `fmt.Errorf` per
  the existing job convention.
- [ ] Unit/integration test in
  `server/jobs/community_purge_reminder_test.go`:
  - Seed three communities: one deleted 29.5 days ago, one
    deleted 28 days ago, one active. Run; assert exactly the
    29.5-day-old community got a push to all snapshot members
    and its reminder field is set.
  - Re-run; assert no second push (idempotent).
  - Empty snapshot: seed a community deleted 29.5 days ago with
    `DeletedSnapshot.MemberUserIds = nil`. Run; assert reminder
    field is set, zero pushes dispatched.
  - Restore-mid-window: seed a soft-deleted community, run the
    job (sets reminder field), then call `RestoreCommunity` (should
    clear the field per Phase 1 task), then re-soft-delete and
    run the job again — should re-send the reminder. Validates
    the cycle.

**Validates:** the job dispatches the right pushes for the right
recipients exactly once per community per delete cycle.

### Phase 3: Wire into server startup + daily ticker

**Goal:** the job runs on the server.

**Tasks:**
- [ ] In `server/main.go` near the existing job spawns
  (~line 870), add a `go func() { ... }()` block that:
  1. Recovers from panics like the other job spawns.
  2. Runs the job once at startup (catch-up).
  3. Then enters a `for { select { case <-ctx.Done(): ; case
     <-ticker.C: ... } }` loop with `time.NewTicker(24 * time.Hour)`.
- [ ] Verify the job doesn't double-dispatch under a fast restart
  loop (idempotency covers this; document expectation in the
  ticker block's comment).
- [ ] Mention the new job in `server/jobs/README.md`'s "Key files"
  list with a one-line description.
- [ ] System test verification: not needed (no client surface
  changes); the integration test in Phase 2 covers the wire
  semantics.

**Validates:** the job actually fires daily in production and at
startup. Acceptance is observed via Cloud Logging entries with
`operation=CommunityPurgeReminderJob`.

## Audit Notes

Reviewed against `docs/server/architecture.md` and
`docs/server/observability.md`. Findings folded into the plan above:

- **Goroutine safety (architecture §Goroutine Safety).** The plan's
  Phase 3 ticker goroutine in `server/main.go` should follow the
  existing pattern at `server/main.go:895` (db pool stats publisher)
  and `:870` (impact backfill) — both use inline `defer recover()`.
  `GoSafe` is the more idiomatic option per architecture §Goroutine
  Safety, but matching the pattern of its peers in main.go is
  preferable for review-time consistency. Either is CI-passing
  per `check-goroutines`. **Pinned the inline-recover form** in
  Phase 3.
- **Logging conventions (observability §Standard Field Names).**
  Pinned the field names every job log line should carry:
  `operation=CommunityPurgeReminderJob`, plus per-iteration
  `community_id`, `recipient_count`, and per-failure `user_id`.
  No `request_id` in scope (jobs are not request-scoped).
  Top-of-job: `start_unix_sec`,
  `lead_seconds=PurgeReminderLeadSeconds`,
  `batch_limit=PurgeReminderBatchSize`. Summary at end:
  `communities_processed`, `pushes_attempted`, `pushes_failed`.
- **Storage primitive shape.** `ClaimCommunityPurgeReminder` and
  `FindCommunitiesNeedingPurgeReminder` should query against the
  flat `deleted_deleted_at_unix_sec` and
  `purge_reminder_sent_at_unix_sec` columns directly, not via the
  read filter. The `FindCommunities*` query needs explicit
  `WHERE deleted_deleted_at_unix_sec > 0` to skip active rows
  since this is the rare query that *opts in* to deleted rows.
  Document the query in the helper comment so future readers
  don't accidentally route through `QueryByField` (which strips
  deleted rows by default).
- **No `connecterr.Internal` needed.** The job runs outside the
  RPC pipeline; errors stay in `fmt.Errorf("...: %w", err)` shape
  per the existing `ImpactBackfillJob` and
  `CommunityCascadeBackfillJob` precedent. Top-level errors are
  logged at ERROR by the goroutine wrapper in `main.go` if
  non-context-canceled; per-iteration failures stay at WARN and
  the loop continues.
- **No new event types.** Recommendation in Open Questions
  confirmed — no `COMMUNITY_EVENT_TYPE_PURGE_REMINDER_SENT` enum
  value. Adding one would require an undo registry classification
  (`ClassificationInternal` per the system-emitted pattern) for
  no real audit benefit; the proto field
  `purge_reminder_sent_at_unix_sec` IS the audit record.
- **Notification dispatch path.** The job calls
  `notificationService.NotifyUser` directly (not through
  `RecordCommunityEventAndNotify`) so the dispatcher's
  deleted-community guard never runs. This is intentional — the
  reminder must fire for soft-deleted communities. Document this
  bypass clearly in the job comment so a future contributor
  doesn't try to "centralize" community-scoped pushes through the
  dispatcher.
- **Push category gating.** The job does NOT consult
  `CommunityNotificationPreferences` because there is no §8
  category for this push. Direct `NotifyUser` skips the category
  filter entirely. **Pinned in the job comment.** If a future
  preference toggle becomes desirable, that's a follow-up
  decision; it isn't a Phase 2 requirement.
- **Stream suppression.** Skipped by recommendation — the
  community is soft-deleted by reminder time, so #1658 has
  already terminated any active streams. Even if a stream is
  somehow live, we want the push to fire (the design's
  "reversible up to the last moment" promise).
- **L10n boundary (i18n.md).** Confirmed: the existing four
  community-lifecycle pushes
  (`COMMUNITY_DELETED`/`COMMUNITY_RESTORED`/`OWNERSHIP_TRANSFERRED`
  + `MEMBER_LEFT`) all use server-side English strings in
  `notifications.go:buildNotification`. Localising just the new
  fifth one would be inconsistent. **Filed as follow-up:**
  localise all five together, plus any other server-emitted push
  copy, in a dedicated PR. Not in scope here.
- **Test isolation (architecture §7).** Use
  `storage.SetupTestStorage(t)` + `t.Cleanup(cleanup)` per the
  established pattern. Mock notification service via
  `notifications.NewMockService()` to capture pushes for
  assertions.
- **Job test location.** `server/jobs/community_purge_reminder_test.go`
  matches the existing per-job `*_test.go` pattern in
  `server/jobs/`.

The plan is now consistent with `docs/server/architecture.md` and
`docs/server/observability.md`. Ready for implementation.

## Related Issues

- **#1619** — parent epic.
- **#1620** — Phase 4 daily purge job (the actual hard-delete). Day-
  before-reminder ships here (Phase 2); hard-delete ships there.
  Until #1620 lands, the reminder fires today and the community
  survives until manual intervention or restore.
- **#1622** — Phase 1 notification dispatcher guard. The
  day-before-purge push doesn't go through the dispatcher (it's
  dispatched directly from the job), so the deleted-community guard
  doesn't apply. Documented in the job comment.
- **#1654** — DeleteCommunity rewrite (the emission point that
  populates `DeletedSnapshot.MemberUserIds`).
- **#1655** — RestoreCommunity (must clear
  `purge_reminder_sent_at_unix_sec` on restore — Phase 1 task).
