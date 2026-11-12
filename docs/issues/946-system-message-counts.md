# Issue #946: System messages should not be included in unread or total message counts

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/946
**Filed:** 2026-03-30 | **Last activity:** 2026-03-30
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

System messages (e.g., "Alice joined", "Loan approved", "Experience created") are
counted in both unread message counts and total message counts throughout the app.
This inflates badge numbers and gives users false urgency — a conversation might
show "3 unread" when all three are automated status updates, not messages from
real people.

The codebase already has partial fixes: `comment_preview.go` excludes creation
anchor messages from its unread count, and `GetConversationMessageCount` skips
creation anchors for total counts. But the main `GetUnreadCounts` RPC and the
unread counting in `GetConversation`, `ListConversations`,
`GetConversationForTransfer`, `GetConversationForRequest`, and
`GetConversationForExperience` all count system messages the same as user
messages. The client-side `InboxNotifier` message stream handler also increments
unread counts for system messages without filtering.

## Context from Comments

No comments on the issue. The description is also empty — the title is the full
specification. This is straightforward enough that no further clarification is
needed.

## Potential Solutions

### Option A: Skip all system messages in unread counts

- **Approach:** Add a `msg.GetSystemMessage() != nil` check to every unread-counting
  loop on the server. Also update `GetConversationMessageCount` to skip all system
  messages (not just creation anchors). On the client, filter system messages in the
  stream handler before calling `incrementUnreadCount`.
- **Pros:** Simple, consistent, easy to understand. System messages are never
  "from" anyone in a meaningful sense, so excluding them from counts is always correct.
- **Cons:** A system message announcing something important (e.g., "Giveaway
  cancelled") would not bump the unread count. Users might miss it if they don't
  open the conversation.
- **Effort:** Small.

### Option B: Extract a shared helper, skip all system messages

- **Approach:** Same logic as Option A, but extract the duplicated unread-counting
  loop into a shared helper function (e.g., `countUnreadUserMessages`) in the `chat`
  library package. All 6 server call sites use it instead of inline loops. Also
  update the client stream handler.
- **Pros:** Eliminates duplication (the same 5-line loop is copy-pasted 6 times).
  Future changes to counting logic happen in one place. Consistent with the existing
  `IsCreationAnchorMessage` helper pattern.
- **Cons:** Slightly more refactoring. The helper needs access to the user ID.
- **Effort:** Small.

### Option C: Configurable per-action exclusion

- **Approach:** Instead of blanket-excluding all system messages, define a set of
  "notification-worthy" system actions that DO count (e.g., cancellation, completion)
  and exclude only the rest.
- **Pros:** More nuanced — important state changes could still bump counts.
- **Cons:** Over-engineered for this issue. Adds a classification layer that needs
  ongoing maintenance as new system actions are added. The comment preview code
  already chose the "exclude all anchors" approach.
- **Effort:** Medium.

## Recommendation

**Option B** — extract a shared helper and skip all system messages. The duplicated
counting loop across 6 RPC handlers is a maintenance risk, and this issue is the
right time to fix it. The helper function is trivial and follows the existing
pattern of `IsCreationAnchorMessage`.

Excluding all system messages (not just creation anchors) is the right call. System
messages represent automated state transitions, not human communication. If a user
needs to know about a state change, the app already surfaces that through UI state
(status pills, modals, push notifications) — the unread badge shouldn't be the
delivery mechanism for system events.

## Open Questions

- **Should `populateLastMessage` also skip system messages?** — Recommendation: Yes.
  The "last message" preview in conversation lists should show the last *human*
  message, not "Loan approved." The comment preview code already does this for
  creation anchors; extend it to all system messages. If the only messages are system
  messages, show nothing (the conversation has no real activity).

- **Should `GenConversationSummary.MessageCount` exclude system messages?** —
  Recommendation: Yes, for consistency. The summary already includes system message
  text in the AI prompt (which is fine for context), but the reported count should
  reflect human messages only.

## Implementation Plan

### Phase 1: Server — shared helper and unread count fix

**Goal:** All unread counts exclude system messages; duplicated loop is eliminated.

**Tasks:**
- [ ] Add `CountUnreadUserMessages(messages []proto.Message, currentUserID string) int32` to a new file `server/chat/unread.go`. The function iterates messages, skips any with `GetSystemMessage() != nil`, and counts those from other users that are unread by `currentUserID`. (New file keeps `conversations.go` focused on conversation creation/retrieval per server architecture guidelines.)
- [ ] Replace the inline unread-counting loop in `GetUnreadCounts` (`server/services/chat/unread_counts.go:86-94`) with a call to the new helper.
- [ ] Replace the inline loop in `GetConversation` (`server/services/chat/conversations.go:276-283`).
- [ ] Replace the inline loop in `ListConversations` (`server/services/chat/conversations.go:393-400`).
- [ ] Replace the inline loop in `GetConversationForTransfer` (`server/services/chat/conversations.go:525-532`).
- [ ] Replace the inline loop in `GetConversationForRequest` (`server/services/chat/conversations.go:604-611`).
- [ ] Replace the inline loop in `GetConversationForExperience` (`server/services/chat/conversations.go:692-699`).
- [ ] Replace the unread counting loop in `buildPreviewWithUserMap` (`server/conversation/comment_preview.go:156-168`). This loop currently only skips creation anchors via `IsCreationAnchorMessage` — it should use the new helper to skip all system messages consistently. (Note: `comment_preview.go` is in the `conversation` package, not the `chat` service package, so the helper must be in the shared `server/chat/` library — which it is.)
- [ ] Update `GetConversationMessageCount` in `server/chat/conversations.go:306-311` to skip all system messages (not just creation anchors). The existing `IsCreationAnchorMessage` check is now subsumed by the broader system message check.
- [ ] Update `populateLastMessage` (`server/services/chat/conversations.go:731`) to find the most recent *user* message for the preview, skipping system messages.
- [ ] Update `GenConversationSummary` `MessageCount` (`server/services/chat/summary.go:122`) to exclude system messages from the reported count. (System message text should still be included in the AI prompt for context.)
- [ ] Add unit tests in `server/chat/unread_test.go`: create message slices with a mix of user and system messages, verify counts exclude system messages and edge cases (all system, all user, empty slice).
- [ ] Update existing tests in `server/services/chat/unread_counts_test.go` and `server/conversation/comment_preview_test.go` to verify system messages are excluded.

**Validates:** Server-side counts are correct. Existing tests still pass.

### Phase 2: Client — filter system messages in stream handler

**Goal:** Real-time unread count increments on the client skip system messages.

**Tasks:**
- [ ] In `inbox_view_model.dart` message stream handler (~line 312), check `message.hasSystemMessage()` and skip the `incrementUnreadCount` call for system messages.
- [ ] Add a unit test for the stream handler verifying system messages don't increment counts.

**Validates:** Client-side badges don't increment for system messages arriving via stream.

### Phase 3: Build verification

**Goal:** Everything compiles and all tests pass.

**Tasks:**
- [ ] Run `npm run build` to verify Go lint + compilation.
- [ ] Run `go test ./...` for server tests.
- [ ] Run `flutter test --reporter expanded` for client tests.

**Validates:** No regressions.

## Related Issues

- #925 — "System message not showing" — related to system message rendering (display, not counting).

## Audit Notes

Audited against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`.

**Changes made:**
1. **Missing call site added:** `server/conversation/comment_preview.go:156-168` was omitted from Phase 1. It has its own unread counting loop that only filters creation anchors, not all system messages. Added as a task.
2. **File placement clarified:** Changed helper location from "conversations.go or unread.go" to definitively `server/chat/unread.go`. Server architecture doc recommends < 500-line focused files; `conversations.go` already handles conversation CRUD.
3. **`IsCreationAnchorMessage` subsumption noted:** Both `GetConversationMessageCount` and `comment_preview.go` use `IsCreationAnchorMessage` — the new "skip all system messages" check subsumes this. Noted in the task descriptions so implementers don't double-filter.
4. **Test file locations specified:** Added `server/chat/unread_test.go` for the new helper and named existing test files to update.
5. **Line references added:** Added specific file paths and line numbers for `populateLastMessage` and `GenConversationSummary.MessageCount` tasks.

**No architectural issues found:**
- Shared helper correctly placed in `server/chat/` library (not in a service package) — follows service independence principle.
- Client change is in the ViewModel stream handler, which is the correct layer for filtering before calling `incrementUnreadCount` on the repository.
- No new user-facing strings introduced — no i18n impact.
- Unread count repository's optimistic delta pattern is unaffected — the delta simply won't be incremented for system messages.
- No caching changes needed — server returns corrected counts, repository reflects them.
