# Issue #1896: Feed: 'Search nearby' header → suggestions sheet → simplified one-row search results with filter button

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1896
**Filed:** 2026-05-13 | **Last activity:** 2026-05-13
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/cowork/App Design/direction-a-working-mock.html](../cowork/App%20Design/direction-a-working-mock.html), [docs/cowork/mocks/simple/new_feed.png](../cowork/mocks/simple/new_feed.png), [docs/cowork/mocks/simple/new_search_suggestions.png](../cowork/mocks/simple/new_search_suggestions.png), [docs/cowork/mocks/simple/new_search_results.png](../cowork/mocks/simple/new_search_results.png), [docs/client/the_feed.md](../client/the_feed.md)

## Problem Statement

Search is currently a dedicated bottom-nav tab that lands the user in
`DiscoverScreen` (full-screen map + draggable bottom sheet with three
typed sections: Help Needed list, Upcoming Events carousel, Available
Gear 2-column grid). Issue #1895 removes the Search tab; that leaves
the search machinery (the [`Search` RPC](../../proto/ripls/api/search_service.proto),
[`SearchNotifier`](../../app/lib/presentation/viewmodels/search_view_model.dart),
[`SearchRepository`](../../app/lib/data/repositories/search_repository.dart),
the [`DiscoverFilterSheet`](../../app/lib/presentation/widgets/discover/discover_filter_sheet.dart))
intact but with no entry point.

This issue rewires the entry point: the Feed's floating header gains a
**"Search nearby"** pill that, on tap, opens a **search-suggestions
sheet** (Quick Search chips + Recent — neither exists today). On submit
or chip-tap, the existing search RPC fires and the user lands in a
**simplified search-results surface**: the same map + bottom-sheet
shape as today, but with **one unified row per item** (replacing the
three typed widget families) and a **Filters button** that opens the
existing `DiscoverFilterSheet` unchanged.

Three sub-pieces are new surfaces — not just rewiring:

1. **Search-nearby pill** in the Feed's floating header (idle → focused).
2. **Suggestions sheet** with Quick Search chips and Recent (recent
   searches storage is also new).
3. **Unified one-row result widget** that replaces the typed
   help/events/gear renderers in the results bottom sheet.

Everything else — the `Search` RPC, the filter sheet, location
auto-fetch, debouncing, caching — is reused as-is.

## Context from Comments

No comments on the issue — it was filed and immediately planned as part
of the four-issue nav restructure (#1895–#1898). The issue body is the
full specification; the mock in `direction-a-working-mock.html` is the
visual source of truth for both the suggestions panel (lines 382–425)
and the simplified results view (lines 427–495).

## Audit Notes (2026-05-12)

This plan was audited against [docs/client/architecture.md](../client/architecture.md),
[docs/client/caching.md](../client/caching.md),
[docs/client/modals.md](../client/modals.md),
[docs/client/accessibility.md](../client/accessibility.md),
[docs/client/i18n.md](../client/i18n.md),
[docs/client/testing.md](../client/testing.md),
[docs/client/logout.md](../client/logout.md),
[docs/client/the_feed.md](../client/the_feed.md), and the relevant
server docs (`architecture.md`, `observability.md`). The
following corrections were folded into the plan body; this section
records what changed and why.

- **Recent-searches storage simplified to a single AsyncNotifier
  (no separate repository).** Per [i18n.md](../client/i18n.md)
  the existing precedent is `localePreferenceProvider` — "a
  Riverpod AsyncNotifier backed by SharedPreferences" — with no
  intermediary repository class. [caching.md](../client/caching.md)
  reserves the `Repository` term and `data/repositories/` directory
  for Stash-backed caching repositories (`CacheService(cacheManager,
  namespace)`). A SharedPreferences-only store doesn't fit that
  pattern; introducing a "repository" that doesn't use `CacheService`
  would mislead the next reader. Phase 1 tasks collapsed accordingly.
- **Pull-to-refresh in Phase 3 reuses the existing
  `searchCacheInvalidationProvider` / `refreshCount` mechanism**
  (caching.md Pattern 6 "Automatic Cache Invalidation with UI
  Preservation"). The plan already implied this; the audit makes it
  explicit so the implementer doesn't reinvent a parallel refresh
  path.
- **Orphan-flagging comment style.** Per [CLAUDE.md "Go Code
  Comments"](../../CLAUDE.md#go-code-comments) (which applies to Dart
  comments too — see also `scripts/check_ephemeral_comments.js` for
  the enforced rule), do not use phase / rollout language in
  in-source comments. Issue numbers are explicitly allowed. Phase 4's
  comment-style task spells out an acceptable phrasing.
- **Server docs scope.** The `Search` RPC is untouched by this issue
  (client-only rewiring). Server `architecture.md` and `observability.md`
  were read; no server changes required, so no structured-logging or
  observability tasks were added.

## Codebase Survey

- **Feed floating header** — [feed_screen.dart:75–144](../../app/lib/presentation/screens/feed/feed_screen.dart#L75)
  passes `FloatingHeader(showMenuIcon: true, title: _feedHeaderTitle())`
  today. After #1895 the hamburger is gone; the header has free
  real estate for the search pill. `FloatingHeader` accepts a `content:`
  slot for arbitrary center widgets — natural place for the search pill.
- **Search RPC + ViewModel + Repository** — fully built and
  production-grade:
  - `Search` RPC at [search_service.proto:26–59](../../proto/ripls/api/search_service.proto#L26).
    Inputs: `query`, `community_ids[]`, `lat/lng`, `max_results`,
    `item_types[]`, `include_completed`. Output: `SearchResultItem[]`
    sorted by composite_score.
  - [`SearchNotifier`](../../app/lib/presentation/viewmodels/search_view_model.dart) with
    `SearchState` carrying `results`, `currentQuery`, `currentCommunityIds`,
    `currentLatitude/Longitude`, `filters: DiscoverFilters`,
    `isLoading`, `error`. `searchWithLocation()` is debounced 300ms and
    auto-fetches location.
  - [`SearchRepository`](../../app/lib/data/repositories/search_repository.dart) wraps
    `SearchService` with cache keyed by query + location + community.
  - **Community scoping today:** `SearchNotifier.build()` watches
    `selectedCommunityProvider` and rebuilds on selection change. With
    #1895 forcing `effectiveCommunityIds` to always return the full
    list, this still works correctly (same call, same data) — but the
    explicit watch becomes inert. Phase 2 tweaks it to read once
    rather than watch.
- **DiscoverScreen** — [discover_screen.dart:34–636](../../app/lib/presentation/screens/discover/discover_screen.dart#L34).
  Hosts the search input today ([line 553–586](../../app/lib/presentation/screens/discover/discover_screen.dart#L553)),
  the map ([line 271–307](../../app/lib/presentation/screens/discover/discover_screen.dart#L271)),
  and the categorized results sheet ([line 627–632](../../app/lib/presentation/screens/discover/discover_screen.dart#L627)).
  After #1895 it's no longer a tab; after #1896 it's the destination
  pushed when a query is submitted from the suggestions sheet. The
  Discover screen's "no-query" empty state (which renders the typed
  sections without a search term) becomes unreachable under the new
  flow — flagged for cleanup, not deleted in this issue.
- **Result widgets — typed today, unified after this issue:**
  - [`discover_help_card.dart`](../../app/lib/presentation/widgets/discover/discover_help_card.dart) — magazine photo card (210px hero / 140px secondary).
  - [`discover_event_carousel.dart`](../../app/lib/presentation/widgets/discover/discover_event_carousel.dart) — horizontal carousel.
  - [`discover_gear_grid.dart`](../../app/lib/presentation/widgets/discover/discover_gear_grid.dart) — 2-col grid.
  - The mock's results view (lines 471–488) shows a single
    `Row` per item: 56×56 thumbnail (with unread coral dot in the
    top-right corner) + title + meta line (`{who} · {community} ·
    {distance}`) + chevron. **Same shape for every item type.**
- **`DiscoverFilterSheet`** — [discover_filter_sheet.dart:19–233](../../app/lib/presentation/widgets/discover/discover_filter_sheet.dart#L19).
  60%-height glass modal. Sections: Map Style, Show on Map, Distance,
  Sort By, Include Completed. Has a `static show(BuildContext)` entry
  point. Reuse as-is; just call `DiscoverFilterSheet.show(context)`
  from the new Filters button.
- **No suggestions surface today** — net-new for #1896. No recent
  searches persistence, no quick-search chip data, no autocomplete.
- **Existing l10n** under `discover*` / `discoverFilter*` in
  [app_en.arb](../../app/lib/l10n/app_en.arb) — reuse where it fits;
  add new `feedSearch*` / `searchSuggestions*` keys.

## Potential Solutions

### Option A: Bolt the pill onto the Feed header; push DiscoverScreen on submit

- **Approach:**
  - Add the "Search nearby" pill to the Feed's `FloatingHeader.content`
    slot. On tap, open a glass bottom-sheet that renders Quick Search
    chips (hardcoded list with l10n) + Recent (read from
    SharedPreferences via a tiny new repository).
  - On submit (Enter / chip tap / recent tap), pop the suggestions
    sheet, persist the term to Recent, and push `DiscoverScreen` with
    an initial query seeded in `SearchNotifier`.
  - Reshape `DiscoverScreen`'s results bottom sheet to render a single
    new `SearchResultRow` widget for every result type, replacing the
    three typed widget families. Add the Filters button → calls
    `DiscoverFilterSheet.show(context)`.
- **Pros:** Smallest surface area of new code. Reuses the entire
  Search RPC + repository + filter sheet untouched. Recent-searches
  storage is a thin slice. The result-row reshape touches DiscoverScreen
  but leaves the underlying machinery intact.
- **Cons:** The DiscoverScreen's "no-query" empty state (categorized
  sections) becomes unreachable but stays in the codebase until the
  cleanup PR — slightly confusing for a contributor reading the screen
  in isolation.
- **Effort:** Medium.

### Option B: Inline search-results within the Feed (no DiscoverScreen push)

- **Approach:** Render the suggestions sheet AND the search-results
  surface in-place over the Feed (as overlays / a state-machine on the
  Feed screen). Don't push DiscoverScreen at all; reshape its results
  widgets into reusable pieces and consume them from the Feed.
- **Pros:** Cleaner conceptually — Feed owns the entire search lifecycle;
  DiscoverScreen becomes plumbing-only and easier to delete. The mock's
  `direction-a-working-mock.html` actually models it this way (one
  `screen` state with `feed` / `searchResults` modes inside the App
  component).
- **Cons:** Larger refactor — DiscoverScreen has a map + draggable sheet
  pattern that's nontrivial to extract. The Feed today is TikTok-style
  full-screen swipes ([the_feed.md](../client/the_feed.md)); turning it
  into a host for an inline map+sheet is a bigger reshape than the issue
  scope suggests ("most of this is wiring the new Feed entry point into
  [Discover] and simplifying the result row").
- **Effort:** Large.

### Option C: Bolt the pill onto the Feed but route to a fresh `SearchResultsScreen`

- **Approach:** Same Feed entry + suggestions sheet as Option A, but
  build a brand-new `SearchResultsScreen` that hosts the map + new
  one-row sheet. Don't touch DiscoverScreen at all — leave it to the
  cleanup PR to delete.
- **Pros:** Cleanest separation between old and new. No risk of
  breaking the existing DiscoverScreen surface while it's still wired
  in `IndexedStack` (the tab itself is removed in #1895, but the
  widget stays for backward routing per #1895's Phase 3).
- **Cons:** Duplicates the map + draggable sheet plumbing that already
  exists in DiscoverScreen — bigger diff, more code to maintain until
  the cleanup PR collapses one. The issue text explicitly says "route
  into the existing Discover screen search and search results
  implementation" — naming the existing screen, not a new one.
- **Effort:** Medium-Large.

## Recommendation

**Option A.** The issue text is explicit: "route into the existing
Discover screen search and search results implementation." That's the
contract. The only judgment call is whether the DiscoverScreen reshape
of result rows is invasive enough to justify Option C's duplication —
and it isn't. The result-row reshape is bounded: replace three widget
references inside `DiscoverBottomSheet` with one new
`SearchResultRow` rendering, decouple the typed sections (help / events
/ gear), and verify the existing per-section grouping either collapses
or remains as a header above the unified rows.

DiscoverScreen stays the destination. Its "no-query" empty state
becomes orphaned-but-alive; the cleanup PR after #1895/#1896/#1897/#1898
collapses it.

## Resolved Decisions (2026-05-12 issue review)

All open questions were resolved by accepting the recommendations.
Decisions are folded into the Implementation Plan below; recorded
here for the audit trail.

- **Recent searches persist in `SharedPreferences`**, keyed by user
  id (`recent_searches_<user_id>`). Cap 8, FIFO, dedupe on insert.
  Wiped automatically on logout via the existing `prefs.clear()`
  step in [logout.md](../client/logout.md). Server-side query
  history deferred.
- **Quick Search chips are hardcoded** — 4 chips ("Help needed
  nearby", "Tools to borrow", "Events this week", "People in your
  circles") with l10n strings and static seed queries. No dynamic
  config; revisit if usage signals demand it.
- **Quick Search chip tap sends its seed query verbatim** through
  `SearchNotifier.searchWithLocation(query: q)`. No filter overrides
  (e.g. no auto-applied `item_types: [EXPERIENCE]` for the events
  chip). Filter surfacing through chips is a future enhancement.
- **Suggestions sheet dismisses** (`Navigator.pop(query)`) on:
  Quick Search chip tap, Recent pill tap, keyboard "Search" action
  on a typed query, or system-barrier tap-outside (handled
  automatically by `showAccessibleModal`).
- **Search query is handed to DiscoverScreen via `SearchNotifier`** —
  the suggestions sheet's caller calls
  `ref.read(searchProvider.notifier).searchWithLocation(query: q)`
  before pushing `DiscoverScreen`. DiscoverScreen reads from the
  same provider on build. No `arguments:` wiring.
- **The "Search nearby" pill sits in `FloatingHeader.content:`** (the
  center slot). It replaces the Feed's current title. If a location
  hint is needed, fold it into the pill's idle-state placeholder
  (italic gray to the right of the search icon).
- **Result-row unread coral dot ships only if data is cheaply
  available.** If `SearchResultItem` plus existing per-user view
  state can produce an "unread for this user" flag without an extra
  RPC, render it; otherwise drop the dot for v1 (the mock reads fine
  without it).
- **Result list is flat**, ordered by the server's
  `composite_score`-descending. No per-type section headers. A
  single "Results for '{query}'" header at the top.
- **The "Search nearby" pill stays pinned** in the floating header
  while results are open — always available to start a new search.

## Implementation Plan

### Phase 1: Add the "Search nearby" pill + suggestions sheet (entry point only)

**Goal:** Ship the Feed-header pill and a working suggestions sheet
that, on submit, opens `DiscoverScreen` seeded with the query. No
result-row reshape in this phase — DiscoverScreen renders results
exactly as it does today. This isolates the new surfaces from the
results-layout reshape so each can be reviewed independently.

**Tasks:**
- [ ] Add new l10n keys to **both** [app/lib/l10n/app_en.arb](../../app/lib/l10n/app_en.arb)
      **and** [app/lib/l10n/app_es.arb](../../app/lib/l10n/app_es.arb):
  - `feedSearchNearbyPlaceholder` — pill copy ("Search nearby").
  - `searchSuggestionsQuickSearchHeader` — section header
    ("QUICK SEARCH").
  - `searchSuggestionsRecentHeader` — section header ("RECENT").
  - `searchSuggestionsChipHelpNearby`, `searchSuggestionsChipToolsToBorrow`,
    `searchSuggestionsChipEventsThisWeek`,
    `searchSuggestionsChipPeopleInCircles` — chip labels.
  - `searchSuggestionsChipHelpNearbyQuery`,
    `searchSuggestionsChipToolsToBorrowQuery`,
    `searchSuggestionsChipEventsThisWeekQuery`,
    `searchSuggestionsChipPeopleInCirclesQuery` — chip seed-query
    strings (kept as l10n so localizations can rewrite them if needed).
  - `a11yOpenSearchSuggestions` — pill tap target semantic label.

      Run `npm run generate` to regenerate `AppLocalizations`.
- [ ] Create the recent-searches store as a **single
      `AsyncNotifier<List<String>>`** that talks directly to
      `SharedPreferences` — no separate repository class. This
      mirrors [`localePreferenceProvider`](../../app/lib/presentation/viewmodels/locale_view_model.dart)'s
      precedent in [i18n.md](../client/i18n.md) and keeps the
      `data/repositories/` directory reserved for `CacheService`-backed
      Stash repositories per [caching.md](../client/caching.md).
  - File: `app/lib/presentation/viewmodels/recent_searches_view_model.dart`.
  - Notifier: `RecentSearchesNotifier extends AsyncNotifier<List<String>>`.
    `build()` reads the JSON-encoded string list from SharedPreferences
    under key `recent_searches_<user_id>` (return `[]` when absent).
    Methods: `Future<void> add(String query)` — dedupes (move-to-front
    if already present), prepends, trims to 8, writes back, and bumps
    state via `state = AsyncData(updated)` after a `ref.mounted` guard.
    `Future<void> clear()` — writes an empty list, state = `AsyncData([])`.
  - Use `AsyncNotifier`'s automatic `build()` disposal handling per
    [architecture.md "Async Safety in Notifiers"](../client/architecture.md#async-safety-in-notifiers);
    add `ref.mounted` checks before state assignments in `add` /
    `clear` (manual methods, not `build`).
  - Logout teardown is already covered — [logout.md](../client/logout.md)
    step 5 calls `prefs.clear()`, which wipes the user's recent
    searches as a side-effect. No new teardown step needed.
  - Unit tests per [docs/client/testing.md](../client/testing.md)
    "ViewModel Testing (Primary Focus)": ProviderContainer +
    `SharedPreferences.setMockInitialValues({})`. Cover: empty state,
    add-then-build returns the new entry, FIFO trim at 9th entry,
    dedupe (re-adding an existing term moves it to the front), clear.
    Disposal-safety test per [architecture.md "Disposal Testing"](../client/architecture.md#disposal-testing)
    (dispose during `add`'s SharedPreferences write completes without
    throwing).
- [ ] Build the suggestions sheet at
      `app/lib/presentation/widgets/search/search_suggestions_sheet.dart`.
  - **Modal shape:** glass bottom-sheet via `showAccessibleModal` +
    `GlassSheet`, per [modals.md "Standard glass-modal shape"](../client/modals.md#standard-glass-modal-shape).
    No new third pattern; reuse the same primitives `unified_time_modal.dart`
    and the new Profile menu modal (#1895 Phase 1) use.
  - Body: a `Column` inside `GlassSheet`:
    - `GlassFieldLabel` "QUICK SEARCH" header.
    - 2×2 grid of Quick Search chips (custom widget — each chip is a
      [`Tappable`](../../app/lib/presentation/widgets/accessibility/)
      with `semanticsLabel: context.l10n.<chipKey>` wrapping the
      icon + label. Color tokens from `AppColors.*(context)`, not
      hardcoded). On tap: `Navigator.pop(query)` with the chip's
      seed-query.
    - `GlassFieldLabel` "RECENT" header.
    - `ref.watch(recentSearchesProvider).when(...)` → a wrap of
      `Tappable` pills (clock icon + text). On tap: `Navigator.pop(query)`.
  - **Optional text-entry field at the top of the sheet** so the user
    can type a freeform query. Use [`GlassSearchInput`](../../app/lib/presentation/widgets/modal/glass/)
    per [modals.md](../client/modals.md#glass-modal-primitives). On
    submit (`TextInputAction.search`), `Navigator.pop(query)`.
  - `static Future<String?> show(BuildContext context)` entry point
    following the canonical pattern in `modals.md`. Returns the
    submitted query string or `null` if dismissed.
- [ ] Build the "Search nearby" pill widget at
      `app/lib/presentation/widgets/search/search_nearby_pill.dart`.
  - Idle state per `new_feed.png`: rounded pill with a search icon
    on the left, "Search nearby" placeholder text in the center, no
    border (just a translucent fill — use `AppColors.modalSurface` or
    add a new `AppColors.searchPillBackground(context)` semantic
    helper).
  - Wrap in [`Tappable`](../../app/lib/presentation/widgets/accessibility/)
    with `semanticsLabel: context.l10n.a11yOpenSearchSuggestions`.
  - On tap:
    ```dart
    final query = await SearchSuggestionsSheet.show(context);
    if (!context.mounted) return;
    if (query == null || query.isEmpty) return;
    await ref.read(recentSearchesProvider.notifier).add(query);
    if (!context.mounted) return;
    ref.read(searchProvider.notifier).searchWithLocation(query: query);
    NavigationHelpers.pushScreen(
      context,
      const DiscoverScreen(),
      useRootNavigator: true,
    );
    ```
    Three `context.mounted` guards per the [Widget Async Exception
    pattern](../client/architecture.md#widget-async-exceptions).
- [ ] In [feed_screen.dart](../../app/lib/presentation/screens/feed/feed_screen.dart),
      pass the new pill via `FloatingHeader.content:`. After #1895
      lands, `showMenuIcon` is gone; just compose
      `FloatingHeader(content: SearchNearbyPill())`. Drop the
      `title:` argument and the `_feedHeaderTitle()` helper if the
      pill fully replaces it (verify against `new_feed.png`).
- [ ] Run `npm run lint:dart:a11y` to confirm the new `Tappable`s all
      have l10n-sourced `semanticsLabel`s.

**Validates:** Tapping the pill opens the suggestions sheet; tapping a
chip or recent pill or submitting a typed query closes the sheet,
adds the term to Recent, and lands the user in `DiscoverScreen` with
results matching the query. Result rendering is unchanged (typed
sections). `flutter test`, `lint:dart:a11y`, and `lint:dart:i18n` pass.

### Phase 2: Simplify the search results to one row per item + add Filters button

**Goal:** Reshape `DiscoverScreen`'s results bottom sheet to render a
single unified `SearchResultRow` widget for every result type, and
add the Filters button that opens the existing `DiscoverFilterSheet`
unchanged. After this phase, the user-visible search experience
matches `new_search_results.png` end-to-end.

**Tasks:**
- [ ] Add l10n keys to **both** ARB files:
  - `searchResultsHeader` — parameterized: `"Results for \"{query}\""`.
  - `searchResultsMatchCount` — parameterized: `"{count, plural, =1{1 match} other{{count} matches}}"`.
  - `searchResultsFiltersButton` — "Filters".
  - `a11ySearchResultRow` — parameterized with title + author + community
    + distance, for the row's `Tappable.semanticsLabel`.
  - `a11yOpenFilters` — Filters button label.

      Run `npm run generate`.
- [ ] Build the unified result row widget at
      `app/lib/presentation/widgets/search/search_result_row.dart`.
  - Layout (per `new_search_results.png` and `direction-a-working-mock.html`
    lines 471–488):
    - 56×56 rounded square thumbnail (`CachedMediaImage` with a
      content-specific `semanticsLabel:` per [accessibility.md](../client/accessibility.md)).
    - **Unread coral dot** in the thumbnail's top-right corner —
      render only if a per-result "unread for this user" flag is
      cheaply derivable from `SearchResultItem` + existing per-user
      view state (no extra RPC). If not, omit the dot for v1; do
      not add new RPC work to surface it. Decide during
      implementation by inspecting whether the search-result payload
      already carries the signal or it requires an extra join.
    - Title (serif/bold, single-line, truncated with ellipsis).
    - Meta line: `{author} · ●{community} · {distance}` where the
      community-dot is a small filled circle colored per community
      (reuse whatever color logic the feed cards use for community
      identity today; if none exists, drop the colored dot for v1).
    - Trailing chevron right.
  - Inputs: a `SearchResultItem` proto. Picks thumbnail / title /
    author / community / distance from the proto fields. **No
    type-specific branching** — every result type renders the same
    way.
  - Wrap the row in [`Tappable`](../../app/lib/presentation/widgets/accessibility/)
    with `semanticsLabel` built from `context.l10n.a11ySearchResultRow(...)`.
    On tap: navigate to the underlying content screen via
    `NavigationHelpers.pushToItemScreen(context, itemType: ..., itemId: ...)`
    per [architecture.md "Navigation & Gestures"](../client/architecture.md#navigation--gestures).
  - Theme-aware colors throughout (`AppColors.*(context)`); no
    `Colors.white` / hardcoded hex.
- [ ] In [discover_bottom_sheet.dart](../../app/lib/presentation/widgets/discover/discover_bottom_sheet.dart):
  - Replace the three typed sections (the `_helpCards`,
    `_eventCarousel`, `_gearGrid` renderers around lines 239–297)
    with a flat `ListView.separated` of `SearchResultRow`s, one per
    `state.results` item, ordered by the existing
    `composite_score`-descending list from the server.
  - Add the header row above the list: `Text("Results for \"$query\"")`
    on the left (using `context.l10n.searchResultsHeader`), and the
    Filters button on the right.
  - **Filters button:** a small rounded pill with a slider icon and
    "Filters" text per the mock (lines 461–469). Tap →
    `await DiscoverFilterSheet.show(context); if (!context.mounted) return;`.
    Reuse the existing `DiscoverFilterSheet` — do not redesign.
  - Optional: render the match-count chip top-right of the map
    (`{count} matches · {distance}`) per the mock lines 450–453.
    Defer to a follow-up if the data isn't readily available.
- [ ] Delete or stub the three typed widget families (`discover_help_card.dart`,
      `discover_event_carousel.dart`, `discover_gear_grid.dart`) IF no
      other surface in the app uses them. Grep first; if any code
      outside the Discover screen consumes them, leave the widgets in
      place and let the cleanup PR delete them once those consumers are
      gone. **Default: leave alone.** The issue note explicitly says
      "Most of this is wiring … and simplifying the result row" —
      keep the diff bounded.
- [ ] In [search_view_model.dart](../../app/lib/presentation/viewmodels/search_view_model.dart):
  - Stop `watch`ing `selectedCommunityProvider` in
    `SearchNotifier.build()` — read once via `ref.read` and pass the
    `effectiveCommunityIds` into the first search call. Per #1895
    Phase 4 the provider is neutered to always return the full list,
    so the watch no longer triggers meaningful rebuilds — turning it
    into a read avoids a no-op rebuild on selection-state churn.
  - **Cache implications:** existing cache keys derived from
    `effectiveCommunityIds.join(',')` now always use the full-portfolio
    key. Per #1895 Phase 4's cache note, this is benign (old per-
    community entries expire on the 30-minute TTL). No manual
    invalidation needed.
- [ ] **Tests:**
  - ViewModel test (per [docs/client/testing.md](../client/testing.md)):
    `SearchNotifier` reads `effectiveCommunityIds` once at build and
    does not rebuild when an irrelevant provider changes.
  - Widget test for `SearchResultRow`: renders title, author,
    community, distance; tap dispatches to the right
    `NavigationHelpers.pushToItemScreen` call for each item type
    (gear / request / experience).
  - Widget test for the bottom-sheet header: Filters button tap calls
    `DiscoverFilterSheet.show`.

**Validates:** Search results render as a flat one-row-per-item list
matching the mock; Filters button opens the existing filter sheet
unchanged; community-filter neutering from #1895 is honored by
SearchNotifier; no regressions in tap-to-detail navigation.

### Phase 3: Polish — pull-to-refresh, empty state, error surface

**Goal:** Round out the search-results surface with the small details
the issue doesn't enumerate but the mock and existing
DiscoverScreen behavior imply.

**Tasks:**
- [ ] **Empty state.** When `state.results` is empty after a
      non-empty query, render a centered helper string (reuse the
      existing `discoverNoResultsMatchFilters` or
      `discoverNothingHereYet` l10n key — already in `app_en.arb`).
- [ ] **Error state.** When `state.error` is non-null, surface it via
      the existing `RpcErrorHandler.localize` pattern per
      [i18n.md](../client/i18n.md). Render in the bottom sheet
      header area, not as a full-screen banner.
- [ ] **Pull-to-refresh** on the results list — wraps the
      `ListView.separated` in a `RefreshIndicator`. Reuse the existing
      `SearchNotifier._refreshInPlace()` machinery (the
      `searchCacheInvalidationProvider` / `refreshCount` mechanism
      documented as [caching.md Pattern 6 "Automatic Cache
      Invalidation with UI Preservation"](../client/caching.md#pattern-6-automatic-cache-invalidation-with-ui-preservation)) —
      **do not** introduce a parallel refresh path. If
      `SearchNotifier` doesn't already expose a public `refresh()`
      method that delegates to `_refreshInPlace()`, add one in
      Phase 2 so the `RefreshIndicator.onRefresh` callback has a
      clean entry point. The notifier already handles the
      `refreshCount` bump internally; the widget just awaits the
      future.
- [ ] **Recent-searches clear control** inside the suggestions sheet:
      a small "Clear" affordance on the RECENT section header. On
      tap: `ref.read(recentSearchesProvider.notifier).clear()`.
- [ ] **Manual visual review** against `new_feed.png`,
      `new_search_suggestions.png`, `new_search_results.png`, AND
      against `direction-a-working-mock.html` for animation /
      transition parity. Verify in dark mode (theme-aware colors).

**Validates:** Edge cases (empty results, network errors, stale
recents) are handled cleanly. Visual parity with the mocks is
confirmed.

### Phase 4: Flag dead code for the cleanup PR

**Goal:** Surface — not delete — the now-orphaned surfaces so the
post-#1895/#1896/#1897/#1898 cleanup PR can find them.

**Tasks:**
- [ ] Add a comment at the top of [discover_screen.dart](../../app/lib/presentation/screens/discover/discover_screen.dart)'s
      "no-query" code path (the empty-state branch that renders the
      typed categorized sections) marking it as unreachable. Per
      [CLAUDE.md "Go Code Comments"](../../CLAUDE.md#go-code-comments)
      (the no-phase-markers rule applies to Dart comments too —
      `scripts/check_ephemeral_comments.js` enforces it), use only
      the stable issue reference, not rollout-phase language. Example
      acceptable phrasing:

      ```dart
      // Unreachable from the app — DiscoverScreen is only entered
      // from the Feed's search submit, which always seeds a query.
      // Slated for deletion in the post-#1895/#1896/#1897/#1898 cleanup PR.
      ```

      Avoid: "after Phase 4", "post-restructure", "new in #1896",
      "pre-cleanup-PR", or any phrasing that references rollout state
      rather than the durable issue ID.
- [ ] If any of the three typed result widgets
      (`discover_help_card.dart`, `discover_event_carousel.dart`,
      `discover_gear_grid.dart`) are now completely unreferenced
      (grep confirms no callers in `lib/`), flag them in the cleanup
      PR's tracking note. Don't delete them in this issue — keeps
      review scope tight.
- [ ] Update the cleanup-PR tracking comment in #1895 to add the
      DiscoverScreen empty state + typed result widgets to the
      sweep list.

**Validates:** Dead code is identified and queued; this issue
ships without churn unrelated to its scope.

## Related Issues

- #1895 — Bottom + floating nav restructure. **Hard dependency** — the
  Feed's floating header is only free for the search pill once #1895
  removes the hamburger. Phase 2's `SearchNotifier` decoupling also
  builds on #1895 Phase 4's `selectedCommunityProvider` neutering.
- #1897 — Inbox floating-nav search bar. Pattern overlap — the inbox
  also gains a search-pill-in-header. After both land, consider
  factoring a shared "header-search-pill" widget; defer that
  refactor until both ship to avoid premature abstraction.
- #1898 — Workshop community carousel. No direct dependency. Same
  cleanup PR.
- (TBD post-#1895/#1896/#1897/#1898) — Dead-code cleanup PR. Adds
  DiscoverScreen's no-query empty state and (if unreferenced) the
  three typed result widgets to its sweep list.
