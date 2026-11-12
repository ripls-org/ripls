# Discover Screen v3 — Redesign

## Reference

- Design mockup: `tmp/discover3/discover.png`
- React reference: `tmp/discover3/discover.jsx`
- Geospatial system docs: `docs/client/geospatial.md`

## Problems

### 1. Search Is Hidden and Disconnected
The current discover screen has a floating search input at the top of the map and
a separate search result modal that slides up from the bottom. The search bar is
visually disconnected from the results — the user types at the top, then must
look down at a modal for results. This split layout creates cognitive overhead.

### 2. Too Many Filter Dropdowns
The search result modal contains five dropdown filters (Type, Location, Time,
State, People). Three of these (Time, State, People) are not even implemented
on the server — they toggle in the UI but don't filter results. The dropdowns
add clutter and confusion without delivering value.

### 3. No "Completed Items" Toggle
Users cannot toggle between viewing only open/active items vs. all items
including completed ones. The server-side search already filters to active items
by default (`e.state = '1'` for requests, state IN `('1', '2')` for unified
requests) but there's no way for the user to include completed items.

### 4. No Community Member Visibility
There's no way to browse items by person. Users can't see who in their community
is sharing what without searching for a specific item first.

### 5. Results Are Unsorted by Category
Search results are returned in a flat list sorted by composite score. Users can't
quickly scan for requests vs. events vs. newly shared gear — everything is mixed
together.

### 6. Map Starts Empty Until Search
The map shows no markers until the zero-state search completes. There's a flash
of empty map before items populate, and the initial load doesn't distinguish
between requests, events, and gear visually.

## Design (from Reference Mockup)

The redesigned discover screen has:

1. **Map fills the screen** with standard home header (no search bar in header)
   and bottom navigation from `home_screen.dart`.
2. **Bottom sheet** (draggable) containing all discover content:
   - **Drag handle** at top
   - **Search bar** with "Search Ripls" placeholder and an **Open/All toggle**
     (clock icon + label) on the right side, separated by a vertical divider
   - **Community member avatars** in a horizontal scrollable row (circle avatars
     with first name below)
   - **"Requests" section** with "See all" link — shows open requests with
     requester avatar, name, request text, time ago, and distance
   - **"Events" section** with "See all" link — shows upcoming experiences with
     calendar icon, title, host name, date/time, and distance
   - **"Sharing" section** with "See all" link — shows newly shared gear with
     box icon, item name, owner name, and distance
3. **Map markers** differentiated by type:
   - Coral/hand icon for requests
   - Sage/calendar icon for events (experiences)
   - Brown/box icon for gear items
4. **Blue dot** for user's current location

## Solutions

### Solution A: Bottom Sheet with Categorized Sections (Recommended)

Move search into the bottom sheet and organize results into three categorized
sections (Requests, Events, Sharing). The bottom sheet replaces the current
search modal entirely.

**Pros:**
- Matches the reference design exactly
- Simpler UX — one place for search and results
- Categorized sections let users scan quickly
- Open/All toggle is clean and minimal
- Avatars give community context at a glance

**Cons:**
- Breaking change from current discover UX
- More complex bottom sheet (sections with headers vs. flat list)
- Need to separate search results by type on the client

### Solution B: Keep Floating Search, Simplify Modal

Keep the search input floating above the map but simplify the modal to match the
new section layout and remove filter dropdowns.

**Pros:**
- Less disruptive change
- Search bar is always visible without expanding sheet

**Cons:**
- Doesn't match reference design
- Still has the disconnected search/results problem

### Recommendation

**Solution A.** It matches the reference designs, solves all the identified
problems, and is a cleaner UX. The existing bottom sheet infrastructure
(DraggableScrollableSheet, screenModalProvider) can be reused.

## Open Questions

1. **"See all" behavior**: Should "See all" for each section expand that section
   inline in the bottom sheet, or navigate to a filtered full-screen list?
   **Recommendation:** Expand inline by scrolling the sheet to full height and
   showing all items of that type. Simpler to implement, keeps the user on the
   discover screen.

2. **Avatar source**: **Resolved.** Use the same approach as the daily screen.
   The server's `GetDailyView` RPC already collects all owners/requesters from
   community items (gear, experiences, requests), batch-fetches their user
   protos, sorts by most recent share time, and returns them as `DailyPerson`
   (user_id, display_name, media_id). For discover, extract unique users from
   the search results on the client side — each `SearchResultItem` already
   contains the full `User` proto (via `gear.owner`, `request.requester`,
   `experience.owner`). Deduplicate by user_id, exclude self, and sort by most
   recent item. No new server endpoint needed.

3. **Person filter**: When tapping an avatar, should it filter all sections to
   that person's items, or navigate to a person-specific view?
   **Recommendation:** Filter all sections to that person's items within the same
   bottom sheet. Add a visible "X" or deselect mechanism to clear the person
   filter.

4. **Marker icons**: **Deferred.** The reference shows typed markers (hand icon
   for requests, calendar for events, box for gear). Keep the current uniform
   ripple circle markers for now. Typed icon markers will be a separate future
   project.

5. **"Open" vs "All" server-side**: The current server search already filters
   requests to active states. For the "All" toggle, do we need a new
   `include_completed` flag on `SearchRequest`, or can we handle it client-side?
   **Recommendation:** Add an `include_completed` field to the proto
   `SearchRequest`. This allows the server to include fulfilled/completed/
   cancelled items in results. Client-side filtering would require fetching
   everything and would be less efficient.

6. **Sheet snap positions**: The reference shows the sheet at roughly 65% height.
   What snap positions should the sheet have?
   **Recommendation:** Three positions: peek (25%, shows search + avatars),
   half (55%, shows search + avatars + first section), full (90%, all content).

## Implementation Plan

### Phase 1: Proto & Server — "include_completed" Support

Add server-side support for including completed items in search results.

**Files:**
- `proto/ripls/api/search_service.proto` — Add `bool include_completed = 8` to
  `SearchRequest`
- `server/storage/protosql_search.go` — Add `includeCompleted` parameter to
  config structs and adjust WHERE clauses (remove state filters when true)
- `server/services/search/service.go` — Pass `include_completed` through to
  storage queries
- `server/services/search/service_test.go` — Add tests for include_completed
  behavior

**Details:**
- When `include_completed = false` (default): keep current behavior — requests
  filtered to active/offers_received states, experiences filtered to
  active/joined states
- When `include_completed = true`: remove state filters, return all non-deleted
  items regardless of state
- Run `npm run generate` after proto change
- Run `npm run build` to verify server compiles

### Phase 2: Client Data Layer — Search API Update

Update the client search service and repository to support the new
`include_completed` parameter.

**Files:**
- `app/lib/services/search_service.dart` — Add `includeCompleted` parameter to
  `search()` method
- `app/lib/data/repositories/search_repository.dart` — Pass through
  `includeCompleted` and include in cache key

### Phase 3: ViewModel Updates — Categorized Results & State

<!-- AUDIT FIX: Moved from Phase 5 to Phase 3. ViewModel must be updated before
     widgets (Phases 4-5) because widgets depend on the categorized getters and
     person filter state. Per architecture.md: "Screens only interact with
     ViewModels" — widgets need the ViewModel contract defined first. -->

Update the search view model to provide categorized results, community member
list, and person filter support.

**Modified files:**
- `app/lib/presentation/viewmodels/search_view_model.dart`:
  - Add `includeCompleted` to `DiscoverFilters` (Freezed field)
  - Add `selectedPersonId` to `DiscoverFilters` (Freezed field)
  - Add computed getters on `SearchState`: `requests`, `events`, `sharing`
    (filtered from results by item type)
  - Add `communityMembers` computed getter on `SearchState` (extracted from item
    owners in search results, same pattern as daily screen's
    `buildCommunityPeople()` — deduplicate User protos, exclude self, sort by
    most recent item)
  - Wire `includeCompleted` into `searchWithLocation()` calls through to
    repository
  - Remove unused filter fields (`timeFilter`, `stateFilter`, `peopleFilter`,
    `locationFilter`)
  - Remove `DiscoverItemType` enum and type-based filtering (replaced by
    section getters)
  - Remove `ModalExpandState` enum (sheet manages its own drag state as local
    widget state, per architecture.md: "Use setState() for local UI state")
  - Add `filterByPerson(String? userId)` method
  - Add `toggleIncludeCompleted()` method
  <!-- AUDIT FIX: Async safety required per architecture.md "Async Safety in
       Notifiers" section. SearchNotifier uses SafeNotifierMixin — all new async
       methods must use safeUpdateState() or check ref.mounted after awaits. -->
  - All new async methods must use `safeUpdateState()` or guard with
    `if (!ref.mounted) return` after every async gap
- Run `flutter pub run build_runner build` after Freezed state changes

**Details:**
- `requests`: filter results where `itemType == REQUEST`
- `events`: filter results where `itemType == EXPERIENCE`
- `sharing`: filter results where `itemType == GEAR`
- Person filter applies across all three section getters
- Zero-state search (empty query) returns all items, populating all three
  sections on first load

**Tests:**
- `app/test/presentation/viewmodels/search_view_model_test.dart`:
  - Test `requests`/`events`/`sharing` getters correctly categorize results
  - Test `communityMembers` deduplicates and excludes self
  - Test `filterByPerson` filters all sections
  - Test `toggleIncludeCompleted` triggers new search with flag
  - Test disposal safety during async search operations

### Phase 4: Section Content Widgets

Build the individual section content widgets that render items in the categorized
layout matching the reference design.

**New files:**
- `app/lib/presentation/widgets/discover/discover_request_row.dart` — Request
  item row: avatar + name + description + time ago + distance
- `app/lib/presentation/widgets/discover/discover_event_row.dart` — Event item
  row: calendar icon badge + title + "Host · Date · Time" + distance
- `app/lib/presentation/widgets/discover/discover_sharing_row.dart` — Gear item
  row: box icon badge + name + "Owner · time ago" + distance
- `app/lib/presentation/widgets/discover/discover_avatar_row.dart` — Horizontal
  scrollable avatar row with tap handler

**Details:**
- Each row widget receives a `SearchDiscoverItem` and renders it according to
  the reference design
- Widgets access data only via `ref.watch(searchProvider)` — never call
  repositories or services directly (per architecture.md rule: "Screens only
  interact with ViewModels")
- Distance displayed in miles (converted from `distance_meters`)
- Time ago calculated from item timestamps
<!-- AUDIT FIX: Avatar media must use ImageCacheKeys.thumbnail() per caching.md
     "Image Cache Keys" section. Never use URL-based caching for user media. -->
- Avatar row renders using `CircleAvatar` with `CachedNetworkImageProvider` +
  initials fallback (matching daily screen's `_PersonAvatar` pattern). Avatar
  images must use `ImageCacheKeys.thumbnail(mediaId)` as cache key, not the
  URL (per caching.md: presigned URLs must not be used as cache keys)
- Tapping an avatar calls `ref.read(searchProvider.notifier).filterByPerson()`
  (coral selection ring); tapping again clears it
<!-- AUDIT FIX: Navigation must use NavigationHelpers per architecture.md
     "Navigation & Gestures" section. Never use raw Navigator.push. -->
- Tapping an item row navigates via `NavigationHelpers.pushToItemScreen()`
  (never raw `Navigator.push(MaterialPageRoute(...))`)

### Phase 5: Bottom Sheet Widget — Core Layout

<!-- AUDIT FIX: Moved from Phase 3 to Phase 5. Now depends on ViewModel (Phase 3)
     and section widgets (Phase 4) being ready. -->

Build the new bottom sheet widget that replaces the current search modal.

**New files:**
- `app/lib/presentation/widgets/discover/discover_bottom_sheet.dart` — Main
  DraggableScrollableSheet with drag handle, search bar, avatar row, and
  sectioned content

**Modified files:**
- `app/lib/presentation/screens/discover/discover_screen.dart`:
  - Replace FloatingSearchInput and DiscoverSearchModal with new bottom sheet
  - Remove the type chip row
  <!-- AUDIT FIX: The reference design shows NO gallery overlay — the bottom sheet
       replaces it entirely. The original plan said "keep gallery overlay" but
       that contradicts the reference mockup. -->
  - Remove the gallery overlay (`DiscoverGalleryOverlay`). The bottom sheet
    replaces both the search modal and the gallery overlay
  <!-- AUDIT FIX: The bottom sheet is always-visible (just at different snap
       heights), so it should be a direct child in the Stack, NOT injected via
       screenModalProvider. screenModalProvider is for conditional modals that
       appear/disappear. -->
  - Add bottom sheet as a direct child in the discover screen's Stack (not via
    `screenModalProvider`). The sheet is always visible at varying snap heights,
    unlike the old search modal which appeared/disappeared
  - Remove `screenModalProvider` injection for discover (no longer a modal)
- `app/lib/presentation/screens/home/home_screen.dart` — Remove
  `_discoverSearchController` and `_discoverSearchHasText` state (search now
  lives in the bottom sheet). Simplify discover AppBar (already transparent)

**Details:**
- Bottom sheet structure:
  - Drag handle (gray bar, centered)
  - Search row: search icon + text input + vertical divider + Open/All toggle
  - Avatar row: horizontal ListView of community member circles
  - "Requests" section: header with "See all", list of request cards
  - "Events" section: header with "See all", list of event cards
  - "Sharing" section: header with "See all", list of gear cards
- Search bar uses `TextField` with `InputDecoration` (no border, hint text
  "Search Ripls")
- Open/All toggle: tappable button with clock icon, calls
  `ref.read(searchProvider.notifier).toggleIncludeCompleted()`
- Snap sizes: [0.25, 0.55, 0.90]
- Sheet drag state is local widget state (`DraggableScrollableController`) — not
  stored in the ViewModel (per architecture.md: "Use setState() for local UI
  state like text field focus")

### Phase 6: Cleanup

Remove deprecated files and code.

**Delete:**
- `app/lib/presentation/widgets/discover_search_modal.dart` — Replaced by
  discover_bottom_sheet
- `app/lib/presentation/widgets/floating_search_input.dart` — No longer used
  (search is in bottom sheet)
- `app/lib/presentation/screens/discover/discover_gallery_overlay.dart` —
  Replaced by bottom sheet sections

**Clean up:**
- Remove `DiscoverItemType` enum and `DiscoverItemTypeLabels` extension
- Remove `ModalExpandState` enum
- Remove `_discoverSearchController` from home_screen.dart
- Remove type chip row from discover_screen.dart
- Remove `screenModalProvider` usage from discover_screen.dart
- Update `docs/client/geospatial.md` to reflect new architecture

### Phase 7: Visual Polish & UX Fixes

Fix visual and UX issues identified after initial implementation.

**Avatar images not showing:**
- The `discover_avatar_row.dart` `_MemberAvatar` widget duplicates avatar
  rendering logic (media URL fetching, initials fallback, cache keys). Replace
  with the shared `UserAvatar` widget (`app/lib/presentation/widgets/user_avatar.dart`)
  which already handles `mediaUrlProvider`, `ImageCacheKeys.thumbnail()`,
  initials fallback, and error states. Keep the coral selection ring and first-
  name label as a wrapper around `UserAvatar`
- Remove all duplicated avatar rendering code from `_MemberAvatar` to keep DRY

**Include current user in avatar row:**
- The `communityMembers` getter currently excludes the current user. Change to
  include the current user and place them first in the list (index 0), followed
  by other members sorted by most recent item. This lets users tap their own
  avatar to see only their items

**Remove filter chips:**
- Remove any remaining filter chip rows from the bottom sheet. The only filter
  control is the Open/All toggle integrated into the search bar. No standalone
  chip widgets should appear

**Search bar visual fixes (match reference design):**
- Wrong height: search bar should be ~48px tall (matching reference), not the
  current oversized height
- Search icon placement: the magnifying glass icon must be inside the input
  box (left-aligned within the rounded container), not outside it
- Border: use a single light border (`1px solid borderLight`) with rounded
  corners (radius 12), no heavy outline or focus border
- The search input, vertical divider, and Open/All toggle are all inside the
  same rounded container as one unified row

**Bottom sheet background color:**
- The bottom sheet background must use the warm cream/card color from the theme
  (`AppColors.cardBackground` or equivalent, ~#FEFBF5), not white or the
  default scaffold background. Match the reference design's warm off-white tone

**"See all" should be a toggle (expand/collapse):**
- Each section (Requests, Events, Sharing) shows a limited preview (e.g., 2
  items) by default. "See all" expands to show all items of that type and the
  label changes to "Show less". Tapping "Show less" collapses back to the
  preview count
- This is local widget state (a `Set<SectionType>` of expanded sections), not
  ViewModel state

**Events/experiences should show thumbnails, not icons:**
- The event rows currently show a calendar icon badge. Replace with the item's
  thumbnail image (first media ID) rendered as a small rounded-square thumbnail
  (42x42, borderRadius 12). Fall back to the calendar icon badge only if the
  experience has no media. Use `CachedMediaImage` with
  `ImageCacheKeys.thumbnail(mediaId)` for the thumbnail

**Faint separator lines between items:**
- Add a faint horizontal divider between items within each section (not above
  the first item). Use a 1px line with the theme's light border color
  (`AppColors.border` or ~#F3EFE8). Match the reference design where dividers
  separate rows but don't appear at the top or bottom of a section

### Phase 8: Fix Empty Discover Screen on First Load

**Problem:**
Sometimes when a user logs in and opens the discover screen, no results are
shown — the bottom sheet sections are empty and the map has no markers. Tapping
the search input area triggers a search and results appear. Unless the community
has zero members and zero shared items (extremely rare), data should always be
visible on first load.

**Evidence from logs:**
The client logs show `SearchViewModel build()` fires and logs "Community
available (Boulder Backcountry Skiers) — triggering initial search" at
08:53:56.696. Location is successfully resolved at 08:53:57.260. However, the
**server logs show no Search RPC was ever received**. The initial search is
triggered on the client but never reaches the server. Every other RPC
(media, impact, transfer context, unread counts, chain) completes normally —
only the search is missing.

**Root cause analysis:**
The initial search is triggered in `SearchViewModel.build()` via
`searchWithLocation()`, which:
1. Fetches the user's location asynchronously
2. Debounces for 300ms
3. Then calls `_executeSearch()` or `_executeSearchWithLocation()`

Likely failure modes:
- **Debounce race**: The 300ms debounce timer may be cancelled by a subsequent
  state change (community re-selection, filter update, or a second `build()`
  invocation) before the search executes
- **Async gap + disposal**: The `build()` method triggers the search, but a
  Riverpod state update (e.g., community provider re-emitting) causes the
  notifier to rebuild, cancelling the in-flight search
- **`ref.mounted` guard**: After the async location fetch completes, a
  `ref.mounted` check may return false (if the notifier was rebuilt during the
  ~500ms location fetch), silently dropping the search
- **Modal visibility gate**: The current code may gate search execution on
  `isModalVisible` or some other UI state that isn't yet true during the initial
  `build()` call

**Fix approach:**
- Add detailed logging to `searchWithLocation()` at each stage: before debounce,
  after debounce, before location fetch, after location fetch, before RPC call,
  after RPC response. This will pinpoint exactly where the initial search is
  being dropped
- Ensure the zero-state search in `build()` bypasses the debounce timer (it's
  the initial load, not a user keystroke — debouncing serves no purpose here)
- Ensure the zero-state search does NOT check `isModalVisible` or any UI state
  gate — results must load regardless of sheet position
- Verify that community provider re-emissions during startup don't cause
  `build()` to re-run and cancel the in-flight search. The logs show
  `SelectedCommunityNotifier` fires multiple times ("Setting communities",
  "Restored persisted community", "Community selection finalized") — each may
  trigger a `ref.watch()` in the SearchNotifier's `build()`, causing rebuilds
- Consider using `ref.keepAlive()` to prevent the notifier from being disposed
  during the initial async search
- Add a guard: if `build()` has already triggered the initial search and it's
  in-flight, don't cancel it on rebuild — let it complete
