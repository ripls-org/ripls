# Issue #947: System messages do not appear in chat until at least one user has added a message

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/947
**Filed:** 2026-03-30 | **Last activity:** 2026-03-30
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

When a conversation contains only system messages (e.g., "Alice expressed interest",
"Giveaway shared"), the chat area within content detail screens (gear, experience,
request) shows "Be the first to message" instead of displaying the system messages.
All system messages appear immediately once a user sends a real text message.

The root cause is an explicit filter in `InlineConversationView` (line 470-481) that
checks `hasRealMessages` — whether any message in the list is NOT a system message.
If all messages are system messages, the widget renders an empty-state placeholder
instead of the `MessageList`, hiding the system messages entirely.

This is a **client-side display bug only**. The server correctly returns all messages
(system and user) via `GetConversationHistory`, and the `ConversationScreen` (full-screen
chat opened from inbox) displays system messages correctly. The bug is isolated to the
inline chat pane embedded in content views.

## Context from Comments

No comments on the issue. The description is brief but sufficient: "The chat area is
blank even though there have been several system messages. They all appear once a user
explicitly adds a chat message."

## Potential Solutions

### Option A: Remove the hasRealMessages filter entirely

- **Approach:** Delete the `hasRealMessages` check in `InlineConversationView`
  (lines 470-481) and always render the `MessageList` when `messages` is non-empty.
- **Pros:** Simplest fix. System messages will appear immediately. The "Be the first
  to message" text was misleading when system messages existed.
- **Cons:** The empty state ("Be the first to message") would only show for
  conversations with literally zero messages, which may be a very brief window
  (system messages are inserted immediately after conversation creation).
- **Effort:** Small (single-line change).

### Option B: Show system messages but keep empty-state for truly empty conversations

- **Approach:** Change the condition from `hasRealMessages` to `messages.isNotEmpty`.
  When there are messages of any kind (system or user), render the `MessageList`.
  Only show "Be the first to message" when `messages.isEmpty`.
- **Pros:** System messages display correctly. True empty state is preserved for the
  brief window before any messages exist. Consistent with how `ConversationScreen`
  handles this.
- **Cons:** None significant. This is what the code should have done originally.
- **Effort:** Small (single-line change).

### Option C: Differentiate empty state text based on message types

- **Approach:** When only system messages exist, show a different empty-state message
  like "No messages yet" instead of "Be the first to message", while still hiding the
  system messages.
- **Pros:** Avoids UI noise from system messages in a fresh conversation.
- **Cons:** Still hides system messages, which is the exact bug being reported.
  Users want to see what happened in the conversation. Doesn't solve the issue.
- **Effort:** Small.

## Recommendation

**Option B** — replace the `hasRealMessages` check with `messages.isNotEmpty`. This
directly fixes the reported bug with minimal code change and aligns the inline view's
behavior with the full-screen `ConversationScreen`, which already displays system
messages correctly.

System messages provide important context (who expressed interest, when something
was shared, when status changed). Hiding them until a user message arrives makes the
conversation feel broken and removes useful context about what has happened.

## Open Questions

- **Should the "Be the first to message" placeholder use an i18n key?** —
  Recommendation: Yes. The current string at line 475 is hardcoded English. While
  fixing it is out of scope for this bug, it should be noted as a follow-up for the
  i18n lint check to catch.

- **Does this interact with issue #946 (system messages in unread counts)?** —
  Recommendation: No conflict. #946 addresses unread *counts* (badges). This issue
  addresses message *display*. Showing system messages in the chat pane while
  excluding them from unread counts is the desired behavior — users should see
  what happened without getting false urgency from badges.

## Implementation Plan

### Phase 1: Fix the inline conversation view filter

**Goal:** System messages appear in the inline chat pane immediately.

**Tasks:**
- [ ] In `app/lib/presentation/widgets/content/inline_conversation_view.dart` (lines 470-481), replace the `hasRealMessages` check with `messages.isEmpty`. When messages exist (system or user), render the `MessageList`. Only show the empty-state placeholder when there are truly no messages.
- [ ] Update or add a unit test verifying that system-only message lists render the `MessageList` widget (not the empty state).
- [ ] Verify that the empty state still appears for conversations with zero messages.

**Validates:** The reported bug is fixed. System messages appear immediately in gear, experience, and request chat panes.

### Phase 2: Build verification

**Goal:** Everything compiles and all tests pass.

**Tasks:**
- [ ] Run `flutter test --reporter expanded` for client tests.
- [ ] Manual smoke test: open a gear/experience/request with only system messages, verify they display.

**Validates:** No regressions.

## Related Issues

- #946 — "System messages should not be included in unread or total message counts" — complementary fix for badge counts. No conflict with this display fix.
- #925 — "System message not showing" — may be the same or related issue.

## Audit Notes

Audited against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`.

**No architectural issues found:**
- Fix is entirely in the presentation layer (widget), no boundary crossings.
- No ViewModel, Repository, or Service changes needed — data already flows correctly.
- No caching impact — messages are already fetched and cached properly.
- Server/client boundary is correct — server already returns system messages in `GetConversationHistory`.

**i18n flag (confirmed):** The hardcoded string `'Be the first to message'` (line 475) is not in `app_en.arb` or `app_es.arb`. The plan correctly flags this as a follow-up. When implementing Phase 1, consider replacing it with `context.l10n` while you're editing the file — it's a one-line change in the same block of code being modified. Add the key to both ARB files per the i18n doc's "each new string needs both locales" rule.

**Theme note (out of scope):** The empty-state text uses hardcoded `Colors.white` (line 476). This widget renders on a dark overlay surface, so white text is intentional — not a dark-mode violation. No change needed.
