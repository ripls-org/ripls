# Issue #1133: [Bug Report] Jumping into chat from notification leads to stale messages

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1133
**Filed:** 2026-04-15 | **Last activity:** 2026-04-15
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a user taps a chat push notification (containing a new message preview),
the app navigates to the correct conversation but displays stale messages —
the new message that triggered the notification is missing. The user expects
to see the message they just read in the notification.

## Context from Comments

No comments on this issue. The description is clear and self-contained: the
user taps a notification, lands on the right chat, but the latest message
isn't there.

## Root Cause Analysis

The notification-to-chat navigation flow has a structural problem:

1. **FCM tap handler** (`fcm_service.dart:routeNotificationPayload`) routes to
   `/gear/{gearId}?tab=chat` (or `/experience/`, `/request/`), not directly to
   a conversation screen.

2. **Gear screen** (`gear_content_view.dart`) initializes, loads gear details,
   then renders `InlineConversationView` with `isActive: true` when `initialTab == 2`.

3. **InlineConversationView._initialize()** (line 135) fetches the conversation
   and context from the chat repository **using cached data** — it calls
   `chatRepository.getConversation()` and `chatRepository.getConversationContext()`
   without first invalidating those caches.

4. **ConversationNotifier.initialize()** calls `loadMessages()`, which **does**
   invalidate the history cache via `chatRepository.refreshConversationHistory()`
   before fetching. So the messages themselves should be fresh.

5. **The real problem**: The `InlineConversationView._initialize()` path does
   NOT invalidate conversation or context caches. But more critically, the
   entire initialization sequence (gear load -> tab switch -> inline conversation
   init -> conversation notifier init -> loadMessages -> startMessageStream)
   introduces enough latency that the fresh fetch may still miss messages
   delivered between the notification push and the fetch completing.

6. **Stream starts too late**: `startMessageStream()` runs AFTER `loadMessages()`
   completes (line 174 in conversation_view_model.dart). Messages arriving in
   the gap between the server's history response and the stream subscription
   are lost.

7. **autoDispose provider**: `conversationProvider` is `autoDispose.family`,
   so if the user previously viewed this conversation and navigated away, the
   provider was disposed. Re-entering creates a fresh notifier that must go
   through the full initialization sequence.

The most likely scenario: the notification arrives, the user taps it within
seconds, the app navigates through the gear screen to the chat tab, the
conversation loads messages from the server, but the message that triggered
the notification either (a) arrived during the gap between history fetch
and stream subscription, or (b) the history fetch returns data that was
already in flight before the new message was committed on the server.

## Potential Solutions

### Option A: Force-refresh on notification entry

- **Approach:** When navigating from a notification, pass a signal through the
  route (e.g., a query parameter like `?tab=chat&refresh=true`) that causes
  `InlineConversationView` to invalidate all conversation caches before
  initializing. Additionally, start the message stream before (or in parallel
  with) loading history so no gap exists.
- **Pros:** Targeted fix, minimal blast radius. Only notification-initiated
  navigations pay the cost of cache invalidation.
- **Cons:** Requires threading a parameter through the router and down to the
  conversation widget. Doesn't fix the stream-before-history race in the
  general case.
- **Effort:** Small.

### Option B: Always refresh conversation history on init (remove history cache for initial load)

- **Approach:** In `ConversationNotifier.loadMessages()`, the code already
  calls `refreshConversationHistory()` before `getConversationHistory()`.
  The issue is that `InlineConversationView._initialize()` calls
  `getConversation()` and `getConversationContext()` from cache without
  refreshing. Fix by also refreshing those caches. Additionally, start the
  message stream concurrently with history loading.
- **Pros:** Fixes the bug for all entry paths, not just notifications.
  Simpler than threading parameters.
- **Cons:** Slightly more network calls when opening a conversation that was
  recently viewed. But conversations are already doing a `refreshConversationHistory`
  on every open, so this is consistent.
- **Effort:** Small.

### Option C: Stream-first architecture

- **Approach:** Reverse the initialization order: start the message stream
  first, accumulate incoming messages, then load history and merge. This
  eliminates the gap entirely.
- **Pros:** Most robust solution. No messages can be missed regardless of
  timing.
- **Cons:** More complex. Requires handling deduplication between streamed
  messages and history. The existing `_messageIds` set handles this for the
  current flow but would need to work in reverse.
- **Effort:** Medium.

## Recommendation

**Option B** — always refresh conversation and context caches on init, and
start the message stream concurrently with history loading.

This is the simplest fix that addresses the root cause. The conversation
view model already refreshes history cache; extending this to conversation
and context caches is consistent. Starting the stream earlier closes the
gap without requiring the complexity of Option C's stream-first dedup.

Option A is too narrow — if this bug exists for notifications, it exists for
any re-entry into a conversation. Option C is the ideal long-term architecture
but is overkill for this bug.

## Open Questions

- **Does `startMessageStream` duplicate messages already in history?** —
  Recommendation: Yes, the `_messageIds` set in `ConversationNotifier`
  already handles dedup (line 87, checked at line 223). Starting the stream
  earlier is safe.

- **Should we also invalidate the conversations list cache on notification
  tap?** — Recommendation: Not necessary for this fix. The conversations list
  is only used by the inbox screen, not the conversation screen. The inbox
  will refresh on its own when the user navigates back.

## Implementation Plan

### Phase 1: Fix cache staleness on conversation open

**Goal:** Ensure fresh data when opening any conversation, eliminating the
stale message bug.

**Tasks:**
- [ ] In `InlineConversationView._initialize()`, call
  `chatRepository.refreshConversation(conversationId)` and
  `chatRepository.refreshConversationContext(conversationId)` before the
  `getConversation()` and `getConversationContext()` calls
- [ ] ~~In `ConversationNotifier.initialize()`, move `startMessageStream()` to
  run concurrently with `loadMessages()` inside the `Future.wait()` block~~
  **[AUDIT: REMOVED]** Running `startMessageStream()` concurrently with
  `loadMessages()` introduces a new race condition: `loadMessages()` clears
  `_messageIds` and overwrites `state.messages` on completion (line 203-230).
  A streamed message arriving mid-load would be added to messages, then
  immediately wiped when `loadMessages()` sets its final state. Keep the
  current sequential order (load first, then stream). The cache refresh fix
  alone should resolve the staleness.
- [ ] Verify that `_handleStreamedMessage()` correctly deduplicates against
  messages loaded by `loadMessages()` using the `_messageIds` set

**Validates:** Opening a conversation from a notification shows the latest
message. The cache refresh ensures fresh history is fetched from the server.

> **[AUDIT NOTE: Architecture layer violation]**
> `InlineConversationView` is a widget (`presentation/widgets/`) that directly
> calls `chatRepositoryProvider`. Per `docs/client/architecture.md`:
> "ViewModels are the ONLY layer that calls Repositories — Screens/Widgets
> never access repositories directly." This is a pre-existing violation (the
> widget already calls `getConversation()` and `getConversationContext()` at
> lines 145-149). Adding `refresh*()` calls extends this violation but keeps
> the change contained to existing code. A follow-up cleanup should move the
> conversation/context fetching and refresh logic into
> `ConversationNotifier.initialize()` so the widget only interacts with the
> ViewModel. That refactor is out of scope for this bug fix.

### Phase 2: Test and verify

**Goal:** Confirm the fix works in the notification flow and doesn't regress
normal conversation opening.

**Tasks:**
- [ ] Test notification tap -> chat opens with latest message visible
- [ ] Test normal conversation open from inbox still works correctly
- [ ] Test that duplicate messages don't appear (stream dedup still works)
- [ ] Verify no performance regression from the additional cache invalidation
  calls (these are local cache clears, not network calls)

**Validates:** No regressions in chat UX; notification deep link shows fresh
messages.

## Related Issues
- #1036 — Chat deep link improvements (related notification routing work)
- #962 — Chat notification routing (prior notification routing fixes)
- #977 — Notification deep links (general deep link infrastructure)
