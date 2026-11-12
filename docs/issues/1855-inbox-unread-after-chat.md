# Issue #1855: Inbox shows unread items even when they have been viewed in chat

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1855
**Filed:** 2026-05-11 | **Last activity:** 2026-05-11
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** [docs/client/inbox.md](../client/inbox.md), [docs/server/architecture.md](../server/architecture.md), [docs/server/observability.md](../server/observability.md), [docs/client/caching.md](../client/caching.md)

## Problem Statement

When a user taps a chat-message push notification, the app deep-links straight
to the relevant gear / experience / request screen with the Chat tab open. The
chat pane fetches the conversation history and calls `MarkMessagesRead`. Yet
when the user returns to the portfolio inbox tab, the same item still renders
with "unread" styling — bold title, bold status row, bold time label — even
though every chat message has been read. The user has to tap the row from the
inbox to clear the styling, which is the duplicate work the issue is reporting.

The root cause is that the inbox row's "unread" appearance is driven by *two*
independent server-side states:

1. `DailyItem.unread_count` — count of chat messages the current user has not
   read in the linked conversation. Cleared by `MarkMessagesRead`.
2. `DailyItem.is_watched_unread` — a boolean on the user's `WatchedItem` row
   for the underlying gear / experience / request, set true on new activity
   and cleared only by `MarkInboxItemRead`.

When the user taps an inbox row, `portfolio_inbox_screen.dart` calls
`markItemRead(item)` (server `MarkInboxItemRead`) *before* navigating, so both
states clear. When the user taps a notification, the deep-link path bypasses
the inbox screen entirely; `MarkInboxItemRead` is never called. The chat pane
clears state (1) but state (2) sticks until the next explicit inbox tap.

## Context from Comments

Only one comment on the issue: `@claude` from a maintainer (maintainer)
triggering this planning run. No design discussion or workaround. The bug
report and screenshot in the body are the entire signal.

## Potential Solutions

### Option A: Server-side — mark the watch entry read inside `MarkMessagesRead`

- **Approach:** In `server/services/chat/messages.go` `MarkMessagesRead`, after
  the per-message read loop succeeds, derive the watch key from
  `conversation.Topic` (the existing `watchKeyFromTopic` helper, already used
  by `SendMessage` to mark watchers *unread*) and call
  `WatchStorage.MarkRead(ctx, authInfo.UserID, watchType, watchItemID)`.
  Handle the transfer-topic case (which `watchKeyFromTopic` currently skips
  with a comment about avoiding a DB lookup) by looking up the transfer to
  get its `gear_id` — `MarkMessagesRead` is rare enough that the extra read
  is fine. Failures are logged at WARN and do not fail the RPC, matching the
  existing pattern for `MarkUnreadForWatchers`.
- **Pros:**
  - One symmetric place: `SendMessage` marks watchers unread, `MarkMessagesRead`
    marks the reader's watch read. Mirror is obvious and discoverable.
  - Fixes both the deep-link path *and* any future client path that calls
    `MarkMessagesRead` without going through the inbox tap.
  - Single source of truth; no client coordination needed across four chat
    pane callsites (`gear_content_view`, `experience_chat_pane`,
    `request_chat_pane`, `community_chat_pane`).
  - The client refresh plumbing already works: the chat repository's
    `markMessagesRead` already invalidates the conversation cache and calls
    `_onDailyInvalidated`, which triggers
    `portfolioCacheInvalidationProvider.notify()`. The portfolio inbox
    notifier listens to that and schedules a refetch with the standard 300 ms
    debounce. Once the server returns `is_watched_unread = false`, the row
    re-renders correctly.
- **Cons:**
  - Couples the chat service to watch storage, but `SendMessage` already does
    that — no new coupling.
  - Transfer-topic conversations require one extra DB read to resolve the
    gear ID. The cost is real but bounded (one row, indexed lookup) and
    `MarkMessagesRead` is not in a hot loop.
- **Effort:** small.

### Option B: Client-side — add `MarkInboxItemRead` to each chat pane's `onMessagesMarkedAsRead` callback

- **Approach:** In each of the four chat pane wiring sites (`gear_content_view.dart`,
  `experience_chat_pane.dart`, `request_chat_pane.dart`,
  `community_chat_pane.dart`), extend the existing
  `onMessagesMarkedAsRead: (conversationId) => resetUnreadForConversation(...)`
  callback to also call `portfolioRepository.markInboxItemRead(itemType, itemId)`
  using whatever item type / id the parent screen already knows. Optionally
  also optimistically update `portfolioInboxProvider.optimisticallyReadItemIds`
  so the inbox UI flips immediately.
- **Pros:**
  - Pure client change; no server build / migration risk.
  - Lets us also fire the optimistic UI update so the row clears the moment
    the chat is read, not 300 ms after the next server roundtrip.
- **Cons:**
  - Four parallel callsites — easy to miss one when a fifth chat surface
    appears. The community chat pane doesn't even have a watched-item
    counterpart, so the wiring has to differ across panes.
  - Existing implicit contract ("a watched item is unread until you mark it
    so on the server") is now also enforced by the chat panes, which is the
    kind of behaviour that quietly diverges from the server's intent over
    time.
  - Doesn't help any future non-app client that calls `MarkMessagesRead`
    (web, integration tests, AI agents) — they all have to re-implement the
    same coupling.
- **Effort:** small, but spread across four files.

### Option C: Both — server fix + thin client optimistic update

- **Approach:** Do Option A as the load-bearing fix, then add a small
  optimistic update on the client (`portfolioInboxProvider.optimisticallyReadItemIds`)
  in the chat pane callback so the row clears in the inbox before the 300 ms
  debounce + roundtrip. The server fix guarantees correctness; the client
  optimistic update is purely about perceived latency.
- **Pros:**
  - Correct (server-enforced) *and* feels instant.
  - Optimistic update is reverted automatically on the next refresh, so even
    if the server fix temporarily fails, the inbox self-heals.
- **Cons:**
  - More moving parts than Option A alone. Worth it only if reviewers think
    the post-refresh delay is noticeable.
- **Effort:** small-medium.

## Recommendation

**Option A** is the right primary fix and should land alone first. It
addresses the root cause — server state divergence between chat-read and
watch-read — at exactly the layer where the divergence happens, mirrors an
existing pattern (`SendMessage` → `MarkUnreadForWatchers`), and removes a
class of "deep link X bypasses inbox-tap-only side effect" bugs we'd
otherwise face every time a new client surface marks messages read.

I am *not* recommending Option C on this branch unless the maintainer asks
for it. The chat repository already invalidates `portfolioCacheInvalidationProvider`
on `markMessagesRead`, which kicks off the standard 300 ms debounced refresh.
After Option A, that refresh returns `is_watched_unread = false` and the row
clears. If the perceived latency is a problem in practice, Option C can be
layered on later in a separate PR — it does not require revisiting Option A.

## Open Questions

- **Should transfer-topic conversations also clear the gear's watch entry?**
  — Recommendation: Yes. `watchKeyFromTopic` currently returns UNSPECIFIED
  for transfer topics with a comment about avoiding a hot-path DB lookup
  inside `SendMessage`. `MarkMessagesRead` is not hot in the same way (it
  fires once per chat open, not once per keystroke), so the implementation
  should look up the transfer's `gear_id` and mark the GEAR watch entry
  read. Done as a small dedicated helper inside `messages.go` so the
  `SendMessage` hot path is untouched.

- **Should `MarkMessagesRead` mark the watch read even when `req.UpToUnixSec`
  is set and there are still unread messages after that cutoff?** —
  Recommendation: No. Only mark the watch read when *all* messages from
  other participants are now read (i.e. when the post-loop unread count
  for this conversation would be zero). The simplest way to express this:
  re-check the participant's unread count after the loop and only mark the
  watch read if it reached zero. This preserves the invariant "watch entry
  is read iff the user is caught up on chat".

- **Should community-wide chats also touch a watch entry?** —
  Recommendation: No. Community-wide chats don't appear as `DailyItem`s and
  have no `WatchedItem` row. `watchKeyFromTopic` already returns UNSPECIFIED
  for `ConversationTopic_CommunityId`. No code change needed here.

- **Backfill: do existing users with stuck `is_watched_unread = true` rows
  need a migration?** — Recommendation: No. The next time they tap into the
  chat, `MarkMessagesRead` will clear the row organically. A migration
  would race with the in-flight bug and isn't worth the complexity for a
  cosmetic issue.

## Implementation Plan

### Phase 1: Server fix in `MarkMessagesRead`

**Goal:** When a user marks all unread messages in a conversation as read,
the corresponding watch entry for that user is also marked read.

**Tasks:**
- [ ] In `server/services/chat/messages.go` `MarkMessagesRead`, capture the
  `conversation` returned by `requireConversationAccess` (currently
  discarded via `_, err := ...`).
- [ ] After the per-message read loop, derive the watch key for the current
  user. Reuse `watchKeyFromTopic` for `gear_id` / `experience_id` /
  `request_id` topics. For `transfer_id` topics, add a new helper
  `resolveTransferWatchKey(ctx, transferID)` that loads the transfer and
  returns `(WatchedItemType_GEAR, transfer.GearId)`. For
  `community_id` / nil topics, no-op.
- [ ] Compute the user's remaining unread count for the conversation. If
  zero (all caught up), call
  `storage.NewWatchStorage(s.storage).MarkRead(ctx, authInfo.UserID, watchType, watchItemID)`.
  `MarkRead` is already a safe no-op when no active watch row exists, so
  no extra guard is needed for users who tapped a stale notification for a
  dismissed item.
- [ ] Reuse the existing `ws := storage.NewWatchStorage(s.storage)` pattern
  from the `SendMessage` block above — same import path, same error
  handling.

**Observability:** Extend the existing `logger` (from
`logging.LoggerWithContext(ctx).With("operation", "MarkMessagesRead", ...)`)
with `"watch_type", watchType.String()` and `"watch_item_id", watchItemID`
before calling `MarkRead`. On failure, use `logger.WarnContext(ctx, "failed
to mark watcher read", "error", err)` — WARN, not ERROR, because the failure
is non-fatal and the RPC still succeeds. This matches the
`MarkUnreadForWatchers` pattern in `SendMessage` above and the rules in
[docs/server/observability.md](../server/observability.md) §"Appropriate Log
Levels".

**Validates:** That clearing chat unread also clears watch unread on the
server, regardless of which client path invoked `MarkMessagesRead`.

### Phase 2: Server tests

**Goal:** Lock in the new behaviour and prevent regression.

**Tasks:**
- [ ] Add `TestMarkMessagesRead_ClearsWatchUnread` in
  `server/services/chat/messages_test.go`. Use the existing helpers
  (`setupTestChatService`, `createTestCommunity`,
  `createTestCommunityMembership`, `createTestLoan`,
  `contextWithAuth`) — same shape as `TestMarkMessagesRead_UpdatesReadFlag`.
  Setup: user A sends a message on a transfer/gear conversation while
  user B watches the gear (seed via
  `storage.NewWatchStorage(sqlStorage).UpsertWatch(...)` and a follow-up
  `MarkUnreadForWatchers` to mimic the live state). Call `MarkMessagesRead`
  as B. Assert: B's `WatchedItem.IsRead` is now true.
- [ ] Add `TestMarkMessagesRead_TransferTopic_ClearsGearWatch` covering the
  transfer-topic → gear-watch path explicitly, including the
  `resolveTransferWatchKey` lookup.
- [ ] Add `TestMarkMessagesRead_PartialRead_LeavesWatchUnread` covering the
  `UpToUnixSec` cutoff case — when there are still unread messages above
  the cutoff, the watch entry stays unread.
- [ ] Add `TestMarkMessagesRead_CommunityTopic_NoOp` to assert no panic /
  no spurious watch row for community-wide conversations.
- [ ] Add `TestMarkMessagesRead_NoWatchEntry_NoOp` to assert that calling
  `MarkMessagesRead` for a user who has no active watch (e.g. a stranger
  who joined the chat without ever auto-watching) succeeds and does not
  insert a watch row.
- [ ] Run `npm run build` (lint + `go build ./...`) and
  `go test ./server/services/chat/...`. Per
  [CLAUDE.md](../../CLAUDE.md) "Go Server Build Verification", the tests
  here are *not* behind the `integration` build tag, so they run via the
  default `go test` invocation.

**Validates:** Each branch of the new logic (gear, experience, request,
transfer, community, partial-read, no-watch) has coverage and won't
silently break.

### Phase 3: Client-side verification

**Goal:** Confirm the inbox actually refreshes after the server fix without
any client changes — i.e. validate the recommendation in this plan.

**Tasks:**
- [ ] Build the Flutter app against a server with Phase 1 + 2 applied.
- [ ] Manual repro: send a chat message to test-user-A from test-user-B,
  let A receive the FCM push, tap it, read the chat, return to the inbox
  tab, confirm the row is no longer bold and the chat-count badge is
  gone. Use the `ripls-app-logs` skill (Android emulator) to capture
  Flutter logs during the repro.
- [ ] If the row still looks unread after ~6 seconds (300 ms debounce + up
  to 5 s cooldown, per [docs/client/inbox.md](../client/inbox.md)
  §"Background refresh"), investigate the client refresh path:
  `chat_repository.dart markMessagesRead` should call `_onDailyInvalidated`,
  `portfolioCacheInvalidationProvider` should fire,
  `PortfolioInboxNotifier._scheduleRefreshInPlace` should run, and
  `_refreshInPlace` should call `_repository.refreshInboxFeed()` +
  `getInboxFeed(timezone)`. If any of those is missing, file a follow-up.
- [ ] **Known refresh-cooldown caveat.** The portfolio inbox notifier
  applies a 5-second cooldown to consecutive invalidations. If another
  invalidation (e.g. a community-event-poll tick) fires immediately before
  the user taps into the chat, the post-`MarkMessagesRead` refresh is
  *deferred* until cooldown ends. The deferred refresh still runs (via
  `_cooldownTimer`), so the inbox eventually corrects itself — but the
  delay can stretch past the user's return to the inbox. If reviewers
  consider this delay user-visible, layer Option C (an optimistic
  `optimisticallyReadItemIds` update from the chat pane callback) in a
  follow-up PR. Do not add it to this PR; this PR keeps scope tight to
  the root-cause server fix.

**Validates:** End-to-end, the reported user-visible bug is gone.

## Related Issues

- #1036 — `chat-deep-link` — the original plan that wired notification taps
  to open the chat tab. The deep-link path it introduced is the one that
  bypasses `markItemRead`; this issue is the corresponding bug.
- No other directly related open issues found.

---

## Audit Notes

Plan audited against:

- [docs/client/architecture.md](../client/architecture.md) — MVVM, async-first,
  Riverpod patterns. No client code changes proposed, so MVVM rules are
  trivially satisfied.
- [docs/client/caching.md](../client/caching.md) — Repository-owned caching;
  notifier-driven invalidation. The recommended path leverages the existing
  `portfolioCacheInvalidationProvider` flow; no new cache wiring needed.
- [docs/client/inbox.md](../client/inbox.md) — Authoritative reference for
  the inbox view, the watch-entry model, and the background-refresh
  debounce/cooldown timing surfaced in Phase 3.
- [docs/client/i18n.md](../client/i18n.md) — No new user-visible strings;
  rule N/A.
- [docs/client/accessibility.md](../client/accessibility.md) — No new
  interactive UI; a11y primitives not involved.
- [docs/client/testing.md](../client/testing.md) — No client tests added.
  Server tests in Phase 2 follow the existing `messages_test.go` table-style
  pattern.
- [docs/server/architecture.md](../server/architecture.md) — Chat service
  already depends on the shared `WatchStorage` library via the
  `SendMessage` → `MarkUnreadForWatchers` path. The Phase 1 change adds a
  symmetric call from the same service to the same library; no new
  service-to-service coupling is introduced.
- [docs/server/observability.md](../server/observability.md) — Logging
  conventions for the new WARN line are explicit in Phase 1 (extend the
  existing `LoggerWithContext` logger, use `WarnContext` for the non-fatal
  failure, include `watch_type` / `watch_item_id` fields).
- [CLAUDE.md](../../CLAUDE.md) — No proto changes, no new
  `CommunityEventType` value, no new exported Go symbols needing
  doc-comment audits, no Flutter file splits. `npm run build` is called out
  in Phase 2.
