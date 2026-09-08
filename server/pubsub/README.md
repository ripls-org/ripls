# server/pubsub

Generic in-process publish/subscribe over typed events.

This package is **pure infrastructure** — no domain knowledge, no storage, no
proto types. Domain-specific buses (the first being
`server/community_event_bus`) compose `Topic[T]` for their event family. When
chat or gear lifecycle wants the same shape, they add their own thin adapter
on top of the same primitive.

## Contract

The interfaces are defined in [`pubsub.go`](pubsub.go); behavior described
below.

### `Topic[T]`

- `Subscribe` registers a subscriber. The returned `Unsubscribe` is idempotent.
  Subscribers with colliding `Name()` return an error.
- `Publish` enqueues into every subscribed handler's ingress channel. Returns
  `nil` on successful enqueue. Subscribers whose ingress is full have the event
  dropped and a `WARN` log emitted; `Publish` itself does not surface drops.
- `Drain` blocks until every in-flight dispatch on every subscriber finishes
  or `ctx` is done. Tests call this; production never does.

### `Subscriber[T]`

- `Name()` is a stable identifier for logs; must be unique within a `Topic`.
- `Handle` processes a single event. Errors are logged and counted in the
  log-based metric stream; they do **not** trigger retries in `MemTopic`.

### Idempotency requirement

Subscribers MUST be idempotent. The same event may be delivered more than
once if the topic is later upgraded to a durable transport (the
`CommunityEvent` table is the durable log; a future poller-based `Topic`
implementation can replay it).

### Delivery semantics

- **`MemTopic` (this package, today):** at-most-once, async fan-out. A
  subscriber may miss events on process crash, panic, ctx cancellation,
  or ingress-full backpressure drop. Subscribers MUST NOT assume they see
  every event.
- **Future durable transport:** at-least-once, paginating from
  `community_event` with a per-subscriber cursor. Same `Subscriber[T]`
  interface; subscribers don't change.

### Ordering

Within a single subscriber, events arrive in publish order — each
subscriber has its own goroutine and serial worker loop. Across
subscribers there is NO global order; they run in parallel.

### Backpressure

Each subscribed handler gets a bounded ingress channel (default 256, see
`SubscribeOptions.IngressBufferSize`). On full, the publish for that
subscriber is **dropped** with a `WARN` log line — see `outcome="dropped"`
in the [observability schema](#observability) below. Subscribers that
can't tolerate drops opt into a (future) durable transport.

### Goroutine safety

Every fan-out goroutine is launched via `logging.GoSafe`, satisfying the
`server/cmd/check-goroutines` CI gate. The dispatch worker has its own
`defer recover()` so the dispatch log line carries the panic outcome
under a single source of truth; `GoSafe`'s outer recover is left
unused (but registered, for safety).

## Observability

Per `docs/server/observability.md`, no Prometheus instrumentation. Each
publish + dispatch outcome emits one structured log line; Cloud
Monitoring's log-based metrics extract from `jsonPayload.duration_ms`
and count by `outcome`. Field naming follows
`docs/server/observability.md` § "Standard Field Names" and § "Streams
and per-event correlation."

`request_id` and `user_id` are inherited automatically through
`logging.LoggerWithContext(ctx)`. Do NOT set them explicitly.

### Per-published event (INFO)

```text
message="pubsub event published"
operation="pubsub.publish"
topic="<topic name>"
```

If the published event implements `LogContextProvider` (see below), its
returned key/value pairs are also folded in (e.g. `event_type`,
`community_event_id`).

### Per-dispatch attempt

```text
message="pubsub event dispatched"
operation="pubsub.dispatch"
topic="<topic name>"
subscriber="<subscriber name>"
duration_ms=<int>
outcome="success" | "failure" | "dropped"
reason="..."   // present when outcome != "success"
error="..."    // present when outcome == "failure" with a returned error
```

Level per outcome (per observability.md § 5):

| Outcome / reason | Level |
|---|---|
| `success` | INFO |
| `failure` (returned error) | WARN |
| `failure` `reason="ctx_canceled"` | INFO (expected during shutdown) |
| `failure` `reason="panic"` | ERROR (system failure) |
| `dropped` `reason="ingress_full"` | WARN |

### `LogContextProvider`

`LogContextProvider` (see [`pubsub.go`](pubsub.go)) is an optional opt-in for
event types that want to contribute extra structured fields to pubsub log
lines. Events that don't implement it just get the topic-level fields.

`*community_event_bus.PublishedEvent` implements `LogContext()` (see
[`../community_event_bus/community_event_bus.go`](../community_event_bus/community_event_bus.go))
and returns `event_type`, `community_event_id`, `community_id` so the
dispatch line is sliceable in Cloud Logging.

## Context propagation

The dispatch goroutine receives a context that is **detached from the
publishing HTTP request lifetime** (so a cancelled HTTP request doesn't
kill in-flight dispatch) but **preserves `request_id` and `user_id`**
(so fan-out log lines stay joinable to the originating RPC). Same
pattern as `server/services/chat/streaming.go#broadcastMessage`.

The derivation is implemented in `deriveDispatchCtx` in
[`memtopic.go`](memtopic.go). It is a strict improvement over today's
`server/community/events.go:77-79` pattern, which uses bare
`context.Background()` and drops `request_id`.

## Tests

Mocks live in `mock.go` and are reused across consumer-package tests
per `docs/server/architecture.md` § "Mock Strategy".

- `MockTopic[T]` — captures publishes for assertion; no real fan-out.
- `FakeSubscriber[T]` — records received events; configurable error /
  panic / sleep behaviors for testing.
