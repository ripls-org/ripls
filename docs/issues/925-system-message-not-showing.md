# Issue #925: [Bug Report] conversation says 1 message

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/925
**Filed:** 2026-03-27 | **Last activity:** 2026-03-27
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** docs/client/chat.md, docs/client/content.md, docs/ai/content4.md

## Problem Statement

When a conversation is first created (e.g., for a giveaway, experience, or request), the server inserts a creation anchor system message ("Experience created", "Giveaway shared", etc.). The portfolio inbox then shows "1 unread" for this conversation, even though no real user has commented. When the user opens the Chat tab, the `InlineConversationView` correctly shows "Be the first to message" (hiding the system-only message). The contradiction is in the inbox — it shouldn't count creation anchor messages as unread comments.

The server already has an `IsCreationAnchorMessage()` helper (`server/chat/system_messages.go:141`) with a comment stating these messages "should not be counted in comment totals or shown as the most recent comment." However, the portfolio inbox assembly functions in `server/services/portfolio/assembly.go` don't use this filter — `computeUnreadCount`, `lastMessagePreview`, and `assembleCommenters` all treat creation anchors the same as user messages.

The `InlineConversationView`'s `hasRealMessages` filter and "Be the first to message" placeholder are **correct behavior** — they should stay.

## Context from Comments

No comments on the issue. The bug report was submitted via in-app feedback with a screenshot showing the discrepancy.

## Potential Solutions

### Option A: Filter creation anchors in portfolio assembly functions (server-side)

- **Approach:** Add `chat.IsCreationAnchorMessage(msg)` filtering to `computeUnreadCount`, `lastMessagePreview`, and `assembleCommenters` in `server/services/portfolio/assembly.go`. This matches how `GetConversationContext` already filters them in `server/services/chat/conversations.go`.
- **Pros:** Fixes the root cause at the source. The `IsCreationAnchorMessage` helper already exists and is documented for exactly this purpose. No client changes needed. Consistent with existing server-side filtering in GetConversationContext.
- **Cons:** None. This is the intended behavior per the existing code comment.
- **Effort:** Small.

### Option B: Filter on the client side in the inbox ViewModel

- **Approach:** Modify the portfolio inbox ViewModel to suppress unread counts when the only messages are system messages.
- **Pros:** No server deploy needed.
- **Cons:** The client doesn't have enough information to distinguish creation anchors from other system messages. The `DailyItem` proto only exposes `unread_count` and `last_message_sender` — not message types. This would require either a new proto field or heuristic guessing. Violates the principle of fixing bugs at the source.
- **Effort:** Medium (needs proto changes or fragile heuristics).

### Option C: Mark creation anchor messages as pre-read on the server

- **Approach:** When inserting creation anchor system messages in `InsertSystemMessage`, set `ParticipantIdToIsRead` to `true` for all participants instead of `false`.
- **Pros:** Fixes unread counts globally, not just in the portfolio inbox.
- **Cons:** Changes the read-status semantics of system messages. Could have unintended effects on other features that check read status. Doesn't fix `lastMessagePreview` or `assembleCommenters` showing creation anchors as recent comments.
- **Effort:** Small, but incomplete fix.

## Recommendation

**Option A** is the correct fix. The `IsCreationAnchorMessage` helper was built for exactly this purpose — the portfolio assembly just isn't using it yet. Three functions need a one-line filter addition:

1. `computeUnreadCount` — skip creation anchors so they don't inflate unread badges
2. `lastMessagePreview` — skip creation anchors so the inbox doesn't show "Experience created" as the latest comment
3. `assembleCommenters` — skip creation anchors so the system actor doesn't appear in the commenter avatar stack

## Open Questions

- **Should non-anchor system messages (e.g., "Thomas RSVPed Going") also be filtered from counts?** Recommendation: No. Those represent real user actions that other participants should see. Only creation anchors — which exist purely to anchor widget cards — should be excluded, per the existing `IsCreationAnchorMessage` definition.

## Implementation Plan

### Phase 1: Filter creation anchors in portfolio assembly

**Goal:** The portfolio inbox no longer shows unread counts, message previews, or commenter avatars for creation anchor system messages.

**Files modified:**
- `server/services/portfolio/assembly.go`

**Tasks:**
- [ ] In `computeUnreadCount`: add `if chat.IsCreationAnchorMessage(msg) { continue }` before the unread check
- [ ] In `lastMessagePreview`: add `if chat.IsCreationAnchorMessage(msg) { continue }` before the latest-message comparison
- [ ] In `assembleCommenters`: add `if chat.IsCreationAnchorMessage(msg) { continue }` before adding to entries
- [ ] Add import for `library/server/chat` package
- [ ] Add unit tests in `server/services/portfolio/assembly_test.go`:
  - Test: conversation with only a creation anchor message returns unread count 0
  - Test: conversation with a creation anchor + a user message returns unread count 1 and the user message as preview
  - Test: conversation with only a creation anchor returns empty commenters list
- [ ] Run `npm run build` to verify lint + compilation
- [ ] Run `go test ./server/services/portfolio/...` to verify tests pass

**Validates:** Opening a newly created item in the portfolio inbox no longer shows a misleading "1 unread" badge when the only message is the creation anchor.

---

## Audit Notes

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`

**Findings:**

1. **Server architecture compliant:** The fix uses an existing shared library function (`chat.IsCreationAnchorMessage`) from the `server/chat` package — not a cross-service dependency. The portfolio service importing the `chat` shared library follows the established pattern.

2. **No client changes needed:** The `InlineConversationView`'s `hasRealMessages` check and "Be the first to message" placeholder are correct UX behavior and should remain.

3. **i18n note:** The hardcoded `'Be the first to message'` string at `inline_conversation_view.dart:474` is not localized (pre-existing, not introduced by this issue). Out of scope for this fix but worth tracking separately.

4. **Existing precedent:** `GetConversationContext` in `server/services/chat/conversations.go` already filters creation anchors from message counts using the same `IsCreationAnchorMessage` function. This fix aligns the portfolio inbox with that established pattern.

## Related Issues

- None identified.
