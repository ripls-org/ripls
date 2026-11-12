# Item Metrics Screen Redesign

## Overview

Redesign the `ItemMetricsScreen` to match the updated reference design. This is primarily a **layout restructuring** — moving impact metrics into the dark hero header, replacing the current metric cards with an explanatory "What Sharing This Means" card, adding a Product Details section, and updating the loan history to use a timeline layout with avatar images.

**Reference materials:**
- [item-metrics.jsx](../../tmp/metrics2/item-metrics.jsx) — React reference implementation
- [item-metrics1.png](../../tmp/metrics2/item-metrics1.png) — Top-half screenshot (header + explanatory card + product details)
- [item-metrics2.png](../../tmp/metrics2/item-metrics2.png) — Bottom-half screenshot (product details + timeline loan history)

## Problems with Current Layout

1. **Cumulative impact bar is a separate dark section** between the hero and the light card area. In the new design, the three headline metrics ($325 SAVED, 2h RECOVERED, 42kg CO₂ AVOIDED) should be **always visible in the hero header** — not conditional on having transactions.

2. **Metric cards (ImpactMetricCard) are the primary content below the hero**. In the new design, the primary content below the hero is a warm "What Sharing This Means" explanatory card that frames the per-action impact. The metric cards are replaced by compact metric tiles inside this card.

3. **No Product Details section exists**. The new design has a structured grid showing Category, Brand, Model, Materials, Weight, CO₂ Footprint, and Estimated Value — all derived from the existing `metadataMetrics` and impact data.

4. **Loan history uses flat cards**. The new design uses a **timeline layout** with dot indicators and a vertical track line, plus `UserAvatar` showing actual profile images.

5. **Per-action metrics in the "What Sharing This Means" card should still be tappable** to open the audit trail screen (preserving existing behavior).

## What Stays the Same

- **Background image in the hero header** — keep `backgroundImageUrl` and gradient overlay
- **Expandable description** — keep the tap-to-expand/collapse behavior on the description text
- **Tapping a metric opens the audit trail screen** — the per-action metric tiles in the explanatory card navigate to `AuditTrailScreen` just like the current `ImpactMetricCard` does
- **Tapping a loan/history item opens the item screen** — loan entries navigate to the transfer detail
- **AppBar with back button and item name** — keep the existing transparent AppBar
- **Sealed-class pattern for gear/request/experience** — keep `ItemMetricDataBase` and `ItemType`
- **All existing data providers** — `gearMetricProvider`, `requestMetricProvider`, `experienceMetricProvider` remain unchanged
- **Empty state** when no transactions exist — keep but adjust positioning

## Proposed Solution

### A. Hero Header Changes

**Current**: Hero section (via `ImpactHeroSection`) shows subtitle (owner name), description, and metadata stat chips. Separately, a `_buildCumulativeImpactBar` shows cumulative totals (conditional on having transactions).

**New**: The hero header always shows the three headline metrics (SAVED, RECOVERED, CO₂ AVOIDED) in a centered row at the bottom of the hero, separated from the description by a subtle horizontal rule. Each metric is **tappable** to open the audit trail screen. Remove the separate cumulative impact bar. Remove the metadata stat chips from the hero.

**Implementation**: Add a new optional parameter to `ImpactHeroSection` (e.g., `heroMetrics`) to display a row of centered metric values. Alternatively, use the existing `metricSelector` slot, but a dedicated parameter is cleaner since this is different from the community metric selector. A third option is to compose the hero metrics directly in `ItemMetricsScreen._buildContent` instead of modifying the shared widget.

**Recommendation**: Add an optional `bottomContent` widget parameter to `ImpactHeroSection`. This keeps the shared widget flexible without coupling it to the specific item metrics layout. The item screen passes a custom hero stats row widget.

### B. "What Sharing This Means" Explanatory Card

**Current**: Below the hero, the screen shows framing text and three `ImpactMetricCard` widgets (large tappable cards with icon, amount, label, confidence dot, chevron).

**New**: A warm-toned card (`#FFF3E6` background, `#F0DCC8` border, 14px border radius) containing:
1. Italic framing text: "Each time a friend borrows this instead of buying one, your circle saves:"
2. A row of three compact metric tiles — each a white rounded box with:
   - Large colored value (coral for money/time, sage for CO₂)
   - Small muted label below
   - Tappable → opens audit trail screen

**Implementation**: Create a new widget `SharingImpactCard` that accepts the three savings values and onTap callbacks. The per-action metrics use the existing `SavingsMetricDetail` data.

### C. Product Details Section

**Current**: Does not exist. Metadata is shown as stat chips in the hero.

**New**: A structured grid section with "PRODUCT DETAILS" header. White card with rounded corners and dividers between rows. Each row shows a label on the left (muted text) and a value on the right (bold text). Rows include:
- Category, Brand, Model, Materials, Weight — from `metadataMetrics`
- CO₂ Footprint — from impact data (`CarbonEstimate`)
- Estimated Value — from impact data (`ValueEstimate`)

**Implementation**: Create a new widget `ProductDetailsCard` that accepts a list of label-value pairs. The item screen maps `metadataMetrics` and impact-derived values into this list.

**Tapping rows**: All rows with values are tappable and open the audit trail screen — including Category, Brand, and Model. The `_navigateToMetricDetail` method already provides a fallback audit trail message ("AI-detected from visual analysis.") when no specific reasoning is available. Only rows showing "—" (unavailable data) are non-tappable.

### D. Loan History Timeline Layout

**Current**: `LoanHistoryCard` widgets in a flat column. Each card shows avatar, borrower name, date range, status badge, and impact metrics.

**New**: Timeline layout with:
1. A vertical track line on the left
2. Dot indicators (coral for most recent, muted for older entries)
3. Cards to the right of the timeline showing:
   - `UserAvatar` (actual profile images, not just initials)
   - Borrower name (bold)
   - Date range (muted)
   - Value saved amount (coral, right-aligned) — only money saved, not all three metrics

**Implementation options**:
1. **Modify `LoanHistoryCard`** to accept a `timelinePosition` parameter and render the timeline track
2. **Create a new `TimelineLoanEntry` widget** and a parent `LoanHistoryTimeline` widget
3. **Build the timeline directly in `ItemMetricsScreen`**

**Recommendation**: Option 2 — Create `LoanHistoryTimeline` (parent) and simplify each entry. The parent manages the vertical track; each entry is a card. This is cleaner than modifying the existing `LoanHistoryCard` which may be used elsewhere. The existing `LoanHistoryCard` can be deprecated or kept for backward compat.

**Avatar**: The reference JSX uses a gradient-circle initials fallback (`Av` component). The actual Flutter implementation should use the existing `UserAvatar` widget which already handles profile images via `CachedNetworkImage` with initials fallback.

### E. Tapping Loan/History Items

**Current behavior**: Loan history cards don't navigate anywhere.

**New behavior**: Tapping a loan entry should navigate to the transfer detail screen. The `Transfer` proto already has the transfer ID needed for navigation.

**Implementation**: Accept an `onTap` callback on each timeline entry. The `ItemMetricsScreen` provides a callback that navigates to the transfer screen using the transfer ID from the loan data.

## Design Decisions (Resolved)

1. **Pre-transaction hero metrics**: Show zero when there are no completed transactions (zero actions = zero cumulative impact). The hero shows server-provided cumulative totals (`stats.impact`), not per-action estimates. Per-action estimates only appear in the "What Sharing This Means" explanatory card.

2. **Metric tappability in hero**: All metrics are tappable — both the hero stats row and the explanatory card metrics open the audit trail screen.

3. **Product Details for requests/experiences**: Show for gear only; for requests/experiences, skip the Product Details section entirely.

4. **Transfer navigation**: Navigate to the transfer detail/loan detail screen if it exists; otherwise wire up later.

5. **Giveaway/Request/Experience history entries**: Use the same timeline layout pattern for all three item types, adapting the entry content (e.g., "Helper" instead of "Borrower", different value labels).

## Architecture Compliance Notes

Audit against [client/architecture.md](../client/architecture.md), [client/caching.md](../client/caching.md), and [server/architecture.md](../server/architecture.md).

### Compliant

- **MVVM layer boundaries preserved**: No new repository or service calls. All data flows through existing ViewModels (`gearMetricProvider`, `requestMetricProvider`, `experienceMetricProvider`). Screens access data via `ref.watch()` only.
- **No manual caches in ViewModels**: All new widgets are stateless presentation; no `Map<String, T>` caches or direct service calls.
- **Reusable widget extraction**: Each major UI section becomes its own widget file (`SharingImpactCard`, `ProductDetailsCard`, `LoanHistoryTimeline`), keeping `build()` methods small per CLAUDE.md guidelines.
- **`UserAvatar` for profile images**: Timeline entries use the existing `UserAvatar` widget which handles `CachedNetworkImage` + `ImageCacheKeys` + initials fallback correctly.
- **File size discipline**: The current `item_metrics_screen.dart` is ~593 lines. Extracting three widgets into separate files will keep all files well under the 1000-line maximum.
- **No server changes required**: This is a pure client-side layout restructuring.
- **`AppBarBackButton` usage**: The existing AppBar uses `AppBarBackButton(color: Colors.white)` — preserved as-is.

### Issues Found and Remediation

1. **`setState` for description expand/collapse (OK)**: `ImpactHeroSection` uses `setState` for `_isDescriptionExpanded`. This is correct per architecture.md — `setState` is allowed for local UI state like toggle/focus, not app state.

2. **`ImpactHeroSection` uses `NetworkImage` directly** (existing issue, out of scope): The hero background image uses `NetworkImage(widget.backgroundImageUrl!)` instead of `CachedNetworkImage` with `ImageCacheKeys.full()`. This violates the CLAUDE.md rule "Always use CachedMediaImage for user-uploaded media." However, this is a pre-existing issue in the shared `ImpactHeroSection` widget used by all three metric screens — fixing it is outside the scope of this layout redesign. Flagged for a separate follow-up.

3. **New widgets should be `StatelessWidget` or `ConsumerWidget`**: `SharingImpactCard` and `ProductDetailsCard` are pure presentation widgets with no state — use `StatelessWidget`. `LoanHistoryTimeline` needs `UserAvatar` which is a `ConsumerWidget` (it calls `ref.watch(mediaUrlProvider)`), so the timeline widget must be a `ConsumerWidget` or contain `Consumer` builders for each avatar.

4. **Testing pattern**: Widget tests for new components should use `ProviderContainer` with mock overrides for `mediaUrlProvider` (needed by `UserAvatar`). Follow the patterns in [client/architecture.md](../client/architecture.md) — mock repositories, test with `ProviderScope(overrides: [...])`.

5. **`--reporter expanded`**: All Flutter test runs must use `--reporter expanded` per CLAUDE.md to avoid the carriage-return output issue.

## Phased Implementation Plan

### Phase 1: Hero Stats Row

**Files changed:**
- [impact_hero_section.dart](../../app/lib/presentation/widgets/impact/impact_hero_section.dart) — Add optional `bottomContent` widget parameter
- [item_metrics_screen.dart](../../app/lib/presentation/screens/item/item_metrics_screen.dart) — Build hero stats row, pass as `bottomContent`

**Changes:**
1. Add `Widget? bottomContent` parameter to `ImpactHeroSection`
2. Render `bottomContent` at the bottom of the hero, below description, above any chips — separated by a subtle white border-top divider
3. In `ItemMetricsScreen._buildContent`, build a `_buildHeroStatsRow` widget showing three centered columns: value, label. Each column is wrapped in `GestureDetector` to open the audit trail screen on tap (same `_showAuditTrail` logic used by the current metric cards).
4. Remove the separate `_buildCumulativeImpactBar` method and its SliverToBoxAdapter
5. Remove metadata stat chips from the hero (they move to Product Details in Phase 3)

**Metrics source**: Use the **server-provided cumulative impact** from `stats.impact` (which is already per-action × transaction count, calculated server-side). When no transactions exist, show zero — do NOT fall back to `potentialImpact`. The per-action estimates belong in the "What Sharing This Means" card only. The ViewModel already exposes `stats.impact` via `GetGearStatsResponse`; format with `SavingsFormatter`. Do not calculate cumulative values on the client — the server owns this computation.

**Test**: Verify the hero shows the three metrics, they are tappable (opening audit trail), and the cumulative bar no longer appears. Ensure the other screens using `ImpactHeroSection` (community, user) still render correctly with `bottomContent: null`.

### Phase 2: "What Sharing This Means" Explanatory Card

**Files changed:**
- **New file**: [sharing_impact_card.dart](../../app/lib/presentation/widgets/impact/sharing_impact_card.dart)
- [item_metrics_screen.dart](../../app/lib/presentation/screens/item/item_metrics_screen.dart) — Replace `ImpactMetricCard` usage with `SharingImpactCard`

**Changes:**
1. Create `SharingImpactCard` as a `StatelessWidget` (pure presentation, no providers):
   - Warm background (`#FFF3E6`), border (`#F0DCC8`), rounded corners (14px)
   - Italic serif framing text
   - Row of three compact metric tiles (white background, rounded, centered text)
   - Each tile: large colored value + small label, tappable via `onTap` callback
2. In `ItemMetricsScreen._buildContent`:
   - Replace the three `ImpactMetricCard` widgets with a single `SharingImpactCard`
   - Wire up `onTap` for each metric tile to call `_showAuditTrail` (preserving existing behavior)
   - Update framing text to match the mock: "Each time a friend borrows this instead of buying one, your circle saves:"

**Test**: Create widget tests for `SharingImpactCard` in `app/test/presentation/widgets/impact/sharing_impact_card_test.dart`. Verify: layout renders three metric tiles, framing text displays, each tile fires its `onTap` callback. Use `flutter test --reporter expanded`.

### Phase 3: Product Details Section

**Files changed:**
- **New file**: [product_details_card.dart](../../app/lib/presentation/widgets/impact/product_details_card.dart)
- [item_metrics_screen.dart](../../app/lib/presentation/screens/item/item_metrics_screen.dart) — Add Product Details section

**Changes:**
1. Create `ProductDetailsCard` widget:
   - "PRODUCT DETAILS" section header (muted uppercase, 11px, letter-spacing)
   - White card with rounded corners (14px), light border
   - Rows: left label (muted 13px) + right value (bold 14px), divided by light separators
   - Optional `onRowTap` callback per row for audit trail navigation
2. In `ItemMetricsScreen._buildContent`:
   - Add Product Details section between the Sharing Impact card and the History section
   - Map `metadataMetrics` to label-value pairs for the card
   - Add CO₂ Footprint and Estimated Value rows from the impact/savings data
   - Only show for `ItemType.gear` (skip for requests and experiences)

**Test**: Widget tests for `ProductDetailsCard` in `app/test/presentation/widgets/impact/product_details_card_test.dart`. Verify: all rows render with correct labels/values, tappable rows fire callbacks, non-tappable rows do not. `StatelessWidget` — no provider overrides needed.

### Phase 4: Timeline Loan History

**Files changed:**
- **New file**: [loan_history_timeline.dart](../../app/lib/presentation/widgets/impact/loan_history_timeline.dart)
- [item_metrics_screen.dart](../../app/lib/presentation/screens/item/item_metrics_screen.dart) — Replace loan history rendering

**Changes:**
1. Create `LoanHistoryTimeline` as a `ConsumerWidget` (needed because child `UserAvatar` calls `ref.watch(mediaUrlProvider)`):
   - Accepts a list of timeline entry data objects
   - Renders vertical track line with dot indicators
   - First entry dot: coral colored; subsequent: muted border color
   - Each entry card: `UserAvatar` (actual profile images), borrower name, date range, value saved (coral, right-aligned)
   - `onEntryTap` callback for navigation to transfer detail
2. In `ItemMetricsScreen._buildGearLoanHistory`:
   - Replace current `LoanHistoryCard` usage with `LoanHistoryTimeline`
   - Pass actual `User` objects (from `loan.recipient`) to `UserAvatar`
   - Wire up `onEntryTap` to navigate to the transfer screen
3. Adapt for requests/experiences if applicable (same timeline pattern, different entry content)

**Test**: Widget tests for `LoanHistoryTimeline` in `app/test/presentation/widgets/impact/loan_history_timeline_test.dart`. Requires `ProviderScope(overrides: [...])` with mock `mediaUrlProvider` since `UserAvatar` is a `ConsumerWidget`. Verify: timeline dots render (first coral, rest muted), entries show borrower name/dates/amount, `onEntryTap` fires with correct transfer ID.

### Phase 5: Polish and Cleanup

**Files changed:**
- [item_metrics_screen.dart](../../app/lib/presentation/screens/item/item_metrics_screen.dart) — Final adjustments
- Possibly [loan_history_card.dart](../../app/lib/presentation/widgets/impact/loan_history_card.dart) — Deprecate or remove if no longer used

**Changes:**
1. Review spacing, padding, and colors against reference screenshots
2. Verify expandable description still works correctly
3. Ensure empty state renders correctly with new layout
4. Test all three item types (gear, request, experience) render correctly
5. Verify audit trail navigation works from both hero metrics and explanatory card
6. Clean up any unused code from the old layout (e.g., if `LoanHistoryCard` is no longer referenced)

### Phase 6: SwipeToCloseMixin, Product Details Audit Trail, and Hero Metrics Data Source

Three fixes that address navigation UX, tappability, and data correctness.

#### 6A. SwipeToCloseMixin on ItemMetricsScreen

**Problem**: `ItemMetricsScreen` uses a plain `Scaffold` without `SwipeToCloseMixin`. When the audit trail screen (which uses `SwipeToCloseMixin` + `buildSwipeableScaffold`) slides in from the right, the parent screen behind it has no slide animation — exposing a black background during the transition.

Per [client/architecture.md](../client/architecture.md): "Wrap full-screen overlay/detail screens with `SwipeToCloseWrapper` for swipe-right-to-close" and "Don't skip swipe gesture support on screens with back buttons."

**Fix**: Add `SwipeToCloseMixin` to `_ItemMetricsScreenState`. This requires:
1. Change from `ConsumerState` to mix in `SingleTickerProviderStateMixin` and `SwipeToCloseMixin`
2. Replace the `Scaffold` in `build()` with `buildSwipeableScaffold()`
3. Update the `AppBarBackButton` to use `onPressed: handleClose` instead of default pop behavior

**Files changed:**
- [item_metrics_screen.dart](../../app/lib/presentation/screens/item/item_metrics_screen.dart)

**Test**: Verify swipe-right closes the screen with slide animation. Verify audit trail opens and closes smoothly without black flash. Verify back button triggers the reverse slide animation.

#### 6B. Product Details Rows Open Audit Trail

**Problem**: In Phase 3, `ProductDetailsCard` rows have optional `onRowTap` callbacks but the plan doesn't specify which rows should be tappable or how they connect to audit trail data.

**Root cause**: In `_buildProductDetails` (~line 277), the `onTap` callback is gated on `metric.confidence != null || metric.reasoning != null`. Category, Brand, and Model metrics from the AI have both `confidence: null` and `reasoning: null` because `GearMetadataFormatter` doesn't populate those fields for simple string values — even though the data *is* AI-detected and `_navigateToMetricDetail` already has a fallback message ("AI-detected from visual analysis.") when reasoning is null. The same gate exists on the Estimated Value row (~line 293).

**Fix**: Make **all** `ProductDetailsCard` rows tappable (when the row has a displayable value), removing the confidence/reasoning null-check gate:

```dart
// BEFORE (gates on confidence/reasoning — Category, Brand, Model excluded):
onTap: isAvailable && (metric.confidence != null || metric.reasoning != null)
    ? () => _navigateToMetricDetail(context, metric)
    : null,

// AFTER (all rows with values are tappable):
onTap: isAvailable ? () => _navigateToMetricDetail(context, metric) : null,
```

Apply the same fix to the Estimated Value row. All rows open the audit trail:
- **Category, Brand, Model, Materials, Weight** → `_navigateToMetricDetail` with fallback "AI-detected from visual analysis."
- **CO₂ Footprint** → `_navigateToMetricDetail` (has provenance from `EstimateGearCarbon`)
- **Estimated Value** → `_navigateToMetricDetail` (has provenance from `ValueEstimate`)

Rows where `displayValue == '—'` (unavailable) remain non-tappable.

**Files changed:**
- [item_metrics_screen.dart](../../app/lib/presentation/screens/item/item_metrics_screen.dart) — Remove confidence/reasoning null-check gate in `_buildProductDetails` (~lines 277 and 293)
- [product_details_card.dart](../../app/lib/presentation/widgets/impact/product_details_card.dart) — Ensure all tappable rows show a subtle chevron indicator

**Test**: Verify tapping **every** product detail row (Category, Brand, Model, Materials, Weight, CO₂ Footprint, Estimated Value) opens its audit trail screen. Verify rows with `displayValue == '—'` remain non-tappable.

#### 6C. Hero Metrics Use Server-Provided Cumulative Impact

**Problem**: The Phase 1 "Metrics source" note originally said to use per-action savings from `displayData.savings`. The hero should instead show **cumulative totals** from the server (`stats.impact`), showing zero when there are no transactions.

The server already provides this via `GetGearStatsResponse`:
- `impact` (field 5) — aggregate impact (per-action × `times_loaned`), server-calculated
- `potential_impact` (field 6) — per-action estimate when no transactions exist

**Fix**: In `_buildHeroStatsRow`, format the hero metrics from `stats.impact` using `SavingsFormatter`. When `stats.impact` is absent (no transactions), display `$0` / `0h` / `0kg` — not the potential impact values. Remove the client-side `CumulativeMetricsCalculator` usage since the server owns cumulative computation.

The "What Sharing This Means" card continues to use per-action data (`displayData.savings`, which may be `potentialImpact` when no transactions exist).

**Data flow summary:**
| Location | Data source | When no transactions |
|----------|-------------|---------------------|
| Hero stats row | `stats.impact` (server cumulative) | Show zeros |
| "What Sharing This Means" card | `displayData.savings` (per-action) | Show per-action estimates from `potentialImpact` |

**Files changed:**
- [item_metrics_screen.dart](../../app/lib/presentation/screens/item/item_metrics_screen.dart) — Use `stats.impact` for hero, remove `CumulativeMetricsCalculator` dependency

**Test**: Verify hero shows cumulative totals matching server data. Verify hero shows zeros when no transactions exist. Verify explanatory card still shows per-action estimates even with zero transactions.

### Phase 7: Fix Content View Navigation to ItemMetricsScreen

#### Problem

All three content views open `ItemMetricsScreen` using the wrong navigation helper:

| File | Method | Line |
|------|--------|------|
| [gear_content_view.dart](../../app/lib/presentation/screens/gear/gear_content_view.dart) | `_showGearEquityModal()` | ~1140 |
| [experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart) | `_showExperienceEquityModal()` | ~1248 |
| [request_content_view.dart](../../app/lib/presentation/screens/request/request_content_view.dart) | `_showRequestEquityModal()` | ~933 |

They all call `NavigationHelpers.pushWithSlide()`, which provides its own slide animation via `PageRouteBuilder` (opaque route, 250ms transition). But after Phase 6A, `ItemMetricsScreen` uses `SwipeToCloseMixin`, which manages its own slide animation internally.

This is the exact problem documented in [client/architecture.md](../client/architecture.md):

> `NavigationHelpers.pushScreen()` — Primary content screens. The destination screen must use `SwipeToCloseMixin` to provide the slide animation.
>
> `NavigationHelpers.pushWithSlide()` — Utility/settings screens that don't need swipe-to-close. The route itself provides the slide animation.

Using `pushWithSlide()` with a `SwipeToCloseMixin` screen causes **double animation** — the route animates in via the `PageRouteBuilder` transition, and then the mixin also runs its slide-in. On close, the mixin slides the content off-screen, but the `PageRouteBuilder` keeps the opaque route present during its own reverse transition, showing a **black background flash**.

The correct approach: `pushScreen()` creates a non-opaque route with zero transition duration, letting `SwipeToCloseMixin` fully own the animation.

#### Fix

In all three content views, change `NavigationHelpers.pushWithSlide()` to `NavigationHelpers.pushScreen()`:

```dart
// BEFORE (wrong — double animation, black flash on close):
NavigationHelpers.pushWithSlide(
  context: context,
  screen: ItemMetricsScreen(...),
);

// AFTER (correct — SwipeToCloseMixin owns the animation):
NavigationHelpers.pushScreen(
  context: context,
  screen: ItemMetricsScreen(...),
);
```

**Files changed:**
- [gear_content_view.dart](../../app/lib/presentation/screens/gear/gear_content_view.dart) — `_showGearEquityModal()`: change `pushWithSlide` → `pushScreen`
- [experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart) — `_showExperienceEquityModal()`: change `pushWithSlide` → `pushScreen`
- [request_content_view.dart](../../app/lib/presentation/screens/request/request_content_view.dart) — `_showRequestEquityModal()`: change `pushWithSlide` → `pushScreen`

**Dependency**: Phase 6A must be completed first (adding `SwipeToCloseMixin` to `ItemMetricsScreen`). Without the mixin, `pushScreen()` would show no animation at all since it relies on the destination screen to animate itself.

**Test**: Open ItemMetricsScreen from each content view (gear, experience, request). Verify:
1. Screen slides in smoothly from the right (single animation, no double-slide)
2. Back button closes with smooth reverse slide (no black flash)
3. Swipe-right-to-close gesture works
4. Opening audit trail from within ItemMetricsScreen also works without black flash (audit trail already uses `SwipeToCloseMixin` + is pushed via `pushScreen` pattern from `_showAuditTrail`)

### Phase 8: Hero Font Consistency, Per-Loan Card Fix, and Provider Invalidation

Three fixes improving visual consistency, data correctness, and navigation freshness.

#### 8A. Hero Metric Font Consistency

**Problem**: The hero stats row in `ItemMetricsScreen` used `fontFamily: 'serif'` (generic fallback) and `Colors.white.withValues(alpha: 0.55)` for labels — inconsistent with the community metrics screen which uses `fontFamily: 'Libre Baskerville'` and coral (`AppColors.lightPrimary`) labels.

**Fix**: Added two shared `TextStyle` constants to `AppTheme`:
- `heroMetricValueStyle` — Libre Baskerville, 22px, w700, white, height 1.2
- `heroMetricLabelStyle` — 10px, w600, 0.5 letter-spacing, `AppColors.lightPrimary` (coral), height 1.2

Updated `_buildHeroStatsRow` in `ItemMetricsScreen` to use these constants instead of inline styles.

**Files changed:**
- [app_theme.dart](../../app/lib/core/theme/app_theme.dart) — Added `heroMetricValueStyle` and `heroMetricLabelStyle`
- [item_metrics_screen.dart](../../app/lib/presentation/screens/item/item_metrics_screen.dart) — Use `AppTheme.heroMetricValueStyle` / `AppTheme.heroMetricLabelStyle`

#### 8B. SharingImpactCard Uses Per-Loan Impact

**Problem**: `displayData.savings` (which feeds the "WHAT SHARING THIS MEANS" card) was computed from `stats.impact` (cumulative). For a gear item with 2 loans, the card would display 2× the per-loan value, contradicting its own framing text ("Each time a friend borrows this…").

Additionally, `potentialImpact` was only populated by the server when `timesLoaned == 0`, making it unavailable for the per-loan display when loans did exist.

**Root cause — server**: All three stats handlers guarded `potentialImpact` behind a zero-transaction check:
```go
if timesLoaned == 0 && s.estimatorCfg != nil { potentialImpact = ... }
```

**Root cause — client**: All three metric VMs used `impact` (cumulative) as the primary source for `displayData.savings`, only falling back to `potentialImpact` when `impact` was absent.

**Fix — server** (all three stats handlers always populate `potentialImpact`):
- `gear/stats.go`: always compute `potentialImpact` via `BuildTransferImpact` (per single loan), regardless of `timesLoaned`
- `request/stats.go`: always compute `potentialImpact` via `BuildRequestImpact`
- `experience/stats.go`: always compute `potentialImpact` via `BuildExperienceImpact` with 1 attendee as the per-session baseline

**Fix — client** (all three metric VMs):
- `displayData.savings` now sources exclusively from `potentialImpact` (per-transaction baseline)
- `cumulativeSavings` in `ItemMetricsScreen._buildContent` continues to source from `stats.impact` (cumulative) for the hero row — unchanged

**Data flow (corrected):**

| Location | Data source | When no transactions |
|----------|-------------|---------------------|
| Hero stats row | `stats.impact` (server cumulative) | Show zeros |
| "WHAT SHARING THIS MEANS" card | `stats.potentialImpact` (server per-action) | Show per-action estimates |

**Files changed:**
- [server/services/gear/stats.go](../../server/services/gear/stats.go)
- [server/services/request/stats.go](../../server/services/request/stats.go)
- [server/services/experience/stats.go](../../server/services/experience/stats.go)
- [gear_metric_view_model.dart](../../app/lib/presentation/viewmodels/gear_metric_view_model.dart)
- [request_metric_view_model.dart](../../app/lib/presentation/viewmodels/request_metric_view_model.dart)
- [experience_metric_view_model.dart](../../app/lib/presentation/viewmodels/experience_metric_view_model.dart)

#### 8C. Provider Invalidation Before Navigation

**Problem**: All three content views opened `ItemMetricsScreen` without invalidating the metric provider first. If the user navigated away and back before the `autoDispose` provider fully disposed, stale Riverpod state could be returned — potentially showing outdated loan counts or impact values.

**Fix**: Added `ref.invalidate(provider(params))` immediately before each `NavigationHelpers.pushScreen` call in the three content views. This forces a fresh provider build on every open, bypassing the autoDispose timing window.

**Files changed:**
- [gear_content_view.dart](../../app/lib/presentation/screens/gear/gear_content_view.dart) — `ref.invalidate(gearMetricProvider(...))`
- [experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart) — `ref.invalidate(experienceMetricProvider(...))`
- [request_content_view.dart](../../app/lib/presentation/screens/request/request_content_view.dart) — `ref.invalidate(requestMetricProvider(...))`

---

**Last Updated:** 2026-02-22
**Status:** Phases 1–8 implemented
