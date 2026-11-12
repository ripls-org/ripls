# Issue #1665: Chat not loading for item shared with multiple communities

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1665
**Filed:** 2026-05-02 | **Last activity:** 2026-05-04
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none (this plan reads `docs/client/chat.md`, `docs/client/sharing.md`, and the #1705 umbrella plan as required reading)
**Audit pass:** 2026-05-04 — see [Audit Notes](#audit-notes) for what changed and why.

## Problem Statement

User opens a gear shared into multiple communities, where they are a member of one but not the others. The chat tab on the gear shows an unread badge ("2") but tapping the chat surfaces a `permission_denied` error toast.

This is one issue ticket, but it covers **two distinct bugs** that happen to surface together for the same user action:

1. **Unread-count data leak** (the badge shows "2"): the server-side unread-count aggregator returns counts from conversations tied to communities the user is no longer a member of. The badge is *displaying data the user shouldn't see at all.*
2. **Chat-load failure** (tapping fails with permission_denied): tapping the badge tries to open the conversation behind it, which is bound to a community the user is not in, so the server (correctly) refuses.

The two are causally linked — the user only taps because the badge tells them there's something to read. Fixing (1) makes (2) unreachable through this particular path. But (2) may also be reachable through other paths and is worth verifying once (1) is fixed.

This issue is part of umbrella #1705. Sibling fixes already shipped: #1707 (gear borrow), #1708 (gear page-load), #1710 (member-list redaction), #1694 (invite picker filter), #1713 (ItemShareSheet typed error).

## Context from Comments

One comment, from briangbrown (the issue author and a member): filed the umbrella #1705 covering this issue plus #1684, #1685, #1703. Notes the unifying root cause — client RPCs scope by single `community_id` from local UI state — and the asymmetry between auto-detecting endpoints (e.g., `ExpressInterest`) and strict-membership endpoints (e.g., `GetGearTransferContext`). No other discussion.

The umbrella plan (`docs/issues/1705-multi-community-scoping.md`) classifies this as Phase 4-chat with two sub-tasks: (i) fix chat-load endpoint scoping (likely category b2 → server-side any-community auth helper), and (ii) fix the unread-count data leak (likely missing membership JOIN in the aggregator query). Both are server-side root causes, distinguishing this from prior Phase 4 fixes which were client-side.

## Codebase Inventory

Research pass via `Explore` agent on 2026-05-04. Full file:line refs follow.

### Half B — Unread-count data leak (the badge-shows-count-you-shouldn't-see bug)

The badge wraps `getUnreadCounts`. Path:

| Layer | File | Line(s) | Behavior |
|-------|------|---------|----------|
| Client repository | `app/lib/data/repositories/unread_count_repository.dart` | 32–50, 72–92 | `getTotalUnreadCount` / `getUnreadCounts` delegate to `_service.getUnreadCounts()`. |
| Client service | `app/lib/services/chat_service.dart` | 405–420 | RPC stub. |
| Server handler | `server/services/chat/unread_counts.go` | 20–118 | `GetUnreadCounts`: at line 37 calls `ListByParticipant(ctx, authInfo.UserID, 0)`; at line 102 reports per `conversation.CommunityId` with no membership filter. |
| Storage query | `server/storage/chat_conversation.go` | 101–108 | `ListByParticipant` SQL: filters on `? = ANY(participant_ids) AND deleted_deleted_at_unix_sec = 0`. **No JOIN to `community_user` and no membership predicate.** |

**Bug class**: leak-class, server-side. The client path is fine — it asks the server "what are my unread counts?" and the server returns counts from conversations the user has *historically participated in*, regardless of whether they currently belong to the conversation's community. Once a user leaves a community whose gear they previously chatted on, the server keeps reporting unread counts from that community's conversations.

### Half A — Chat-load failure (the tap-fails bug)

| Layer | File | Line(s) | Behavior |
|-------|------|---------|----------|
| Client (gear) | `app/lib/presentation/screens/gear/gear_content_view.dart` | 525–549 | `_buildChatPane` passes `gear.conversationId` to `InlineConversationView`. The conversationId comes from `state.gearDetails.conversationId`, which is per-community. |
| Client viewmodel | `app/lib/presentation/viewmodels/gear_view_model.dart` | 295–299 | `loadGearDetails` uses `resolveCommunityId({…, userMemberIds})` (#1708 fix) to pick a community the user *is* in before fetching gear details. |
| Client repository | `app/lib/data/repositories/chat_repository.dart` | n/a | Calls `getConversation`. |
| Server handler | `server/services/chat/conversations.go` | 249–316 | `GetConversation`. Line 265 fetches by ID; lines 302–309 call `addUserAsParticipant` for gear/experience/request conversations. |
| Server membership check | `server/services/chat/conversations.go` | 102–160 | `addUserAsParticipant`: collects all `sharedCommunityIDs` for the gear/experience/request (line 120), batches `GetCommunitiesWithMembership` (line 133), and grants access if the user is a member of **any** active shared community (138–157). |

**Bug class assessment**: ambiguous. The agent's read of the membership-check logic says `GetConversation` *already* grants access if the user is in any shared community of the gear — i.e., the chat-load path is correctly any-community-aware on the server today. After #1708's client fix, the conversationId the gear screen reads should also belong to a member-community.

That said, the user reported a real `permission_denied` toast. Possible explanations:
- (a) **Pre-#1708 behavior**: User was on a build before #1708 merged. `loadGearDetails` was passing a non-member `state.communityId`, so `gearDetails.conversationId` belonged to a community the user wasn't in, so opening that specific conversation hit a different gating path. Repro now (post-#1708) may not surface the failure on the chat tab.
- (b) **Other gating path**: a different RPC in the chat-open flow (e.g., `ListMessages`, presence updates, send-message permissions) may still gate on `RequireMemberOfActiveCommunity(conversation.CommunityId, …)` rather than the any-shared-community helper. Worth verifying.
- (c) **Unread-count tap navigation**: tapping the badge may route to a *specific* conversation by `conversationId` (the one with unread messages), bypassing the gear-page resolution. If the unread came from a non-member community, this tap navigates straight to a non-member conversation.

Hypothesis (c) is the strongest explanation for why fixing Half B fixes the user's symptom: once the leaked counts disappear, the user has no badge to tap, so the chat-load failure path (real or not) isn't reached.

### Conversation model relevant to the fix

Per `docs/client/sharing.md`:
- **Gear**: one conversation per community (`CommunityGear.conversation_id`). Multi-community gear has N conversations.
- **Request**: one conversation per community (`CommunityRequest.conversation_id`). Same shape as gear.
- **Experience**: one *global* conversation (`Experience.conversation_id`) shared across all communities. Per-community conversations exist in storage but are not surfaced.

Implication: the unread-count leak shape is identical for gear and request (per-community conversation, missing membership filter). For experience the global conversation is reachable from any community, so the leak shape is different — counts on the global conversation aren't community-scoped to begin with. Likely no leak for experiences, but worth confirming.

### Existing helpers / precedents

- `server/auth/community_active.go` — `RequireMemberOfActiveCommunity` (single-community check).
- `server/auth/membership.go` — likely site for a new `IsMemberOfAnyCommunity(communityIDs, userID) bool` helper if needed; the umbrella plan calls this out as the Phase 3 server-side helper (`docs/issues/1705-multi-community-scoping.md` §"Phase 3").
- `app/lib/core/utils/community_resolver.dart` — client-side `resolveCommunityId({…, userMemberIds})`. Already imported by `loadGearDetails`.
- `app/lib/core/utils/community_helper.dart` — `CommunityHelper.invitableSharedCommunities`. Same intersection pattern at the picker layer.
- Related shipped Phase 4 fixes: #1708 (gear page-load), #1710 (AccessSheet redaction + ManageMembers error binding), #1694 (invite picker filter).

## Potential Solutions

### Option A: Fix Half B server-side (membership-aware `ListByParticipant`); leave Half A pending verification

Modify the unread-count query to filter to conversations whose `community_id` corresponds to a community the user is currently a member of. Two sub-options:

- **A1 (SQL JOIN, preferred)**: add a sibling to `ListByParticipant` (e.g., `ListByParticipantInActiveCommunities`) that enforces `EXISTS (SELECT 1 FROM community_user WHERE community_id = chat_conversation.community_id AND user_id = ? AND COALESCE(deleted_deleted_at_unix_sec, 0) = 0)`. Single round trip, indexed, mirrors the existing soft-delete idiom in `ListByParticipant` itself (`chat_conversation.go:104–105`).
- **A2 (in-Go filter via existing helper)**: keep the SQL as-is, fetch then filter in `unread_counts.go` using the existing `storage.ProtoSQLStorage.GetCommunitiesWithMembership(communityIDs, userID)` (`server/storage/community.go:87`). Drop conversations whose `community_id` isn't in the returned membership set. Trades single-round-trip efficiency for reuse of an existing, already-tested helper that aligns with `docs/server/architecture.md` §2 ("Shared Functionality in Libraries").

After A: the badge no longer shows leaked counts → the user's reported tap-fails-with-permission-denied is unreachable through the badge. Verify the chat-load path independently against fresh logs from the post-#1708 build to confirm Half A doesn't exist (or files a separate sub-issue if it does).

- **Pros**: Closes the *security/privacy concern* (counts from communities the user isn't in are visible). Server-only change. Doesn't disturb the client. Fits squarely within the #1705 umbrella's "category leak" classification.
- **Cons**: Doesn't address Half A directly — relies on the Half B fix making it unreachable through this user's path. If Half A is reachable through a different path (deep link, push notification tap), it stays open.
- **Effort**: Small. One server change, one storage change, one or two server tests.

### Option B: Fix both halves: A1 + add server-side any-community auth helper for chat endpoints

A1 plus retrofit the chat conversation handlers to use a new `auth.IsMemberOfAnyCommunityFor(itemID, userID)` helper anywhere they currently call `RequireMemberOfActiveCommunity(conversation.CommunityId, userID)`. The agent's research suggests `addUserAsParticipant` already does this, but other RPCs in the chat surface (`ListMessages`, `MarkRead`, `SendMessage`) may not. Audit each.

- **Pros**: Closes both halves definitively. Brings chat into alignment with the any-community pattern that the umbrella plan recommends for read paths. Addresses (b)/(c) hypotheses for Half A directly.
- **Cons**: Larger scope. Auth model change requires per-endpoint security analysis (which RPCs really need any-community vs. specific-community? `SendMessage` arguably should require active membership — sending into a community you left is wrong). Cross-cuts the "scope after #1665" boundary into Phase 3 territory.
- **Effort**: Medium. Audit + per-endpoint changes + tests.

### Option C: Fix only Half A client-side; defer Half B to a separate ticket

Mirror #1708's client-side resolution pattern to chat. When the chat tab opens for a multi-community gear, resolve to a member-community before reading `conversationId`. The agent's research suggests this is already done by #1708's `loadGearDetails`, so Option C may be a no-op.

- **Pros**: Minimal surface area; matches the Phase 4-gear-page-load pattern.
- **Cons**: Doesn't address the leak (the higher-severity bug). Likely already done. The user's reported symptom (badge → tap fails) is rooted in Half B, not Half A.
- **Effort**: Trivial-to-zero, but doesn't actually fix the reported issue.

## Recommendation

**Option A1** (server-side SQL filter on `ListByParticipant`) for Phase 1, with a Phase 2 verification of Half A.

Why:
- The user's reported symptom is rooted in Half B (the badge appearing where it shouldn't). Fixing Half B makes the symptom non-reproducible through the badge-tap path.
- Half B has a real privacy concern (the *count* leaks the existence of activity in communities the user isn't in), independent of whether Half A is reachable. This is the higher-severity half.
- Half A may be a phantom — the agent's research suggests `addUserAsParticipant` already grants any-community access. Burning effort on a fix that may be a no-op is poor sequencing. Phase 2 verifies it before fixing.
- A1 over A2: SQL JOIN is the right level. Filtering in Go is a workaround for an SQL gap, not a fix. Once we touch the storage layer we should fix it there.

Option B is the right long-term answer if Phase 2 surfaces real Half A bugs, but should be split into a follow-up rather than block this PR.

## Open Questions

- **Q1: Does the unread-count leak actually surface across community boundaries today, or is it specific to "user-left-community-after-participating" scenarios?** — Recommendation: **likely both, but the user-left-community case is the one observed in the wild**. The leak surface is "any conversation with the user in `participant_ids`," and `participant_ids` is updated when the user opens a conversation. Once added, the user remains a participant even after leaving the community. Verify via repro: have a test user join a community, open a gear chat there, leave the community, then check whether the gear's chat unread count still appears on the user's tab. Fix should cover this scenario (most natural reading of the SQL change). A separate scenario — being mistakenly added to `participant_ids` for a community they were never in — would be a different bug; not in scope here.
- **Q2: Should the server return zero counts or omit conversations entirely from the response when the user isn't a current member?** — Recommendation: **omit entirely**. The unread-count response is a list of `(conversationId, count)` pairs; emitting `count = 0` for non-member conversations gives the client a list it shouldn't even know exists. Omitting means the client UI naturally has nothing to render or tap.
- **Q3: Is there an analogous leak in any *other* aggregate endpoint that scans by participant or member-of-community?** — Recommendation: **defer to Phase 5 audit**. Out of scope for this PR; flag in the umbrella plan. Candidates per the umbrella inventory: feed aggregates (`server/services/feed/`), portfolio aggregates (`server/services/portfolio/`), badge counts. None reported as bugs yet, so don't preempt.
- **Q4: For Half A verification (Phase 2), what's the canonical repro?** — Recommendation: a fresh logcat capture on a post-#1708 build, on a multi-community gear where the user is a member of one community but not all, with the unread badge first cleared. If chat-load fails when there's no badge, the bug is independent of the leak and needs a separate fix. If it doesn't fail, close Half A as fixed-by-#1708.
- **Q5: Should `participant_ids` be cleaned up when a user leaves a community?** — Recommendation: **no, not in this PR**. That's a data-retention question that affects history (e.g., past-participant visibility), not the current bug. The fix here is scoping the *current view* of unread counts to current membership; participation history can stay intact.
- **Q6: How should the SQL change be tested?** — Recommendation: **integration test with `pgvector/pgvector:pg16` testcontainer** (the existing pattern per `CLAUDE.md` §Testing). Set up a user, two communities, a gear shared with both, a conversation tied to community A, the user as a participant in that conversation. Then remove the user from community A. Verify `ListByParticipant` (or whatever helper we expose) does NOT return that conversation for the user. Also verify it DOES return conversations tied to community B (where the user is still a member).

## Implementation Plan

### Phase 1: Server-side membership filter on unread-count query
**Goal:** Close the unread-count data leak. Stop returning counts from conversations whose communities the user is not a current member of.

**Tasks:**
- [ ] Add a new storage method `ListByParticipantInActiveCommunities(ctx, userID, limit) ([]*models.ChatConversation, error)` in `server/storage/chat_conversation.go`. Implementation: same as existing `ListByParticipant` but with an additional predicate enforcing current community membership.
  - **Schema reality** (verified during audit): `community_user`'s soft-delete metadata is the proto-mapped `DeletedMetadata` message, which materializes as the columns `deleted_deleted_by_user_id` and `deleted_deleted_at_unix_sec` (compare `server/storage/community.go:187` for the existing restore query that updates these columns). The active-membership predicate therefore looks like:
    ```sql
    EXISTS (
      SELECT 1 FROM community_user cu
      WHERE cu.community_id = chat_conversation.community_id
        AND cu.user_id = $1
        AND COALESCE(cu.deleted_deleted_at_unix_sec, 0) = 0
    )
    ```
    (Mirrors the soft-delete idiom already in `ListByParticipant` itself: `COALESCE(deleted_deleted_at_unix_sec, 0) = 0` at `chat_conversation.go:104–105`.)
  - **30-day rejoin window**: per `proto/ripls/models/community.proto:73-94`, a `CommunityUser` row is soft-deleted on leave and may be restored on rejoin. The predicate above correctly excludes soft-deleted rows, which matches the user-facing definition of "active member." Audit confirmed `auth.IsMemberOfCommunity` (`server/auth/community.go:16-39`) does NOT explicitly filter on the deleted column; it relies on `QueryByFields` semantics. If `QueryByFields` does NOT exclude soft-deleted rows by default, then `IsMemberOfCommunity` itself is buggy and a separate ticket; either way, the unread-count fix should explicitly enforce the predicate (defensive, fewer assumptions about upstream behavior).
  - **Experience global conversations**: `Experience.conversation_id` (`docs/client/sharing.md`) is global, not community-scoped. Inspect the `community_id` column for global-experience rows in `chat_conversation` — if it's empty/unset, the EXISTS subquery will fail and counts will be erroneously dropped. Need to either (a) include rows with empty `community_id` unconditionally via `OR community_id = ''`, or (b) use a `LEFT JOIN` and a different predicate. Verify the actual storage shape during implementation before committing to (a) vs (b).
- [ ] Wire `unread_counts.go:37` to call the new method instead of `ListByParticipant`. Keep the existing method intact (it has other callers; do not break them).
- [ ] Logging: preserve the existing `operation`/`user_id` keys per `docs/server/observability.md`. If adding a "how many were filtered out" signal is useful for observability, log at `DEBUG` (not INFO/WARN — this is internal aggregation state, not an unexpected condition or user action). Field name suggestion: `filtered_conversation_count` (descriptive, follows the snake_case standard-field convention). Per observability.md §"Standard Field Names," do not reuse `request_id` for any non-correlation purpose.
- [ ] Errors: the new storage method must propagate query errors via `connecterr.Internal(ctx, op, err, kv...)` per `CLAUDE.md` and `observability.md` (raw `connect.NewError(connect.CodeInternal, err)` is gated by `scripts/check_codeinternal.js`). The membership-filter SQL is a single round trip; on failure we want the generic public message and the detailed err logged server-side.
- [ ] Storage-layer unit test in `server/storage/chat_conversation_test.go` (or sibling) covering: user in member-community returns conversations; user removed from community no longer returns them; experience global conversations (empty community_id) still return.
- [ ] Service-layer test in `server/services/chat/unread_counts_test.go` (existing file or new) that exercises the full `GetUnreadCounts` handler with a multi-community-gear fixture: user removed from one community sees only the other's unread count.
- [ ] N+1 guard via `storage.AssertMaxQueries(t, ctx, max, fn)` per CLAUDE.md §"SQL Efficiency" — verify the new path is single-query.
- [ ] `npm run build` (gofumpt + golangci-lint + `go build ./...`) clean. `go test ./server/...` passes. Integration test (`go test -tags=integration ./...`) clean for chat services.

**Validates:** A user who has left a multi-community-gear's other community no longer sees a leaked unread count on that gear. Chat tab badge accurately reflects only conversations in current member communities. The user's reported "I see a 2 bubble" symptom no longer reproduces.

### Phase 2: Verify Half A (chat-load failure) on post-fix build
**Goal:** Determine whether the chat-load failure exists independently or is purely a downstream effect of the badge-leak.

**Tasks:**
- [ ] Build the Phase 1 server change and deploy to dev.
- [ ] Manual repro on the multi-community gear `c2dfd843-...`: confirm badge no longer appears. Tap the chat tab directly; capture logcat for any RPC failures.
- [ ] If chat-load succeeds with no permission_denied, document under Half A as "fixed by #1708 + Phase 1 of #1665"; close this issue.
- [ ] If chat-load still fails, capture the exact failing RPC + stack and file a sub-issue (#1665a or similar) for the residual server-gating gap. Most likely candidates: `ListMessages`, `MarkRead`, presence updates, or a deep-link / notification-tap path that bypasses the gear-page resolution. Plan a server-side fix (Option B-style) in that sub-issue.

**Validates:** Closes the user's reported issue, or surfaces a precise residual bug worth its own ticket. Either outcome is a clean stopping point.

### Phase 3: Membership filter audit on adjacent aggregates (deferred — feeds umbrella Phase 5)
**Goal:** Find sibling leaks in other aggregate endpoints.

**Tasks:**
- [ ] Audit `server/services/feed/`, `server/services/portfolio/`, and any other `*Counts*` / `*Aggregate*` endpoint for the same shape: scan-by-participant or scan-by-community without a current-membership predicate.
- [ ] File any findings under the #1705 umbrella as Phase 4-other instances.

**Validates:** Confidence that the unread-count leak isn't a representative of a wider class.

## Audit Notes
2026-05-04 — audit pass against `docs/client/{architecture,caching,i18n,accessibility,testing}.md` and `docs/server/{architecture,observability}.md`. Changes folded into the plan above; rationale below for traceability.

### Q1: SQL predicate column names — corrected
Original draft used `cu.deleted_at IS NULL`. The actual codebase shape (verified during audit) is the proto-`DeletedMetadata` field that materializes as columns `deleted_deleted_by_user_id` and `deleted_deleted_at_unix_sec`. Compare:
- `server/storage/chat_conversation.go:104–105` — the existing `ListByParticipant` already filters on `COALESCE(deleted_deleted_at_unix_sec, 0) = 0` for `chat_conversation`'s own soft-delete column. The new method should use the same idiom for `community_user`.
- `server/storage/community.go:187` — restore-community update writes both `deleted_deleted_by_user_id` and `deleted_deleted_at_unix_sec`, confirming the column shape.

Updated the Phase 1 task list with the corrected SQL.

### Q2: Architectural alignment — Option A2 reframed as "reuse the existing library helper"
`docs/server/architecture.md` §2 ("Shared Functionality in Libraries") prefers reusing existing helpers over duplicating logic. `storage.ProtoSQLStorage.GetCommunitiesWithMembership` (`server/storage/community.go:87`) already encapsulates the join from a list of community IDs to a user's memberships, and is used by `addUserAsParticipant` (`server/services/chat/conversations.go:102–160`). A2 (Go-side filter using this helper) is therefore not just "a workaround for a SQL gap" — it's the architecturally preferred approach if we want minimum new code surface.

That said, the plan still recommends A1 (SQL JOIN) for the unread-count specifically because:
- It avoids a second round trip on a hot read path (badge polling).
- It keeps the soft-delete predicate co-located with `chat_conversation`'s own soft-delete predicate at `chat_conversation.go:104–105`, which is the consistent neighborhood.
- The `EXISTS` subquery is a small, self-contained change; the perf benefit over A2 outweighs the duplication concern.

Updated A2's framing to point at the existing helper. Recommendation unchanged.

### Q3: 30-day rejoin window
`proto/ripls/models/community.proto:73-94` documents the 30-day rejoin window: a `CommunityUser` row is soft-deleted on leave, restored on rejoin within the window. The active-membership predicate `COALESCE(deleted_deleted_at_unix_sec, 0) = 0` correctly excludes soft-deleted rows — matching the user-facing definition of "active member." Called this out explicitly in the Phase 1 task list so reviewers don't have to re-derive it.

### Q4: Existing `IsMemberOfCommunity` looks like it doesn't filter on soft-delete
`server/auth/community.go:16-39` calls `s.QueryByFields(ctx, {community_id, user_id}, &models.CommunityUser{})` with no explicit filter on `deleted`. Two possibilities: (a) `QueryByFields` excludes soft-deleted rows by default, or (b) `IsMemberOfCommunity` is itself buggy and treats soft-deleted memberships as active. Either way, the unread-count fix should explicitly enforce the predicate (defensive, fewer assumptions about upstream behavior). Flagged this in the Phase 1 task list as a separate-ticket candidate if (b) turns out to be the case.

### Q5: Service independence
Architecture.md §1 forbids services calling other services. Phase 1 changes are entirely within the storage layer (new method on `ProtoSQLStorage`) and the chat service handler (`unread_counts.go`). No cross-service injection introduced. ✓

### Q6: File organization
Architecture.md §3 prefers files <1000 lines. `chat_conversation.go` is small; adding a new sibling method there (next to `ListByParticipant`) is the right neighborhood. ✓

### Q7: Testing layers
Architecture.md §4 prefers unit tests over integration tests. Phase 1 task list includes both: a storage-layer unit test (mock or thin DB harness) and a service-layer integration test using `pgvector/pgvector:pg16` testcontainer per `CLAUDE.md` §Testing. The integration test is the one that proves the cross-table JOIN works end-to-end. ✓

### Q8: Observability
`docs/server/observability.md` §"Standard Field Names" is explicit about the meaning of `request_id` (the HTTP correlation ID) and `target_request_id` (the entity ID for `models.Request`). The plan does not introduce any field-name collisions. Updated the Phase 1 logging task to:
- Use `DEBUG` (not INFO) for the optional "how many were filtered out" log, since this is internal aggregation state, not an unexpected condition.
- Use the descriptive field name `filtered_conversation_count` (snake_case, follows the standard convention).

### Q9: Connect error propagation
`CLAUDE.md` and `observability.md` mandate `connecterr.Internal(ctx, op, err, kv...)` for `CodeInternal` errors. Raw `connect.NewError(connect.CodeInternal, err)` is gated by `scripts/check_codeinternal.js`. Added an explicit task to use the wrapper if the new query path surfaces errors back to the client.

### Q10: Phase 2 verification — client work happens on the existing post-#1708 build
Phase 2 is a verification step, not an implementation step. The "tasks" are: deploy Phase 1, repro on dev, capture logs, decide whether to file a sub-issue. No client architecture concerns (no new code paths). ✓

### Q11: i18n / a11y / caching — N/A for this PR
- No new user-visible strings.
- No new client widgets or interactions.
- No new client-side caching (the unread-count repository already caches via the existing `CacheService` namespace; the fix is upstream of caching).

### Q12: Scope creep guardrails
The plan explicitly defers Half A (chat-load failure) to Phase 2 and Phase 3 (audit of adjacent aggregates) to the umbrella's Phase 5. This matches the project guidance to keep PRs surgical (per `CLAUDE.md`). No changes needed.

## Related Issues
- #1705 — Umbrella for the multi-community client scoping pattern. This issue is the Phase 4-chat slice.
- #1707 — Phase 1 of #1705 (gear borrow). Sibling client-side fix.
- #1708 — Phase 4-gear-page-load. May have already addressed Half A; Phase 2 verifies.
- #1710 — Phase 4-members. Same membership-intersection theme at a different surface.
- #1694 — Invite picker filter. Sibling of #1710 / #1713.
- #1713 — ItemShareSheet typed-error follow-up to #1694.
- #1684, #1685, #1703 — Closed by Phase 1 (#1707).
- #1667, #1676 — Closed by Phase 4-members (#1710).
