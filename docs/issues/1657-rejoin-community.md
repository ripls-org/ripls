# Issue #1657: Phase 2 of #1619: implement RejoinCommunity end-to-end

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1657
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§§ 2.6, 2.7)

## Problem Statement

`RejoinCommunity` is the inverse of the non-owner branch of
`LeaveCommunity` (#1656): a user who left within the past 30 days
gets to walk back in without a fresh invite. It's currently a stub
returning `Unimplemented` (`server/services/community/lifecycle.go`
after the Phase 2 file move from #1656).

The acceptance contract from the issue body, in plain terms:

- **Path exists:** un-soft-delete the caller's most recent
  `CommunityUser` row in this community.
- **Window:** `now - deleted.deleted_at_unix_sec ≤ 30 days`.
- **Community must be active** (§2.7 — the rejoin clock does not
  override the community's deleted state).
- On success: clear `Deleted`, emit
  `COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW`, recompute
  regions.
- Each negative path returns `FailedPrecondition` with a distinct
  reason (`no_membership_to_rejoin`, `rejoin_window_expired`,
  `community_deleted`, `already_member`).
- Race semantics: idempotent on double-tap (recommended in the
  issue body) — surface `already_member` on second hit only when the
  row was active before the call started.

The Phase 1 `community_user_unique` constraint on
`(community_id, user_id)` means we never insert a fresh row — the
implementation always un-soft-deletes the existing one. The Phase 2
soft-delete path of #1656 is what creates the row this RPC operates
on, so the dependency is hard.

## Context from Comments

The issue has **no comments**. All context lives in the issue body
and design doc §2.6 / §2.7. Two notable nuances from the body:

- The issue explicitly recommends **idempotent on double-tap** for
  the "caller is already an active member" case, with the caveat
  "confirm in code review." I'm carrying that recommendation
  forward.
- An open question is flagged in the body: should
  `CommunityUser.created_at_unix_sec` be **updated to `now` on
  rejoin** (so Settings → Communities surfaces the rejoin date),
  or **kept as the original** (so "joined at" is historical)? The
  body leaves this for the implementer to decide.

## Codebase audit

What already exists this RPC will lean on:

- **Proto.** `RejoinCommunityRequest{community_id}` and
  `RejoinCommunityResponse{}` exist in
  `proto/ripls/api/community_service.proto:287–293`. RPC registered
  at `community_service.proto:814`.
- **Event type.** `COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW = 43`
  is already in `proto/ripls/models/community.proto`. Already
  classified in `server/undo/registry.go:306` as
  `ClassificationClientOnly` (inverse is `LeaveCommunity` within
  the same window).
- **Schema invariant.** The `community_user_unique` constraint on
  `(community_id, user_id)` was installed by Phase 1 (#1644 +
  #1652). One row per (community, user) — we un-soft-delete it,
  never insert.
- **Region recompute.** `Service.recomputeCommunityRegions` is
  already used by `LeaveCommunity` and exists on the service.
- **Race-safe claim pattern.** `ClaimCommunityRestore`
  (`server/storage/community.go:180`) and
  `ClaimCommunityOwnerHandoff` (added in #1656 Phase 1) are the
  template for the conditional-UPDATE-with-RowsAffected idiom.
  `RejoinCommunity` needs a parallel `ClaimCommunityUserRejoin`
  helper that clears the `CommunityUser.deleted` flat columns
  conditional on the row being currently soft-deleted.
- **Notification dispatcher.** `MEMBER_REJOINED_WITHIN_WINDOW` is
  not yet in `shouldNotify` — it needs an entry, but per design
  doc §8 there is **no push notification** for this event (only
  the community owner cares about MEMBER_LEFT; rejoin is a quiet
  event by design). Concretely: leave `shouldNotify` as-is,
  returning `false` for this event type.
- **`LeaveCommunity`-style branch detection.** #1656's lifecycle
  handler is the structural model — auth, fetch with IncludeDeleted,
  community-deleted FailedPrecondition, membership lookup, branch.

What's missing and must be built:

- **`ClaimCommunityUserRejoin` storage helper.** Conditional UPDATE
  on `community_user` setting `deleted_deleted_by_user_id = NULL`
  and `deleted_deleted_at_unix_sec = 0` only when the row is
  currently soft-deleted (i.e. `deleted_deleted_by_user_id IS NOT
  NULL`) and the soft-delete is within the 30-day window. Returns
  `(claimed bool, err error)`. Mirrors `ClaimCommunityRestore`.
- **Probe helpers** for the lost-claim branch:
  `ProbeCommunityUserDeletedAt` to read just the flat
  `deleted_deleted_at_unix_sec` for a given (community_id, user_id)
  pair, used to distinguish `rejoin_window_expired` from race-lost
  (`already_member`).
- **`RejoinCommunity` handler.** Replace the stub with the full
  validation + claim + resync + event sequence.
- **Constants.** A shared `RejoinWindowDays = 30` constant in
  `server/community/` so it's reused by the daily purge job
  (#1620) when that lands. Same constant should be referenced by
  the design doc copy in `notifications.go` if any new copy
  surfaces it (none in this PR).

## Potential Solutions

### Option A: Mirror #1656's structure — race-safe claim + handler in lifecycle.go (recommended)

- **Approach:** add `ClaimCommunityUserRejoin` storage helper,
  rewrite the stub handler in `lifecycle.go` to do auth → fetch
  community → reject if community deleted → fetch leaver row
  (IncludeDeleted) → branch on present/absent/active/expired → claim
  → resync proto → emit event → recompute regions.
- **Pros:**
  - Same shape as `RestoreCommunity` (#1655) and the
    `LeaveCommunity` branches (#1656); minimum cognitive overhead
    for a reviewer who has just paged that work in.
  - Atomicity comes from the conditional UPDATE; matches the
    transactionless idiom used everywhere else in this codebase.
  - The window check lives in SQL (`deleted_deleted_at_unix_sec >
    now - 30d`) so a slow handler can't open the window
    accidentally.
- **Cons:**
  - Needs a new storage helper (small, ~30 lines).
  - The window-expiry-vs-already-member disambiguation requires a
    follow-up read on claim loss. Acceptable per
    `RestoreCommunity`'s precedent.
- **Effort:** small-to-medium. ~250 lines including tests, plus the
  helper and unit coverage.

### Option B: Inline UPDATE without a dedicated helper

- **Approach:** put the conditional UPDATE inline in the handler.
- **Pros:** less code.
- **Cons:** breaks the pattern set by every other Phase 2
  community-lifecycle RPC (`ClaimCommunityRestore`,
  `ClaimCommunityOwnerHandoff`). Storage helpers are the
  established home for race-safe write primitives. Mixing
  inline-SQL-in-the-service for this one would be cute and wrong.
- **Effort:** small.

### Option C: Use storage.Update on the unmarshalled row

- **Approach:** read the row with `IncludeDeleted: true`, set
  `cu.Deleted = nil` in memory, call `storage.Update(ctx, cu)`.
- **Pros:** no new helper at all.
- **Cons:** this is racy. Two concurrent rejoin calls each read the
  soft-deleted row, both clear it in memory, both Update — both
  succeed and the second one happily emits a duplicate
  `MEMBER_REJOINED` event. The conditional UPDATE pattern exists
  precisely to make this not the case. This option is rejected.
- **Effort:** trivial but wrong.

## Recommendation

**Option A.** It's the smallest change that preserves the
codebase's established race-safety pattern. The dedicated helper
also gives the daily purge job (#1620) a clean place to hang
related primitives later (e.g., `ClaimCommunityUserPurge` for
hard-delete after the 30-day window).

## Open Questions

- **Should `created_at_unix_sec` be updated to `now` on rejoin?** —
  *Recommendation:* **keep the original**. `created_at_unix_sec` is
  the historical "first joined at" timestamp; it's referenced by
  Settings → Communities, by leaderboards, and possibly by impact
  metrics that key on tenure. Surfacing the rejoin date is a
  separate concern; if a UX needs it, add an explicit
  `last_rejoined_at_unix_sec` field rather than overload the
  existing one. Confirm in PR review.
- **Should `MEMBER_REJOINED_WITHIN_WINDOW` push-notify the
  community owner the way `MEMBER_LEFT` does today?** —
  *Recommendation:* **no.** Design doc §8 lists no push for rejoin.
  Owners care about departures (signal of churn); silent rejoins
  are pleasant noise that doesn't need an interrupt. The event row
  remains in `community_event` for audit and any
  later-implemented activity feed. Leave `shouldNotify` returning
  `false` for this event type (which is its current default).
- **Window edge: exactly 30 days?** — *Recommendation:* **inclusive
  on the boundary.** `now - deleted_at_unix_sec <= 30*24*3600`.
  Matches the user-visible promise ("you have 30 days") and avoids
  off-by-one annoyance. Document the inclusive boundary in the
  helper's doc.
- **Idempotency on double-tap (already-active row)?** —
  *Recommendation:* **return `FailedPrecondition: already_member`,
  not silent success**. A double-tap from a stale UI is a real
  thing, but so is a programmatic duplicate that should surface as
  an error so the client can refetch state. Returning
  `FailedPrecondition` is the explicit signal; the client can
  handle it the same way it handles a stale GetCommunity (refetch
  the community, the active membership is already there). Issue
  body recommended idempotent silent-success; flagging this for PR
  review since the recommendation is opposite. (If the reviewer
  prefers idempotent, the change is one-line.)
- **What if the leaver's row exists but `deleted` is nil because
  someone else's rejoin already cleared it?** — Same as the
  already-active case → `FailedPrecondition: already_member`.
- **What if there is no `CommunityUser` row at all (user was never
  a member)?** — `FailedPrecondition: no_membership_to_rejoin`.
  Don't 404 — this is a precondition violation against an existing
  community, not a missing community.

## Implementation Plan

### Phase 1: Storage primitive

**Goal:** the race-safe primitive lands first with its own
coverage so the service handler builds on stable ground.

**Tasks:**
- [ ] Add `RejoinWindowDays = 30` and `RejoinWindowSeconds = 30 * 24 * 3600` to the existing `server/community/constants.go` (already holds `MaxCommunityMembers`).
- [ ] Add `ClaimCommunityUserRejoin(ctx, communityID, userID,
  nowUnixSec) (claimed bool, err error)` in
  `server/storage/community.go`. Conditional UPDATE on
  `community_user` that succeeds iff (a) row exists, (b)
  `deleted_deleted_by_user_id IS NOT NULL` (currently
  soft-deleted), (c) `deleted_deleted_at_unix_sec >= nowUnixSec -
  RejoinWindowSeconds` (within window). Inclusive boundary
  documented in the helper's comment.
- [ ] Add `ProbeCommunityUserDeletedAt(ctx, communityID, userID)
  (deletedAtUnixSec int64, found bool, err error)` for the
  lost-claim disambiguation branch. Returns `found=false` when no
  row exists at all.
- [ ] Unit tests in `server/storage/community_test.go`:
  - claim succeeds within window
  - claim fails on already-active row
  - claim fails on past-window row
  - claim fails on missing row
  - probe distinguishes the three states
  - inclusive boundary at exactly 30 days

**Validates:** the primitive gives clean, race-safe semantics for
every relevant terminal state, and the disambiguation read-back is
unambiguous.

### Phase 2: Service handler rewrite

**Goal:** the stubbed `RejoinCommunity` becomes the real handler,
with full integration coverage.

**Tasks:**
- [ ] Replace the stub in
  `server/services/community/lifecycle.go:RejoinCommunity` with:
  1. `auth.RequireAuth`.
  2. Logger setup with `operation=RejoinCommunity`,
     `user_id`/`user_email` (masked), `community_id`.
  3. Fetch community with `IncludeDeleted: true`.
     - NotFound on missing → `connect.CodeNotFound`.
     - Soft-deleted →
       `FailedPrecondition: community_deleted` (§2.7).
  4. Pre-flight: read the caller's `CommunityUser` with
     `IncludeDeleted: true`.
     - No row at all → `FailedPrecondition: no_membership_to_rejoin`.
     - Row active (Deleted == nil) →
       `FailedPrecondition: already_member`.
     - Row soft-deleted, past window →
       `FailedPrecondition: rejoin_window_expired`.
  5. `ClaimCommunityUserRejoin(ctx, communityID, userID, now)`.
     - On loss, `ProbeCommunityUserDeletedAt` to disambiguate:
       window expired → `rejoin_window_expired`; missing →
       `no_membership_to_rejoin`; row now-active →
       `already_member`.
  6. Resync the in-memory `CommunityUser` (`Deleted = nil`,
     leave `created_at_unix_sec` untouched per recommendation
     above) and `storage.Update` so binary_proto agrees with the
     flat columns.
  7. Emit `COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW`
     with `actor=caller`. No notification per design §8.
  8. Recompute regions via `s.recomputeCommunityRegions`.
- [ ] Use `connecterr.Internal(ctx, "RejoinCommunity", err, ...)` for every internal-error return — bare `connect.NewError(connect.CodeInternal, err)` fails CI (`scripts/check_codeinternal.js`).
- [ ] Verify `MEMBER_REJOINED_WITHIN_WINDOW` is not in
  `shouldNotify` (it shouldn't be; if a future PR adds it, the
  per-recipient gating still applies). No change to
  `notifications.go` for this PR.
- [ ] Integration tests in
  `server/integration_tests/community_rejoin_test.go`:
  - **Sunny path within 30 days:** leaver rejoins; `Deleted` is
    nil; `MEMBER_REJOINED_WITHIN_WINDOW` event recorded;
    `created_at_unix_sec` preserved from original join.
  - **Window expired:** leaver's `deleted_at_unix_sec` is 31 days
    ago → `FailedPrecondition: rejoin_window_expired`.
  - **§2.7 — community deleted:** leaver tries to rejoin a
    community in deleted state →
    `FailedPrecondition: community_deleted`.
  - **Already an active member:** double-tap or stale-UI →
    `FailedPrecondition: already_member`. (If reviewer prefers
    idempotent silent success, this test flips trivially.)
  - **No membership ever:** stranger calls `RejoinCommunity` on a
    community they never joined →
    `FailedPrecondition: no_membership_to_rejoin`.
  - **Boundary: exactly 30 days:** `deleted_at_unix_sec = now -
    30*24*3600`, claim succeeds (inclusive boundary).
  - **Region recompute:** seed a leaver with a primary location;
    rejoin; verify a `CommunityRegion` row appears (or the
    expected count for that region increments).

**Validates:** the full RPC contract end-to-end including all
five negative paths and the §2.7 cross-cut with community
soft-delete.

### Phase 3: Concurrency stress (optional, lightweight)

**Goal:** confirm the conditional-UPDATE race-safety claim under
real concurrent load.

**Tasks:**
- [ ] One integration test that fires N concurrent
  `RejoinCommunity` calls for the same caller and asserts: at most
  one returns nil; the rest return
  `FailedPrecondition: already_member`; database has exactly one
  active `CommunityUser` row for the caller; exactly one
  `MEMBER_REJOINED_WITHIN_WINDOW` event recorded.

**Validates:** the storage primitive's race-safety claim, mirrors
the §9.1 owner-leave race test pattern from #1656 Phase 3.

## Audit Notes

Reviewed against `docs/server/architecture.md` and
`docs/server/observability.md`. Findings folded into the plan above:

- **Constants location.** `server/community/constants.go` already
  exists (currently holds `MaxCommunityMembers`). Add
  `RejoinWindowDays` and `RejoinWindowSeconds` there — no new file
  needed. Pinned in Phase 1.
- **Storage-layer race-safety primitive (architecture §1).** Plan
  correctly puts the conditional UPDATE in
  `server/storage/community.go` next to `ClaimCommunityRestore` /
  `ClaimCommunityOwnerHandoff`. Service handler remains thin;
  no service-to-service calls. ✓
- **`connecterr.Internal` usage (architecture §Error wrapping).**
  Every internal-error return in the new handler must use
  `connecterr.Internal(ctx, "RejoinCommunity", err, kv...)` —
  bare `connect.NewError(connect.CodeInternal, err)` fails CI via
  `scripts/check_codeinternal.js`. Pinned in Phase 2 task list.
- **Logging conventions (observability §Standard Field Names).**
  Pinned the field names every handler log line must carry:
  `operation=RejoinCommunity`, `user_id`, `user_email` (masked via
  `logging.MaskEmail`), `community_id`. Use
  `logging.LoggerWithContext(ctx).With(...)` at the top of the
  handler. The storage primitive should log nothing — Phase 1
  helpers in this codebase don't log; the service handler is the
  layer that owns the user-facing context. (Mirrors
  `ClaimCommunityRestore` / `ClaimCommunityOwnerHandoff`.)
- **Goroutine safety.** No new goroutines.
  `RecordCommunityEventAndNotify` already uses `GoSafe` for the
  notify fan-out, and there's no notification dispatch for
  `MEMBER_REJOINED_WITHIN_WINDOW` anyway (per design §8 and the
  recommendation in Open Questions). ✓
- **Integration test location.** Pinned to
  `server/integration_tests/community_rejoin_test.go` to match
  the per-feature pattern set by #1654 / #1655 / #1656.
- **Notification dispatcher.** Confirmed: no entry should be added
  to `shouldNotify` for `MEMBER_REJOINED_WITHIN_WINDOW`. The
  default `false` is the design-intended behavior. The
  `CommunityEvent` row is still recorded for audit; it's the push
  that's intentionally absent.
- **Test isolation (architecture §7).** Use `storage.SetupTestStorage(t)`
  + `t.Cleanup(cleanup)` per the established pattern; no shared
  state across tests.
- **Race-test scope.** Phase 3's optional concurrency test should
  use the same goroutine + `sync.WaitGroup` shape as #1656 Phase
  3's owner-handoff race test (`TestLeaveCommunity_OwnerHandoffRace`).
  Acceptable terminal states for losing calls:
  `FailedPrecondition: already_member`. The DB invariant is
  exactly one active `CommunityUser` row + exactly one
  `MEMBER_REJOINED_WITHIN_WINDOW` event.

The plan is now consistent with `docs/server/architecture.md` and
`docs/server/observability.md`. Ready for implementation.

## Related Issues

- **#1619** — parent epic.
- **#1643** — proto + RPC stub this issue replaces.
- **#1656** — `LeaveCommunity` rewrite that creates the
  soft-deleted membership rows this RPC operates on (PR #1709
  open).
- **#1620** — daily community purge job. Consumes the same
  `RejoinWindowDays` constant when the daily job hard-deletes
  rows past the window.
- **#1652** — Phase 1 `community_user_unique` constraint that
  makes "un-soft-delete the existing row" the only legal shape for
  rejoin.
