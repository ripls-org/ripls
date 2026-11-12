# Issue #1028: Reactions to messages

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1028
**Filed:** 2026-04-04 | **Last activity:** 2026-04-04
**Labels:** enhancement, user-feedback | **Assignees:** unassigned
**Context docs:** none (Signal app screenshots provided as UI reference)

## Problem Statement

Users want to react to chat messages with emoji (e.g., thumbs up) so others
know they saw a message without typing a reply. This is an industry-standard
messaging feature that the app currently lacks entirely — no reaction storage,
no reaction RPCs, no reaction UI.

## Context from Comments

The maintainer (maintainer) defined the full UX spec:

1. **Long-press** on a message → emoji reaction bar appears (Signal-style)
2. **Quick emoji**: heart, thumbs up, thumbs down, plus an ellipsis for the
   full emoji picker
3. **Tap an emoji** → it attaches to the message bubble as a small reaction badge
4. **Tap the reaction badge(s)** → bottom sheet showing a table: reaction →
   user who reacted
5. **Remove reaction**: the user who added it can tap their entry in the bottom
   sheet to remove it (with "tap to remove" hint text)

Visual reference: Signal's reaction bar — a dark pill-shaped row of 6 quick
emoji + `...`, floating directly above the long-pressed message.

## Potential Solutions

### Option A: Dedicated reaction storage + new RPCs

- **Approach:** Add a `MessageReaction` model to protos. Store reactions in a
  separate table (message_id, user_id, emoji, created_at). Add `AddReaction`
  and `RemoveReaction` RPCs. Broadcast reaction updates via the existing
  `StreamMessages` stream using a new oneof variant. Client gets reactions
  alongside message history.
- **Pros:** Clean separation — reactions are their own entity. Easy to query
  "who reacted to what." Scales to many reactions per message. Reaction updates
  are lightweight (no full message re-fetch).
- **Cons:** New table, new RPCs, new stream message type. More proto surface.
- **Effort:** Medium.

### Option B: Reactions as a field on ChatMessage

- **Approach:** Add a `repeated MessageReaction reactions` field directly to
  the `ChatMessage` storage model and the API response types
  (`MessageHistoryItem`, `StreamMessagesResponse`). Reactions are stored inline
  with the message. `AddReaction`/`RemoveReaction` RPCs modify the message
  record and re-broadcast it.
- **Pros:** Simpler storage — no new table. Reactions travel with messages
  naturally. History API returns reactions without extra joins.
- **Cons:** Each reaction update re-serializes the whole message proto. With
  many reactions per message this could grow, but in practice community chats
  won't have thousands of reactions per message. The storage layer's protosql
  handles repeated nested messages via the serialized proto_data blob, so
  reactions don't need flattened columns.
- **Effort:** Small–medium.

### Option C: Reactions as system messages

- **Approach:** When a user reacts, send a system message (e.g.,
  `CHAT_SYSTEM_ACTION_REACTED`) that references the target message. Client
  aggregates reactions by scanning system messages.
- **Pros:** No schema changes to ChatMessage. Uses existing message
  infrastructure.
- **Cons:** Terrible UX — reaction system messages clutter the chat. Expensive
  to aggregate. Removing a reaction requires another system message. Fragile.
- **Effort:** Small but poor outcome.

## Recommendation

**Option B.** Reactions as a repeated field on `ChatMessage` is the simplest
approach that works well at the scale of community group chats (tens of
participants, not thousands). The protosql layer stores nested messages in the
serialized `proto_data` blob, so no schema migration or new table is needed —
reactions serialize naturally. The existing `broadcastMessage()` mechanism
already broadcasts full `StreamMessagesResponse` messages; we extend it to
carry a new "reaction update" variant so that reaction changes stream to all
participants in real time.

The tradeoff vs Option A (separate table) is that we can't efficiently query
"all reactions by user X across all messages" — but that's not a needed feature.
What we need is "what reactions does this message have?" which is trivially
answered when reactions live on the message.

## Open Questions

- **Which quick-reaction emoji?** — Recommendation: ❤️ 👍 👎 😂 😮 😢 (6
  emoji matching Signal's defaults). The `...` opens the platform emoji picker.
  These cover the most common emotional responses.
- **Can a user add multiple different reactions to the same message?** —
  Recommendation: No, one reaction per user per message (same as Signal,
  iMessage, Slack). Tapping a different emoji replaces the previous one.
- **Should reactions trigger push notifications?** — Recommendation: Not in
  the initial implementation. Reactions are lightweight acknowledgments. If users
  request it later, add a notification preference.
- **Should reactions work on system messages?** — Recommendation: No. Only
  user messages (bubbles) should be reactable. System messages are informational
  pills.
- **How should reactions display in the immersive (dark) chat?** —
  Recommendation: Small emoji badges below/beside the bubble, semi-transparent
  background pill matching immersive styling. The reaction picker bar should
  use the same dark pill aesthetic as the Signal reference.

## Implementation Plan

### Phase 1: Proto + server storage

**Goal:** Reactions can be stored and retrieved with messages.

**Tasks:**

- [ ] Add `MessageReaction` message to `proto/ripls/models/chat.proto`:
  `user_id` (string), `emoji` (string), `created_at_unix_sec` (int64)
- [ ] Add `repeated MessageReaction reactions` field to `models.ChatMessage`
- [ ] Add `Reaction` message to `proto/ripls/api/chat_service.proto`:
  `user_id` (string), `emoji` (string), `sender` (User)
- [ ] Add `repeated Reaction reactions` field to `api.MessageHistoryItem` and
  `api.StreamMessagesResponse` (the message envelope — NOT inside the
  `UserMessage` oneof content, since reactions are per-message metadata)
- [ ] Add `AddReactionRequest` / `AddReactionResponse` (conversation_id,
  message_id, emoji)
- [ ] Add `RemoveReactionRequest` / `RemoveReactionResponse` (conversation_id,
  message_id)
- [ ] Add `AddReaction` and `RemoveReaction` RPCs to ChatService
- [ ] Run `npm run generate` to regenerate Go + Dart code

**Validates:** Proto compiles, generated code is available.

### Phase 2: Server RPC implementation

**Goal:** Reactions can be added/removed via API and broadcast in real time.

**Tasks:**

- [ ] Implement `AddReaction` handler in `server/services/chat/reactions.go`:
  fetch message, enforce one-reaction-per-user (replace existing), update
  message, store, broadcast reaction update. Use a transaction
  (`storage.BeginTx`) around the read-modify-write cycle to prevent lost
  reactions when two users react simultaneously.
- [ ] Implement `RemoveReaction` handler: fetch message, remove user's
  reaction, update message, store, broadcast. Same transactional pattern.
- [ ] Add a `ReactionUpdate` variant to the stream broadcast: include
  message_id, conversation_id, and updated reactions list so clients can patch
  their local state
- [ ] Ensure `GetConversationHistory` returns reactions on each
  `MessageHistoryItem` (they come from the stored `ChatMessage` naturally)
- [ ] Add unit tests for add/remove/replace/idempotent-remove in
  `reactions_test.go`

**Validates:** Reactions round-trip through the API; stream broadcasts updates.

### Phase 3: Client — reaction picker UI

**Goal:** Long-pressing a message shows the Signal-style reaction bar.

**Tasks:**

- [ ] Add `reactions` field to client `ChatMessage` model (list of Reaction
  protos parsed from `MessageHistoryItem`)
- [ ] Add long-press `GestureDetector` to message bubbles in `MessageList`
- [ ] Build `ReactionPickerOverlay` widget: pill bar with 6 quick emoji +
  `...` button, positioned above the long-pressed bubble using
  `CompositedTransformFollower` (same pattern as mention overlay). Use
  `AppColors.cardBackground(context)` and `AppColors.border(context)` for
  theme-aware styling — not hardcoded dark colors — so it works in both
  standard (ConversationScreen) and immersive (InlineConversationView) modes.
  For immersive mode, wrap in a dark `Theme` (same approach as
  `MentionableTextField.immersive`).
- [ ] `...` button opens platform emoji picker (e.g., `emoji_picker_flutter`
  package or system input)
- [ ] On emoji tap: call `addReaction` on the ViewModel → repository →
  service → server RPC; dismiss overlay
- [ ] Add `addReaction` and `removeReaction` methods to all three layers:
  `ChatService` (RPC call), `ChatRepository` (service call + history cache
  invalidation), `ConversationNotifier` (repository call + optimistic state
  update). Widgets call the Notifier only — never the repository or service
  directly.
- [ ] Handle streamed reaction updates in `_handleStreamedMessage` to update
  local message state

**Validates:** User can long-press, pick an emoji, and see it persist.

### Phase 4: Client — reaction badges + detail sheet

**Goal:** Reactions display on message bubbles; tapping shows who reacted.

**Visual reference (Signal):**

- **Reaction badge**: A single emoji (or grouped emoji row) displayed just
  below the bottom-right corner of the message bubble. No pill background —
  just the emoji character itself. Positioned to slightly overlap the bubble's
  bottom edge.
- **Reaction detail sheet**: Bottom sheet with filter tab row at top ("All - N"
  pill, then each distinct emoji + count), followed by a user list. Each row:
  avatar + user name on the left, the emoji they reacted with on the right. If
  the current user's row, subtitle reads "Tap to remove". The original message
  remains visible above the sheet for context.

**Tasks:**

- [ ] Build `ReactionBadge` widget: emoji (or grouped emoji row if multiple
  distinct reactions) displayed just below the bubble's bottom edge, aligned to
  the right for outgoing messages, left for incoming. For multiple identical
  reactions, show emoji + count (e.g., "👍 2"). No pill background — bare emoji
  matching the Signal style.
- [ ] Integrate `ReactionBadge` into `_buildBubbleContent` in `MessageList`,
  positioned below the bubble with slight vertical overlap
- [ ] Build `ReactionDetailSheet` (modal bottom sheet):
  - Top row: filter tabs — "All - N" pill (selected), then each distinct
    emoji + count as a tappable filter
  - Body: list of users filtered by selected tab. Each row: avatar + name on
    left, emoji on right. Current user's row shows localized "Tap to remove"
    subtitle (use `context.l10n` — add ARB keys for "All", "Tap to remove",
    and any other user-visible strings) and calls `removeReaction` on the
    Notifier (not the repository directly)
- [ ] Wire tap on `ReactionBadge` to open `ReactionDetailSheet`
- [ ] Add optimistic UI: show reaction badge immediately on tap, revert on
  server error
- [ ] Ensure reactions render correctly in both standard and immersive
  (dark-background) bubble modes

**Validates:** Full reaction UX works end-to-end.

## Architecture Audit

Audited against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, and `docs/server/architecture.md`.

**Changes made during audit:**

1. **Proto placement fix**: Moved `repeated Reaction reactions` from
   `UserMessage` (oneof content) to `MessageHistoryItem` and
   `StreamMessagesResponse` (the message envelope). Reactions are per-message
   metadata, not part of the user/system message oneof.
2. **Concurrency safety**: Added transaction requirement for the read-modify-
   write cycle in `AddReaction`/`RemoveReaction` handlers to prevent lost
   reactions under concurrent writes.
3. **Data access layering**: Clarified that widgets call the Notifier only.
   `addReaction`/`removeReaction` follow the full chain: Notifier → Repository
   (with history cache invalidation) → Service → RPC.
4. **Theme-aware colors**: Changed reaction picker from hardcoded dark colors to
   `AppColors` theme-aware helpers + `immersive` dark `Theme` wrapper pattern
   (matching the `MentionableTextField` approach from #1026).
5. **i18n**: Added note to use `context.l10n` ARB keys for "Tap to remove",
   "All", and other user-visible strings in the detail sheet.

**Compliant (no changes needed):**

- Server file organization: `reactions.go` + `reactions_test.go` as separate
  focused files within `server/services/chat/`.
- Proto package separation: models `MessageReaction` uses `user_id` (string);
  API `Reaction` uses `sender` (User). No cross-package imports.
- Dedicated request/response types per RPC (not reused).
- Caching: reactions travel inline with messages; `ChatRepository` handles
  invalidation after mutations.
- No service-to-service dependencies — all reaction logic is within the chat
  service.
- Separate widgets (`ReactionPickerOverlay`, `ReactionBadge`,
  `ReactionDetailSheet`) keep build methods small.

## Related Issues

- #1026 — @-mentions fix (same chat infrastructure, recently modified files)
