# Issue #1675: Chat failure

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1675
**Filed:** 2026-05-03 | **Last activity:** 2026-05-03
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

User `user12@example.org` opened the in-app chat for a piece of gear that
Bryan shared and saw the toast **"Failed to load chat: you don't have
permission to perform this action"**. The toast text is the client's
localized rendering of a Connect `CodePermissionDenied` returned by the
server. The reporter notes the gear is "shared with 23 other people",
which strongly implies a multi-community share.

The likely root cause: gear conversations have a single `community_id`
on the `ChatConversation` record (the community where the gear was
*first* shared), but a gear can be shared with many communities via
`CommunityGear`. The server's permission gate for the chat looks at
**only** that one community_id and rejects every member of any *other*
community the gear is shared with.

The user reported this on the `chat_perm` branch, which has not yet
diverged from `main` — so no fix is in flight.

## Context from Comments

The issue has no comments — only the bug-report description, a
screenshot from in-app feedback, and device context. That's a signal to
flag the missing reproduction details below in **Open Questions** rather
than treat the report as fully specified.

What the description tells us:
- Reporter: `user12@example.org`
- Item shared by Bryan, "shared with 23 other people" (no breakdown of
  whether that means 23 communities or 23 members of one community)
- App 0.19.0 build 6338, iOS 26.3.1, **PROD** environment
- The error toast is the localized text for `Code.permissionDenied`
  (`app/lib/core/errors/rpc_error_handler.dart:275`,
  `app/lib/l10n/app_en.arb:950`).

## Codebase findings

The chat screen calls `chatRepository.getConversation` and
`getConversationContext` in parallel during init
(`app/lib/presentation/widgets/content/inline_conversation_view.dart:209-217`).
Either failing surfaces the same toast at line 254. Both server endpoints
gate on the conversation's stored `community_id`:

- `server/services/chat/conversations.go:288` — `GetConversation` checks
  participant list, then for non-community/non-experience/non-request
  topics with a gear topic falls through to `addUserAsParticipant`
  (line 341–348).
- `server/services/chat/conversations.go:101` — `addUserAsParticipant`
  has explicit multi-community handling for **Experience** (queries
  `CommunityExperience`) and **Request** (queries `CommunityRequest`),
  but **no** corresponding branch for **Gear** (no `CommunityGear`
  query). Gear topics fall through to the "primary community" check at
  line 175–188, which uses only `conversation.CommunityId`.
- `server/services/chat/helpers.go:16` — `requireConversationAccess`
  (used by `GetConversationContext`) checks participant list, then
  exclusively `auth.RequireMemberOfActiveCommunity(ctx, …,
  conversation.CommunityId, userID)`. No multi-community handling for
  any topic type.

Conversations are gear-scoped (one per gear, not per share):
`server/services/community/gear_sharing.go:255-289` shows that on the
**first** share, `gear.ConversationId` is set and reused across every
subsequent `ShareGear` call. The conversation's `CommunityId` is
permanently the first community of the share (set inside
`chatlib.CreateOrGetConversation`).

The existing test coverage matches this gap: `conversations_gear_test.go`
covers (1) participant access and (2) member-of-`conversation.CommunityId`
access, but not (3) member-of-another-community-the-gear-is-shared-with.

## Potential Solutions

### Option A: Teach the chat permission gate about multi-community gear
- **Approach:** Add a `case topic.GetGearId() != ""` branch to
  `addUserAsParticipant` (mirroring the Experience/Request branch) that
  queries `CommunityGear` for every community the gear is shared with
  and grants access if the caller is an active member of any of them.
  Apply the same change to `requireConversationAccess` so
  `GetConversationContext` agrees. Add a regression test that puts the
  caller in a *secondary* community and asserts access succeeds.
- **Pros:** Addresses the root cause. Symmetric with how Experience and
  Request multi-community sharing already works. Both server endpoints
  the client calls will agree.
- **Cons:** One extra `CommunityGear` query per chat-load when the
  caller is not yet a participant. The query is bounded by share fan-out
  (typical: 1–10) and is one-shot per-user (the user becomes a
  participant after first access).
- **Effort:** Small.

### Option B: Pivot the chat conversation off `community_id` entirely
- **Approach:** Remove `ChatConversation.community_id` (or treat it as
  informational only) and resolve permission purely from topic+identity:
  for gear topics, query `CommunityGear` for the gear and check
  membership; for experience/request, the existing helpers; for
  community-wide chat, the topic's community id directly.
- **Pros:** Removes a class of bugs where `community_id` drifts from
  reality (community soft-delete, gear unshared from the seed community
  but still shared elsewhere, etc.). Simpler mental model.
- **Cons:** Larger surface area — `community_id` is read by lots of code
  (notifications, system-message routing, `ConversationItem` responses,
  validateAndCleanParticipants, etc.). Wire-format implications. Out of
  scope for a single user-feedback fix.
- **Effort:** Large.

### Option C: Eagerly add every CommunityGear member as a participant on share
- **Approach:** When `ShareGear` runs and the conversation already
  exists, append every member of the new community to
  `conversation.ParticipantIds`. Permission checks pass because
  participant lookup short-circuits.
- **Pros:** No permission-gate change needed.
- **Cons:** `ParticipantIds` becomes O(total members across all sharing
  communities), which is the bloat that participants-as-dynamic-list
  was designed to avoid (see `validateAndCleanParticipants` comment on
  experience/request/community conversations skipping cleanup
  precisely because they span communities). New community members
  added *after* the share don't get appended unless we also hook
  community-join. Strictly more code than Option A and more state to
  maintain.
- **Effort:** Medium.

## Recommendation

**Option A.** It is the smallest change that fixes the user-visible bug,
keeps gear-conversation behavior consistent with the already-shipped
multi-community handling for experiences and requests, and is testable
with a clear regression case. Option B is the right long-term shape but
should be its own initiative, not a hotfix. Option C re-introduces the
participant-bloat the system already moved away from.

I'm assuming the report does describe a multi-community gear (Bryan's
sharing pattern across multiple communities is consistent with this).
If reproduction shows the gear is shared with **only one** community, a
different root cause is in play — see open questions below.

## Open Questions

- **Was the gear actually shared with multiple communities, or with one
  community that has 23 members?** — Recommendation: pull the gear
  record + `CommunityGear` rows for the affected gear from PROD before
  writing the fix. The screenshot URL identifies the feedback
  submission; cross-referencing Bryan's shared gear at the report
  timestamp should narrow it to one gear. If only one community is
  involved, look instead at (a) Ali's `CommunityUser` row for that
  community (was she removed?), (b) the community's `Deleted` timestamp,
  and (c) `validateAndCleanParticipants` having stripped her from
  `participant_ids` after she was previously added.
- **Should `requireConversationAccess` also gain a Request branch?** —
  Recommendation: yes, in the same change. It currently has no
  multi-community handling for *any* topic, so a Request conversation
  reached via `GetConversationContext` could exhibit the same bug,
  even though `GetConversation` would succeed (asymmetry between the
  two endpoints the client calls in parallel is itself a smell).
  Audit needed: does any existing flow cause this asymmetric failure
  today, or is `addUserAsParticipant` always called first?
- **Is there a server log for this exact request we can inspect?** —
  Recommendation: search Cloud Run logs for the request_id of Ali's
  `GetConversation` / `GetConversationContext` calls around the issue
  timestamp (2026-05-03 15:21 UTC). Both handlers log
  `WarnContext` / `ErrorContext` with `conversation_access denied` or
  `failed to check community membership`. The presence/absence of one
  vs the other tells us which path failed.

## Implementation Plan

### Phase 1: Reproduce + add failing test
**Goal:** Prove the bug exists in code, not just in production logs.
**Tasks:**
- [ ] Pull the affected gear's `CommunityGear` rows from PROD to confirm
      the shape (multi-community vs. single-community).
- [ ] Add a regression test in
      `server/services/chat/conversations_gear_test.go`:
      `TestGetConversation_GearMultiCommunityMemberAccess` — gear
      shared with communities A and B, conversation has
      `community_id=A`, caller is a member of B only, expect success
      and that the caller is appended to `participant_ids`.
- [ ] Add a parallel test for `GetConversationContext` that exercises
      `requireConversationAccess` with the same fixture.
- [ ] Run the tests; confirm they fail with `CodePermissionDenied`.
**Validates:** Root cause is the multi-community gap and not something
adjacent (soft-deleted community, removed CommunityUser, etc.).

### Phase 2: Server fix
**Goal:** Make both endpoints agree on access for multi-community gear.
**Tasks:**
- [ ] In `server/services/chat/conversations.go` `addUserAsParticipant`,
      add a `topic.GetGearId() != ""` branch that queries
      `CommunityGear` by `gear_id`, collects the community ids, and
      uses `store.GetCommunitiesWithMembership` exactly like the
      Experience/Request branches.
- [ ] In `server/services/chat/helpers.go` `requireConversationAccess`,
      replace the single-community check with topic-aware membership
      resolution: gear → `CommunityGear` fan-out; experience →
      `CommunityExperience`; request → `CommunityRequest`; community
      topic → membership in `topic.GetCommunityId()`. Reuse a small
      helper (e.g. `collectSharedCommunityIDs`) so both call sites
      share logic.
- [ ] Make sure the existing
      `TestGetConversation_GearNonCommunityMemberDenied` and
      `TestGetConversationContext_NonCommunityMemberDenied` tests still
      pass — outsiders (not in *any* sharing community) must still be
      denied.
- [ ] `npm run build` to clear lint+compile.
**Validates:** Phase 1's failing tests pass; existing denial tests
still pass.

### Phase 3: Backfill confidence + ship
**Goal:** Confirm the fix covers the production scenario and nothing
else regressed.
**Tasks:**
- [ ] Run the full chat package tests: `go test ./server/services/chat/...`.
- [ ] Spot-check `validateAndCleanParticipants` doesn't strip the newly
      added participant on the next access — for gear, it currently
      filters by `conversation.CommunityId` membership only, which
      will *re-strip* the secondary-community user we just added. Add
      gear to the early-return list at line 49–53, or extend the
      validity set to all sharing communities. Add a test asserting
      this doesn't regress on the second `GetConversation` call.
- [ ] Open PR; reference issue #1675; include the Phase 1 test in the
      diff so reviewers can read the failing-then-passing trail.
**Validates:** No second-access regression; no participant churn after
the fix; user-visible behavior matches expectation.

## Related Issues

- #1637 — community-wide chat (introduced the topic-aware handling for
  community conversations). Same code paths.
- #1660 / #1646 — `common_chat` work that landed the multi-community
  experience/request handling this fix mirrors.
- #1670 — former-member placeholder. Adjacent in spirit (chat resilience
  to membership state changes) but unrelated to permission gating.

---

## Audit Findings (2026-05-03)

The plan above was reviewed against `docs/server/architecture.md` and
`docs/server/observability.md`. No structural rewrites were needed; the
fixes below tighten conventions before implementation begins.

### Helper placement

`server/services/chat/` already has an `authorization.go` (defines
`requireParticipant`). The proposed `collectSharedCommunityIDs` helper
should live there — not in `helpers.go` or inline in
`conversations.go`. `conversations.go` is already 974 lines; the
`< 1000` guidance in `docs/server/architecture.md` makes any further
growth in that file a smell. Likewise, move `requireConversationAccess`
(currently in the 48-line `helpers.go`) into `authorization.go` so all
conversation-permission gating lives in one file.

**Plan delta (Phase 2):**
- [ ] Create `collectSharedCommunityIDs(ctx, store, topic) ([]string, error)`
      in `server/services/chat/authorization.go`. Switch on the topic
      oneof: gear → `CommunityGear`, experience →
      `CommunityExperience`, request → `CommunityRequest`,
      community-topic → `[]string{topic.GetCommunityId()}`.
- [ ] Move `requireConversationAccess` into `authorization.go` and
      rewrite it to consult `collectSharedCommunityIDs` plus
      `store.GetCommunitiesWithMembership` (matches the batched pattern
      `addUserAsParticipant` already uses).
- [ ] Refactor the experience/request branches in
      `addUserAsParticipant` to call the new helper, so the gear branch
      isn't a one-off and the three topic types share one path.
- [ ] After the move, delete `helpers.go` if empty; otherwise leave a
      `// Package-level conversation helpers …` doc-only file.

### Logging conventions

`docs/server/observability.md` is explicit: **user errors (invalid
input, not found, permission denied) are INFO or WARN, not ERROR.
ERROR is reserved for system failures.**

The current `requireConversationAccess` correctly emits `WarnContext`
on access denial, but the existing `addUserAsParticipant` calls
`logger.Error(...)` for downstream storage failures, which is fine —
those *are* system failures. The new code paths must:

- Log permission-denied returns at **WARN** (or rely on the caller's
  WARN, as `GetConversationContext` already does at
  `context.go:39`).
- Log storage failures (e.g. `QueryByField` on `CommunityGear`) at
  ERROR with `error` field, then return via `connecterr.Internal`.
- Use `logging.LoggerWithContext(ctx)` and the standard field set:
  `operation`, `conversation_id`, `user_id`, `gear_id`,
  `community_id`. Do **not** invent ad-hoc field names.

**Plan delta (Phase 2):**
- [ ] In every new error-return site, choose between
      `connect.NewError(CodePermissionDenied, …)` (no internal log
      needed; let the caller WARN) and
      `connecterr.Internal(ctx, "AddUserAsParticipantForGear", err,
      "gear_id", …)` (which logs server-side and returns the fixed
      `"internal server error"` to the wire). Bare
      `connect.NewError(CodeInternal, err)` is gated by
      `scripts/check_codeinternal.js` and will fail CI.

### Test layer

Unit tests are the right level — `docs/server/architecture.md` §4
calls them "always preferable to integration tests" and the existing
`conversations_gear_test.go` is already unit-style with table-friendly
shape. Two refinements:

- The new tests should be table-driven (one `t.Run` per scenario) once
  more than two cases land. Phase 1 adds two new cases; if Phase 3's
  re-strip regression test is added, fold all three into a table.
- Reuse `setupTestChatService(t)`, `createTestUsers`,
  `createTestCommunity`, `addUserToCommunity` rather than rebuilding
  fixtures. The pattern is already in `common_test.go`.

**Plan delta (Phase 1 + 3):**
- [ ] When the second new test lands, refactor into a single table
      `TestGetConversation_GearMultiCommunity` with cases:
      `member_of_primary`, `member_of_secondary`,
      `member_of_no_sharing_community_denied`,
      `secondary_member_persists_after_revisit` (the Phase 3 case).

### Service-independence check

The fix only touches `server/services/chat/`. It queries
`CommunityGear` directly via `s.storage.QueryByField`, which is
already how `IsGearSharedWithCommunity` works in
`server/services/community/gear_sharing.go:446`. No
service-to-service dependency is introduced — clean per
architecture §1.

### What was *not* changed in the plan

- The proto layer is untouched; no API surface changes are needed for
  this fix (Option A's contract).
- No client-side changes — the existing
  `inline_conversation_view.dart:254` toast text comes from
  `Code.permissionDenied` localization and remains correct on the
  *legitimate* denial path (true outsiders).
- No caching, i18n, or accessibility work to audit (server-only fix).
