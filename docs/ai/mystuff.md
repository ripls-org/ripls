# My Stuff: Portfolio Items Screen Redesign

## Problems

1. **Inconsistent search UX**: The portfolio items screen uses a toggle-based search mode (tap search icon → AppBar transforms into TextField → tap back to close). The discover screen keeps the search bar always visible in the FloatingHeader. This inconsistency creates a fragmented user experience.

2. **Inline filter chips feel cramped**: The `ProfileFilterBar` renders two `PopupMenuButton` dropdown chips horizontally below the header. These are small tap targets, hard to discover, and visually cluttered compared to a modal approach.

3. **No community filtering**: The current filters only support Type (Loans/Giveaways/Requests/Experiences) and Status (On Loan/Available/Not Shared). Users can't filter their items by which community they're shared with — a common need when managing items across multiple communities.

4. **No debouncing on search**: The current `setSearchQuery` updates state immediately on every keystroke, unlike the discover screen's 300ms debounce. This causes unnecessary rebuilds.

5. **Too many header modes**: The portfolio screen currently manages 3 distinct FloatingHeader configurations (default, search, selection) which adds complexity. Moving search into the always-visible content slot reduces this to 2 modes (default and selection).

## Potential Solutions

### Search Bar

**Option A — Always-visible search (recommended)**: Copy the discover screen's approach: put the TextField directly in `FloatingHeader.content`. The search bar is always visible, no toggle needed.

**Option B — Search icon expands inline**: Keep the search icon but animate expansion into the header when tapped. More complex, no clear UX benefit.

### Filter UI

**Option A — Modal bottom sheet (recommended)**: Copy `DiscoverFilterSheet` pattern — a `showModalBottomSheet` at 60% screen height with scrollable sections. Consistent with discover, more room for filter options, better discoverability.

**Option B — Expandable panel below header**: Slide-down panel with filter options. Unconventional, takes screen real estate from the item list.

### Filter Button

**Option A — Badge count on tune icon (recommended)**: Copy discover's exact approach: `Icons.tune` with an animated container background + count badge overlay when filters are active.

**Option B — Text label**: "Filters (2)" text button. Less visual, inconsistent with discover.

## Recommendations

1. Copy the discover screen's FloatingHeader layout exactly: always-visible TextField in `content`, filter button with count badge in `trailing` (before the edit button).
2. Create a new `PortfolioFilterSheet` modeled directly on `DiscoverFilterSheet`, with Type and Community sections (no Map Style, Distance, or Sort sections that are discover-specific).
3. Remove `ProfileFilterBar` widget entirely — its functionality moves into the modal.
4. Add 300ms debounce to search in `ProfileGearNotifier`.
5. Keep the edit button in `trailing`, placed after the filter button.

## Open Questions

1. **Status filter**: The current filter bar has a Status filter (On Loan / Available / Not Shared). Should this move into the new filter modal as a section, or be dropped? **Recommendation**: Include it in the modal as a third section — it's useful and there's room.

2. **Sort options**: The discover filter modal has a Sort By section. Should the portfolio filter modal also offer sorting (e.g., Alphabetical, Newest, Recently Active)? **Recommendation**: Skip for now; items are already sorted alphabetically. Can add later if needed.

3. **"Include completed" toggle**: The discover modal has this for completed transfers. Should the portfolio modal have a similar toggle to show/hide completed giveaways or returned loans? **Recommendation**: Skip for now. Portfolio items show the user's own items, not transfers.

4. **Community list source**: Where do we get the user's communities for the filter? The `ProfileGearState` already has `communitiesById` which contains all communities the user's items are shared with. This should be sufficient. Should "Not shared" be a filter option alongside communities? **Recommendation**: Yes, include "Not shared with any community" as a special option.

5. **Header layout in selection mode**: When edit/selection mode is active, the header currently shows "N selected" + action buttons (location, share, delete). Should the search bar remain visible during selection mode? **Recommendation**: No — keep the current selection mode header as-is. Search + selection simultaneously adds complexity without clear benefit.

## Audit Notes

> **Audited against**: `docs/client/architecture.md`, `docs/client/caching.md`,
> `docs/client/i18n.md`, `docs/server/architecture.md`

**Issues found and fixed:**

1. **[i18n] Separate i18n phase violates co-location rule** — `docs/client/i18n.md`
   requires strings to be added in ARB files alongside the widgets that use them,
   not deferred to a later phase. Merged i18n work into Phases 1 and 2 where the
   UI is built; deleted standalone Phase 5.

2. **[architecture] Debounce Timer needs cleanup** — `docs/client/architecture.md`
   states regular Notifiers (non-AsyncNotifier) must guard async gaps with
   `ref.mounted` checks. A `Timer` in a `Notifier` also needs cancellation via
   `ref.onDispose`. Added explicit `ref.onDispose(() => _debounce?.cancel())`
   requirement to Phase 1.

3. **[architecture] Freezed `Set` fields need `@Default({})`** — Freezed handles
   `Set` equality correctly, but `Set<ProfileItemType>` needs an explicit
   `@Default({})` annotation and `build_runner` regeneration. Added
   `flutter pub run build_runner build` step to Phase 2.

4. **[architecture] Filter sheet reads ViewModel directly** — Confirmed compliant.
   `DiscoverFilterSheet` (the reference) is a `ConsumerWidget` that reads from
   `searchProvider` (a ViewModel-layer provider). `PortfolioFilterSheet` reading
   from `profileGearViewModelProvider` follows the same pattern: Widgets → ViewModels
   → Repositories. No layer violation.

5. **[caching] No caching changes needed** — Confirmed compliant. All filtering
   is client-side against already-cached data in `ProfileGearState`. Community
   data comes from `communitiesById` which is already populated by `loadGear()`.
   No new repository or cache key changes required.

6. **[architecture] Verify `ProfileFilterBar` is not shared** — Before deleting
   `profile_filter_bar.dart`, confirm no other screen imports it. Added
   verification step to Phase 3.

7. **[server] No server changes** — Confirmed compliant. This is a pure
   client-side UI refactor. No proto, RPC, or storage changes needed.

## Implementation Plan

### Phase 1: Add search bar and filter button to FloatingHeader

**Goal**: Replace the toggle-based search with an always-visible search bar, add a filter button with badge, and keep the edit button.

**Files modified**:
- `app/lib/presentation/screens/portfolio/portfolio_screen.dart` — Rewrite `_buildItemsHeader()` to use a single FloatingHeader with:
  - `content`: TextField (always visible, same styling as discover)
  - `trailing`: Row containing filter button + edit button
  - Remove `_isSearching` state and the separate search-mode header
  - Move `_searchController` management here (clear on dispose)
- `app/lib/presentation/viewmodels/profile_gear_view_model.dart` — Add 300ms debounce to `setSearchQuery()` using a `Timer`. Cancel timer via `ref.onDispose(() => _debounce?.cancel())` in `build()`. Remove `isSearching` field from `ProfileGearState` (no longer needed since search is always visible). Remove `startSearch()` / `stopSearch()` methods.
- `app/lib/l10n/app_en.arb` — Add `portfolioSearchHint`, `portfolioFiltersAccessibility`, `portfolioFiltersLabel`
- `app/lib/l10n/app_es.arb` — Spanish translations for the above keys

**Files deleted**:
- None yet

**Estimated scope**: ~100 lines changed

### Phase 2: Create PortfolioFilterSheet

**Goal**: Build the filter modal, modeled on `DiscoverFilterSheet`.

**New files**:
- `app/lib/presentation/widgets/portfolio/portfolio_filter_sheet.dart` — `ConsumerWidget` with:
  - `static show(BuildContext context)` method (same pattern as `DiscoverFilterSheet.show`)
  - Handle + header row with title "Filters", reset button, close button
  - **Type section**: 2×2 grid of category chips (Loans, Giveaways, Requests, Experiences) — same visual style as discover's category chips, but toggle individually with checkboxes
  - **Community section**: Vertical list of community chips built from `communitiesById` in state, plus a "Not Shared" option. Each toggleable.
  - **Status section** (if including per Open Question 1): Row of pills (On Loan, Available, Not Shared) — same pill widget style as discover's distance/sort pills
  - All user-facing strings use `context.l10n.*` keys (added in this phase)

**Files modified**:
- `app/lib/presentation/viewmodels/profile_gear_view_model.dart` — Expand `ProfileFilters` to include:
  - `@Default({}) Set<ProfileItemType> selectedTypes` (replaces single `selectedType`, allows multi-select)
  - `@Default({}) Set<String> selectedCommunityIds` (empty = show all)
  - `@Default(false) bool showNotShared` (show items not shared with any community)
  - `int get activeFilterCount` computed property
  - Add methods: `toggleTypeFilter(type)`, `toggleCommunityFilter(id)`, `toggleShowNotShared()`, `resetFilters()`
  - Update `filteredGearItems`, `filteredRequestItems`, `filteredExperienceItems` getters to apply community filtering
- `app/lib/l10n/app_en.arb` — Add `portfolioFilterTitle`, `portfolioFilterType`, `portfolioFilterCommunity`, `portfolioFilterStatus`, `portfolioFilterNotShared`, `portfolioFilterReset`
- `app/lib/l10n/app_es.arb` — Spanish translations for the above keys

**Post-step**: Run `flutter pub run build_runner build` to regenerate `.freezed.dart` files after `ProfileFilters` changes, then `npm run generate` which also runs `flutter gen-l10n` for ARB changes.

**Estimated scope**: ~300 lines new, ~80 lines changed

### Phase 3: Remove ProfileFilterBar and wire up filter button

**Goal**: Remove the old inline filter chips and connect the filter button to the new modal.

**Pre-step**: Grep for `profile_filter_bar` imports across the codebase to confirm only `portfolio_items_screen.dart` uses it. If other screens import it, refactor those first.

**Files modified**:
- `app/lib/presentation/screens/portfolio/portfolio_items_screen.dart` — Remove the `ProfileFilterBar()` widget from the build tree. Remove any padding/spacing that was around it.
- `app/lib/presentation/screens/portfolio/portfolio_screen.dart` — Wire filter button `onTap` to `PortfolioFilterSheet.show(context)`. Compute `activeFilterCount` from state and pass to badge.

**Files deleted**:
- `app/lib/presentation/widgets/profile_filter_bar.dart` — No longer needed; all filter UI is in the modal.

**Estimated scope**: ~30 lines removed, ~10 lines changed

### Phase 4: Tests

**Goal**: Test the new filter sheet and updated header behavior.

**New files**:
- `app/test/presentation/widgets/portfolio/portfolio_filter_sheet_test.dart` — Tests for:
  - Sheet opens via `PortfolioFilterSheet.show()`
  - Type chips toggle correctly
  - Community chips render from state and toggle
  - Reset button clears all filters
  - Active filter count badge updates
  - Close button dismisses sheet
  - All test `MaterialApp` wrappers include localization delegates (per `app/test/helpers/l10n_helpers.dart`)
- `app/test/presentation/screens/portfolio/portfolio_items_screen_test.dart` — Update existing tests (if any) to account for removed filter bar and always-visible search

**Files modified**:
- Update any existing portfolio screen tests that reference `ProfileFilterBar`, search mode toggling, or the old header layout

**Estimated scope**: ~200 lines new tests
