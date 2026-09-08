---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: The Feed — vertical full-screen multi-community activity stream built server-side from community events and AI stories, with within/cross-community dedup, three-tier unread/read/action-required ordering, a 14-day horizon, live-opportunity exemptions, and viewer-scoped participation fields.
  globs: [app/lib/presentation/screens/feed/**, server/services/feed/**]
  triggers: [feed, getfeed, community-event, story, deduplication, action-required, unread, ephemeral, horizon, live-opportunity, viewer-rsvp]
  lens: [domain, client, server]
  domain: client
freshness:
  verified_commit: "82c3518d0"
  verified_on: "2026-08-17"
---
# The Feed

The feed is a vertical-scrolling, TikTok-style stream of community activity. Each card fills the full screen. Users swipe up to advance through items.

The feed is **multi-community**: the client sends all currently-enabled community IDs in a single `GetFeed` request, and the server merges and deduplicates items across all of them before returning a unified list. Switching which communities are enabled triggers a cache invalidation and a fresh fetch.

**Invariant: an item must appear at most once in the feed, regardless of how many communities it is shared with.** The server enforces this via entity-scoped deduplication across communities (see [Cross-Community Deduplication](#cross-community-deduplication) below).

## What Appears in the Feed

Feed items are generated server-side from two sources: **community events** and **stories**.

### Community Events

Every significant action in a community produces a `CommunityEvent` (gear shared, request created, experience created, transfer milestones, RSVP activity, etc.). The feed converts these events into typed `FeedItem` cards.

**Within-community deduplication:** Multiple events about the same item (e.g., a gear share followed by someone expressing interest in that gear) are collapsed into a single card. Only the most recent event per item is shown. The item key is derived from the underlying entity:

| Event types grouped | Item key |
|---|---|
| `GEAR_SHARED`, `TRANSFER_INTEREST_EXPRESSED`, `TRANSFER_RECIPIENT_SELECTED`, `TRANSFER_ACTIVE` | `gear:<gear_id>` |
| `REQUEST_CREATED`, `REQUEST_OFFER_MADE` | `request:<request_id>` |
| `EXPERIENCE_CREATED`, `EXPERIENCE_RSVP_*` | `experience:<experience_id>` |
| `COMMUNITY_CREATED` | `community:<community_id>` |

Terminal events (`TRANSFER_COMPLETED`, `TRANSFER_CANCELLED`, `REQUEST_FULFILLED`, `REQUEST_CANCELLED`) produce no card — completed items surface as Story cards instead.

**Card types rendered from events:**

| `FeedItemType` | Rendered as |
|---|---|
| `FEED_ITEM_TYPE_GEAR_SHARED` | `GearContentView` (gear card) |
| `FEED_ITEM_TYPE_REQUEST_CREATED` | `RequestContentView` (request card) |
| `FEED_ITEM_TYPE_COMMUNITY_CREATED` | Community creation card |
| `FEED_ITEM_TYPE_EXPERIENCE_CREATED` | Experience card |

#### Viewer-scoped participation

`ExperienceCreatedPayload.viewer_rsvp` and `RequestCreatedPayload.viewer_has_offered` carry the **viewer's own** standing on the item, so a card can say "Going" or "Offered" instead of inviting someone to do what they have already done (#2800). `RequestCreatedPayload.can_edit` likewise identifies the requester.

Both are computed from the RSVP and offer rows the feed already batch-fetches for `yes_count` / `maybe_count` / `offer_count`, so they cost no additional queries. Both are **`optional`**: absent means *could not be determined* (the batch fetch failed), which clients must not read as "has not responded" — the correct fallback is the neutral invite, not an assertion. Neither is scoped by community; an RSVP is the viewer's own data being shown back to them, and the sibling counts already aggregate across communities the same way.

`viewer_rsvp` uses `FeedRSVPIntention`, which mirrors `RSVPIntention` from the experience service. It cannot reference that enum directly: `experience_service.proto` imports `portfolio.proto`, which imports `feed_service.proto`.

Lifecycle events (transfer milestones, RSVPs, offer made) reuse the card type of their parent item — a `TRANSFER_INTEREST_EXPRESSED` event renders as a gear card, an `EXPERIENCE_RSVP_YES` event renders as an experience card. Lifecycle events cause the item to be marked `is_unread = true` if they occurred after the user's last view.

### Stories

Stories are AI-generated narrative cards that celebrate completed transactions (loans, giveaways, fulfilled requests, experience conclusions, new member welcomes, RSVP clusters). They appear in the feed alongside event-based cards, sorted by recency.

See [server/storage/story.go](../../server/storage/story.go) for storage and [server/services/feed/service.go](../../server/services/feed/service.go) for how stories are assembled into feed items.

### Ordering

Feed items are sorted into three tiers, each sorted internally by timestamp descending:

1. **Unread items** — sorted by `unread_at_unix_sec` descending (most recently became unread first). An item's `unread_at_unix_sec` equals its `last_activity_at` when `is_unread` is true, and 0 when read.
2. **Read items** — sorted by `last_activity_at_unix_sec` descending.
3. **Action-required items** — sorted by `last_activity_at_unix_sec` descending. These require the owner to take action (see below) and should not be hidden if they are read.

For event cards, `last_activity_at` is `max(event.occurred_at, latest_message_in_conversation)`. A new chat message on a gear item causes that item to bubble up within its tier.

#### Action-Required Items (Pinned to Top)

Items that require the current user to take action are sorted to the top of the feed, ahead of all other items. Specifically, an item is pinned when another user has made a request on something owned by the current user — e.g., a borrow request on the user's gear or a request to receive a giveaway item — and the owner has not yet acted on it (selected a recipient, approved/declined, etc.).

Once the owner acts on the item (selects a recipient, approves, declines, or the request is withdrawn), it loses its priority position and returns to the unread or read tier based on its `is_unread` state.

The server computes pinning by checking whether a gear item owned by the viewing user has any pending transfer interest (expressed but not yet acted upon). The `is_action_required` field is a sort/display hint only — it does not affect `is_unread`. The client trusts the server's sort order.

### Cross-Community Deduplication

When `GetFeed` is called with multiple community IDs, the server calls
`generateFeedItems` once per community and merges the results. Because an item
can be shared with more than one community, the merge must deduplicate on the
**entity**, not the event.

The server uses an entity-scoped key (`"gear:<id>"`, `"experience:<id>"`,
`"request:<id>"`, `"community:<id>"`) extracted from each `FeedItem`'s payload
via `feedItemEntityKey()`. This mirrors the `itemKeyForEvent` key used for
within-community deduplication. Stories and nudges have no shared entity ID and
fall back to their item ID (which is already unique across communities).

**When the same item appears in two communities, the first community's
`FeedItem` wins** (ordered by the `community_ids` array from the client). This
includes the `is_unread` computation, which is scoped to that community's view
record. An item viewed in Community B but not A will appear unread if A is
listed first — this is an accepted limitation of the current design.

**Implementation:** [`server/services/feed/service.go`](../../server/services/feed/service.go) — `feedItemEntityKey()` and the deduplication loop in `GetFeed`.

### Live Opportunities (Upcoming Events, Wanted Requests, Open Giveaways)

An item is a **live opportunity** while the thing it offers is still open. Live items **always appear in the feed** regardless of whether the user has seen them, bypassing both the staleness expiry and the [horizon](#the-horizon). A thing that has not happened yet cannot be stale, however long ago it was posted — an event created six weeks in advance still belongs on the feed the day before it happens.

`isLiveOpportunity()` ([freshness.go](../../server/services/feed/freshness.go)) decides this per event type:

| Event | Live while |
|---|---|
| `EXPERIENCE_CREATED`, `EXPERIENCE_RSVP_*` | not completed/cancelled **and** the scheduled start is no more than `experienceLiveGraceSeconds` (24 h) in the past |
| `REQUEST_CREATED`, `REQUEST_OFFER_MADE` | not fulfilled/cancelled **and** `needed_by` is in the future |
| `GEAR_SHARED` | the community listing has `availability == AVAILABILITY_FOR_GIVEAWAY` |
| `TRANSFER_INTEREST_EXPRESSED`, `TRANSFER_RECIPIENT_SELECTED`, `TRANSFER_ACTIVE` | a community listing exists (an open transfer stays visible to both parties until resolved) |

Two consequences worth stating plainly:

- **An experience with no scheduled time, and a request with no needed-by date, are not live.** They have nothing to be ahead of, so they age out on the horizon like anything else. A request nobody answered in two weeks is no longer news.
- **A past-dated event stops being live.** The grace window keeps it through the day it happens and the day after; then it ages out. Previously experiences and requests were exempt from expiry *unconditionally*, which left events that had already happened advertising themselves — with a Join button — indefinitely (#2799).

Live items leave the feed when the opportunity closes: they are **archived** (completed, cancelled, or fulfilled), at which point they may surface as a Story card instead — or the date passes, after which the horizon applies.

Stories are never live opportunities. A story commemorates something that already happened, so it always ages out.

### The Horizon

`feedHorizonSeconds` (**14 days**) is an absolute bound: an item whose `last_activity_at` is older than that is dropped **regardless of view state**, unless it is a live opportunity.

The horizon exists because the seen-based expiry below can only fire for items the viewer actually scrolled to, and on the Home pulse — now the feed's only surface — nothing is ever marked viewed. Unseen items are permanently "fresh", so without an absolute bound a community accumulates every story and gear share it has ever produced (#2799).

Two weeks is deliberately double `unreadWindowSeconds`, so nothing can be unread and beyond the horizon at the same time; otherwise the sidebar "New" badge could advertise an item `GetFeed` no longer returns. `TestIsWithinHorizonExceedsUnreadWindow` pins that relationship.

### Filtered-Out Items

Items are excluded from the feed in three ways:

1. **Archived items**: Gear, requests, and experiences that have been archived (completed or cancelled) produce no event card. The terminal event type maps to `nil` in `eventToFeedItem()`, and deduplication means the terminal event wins, so the card disappears.

2. **Expired items**: Items the user has fully seen and that have had no new activity in more than 24 hours are filtered out (see [Freshness](#freshness) below). **Exception:** live opportunities are exempt — see [Live Opportunities](#live-opportunities-upcoming-events-wanted-requests-open-giveaways) above.

3. **Items past the horizon**: anything whose last activity is older than 14 days, seen or not, unless it is a live opportunity. See [The Horizon](#the-horizon).

---

## Read / Unread Tracking

### View Records

Every time a user scrolls to a feed item, the client calls `MarkFeedItemsViewed`, which upserts a `FeedItemView` record in the database (one record per user × community × item).

The `FeedItemView` model ([proto/ripls/models/feed.proto](../../proto/ripls/models/feed.proto)) stores:

| Field | Meaning |
|---|---|
| `view_count` | Total times viewed |
| `first_viewed_at_unix_sec` | When first seen |
| `last_viewed_at_unix_sec` | When last seen (any view) |
| `last_seen_at_unix_sec` | When last seen via `MarkFeedItemsViewed` |
| `views_since_last_activity` | Views since the last time there was new activity on this item |

`last_seen_at_unix_sec` is the primary field used for freshness and badge calculations. `last_viewed_at_unix_sec` exists as a fallback for legacy records created before `last_seen_at` was added — those records have `last_seen_at = 0`. See [Effective Last Seen](#effective-last-seen) below.

View tracking is triggered automatically as the user swipes: `FeedNotifier.setPageIndex()` calls `markCurrentItemViewed()`, which calls `MarkFeedItemsViewed` and immediately increments the local view count (optimistic update). `FeedScreen` additionally marks the first card after `initialize()` returns, because the page controller settles on page 0 without firing its listener.

**This only happens on the swipe `FeedScreen`, which no dock destination points at.** The Home pulse ([inbox.md](inbox.md) → *Community pulse*) renders the same items as a scrolling list and marks nothing viewed, so in a shipped build essentially no view records are created. Everything below therefore describes a mechanism that is currently inert in practice; [the horizon](#the-horizon) is the operative bound. Whether the pulse should mark items viewed as they scroll past is an open question, not a settled design.

Implementation: [server/storage/feed.go](../../server/storage/feed.go), [app/lib/data/repositories/feed_repository.dart](../../app/lib/data/repositories/feed_repository.dart).

### Freshness

An item remains in the feed as long as it is "fresh". Freshness is computed server-side in `isItemFresh()` ([server/services/feed/service.go](../../server/services/feed/service.go)) using this logic:

1. **Never seen** (`view == nil`) → always fresh.
2. **New activity since last seen** (`last_seen_at < last_activity_at`) → always fresh. A new chat message, RSVP, or transfer event causes a card to stay visible even if the user previously viewed it.
3. **Seen and stale** (`last_seen_at >= last_activity_at` and `now - last_seen_at > 24h`) → expired, removed from feed.

The 24-hour window is configurable per community via `Community.feed_expiry_hours` (defaults to 24). Once a user has seen an item and no new activity has occurred, it stays visible for up to 24 hours before disappearing — so a pull-to-refresh does not immediately produce an empty feed.

### Effective Last Seen

Legacy view records (created before `last_seen_at_unix_sec` was added to the model) have `last_seen_at = 0`. The helper `effectiveLastSeenAt(view)` returns `last_seen_at` when it is non-zero, and falls back to `last_viewed_at` otherwise. This prevents old records from being misread as "never seen" by the freshness logic. See [server/services/feed/service.go](../../server/services/feed/service.go).

### Unread State

Each `FeedItem` carries a single `is_unread` boolean, computed entirely server-side. This is the **only** signal the client needs for visual indicators on feed cards and for the sidebar "New" badge. The server owns the definition of what constitutes a material change — clients never evaluate unread rules themselves.

An item is `is_unread = true` when **all** of the following are true:

- **Activity is within the last 7 days** — the item's `last_activity_at` is no more than 7 days old (`unreadWindowSeconds = 7 * 24 * 3600` in [service.go](../../server/services/feed/service.go)). Activity older than 7 days is expired and never triggers unread, regardless of view state.

AND **any** of the following:

1. **Never seen** — the user has no `FeedItemView` record for this item.
2. **New events since last seen** — a lifecycle event (transfer interest, recipient selected, RSVP, offer, etc.) occurred after `effectiveLastSeenAt(view)`.
3. **New comments from others** — the conversation's `last_message_at` is after `effectiveLastSeenAt(view)` and the latest message was not authored by the viewing user.

The `is_action_required` flag (see [Ordering](#ordering)) is a **sort hint only** — it does not affect `is_unread`. Action-required items are sorted to the top of the feed but follow the same unread rules as every other item. Once the user scrolls past an action-required item, it is read.

The unread state is cleared as soon as the user **scrolls to the item in the feed** — they do not need to open the conversation or read individual messages. Viewing the item (which triggers `MarkFeedItemsViewed`) updates `last_seen_at_unix_sec`, causing `is_unread` to flip to `false` on the next feed load.

This is distinct from the chat-level unread count (which tracks individual unread messages within a conversation). The feed-level `is_unread` is a lightweight "something changed" signal, not a message-level read receipt.

### No Rich Badges

Previous iterations carried `unseen_event_count`, `unseen_comment_count`, and `most_recent_event_type` on `FeedItem` for rich badge text. These have been removed in favor of the single `is_unread` dot. The client renders a simple visual indicator — no "3 new comments" or "Recipient selected" text. This keeps the client trivially simple and lets the server evolve unread rules without client updates.

---

## The "New" Badge (Sidebar Indicator)

The sidebar shows a dot next to each community that has unseen feed items. This is powered by the `GetFeedStatus` RPC, which returns a `community_id → bool` map without generating full feed payloads.

### How It Works

`communityHasNewFeedItems()` ([server/services/feed/feed_status.go](../../server/services/feed/feed_status.go)) uses the same `isItemUnread()` helper as `GetFeed` — a community has new items if **any** of its feed items would be `is_unread = true`. The 7-day unread window applies uniformly: activity older than 7 days never triggers the badge, whether the item has been seen or not.

The check applies the same deduplication as `GetFeed` — it evaluates only the champion (most recent) event per item key. This is critical: the user can only ever view the champion event card in the feed, so only the champion event's view record should determine the badge.

### Client-Side Badge

`FeedStatusNotifier` ([app/lib/presentation/viewmodels/feed_view_model.dart](../../app/lib/presentation/viewmodels/feed_view_model.dart)) fetches and caches the status map. It rebuilds automatically when `feedStatusCacheInvalidationProvider` is incremented — which happens inside `FeedRepository.markItemsViewed()` every time the user views a feed item. This is Riverpod Pattern 7: a counter provider that drives reactive invalidation without coupling repositories to view models.

`communityHasNewFeedProvider` is a `Provider.family<bool, String>` that returns `true` when the given community has unseen items. The sidebar watches this provider per community.

---

## Cache Invalidation

The feed cache uses a **single key** (`feed:list`) for the first page and
`feed:list:<page_token>` for subsequent pages, regardless of which communities
are enabled. The entire cache is invalidated when the enabled community set
changes, so the next `initialize()` call fetches a fresh merged result.

Cache is invalidated in these situations:

| Action | What is invalidated |
|---|---|
| User marks items viewed (`MarkFeedItemsViewed`) | `feed:list` + `status` |
| User pulls to refresh | All feed caches (`invalidateAllFeeds()`) + gear/request/user/media caches |
| User creates new content (via `PostCreationService`) | All feed caches (`invalidateAllFeeds()`) before navigating to feed tab |
| User shares gear with a community | `feed:list` (via `CommunityRepository`) |

`FeedNotifier.initialize()` always calls `invalidateFeed()` first, so the feed
is always fresh when the tab opens regardless of prior cache state.

Implementation: [app/lib/data/repositories/feed_repository.dart](../../app/lib/data/repositories/feed_repository.dart), [app/lib/data/repositories/community_repository.dart](../../app/lib/data/repositories/community_repository.dart), [app/lib/presentation/viewmodels/feed_view_model.dart](../../app/lib/presentation/viewmodels/feed_view_model.dart).

---

## Pagination

Feed items are loaded in batches of 20 (max 50 per request). The server uses offset-based pagination: `page_token` in `GetFeedRequest` is a string-encoded integer offset into the sorted feed item list. When more items exist beyond the returned page, `GetFeedResponse.next_page_token` carries the offset for the next batch; an empty `next_page_token` means the end of the feed has been reached.

**Client flow:**

1. `FeedNotifier.initialize()` calls `getFeed(communityIds)` with no page token, receiving the first 20 items.
2. As the user swipes, `FeedNotifier.setPageIndex(index)` fires. When `index >= items.length - 3`, `loadMore()` is called proactively (3 items before the end of the current batch).
3. `loadMore()` calls `getFeed(communityIds, pageToken: state.nextPageToken)` and appends the result to `state.items`.
4. This continues until `next_page_token` is empty, at which point `hasMore` becomes `false` and `loadMore()` stops.

The `PageView` widget renders all loaded items; new batches arrive before the user reaches the last loaded item, so scrolling feels uninterrupted.

**Relationship to the "New" badge:** `communityHasNewFeedItems` checks all fresh items server-side, not just the first page. The badge clears only once all fresh items have been marked viewed — which requires the user to scroll through the full feed across all batches.

---

## Server-Side Feed Generation Pipeline

Entry point: `GetFeed` in [server/services/feed/service.go](../../server/services/feed/service.go).

1. Authenticate and verify membership in all requested communities.
2. For each community: fetch all `CommunityEvent` records and up to 50 recent stories.
3. Within each community: batch-fetch all related entities (gear, requests, experiences, CommunityGear/Request/Experience join records, locations, offer counts, RSVP counts, users, conversation IDs, latest chat message timestamps). Deduplicate events by item key; apply freshness filter; convert surviving events to `FeedItem`s via `eventToFeedItem()`. Compute `is_unread` and `is_action_required`.
4. Merge per-community item lists using entity-scoped deduplication (`feedItemEntityKey`) so items shared with multiple communities appear exactly once. The first community's version wins.
5. Sort the merged list: unread first (by `unread_at` desc), then read (by `last_activity_at` desc), then action-required (by `last_activity_at` desc).
6. Append the terminator card on the first page (requires stock imagery provider), extracted before pagination so it always lands on page 1. Generic per-user nudges (LLM-written, interleaved every 8 items) were removed in #2936 — the Home surface now offers unconditional, hand-written affordances instead; only the terminator and the momentum engine's host prompts still use this path.
7. Apply offset-based pagination using `page_token` (string-encoded integer offset). Return up to `page_size` items (default 20, max 50). Set `next_page_token` to the next offset if more items remain.
8. Re-attach the terminator nudge as the final item on page 1.

All entity lookups in steps 2–4 are batch queries. There are no per-item queries inside the generation loop.

---

## Key Files

**Server:**
- [server/services/feed/service.go](../../server/services/feed/service.go) — Feed generation, deduplication, freshness logic
- [server/services/feed/feed_status.go](../../server/services/feed/feed_status.go) — Lightweight "New" badge status check
- [server/storage/feed.go](../../server/storage/feed.go) — View tracking storage (`FeedItemView` CRUD, batch queries)
- [server/services/feed/service_test.go](../../server/services/feed/service_test.go) — Feed service tests

**Proto:**
- [proto/ripls/api/feed_service.proto](../../proto/ripls/api/feed_service.proto) — RPC definitions, `FeedItem`, all payload types
- [proto/ripls/models/feed.proto](../../proto/ripls/models/feed.proto) — `FeedItemView` storage model

**Client:**
- [app/lib/services/feed_service.dart](../../app/lib/services/feed_service.dart) — RPC client wrapper
- [app/lib/data/repositories/feed_repository.dart](../../app/lib/data/repositories/feed_repository.dart) — Caching layer, view tracking, status invalidation
- [app/lib/presentation/viewmodels/feed_view_model.dart](../../app/lib/presentation/viewmodels/feed_view_model.dart) — `FeedNotifier`, `FeedStatusNotifier`, `communityHasNewFeedProvider`
- [app/lib/presentation/screens/feed/feed_screen.dart](../../app/lib/presentation/screens/feed/feed_screen.dart) — Feed UI, `PageView`, pull-to-refresh
