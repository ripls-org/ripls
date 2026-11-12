# Issue #1655: Phase 2 of #1619 — implement RestoreCommunity end-to-end

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1655
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§2.5, §2.7, §6.2, §7.6, §8); docs/server/architecture.md §"Storage Layer"; docs/server/observability.md

## Audit Notes (2026-05-04)

`/ripls-audit` checked this plan against `docs/server/architecture.md`,
`docs/server/observability.md`, and the post-#1704 codebase. Findings,
all incorporated below:

1. **Notification recipients: drop the snapshot-passthrough machinery.**
   #1704's cascade did NOT touch `CommunityUser` rows, so the snapshot
   members are still the current active member set after the
   community is restored (the restorer was already in the snapshot, so
   their CommunityUser was already active). For `COMMUNITY_RESTORED`,
   `getNotificationRecipients` can use the same `GetCommunityMemberIDs`
   helper as the existing broadcast events — minus the actor — without
   any snapshot capture. Phase 4 simplified significantly.
2. **Crash-window recoverability gap is real.** The two-step claim-then-update
   has a small but observable failure mode: if storage.Update fails
   after `ClaimCommunityRestore` succeeds, the row's flat
   `deleted_*` columns are clear but `binary_proto` still has
   `Deleted` set. A subsequent RestoreCommunity call would read
   `Community.Deleted != nil` from `binary_proto`, then call
   `ClaimCommunityRestore` which finds 0 rows affected (flat columns
   already clear) and returns FailedPrecondition. The community
   becomes un-restorable from the user surface. **Recommendation:**
   add a "consistency repair" branch in Phase 3: when the read shows
   `Community.Deleted != nil` AND `ClaimCommunityRestore` returns
   `false`, re-read with a fresh query, and if the flat column shows
   the row is actually active, treat it as the "we crashed mid-restore
   on a previous attempt" case — fall through to the resync `Update`
   step. This recovers the half-state.
3. **`AssertMaxQueries` regression lock.** Like #1704's
   DeleteCommunity test, RestoreCommunity should pin its query
   count: 1 (auth) + 1 (Community read) + 1 (claim) + 1 (Update) +
   5 cascade queries + 5 cascade update batches (worst case = sum of
   rows) + 1 (CommunityUser lookup) + 1 (CommunityUser
   update/insert) + 1 (event insert) + N (notification reads). Pin
   the worst case for a 1-row-per-cascade-type fixture with headroom
   for test stability. Phase 5 task list now includes this.
4. **Standard log fields.** Plan uses `restorer_id` for clarity in
   logs about a specific role; the canonical authenticated-user
   field is `user_id` per `docs/server/observability.md`. Both are
   acceptable; recommend keeping `user_id` as the standard
   correlation key (already injected by `LoggerWithContext`) and
   adding `restorer_id` as an explicit semantic alias only on
   error-recovery paths where the role distinction matters.
5. **`connecterr.Internal` on storage error paths.** Plan
   already calls this out for the architecture-doc error-wrapping
   rule; Phase 3 task list explicit.
6. **`firesForDeletedCommunity` exception is NOT needed for
   COMMUNITY_RESTORED.** The dispatcher's `IsActive` gate sees the
   now-active community naturally because the restore clears
   `Community.Deleted` BEFORE emitting the event. The existing
   doc-comment at `notifications.go:152-156` already calls this out.
   Phase 4 task list confirmed.
7. **File-size concern.** `cascade_delete.go` is at ~510 lines after
   #1704 (5 helpers added). This plan adds 5 more inverse helpers
   ≈ +130 lines → ~640 total. Still well under the 1000-line cap.
   No extraction needed.
8. **Test fixtures.** All `&models.Community{...}` literals must
   include `OwnerUserId` to satisfy the `community_owner_required`
   CHECK from #1652. Phase 5 task list already says this.
9. **Client docs do not apply.** Pure server-side; no API surface
   change beyond replacing `Unimplemented` with success/error codes.

## Problem Statement

`server/services/community/lifecycle.go:RestoreCommunity` returns
`Unimplemented` today (the stub from #1643). After #1654, the server
can produce soft-deleted communities; nothing can bring them back. This
issue makes the inverse work end-to-end:

- Caller eligibility: must be in the `Community.deleted_snapshot.member_user_ids`
  set captured at delete time.
- Atomic flip: clear `Deleted` + `DeletedSnapshot`, set
  `OwnerUserId = caller.id` (the restorer becomes the new owner per
  §2.5), and un-soft-delete the cascade rows that #1654 soft-deleted.
- Membership: ensure the restorer has an active `CommunityUser` row —
  un-soft-delete if one exists, insert one if not (the §7.6 edge case
  where the restorer left the community before it was deleted).
- Race: two members tap Restore simultaneously; first wins, second sees
  `FailedPrecondition`.
- Emit `COMMUNITY_RESTORED` and notify the snapshot members per §8.
- §2.7 edge case: a leaver who was NOT a member at delete time must get
  `PermissionDenied` (`not_in_restorer_snapshot`) — they cannot restore
  a community they weren't part of when it was deleted.

## Context from Comments

The issue has no comments; it's a Phase 2 sub-issue I filed yesterday.
Authoritative inputs:

- **Parent #1619** sequencing (this is step 7a).
- **`docs/community_delete_and_leave.md`** §2.5 (user-visible behavior),
  §2.7 (the leaver-of-a-deleted-community edge case), §6.2 (the RPC
  table), §7.6 (the previously-departed restorer edge case), §8
  (notification copy).
- **#1654 (just merged)** — the cascade pattern this issue inverses.

## Codebase research

Confirmed against `main` after #1704:

- **`lifecycle.go:RestoreCommunity`** is a one-line stub returning
  `Unimplemented`.
- **No SQL transactions in this codebase.** `grep -rn "BeginTx" server/storage/`
  returns nothing. "Atomic" semantics are achieved via single-row UPDATE
  statements; the proto-SQL `Update` writes flat columns and `binary_proto`
  in one SQL statement.
- **Race protection prior art:** `protosql.go:DecrementFieldIfPositive`
  (line 563) uses a conditional `UPDATE ... WHERE id = $1 AND <field> > 0`
  and inspects `RowsAffected()` to detect the race. Same pattern works
  here: `UPDATE community SET deleted_deleted_by_user_id = NULL WHERE
  id = $1 AND deleted_deleted_by_user_id IS NOT NULL`.
- **`buildDeletedFilter` and `IncludeDeleted: true`** are the read-side
  knobs we need to fetch a soft-deleted Community.
- **Repeated fields are skipped from flat columns**
  (`protosql_schema.go` — "repeated fields … can't be mapped to simple
  SQL columns"). So `deleted_snapshot.member_user_ids` lives only in
  `binary_proto`, not in a flat column. The conditional-UPDATE race
  guard only needs to touch `deleted_deleted_by_user_id`.
- **`GetByID` returns `binary_proto`** verbatim and unmarshals. So if
  the flat `deleted_*` columns are cleared but `binary_proto` still
  has `Deleted` populated, a `GetByID` returns a Community with
  `Community.Deleted != nil` even though the read filter would let it
  through. This is the inconsistency window the implementation must
  minimize.
- **The dispatcher exception (`firesForDeletedCommunity`) is NOT
  needed for `COMMUNITY_RESTORED`**. RestoreCommunity clears
  `Community.Deleted` BEFORE emitting the event, so the existing
  `IsActive` gate sees an active community and the dispatcher fires
  normally. The doc at `notifications.go:152-156` already calls this
  out.
- **`CommunityUser` uniqueness:** the `community_user_unique`
  constraint from #1652 means we can't blindly INSERT — must check
  for an existing row first (active or soft-deleted) and either
  un-soft-delete or insert.
- **`MaxCommunityMembers = 32`** — restorer-rejoin doesn't need to
  worry about hitting the cap because they were a member at delete
  time (so they're already counted in the snapshot, which today is
  the only reasonable bound on community size).

## Potential Solutions

### Option A: Two-step claim-then-update + cascade un-restore (recommended)

- **Approach:**
  1. Read Community with `IncludeDeleted: true`. Validate caller is in
     `DeletedSnapshot.MemberUserIds`.
  2. Race-safe claim via raw conditional `UPDATE community SET
     deleted_deleted_by_user_id = NULL WHERE id = $1 AND
     deleted_deleted_by_user_id IS NOT NULL`. If 0 rows affected,
     return `FailedPrecondition`.
  3. We won the race. Mutate the in-memory community: clear `Deleted`,
     clear `DeletedSnapshot`, set `OwnerUserId = caller.id`. Call
     `storage.Update` to resync `binary_proto` and the rest.
  4. Cascade un-soft-delete the five join types (inverse of #1654).
  5. Ensure the restorer has an active `CommunityUser` (un-soft-delete
     existing or insert).
  6. Emit `COMMUNITY_RESTORED`. Notify snapshot members minus the
     restorer.
- **Pros:** Race-safe at the claim step; mirrors the existing
  `DecrementFieldIfPositive` pattern. No transaction primitives needed.
  Cascade reuses existing helpers (just inverted). Small inconsistency
  window between claim and resync (microseconds in the happy path) is
  bounded and recoverable.
- **Cons:** A crash between step 2 and step 3 leaves the row with
  flat `deleted_*` cleared but `binary_proto` still showing the prior
  Deleted state. `GetByID` reads from `binary_proto` so consumers see
  `Community.Deleted != nil`. Inconsistent. Need a doc note + a
  `WarnContext` log so an oncall can spot it.
- **Effort:** Medium.

### Option B: Hand-rolled single conditional UPDATE that writes everything

- **Approach:** Write a `RestoreCommunityAtomic(ctx, id, newOwner,
  restoredProto)` storage helper that issues a single `UPDATE` with
  the conditional WHERE plus all the SET clauses
  (`deleted_*=NULL, owner_user_id=$, binary_proto=$, updated_at=$`).
  Returns `(claimed bool, error)`.
- **Pros:** Truly atomic: no inconsistency window.
- **Cons:** Hardcodes column names — fragile if the proto schema
  evolves. Bypasses the proto-SQL `extractFieldValues` reflection so
  there's no protection against future schema drift. The
  inconsistency window in Option A is so short in practice that the
  belt-and-suspenders rarely pays off.
- **Effort:** Medium-high (more code, less robust to future change).

### Option C: Optimistic read-modify-write, accept the race

- **Approach:** Skip the conditional. Read with `IncludeDeleted`,
  mutate, `storage.Update`. Both racers succeed, last writer wins
  on `owner_user_id`.
- **Pros:** Simplest possible implementation.
- **Cons:** Two `COMMUNITY_RESTORED` events fire (one per racer);
  members get duplicate pushes; the "loser" believes they restored
  the community but their owner-claim was overwritten. Bad UX even
  if the database state ends up consistent.
- **Effort:** Small but wrong.

## Recommendation

**Option A.** It mirrors the established `DecrementFieldIfPositive`
race pattern, reuses the cascade helpers from #1704 (just inverted),
and the crash-window inconsistency is rare and recoverable. Document
the small window in the implementation comment so an oncall doesn't
spend time chasing a `Community.Deleted != nil` ghost.

## Open Questions

- **Cascade un-soft-delete: same metadata-vs-clear?** #1654's cascade
  set `Deleted = &models.DeletedMetadata{...}` on each join row.
  Restore's inverse should set `Deleted = nil`. **Recommendation:**
  clear, not "mark restored." The semantic of `DeletedMetadata`
  presence is "this row is soft-deleted"; clearing the field
  restores it. No "restored_at" field exists on join rows — keep it
  that way.
- **Cascade un-soft-delete: should we restore rows soft-deleted
  *before* the community-delete cascade?** The §5 rule: "Restore
  reverses the Delete, not history. Anything that was already
  cancelled or removed *before* the delete stays cancelled." So
  the inverse helpers must skip rows whose
  `deleted.deleted_at_unix_sec < community.deleted.deleted_at_unix_sec
  - epsilon`, OR (cleaner) only un-soft-delete rows whose
  `deleted.deleted_by_user_id == community.deleted.deleted_by_user_id`
  AND `deleted.deleted_at_unix_sec` is within a small window of the
  community delete time. **Recommendation:** the cleanest signal is
  the deleter+timestamp match — Phase 1's snapshot, plus the
  `DeletedMetadata` already populated by #1654's cascade, gives us
  this for free. Helper signature:
  `CascadeRestoreCommunityGearByCommunityID(ctx, storage, communityID,
  communityDeletedAt int64, communityDeletedBy string)`. Only restore
  rows whose `deleted.deleted_by_user_id == communityDeletedBy AND
  deleted.deleted_at_unix_sec >= communityDeletedAt`. This is
  belt-and-suspenders against future re-delete cycles.
- **Restorer's CommunityUser row.** Three states: (a) active row
  exists (caller was in snapshot AND never left after delete) — no
  change needed; (b) soft-deleted row exists (caller had left
  before the community was deleted but is in the snapshot…wait, §7.6
  says they wouldn't be in the snapshot in that case); (c) no row
  exists (deleted by something else). **Recommendation:** in the
  common case the caller has an active row (they were a member at
  delete time, the membership wasn't deleted by the community-delete
  cascade since #1654 doesn't soft-delete `CommunityUser` rows).
  Verify this by re-reading the snapshot logic in #1654 — yes,
  CommunityUser is NOT in the cascade. So the restorer's active row
  should already exist. If it doesn't (rare data-corruption case),
  insert a fresh one. Skip the un-soft-delete branch entirely; #1656
  handles soft-deleted memberships.
- **§2.7 edge case test.** A leaver who's NOT in the snapshot calls
  Restore on the deleted community. **Recommendation:** integration
  test: seed a community + 2 active members + a leaver (membership
  hard-deleted before the cascade work). Owner deletes community
  → snapshot captures only the 2 active members. Leaver calls
  RestoreCommunity → expects `PermissionDenied` with reason
  `not_in_restorer_snapshot`.
- **`CommunityUser` un-soft-delete in this PR vs deferring to #1656.**
  Strictly, #1656 (LeaveCommunity rewrite) is what introduces
  soft-deleted CommunityUser rows. Until #1656 ships, no
  CommunityUser is ever soft-deleted, so the un-soft-delete branch
  here would be dead code. **Recommendation:** still write the
  un-soft-delete branch — it's three lines and prevents a follow-up
  PR after #1656. Cover it with a unit test that explicitly seeds a
  soft-deleted CommunityUser and asserts the restore reactivates it.
- **Notification copy.** Per §8: `"<restorer> restored <name>.
  <restorer> is the new owner."` Same shape as #1654's copy. The
  buildNotification case should be straightforward; the IsActive
  guard does NOT need bypassing because RestoreCommunity clears
  `Community.Deleted` BEFORE emitting the event.

## Implementation Plan

### Phase 1 — Cascade restore helpers
**Goal:** Five new `CascadeRestore*ByCommunityID` helpers in
`cascade_delete.go`, parallel to the `Cascade*Delete*ByCommunityID`
family from #1704.
**Tasks:**
- [ ] `CascadeRestoreCommunityGearByCommunityID(ctx, storage,
      communityID, communityDeletedBy, communityDeletedAt)`
- [ ] Same for `CommunityRequest`, `CommunityExperience`,
      `CommunityNotificationPreferences`, `CommunityInvitationLink`.
- [ ] Each filters by `community_id` AND
      `deleted_deleted_by_user_id == communityDeletedBy` AND
      `deleted_deleted_at_unix_sec >= communityDeletedAt`. Only
      restores rows that were soft-deleted by the same actor at
      the same time as the community itself, per §5 rule.
- [ ] Implementation: `s.QueryByField(... IncludeDeleted: true)`,
      iterate, check the deleted-by + deleted-at conditions in Go,
      `row.Deleted = nil`, `s.Update`. Mirror existing helpers'
      DEBUG logging.
- [ ] Unit tests in `cascade_delete_test.go` covering: rows
      restored, rows skipped (different deleted_by), rows skipped
      (deleted-at predates community delete), idempotent re-run.
**Validates:** Phase 2 has clean inverse helpers to call.

### Phase 2 — Race-safe storage claim
**Goal:** Atomic claim of the restore "lock" on the community row.
**Tasks:**
- [ ] In `server/storage/community.go` (already exists), add
      `ClaimCommunityRestore(ctx, communityID string) (bool, error)`.
      Issues `UPDATE "community" SET deleted_deleted_by_user_id =
      NULL, deleted_deleted_at_unix_sec = 0 WHERE id = $1 AND
      deleted_deleted_by_user_id IS NOT NULL`. Returns
      `(rowsAffected > 0, nil)`.
- [ ] Comment block at the helper explaining: this is an atomic
      "first-wins" claim. The caller MUST follow up with a
      `storage.Update` to resync `binary_proto`; until that
      lands, `GetByID` will return `Community.Deleted != nil`
      from the still-stale `binary_proto`.
- [ ] Unit test: seed a deleted community, call ClaimCommunityRestore
      from two goroutines simultaneously, assert exactly one returns
      true.
**Validates:** Race protection is in place; the second restorer is
guaranteed to see `false`.

### Phase 3 — RestoreCommunity service implementation
**Goal:** Replace the stub with the full flow.
**Tasks:**
- [ ] Read Community with `IncludeDeleted: true`. If
      `Community.Deleted == nil` → `FailedPrecondition` with reason
      `community_not_deleted`.
- [ ] Verify caller is in `DeletedSnapshot.MemberUserIds`. If not →
      `PermissionDenied` with reason `not_in_restorer_snapshot`.
- [ ] Capture `communityDeletedBy` and `communityDeletedAt` for the
      cascade helpers.
- [ ] Call `ClaimCommunityRestore`. If false: most likely a
      legitimate "race lost" (another restorer beat us). But it
      could also be the rare crash-window case where a previous
      restore attempt cleared the flat columns but failed to resync
      `binary_proto`. **Recovery branch:** re-read the Community
      with `IncludeDeleted: true` AND a separate raw-SQL probe of
      `deleted_deleted_by_user_id` from the flat column. If the
      flat column says active but the proto says deleted, we're in
      the crash-window state — fall through to the resync `Update`
      step. Otherwise return `FailedPrecondition`
      (`community_not_deleted`).
- [ ] Mutate in-memory: clear `Deleted`, clear `DeletedSnapshot`,
      set `OwnerUserId = authInfo.UserID`. Call `storage.Update` to
      resync `binary_proto`. Storage error here means inconsistency
      window stays open — log at WARN with `community_id`,
      `restorer_id`, `error` for an oncall to investigate.
- [ ] Run the five cascade-restore helpers. Best-effort with WARN
      logging on per-helper failures, mirroring #1704.
- [ ] Ensure the restorer's `CommunityUser`:
  - [ ] Read by `(community_id, user_id)`.
  - [ ] If active row exists: no-op.
  - [ ] If soft-deleted row exists: clear `Deleted`, `storage.Update`.
  - [ ] If no row exists: `storage.Insert(&models.CommunityUser{...})`.
- [ ] Emit `COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED` via
      `RecordCommunityEventAndNotify`. The dispatcher's `IsActive`
      gate sees the now-active community naturally — no
      `firesForDeletedCommunity` exception needed.
- [ ] All storage error paths use `connecterr.Internal(ctx, op, err,
      kv...)` per the architecture-doc rule.
**Validates:** RestoreCommunity returns success and the resulting
state matches the design.

### Phase 4 — Notification routing for COMMUNITY_RESTORED
**Goal:** Members get the §8 push.
**Tasks:**
- [ ] Add `COMMUNITY_RESTORED` to `shouldNotify`'s broadcast list.
- [ ] Add a case in `getNotificationRecipients` for
      `COMMUNITY_RESTORED`: same shape as the existing broadcast
      events (`EXPERIENCE_CREATED`, `GEAR_SHARED`, etc.) —
      `GetCommunityMemberIDs(ctx, s, event.CommunityId)` minus the
      actor. **Why this works:** #1704's cascade did NOT touch
      `CommunityUser` rows, so the snapshot members are still
      active members after restore (the restorer was already in
      the snapshot, so their CommunityUser was already active).
      No snapshot-passthrough or capture-and-pass mechanism
      needed — the simple member-list lookup gives the right set.
      A reader-comment near the case should call this out so a
      future maintainer doesn't replace it with a snapshot lookup
      and break things.
- [ ] Add a `buildNotification` case for
      `COMMUNITY_RESTORED`: title `"<restorer> restored <name>"`,
      body `"<restorer> is the new owner."` Use the existing
      `getCommunityName` helper — by dispatch time the community
      is active, so the standard read path (no `IncludeDeleted`)
      works.
- [ ] **Do NOT** add `COMMUNITY_RESTORED` to
      `firesForDeletedCommunity`. RestoreCommunity clears
      `Community.Deleted` before emitting the event, so the
      existing `IsActive` gate sees an active community and lets
      the dispatch through naturally.
**Validates:** Members get the push; notification copy matches §8.

### Phase 5 — Tests
**Goal:** Lock the behavior.
**Tasks:**
- [ ] Extend `community_delete_test.go` (or new
      `community_restore_test.go`):
  - [ ] **Happy path:** delete a community, restore as a snapshot
        member, assert: Community.Deleted is nil, DeletedSnapshot
        is nil, OwnerUserId equals restorer, all five cascade row
        types are un-soft-deleted, restorer has an active
        CommunityUser row, COMMUNITY_RESTORED event recorded,
        snapshot members (minus restorer) got the §8 push.
  - [ ] **Eligibility deny:** call as a non-snapshot user →
        `PermissionDenied`, reason includes
        `not_in_restorer_snapshot`.
  - [ ] **Not-deleted:** call on an active community →
        `FailedPrecondition`, reason includes
        `community_not_deleted`.
  - [ ] **Race:** simulate concurrent restores via two goroutines
        calling ClaimCommunityRestore directly (or two synchronous
        RestoreCommunity calls); assert exactly one wins.
  - [ ] **§2.7 leaver edge case:** seed community + 2 members +
        a leaver who hard-leaves before delete; owner deletes;
        leaver calls RestoreCommunity → `PermissionDenied`.
  - [ ] **§5 cascade-respect:** seed a join row that was
        soft-deleted BEFORE the community delete (different
        deleted_by/at). After restore, assert that join row stays
        soft-deleted while the cascade-deleted ones are restored.
  - [ ] **CommunityUser re-creation:** explicitly delete the
        restorer's CommunityUser row before restoring; assert
        restore re-creates it.
- [ ] All `&models.Community{...}` test fixtures include
      `OwnerUserId` to satisfy the CHECK from #1652.
- [ ] **`AssertMaxQueries` regression lock** for the happy path
      (1-row-per-cascade-type fixture). Like #1704's DeleteCommunity
      test, pin the worst-case query count with headroom for test
      stability so the cascade can't slip into N+1 on per-row
      updates.
- [ ] **Crash-window recovery test** — explicitly simulate the
      half-state by manually clearing the flat `deleted_*` columns
      via raw SQL while leaving `binary_proto` intact, then call
      RestoreCommunity. Assert the recovery branch from Phase 3
      kicks in and the row reaches the fully-restored state.
**Validates:** Acceptance criteria from #1655 are mechanically
enforced; behavior is locked.

### Phase 6 — Verify and ship
**Goal:** All gates green; PR opened.
**Tasks:**
- [ ] `npm run build` clean.
- [ ] `go build -tags=integration ./...` clean.
- [ ] `npm run lint:ephemeral-comments` clean.
- [ ] `go test ./...` clean modulo the pre-existing health flake.
- [ ] PR title: `Phase 2 of #1619: implement RestoreCommunity
      end-to-end (issue #1655)`.
- [ ] PR body: list the cascade-restore behavior (§5 respect),
      the race-claim mechanism, the snapshot-passthrough for
      notification recipients, and the membership re-creation
      branches.
**Validates:** #1655 closes; #1656/#1659 unblocked.

## Related Issues

- **#1619** — Parent. This is Phase 2 step 7a.
- **#1654** — DeleteCommunity rewrite (just merged). The inverse
  this issue implements; cascade pattern source.
- **#1643** — Proto + RPC stub this PR replaces.
- **#1652** — Phase 1 data migration. The `community_owner_required`
  CHECK constrains how we set OwnerUserId; the
  `community_user_unique` constraint shapes the membership
  re-creation branch.
- **#1656** — LeaveCommunity rewrite. Will introduce soft-deleted
  CommunityUser rows that this issue's "un-soft-delete or insert"
  membership branch handles.
- **#1659** — Notification copy + day-before-purge. The
  `COMMUNITY_RESTORED` snapshot-passthrough helper this issue
  builds is reusable for the day-before-purge reminder.
