# server/community_event_bus

CommunityEvent-specific publisher built on `server/pubsub`. Emit sites depend
on `Publisher`; subscribers consume `*PublishedEvent`.

This package is the **first** domain adapter on `server/pubsub`. The intent
is that future event families — chat messages, gear lifecycle, user
lifecycle — get parallel adapters (`server/chat_event_bus`,
`server/gear_event_bus`, etc.) using the same `pubsub` primitive. See
[`docs/issues/510-community-event-pubsub-v2.md`](../../docs/issues/510-community-event-pubsub-v2.md)
for the broader plan.

## Public surface

Defined in [`community_event_bus.go`](community_event_bus.go):

- `Publisher` — what emit sites depend on. Single method that persists +
  dispatches.
- `PublishedEvent` — what subscribers receive. Wraps the bare
  `*models.CommunityEvent` with optional pre-fetched denormalized context
  (`Gear`, `Transfer`, `Request`, `Experience`, `Actor`) so each subscriber
  doesn't re-fetch the same row. Each pointer field may be nil if the event
  doesn't reference the corresponding entity (G12 in the v2 plan). For the
  member-broadcast event types (`SnapshotsMembers`), it also carries
  `MemberIDsAtPublish` — the community's member IDs captured at publish
  time, so the push audience is pinned to membership as of the event
  instead of racing joins that land before the async dispatch (#2657).
  Nil when the type isn't snapshotted or the capture failed; consumers
  fall back to a live lookup.
- `SnapshotsMembers` — predicate for which event types carry the member
  snapshot (the "all community members minus actor" push-audience types).
- `Subscriber` — type alias for `pubsub.Subscriber[*PublishedEvent]`.

## Publisher contract

`Publish(ctx, event)`:

1. Defaults `event.OccurredAtUnixSec` from `clock.UnixSec(ctx)` if zero.
2. **Inserts the storage row** synchronously. Storage failure is
   surfaced to the caller as an error; subscribers are not invoked.
3. **Pre-fetches denormalized context** based on which Topic field is set:
   - `event.GearId` → fetches `*models.Gear` if non-empty.
   - `event.GetTransferId()` → fetches `*models.Transfer` if set.
   - `event.GetRequestId()` → fetches `*models.Request` if set.
   - `event.GetExperienceId()` → fetches `*models.Experience` if set.
   - `event.ActorId` → fetches the API-shaped `*api.User` via
     `services.FetchAPIUser` if non-empty.
   Pre-fetch failures are logged at WARN and the corresponding field is
   left nil. Subscribers MUST handle nil entities gracefully.
4. **Snapshots the member audience** for `SnapshotsMembers` event types:
   the community's `CommunityUser` IDs are captured onto
   `MemberIDsAtPublish` so the notification subscriber's broadcast
   audience is membership as of publish, not as of dispatch (#2657).
   Publishers write their own membership changes before calling
   `Publish`, so the snapshot includes the acting request's writes. A
   failed capture logs at WARN and leaves the field nil (consumers fall
   back to a live lookup).
5. Publishes a `*PublishedEvent` to the underlying `pubsub.Topic`,
   which fans out asynchronously to every registered subscriber.
6. Returns the inserted `event.Id` and the storage error (if any).

## Soft-delete gate placement

The community soft-delete gate (`firesForDeletedCommunity` /
`community.IsActive`) is intentionally **NOT** in the publisher. The
audit-trail row is recorded for every event, including events on
soft-deleted communities. Each subscriber decides how to gate per
its own semantics — see G13 in the v2 plan. Concretely:

- The notification subscriber gates push delivery on `IsActive` AND
  `!firesForDeletedCommunity(event_type)`. Same as today.
- The stream subscriber does not gate (streams self-clean via
  membership cascade; broadcast to no listeners is a no-op).
- Future subscribers declare their gate at construction time.

## Observability

Each `Publish` call emits one INFO log line via the underlying
`pubsub.MemTopic`; each per-subscriber dispatch emits another. Field
naming follows `docs/server/observability.md` § "Standard Field Names":

- `community_event_id` — the row ID. NOT `event_id`.
- `community_id` — the parent community.
- `event_type` — the enum value (e.g.
  `COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE`).
- `topic="community_events"` — the underlying pubsub topic name.
- `request_id` and `user_id` are inherited from
  `logging.LoggerWithContext(ctx)`.

`*PublishedEvent` implements `pubsub.LogContextProvider`, so these
event fields appear automatically on every dispatch line for every
subscriber.

## Mock for emitter unit tests

`MockBus` (see [`mock.go`](mock.go)) records `Publish` calls without
invoking subscribers and returns synthetic `event.Id`s so callers'
downstream `Story.CommunityEventId` paths are still exercised. Use it
when the test doesn't care about subscriber side effects; assert on
the events via `MockBus.Captured()`.

For tests that DO want to observe subscriber behavior, use
`NewInProcessBus` (see [`inprocess.go`](inprocess.go)) with a real
`pubsub.MemTopic` and a `pubsub.FakeSubscriber`.
