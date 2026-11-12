# Issue #1654: Phase 2 of #1619 — rewrite DeleteCommunity with snapshot + cascade soft-delete

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1654
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§2.1, §6.2, §6.4, §8); docs/server/architecture.md §"Storage Layer"; docs/server/observability.md

## Audit Notes (2026-05-01)

`/ripls-audit` checked this plan against `docs/server/architecture.md`,
`docs/server/observability.md`, and the existing cascade/notification
code. Findings, all incorporated below:

1. **`MaxCommunityMembers` confirmed = 32** (`server/community/constants.go:5`).
   The snapshot's worst case is 32 user IDs ≈ a few KB of binary_proto.
   No special pagination needed.
2. **`connecterr.Internal` on storage errors.** The existing
   `DeleteCommunity` already uses `connecterr.Internal(ctx, op, err, ...)`
   per `docs/server/architecture.md` §"Error wrapping". The rewrite
   must preserve this — never bare `connect.NewError(connect.CodeInternal, err)`.
   Phase 2 task list now spells this out.
3. **Log levels for cascade failures.** The plan said "log per-helper
   failures, continue" without specifying. Phase 2 now specifies WARN
   level with structured fields (`community_id`, `cascade_target`,
   `error`) per `docs/server/observability.md` §"Log levels" — cascade
   failures are recoverable on retry, not system failures.
4. **`buildNotification` needs `IncludeDeleted: true` too.** Phase 3
   already calls this out for `getNotificationRecipients`, but
   `buildNotification` also reads the Community (for the name in the
   §8 copy). Phase 3 task list now makes this explicit so both
   functions consistently use `IncludeDeleted: true` for
   COMMUNITY_DELETED.
5. **`notifications.go` is at 789 lines.** Adding the COMMUNITY_DELETED
   case + helper takes it to ~840. Still under the 1000-line cap
   (`docs/server/architecture.md` §"File Size Guidelines"), but
   anticipating #1659 — which adds COMMUNITY_RESTORED,
   OWNERSHIP_TRANSFERRED, day-before-purge copy + the daily job hook —
   this file will likely cross 1000. Phase 5 now adds a one-line note
   to flag this in the PR description so #1659 can plan an extraction
   (e.g., `community_lifecycle_notifications.go`) without surprise.
6. **Zero-member edge case.** A community can theoretically have
   `len(active_members) == 0` (e.g., after a malformed join history).
   Phase 4 now adds an explicit zero-member test case: snapshot is
   empty, cascade is no-op, DeleteCommunity succeeds, no notification
   is dispatched (no recipients).
7. **No deploy-window race in this PR.** Unlike #1652, this PR adds
   no new DB constraints. The cascade helpers and the snapshot/delete
   sequence are pure additive Go code. New binary deploys atomically
   replace old binary's behavior at the request level; no shared-DB
   surprise. Phase 5 task list confirms this for the PR description.
8. **Test fixtures must populate `OwnerUserId`.** All `&models.Community{...}`
   literals added in Phase 4 must include `OwnerUserId` to satisfy the
   `community_owner_required` CHECK from #1652. The existing test
   harness already established this pattern in #1652's sweep.
9. **Client docs do not apply.** Pure server-side rewrite; no API
   surface change, no Flutter touchpoint.

## Problem Statement

`server/services/community/lifecycle.go:DeleteCommunity` ships only a partial
soft-delete today: it flips `Community.deleted` and stops. The cascade is left
as a TODO at `lifecycle.go:194`. Authorization checks `community.CreatorId`
instead of `community.OwnerUserId`. No event fires, no notification dispatches,
no snapshot of eligible-restorers is taken, and the join rows
(`CommunityGear`, `CommunityRequest`, `CommunityExperience`,
`CommunityNotificationPreferences`, `CommunityInvitationLink`) keep pointing
at a now-soft-deleted parent.

This issue rewrites the function to do the full job per the design doc, so
that:
- Phase 1's read-path filters (#1621) finally see a real soft-deleted state
  produced by user action (not a forced flip in a test).
- Phase 1's notification dispatcher guard (#1622) starts gating real pushes
  rather than test-induced ones.
- Phase 1's background-job write guard (#1623) sees its own first real
  exercise.
- The deleted-state object is structured for #1655 (`RestoreCommunity`) to
  consume (snapshot + reversible cascade).
- The eligible-restorer set is fixed at delete time per §2.5, not derived
  from later membership state.

## Context from Comments

The issue has no comments — it was filed in this same session as part of
the Phase 2 sub-issue carving. The authoritative inputs are:
- **Parent #1619** sequencing (this is step 9).
- **`docs/community_delete_and_leave.md`** §2.1 (user-visible behavior),
  §6.2 (the RPC table), §6.4 (audit trail event), §8 (notifications copy).
- **#1571** — the existing "stale community-gear after leave" issue
  partially shares the cascade helper this issue extends. Out of scope for
  this issue; closed by #1656 (LeaveCommunity rewrite).

## Codebase research

Confirmed against the actual repo state on `main` after #1652:

- **`server/services/community/lifecycle.go:DeleteCommunity`** (lines
  150–199) is the function we're rewriting. The TODO at line 194 is the
  honest acknowledgment that the cascade is missing.
- **`server/storage/cascade_delete.go`** already has the right pattern for
  cascade-by-parent-id: `CascadeDeleteCommunityGearByGearID`,
  `CascadeDeleteCommunityRequestByRequestID`,
  `CascadeDeleteCommunityExperienceByExperienceID` (cascade *into* community
  join rows when the parent gear/request/experience is deleted). What we
  need is the inverse direction — cascade by *community_id* — for the same
  three join types plus `CommunityNotificationPreferences` and
  `CommunityInvitationLink`. The pattern is mechanical: copy each existing
  helper, change the field name from `gear_id` to `community_id`, change
  the model. Idempotent skip-already-deleted check stays the same.
- **`server/community/events.go:RecordCommunityEventAndNotify`** is the
  central emit-and-notify entry point. It (a) inserts the
  `CommunityEvent` row, (b) broadcasts to active streams via
  `eventBroadcaster`, (c) asynchronously calls `notifyTargetedUsers`.
  The `eventBroadcaster` hook is what #1658 (stream signal) will use.
- **`server/community/notifications.go:notifyTargetedUsers` line 397**
  short-circuits to "skip" if the community is currently soft-deleted.
  This is #1622's guard. **Critical:** for `COMMUNITY_DELETED`, the
  community IS soft-deleted at the moment we want to notify, so the guard
  blocks the very notification we want to send. The existing comment at
  lines 152–156 already calls out that "day-before-purge ships on a
  separate code path that does NOT call this helper — it deliberately
  fires *for* deleted communities. COMMUNITY_RESTORED works naturally
  because RestoreCommunity clears Community.Deleted before dispatching."
  This issue needs the same exception for `COMMUNITY_DELETED`.
- **`shouldNotify` (line 18)**, **`getNotificationRecipients` (line 204)**,
  and **`buildNotification` (line 550)** all need a `COMMUNITY_DELETED`
  case. Today none of these handle the event.
- **`Community.OwnerUserId`** is populated for every active row after
  #1652. Switching the auth check from `CreatorId` to `OwnerUserId` is now
  safe.
- **The undo registry already classifies `COMMUNITY_DELETED`** as
  `ClassificationRetentionRestore` (added in #1643) — no registry change
  needed.

## Potential Solutions

### Option A: Single PR, full rewrite + cascade helpers + notification path (recommended)

- **Approach:** Add five `Cascade*ByCommunityID` helpers to
  `cascade_delete.go`. Rewrite `DeleteCommunity` end-to-end. Wire
  `COMMUNITY_DELETED` into `shouldNotify` /
  `getNotificationRecipients` / `buildNotification`. Add a deleted-event
  exception to `notifyTargetedUsers`'s `IsActive` gate so the
  just-deleted community can still notify its members. Cover with a
  fat integration test.
- **Pros:** Self-contained — every consumer of the new behavior lives
  in this one PR. No partial states between PRs. Reviewer sees the full
  flow on a single page.
- **Cons:** Larger diff (~400–600 lines including tests), touches three
  files in `server/community/notifications.go` plus the cascade file
  plus the lifecycle file.
- **Effort:** Medium.

### Option B: Split — cascade helpers first, then rewrite

- **Approach:** PR1 adds the five `Cascade*ByCommunityID` helpers and
  unit tests. PR2 rewrites `DeleteCommunity` and wires notifications.
- **Pros:** PR1 is small and reviewable in isolation.
- **Cons:** PR1 lands code with no callers — has to be merged on
  faith, and `unused` lint would flag the helpers if not for some
  exemption. The notification wiring is the actual high-risk piece;
  splitting doesn't reduce that risk.
- **Effort:** Medium-with-coordination.

### Option C: Skip the dispatcher exception; ship notifications in #1659

- **Approach:** Rewrite `DeleteCommunity` and the cascade as in A, but
  do NOT add the deleted-event exception to `notifyTargetedUsers`.
  Notifications for `COMMUNITY_DELETED` ship in #1659 alongside the
  day-before-purge reminder, since both need the same exception
  pattern.
- **Pros:** Cleaner separation of concerns. #1659 owns *all*
  community-deleted notification copy in one place.
- **Cons:** This issue's PR ships a soft-delete that records the event
  but doesn't tell users — the cascade is invisible until #1659 lands.
  Practical impact is small (Phase 2 ships in a tight cluster) but the
  explicit acceptance criterion in #1654 is "Notify all snapshot
  members." Slightly violates the issue's contract.
- **Effort:** Smaller for this PR; the work moves to #1659.

## Recommendation

**Option A.** The notifications wiring is small enough (about 30–50
lines across three functions) that splitting it costs more in review
overhead than it saves. The dispatcher exception pattern is also
already documented in the existing comment at lines 152–156, so
applying it for `COMMUNITY_DELETED` is well-understood. Shipping the
event without the notification would also leave the explicit
acceptance criterion unmet.

If the diff size becomes a concern during review, the notifications
half can be carved off after the fact, but the default should be
"ship together, since they unblock each other."

## Open Questions

- **Cascade transactionality.** The existing `Cascade*` helpers are
  not transactional — they iterate and call `storage.Update` per row.
  A failure mid-way leaves a half-cascaded state. **Recommendation:**
  match the existing pattern. The Phase 1 read filters (#1621) and
  background-job write guards (#1623) handle dangling rows correctly
  (they short-circuit when the parent is deleted), so a half-cascade
  is *visually* correct even if it's *internally* inconsistent. The
  cascade helpers are idempotent — a re-run of `DeleteCommunity` (or
  a separate cleanup pass, or just the next user action triggering
  it) closes the gap. Document the at-most-once-but-resumable
  property in a comment.
- **Order: snapshot first, then deleted, then cascade?** The snapshot
  must be taken from active members BEFORE we soft-delete the
  community (since the read filter would mask members of a
  soft-deleted parent). **Recommendation:** sequence the writes:
  (1) query active members, (2) update Community with both
  `deleted` and `deleted_snapshot` populated in the same `Update`
  call, (3) cascade. The single Update keeps the snapshot and the
  delete atomic; the cascade is best-effort post-write.
- **`OwnerUserId` empty fallback.** Today every active row has
  `OwnerUserId` populated (per #1652). But if a row somehow has
  `OwnerUserId == ""` and `CreatorId == "user-x"`, today's check
  rejects user-x; the new check would also reject (empty string
  doesn't equal anything). **Recommendation:** straight switch to
  `OwnerUserId`. The CHECK constraint from #1652 makes
  `OwnerUserId == ""` impossible on active rows, so the fallback
  doesn't need engineering.
- **Notification dispatcher exception shape.** The cleanest place
  to add the exception is `notifyTargetedUsers` in
  `notifications.go`. Two options: (a) hardcode an event-type
  switch, (b) add a flag-style helper that lists "events that
  intentionally fire for soft-deleted communities."
  **Recommendation:** (b) — a small `firesForDeletedCommunity(eventType) bool`
  helper that returns true for `COMMUNITY_DELETED`,
  `COMMUNITY_RESTORED` (already works naturally; documented for
  symmetry), and the day-before-purge reminder once #1659 lands.
  Keeps the exception list discoverable and avoids growing the
  switch in `IsActive`'s caller.
- **Snapshot includes the deleter?** Per §2.5 "Eligible restorers:
  any user who was a `CommunityUser` of this community at the
  moment it was deleted." The deleter is by definition a member at
  delete time (they have to be the owner). **Recommendation:**
  yes, include them in the snapshot. They're an eligible restorer.
  The §8 notification copy excludes the deleter from the recipient
  list — that's a separate concern (handled in
  `getNotificationRecipients`).
- **What if the community has thousands of members?** The snapshot
  is a `repeated string`, so 10k members ≈ 360 KB of binary_proto
  for the deleted_snapshot field. Practical limit on community
  size today is enforced by `communitylib.MaxCommunityMembers`
  (32 per `docs` and existing call sites — verify in code).
  **Recommendation:** no special handling. The cap from the
  membership join already bounds this; no need to anticipate
  10k-member communities.
- **Does broadcast on event delete work?** `RecordCommunityEventAndNotify`
  calls `eventBroadcaster` which feeds open streams (#1658's
  signal). The broadcaster also runs through `IsActive`-style
  gating? **Recommendation:** check during implementation. If yes,
  add the same exception. If no (broadcaster fires regardless),
  no change needed. Audit `server/services/community/streaming.go`.

## Implementation Plan

### Phase 1 — Cascade helpers
**Goal:** Five new `Cascade*ByCommunityID` helpers in
`cascade_delete.go`, parallel to the existing ByGearID / ByRequestID /
ByExperienceID family.
**Tasks:**
- [ ] `CascadeDeleteCommunityGearByCommunityID(ctx, storage, communityID, deletedMetadata)`
- [ ] `CascadeDeleteCommunityRequestByCommunityID(ctx, storage, communityID, deletedMetadata)`
- [ ] `CascadeDeleteCommunityExperienceByCommunityID(ctx, storage, communityID, deletedMetadata)`
- [ ] `CascadeDeleteCommunityNotificationPreferencesByCommunityID(ctx, storage, communityID, deletedMetadata)`
- [ ] `CascadeDeleteCommunityInvitationLinkByCommunityID(ctx, storage, communityID, deletedMetadata)`
- [ ] All idempotent: skip rows already marked deleted (mirrors the
      existing pattern at `cascade_delete.go:104-107`).
- [ ] All log per-row at DEBUG with `community_id` and the entity ID.
- [ ] Unit tests in `cascade_delete_test.go` (extend existing) covering:
  - [ ] Sunny path: seed N rows, run, all marked deleted.
  - [ ] Idempotent: re-run, no errors, no extra updates.
  - [ ] Empty-input case (no rows for that community).
**Validates:** Phase 2's `DeleteCommunity` rewrite has clean building
blocks to call.

### Phase 2 — DeleteCommunity rewrite
**Goal:** Full soft-delete behavior, no TODO.
**Tasks:**
- [ ] Switch the owner check from `community.CreatorId` to
      `community.OwnerUserId`.
- [ ] Snapshot active members: query `CommunityUser` by `community_id`
      with default `IncludeDeleted: false`, collect `user_id` values,
      populate `Community.deleted_snapshot.member_user_ids`. This
      query uses `QueryByField` → existing helper.
- [ ] Single `Update` call sets both `deleted` (DeletedMetadata with
      caller + now) and `deleted_snapshot` together — atomic write.
- [ ] After the Update succeeds, run the five
      `Cascade*ByCommunityID` helpers. Best-effort: log per-helper
      failures at **WARN** with `operation`, `community_id`, the
      cascade target name, and `error` per
      `docs/server/observability.md` §"Standard Field Names" — then
      continue. Cascade failures are recoverable on retry, not
      system failures (no ERROR level).
- [ ] Replace the TODO comment at the old `lifecycle.go:194` with
      either a "// covered by Cascade* calls above" or simply
      delete the comment.
- [ ] Emit `COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED` via
      `communitylib.RecordCommunityEventAndNotify`. The
      notification dispatcher guard exception (Phase 3) will let
      the recipients receive the push.
- [ ] All storage error paths use `connecterr.Internal(ctx, op, err,
      kv...)` per `docs/server/architecture.md` §"Error wrapping" —
      never bare `connect.NewError(connect.CodeInternal, err)`.
      The existing `DeleteCommunity` already does this correctly;
      preserve the pattern.
**Validates:** Old TODO closed; soft-delete is complete from a
storage perspective; events are emitted.

### Phase 3 — Notification routing for COMMUNITY_DELETED
**Goal:** Make the §8 copy fire to the snapshot members.
**Tasks:**
- [ ] Add `COMMUNITY_DELETED` to `shouldNotify`'s broadcast list.
- [ ] Add a case to `getNotificationRecipients` that returns the
      `deleted_snapshot.member_user_ids` (read from the Community
      proto) minus the actor. This needs a fresh Community read with
      `storage.QueryOptions{IncludeDeleted: true}`, since by the
      time the goroutine dispatches, the read filter would otherwise
      hide the row.
- [ ] Add a case to `buildNotification` that produces the §8 copy:
      `"<owner> deleted <name>. You have 30 days to restore it."`
      Reuse the existing `getUserName` helper for the owner name and
      pull the community name from a Community read that **also
      uses `IncludeDeleted: true`** (same constraint as
      `getNotificationRecipients` — both functions read a community
      that, from the read filter's perspective, no longer exists).
      Consider passing the Community through from
      `notifyTargetedUsers` to avoid two reads, or add a small
      memoized helper.
- [ ] Add a `firesForDeletedCommunity(eventType) bool` helper to
      `notifications.go` returning true for `COMMUNITY_DELETED`. Wrap
      the `IsActive` gate at line 397 in `notifyTargetedUsers` so the
      gate is skipped when this helper returns true.
- [ ] Update the doc-comment block at lines 137–156 to mention
      `COMMUNITY_DELETED` as another deliberate exception (in
      addition to day-before-purge from #1659).
- [ ] Add `CategoryFor(COMMUNITY_DELETED)` →
      `NOTIFICATION_CATEGORY_UNSPECIFIED` so it bypasses per-user
      preference filtering. Per §1 of the design doc, deletion is
      a system-critical event; users cannot opt out.
**Validates:** Members get the push when the owner deletes the
community.

### Phase 4 — Tests
**Goal:** Lock the behavior so future refactors can't silently
regress.
**Tasks:**
- [ ] Extend `server/integration_tests/community_deleted_filter_test.go`
      (or new `community_delete_test.go`): create community + 3
      members + gear + request + experience + invitation link +
      notification prefs row. Call `DeleteCommunity` as the owner.
      Assert:
  - [ ] All five cascade row types are soft-deleted.
  - [ ] `Community.deleted` is set with the caller's ID.
  - [ ] `Community.deleted_snapshot.member_user_ids` contains
        exactly the 3 active members.
  - [ ] A `COMMUNITY_DELETED` event row exists in `CommunityEvent`
        with `actor_id = caller`, `community_id = <id>`.
  - [ ] The notification dispatcher saw the right recipients (use
        the `notificationDone` channel and a capture-mode
        notification service mirroring existing tests).
- [ ] Negative test: non-owner caller → `PermissionDenied`.
- [ ] Negative test: already-deleted community → either
      `FailedPrecondition` or no-op (decide and document).
- [ ] Edge case: zero-member community (active membership query
      returns empty). DeleteCommunity succeeds, snapshot is empty
      slice, all five cascade calls are no-ops, no notification
      dispatched (no recipients).
- [ ] All `&models.Community{...}` test fixtures populate
      `OwnerUserId` to satisfy the `community_owner_required` CHECK
      from #1652 — same pattern established in that PR's sweep.
- [ ] `AssertMaxQueries` regression lock on the cascade. Counter:
      1 (auth) + 1 (Community read) + 1 (members query) + 1
      (Community update) + 5 cascade queries (one per join type) +
      5 cascade-update batches in worst case (per-row updates =
      sum-of-rows). Pin the worst-case query count for a fixture
      with 1 row per cascade type, leaving headroom for test
      stability.
- [ ] Streaming test: open a `StreamCommunityEvents` for the
      community in another goroutine, delete the community, assert
      a `COMMUNITY_DELETED` event arrives via the stream. (Even
      without #1658's terminal-close-and-signal — that ships
      separately — the event broadcast already works.)
**Validates:** Acceptance criteria from #1654 are mechanically
enforced; behavior is locked.

### Phase 5 — Verify and ship
**Goal:** All gates green; PR opened.
**Tasks:**
- [ ] `npm run build` clean.
- [ ] `go build -tags=integration ./...` clean.
- [ ] `go test ./...` clean (modulo the pre-existing
      `TestHealthEndpoint_Integration` flake).
- [ ] `npm run lint:ephemeral-comments` clean.
- [ ] PR title: `Phase 2 of #1619: rewrite DeleteCommunity with
      snapshot + cascade soft-delete (issue #1654)`.
- [ ] PR body lists every cascade type, the dispatcher exception,
      and the §8 copy verbatim.
- [ ] PR body confirms **no deploy-window race**: this PR adds no
      new DB constraints (cascade helpers + Go logic only). Unlike
      #1652's CHECK addition, new binary deploys don't introduce
      shared-DB invariants that the old binary would violate.
- [ ] PR body flags that `server/community/notifications.go` will
      be at ~840 lines after this change. #1659 (the next
      notification-routing addition) should plan to extract a
      `community_lifecycle_notifications.go` to stay under the
      1000-line cap.
**Validates:** #1654 closes; #1655 / #1658 / #1659 are unblocked.

## Related Issues

- **#1619** — Parent: full community soft-delete project. This is
  Phase 2 step 9.
- **#1655** — RestoreCommunity end-to-end. Hard-depends on this
  PR landing first (needs the snapshot to read from and the
  cascade pattern to inverse).
- **#1658** — Stream signal for `community_deleted`. The
  broadcaster hook fires here; #1658 adds the terminal-close
  semantics on top.
- **#1659** — Notification copy + day-before-purge. Adds
  `COMMUNITY_RESTORED` and `OWNERSHIP_TRANSFERRED` copy plus the
  day-before reminder. Reuses the
  `firesForDeletedCommunity` helper this PR introduces for the
  reminder path.
- **#1656** — LeaveCommunity rewrite. Sole-member-leave path
  reuses the snapshot + cascade logic this PR builds.
- **#1571** — Stale community-gear after leave. Closed-as-superseded
  by #1656 once that ships, *not* this issue. Relevant here only
  because the cascade helper extension brings the codebase one
  step closer to that closure.
- **#1185** — DeleteGear cascade audit. Same prior-art pattern;
  the helpers in this issue mirror the shape introduced there.
- **#1621, #1622, #1623** — Phase 1 read-path filter, notification
  dispatcher guard, background-job write guard. All three start
  exercising real soft-deleted state for the first time after
  this PR ships.
