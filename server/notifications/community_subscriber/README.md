# server/notifications/community_subscriber

Push-notification subscriber for the `community_event_bus`. The single
`Subscriber` registered on the bus replaces the old in-place dispatch in
`server/community/notifications.go::notifyTargetedUsers`.

## What it owns

- **Recipient resolution** — for every `CommunityEventType`, decide which
  user IDs receive a push. See `recipients.go`. Member-broadcast types
  ("all community members minus the actor") resolve from the envelope's
  publish-time `MemberIDsAtPublish` snapshot when present, so a user who
  joins between publish and this async dispatch is not pushed an event
  that predates their membership (#2657); an absent snapshot (failed
  capture or a non-`InProcessBus` publisher) falls back to a live
  membership lookup with a WARN. All other types resolve live from
  storage at dispatch time.
- **Soft-delete gate** — `community.IsActive` is checked before any
  dispatch except for `firesForDeletedCommunity` event types
  (`COMMUNITY_DELETED`, plus future at-or-after-delete reminders).
  Audit-trail rows are recorded by the publisher regardless; this gate
  affects only push delivery.
- **Per-user preference gating** — `community.CategoryEnabled` against
  the user's `CommunityNotificationPreferences` row, batched via
  `community.FetchPreferencesForUsers`.
- **Active-stream suppression** — users with an active
  `StreamUserEvents` connection skip push (the event reaches them
  via the stream subscriber instead). The check takes only a user id:
  one stream covers every community they belong to.
- **Cross-community duplicate suppression** — when one user action shares
  an item into several communities, the bus fans out one
  `CommunityEvent` per community, each carrying the originating RPC's
  `request_id`. The per-recipient dedup gate (`dedup.go`) collapses these
  to a single push per `(request_id, event_type, item_id, user_id)`,
  fixing #2088. Reservation happens after the actor/stream/category
  gates and is released on send failure so a sibling can retry. Inert
  when there is no `request_id` (background-job publishers) or no item
  id (single-community event types).
- **Push copy assembly** — per-event `title` / `body` strings. See
  `copy.go`.

## What it does not own

- The storage row insert (the publisher does that synchronously before
  fanning out).
- The audit log (the inserted `CommunityEvent` row IS the audit log).
- Stream broadcast — that is `server/services/community/stream_subscriber.go`.
- The list of *which* event types ever fire push — `ShouldNotify` is
  resolved here by checking whether any recipient set is non-empty.

## Construction

The subscriber struct, constructor, and `HandleEvent` entry point live in
[`subscriber.go`](subscriber.go). Wire-up in `main.go`:

1. Construct the subscriber with `New(storage, notificationService)`.
2. Register on the bus via `bus.Subscribe(sub)`.
3. After the community service is constructed, call
   `sub.SetStreamChecker(communityService.HasActiveUserStream)` so push
   suppression for active streamers can route. The setter is a late-bind
   to break the chicken-and-egg between stream-registry construction
   and notification subscription.

## Idempotency

Push delivery via FCM is naturally **non-idempotent**: re-handing the
same event would produce a duplicate notification. This is acceptable
under the in-memory `MemTopic` because semantics are at-most-once.
**Do not migrate this subscriber to a durable transport without first
adding an `event_id`-keyed dedup table** (see G6 in the plan).

## Soft-delete behavior

The gate is implemented in [`subscriber.go`](subscriber.go) as part of
`HandleEvent`. The decision is:

- If `community.FiresForDeletedCommunity(event.EventType)` returns
  true, dispatch proceeds even for soft-deleted communities. Today
  this covers `COMMUNITY_DELETED`; future at-or-after-delete reminders
  extend the list.
- Otherwise, `community.IsActive(ctx, storage, communityID)` is called.
  If false, the dispatch is logged at INFO and skipped — the
  `CommunityEvent` row is still recorded.

## Observability

Logs use `logging.LoggerWithContext(ctx)` so `request_id` and `user_id`
inherited via `pubsub.MemTopic`'s `deriveDispatchCtx` propagate
automatically. Per-dispatch outcome lines are emitted by the underlying
`pubsub.MemTopic`, NOT by this subscriber. Subscriber-internal logs
should set `operation="community_notifications.handle"` (or a more
specific value for sub-steps like `recipients`, `copy`,
`preferences_lookup`).

### Per-recipient outcome logging

Every push attempt — successful, suppressed, or failed — emits a single
structured INFO (or WARN) log line per recipient keyed by
`community_event_id` and `user_id`. The `outcome` field takes one of the
following values (defined as constants in `subscriber.go`):

| `outcome` | Meaning |
|-----------|---------|
| `sent` | `NotifyUser` returned nil; push accepted by the service layer. |
| `suppressed_actor` | Recipient is the event actor — never self-notify. |
| `suppressed_active_stream` | Recipient has an active `StreamUserEvents` connection; push suppressed in favor of the stream. |
| `suppressed_category` | Recipient disabled this notification category in their preferences. |
| `suppressed_duplicate` | Recipient was already notified for this same user action — a later per-community sibling event sharing the originating RPC's `request_id` was collapsed (see #2088). |
| `suppressed_no_devices` | Logged by `service.go` when the user has no registered device tokens. |
| `send_failed` | `NotifyUser` returned a non-nil error. |
| `skipped_community_deleted` | Community was soft-deleted before the push; applies to non-`FiresForDeletedCommunity` event types. |

`community_event_id` is threaded into the dispatch context via
`notifications.WithCommunityEventID` so the same correlation key appears
in `service.go` and `fcm/provider.go` logs. This aligns with the
`StreamUserEvents` per-event logging convention described in
`docs/server/observability.md` §"Streams and per-event correlation".

**Recommended Cloud Logging filter for missed pushes:**

```
jsonPayload.operation = "community_notifications.handle"
AND jsonPayload.outcome != "sent"
```

To investigate a specific event:

```
jsonPayload.community_event_id = "<event-id>"
```
