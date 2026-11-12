# Issue #1341: [error_handling] Surface chat stream disconnections to UI with reconnection

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1341
**Filed:** 2026-04-25 | **Last activity:** 2026-04-26
**Labels:** auto-filed, review:error_handling | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Two real-time message streams — one in `ConversationNotifier` (per-conversation live
feed) and one in `InboxNotifier` (inbox unread-count updates) — silently die on network
errors. Both streams only log in their `onError` handlers. There is no reconnection
logic and no state update, so users see a frozen chat screen with no indication that
new messages are not arriving.

Additionally, `ChatService.streamMessages()` (the underlying gRPC call) catches all
transport errors internally and ends the stream without re-throwing. This means the
`onDone` callback fires instead of `onError` when a network error occurs. Both
viewmodels currently set no `onDone` handler, so a silent network drop causes the
stream to end with no action taken.

## Context from Comments

- **maintainer (collaborator, 2026-04-26):** "@claude implement the reconnect, only
  show UI if it fails" — this clarifies the behavior: silently retry with backoff;
  only surface an error banner in the UI when all reconnection attempts are exhausted.
  This supersedes the issue body's suggestion of a "reconnecting..." banner during retry.

## Potential Solutions

### Option A: Inline reconnection in each ViewModel

Add reconnection logic directly to each notifier with fields for retry counts and
timers. `ConversationNotifier` adds `isStreamDisconnected` to its Freezed state.
`InboxNotifier` fails silently after max retries (no UI needed for background stream).

- **Approach:** Add `_reconnectAttempts` counter and `_reconnectTimer` to
  `ConversationNotifier`. Add per-conversation maps to `InboxNotifier`. Handle both
  `onError` and `onDone`. Cancel timers in `onDispose`.
- **Pros:** Self-contained, no new abstractions. Minimal diff. Follows existing patterns.
- **Cons:** Some duplication between the two notifiers. Reconnect logic is embedded in
  business logic.
- **Effort:** Small

### Option B: Shared ReconnectingStream wrapper

Create a utility class `ReconnectingStream<T>` that wraps any `Stream<T>` and handles
reconnection with backoff. Both notifiers use this wrapper.

- **Approach:** Implement `ReconnectingStream` that takes a factory function `Stream<T>
  Function()` and exposes a single stream with transparent reconnection.
- **Pros:** No code duplication. Reusable for future streams.
- **Cons:** More abstraction than the problem warrants. Requires new file and tests for
  the wrapper itself. The inbox and conversation streams have different reconnect
  semantics (inbox: silent, conversation: surface error) making a shared wrapper awkward.
- **Effort:** Medium

### Option C: Convert stream to periodic polling after failure

On stream failure, fall back to periodic polling (e.g., `loadMessages()` every 30s)
until the stream reconnects.

- **Approach:** In `onDone`/`onError`, start a timer that calls `loadMessages()` and
  re-attempts `startMessageStream()`. If stream reconnects, cancel the polling timer.
- **Pros:** Ensures messages are delivered even when streaming is unavailable.
- **Cons:** Polling is expensive and duplicates the existing pull-to-refresh behavior.
  Adds complexity and may cause duplicate messages or ordering issues. Over-engineered
  for this issue.
- **Effort:** Medium-Large

## Recommendation

**Option A: Inline reconnection.** The duplicated logic between `ConversationNotifier`
and `InboxNotifier` is small (~30 lines each) and each has genuinely different behavior
(conversation shows UI error, inbox fails silently). A shared abstraction would need
conditional behavior for the two cases — less clear than inline code. Option C is
over-engineered.

Reconnect schedule: exponential backoff with base 1s, max 5 attempts (1s, 2s, 4s,
8s, 16s). After 5 failures, `ConversationNotifier` sets `isStreamDisconnected = true`
in state; `InboxNotifier` logs a warning and stops retrying.

The UI banner (from `isStreamDisconnected`) should appear in `InlineConversationView`
above the message list, with a prompt for the user to pull-to-refresh.

## Open Questions

- **Should the reconnect counter reset when a message successfully arrives?**
  Recommendation: Yes — reset `_reconnectAttempts = 0` on each received message so
  a recovered connection starts fresh.

- **Should `onDone` always trigger reconnection, including when the notifier is being
  disposed?**
  Recommendation: Guard with `ref.mounted` check — skip reconnect if the notifier is
  being torn down. This avoids async callbacks racing against disposal.

- **Should `isStreamDisconnected` be cleared automatically when the stream reconnects?**
  Recommendation: Yes — when a message successfully arrives after a disconnect, clear
  `isStreamDisconnected`. The banner should disappear without user action.

- **Does InboxNotifier need UI state for stream disconnection?**
  Recommendation: No — inbox stream failures only delay unread count updates, not
  visible messages. Silent retry is sufficient. Users can pull-to-refresh.

- **What delay base and max attempts to use?**
  Recommendation: base 1s, factor 2, max 5 attempts (delays: 1, 2, 4, 8, 16s).
  Total max wait ≈ 31s. Reasonable for transient network issues without holding a
  long open retry loop.

## Implementation Plan

### Phase 1: Add reconnection to ConversationNotifier

**Goal:** Silently reconnect the per-conversation stream with backoff; surface a
"connection lost" banner only after max retries.

**Tasks:**
- [ ] Add `@Default(false) bool isStreamDisconnected` to `ConversationState` (freezed)
- [ ] Add `_reconnectAttempts`, `_maxReconnectAttempts`, `_reconnectTimer` fields
      to `ConversationNotifier`
- [ ] Cancel `_reconnectTimer` in `ref.onDispose`
- [ ] Add `onDone` handler to `startMessageStream()` that guards with `if (!ref.mounted) return`
      before calling `_scheduleReconnect` (avoids racing against disposal)
- [ ] Change `onError` handler to guard with `if (!ref.mounted) return` and call
      `_scheduleReconnect` instead of just logging
- [ ] Reset `_reconnectAttempts` and clear `isStreamDisconnected` on each received message
- [ ] Implement `_scheduleReconnect`: increments counter, schedules timer with backoff,
      sets `isStreamDisconnected = true` after max attempts. Every state assignment inside
      `_scheduleReconnect` must be preceded by a `if (!ref.mounted) return` guard because
      it runs from a `Timer` callback that may fire after disposal.
      **Before calling `startMessageStream()` from the timer, cancel and null out
      `_messageStreamSubscription`** to avoid a double-subscription if the old stream
      recovers concurrently.
- [ ] Add a disconnection banner to `InlineConversationView._buildMessageArea`
      when `convState.isStreamDisconnected` is true. The banner **must**:
  - Use `context.l10n.<key>` for all visible strings — add the key to both
    `app_en.arb` and `app_es.arb` before running `npm run generate` (CI lint
    `npm run lint:dart:i18n` enforces no hardcoded strings).
  - Use `AppColors` helpers (e.g. `AppColors.surface(context)`,
    `AppColors.textPrimary(context)`) for all colors — never `Colors.*` literals,
    which break dark mode.

**Validates:** ConversationNotifier reconnects silently; banner appears only on failure.

### Phase 2: Add reconnection to InboxNotifier

**Goal:** Silently reconnect per-conversation inbox streams with backoff; no UI change.

**Tasks:**
- [ ] Add `_reconnectAttempts Map<String, int>` and `_reconnectTimers Map<String, Timer>`
      to `InboxNotifier`
- [ ] Cancel all `_reconnectTimers` in `ref.onDispose` (in addition to the existing
      `_messageStreamSubscriptions` cancel already there)
- [ ] Extract `_subscribeToConversation(String conversationId)` method for single-
      conversation subscription (called both from `_startListeningToAllConversations`
      and from the reconnect path). Inside this method, **cancel and remove any existing
      entry in `_messageStreamSubscriptions[conversationId]`** before creating the new
      subscription to prevent orphaned subscriptions when reconnecting.
- [ ] Add `onDone` handler that guards with `if (!ref.mounted) return` before calling
      `_scheduleInboxReconnect`
- [ ] Change `onError` handler to guard with `if (!ref.mounted) return` and call
      `_scheduleInboxReconnect`
- [ ] Implement `_scheduleInboxReconnect`: per-conversation backoff, silent failure
      after max attempts. `inboxProvider` is a plain `NotifierProvider` (not autoDispose),
      so the notifier lives for the full app session; nonetheless, guard each state access
      inside timer callbacks with `if (!ref.mounted) return` for correctness.
- [ ] Reset attempt counter on successful message receipt

**Validates:** Inbox streams reconnect silently; no regression in unread count behavior.

### Phase 3: Tests

**Goal:** Verify reconnection and banner behavior are correct.

**Tasks:**
- [ ] ConversationNotifier: test that `startMessageStream` reconnects after `onDone`
- [ ] ConversationNotifier: test that `isStreamDisconnected` becomes true after max retries
- [ ] ConversationNotifier: test that `isStreamDisconnected` clears when stream recovers
- [ ] ConversationNotifier: test that disposing the notifier while a reconnect timer is
      pending completes without throwing (disposal safety test — required for all async
      Notifiers per architecture guidelines)
- [ ] InboxNotifier: test that inbox stream reconnects silently after error/done
- [ ] InboxNotifier: test that no state change occurs after max retries
- [ ] InboxNotifier: test that no orphaned subscription is left when `_subscribeToConversation`
      is called a second time for the same conversation ID

**Validates:** Reconnection logic is exercised end-to-end in unit tests.

## Related Issues

- None referenced in comments.
