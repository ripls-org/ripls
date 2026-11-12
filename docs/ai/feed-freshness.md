# Feed Freshness: Unread Tracking & Item Expiry

## Problem Statement

The feed grows unboundedly over time, showing items all the way back to the
beginning. There is no concept of "you've already seen this and nothing has
changed." Users scroll through pages of stale items they've already reviewed
to find anything new.

Additionally, the feed shows when things are *created* but not when they
*change*. A gear item shared last week that just had its recipient selected,
or a request that just got three new offers, looks identical to a stale item.
The feed misses comments, state changes, RSVPs, offers, and detail updates —
even though many of these already have `CommunityEventType` enum values that
the feed's `eventToFeedItem()` silently drops.

### Desired behavior

The feed should be a curated, fresh stream where:

- Items with unseen activity always surface, ordered by most recent change
- Each item appears at most once (deduplicated), showing the most recent event
- Items that have been seen and are unchanged expire after a short grace period
- Users find expired items via the Discover screen search
- Activity context badges show what changed ("3 new comments", "Recipient selected")

## Current Architecture

### How the feed works today

1. `CommunityEvent` records are created by services (community, transfer,
   request, experience) via `RecordCommunityEventAndNotify()`
2. `FeedService.GetFeed()` (`server/services/feed/service.go:42`) queries all
   `CommunityEvent`s for a community, batch-fetches related data, and converts
   each event to a `FeedItem` via `eventToFeedItem()` (line 364)
3. Only 4 event types are converted: `GEAR_SHARED`, `REQUEST_CREATED`,
   `COMMUNITY_CREATED`, `EXPERIENCE_CREATED`. Plus `Story` items. All other
   event types return nil and are skipped
4. Items are sorted by `occurred_at_unix_sec` descending and paginated
5. `FeedItemView` (`proto/ripls/models/feed.proto`) tracks per-user view
   counts (how many times viewed) but not "last seen at" relative to item
   changes

### What CommunityEvent types exist but aren't surfaced

```
TRANSFER_INTEREST_EXPRESSED    TRANSFER_RECIPIENT_SELECTED
TRANSFER_ACTIVE                TRANSFER_COMPLETED
TRANSFER_CANCELLED             TRANSFER_INTEREST_WITHDRAWN
REQUEST_OFFER_MADE             REQUEST_OFFER_SELECTED
REQUEST_FULFILLED              REQUEST_CANCELLED
REQUEST_OFFER_WITHDRAWN        EXPERIENCE_RSVP_YES
EXPERIENCE_RSVP_MAYBE          EXPERIENCE_RSVP_NO
GEAR_UNSHARED                  MEMBER_LEFT
INVITATION_LINK_USED
```

### What doesn't generate CommunityEvents at all

- **Chat messages / comments** — these go through the chat system
  (`ChatService.SendMessage`) and are not recorded as community events

## Approach: Activity-ordered, deduplicated, expiring feed

### Seen tracking

`FeedItemView` gains two new fields for tracking whether a user has seen
the current state of an item:

- `last_seen_at_unix_sec` — updated when the user views an item via
  `MarkFeedItemsViewed`
- `views_since_last_activity` — incremented on each view when the user has
  already seen the current state, reset to 0 when new activity occurs

An item is "unseen" if `last_activity_at > last_seen_at`.

### Freshness expiry

Items that have been seen and have not changed should not persist
indefinitely. After a grace period, they are excluded:

- Exclude when `last_seen_at >= last_activity_at` AND
  (`views_since_last_activity >= max_views` OR `now - last_seen_at > expiry_hours`)
- Default thresholds: `max_views = 2`, `expiry_hours = 24`. Configurable per
  community so admins can tune freshness per circle
- Items with new activity always resurface regardless of prior history
- Users find expired items via Discover screen search

### Per-item deduplication

Each item appears at most once in the feed. Events are grouped by item ID,
keeping only the most recent event per item.

### Conversation activity

`ChatConversation.last_message_at_unix_sec` provides comment activity —
without creating a `CommunityEvent` per comment.

- `last_activity_at = max(most_recent_event.occurred_at, conversation.last_message_at)`
  per item, computed at feed generation time from data already fetched
- Only other people's comments count — the viewing user's own messages are
  excluded when computing `last_message_at` for activity ordering

### Lifecycle events

The feed surfaces lifecycle event types that are currently dropped by
`eventToFeedItem()`. These don't get their own card types — they update the
existing content view for the item. The most recent event type is carried
on the `FeedItem` as an activity badge.

### Activity context on feed items

Each `FeedItem` carries a single `bool is_unread` field, computed
server-side. The client renders a simple unread dot — no rich badge text.
The server owns the complete definition of what constitutes a material
change (see [Unread State](#unread-state) in the feed docs).

### Sidebar "New" indicator

The sidebar currently shows an unread *message count* per community via
`unreadMessageCountForCommunityProvider`, which reads from
`ChatService.GetUnreadCounts()` — counting unread chat messages. This
should change to a simple "New" indicator that reflects whether a community
has unseen *feed* updates (new or changed items the user hasn't viewed).

The unread chat message count is still useful on individual content views
(the chat pill in `ContentActionBar`), so that mechanism stays. The sidebar
just stops using it and switches to feed-freshness-based awareness.

**How it works:**
- A lightweight dedicated RPC `GetFeedStatus` returns
  `map<string, bool> community_id_to_has_new` for all communities at once,
  avoiding a full `GetFeed` call per community
- The sidebar renders community names immediately from cache, then fires an
  async `GetFeedStatus` fetch. "New" pills appear once the response arrives
  (may flash in after sidebar is already visible — acceptable trade-off for
  never blocking sidebar open)
- The sidebar replaces `UnreadBadge(count: unreadCount)` with a simple
  "New" pill when the community has unseen feed items
- The "New" indicator clears when the user opens that community's feed
  and scrolls through the items (triggering `MarkFeedItemsViewed`)

## Open Questions (Resolved)

1. **Expiry thresholds:** Make configurable per community. Default: 2 views
   or 24 hours. Store thresholds on the community model (or a community
   settings record) so admins can tune freshness per circle.

2. **Comment-only resurfacing:** Only other people's comments resurface an
   item. The user's own comments do not count as new activity. When
   computing `last_activity_at` from `conversation.last_message_at`, filter
   out messages authored by the viewing user.

3. **Empty feed state:** Show a simple "You're all caught up" message. This
   is a placeholder — the nudges work (`docs/ai/nudges.md`) will replace it
   with a proper terminator card later.

4. **Lifecycle event badge priority:** Show the most recent event type plus
   a total count of unseen events. Format: "Recipient selected · +2
   updates". The `FeedItem` carries both `most_recent_event_type` and
   `unseen_event_count`.

5. **`GetFeedStatus` polling frequency:** Fetch on every sidebar open.
   The sidebar shows community names immediately from cache, then does an
   async fetch for `GetFeedStatus` to update the "New" badges. This means
   badges may flash in after the sidebar is already visible, but community
   names are never delayed.

6. **Terminal items as stories:** Just show stories. Story generation
   (AI or template fallback) takes seconds and happens synchronously when
   the terminal event fires. No need for a transitional content view state.

## Implementation Plan

### Phase 1: Server — seen tracking + freshness

**Goal:** Track when users last saw each item and expire stale items.

1. Add `int64 last_seen_at_unix_sec = 9` to `FeedItemView` in
   `proto/ripls/models/feed.proto`
2. Add `int32 views_since_last_activity = 10` to `FeedItemView`
3. Update `FeedStorage.IncrementViewCountBatch` to set
   `last_seen_at_unix_sec = now` and increment `views_since_last_activity`
   when `last_seen_at >= last_activity_at`, reset counter to 0 otherwise
4. Update `FeedStorage.GetViewCountsForItems` to return both timestamps and
   view-since-activity count
5. Add per-community freshness thresholds: `int32 feed_expiry_views`
   (default 2) and `int32 feed_expiry_hours` (default 24) on the community
   settings. Read these in `generateFeedItems()` with fallback to defaults
6. Add freshness filter in `generateFeedItems()`: exclude items where
   `last_seen_at >= last_activity_at` AND (`views_since_last_activity >=
   feed_expiry_views` OR `now - last_seen_at > feed_expiry_hours`)
7. Add unit tests for seen tracking and freshness expiry (including custom
   thresholds)

### Phase 2: Server — lifecycle events

**Goal:** Surface lifecycle event types in the feed.

1. Add `eventToFeedItem()` handlers for lifecycle event types:
   `TRANSFER_INTEREST_EXPRESSED`, `TRANSFER_RECIPIENT_SELECTED`,
   `TRANSFER_ACTIVE`, `TRANSFER_COMPLETED`, `TRANSFER_CANCELLED`,
   `REQUEST_OFFER_MADE`, `REQUEST_FULFILLED`, `REQUEST_CANCELLED`,
   `EXPERIENCE_RSVP_YES`, `EXPERIENCE_RSVP_MAYBE`, `EXPERIENCE_RSVP_NO`
2. These handlers reuse the same content view card types as creation events
   (gear, request, experience) — they just map to an existing item. The
   event type is carried forward on the `FeedItem` for badge display
3. Terminal items (completed/cancelled) are included as stories with
   `is_story = true`, with 7-day hard cap
4. Add unit tests for lifecycle event handlers

### Phase 3: Server — per-item deduplication + conversation activity

**Goal:** Each item appears at most once, ordered by most recent activity
across events and comments.

1. In `generateFeedItems()`, group events by item ID, keep only the most
   recent event per item
2. Batch-fetch `ChatConversation` records for all items. Extract
   `last_message_at_unix_sec` per item, excluding messages authored by the
   viewing user (only other people's comments count as new activity)
3. Compute `last_activity_at = max(event.occurred_at, conversation.last_message_at)`
   per item. Sort by `last_activity_at` descending
4. Apply freshness filter using `last_activity_at` instead of
   `occurred_at_unix_sec`
5. Add `int64 last_activity_at_unix_sec` to `FeedItem` in
   `proto/ripls/api/feed_service.proto`. Also add `int32
   unseen_comment_count`, `CommunityEventType most_recent_event_type`,
   and `int32 unseen_event_count` temporarily — these will be removed
   in Phase 7 when `is_unread` replaces them. Populate via batch queries
   (not per-item loops)
6. Pagination: deduplication changes effective page size. Fetch more events
   than `pageSize` and trim after dedup, or switch to cursor-based
   pagination keyed on `last_activity_at`
7. Performance: verify with `storage.AssertMaxQueries` that query count
   stays bounded after adding conversation fetch
8. Add unit tests for deduplication, conversation merging, and edge cases
   (items with no conversation, items with conversation-only activity)

### Phase 4: Client — activity-ordered feed

**Goal:** Render the deduplicated, activity-ordered feed. Rich badge
rendering is temporary — Phase 7 replaces it with a simple unread dot.

1. Update `feed_view_model.dart`: server returns pre-filtered, pre-ordered
   items. Client trusts server order. `FeedState` works as-is
2. Update `feed_screen.dart` item rendering:
   - Temporary activity badges using `unseen_event_count`,
     `unseen_comment_count`, `most_recent_event_type` (these fields will
     be removed in Phase 7)
   - Use `AppColors.*` for badge styling
3. Add empty feed state: simple "You're all caught up" message when
   freshness expiry removes all items. This is a placeholder — the nudges
   work (`docs/ai/nudges.md`) will replace it with a terminator card later
4. Add i18n key `feedAllCaughtUp` ("You're all caught up" /
   "Estás al día") to both `app_en.arb` and `app_es.arb`
5. Seen tracking: `markCurrentItemViewed()` already calls
   `MarkFeedItemsViewed`, which now updates `last_seen_at_unix_sec`
6. Add widget tests for feed rendering

### Phase 5: Sidebar "New" indicator

**Goal:** Replace the sidebar's unread message count with a feed-freshness
"New" indicator.

1. Add `GetFeedStatus` RPC to `FeedService` in
   `proto/ripls/api/feed_service.proto`:
   ```
   rpc GetFeedStatus(GetFeedStatusRequest) returns (GetFeedStatusResponse);

   message GetFeedStatusRequest {}
   message GetFeedStatusResponse {
     map<string, bool> community_id_to_has_new = 1;
   }
   ```
   This is a lightweight RPC that checks for unseen items across all the
   user's communities without generating full feed responses. It queries
   `FeedItemView` records and `CommunityEvent` timestamps to determine
   if any community has items where `last_activity_at > last_seen_at`

2. Server implementation in `server/services/feed/feed_status.go`:
   - List user's community memberships (1 query)
   - For each community, check if any feed items have activity newer than
     the user's last seen timestamp (batch query, not per-community loop)
   - Return map of community ID → has new items

3. Client data layer: new `feed_status_service.dart` (wraps the
   `GetFeedStatus` RPC) + `feed_status_repository.dart` with cache
   namespace `'feed_status'`. Uses the standard global TTL — freshness is
   handled by explicit invalidation, not a custom TTL (the caching system
   does not support per-repository TTL)

4. Client ViewModel: new `FeedStatusNotifier` (Notifier, not autoDispose —
   lives for the app session) with `FeedStatusState` containing
   `Map<String, bool> communityHasNew`. Follows the same pattern as
   `UnreadCountNotifier`: the Notifier calls the repository, and a family
   provider extracts per-community data. Per architecture rule 1, the
   Notifier is the only layer that calls the repository

5. New provider: `communityHasNewFeedProvider` (Provider.family, keyed by
   community ID) that reads from `feedStatusProvider` state — same pattern
   as `unreadMessageCountForCommunityProvider` reads from
   `unreadCountProvider`

6. Update `sidebar.dart` `_buildCommunityItem()`:
   - Replace `ref.watch(unreadMessageCountForCommunityProvider(community.id))`
     with `ref.watch(communityHasNewFeedProvider(community.id))`
   - Replace `UnreadBadge(count: unreadCount)` with a "New" pill widget.
     Use `AppColors.*` for all pill colors (background, text, border) to
     ensure theme awareness in both light and dark modes — no hardcoded
     coral or amber values
   - The unread message count provider and `UnreadBadge` remain available
     for use in content views (chat pill) — only the sidebar changes

7. Invalidate feed status cache via cross-repository invalidation:
   `FeedRepository.markItemsViewed()` calls
   `FeedStatusRepository.invalidate()` after the server RPC succeeds.
   The `FeedStatusNotifier` listens for this invalidation (same event-driven
   pattern as `dailyCacheInvalidationProvider` in caching.md Pattern 7)
   and refreshes in place. The sidebar's "New" pill disappears reactively

8. Add i18n key `sidebarNewIndicator` to both ARB files (value: "New" /
   "Nuevo"). Use `sidebar*` prefix per i18n naming convention

9. Add unit tests for `GetFeedStatus` RPC and widget tests for sidebar
   "New" indicator

### Phase 6: Testing and polish

1. Server integration tests: deduplication, conversation merging, seen
   filtering end-to-end
2. Server tests: freshness expiry (items expire after 2 views or 24 hours)
3. Server tests: `GetFeedStatus` returns correct new/not-new per community
4. Client tests: feed renders in server-provided order
5. Client tests: sidebar shows "New" pill correctly and clears after viewing
6. Performance: verify with `storage.AssertMaxQueries` that query count
   stays bounded
7. Edge cases: items with no conversation, items with conversation-only
   activity, terminal items with 7-day cap

### Phase 7: Migrate existing work to unified `is_unread`

**Goal:** Retrofit the existing Phases 1–6 implementation to expose a single
`is_unread` boolean on each `FeedItem`. This consolidates all "material
change" logic on the server so clients use one field instead of interpreting
`unseen_event_count`, `unseen_comment_count`, and freshness rules
independently.

#### Proto

1. Add `bool is_unread = 9` to `FeedItem` in
   `proto/ripls/api/feed_service.proto`
2. Remove four fields from `FeedItem` that are now redundant:
   - `int32 view_count = 3` — client no longer needs to derive
     seen/unseen state; server tracks views in `FeedItemView` internally
   - `int32 unseen_comment_count = 6` — replaced by `is_unread`
   - `CommunityEventType most_recent_event_type = 7` — replaced by
     `is_unread`
   - `int32 unseen_event_count = 8` — replaced by `is_unread`
   - Mark these field numbers as `reserved` in the proto to prevent
     accidental reuse
3. Run `npm run generate` to regenerate Go and Dart code

#### Server: compute `is_unread` from existing signals

4. In `generateFeedItems()` (`server/services/feed/service.go`), after
   building each `FeedItem`, compute `is_unread` from data already
   available in the pipeline. An item is unread when its most recent
   activity is **within the last 7 days** (`unreadWindowSeconds =
   7 * 24 * 3600`) AND **any** of:
   - **Never seen** — no `FeedItemView` record exists (`view == nil`)
   - **New events since last seen** — the champion event's `occurred_at`
     is after `effectiveLastSeenAt(view)`
   - **New comments from others** — the conversation's `last_message_at`
     is after `effectiveLastSeenAt(view)` and the latest message was not
     authored by the viewing user
   - Activity older than 7 days is expired and never triggers unread,
     regardless of view state
   - Implementation: extract an `isItemUnread(view, lastActivityAt, now)`
     helper so the logic is testable in isolation
5. Remove all server code that computed the deleted fields:
   `unseen_event_count`, `unseen_comment_count`, `most_recent_event_type`,
   and `view_count` population in `eventToFeedItem()` / story assembly.
   The `isItemUnread()` helper replaces this logic

#### Server: update `GetFeedStatus`

6. Refactor `communityHasNewFeedItems()` in `feed_status.go` to use the
   same `isItemUnread()` helper. The 7-day window is built into the
   helper, so the separate `badgeWindowHours` parameter is removed.
   After this, the feed and the sidebar badge use identical unread
   definitions — a community shows "New" if and only if its feed would
   contain at least one item with `is_unread = true`

#### Client: use `is_unread` as the single indicator

7. Update `feed_screen.dart`: render an unread dot on items where
   `is_unread = true`. Use `AppColors.*` for styling
8. Remove all client code that referenced the deleted fields:
   - Remove any badge rendering logic that used `unseen_event_count`,
     `unseen_comment_count`, or `most_recent_event_type`
   - Remove `view_count` optimistic update logic in
     `markCurrentItemViewed()` (the server still tracks views, but the
     count is no longer on the wire)
   - Remove i18n keys added in Phase 4 for rich badge text:
     `feedNewComments`, `feedRecipientSelected`, `feedOfferMade`,
     `feedRsvpAdded`, `feedUpdatesCount`
9. The sidebar already reads from `GetFeedStatus` via
   `communityHasNewFeedProvider` — no sidebar changes needed since the
   server now computes `has_new` from the same unified logic
10. The existing `markCurrentItemViewed()` scroll handler continues to
    call `MarkFeedItemsViewed`, which updates `last_seen_at_unix_sec`.
    This causes `is_unread` to flip to `false` on the next feed load.
    No additional client logic needed

#### Tests

11. Server unit tests for `isItemUnread()` helper:
    - Returns `true` when no view record (never seen, recent activity)
    - Returns `true` when new event since last seen
    - Returns `true` when new comment from another user since last seen
    - Returns `false` when only the viewer's own comments are new
    - Returns `false` when seen and no new activity
    - Returns `false` after `MarkFeedItemsViewed` clears the state
    - Returns `false` when activity is older than 7 days (never seen)
    - Returns `false` when activity is older than 7 days (has view)
12. Server integration test: `GetFeedStatus` and `GetFeed` agree —
    `has_new = true` for a community if and only if at least one item
    in its feed has `is_unread = true`
13. Client widget tests:
    - Unread dot shown when `is_unread = true`
    - Unread dot hidden when `is_unread = false`
14. Remove or update existing tests that asserted on deleted fields
    (`view_count`, `unseen_event_count`, `unseen_comment_count`,
    `most_recent_event_type`)

### Phase 8: Action-required pinning

**Goal:** Add the fourth `is_unread` trigger: items where another user has
requested to borrow or receive the current user's gear. Pin these items to
the top of the feed so the owner sees them immediately.

**Depends on:** Phase 7 (`is_unread` field and `isItemUnread()` helper).

#### Server: action-required detection

1. Add an `isActionRequired()` helper: for gear-type items where the
   viewing user is the gear owner, check whether there are pending
   transfer interest records (interest expressed, no recipient selected,
   not withdrawn/cancelled). Similarly for request-type items with
   pending offers. Set `is_action_required = true` on the `FeedItem`
2. **`is_action_required` is a sort/display hint only — it does NOT
   affect `is_unread`.** The unread state follows the same 7-day window
   rules as every other item. Once the user scrolls past an
   action-required item, it is read. The item only reappears as unread
   when new activity occurs (because the underlying interest generates
   new events)
3. This check must use the already-batch-fetched interest/offer count
   data from the feed pipeline — no additional per-item queries

#### Server: action-required sort priority

3. After computing `is_unread`, identify action-required items (the
   subset of unread items where the viewer owns the gear/request and
   has pending interest/offers). Sort these to the front of the final
   list. Within action-required items, sort by `last_activity_at_unix_sec`
   descending. All other items follow in their normal activity order
4. No `is_pinned` field on the proto — the client trusts server sort
   order. Action-required items appear first naturally. If the client
   needs to distinguish them visually (e.g., an "Action required" badge
   vs a generic unread dot), add `bool is_action_required = 10` to
   `FeedItem` as a display hint only

#### Server: `GetFeedStatus` update

5. No changes needed — `communityHasNewFeedItems()` already uses
   `isItemUnread()` (from Phase 7). Since `is_action_required` does not
   affect `is_unread`, the sidebar badge clears normally when the user
   views all items. No action-required-specific logic in feed_status.go

#### Client

6. If `is_action_required` is added (step 4), render an "Action
   required" badge on those items instead of the generic unread dot.
   Otherwise, the generic unread indicator from Phase 7 is sufficient —
   the item's position at the top of the feed already signals urgency
7. Add i18n key `feedActionRequired` ("Action required" /
   "Acción requerida") — only if `is_action_required` field is added

#### Clearing unread state

8. Scrolling past an action-required item clears `is_unread` via the
   existing `MarkFeedItemsViewed` mechanism — same as any other item.
   The item stays sorted to the top (via `is_action_required`) but the
   sidebar "New" badge clears. The item reappears as unread only when
   new activity occurs within the 7-day window

#### Tests

9. Server unit tests:
   - Item action-required when pending interest on viewer's gear
   - Item not action-required when interest withdrawn
   - Item not action-required when recipient already selected
   - Item not action-required when viewer is not the gear owner
   - Action-required items sorted before non-action-required items
   - `GetFeedStatus` returns `has_new = false` after viewing all items
     even when action-required items exist (sidebar badge clears)
   - Activity older than 7 days does not trigger unread
10. Client widget tests (if `is_action_required` added):
    - "Action required" badge shown for action-required items
    - Generic unread dot shown for other unread items

## Audit Log

Changes made during architecture compliance review against
`docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`:

1. **Duplicate step numbering fixed (Phase 4).** Phase 4 had two steps
   numbered "4". Renumbered to 4, 5, 6.

2. **Missing ViewModel layer added (architecture.md: rule 1 — ViewModels
   are the ONLY layer that calls Repositories).** Phase 5 originally went
   straight from a family provider to the repository, skipping the ViewModel
   layer. Added `FeedStatusNotifier` (Notifier, not autoDispose) following
   the same pattern as `UnreadCountNotifier` → `unreadCountProvider` →
   `unreadMessageCountForCommunityProvider`. The Notifier calls the
   repository; the family provider extracts per-community data from the
   Notifier state.

3. **"Short TTL" removed (caching.md: Global TTL Configuration).** Phase 5
   originally said `feed_status_repository.dart` should use a "short TTL."
   The caching system uses a single global TTL — per-repository TTLs are
   not supported. Corrected to use the standard global TTL with explicit
   invalidation for freshness.

4. **Cross-repository invalidation pattern specified (caching.md: Pattern
   7).** Phase 5 originally said `MarkFeedItemsViewed` in the ViewModel
   should "trigger a feed status cache invalidation." This leaked cache
   concerns into the ViewModel. Corrected: `FeedRepository.markItemsViewed()`
   calls `FeedStatusRepository.invalidate()` (repository-to-repository
   invalidation), and `FeedStatusNotifier` listens via an invalidation
   provider — same event-driven pattern as `dailyCacheInvalidationProvider`.

5. **Sidebar pill colors made theme-aware (architecture.md: Colors and
   Theming).** Phase 5 originally said "same coral styling as current badge."
   Hardcoded colors violate the theming rule. Corrected to use `AppColors.*`
   for all pill colors.

6. **`most_recent_event_type` changed from string to enum
   (architecture.md: type safety).** Phase 3 step 5 originally used
   `string most_recent_event_type`. Changed to `CommunityEventType` enum
   for end-to-end type safety, matching the existing proto enum.

7. **Duplicate step 7 numbering fixed (Phase 5).** Phase 5 had two steps
   numbered "7". Renumbered to 7, 8, 9.
