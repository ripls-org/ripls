# Issue #1723: Phase 3 of #1619: Add ListRejoinableCommunities RPC

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1723
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§§ 2.6, 2.7)

## Problem Statement

The Settings → Communities 'recently left' surface (#1721) needs a server endpoint that returns the **active** communities the caller can rejoin without a fresh invite — i.e., communities where the caller has a soft-deleted `CommunityUser` row less than 30 days old AND the community itself is still active.

The §2.7 cross-cut is load-bearing: a community in soft-deleted state must NOT appear in this list, even when the caller's individual rejoin window is still open. The deleted-list surface (#1717 / #1722) handles those cases; this surface is exclusively for "left a still-active community."

The existing `ListCommunities` RPC is active-membership-scoped — it joins from `CommunityUser` rows that are NOT soft-deleted, then strips soft-deleted communities. It cannot dual-purpose to surface soft-deleted memberships. Per CLAUDE.md proto convention ("Never reuse RPC request or response message types across multiple RPCs"), this needs its own dedicated request/response shape.

Compared to the parallel issue #1722 (`ListDeletedCommunitiesForRestore`), this query is straightforward:

- It scans `CommunityUser` rows by `user_id`, which is already an indexed column (the unique constraint `community_user_unique` on `(community_id, user_id)` from Phase 1 #1652 covers this and the per-user lookup is cheap).
- It joins to `Community` to apply the §2.7 active-community filter.
- No new schema, no GIN index, no denormalization — the query speaks the existing flat columns directly.

## Context from Comments

The issue has **no comments** — filed by me earlier in this session as part of the Phase 3 client-wiring breakout. The body is the spec. The user explicitly asked for a proto-check before this issue was filed; the result confirmed `ListCommunities` does not surface soft-deleted memberships under any flag.

## Codebase audit

What this RPC will lean on:

- **Index coverage.** `community_user_unique` on `(community_id, user_id)` (added by #1652 / Phase 1) covers both forward (membership lookup) and per-user reverse scans. No new indexes needed; the per-user query plan uses the unique index.
- **`RejoinWindowSeconds` constant.** Lives in `server/community/constants.go`, added by #1657. Use it directly so the rejoin window stays in lockstep with `RejoinCommunity`.
- **Pattern: opting INTO deleted rows.** Two recent precedents to mirror — `FindCommunitiesNeedingPurgeReminder` (#1659, in `server/storage/community.go`) and `FindCommunitiesEligibleForRestore` (proposed in #1722). Both query the flat `deleted_*` columns directly rather than going through `QueryByField` (which strips deleted rows by default). This RPC follows the same shape but joins two tables.
- **No snapshot column needed.** Unlike #1722, the query here is on a per-row column (`CommunityUser.deleted_*`) rather than a snapshot array on the parent. The existing flat columns are sufficient — no denormalization required.
- **Service handler precedent.** `ListCommunities` in `server/services/community/membership.go:118` is the structural model — auth, fetch from storage, map to API items, log result count.
- **API field shape.** The badge math on the client (`Left X days ago, Y days to rejoin`) needs `left_at_unix_sec` (sourced from `CommunityUser.Deleted.DeletedAtUnixSec`). Per CLAUDE.md: each RPC gets its own dedicated request/response types — distinct `RejoinableCommunityItem` (not reusing `CommunityItem`).

What's missing and must be built:

- **`FindRejoinableCommunitiesForUser` storage helper.** New method in `server/storage/community.go` that runs the JOIN query against `community_user` ⋈ `community`, returning paired `(*models.CommunityUser, *models.Community)` so the service handler can populate both `community.Name`/`Description`/`MediaIds` AND the `left_at_unix_sec` from the CommunityUser row.
- **Proto definitions** in `proto/ripls/api/community_service.proto`: request, response, item, RPC registration.
- **Service handler** in `server/services/community/membership.go` next to `ListCommunities`.
- **Tests** at storage-helper, service-handler, and integration levels.

## Potential Solutions

### Option A: Two-table JOIN query in storage helper (recommended)

- **Approach:** new `FindRejoinableCommunitiesForUser(ctx, userID, nowUnixSec, windowSeconds, limit) ([]CommunityUserWithCommunity, error)` runs a JOIN against `community_user` and `community`. Returns paired structs so the service handler has everything in one round-trip. Matches the shape of the existing `GetCommunityWithMembership` helper (which JOINs the same two tables).
- **Pros:**
  - One round-trip — no N+1 risk for the per-row community fetch.
  - Mirrors the existing `GetCommunityWithMembership` JOIN shape.
  - No schema changes, no migration, no denormalization.
  - The unique-constraint index covers the per-user CommunityUser scan.
- **Cons:**
  - Two-table JOIN return type is a small cognitive bump vs. returning a flat list of communities — but the existing `CommunityWithMembership` precedent normalizes this.
- **Effort:** small. ~250 lines including tests.

### Option B: Two-step fetch (CommunityUser query → batch GetByIDs for Community)

- **Approach:** query soft-deleted `CommunityUser` rows for the caller, collect community IDs, then batch-fetch the active `Community` rows separately and filter to active.
- **Pros:**
  - Matches `ListCommunities`'s existing shape (which does query memberships → batch-fetch communities).
  - Uses storage primitives already wrapped in helpers.
- **Cons:**
  - Two round-trips instead of one.
  - Filtering for active-community happens in two places (the helper's filter on the second-stage fetch, and the read filter strips soft-deleted communities by default — but we're querying with IncludeDeleted=false so this works).
  - Doesn't enforce the §2.7 community-active gate atomically with the membership lookup; if a community gets soft-deleted between the two reads, the resulting list is briefly stale (visible to the caller for one read).
- **Effort:** small. Acceptable but less direct than Option A. Picked Option A.

### Option C: Linear scan with binary_proto unmarshal

- **Approach:** scan all `CommunityUser` rows for the caller (including soft-deleted), unmarshal each, filter in Go, then per-row-fetch the parent community.
- **Pros:** none meaningful.
- **Cons:** classic N+1 pattern flagged by CLAUDE.md SQL Efficiency rules. Rejected.

## Recommendation

**Option A.** A single JOIN query is the right shape — the data sits in two normal SQL tables already indexed, and the §2.7 active-community gate becomes part of the WHERE clause atomically. The existing `GetCommunityWithMembership` JOIN helper is structurally identical (same two tables, same JOIN key, slightly different filter), so reviewers and future readers immediately recognize the pattern.

## Open Questions

- **Inclusive or exclusive 30-day boundary?** —
  *Recommendation:* inclusive — `cu.deleted_deleted_at_unix_sec >= now - RejoinWindowSeconds`. Matches the inclusive semantics already established by `ClaimCommunityUserRejoin` (#1657 storage helper) so the listing surface and the action surface agree on the boundary. A user whose left-at is exactly 30 days ago appears in the list AND can successfully rejoin.
- **Should the list include results when the user has a soft-deleted CommunityUser row from a community they already rejoined and re-left?** —
  *Recommendation:* per the schema, only ONE CommunityUser row exists per (community_id, user_id) due to the unique constraint. Rejoin un-soft-deletes the row; a subsequent leave re-soft-deletes the same row with the new `deleted_at`. So the listing surfaces the most-recent leave, which is correct.
- **Limit semantics?** —
  *Recommendation:* `LIMIT 100` hardcoded. Realistic per-user count of "communities you left in the past 30 days" is single digits. Document inline as a sanity cap.
- **Order: most-recently-left first vs. oldest-left first?** —
  *Recommendation:* most-recently-left first (`ORDER BY cu.deleted_deleted_at_unix_sec DESC`). The user's mental model is reverse-chronological ("the community I left yesterday"), and the urgent-deadline ordering used by #1722's deleted-list (oldest first → closest to purge) doesn't apply here because rejoin doesn't have a "running out" badge in the same way the deleted-community countdown does.
- **Should the response include the user's original `created_at_unix_sec` (joined-at)?** —
  *Recommendation:* not in this PR. The badge math only needs `left_at_unix_sec`. The rejoin confirmation copy doesn't reference how long the user was originally a member. Add later if a UX surface needs it.
- **Should we surface the inviter or community owner so the rejoin confirmation can show 'You were invited by <name>'?** —
  *Recommendation:* not in this PR. Keeps the response shape minimal and avoids pulling in a User join. Add in a follow-up if the UX warrants it.

## Implementation Plan

### Phase 1: Storage helper

**Goal:** the `FindRejoinableCommunitiesForUser` helper exists with full unit coverage.

**Tasks:**
- [ ] Add a paired-result type alongside `CommunityWithMembership` in `server/storage/community.go`:
  ```go
  // RejoinableCommunity pairs a soft-deleted CommunityUser row with
  // its (still-active) parent Community for the §2.6 rejoin
  // listing.
  type RejoinableCommunity struct {
      Community  *models.Community
      Membership *models.CommunityUser
  }
  ```
- [ ] Add `FindRejoinableCommunitiesForUser(ctx, userID, nowUnixSec, windowSeconds int64, limit int) ([]RejoinableCommunity, error)`:
  - JOIN `community_user cu` to `community c` on `c.id = cu.community_id`.
  - WHERE `cu.user_id = $1 AND cu.deleted_deleted_by_user_id IS NOT NULL AND cu.deleted_deleted_at_unix_sec >= $2 AND (c.deleted_deleted_by_user_id IS NULL OR c.deleted_deleted_by_user_id = '')`.
  - ORDER BY `cu.deleted_deleted_at_unix_sec DESC`.
  - LIMIT `$3`.
  - Return paired unmarshalled protos.
- [ ] Storage unit tests in `server/storage/community_test.go`:
  - User has one soft-deleted membership in an active community → returned.
  - User has soft-deleted membership > 30 days old → not returned.
  - User has soft-deleted membership < 30 days, but community is also soft-deleted → not returned (§2.7).
  - User has only active memberships → empty list.
  - User had a soft-deleted membership but rejoined (membership now active again) → not returned.
  - User has memberships across multiple communities meeting all criteria → all returned, ordered most-recently-left first.
  - Limit caps results.
  - Limit must be positive (returns error for 0/negative).
  - Inclusive boundary at exactly `windowSeconds` ago.

**Validates:** the helper returns the exact set called out by the issue's acceptance test list, and the §2.7 filter is enforced atomically.

### Phase 2: Proto + service handler + integration test

**Goal:** the RPC behaves end-to-end.

**Tasks:**
- [ ] Add to `proto/ripls/api/community_service.proto`:
  - `ListRejoinableCommunitiesRequest` (empty).
  - `ListRejoinableCommunitiesResponse { repeated RejoinableCommunityItem communities = 1; }`.
  - `RejoinableCommunityItem { string id = 1; string name = 2; string description = 3; repeated string media_ids = 4; int64 left_at_unix_sec = 5; }`.
  - `rpc ListRejoinableCommunities(...)` registered on `CommunityService`.
- [ ] Run `npm run generate`.
- [ ] Service handler in `server/services/community/membership.go` next to `ListCommunities`:
  - Auth via `auth.RequireAuth`.
  - Logger setup with `operation=ListRejoinableCommunities`, `user_id`, `user_email` (masked).
  - Call helper with `time.Now().Unix()` and `communitylib.RejoinWindowSeconds`, limit=100.
  - Map each `RejoinableCommunity` to `*api.RejoinableCommunityItem`, populating `left_at_unix_sec` from `Membership.GetDeleted().GetDeletedAtUnixSec()`.
  - Use `connecterr.Internal(ctx, "ListRejoinableCommunities", err, ...)` for storage failures.
  - Log result count at INFO.
- [ ] Service handler unit tests:
  - Happy path returns populated items in DESC order.
  - Empty result returns empty list.
  - Auth failure returns Unauthenticated.
  - Storage error returns CodeInternal via `connecterr.Internal`.
- [ ] Integration test in `server/integration_tests/community_list_rejoinable_test.go`:
  - Seed two users (owner, leaver). Owner creates community, leaver joins.
  - Leaver calls `LeaveCommunity` (real RPC) → membership soft-deleted.
  - Leaver calls `ListRejoinableCommunities` → sees the community.
  - Owner calls `DeleteCommunity` (real RPC) → community soft-deleted.
  - Leaver calls `ListRejoinableCommunities` again → empty list (§2.7).
  - Owner restores via real `RestoreCommunity` → community active again.
  - Leaver calls `ListRejoinableCommunities` → community appears again.
  - Leaver calls `RejoinCommunity` (real RPC) → membership active.
  - Leaver calls `ListRejoinableCommunities` → empty list (no longer soft-deleted).

**Validates:** the full RPC contract through real lifecycle transitions. Proves the §2.7 cross-cut works against the actual state machine.

## Audit Notes

Reviewed against `docs/server/architecture.md` and
`docs/server/observability.md`. Findings folded into the plan above:

- **`connecterr.Internal` (architecture §Error wrapping).** Plan
  already specifies `connecterr.Internal(ctx, "ListRejoinableCommunities", err, ...)`
  for storage failures — bare
  `connect.NewError(connect.CodeInternal, err)` fails CI via
  `scripts/check_codeinternal.js`. ✓
- **Logging conventions (observability §Standard Field Names).**
  Pinned `operation=ListRejoinableCommunities`, `user_id`,
  `user_email` (masked via `logging.MaskEmail`), and a `result_count`
  on the summary INFO log. No `request_id` to add — Connect
  middleware injects it automatically via `LoggerWithContext(ctx)`.
- **SQL efficiency rules (CLAUDE.md SQL Efficiency).** JOIN
  query in storage helper + indexed scan via the existing
  `community_user_unique` constraint + `LIMIT 100`. ✓
- **Service independence (architecture §1).** Storage helper
  owns the JOIN; service handler stays thin. No service-to-service
  calls. ✓
- **Goroutine safety.** No new goroutines. ✓
- **Proto-convention conformance.** `RejoinableCommunityItem`
  intentionally distinct from `CommunityItem` and from #1722's
  `DeletedCommunityItem` — three separate item types for three
  separate RPCs, per "no reusing message types across RPCs." ✓
- **Test isolation (architecture §7).** Use
  `storage.SetupTestStorage(t)` + `t.Cleanup(cleanup)` per the
  established pattern.
- **Integration test location.** Pinned to
  `server/integration_tests/community_list_rejoinable_test.go`
  matching the per-feature pattern from #1654 / #1655 / #1656 /
  #1657 / #1658 / #1722.
- **Constants reuse.** Plan correctly imports
  `communitylib.RejoinWindowSeconds` from
  `server/community/constants.go` (added by #1657). Don't
  hardcode the 30-day boundary in this RPC — the constant is the
  single source of truth across `RejoinCommunity`, the
  day-before-purge reminder, and this listing surface.
- **Inclusive-boundary semantics.** Plan agrees with the
  inclusive boundary established by `ClaimCommunityUserRejoin`
  (#1657). Document the inclusive semantics in the helper
  comment so future readers don't accidentally make them
  exclusive.
- **Two-table JOIN return shape.** The new `RejoinableCommunity`
  struct mirrors the existing `CommunityWithMembership` shape in
  the same file. Reviewers will recognize the pattern. ✓

The plan is now consistent with `docs/server/architecture.md` and
`docs/server/observability.md`. Ready for implementation.

## Related Issues

- **#1619** — parent.
- **#1721** — client consumer (recently-left list + rejoin screen). Hard-blocked on this PR.
- **#1657** / PR #1715 — RejoinCommunity (the action this list feeds into).
- **#1656** / PR #1709 — LeaveCommunity (creates the soft-deleted memberships this list reads).
- **#1659** / PR #1716 — day-before-purge reminder (sibling pattern: opts INTO deleted rows; mirror its `Find*` helper shape).
- **#1722** — sibling Phase 3 server issue: `ListDeletedCommunitiesForRestore` (parallel shape for the restore path; different table-shape concerns since this issue's data is per-row, not array-membership).
