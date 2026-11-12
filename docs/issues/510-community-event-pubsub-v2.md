# Issue #510: Create pub/sub type subsystem for community events (v2)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/510
**Filed:** 2025-12-23 | **Last activity:** 2026-05-10
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/issues/510-community-event-pubsub.md` (v1 plan being vetted), `docs/server/architecture.md`, `docs/server/observability.md`, issues #1613 and #1614 (pivot from Prometheus to log-based metrics).
**Supersedes (does not replace):** `510-community-event-pubsub.md` — v1 stays for diff/audit purposes; this v2 is the working plan.

## Status

This plan is the single source of truth for #510 implementation across PRs.
Update the table as each PR lands. The "Owner" column is filled when work
starts; until then it stays unassigned.

| PR | Title | Status | Owner | Closes |
|----|-------|--------|-------|--------|
| 1 | Foundation: `pubsub` library + `community_event_bus` skeleton | landed (commit `15b950d8d`, branch `pub_sub`) | — | — |
| 2 | Migrate dispatch into subscribers (notification + stream); slim community lib | landed (commit `da1d1edbd`, branch `pub_sub_pr2`) | — | — |
| 3 | Migrate emit sites + delete legacy shim | landed (commit `d056b7a52`, branch `pub_sub_pr3`) | — | #510 partial |
| 4 | Story-creation subscriber (closes #510) | landed (commit `8f87b9973`, branch `pub_sub_pr4`) | — | #510 |

Subsequent reuse work — chat events, gear lifecycle, equity computation —
files separate issues once #510 is closed. They are sketched in
[Future reuse opportunities](#future-reuse-opportunities) for design context
but are out of scope for this issue.

## Why a v2

The user asked us to "take existing plan and thoroughly vet it for long-term
best practice, extensible design. also look for other opportunities to reuse
the pub/sub interface to further decouple services and/or factor shared
logic into libraries. plan can break things down into multiple PRs as
necessary."

v2 keeps v1's direction (Option A — in-process publisher with explicit
subscribers) but:

1. Splits the design into **two layers** — a small generic `pubsub` library
   and a domain-specific `community_event_bus` on top — so the same
   machinery can decouple chat, gear, and user lifecycle later.
2. Tightens the **publisher / subscriber contract** in ways v1 left
   implicit (delivery semantics, idempotency, error handling, observability,
   replay, backpressure).
3. **Reshapes the phase plan into four shippable PRs** instead of v1's two
   large migration phases (and instead of the ten-PR breakdown an earlier
   v2 draft proposed).
4. Adds a **future-subscribers register** sketching the two surviving
   future consumers from the issue body (transition notifications, equity).
   Skill processing is excluded — the skill system has been removed from
   the codebase.
5. Identifies **other event families** (chat, gear, user, media) where the
   same interface is the right answer and proposes which to migrate now vs.
   later.

## Audit of v1 plan

v1 is a sound starting point — Option A is the right shape, the failure model
matches today's behavior, and the open-question recommendations are
reasonable. The audit below records gaps that v2 fixes.

### Strengths in v1 (carry forward)

- Correctly rejects Option B (hook list inside `server/community`) on
  architectural grounds: keeping the dispatcher in the library being depended
  on doesn't satisfy the maintainer's stated goal.
- Correctly defers Option C (durable outbox) for now (v2 explains the
  trade-off in [Durable outbox](#durable-outbox-explanation--recommendation)).
- Captures the soft-delete gate (`firesForDeletedCommunity` / `IsActive`)
  as a per-subscriber concern, not a bus concern.
- Recognizes chat system messages must NOT move to a subscriber, because
  they are a precondition of the event (the `system_chat_message_id` is
  persisted on the emitted `CommunityEvent` so undo flows can soft-delete
  the message via direct read — see
  `server/services/transfer/lifecycle.go:113-122`). v2 keeps this.

### Gaps in v1 (v2 fixes)

#### G1. The `Publisher` interface bakes in CommunityEvent

v1 defines `Publisher.Publish(ctx, *models.CommunityEvent, done) (string, error)`. This interface can never be reused for chat, gear, or user lifecycle without duplication. The user explicitly asked for opportunities to **reuse the pub/sub interface**.

**Fix:** Split into two layers.

- `server/pubsub`: generic, type-parametric, knows nothing about
  `CommunityEvent` or storage. Provides `Topic[T]` and `Subscriber[T]`
  over an in-process `MemTopic[T]`. Reusable by any future event family.
- `server/community_event_bus`: a CommunityEvent-specific adapter. Owns the
  storage insert, the timestamp default, and the soft-delete-aware
  orchestration. Exposes a non-parametric `Publisher` so emit sites stay
  simple.

This is the same pattern as `storage` (generic) + `auth.UserManager`
(domain-specific) elsewhere in the server.

#### G2. Test signaling via per-call `done chan` doesn't scale to N subscribers

v1 keeps the existing `notificationDone` channel. With N subscribers, "the
dispatch finished" needs to mean "all N finished," and tests would need to
count N drains per publish.

**Fix:** Promote completion-waiting to the bus.

- `MemTopic[T]` exposes `Drain(ctx)` that blocks until all in-flight
  dispatches across all subscribers finish or `ctx` is done.
- `community_event_bus` exposes `Drain(ctx)` that delegates.
- Tests call `bus.Drain(ctx)` instead of channel-counting.
- Production never calls `Drain`.
- The `done chan struct{}` pattern is removed from the public API; existing
  test-only call sites migrate to `Drain`.

#### G3. No specified delivery semantics

v1 doesn't say in writing what guarantees subscribers get. This matters for
durable subscribers in the future.

**Fix:** Define explicitly:

- **In-process semantics (this work):** at-most-once, async fan-out. A
  subscriber may miss events on process crash, panic-after-recover, or
  context cancellation. Subscribers MUST be idempotent and MUST NOT assume
  they see every event.
- **Durable semantics (future work):** the `CommunityEvent` table is
  already the durable log. A future durable subscriber paginates that
  table directly with a per-subscriber cursor — no separate outbox table
  needed. See [Durable outbox](#durable-outbox-explanation--recommendation).
- **Ordering:** within a single subscriber, events arrive in publish order
  (per-subscriber goroutine, no concurrency within a subscriber). Across
  subscribers there is NO global order — they run in parallel.

#### G4. No backpressure model

v1's per-subscriber goroutine spawns one goroutine per (event × subscriber).
A slow subscriber under load piles up goroutines unboundedly.

**Fix:** Each subscriber gets a bounded ingress channel (default size 256).
The publisher non-blockingly tries to enqueue:

- On success: the subscriber's worker goroutine consumes serially.
- On full: drop the event for that subscriber, increment the
  per-subscriber `events_dropped` counter, log at WARN with the event ID.

This bounds memory and surfaces saturation. Subscribers that can't afford
drops opt into the (future) durable cursor-based model.

#### G5. Observability needs to be log-based, not Prometheus

v1 specifies log fields but doesn't pin a metrics surface. The codebase is
**actively pivoting away from Prometheus** (#1613 replaces the Prometheus
pipeline with Cloud Monitoring log-based metrics; #1614 deletes the
unused `/metrics` endpoint). v2 must align.

**Fix:** No Prometheus instrumentation in `server/pubsub`. Instead, every
dispatch outcome emits a structured log line with fields that Cloud
Monitoring's log-based DISTRIBUTION and COUNTER metrics can extract
(`EXTRACT(jsonPayload.<field>)`). Field names match the conventions in
`docs/server/observability.md` § "Standard Field Names" — in particular
`community_event_id` (not `event_id`), so the field aligns with the
existing community-event-stream per-event correlation key (§ "Streams
and per-event correlation").

The schema (every line uses `logging.LoggerWithContext(ctx)` so
`request_id` and `user_id` are inherited automatically; do NOT set
either explicitly):

- **Per published event** (INFO):
  ```
  message="pubsub event published"
  operation="pubsub.publish"
  topic="community_events"
  event_type="..."           // e.g. "COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE"
  community_event_id="..."
  community_id="..."
  ```
- **Per dispatch attempt** (level depends on outcome — see below):
  ```
  message="pubsub event dispatched"
  operation="pubsub.dispatch"
  topic="..."
  subscriber="..."           // e.g. "community_notifications"
  event_type="..."
  community_event_id="..."
  duration_ms=42
  outcome="success" | "failure" | "dropped"
  reason="..."               // when outcome != "success"; e.g. "panic", "ctx_canceled", "ingress_full"
  error="..."                // when outcome == "failure" with a returned error
  ```

Log level per outcome (per observability.md § 5 "ERROR is reserved for
system failures"):

- `outcome="success"` — INFO.
- `outcome="failure"`, `reason="ctx_canceled"` — INFO. Expected during
  shutdown.
- `outcome="failure"`, returned error — WARN. Expected, retriable in a
  durable transport, dropped here.
- `outcome="failure"`, `reason="panic"` — ERROR. A subscriber panic IS a
  system failure and must page.
- `outcome="dropped"`, `reason="ingress_full"` — WARN. Backpressure
  signal.

Terraform-side log-based metrics (added in a follow-up to #1613, NOT in
this work) extract from the field schema above. The exact metric and
alert names live in the follow-up issue; the contract this work commits
to is the field schema itself, which is stable.

This puts the metrics-shape decisions exactly where #1613 puts everything
else — in the structured log line — and avoids reintroducing a Prometheus
dependency that's actively being removed.

#### G14. Subscriber dispatch context propagation

Today's `community.RecordCommunityEventAndNotify` spawns the
notification goroutine with `context.Background()` plus a fresh timeout
(`server/community/events.go:77-79`). That detaches from the request
lifetime correctly, but it also **drops `request_id`** — the fan-out
log lines aren't joinable to the originating RPC.

`docs/server/observability.md` § "Streams and per-event correlation"
explicitly says the opposite is correct: "When fanning out an event to
multiple subscribers […], pass the originating RPC's `ctx` into the
broadcaster so the fan-out log lines carry the *originator's*
`request_id`."

**Fix:** the dispatch ctx is detached from request lifetime AND
preserves `request_id`. `MemTopic` does this once per dispatch:

```go
// In pubsub.MemTopic.dispatch:
publishCtx := ctx  // for panic-recovery logging via logging.GoSafe
logging.GoSafe(publishCtx, "pubsub-dispatch-"+sub.Name(), func() {
    // Detach from request lifetime so a cancelled HTTP request doesn't
    // kill in-flight dispatch, but PRESERVE request_id so fan-out log
    // lines stay joinable to the originating RPC. Same pattern as
    // services/chat/streaming.go#broadcastMessage.
    base := context.Background()
    if rid := logging.RequestIDFromContext(publishCtx); rid != "" {
        base = logging.WithRequestID(base, rid)
    }
    // (Optionally also propagate user_id here — confirm during PR 1
    // whether logging exposes a similar helper; if not, file a tiny
    // follow-up for symmetry.)
    dispatchCtx, cancel := context.WithTimeout(base, subscriberTimeout)
    defer cancel()
    handle(dispatchCtx, sub, evt)
})
```

Each subscriber's `Handle` then logs via
`logging.LoggerWithContext(ctx)` and inherits `request_id` automatically
— matching the streams convention. This is a strict improvement over
today's behavior, where notification log lines lose the
originating-RPC correlation.

#### G15. Goroutine name + recover discipline

Per `docs/server/architecture.md` § "Goroutine Safety", every
fire-and-forget goroutine in `server/` must be wrapped in
`logging.GoSafe(ctx, "name", fn)` and is enforced by the
`server/cmd/check-goroutines` CI gate.

**Fix:** `MemTopic`'s dispatch goroutine uses
`logging.GoSafe(ctx, "pubsub-dispatch-"+sub.Name(), ...)`. The unit test
file goes through `npm run lint:go` like everything else; no allowlist
addition needed (this isn't the special-cased streaming sender).

#### G16. Emit-site error wrapping discipline

Per `docs/server/architecture.md` § "Error wrapping", emit sites that
return `connect.CodeInternal` errors must use
`connecterr.Internal(ctx, op, err, kv...)`, not bare
`connect.NewError`. PR 3 must preserve this — the
`bus.Publish(...)` call replaces a bare function call but the emitter's
error wrapping should match today's pattern at every call site.

Concretely, today's pattern at `services/transfer/lifecycle.go:516-523`:

```go
if err := community.RecordCommunityEventAndNotify(ctx, s.storage, event, s.notificationService, s.notificationDone); err != nil {
    logger.ErrorContext(ctx, "failed to record event for community", ...)
    return "", err
}
```

… becomes:

```go
if _, err := s.bus.Publish(ctx, event); err != nil {
    logger.ErrorContext(ctx, "failed to publish community event", ...)
    return "", err
}
```

The returned error is a wrapped storage error from the publisher; the
caller's existing
`return nil, connecterr.Internal(ctx, "OpName", err, ...)` lines stay.

#### G6. Idempotency requirement is implicit

Once any subscriber moves to a durable transport, that transport will
retry; if the subscriber isn't idempotent, retries cause duplicate
notifications, duplicate stories, double-counted equity.

**Fix:** Idempotency is a documented contract on `pubsub.Subscriber[T]`
and stated in each subscriber's README. Each subscriber's tests include an
idempotency case (handle the same event twice; side effect once). For the
subscribers we ship in this work:

- **Notification:** push delivery is naturally non-idempotent (FCM does
  not de-dupe). Today's at-most-once gives us "best-effort, no double
  pushes." Documented constraint: **don't migrate notifications to durable
  transport without first adding an `event_id`-keyed dedup table.**
  Notifications stay at-most-once for the foreseeable future.
- **Stream broadcast:** trivially idempotent (re-sending an event ID
  downstream is a no-op the client filters).
- **Story (PR 4):** today's inline call already de-dupes by
  `RelatedEntityID` inside `story.Generator`. Carries forward.
- **Equity (future):** uses publishing `event_id` as the idempotency
  key. Documented when that subscriber ships.

#### G7. No replay / backfill story for new subscribers

When equity computation ships, that subscriber needs to process the
historical event log, not just events from "now."

**Fix:** Document the contract: the `CommunityEvent` table IS the
audit log. New durable subscribers paginate that table at startup and
in steady state, with a last-processed cursor stored per-subscriber
(e.g. `models.SubscriberCursor{subscriber_name, last_event_id,
last_event_at_unix_sec}`). Cursor table ships with the first subscriber
that needs it (probably equity), NOT in this work.

#### G8. v1 phases are hard to review

v1 Phase 2 alone moves ~600 lines into a new package and edits 4–6 test
files. v1 Phase 3 edits 33 emit sites across 6 services. Each is too big
to review in one go.

**Fix:** v2 splits into four shippable PRs (see
[Implementation Plan](#implementation-plan)). Each PR is a coherent
checkpoint that compiles, passes tests, and produces no behavior change
visible to clients.

#### G9. Phase 4 cleanup is mis-scoped

v1 Phase 4 says "delete `events.go` if it has no remaining function." But
several helpers in `server/community/notifications.go` are reused by the
chat service (`services/chat/notifications.go:33,58,75,93`) — they're
domain helpers, not dispatch.

**Fix:** v2 explicitly states which functions stay and which move:

- **Stay in `server/community/`:** `IsActive`, `GetCommunityMemberIDs`,
  `CategoryFor`, `CategoryEnabled`, `FetchPreferencesForUsers`,
  `firesForDeletedCommunity`, `regions.go`, `membership.go`, `constants.go`.
- **Move to the notification subscriber package:**
  `getNotificationRecipients`, `notifyTargetedUsers`, `buildNotification`,
  `getGearOwnerID`, `getTransferRecipientID`, `getRequestCreatorID`,
  `getRequestOfferers`, `getUserName`, `getGearName`, `getExperienceName`,
  `getCommunityName`, `firstName`, `getRequestTitle`, `shouldNotify`.
- **Delete:** `server/community/events.go` entirely (the package-level
  globals it defines are gone; the publisher is `server/community_event_bus`).

#### G10. v1 doesn't sketch the named future subscribers

The issue body originally named "transition notifications, equity
computation, skill processing." Skills are out (the skill system has been
removed from the codebase). The remaining two need a sanity check that the
proposed `Subscriber` interface fits them.

**Fix:** v2 sketches each in [Future Subscribers](#future-subscribers).

#### G11. v1 misses other obvious event-family candidates

The user asked us to look for "other opportunities to reuse the pub/sub
interface." v1 doesn't survey them. v2 does, in
[Future reuse opportunities](#future-reuse-opportunities). The headline
candidates are chat-message events (replaces a chunk of
`services/chat/notifications.go`) and gear-lifecycle events (replaces
several inline embedding-refresh / search-index calls). v2 doesn't
migrate these; the design supports them as separate follow-ups.

#### G12. Per-publish entity-fetch fan-out

After the refactor, every subscriber that needs the gear/transfer/user
referenced by an event would re-fetch it from storage. Today the fetch
happens once inside `notifyTargetedUsers`; tomorrow notification + stream
+ story could each fetch the same gear. With multiple subscribers this is
an N+1 multiplier — the exact pattern that PR #6 of the SQL refactor
just paid down (`docs/ai/sql_improvement_plan.md`).

**Fix:** Introduce a per-publish payload that carries pre-fetched
entities. The publisher fetches once based on which `Topic` field is set
on the event; subscribers consume the payload directly:

```go
// Subscribers receive PublishedEvent (NOT the bare CommunityEvent).
type PublishedEvent struct {
    Event      *models.CommunityEvent

    // Pre-fetched denormalized context. Each may be nil for events that
    // don't reference the corresponding entity. The publisher populates
    // these based on the event's Topic / GearId fields.
    Gear       *models.Gear
    Transfer   *models.Transfer
    Request    *models.Request
    Experience *models.Experience
    Actor      *api.User
}
```

Subscribers that don't need a field ignore it; subscribers that do skip
the storage round trip. This is a meaningful win over v1's silent
implication that each subscriber re-fetches.

#### G13. Soft-delete gate placement

v1 puts the gate in the notification subscriber, not the publisher. Sound
reasoning, but v1 doesn't say what other subscribers do with the gate.

**Fix:** Each subscriber declares its soft-delete behavior in its README:

- **Notification subscriber:** gates on `IsActive(communityID)` AND
  `!firesForDeletedCommunity(event_type)`. Same as today.
- **Stream subscriber:** no gate. Streams self-clean when communities
  are soft-deleted via the existing membership cascade; dispatching to
  no subscribers is a no-op.
- **Story subscriber (PR 4):** gates on `IsActive`. Today's inline call
  doesn't explicitly gate, but the upstream lifecycle paths do; the
  subscriber preserves current behavior.
- **Future subscribers:** declare their gate at construction time.

## Naming

`server/community_event_bus` matches the user's preferred naming
(per the round-2 review): explicit and unambiguous. Imports use a short
alias to avoid awkward call sites:

```go
import cebus "type2fun.org/ripls/server/community_event_bus"
// cebus.Publisher
// cebus.NewInProcessBus(...)
// cebus.PublishedEvent
```

The `_bus` suffix matches the pattern other internal packages already use
underscores for (`server/impact_metrics`). When other event families ship,
they take parallel names: `server/chat_event_bus`, `server/gear_event_bus`,
etc. Each is a thin domain adapter over the same `server/pubsub`
foundation.

## Durable outbox: explanation & recommendation

The user asked us to consider whether to build a durable outbox into this
design. This section explains what an outbox is, what it would buy us,
what it would cost, and why this plan defers it.

### What is a durable outbox

The transactional outbox pattern is a way to make pub/sub crash-safe and
multi-replica safe:

1. The emitter writes the domain change AND a row representing the event
   to an "outbox" table inside the **same database transaction**. Either
   both succeed or neither does.
2. A separate dispatcher process polls the outbox for un-dispatched rows,
   fans them out to subscribers, and marks each row dispatched-per-subscriber.
3. On failure (subscriber returns error, FCM call times out), the
   dispatcher retries on the next poll cycle with exponential backoff.
4. Multi-replica safety comes from `SELECT ... FOR UPDATE SKIP LOCKED` so
   only one dispatcher claims a row at a time.

Properties:

- **At-least-once delivery.** Subscribers must be idempotent.
- **Crash-safe.** If the server crashes after the DB commit but before
  dispatch, the next poll picks the row up.
- **Multi-replica safe.** No more "events fire on replica A but the stream
  is on replica B" problem.

### What it would buy us today

- **Crash safety for push notifications.** Today's flow inserts the
  `CommunityEvent` row, then fires the push asynchronously via
  `logging.GoSafe` with a 30s timeout. If the server crashes during that
  window, the push is lost. With outbox, the next dispatcher cycle would
  retry it.
- **Multi-replica readiness.** Today's stream broadcaster is in-process;
  events fired on replica A don't reach streams on replica B. The outbox
  doesn't directly fix this for streams (they need sub-second delivery,
  which polling can't satisfy), but it does fix it for non-realtime
  subscribers (notifications, story creation, equity).
- **Retry on flaky external calls.** FCM occasionally returns transient
  errors; today we drop the push. Outbox retries it.
- **A clean home for equity.** Equity computation MUST be at-least-once
  (don't double-count, don't miss). It needs durability from day one.

### What it would cost

- **Schema migration.** Either a new `community_event_dispatch{event_id,
  subscriber, dispatched_at_unix_sec, attempt_count}` table, or a
  `dispatch_status` column on `community_event` plus a
  `community_event_dispatch_subscribers` map column. Tradeoffs in either
  direction.
- **Polling latency.** Even at 1s poll intervals, the user-visible push
  latency goes from ~50ms to ~500ms p50. Acceptable for push but
  unacceptable for streams; we'd need to keep the in-process bus for
  streams in addition to the outbox dispatcher.
- **Dispatcher process.** A long-lived goroutine (or, multi-replica,
  a `SELECT ... FOR UPDATE SKIP LOCKED` claim loop) needs careful health
  management, lag metrics, and DLQ semantics.
- **Test surface.** All subscriber tests need to exercise the dispatch
  path AND the persistence path; idempotency tests become non-optional.
- **More code.** Roughly 2–3× the v2 PR-1 size to ship the outbox shell
  on top of the in-memory bus.

### Critical observation: we already have most of an outbox

The `CommunityEvent` table is the durable log. Today's flow already
writes the row before dispatch fires. We DO NOT need a separate outbox
table — we just need a poller and per-subscriber cursors. That's
genuinely a small amount of additional infrastructure: a
`subscriber_cursor{subscriber_name, last_event_id_processed,
last_processed_at_unix_sec}` table and a poller goroutine.

### Recommendation: defer to a follow-up, design for clean swap-in

Build the in-process bus now (PRs 1–4). Do NOT build the durable poller.

Reasons:

1. **No call-volume motivation.** Push delivery is best-effort by design;
   users tolerate occasional misses. Streams are sub-second by spec and
   can't run through a poller anyway.
2. **No multi-replica deployment today.** Cloud Run runs a single
   instance for ripls (community size). The streams already won't survive
   multi-replica scaling without separate work.
3. **The architecture is durability-ready.** Subscribers must be
   idempotent (G6). The `Subscriber[T]` interface is the same shape for
   in-process and durable transports. The CommunityEvent table is already
   the persistent log. Adding the cursor table + poller in a future PR
   doesn't require touching emitters, the publisher, or other subscribers.
4. **The first durable subscriber is equity, which is P4 / icebox.** The
   work that motivates durability hasn't been prioritized; building the
   infrastructure ahead of the first consumer is YAGNI.

What v2 commits to in this work, to make the future swap clean:

- The CommunityEvent table is the canonical event log. The publisher
  always writes the row inline; the in-process fan-out is purely a
  delivery optimization.
- The `pubsub.Subscriber[T]` contract requires idempotency.
- The durable upgrade path is documented in `pubsub/README.md`: a
  durable subscriber paginates `community_event` by
  `(occurred_at_unix_sec, id)`, tracks its cursor in a
  `subscriber_cursor` row, and uses the publishing `event_id` as its
  idempotency key.

If, during implementation, we discover a concrete reliability issue
that durability would solve (e.g. a real production push-loss
incident), we revisit. Otherwise the durable poller ships with the
first durable subscriber as a separate issue.

## Problem Statement

(Same as v1 — restated so v2 is self-contained.)

Today, every write-side RPC that affects community state calls
`community.RecordCommunityEventAndNotify` in `server/community/events.go`.
That function does five things at once:

1. Inserts the `CommunityEvent` storage row.
2. Broadcasts the event to active `StreamCommunityEvents` subscribers via
   the package-level `eventBroadcaster` global.
3. Decides who to notify based on `event_type`
   (`getNotificationRecipients`).
4. Resolves per-user `CommunityNotificationPreferences` and gates by
   category.
5. Builds a localized `Notification` payload and dispatches via the
   `notifications.Service`.

Every emitter (transfer, request, experience, login, gear-sharing,
invitations) imports `server/community` to call this function. The
community library is the de-facto dispatch hub. The maintainer's
2026-05-10 issue comment requires us to **sever those dependencies** —
the community service should shrink to a regular subscriber, not the place
every emitter routes through.

## Recommendation

**Option A: in-process publisher with explicit subscriber registration,
implemented as a generic `pubsub` library with a `community_event_bus`
domain adapter on top.**

This satisfies every constraint in v1 plus the v2 audit findings:

- The community service stops owning dispatch — it becomes one subscriber.
- The publisher is reusable for other event families (chat, gear, user).
- Delivery semantics, idempotency, backpressure, observability, and
  replay are explicit contracts.
- Each migration step ships in its own PR.

This aligns with `docs/server/architecture.md`:

- **Service Independence (§ Core Design Principles 1):** "Services should
  not call or depend on other services." Today every emitter imports
  `server/community`; after this work, every emitter depends on
  `server/community_event_bus.Publisher` (an interface). The community
  library no longer participates in cross-service dispatch.
- **Shared Functionality in Libraries (§ 2):** `server/pubsub` is a clean
  shared library — single responsibility, comprehensive unit tests,
  minimal API surface, interface-based for testability.
  `server/community_event_bus` is the same pattern at the domain-adapter
  layer.
- **Short Focused Files (§ 3):** The current 900-line
  `server/community/notifications.go` splits into 4–5 files inside the
  notification subscriber, each <500 lines.

## Architecture

### Two-layer design

```
┌───────────────────────────────────────────────────────────────┐
│ server/pubsub                                                 │
│   - Topic[T] interface (Subscribe, Publish, Drain)            │
│   - Subscriber[T] interface (Name, Handle)                    │
│   - MemTopic[T]: in-memory bounded-channel implementation     │
│   - Test helpers: MockTopic[T], FakeSubscriber[T]             │
│                                                               │
│   No domain knowledge. No storage. Pure infrastructure.       │
└───────────────────────────────────────────────────────────────┘
                              ▲
                              │ uses
                              │
┌───────────────────────────────────────────────────────────────┐
│ server/community_event_bus  (alias: cebus)                    │
│   - Publisher interface  (Publish(ctx, *CommunityEvent) ...)  │
│   - PublishedEvent struct (event + prefetched denorm context) │
│   - InProcessBus: concrete Publisher implementation           │
│       - Inserts CommunityEvent row                            │
│       - Pre-fetches gear/transfer/request/experience/actor    │
│       - Forwards PublishedEvent into pubsub.MemTopic          │
│   - Mock/Fake for emitter unit tests                          │
└───────────────────────────────────────────────────────────────┘
                              ▲
                              │ subscribes via the topic
                              │
┌────────────────────────┐ ┌──────────────────────┐ ┌──────────┐
│ notifications/         │ │ services/community/  │ │ story/   │
│ community_subscriber/  │ │ stream_subscriber.go │ │ subscriber│
│ - HandleEvent          │ │ - HandleEvent        │ │ (PR 4)    │
│ - Soft-delete gate     │ │ - Per-stream fan-out │ │           │
│ - Recipients/copy/etc  │ │                      │ │           │
└────────────────────────┘ └──────────────────────┘ └──────────┘
```

### `server/pubsub` interface

```go
// Package pubsub provides a generic in-process publish/subscribe primitive
// over typed events. Subscribers consume serially per-subscriber; the
// publisher fans out to all subscribers concurrently. Default semantics
// are at-most-once, in-memory, with bounded-channel backpressure.
package pubsub

// Topic is the publish/subscribe surface for events of type T.
// In-memory implementations live in this package; durable
// implementations may live in server/storage or a sibling package.
type Topic[T any] interface {
    Subscribe(s Subscriber[T]) (Unsubscribe, error)
    Publish(ctx context.Context, event T) error
    Drain(ctx context.Context) error
}

// Subscriber consumes events of type T. Implementations MUST be idempotent
// (the same event may be delivered more than once if the topic is later
// upgraded to a durable transport).
type Subscriber[T any] interface {
    Name() string
    Handle(ctx context.Context, event T) error
}

type Unsubscribe func()
```

### `server/community_event_bus` interface

```go
package community_event_bus

// Publisher is the surface emit sites depend on. The interface is
// concrete (not parametric) so emitters keep their dependencies simple.
type Publisher interface {
    // Publish persists the event row, pre-fetches denormalized context,
    // and dispatches to every subscriber asynchronously. Returns the
    // inserted event ID. Subscriber failures do NOT propagate up.
    Publish(ctx context.Context, event *models.CommunityEvent) (eventID string, err error)
}

// PublishedEvent is what subscribers receive. Pre-fetched fields are
// populated based on which Topic field is set on the event.
type PublishedEvent struct {
    Event      *models.CommunityEvent
    Gear       *models.Gear        // may be nil
    Transfer   *models.Transfer    // may be nil
    Request    *models.Request     // may be nil
    Experience *models.Experience  // may be nil
    Actor      *api.User           // may be nil
}

type Subscriber = pubsub.Subscriber[*PublishedEvent]
```

`InProcessBus` is the concrete implementation. Its constructor takes
`*storage.ProtoSQLStorage` (for the insert + pre-fetch) and a
`pubsub.Topic[*PublishedEvent]` (so tests can inject a `MockTopic`).

### Failure modes and observability

All observability flows through structured logs (no Prometheus metrics,
per #1613/#1614). Each dispatch outcome emits one log line; Cloud
Monitoring's log-based metrics extract from `jsonPayload.duration_ms`
and count by `outcome` for SLI/SLO and alerting. Field naming follows
`docs/server/observability.md` § "Standard Field Names" and § "Streams
and per-event correlation" — `community_event_id` (NOT `event_id`) for
the entity ID, `request_id` reserved for HTTP correlation and inherited
via `logging.LoggerWithContext`.

| Failure mode | Level | What happens | Log shape |
|---|---|---|---|
| Storage insert fails | (caller's existing path) | `Publish` returns error to caller; emitter handles | Caller logs at ERROR with `connecterr.Internal` |
| Subscriber `Handle` returns error | WARN | One dispatch line; counted by log-based metric | `outcome="failure"`, `error="..."` |
| Subscriber panics | ERROR | `logging.GoSafe` recovers; pages via #1613 alerting | `outcome="failure"`, `reason="panic"`, `panic="..."`, `stack="..."` |
| Subscriber ingress full | WARN | Event dropped for that subscriber | `outcome="dropped"`, `reason="ingress_full"` |
| ctx canceled mid-dispatch | INFO | Expected during shutdown; dispatch may not finish | `outcome="failure"`, `reason="ctx_canceled"` |

Alerting (terraform follow-up to #1613): page on
`reason="panic"` (any rate) AND on `outcome="failure"` rate > 0 for
any subscriber sustained 5m; page on `outcome="dropped"` rate > 0
sustained 1m.

## Open Questions

These survive from v1 with refined recommendations.

- **Should the publisher insert the storage row, or do emitters insert
  and then publish?** — Recommendation: **publisher inserts**, same as
  v1. Locking the contract prevents emitters from forgetting and
  preserves the audit-trail invariant. Plus the publisher is now also
  the place that pre-fetches denormalized context (G12) — inserting is
  the natural point.

- **Where does the soft-delete gate live?** — Recommendation:
  **per-subscriber** (G13).

- **How do subscribers signal completion to tests?** — Recommendation:
  **`bus.Drain(ctx)`** instead of v1's per-call `done` channel (G2).

- **Synchronous publisher API or fully async?** — Recommendation:
  **synchronous insert + pre-fetch, async per-subscriber fan-out**.

- **Story creation: subscriber or kept as-is?** — Recommendation:
  **subscriber, in PR 4**.

- **Do `experience` planning events and `chat` system messages move
  to subscribers too?** — Recommendation: **planning events: yes via
  PR 3 emit-site migration; chat system messages: no, ever** (chat
  system messages are a precondition of the event, not a reaction).

- **Should we ship a generic `pubsub` library now even though
  `community_event_bus` is the only consumer?** — Recommendation: **yes**.
  The two-layer structure costs ~150 lines of generic code in exchange
  for tractable chat/gear/user-lifecycle migrations later.

- **Where do `models.SubscriberCursor` and other replay/backfill
  primitives live?** — Recommendation: **don't build them in this work**
  (G7). Documented contract is enough until the first durable subscriber
  ships.

- **How do we test that a new event type is handled by the right
  subscribers?** — Recommendation: **per-subscriber switch + test**.
  Each subscriber's `Handle` has its own `event_type` switch; each has
  its own `TestEverySubscribedEventTypeHasHandler` mirroring the
  existing `TestEveryEventTypeClassified` (CLAUDE.md § "Adding New
  CommunityEventType Values").

## Implementation Plan

Four PRs. Each is independently shippable, compiles, passes the test
suite, and produces no client-visible behavior change. Update the
[Status table](#status) as each lands.

### PR 1: Foundation — `pubsub` library and `community_event_bus` skeleton

**Goal:** Land the generic primitive AND the domain adapter, both with
zero production callers. This bundles v1's would-be PR1 + PR2 because
they're each small (~400 lines) and reviewing them together makes the
contract obvious — the generic shape is meaningless without the concrete
adapter showing how it's used.

**Files (new):**
- `server/pubsub/` — `doc.go`, `README.md`, `pubsub.go` (interfaces),
  `memtopic.go` (impl), `mock.go`, `memtopic_test.go`. README documents
  delivery semantics, idempotency contract, backpressure model, and the
  durable upgrade path.
- `server/community_event_bus/` — `doc.go`, `README.md`,
  `community_event_bus.go` (`Publisher`, `PublishedEvent`, `Subscriber`
  alias), `inprocess.go` (`InProcessBus`), `prefetch.go` (entity
  pre-fetch), `mock.go`, `inprocess_test.go`. README documents per-
  subscriber soft-delete-gate expectations.

**Wiring in `main.go`:** construct the bus next to `notificationService`
but don't pass it to any emitter yet:

```go
publishedTopic := pubsub.NewMemTopic[*cebus.PublishedEvent]("community_events")
bus := cebus.NewInProcessBus(sqlStorage, publishedTopic)
_ = bus  // unused until PR 2
```

**Tests cover:** publish + dispatch happy path; multi-subscriber
isolation (one bad subscriber doesn't poison others); panic recovery;
backpressure (full channel drops + log line); ctx cancellation; ordering
within a subscriber; pre-fetch correctness for each topic field; insert-
fails-no-dispatch; `Drain` correctness.

**Validates:** Build clean; no behavior change. Sets up the contract.

### PR 2: Migrate dispatch into subscribers + slim community lib

**Goal:** Carve `notifyTargetedUsers` and the stream broadcaster out of
their current locations into proper subscribers. Wire both subscribers
to the bus from PR 1. The legacy `community.RecordCommunityEventAndNotify`
keeps working — internally it now constructs an event, calls the bus,
and the bus dispatches. **No emitter changes; no client-visible
behavior change.**

This bundles v1's PR3 + PR4 + the slim-down step that v1 had as PR6.
Bundling makes sense because:
- Both subscribers ship together so the bus has all its production
  consumers from day one of dispatch migration.
- The community library shrink is a direct consequence of the
  notification carve-out; doing it in the same PR keeps the diff
  reviewable as one coherent move.

**Files (new):**
```
server/notifications/community_subscriber/
├── doc.go
├── README.md
├── subscriber.go       # Subscriber struct + Name() + Handle() + soft-delete gate
├── recipients.go       # getNotificationRecipients() + per-event helpers
├── copy.go             # buildNotification() + per-event copy
├── preferences.go      # category-enable check (delegates to community.CategoryEnabled)
└── *_test.go
```

**Files (modified):**
- `server/services/community/stream_subscriber.go` (NEW): hosts
  `eventStreams`, `eventStreamsMu`, `BroadcastCommunityEvent`,
  `HasActiveEventStream`, `registerEventStream`, `unregisterEventStream`,
  `communityEventToItem` (moved from `service.go`/streaming files).
  Adds `(s *Service) StreamSubscriber() cebus.Subscriber`.
- `server/community/notifications.go`: shrinks from 900 → ~250 lines.
  Keeps the cross-cutting helpers (`IsActive`, `GetCommunityMemberIDs`,
  `CategoryFor`, `CategoryEnabled`, `FetchPreferencesForUsers`,
  `firesForDeletedCommunity`).
- `server/community/events.go`: replaced with a transitional shim that
  takes a package-level `Publisher` set in `SetPublisher(bus)` from
  `main.go`. Body forwards to `bus.Publish(...)`. Tests that don't
  configure a publisher hit the legacy fallback (preserved verbatim
  inside this file) so they continue to pass.

**Files (deleted):** `server/community/events_test.go` for the legacy
fallback's tests is collapsed; the bus's own tests cover the new path.

**Wiring in `main.go`:**

```go
notifSub := commsub.New(sqlStorage, notificationService, nil /* streamChecker bound below */, community.FetchPreferencesForUsers)
unsubNotif, err := publishedTopic.Subscribe(notifSub)
defer unsubNotif()
unsubStream, err := publishedTopic.Subscribe(communityService.StreamSubscriber())
defer unsubStream()
notifSub.SetStreamChecker(communityService.HasActiveEventStream)  // late-bind
community.SetPublisher(bus)  // legacy shim entry point
```

**Test migration:** existing tests in `community/notifications_test.go`,
`community/events_test.go`, `services/community/notifications_test.go`,
`services/community/streaming_test.go` are updated to:
- Construct an `InProcessBus` with the notification + stream subscribers
  registered.
- Call `bus.Publish(...)` and assert via `bus.Drain(ctx)`.
- Reuse `notifications/noop` and `notifications/mock` per
  architecture-doc § "Mock Strategy".

**Validates:** All existing tests pass. `community/notifications.go`
shrinks from 900 → ~250 lines. New subscriber package is the single
home of dispatch logic. Two subscribers (notifications + stream) both
running on the bus in production.

### PR 3: Migrate emit sites + delete legacy shim

**Goal:** Inject `cebus.Publisher` into emitter constructors; replace
every `community.RecordCommunityEventAndNotify` call site with
`s.bus.Publish(ctx, event)`. After this PR, `community.RecordCommunityEventAndNotify`,
`community.SetPublisher`, and the legacy fallback are all deleted.
`server/community/events.go` no longer exists.

**Constructor changes:**
- `community.New` → add `bus cebus.Publisher` parameter.
- `experience.New` → ditto.
- `transfer.New` → ditto.
- `request.New` → ditto.
- `login.New` (or `login.Config`) → ditto.

**Call-site migrations (~33 sites across 11 files):**
- `server/services/community/lifecycle.go` (8 sites)
- `server/services/community/invitations.go` (1)
- `server/services/community/gear_sharing.go` (2)
- `server/services/experience/{events,lifecycle,planning_events,share,undo}.go` (~7)
- `server/services/request/{events,undo}.go` (~5)
- `server/services/transfer/{lifecycle,interest,undo}.go` (~7)
- `server/services/login/registration.go` (1)

The signature swap is mechanical:
```go
// Before
err := community.RecordCommunityEventAndNotify(ctx, s.storage, event, s.notificationService, s.notificationDone)
// After
_, err := s.bus.Publish(ctx, event)
```

`s.notificationDone` is removed from emitter structs — tests use
`bus.Drain(ctx)` instead.

**Test updates:** every emitter test that constructs its service updates
the constructor to pass `cebus.NewMockBus()` (or, where the test
asserts side effects, `cebus.NewInProcessBus(storage, pubsub.NewMemTopic(...))`
with stub subscribers).

**Cleanup at the end of this PR:**
- Delete `server/community/events.go`.
- Delete the transitional shim from PR 2.
- Confirm `git grep RecordCommunityEventAndNotify` returns nothing in
  production code.
- Update `server/community/doc.go` and add `server/community/README.md`
  describing the slimmed scope.
- Update `server/services/community/README.md` to describe the service
  as both an emitter (lifecycle/invitations/gear_sharing) and a
  subscriber (stream broadcast).

**Validates:** Build clean; full test suite passes including
`go build -tags=integration ./...`; no production caller of the legacy
function remains; `git grep "server/community\"" server/services/`
returns only the slim-helper imports (`IsActive`, `GetCommunityMemberIDs`,
preferences, regions, membership). **This PR closes the architectural
goal of #510** — community service no longer owns dispatch.

### PR 4: Story-creation subscriber (closes #510)

**Goal:** Eliminate `story.Creator` injection into community, transfer,
request, experience services by making story creation a subscriber.
This is the v1 Phase 5 work. Bundling it into the closing PR keeps the
"community service as subscriber, not dispatch hub" thesis tight by
removing the last piece of cross-service injection that the dispatch
machinery enabled.

**New package:** `server/story/subscriber/`. The subscriber listens for
the events that today trigger inline `CreateStory` calls:

- `COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED` (loan + giveaway) →
  `storyCreator.CreateStory({StoryType: "transfer", ...})`
- `COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED` →
  `storyCreator.CreateStory({StoryType: "request", ...})`
- `COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED` →
  `storyCreator.CreateStory({StoryType: "experience", ...})`
- `COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED` (audit current behavior;
  add only if today's code creates a story for this event).

**Constructor signatures cleanup:**
- `community.New` no longer takes `story.Creator`.
- `transfer.New` no longer takes `story.Creator`.
- `request.New` no longer takes `story.Creator`.
- `experience.SetStoryCreator` removed.

**Story payload assembly:** today's lifecycle code builds the
`CreateStoryRequest` inline with denormalized fields (gear name,
community name, etc.). The subscriber needs the same denorm context —
exactly what `PublishedEvent` carries (G12). The subscriber reads
`evt.Gear`, `evt.Transfer`, `evt.Request`, `evt.Experience` from the
payload and constructs the request without re-fetching.

**Idempotency:** today's `CreateStory` already de-dupes by
`RelatedEntityID` inside `story.Generator`. The subscriber inherits
this. Documented in subscriber README.

**Soft-delete gate:** subscriber checks `community.IsActive(...)` before
creating, matching today's behavior.

**Tests:** existing `*_test.go` cases that asserted `MockStoryCreator`
saw a `CreateStoryRequest` migrate to the subscriber's tests. Lifecycle
tests that just want the side-effect to happen call `bus.Drain(ctx)`
and inspect the mock.

**Validates:** `git grep "story.Creator" server/services/` returns only
constructor cleanup. Stories still get created for the same set of
events (verified by integration tests in
`services/transfer/lifecycle_test.go`,
`services/request/lifecycle_test.go`,
`services/experience/lifecycle_test.go`).

This **closes #510**. The community service is no longer the dispatch
hub; emitters depend on `cebus.Publisher` only; new subscribers
(equity, transition reminders, future) plug into the same bus without
touching emitters.

## Future Subscribers

Sanity-check: each named future subscriber fits the proposed `Subscriber`
interface.

### Transition notifications

- **Trigger model:** time-based, not event-reaction. "Your loan is
  ending tomorrow" fires on a schedule, not in response to a
  `TRANSFER_ACTIVE` event.
- **Fit:** **NOT a `pubsub` subscriber on the community bus.** Belongs
  in `server/jobs/` like `community_purge_reminder`. The community-event
  log might be the trigger source (a job reads recent `TRANSFER_ACTIVE`
  events to find loans that are now near their end-date), but the
  dispatch is scheduled, not reactive.
- **Status:** flagged in the issue body but the wrong abstraction.
  A future `gear_event_bus` may grow a "loan-due-soon" event from a
  job; that event then drives notifications via the existing
  notification subscriber pattern.

### Equity computation

- **Trigger model:** every `TRANSFER_COMPLETED`, `REQUEST_FULFILLED`,
  `EXPERIENCE_COMPLETED` updates a per-user/per-community equity score.
- **Fit:** **YES, community-event subscriber.** Needs at-least-once
  durability; ships with the durable cursor pattern documented in G7.
- **Idempotency key:** `event_id`. Subscriber stores
  `{event_id, processed_at}` per-row in a `subscriber_cursor` table;
  a second delivery is a no-op.
- **Storage:** `models.UserCommunityEquity{user_id, community_id,
  score, last_event_id, last_event_at_unix_sec}`. Updated
  transactionally on every event.

(Skill processing is removed — the skill system has been deleted from
the codebase.)

The interface admits both. v2's design holds.

## Future reuse opportunities

User asked for "other opportunities" beyond the named subscribers.
Ranked by decoupling payoff:

| Family | Today's coupling | Payoff if migrated | Recommended timing |
|---|---|---|---|
| Chat messages | `services/chat/notifications.go` (190 lines) and `services/chat/messages.go` broadcasts inline | Removes `chat→community` import for soft-delete gate (kept) and recipient lookup (moves to bus); chat service shrinks. Same observability and idempotency wins as community-event work. | After #510 closes; file as separate issue. New package `server/chat_event_bus`. |
| Gear lifecycle | `services/gear/save.go` calls embedding refresh + impact recompute inline | Decouples search/embedding/impact from gear writes; enables future hooks (workshop pre-warm, equity over gear lifecycle). | After chat. New package `server/gear_event_bus`. |
| User lifecycle | `services/login/registration.go` emits one `INVITATION_LINK_USED` community event today; welcome email + default community membership inline | Welcome email becomes a subscriber; future onboarding hooks (analytics, referral credit). | Lower priority; file when a second consumer appears. |
| Media events | `services/media/upload.go` post-processing inline | Modest payoff today; thumbnails/AI-tagging both run inline. Clear win if we add async media jobs. | Defer until a concrete second consumer appears. |

For #510 specifically: **only do the community-event work.** The
`pubsub` library is intentionally generic so the chat / gear / user
/ media migrations can ship one-at-a-time later, each as its own issue
+ PR, with no foundational changes needed.

## Related Issues

- **#1657, #1658, #1659** — community delete/restore lifecycle. v2
  preserves the `firesForDeletedCommunity` / `IsActive` gate inside the
  notification subscriber. Tests in
  `services/community/streaming_test.go` and
  `community/events_test.go` are the regression coverage.
- **#1622, #1623** — soft-delete gates around notification dispatch and
  async writes. v2 keeps the notification gate in the subscriber and
  the async-write gate in the caller.
- **#1613, #1614** — pivot from Prometheus to log-based metrics. v2's
  observability section (G5) emits structured log fields that Cloud
  Monitoring's log-based metrics can extract; no Prometheus
  instrumentation is added.
- **Issue body sub-tasks** ("transition notifications, equity
  computation, skill processing") — addressed in
  [Future Subscribers](#future-subscribers); transition notifications
  are reclassified as a job, equity ships as a separate issue once
  this work closes, skill processing is dropped (system removed).

## Diff from v1 (summary)

For reviewers comparing v1 → v2:

- Added: two-layer architecture (`pubsub` + `community_event_bus`).
- Added: `PublishedEvent` with pre-fetched denormalized context (G12).
- Added: explicit delivery semantics (G3), backpressure (G4),
  log-based observability schema aligned with #1613 (G5),
  idempotency requirement (G6), durable-replay contract (G7),
  ctx-propagation discipline (G14), goroutine-name discipline (G15),
  emit-site error-wrapping discipline (G16).
- Added: future subscribers register; transition notifications
  reclassified as a job, not a subscriber; skill processing dropped.
- Added: other event families survey.
- Added: durable outbox explanation with explicit defer recommendation.
- Added: status tracking table at the top of the doc.
- Replaced: per-call `done chan` → `bus.Drain(ctx)` (G2).
- Replaced: v1 Phase 2-3 → four shippable PRs.
- Sharpened: G9 community library teardown manifest (which functions
  move, which stay).
- Sharpened: G13 per-subscriber soft-delete gate documentation.
- Renamed: `eventbus` → `community_event_bus` to match its
  domain-specificity.

## Audit notes (changed during /ripls-audit run)

The audit checked v2 against `docs/server/architecture.md` and
`docs/server/observability.md`. Findings folded back into the plan:

- **Field names:** the plan now uses `community_event_id` (not
  `event_id`) for the entity ID in every log shape, matching
  observability.md § "Streams and per-event correlation" — the same
  field the existing community-event stream handlers use to slice
  per-event log lines within a stream's `request_id`. (G5, Failure
  modes table.)
- **`request_id` propagation across fan-out (G14, new):** v1 implicitly
  inherited today's `community/events.go:77-79` shape, which spawns the
  notification goroutine with `context.Background()` plus a fresh
  timeout — that detaches from request lifetime correctly, but it also
  drops the request correlation ID, contradicting observability.md §
  "Streams and per-event correlation." v2 specifies the correct shape:
  detach from request lifetime, preserve `request_id`. This is a
  strict improvement over today's behavior.
- **Log levels per outcome (G5, sharpened):** subscriber-returned
  errors stay WARN (handled, retriable in a future durable transport);
  subscriber panics escalate to ERROR per observability.md § 5
  (system failures); ctx-cancel during shutdown is INFO (expected);
  backpressure drop is WARN.
- **`operation` field values:** specified per log line
  (`pubsub.publish`, `pubsub.dispatch`, plus per-subscriber
  `<subscriber_name>.handle` from each subscriber's logs).
- **Goroutine discipline (G15, new):** explicit reference to
  `logging.GoSafe(ctx, "pubsub-dispatch-"+sub.Name(), ...)` plus the
  `server/cmd/check-goroutines` CI gate. No allowlist additions
  needed.
- **Error wrapping at emit sites (G16, new):** emitters keep their
  existing `connecterr.Internal(ctx, "OpName", err, ...)` wrapping
  pattern; the `bus.Publish` swap is purely the inner call
  replacement. Called out in PR 3 so the migration doesn't
  accidentally regress to bare `connect.NewError(connect.CodeInternal,
  err)`.
- **No client-facing changes:** verified against `docs/client/*` —
  this is server-only work. Client architecture, caching,
  accessibility, i18n, and testing docs impose no plan changes. Push
  notification payloads are unchanged (the subscriber rebuilds the
  same `models.Notification` shape today's
  `buildNotification` produces).
- **Observability.md staleness note:** observability.md § "Future
  Enhancements" still references the now-deprecated Prometheus
  metrics path (a doc cleanup tracked under #1614). v2 ignores the
  stale section and aligns with #1613's log-based direction. No
  changes to observability.md from this work; #1614's cleanup will
  catch the stale section.
