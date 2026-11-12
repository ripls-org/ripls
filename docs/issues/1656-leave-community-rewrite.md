# Issue #1656: Phase 2 of #1619: rewrite LeaveCommunity with cascade, soft-delete, and owner-handoff

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1656
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§§ 2.2, 2.3, 2.4, 4, 6.2)

## Problem Statement

Today's `LeaveCommunity` (`server/services/community/membership.go:20`) is a
3-step happy-path:

1. Look up `CommunityUser`, error if not present.
2. Reject with `PermissionDenied` if `community.creator_id == caller`
   ("creator cannot leave community; delete it instead").
3. **Hard-delete** the membership row.
4. Emit `MEMBER_LEFT` and recompute regions.

That's wrong on three axes for the #1619 model that landed in Phase 1:

- **Hard-delete blocks the 30-day rejoin window.** Once the row is gone,
  `RejoinCommunity` (#1657) has nothing to un-soft-delete. Membership must
  carry `DeletedMetadata` like every other soft-deletable entity.
- **No leaver-scoped cleanup.** The leaver's `CommunityGear`, open
  `CommunityRequest` rows, active community-scoped transfers, RSVPs,
  notification preferences, and invitation links all stay live and
  visible. #1571 ("stale community-gear after leave") is exactly this
  bug; #1656 closes it as superseded.
- **Owner trapping.** With `creator_id == caller` the owner can't leave
  at all. Phase 1 already added `owner_user_id` and the proto already
  carries `LeaveCommunityRequest.new_owner_user_id` (verified at
  `proto/ripls/api/community_service.proto:268`). The server has to
  validate the candidate, transfer ownership, and soft-delete the leaver
  atomically — never producing zero or two owners.
- **Sole-member leave.** Today this would error (sole member is
  typically the creator). The doc says it should convert to a
  `DeleteCommunity` so the community goes into the 30-day soft-delete
  state and the leaver can restore it.

This issue is the rewrite.

## Context from Comments

The issue has **no comments** — all the context lives in the issue body
and `docs/community_delete_and_leave.md`. The body also flags two
pieces of upstream/downstream coupling that were not obvious:

- "Closes #1571 as superseded once this ships." The §4 cascade row for
  `CommunityGear` *is* #1571's fix.
- "Soft-coupled with #1654 (sole-member-leave reuses DeleteCommunity
  logic). Land #1654 first; this issue calls it." #1654 is merged
  (PR #1704), so the dependency is satisfied.
- The body explicitly calls out an `AssertMaxQueries` lock on the
  cascade path to prevent N+1 regressions, with the expected query
  count documented.

## Codebase audit

What already exists that this rewrite leans on:

- **Proto.** `LeaveCommunityRequest.new_owner_user_id` is already
  `optional string`. `Community.owner_user_id` is already populated
  for every active row by #1644's backfill + the
  `community_owner_required` CHECK constraint added in #1652.
  `CommunityUser.deleted` (DeletedMetadata) is already in the schema
  from #1643.
- **Event types.** `COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED = 42`
  and `COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW = 43`
  already exist (`proto/ripls/models/community.proto:304,308`).
  `CommunityEvent.object_user_id` is the slot for the new owner on
  ownership-transferred events.
- **Undo registry.** All four event types (`MEMBER_LEFT`,
  `OWNERSHIP_TRANSFERRED`, `MEMBER_REJOINED_WITHIN_WINDOW`,
  `COMMUNITY_DELETED`) are already classified in
  `server/undo/registry.go`. Nothing to add for #1656.
- **Cascade helpers.**
  `CascadeDeleteCommunity{Gear,Request,Experience,NotificationPreferences,InvitationLink}ByCommunityID`
  exist in `server/storage/cascade_delete.go`. They're community-scoped,
  not leaver-scoped — we need new leaver+community-scoped helpers.
- **DeleteCommunity.** Recently rewritten in #1654 as
  `lifecycle.go:DeleteCommunity` and is the call target for the
  sole-member-leave case.
- **Race-safe claim pattern.** `ClaimCommunityRestore`
  (`server/storage/community.go:180`) is the template for the
  ownership-transfer atomic write — conditional UPDATE returning
  `RowsAffected`. This codebase **does not use SQL transactions**;
  atomicity comes from single-row conditional UPDATEs.

What's missing and must be built:

- **Leaver-scoped cascade helpers.** Five of them, mirroring the
  community-scoped ones but filtered by leaver+community:
  `CascadeDeleteCommunityGearByOwnerInCommunity`,
  `CancelLeaverOpenCommunityRequests`,
  `CancelLeaverActiveTransfersInCommunity`,
  `DropLeaverRSVPsInCommunity`,
  `DeleteLeaverCommunityNotificationPreferences`,
  `RevokeLeaverCommunityInvitationLinks`.
- **`ClaimCommunityOwnerHandoff` storage helper.** Conditional UPDATE
  that swaps `Community.owner_user_id` only if the current owner is
  still the leaver AND the candidate has an active `CommunityUser`
  row in the community. Returns `RowsAffected > 0`. Mirrors
  `ClaimCommunityRestore`'s shape but with a richer WHERE clause that
  requires the candidate's active membership via subselect.
- **`OWNERSHIP_TRANSFERRED` notification routing.** Three additions
  in `server/community/notifications.go`: `shouldNotify` returns
  true; `getNotificationRecipients` returns `[new_owner_user_id]`;
  `buildNotification` produces "You're now the owner of `<name>`."
- **`MEMBER_LEFT` and §2.7 deleted-community branch.** The dispatcher
  should already skip notifications for deleted communities (#1622),
  so leaving a deleted community is a defensive no-op for
  `MEMBER_LEFT` notifications. Verify in tests.

## Potential Solutions

### Option A: Single-RPC monolith (recommended)

- **Approach:** Keep `LeaveCommunity` as the one RPC. Branch internally:
  *non-owner*, *owner-with-handoff*, *sole-member*. Sole-member calls
  the existing `DeleteCommunity` handler (or its lifted core) inline so
  the leaver ends up with both `MEMBER_LEFT` (skipped — fully covered by
  the synthesized `COMMUNITY_DELETED`) and the soft-deleted community.
  Atomicity for owner-leave comes from `ClaimCommunityOwnerHandoff`,
  not from a SQL transaction.
- **Pros:**
  - Matches the proto contract exactly. `LeaveCommunityRequest.new_owner_user_id`
    is already there.
  - One RPC = one round-trip = no client-side orchestration.
  - Reuses #1654's `DeleteCommunity` for sole-member, no logic duplication.
  - Race-safe owner handoff via the same pattern that #1655 used for restore.
- **Cons:**
  - The handler grows. Mitigated by extracting per-branch helpers
    (`leaveAsNonOwner`, `leaveAsOwner`, `leaveAsSoleMember`).
  - Sole-member detection requires an extra query (active member count)
    before branching. This is a single `COUNT(*)`, not N+1.
- **Effort:** medium-large. ~600-800 lines including new cascade helpers,
  storage method, notification routing, integration tests.

### Option B: Split into three RPCs

- **Approach:** `LeaveCommunity`, `TransferOwnershipAndLeave`,
  `LeaveSoleMemberCommunity`. Client picks based on context.
- **Pros:** each handler simpler.
- **Cons:**
  - Contradicts §6.2 of the design doc, which explicitly says "one RPC
    (not two), so there is no window where ownership is undefined."
  - Adds three RPCs to the proto surface for what's logically one user
    action.
  - Forces the client to know the sole-member case before calling, which
    introduces a TOCTOU window: client sees "two members," picks the
    non-handoff path, by the time the call lands the other member left.
- **Effort:** medium, but **wrong shape** for the design.

### Option C: Hybrid — RPC stays one, but split `lifecycle.go` from `membership.go`

- **Approach:** Same as Option A behaviorally, but move the rewritten
  `LeaveCommunity` from `membership.go` to `lifecycle.go` next to
  `DeleteCommunity` and `RestoreCommunity`. The three are deeply
  intertwined (sole-member-leave calls Delete; the snapshot/cascade
  logic lives in `lifecycle.go`).
- **Pros:** keeps lifecycle code co-located. Makes the sole-member
  reuse trivially direct (no cross-file call).
- **Cons:** larger move, breaks `git blame` on the existing function.
  But the entire body is being rewritten anyway, so the blame loss is
  notional.
- **Effort:** same as A, plus a 30-second `git mv` of the function.

## Recommendation

**Option C.** Single RPC (per design doc), but co-locate the rewritten
function with `DeleteCommunity` / `RestoreCommunity` in `lifecycle.go`.
Move `RejoinCommunity` (currently a stub in `membership.go`) along with
it so `lifecycle.go` owns the full community-lifecycle surface and
`membership.go` shrinks back to its actual scope: list / search membership.

The body is being rewritten end-to-end, so there is no `git blame` cost
beyond what the rewrite already incurs. The win is that
sole-member-leave can call `DeleteCommunity`'s core (or a lifted
`deleteCommunityInternal` helper) without an unnatural cross-file call,
and a future reader looking at "what does the community lifecycle do?"
finds it all in one file.

## Open Questions

- **Should `leaveAsSoleMember` call `DeleteCommunity` directly or share
  a lifted internal helper?** —
  *Recommendation:* lift a `deleteCommunityInternal(ctx, community,
  actor)` helper that takes an already-fetched `*models.Community` and
  the actor, and have both `DeleteCommunity` and `leaveAsSoleMember`
  call it. This avoids re-doing the auth check, re-fetching the
  community, and the "is community already deleted?" no-op early-out
  (which the leave path doesn't need to consider — by definition
  we're about to soft-delete it).
- **Is `OWNERSHIP_TRANSFERRED` event recipient really only the new owner?** —
  *Recommendation:* yes, per §8 of the doc. The community is still
  active; everyone else can read the event log if they care. Push goes
  to the new owner only.
- **For owner handoff, do we need a `SELECT ... FOR UPDATE` on
  `Community`, or is `ClaimCommunityOwnerHandoff` (conditional UPDATE
  with member subselect) sufficient?** —
  *Recommendation:* the conditional UPDATE is sufficient and matches
  this codebase's transactionless idiom. The WHERE clause asserts (a)
  current owner is still the leaver and (b) candidate has an active
  membership. If either flips during the call, RowsAffected = 0 and
  we return `FailedPrecondition`. Document the trade-off in a comment
  on the helper. Reserve `FOR UPDATE` for a future refactor if we ever
  introduce a transaction-using path here.
- **Should leaver's `CommunityExperience` rows where leaver hosts stay
  active?** —
  *Recommendation:* yes, per §4 of the doc: "Events belong to the
  community. The leaver is responsible for handing off logistics
  before they go." Document explicitly in the cascade table comment
  so a future reader doesn't add an over-eager cleanup.
- **Should the leaver's authored `ChatMessage` rows in
  community-scoped conversations be cleaned up?** —
  *Recommendation:* no, per §4 ("leave in place, history"). Leaver's
  `Conversation` rows (1:1 chats they initiated) are personal, not
  community-scoped, and untouched. Leaver's contribution to a
  community-scoped group conversation stays in place; if the
  community is later deleted, #1622's deleted-community guard hides
  the conversation entirely.
- **What if the leaver IS the candidate `new_owner_user_id`?** —
  *Recommendation:* `InvalidArgument`. The candidate must be a *current
  active member other than the caller*. Validate early.
- **What if the candidate is shadow user / not yet registered?** —
  *Recommendation:* `InvalidArgument`. Ownership requires a real
  account. Validate by reading `CommunityUser.user_id` (non-empty,
  registered). Shadow-user membership rows have `shadow_user_id` set
  and `user_id` empty.
- **`AssertMaxQueries` budget for the cascade?** —
  *Recommendation:* document the expected count in a constant. For the
  non-owner path: 1 community read, 1 active-member-count, 5 cascade
  scans (gear, requests, transfers, RSVPs, notification prefs,
  invitation links — but each cascade does its own QueryByField +
  per-row Update, so the *bound* depends on row counts). Use
  `AssertMaxQueries` as a *regression guard* on a small fixture
  (e.g. `≤ 30` queries for a leaver with 2 gear / 1 request / 1
  transfer / 1 RSVP), not as a hard budget on production data. Land
  with a comment explaining the fixture-bound nature.

## Implementation Plan

### Phase 1: Storage helpers and cascade primitives

**Goal:** the building blocks the service handler needs, with their
own unit/integration coverage so the service-layer rewrite has stable
ground.

**Tasks:**
- [ ] Add `ClaimCommunityOwnerHandoff(ctx, communityID, fromUserID, toUserID) (claimed bool, reason string, err error)` in `server/storage/community.go`. Conditional UPDATE on `community` table that succeeds iff (a) `owner_user_id = fromUserID`, (b) the row is not deleted, (c) `toUserID` has an active `CommunityUser` row in the community. Returns `(false, "candidate_not_member", nil)` and `(false, "ownership_changed", nil)` distinguishably so the service can map to error codes; or return a single boolean and let the service do a follow-up read for the reason if simpler. Pattern-match on `ClaimCommunityRestore`.
- [ ] Add six leaver+community-scoped cascade helpers in `server/storage/cascade_delete.go`:
  - `CascadeDeleteCommunityGearByOwnerInCommunity(ctx, storage, communityID, ownerUserID, deletedMetadata)` — soft-deletes `CommunityGear` rows whose `Gear.OwnerId == ownerUserID` and `community_id == communityID`. Either a JOIN-style query or a two-step (gear-by-owner + community-gear-by-gear-and-community).
  - `CancelLeaverCommunityRequestsInCommunity(ctx, storage, communityID, userID, deletedMetadata)` — silently transition `Request` rows authored by leaver to `REQUEST_STATE_CANCELLED` and soft-delete the `CommunityRequest` join row. **No `REQUEST_CANCELLED` event, no chat retraction:** these are subsumed by the leaver's single `MEMBER_LEFT` event. (See Audit Notes for the rationale; do NOT call `request.Service.CancelRequest` from the community handler — architecture §1.)
  - `CancelLeaverActiveTransfersInCommunity(ctx, storage, communityID, userID, cancelMetadata)` — silently set `state = TRANSFER_STATE_CANCELLED` for `Transfer` rows where `community_id == communityID` AND (`owner_id == userID` OR `recipient_id == userID`) AND `state IN (INTEREST_EXPRESSED, RECIPIENT_SELECTED, ACTIVE)`. **No `TRANSFER_CANCELLED` event** — same rationale as `CancelLeaverCommunityRequestsInCommunity`.
  - `DropLeaverExperienceRSVPsInCommunity(ctx, storage, communityID, userID)` — query `CommunityExperience` for the community's experience IDs, then `CommunityExperienceUser` filtered by user_id IN (community's experiences). Hard-delete (it's a join/preference row, not history).
  - `DeleteLeaverCommunityNotificationPreferences(ctx, storage, communityID, userID)` — `CommunityNotificationPreferences` row keyed `(community_id, user_id)`. Hard-delete; preferences are reset on rejoin.
  - `RevokeLeaverCommunityInvitationLinks(ctx, storage, communityID, userID, deletedMetadata)` — soft-delete `CommunityInvitationLink` rows where `community_id == communityID` AND `created_by_user_id == userID`.
- [ ] Unit tests for each cascade helper: empty case, single-row case, multi-row case, idempotency (rerun is no-op), and a "wrong scope" case (rows that should NOT be touched).
- [ ] `AssertMaxQueries` integration test that constructs a fixture leaver with bounded data and asserts the query count for the full cascade is below the documented bound.

**Validates:** primitives are race-safe, idempotent, scoped correctly, and bounded in query count.

### Phase 2: Service handler rewrite — non-owner branch

**Goal:** the simplest of the three branches lands first, in isolation,
fully tested. This proves the cascade works end-to-end before the
ownership-transfer complexity lands.

**Tasks:**
- [ ] (file move) `git mv` `LeaveCommunity` and `RejoinCommunity` from `membership.go` into `lifecycle.go`. Update tests and any package-internal references.
- [ ] In `lifecycle.go`, lift the existing `DeleteCommunity` body into a `deleteCommunityInternal(ctx, community, actor)` helper that takes an already-fetched community and runs snapshot+cascade+event. `DeleteCommunity` RPC becomes a thin wrapper: auth, fetch, owner-only check, no-op-if-deleted, then call `deleteCommunityInternal`. Verify all #1654 tests still pass.
- [ ] Rewrite `LeaveCommunity` skeleton:
  1. Auth.
  2. Fetch community with `IncludeDeleted: true` (treat already-deleted community as `FailedPrecondition: community_deleted` — leaving a deleted community is meaningless; the user sees nothing of it).
  3. Fetch the leaver's `CommunityUser` (active rows only). If absent, `NotFound`.
  4. Detect sole-member: count active members. If 1 → branch to `leaveAsSoleMember` (Phase 4).
  5. Detect owner: `community.owner_user_id == authInfo.UserID`. If true → branch to `leaveAsOwner` (Phase 3).
  6. Else → `leaveAsNonOwner` (this phase).
- [ ] `leaveAsNonOwner`:
  1. Run the six cascade helpers from Phase 1 with `now`-based `DeletedMetadata`. Best-effort with WARN per helper, mirroring the #1654 pattern.
  2. Soft-delete the leaver's `CommunityUser` row (set `Deleted = DeletedMetadata`, `Update`).
  3. Emit `MEMBER_LEFT`. Existing dispatcher already routes to community owner only.
  4. Recompute regions (existing `recomputeCommunityRegions`).
- [ ] Use `connecterr.Internal(ctx, "LeaveCommunity", err, ...)` for every internal-error return — bare `connect.NewError(connect.CodeInternal, err)` fails CI (`scripts/check_codeinternal.js`).
- [ ] Logger setup at the top of the handler: `logger := logging.LoggerWithContext(ctx).With("operation", "LeaveCommunity", "user_id", authInfo.UserID, "user_email", logging.MaskEmail(authInfo.Email), "community_id", req.Msg.CommunityId)` — extend with `new_owner_user_id` inside `leaveAsOwner`. Cascade-helper failures land at WARN with `cascade_target` matching the existing #1654 pattern.
- [ ] Integration tests live at `server/integration_tests/community_leave_test.go` (matching the per-feature pattern set by #1654 / #1655, not the legacy `main_integration_test.go`):
  - Happy path — every category in §4 cleaned up, leaver's `CommunityUser` soft-deleted, event emitted, owner notified.
  - Leaver has no gear / no requests / no transfers — no-op cascades, no errors.
  - Already-soft-deleted leaver (e.g. crash recovery) — idempotent: claim+resync don't crash. *Per spec we shouldn't see this since pre-state is "active member"; but defensive test.*
  - Caller is not a member — `NotFound`.
  - Community is in soft-deleted state — `FailedPrecondition: community_deleted`.

**Validates:** the cascade is correct, soft-delete preserves the rejoin window, MEMBER_LEFT still fires, and the §4 ledger is fully implemented for the most common case.

### Phase 3: Owner-leave with handoff

**Goal:** owner-leave is atomic and race-safe. This is the highest-risk
branch because it moves ownership; we want it under heavy concurrent
test pressure before merging.

**Tasks:**
- [ ] `leaveAsOwner`:
  1. Validate `req.Msg.NewOwnerUserId` is set → else `InvalidArgument: missing_new_owner`.
  2. Validate `NewOwnerUserId != caller` → else `InvalidArgument: candidate_is_caller`.
  3. Look up the candidate's `CommunityUser` row (active only). If absent, return `FailedPrecondition: candidate_not_member`.
  4. `ClaimCommunityOwnerHandoff(ctx, communityID, caller, NewOwnerUserId)`:
     - If claimed: continue.
     - If not claimed and candidate still member but owner changed: `FailedPrecondition: ownership_changed`.
     - If candidate disappeared mid-call: `FailedPrecondition: candidate_not_member`.
  5. Resync in-memory `community` (`OwnerUserId = NewOwnerUserId`, `UpdatedAtUnixSec = now`) and `Update` to rewrite `binary_proto`.
  6. Run §4 cascade for the leaver (six helpers, same as non-owner).
  7. Soft-delete the leaver's `CommunityUser` row.
  8. Emit `OWNERSHIP_TRANSFERRED` (actor = caller, object_user_id = new owner). Notification (added in this phase) fires only to the new owner.
  9. Emit `MEMBER_LEFT` (actor = caller). Existing dispatcher routes to current owner — by this point that's the new owner — so they get one push for OWNERSHIP_TRANSFERRED but not a duplicate "X left" since you don't notify yourself.
  10. Recompute regions.
- [ ] Wire `OWNERSHIP_TRANSFERRED` into `server/community/notifications.go` (server-side Go, not ARB — push copy is not localized today, see precedent for `COMMUNITY_DELETED` / `COMMUNITY_RESTORED`):
  - `shouldNotify` returns true.
  - `getNotificationRecipients` returns `[event.ObjectUserId]`.
  - `buildNotification` returns title `"You're now the owner of <name>."` body `"<oldOwner> has transferred ownership of <name> to you."` (or per §8: `"You're now the owner of <name>."` as the body — pick whichever reads better in app context, both are spec-compatible).
- [ ] Integration tests:
  - Owner-leave happy path — new owner gets push, ownership flips, leaver is soft-deleted, cascade runs.
  - `new_owner_user_id` unset → `InvalidArgument`.
  - `new_owner_user_id == caller` → `InvalidArgument`.
  - Candidate not a member → `FailedPrecondition: candidate_not_member`.
  - Candidate is shadow user (no real account) → `FailedPrecondition` or `InvalidArgument` (decide and document; recommend `InvalidArgument` since it's the request that's malformed).
  - **Owner-leave race:** two concurrent owner-leaves from the same owner targeting two different candidates. Use `goroutine` + `sync.WaitGroup` + the test storage. Assert exactly one succeeds, the other gets `FailedPrecondition: ownership_changed`, and the community ends with exactly one valid owner. Mirror the §9.1 test description.
  - **Candidate-leaves-while-owner-handoff race:** owner is leaving with handoff to A; concurrently A leaves. Assert one consistent terminal state. Either the handoff lands first (A is now owner, then A leaves with their own handoff or it errors if A is now sole-member and didn't pass `new_owner`), or A's leave lands first (handoff finds A is no longer a member, returns `FailedPrecondition`).

**Validates:** the never-zero-owners, never-two-owners invariant. `OWNERSHIP_TRANSFERRED` notification arrives. Race coverage in CI prevents regression.

### Phase 4: Sole-member-leave converts to delete

**Goal:** the §2.4 case: leaver is the only active member. Convert to
soft-delete with the leaver as the actor, so the leaver can later
restore from Settings → Communities.

**Tasks:**
- [ ] `leaveAsSoleMember`:
  1. Call `deleteCommunityInternal(ctx, community, caller)` (the helper lifted in Phase 2). This runs the snapshot (single member = caller), cascade-soft-deletes all join rows, emits `COMMUNITY_DELETED`, notifies snapshot members (just the caller — and the dispatcher doesn't notify the actor about their own action).
  2. Do NOT separately soft-delete the leaver's `CommunityUser`. The §6.4 audit event is `COMMUNITY_DELETED`, not `MEMBER_LEFT`. The membership row stays active under a deleted community, which is consistent with #1654's behavior.
  3. Do NOT emit `MEMBER_LEFT`.
- [ ] Integration tests:
  - Sole-member leave → community is soft-deleted with caller in snapshot, all join rows cascaded.
  - Sole-member-leave → restore: caller subsequently calls `RestoreCommunity` (#1655). Assert restore succeeds and caller is the new owner with active membership.
  - Sole-member-leave → 31 days later → community is purged (deferred to #1620, not testable here, but document the round-trip).
  - Owner-leave-with-handoff race that *concurrently* drops the second-to-last member: owner picks A as new owner; concurrently A leaves. After the dust settles, the community should be in a consistent state — either owned by A (who then becomes sole member of a 1-member community) or owned by the original caller (because A's leave landed first and the handoff failed). Assert no zero-owner state.

**Validates:** the §2.4 conversion is clean, the sole-member leaver retains restore eligibility, and the cross-flow with #1655 round-trips correctly.

### Phase 5: System-test and docs polish

**Goal:** the workflow doc and system-test corpus reflect the new behavior so future regressions get caught by the corpus, not just unit tests.

**Tasks:**
- [ ] Update `docs/workflows/community.md` (or whatever the workflow doc is named) with: leave-as-non-owner, owner-leave-with-handoff, sole-member-leave-as-delete, and how each interacts with rejoin and restore.
- [ ] Run `/ripls-update-system-tests` (per §9.3 of the design doc) to regenerate the system-test corpus.
- [ ] Cross-link the leave flow from `docs/deleting_things.md`.
- [ ] Verify `TestEveryEventTypeClassified` still passes (no changes — registry is already covered).
- [ ] Verify `TestEveryServerUndoableActionHasUndoDataValidator` is unaffected (`MEMBER_LEFT` is `ClassificationClientOnly`, no UndoData needed).
- [ ] Close #1571 as superseded with a note pointing at #1656's PR.

**Validates:** the corpus and docs are in sync with shipped behavior; #1571 is officially closed.

## Audit Notes

Reviewed against `docs/server/architecture.md` and
`docs/server/observability.md`. Findings folded into the plan above:

- **Service-to-service boundary (architecture §1).** The plan originally
  said `leaveAsSoleMember` would "call `DeleteCommunity` directly." That
  is in-service (same `community.Service`, same package), so it does
  not violate the cross-service rule — but the `deleteCommunityInternal`
  helper lift in Phase 2 is still the right call because it avoids
  re-doing auth/fetch/no-op-early-out on a code path where the caller
  already has the community in hand. **No change beyond keeping the
  lift explicit.**
- **Cross-service writes via storage helpers, not RPC re-entry.** The
  cascade touches `Request` (request service domain) and `Transfer`
  (transfer service domain). Per architecture §1, the leave handler
  must not call `request.Service.CancelRequest` or
  `transfer.Service.CancelTransfer`. The plan already puts the work in
  `server/storage/cascade_delete.go`, which is the correct shared
  layer. **Added clarifying note:** the storage helpers do a *silent*
  state change (no `REQUEST_CANCELLED` / `TRANSFER_CANCELLED` events,
  no chat retractions). User-visible signal for the leaver's cancelled
  artifacts is the single `MEMBER_LEFT` event. If we later decide
  per-artifact retraction is worth the noise, factor cancel-core helpers
  out of the request and transfer services into shared libraries — do
  not call the services from the community handler.
- **Notification copy is server-side Go, not ARB (i18n).** Phase 3's
  task list said "Add ARB strings if any user-facing copy is generated
  server-side." Server-generated push title/body lives in
  `server/community/notifications.go:buildNotification` (see
  `COMMUNITY_DELETED` and `COMMUNITY_RESTORED` precedent). Push copy
  is **not** localized today and ARB does not factor in. **Removed the
  ARB step;** the only deliverable for Phase 3 notification work is
  the three additions in `notifications.go`.
- **Integration test location.** Architecture §7 puts integration
  tests in `server/main_integration_test.go`, but #1654 and #1655
  established the per-feature pattern under
  `server/integration_tests/community_*_test.go`. **Pinned the
  expected file path:** `server/integration_tests/community_leave_test.go`.
- **Logging conventions (observability §Standard Field Names).**
  Pinned the field names every handler/cascade-helper log line should
  carry: `operation` (e.g. `LeaveCommunity`, `leaveAsOwner`,
  `CascadeDeleteCommunityGearByOwnerInCommunity`), `community_id`,
  `user_id`, `user_email` (masked via `logging.MaskEmail`), and for
  owner-leave specifically `new_owner_user_id`. Use
  `logging.LoggerWithContext(ctx).With(...)` at the top of every
  exported handler. Cascade-helper failures land at WARN with
  `cascade_target` matching the existing #1654 pattern.
- **`connecterr.Internal` (architecture §Error wrapping).** All
  internal-error returns must use `connecterr.Internal(ctx, op, err,
  kv...)`. The bare `connect.NewError(connect.CodeInternal, err)`
  pattern fails CI via `scripts/check_codeinternal.js`. **Added
  explicit reminder in Phase 2 task list.**
- **Goroutine safety (architecture §Goroutine Safety).** This rewrite
  spawns no new goroutines. `RecordCommunityEventAndNotify` already
  uses `GoSafe` internally for the notify fan-out. **No additional
  work needed.**
- **N+1 and `AssertMaxQueries`.** Confirmed
  `storage.AssertMaxQueries(t, ctx, max, fn)` is the right helper
  (CLAUDE.md SQL Efficiency §8). Phase 1's regression test should use
  a documented small fixture and assert on a max bound, with the bound
  derived empirically from the first passing run.
- **`ListCommunityCandidates` RPC.** Per the issue body, the
  owner-leave handoff picker is "**new** or reuse `ListCommunityUsers`."
  **Pinned to reuse:** the client filters `ListCommunityUsers` results
  to active members minus the caller, no new RPC needed for #1656.
  Server still re-validates the candidate at handoff time — the client
  list is advisory, not authoritative.
- **Audit-trail event ordering for owner-leave.** Phase 3 originally
  emitted `OWNERSHIP_TRANSFERRED` then `MEMBER_LEFT`. **Confirmed
  this order is correct.** Reading the event log left-to-right, a
  support engineer sees ownership move to the new owner first, then
  the previous owner depart — which matches the user's mental model
  and the §6.4 audit requirement. The push notification dispatcher
  does not double-notify the new owner because they are the actor of
  exactly one event (object_user_id), not the actor of both.

The plan is now consistent with `docs/server/architecture.md` and
`docs/server/observability.md`. Ready for implementation.

## Related Issues

- **#1619** — parent epic.
- **#1571** — closes-as-superseded by Phase 1's `CommunityGear` cascade row.
- **#1654** — DeleteCommunity rewrite (sole-member-leave reuses; merged).
- **#1655** — RestoreCommunity (sole-member-leave→restore round-trip; PR #1706 open).
- **#1657** — RejoinCommunity (un-soft-delete the row this issue creates).
- **#1622** — notification dispatcher's deleted-community guard (handles "leaver in deleted community" path).
- **#1620** — daily community purge job (eventually hard-deletes the row this issue soft-deletes).
