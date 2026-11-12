# Issue #2253: Chat enhancements: ability to edit and delete, and inline reply to messages

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2253
**Filed:** 2026-05-30 | **Last activity:** 2026-05-30
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none (screenshots in issue: Ripls discussion thread + iOS iMessage context menu reference)

## Problem Statement

A user wrote a message in a discussion thread and wanted to **edit** or **delete**
it, but found no affordance to do so. They expect the iMessage/WhatsApp pattern:
long-press a message and a menu appears offering edit/delete (and other actions).
The second screenshot in the issue is the iOS iMessage context menu, showing the
emoji reaction row on top and a vertical action menu below (Reply, Edit, Undo Send,
Copy, ...). That is the target mental model.

The maintainer comment (maintainer, the repo owner — authoritative) scoped the
work into three concrete features:

1. **Edit / Delete own messages.** On long-press of a message *authored by the
   current user*, augment the existing reaction emoji menu to also offer **Edit**
   and **Delete**.
2. **Haptic "buzz" on long-press.** Long-press should fire a haptic on activation,
   matching how other messaging apps confirm the gesture registered.
3. **Inline reply (quoting replies).** Long-press on *any* message (own or others')
   should offer **Reply**, which composes a new message that quotes the one being
   replied to.

The key implementation constraint surfaced during research: today the long-press
reaction picker is **deliberately disabled on the current user's own messages**
(`chat_bubble.dart:154` — `onLongPress: ... && !isCurrentUser`). So a user literally
*cannot* long-press their own message right now, which is exactly why edit/delete
feels absent. This is consistent with the user's note that they "thought they saw
something like this before" — they saw the reaction picker, but only ever on *other*
people's messages.

No contradictions between the description and the comment; the comment is a clean
superset that defines scope.

## Context from Comments

One comment, from the repo owner (authoritative direction), reproduced above. It
adds two things beyond the raw feedback:

- **Reply is in scope and applies to all messages**, not just the author's own.
  Reply produces "quoting replies" (a quoted snippet of the parent message, à la
  WhatsApp/Signal), not threaded sub-conversations.
- **Haptic feedback on long-press** is an explicit requirement, not just a nice-to-have.

There is no disagreement to resolve. The only open product detail the comment leaves
implicit is the **edit window / deletion semantics** (can you edit forever? does
delete tombstone or fully remove?) — addressed in Open Questions.

## Codebase Findings (what already exists)

The good news: the data model and UI are already 80% scaffolded for this.

**Proto / storage — delete is already anticipated.**
- `proto/ripls/models/chat.proto:209` — `ChatMessage` already has
  `optional DeletedMetadata deleted = 12`, and its comment literally says:
  *"A future manual 'delete my message' UI will reuse the same field."* Storage
  already filters soft-deleted messages from every read path
  (`server/storage/chat_message.go:33`, `GetByID`, `QueryByField` — proven by
  `server/chat/soft_delete_test.go`).
- `UserChatMessage` (`chat.proto:110`) has **no edit marker** and **no reply
  reference** yet — both need new fields.
- Reactions already work end-to-end: `AddReaction`/`RemoveReaction` RPCs
  (`chat_service.proto:567`), stored inline on `ChatMessage.reactions`, broadcast
  over the chat event bus (`server/chat_event_bus/chat_event_bus.go`,
  `KindReactionUpdate`).

**Server.**
- Chat service: `server/services/chat/` — `messages.go` (SendMessage),
  `reactions.go` (Add/RemoveReaction), `message_converters.go`,
  `authorization.go` / `helpers.go` (`requireConversationAccess`).
- Event bus kinds today: `KindUserMessage`, `KindSystemMessage`,
  `KindReactionUpdate`, `KindSystemMessageUpdate`. Edit/delete will need new
  kinds (e.g. `KindUserMessageUpdate`, `KindMessageDelete`) so live streams
  reflect the change without a refetch.
- Undo registry (`server/undo/registry.go`) — soft-delete maps cleanly to
  `ClassificationRetentionRestore`. **No new `CommunityEventType` is required
  unless we choose to emit community events for edit/delete** (these are direct
  message mutations, not community-feed events). See Open Questions.

**Client (Flutter).**
- Single message render: `app/lib/presentation/widgets/chat/message_list/chat_bubble.dart`.
  Long-press trigger at line 154 (currently `!isCurrentUser`-gated);
  `_showReactionPicker()` builds an `Overlay` entry positioned above the bubble.
- The picker itself: `app/lib/presentation/widgets/chat/reaction/reaction_picker.dart`
  — a pill row of 6 quick emoji + a "···" ellipsis to the full `EmojiPickerSheet`.
  **This is the widget to augment** with an action row (Edit/Delete/Reply).
- `isCurrentUser` is already computed at `message_list.dart:308`
  (`chatMessage.senderId == currentUserId`) and threaded into `ChatBubble`.
- Service/repo/viewmodel chain for mutations is well-established:
  `chat_service.dart` → `chat_repository.dart` (cache-invalidation) →
  `conversation_messages_notifier.dart` (optimistic update + revert-on-failure,
  the pattern used by `addReaction`/`removeReaction`/`sendMessage`).
- Haptics: `HapticFeedback.lightImpact()` / `mediumImpact()` used elsewhere
  (`camera_viewport.dart`, `plus_button_modal.dart`); none on chat long-press yet.
- l10n: `app/lib/l10n/app_en.arb` + `app_es.arb`, accessed via `context.l10n.*`.
  Reaction a11y keys (`a11yChatPickEmoji`, etc.) are the naming model.

**Reference UI (WhatsApp / Signal), per the user's suggestion.** Both apps render
a reply as a **quoted preview block inside the new message's bubble**: a thin
colored accent bar on the leading edge, the original sender's name in the accent
color, and a 1–2 line truncated snippet of the quoted text (or a media thumbnail).
Tapping the quote scrolls to the original. Both also offer **swipe-to-reply**
(swipe the bubble right → single haptic buzz → compose) *in addition* to the
long-press menu. While replying, a **compose banner** sits above the text input
showing the quoted message with an "✕" to cancel. This plan adopts the quoted-block
+ compose-banner styling; swipe-to-reply is proposed as an optional fast-follow.

## Architecture Audit (ripls-audit)

Audited against `docs/client/{architecture,caching,i18n,accessibility,testing}.md`
and `docs/server/{architecture,observability}.md`. Findings folded into the phase
tasks below; the load-bearing ones:

**Client layering (architecture.md §Key Architectural Rules).** ViewModels call
**repositories only**, repositories call **services only**. So the notifier
(`conversation_messages_notifier.dart`) must call
`chatRepository.deleteMessage()` / `.editMessage()` — never `chatService` directly.
The repository owns the RPC call **and** cache invalidation. Optimistic UI lives in
the notifier via `state.copyWith(...)` on the Freezed `ConversationState`.

**Async safety (architecture.md §Async Safety in Notifiers).** The conversation
notifier is screen-scoped; every notifier method that assigns `state` after an
`await` needs an `if (!ref.mounted) return;` guard (or `SafeNotifierMixin`). The
revert-on-failure branch needs the same guard. Disposal tests are required for new
async methods.

**Cache invalidation (caching.md Pattern 7).** `ChatRepository` mutations already
call `dailyCacheInvalidationProvider.notify()` and invalidate the conversation
cache. `deleteMessage`/`editMessage` must do the same so the daily screen and
conversation history stay fresh. Edits/deletes do **not** change unread counts, so
leave `UnreadCountRepository` deltas untouched.

**Accessibility (accessibility.md).** The new Edit/Delete/Reply controls must use
the `Tappable` (or `IconAction`) primitive — raw `GestureDetector`/`InkWell` fails
`avoid_raw_gesture_detector` in CI. Every `semanticsLabel`/`tooltip` must come from
`context.l10n` (the `require_l10n_for_semantic_labels` rule rejects literals), using
the `a11y` key prefix. A delete-confirmation bottom sheet must go through
`showAccessibleModal`; a confirmation **dialog** is a documented widget async
exception (guard with `if (!mounted) return;`).

**i18n (i18n.md).** Every new key lands in **both** `app_en.arb` and `app_es.arb`
(the `lint:dart:l10n-parity` gate fails otherwise) with `@key` metadata. The
"edited" marker and "Replying to {name}" string need ICU placeholders declared in
`@key`. Viewmodels never resolve strings — widgets call `context.l10n.*` in
`build()`.

**Theming (architecture.md §Colors and Theming).** The reply quoted-block accent
bar, background, and "edited" label must use `AppColors.*(context)` (theme-aware) —
no hardcoded `Colors.*` / hex — and must handle both the standard and immersive
(glass) bubble styles.

**Client testing (testing.md).** Tests target **ViewModels and repositories with
`ProviderContainer` + Mockito**, not widgets. So: test
`ConversationMessagesNotifier.deleteMessage/editMessage` (optimistic apply +
revert-on-failure + `ref.mounted` disposal) and `ChatRepository` cache invalidation.
The picker's own-message gating and quoted-block rendering are pure UI — verify
manually / via future golden tests, not unit tests.

**Server error handling (architecture.md §Error wrapping).** Internal failures use
`connecterr.Internal(ctx, op, err, kv...)` — bare `connect.NewError(CodeInternal,…)`
fails `check_codeinternal.js`. Authz uses explicit codes: unauthenticated →
`CodeUnauthenticated`, non-author → `CodePermissionDenied`, missing message →
`CodeNotFound`. Fail-fast (CLAUDE.md): no warn-and-continue.

**Server observability (observability.md).** Each new handler builds
`logging.LoggerWithContext(ctx).With("operation", "EditMessage"/"DeleteMessage",
"conversation_id", …, "message_id", …)`. `message_id` is the documented per-event
correlation field for chat. Authz failures log at INFO/WARN (user errors), system
failures at ERROR. When broadcasting the edit/delete event, thread the originating
RPC `ctx` into the bus so the fan-out log lines carry the originator's `request_id`
(observability.md §Streams). Any fire-and-forget goroutine uses `logging.GoSafe`.

**Server file org (architecture.md §3).** Keep `messages.go` under 1000 lines — put
`EditMessage`/`DeleteMessage` in `messages.go` if it stays small, else a new
`message_edits.go`. Reply is **not** a new RPC (no request-type reuse concern); it's
an optional `reply_to_message_id` field added to the existing `SendMessageRequest`.

## Potential Solutions

The three features split naturally. Delete and Edit are "mutate my own message";
Reply is "compose referencing another message." Two architectural decisions drive
the options: **(1) how the menu is presented**, and **(2) how reply is modeled in
the proto**.

### Option A: Augment the existing overlay picker (minimal, incremental)

- **Approach:** Keep the lightweight `Overlay`-based `ReactionPicker`. Add a second
  row beneath the emoji pill containing context-appropriate actions: **Reply**
  always; **Edit** + **Delete** only when `isCurrentUser`. Enable long-press on own
  messages (drop the `!isCurrentUser` gate, branch the menu contents instead). Add
  `HapticFeedback.mediumImpact()` at the top of `_showReactionPicker()`.
  Proto: add `edited_at_unix_sec` to `UserChatMessage`, add a `ReplyContext`
  sub-message (`reply_to_message_id` + denormalized `quoted_sender_id` /
  `quoted_text` / `quoted_media_id`). New RPCs `EditMessage`, `DeleteMessage`;
  reply reuses `SendMessage` with a new optional `reply_to_message_id` request field.
- **Pros:** Smallest diff; reuses the proven overlay + optimistic-update plumbing;
  no new full-screen surfaces; ships fastest; matches the issue's "augment the
  current reaction emoji menu" phrasing exactly.
- **Cons:** The overlay pill gets visually busier; an action row under emoji is less
  iOS-native than the iMessage full-context-menu the screenshot shows. Edit-in-place
  needs an editing affordance (inline field or modal) the overlay doesn't provide
  on its own.
- **Effort:** Medium (server small, client medium).

### Option B: Full iMessage-style context menu (matches the screenshot literally)

- **Approach:** Replace the overlay pill with an iOS-style context menu on
  long-press: dim the background, lift the pressed bubble, show the emoji reaction
  row on top and a vertical list of actions (Reply / Edit / Delete / Copy) below —
  i.e. reproduce the second screenshot. Same proto/RPC changes as Option A.
- **Pros:** Closest to the reference the user posted; room to add Copy/Report/etc.
  later; visually polished.
- **Cons:** Significantly more UI work (background scrim, hero-lift animation,
  positioning math, dark/immersive-mode theming for the chat's glass background);
  higher risk of regressions in the existing reaction flow; over-scopes what the
  maintainer asked for ("augment the current menu").
- **Effort:** Large (client), small/medium (server, same as A).

### Option C: Long-press menu (A) + swipe-to-reply fast-follow

- **Approach:** Option A for the menu, plus a `Dismissible`/`GestureDetector`
  horizontal-swipe on the bubble that triggers reply directly with a single haptic
  buzz (the WhatsApp/Signal gesture). Edit/Delete stay menu-only.
- **Pros:** Reply becomes the fast, muscle-memory gesture users expect; still small
  incremental server work; the swipe is additive and low-risk.
- **Cons:** Swipe gesture must not fight the message list's vertical scroll; extra
  client polish; arguably beyond the issue's explicit ask.
- **Effort:** Medium-large (client), small/medium (server).

## Recommendation

**Ship Option A first, with the proto/server work designed so Option B or C can be
layered on without rework.**

Rationale:
- The maintainer's words were "augment the current reaction emoji menu," which is
  precisely Option A. The iMessage screenshot is the user's *example of the concept*,
  not a spec to pixel-match.
- The data-model and optimistic-update plumbing is the long pole, and it is identical
  across all three options. Getting the proto fields (`edited_at_unix_sec`,
  `ReplyContext`, `reply_to_message_id`) and the three server paths right is the
  durable work; the menu presentation is swappable UI on top.
- Option A is the fastest path to closing the user's actual pain (can't edit/delete
  own messages), and de-risks the larger UI investment by validating the backend
  contract first.

Then treat **swipe-to-reply (C)** as a fast-follow once reply lands, and consider
the **full context menu (B)** later as a polish pass if the action row feels cramped.
The phasing below reflects this: backend + delete first (lowest risk, model already
scaffolded), then edit, then reply.

## Open Questions

- **Delete semantics: full soft-delete vs. tombstone ("This message was deleted")?**
  — Recommendation: **full soft-delete** (the message vanishes from history),
  reusing the existing `DeletedMetadata` filter. It is already built and tested,
  and for a small-community trust context a tombstone adds clutter without much
  value. A tombstone can be added later by *not* filtering deleted user messages
  and rendering a placeholder, but start with the simpler, already-working path.

- **Is there an edit time window (e.g. 15 min like iMessage), or unlimited?**
  — Recommendation: **unlimited edits** for v1, but **stamp `edited_at_unix_sec`**
  and render a subtle "edited" label (like WhatsApp). Trust context is small;
  a time window is friction without clear benefit. The timestamp leaves room to
  add a window later purely client-side.

- **Should an edit preserve original text / show edit history?**
  — Recommendation: **no history** for v1 — overwrite `text`, set
  `edited_at_unix_sec`, render "edited". Edit history is a heavier feature with
  privacy implications; defer unless requested.

- **Do edit/delete emit anything to other participants' live streams?**
  — Recommendation: **yes** — add `KindUserMessageUpdate` and `KindMessageDelete`
  event-bus kinds so open streams update in place (mirroring how reactions already
  broadcast). Without this, other viewers would see stale text until refetch.

- **Can you delete/edit a message that has reactions or replies pointing at it?**
  — Recommendation: **allow it.** On delete, the denormalized quote snippet already
  copied into any reply stays intact (that's why we denormalize). Reactions on a
  deleted message disappear with it. No referential blocking needed.

- **Does edit/delete need a new `CommunityEventType` (undo registry)?**
  — Recommendation: **no.** These are direct message mutations, not community-feed
  actions. Delete is inherently reversible via the soft-delete/restore field
  (`ClassificationRetentionRestore` semantics) and edit is reversible by re-editing;
  neither needs the snackbar/story undo plumbing. If we later decide deleted
  messages should be restorable from a "recently deleted" UI, revisit then. Confirm
  with maintainer since the registry test (`TestEveryEventTypeClassified`) is strict
  — but only fires if we add an enum value, which this plan does not.

- **Who can delete — only the author, or also community owners/moderators?**
  — Recommendation: **author only** for v1 (server-enforced: `sender_id == caller`).
  Moderation deletion is a separate concern; scope it out.

- **Edit affordance UX: inline-editable bubble or a modal/prefilled compose bar?**
  — Recommendation: **prefill the existing compose bar** with the message text and
  switch it to "edit mode" (a banner above the input: "Editing message · ✕"),
  reusing the `MentionTextController` and send button (relabeled "Save"). This
  reuses the most code and handles @mentions/media consistently. Avoids building a
  separate inline editor inside the bubble.

## Implementation Plan

### Phase 1: Proto + server contract (edit / delete / reply backend)
**Goal:** Land the durable data model and RPCs; the long pole that all UI options share.
**Tasks:**
- [ ] `proto/ripls/models/chat.proto`: add `optional int64 edited_at_unix_sec` to
      `UserChatMessage`; add a `ReplyContext` message (`reply_to_message_id`,
      denormalized `quoted_sender_id`, `quoted_text`, optional `quoted_media_id`)
      and an `optional ReplyContext reply_to` field on `UserChatMessage`. Place
      fields near semantically related ones per proto-field-order convention.
- [ ] `proto/ripls/api/chat_service.proto`: add `EditMessage(EditMessageRequest)
      returns (EditMessageResponse)` and `DeleteMessage(DeleteMessageRequest)
      returns (DeleteMessageResponse)`, each with its own dedicated request/response
      types (no reuse). Add `optional string reply_to_message_id` to the existing
      `SendMessageRequest` (reply reuses SendMessage). Mirror `edited_at`/`reply_to`
      onto the API message item type.
- [ ] `server/services/chat/` (keep `messages.go` < 1000 lines; else `message_edits.go`):
      implement `EditMessage` (authz: caller is sender, message is a `UserChatMessage`,
      not deleted; overwrite text, stamp `edited_at_unix_sec`) and `DeleteMessage`
      (authz: caller is sender; populate `DeletedMetadata`). In `SendMessage`, when
      `reply_to_message_id` is set, fetch the parent, validate same-conversation, and
      denormalize the quote snippet into `ReplyContext`. Use explicit Connect codes
      (`CodeUnauthenticated` / `CodePermissionDenied` / `CodeNotFound`) and
      `connecterr.Internal(ctx, op, err, kv...)` for storage failures — never bare
      `CodeInternal`. Fail-fast on every error (no warn-and-continue).
- [ ] Structured logging in each handler:
      `logging.LoggerWithContext(ctx).With("operation", "EditMessage"/"DeleteMessage",
      "conversation_id", convID, "message_id", msgID)`. Authz/user errors log at
      INFO/WARN; system failures at ERROR.
- [ ] `server/chat_event_bus/`: add `KindUserMessageUpdate` and `KindMessageDelete`;
      broadcast from the new handlers so live streams update in place. Thread the
      originating RPC `ctx` into the broadcast so fan-out log lines carry the
      originator's `request_id`; wrap any fire-and-forget goroutine in
      `logging.GoSafe`.
- [ ] `message_converters.go`: surface `edited_at` and `reply_to` in history/stream
      responses.
- [ ] Server tests (unit-first, table-driven; integration for the cross-component
      flow): edit/delete authz (non-author rejected with `CodePermissionDenied`),
      edit stamps timestamp, delete filtered from history (extend `soft_delete_test.go`),
      reply denormalization, event-bus broadcast assertions, and a `main_integration_test.go`
      round-trip. Run `npm run generate` then `npm run build` (and
      `go build -tags=integration ./...` since signatures change).
**Validates:** The data contract and permission model are correct independent of UI.

### Phase 2: Long-press menu + delete + haptic (client)
**Goal:** User can long-press their own message and delete it; haptic fires on activation.
**Tasks:**
- [ ] `chat_bubble.dart:154`: enable long-press on own messages (remove the
      `!isCurrentUser` gate; branch menu contents on `isCurrentUser`). Add
      `HapticFeedback.mediumImpact()` at the start of `_showReactionPicker()`.
- [ ] `reaction_picker.dart`: add an action row below the emoji pill — **Reply**
      always, **Edit** + **Delete** only when the message is the current user's.
      Build each action with the `Tappable`/`IconAction` accessibility primitive
      (never raw `GestureDetector`); every `semanticsLabel` from `context.l10n` with
      the `a11y` prefix. Wire new callbacks (`onReply`, `onEdit`, `onDelete`).
- [ ] `chat_service.dart`: add the `deleteMessage` RPC wrapper.
      `chat_repository.dart`: add `deleteMessage()` that calls the service, invalidates
      the conversation cache, and calls `dailyCacheInvalidationProvider.notify()`
      (mirroring existing chat mutations). Leave `UnreadCountRepository` untouched.
- [ ] `conversation_messages_notifier.dart`: add `deleteMessage()` that calls
      **`chatRepository.deleteMessage()`** (not the service), with optimistic removal
      via `state.copyWith` + revert-on-failure (the `addReaction` pattern). Guard every
      post-`await` state assignment with `if (!ref.mounted) return;`. Handle the
      incoming `KindMessageDelete` stream event to remove the message for other viewers.
- [ ] Delete confirmation: dialog (documented widget async exception — guard with
      `if (!mounted) return;`) or `showAccessibleModal` bottom sheet. l10n keys in
      **both** `app_en.arb` + `app_es.arb` with `@key` metadata (`chatActionDelete`,
      `chatActionEdit`, `chatActionReply`, `chatDeleteConfirm*`, plus `a11y*` labels).
- [ ] Client tests (`--reporter expanded`, `ProviderContainer` + Mockito): notifier
      `deleteMessage` optimistic-apply, revert-on-failure, and disposal-during-async;
      repository cache-invalidation + `dailyCacheInvalidationProvider` notify. Picker
      gating is pure UI — verify manually, not as a unit test.
**Validates:** The core user complaint (can't delete own message) is resolved; the
menu-augmentation pattern and stream-update handling work end to end.

### Phase 3: Edit (client)
**Goal:** User can edit their own message; others see the update live with an "edited" marker.
**Tasks:**
- [ ] Compose bar "edit mode": banner above input ("Editing · ✕"), prefill
      `MentionTextController` with current text, relabel send → "Save", route to
      `editMessage` instead of `sendMessage` while in edit mode. Edit-mode flag lives
      in `ConversationState` (Freezed `copyWith`), not local widget state.
- [ ] `chat_service.dart` (RPC wrapper) → `chat_repository.dart`
      (`editMessage()` + cache invalidation + `dailyCacheInvalidationProvider.notify()`)
      → `conversation_messages_notifier.dart` (`editMessage()` calling the **repository**,
      optimistic text swap via `state.copyWith` + revert-on-failure, `ref.mounted`
      guards). Handle `KindUserMessageUpdate` stream event.
- [ ] `chat_bubble.dart`: render a subtle "edited" label (theme-aware `AppColors`,
      both standard + immersive styles) when `edited_at` is set.
- [ ] l10n keys in both ARB files (`@key` metadata; ICU placeholder for "edited"
      if parameterized) + `a11y` labels. Client tests (`ProviderContainer` + Mockito):
      notifier `editMessage` optimistic/revert/disposal, repository invalidation.
**Validates:** Full edit round-trip including live propagation and the edited marker.

### Phase 4: Inline reply / quoting (client)
**Goal:** User can reply to any message; the reply renders a quoted preview block.
**Tasks:**
- [ ] Reply compose banner above input (quoted sender + snippet + ✕), reusing the
      edit-mode banner infrastructure from Phase 3; reply-target lives in
      `ConversationState`. On send, the notifier passes `reply_to_message_id` to
      `chatRepository.sendMessage(...)` (repository → service), keeping layering intact.
- [ ] `chat_bubble.dart`: render the `ReplyContext` quoted block inside the bubble —
      leading accent bar, sender name, truncated snippet (or media thumbnail), all via
      theme-aware `AppColors.*(context)`. Tapping it scrolls to / highlights the
      original message (WhatsApp/Signal styling). The tap target uses `Tappable` with
      an `a11y` `context.l10n` label.
- [ ] Handle immersive (glass) vs. standard bubble theming for the quote block.
- [ ] l10n in both ARB files: "Replying to {name}" (ICU `{name}` placeholder declared
      in `@key`) + quoted-message `a11y` label. Client tests (`ProviderContainer` +
      Mockito): notifier reply-target state + send-with-reply path. Quoted-block
      rendering is UI — manual / future golden, not a unit test.
**Validates:** Reply round-trip with correct quoted styling in both bubble modes.

### Phase 5 (optional fast-follow): Swipe-to-reply — tracked as #2260
**Goal:** WhatsApp/Signal swipe-right-to-reply gesture with a single haptic buzz.
**Note:** Split into its own issue (#2260) so Phases 1–4 ship the menu-based flow
first; this phase depends on the reply plumbing from Phase 4.
**Tasks:**
- [ ] Add a horizontal-swipe gesture on the bubble that triggers reply without the
      menu; single `HapticFeedback.mediumImpact()` on activation; ensure it does not
      conflict with vertical list scrolling.
- [ ] Tests / manual verification on iOS + Android.
**Validates:** The fast reply gesture users expect; confirms the gesture coexists
with scroll.

## Related Issues
- #2260 — swipe-to-reply gesture; fast-follow split out of this plan's Phase 5.
- #1028 (`1028-message-reactions.md`) — the long-press reaction picker this feature
  augments; foundational UI and optimistic-update patterns.
- #964 (`964-generalized-undo.md`) — undo/registry + `DeletedMetadata` soft-delete
  framework that delete reuses.
- #1188 (`1188-coalesce-system-messages.md`) / #2159 — system-message templating;
  context for how `ChatMessage` rendering and the event bus behave.
- #1124 (`1124-multi-photo-chat.md`) — media in chat messages; relevant to quoting
  media in replies and editing messages with attachments.
