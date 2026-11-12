# Issue #2290: Seed entity conversation with creation description as the first comment

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2290
**Filed:** 2026-06-04 | **Last activity:** 2026-06-04
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [docs/client/create.md](../client/create.md)

> **Audit note (ripls-audit):** Reviewed against `docs/server/architecture.md` and
> `docs/server/observability.md` (client caching/i18n/a11y/testing docs are
> largely N/A — this is a server-only change with no client work). The plan is
> compliant with the service-independence and shared-library principles. Audit
> tightened four things: (1) confirmed `InsertUserMessageOnBehalfOf` already
> publishes to `chat_event_bus` with push suppressed, so live viewers see the
> seeded comment and no spurious push fires — this removes a latent risk;
> (2) added structured-logging field requirements (notably `target_request_id`,
> **not** `request_id`, for the request path); (3) pinned the helper's home to
> the `server/chat` shared library; (4) added an isolated unit test for the
> helper and a comment-count interaction check. Changes are folded into the
> sections below and marked **[audit]**.

## Problem Statement

When a user creates an entity through the Unified Create flow — Event
(Experience), Item (Gear), or Request — they author a **description** during
creation. Today that description is stored as a field on the entity and rendered
in the entity's "info" surfaces, but it never appears in the entity's
**conversation thread**. The thread instead opens with only a *system* anchor
message ("X created an event: {name}", "X created a request: '{title}'", "X
shared {gear} for giveaway/loan") whose template uses only the entity *name/title* —
not the description.

The goal: after an entity is created and its conversation is first established,
post the creator's description as the **first user comment** in that
conversation, attributed to the creator. This gives every reader immediate
context and a natural anchor for replies.

This turns out to be a small, well-precedented server-side change. The codebase
already has the exact helper and pattern for it
([`server/planning/chat.go`](../../server/planning/chat.go) posts a need's note
as a user message via `InsertUserMessageOnBehalfOf`), so we are extending an
existing pattern rather than inventing one.

### Key correction to the issue's framing

The issue describes the save paths as `saveExperience` / `saveGear` /
`submitRequest`. That is the *client* vocabulary. On the server, the
conversation does **not** exist at `SaveExperience` / `SaveGear` time — those
RPCs only persist the entity with an empty `conversation_id`. The conversation
is created lazily:

| Entity | Conversation created in | File |
|--------|------------------------|------|
| Experience | `ShareExperience` | [server/services/experience/share.go:81-101](../../server/services/experience/share.go#L81-L101) |
| Gear | `ShareGear` | [server/services/community/gear_sharing.go:262-278](../../server/services/community/gear_sharing.go#L262-L278) |
| Request | `SubmitRequest` (submit *is* the share for requests) | [server/services/request/lifecycle.go:105-122](../../server/services/request/lifecycle.go#L105-L122) |

So the first comment must be seeded in the **Share** handlers (Submit for
requests), not the Save handlers. This is also where the existing anchor system
messages are emitted, gated by an "is this conversation new?" flag — which is
exactly the gate we need to satisfy the "no duplicate on re-share" acceptance
criterion.

## Context from Comments

**The issue has no comments.** It was filed and has had no discussion. The
analysis below resolves the four open questions the issue itself raised; those
recommendations should be confirmed before/while implementing, but none block a
first pass.

## Existing Architecture (research findings)

**Conversation model.** Conversations are `ChatConversation`
([proto/ripls/models/chat.proto](../../proto/ripls/models/chat.proto)) scoped to
an entity via `ConversationTopic` (a oneof over `experience_id`, `gear_id`,
`request_id`, etc.). Messages are `ChatMessage`, a oneof of
`UserChatMessage` (sender + text + media + optional need/contribution refs) and
`SystemChatMessage` (actor + `ChatSystemAction` + description).

**Anchor messages today.** On first conversation creation each Share handler
emits a *system* anchor message via
`systemMessageWriter.InsertLocalized(...)`:
- Experience → `CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED`, gated by
  `isNewConversation` ([share.go:116-128](../../server/services/experience/share.go#L116-L128)).
- Request → `CHAT_SYSTEM_ACTION_REQUEST_CREATED`
  ([lifecycle.go:144-151](../../server/services/request/lifecycle.go#L144-L151)).
  (Note: `SubmitRequest` always creates a fresh conversation, so no explicit
  "new" flag is needed — but see Open Questions on the request re-share path.)
- Gear → `CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED` / `..._LOAN_SHARED`, gated by
  `conversationCreated` ([gear_sharing.go:296-308](../../server/services/community/gear_sharing.go#L296-L308)).

These templates use only the entity name/title — the description is **not**
shown in chat anywhere today, so there is no existing duplication.

**The helper we need already exists.**
`SystemMessageWriter.InsertUserMessageOnBehalfOf(ctx, conversationID, actorID, text, opts)`
([system_messages.go:413](../../server/chat/system_messages.go#L413)) posts a
user-authored message on behalf of an actor. It validates that `text != ""`,
initializes per-participant read status (sender = read), updates the
conversation's `LastMessageAtUnixSec`, and **publishes to `chat_event_bus`** with
`WithOnBehalfOf(true)` ([system_messages.go:482-489](../../server/chat/system_messages.go#L482-L489)).
The precedent for "post a description/note as a user message" is
[`PostNeedAdded`](../../server/planning/chat.go#L15-L48), which calls exactly
this helper when a need carries a note.

**[audit] Live delivery + no push, confirmed.** Because the helper publishes on
the chat bus, anyone with the conversation open via `StreamMessages` sees the
seeded comment in real time — no reload needed. And `WithOnBehalfOf(true)`
suppresses the push notification, which is exactly what we want for an
auto-seeded message (the entity's own creation-notification path already
notifies). This matches the **Domain Event Buses** pattern in
[docs/server/architecture.md](../server/architecture.md#pattern-4-domain-event-buses)
and means **no new bus wiring** is required.

**[audit] i18n: do NOT localize the first comment.** The seeded comment is the
user's own description text — it must go through `InsertUserMessageOnBehalfOf`
(a `UserChatMessage`), **not** through `InsertLocalized` / a `LocalizedMessage`
template like the anchor messages do. User-authored content is never localized,
and the "Event vs Experience" terminology rule does not apply (that governs
app-generated copy, not user text).

**No new enum values.** A first comment is a `UserChatMessage`, not a system
action and not a `CommunityEvent`. Posting it requires **no** new
`ChatSystemAction` and **no** new `CommunityEventType`, so the
`TestEveryEventTypeClassified` / `server/undo/registry.go` gate does **not**
apply. Description fields are plain `string` on all three models
([experience.proto:32](../../proto/ripls/models/experience.proto#L32),
[gear.proto:29](../../proto/ripls/models/gear.proto#L29),
[request.proto:39](../../proto/ripls/models/request.proto#L39)) — so the
empty-skip check is `if desc != ""`, not a nil check.

## Potential Solutions

### Option A: Server-side, in each Share handler, right after the anchor message (recommended)

- **Approach:** In each of the three Share/Submit handlers, immediately after
  the existing anchor `InsertLocalized` call and **inside the same
  new-conversation gate**, call `InsertUserMessageOnBehalfOf` with the entity's
  description, skipping when the description is empty. The in-memory entity
  struct (`expStored` / `gear` / `request`) already holds `.Description` and the
  `conversationID` + `authInfo.UserID` are in scope at that exact point. Add a
  shared helper to avoid copy-pasting the empty-check + error-handling three
  times. **[audit] Home the helper in the `server/chat` shared library** (e.g.
  `chat.PostCreationDescription(...)`, a free function taking a
  `*chat.SystemMessageWriter`), **not** on any one service — the three call
  sites live in three different service packages (`experience`, `community`,
  `request`), so per the "shared functionality → library" and
  "service-independence" principles in
  [docs/server/architecture.md](../server/architecture.md#core-design-principles)
  it must not live on a service. This mirrors where `SystemMessageWriter` and
  the `PostNeed*` helpers already sit.
- **Pros:** Authoritative — the comment exists regardless of client version or
  whether the client crashes after save. Single, well-defined insertion point
  per type. Naturally deduped: the new-conversation gate guarantees one comment
  even when an entity is later re-shared into more communities. Reuses an
  existing, tested helper and an established precedent. No proto/enum/migration
  churn.
- **Cons:** Three near-identical call sites (mitigated by the shared helper).
  Ordering of the anchor (system) message vs. the description (user) message
  within the same `clock.UnixSec(ctx)` tick must be deterministic so the anchor
  stays first (see Open Questions).
- **Effort:** Small.

### Option B: Server-side, emit description inside a new `SystemChatMessage` field

- **Approach:** Extend the anchor `SystemChatMessage` to carry the description
  and render it as part of the anchor card, rather than as a separate user
  comment.
- **Pros:** Single message; no ordering question; visually "owned" by the
  creation event.
- **Cons:** Does **not** satisfy the issue's intent — the description should be
  the creator's **first comment** (a user message people can reply to / react
  to), not a styled system banner. Requires proto changes and client rendering
  work. More invasive for a worse fit.
- **Effort:** Medium.

### Option C: Client-side, after save, call `ChatService.sendMessage`

- **Approach:** After `handlePostCreation`, the client posts the description to
  the new conversation via the existing
  [`ChatService.sendMessage`](../../app/lib/services/chat_service.dart#L39).
- **Pros:** No server change; reuses the public send path (mentions, etc.).
- **Cons:** Not authoritative — lost if the client drops between save and post,
  or on older clients. Requires the client to know the conversation ID
  immediately (Share responses don't all return it today). Would fire a push
  notification (sendMessage is the full RPC path) — undesirable for an
  auto-seeded comment. Harder to dedupe across the multi-community share loop.
- **Effort:** Medium, and worse correctness.

## Recommendation

**Option A.** It directly matches the issue's intent ("first **comment**"),
reuses the exact helper (`InsertUserMessageOnBehalfOf`) and precedent
(`PostNeedAdded`) already in the codebase, is authoritative on the server,
dedupes for free via the existing new-conversation gate, delivers live over the
chat bus while sending no push (both confirmed in the helper — see
**[audit]** above), and needs no proto, enum, or DB migration. Factor the
empty-check + insert + warn-don't-fail into one shared helper in `server/chat`
so all three sites stay consistent.

## Open Questions

- **Anchor-first ordering.** The anchor (system) message and the description
  (user) message will share the same `SentAtUnixSec` tick. Does the
  conversation history sort stably so the anchor renders before the first
  comment? — **Recommendation:** Confirm the history query's secondary sort
  (likely message ID / insertion order). If it isn't deterministic, insert the
  description with the anchor's timestamp `+1`s, or add an explicit sequence
  tiebreak. Verify with a server test asserting anchor-then-description order.
- **Request re-share path.** `SubmitRequest` seeds the anchor, but `ShareRequest`
  ([server/services/request/sharing.go](../../server/services/request/sharing.go))
  reuses the conversation and emits no anchor. Should the first comment ever be
  seeded from `ShareRequest`? — **Recommendation:** No. Seed only in
  `SubmitRequest` (the creation moment), mirroring how the anchor works, so a
  request shared into a second community doesn't double-post. No change to
  `ShareRequest`.
- **Empty description.** Requests/Gear can be created with a short or empty
  description. — **Recommendation:** Skip seeding when
  `strings.TrimSpace(description) == ""` (the helper already rejects `""`, but
  skip explicitly so it isn't treated as an error). Matches the acceptance
  criteria.
- **Duplication with the info card.** The entity's description is already shown
  in the info/anchor card at the top of the thread. Is restating it as the first
  comment intentional? — **Recommendation:** Yes, intentional — the issue
  explicitly wants a *repliable/reactable* first comment, and the anchor card
  uses only the name/title, not the description. Confirm with design, but
  proceed; it is additive, not a literal duplicate.
- **Description edited after creation.** If the creator later edits the entity
  description, the seeded comment will not change. — **Recommendation:** Accept
  this — the comment is a point-in-time creator statement (like any chat
  message), not a live mirror of the field. Out of scope for this issue.
- **Media on the first comment.** Should the first comment carry the entity's
  hero media? — **Recommendation:** No for v1 — text only, matching
  `PostNeedAdded`. The hero media is already shown on the entity. Revisit only
  if requested.
- **[audit] Comment-count / "most recent comment" interaction.** Anchor system
  messages are excluded from comment totals via
  [`IsCreationAnchorMessage`](../../server/chat/system_messages.go#L499); the
  seeded description is a *real* `UserChatMessage`, so it **will** count as the
  first comment and could surface as "most recent comment" / bump unread badges
  for other participants. — **Recommendation:** This is the desired behavior
  (it *is* a comment), and push is already suppressed, so no notification spam.
  Verify in Phase 3 that comment counters and any "latest comment" preview read
  sensibly with exactly one seeded comment, and that the seeded message is
  pre-marked read for the creator (the helper already does this).

## Implementation Plan

### Phase 1: Shared helper + Experience path
**Goal:** Establish the one helper and prove the end-to-end behavior on
Experiences (the richest path).
**Tasks:**
- [ ] Add a shared helper in `server/chat`, e.g. `chat.PostCreationDescription(ctx, writer, conversationID, actorID, description) error`, that trims the description, returns early (no-op, no error) when empty, otherwise calls `InsertUserMessageOnBehalfOf` with empty `UserMessageOptions`. Keep warn-don't-fail semantics at the call sites (consistent with the existing anchor inserts).
- [ ] **[audit] Observability:** on the warn-and-continue path at each call site, log at `WARN` (per `docs/server/observability.md` — a non-fatal side-effect failure, not a system failure) via `logging.LoggerWithContext(ctx)` with `operation`, `conversation_id`, `error`, and the entity ID using the **correct field name**: `experience_id`, `gear_id`, and **`target_request_id`** for the request path (never `request_id` — that key is reserved for the HTTP correlation ID and `LoggerWithContext` would collide). The handlers already build a `logger` with most of these.
- [ ] Call it in [share.go](../../server/services/experience/share.go) immediately after the `EXPERIENCE_CREATED` anchor insert, **inside the `isNewConversation` gate**, passing `expStored.Description`.
- [ ] Resolve the anchor-first ordering question (deterministic sort or timestamp tiebreak).
- [ ] **[audit] Isolated unit test for the helper** (architecture doc: test at the most local level): empty/whitespace description → no insert, nil error; non-empty → exactly one `UserChatMessage` by the actor; assert the chat-bus publish fires with on-behalf-of (push suppressed). Use the existing `server/chat` test harness / mock bus.
- [ ] Server test: sharing a new experience with a non-empty description produces exactly two messages (anchor, then description user-message by the creator); empty description produces only the anchor; re-sharing to a second community adds neither.
- [ ] `npm run build` (and `go build -tags=integration ./...` if any signature an integration test calls changes).
**Validates:** The helper, the gating, and the ordering all work on one path
before fanning out.

### Phase 2: Gear + Request paths
**Goal:** Apply the proven helper to the remaining two entity types.
**Tasks:**
- [ ] Call the helper in [gear_sharing.go](../../server/services/community/gear_sharing.go) inside the `conversationCreated` gate, after the `GIVEAWAY_SHARED` / `LOAN_SHARED` anchor, passing `gear.Description`.
- [ ] Call the helper in [lifecycle.go](../../server/services/request/lifecycle.go) `SubmitRequest`, after the `REQUEST_CREATED` anchor, passing `request.Description`. Do **not** touch `ShareRequest`.
- [ ] Server tests mirroring Phase 1 for gear (loan + giveaway) and request, including the empty-description skip and the no-duplicate-on-re-share assertion (gear shared into 2+ communities).
- [ ] `npm run build`.
**Validates:** All three entity types satisfy the acceptance criteria with
identical semantics.

### Phase 3: Client verification (no client code expected)
**Goal:** Confirm the seeded comment renders correctly in the app and is
attributed to the creator, with no client changes required.
**Tasks:**
- [ ] Manually create one of each type via Unified Create and open the resulting conversation; confirm the description shows as the creator's first comment beneath the anchor card, is reactable/repliable, and triggered no spurious push.
- [ ] If the conversation history widget special-cases the anchor message, confirm a leading user message renders without layout issues.
**Validates:** End-user behavior matches the issue intent across Event / Item /
Request.

## Related Issues
- #1679 — established the per-gear (not per-community) conversation + the
  `conversationCreated` gate that this change relies on to avoid duplicate
  anchors/comments on multi-community shares.
