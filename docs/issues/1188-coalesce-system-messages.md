# Issue #1188: Coalesce rapid system messages in chat (RSVP, future actions)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1188
**Filed:** 2026-04-18 | **Last activity:** 2026-04-20
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Audit Changelog (2026-04-20)

Reviewed against `docs/server/architecture.md`, `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`. Changes made to the original
plan:

- **Added** a cache-invalidation step for `ChatRepository` on the client side.
  The original plan only updated `ConversationNotifier` in-memory state; the
  cached `getConversationHistory` response (keyed `chat:conversation:{id}:history*`)
  would have held stale pre-coalesce content until TTL. Now Phase 2 calls
  `ChatRepository.refreshConversationHistory(conversationId)` from the stream
  handler on `SystemMessageUpdate`. ViewModel → Repository is the sanctioned
  direction per `docs/client/architecture.md`.
- **Sharpened** the `ParticipantIdToIsRead` recommendation: keep the actor's
  own entry as read (`pid == actorID`), matching the pattern already in
  `InsertUserMessageOnBehalfOf`. Bulk-resetting would have re-marked the
  actor's own card unread.
- **Picked a concrete query strategy** for coalesce lookup (was an open
  question): use `QueryByFields({conversation_id, system_message_actor_id})`
  then filter in Go by action family and time window. Adds a `LIMIT`-style
  safeguard so long conversations don't fetch unbounded rows. Also documents
  why per-enum `QueryByFields` loops are not acceptable (N+1).
- **Added** explicit structured-logging guidance for the new coalesce
  branches (`logging.LoggerWithContext`). Consistent with
  `docs/server/architecture.md` observability section.
- **Flagged** the `LastMessageAtUnixSec` re-bump semantics for inbox
  ordering.
- **Flagged** that no user-visible strings are added (server composes the
  text), so `docs/client/i18n.md` has no impact here — but noted so a
  reviewer knows it was considered.
- **No caching changes on the client** beyond the invalidation call above.
  Messages are held in `ConversationState` (Riverpod) rather than Stash,
  per the existing pattern, so no additional repository methods are
  required.

## Problem Statement

When a user rapidly repeats or revises a chat-posting action (today: RSVP;
soon: experience need/contribution actions from #1186), each change posts a
separate system card. Chat fills with near-duplicate entries:

```
[Gary] RSVP YES      — 2:01 PM
[Gary] RSVP MAYBE    — 2:02 PM
[Gary] RSVP YES      — 2:03 PM
```

This is noise. It buries real conversation, inflates unread counts, and makes
the conversation timeline hard to read. It is a generic problem across every
action type that `SystemMessageWriter.InsertSystemMessage` handles, so the fix
belongs in the writer, not in each call site.

Desired end state: within a short window (~5 min) the latest revision by the
same actor, for the same action family, replaces the prior card in place —
same message id, updated text and timestamp, single broadcast.

## Context from Comments

No comments on the issue. The description is detailed and actionable, but a
few decisions are not pinned down (action-family grouping, window
configurability, client update semantics). Those are flagged in Open
Questions below.

## Codebase context

- [server/chat/system_messages.go:44](../../server/chat/system_messages.go#L44)
  — `InsertSystemMessage` is the single entry point. It fetches the
  conversation, initializes per-participant read status, inserts a
  `ChatMessage` with a `SystemChatMessage` oneof payload, bumps
  `LastMessageAtUnixSec`, and broadcasts via
  `MessageBroadcaster.BroadcastMessage`. No coalescing today.
- [server/chat/system_messages.go:247](../../server/chat/system_messages.go#L247)
  — `IsCreationAnchorMessage` enumerates actions that anchor info cards and
  must never collapse (EXPERIENCE_CREATED, REQUEST_CREATED, GIVEAWAY_SHARED,
  LOAN_SHARED).
- [server/services/experience/rsvp.go:212-234](../../server/services/experience/rsvp.go#L212-L234)
  — RSVP writes one of three actions (RSVP_YES/MAYBE/NO) based on the incoming
  intention, guarded by `intentionChanged` so repeat-submissions of the same
  value already skip side effects. Rapid *changes* between YES/MAYBE/NO still
  each post a card.
- [server/storage/protosql_schema.go:35-69](../../server/storage/protosql_schema.go#L35-L69)
  — proto fields flatten to columns: `ChatMessage.system_message.action`
  becomes the `system_message_action` column, `system_message.actor_id`
  becomes `system_message_actor_id`. `QueryByFields` can filter on these
  directly.
- [server/chat/system_messages.go:115-127](../../server/chat/system_messages.go#L115-L127)
  — `BroadcastMessage` sends a `StreamMessagesResponse` with a
  `SystemMessage` payload. Today every broadcast is treated as a new message
  by the client.
- [app/lib/presentation/viewmodels/conversation_view_model.dart:306-421](../../app/lib/presentation/viewmodels/conversation_view_model.dart#L306-L421)
  — the stream handler dedupes on `_messageIds.contains(messageId)`. A
  re-broadcast of an existing message id is currently *discarded* — it will
  not update the UI in place. Any server-side coalescing that reuses message
  ids requires a matching client-side update path (see Option A below).
- `ReactionUpdate` already exists as a dedicated oneof variant on
  `StreamMessagesResponse` (proto/ripls/api/chat_service.proto:78,91-98). It
  is the precedent for "lightweight update to an existing message" and the
  right model to copy.

## Potential Solutions

### Option A: Coalesce in `SystemMessageWriter`, broadcast via a new `SystemMessageUpdate` variant (recommended)

- **Approach:**
  1. Introduce an `actionFamily(action)` helper in `server/chat/` that maps
     enum values to coalescing families (e.g., RSVP_YES/MAYBE/NO → `"rsvp"`,
     experience need/contribution actions each map to their own family,
     creation anchors and one-shot events like APPROVED/CANCELLED/COMPLETED
     return the empty family meaning "never coalesce"). The set of
     non-coalescing actions **must be the superset of**
     `IsCreationAnchorMessage` in [server/chat/system_messages.go:247](../../server/chat/system_messages.go#L247)
     — add a shared helper rather than maintain two parallel lists.
  2. In `InsertSystemMessage`, before inserting: if the action's family is
     non-empty, run
     `QueryByFields({conversation_id, system_message_actor_id})` against
     `ChatMessage` (both are flattened columns today), then in Go filter
     to rows whose `SystemMessage.Action` is in the family and whose
     `SentAtUnixSec` is within the window. Pick the most recent match.
     Do **not** run `QueryByFields` once per enum value in the family —
     that's the `SQL Efficiency` N+1 pattern called out in CLAUDE.md.
     Apply a safety `head/limit` by sorting on `sent_at_unix_sec DESC` in
     Go and breaking after the first in-window match; a long conversation
     should still only scan recent user/actor rows, not the whole table.
     If profiling shows unacceptable scan cost, escalate to a dedicated
     `system_message_coalesce_key` proto field (see Phase 3).
  3. If a match is found: update the existing `ChatMessage` — overwrite
     `SystemChatMessage.Action`, `SystemChatMessage.Description`, and
     `ChatMessage.SentAtUnixSec`. For `ParticipantIdToIsRead`: set each
     participant's flag to `pid == actorID` so the actor's own card stays
     read and everyone else sees a fresh unread (mirrors
     [server/chat/system_messages.go:178-183](../../server/chat/system_messages.go#L178-L183)).
     Persist via `storage.Update`. Then bump
     `conversation.LastMessageAtUnixSec = now` (same as the insert path)
     so inbox sort order reflects the update. Otherwise fall through to
     the existing insert path.
  4. Add a new `SystemMessageUpdate` message on `StreamMessagesResponse`
     carrying `message_id`, `sent_at_unix_sec`, `actor`, `action`, and
     `description`. On coalesce, broadcast this variant instead of a fresh
     `SystemMessage`. On fresh insert, broadcast `SystemMessage` as today.
  5. All new server branches use `logging.LoggerWithContext(ctx)` with the
     operation name (`"CoalesceSystemMessage"`) and the existing fields
     (`conversation_id`, `actor_id`, `action`), per
     [docs/server/architecture.md](../server/architecture.md#observability).
  6. Client: in `_handleMessage` in `conversation_view_model.dart`, add a
     branch on `hasSystemMessageUpdate()` that finds the existing message
     by id and rewrites its fields in place (mirrors the existing
     `_handleReactionUpdate` branch). After updating state, call
     `ref.read(chatRepositoryProvider).refreshConversationHistory(conversationId)`
     so the cached `getConversationHistory` response for this conversation
     is dropped — otherwise a later navigate-away-and-back would re-read
     stale pre-coalesce content from Stash. Do **not** change the normal
     `_messageIds` dedupe path.
- **Pros:** Single choke point; every current and future caller benefits
  automatically; client change is narrow and follows an existing precedent;
  action-family mapping lives with the writer so domain teams don't
  reimplement it; wire-compatible proto change (adding a new oneof variant).
- **Cons:** Requires a proto change and coordinated client update; editing
  stored messages means conversation history no longer reflects the raw
  sequence of actions (acceptable — that's the point); unread-counts need
  care (resetting `ParticipantIdToIsRead` on update is the right call but
  must be tested against notification/badge paths).
- **Effort:** Medium.

### Option B: Client-only coalescing

- **Approach:** Server keeps posting a card per action. Flutter collapses
  consecutive system messages from the same actor with the same action
  family, within a window, into one rendered card — latest wins visually.
  No server or proto changes.
- **Pros:** No proto or schema impact; quick to ship; conversation history
  retains full fidelity on disk.
- **Cons:** The storage and wire still carry the spam — every other
  consumer (unread counts, web, potential notifications, summaries,
  exports) has to re-implement the collapse or re-inherit the noise; the
  Dart viewmodel grows another special case; doesn't address the core
  complaint that chat *contains* spammy cards (it just hides them).
- **Effort:** Small, but shallow — the noise isn't actually gone.

### Option C: Pre-check in each action handler

- **Approach:** In each caller (`RSVPToExperience`, future experience need
  RPCs, etc.), query for a recent matching system message before calling
  `InsertSystemMessage`. If found, update it; otherwise insert.
- **Pros:** No change to the shared library; each caller keeps full
  control over coalescing semantics.
- **Cons:** Duplicates logic at every call site — forgetting to add it in
  a new handler silently brings back the noise; the issue itself asks for
  a "generic capability in `SystemMessageWriter`"; higher churn and
  regression risk.
- **Effort:** Medium and spread across many files.

## Recommendation

**Option A.** The issue is explicit that this should be generic in the
writer, and Option A puts the logic at the single point that already owns
system-message insertion. The proto change is small (one new oneof variant,
modeled directly on `ReactionUpdate`), the client change is local
(one new handler branch), and the action-family table is easy to extend as
#1186 lands. Options B and C both leave persistent noise in the
durable conversation history.

## Open Questions

- **Should RSVP_YES → RSVP_NO collapse into a single card, or only
  identical-enum repeats?** — Recommendation: collapse by *family*, so all
  RSVP transitions within the window become a single card whose action
  reflects the latest intention. This matches the example in the issue
  body ("Gary RSVP YES → MAYBE → YES" ending as a single "RSVP YES"
  card). For experience needs, need-added and need-removed map to the
  same family so rapid add/undo doesn't leave orphan cards; need-claimed
  gets its own family since it's a different semantic action.
- **Window duration: hard-coded or configurable?** — Recommendation: a
  single package-level constant in `server/chat/` (default 5 minutes),
  overridable via a `SystemMessageWriter` option. Don't expose per-action
  tuning yet — YAGNI; revisit only if a family genuinely needs a
  different window.
- **Should the coalesced update reset `ParticipantIdToIsRead` so the card
  re-appears unread for other participants?** — Recommendation: yes for
  everyone *except* the actor. Use the
  `InsertUserMessageOnBehalfOf`-style initializer:
  `participantReadStatus[pid] = (pid == actorID)`. The actor obviously
  knows about their own action and shouldn't see their own toggle as
  "unread." If further testing shows even that is too noisy (e.g., a
  host watching three people toggle RSVP over five minutes), switch to
  "only mark unread if the *action enum* actually changed, not just the
  text/timestamp."
- **Inbox ordering after coalesce (new question surfaced in audit):**
  re-bumping `conversation.LastMessageAtUnixSec` on every coalesce will
  re-promote the conversation in inbox sort order even though the user
  is only toggling. — Recommendation: accept this for Phase 1/2; the
  coalesced card *is* genuinely the latest activity in that
  conversation, and anything else would diverge from the existing
  "last message bumps inbox" invariant. Revisit only if the daily/inbox
  churn becomes a user complaint.
- **How should unread counts handle an update that reuses a message
  id?** — Recommendation: existing unread counters iterate messages and
  check `ParticipantIdToIsRead`; resetting that map on update covers it
  without a counter change. Verify with a unit test in
  `server/services/chat/unread_counts.go` paths.
- **Should creation anchors (EXPERIENCE_CREATED etc.) coalesce?** —
  Recommendation: no. `actionFamily()` returns empty string for every
  action in `IsCreationAnchorMessage`, guaranteeing they insert a new
  message every time (which also means they shouldn't be posted twice
  in the first place — that's a separate invariant).

## Implementation Plan

### Phase 1: Server-side coalescing primitive, proven with RSVP

**Goal:** Rapid RSVP changes update a single card in place on the server,
with broadcast still arriving at existing clients as a normal
`SystemMessage` (no client change yet — they'll just see the latest card
appear as a "new" message, which is acceptable as a stepping stone).

**Tasks:**
- [ ] Add `actionFamily(models.ChatSystemAction) string` in
      `server/chat/system_messages.go`; map RSVP_YES/MAYBE/NO → `"rsvp"`;
      return `""` for everything else (including all creation anchors).
      Make `IsCreationAnchorMessage` delegate to this helper so there is
      one authoritative list.
- [ ] Add `coalesceWindowSec` package constant (default 300).
- [ ] In `InsertSystemMessage`, before the insert: if `actionFamily(action)
      != ""`, call `QueryByFields` with `conversation_id` and
      `system_message_actor_id` (single query — no N+1 per enum), then in
      Go filter to the action family and window and pick the most recent
      match. On hit, update in place (see Approach §3 for the read-status
      rule and `LastMessageAtUnixSec` bump) and broadcast; on miss, fall
      through to the existing insert path.
- [ ] Use `logging.LoggerWithContext(ctx)` with `operation="CoalesceSystemMessage"`
      on both hit and miss branches, matching
      [docs/server/logging.md](../server/logging.md) conventions.
- [ ] Unit tests in `server/chat/system_messages_test.go`:
      YES→MAYBE→YES coalesces to one message;
      outside the window produces a second message;
      different actors never coalesce;
      a non-coalescing action (e.g., APPROVED) always inserts;
      `ParticipantIdToIsRead` on coalesce marks others unread but keeps
      the actor read;
      `conversation.LastMessageAtUnixSec` advances on coalesce.
- [ ] Regression test in `server/services/experience/system_messages_test.go`:
      three rapid RSVP changes produce exactly one `ChatMessage` row with
      the latest action.
- [ ] Add an `AssertMaxQueries` check around a coalesce call in a test to
      confirm the coalesce path is O(1) queries per insert (per the SQL
      efficiency rules in CLAUDE.md).
- [ ] Run `npm run build` and `go test ./server/...`.

**Validates:** Coalescing works end-to-end for the current noisy case;
persistence and unread mechanics are correct; no regression in anchor
messages.

### Phase 2: Dedicated `SystemMessageUpdate` broadcast + client in-place rendering

**Goal:** When the server coalesces, active clients update the existing
card in place (same id, new text/timestamp) instead of appending a fresh
card that looks visually duplicate.

**Tasks:**
- [ ] Add `SystemMessageUpdate` message in
      `proto/ripls/api/chat_service.proto` with `message_id`,
      `sent_at_unix_sec`, `actor`, `action`, `description`; add it as a
      new variant on the `StreamMessagesResponse` oneof (next free tag).
- [ ] `npm run generate` and fix any regeneration fallout.
- [ ] In `InsertSystemMessage`, on coalesce path broadcast
      `SystemMessageUpdate`; on insert path keep broadcasting
      `SystemMessage`.
- [ ] In
      [app/lib/presentation/viewmodels/conversation_view_model.dart](../../app/lib/presentation/viewmodels/conversation_view_model.dart):
      in `_handleMessage`, add a `hasSystemMessageUpdate()` branch that
      finds the message by `messageId` and rewrites its
      `systemMessage.action`, `description`, and `sentAtUnixSec` in place;
      do *not* remove/re-add the id from `_messageIds`. After mutating
      state, call
      `ref.read(chatRepositoryProvider).refreshConversationHistory(conversationId)`
      to evict the stale cached `getConversationHistory` response. Keep
      all business logic in the ViewModel — no Stash access from widgets
      (per [docs/client/architecture.md](../client/architecture.md) and
      [docs/client/caching.md](../client/caching.md)).
- [ ] No new user-visible strings are added (the server composes text
      for all RSVP and experience-coord actions today); confirm with a
      quick `grep` over `app/lib/l10n/*.arb` that no new l10n keys are
      needed. See [docs/client/i18n.md](../client/i18n.md) for the rule
      "viewmodels never resolve strings" — we comply because this
      pipeline never touches localized text on the client.
- [ ] Unit test the ViewModel handler: feed a `StreamMessagesResponse`
      with an initial `SystemMessage`, then one with
      `SystemMessageUpdate` for the same id, and assert
      (a) `state.messages.length == 1`, (b) the field values updated,
      (c) `refreshConversationHistory` was called on a mocked
      `ChatRepository`. Add a disposal test per
      `docs/client/architecture.md` §Disposal Testing.
- [ ] Manual validation on simulator via `/ripls-app-logs`: RSVP
      YES→MAYBE→YES while the conversation is open and confirm a single
      card updates, not three cards appearing.

**Validates:** The client pathway matches the server pathway; users see a
live-updating single card instead of a newly-spawned duplicate; the
proto change is wire-compatible (old clients ignore unknown oneof
variants — verify this in Dart by checking the proto3 default behavior).

### Phase 3: Extend coalescing to experience collaborative list actions (ties to #1186)

**Goal:** Once #1186 has landed the experience-need/contribution action
writers, extend the coalescing table so rapid add/remove or repeated
claims collapse the same way RSVP does.

**Tasks:**
- [ ] In `actionFamily`, map:
      `EXPERIENCE_NEED_ADDED` + `EXPERIENCE_NEED_REMOVED` → `"exp_need"`
      per (conversation, actor, need id) — note this needs an extra
      grouping key beyond actor (the specific need). Recommend adding
      `optional string coalesce_key = 4;` to
      [proto/ripls/models/chat.proto](../../proto/ripls/models/chat.proto)
      `SystemChatMessage`, populated by the writer from the call site
      (e.g., the need id), and include it in the coalesce `QueryByFields`
      lookup. This keeps the flattened column single-value and avoids
      any per-enum fan-out. Proto3 `optional` is wire-compatible and
      aligns with the "use `optional` for conditionally-present fields"
      convention in CLAUDE.md. Update
      [docs/proto_conventions.md](../proto_conventions.md) if the field
      introduces a new pattern worth documenting.
- [ ] Decide on the coalesce-key question above (flagged to the #1186
      author) and implement accordingly.
- [ ] `EXPERIENCE_CONTRIBUTION_ADDED` + `EXPERIENCE_CONTRIBUTION_REMOVED`
      → `"exp_contribution"` with the same coalesce-key treatment.
- [ ] `EXPERIENCE_NEED_CLAIMED` → `"exp_need_claim"` (own family — a
      claim is semantically distinct from proposing a need).
- [ ] Extend the experience-service tests to assert coalescing holds for
      rapid add/undo and rapid claim toggles.

**Validates:** The primitive generalises as designed; the "generic
capability" promise of the issue is real, not just RSVP-shaped.

## Related Issues

- [#1186](https://github.com/Type2Fun-Foundation/ripls/issues/1186) — Experience collaborative lists. Depends on Phase 1 and Phase 2 of this plan; Phase 3 of this plan depends on #1186 landing its action writers.
- [#1193](https://github.com/Type2Fun-Foundation/ripls/issues/1193) — Plan tab descriptions chat noise (related chat-noise theme).
- [#920](https://github.com/Type2Fun-Foundation/ripls/issues/920) — RSVP realtime (context for the current RSVP → chat pathway).
