# Issue #1193: Rethink plan tab UX: surface request/offer descriptions and reduce chat system message noise

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1193
**Filed:** 2026-04-20 | **Last activity:** 2026-04-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/experience_needs.md](../experience_needs.md)

## Audit Notes (2026-04-20)

This plan was audited against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), and [docs/server/architecture.md](../server/architecture.md). Changes made in-place:

1. **i18n (Phase 1 + Phase 2):** All new user-visible strings (reaction UI labels, any activity-card or reference-chip copy) must be added to both `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb` before running `npm run generate`. Widgets resolve via `context.l10n.*`; viewmodels never resolve strings. Added explicit tasks.
2. **Theming (Phase 1):** The new reaction badge on system messages must use `AppColors.*(context)` helpers — never `Colors.white`, `Colors.grey[*]`, or hardcoded hex. Added to the Phase 1 task list.
3. **Server architecture — library vs service-to-service (Phase 2):** Clarified that `InsertUserMessageOnBehalfOf` belongs in the shared `server/chat` **library** (alongside the existing `SystemMessageWriter` in [system_messages.go](../../server/chat/system_messages.go)), not as a cross-service dependency. The `server/services/experience` package imports from `server/chat` the same way it does today for `InsertSystemMessage`.
4. **Proto conventions (Phase 2):** Any new field on `UserChatMessage` (e.g. a reference to the related need/contribution) must use `optional` per the proto conventions. API and models protos must stay separate — if the reference is exposed to the client, it belongs in `proto/ripls/models/chat.proto` with a corresponding API-layer conversion in [proto/ripls/api/chat_service.proto](../../proto/ripls/api/chat_service.proto) mirror fields. Do not cross-import.
5. **Structured logging (Phase 2):** Every new server path must use `logging.LoggerWithContext(ctx)` with standard fields (`operation`, `experience_id`, `need_id`, `contribution_id`). No `log.Printf` / `fmt.Printf`.
6. **Fail-fast (Phase 2):** If the on-behalf-of user-message insertion fails, the mutation must return an error (or fall back to the system message path deterministically). No silent `warn-and-continue` — per CLAUDE.md error-handling rules.
7. **Spec doc drift:** When Phase 2 lands, update [docs/experience_needs.md](../experience_needs.md)'s Chat Integration table to reflect that add/edit mutations with a description post a user message (not a system message). Added as an explicit task.
8. **Self-message reaction gate (Phase 1):** The existing long-press gate in [message_list.dart:256](../../app/lib/presentation/widgets/chat/message_list.dart#L256) suppresses the picker for `isCurrentUser`. Same rule should apply to system messages that were emitted by the current user's own action. Flagged as an open question because the actor on a system message is not obviously attributable client-side — the `SystemChatMessage` has no `sender_id` in the model.
9. **Chat is stream-driven, not Stash-cached (Phase 2):** The new user message posted server-side will reach clients via the existing `broadcastMessage` stream path in the chat service (see `AddReaction`'s `s.broadcastMessage(...)` in [reactions.go:84-93](../../server/services/chat/reactions.go#L84-L93)). No `ExperienceRepository` or `ChatRepository` cache invalidation is required for the chat message itself. The `needs_and_contributions:{experienceId}` cache still needs its existing force-refresh after the need/contribution mutation; that is unchanged.
10. **Disposal safety tests:** Any edit to `conversation_view_model.dart` (an async `Notifier`) requires a disposal safety test per [docs/client/architecture.md §Disposal Testing](../client/architecture.md). Added to Phase 2 task list.

## Problem Statement

The Plan tab's Request/Offer feature ("needs" and "contributions") has two intertwined UX problems.

**Descriptions are stranded.** A Request's `note` and a Contribution's `description` are optional free-text fields that give meaningful context — "we need a cooler big enough for 20 drinks", "I'm bringing my vegan chili, no dairy". Today these only render inside the pill detail modal after a tap ([experience_needs_section.dart:165](../../app/lib/presentation/widgets/experience/experience_needs_section.dart#L165) passes note to the sheet, the pill itself shows title only). On the Plan tab the pills show only the title — so the context is effectively hidden. Inlining it on the pill would make the Plan tab a wall of text as soon as there are more than a handful of items.

**Chat system messages are noisy and inert.** Per [docs/experience_needs.md](../experience_needs.md) Chat Integration, every mutation (add/remove-need, add/remove-contribution, claim) fires a system message into the Experience conversation. Two problems: the chat floods in active experiences, and system messages render via a stripped-down path ([message_list.dart:120-126](../../app/lib/presentation/widgets/chat/message_list.dart#L120-L126)) that has no long-press / reaction UI. Participants naturally want to react ("🙏", "thanks!") to "Alice is bringing the ice" but can't.

**Important finding from codebase research:** the server's `AddReaction` / `RemoveReaction` RPCs [do not gate by message type](../../server/services/chat/reactions.go#L22-L100) — they already work on `SystemChatMessage`. The blocker is purely client-side: the Flutter chat renderer calls `_buildSystemLabel` for system messages and never wires the long-press gesture that exposes the reaction picker. This significantly shrinks the scope of option B below.

## Context from Comments

No comments on the issue yet — this plan is based solely on the issue description, the linked spec, and codebase research. Since there's no discussion, the Open Questions section should be treated as the alignment gate before implementation starts.

## Potential Solutions

The two halves can ship independently. The descriptions/comments work (A) and the chat-noise work (B) are decoupled — A is primarily a data-model + UI job, B is primarily a client UX job with optional server-side batching. A reviewer could reasonably approve one and defer the other.

### Option A1: Descriptions-as-comments — new `ExperienceNeedComment` entity

- **Approach:** Add an `ExperienceNeedComment` proto model keyed by need/contribution ID with `author_id`, `body`, `created_at`. Comments render in the detail modal as a scrollable list and post into the Experience chat as regular user messages authored by the commenter. Needs/contributions keep their existing `note` / `description` (authored at creation time, shown on the pill in the modal as the "opening statement"), and comments become a reply layer underneath.
- **Pros:** Clean data model; comments are first-class user messages in chat and therefore already get reactions + replies + normal authorship display. Per-need comment threads become possible later. Matches the issue author's "inclination: treat descriptions more like comments."
- **Cons:** Larger scope — new proto, new storage table, new RPCs (AddComment, ListComments, RemoveComment), new UI. Raises a chat/linkage question: how does a chat message render its "about this need" affinity? (Likely a lightweight reference chip in the bubble.)
- **Effort:** Large.

### Option A2: Descriptions-as-comments — reuse `note` / `description`, add chat mirroring

- **Approach:** Keep the existing `note` / `description` fields. When a need is added or a contribution is added/edited *with* a note/description, post the note body as a **user message** in the chat (authored by the proposer/contributor), separate from the system message. Pills on the Plan tab stay title-only; the modal shows the note; chat surfaces it where conversations naturally happen.
- **Pros:** No new entity, no new RPCs. Ships the "description visible in both modal and chat" behavior in a single pass. Because it's a user message, reactions and replies work for free.
- **Cons:** Only one description per need (no follow-up commentary). Editing the note on a contribution would need to edit-or-repost the linked chat message, which is fiddly. Doesn't fully realize the "comments" vision from the issue.
- **Effort:** Medium.

### Option B1: Reduce chat noise — enable reactions on system messages (client-only)

- **Approach:** Extend the client's system message renderer to support the same long-press → reaction picker flow as regular bubbles. Surface any existing reactions on system messages in a compact badge. Leave the server untouched (it already supports this).
- **Pros:** Unblocks reactions on Request/Offer activity with zero server work and zero proto changes. Directly addresses "users can't react to system messages." Low risk.
- **Cons:** Doesn't reduce the *volume* of system messages — only makes each one reactable. Replies are still not possible (chat proto has no `reply_to_id`; see section C of research).
- **Effort:** Small.

### Option B2: Reduce chat noise — cluster mutations into a single system message

- **Approach:** Introduce a short debounce/coalesce window on the server for needs system messages keyed by `(experience_id, actor_id)`. Within the window, collapse multiple adds ("Alice added: ice, napkins, cups") or an add+claim sequence into one message. Implemented either via a write-behind buffer, or via an update-last-message pattern that replaces the previous system message in-place if it's recent and from the same actor.
- **Pros:** Materially cuts chat noise without removing the activity signal. Complements B1 rather than replacing it.
- **Cons:** In-place update of a recent system message interacts awkwardly with streaming clients and reactions (if a reaction lands on the message mid-coalesce, does the coalesced message carry it?). Debouncing delays the message's appearance, which may feel broken for a user who just added the need. Adds complexity to `SystemMessageWriter`.
- **Effort:** Medium.

### Option B3: Reduce chat noise — convert needs activity to user messages

- **Approach:** Replace the system message path entirely for needs mutations. Instead of a `SystemChatMessage`, post a normal `UserChatMessage` authored by the actor, styled in the UI as a "[Alice] added a need: ice" activity card. This gives reactions + replies (if reply support ever lands) for free and makes authorship first-class.
- **Pros:** Eliminates the system-vs-user split for this feature. Unlimited future flexibility. Reactions work via the existing path.
- **Cons:** Semantic shift — "activity" messages start living in users' sent-message histories. May appear in notifications differently. Requires adding a structured "activity card" rendering path on the client to avoid these looking like the user typed "added a need: ice." Doesn't reduce noise (every action still = one chat entry) unless combined with B2.
- **Effort:** Medium-large.

## Recommendation

**Ship A2 + B1 first, then evaluate A1 and B2 based on user feedback.** Specifically:

1. **A2 for descriptions:** Keep the existing `note` / `description` fields. When a need is added or a contribution is added *with* a description, post the description body as a **user message** from the actor into the Experience chat, in addition to the existing system message (which keeps its one-line "Alice added a need: ice" summary). This gives the issue author's "descriptions visible in both modal and chat" without new proto/storage work, and reactions/replies work automatically because it's a user message.

2. **B1 for reactions:** Wire up the long-press → reaction picker on system messages in the client. Zero server work required (server already supports it). This lets participants react to "Alice added a need: ice" with 🙏 or 🎉 without any other change to the activity model.

Rationale for recommending A2 over A1: the research showed comments-as-entity is net-new data modeling and the issue is already broad. A2 delivers the visible behavior quickly; if users later want follow-up commentary threads (multiple comments per need), A1 can be built on top without invalidating A2. Rationale for recommending B1 over B2/B3: B1 is the minimum change that removes the "can't react" complaint, and noise can be addressed separately once we see whether B1 alone feels better.

This depends on whether the team considers "description shows up as a user message from the actor" acceptable — some products treat that as misleading authorship. See Open Questions below. If that's a no, fall back to A1.

## Decisions

All recommendations from the prior draft were accepted by the issue author on 2026-04-20. Captured here for traceability:

- **Authorship — user message vs styled activity card:** **Decided: post as a regular user message** (same bubble, same avatar) with a subtle attached reference chip linking back to the need/contribution. **Additional scope added by the author:** rework the add-need and add/edit-contribution sheets so the description entry affordance looks and feels like composing a chat comment. The goal is a tight visual + conceptual link between "I typed this here" and "it appears in chat as my comment under my avatar." Added as a new task under Phase 2 client work.
- **System message vs user message when description is present:** **Decided: replace.** If the note/description is non-empty, post only the user message; if empty, keep the existing system-message path. Two messages per action is worse noise than today.
- **Pill tap-to-open-modal behavior:** **Decided: unchanged.** Tap still opens the full detail modal; the modal remains the system of record for a need's full content. The value of this change is that the description *also* surfaces in chat.
- **Clustering (B2) priority:** **Decided: defer.** Ship Phase 1 + Phase 2 first, then re-evaluate based on observed chat noise.
- **Replies/threads on needs activity:** **Decided: out of scope.** Chat proto has no thread model; reactions (Phase 1) plus user-message descriptions (Phase 2) cover the "users want to respond" concern without a thread data model.
- **Existing stored system-message note snippets:** **Decided: leave in place unchanged.** This plan does not rewrite history; it only changes what new mutations post going forward.
- **Reactions on self-authored system messages:** **Decided: do not gate.** `SystemChatMessage` has no `sender_id`; adding one is out of scope. Reactions apply to all system messages regardless of actor. Revisit only if users complain.

## Implementation Plan

### Phase 1: Enable reactions on system messages (client-only)

**Goal:** Users can react to existing needs/contributions system messages with no server changes.
**Tasks:**
- [ ] In [app/lib/presentation/widgets/chat/message_list.dart](../../app/lib/presentation/widgets/chat/message_list.dart), refactor `_buildSystemLabel` so the label is wrapped in a GestureDetector that honors the same `onLongPress → _showReactionPicker` flow used by chat bubbles (`message_list.dart:256-261`). Decide up front whether self-authored system-message actions should also suppress the picker (mirrors the `!isCurrentUser` gate on chat bubbles) — see Open Questions; if yes, this requires knowing the actor, which `SystemChatMessage` does not currently expose.
- [ ] Display a `ReactionBadge` beneath the system label when `chatMessage.reactions.isNotEmpty` (mirror `message_list.dart:287-292`).
- [ ] Handle tap on the reaction badge to show the detail sheet (mirror `_showReactionDetailSheet`).
- [ ] **Theming:** Use `AppColors.*(context)` helpers for any new background, border, or text color introduced by the reaction surface on system messages. Do not use `Colors.white`, `Colors.grey[*]`, or hardcoded hex (per [docs/client/architecture.md §Colors and Theming](../client/architecture.md)).
- [ ] **i18n:** If the reaction detail sheet introduces any new user-visible strings (e.g. "React to activity" helper copy, accessibility labels), add them to both `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb` with `@key` metadata, then run `npm run generate`. Resolve via `context.l10n.*` in the widget — viewmodels never resolve strings (per [docs/client/i18n.md](../client/i18n.md)).
- [ ] Confirm `onReactionSelected` / `onReactionRemoved` callbacks in the conversation view model don't gate by message type — if they do, remove that gate.
- [ ] Widget test: long-press on a system message shows the picker; selecting an emoji invokes the callback with the system message's ID.
- [ ] Manual test on Android + iOS: confirm long-press works and streaming reaction updates render on system messages.

**Validates:** That reactions work end-to-end on system messages (server already supports, per [reactions.go:22-100](../../server/services/chat/reactions.go#L22-L100)) and that the existing `ReactionUpdate` streaming path carries the update through for `SystemChatMessage` IDs.

### Phase 2: Post description-as-user-message on needs/contributions mutations

**Goal:** When a participant adds a need *with a note* or adds/edits a contribution *with a description*, the text appears in chat as a user message from the actor (visible, reactable, replyable if replies ever land). Bare adds keep the existing system message.

**Server tasks:**
- [ ] **Library helper, not service-to-service.** Add `InsertUserMessageOnBehalfOf(ctx, convID, actorUserID, body, optionalRef)` to the shared `server/chat` library alongside the existing `SystemMessageWriter` in [server/chat/system_messages.go](../../server/chat/system_messages.go). Do not couple `experience` and `chat` services directly — follow the same library-import pattern used today for `InsertSystemMessage` (per [docs/server/architecture.md §Service Independence](../server/architecture.md)).
- [ ] In [server/services/experience/needs.go](../../server/services/experience/needs.go) `AddExperienceNeed` handler: if `request.note` is non-empty, call the new library helper to post a user chat message authored by the proposer with body = note; skip the system message. If `note` is empty, keep the existing system message path.
- [ ] Same pattern in [server/services/experience/contributions.go](../../server/services/experience/contributions.go) for `AddExperienceContribution` (use `description`), and in the edit handler so editing a description mirrors to chat (decide: new message vs in-place edit of the existing linked message — recommend new message, flagged as "edited").
- [ ] **Proto conventions.** If tagging the chat message with a reference to the source need/contribution is in scope, add `optional string experience_need_id` and `optional string experience_contribution_id` (or a single `optional` message holding both) to `UserChatMessage` in [proto/ripls/models/chat.proto](../../proto/ripls/models/chat.proto). Mirror exposure in [proto/ripls/api/chat_service.proto](../../proto/ripls/api/chat_service.proto). Never import across the `api` / `models` boundary; conversion lives in the chat service. Use `optional` per [CLAUDE.md proto field conventions](../../CLAUDE.md).
- [ ] **Structured logging.** Every new code path uses `logging.LoggerWithContext(ctx)` with fields `operation`, `experience_id`, `need_id` or `contribution_id`, and `actor_user_id`. No `log.Printf` / `fmt.Printf` (per [docs/server/observability](../server/architecture.md#observability)).
- [ ] **Fail-fast.** If the on-behalf-of user-message insertion fails, return an error from the mutation — do not silently fall back to the system message and continue. Per CLAUDE.md: "Every error should either be returned to the caller (preferred) [or] have an explicit comment explaining why it's safe to ignore."
- [ ] Update [server/services/experience/needs_system_messages_test.go](../../server/services/experience/needs_system_messages_test.go) and [needs_and_contributions_test.go](../../server/services/experience/needs_and_contributions_test.go) to cover: add-with-note → user message posted, no system message; add-without-note → system message posted, no user message; insertion failure → mutation returns the error.
- [ ] Unit tests for the new `InsertUserMessageOnBehalfOf` library helper in [server/chat/system_messages_test.go](../../server/chat/system_messages_test.go) or a new `server/chat/user_messages_test.go`.

**Client tasks:**
- [ ] **Comment-style description entry (new scope).** Rework the description field in `AddNeedSheet` and `AddContributionSheet` in [app/lib/presentation/widgets/experience/experience_needs_sheets.dart](../../app/lib/presentation/widgets/experience/experience_needs_sheets.dart) so it looks and feels like composing a chat comment. Specifically: placeholder copy matching the chat composer (e.g. "Add a comment…"), typography and input treatment aligned with the conversation input widget, and a primary action button styled to visually rhyme with "send." The goal is to make the connection between "I typed this description here" and "it appeared as my comment in chat" immediate and obvious. Cross-reference the existing chat composer (find it via `conversation_view_model.dart` / chat screen) to reuse tokens and avoid drift. Add ARB entries (`experienceAddNeedDescriptionPlaceholder`, `experienceAddContributionDescriptionPlaceholder`, `experienceDescriptionPostAction`, etc.) to both `app_en.arb` and `app_es.arb`.
- [ ] Update [app/lib/presentation/viewmodels/conversation_view_model.dart](../../app/lib/presentation/viewmodels/conversation_view_model.dart) and [app/lib/presentation/widgets/chat/message_list.dart](../../app/lib/presentation/widgets/chat/message_list.dart) to render the optional "about this need" chip when the streamed `UserChatMessage` carries `experience_need_id` or `experience_contribution_id`.
- [ ] **No cache changes.** Chat is stream-driven via the existing `broadcastMessage` path in the chat service — no `ExperienceRepository` or `ChatRepository` cache invalidation is needed for the chat message itself. The existing `needs_and_contributions:{experienceId}` force-refresh after the need/contribution mutation is unchanged (per [docs/client/caching.md](../client/caching.md)).
- [ ] **i18n.** Add ARB entries for the reference chip label(s) (e.g. `experienceChatAboutNeed`, `experienceChatAboutContribution`) to both `app_en.arb` and `app_es.arb` with placeholders for the need/contribution name. Resolve via `context.l10n.*` in the message bubble widget. Run `npm run generate` (per [docs/client/i18n.md](../client/i18n.md)).
- [ ] **Theming.** Any chip color must use `AppColors.*(context)`.
- [ ] **Disposal safety test.** If `conversation_view_model.dart` changes accept or process the new reference fields, add a disposal safety test per [docs/client/architecture.md §Disposal Testing](../client/architecture.md).
- [ ] Widget test for the chip rendering (present vs absent).

**Cross-cutting tasks:**
- [ ] Update [docs/experience_needs.md](../experience_needs.md)'s Chat Integration table to reflect that add/edit mutations *with* a description post a user message (not a system message), and that bare mutations keep the system-message path.
- [ ] Manual test: add a need with a note from the Plan tab → confirm the note appears in chat as a user message authored by the proposer, is reactable via Phase 1's long-press flow, and carries the reference chip linking back to the need.

**Validates:** That descriptions make it from the Plan tab's add-need flow into chat without an optimistic-update race; that the server library boundary (chat library, not service-to-service) holds; and that the reference chip correctly links back to the need.

### Phase 3: Re-evaluate noise and clustering based on real use

**Goal:** Decide whether chat noise is still a problem after Phase 1 + 2 shipped; if so, implement clustering (B2) or promote existing system messages to user messages for all actions (B3).
**Tasks:**
- [ ] Gather feedback (internal dogfood + early users) on whether the Plan tab chat still floods.
- [ ] If yes: prototype clustering in [server/chat/system_messages.go](../../server/chat/system_messages.go) with a short (~30s) actor-scoped coalesce window. Decide in-place-update vs replace-last-message based on how the streaming clients handle it (test both).
- [ ] Alternately, consider converting the residual system messages (for bare adds, removes, claims) to styled user-message activity cards for consistency with Phase 2.
- [ ] No tasks here are committed until Phases 1–2 are in users' hands.

**Validates:** That any noise-reduction work is grounded in observed usage rather than speculation.

## Related Issues

- No directly related issues found in the current repo — the needs feature is described as "Production-ready" in [docs/experience_needs.md](../experience_needs.md) so this is a follow-up refinement rather than a bug fix.
