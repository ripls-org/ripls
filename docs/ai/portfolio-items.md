# Portfolio Items Redesign

## Problems

1. **Sort order is alphabetical, not chronological.** The three `filtered*Items` getters in `profile_gear_view_model.dart` sort by name (`a.name.toLowerCase().compareTo(…)`). Users expect to see their newest items first — the current alphabetical sort buries recently added items and makes the list feel static.

2. **No quick filter chips.** Filtering requires opening the `PortfolioFilterSheet` bottom sheet, toggling checkboxes, and closing it. This is too many taps for the most common filters (status: Available / On Loan / Pending). The reference design shows a horizontal row of pills with counts ("All 8", "Available 5", "On Loan 1", "Pending 2") that let users filter in one tap.

3. **Item cards are text-only.** `ProfileGearCard`, `ProfileRequestCard`, and `ProfileExperienceCard` are flat text rows with a divider. They lack visual weight and look nothing like the rich, photo-backed cards on the portfolio inbox screen (`InboxActionCard`). The reference design shows 130px-tall cards with a full-bleed background image, gradient scrim, white title text, location pin, community badges, and a compound status badge.

## Reference Design

See [portfolio-items.jsx](../../tmp/portfolio2/portfolio-items.jsx) and [portfolio-items.png](../../tmp/portfolio2/portfolio-items.png).

Key visual elements per card (130px height):
- **Full-bleed background image** from the item's first media ID, with `BoxFit.cover`
- **Fallback** when no image: type-colored gradient (same pattern as `InboxActionCard._buildFallbackBackground`)
- **Gradient scrim**: bottom-heavy (`rgba(0,0,0,0.72)` at bottom → `rgba(0,0,0,0.08)` at top)
- **Top-right**: compound status badge — status pill + borrower/due-date segment when applicable
- **Top-left**: selection checkbox (only in selection mode)
- **Bottom-left**: item name (17px serif, white, shadow) + location row (pin icon + truncated address)
- **Bottom-right**: community initials badges (frosted `rgba(255,255,255,0.75)` background)

## Potential Solutions

### Sort Order

| Option | Pros | Cons |
|--------|------|------|
| A. Sort by `created_at_unix_sec` descending | Intuitive, standard UX | `GearItem` proto currently lacks `created_at_unix_sec` |
| B. Sort by media ID / item ID (ULID/UUID v7 embeds timestamp) | No proto change needed if IDs are time-ordered | Fragile — depends on ID scheme; items without media wouldn't sort correctly |
| C. Add a `created_at_unix_sec` field to `GearItem` | Clean, future-proof | Requires proto change + server backfill |

**Recommendation**: Option C — add `created_at_unix_sec` to `GearItem` (field 10). `Request` and `Experience` already have this field. This is the only approach that is reliable across all item types and doesn't depend on implementation details. The server already stores `created_at` on the gear model, so this is just plumbing it through to the API message.

### Filter Chips

| Option | Pros | Cons |
|--------|------|------|
| A. Horizontal `FilterChip` row above the list | Matches reference, one-tap, shows counts | Needs a "quick filter" concept separate from the full filter sheet |
| B. Replace the full filter sheet with chips only | Simple | Loses community and type filtering |
| C. Chips for status + keep full filter sheet for advanced filters | Best of both worlds | Two filter mechanisms to maintain |

**Recommendation**: Option C — add a horizontal chip bar for the most common status filters (All, Available, On Loan, Pending) and keep the existing filter sheet icon for advanced filtering (type, community). The chip bar is a view-layer change that drives the same `ProfileFilters` state. Counts are computed from the unfiltered item list.

### Visual Item Cards

| Option | Pros | Cons |
|--------|------|------|
| A. New unified `PortfolioItemCard` widget | Single widget for all 3 item types; matches reference exactly | Replaces 3 existing card widgets in items screen context |
| B. Add image background to each existing card widget | Less code churn | Three widgets to maintain in parallel; inconsistent if one drifts |
| C. Reuse `InboxActionCard` | Already has background image logic | Wrong shape — inbox card has action bar, people chips, conversation-specific layout |

**Recommendation**: Option A — create a new `PortfolioItemCard` widget. A single widget is simpler than maintaining three, and the layout (background image + scrim + title + status badge + location + community chips) is uniform across gear, requests, and experiences. The existing per-type card widgets remain available for other screens (e.g., profile view) but are no longer used on the portfolio items tab.

## Open Questions (Resolved)

1. **GearItem `created_at_unix_sec`**: Confirm that the server-side `gear` model
   stores a creation timestamp that can be exposed on `GearItem`. If not, we'll
   need a migration or fallback to ID-based ordering.

2. ~~**Chip filter scope**~~ **RESOLVED:** Chips filter across all item types in
   a single unified list. No grouping by type. Default sort is reverse creation
   date; when a search query is active, sort by relevance instead.

3. ~~**Selection mode interaction with chips**~~ **RESOLVED:** Selection is only
   for visible items. When a chip filter hides items, their selection IDs are
   cleared.

4. ~~**Existing card widgets**~~ **RESOLVED:** `ProfileGearCard`,
   `ProfileRequestCard`, and `ProfileExperienceCard` are only imported by
   `portfolio_items_screen.dart`. They (and their tests:
   `profile_request_card_test.dart`, `profile_experience_card_test.dart`) should
   be deleted in Phase 4. If anything breaks, the build will catch it.

5. ~~**Request and Experience status mapping**~~ **RESOLVED:** Reuse the existing
   `UnifiedItemData` model and its converter extensions in
   `app/lib/presentation/models/unified_item_converters.dart`. These already
   compute `statusText` and `statusSeverity` for each item type:
   - **Gear**: "On Loan to @name", "Available for Loan", "Available for Giveaway"
   - **Requests**: "Open", "Fulfilled by @name", "N offers"
   - **Experiences**: "No RSVPs yet", "N attending"

   A new `GearItemToUnifiedItem` extension (for `GearItem` + optional
   `Transfer`) will be added alongside the existing `CommunityGearItemToUnifiedItem`
   extension. The `PortfolioItemCard` widget maps `StatusSeverity` to themed
   colors (success → sage, warning → coral, info → primary, neutral → tertiary).

## Architecture Compliance Notes

> **AUDIT (2026-03-28):** The following constraints were added after reviewing
> `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`,
> and `docs/server/architecture.md`.

1. **i18n — all chip labels must come from ARB files.** The filter chip labels
   ("All", "Available", "On Loan", "Pending") and any new user-visible strings
   (empty-state text, count formatting) must be added to `app_en.arb` +
   `app_es.arb` and accessed via `context.l10n.*`. ViewModels must not resolve
   display strings — expose enum/state and let the widget resolve text in
   `build()`. (ref: `docs/client/i18n.md`)

2. **Theming — no hardcoded colors.** The JSX reference uses hex values
   (`#D4714E`, `#6B8F71`, etc.) for illustration. The Flutter implementation
   must use `AppColors.*` helpers for all background, text, border, and badge
   colors so dark mode works. Scrim gradients over images (black with varying
   alpha) are acceptable as hardcoded since they overlay photos, not themed
   surfaces. (ref: `docs/client/architecture.md` — Colors and Theming)

3. **Image cache keys — use `thumbnail` for 130px cards.** Per caching docs,
   any display context < 200px must use `ImageCacheKeys.thumbnail(mediaId)`.
   The 130px card height falls in this range. Use `mediaObjectProvider` to
   resolve the URL (same pattern as `InboxActionCard`). (ref:
   `docs/client/caching.md` — Image Cache Keys)

4. **Layer discipline — card widget may use `mediaObjectProvider`.** The
   `PortfolioItemCard` will be a `ConsumerWidget` that calls
   `ref.watch(mediaObjectProvider(mediaId))` for background image resolution.
   This follows the established `InboxActionCard` pattern where the widget
   directly watches a media provider. The widget must NOT read repositories
   or services directly. (ref: `docs/client/architecture.md` — Key
   Architectural Rules)

5. **Tests accompany each phase, not a separate final phase.** Architecture
   docs require test coverage alongside new functionality. Each phase below
   includes its own test section. (ref: `docs/server/architecture.md` §4;
   `docs/client/architecture.md` — Testing)

6. **Unified list reuses `UnifiedItemData`.** The existing model at
   `app/lib/presentation/models/unified_item_data.dart` (Freezed sealed class)
   already carries `type`, `title`, `mediaId`, `statusText`,
   `statusSeverity`, `locationName`, `dateTime`, and selection state. Converter
   extensions in `unified_item_converters.dart` handle `CommunityGearItem`,
   `Request`, `Experience`, and `Transfer`. A new `GearItemToUnifiedItem`
   extension will be added for `GearItem` + optional `Transfer`. The view model
   getter merges all items into `List<UnifiedItemData>` sorted by
   `createdAtUnixSec` descending. No new sealed class is needed.

7. **Server: no service-to-service imports.** The `created_at_unix_sec` field
   is populated in the gear service's own assembly logic from the stored model.
   No cross-service calls needed. (ref: `docs/server/architecture.md` §1)

## Implementation Plan

### Phase 1: Proto & Sort — add `created_at_unix_sec` to `GearItem`

**Files:**
- `proto/ripls/api/gear_service.proto` — add `int64 created_at_unix_sec = 10` to `GearItem`
- `server/services/gear/` — populate the new field from the stored model's creation timestamp in `ListUserGear` and any other RPC that returns `GearItem`
- `npm run generate` — regenerate Go + Dart code

**Unified item converter:**
- `app/lib/presentation/models/unified_item_converters.dart` — add a new
  `GearItemToUnifiedItem` extension on `GearItem` that accepts an optional
  `Transfer` and converts to `UnifiedItemData`. Follow the existing
  `CommunityGearItemToUnifiedItem` pattern for `statusText` / `statusSeverity`.
  Populate `dateTime` from the new `created_at_unix_sec` field.

**View model change:**
- `app/lib/presentation/viewmodels/profile_gear_view_model.dart` — add a
  `filteredItems` getter on `ProfileGearState` that:
  1. Converts each gear/request/experience to `UnifiedItemData` using the
     converter extensions (passing `transfersByGearId` for gear items)
  2. Applies the existing search, type, status, and community filters
  3. Sorts by `dateTime` descending (newest first) for the default case; when
     `searchQuery` is non-empty, sort by relevance (name-match priority) instead
  4. Returns a single `List<UnifiedItemData>`

  The three separate `filteredGearItems` / `filteredRequestItems` /
  `filteredExperienceItems` getters are kept for backward compatibility with
  chip counts and selection logic but are no longer used for rendering.

**Tests:**
- Server unit test: verify `ListUserGear` response includes `created_at_unix_sec` > 0 for each item
- ViewModel unit test: items sorted by `created_at_unix_sec` descending; gear/request/experience interleaved correctly

**Verification:** existing tests pass; items screen shows newest items at top.

### Phase 2: Filter Chip Bar

**i18n (add to both `app_en.arb` and `app_es.arb`):**
- `portfolioItemsFilterAll` — "All"
- `portfolioItemsFilterAvailable` — "Available"
- `portfolioItemsFilterOnLoan` — "On Loan"
- `portfolioItemsFilterPending` — "Pending"

**New widget:**
- `app/lib/presentation/widgets/portfolio/portfolio_status_chips.dart` — horizontal scrollable row of `ChoiceChip`-style pills using `context.l10n.*` for labels, each with a count badge. Chip background/text colors must use `AppColors.*` (e.g. `AppColors.textPrimary(context)` for active chip, `AppColors.surface(context)` for inactive). Selecting a chip updates `ProfileFilters` status flags via the view model.

**Modified files:**
- `app/lib/presentation/screens/portfolio/portfolio_items_screen.dart` — insert chip bar between header and item list; wire chip selection to `profileGearViewModelProvider.notifier.updateFilters()`
- `app/lib/presentation/viewmodels/profile_gear_view_model.dart` — add a `statusChipCounts` getter that computes {All: N, Available: N, OnLoan: N, Pending: N} from the unfiltered + type-filtered item lists. This is a computed getter on `ProfileGearState`, not a new async call.

**Tests:**
- `app/test/presentation/widgets/portfolio/portfolio_status_chips_test.dart` — renders all chips with localized labels, shows correct counts, tap changes filter, active chip styling. Widget tests must include `localizationsDelegates` and `supportedLocales` on their `MaterialApp` wrapper (see `app/test/helpers/l10n_helpers.dart`).

**Verification:** tapping a chip filters the list instantly; counts update when items load.

### Phase 3: Visual Item Card + Unified List

**New widget:**
- `app/lib/presentation/widgets/portfolio/portfolio_item_card.dart` — `ConsumerWidget`, ~130px tall card with:
  - Full-bleed `CachedMediaImage` background via `ref.watch(mediaObjectProvider(mediaId))` using `ImageCacheKeys.thumbnail(mediaId)` cache key, or type-colored gradient fallback via `AppColors` semantic colors (not hardcoded hex)
  - Bottom-heavy gradient scrim (black with varying alpha — acceptable as image overlay)
  - Top-right: compound status badge (status label + optional borrower/due date). Badge background/foreground colors via `AppColors.sage(context)` / `AppColors.primary(context)` etc.
  - Top-left: selection checkbox (when in selection mode)
  - Bottom-left: item name (serif, white, text shadow) + location row (pin + address, white with alpha)
  - Bottom-right: community initials badges (frosted `Colors.white.withValues(alpha: 0.75)` — acceptable for image overlay context)
  - `onTap` callback; `isSelected` / `isSelectionMode` flags
  - Accepts a `UnifiedItemData` instance and a `List<GetCommunityResponse>` for
    community chips. Status badge derives color from `statusSeverity` and label
    from `statusText`. No raw field maps.

**Modified files:**
- `app/lib/presentation/screens/portfolio/portfolio_items_screen.dart` — replace `_buildGearCard`, `_buildRequestCard`, `_buildExperienceCard` with a single builder that iterates the unified `filteredItems` list from the view model and maps each `UnifiedItemData` to a `PortfolioItemCard`. Remove the three separate list sections.
- Ensure pull-to-refresh, empty state, error state, and selection mode all work with the unified list.
- Update the item count label to reflect the unified count.
- Selection mode: when a filter chip hides items, clear those items' selection IDs.

**Tests:**
- `app/test/presentation/widgets/portfolio/portfolio_item_card_test.dart` — renders with image (mock `mediaObjectProvider`), renders fallback gradient, shows status badge variants, compound badge with borrower + due date, selection mode checkbox, community chips, location label. Must include localization delegates.

**Verification:** cards render with images; fallback gradient appears for items without media; mixed item types interleaved by date; bulk operations still work.

### Phase 4: Polish & Cleanup

**Changes:**
- Verify all new strings pass `npm run lint:dart:i18n`
- Verify dark mode renders correctly (no invisible text, no broken badges)
- Delete the old card widgets (only used by `portfolio_items_screen.dart`):
  - `app/lib/presentation/widgets/profile_gear_card.dart`
  - `app/lib/presentation/widgets/profile_request_card.dart`
  - `app/lib/presentation/widgets/profile_experience_card.dart`
  - `app/test/presentation/widgets/profile_request_card_test.dart`
  - `app/test/presentation/widgets/profile_experience_card_test.dart`
- Run `npm run build` to verify server lint + compile
- Run `flutter test --reporter expanded` to verify nothing else imported the
  deleted widgets. If anything breaks, restore the widget and keep it.

**Verification:** CI-clean build; dark mode visual check; no lint warnings.
