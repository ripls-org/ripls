---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Real-time cross-user updates — three-layer hybrid (gRPC streaming, FCM push, polling backstop) converging into a client Event Router that dedupes by event ID and triggers in-place refreshes.
  globs: [app/lib/**, server/chat/**, server/notifications/**]
  triggers: [realtime, streaming, fcm, push, polling, event-router, invalidation]
  lens: [domain, client, server]
  domain: cross-cutting
freshness:
  verified_commit: "02b90ef9b"
  verified_on: "2026-09-21"
---
# Real-Time Updates

## Overview

The app uses a three-layer hybrid architecture for real-time cross-user updates: **gRPC streaming** for sub-second delivery when foregrounded, **FCM push notifications** for backgrounded/terminated state, and **polling** as a correctness backstop. All three layers converge into a single client-side Event Router that deduplicates by event ID and dispatches to cache invalidation providers, triggering in-place ViewModel refreshes without loading spinners.

The stream and the poll are **per user, not per community** (#2867). One connection and one request cover every community the caller belongs to, whatever their number. Membership is resolved server-side on each event, so joining or leaving a community takes effect on the open stream with no client action and no reconnect.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Server                                   │
│                                                                 │
│  service publishes CommunityEvent ──► community_event_bus       │
│       │                                                         │
│       ├──► stream subscriber                                    │
│       │      └─► resolve members ──► their per-user streams     │
│       │                                                         │
│       └──► push subscriber ──► FCM                              │
│            (only for members NOT streaming)                      │
│                                                                 │
│  ListUserEvents() ◄── one poll per client, membership resolved  │
│                       inside the query                          │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                        Client                                   │
│                                                                 │
│  One per user, whatever the community count:                    │
│  ┌───────────────────┐  ┌───────────────┐                       │
│  │ StreamUserEvents  │  │ Poll (30s)    │  ┌──────────────────┐ │
│  │ (all communities) │  │ ListUserEvents│  │ FCM (background) │ │
│  └─────────┬─────────┘  └───────┬───────┘  └────────┬─────────┘ │
│            │                    │                    │           │
│            └──────────┬─────────┘────────────────────┘           │
│                       ▼                                          │
│             ┌────────────────┐                                   │
│             │  Event Router  │                                   │
│             │  (dedup by ID) │                                   │
│             └───────┬────────┘                                   │
│                     │                                            │
│   ┌──────────────┬──┴──────────┬─────────────┐                  │
│   ▼              ▼             ▼             ▼                   │
│ transfer      content     portfolio       feed                  │
│ Invalidation  Invalidation Invalidation  Invalidation           │
│   │              │             │             │                   │
│   ▼              ▼             ▼             ▼                   │
│ ViewModels refresh in-place (no spinner)                        │
└─────────────────────────────────────────────────────────────────┘
```

## Three Delivery Layers

| Layer | Mechanism | When Active | Latency | Role |
|-------|-----------|-------------|---------|------|
| **Stream** | `StreamUserEvents` — one gRPC server-stream per user | App foregrounded | < 1s | Fast path |
| **Push** | FCM notification | App backgrounded/terminated | 1-5s | Offline path |
| **Poll** | `ListUserEvents` — one request per user | App foregrounded | ≤ 30s | Correctness backstop |

The stream delivers events instantly. The poll catches anything the stream missed (network glitch, dropped connection, GC pause). Push covers the app when it's not running. All three feed into the Event Router, which deduplicates by event ID — redundant delivery from multiple layers is harmless and expected.

| App State | Stream | Poll | Push | Effective Latency |
|-----------|:------:|:----:|:----:|-------------------|
| Foreground, on affected screen | Active | Every 30s | Suppressed | < 1s (stream) |
| Foreground, other screen | Active | Every 30s | Suppressed | < 1s (stream) |
| Background | Closed | Stopped | Active | 1-5s (FCM) |
| Terminated | N/A | N/A | Active | 1-5s (FCM), catch-up on relaunch |
| Foreground, stream dropped | Reconnecting | Every 30s | Briefly active | ≤ 30s (poll) |

## Why Per-User (and why it used to be per-community)

Streams and polls are scoped **per user**. One `StreamUserEvents` connection and
one `ListUserEvents` request cover the caller's whole portfolio, so a client's
connection and request counts are constant no matter how many communities they
belong to.

They were originally per-community, which matched every existing API shape —
`ListCommunityEvents`, the `community_event_bus` dispatch, and the notification
recipient logic are all community-scoped — on the reasoning that "users belong
to 1-5 communities, so the connection count is trivial."

**That reasoning expired.** The phone-first pivot (#2492) made every item spawn
an [ad-hoc per-item community](ad_hoc_communities.md), so membership grows with
*item* count. A seeded user reached 48 communities, which meant 48 permanent
streams and 48 poll timers. Two things broke (#2867):

- **Connection starvation.** A browser caps connections per origin (~6 on
  HTTP/1.1). Streams never complete by design, so they consumed every slot and
  ordinary unary RPCs were queued in the browser and never sent — the Home tab
  spun forever while the server saw no request at all. Cloud Run's HTTP/2 hid
  this in deployed environments; `http://localhost:8080` is forced to HTTP/1.1
  because browsers refuse cleartext HTTP/2, so it surfaced in local development
  and e2e first.
- **Load proportional to membership.** 48 poll timers is 1.6 req/s *per user*
  against a design that assumed ~0.1, and one blocked goroutine per membership
  rather than per user. The old poller also staggered timer starts by 3s × index,
  so the 48th community was not polled until 141 seconds after launch.

`StreamCommunityEvents` was deprecated by #2867 and deleted by #2869. A build
predating the per-user client gets `UNIMPLEMENTED` when it opens the stream and
falls back to polling `ListCommunityEvents`, which is *not* deprecated — reading
one community's activity is still a legitimate query; it is simply no longer the
backstop. Those builds also stop having push suppressed, so they keep receiving
events, just at poll latency rather than sub-second.

### How the audience is resolved

A per-user stream names no community, so the bus fan-out
(`stream_subscriber.go`) resolves the event's community membership live, per
event, and delivers to those users' streams. That live resolution is what makes
join and leave work with no client involvement, and it is bounded by the
32-member community cap. The lookup is gated on at least one user stream being
open, so an idle server pays nothing for it.

Two behaviors differ from the per-community stream as a result:

- **`COMMUNITY_DELETED` is not terminal.** On a per-community stream it ended
  the subscription, since nothing else would ever arrive on it. On a per-user
  stream it is one community among many, so ending would take every other
  community's realtime with it; the event is forwarded like any other and
  `event_router.dart` handles it, clearing the community list, the restorable
  list and the feed/search/content caches so the community drops off every
  surface. That handler is the replacement for the terminating stream, not an
  incidental extra — #2867 assumed it existed and it did not until #2869.
- **There is no membership gate at open time.** The stream names no community to
  check; every event is filtered through live membership before delivery.

## Server Architecture

### Event Recording and Broadcast

All community events are persisted and then published to the in-memory
`community_event_bus` (`server/community_event_bus/`, an `InProcessBus.Publish`),
regardless of which service triggers them (transfer, request, experience,
community). Two subscribers registered on the bus then fan the event out:

1. **Stream subscriber** (`server/services/community/stream_subscriber.go`) resolves the event's community members and broadcasts to their per-user streams via `broadcastToUserStreams` — including the attributed actor's own stream. (The actor used to be skipped as "they see their own action via local cache invalidation", but bus-derived events broke that assumption in #2702: an auto-fulfillment is attributed to the requester while the helper's handoff triggered it, so skipping the actor left the requester's open screen stale for a poll interval. Self-delivery is a cheap idempotent refresh; the client's event router dedups by event id.)
2. **Push subscriber** (`server/notifications/community_subscriber/subscriber.go`) sends push notifications to targeted recipients, but **suppresses push** for any recipient who has an active event stream for that community. The poll backstop ensures delivery even if the stream drops during the suppression window.

Subscribers are registered on the bus at startup in `server/wiring.go` (`communityEventBus.Subscribe(...)`). The stream subscriber is co-located with the community service because it shares the in-memory stream registry; the push subscriber's stream-presence check is late-bound via `SetStreamChecker(communityService.HasActiveUserStream)`.

### Stream Registries

One registry: `userStreams` (userID → channels, `user_streaming.go`). The
per-community `eventStreams` registry was deleted with its RPC in #2869. It
mirrors the chat stream registry pattern in `chat/streaming.go`:

- **Register before query**: The handler registers its channel before querying catch-up events, closing the race window where an event could arrive between query and registration.
- **Catch-up replay**: If the client provides `since_unix_sec`, the server replays events since that timestamp (capped at 7 days) before streaming new events. Sent event IDs are tracked to avoid duplicates. The per-user replay spans every community the caller belongs to via `FindEventsForUserSince`, which resolves membership inside the query and bounds the result rather than loading each community's full history.
- **Broadcast with timeout**: Messages are cloned via `proto.Clone` for each recipient and sent with a 1-second timeout to prevent a slow consumer from blocking other streams.
- **Channel never closed on unregister**: A broadcaster snapshots the subscriber slice under RLock and sends outside the lock, so it can still hold a channel after unregister returns. Closing would risk a send-on-closed-channel panic; the send timeout bounds the hang instead.

### Push Suppression

When the push subscriber (`community_subscriber.Subscriber.Handle`) prepares to send a push notification to a recipient, it first checks `HasActiveUserStream(userID)` (via the late-bound `StreamChecker`). If the user has an active stream, the push is skipped — the user will receive the event via the stream instead. This saves FCM quota and avoids redundant processing on the client.

The hook takes no community: a per-user stream covers every community the user belongs to, so stream presence is a property of the user alone (#2869 dropped the `communityID` parameter the per-community registry needed). The chat subscriber's same-named `StreamChecker` is still keyed by conversation — the two answer different questions and are not meant to match.

### Event Types

Every `CommunityEventType` flows through streams and polls. Push notifications are filtered to high-value targeted events via `ShouldNotify()` (in `server/notifications/community_subscriber/recipients.go`). See `proto/ripls/api/community_service.proto` for the full enum (~28 event types covering transfers, requests, experiences, gear sharing, RSVPs, and membership/community lifecycle changes).

## Client Architecture

### Event Router

`EventRouter` in `app/lib/services/event_router.dart` is a Riverpod Provider singleton that provides unified event handling regardless of delivery vector. It:

- **Deduplicates** by event ID using a bounded LRU set (500 entries). The same event arriving via stream and poll triggers only one invalidation cycle.
- **Batch-routes** poll results via `routeCommunityEvents()`, collecting all affected providers into a set and notifying each at most once per poll cycle.
- **Routes FCM events** via `routeFcmEvent()`, which constructs a lightweight `CommunityEventItem` from the FCM data payload (which already contains `event_id` and `event_type`).
- **Dispatches to invalidation providers** based on event type, with two levels of granularity:
  - **Listing-level events** (gear shared, request created, transfer state changes) fire broader providers including feed, search, and portfolio.
  - **Detail-level events** (offers, interest expressed) fire only the content provider, avoiding unnecessary feed and search reloads. RSVP and host roster changes additionally fire the portfolio provider so Home's "Up next" subtitle reflects the new going/maybe/invited state.
  - **Recipient-scoped events** dispatch only for the member the event was addressed to, and nothing for anyone else on the same per-user stream. `ITEM_SHARED_WITH_USER` (#3106) is the first of these: the item's community only just gained the recipient as a member, so it fires the full listing-level set for them alone rather than the whole community.

### Community Event Stream Service

`CommunityEventStreamService` in `app/lib/services/community_event_stream.dart` manages the user's single gRPC server-stream. It tracks the last received event timestamp and passes it as `since_unix_sec` on reconnect for catch-up replay. On stream error or completion it reconnects with exponential backoff (2s base, 60s cap, jitter bounded by the base delay), resetting the backoff counter on successful event receipt.

Both reconnect paths check that the service has not been disconnected first. Without that check a stream error arriving after teardown resurrects a stream nothing will close, which can route the previous user's events into the next user's just-cleared caches — see [logout.md](client/logout.md) § Invariants.

### Community Event Poller

`CommunityEventPoller` in `app/lib/services/community_event_poller.dart` runs one 30-second timer, plus an immediate poll on start so a resume does not wait a full interval. On a cold start the timestamp is seeded to "now" so the first poll doesn't return the entire event history (the app already has fresh data from its normal load); a resume keeps the previous mark to catch up the backgrounded gap. Poll results are batch-routed through the Event Router.

`_poll` re-checks that it is still running after its `await` before routing, for the same logout-invariant reason as the stream's reconnect guard: a request already in flight when the poller stops must not repopulate caches that logout just cleared.

### FCM Integration

The FCM service's `onCommunityEvent` callback in `app/lib/services/fcm_service.dart` fires when a foreground push notification has `type: community_event` in its data payload. The callback passes the data map to `EventRouter.routeFcmEvent()` for deduplication and routing.

### Lifecycle

Streams and pollers follow the app lifecycle:

- **Startup**: Both start after communities load successfully (see [startup.md](client/startup.md), Phase 5).
- **Community join/create**: nothing to do. The server resolves membership per event, so a newly joined community is covered by the open stream and the next poll.
- **App resume**: both restart; neither takes a community list.
- **App pause**: Both stop (streams disconnect, poll timers cancel). Timestamps are preserved for catch-up on resume.
- **Logout**: Both reset (stop + clear timestamps) before cache clearing. See [logout.md](client/logout.md).

### Cache Invalidation Routing

The Event Router maps event types to specific invalidation providers. ViewModels that watch these providers refresh in-place without loading spinners. The mapping distinguishes between events that change listings (which fire feed/search/portfolio providers) and events that only change detail screens (which fire the content provider alone).

| Event Category | Examples | Providers Notified |
|---------------|----------|-------------------|
| Transfer completion | `TRANSFER_COMPLETED` | transfer, portfolio, impact |
| Transfer state changes | `RECIPIENT_SELECTED`, `CANCELLED`, `ACTIVE` | transfer, portfolio |
| Transfer detail | `INTEREST_EXPRESSED`, `PICKUP_PROPOSED` | transfer |
| Content listing changes | `GEAR_SHARED`, `GEAR_UNSHARED`, `REQUEST_CREATED`, `REQUEST_CANCELLED`, `EXPERIENCE_CREATED` | content, search, feedStatus, feedListing |
| Request fulfillment | `REQUEST_FULFILLED` | content, search, feedStatus, feedListing, impact |
| Content detail changes | `OFFER_MADE`, `OFFER_WITHDRAWN` | content |
| Planning list changes | `PLANNING_NEED_*`, `PLANNING_CONTRIBUTION_*` | content |
| RSVP + host roster changes | `EXPERIENCE_RSVP_*`, `EXPERIENCE_ROSTER_CHANGED` | content, portfolio |
| Membership | `INVITATION_LINK_USED`, `MEMBER_LEFT`, `COMMUNITY_CREATED` | portfolio |

`feedStatus` — incremented by listing-level events and by `FeedRepository.markItemsViewed()`; drives the sidebar "New" indicator via `FeedStatusNotifier`.

`feedListing` — incremented only by listing-level events; drives the in-place feed refresh in `FeedNotifier`. Detail-level events (RSVPs, offers) do **not** fire this provider, preventing unnecessary feed network calls and widget reinitialization.

## Server Load Estimates

At 1000 concurrent foreground users, **independent of how many communities each belongs to**:

- **Poll**: ~33 req/s (1000 / 30s). One indexed JOIN per request, resolving membership inside the query.
- **Stream**: ~1000 persistent connections. Each is a goroutine blocked on a channel (~8KB each, ~8MB total).
- **Push suppression**: FCM volume drops proportionally for foreground users.

The per-community shape made both figures scale with total memberships instead. At the ad-hoc community counts the phone-first pivot produces, that was roughly 16x these numbers — which is the load half of #2867.

## Design Decisions

**Per-user (not per-community)**: a client's connection and request counts must not scale with membership, because membership scales with items created. See "Why Per-User" above.

**Stream + poll both active**: Redundant by design. Stream optimizes latency, poll guarantees correctness. Deduplication makes the overlap free.

**Separate from chat streaming**: Chat streams are per-conversation with message content. Event streams are per-user with invalidation signals. Different cardinality, different semantics. Keeping them separate avoids coupling.

**7-day catch-up cap**: Prevents dormant users from receiving huge backlogs. Events are invalidation signals, not state reconstruction — older events are irrelevant.

**Batch routing**: A single poll returning multiple events of the same type (e.g., 3 gear shares) notifies each affected provider exactly once, not three times.

## Key Files

| Component | File |
|-----------|------|
| Stream/poll proto definitions | `proto/ripls/api/community_service.proto` |
| Server per-user stream registry + handler | `server/services/community/user_streaming.go` |
| Server portfolio-wide poll RPC | `server/services/community/user_events.go` |
| Cross-community event query | `server/storage/community_events.go` |
| Server event bus (publish + dispatch) | `server/community_event_bus/` |
| Server stream broadcast subscriber | `server/services/community/stream_subscriber.go` |
| Server push subscriber + push suppression | `server/notifications/community_subscriber/subscriber.go` |
| Server stream registry on Service struct | `server/services/community/service.go` |
| Client Event Router | `app/lib/services/event_router.dart` |
| Client stream service | `app/lib/services/community_event_stream.dart` |
| Client poll service | `app/lib/services/community_event_poller.dart` |
| Client FCM handler | `app/lib/services/fcm_service.dart` |
| Provider definitions | `app/lib/services/providers.dart` |
| Lifecycle wiring | `app/lib/main.dart` |
| Startup sequence | `docs/client/startup.md` |
| Logout teardown | `docs/client/logout.md` |

## Related

- [Client Architecture](client/architecture.md) — MVVM + Riverpod patterns
- [Client Caching](client/caching.md) — cache invalidation patterns (Pattern 7)
- [Client Startup](client/startup.md) — initialization sequence
- [Client Logout](client/logout.md) — teardown sequence
- Issue #999 — original feature request
