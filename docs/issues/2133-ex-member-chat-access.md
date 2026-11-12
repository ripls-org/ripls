# Issue #2133: Security: ex-community-members retain chat access via stale ChatConversation.ParticipantIds

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2133
**Filed:** 2026-05-23 | **Last activity:** 2026-05-23
**Labels:** bug, security, server | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a user leaves (or is removed from) every community a chat conversation is shared with, they keep read access to that conversation — including future messages — because two invariants are missing:

1. `requireConversationAccess` (`server/services/chat/helpers.go:18`) already has the right re-check logic on the fan-out path (`collectSharedCommunityIDs` + `auth.RequireMemberOfActiveCommunity` at lines 32-53), but the `ParticipantIds` short-circuit at lines 25-30 fires *before* the re-check. Once a user lands in the participant list (because they once were a community member), the short-circuit bypasses the re-check forever — the authorization regresses to "are you in the (stale) participant array."

2. `runLeaverCascade` (`server/services/community/lifecycle.go:767-807`) covers `community_gear`, `community_request`, `transfer`, `experience_rsvp`, `community_notification_preferences`, and `community_invitation_link`, but does **not** strip the leaver from `chat_conversation.participant_ids`. So the participant entry persists across leave events. `ListConversations` continues to surface the conversation in the ex-member's inbox because `ListByParticipant` is purely participant-based (`server/storage/chat_conversation.go:54-73`).

The existing defensive `validateAndCleanParticipants` (`server/services/chat/conversations.go:30-96`) explicitly skips gear / experience / request / community topics — those are exactly the topic kinds where the bug manifests, so the defensive cleanup never runs on the conversations that need it.

Net effect: an ex-member who knows a conversation ID (or whose client already cached it) can call `GetConversationHistory`, `MarkMessagesRead`, `StreamMessages`, `GetConversationContext`, `GenConversationSummary`, and `ListConversations` to keep reading the conversation indefinitely with whatever valid auth token they have (1 h access, 90 d refresh). Combined with #2132 (`SendMessage` missing authorization), they can also post.

## Context from Comments

Maintainer Brian Brown left one substantive comment that materially shapes the fix:

- **A template for the read-path fix already exists in the codebase.** `chat.GetConversationForRequest` (`server/services/chat/conversations.go:627-697`) deliberately bypasses the participant short-circuit and uses `auth.RequireMemberOfActiveCommunity` directly. Its inline comment ("not just a participant") confirms someone already closed this same gap for the request-conversation reader. Generalizing that pattern to `requireConversationAccess` is the smallest correct change.
- **Full list of RPCs that inherit the bug** (every reader that calls `requireConversationAccess`): `GetConversationHistory`, `MarkMessagesRead`, `StreamMessages`, `GetConversationContext`, `GenConversationSummary`. Plus the inbox-scoped reader `ListConversations` via `ListByParticipant`. Plus the lazy-add reader `GetConversationForExperience` (`conversations.go:728-734`) which short-circuits on existing participant membership before the active-membership re-check.
- **Disclosure surface widened by this bug.** `chat.GetConversationContext` for `Gear` topics returns `INTEREST_EXPRESSED` and `RECIPIENT_SELECTED` transfers plus the borrower/recipient `api.User` proto — intentional for active members, but with this bug those proto User identities leak to ex-members indefinitely. The fix to `requireConversationAccess` is sufficient; `getGearTransferContext` itself does not need to change.
- **RPCs that must keep their current behavior:** `chat.GetConversationForRequest` (already correct), `chat.GetConversationForCommunity` (uses `auth.IsMemberOfCommunity` directly), and `chat.GetConversationForTransfer` — the last is the intentional escape hatch: a transfer must be wrappable up after either party leaves, so it should keep its participant-style check.
- **Express the carve-out cleanly:** keep the participant short-circuit in `requireConversationAccess` only for `ConversationTopic_TransferId`; require active community membership for every other topic kind.

The author also requested `@claude plan` (rather than direct implementation) on the issue, so the right output here is a plan PR.

## Potential Solutions

### Option A: Read-path fix only (tighten `requireConversationAccess`; do not cascade)

- **Approach:** Remove the `ParticipantIds`-based short-circuit in `requireConversationAccess` for every topic kind except `TransferId`. Always call `collectSharedCommunityIDs` + `auth.RequireMemberOfActiveCommunity`. Apply the same change to `GetConversationForExperience`'s participant short-circuit. Leave `ParticipantIds` stale on disk; let `ListByParticipant` continue to surface the conversation in the ex-member's inbox — but every actual read returns `PermissionDenied`.
- **Pros:** Smallest, most surgical change. One file (`helpers.go`) plus one small block in `conversations.go`. No new cascade plumbing, no risk of misclassifying when to strip participants. Closes the data-disclosure side of the bug completely (every actual read is denied).
- **Cons:** `ListConversations` still shows the conversation in the ex-member's inbox preview (`LastMessageText`, `LastMessageSender`, `UnreadCount`) — the row is rendered from `ChatConversation` proto fields that are already in the row that `ListByParticipant` returns, no `requireConversationAccess` check. This is a residual information leak: preview of the latest non-system message text, sender display name, and unread counter for an inbox the ex-member should no longer see. `ParticipantIds` is also semantically wrong — every consumer that walks the array now sees a "ghost" participant.
- **Effort:** Small.

### Option B: Read-path fix + ListConversations server-side filter (no cascade)

- **Approach:** Option A plus: in `ListConversations`, after fetching the raw conversations from `ListByParticipant`, drop any conversation the caller would be denied by `requireConversationAccess`. The cleanest way is to reuse the same membership predicate per conversation: for each conversation, call `collectSharedCommunityIDs` + a batched `GetCommunitiesWithMembership` (one DB round-trip across all candidate communities) and only keep conversations where the caller is still in an active shared community OR the topic is a transfer where they're a participant.
- **Pros:** Closes both the read and the inbox-listing leak without changing on-disk participant lists. No new cascade plumbing, no behavior change to leave/rejoin invariants. Inbox is correct from the caller's perspective.
- **Cons:** Adds per-`ListConversations`-call work proportional to N (cached communities, but still one batched query per call). Leaves `ParticipantIds` stale on disk forever — which is honest about the multi-community semantics (the comment at `conversations.go:46-54` explicitly warns single-community pruning would evict legitimate cross-community participants) but means every other consumer that walks the array still sees the ghost. No retroactive cleanup of historical bad rows.
- **Effort:** Small-medium.

### Option C: Read-path fix + leaver cascade (recommended)

- **Approach:** Option A, plus a new cascade row in `runLeaverCascade` that, for the leaver's community, recomputes whether the leaver is still in any *other* community each affected chat conversation is shared with — and, if not, drops them from `ChatConversation.ParticipantIds`. Concretely: enumerate `chat_conversation` rows that (a) reference a topic shared with this community via `CommunityGear` / `CommunityExperience` / `CommunityRequest` and (b) currently contain the leaver in `participant_ids`. For each, re-fan-out via `collectSharedCommunityIDs`-equivalent storage helpers; if the leaver no longer has an active CommunityUser row in any remaining shared community, splice them out of `ParticipantIds` and `Update` the conversation. Community-wide topics (`topic_community_id`) and transfer topics are handled separately: community-wide simply drops the leaver if their only sharing path is the community being left; transfer topics are intentionally exempt (mirrors `GetConversationForTransfer`).
- **Pros:** Restores the data integrity invariant — `ParticipantIds` no longer lies. `ListByParticipant` stops surfacing ghost conversations to ex-members at the SQL layer (the inbox query becomes correct again with zero per-call cost). Defense-in-depth: even if a future RPC accidentally trusts `ParticipantIds`, the array is honest. Composable with the read-path fix — both must land for full coverage of the existing bad rows AND the future inbox-correctness invariant, but each is independently valuable.
- **Cons:** Largest change of the three. New storage helper, careful query design to avoid N+1 (one pass per affected join-table community-scoped query, then one batched recomputation across the set). Risk of accidentally pruning legitimate cross-community participants if the recomputation is wrong; must mirror `collectSharedCommunityIDs` precisely. Does not retroactively clean up existing stale rows for users who already left — a follow-on backfill job (or an on-read lazy cleanup) handles that, can be a separate issue.
- **Effort:** Medium.

## Recommendation

**Option C: Read-path fix + leaver cascade.**

Option A alone is the bare minimum to close the data-disclosure CVE, and it should land first because it's the actual security boundary fix. But on its own it leaves a stale-`ParticipantIds` data-quality bug that will keep biting (inbox previews leak, `ListConversations` shows ghost rows, future code that trusts `ParticipantIds` re-introduces the same vulnerability). The maintainer comment explicitly endorses both changes as the recommended fix in the issue body ("Two complementary changes"). The cascade extension matches the existing pattern in `cascade_leaver.go` — six other leaver-scoped helpers already live there with idempotent, single-community semantics — so the change is mechanical to slot in.

Option B is a viable interim if Option C turns out to require non-trivial query design, but it adds permanent per-call overhead and leaves the data-quality issue unsolved. Defer it.

Concrete fix for `requireConversationAccess` per the maintainer's guidance: keep the participant short-circuit **only** for `ConversationTopic_TransferId` (the existing intentional escape hatch). For every other topic kind, fall through to `collectSharedCommunityIDs` + `auth.RequireMemberOfActiveCommunity` (batched via `GetCommunitiesWithMembership` to stay within the SQL-efficiency rules).

## Open Questions

- **Should we batch the per-community membership probe in `requireConversationAccess`?** The current code loops `auth.RequireMemberOfActiveCommunity` one community at a time (`helpers.go:46-50`). For gear/experience/request topics, the candidate community set can be more than one. **Recommendation:** Yes — switch to a single `GetCommunitiesWithMembership` batched call (the same pattern `addUserAsParticipant` already uses at `conversations.go:130-148`). One DB round-trip regardless of fan-out, matches the SQL-efficiency rules in CLAUDE.md.
- **Should `GetConversationForExperience`'s short-circuit at `conversations.go:728-734` be removed too?** That RPC walks `ParticipantIds` first and only falls through to `addUserAsParticipant` (which does an active-membership check) on miss. An ex-member already in the list bypasses the active-membership re-check. **Recommendation:** Yes. Replace the short-circuit with the same logic as the new `requireConversationAccess`: always require active membership in any shared community. The participant list is a denormalization, not an authoritative grant.
- **What about historical stale rows?** Users who already left communities have stale `ParticipantIds` entries today. The Option-C cascade fixes the invariant going forward but won't retroactively clean up the existing bad rows. **Recommendation:** Out of scope for this issue. Once the read-path fix lands, the bad rows are no longer a security issue (every read is gated on current membership). File a separate cleanup ticket if/when the inbox-preview leak becomes a UX issue. Mention this explicitly in the PR description.
- **Should we add an explicit on-read lazy cleanup (extend `validateAndCleanParticipants` to handle gear/experience/request topics)?** It would retroactively clean up stale rows as the conversations get fetched. **Recommendation:** No — the existing comment at `conversations.go:45-54` correctly identifies that single-community membership validation would evict legitimate cross-community participants. The right cleanup is the cascade-driven one in Option C (which checks every shared community, not just `conversation.CommunityId`). Lazy cleanup using the multi-community check is fine in theory but adds write traffic to read paths and obscures the fix; defer.
- **Does removing the participant short-circuit break `GetConversation`'s preview-access semantic?** `GetConversation`'s test case "community member (non-participant) gets preview access" at `authorization_test.go:121` expects a non-participant community member to still get the conversation. Inspecting the code, `GetConversation` does NOT go through `requireConversationAccess`; it has its own ad-hoc auth at `conversations.go:294-332` (community-member check via `IsMemberOfCommunity` or `addUserAsParticipant`). So `GetConversation` is unaffected. **Recommendation:** Leave `GetConversation` alone. The fix scope is `requireConversationAccess`, `GetConversationForExperience`, and the cascade — `GetConversation` already does the right thing for its preview-access semantic.
- **What event/audit signal should the cascade emit?** The other six cascade helpers are silent (the `MEMBER_LEFT` event is the umbrella signal per the `cascade_leaver.go` header comment). **Recommendation:** Match the pattern — silent storage update, no new event type. Logging at DEBUG with `conversation_id` and `participant_count_before/after` for ops visibility.

## Implementation Plan

### Phase 1: Tighten `requireConversationAccess` (the actual security boundary fix)

**Goal:** Close the read/stream disclosure CVE so an ex-member cannot read or stream messages even with a stale `ParticipantIds` entry.

**Tasks (TDD order — write the failing test first):**
- [ ] Extend `TestRequireConversationAccess` in `server/services/chat/authorization_test.go` with new subtests **before** touching `helpers.go`:
  - `ex-member with stale participant entry is denied for gear topic`: user joins community, becomes participant in a gear conversation, leaves community (delete the CommunityUser row), call `requireConversationAccess` — expect `PermissionDenied`. This test fails before the fix.
  - Same shape for `experience` and `request` topics.
  - `participant in transfer conversation is still allowed after leaving community`: codifies the intentional escape hatch. Must pass both before and after the fix.
  - `ex-member of one community remains allowed when still member of another shared community`: gear shared with C1 and C2; user member of both; participant; leaves C1; still member of C2; expect access granted.
- [ ] In `server/services/chat/helpers.go`, change `requireConversationAccess` so the `ParticipantIds` short-circuit (lines 25-30) fires only when the topic is a transfer. Concretely: detect `ConversationTopic_TransferId` first; for that kind, keep the existing participant loop and reject non-participants with `PermissionDenied`. For all other kinds (gear / experience / request / community / no-topic-with-community-id), skip the participant check and always go through `collectSharedCommunityIDs` + a batched membership probe.
- [ ] Switch the per-community membership loop at `helpers.go:46-50` to a single `s.storage.GetCommunitiesWithMembership(ctx, sharedCommunityIDs, userID)` call. Iterate the returned map: grant access on the first pair where `Community.Deleted` is nil and `Membership != nil`. Mirror the pattern at `conversations.go:130-148`.
- [ ] **Error wrapping:** any storage error returned from `GetCommunitiesWithMembership` must go through `connecterr.Internal(ctx, "requireConversationAccess", err, "conversation_id", conversationID, "user_id", userID)`. Bare `connect.NewError(connect.CodeInternal, err)` is gated by `scripts/check_codeinternal.js` (CLAUDE.md → Error Handling → "Connect CodeInternal errors").
- [ ] **Structured logging on deny path.** Add INFO-level logging on the new deny branch (every other branch already logs at higher layers). Fields per `docs/server/observability.md` and CLAUDE.md → Structured Logging: `operation="requireConversationAccess"`, `user_id`, `conversation_id`, `topic_kind` ("gear" | "experience" | "request" | "community" | "transfer" | "none"), `shared_community_count`. Use INFO (deny is not an error). The Connect error stays `PermissionDenied`.
- [ ] In `server/services/chat/conversations.go`, in `GetConversationForExperience` at lines 728-734, remove the participant-list short-circuit. Always call `addUserAsParticipant` (which already enforces active membership in any shared community). Document why in a one-line comment: stale ParticipantIds entries from prior memberships must not grant access.
- [ ] Add a `GetConversationForExperience`-specific test in `server/services/chat/conversations_test.go`:
  - **Denial path:** caller is in `ParticipantIds` but no longer in any shared community → expect `PermissionDenied`.
  - **Lazy-add side effect path:** caller is NOT in `ParticipantIds` but IS in an active shared community → expect success AND assert `addUserAsParticipant` re-adds them to `conversation.ParticipantIds`. This guards the cascade vs. lazy-add cycle: after a leave the cascade strips the participant; on the next access by a re-joined user, the lazy-add must put them back.
- [ ] **Query-count regression test (SQL Efficiency rule 8).** Add a subtest using `storage.AssertMaxQueries(t, ctx, 3, fn)` around `requireConversationAccess` for a gear-topic conversation with two shared communities: budget = 1 (`GetByID(conversation)`) + 1 (`collectSharedCommunityIDs` `QueryByField(community_gear)`) + 1 (batched `GetCommunitiesWithMembership`) = 3. Prevents a future refactor from regressing to the per-community loop.

**Validates:** Every read/stream/context RPC is denied to ex-members. The maintainer comment lists six RPCs that inherit the fix; they all close at once because they share `requireConversationAccess`. Query budget is bounded.

### Phase 2: Extend `runLeaverCascade` to strip stale ParticipantIds

**Goal:** Restore the on-disk invariant that `ChatConversation.ParticipantIds` reflects current eligibility, so `ListByParticipant` stops surfacing the conversation in ex-members' inboxes and downstream consumers of the array are honest.

**Tasks:**
- [ ] In `server/storage/cascade_leaver.go`, add `CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, storage, communityID, leaverUserID string) error`. Skeleton (the per-topic-kind work is batched — no per-conversation re-fan-out; CLAUDE.md → SQL Efficiency rules 1 "Never query in a loop" and 5 "Use JOINs for related data"):
  1. **Candidate set per topic kind, fully batched:**
     - **Gear:** `QueryByField("community_id", communityID, &CommunityGear{})` → collect distinct `gear_id`s → ONE `QueryByFieldIn("topic_gear_id", gearIDs, &ChatConversation{})` for the conversations + ONE `QueryByFieldIn("gear_id", gearIDs, &CommunityGear{})` for the *full* community fan-out per gear (so we know every OTHER community each gear is shared with). Group the second result by gear_id into a `map[gearID][]communityID`.
     - **Experience:** same shape via `CommunityExperience` → `topic_experience_id`.
     - **Request:** same shape via `CommunityRequest` → `topic_request_id`.
     - **Community-wide:** `QueryByField("topic_community_id", communityID, &ChatConversation{})`. No fan-out — community-wide topics live in a single community.
  2. **Single batched membership probe for the leaver across the union of all remaining shared communities.** Collect every OTHER `communityID` (excluding `communityID`) referenced by any candidate gear/experience/request, deduplicate, then call `GetCommunitiesWithMembership(ctx, otherCommunityIDs, leaverUserID)` ONCE. Build a `map[communityID]bool` of "leaver is an active member here."
  3. **In-memory recomputation per conversation:** for each candidate conversation, if the leaver is NOT in `ParticipantIds`, skip. Otherwise check the membership map against the conversation's per-topic fan-out (already loaded in step 1). If the leaver has no remaining active shared community for this topic, splice them out of `ParticipantIds` and call `storage.Update`. (The participant_ids TEXT[] denormalization column auto-syncs in the same transaction per `chat_conversation_schema.go`.)
  4. **Transfer-topic conversations are NOT touched** — intentional escape hatch (mirrors `GetConversationForTransfer`).
- [ ] **Query-budget assertion.** Add `TestCascadeRemoveLeaverFromChatConversationsInCommunity_QueryBudget` using `storage.AssertMaxQueries`. With G gear + E experience + R request conversations spread across N other communities, the helper must issue O(1) round-trips: 4 candidate-set scans (CommunityGear, CommunityExperience, CommunityRequest, ChatConversation-by-community), 3 conversation fan-outs (one `QueryByFieldIn` per topic kind), 1 batched `GetCommunitiesWithMembership`, plus per-update writes (necessarily one per stripped conversation). Bound the read side at ≤ 8 queries regardless of fixture size.
- [ ] Wire the new helper into the cascades slice in `server/services/community/lifecycle.go:776-798` as `"chat_conversation_participant"` (descriptive name matches what the cascade actually mutates — important when the WARN log line fires). The closure does not use the `*models.DeletedMetadata` parameter; mirror the `transfer` entry at line 786 that ignores it.
- [ ] **Fail-fast carve-out.** Document inline in the cascade closure why this row uses the existing best-effort `WARN-and-continue` loop (CLAUDE.md → Error Handling: "warn and continue" is the documented exception, not the default). Rationale: the helper is idempotent — a retry from any later operation that walks the leaver state (or a manual re-run) re-converges; the leaver's `MEMBER_LEFT` event is the user-visible signal that covers the whole cascade. Match the header comment style at `cascade_leaver.go:11-18`.
- [ ] **Logging:** at DEBUG, fields per `docs/server/observability.md` and CLAUDE.md → Structured Logging standard field names — `operation`, `community_id`, `user_id`, `conversation_id`, `topic_kind`, `participant_count_before`, `participant_count_after`. For request-topic logs, use the field name `target_request_id` (NOT `request_id` — the latter is reserved for the HTTP correlation ID per CLAUDE.md → Structured Logging "Standard field names").
- [ ] Add `TestCascadeRemoveLeaverFromChatConversationsInCommunity` in `server/storage/cascade_leaver_test.go`. Subtests, mirroring the existing structure:
  - empty community is a noop
  - drops leaver from gear conversation where this was the only shared community
  - retains leaver in gear conversation where the gear is also shared with another community the leaver is still in
  - drops leaver from experience and request conversations on the same basis
  - drops leaver from community-wide conversation
  - **leaves transfer-topic conversation untouched, even when transfer is also tied to the same shared community** (codifies the cascade-side escape hatch, not just the read-side)
  - idempotent: re-running has no effect after participants are already stripped
  - leaves non-leaver participants untouched
- [ ] Add `TestRunLeaverCascade_StripsChatParticipants` in `server/services/community/lifecycle_test.go` to assert the cascade row fires end-to-end via the `LeaveCommunity` RPC.
- [ ] **End-to-end integration test.** Per `docs/server/architecture.md` → Testing Strategy ("Integration Tests required for major features"), add (or extend) a test that drives a full `LeaveCommunity` RPC against a live storage container and asserts: (a) the gear conversation is absent from the leaver's `ListConversations` response afterward; (b) `requireConversationAccess` returns `PermissionDenied` for the same conversation; (c) other participants still see the conversation in their `ListConversations`. Place in `server/services/community/lifecycle_integration_test.go` (build-tag `integration`) or extend an existing leave-cascade integration test.
- [ ] **Rejoin symmetry test.** Add a regression test in `server/services/community/lifecycle_test.go`: user joins community, becomes participant in a gear conversation, leaves (cascade strips them), then rejoins via `RejoinCommunity`. Assert that on the next `GetConversationForExperience` (or equivalent gear-conversation read path) the lazy-add side effect re-adds them to `ParticipantIds`. This guards the leave/rejoin invariant: `RejoinCommunity` does not need to re-add chat participants because the next read does.
- [ ] Add a regression test in `server/services/chat/conversations_test.go` for `ListConversations`: user is participant of a gear conversation, leaves the only shared community, then calls `ListConversations` — expect the conversation is absent. This validates the cascade and `ListByParticipant` together.

**Validates:** `ParticipantIds` is honest after a `LeaveCommunity`. The ex-member's inbox stops showing the ghost conversation. Cross-community participants are preserved. Transfer-topic carve-out holds at both layers (read and cascade). Rejoin works without explicit re-add. Existing leaver-cascade tests still pass.

### Phase 3 (follow-on, not in this PR): Historical-row backfill

**Goal:** Clean up the stale `ParticipantIds` entries that exist today (users who already left before Phase 2 shipped).

**Tasks (done as part of Phase 2's PR, not deferred indefinitely — per CLAUDE.md TODO Comments rule, deferred work needs a tracking issue or it rots):**
- [ ] **Before merging Phase 2, file a separate GitHub issue** titled something like "Backfill chat conversation ParticipantIds for historical leavers" with a body that captures: motivation (closes the inbox-preview leak for users who already left before #2133 shipped), proposed approach (one-shot backfill job that iterates soft-deleted `CommunityUser` rows and invokes `CascadeRemoveLeaverFromChatConversationsInCommunity` per `(communityID, leaverUserID)` pair, idempotent so safe to re-run), and the scope ("not security-critical once #2133 Phase 1 lands; quality-of-life only").
- [ ] **In the same PR, add a `TODO(#NNNN):` near the new cascade helper** referencing the freshly filed backfill issue, with body text per CLAUDE.md TODO Comments rule (2-4 lines: what's wrong today, what would let the TODO be removed, what blocked doing it now). Place near the top of `CascadeRemoveLeaverFromChatConversationsInCommunity` in `server/storage/cascade_leaver.go`.

**Validates:** Historical leakage in inbox previews stops. Not security-critical once Phase 1 lands.

## Related Issues

- #2132 — `ChatService.SendMessage` skips participant/community-member authorization. Same threat actor (ex-member with a stale token); complementary write surface. Fix is independent (replace ad-hoc auth in `SendMessage` with a call to `requireConversationAccess`) but lands cleanly on top of this fix — once Phase 1 here is in, #2132's fix automatically inherits the ex-member denial.
