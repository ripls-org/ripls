# Issue #1722: Phase 3 of #1619: Add ListDeletedCommunitiesForRestore RPC

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1722
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§§ 2.5, 7.1)

## Problem Statement

The Settings → Communities deleted-list surface (#1717) needs a server endpoint that returns the soft-deleted communities the caller is **eligible to restore** — i.e., communities where the caller's `user_id` appears in `Community.deleted_snapshot.member_user_ids`.

The existing `ListCommunities` RPC is membership-scoped:

- It queries `CommunityUser` rows for the caller and joins to active `Community` rows.
- It strips soft-deleted communities by default (the read filter excludes `deleted_deleted_at_unix_sec > 0`).
- Its request type has no `include_deleted` flag, and per CLAUDE.md proto convention ("Never reuse RPC request or response message types across multiple RPCs") it should not be extended to dual-purpose.

So a new RPC is required. The shape is dictated by the issue body and §2.5 of the design doc:

- Returns one entry per eligible deleted community.
- Each entry carries enough metadata for the client to render the **'Deleted, X days left'** badge: at minimum `id`, `name`, `description`, `media_ids`, `deleted_at_unix_sec`. Optional `deleted_by_user_id` lets the restore confirmation screen surface 'Deleted by <name>'.

The technical wrinkle is that `Community.deleted_snapshot.member_user_ids` is a `repeated string` field inside a nested optional message. The proto-SQL flattener skips nested messages when nil and skips repeated fields entirely (`extractFieldValuesRecursive` in `server/storage/protosql_schema.go:187,200`). So the snapshot member set is **only** present in `binary_proto` today — there is no SQL column to query against. We need to denormalize it to a queryable column, in lockstep with the existing `participant_ids` pattern used for `ChatConversation` and `Story`.

## Context from Comments

The issue has **no comments** — it was filed by me earlier in this session. The body is the spec. One detail that surfaced during the issue creation but isn't in the body: the user explicitly asked for a proto-check before this issue was filed, and the result confirmed `ListCommunities` does not surface soft-deleted communities under any flag.

## Codebase audit

What this RPC will lean on:

- **Proto + RPC scaffolding.** `proto/ripls/api/community_service.proto` already follows the pattern — request/response message pair plus `rpc` declaration on the service. Mirror the `ListCommunities` shape but with the new `DeletedCommunityItem` carrier.
- **Storage scaffolding.** New helper in `server/storage/community.go`, parallel to existing `FindCommunitiesNeedingPurgeReminder` (#1659) which is the other recent query that opts INTO deleted rows.
- **Denormalized array column pattern.** `chat_conversation_schema.go` and `story_schema.go` both add a `participant_ids TEXT[]` column with a GIN index, backfill from existing rows on startup, and maintain it via a wrapper helper. The shape this PR needs is identical: add `snapshot_member_user_ids TEXT[]` to the `community` table, GIN-indexed, and keep it in sync on every Update where `Deleted` is set.
- **Wrapper-helper precedent.** `server/storage/chat_conversation.go` defines `ChatConversationStorage` which wraps `ProtoSQLStorage` and overrides `Insert`/`Update` to do a two-phase write (proto INSERT/UPDATE, then `UPDATE participant_ids = $1`). The same pattern fits here, but the snapshot is only set on `DeleteCommunity` (lifecycle.go) and cleared on `RestoreCommunity` — two well-known call sites — so a tightly-scoped wrapper helper would be over-engineering. Inline updates at the two call sites are sufficient.
- **Service registration.** `server/services/community/service.go` already registers RPCs via the generated Connect handler. The new RPC slots in alongside `ListCommunities`.
- **Auth.** `auth.RequireAuth(ctx)` returns the caller's `authInfo`; no special permission gating beyond authentication. Eligibility is enforced by the WHERE clause (caller in snapshot) — there's no extra layer to add.

What's missing and must be built:

- **Schema column + GIN index** for `community.snapshot_member_user_ids TEXT[]` with a startup backfill from existing soft-deleted rows.
- **Snapshot-write call sites.** `DeleteCommunity` already writes the snapshot to `binary_proto`; it must also `UPDATE community SET snapshot_member_user_ids = $1`. `RestoreCommunity` must clear it (set to NULL or `'{}'`). The proto-Update path will not propagate the array column for the same reason flat columns don't auto-clear: nested-message updates skip repeated children.
- **`FindCommunitiesEligibleForRestore` storage helper** mirroring `FindCommunitiesNeedingPurgeReminder` — opts INTO deleted rows, queries the new column with `$userID = ANY(snapshot_member_user_ids)`, returns full `*models.Community` so the service can map to `DeletedCommunityItem`.
- **`ListDeletedCommunitiesForRestore` RPC handler** in `server/services/community/membership.go` (alongside `ListCommunities`) — auth, call helper, build response items.
- **Tests** at storage-helper and service-handler levels, plus the schema backfill startup test.

## Potential Solutions

### Option A: Denormalized `snapshot_member_user_ids TEXT[]` + GIN index (recommended)

- **Approach:** add the array column + GIN index following the `participant_ids` precedent. Inline the array sync into `DeleteCommunity` and `RestoreCommunity` (two call sites, no wrapper helper). New storage helper queries `WHERE $1 = ANY(snapshot_member_user_ids) AND deleted_deleted_at_unix_sec > 0`. New RPC handler maps to the response.
- **Pros:**
  - Matches the established codebase pattern exactly.
  - GIN-indexed lookup is O(log N) — fast enough for any realistic per-user eligible-deleted-community count.
  - Read query is one round-trip, no per-row binary_proto unmarshal.
  - The startup backfill is identical to the existing chat_conversation / story migrations.
- **Cons:**
  - Two write call sites need the manual `UPDATE` (vs. one-shot via wrapper).
  - Adds a column + index to the community table — modest storage cost.
- **Effort:** medium. ~600 lines including tests, schema migration, two service-handler edits, the new helper, and the new RPC.

### Option B: Linear scan with binary_proto unmarshal at query time

- **Approach:** new helper that queries soft-deleted communities (no array filter), unmarshals each `binary_proto`, filters in Go for snapshot membership.
- **Pros:**
  - No new column, no migration.
- **Cons:**
  - O(deleted_communities) unmarshal per RPC call. Even with a 30-day soft-delete window the global count grows with usage; per-user containment check on every row in Go memory is wasteful.
  - Violates the SQL Efficiency rules in CLAUDE.md ("Push filtering to SQL").
  - No GIN index means no escape hatch when the deleted-community count grows.
- **Effort:** small. Rejected as a correctness/performance regression vs. the established pattern.

### Option C: Side table `community_snapshot_member` row-per-(community, user)

- **Approach:** add a new `community_snapshot_member` table populated by `DeleteCommunity` and cleared by `RestoreCommunity`. Query by `WHERE user_id = $1` joined to `community`.
- **Pros:**
  - Composable for other queries that might want the snapshot set in different shapes.
  - Indexed on `(user_id)` with normal B-tree.
- **Cons:**
  - New table = new migration + schema lifecycle to maintain.
  - The other `participant_ids`-style features (chat conversations, stories) didn't pick this shape — adding a new shape here breaks the convention without clear benefit.
  - Per-snapshot N inserts at delete time (vs. one array UPDATE).
- **Effort:** medium-large. Rejected as over-engineered for this use case.

## Recommendation

**Option A.** Matches every existing precedent for "repeated proto field that needs to be queryable" in this codebase, fits the CLAUDE.md SQL efficiency rules, and the GIN index gives the query the right performance shape from day one. The startup backfill + dual-call-site sync mirrors what `participant_ids` already does for two other entities.

## Open Questions

- **Should the wrapper-helper pattern (à la `ChatConversationStorage`) be used, or inline the array sync into the two service call sites?** —
  *Recommendation:* inline. Snapshot writes happen at exactly two well-known points (`DeleteCommunity` sets, `RestoreCommunity` clears). A wrapper that intercepts every `Update(*models.Community)` would have to detect the snapshot delta to know when to fire — extra logic and risk for no real safety win. Inline `s.storage.UpdateCommunitySnapshotMembers(ctx, communityID, memberIDs)` and `s.storage.ClearCommunitySnapshotMembers(ctx, communityID)` helpers, called explicitly from the two service handlers, keep the two writes obvious in the call site they belong to.
- **Should `ClaimCommunityRestore` also clear the snapshot column the way it clears `purge_reminder_sent_at_unix_sec`?** —
  *Recommendation:* yes. Same justification as for the purge-reminder field — the conditional-UPDATE primitive is the atomic restore-claim point, and adding `snapshot_member_user_ids = '{}'` (or NULL — pick one and document) to the SET clause keeps the restore atomic across all three columns. Saves a separate roundtrip.
- **Limit semantics?** —
  *Recommendation:* `LIMIT 100` hardcoded inside the helper; no client-controlled paging. The expected per-user count of eligible deleted communities is in the single digits (you'd have to be in 100+ communities that were all deleted in the past 30 days to hit this). Document inline so future readers know it's a sanity cap, not a paging boundary.
- **Should `DeletedCommunityItem` carry the `created_at_unix_sec` of the original community?** —
  *Recommendation:* not in this PR. The badge math (§7.1) only needs `deleted_at_unix_sec`, and the restore confirmation copy doesn't reference create-date. Add it later if a UX surface needs it.
- **Should we backfill existing soft-deleted communities' snapshot column on first deploy?** —
  *Recommendation:* yes — the startup migration must walk existing soft-deleted rows and populate `snapshot_member_user_ids` from each row's `binary_proto.DeletedSnapshot.MemberUserIds`. Without it, the day-deploy + day-29 cutoff means the deleted-list won't surface communities that were soft-deleted before this PR shipped. The migration is bounded by the existing soft-deleted-community count (small) and runs once at startup like the other snapshot-column migrations.

## Implementation Plan

### Phase 1: Schema + denormalization

**Goal:** the new column exists, is indexed, is backfilled, and stays in sync.

**Tasks:**
- [ ] Add `community_snapshot_schema.go` in `server/storage/` mirroring `chat_conversation_schema.go`:
  - `InitializeCommunitySnapshotMemberColumn(ctx)` adds `community.snapshot_member_user_ids TEXT[]` if missing, creates a GIN index, and backfills the column from `binary_proto.DeletedSnapshot.MemberUserIds` for every soft-deleted row where the column is currently NULL.
- [ ] Wire it into the storage initialization sequence (`InitializeCommunityOwnerUserID`, `InitializeCommunityUserUniqueness`, etc., in `community_schema.go` or wherever those are called from).
- [ ] Storage helpers in `server/storage/community.go`:
  - `UpdateCommunitySnapshotMembers(ctx, communityID, memberIDs []string) error` — `UPDATE community SET snapshot_member_user_ids = $1 WHERE id = $2`.
  - `ClearCommunitySnapshotMembers(ctx, communityID) error` — `UPDATE community SET snapshot_member_user_ids = NULL WHERE id = $1`.
  - Extend `ClaimCommunityRestore` to also set `snapshot_member_user_ids = NULL` in the existing UPDATE (cleared atomically alongside `deleted_*` and `purge_reminder_sent_at_unix_sec`).
- [ ] Update `server/services/community/lifecycle.go`:
  - In `deleteCommunityInternal`, after the existing `storage.Update(community)` write, call `UpdateCommunitySnapshotMembers(ctx, community.Id, memberIDs)`.
  - `RestoreCommunity` already calls `ClaimCommunityRestore` (now extended) and a follow-up `storage.Update`; the snapshot column is cleared by the claim, so no separate call needed.
- [ ] Storage unit tests:
  - Backfill picks up an existing soft-deleted community and populates the column.
  - `UpdateCommunitySnapshotMembers` writes the array correctly.
  - `ClearCommunitySnapshotMembers` and the extended `ClaimCommunityRestore` both clear the column.
  - GIN index exists after `InitializeCommunitySnapshotMemberColumn` runs.

**Validates:** the column is present, indexed, backfilled, and stays in sync across the lifecycle.

### Phase 2: Find helper + RPC

**Goal:** the RPC behaves as the issue spec says.

**Tasks:**
- [ ] Add `proto/ripls/api/community_service.proto`:
  - `ListDeletedCommunitiesForRestoreRequest` (empty).
  - `ListDeletedCommunitiesForRestoreResponse` with `repeated DeletedCommunityItem communities`.
  - `DeletedCommunityItem` with `id`, `name`, `description`, `media_ids`, `deleted_at_unix_sec`, `deleted_by_user_id`.
  - `rpc ListDeletedCommunitiesForRestore(...)` registered on `CommunityService`.
- [ ] Run `npm run generate`.
- [ ] `FindCommunitiesEligibleForRestore(ctx, userID, limit) ([]*models.Community, error)` in `server/storage/community.go` mirroring `FindCommunitiesNeedingPurgeReminder`:
  - `WHERE deleted_deleted_at_unix_sec > 0 AND $1 = ANY(snapshot_member_user_ids)`.
  - `ORDER BY deleted_deleted_at_unix_sec ASC` (oldest deletion = closest to purge = most-urgent first).
  - `LIMIT 100` hardcoded.
- [ ] Service handler in `server/services/community/membership.go` next to `ListCommunities`:
  - Auth.
  - Logger setup with `operation=ListDeletedCommunitiesForRestore`, `user_id`, `user_email` (masked).
  - Call helper, map each `*models.Community` to `*api.DeletedCommunityItem`, populate `deleted_at_unix_sec` and `deleted_by_user_id` from `community.GetDeleted()`.
  - Log result count at INFO.
  - Use `connecterr.Internal(ctx, "ListDeletedCommunitiesForRestore", err, ...)` for storage failures.
- [ ] Storage unit tests for `FindCommunitiesEligibleForRestore`:
  - Caller in snapshot of one deleted community → returned.
  - Caller in snapshot of multiple deleted communities → all returned, ordered by oldest first.
  - Caller in snapshot of a since-restored community → not returned.
  - Caller NOT in snapshot → not returned.
  - Active community in caller's `CommunityUser` rows → not returned.
  - Empty result is normal.
  - Limit caps results.
- [ ] Service handler unit tests:
  - Happy path returns the populated items.
  - Empty result returns empty list, no error.
  - Auth failure returns Unauthenticated.
  - Storage error returns CodeInternal via `connecterr.Internal`.

**Validates:** the RPC returns the right set, in the right order, with the right field shape, on every input.

### Phase 3: Smoke test against real lifecycle

**Goal:** confirm the end-to-end flow against testcontainers — DeleteCommunity populates the column, the RPC surfaces it, RestoreCommunity clears it.

**Tasks:**
- [ ] Integration test in `server/integration_tests/community_list_deleted_test.go`:
  - Seed two users (deleter, snapshot-member). Deleter creates a community via `CreateCommunity`, snapshot-member is added.
  - Deleter calls `DeleteCommunity`. Both users have entries in the snapshot.
  - Snapshot-member calls `ListDeletedCommunitiesForRestore` → sees the community.
  - Snapshot-member calls `RestoreCommunity` → community restored.
  - Snapshot-member calls `ListDeletedCommunitiesForRestore` again → empty list (column was cleared by the restore claim).
  - Re-delete → snapshot-member calls again → community appears again (proves the cycle).

**Validates:** the column lifecycle is correct end-to-end through the real RPCs, not just direct storage manipulation.

## Audit Notes

Reviewed against `docs/server/architecture.md` and
`docs/server/observability.md`. Findings folded into the plan above:

- **Schema-file location.** Plan suggests a new
  `community_snapshot_schema.go` mirroring `chat_conversation_schema.go`.
  Either that or extending the existing
  `server/storage/community_schema.go` (which already holds
  `InitializeCommunityOwnerUserID` and
  `InitializeCommunityUserUniqueness`) is fine. **Pinned: extend
  the existing `community_schema.go`** — all community-table
  schema migrations stay in one file, mirroring how
  `chat_conversation_schema.go` colocates its own. Add a new
  exported `InitializeCommunitySnapshotMemberColumn` function and
  call it from the same startup sequence as the other community
  schema initializers.
- **Backfill logging (observability §Logging Usage Pattern).**
  The startup backfill should follow the same shape as
  `backfillCommunityOwnerUserID` and the existing snapshot-column
  backfills: `operation=backfillCommunitySnapshotMembers`,
  `community_count=<int>`, `duration_ms=<int>` at INFO. Per-row
  unmarshal failures stay at WARN with `community_id` and `error`
  and continue (don't abort the migration on a single bad row).
- **`connecterr.Internal` (architecture §Error wrapping).** Plan
  already specifies `connecterr.Internal(ctx, "ListDeletedCommunitiesForRestore", err, ...)`
  for storage failures. ✓
- **Goroutine safety.** No new goroutines. ✓
- **Service independence (architecture §1).** Plan correctly puts
  the WHERE-against-snapshot query in the storage layer and keeps
  the service handler thin. No service-to-service calls. ✓
- **SQL efficiency rules (CLAUDE.md SQL Efficiency).** GIN index
  + push filter to SQL + LIMIT 100. ✓ Specifically calls out the
  reason linear scan was rejected.
- **Proto-convention conformance.** New `DeletedCommunityItem`
  type intentionally distinct from `CommunityItem` (which lives
  in `ListCommunitiesResponse`) per the "no reusing message types
  across RPCs" rule. ✓
- **Test isolation (architecture §7).** Use
  `storage.SetupTestStorage(t)` + `t.Cleanup(cleanup)` per the
  established pattern.
- **Integration test location.** Pinned to
  `server/integration_tests/community_list_deleted_test.go`
  matching the per-feature pattern set by #1654 / #1655 / #1656 /
  #1657 / #1658.
- **`ClaimCommunityRestore` extension.** Plan recommends adding
  `snapshot_member_user_ids = NULL` to the existing claim's SET
  clause (alongside `purge_reminder_sent_at_unix_sec = 0` from
  #1659). This stays consistent with the established pattern of
  the claim primitive being the atomic clearing point for all
  flat columns derived from the soft-delete state. ✓

The plan is now consistent with `docs/server/architecture.md` and
`docs/server/observability.md`. Ready for implementation.

## Related Issues

- **#1619** — parent.
- **#1717** — client consumer (deleted-list + restore screen). Hard-blocked on this PR.
- **#1655** / PR #1706 — RestoreCommunity (the action this list feeds into).
- **#1654** / PR #1709 — DeleteCommunity (writes the snapshot this list reads).
- **#1659** / PR #1716 — day-before-purge reminder (sibling pattern: opts INTO deleted communities; mirror its `Find*` helper shape).
- **#1723** — sibling Phase 3 server issue: `ListRejoinableCommunities` (parallel shape for the rejoin path).
