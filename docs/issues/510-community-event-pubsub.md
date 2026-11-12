# Issue #510: Create pub/sub type subsystem for community events

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/510
**Filed:** 2025-12-23 | **Last activity:** 2026-05-10
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Today, almost every write-side RPC across the server emits a `CommunityEvent`
by calling a single shared library function — `community.RecordCommunityEventAndNotify`
in `server/community/events.go`. That one function does five things at once:

1. Inserts the `CommunityEvent` storage row.
2. Broadcasts the event to active `StreamCommunityEvents` subscribers (via a
   package-level `eventBroadcaster` global wired up by the community service
   constructor through `community.SetEventBroadcaster`).
3. Decides which user IDs to notify based on `event_type`
   (`getNotificationRecipients`).
4. Resolves per-user `CommunityNotificationPreferences` and gates by category.
5. Builds a localized `Notification` payload and dispatches push via the
   injected `notifications.Service`.

The maintainer's stated goal (see issue comment 2026-05-10) is to **sever
cross-service dependencies** by factoring this out of the community library
into a central pub/sub subsystem. The community service should shrink and
become *a subscriber* of community events, not the place where every other
service routes through to deliver notifications.

The current shape has three concrete problems:

- **Hidden coupling.** Every emitter is on the hot path of every consumer.
  Adding a new subscriber (impact metrics, equity computation, skill
  processing, transition notifications — explicitly listed in the issue
  body) means editing `notifications.go` or threading more globals.
- **Single dispatch site.** Recipients, preferences, copy, and push all live
  in one `~900-line` file (`server/community/notifications.go`). New event
  consumers can't be added without editing it.
- **Globals for wiring.** `eventBroadcaster` and `streamChecker` are
  package-level vars set at startup. That works for two consumers; it
  doesn't extend to N.

The deliverable is a designed pub/sub subsystem that lets services emit
once and any number of subscribers consume independently — without
emitters knowing who consumes.

## Context from Comments

Four comments. Key signals:

- **2026-03-04 (tescobar, P4):** "Architectural foundation for future
  features (nudges, notifications, event processing). Not needed until
  those features are prioritized." — confirms this is foundational and
  not blocking a specific user-facing bug.
- **2026-04-24 (maintainer, triage):** Iceboxed as P4. No semantic
  change.
- **2026-05-10 (maintainer, primary direction):** "Many services
  depend on the community service for sending notifications, and also
  for acting when other events happen. Consider a design where these
  things are factored out of the service to a central notification
  system, like say a pub/sub for community events. This probably means
  that the community service itself would shrink a lot not to include
  notifications and itself become a dependent of the pub/sub or
  whatever."

The 2026-05-10 comment is the authoritative direction — it explicitly
asks for *design exploration*, not implementation. That makes this an
RFC-style issue: the deliverable is a plan and recommendation, not a
finished system.

The prior wording in the issue body ("transition notifications, equity
computation, skill processing") names three concrete future subscribers.
The design must accommodate them as a sanity check.

## Potential Solutions

### Option A: In-process publisher with explicit subscriber registration

- **Approach:** Create a new package `server/eventbus` exposing a
  `Publisher` interface and a concrete `InProcessBus` implementation.
  Subscribers register at startup with a typed `Subscribe(handler)` call
  that returns an `Unsubscribe` function. Publication is synchronous
  insert + asynchronous fan-out (per-subscriber goroutine, bounded by
  `logging.GoSafe`, with a per-subscriber timeout). Move the storage
  insert and the soft-delete gate into the publisher itself; move
  notification dispatch, story creation, and stream broadcasting into
  separate subscriber packages
  (`server/notifications/community_subscriber`,
  `server/story/subscriber`, `server/services/community/stream_subscriber`).
  Wire all subscribers in `main.go`. Replace
  `community.RecordCommunityEventAndNotify` with a thin
  `eventbus.Publish(ctx, event)` call at every emit site.
- **Pros:**
  - Eliminates cross-service-via-library coupling: emitters know about
    `eventbus.Publisher` only.
  - The community service genuinely shrinks — it owns *one* subscriber
    (the stream broadcaster), not the whole pipeline.
  - Subscribers are independently testable (each takes a stub event and
    a stub storage / notification service).
  - Clear extension point for the three future subscribers named in the
    issue body — each becomes a new package implementing
    `eventbus.Subscriber`.
  - Replaces the `eventBroadcaster` / `streamChecker` package-level
    globals with explicit DI.
- **Cons:**
  - Requires editing every emit site (~33 call sites in 16 files).
  - Async fan-out means subscriber failure no longer surfaces inline to
    the emitter — needs explicit per-subscriber failure logging and a
    test signal mechanism.
  - In-process only: a multi-replica deploy still needs broadcasting
    over an external transport, not solved here. Acceptable for now —
    the existing system is also in-process.
- **Effort:** Large (one or two phases of structural work + per-emit-site
  rewrite + test migration). Estimated 3–5 PRs.

### Option B: Hook-style dispatcher kept inside `server/community`

- **Approach:** Keep `community.RecordCommunityEventAndNotify` as the
  single emit site, but turn the body into a list of `EventHook` callbacks
  registered at startup (today's `eventBroadcaster` global generalized to
  a slice). Notification dispatch, story creation, and stream broadcast
  become hooks instead of inline code. Hooks run synchronously (same
  goroutine model as today) but each is its own function.
- **Pros:**
  - Smallest diff: emitters don't change at all.
  - Removes the hardcoded `if shouldNotify { notify }` branch from the
    emitter; each event-type-aware hook owns its own dispatch decision.
  - Solves the "globals don't scale to N" half of the problem without
    moving the bus out of `server/community`.
- **Cons:**
  - **Doesn't satisfy the maintainer's stated goal.** The 2026-05-10
    comment explicitly says the community service / library should
    *shrink* and become a *dependent* of the pub/sub. Keeping the
    dispatcher inside `server/community` is the opposite of that.
  - Cross-service coupling stays — every emitter still imports
    `server/community`.
  - Doesn't help the three future subscribers (impact, equity, skills);
    they would still register hooks inside the community library.
- **Effort:** Small. One PR.

### Option C: Storage-backed outbox with a polling dispatcher

- **Approach:** Emitters insert a `CommunityEvent` row in the same
  transaction as their domain mutation, with a `dispatched=false` flag.
  A background dispatcher process (re-using the
  `server/jobs/` machinery — same shape as
  `community_purge_reminder.go`) polls undispatched events and fans out
  to subscribers. Subscribers register handlers with the dispatcher.
  Re-uses the existing `CommunityEvent` table; adds a small
  `CommunityEventDispatch` row per (event, subscriber) tracking
  delivery status.
- **Pros:**
  - **Durable.** Survives crashes — the only design here that does.
  - Makes equity computation, skill processing, and transition
    notifications naturally idempotent and retriable.
  - Multi-replica safe.
- **Cons:**
  - Materially more complex than today's model. Polling latency
    (~seconds) is incompatible with `StreamCommunityEvents` real-time
    fan-out, which needs sub-second delivery — would need to keep the
    in-process broadcaster *in addition to* the outbox dispatcher.
  - Schema migration: `dispatched_at`, `dispatched_subscribers`, or a
    new `community_event_dispatch` table.
  - The four issue-named consumers (transition, equity, skills,
    notifications) don't all need durability. Push notifications fire
    once at action time and are intentionally best-effort; equity
    computation can be re-derived from event history. Adding the
    durability machinery is over-engineering for the immediate ask.
- **Effort:** Large+. Two or three PRs minimum, plus a schema
  migration.

## Recommendation

**Option A: in-process publisher with explicit subscriber registration.**

It is the only option that satisfies the maintainer's stated goal — the
community service stops owning the dispatch machinery and becomes one
subscriber among several. It maps cleanly onto the three future
subscribers named in the issue body (impact, equity, skill). It uses
patterns already present in the codebase (`story.Creator` interface
injection in `main.go`, `logging.GoSafe` for async dispatch with
recover, `notifications.Service` interface for the push provider).

Option A also aligns directly with `docs/server/architecture.md` §
"Core Design Principles":
- **Service Independence**: "Services should not call or depend on
  other services… Factor out shared functionality into standalone
  libraries". `eventbus` is exactly such a library — emitters depend
  on the `Publisher` interface, not on the community service.
- **Shared Functionality in Libraries**: every named example in the
  architecture doc (`storage.ProtoSQLStorage`, `auth.UserManager`,
  `notifications.Service`) is the shape `eventbus.Publisher` follows.

Option B is rejected because it leaves the architectural problem in
place — the maintainer comment explicitly contradicts it, and it
violates the "Services should not call or depend on other services"
principle by keeping `server/community` as a de-facto dispatch service.

Option C's durability is a real benefit for some future subscribers
(equity computation in particular), but bolting an outbox onto every
event today is over-engineering for the immediate ask. Option A's
publisher interface deliberately leaves room for an async/durable
implementation behind the same interface — once equity computation is
prioritized (P4 today), it can ship as a `DurableSubscriber` wrapping
Option C's machinery for the small set of events that need it, without
changing emit sites or other subscribers. **Option A first; revisit
durability when a concrete subscriber needs it.**

## Open Questions

- **Should `eventbus.Publisher` insert the storage row, or do emitters
  insert and then publish?** — Recommendation: **publisher inserts**,
  matching today's `RecordCommunityEventAndNotify` contract. Emitters
  pass an unpersisted `CommunityEvent`; the publisher returns the
  inserted ID (and writes it back into the message via `event.Id =
  id`). Gives one durable contract for "emit an event," removes the
  chance of an emitter forgetting to insert, and preserves the existing
  audit-trail invariant ("the row is recorded regardless of whether
  push fires").

- **Where does the soft-delete gate (`firesForDeletedCommunity` /
  `IsActive`) live?** — Recommendation: **in the
  `notifications/community_subscriber`**, not the publisher. The gate
  is push-specific; the audit-trail row should still be recorded for a
  community deletion event even if push is suppressed. Streaming and
  story subscribers may want their own gates with different semantics.
  Keeping the gate per-subscriber is more honest than today's
  "soft-delete-aware emitter."

- **How do subscribers signal completion to tests?** — Recommendation:
  **publisher accepts an optional `chan struct{}` per call**, mirroring
  today's `notificationDone` parameter; per-subscriber goroutines
  `<-chan` into it after they finish. Tests opt in by passing a
  buffered channel; production passes nil. Keeps the existing test
  helpers in `community/events_test.go` and
  `services/community/streaming_test.go` viable.

- **Synchronous publisher API or fully async?** — Recommendation:
  **synchronous insert (returns `(eventID, error)`), async fan-out**.
  Mirrors today's behavior — the storage write must be inline so the
  emitter can write the resulting ID back into related rows (e.g.
  `Story.CommunityEventId` from `transfer/lifecycle.go`). Subscriber
  fan-out runs via `logging.GoSafe` with a per-subscriber 30s timeout,
  identical to today's notification path.

- **Do `experience` planning events and `chat` system messages move
  to subscribers too?** — Recommendation: **out of scope for this
  plan**. Both stay where they are today. Chat system messages are
  written *as part of the action* (transfer state change), not as a
  reaction to an event; treating them as subscribers would couple
  ordering across services. Planning events already go through
  `RecordCommunityEventAndNotify` and naturally migrate as emit sites
  in Phase 4.

- **Story creation: subscriber or kept as-is?** — Recommendation:
  **subscriber, in Phase 5**. Today, `story.Creator` is injected into
  three services (community, transfer, request) and called inline at
  state transitions. Moving it to a subscriber lets future story
  consumers (e.g. equity recap) drop in cleanly. Defer to Phase 5
  because it's not strictly necessary to satisfy the issue and adds
  scope.

## Implementation Plan

The plan ships in five phases. Phases 1–4 are the minimum needed to
close the issue (community service no longer owns dispatch). Phase 5 is
optional cleanup that completes the migration.

### Phase 1: Introduce `server/eventbus` and the publisher contract

**Goal:** Land the new package with the publisher interface, the
in-process implementation, and a stub `Subscriber` interface — *no
behavior change yet*. The package is unused until Phase 2.

**Tasks:**
- [ ] Create `server/eventbus/` with `doc.go` and a `README.md`.
  Files: `eventbus.go` (interfaces), `inprocess.go`
  (`InProcessBus`), `mock.go` (`*FakeBus` for use in service tests),
  `inprocess_test.go`. Each file <500 lines per
  `docs/server/architecture.md` § "File Size Guidelines".
- [ ] Define `eventbus.Publisher` interface:
  ```go
  type Publisher interface {
      Publish(ctx context.Context, event *models.CommunityEvent, done chan struct{}) (eventID string, err error)
  }
  ```
  with the contract: insert storage row, then dispatch to all registered
  subscribers asynchronously, returning the row ID.
- [ ] Define `eventbus.Subscriber` interface:
  ```go
  type Subscriber interface {
      Name() string
      HandleEvent(ctx context.Context, event *models.CommunityEvent) error
  }
  ```
- [ ] Implement `eventbus.InProcessBus` with `Subscribe(Subscriber)
  Unsubscribe` and synchronous insert + async fan-out (per-subscriber
  goroutine via `logging.GoSafe` with a 30s timeout). Per-subscriber
  errors are logged with the canonical fields from
  `docs/server/observability.md`: `operation` = `"eventbus.fanout"`,
  `subscriber` = subscriber name, `event_type`, `event_id`,
  `community_id`, plus `error` and `duration_ms`. `request_id` is
  reserved for the HTTP correlation key (see CLAUDE.md) and is
  inherited via `logging.LoggerWithContext(ctx)` automatically — do
  not set it manually.
- [ ] Use `connecterr.Internal(ctx, "eventbus.Publish", err, kv...)`
  if and when `Publish` ever needs to surface a Connect error (it
  doesn't today — the only caller-visible failure is the storage
  insert, and emitters wrap it themselves). Document this contract
  in `eventbus.go`.
- [ ] Add unit tests covering: insert + fan-out, subscriber failure
  isolation (one bad subscriber doesn't poison others), the test-signal
  `done` channel, ctx cancellation, panic recovery (via `logging.GoSafe`).
  Mocks live in `eventbus/mock.go` per the architecture doc's
  "Define mocks in the library package… Reuse mocks across services"
  guidance.
- [ ] Add `doc.go` package comment ("Package eventbus provides…")
  and `README.md` explaining the emitter/subscriber/publisher trio
  and the failure model.

**Validates:** The interface is callable, tests pass, `npm run build`
clean. No emit sites yet use it.

### Phase 2: Move dispatch into subscribers (community service is the first)

**Goal:** Migrate the existing three behaviors of
`RecordCommunityEventAndNotify` into subscribers, with the publisher as
the orchestrator. The community library still exposes
`RecordCommunityEventAndNotify` at this stage — it now delegates to
`eventbus.Publish` internally.

**Tasks:**
- [ ] Create `server/notifications/community_subscriber/`. Move the
  bulk of `server/community/notifications.go` (`getNotificationRecipients`,
  `buildNotification`, `notifyTargetedUsers`, `firesForDeletedCommunity`,
  `IsActive`-gate use, `CategoryFor`, `CategoryEnabled`,
  `FetchPreferencesForUsers`) into it as a `Subscriber` implementation.
  Constructor takes `*storage.ProtoSQLStorage`, `notifications.Service`,
  and a `streamChecker func(string, string) bool`. The current
  `notifications.go` is ~900 lines, so split it across `subscriber.go`
  (entry point + `HandleEvent` dispatch), `recipients.go`
  (`getNotificationRecipients` switch), `copy.go`
  (`buildNotification` switch + denormalization helpers),
  `preferences.go` (`CategoryFor`, `CategoryEnabled`,
  `FetchPreferencesForUsers`, `IsActive`), each <500 lines per the
  architecture doc.
- [ ] Create `server/services/community/stream_subscriber.go`. Move the
  in-memory event-stream registry (`eventStreams`, `eventStreamsMu`,
  `BroadcastCommunityEvent`, `HasActiveEventStream`,
  `registerEventStream`, `unregisterEventStream`,
  `communityEventToItem`) here. The subscriber's `HandleEvent` calls
  `BroadcastCommunityEvent`. Stays inside `services/community` because
  it owns per-stream state shared with `StreamCommunityEvents`. The
  `services/community/Service` exposes `StreamSubscriber()` so
  `main.go` can register it on the bus after construction.
- [ ] Rewrite `server/community/events.go::RecordCommunityEventAndNotify`
  to be a thin shim that takes an `eventbus.Publisher` parameter and
  forwards to it. Migration is internal — emitter signatures stay
  the same in this phase to keep the diff bounded; Phase 3 swaps
  emitters over to `eventbus.Publish` directly.
- [ ] Wire it all in `main.go`:
  ```go
  bus := eventbus.NewInProcessBus(sqlStorage)
  notifSub := notifsubscriber.New(sqlStorage, notificationService, /* streamChecker filled below */)
  bus.Subscribe(notifSub)
  // streamSubscriber created after communityService since they share state.
  bus.Subscribe(communityService.StreamSubscriber())
  notifSub.SetStreamChecker(communityService.HasActiveEventStream)
  community.SetPublisher(bus) // transitional, removed in Phase 3
  ```
- [ ] Remove `community.SetEventBroadcaster` and `community.SetStreamChecker`
  globals. `streamChecker` becomes a setter on the notification
  subscriber populated after the community service is constructed
  (mirrors the existing late-bind ordering — see `service.go:78
  registerBroadcaster()`).
- [ ] Migrate existing tests in `server/community/events_test.go`,
  `server/community/notifications_test.go`,
  `server/services/community/notifications_test.go`, and
  `server/services/community/streaming_test.go`: they now use
  `eventbus.NewInProcessBus` directly with a stub subscriber, or
  use a real subscriber wired against a stub `notifications.Service`.
  Per `docs/server/architecture.md` § "Mock Strategy", reuse the
  shared `notifications/noop` and `notifications/mock` packages
  rather than introducing per-subscriber test doubles.

**Validates:** Existing test suite passes. The community service shrinks
by ~600 lines (most of `server/community/notifications.go` moves out).
No behavior change visible to clients.

**Audit notes (changed during /ripls-audit run):**
- Added explicit reference to `docs/server/architecture.md` Service
  Independence and Shared Functionality principles in the
  Recommendation, since Option A is the architecturally-blessed shape.
- Pinned the per-subscriber log-field schema to the canonical names
  in `docs/server/observability.md` and called out the `request_id`
  reservation from CLAUDE.md so a future implementer doesn't mint a
  new key for the HTTP correlation ID.
- Specified file-level decomposition for the new
  `notifications/community_subscriber/` package so the migration
  doesn't reintroduce a 900-line file (the architecture doc's
  "<500 lines target / <1000 lines max" rule).
- Directed test mocks to the existing `notifications/noop` and
  `notifications/mock` packages per the architecture doc's shared-mock
  guidance, rather than minting per-subscriber doubles.
- Called out `logging.GoSafe` enforcement (`server/cmd/check-goroutines`
  CI gate) so the publisher's fan-out goroutine doesn't slip through.
- Confirmed no client-facing changes (this is a server-only refactor,
  so the client architecture / caching / accessibility / i18n /
  testing reference docs do not impose any plan changes).

### Phase 3: Migrate emit sites away from `RecordCommunityEventAndNotify`

**Goal:** Replace the 33 `community.RecordCommunityEventAndNotify`
production call sites with `eventbus.Publish` (or, more precisely,
`s.eventBus.Publish(...)` on each emitter service), and inject
`eventbus.Publisher` into every emitter constructor.

**Tasks:**
- [ ] Add an `eventBus eventbus.Publisher` field and constructor
  parameter to: `community.Service`, `experience.Service`,
  `transfer.Service`, `request.Service`, `login.Service`,
  `chat.Service`. Wire `bus` into each in `main.go`.
- [ ] Migrate call sites in:
  - `server/services/community/lifecycle.go` (8 sites)
  - `server/services/community/invitations.go` (1)
  - `server/services/community/gear_sharing.go` (2)
  - `server/services/experience/{events,lifecycle,planning_events,share,undo}.go` (~7)
  - `server/services/request/{events,undo}.go` (~5)
  - `server/services/transfer/{lifecycle,interest,undo}.go` (~7)
  - `server/services/login/registration.go` (1)
- [ ] Delete `community.RecordCommunityEventAndNotify` and its globals
  (`eventBroadcaster`, `streamChecker`,
  `notificationTimeout` if no longer used,
  `SetEventBroadcaster`, `SetStreamChecker`). Confirm
  `git grep RecordCommunityEventAndNotify` returns zero.
- [ ] Update unit tests at each emit site to pass a fake `eventbus.Publisher`
  (or a real `InProcessBus` with no subscribers).

**Validates:** `npm run build` passes. Full test suite passes including
integration tag. No production caller imports
`RecordCommunityEventAndNotify`.

### Phase 4: Finalize the community library teardown

**Goal:** Make `server/community/` a small library again — only the
helpers that are genuinely community-domain (region computation,
preference filter, `IsActive`, `GetCommunityMemberIDs`,
`FetchPreferencesForUsers`) remain. The dispatch machinery is gone.

**Tasks:**
- [ ] Verify `server/community/notifications.go` no longer contains
  `notifyTargetedUsers`, `getNotificationRecipients`,
  `buildNotification`, or any push-aware code. Helpers used by the
  notification subscriber (`CategoryFor`, `CategoryEnabled`,
  `FetchPreferencesForUsers`, `IsActive`) stay if they are useful to
  more than one consumer; otherwise they move into the subscriber too.
- [ ] Delete `server/community/events.go` if it has no remaining
  function (the publisher contract has fully replaced it).
- [ ] Update `server/community/README.md` and `server/community/doc.go`
  to describe the slimmed scope.
- [ ] Update `server/services/community/README.md` to mention the
  `stream_subscriber.go` and document that the service is now an
  *event subscriber* rather than the dispatch hub.

**Validates:** `npm run lint:dart:size` is unaffected (this is server-only
work). `git diff --stat server/community/` shows a net deletion of
several hundred lines. Architecture audit (`/ripls-audit`) confirms
the new layering: emitter services → `eventbus.Publisher` → 
subscribers (notification, stream, future).

### Phase 5 (optional): Move story creation to a subscriber

**Goal:** Eliminate `story.Creator` injection into community, transfer,
and request services by making story creation a subscriber that listens
for the same `*_COMPLETED` and `REQUEST_FULFILLED` events that already
trigger it.

**Tasks:**
- [ ] Create `server/story/subscriber/`. Map relevant
  `CommunityEventType` values to `CreateStoryRequest` shapes; emit
  via the existing `story.Creator`.
- [ ] Remove `story.Creator` from constructor signatures of the three
  affected services.
- [ ] Migrate the per-service `CreateStory` call sites to the
  subscriber. Tests at each site move to the subscriber.

**Validates:** `git grep story.Creator` returns only the subscriber
package and `main.go`. The same set of stories is created for the
same actions (verified by integration tests in
`server/services/transfer/lifecycle_test.go` etc.).

## Related Issues

- **#1657, #1658, #1659** — community delete/restore lifecycle. These
  PRs wired the existing `eventBroadcaster` global and added the
  `firesForDeletedCommunity` exception. The new subscriber-based design
  preserves that semantic by moving the gate into the notification
  subscriber. Tests in `services/community/streaming_test.go` and
  `community/events_test.go` continue to be the regression coverage.
- **#1622, #1623** — soft-delete gates around notification dispatch and
  async writes. The new design moves the notification gate to the
  notification subscriber and keeps the async-write gate at the
  caller (no change). These tests must continue to pass.
- **Issue body sub-tasks** ("transition notifications, equity
  computation, skill processing") — these become future subscribers
  filed under separate issues once prioritized. Not in scope for #510
  itself; the deliverable here is the platform that lets them ship.
