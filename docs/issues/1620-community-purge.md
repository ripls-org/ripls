---
issue: 1620
title: 'Daily community purge job (hard-delete soft-deleted communities and memberships)'
---

# Issue #1620: Daily community purge job (hard-delete soft-deleted communities and memberships)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1620
**Filed:** 2026-04-30 | **Last activity:** 2026-04-30
**Labels:** none | **Assignees:** briangbrown
**Context docs:** docs/community_delete_and_leave.md (§§ 6.6, 10)

## Phase 1 reconnaissance findings (2026-05-06)

Recon during Phase 1 implementation surfaced four corrections to
the plan:

a. **`FeedItemView`, not `feed_entry`.** The only persistent
   feed-related storage is `FeedItemView` (table name
   `"FeedItemView"`, model: `models.FeedItemView`). Per-row
   `community_id`. Helper renamed to
   `HardDeleteFeedItemViewsByCommunityID`; index renamed to
   `idx_feeditemview_community`.

b. **Reminder claim is a column, not a row.** #1659 tracks
   purge-reminder dispatch state via the
   `purge_reminder_sent_at_unix_sec` *column* on the `community`
   table itself (see `server/storage/community.go:413
   ClaimCommunityPurgeReminder`). When the community is
   hard-deleted, the column goes with it. Audit finding #5
   (originally requiring
   `HardDeleteCommunityPurgeReminderClaimByCommunityID`) is
   withdrawn.

c. **`ChatMessage` has no `community_id`.** Messages reference
   `conversation_id`. Hard-delete by community is a two-step:
   first query the chat_conversations for that community to get
   their IDs, then `DeleteByFieldIn(ctx, "chat_message",
   "conversation_id", convIDs)`, then delete the conversations
   themselves. The helper signature stays the same;
   implementation does the two-step internally.

d. **Transaction support added in Phase 1** (was originally
   flagged as a Phase 2 blocker). `server/storage/tx.go`
   exposes `WithTx(ctx, opts, fn func(*ProtoSQLStorage)
   error) error` using the canonical Go idiom: deferred
   `Rollback` (safe after `Commit` because `sql.ErrTxDone` is
   discarded), context-aware `BeginTx`, panic-safe via the
   defer chain. The derived `*ProtoSQLStorage` handed to fn
   routes every query through an `InstrumentedTx` wrapper so
   transactional queries flow through the same per-request
   stats counter as non-transactional ones. Reusable for
   #517 GDPR work scoped by user_id. Phase 2's per-community
   atomicity safeguard now has the primitive it needs.

## Audit findings (2026-05-06)

Reviewed against `docs/server/{architecture,observability}.md`. Most of
the plan tracks the conventions established in #1656/#1659 server work.
Changes applied below:

1. **`logging.GoSafe` for the daily ticker.** Per architecture.md
   "Goroutine Safety", every goroutine spawned in `server/` must be
   wrapped with `logging.GoSafe(ctx, "name", fn)` to recover from
   panics; CI enforces it via `server/cmd/check-goroutines`. The
   plan now calls this out explicitly for the `time.NewTicker` loop
   in `main.go`.

2. **Transaction pattern named.** Per architecture.md storage best
   practices the per-community purge uses
   `tx, err := s.storage.BeginTx()`, `defer tx.Rollback()`,
   `tx.Commit()`. Plan was vague ("BEGIN tx, COMMIT") — now matches
   the documented pattern.

3. **Log levels broken out.** Per observability.md: per-tick
   summary at INFO, per-community success at INFO (one line),
   per-community failure at ERROR (with `community_id` + `error`).
   "ERROR reserved for system failures" — a community whose purge
   transaction rolls back is exactly that. Top-level summary uses
   `error_count` not `errors`.

4. **Field naming consistency.** Per-table row-count keys all use
   the `<table>_rows_deleted` suffix (`gear_rows_deleted`,
   `request_rows_deleted`, …) so log queries like
   `jsonPayload.gear_rows_deleted > 0` work uniformly. Job-level
   keys: `community_id` (existing standard),
   `communities_purged`, `memberships_purged`, `error_count`,
   `duration_ms`.

5. **Reminder-claim cascade.** #1659's
   `ClaimCommunityPurgeReminder` writes per-community claim state
   (`storage.ClaimCommunityPurgeReminder` row). When the community
   is hard-deleted that row dangles unless we cascade it. Phase 1
   adds `HardDeleteCommunityPurgeReminderClaimByCommunityID` and
   the integration test asserts the claim row is gone too.
   Without this, an operator running the migration will see
   orphan claim rows accumulate forever.

6. **`feed_entry` table-name verification.** Recon found
   `Feed.community_id` in the proto but no corresponding
   `feed_entry` row in `protosql_schema.go`'s
   `tablesAndIndexes` slice. Phase 1's first task is to verify
   the actual feed-table name (likely just `feed`) and confirm
   the cascade scope. If feed entries are not stored as their
   own rows but rolled into something else, the helper isn't
   needed; if they are, the cascade list updates accordingly.
   Either way the integration test pins what gets removed.

7. **Test setup helper.** Per CLAUDE.md the recommended pattern
   is `storage.SetupTestStorage(t)` for unit tests; the plan
   now names this helper explicitly.

8. **`storage.PurgeWindowSeconds` constant.** The reminder job
   uses `communitylib.PurgeReminderLeadSeconds`. The purge job
   should pull from a sibling constant — recommendation:
   `communitylib.PurgeWindowSeconds` (= 30 days in seconds).
   If the constant doesn't exist yet, Phase 1 adds it next to
   the existing `RejoinWindowSeconds`. Keeps the magic number
   out of the job.

## Problem Statement

#1619's soft-delete design hides communities (and their cascade) for 30 days
after `DeleteCommunity` or sole-member `LeaveCommunity`, then promises a hard
purge on day 31. Same window applies to individual memberships: a leaver has
30 days to rejoin via `RejoinCommunity`; after that the soft-deleted
`CommunityUser` row should be physically removed.

Today nothing ever hard-deletes those rows. Every soft-delete is permanent
clutter. With #1619 about to ship to production, this becomes the gap
between what we promise users ("we'll permanently delete on day 31") and
what the database does (keeps everything forever).

The job needs to:

1. Hard-delete `Community` rows whose `deleted.deleted_at_unix_sec < now -
   30 days`, plus every cascade table scoped to that community: ten join
   tables under `server/storage/`, plus chat conversations + messages,
   stories, nudges, and feed entries.
2. Hard-delete `CommunityUser` rows whose `deleted.deleted_at_unix_sec <
   now - 30 days` independent of the community itself (a member's rejoin
   window expired, but the community is still active).
3. Run idempotently on a 24-hour tick.

## Context from Comments

No comments. Body is detailed enough to plan from. The interesting
ambiguity to flag in Open Questions: the issue lists 12 cascade tables
without specifying *order* (foreign-key constraints matter for hard
delete) and lists "feed entries" without a confirmed schema reference;
recon shows `Feed.community_id` exists (`proto/ripls/models/feed.proto`)
but no community-scoped index in `protosql_schema.go` — so the job's
purge query against feed will table-scan unless indexed.

## Codebase reconnaissance

- **No formal scheduler abstraction.** Each daily job spawns its own
  goroutine + `time.NewTicker(interval)` and is registered in
  `server/main.go`. The closest precedent is the day-before-purge
  reminder shipped via #1659 / `server/jobs/community_purge_reminder.go`,
  registered at `main.go:922-945`. Mirror that pattern.

- **Soft-delete cascade helpers already exist** in
  `server/storage/cascade_delete.go` for community-scoped tables:
  `CascadeDeleteCommunityGearByCommunityID:316`,
  `…RequestByCommunityID:348`, `…ExperienceByCommunityID:380`,
  `…NotificationPreferencesByCommunityID:412`,
  `…InvitationLinkByCommunityID:445`. Pattern: set `DeletedMetadata` on
  every join row scoped to the community ID. **None of these are
  hard-delete helpers** — the issue body is explicit that "new helpers
  (or new mode) needed for hard-delete cascading on community ID."

- **Storage primitives for hard-delete are ready.**
  `server/storage/protosql.go` exposes `Delete(ctx, msg)` (single,
  line:412), `DeleteByField(ctx, table, field, value)` (line:463), and
  `DeleteByFieldIn(ctx, table, field, values)` (line:484). All return
  `rowsAffected`. These are the building blocks the new helpers will
  call.

- **Soft-delete schema.** `proto/ripls/models/community.proto:54` has
  `optional DeletedMetadata deleted = 20;`. `DeletedMetadata`
  (`proto/ripls/models/common.proto:9-15`) carries
  `deleted_by_user_id` and `deleted_at_unix_sec`. Same shape on
  `CommunityUser` (community.proto:101). **No SQL index on
  `deleted_at_unix_sec`** in `server/storage/protosql_schema.go` —
  the daily WHERE clause will scan, which is fine at low volume but a
  partial index `WHERE deleted IS NOT NULL` would be cheap and right.

- **Per-community-ID columns confirmed** for: `community_user`,
  `community_gear`, `community_request`, `community_experience`,
  `community_notification_preferences`, `community_invitation_link`,
  `community_event`, `community_region`, `chat_conversation`, `story`,
  `stored_nudge`. Feed entries scope by `Feed.community_id` (proto)
  but the table is not indexed on it.

- **Logging convention.** Mirror
  `server/jobs/community_cascade_backfill.go:41-48` — single
  structured log line per community processed with one key per table
  carrying its row-count.

- **GDPR / account-deletion (#517)** wants the same machinery,
  scoped by user-ID instead of community-ID. The storage helpers
  added here should be designed so a future hard-delete-by-user-ID
  variant can call the same primitives.

## Potential Solutions

The shape is fixed by spec. The real choices are how to organize
the cascade helpers and how to structure the job.

### Option A: Per-table hard-delete helpers + thin orchestrator

- **Approach:**
  - Add `server/storage/hard_cascade_delete.go` with one helper per
    cascade table, mirroring the soft-delete helper signatures:
    `HardDeleteCommunityGearByCommunityID(ctx, communityID) (int64, error)`,
    etc. Each calls `DeleteByField` directly.
  - Cover: `community_user`, `community_gear`, `community_request`,
    `community_experience`, `community_notification_preferences`,
    `community_invitation_link`, `community_event`,
    `community_region`, `chat_conversation` (which itself cascades
    to `chat_message`), `story`, `stored_nudge`, `feed_entry`.
  - Add `server/jobs/community_purge.go` with two entry points:
    `PurgeExpiredCommunities(ctx) (int, error)` and
    `PurgeExpiredMemberships(ctx) (int, error)`. The first iterates
    expired communities, calls every helper, then deletes the
    `Community` row last. The second hard-deletes expired
    `CommunityUser` rows whose parent community is still active
    (the per-member rejoin-window-expired case).
  - Single 24h ticker in `server/main.go` invokes both in sequence.
  - Idempotency: by definition. After a successful purge the
    rows are gone, so a second run finds zero matches. No claim
    table needed.
- **Pros:** every helper is independently testable; signatures
  match the existing soft-delete cascade pattern so a reviewer can
  read both files in parallel; helpers are reusable for #517's
  user-deletion path.
- **Cons:** 12 tiny helpers is a lot of surface; a missed table
  silently leaks rows. Mitigated by an integration test that
  creates fixtures across every cascade table and asserts row
  count = 0 after purge.
- **Effort:** medium (~600 LOC including tests).

### Option B: Single `HardDeleteCommunity(ctx, communityID)` god-function

- **Approach:** one helper that runs all cascade table deletes
  internally, returning a per-table count map. Job calls it once
  per expired community.
- **Pros:** smaller call site; no chance of "forgetting" to call a
  helper at the orchestrator level.
- **Cons:** harder to reuse for #517's user-deletion path (it
  scopes by user, not community). Tests have to assert a map
  instead of independent helpers. Single function balloons over
  time as new community-scoped tables are added.
- **Effort:** small-medium.

### Option C: Generic `HardDeleteCascade` registry

- **Approach:** introduce a registry of `(table, foreignKey)`
  pairs and a single `HardDeleteCascade(ctx, communityID)` that
  iterates the registry. Adding a new community-scoped table
  becomes a one-line registry append.
- **Pros:** very DRY; new tables are append-only.
- **Cons:** indirection at a layer that doesn't need it yet (we
  have ~12 tables, not 50); mismatch with existing soft-delete
  helper convention; harder to grep for who deletes what.
- **Effort:** medium, but the abstraction is premature.

## Recommendation

**Option A.** Matches the existing soft-delete cascade helper
convention 1:1 — easy review, easy grep ("which helper handles
chat?"). Helpers can be reused for #517 once GDPR work starts.
Idempotency falls out naturally; no claim table.

The 12-helper sprawl is the real risk. Mitigate with an
integration test that:

1. Creates a community plus one row in every cascade table.
2. Soft-deletes the community with `deleted_at_unix_sec = now -
   31 days`.
3. Runs the job.
4. Asserts every cascade table is empty AND that an unrelated
   community's rows are untouched.

That single test pins the "forgot a helper" failure mode for the
life of the job.

## Open Questions

- **Order of hard-deletes.** Recommendation: child rows before
  parent. Concretely: chat_message before chat_conversation;
  every join table before `community`. PostgreSQL won't enforce
  cascade FK order automatically because we don't model FK
  constraints in protosql; but the order matters for the *story*
  we tell when a partial failure happens — children should be
  gone first so a retry after a mid-job crash doesn't leave
  dangling join rows pointing at a deleted parent.

- **Single transaction or many?** Recommendation: one transaction
  per community (BEGIN, run all helpers, COMMIT). Per-table
  separate transactions risk a half-purge if the process crashes.
  Per-community keeps the row count bounded — even a community
  with 10k gear shares fits comfortably in a single transaction.
  The membership-only purge (Option A's second function) uses a
  single batched delete with no FK dependencies; one transaction
  is fine.

- **Volume bound.** Recommendation: process up to 100 expired
  communities per tick. Anything more is a backlog signal worth
  paging on; better to surface as a `purge_backlog` metric than
  silently grind through 10k rows. Same cap on the membership
  purge (`DELETE … LIMIT 100`).

- **Time of day.** Recommendation: tick from server start, just
  like CommunityPurgeReminderJob. Locking it to a specific clock
  time (3 AM UTC etc.) needs a scheduling abstraction that
  doesn't exist yet. Defer.

- **Index on `deleted.deleted_at_unix_sec`.** Recommendation: add
  a partial index on both `community(deleted_at_unix_sec) WHERE
  deleted IS NOT NULL` and `community_user(deleted_at_unix_sec)
  WHERE deleted IS NOT NULL`. The daily query is `WHERE deleted
  IS NOT NULL AND deleted_at_unix_sec < ?` and at any non-trivial
  scale a sequential scan is slow. Cheap to add now.

- **Feed entries `community_id` index.** Recommendation: add
  `CREATE INDEX IF NOT EXISTS idx_feed_entry_community ON
  "feed_entry" (community_id)` alongside the helper. Without it,
  `HardDeleteFeedEntriesByCommunityID` scans the entire feed
  table once per expired community.

- **Day-before-purge reminder alignment.** Recommendation: leave
  alone. The reminder job (#1659 / `community_purge_reminder.go`)
  already runs on its own 24h tick at `now + 24h` from the soft-
  delete; the purge job runs at `now + 30d`. The two are
  independent — there's no shared state to coordinate. If the
  reminder fires at hour T and the purge fires at hour T+24,
  that's the spec.

- **Does the membership purge ever delete an active member?**
  Recommendation: no — the WHERE clause is `WHERE deleted IS
  NOT NULL AND deleted_at_unix_sec < ?`. An active membership
  has `deleted` unset. But the test should pin this explicitly.

- **What about communities whose deleted-at is in the future
  (clock skew)?** Recommendation: ignore them. The query
  `WHERE deleted_at_unix_sec < now - 30d` excludes them
  naturally. Future-dated rows will eventually age in.

## Safeguards

Hard delete is irrecoverable. A single bug that flips active
communities into soft-deleted state could cascade into permanent
data loss within 24 hours. The plan defends against that with
five gates, each of which alone is sufficient to stop a runaway
delete:

### 1. Threshold gate

Per tick, compute
`threshold = min(100, max(5, 0.05 * total_communities))` —
the lesser of the absolute cap (100, also the per-tick `LIMIT`)
and 5 % of total communities, with a 5-row absolute floor for
tiny databases. If `len(candidates) > threshold`, log at
`ERROR`:

```
operation=PurgeCommunityRun
candidate_count=<n>
total_communities=<n>
threshold=<n>
aborted=true
reason=threshold_exceeded
```

…and skip the entire run. The ERROR-level log triggers
observability.md's existing "Server Error Logged" alert
(rate-limited to 1 per 5 min). Operator investigates before
the next tick.

### 2. Dry-run mode

Server flag `--community-purge-dry-run` (default false). When
set, the job runs the SELECT and the cascade helpers inside
`BeginTx()` to get real per-table row counts, then **always
`tx.Rollback()` instead of `tx.Commit()`** — no rows are
actually deleted. Logs include `dry_run=true` on every line.
Used for the first prod week to confirm no surprises before
flipping to live deletes. Tests pass the value directly via
the `NewCommunityPurgeJob(s, disabled, dryRun)` constructor.

### 3. Kill switch

Server flag `--community-purge-disabled` (default false). When
set, the job logs once per Run (`operation=PurgeCommunityRun
disabled=true`) and returns immediately. Lets ops stop the
job mid-incident with a config flip and restart, without a
redeploy. Tests pass the value directly via the constructor.

### 4. Audit table

New table `community_purge_audit` written **before** the
delete inside the per-community transaction:

```
community_id        TEXT PRIMARY KEY
name                TEXT
deleted_at_unix_sec INT8
purged_at_unix_sec  INT8
snapshot_member_user_ids JSONB  -- from Community.deleted_snapshot
per_table_row_counts JSONB      -- {"gear": 12, "request": 3, ...}
```

Survives the hard delete. Lets a future operator reconstruct
"who had access, what was attached" forensically. Combined
with PostgreSQL point-in-time-recovery backups, this is the
difference between "lost their community forever" and
"rebuilt it within an hour." Audit rows are themselves
retained — no automatic cleanup; volume is small (one row
per purge event).

### 5. Per-row re-check before delete

Inside each per-community transaction, immediately before
running the cascade helpers, re-`SELECT … FOR UPDATE` the
community row and verify
`deleted IS NOT NULL AND deleted_at_unix_sec < now -
PurgeWindowSeconds`. If the check fails (a `RestoreCommunity`
slipped in between candidate selection and cascade), log at
`WARN` (`operation=PurgeCommunity restore_race=true
community_id=...`), `tx.Rollback()`, continue to the next
community.

### Defense-in-depth stack

```
candidates = SELECT ... LIMIT 100
   ↓
if --community-purge-disabled → log + skip ticker iteration
   ↓
if len(candidates) > threshold → log ERROR, skip entire run
   ↓
for each community:
   BeginTx()
   re-SELECT community FOR UPDATE, verify still expired
   run cascade helpers (per Phase 1)
   INSERT into community_purge_audit (with per-table counts)
   if --community-purge-dry-run → tx.Rollback() (always)
   else → tx.Commit()
```

Considered and excluded:

- **Two-phase "to_purge" state** — the 30-day window is the
  soft fence; a second 1-day fence is over-engineering.
- **Time-of-day window** — needs Cloud Scheduler infra; defer
  alongside the wall-clock-vs-uptime recommendation in
  Phase 3.
- **`pg_dump`-before-delete** — operationally complex and
  GCP-specific. The audit table covers 90 % of the value at
  10 % of the cost.

## Implementation Plan

### Phase 1: Hard-delete cascade helpers + indexes

**Goal:** every community-scoped table has a
`HardDelete{Table}ByCommunityID` helper returning rows-affected.
Two partial indexes added so the daily WHERE clause is cheap.

**Tasks:**
- [ ] Verify the actual feed-storage table name in
  `server/storage/protosql_schema.go`'s `tablesAndIndexes`
  slice and confirm whether feed rows are scoped per
  community. If yes, include `HardDeleteFeedEntriesByCommunityID`
  in the helper set; if not, drop it from the cascade list.
- [ ] Add `communitylib.PurgeWindowSeconds = 30 * 86400` next to
  the existing `RejoinWindowSeconds` constant.
- [ ] Register the new `community_purge_audit` table in
  `server/storage/protosql_schema.go` (proto: new
  `models.CommunityPurgeAudit` with the columns listed in the
  Safeguards section). Audit rows are never soft-deleted —
  they survive the cascade.
- [ ] Add `server/storage/hard_cascade_delete.go` with one helper
  per table:
  - `HardDeleteCommunityUserByCommunityID`
  - `HardDeleteCommunityGearByCommunityID`
  - `HardDeleteCommunityRequestByCommunityID`
  - `HardDeleteCommunityExperienceByCommunityID`
  - `HardDeleteCommunityNotificationPreferencesByCommunityID`
  - `HardDeleteCommunityInvitationLinkByCommunityID`
  - `HardDeleteCommunityEventByCommunityID`
  - `HardDeleteCommunityRegionByCommunityID`
  - `HardDeleteChatMessagesByCommunityID` (via conversation lookup)
  - `HardDeleteChatConversationsByCommunityID`
  - `HardDeleteStoriesByCommunityID`
  - `HardDeleteStoredNudgesByCommunityID`
  - `HardDeleteFeedEntriesByCommunityID` *(if Phase 1's
    feed-table verification confirms it's needed)*
  - `HardDeleteCommunityPurgeReminderClaimByCommunityID` —
    cascades #1659's per-community reminder-claim row
  - `HardDeleteCommunityByID` (the parent row, last)
- [ ] Each helper uses `DeleteByField` and returns
  `(int64, error)` (rowsAffected, err).
- [ ] Add to `server/storage/protosql_schema.go`:
  - Partial indexes on `community(deleted_at_unix_sec) WHERE
    deleted IS NOT NULL` and `community_user(deleted_at_unix_sec)
    WHERE deleted IS NOT NULL`.
  - `idx_feed_entry_community` on the feed-storage table
    (only if the verification step above confirms a per-row
    `community_id` column exists).
- [ ] `server/storage/hard_cascade_delete_test.go`: per-helper
  unit tests using `storage.SetupTestStorage(t)` plus an
  `AssertMaxQueries` integration test that creates fixtures
  across every cascade table, calls every helper, and asserts
  rows-affected counts. Test must include a row in the
  reminder-claim table to pin the cascade.

**Validates:** every helper does what its name says; the schema
gains the indexes the daily query needs; a missed-helper failure
mode is pinned by the integration test.

### Phase 2: Purge orchestrator + daily job

**Goal:** community + membership purges run once per 24h on
server boot.

**Tasks:**
- [ ] `server/jobs/community_purge.go` with:
  - `func RunDailyPurge(ctx, ...) error` — top-level entry.
    First reads the kill-switch flag plumbed in via the
    `NewCommunityPurgeJob(s, disabled, dryRun)` constructor
    (Safeguard #3); if true, log once and return immediately.
  - `purgeExpiredCommunities(ctx, ...) (int, error)` — query
    `community WHERE deleted IS NOT NULL AND deleted_at_unix_sec
    < now - communitylib.PurgeWindowSeconds`. Cap at 100 per
    tick.
    - **Threshold gate (Safeguard #1).** Read
      `total_communities = COUNT(*) FROM community`. Compute
      `threshold = min(100, max(5, 0.05 * total))`. If
      `len(candidates) > threshold`, log `ERROR` with
      `aborted=true reason=threshold_exceeded` and return
      without iterating. (CI alert fires; operator
      investigates.)
    - **Dry-run flag (Safeguard #2).** Pulled from the
      job's dryRun field (set via constructor from the
      `--community-purge-dry-run` server flag). Carry through
      every log line as `dry_run=true|false`.
    - For each community:
      - `tx, err := s.storage.BeginTx()` ...
        `defer tx.Rollback()` (tx-aware variants of the
        helpers from Phase 1).
      - **Per-row re-check (Safeguard #5).** `SELECT FROM
        community WHERE id = $1 FOR UPDATE`; verify
        `deleted IS NOT NULL AND deleted_at_unix_sec <
        now - PurgeWindowSeconds`. On mismatch, log
        `WARN operation=PurgeCommunity restore_race=true
        community_id=...`, rollback, continue.
      - **Audit row (Safeguard #4).** `INSERT INTO
        community_purge_audit` with community_id, name,
        deleted_at_unix_sec, purged_at_unix_sec,
        snapshot_member_user_ids,
        per_table_row_counts (filled in after helpers run).
      - Run every helper in child-before-parent order:
        `chat_message`, `chat_conversation`, `story`,
        `stored_nudge`, `feed_entry` *(if applicable)*,
        `community_user`, `community_gear`,
        `community_request`, `community_experience`,
        `community_notification_preferences`,
        `community_invitation_link`, `community_event`,
        `community_region`,
        `community_purge_reminder_claim`, `community`.
      - Update the audit row's `per_table_row_counts`
        column with the helper-returned counts.
      - **If dry-run:** `tx.Rollback()` (audit row is rolled
        back too — that's fine; dry-run means "would have
        happened"). **Else:** `tx.Commit()`.
      - Single structured log line per community using
        `logging.LoggerWithContext(ctx)`:
        `operation=PurgeCommunity`, `community_id`,
        per-table `<table>_rows_deleted` counts,
        `duration_ms`, `dry_run`. Level: `INFO` on success.
      - On error: ROLLBACK fires from defer; log
        `operation=PurgeCommunity`, `community_id`, `error`
        at `ERROR`; continue to the next community.
  - `purgeExpiredMemberships(ctx, ...) (int, error)` — query
    `community_user WHERE deleted IS NOT NULL AND
    deleted_at_unix_sec < now -
    communitylib.RejoinWindowSeconds`. Cap at 100. Single
    batched `DeleteByFieldIn`. Log
    `operation=PurgeMemberships`, `rows_deleted`,
    `duration_ms` at `INFO`.
- [ ] Register in `server/main.go` next to the
  CommunityPurgeReminderJob (`main.go:922-945` is the
  precedent). 24h ticker, run-on-startup. **The ticker
  goroutine MUST use
  `logging.GoSafe(ctx, "community-purge", fn)`** —
  required by `server/cmd/check-goroutines` (CI gate).
- [ ] Top-level summary log line per tick using
  `logging.LoggerWithContext(ctx)`:
  `operation=PurgeCommunityRun`, `communities_purged`,
  `memberships_purged`, `error_count`, `duration_ms` at
  `INFO`.
- [ ] Integration test in `server/jobs/community_purge_test.go`:
  - Use `storage.SetupTestStorage(t)`.
  - Create active + soft-deleted communities (one expired, one
    not-yet-expired).
  - Each soft-deleted community has fixtures in every cascade
    table including the reminder-claim row.
  - Create soft-deleted memberships (one expired, one fresh) on
    a still-active community.
  - Run the job.
  - Assert: expired community + all its cascade rows gone
    (including the reminder-claim row); not-yet-expired
    community untouched; expired membership gone; fresh
    membership intact; active community untouched.
  - Assert one `community_purge_audit` row was written for the
    expired community with non-empty `per_table_row_counts`.
  - Run the job a second time — assert zero rows changed
    (idempotency).
- [ ] Integration test for **threshold gate** (Safeguard #1):
  create N communities all soft-deleted past the window where
  N > threshold; run the job; assert zero rows deleted, an
  ERROR log with `aborted=true reason=threshold_exceeded`,
  and zero audit rows.
- [ ] Integration test for **dry-run** (Safeguard #2):
  construct the job with `dryRun=true`; run; assert zero
  rows deleted, zero audit rows committed, but the log line
  carries the per-table counts that *would* have been
  deleted (proves the cascade ran inside the rolled-back
  tx). Re-run with `dryRun=false`; assert the rows are now
  actually deleted.
- [ ] Integration test for **kill switch** (Safeguard #3):
  construct the job with `disabled=true`; run; assert it
  logs once and returns immediately, no SELECT issued.
- [ ] Integration test for **restore race** (Safeguard #5):
  pick a candidate, but between candidate selection and
  cascade clear the `deleted` field on that row; assert
  the per-row re-check fires, that community is skipped
  with a `WARN restore_race=true`, and the rest of the
  batch processes normally.

**Validates:** the user-visible promise ("permanently deleted on
day 31") is now backed by code; the membership rejoin window
correctly expires; the job is idempotent; and each of the five
safeguards stops a runaway delete in its respective failure
mode.

### Phase 3: Operational verification + observability

**Goal:** confirm the job works against a real DB and the logs
are readable enough for oncall.

**Tasks:**
- [ ] **First-prod-week dry-run.** Deploy with
  `--community-purge-dry-run=true` in the prod server
  args. Wait one full 24h tick + at least one
  restart-driven catch-up. Read the resulting log lines:
  for every candidate community, the per-table counts
  should be small and look reasonable (no community with
  10k+ rows in any single table). Confirm no "would have
  been deleted" candidate looks unexpected.
- [ ] **Flip to live.** Once the dry-run logs are clean,
  remove `--community-purge-dry-run=true` from the prod
  args (or set it to false). Watch the next 24h tick.
- [ ] Run job manually against the local server with one
  pre-populated soft-deleted community older than 30 days.
- [ ] Verify the structured log line shows expected counts and
  that `community_purge_audit` has the corresponding row.
- [ ] Add a `purge_backlog` gauge or surface the
  `communities_purged_per_tick` count somewhere observable —
  even just the structured log makes it greppable for now;
  proper metric can land later. Should also surface
  `threshold_exceeded` events as a metric since they
  indicate either a bug or unexpected operator action.
- [ ] **Wall-clock follow-up note.** The plan's process-relative
  ticker pattern matches #1659 and is fine for current volume.
  Add a follow-up issue to migrate the trigger to Cloud
  Scheduler (HTTP target → `/internal/jobs/community-purge`)
  once volume warrants timing precision. The orchestrator
  function stays identical; only the trigger changes.
- [ ] Update `docs/community_delete_and_leave.md` §6.6 to drop
  the "TODO: implement" note and link to this PR.

**Validates:** the operator story for "did the purge fire today,
how much did it process" is answerable from logs.

## Related Issues

- #1619 — parent (Phase 4 of #1619).
- #517 — broader hard-delete / GDPR scope. Helpers added here
  are reusable for the user-deletion path.
- #1659 — day-before-purge reminder; lives in
  `server/jobs/community_purge_reminder.go`. Independent timer,
  no coordination needed.
- #1571 — superseded by #1656; mentioned only because the
  cascade-on-leave work that closed it set up the soft-delete
  state this job will eventually drain.
