# Issue #1256: Chat doesn't show the message from the notification

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1256
**Filed:** 2026-04-24 | **Last activity:** 2026-04-29
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Audit notes (2026-04-29)

Reviewed against `docs/client/architecture.md` and `docs/client/caching.md`.
Adjustments applied to the original draft:

- **Non-destructive merge by default.** The original Option A called
  `loadMessages()` from both the resume hook and the gap-fill path.
  `loadMessages()` clears `_messageIds` and overwrites `state.messages`,
  which would silently drop pending optimistic sends. Replaced with a single
  shared `_mergeServerHistory()` helper that only adds messages whose IDs
  are not already known, leaving pending/failed entries untouched. Both
  Phase 1 and Phase 2 use this helper. `loadMessages()` itself is left
  unchanged (still used at first-init).
- **Async safety made explicit.** `conversationProvider` is
  `NotifierProvider.autoDispose.family`; per the architecture doc's "Async
  Safety in Notifiers" section, every async gap requires a `ref.mounted`
  check before the next state assignment. The reconnect timer callback,
  previously synchronous, becomes `async` with explicit `ref.mounted`
  checks before and after the merge `await`. Same rule applies to
  `refreshAfterResume()`.
- **`mark-as-read` decoupled.** `loadMessages()` calls `markMessagesRead()`
  internally as part of its initial-load behaviour. To avoid resurrecting
  that destructive code path, `refreshAfterResume()` invokes
  `chatRepository.markMessagesRead()` directly after the non-destructive
  merge and reuses the existing `_onMessagesMarkedAsRead` callback.
- **Repository boundary upheld.** All cache invalidation goes through
  `chatRepository.refreshConversationHistory()`; nothing in the plan
  reaches into `StashCacheManager` or service clients directly. Matches
  `docs/client/caching.md` Pattern 1.

## Problem Statement

When a user receives a chat-message push notification while the device is asleep
and the chat screen is already open in the background, tapping the notification
deep-links them to the correct gear/experience/request on the Chat tab — but the
message that triggered the notification is not shown. They see the cached state
from before the device went to sleep. The user must back out and re-enter, or
manually refresh, to see the new message.

The user explicitly suggested adding a manual pull-to-refresh as a backup. A
`RefreshIndicator` is already wired in `app/lib/presentation/widgets/chat/message_list.dart:213`
calling `ConversationNotifier.loadMessages()`, but the indicator sits at the top
of the `reverse: true` list — i.e., above the *oldest* messages, not the newest
ones. That makes it largely undiscoverable as "show me new stuff." So the user's
suggestion is partly already met but is positioned poorly; the real fix is
making the automatic refresh work.

The recently-merged PR #1423 (issue #1341) added exponential-backoff reconnect
to `ConversationMessagesMixin` when the stream raises `onError` or `onDone`.
That is necessary but not sufficient for this issue: (a) iOS suspends socket
I/O during sleep without firing `onDone`/`onError` on the Dart side until the
next write, so the stream sits in a zombie state; (b) even when reconnect does
fire, it only re-subscribes — it does not re-fetch history, so any messages
delivered to the server during the disconnect window are lost from the local
state until the user manually navigates away and back.

## Context from Comments

- **maintainer (collaborator, 2026-04-25):** Triaged as **P1** — "Broken
  notification deep-link + stale cache means Henry taps a notification, lands
  on home, and gives up. Directly breaks the response loop (stories #5, #7,
  #15) — seeker-protective concern per village-blueprint."
- **maintainer (collaborator, 2026-04-29):** "@claude be sure to check out
  #1341 to judge if that fixed this already." — Action: PR #1423 (merged
  2026-04-27) added `onError`/`onDone` reconnection but the user-described
  scenario is not covered. See "Why #1341/#1423 doesn't fix this" below.

## Why #1341/#1423 doesn't fix this

PR #1423 wires reconnection only when the underlying `Stream<StreamMessagesResponse>`
fires `onDone` or `onError` on the Dart subscription. The bug report's scenario
fails three different ways that #1423 does not address:

1. **iOS sleep silently zombifies the socket.** iOS suspends background
   network activity. The HTTP/2 framing layer in Connect-Go's Dart client does
   not detect the suspension proactively — `onError`/`onDone` only fire when
   the next read or write fails, which can be never if the app stays
   foregrounded after wake. Without an explicit kick on resume, the reconnect
   loop never starts.
2. **Reconnect fills no gap.** `_doStartStream` in
   `app/lib/presentation/viewmodels/conversation_messages_notifier.dart:174`
   re-subscribes via `chatRepository.streamMessages(...)`. Streaming RPCs only
   yield messages sent *after* the subscription opens, so any messages
   delivered to the server during the silent-zombie window stay missing from
   `state.messages` until the next full history fetch.
3. **Notification tap on the same route is a no-op.** `fcmService.onNavigate`
   in `app/lib/main.dart:446` calls `router.go(route)`. When the user is
   already on `/gear/<id>?tab=chat`, `go_router` does not rebuild the destination
   widget, and `_InlineConversationViewState._initialize()` is gated by the
   `_initialized` flag (set on first activation), so nothing re-runs.

## Potential Solutions

### Option A: Resume-driven refresh in `InlineConversationView` + gap-fill on stream reconnect (recommended)

- **Approach:** Two complementary changes plus one shared helper:
  1. Extract a non-destructive `_mergeServerHistory()` helper in
     `ConversationMessagesMixin` that fetches history via the chat repository
     and merges any messages whose `messageId` is not already in `_messageIds`.
     Pending/failed optimistic messages have temporary `tempId`s and so are
     never clobbered. This is the existing logic from
     `refreshMessagesAfterMutation()` (lines 676-720), promoted to a private
     shared helper.
  2. Make `_InlineConversationViewState` register a `WidgetsBindingObserver`.
     On `AppLifecycleState.resumed`, if `widget.isActive`, call a new
     `ConversationNotifier.refreshAfterResume()` that (a) cancels the current
     stream subscription, (b) awaits `_mergeServerHistory()`, (c) resets
     `_reconnectAttempts` and clears `isStreamDisconnected`, and (d) calls
     `startMessageStream()` to open a new subscription.
  3. In `ConversationMessagesMixin._scheduleReconnect`, change the timer
     callback to `async`, await `_mergeServerHistory()` before
     `_doStartStream()`, and add `ref.mounted` guards before and after the
     await. Wrap the merge in a try/catch so a transient history-fetch
     failure does not block the stream re-subscribe.
- **Pros:** Targets the three failure modes with one shared, non-destructive
  merge helper. No new Riverpod providers. Pending/failed local messages and
  their timeout timers are preserved automatically. Tests can use the
  existing `@visibleForTesting reconnectDelay` test hook to drive the timer.
- **Cons:** Adds one `WidgetsBindingObserver` per active
  `InlineConversationView`. In practice only one chat view is active at a
  time — `AutomaticKeepAliveClientMixin` keeps the widget alive across tab
  swaps but only the active one passes `widget.isActive == true` — so
  observer count stays low. Multiple observers coexist with the root one in
  `app/lib/main.dart:316` without conflict (Flutter dispatches lifecycle
  events to every registered observer). Refreshing the conversation
  context, transfer details, or parent gear/experience/request on resume is
  a separate concern that we deliberately do *not* trigger here — only
  message history.
- **Effort:** Small.

### Option B: Centralised "active conversation" registry + global resume hook

- **Approach:** Introduce a `Provider<String?>` tracking the currently-active
  conversation ID. The root `_RiplsAppState.didChangeAppLifecycleState`
  `resumed` branch (`app/lib/main.dart:336`) reads it and calls
  `conversationProvider(activeId).notifier.refreshAfterResume()`. Each
  `InlineConversationView` writes/clears the registry in its lifecycle
  callbacks.
- **Pros:** Lifecycle handling lives in one place (the existing root observer).
  No new `WidgetsBindingObserver` instances.
- **Cons:** Adds a new global Riverpod provider whose only purpose is to bridge
  the root lifecycle observer to a leaf widget. Requires careful clear-on-pop
  logic to avoid pointing at a disposed conversation. The active registry
  duplicates information the widget tree already encodes.
- **Effort:** Medium.

### Option C: Re-fetch via push-payload data without lifecycle hooks

- **Approach:** Extend the FCM push payload server-side to include the
  message body, sender, and `sent_at`, and have `FCMService._onForegroundMessageReceived`
  + `_onMessageOpenedApp` insert the message directly into the
  `ConversationNotifier.state.messages` if a matching conversation provider
  exists.
- **Pros:** No history re-fetch needed; lowest latency path.
- **Cons:** Requires server-side changes to push payload (cross-cutting),
  introduces a second message-ingestion pathway that has to stay consistent
  with the streaming pathway, and only fixes the "got a notification" subset
  — not other resume scenarios (e.g., user came back to the app without a
  notification but missed messages while away). Doesn't address the silent-
  zombie stream problem in the general case.
- **Effort:** Large (server + client + payload schema).

## Recommendation

**Option A.** It directly addresses all three identified failure modes with a
small, contained client-side change and reuses the existing `loadMessages()`
pathway. Option B's centralisation is not worth the new global state for a
single observer per active chat. Option C is a payload-schema change that
solves a narrower problem and would still leave the silent-zombie stream issue
in the general case.

The two halves of Option A are independent commits but should ship in the same
PR — both are necessary; either alone leaves a hole. (Lifecycle resume alone
misses messages that arrive during a brief in-app network blip; reconnect
gap-fill alone misses iOS sleep where reconnect doesn't fire.)

Pull-to-refresh is intentionally left as-is (already wired). A follow-up could
move the `RefreshIndicator` to the visual bottom (e.g., a custom overscroll
gesture in the reverse list) but that is out of scope here — fixing the
automatic case removes the urgency.

## Open Questions

- **Should `refreshAfterResume` also re-fetch conversation context, transfer
  details, and gear/experience/request details (the full `refresh()`
  pathway)?** — Recommendation: **No, only `loadMessages()` plus stream
  restart.** A resume should be cheap; the full `refresh()` invalidates seven
  repositories and re-fetches everything. The user's complaint is specifically
  about messages, and the existing per-event mutation handlers already pull
  context updates when they matter. If we discover post-merge that other state
  (e.g., participant list after a JOIN) is also stale on resume, we can extend
  `refreshAfterResume` then.
- **Should reconnect gap-fill always call `loadMessages`, or only when
  `_reconnectAttempts > 0`?** — Recommendation: **Only when
  `_reconnectAttempts > 0`.** The first connection attempt right after
  `initialize()` already runs `loadMessages()`, so re-running it inside
  `_scheduleReconnect`'s timer callback only happens after at least one
  failure — i.e., when there is a real gap to fill.
- **Should the resume hook fire even if the chat tab is collapsed
  (`isExpanded == false`)?** — Recommendation: **Yes, fire whenever
  `widget.isActive` is true.** The collapsed view also reads
  `convState.messages` (showing the last 3) and stale messages there are just
  as confusing. `isActive` is the existing gate for "this view is the live
  one"; reuse it.
- **Should we cancel the existing stream subscription before
  `loadMessages()`, or after?** — Recommendation: **Before.** Cancelling first
  prevents a race where the stream fires a `_handleStreamedMessage` callback
  in parallel with `loadMessages()` clearing `_messageIds`. The existing
  `_doStartStream` already cancels before subscribing; `refreshAfterResume`
  should do the same explicitly.
- **What happens if the user backgrounded the app *while sending* a message
  (state == `MessageState.pending`)?** — Recommendation: **Use the
  non-destructive merge helper, not `loadMessages()`.** `loadMessages()`
  clears `_messageIds` and overwrites `state.messages`, which would drop any
  pending optimistic message whose `tempId` is not in the server response.
  The shared `_mergeServerHistory()` helper described in Option A only
  *adds* messages whose IDs aren't already known, so pending/failed entries
  and their timeout timers in `_pendingMessageTimers` survive untouched.
  Note: `loadMessages()` also calls `markMessagesRead()` when there's an
  unread count. Resume should do the same (the user is now actively viewing
  the conversation), so `refreshAfterResume()` invokes
  `markMessagesRead()` directly via the chat repository after the merge —
  not by calling `loadMessages()` wholesale.

- **Should the resume hook be confined to `InlineConversationView`, or do
  other chat surfaces need it too?** — Recommendation: **Confined to
  `InlineConversationView`.** That widget is the only visible host for
  `conversationProvider(...)` in the app today —
  `experience_chat_pane.dart`, `request_chat_pane.dart`, and
  `gear_content_view.dart` all wrap `InlineConversationView`, so adding the
  observer there covers every entry point. If a future surface adopts
  `conversationProvider` without going through `InlineConversationView`,
  the same pattern can be repeated; a single shared mixin is over-engineered
  for one current consumer.

- **Is async safety in `refreshAfterResume()` and the reconnect timer a
  concern, given that `conversationProvider` is `autoDispose.family`?** —
  Recommendation: **Yes — every async gap needs an explicit `ref.mounted`
  check before the next state assignment, per
  `docs/client/architecture.md` "Async Safety in Notifiers".** The notifier
  can be disposed mid-await (e.g., user backs out of the chat screen during
  the merge fetch). Both new code paths must check `ref.mounted` after
  every await before mutating `state` or scheduling further work.

## Implementation Plan

### Phase 1: Extract non-destructive merge helper + reconnect gap-fill

**Goal:** When the stream reconnects after at least one failure, re-fetch
history first so messages received during the disconnect window are merged
into local state. Establishes the shared merge helper that Phase 2 also uses.

**Tasks:**
- [ ] In `app/lib/presentation/viewmodels/conversation_messages_notifier.dart`,
      extract a private helper
      `Future<void> _mergeServerHistory(String conversationId)` that:
      1. calls `chatRepository.refreshConversationHistory(conversationId)`
         to invalidate the cache,
      2. calls `chatRepository.getConversationHistory(conversationId: ...)`,
      3. checks `ref.mounted` before touching state,
      4. merges into `state.messages` only the entries whose `messageId`
         is not already in `_messageIds` (the existing dedupe set), using
         the same `ChatMessage`-construction logic as
         `refreshMessagesAfterMutation()` (lines 676-720),
      5. updates `_messageIds` for the new messages,
      6. emits a `state.copyWith(messages: ...)` only if at least one
         message was added (avoid spurious rebuilds).
- [ ] Refactor `refreshMessagesAfterMutation()` to delegate to
      `_mergeServerHistory()` so there is exactly one merge implementation.
- [ ] In `_scheduleReconnect` (currently lines 213-237), change the
      `_reconnectTimer` callback to async. New shape:
      ```dart
      _reconnectTimer = Timer(delay, () async {
        if (!ref.mounted) return;
        _messageStreamSubscription?.cancel();
        _messageStreamSubscription = null;
        try {
          await _mergeServerHistory(conversationId);
        } catch (e) {
          _log.warning('Reconnect history gap-fill failed: $e');
        }
        if (!ref.mounted) return;
        _doStartStream();
      });
      ```
- [ ] Confirm via inspection that
      `_pendingMessageTimers` and pending optimistic entries in
      `state.messages` are not touched by the merge — `_messageIds`
      already contains their `tempId`s, so the dedupe naturally skips
      them.
- [ ] Add a unit test in
      `app/test/presentation/viewmodels/conversation_view_model_test.dart`
      that:
      - uses the existing `reconnectDelay` test override to make the
        backoff zero-duration,
      - simulates a stream `onDone` (so the reconnect timer fires),
      - has the mock `getConversationHistory` return one new message
        that was not in the previous local state,
      - asserts the new message appears in `state.messages` after the
        reconnect timer completes,
      - asserts no duplicates when the new message also arrives on the
        re-subscribed stream,
      - asserts a pre-existing `MessageState.pending` entry survives.

**Validates:** Reconnect now fills the gap from a brief network drop, not just
re-opens the subscription, and the merge helper is reusable for Phase 2.

### Phase 2: Resume hook on `InlineConversationView`

**Goal:** When the app returns to the foreground while a chat view is active,
re-fetch history and re-subscribe the stream, regardless of whether the
underlying stream noticed it died.

**Tasks:**
- [ ] In `app/lib/presentation/viewmodels/conversation_view_model.dart`, add
      `Future<void> refreshAfterResume()` that:
      - cancels `_messageStreamSubscription` and clears the field,
      - calls the Phase 1 helper `_mergeServerHistory(conversationId)`
        (do **not** call `loadMessages()` — it would clobber pending
        optimistic messages),
      - if there were unread messages, calls
        `chatRepository.markMessagesRead(conversationId: ...)` and
        invokes the existing `_onMessagesMarkedAsRead` callback,
      - guards every state assignment with `ref.mounted` per
        `docs/client/architecture.md` async-safety rules,
      - resets `_reconnectAttempts = 0` and emits
        `state.copyWith(isStreamDisconnected: false)` if the flag was set,
      - calls `startMessageStream()` to open a fresh subscription.
- [ ] In `app/lib/presentation/widgets/content/inline_conversation_view.dart`,
      add `with WidgetsBindingObserver` to `_InlineConversationViewState`.
      Register via `WidgetsBinding.instance.addObserver(this)` in
      `initState`, deregister via
      `WidgetsBinding.instance.removeObserver(this)` in `dispose` (before
      the existing `super.dispose()`).
- [ ] Override `didChangeAppLifecycleState`. On
      `AppLifecycleState.resumed`: if
      `mounted && widget.isActive && _initialized`, call
      `ref.read(conversationProvider(widget.conversationId).notifier).refreshAfterResume()`
      and `unawaited` it (the future is fire-and-forget; the notifier
      handles its own errors). Skip when `!_initialized` — the existing
      `_initialize()` path fetches fresh data anyway.
- [ ] Add a unit test for `refreshAfterResume()` that asserts:
      - the existing stream subscription was cancelled,
      - `chatRepository.refreshConversationHistory` and
        `getConversationHistory` were called,
      - new server messages appear in `state.messages`,
      - a pre-existing `MessageState.pending` entry survives,
      - `markMessagesRead` was called when `unreadCount > 0`,
      - `_onMessagesMarkedAsRead` callback fires,
      - `isStreamDisconnected` is cleared,
      - `startMessageStream()` re-subscribes.
- [ ] Add a widget test for `InlineConversationView` (or a unit test for
      `_InlineConversationViewState` via a `tester.pumpWidget(...)`
      harness) that:
      - constructs the widget with `isActive: true` and a mock
        `ConversationNotifier` injected via
        `ProviderScope.overrides`,
      - dispatches an `AppLifecycleState.resumed` via
        `tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed)`
        (the canonical test API; if that helper is unavailable in the
        Flutter version, fall back to
        `WidgetsBinding.instance.handleAppLifecycleStateChanged`),
      - asserts the mock notifier's `refreshAfterResume()` was invoked,
      - asserts no invocation when `isActive: false`.

**Validates:** Tapping a notification while already on the chat screen, or
resuming the app from sleep with the chat screen active, refreshes the
message list without dropping in-flight optimistic messages.

### Phase 3: Manual smoke test on iOS

**Goal:** Confirm the fix on the original failure environment.

**Tasks:**
- [ ] Build a debug iOS bundle and install on a physical device (notifications
      do not deliver in the simulator).
- [ ] On Device A, open a gear chat with another user. Lock the device.
- [ ] On Device B, send a chat message in the same conversation.
- [ ] Wait for the push notification on Device A. Tap it.
- [ ] Confirm the new message is rendered without further interaction.
- [ ] Repeat the test once with the chat screen open and Device A foregrounded
      but with airplane mode toggled off-on for ~3 s on Device A — confirm
      reconnect gap-fill.

**Validates:** End-to-end coverage of the original user complaint on the
exact platform that surfaced it.

## Related Issues

- #1341 / PR #1423 — Added the exponential-backoff reconnect that this issue
  builds on. Phase 1 here extends the same reconnect path with gap-fill.
- #1064 — Adjacent stale-cache class of bug ("request cache stale"); the
  refresh-on-resume pattern from Phase 2 is conceptually similar but scoped to
  conversations.
- #1036 / #1036 v2 — Chat deep-link plans; relevant background on how
  notification routing currently works.
