# Filter Modal Redesign

> ## Audit Summary (2026-03-15)
>
> Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
> and `docs/server/architecture.md`. Six inline fixes applied, marked with
> **AUDIT** callouts:
>
> 1. **Theming compliance** (Reference Design § Visual Style): All UI colors
>    must use `AppColors.xxx(context)` — added color mapping table. Gradient
>    thumbnails for map styles are exempt (they depict map appearance, not app
>    chrome).
> 2. **Client/API boundary** (Solutions § Category Filtering): Client must only
>    use `ripls/api/` protos, never `ripls/models/`. Sharing vs. Giving
>    distinction must be resolved via `api.Gear` fields or the categories
>    should be collapsed to match `SearchItemType`.
> 3. **Freezed field types** (Phase 1 § Task 1): `sortBy` must be a plain Dart
>    enum, `mapStyle` must be `String` — clarified from "enum or String".
> 4. **No unnecessary RPCs** (Phase 1 § Task 3): Category, distance, and sort
>    filters are client-side only — notifier methods must not re-execute search
>    RPCs.
> 5. **Test pattern** (Phase 1 § Tests): Must use `ProviderContainer` with mock
>    repositories, following existing test patterns.
> 6. **Persistence pattern** (Solutions § Persisting Filter State, Phase 5):
>    Filter preferences are user settings, not API data — SharedPreferences is
>    correct, not the Stash-based repository/cache pattern.
>
> **Verdict**: Plan is architecturally sound. No structural violations. The
> client-side filtering approach correctly avoids unnecessary server changes
> and keeps all filter logic in the ViewModel layer where it belongs.

## Table of Contents

1. [Problem Statement](#problem-statement)
2. [Current State](#current-state)
3. [Reference Design](#reference-design)
4. [Solutions & Recommendations](#solutions--recommendations)
5. [Open Questions](#open-questions)
6. [Phased Implementation Plan](#phased-implementation-plan)

---

## Problem Statement

The current search filter modal on the discover screen is a minimal scaffold with
only one functional filter (Include Completed toggle) and three "Coming soon"
placeholders. The design needs to be upgraded to a richer, more polished modal
that matches the reference design, adding functional category filters, distance
options, sort-by controls, a map style picker, and a prominent "Show Results"
action button.

### Issues with Current Implementation

1. **Incomplete filters**: Distance, item type, and date range are placeholders
   with no functionality
2. **No map style control**: The map is hardcoded to `MapboxStyles.STANDARD`
   with the "faded" theme — users cannot switch styles
3. **No category filtering**: Users cannot toggle which item types (requests,
   events, sharing, giving) appear on the map and in the bottom sheet
4. **No distance filtering**: The `SearchRequest` proto accepts lat/lng but has
   no radius/distance filter field — all results are returned regardless of
   distance
5. **No sort control**: Results are always sorted by the server's composite
   score (a blend of semantic relevance and proximity) — users cannot choose
   nearest-first or newest-first
6. **Visual design gap**: The current modal uses a flat list of rows with
   dividers; the reference uses a scrollable sheet with section headers, chip
   grids, map thumbnails, and a sticky apply button

---

## Current State

### Filter Sheet UI

- **File**: `app/lib/presentation/widgets/discover/discover_filter_sheet.dart`
- Modal bottom sheet with handle, title, one toggle row, and three "Coming soon"
  rows
- Opened via `DiscoverFilterSheet.show(context)` from the filter button in the
  floating header

### Filter State

- **File**: `app/lib/presentation/viewmodels/search_view_model.dart`
- `DiscoverFilters` (freezed): `includeCompleted` (bool) + `selectedPersonId`
  (String?)
- `SearchNotifier.toggleIncludeCompleted()` re-executes the search RPC
- `filterByPerson()` applies a client-side filter (no network call)

### Search API

- **File**: `proto/ripls/api/search_service.proto`
- `SearchRequest` fields: `query`, `strategy`, `community_id`, `latitude_deg`,
  `longitude_deg`, `item_types` (repeated enum), `max_results`,
  `include_completed`
- `SearchResultItem` includes `distance_meters` and `composite_score`
- `SearchItemType` enum: `GEAR`, `REQUEST`, `EXPERIENCE`, `USER`
- **Note**: The proto already has `item_types` (repeated filter) and
  `distance_meters` (on results) — these are just unused by the client

### Map Style

- **File**: `app/lib/presentation/screens/discover/discover_screen.dart` (line
  266)
- Hardcoded: `styleUri: MapboxStyles.STANDARD`
- **File**: `app/lib/core/utils/discover_map_helper.dart` (lines 52-59)
- Applies `{"theme": "faded"}` to the basemap after style loads

### Available Mapbox Styles

From `mapbox_maps_flutter` (`MapboxStyles` class):

| Constant             | Style URI                                      | Reference Label |
| -------------------- | ---------------------------------------------- | --------------- |
| `STANDARD`           | `mapbox://styles/mapbox/standard`              | Streets         |
| `OUTDOORS`           | `mapbox://styles/mapbox/outdoors-v12`          | Outdoor         |
| `SATELLITE`          | `mapbox://styles/mapbox/satellite-v9`          | Satellite       |
| `DARK`               | `mapbox://styles/mapbox/dark-v11`              | Dark            |
| `STANDARD_SATELLITE` | `mapbox://styles/mapbox/standard-satellite`    | (not in ref)    |
| `LIGHT`              | `mapbox://styles/mapbox/light-v11`             | (not in ref)    |
| `MAPBOX_STREETS`     | `mapbox://styles/mapbox/streets-v12`           | (not in ref)    |
| `SATELLITE_STREETS`  | `mapbox://styles/mapbox/satellite-streets-v12` | (not in ref)    |

---

## Reference Design

The reference (`tmp/filter/modal.png` and `tmp/filter/modal.jsx`) specifies a
bottom sheet with these sections, top to bottom:

### Layout

- Drag handle at top
- **Header row**: "Filters" title (serif, bold) + Reset button (when active
  filters) + X close button
- **Scrollable body** with sections:

### Sections

1. **Map Style** — 4 mini map thumbnails in a 4-column grid (Streets, Outdoor,
   Satellite, Dark), coral border on active, label below each
2. **Show on Map** — 2×2 grid of category chips (Requests, Events, Sharing,
   Giving), each with label + checkbox, coral border when active
3. **Distance** — 5 horizontal pills (½ mi, 1 mi, 5 mi, 10 mi, Any), coral
   background on active
4. **Sort By** — 3 horizontal pills (Relevancy, Nearest, Newest), coral
   background on active
5. **Include Completed** — toggle row with description text
6. **Sticky "Show Results" button** — full-width coral button pinned at bottom,
   separated by a thin border

### Visual Style

- Section headers: uppercase, small, muted, letter-spaced
- Chips: rounded (14px), cream background, coral border + filled checkbox when
  active
- Distance/sort pills: rounded (12px), cream background, coral background +
  border when active
- Toggle: iOS-style switch, coral when on
- Apply button: full-width, coral, white text, rounded (16px), shadow

> **AUDIT — Theming compliance**: The reference JSX uses hardcoded hex colors
> (`#FDF8F0`, `#D4714E`, etc.). The Flutter implementation **must not** use
> hardcoded colors. All colors must use `AppColors.xxx(context)` helpers to
> support dark mode (per `docs/client/architecture.md` — Colors and Theming).
> Map the reference palette to existing `AppColors` semantics:
>
> | Reference color | Flutter equivalent |
> |---|---|
> | cream / card bg | `AppColors.surface(context)` or `AppColors.cardBackground(context)` |
> | coral (active) | `AppColors.primary(context)` |
> | coral bg (soft) | `AppColors.primary(context).withOpacity(0.15)` or a new `AppColors.primarySoft(context)` |
> | text / textSec / textMute | `AppColors.textPrimary/textSecondary/textTertiary(context)` |
> | border / borderLt | `AppColors.border(context)` / `AppColors.divider(context)` |
> | white (#fff) backgrounds | `AppColors.cardBackground(context)` — never `Colors.white` |

---

## Solutions & Recommendations

### Category Filtering (Show on Map)

**Options:**

1. **Client-side filtering** (recommended for Phase 1): The search results
   already include `item_type` on each `SearchResultItem`. Filter the result
   list in `SearchState` getters, similar to how `selectedPersonId` works today.
   No proto or server changes needed.
2. **Server-side filtering**: The `SearchRequest.item_types` repeated field
   already exists but is unused by the client. Pass the active categories to the
   server to reduce payload size and improve relevance scoring.

**Recommendation**: Start with client-side filtering (option 1) since the data
is already available and `item_types` maps cleanly to the 4 UI categories. The
"Sharing" category maps to `SEARCH_ITEM_TYPE_GEAR` and "Giving" requires
distinguishing gear by transfer type — this may need a client-side check on the
gear's `sharing_type` or similar field rather than the search item type enum.

> **AUDIT — Client/API boundary**: The client must only reference protos from
> `ripls/api/`, never `ripls/models/` (per CLAUDE.md — Flutter & Protos). The
> Sharing vs. Giving distinction must be resolved using fields available on
> `api.Gear`, not storage models. Verify that `api.Gear` exposes a transfer
> type or sharing mode field before planning the split. If it doesn't, either
> add the field to the API proto or collapse to 3 categories (Gear, Requests,
> Events) matching the existing `SearchItemType` enum.

**Mapping:**

| UI Category | SearchItemType                          | Notes                      |
| ----------- | --------------------------------------- | -------------------------- |
| Requests    | `SEARCH_ITEM_TYPE_REQUEST`              | Direct match               |
| Events      | `SEARCH_ITEM_TYPE_EXPERIENCE`           | Direct match               |
| Sharing     | `SEARCH_ITEM_TYPE_GEAR` + lending check | Gear where type is lending |
| Giving      | `SEARCH_ITEM_TYPE_GEAR` + giving check  | Gear where type is giving  |

### Distance Filtering

**Options:**

1. **Client-side filtering** (recommended): Each `SearchResultItem` already has
   `distance_meters`. Filter results where `distance_meters` exceeds the
   selected threshold. Conversion: 1 mi = 1609.34 m.
2. **Server-side filtering**: Add an `optional double max_distance_meters` field
   to `SearchRequest`. The server would add a `WHERE ST_DWithin(...)` clause.
   More efficient but requires proto + server changes.

**Recommendation**: Client-side for now. The result set is small (max 50) and
the distance is already computed. Server-side can be added later if result counts
grow.

### Sort By

**Options:**

1. **Client-side sorting** (recommended): Sort the `results` list by different
   fields:
   - Relevancy: keep server order (`composite_score` descending) — this is the
     default
   - Nearest: sort by `distance_meters` ascending
   - Newest: sort by item created timestamp (available on Gear, Request,
     Experience protos)
2. **Server-side sorting**: Add a `sort_by` enum field to `SearchRequest`.

**Recommendation**: Client-side sorting. The data is already present and the
result set is bounded.

### Map Style Picker

**Approach**: Store the selected map style in `DiscoverFilters`. When the style
changes, the discover screen rebuilds the `MapWidget` with the new `styleUri`.
The `DiscoverMapHelper.onStyleLoaded()` already re-applies the faded theme — it
should conditionally skip that for non-Standard styles.

**Thumbnail rendering**: The reference shows mini map previews as thumbnails.
Options:
1. **Static asset images** (recommended): Bundle 4 small PNG thumbnails of each
   map style. Simple, no runtime cost.
2. **Gradient approximation**: Use `Container` with gradient decoration to
   approximate each style's look (as the JSX reference does). No assets needed,
   but less realistic.
3. **Live map snapshots**: Use Mapbox's snapshot API. Heavy, not worth the
   complexity.

**Recommendation**: Use gradient approximation (option 2) matching the reference
JSX. It looks good in the reference and requires no asset management.

> **AUDIT — Theming**: The gradient colors for map style thumbnails are
> presentational (they represent actual map colors, not UI theme colors), so
> hardcoded gradients are acceptable here — they depict what the map looks like,
> not app chrome. However, the selection border, label text, and surrounding
> container must still use `AppColors`.

### Filter Count Badge

The reference shows a badge on the filter icon in the floating header when
filters are active. The count logic from the JSX:
- +1 for each deselected category (out of 4)
- +1 if includeCompleted is on
- +1 if distance is not "any"
- +1 if sortBy is not "relevancy"

### Reset Button

Shows conditionally when `totalFilters > 0`. Resets all filters to defaults.

### Persisting Filter State

**Options:**
1. **In-memory only** (recommended for now): Filters reset when the app
   restarts. Simple, matches current behavior.
2. **SharedPreferences**: Persist across sessions. Add later if users request it.

> **AUDIT — Repository pattern**: If persistence is added later, filter
> preferences are user settings, not cached API data. They should NOT go
> through the Stash-based repository/cache pattern (which is for API response
> caching). SharedPreferences accessed directly from the ViewModel is acceptable
> for local-only settings that never touch the server. This is analogous to
> theme preference or locale — not repository-managed data.

---

## Open Questions

1. **Sharing vs. Giving distinction**: How does the client distinguish lending
   gear from giving gear in search results? Is there a field on `Gear` or
   `SearchResultItem` that indicates the transfer type? If not, we may need to
   add one or change the "Show on Map" categories to match the existing
   `SearchItemType` enum (Gear, Requests, Events) instead of the 4-category
   split.

2. **Map style and markers**: When switching to Satellite or Dark styles, do the
   current thumbnail markers remain readable? May need to add a white border or
   shadow to markers for dark backgrounds.

3. **"Faded" theme on non-Standard styles**: The current code applies
   `{"theme": "faded"}` to the basemap. This config property is specific to
   `MapboxStyles.STANDARD`. Other styles (Outdoors, Satellite, Dark) don't
   support import config properties the same way. The code should only apply the
   faded theme for the Standard style.

4. **Map widget rebuild vs. style swap**: Changing `styleUri` on the `MapWidget`
   may require a full widget rebuild (losing camera position) vs. calling
   `mapboxMap.loadStyleURI()` at runtime which preserves camera state. Need to
   test both approaches. `loadStyleURI()` is preferred if it works reliably.

5. **Distance "Any" default**: The reference defaults to "5 mi" selected. Should
   the app default to "Any" (current behavior — no distance filtering) or "5 mi"
   to encourage local discovery?

---

## Phased Implementation Plan

### Phase 1: Filter State & Data Model

**Goal**: Expand `DiscoverFilters` with all new filter fields and add notifier
methods. No UI changes yet.

**Files to modify:**

- `app/lib/presentation/viewmodels/search_view_model.dart`

**Tasks:**

1. Add fields to `DiscoverFilters` (Freezed class — all fields get `copyWith`
   and value equality automatically):
   - `showRequests` (bool, default true)
   - `showEvents` (bool, default true)
   - `showSharing` (bool, default true)
   - `showGiving` (bool, default true)
   - `maxDistanceMiles` (double?, default null = "Any")
   - `sortBy` (enum: relevancy, nearest, newest — default relevancy)
   - `mapStyle` (String, default `MapboxStyles.STANDARD`)
   > **AUDIT — Freezed compliance**: `sortBy` must be a plain Dart enum (not
   > Freezed union), since it's a field inside a Freezed class. Define as:
   > `enum DiscoverSortBy { relevancy, nearest, newest }`. `mapStyle` should
   > be `String` (not "enum or String") since it stores Mapbox style URIs
   > directly.
2. Add computed getters on `SearchState`:
   - `categoryFilteredResults`: apply category + distance filters to
     `_personFiltered`
   - `sortedResults`: apply sort order to `categoryFilteredResults`
   - Update `filteredResults` to use `sortedResults`
   - Update `requests`, `events`, `sharing` getters to use filtered+sorted
     results
   - Update `itemsWithLocations` to use filtered results
3. Add notifier methods on `SearchNotifier` (the ViewModel — only layer that
   owns state mutations, per architecture rules):
   - `toggleCategory(String category)` — client-side only, no network call
   - `setMaxDistance(double? miles)` — client-side only, no network call
   - `setSortBy(DiscoverSortBy sortBy)` — client-side only, no network call
   - `setMapStyle(String styleUri)` — client-side only, no network call
   - `resetFilters()` — resets to `const DiscoverFilters()` defaults
   > **AUDIT — No repository calls needed**: Category, distance, and sort
   > filters are applied client-side to already-fetched results. These methods
   > should only update `state.filters` via `copyWith()`, not trigger new
   > search RPCs. Only `toggleIncludeCompleted()` needs to re-execute the
   > search (server-side filter). Map style is purely UI state.
4. Add `activeFilterCount` getter on `DiscoverFilters`
5. Add unit tests for all new filter logic

**Tests:**

- `app/test/presentation/viewmodels/search_view_model_filter_test.dart`
  - Category filtering correctly includes/excludes item types
  - Distance filtering converts miles to meters and filters correctly
  - Sort by nearest orders by distance_meters ascending
  - Sort by newest orders by created timestamp descending
  - Reset restores all defaults
  - `activeFilterCount` returns correct count
  - `filteredResults` applies filters in correct order (person → category →
    distance → sort)
  - `itemsWithLocations` respects active category filters

> **AUDIT — Test pattern compliance**: Tests must use `ProviderContainer`
> with mock repositories (per `docs/client/architecture.md` — Testing). The
> existing `search_view_model_test.dart` pattern should be followed. Override
> `searchRepositoryProvider` with a mock that returns canned
> `SearchResultItem` protos with known `item_type`, `distance_meters`, and
> timestamps, then verify that computed getters return correctly filtered and
> sorted subsets.

---

### Phase 2: Filter Sheet UI Redesign

**Goal**: Rebuild `DiscoverFilterSheet` to match the reference design with all
sections.

**Files to modify:**

- `app/lib/presentation/widgets/discover/discover_filter_sheet.dart`

**Tasks:**

1. Rebuild the sheet structure:
   - Drag handle + header row (title, Reset button, close button)
   - Scrollable body with sections
   - Sticky "Show Results" button at bottom
2. Build section widgets (private methods within the sheet — consistent with
   architecture guidance to break down large `build()` methods):
   - `_buildMapStyleSection`: 4-column grid of gradient-approximation
     thumbnails with labels and selection border
   - `_buildCategorySection`: 2×2 grid of category chips with checkbox
   - `_buildDistanceSection`: horizontal row of 5 distance pills
   - `_buildSortBySection`: horizontal row of 3 sort pills
   - `_buildIncludeCompletedSection`: toggle row (migrated from current)
3. Style all sections with uppercase muted section headers — use
   `AppColors.textTertiary(context)` for header text, not hardcoded colors
4. Wire each control to the corresponding `SearchNotifier` method via
   `ref.read(searchProvider.notifier)` (per architecture: `ref.read()` in
   event handlers, never `ref.watch()`)
5. Show Reset button when `activeFilterCount > 0`
6. "Show Results" button dismisses the sheet (filters apply immediately via
   state)

**Tests:**

- `app/test/presentation/widgets/discover/discover_filter_sheet_test.dart`
  - Sheet renders all sections
  - Tapping a category chip calls `toggleCategory`
  - Tapping a distance pill calls `setMaxDistance`
  - Tapping a sort option calls `setSortBy`
  - Tapping a map style calls `setMapStyle`
  - Reset button calls `resetFilters`
  - Show Results button closes the sheet

---

### Phase 3: Map Style Switching

**Goal**: Connect the map style picker to the actual Mapbox map so selecting a
style changes the map behind the sheet.

**Files to modify:**

- `app/lib/presentation/screens/discover/discover_screen.dart`
- `app/lib/core/utils/discover_map_helper.dart`

**Tasks:**

1. Watch `filters.mapStyle` from `searchProvider` in the discover screen via
   `ref.watch(searchProvider.select((s) => s.value?.filters.mapStyle))` — the
   `.select()` ensures only map style changes trigger rebuilds
2. When `mapStyle` changes, call `mapboxMap.loadStyleURI(newStyleUri)` rather
   than rebuilding the widget (preserves camera + markers)
   > **AUDIT — DiscoverMapHelper is a utility, not a ViewModel**: The style
   > switch call goes through `DiscoverMapHelper` (which wraps Mapbox SDK
   > lifecycle) — this is correct. The helper is a utility class, not a
   > controller or ViewModel, consistent with the "No Controllers" rule.
3. In `DiscoverMapHelper.onStyleLoaded()`, only apply `{"theme": "faded"}` when
   the current style is `MapboxStyles.STANDARD` — pass the current style URI
   as a parameter or store it on the helper instance
4. After style loads, re-add markers (Mapbox clears annotations on style
   change) — the helper's `addItemMarkersToMap()` must be re-called from the
   screen's style-loaded callback
5. Test marker readability on all 4 styles; add white border/shadow to marker
   thumbnails if needed on dark backgrounds

**Tests:**

- Manual testing: switch styles and verify map updates, markers re-appear,
  camera position is preserved

---

### Phase 4: Filter Badge on Header

**Goal**: Show an active filter count badge on the filter icon in the floating
header.

**Files to modify:**

- `app/lib/presentation/screens/discover/discover_screen.dart`
  (`_buildFilterButton`)

**Tasks:**

1. Read `activeFilterCount` from search state
2. Replace the simple `IconButton` with a `Stack` containing the icon and a
   positioned count badge (coral circle with white number)
3. Use coral-tinted background on the icon container when filters are active
   (matching the reference's filter icon treatment)

**Tests:**

- `app/test/presentation/screens/discover/discover_screen_test.dart`
  - Badge shows correct count
  - Badge hidden when no active filters
  - Icon tint changes with active filters

---

### Phase 5: Polish & Edge Cases

**Goal**: Handle edge cases and visual polish.

**Tasks:**

1. **Empty state**: When all categories are deselected, show a message like "No
   categories selected" instead of an empty list
2. **Distance + map interaction**: When a distance filter is active, consider
   drawing a radius circle on the map to show the filtered area visually
3. **Animation**: Add `AnimatedContainer` transitions on chip/pill selection
   (matching the reference's `transition: "all 0.2s ease"`)
4. **Accessibility**: Ensure all filter controls have semantic labels
5. **Persistence** (optional): Save filter preferences to SharedPreferences —
   this is local-only user settings (not API data), so direct
   SharedPreferences access from the ViewModel is acceptable (no repository
   needed)
6. **Map style marker adaptation**: If markers look bad on dark/satellite
   styles, add a white border ring to the thumbnail annotation images

---

### Dependency Graph

```
Phase 1 (State) ──► Phase 2 (UI) ──► Phase 4 (Badge)
                         │
                         ▼
                    Phase 3 (Map Style)
                         │
                         ▼
                    Phase 5 (Polish)
```

Phases 1 and 2 are sequential (UI needs state). Phase 3 can start after Phase 2.
Phase 4 is independent once the filter count getter exists (Phase 1). Phase 5 is
final polish.
