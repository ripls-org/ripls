# Issue #1842: Chat-message events on server/pubsub (chat_event_bus)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1842
**Filed:** 2026-05-10 | **Last activity:** 2026-05-10
**Labels:** claude-fix | **Assignees:** unassigned
**Context docs:** `docs/issues/510-community-event-pubsub-v2.md` (the bus pattern this plan follows), `docs/server/architecture.md`, `docs/server/observability.md`
**Depends on:** PR #1841 (`Community-event pub/sub`) landing first — that PR introduces `server/pubsub` and `server/community_event_bus`. Until it merges, this work cannot start.

## Problem Statement

`server/services/chat/notifications.go::notifyParticipants` is ~190 lines
of dispatch logic that lives inside the chat service: recipient
resolution (membership-aware for community-wide conversations),
per-user preference gating, soft-delete gating on the parent community,
active-stream + foreground suppression, and push payload assembly. That
file imports `server/community` four times — for `IsActive`,
`GetCommunityMemberIDs`, `FetchPreferencesForUsers`, and
`CategoryEnabled` — making chat structurally dependent on the
community library for cross-cutting notification concerns.

In parallel, `server/services/chat/streaming.go::broadcastMessage` is
called inline from four sites (`SendMessage`, `AddReaction`,
`RemoveReaction`, the `MessageBroadcaster` adapter that
`server/chat/system_messages.go` uses). The chat service therefore
plays publisher *and* dispatcher: every emit site has to know to call
`broadcastMessage` then schedule `notifyParticipants` in a goroutine.

PR #1841 introduced a generic `server/pubsub` primitive plus a
domain adapter (`server/community_event_bus`) that solves the
equivalent problem for community events. The primitive is
type-parametric (`pubsub.Topic[T]`, `pubsub.Subscriber[T]`) so other
event families can ship their own thin adapters. This issue is the
chat-message migration of that pattern: rebuild chat events as a
publisher with two subscribers (push notifications, stream broadcast)
so emit sites no longer know fan-out shape, and the chat service
stops importing `server/community` for dispatch concerns.

The acceptance criteria from the issue body:

- `services/chat/notifications.go` shrinks to ≤ 50 lines (just the
  chat service's own subscription wiring, no dispatch logic).
- `services/chat/streaming.go` broadcasts via the chat bus.
- No client-visible behavior change: same notifications, same stream
  messages.
- Documented in `docs/server/architecture.md` as the second domain
  adapter on `pubsub`.

## Context from Comments

The issue has no comments. The body itself is the spec: it names the
new packages (`server/chat_event_bus`,
`server/notifications/chat_subscriber`), points at the lines of
existing code that move (`services/chat/notifications.go:33,58,75,93`),
and states the acceptance criteria above. There are no maintainer
clarifications or disagreements to reconcile.

The single open question the body implicitly raises — "what shape is
the published event?" — is answered by analogy to community events:
parametric `pubsub.Topic[T]`, with `T = *chat_event_bus.PublishedEvent`
wrapping the `*models.ChatMessage` plus the prefetched
`*models.ChatConversation` (and any other denorm context the two
subscribers need so they don't re-fetch the same row).

## Potential Solutions

### Option A — Mirror `community_event_bus` exactly: new `chat_event_bus` package + two subscribers

- **Approach:** Create `server/chat_event_bus` modelled byte-for-byte on
  `server/community_event_bus`. The published event is a struct of
  `*models.ChatMessage` plus prefetched conversation, prefetched sender
  user, and (for transfer-topic conversations) prefetched transfer.
  Two subscribers — `server/notifications/chat_subscriber` for push,
  and a new `chatStreamSubscriber` co-located in
  `server/services/chat` for live stream fan-out — register on the
  bus in `main.go`. Emit sites (`SendMessage`,
  `system_messages.InsertSystemMessage`, `mention_handler`,
  `reactions.AddReaction`, `reactions.RemoveReaction`) call
  `chatEventBus.Publish(...)` and stop calling `broadcastMessage` /
  `notifyParticipants` directly. The old `MessageBroadcaster`
  interface in `server/chat/system_messages.go` is replaced by a
  Publisher dependency on the same package.
- **Pros:**
  - Identical mental model and code shape as
    `community_event_bus`. A reader who knows one knows the other.
  - The `pubsub` primitive's contract (at-most-once, idempotency,
    backpressure, observability) is already proven in production by
    PR #1841 — we inherit it for free.
  - Hits all four acceptance criteria cleanly: `notifications.go`
    becomes a constructor call + a `Subscribe` line, `streaming.go`
    keeps only the stream registry, `architecture.md` gets a
    one-section update naming chat as the second adapter.
  - Sets up the future durable-retry path the issue body flags
    ("durable retry on flaky FCM if/when that ships"): a future
    durable subscriber paginates `ChatMessage` rows by `id` cursor,
    same shape as the community-event durable plan.
- **Cons:**
  - Touches every chat emit site (5 files in `services/chat/`, plus
    `server/chat/system_messages.go`). Test surface is non-trivial.
  - Two oneof variants to fan out (`UserMessage`, `SystemMessage`,
    plus `ReactionUpdate` and `SystemMessageUpdate` events that
    aren't messages at all) — the bus event needs a discriminator
    field, not just `*ChatMessage`. See Open Questions.
- **Effort:** Large. Comparable in scope to PR #1841 but on a
  smaller surface; estimate 4 phased PRs.

### Option B — Reuse `community_event_bus`, add `CHAT_MESSAGE_SENT` event type

- **Approach:** Add `COMMUNITY_EVENT_TYPE_CHAT_MESSAGE_SENT` to the
  existing `models.CommunityEventType` enum. Each chat send writes a
  CommunityEvent with that type, populating a new `chat_message_id`
  field. The community-event bus's existing notification subscriber
  grows a chat code path; the existing community stream subscriber
  also handles chat broadcasts. No new packages.
- **Pros:**
  - One bus instead of two. Less wiring.
  - Reuses the existing prefetch + log fields machinery directly.
- **Cons:**
  - **Conflates audit semantics.** `CommunityEvent` is the durable
    audit log of community-scoped lifecycle actions. Chat messages
    already have their own durable log (`ChatMessage`). Writing
    every chat send as a community-event row roughly doubles the
    write rate of that table for an audit purpose it doesn't serve.
  - **Conflates notification semantics.** Chat notifications
    suppress on `hasActiveStream && isInForeground`. Community
    notifications suppress only on active-stream membership. Same
    subscriber would need two branches, which is what we're trying
    to get away from.
  - DM-style conversations have no `community_id` (they bypass the
    soft-delete gate today via `conversation.CommunityId == ""`).
    Forcing them into `CommunityEvent` requires inventing a sentinel
    or a nullable field — both ugly.
  - Directly contradicts the v2 plan's
    [Future reuse opportunities](docs/issues/510-community-event-pubsub-v2.md#future-reuse-opportunities)
    table, which lists chat as its own adapter.
- **Effort:** Medium, but high cleanup cost later when this proves
  to be the wrong shape.

### Option C — Inline subscriber list inside `chat.Service`, no separate package

- **Approach:** Define `Subscriber` interface inside
  `server/services/chat/`. `Service` keeps an `[]Subscriber`. Emit
  sites call `s.publish(event)` which loops over subscribers and
  calls them synchronously. No `pubsub` library, no separate bus
  package.
- **Pros:**
  - Smallest change. No new packages.
  - Simpler to reason about for someone who has never seen pubsub.
- **Cons:**
  - **Re-implements the `pubsub` primitive in chat.** Backpressure,
    bounded ingress, drain-for-tests, structured-log fanout — all
    exist in `server/pubsub` and would have to be either rebuilt or
    skipped. Skipping them means chat regresses below the bar PR
    #1841 set.
  - **Doesn't decouple chat from the dispatch graph.** The push
    subscriber still has to live somewhere; if it stays in
    `services/chat`, `notifications.go` doesn't shrink.
  - Doesn't satisfy the "second domain adapter on `pubsub`"
    acceptance criterion — there is no adapter, just a private
    list.
- **Effort:** Small, but doesn't solve the stated problem.

## Recommendation

**Option A.** It is the only option that hits all four acceptance
criteria, follows the pattern PR #1841 just established (so two
adapters share one mental model), and leaves the `CommunityEvent`
audit log alone. The two main costs — touching every emit site and
discriminating message vs. reaction events — are the actual work the
issue is asking for, not avoidable accidents.

The four-PR phasing below intentionally mirrors the four-PR phasing
of #510 / PR #1841 so each PR is a small, reviewable, individually
shippable increment.

## Open Questions

- **What event type does the bus carry — `*ChatMessage` directly, or a
  wrapper with a discriminator?** — Recommendation: a wrapper.
  `chat_event_bus.PublishedEvent` carries a `Kind` enum
  (`KIND_USER_MESSAGE`, `KIND_SYSTEM_MESSAGE`, `KIND_REACTION_UPDATE`,
  `KIND_SYSTEM_MESSAGE_UPDATE`) plus the `*ChatMessage` (always set
  except for reaction-update where `MessageId` is set), the
  `*ChatConversation`, and prefetched `*api.User` for the sender /
  actor. The notification subscriber filters to the two message kinds
  (reactions + updates don't fire push); the stream subscriber
  handles all four. This mirrors how
  `community_event_bus.PublishedEvent` carries a typed `Event` plus
  prefetched context, just with a slightly broader `Kind` because
  the chat stream surfaces non-message events.

- **Where does the stream subscriber live — `server/notifications/chat_subscriber` or `server/services/chat`?** — Recommendation:
  co-located in `server/services/chat` (new file
  `stream_subscriber.go`), exactly like
  `community.StreamSubscriber()` in PR #1841. The stream registry is
  in-memory state owned by the chat service; moving the subscriber
  across a package boundary would require exposing the registry,
  which we don't want.

- **Do system messages still fire push?** — Recommendation: no, same as
  today. `server/chat/system_messages.go` already documents
  "Push notifications are NOT sent for system messages — community
  events handle notifications instead." The new
  `chat_subscriber.shouldNotify(kind)` helper short-circuits any kind
  other than `KIND_USER_MESSAGE`, preserving the current behavior.

- **What happens to `chat.Service.notificationDone` and
  `NewWithNotificationSignal`?** — Recommendation: delete both.
  Replace with `chatBus.Drain(ctx)` in tests, mirroring the
  `bus.Drain(ctx)` migration in #510 G2. The `notificationDone`
  channel becomes dead weight once `notifyParticipants` is no
  longer called from inside `SendMessage`.

- **Does the publisher prefetch the `*models.Transfer` for
  transfer-topic deep-links, or does the subscriber?** —
  Recommendation: publisher. The current code does the lookup inline
  in `notifications.go:172`. Moving it into
  `chat_event_bus.prefetch` keeps the subscriber pure (no storage
  reads in the dispatch path) and matches the `community_event_bus`
  prefetch pattern.

- **Does this affect `MessageBroadcaster` callers in
  `server/planning` and `server/services/community/gear_sharing.go`?** —
  Recommendation: yes, but transparently. They call
  `systemMessageWriter.InsertSystemMessage`, which in turn calls
  the broadcaster. We change `SystemMessageWriter` to depend on the
  chat bus's Publisher instead of `MessageBroadcaster`; the
  inserter's public signature stays the same. None of these callers
  change.

- **What's the topic name for log fields?** — Recommendation:
  `topic="chat_messages"`, mirroring `topic="community_events"` from
  PR #1841. Field naming follows
  `docs/server/observability.md` § "Streams and per-event correlation"
  (line 152, which already names `message_id` as the per-event
  identifier on chat-stream log lines): `message_id`,
  `conversation_id`, `chat_event_kind`. Use `message_id` rather than
  inventing a new `chat_message_id` field — the existing
  `services/chat/streaming.go` lines emit `message_id`, and the
  bus-published lines should join cleanly with them. Do NOT use
  `event_type` — that field is reserved for the community-event
  enum string.

- **Does the publisher gate on parent-community soft-delete, or do
  subscribers?** — Recommendation: subscribers, same rule as #510
  G13. The audit-trail row (`ChatMessage`) is already inserted by
  the calling RPC before publish; the bus publishes regardless. The
  push subscriber gates on `community.IsActive`; the stream
  subscriber doesn't gate (in-memory streams self-clean via
  membership cascade and broadcast-to-no-one is a no-op, same as
  the community stream subscriber).

## Implementation Plan

### Phase 1 — Skeleton: `server/chat_event_bus` package and prefetcher

**Goal:** Stand up the new bus package with full unit tests but no
emit-site or subscriber wiring yet. Equivalent in scope to PR #1841
PR 1.

**Publisher contract — important difference from `community_event_bus`:**
`community_event_bus.Publish(ctx, *models.CommunityEvent)` inserts the
audit row AND dispatches. The chat bus does NOT insert any row. The
`ChatMessage` audit row is already inserted by the calling RPC before
`Publish` is called (see `services/chat/messages.go::SendMessage`,
`server/chat/system_messages.go::InsertSystemMessage`,
`reactions.go::AddReaction`, etc.) — chat is a different ownership
model where the storage write is part of the RPC's atomic unit. The
bus's job is dispatch + prefetch + LogContext only. The interface
shape is therefore:

```go
type Publisher interface {
    Publish(ctx context.Context, kind Kind, msg *models.ChatMessage, conversation *models.ChatConversation, opts ...PublishOption) error
}
```

`opts` carries `WithMentionedUserIDs([]string)` and
`WithOnBehalfOf(bool)`. The bus runs `prefetch.go` (sender User,
transfer for transfer-topic deep links) and wraps everything in
`*PublishedEvent` before dispatching. The interface returns `error`
only for nil-input invariants — actual dispatch failures are surfaced
via log lines from the underlying `pubsub.MemTopic`, matching
`community_event_bus`. Reaction-update events (where there is no new
message, just a `MessageId` + updated reactions) pass `msg=nil` plus
`WithReactionUpdate(*api.ReactionUpdate)`; the subscriber dispatches
on `Kind` rather than on `msg != nil`.

**Tasks:**

- [ ] Create `server/chat_event_bus/` with `doc.go`, `README.md`,
      `chat_event_bus.go` (defines `Publisher` interface,
      `PublishedEvent` struct, `Kind` enum, `PublishOption` functional
      options, type aliases for `Subscriber`/`Topic`, `LogContext()`
      per `pubsub.LogContextProvider`), `inprocess.go` (in-process
      `Publisher` backed by `pubsub.MemTopic[*PublishedEvent]`),
      `prefetch.go` (hydrates sender User and transfer-topic transfer
      — conversation is already provided by the caller), `mock.go`
      (test mock matching `community_event_bus.MockBus`).
- [ ] Decide topic name: `chat_event_bus.TopicName = "chat_messages"`.
- [ ] Unit tests covering: publish dispatches to all subscribers in
      order, prefetch failures leave fields nil and log WARN, drop on
      full ingress is logged, `Drain` blocks until all subscribers
      finish, nil-message Publish with `Kind=KIND_REACTION_UPDATE` is
      accepted while nil-message Publish with `Kind=KIND_USER_MESSAGE`
      returns an error.
- [ ] Update `docs/server/architecture.md` to add a one-paragraph
      section on `chat_event_bus` as the second domain adapter on
      `pubsub`. Cross-link to `community_event_bus`. (Note: the
      current `architecture.md` does NOT yet document
      `community_event_bus`; this PR adds the bus-pattern subsection
      that covers both adapters — see Phase 4 for the import-graph
      writeup.)

**Validates:** The bus compiles, tests pass, no production wiring
yet so nothing else breaks.

### Phase 2 — `server/notifications/chat_subscriber` (push side)

**Goal:** Stand up the chat push subscriber in its own package and
register it on the bus in `main.go`. **No emit-site changes in this
phase** — the bus has no publishers yet, so the subscriber receives no
events and the existing in-line `notifyParticipants` path remains the
only thing producing pushes. This isolates the new package from any
behavioral risk and lets Phase 3 land the emit-site cutover as a single
publisher-and-fallback-removal atomic step. Equivalent to PR #1841 PR
2.

**Tasks:**

- [ ] Create `server/notifications/chat_subscriber/` with `doc.go`,
      `README.md`, `subscriber.go`, `recipients.go`, `copy.go`,
      `helpers_test.go`, `recipients_test.go`. Mirror the layout of
      `notifications/community_subscriber/` exactly.
- [ ] `subscriber.go::Handle` does, in order: short-circuit on kind
      != `KIND_USER_MESSAGE` and on `OnBehalfOf=true`; soft-delete gate
      via `community.IsActive` for non-empty `community_id`; resolve
      recipients (delegating to `recipients.go::resolveRecipients`,
      which moves the membership-aware lookup currently at
      `notifications.go:56-63`); preference gate via
      `community.FetchPreferencesForUsers` + `community.CategoryEnabled`;
      active-stream + foreground suppression via the late-bound
      `StreamChecker` and `ForegroundChecker` hooks (same shape as
      `community_subscriber.SetStreamChecker`); copy assembly via
      `copy.go::buildNotification` (sender name truncation, mention
      title, `ChatMessagePayload` deep-link IDs); FCM dispatch via
      `notifications.Service.NotifyUser`.
- [ ] Add `StreamChecker` and `ForegroundChecker` types and
      `Set*Checker` methods on `Subscriber`, same atomic-pointer
      pattern as `community_subscriber`.
- [ ] Wire in `main.go`: construct
      `chatEventTopic := pubsub.NewMemTopic[*chat_event_bus.PublishedEvent](chat_event_bus.TopicName)`,
      `chatEventBus := chat_event_bus.NewInProcessBus(sqlStorage, chatEventTopic)`,
      `chatNotifSub := chat_subscriber.New(sqlStorage, notificationService)`,
      `chatNotifSub.SetStreamChecker(chatService.HasActiveStream)`
      (export the existing private method on `Service`),
      `chatNotifSub.SetForegroundChecker(chatService.IsUserInForeground)`
      (also exported), then
      `chatEventBus.Subscribe(chatNotifSub)`. The chat service still
      owns the stream + presence registries; only the read hooks are
      exported.
- [ ] **Do not change emit sites or remove the in-line
      `notifyParticipants` call in this phase.** No emit site calls
      `Publish` yet, so the subscriber is registered but idle. The
      in-line `notifyParticipants` keeps producing pushes exactly as
      today. Phase 3 atomically replaces emit-site `notifyParticipants`
      calls with `Publish` and deletes the in-line path.
- [ ] Tests: `subscriber_test.go` uses an in-process bus, real
      subscriber, mock notification service, and a hand-constructed
      `PublishedEvent` to assert the full Handle path produces the same
      `Notification` as today's `notifyParticipants` for representative
      shapes (community-wide, participant-list, mentioned, suppressed,
      preference-disabled). `recipients_test.go` covers community-wide
      vs. participant-list paths. Copy tests assert mention title and
      100-char truncation match today's behavior.

**Validates:** New package compiles, is registered on the bus, and
unit tests cover every branch of the old `notifyParticipants`. Because
no emit site has switched yet, no production behavior has changed —
this phase is safe to ship in isolation.

### Phase 3 — Stream subscriber + emit-site migration

**Goal:** Move `broadcastMessage` into a subscriber and migrate every
emit site to publish on the bus instead of calling
`broadcastMessage` / `notifyParticipants` directly. Delete the
in-line dispatch from Phase 2. Equivalent to PR #1841 PR 3.

**Goroutine safety note:** `pubsub.MemTopic` (introduced by PR
#1841) owns its per-subscriber dispatch goroutines, each wrapped in
`logging.GoSafe`-equivalent panic recovery (see
`server/pubsub/memtopic.go` on the `community_event_bus` branch).
The current `SendMessage` flow uses
`logging.GoSafe(ctx, "chat-notify-participants", …)` to spawn the
notify goroutine; that wrapper is deleted along with the in-line
`notifyParticipants` call, since dispatch now happens inside
MemTopic's already-recovered worker. `npm run lint:go` runs
`server/cmd/check-goroutines`, which must continue to pass — Phase
3 introduces no bare `go func()` statements outside `MemTopic`'s
own implementation.

**Tasks:**

- [ ] Add `server/services/chat/stream_subscriber.go` with
      `StreamSubscriberName = "chat_stream"` and a
      `streamSubscriber` type whose `Handle` calls
      `s.broadcastMessage(ctx, conversationID, response)` for every
      kind. Convert `*PublishedEvent.ChatMessage` → the appropriate
      `*api.StreamMessagesResponse` variant in this subscriber, NOT
      in emit sites.
- [ ] Add `chat.Service.StreamSubscriber()` method returning the
      subscriber, same shape as `community.Service.StreamSubscriber`.
- [ ] Update `chat.New(...)` to take the bus Publisher as a
      constructor arg. Remove `notificationService` from the
      constructor signature (the subscriber owns it now); remove
      `BroadcastMessage` and `NotifyParticipants` exported methods.
- [ ] Migrate emit sites:
      - `services/chat/messages.go::SendMessage` — replace the
        `broadcastMessage` call and the `GoSafe` notify goroutine
        with `chatEventBus.Publish(ctx, chat_event_bus.KIND_USER_MESSAGE,
        message, conversation, chat_event_bus.WithMentionedUserIDs(mentionedUserIDs))`.
      - `services/chat/mention_handler.go::insertAddedViaMentionMessage`
        — replace `broadcastMessage` with
        `Publish(KIND_SYSTEM_MESSAGE, message, conversation)`.
      - `services/chat/reactions.go::AddReaction` /
        `RemoveReaction` — replace `broadcastMessage` with
        `Publish(KIND_REACTION_UPDATE, nil, conversation,
        WithReactionUpdate(messageID, reactions))`.
      - `server/chat/system_messages.go::InsertSystemMessage` and
        `tryCoalesce` — change the `broadcaster MessageBroadcaster`
        field to `bus chat_event_bus.Publisher`. The new-message
        path publishes `KIND_SYSTEM_MESSAGE`; the coalesce path
        publishes `KIND_SYSTEM_MESSAGE_UPDATE`. The
        `MessageBroadcaster` interface goes away.
      - `server/chat/system_messages.go::InsertUserMessageOnBehalfOf`
        — publishes `KIND_USER_MESSAGE` with `WithOnBehalfOf(true)`,
        signalling "no push". The notification subscriber
        short-circuits on `OnBehalfOf=true` even for
        `KIND_USER_MESSAGE`. Preserves the current documented behavior
        ("Push notifications are NOT sent — the experience's existing
        notification path handles that").
- [ ] Delete `server/services/chat/notifications.go` outright (down
      from 199 lines → 0, well under the ≤ 50 acceptance criterion;
      keep just the file with package-level constants if any are
      still needed — most likely nothing).
- [ ] Slim `server/services/chat/streaming.go`: keep only
      `StreamMessages`, `registerStream`, `unregisterStream`,
      `hasActiveStream`. The `broadcastMessage` method moves into
      the new `stream_subscriber.go` (still on `*Service` via a
      receiver-method-on-subscriber-call pattern, since it touches
      `s.streams`).
- [ ] Update `chat.Service`: drop `notificationDone`,
      `NewWithNotificationSignal`, `BroadcastMessage`,
      `NotifyParticipants`. Tests use `chatBus.Drain(ctx)` instead.
- [ ] Wire in `main.go`: after constructing `chatService`, register
      `chatEventBus.Subscribe(chatService.StreamSubscriber())` and
      pass `chatEventBus` into `chat.New(...)`. Update the order so
      bus → service → subscriber registration mirrors the
      community-event flow (#1841 main.go:807-832).
- [ ] Migrate every chat-related test setup to construct the bus
      first, then service, then subscribe. Pattern matches
      `services/community/common_test.go` post-#1841.

**Validates:** All chat tests pass without `notificationDone`.
`services/chat/notifications.go` is gone or trivial. Acceptance
criterion #1 is met.

### Phase 4 — Cleanup, docs, and architecture entry

**Goal:** Tidy up the leftover `MessageBroadcaster` shim, update
docs, and confirm acceptance criteria. Equivalent to PR #1841 PR 4.

**Tasks:**

- [ ] Delete the `MessageBroadcaster` interface from
      `server/chat/system_messages.go` (replaced by Publisher
      dependency in Phase 3).
- [ ] Update `server/services/chat/README.md` to describe the bus +
      subscriber model (mirrors how `services/community/README.md`
      was rewritten in #1841).
- [ ] Add `server/chat_event_bus/README.md` cross-references in
      `server/community_event_bus/README.md` ("see also") and in
      `server/pubsub/README.md` ("known adapters"). The
      pubsub README explicitly anticipates this in its "future event
      families" section; update that section to move chat from
      "future" to "current".
- [ ] Update `docs/server/architecture.md` Service Independence
      section (§ "Core Design Principles" → "1. Service
      Independence"): chat no longer imports `server/community` for
      dispatch concerns — the four imports flagged in the issue
      body (`IsActive`, `GetCommunityMemberIDs`,
      `FetchPreferencesForUsers`, `CategoryEnabled`) now live in
      `notifications/chat_subscriber/`, which legitimately depends
      on `community` as a shared library (subscriber → library, not
      service → service). The `services/chat` import graph trims
      to `chat_event_bus`, `notifications`, `storage`, `auth`,
      `clock`, `chat` (lib), `services` (for `FetchAPIUser`).
- [ ] Confirm `goroutine` check passes:
      `go run ./server/cmd/check-goroutines`.
- [ ] Confirm all four acceptance criteria from the issue body are
      met; close the issue from the merge commit.

**Validates:** Issue acceptance criteria. No regressions in chat
streaming, chat notifications, or system-message coalescing.

## Audit notes (changed during /ripls-audit run)

- **Per-event log field is `message_id`**, not a new
  `chat_message_id`. Matches `docs/server/observability.md` § "Streams
  and per-event correlation" (line 150: "Chat streams use `message_id`
  on per-event log lines"). Keeps bus-published log lines joinable
  with existing chat-stream lines under the same field.
- **Goroutine safety:** the `logging.GoSafe(ctx,
  "chat-notify-participants", …)` wrapper at
  `services/chat/messages.go:155` is deleted along with the in-line
  `notifyParticipants` call. Dispatch goroutines are owned by
  `pubsub.MemTopic`, which already recovers panics; the
  `server/cmd/check-goroutines` lint must continue to pass (no bare
  `go func()` outside `MemTopic`'s implementation).
- **Import graph (Phase 4):** chat → community is broken (the four
  `notifications.go` imports — `IsActive`, `GetCommunityMemberIDs`,
  `FetchPreferencesForUsers`, `CategoryEnabled` — move into
  `notifications/chat_subscriber/`). `chat_subscriber` → `community`
  is fine because that is subscriber → shared-library, the pattern
  `docs/server/architecture.md` § "Core Design Principles" §1
  explicitly endorses.
- **Publisher contract differs from `community_event_bus`** (added
  to Phase 1): chat bus does NOT insert a storage row. `ChatMessage`
  is already written by the calling RPC before publish. The bus only
  prefetches denormalized context (sender User, transfer for
  transfer-topic deep links — conversation is provided by the caller)
  and dispatches. `Publish(ctx, kind, msg, conversation, opts...)`
  with functional options (`WithMentionedUserIDs`, `WithOnBehalfOf`,
  `WithReactionUpdate`) is cleaner than a fat `*PublishedEvent` arg
  and makes the asymmetry from `community_event_bus` explicit at the
  call site.
- **Phase 2 sequencing clarified:** no emit-site changes in Phase 2.
  The subscriber is registered on the bus but receives no events
  because no caller has switched to `Publish` yet. The in-line
  `notifyParticipants` keeps producing pushes exactly as today. Phase
  3 atomically swaps every emit site to `Publish` and deletes the
  in-line path — eliminates the risk of a transient double-push
  window while still letting the new package land in its own
  reviewable PR.
- **`architecture.md` is freshly extended, not amended.** `git log
  -- docs/server/architecture.md` shows the doc was not updated when
  #1841 landed, so the new bus-pattern subsection introduced here is
  the first mention of both `community_event_bus` and
  `chat_event_bus`. Phase 1 writes the bus-pattern paragraph; Phase 4
  follows up with the import-graph subsection.

## Related Issues

- **#1841 / #510** — the parent work that introduces `server/pubsub`
  and `server/community_event_bus`. **This issue is blocked on
  #1841 merging.** When work starts, the first thing to do is
  rebase on top of `main` after #1841 lands.
- **#1622, #1623** — soft-delete gates around chat notifications.
  This work preserves both gates (community-soft-delete in the push
  subscriber, conversation-soft-delete already enforced upstream).
  Tests in `services/chat/notifications_test.go` cover the
  community-deleted gate and must continue to pass.
- **#1186** — system-message coalescing. The
  `tryCoalesce` path in `server/chat/system_messages.go` continues
  to fire the existing `SystemMessageUpdate` broadcast — Phase 3
  preserves this by adding `KIND_SYSTEM_MESSAGE_UPDATE` to the
  bus's `Kind` enum.
- **#1613, #1614** — log-based metrics, no Prometheus. The chat bus
  inherits `pubsub`'s structured-log surface from PR #1841; no new
  Prometheus instrumentation is added. Field names follow
  `docs/server/observability.md`.
- **Future durable retry on flaky FCM** — flagged in the issue body
  but explicitly out of scope. The `ChatMessage` table already
  serves as the durable log a future durable subscriber would
  paginate, mirroring the community-event durable plan in #510 G3.
