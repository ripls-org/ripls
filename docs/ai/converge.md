# Navigation Convergence: Multi-Community Context

## Problems

### 1. Single-Community Context Is Limiting
The app currently operates within a single community at a time. Users must switch communities in the sidebar to see different feeds, discover screens, and metrics. This creates friction:
- Users miss activity in non-selected communities
- Switching communities is a full context reset (feed reinitializes, discover clears markers)
- No cross-community view of what needs attention

### 2. No Dedicated Inbox
There is no top-level inbox tab. Users discover pending actions (loan requests, RSVPs, help offers, messages) by navigating into individual items or through the portfolio screen. The reference design shows a "Time" inbox as the second tab with clustered activity cards organized by urgency ("Needs your attention" vs "FYI").

### 3. Items Live in One Community
When creating gear, experiences, or requests, items are bound to the currently selected community. The reference design shows items shared across multiple communities with per-community conversation threads and sponsor/provenance banners.

### 4. Create Button Position
The current create button (+) is in the 4th bottom nav position. The reference design places it in the center (3rd position) as a prominent floating action button, with the inbox taking the 2nd slot.

## Current Architecture

### State Management
- `SelectedCommunityState` holds a single `CommunityItem? selectedCommunity` plus the full list
- `SelectedCommunityNotifier` persists the single selection to SharedPreferences
- All screens (feed, discover, metrics) read `selectedCommunityProvider` and filter by that one ID
- Server APIs (`GetFeed`, `Search`, `GetCommunityMetrics`) accept a single `community_id`

### Navigation
- `HomeScreen` uses a 4-tab `IndexedStack`: Feed (0), Discover (1), Metrics (2), Add (3 - modal)
- Sidebar selects one community or deselects (portfolio mode)
- Feed shows empty state when no community selected

### Key Files
- Sidebar: `app/lib/presentation/screens/home/sidebar.dart`
- Home/nav: `app/lib/presentation/screens/home/home_screen.dart`
- State: `app/lib/services/providers.dart` (lines 759-935)
- Feed VM: `app/lib/presentation/viewmodels/feed_view_model.dart`
- Feed screen: `app/lib/presentation/screens/feed/feed_screen.dart`
- Discover: `app/lib/presentation/screens/discover/discover_screen.dart`
- Metrics: `app/lib/presentation/screens/impact_metrics/metrics_screen.dart`
- Portfolio inbox: `app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart`
- Inbox VM: `app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart`

## Reference Design Analysis

From `docs/cowork/App Design/trust-relay-mock.jsx`:

### Sidebar
- Multi-select toggles per community (checkbox-style, not radio)
- "Select all" / "Clear all" toggle
- Badge on hamburger showing enabled count
- Default: all communities enabled

### Bottom Nav (5 items)
1. **Pulse** (Activity icon) - Full-screen feed
2. **Time** (Calendar icon) - Portfolio inbox with weekly calendar, clustered action cards
3. **+** (Plus icon) - Create button, centered, accent-colored circle
4. **Search** (Search icon) - Discover screen
5. **Impact** (BarChart3 icon) - Metrics screen

### Feed
- Filters items by `enabled[item.community]` - any enabled community shows its items
- Items have a `communities` array (multi-community) and a primary `community`
- Per-community chat tabs with community dropdown picker on multi-community items

### Inbox ("Time" / "Home" screen)
- Weekly day strip with community-colored dots showing scheduled activity
- Cards grouped by "Needs your attention" and "FYI"
- Each card shows clustered activity (RSVPs, offers, chats, reviews)
- Community badges per card showing which communities the item is shared in

### Create Flow
- Items can be assigned to multiple communities
- "Share with another group" button in community picker dropdown

## Potential Solutions

### A. Sidebar: Multi-Select Communities

**Option A1: Set of enabled community IDs (recommended)**
Replace `CommunityItem? selectedCommunity` with `Set<String> enabledCommunityIds`. Default to all enabled. Persist the set to SharedPreferences.

**Option A2: Per-community boolean map**
Use `Map<String, bool>` keyed by community ID. Equivalent semantically but slightly more verbose.

**Recommendation:** A1. A `Set<String>` is clean, serializable, and the empty-set case is unambiguous. The notifier exposes `toggle(id)`, `enableAll()`, `disableAll()` methods.

### B. Server API: Multi-Community Queries

**Option B1: Repeated `community_ids` field**
Add `repeated string community_ids = N` to `GetFeedRequest`, `SearchRequest`, `GetCommunityMetricsRequest`. Server returns the union. Keep the old `community_id` field for backward compatibility during migration.

**Option B2: Client-side merge**
Make parallel requests per community and merge client-side.

**Recommendation:** B1. Client-side merge creates N+1 API calls, higher latency, and complex deduplication (items shared across communities). A server-side union query is straightforward with `WHERE community_id = ANY($1)`.

### C. Bottom Nav Reorganization

**Option C1: 5 tabs (matching reference)**
Feed, Inbox, Create(+), Discover, Impact. The create button is a styled FAB in the center.

**Option C2: 4 tabs + FAB overlay**
Keep 4 real tabs, overlay the FAB between tabs 2 and 3. Avoids changing IndexedStack indexing.

**Recommendation:** C1. The reference design uses 5 items and the centered create button is a clear affordance. The IndexedStack only needs 4 entries (create opens a modal, not a screen).

### D. Inbox Tab

**Option D1: Promote existing PortfolioInboxScreen**
The app already has `PortfolioInboxScreen` and `PortfolioInboxViewModel` with inbox items, filtering, and action cards. Lift this into the bottom nav as a top-level tab, adapting it to aggregate across enabled communities.

**Option D2: Build new InboxScreen from scratch**
Create a new screen matching the reference design's weekly calendar + clustered cards layout.

**Recommendation:** D1 as the starting point. The existing portfolio inbox already has the data fetching, action card rendering, and filter infrastructure. Add the weekly calendar strip and cluster grouping on top. This reuses the most code.

### E. Multi-Community Item Creation

**Option E1: Community picker modal during creation**
After the user fills in item details, show a modal with checkboxes for their communities. Default to the most recently used community. Server creates the item and shares it to selected communities.

**Option E2: Community field in creation form**
Add a community selector inline in the creation form (chip selector).

**Recommendation:** E1. A modal keeps the creation forms clean and the community selection is a distinct step. The reference design shows this pattern ("Share with another group" in the dropdown). This also requires a new server API field (`repeated string community_ids`) on create requests.

### F. Feed Multi-Community Filtering

**Option F1: Pass enabled set to feed API**
Feed VM reads the enabled community set and passes all IDs to `GetFeed`. Server returns items from any of those communities.

**Option F2: Subscribe to community changes reactively**
Watch `enabledCommunityIdsProvider` and reinitialize feed when the set changes.

**Recommendation:** Both. F1 for the API call, F2 for reactivity. When the user toggles a community in the sidebar, the feed should refresh with the new set.

## Open Questions

1. **Server API versioning**: Should we add `community_ids` alongside existing `community_id` fields and handle both, or deprecate the single-ID field?

   **Recommendation:** Add `repeated string community_ids` alongside the existing `community_id` field. Server logic checks `community_ids` first; if empty, falls back to `community_id`. This avoids a coordinated client/server deploy and lets the mobile app roll out gradually. Deprecate `community_id` after the app no longer sends it (track via server logs). No need to remove it from the proto — just stop reading it once all clients have migrated.

2. **Realtime updates**: The app has realtime stream support. When enabled communities change, should we resubscribe the stream, or filter events client-side?

   **Recommendation:** Filter client-side. The realtime stream already delivers events for all the user's communities. Adding a community filter to the stream subscription would require server-side subscription management and reconnection logic for every sidebar toggle. Client-side filtering is a single `if (!enabledIds.contains(event.communityId)) return;` check — negligible cost, zero latency, and no reconnection flicker. If bandwidth becomes a concern (unlikely given event volumes), revisit with server-side filtering later.

3. **Item deduplication**: Items shared across multiple enabled communities should appear once in the feed, not once per community. Which community's context (conversation, sponsor) should display?

   **Recommendation:** Deduplicate server-side using `SELECT DISTINCT ON (item_id)` ordered by the item's primary community (the community it was originally created in). The feed response should include the item's full `repeated string community_ids` list so the client can render community tags. For conversations, follow the reference design: the community selector is integrated into the chat tab itself rather than a separate dropdown. When a user taps the chat tab on a multi-community item, it shows the primary community's conversation with a community picker built into the tab button (showing the community short name and a chevron). Tapping the picker while already on the chat tab opens a community list to switch threads. For sponsors, show the sponsor relevant to the community through which the viewing user discovered the item — this requires the server to annotate which community "surfaced" each feed item.

4. **Portfolio mode**: Currently deselecting all communities enters "portfolio mode" showing the user's own items. With multi-select, what does "no communities selected" mean?

   **Recommendation:** Allow zero communities selected — it means "show only the user's own items" (portfolio mode). When no communities are enabled, the feed, discover, and metrics screens show only items the user owns or is directly involved in, regardless of community. This preserves the existing portfolio concept naturally within the multi-select model: "Select all" shows everything, "Clear all" narrows to your own items, and any subset in between filters accordingly. The sidebar's "Clear all" / "Select all" toggle works as-is. The existing `PortfolioScreen` remains accessible via the user's avatar/name in the sidebar header for the full portfolio management experience.

5. **Inbox data source**: The reference "Time" screen shows clustered activity (RSVPs, offers, chats). Does the existing inbox API (`GetInbox`) support cross-community aggregation, or does it need the same `community_ids` treatment?

   **Recommendation:** The existing `GetInbox` API likely fetches by user, not by community — inbox items are "things that need your attention" regardless of community context. Verify this by checking the inbox query. If it already returns all the user's items, no API change is needed; just add community ID tags to the response so the client can display community badges per card. If it does filter by community, apply the same `community_ids` pattern. The clustering logic (grouping RSVPs, offers, chats per item) should be server-side to avoid sending raw event lists to the client.

6. **Metrics aggregation**: When multiple communities are enabled, should metrics show a combined total or per-community breakdown?

   **Recommendation:** Both, matching the reference design. Show combined totals at the top (the three metric rings: money saved, CO2, time) aggregated across all enabled communities. Below that, show a per-community breakdown table so users can see which communities drive the most impact. The API should accept `repeated string community_ids` and return both a `totals` object and a `repeated CommunityMetrics per_community` list. This avoids multiple API calls and lets the server compute the aggregation efficiently.

7. **Offline/cache behavior**: The current cache is keyed by `namespace:item_id`. With multi-community feeds, the feed cache key needs to incorporate the enabled set (or be invalidated on toggle).

   **Recommendation:** Invalidate the feed cache on community toggle rather than keying by the enabled set. Keying by set creates a combinatorial explosion of cache entries (2^N for N communities) that rarely get reused. Instead, keep the feed cache key simple (`feed:list`) and call `invalidateFeed()` whenever `enabledCommunityIds` changes. Individual item caches (`feed:item_id`) remain valid across toggles since the item data doesn't change — only the list composition changes. This matches the existing pattern where `selectCommunity()` already invalidates the feed cache.

8. **Search scope**: Should discover/search have its own community filter, or use the sidebar selection?

   **Recommendation:** Search uses the sidebar's enabled communities exclusively — no separate community filter in the search/discover UI. This keeps the mental model simple: the sidebar is the single source of truth for "which communities am I looking at right now," and it applies uniformly to feed, inbox, discover, and metrics. Remove any existing community filter from the search UI. If a user wants to search a different set of communities, they adjust the sidebar.

## Architecture Audit

The following issues were identified by auditing this plan against the reference
architecture docs (`docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`). All fixes are applied
inline in the phased plan below.

1. **[FIXED] Data layer boundary violation**: Original plan had screens passing
   community IDs directly to APIs. Per architecture rule "ViewModels are the
   ONLY layer that calls Repositories" and "Repositories are the ONLY layer
   that calls Services", all data flow must go Screen → ViewModel → Repository
   → Service. Fixed: plan now specifies the full layer chain.

2. **[FIXED] Feed cache key strategy underspecified**: The feed repository
   currently caches as `feed:$communityId`. With multi-community feeds, the
   cache key must change. Using a sorted, joined key
   (`feed:$sortedJoinedIds`) would create a combinatorial explosion. Fixed:
   use a single `feed:list` key and invalidate on community toggle, matching
   the Q7 recommendation.

3. **[FIXED] Feed repository + service layer missing**: Original plan only
   mentioned feed screen and view model changes but omitted
   `FeedRepository.getFeed()` and `FeedService.getFeed()`, which both take a
   single `communityId` string. Fixed: plan now includes repository and
   service updates to accept `List<String> communityIds`.

4. **[FIXED] Server membership validation**: The server's `GetFeed` and
   `Search` handlers call `requireMembership()` for a single community ID.
   With `community_ids`, the server must validate membership in ALL provided
   communities. Fixed: added to server change items.

5. **[FIXED] Server feed generation**: `generateFeedItems()` generates feed
   for a single community. With `community_ids` it must loop across
   communities and deduplicate. Fixed: added to server change items.

6. **[FIXED] MarkFeedItemsViewed gap**: `MarkFeedItemsViewedRequest` also
   takes `community_id`. Items from a multi-community feed still belong to a
   specific community — the view-mark call should continue using the item's
   own community_id, not the enabled set. No proto change needed, but the
   client must pass the correct per-item community_id. Fixed: noted in plan.

7. **[FIXED] Bottom nav index mapping**: With 5 nav items but only 4
   IndexedStack entries, the create button at visual index 2 must be skipped
   when mapping to IndexedStack indices. Fixed: added explicit index mapping
   table.

8. **[FIXED] i18n compliance**: All new user-visible strings (empty states,
   button labels, section headers) must be added to both `app_en.arb` and
   `app_es.arb`. ViewModels must not resolve strings — they expose typed
   state. Fixed: added i18n callout to each phase.

9. **[FIXED] SearchNotifier community watch**: `SearchNotifier.build()` watches
   `selectedCommunityProvider` and triggers initial search on community change.
   This must change to watch the new `enabledCommunityIdsProvider` and pass the
   set to search. Fixed: added to discover screen changes.

10. **[FIXED] Server multi-community creation**: Creating an item shared to
    multiple communities requires creating multiple junction records
    (`CommunityGear`, `CommunityRequest`, `CommunityExperience`) and multiple
    conversation threads in a single transaction. Fixed: added to Phase 3
    server changes.

11. **[FIXED] Portfolio inbox already cross-community**: The
    `GetPortfolioInboxWeek` API takes no `community_id` — it already fetches
    across all the user's communities. No API change needed for the inbox tab.
    Fixed: clarified in Phase 2.

## Phased Implementation Plan

### Phase 1: Multi-Select Sidebar + State Foundation
**Goal:** Replace single-community selection with multi-select. All screens continue to work but now aggregate across enabled communities.

**Changes:**
1. **State migration** (`providers.dart`):
   - Keep `List<CommunityItem> communities` for name/detail lookup
   - Replace `CommunityItem? selectedCommunity` with `Set<String> enabledCommunityIds`
   - Add methods: `toggleCommunity(id)`, `enableAll()`, `disableAll()`, `isEnabled(id)`
   - Add derived getter: `List<CommunityItem> get enabledCommunities` (filters communities list by enabledCommunityIds)
   - Persist set to SharedPreferences (JSON-encoded list)
   - Default: all communities enabled on first load
   - Keep `communities` list for downstream consumers that need community names/details

2. **Sidebar update** (`sidebar.dart`):
   - Replace single-select tap with toggle behavior (checkmark per community)
   - Add "Select all" / "Clear all" button
   - Show enabled count badge on hamburger icon
   - Zero communities selected = portfolio mode (user's own items only)
   - **i18n**: Add ARB entries for "Select all", "Clear all" labels (both locales)

3. **Proto + server API** (`feed_service.proto`, `search_service.proto`):
   - Add `repeated string community_ids` to `GetFeedRequest`, `SearchRequest`
   - Server: accept either `community_id` or `community_ids`, prefer `community_ids` if present
   - **Server `GetFeed` handler**: Validate membership in ALL provided community_ids (loop `requireMembership`). Call `generateFeedItems` per community and merge results. Deduplicate by item ID using a seen-set (not SQL DISTINCT — feed items are generated in-memory, not queried directly)
   - **Server `Search` handler**: Validate membership in ALL provided community_ids. Execute search per community and merge/deduplicate results by item ID. Cap combined results at `max_results`
   - **`MarkFeedItemsViewed`**: No proto change needed. Each feed item belongs to a specific community — the client must continue passing the item's own `community_id` (available from the feed item response), not the enabled set

4. **Feed data layer** (full chain: screen → viewmodel → repository → service):
   - **`FeedService.getFeed()`**: Change signature from `getFeed({required String communityId, ...})` to `getFeed({required List<String> communityIds, ...})`. Build `GetFeedRequest` with `communityIds` field
   - **`FeedRepository.getFeed()`**: Change signature from `getFeed(String communityId, ...)` to `getFeed(List<String> communityIds, ...)`. Cache key: use `'feed:list'` (single key). Invalidate on community toggle via `invalidateFeed()` (no parameter needed)
   - **`FeedNotifier.initialize()`**: Change from `initialize(String communityId)` to `initialize(List<String> communityIds)`. Read enabled set from `selectedCommunityProvider`
   - **`FeedScreen`**: Watch `selectedCommunityProvider.enabledCommunityIds` instead of `selectedCommunityProvider.selectedCommunity`. Detect changes by comparing sets, reinitialize feed via `ref.read(feedProvider.notifier).initialize(communityIds)`. When zero communities enabled, show portfolio-mode empty state

5. **Discover data layer** (`discover_screen.dart`, `search_view_model.dart`, `search_repository.dart`, `search_service.dart`):
   - **`SearchService`**: Change `communityId` parameter to `List<String> communityIds`
   - **`SearchRepository`**: Update cache key strategy for multi-community
   - **`SearchNotifier.build()`**: Watch `selectedCommunityProvider.enabledCommunityIds` instead of `selectedCommunityProvider.selectedCommunity`. Trigger initial search with the full enabled set. Guard against duplicate searches using the set (not a single ID)
   - **`SearchNotifier.searchWithLocation()`**: Change `communityId` parameter to `List<String> communityIds`
   - **`DiscoverScreen`**: Read enabled community IDs from provider, pass to search notifier. Remove any community-specific filter from the UI — sidebar is the sole filter
   - Show markers from all enabled communities

6. **Metrics screen** (`metrics_screen.dart`):
   - Read `enabledCommunityIds` from provider
   - Fetch metrics for all enabled communities (may need API update or parallel fetches)
   - Show combined or per-community view

**i18n**: All new user-facing strings (empty states, labels) must be added to both `app_en.arb` and `app_es.arb` before the phase is complete.

**Testing:** Verify feed shows items from multiple communities, sidebar toggles work, state persists across app restarts. Verify that the full data chain (screen → VM → repo → service → server) passes community IDs correctly.

### Phase 2: Bottom Nav Reorganization + Inbox Tab
**Goal:** Add inbox as a top-level tab, reposition create button.

**Changes:**
1. **Bottom nav** (`home_screen.dart`):
   - Change from 4 items to 5: Feed, Inbox, Create(+), Discover, Impact
   - Create button styled as centered FAB (accent color circle, no label)
   - IndexedStack has 4 entries (Feed=0, Inbox=1, Discover=2, Impact=3)
   - Create(+) at visual index 2 opens modal, not a screen — skip in IndexedStack
   - **Index mapping** (visual → IndexedStack):

     | Visual pos | Label    | Nav index | IndexedStack index |
     |------------|----------|-----------|--------------------|
     | 0          | Feed     | 0         | 0                  |
     | 1          | Inbox    | 1         | 1                  |
     | 2          | Create   | 2         | — (modal)          |
     | 3          | Discover | 3         | 2                  |
     | 4          | Impact   | 4         | 3                  |

   - Update `_onBottomNavTap`: index 2 triggers `_onAddTap()`, indices 3/4 map to IndexedStack 2/3
   - Update `_logScreenViewEvent`: adjust tab index mapping
   - Update `_buildAppBar`: adjust case indices for new mapping
   - Update `HomeNotifier`/`HomeState.selectedIndex`: store IndexedStack index (0–3), not visual index
   - **Scroll controllers**: Add `inboxScrollControllerProvider` for the new tab

2. **Inbox tab** (promote/adapt `portfolio_inbox_screen.dart`):
   - Lift PortfolioInboxScreen to be a top-level tab in the IndexedStack
   - **No API change needed**: `GetPortfolioInboxWeek` already fetches across all the user's communities (no community_id parameter)
   - Add community badge tags to inbox cards showing which communities each item is shared in (data already available in `DailyItem.communityId` / `communityName`)
   - Add weekly day strip calendar (from reference design)
   - Group cards into "Needs your attention" and "FYI" sections

3. **Inbox view model** (`portfolio_inbox_view_model.dart`):
   - Already fetches across all communities — no data-layer changes needed
   - Add clustering logic (group RSVPs, offers, chats per item) as UI-level grouping in the ViewModel (not service layer)
   - Sort by urgency/recency

4. **Unread badge** (`unread_count_view_model.dart`):
   - Show unread count on inbox tab icon
   - Aggregate across enabled communities

**i18n**: Add ARB entries for new tab labels ("Inbox"), section headers ("Needs your attention", "FYI"), and any new empty state messages.

**Testing:** Verify nav layout matches reference, inbox shows cross-community items, create modal opens from center button. Verify index mapping is correct — tapping each nav item reaches the right screen.

### Phase 3: Multi-Community Item Creation
**Goal:** Allow items to be shared to multiple communities at creation time.

**Changes:**
1. **Community picker modal** (new widget in `app/lib/presentation/widgets/`):
   - Checkbox list of user's communities (read from `selectedCommunityProvider.communities`)
   - Heading uses `context.l10n` key (not hardcoded string)
   - Confirm button
   - Returns `List<String>` of selected community IDs

2. **Create flows** (`create_gear_modal.dart`, `request_creation_modal.dart`, `experience_creation_modal.dart`):
   - Add "Share to communities" button that opens picker modal
   - Show selected community chips/tags
   - Remove requirement for single pre-selected community
   - Default selection: currently enabled communities (or most recently used)
   - **`request_creation_modal.dart`**: Change `communityId` constructor parameter to `List<String> communityIds`
   - **`experience_creation_modal.dart`** and **`create_gear_modal.dart`**: Add `communityIds` parameter (currently they have no explicit community parameter)

3. **Proto + server API**:
   - Add `repeated string community_ids` to create request messages (gear, request, experience)
   - **Server must create junction records in a transaction**: For each selected community, insert `CommunityGear`/`CommunityRequest`/`CommunityExperience` junction record. Create per-community conversation thread for each. Wrap in `storage.BeginTx()` with deferred rollback per server architecture patterns
   - Server validates membership in all provided community_ids before creating

4. **Feed card updates**:
   - Show community tags on items shared to multiple communities
   - Community selector integrated into the chat tab button (shows community short name + chevron for multi-community items; tapping while on chat tab opens community picker inline)

**i18n**: Add ARB entries for community picker heading, confirm button, "Share to communities" label, community tag labels.

**Testing:** Verify items can be created and shared to multiple communities, conversations are separate per community, feed shows community tags. Verify server creates all junction records and conversations in one transaction.

### Phase 4: Polish and Edge Cases
**Goal:** Handle remaining UX details and edge cases.

**Changes:**
1. **Portfolio access**: Tap user avatar/name in sidebar header navigates to full `PortfolioScreen` via `NavigationHelpers.pushWithSlide()` (not raw `Navigator.push`). Zero communities selected also shows portfolio-style view in feed/discover/metrics
2. **Empty states**: Update empty state messages for multi-community context. All strings via `context.l10n` — add ARB entries to both `app_en.arb` and `app_es.arb`
3. **Realtime updates**: Filter stream events client-side by enabled community set (no resubscription needed)
4. **Cache invalidation**: Invalidate `feed:list` cache key when `enabledCommunityIds` changes. Individual item caches (`feed:$itemId`) remain valid. Use the existing `invalidateFeed()` pattern via repository, not direct cache access from ViewModels
5. **Deep links**: Ensure deep links into items auto-enable the relevant community
6. **Performance**: Verify that enabling all communities doesn't cause excessive data fetching
7. **Desktop layout**: Ensure sidebar multi-select works well in always-visible desktop mode
8. **Colors**: All new UI elements must use `AppColors.xxx(context)` for theme-aware colors (no hardcoded `Colors.white`, `Colors.grey`, or hex values)
