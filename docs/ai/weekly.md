# Weekly Stats Redesign

Redesign the daily screen to focus on conveying weekly impact stats, replacing inline metric breakdowns with dedicated detail screens and adding a visual "days with impact" hero area.

**Reference files:**
- [tmp/weekly/weekly.jsx](../../tmp/weekly/weekly.jsx) — React reference implementation
- [tmp/weekly/weekly.png](../../tmp/weekly/weekly.png) — Reference view for the updated daily screen
- [tmp/weekly/table.png](../../tmp/weekly/table.png) — Reference view for the tabular metric detail screen
- [docs/client/modals.md](../client/modals.md) — Modal best practices

---

## Problems

### 1. Weekly impact is buried in expandable panels

The current design shows metric breakdowns inline via `DailyMetricBreakdown` — an `AnimatedSize` glass card that expands below the stat pills. This works for quick glances but has drawbacks:

- **Limited space:** Line items are squeezed into a narrow card within the scroll view. Users with many completed items this week see a long inline list that pushes "Needs Attention" items far down the screen.
- **No persistent summary:** The total value for a metric and its explanation (subtitle, summary) disappear when the panel collapses. Users can't compare across metrics without toggling back and forth.
- **No dedicated header/context:** The inline panel lacks a proper title area with back navigation. It feels like a tooltip, not a first-class view.

### 2. No visual streak or day-by-day progress

The current header shows today's date and shared value, but there is no indication of which days this week had community activity. The reference design introduces a "days with impact" row — seven circles (M–S) with checkmarks on active days — giving users an immediate sense of weekly momentum.

### 3. "Your Actions Today" duplicates metric data

The "Your Actions Today" section lists concrete actions the user took today (lent items, RSVPs, help offers). This overlaps heavily with the metric breakdown data — the same completed items appear in both places. In the reference design, today's actions fold into the "Actions" metric table, eliminating redundancy.

### 4. No background image treatment for the weekly hero

The community metrics screen uses a striking background image with a semi-opaque overlay (via `ImpactHeroSection`) to create visual weight. The current daily screen has no equivalent — the stat pills sit on the plain cream background. Adding a background image from this week's activity would create a stronger visual identity for the weekly section.

---

## Reference Design Analysis

### Main Screen ([weekly.png](../../tmp/weekly/weekly.png))

From top to bottom:

1. **"THIS WEEK" header** — coral uppercase label (left) + shared value with "SHARED VALUE ›" tap target (right, sage green)
2. **Dark week card** — dark background (`#2D2A26`) with rounded corners (22px radius), containing:
   - Week navigation arrows (‹ / ›) with date range label ("Mar 2 – 8")
   - Seven day circles (M T W T F S S) — coral filled with checkmark when active, dark outline when inactive
   - Day letters below each circle
3. **Metrics grid** — 3+2 layout of white rounded cards (22px radius), each tappable:
   - Row 1: Quality minutes ("47mins / SHARED"), Money saved ("$82 / SAVED"), Time recovered ("135mins / RECOVERED")
   - Row 2: CO₂ avoided ("3.2kg / CO₂ AVOIDED"), Actions count ("4 / ACTIONS")
   - Values in large weight (18pt), labels in small uppercase (9pt)
   - Color-coded values: coral for quality/actions, dark for saved, sage for recovered/CO₂
4. **Friends row** — small avatar circles (30px) for community members
5. **Needs Attention** — item rows with 64px thumbnails, title, subtitle (person + message), time-ago

**Key differences from current design:**
- No expandable inline metric panel — tapping a metric opens a full detail screen
- No "Your Actions Today" section
- No quality-minutes ring — replaced by grid card matching other metrics
- Shared value moved from header subtitle to a tappable link on the right
- Date header replaced by "THIS WEEK" label
- Week card is dark-themed (not glass/white)

### Metric Detail Screen ([table.png](../../tmp/weekly/table.png))

Full-screen page that slides in from the right when a metric is tapped:

1. **Back button** — coral "‹ Back" in top left
2. **Title area** — metric name in large serif (22pt, "Shared Value") + total value in coral (22pt, right-aligned) + subtitle below (13pt, muted)
3. **Divider line**
4. **Item list** — scrollable, each row showing:
   - Item name (15pt, 600wt)
   - Status/context below (13pt, muted — e.g., "Available", "Derek saved", "Mon")
   - Value on the right (16pt, 700wt, coral)
   - Divider between rows

---

## Proposed Solution

### Approach: Phased refactor of the daily screen

Replace the current header/metrics/actions-today area with the reference design layout. Metric detail becomes a new screen pushed via `NavigationHelpers.pushWithSlide()` (standard right-to-left page transition). No server API changes are required — all data already exists in `GetDailyViewResponse`.

### Architecture Decisions

**Metric detail as a pushed route (not a modal):** The reference design shows a full-screen view with a "Back" button, not a bottom sheet. Per `docs/client/architecture.md`, all forward navigation must use `NavigationHelpers` — never raw `Navigator.push(MaterialPageRoute(...))`. Since the metric detail screen is a utility/data-display screen (not a primary content screen needing swipe-to-close), use `NavigationHelpers.pushWithSlide()`. This provides the correct slide-from-right animation while keeping the previous screen stationary (no blank background flash). The back button uses standard `Navigator.pop(context)`.

> **[AUDIT FIX]** Changed from `Navigator.push(MaterialPageRoute(...))` to `NavigationHelpers.pushWithSlide()`. Raw `MaterialPageRoute` animates both incoming and outgoing screens simultaneously, causing the daily screen to slide left and flash a blank background. This violates the Navigation & Gestures section of `docs/client/architecture.md`.

**Dark week card with background image:** The reference JSX uses a solid dark background with an ambient glow. The user requested a background image with semi-opaque overlay, like `ImpactHeroSection` in the community metrics screen. We'll combine both: dark card with an optional background image (from this week's actions) overlaid with a semi-opaque dark gradient. When no action has media, fall back to the solid dark style from the reference.

**"Actions" as a metric:** The reference design adds an "ACTIONS" card to the metrics grid. The data source is the existing `actions_today` field from `GetDailyViewResponse`. This means actions are always accessible via the metric detail screen, even though the dedicated "Your Actions Today" section is removed.

**Week navigation (deferred):** The reference JSX includes ‹/› arrows to browse previous weeks. This requires a server API change (passing a week offset or date range). We'll build the UI with week nav arrows in place but initially only show the current week. Server support for historical weeks can be added later.

**Shared value detail:** The reference design shows "SHARED VALUE ›" as a tappable link that opens the same tabular detail screen. The data source is the existing `shared_items` field.

---

## Open Questions

1. **Background image source:** Which media should appear behind the week card? Options:
   - **(a) First action's background media** — use `background_media_id` from the first `DailyAction` that has one (most recent action with an image)
   - **(b) Any completed item media** — scan metric breakdown items for one with media
   - **(c) Community cover image** — always use the community's cover image (already available via community provider)
   - **Recommendation:** Option (a) — most dynamic and personal. Falls back to solid dark if no action has media.

2. **Week navigation scope:** Should the initial implementation include the ‹/› week arrows (UI only, disabled) or omit them entirely until the server supports historical weeks?
   - **Recommendation:** Include the arrows but show them disabled/hidden for now. This keeps the layout faithful to the reference and makes the server work additive.

3. **Quality minutes ring vs. grid card:** The current design uses a custom-painted ring widget for quality minutes. The reference replaces it with a flat grid card like the other metrics. Should we preserve the ring anywhere (e.g., inside the metric detail screen)?
   - **Recommendation:** Remove the ring entirely and use the flat grid card. The ring added visual interest but took disproportionate horizontal space. The flat card is consistent with the other metrics and matches the reference exactly.

4. **"Actions" metric — what time range?** The current `actions_today` only includes today's actions. Should the "ACTIONS" metric card count this week's actions (requiring a new server field) or just today's?
   - **Recommendation:** For phase 1, show today's actions count and list. In a future phase, add a `weekly_actions` field to `WeeklyMetrics` on the server for full-week action history.

---

## Phased Implementation Plan

### Phase 1: Metric Detail Screen

**Goal:** Create the tabular detail screen that metric taps will navigate to, following [table.png](../../tmp/weekly/table.png).

**Changes:**

- **New file:** `app/lib/presentation/screens/daily/metric_detail_screen.dart`
  - Full-screen `StatelessWidget` (or `ConsumerWidget` if it needs providers)
  - Accepts parameters: `title`, `subtitle`, `totalValue`, `totalValueColor`, `items` (list of `MetricDetailItem`), and optional `onItemTap` callback
  - `MetricDetailItem` data class: `name`, `status`, `value`, `contentId`, `itemType`
  - Layout per the reference: back button, title row with total value, subtitle, divider, scrollable item list
  - Static `show()` method that calls `NavigationHelpers.pushWithSlide()`
  - Back button uses `Navigator.pop(context)` — standard route pop
  - Cream background (`AppColors.dailyBackground` / `#FDF8F0`)
  - Coral values, serif title font — matching existing daily screen typography

- **New file:** `app/lib/presentation/models/metric_detail_item.dart`
  - Simple data class: `name`, `status`, `value`, `contentId`, `itemType`
  - Used by the detail screen and by the daily view model to prepare data

> **[AUDIT FIX]** Moved from `screens/daily/` to `presentation/models/`. Per `docs/client/architecture.md`, UI-specific model classes belong in `presentation/models/`, not alongside screen files.

- **Modify:** `app/lib/presentation/viewmodels/daily_view_model.dart`
  - Add helper methods to build `List<MetricDetailItem>` from the existing breakdown data:
    - `sharedValueItems()` — from `response.sharedItems`
    - `qualityMinutesItems()` — from `response.weeklyMetrics.richMinutesBreakdown`
    - `costSavedItems()` — from `response.weeklyMetrics.costBreakdown`
    - `timeRecoveredItems()` — from `response.weeklyMetrics.timeBreakdown`
    - `co2AvoidedItems()` — from `response.weeklyMetrics.co2Breakdown`
    - `actionsItems()` — from `response.actionsToday`
  - Remove `expandedMetric` state field and `toggleMetric()` method (no longer needed)

- **Modify:** `app/lib/presentation/screens/daily/daily_screen.dart`
  - Replace metric tap handler: instead of `toggleMetric()`, call `MetricDetailScreen.show()` with the appropriate data
  - Wire up item taps in the detail screen to navigate to GearScreen/ExperienceScreen/RequestScreen (same as current metric breakdown taps)
  - Add `context.mounted` guard after awaiting `MetricDetailScreen.show()` before triggering `onTabVisible()` refresh (per architecture.md async safety rules)

> **[AUDIT FIX]** Added `context.mounted` guard requirement. Per `docs/client/architecture.md`: "Add a `if (!context.mounted) return;` guard between `Navigator.pop()` and any subsequent navigation call." The detail screen's item taps also need this guard when navigating from item row to content screen.

**Tests:**
- Widget test for `MetricDetailScreen`: renders title, subtitle, total value, item rows
- Widget test: tapping back button pops the route
- Widget test: tapping an item row calls onItemTap with correct data
- Unit test: view model helper methods build correct `MetricDetailItem` lists from mock response data

---

### Phase 2: Weekly Header and Day Circles

**Goal:** Replace the current `DailyHeader` + `DailyStatPills` + `DailyMetricBreakdown` area with the new layout from [weekly.png](../../tmp/weekly/weekly.png): "THIS WEEK" header, dark week card with day circles, and metrics grid.

**Changes:**

- **New file:** `app/lib/presentation/widgets/daily/weekly_header.dart`
  - "THIS WEEK" coral uppercase label (left) + shared value with "SHARED VALUE ›" (right, sage green)
  - Accepts `onSharedValueTap` callback; `DailyScreen` wires this to open `MetricDetailScreen` with shared value items
  - Uses existing `DailyState.response.sharedValue` for the formatted total

> **[AUDIT FIX]** Changed from widget calling `MetricDetailScreen.show()` directly to accepting an `onSharedValueTap` callback. Per `docs/client/architecture.md`, screens own navigation — widgets should call back to the parent screen via callbacks or ViewModel methods, not navigate directly.

- **New file:** `app/lib/presentation/widgets/daily/weekly_day_card.dart`
  - Dark rounded card (`#2D2A26`, 22px radius)
  - Background image layer: if a background media URL is available, show it with `CachedMediaImage` using `ImageCacheKeys.full(mediaId)` (card is >= 200px, so full-resolution per caching.md) + semi-opaque dark gradient overlay (matching `ImpactHeroSection` pattern: `Colors.black.withValues(alpha: 0.7)` top, `0.8` bottom)
  - Week navigation row: ‹/› arrows (initially disabled/hidden) + date range label (from `weeklyMetrics.weekLabel`, stripped of "This week · " prefix)
  - Day circles row: 7 circles (M T W T F S S), coral filled + checkmark when active, dark outline when inactive
  - Day letter labels below each circle

- **Proto change:** `proto/ripls/api/daily.proto`
  - Add `repeated bool active_days = 16;` to `WeeklyMetrics` — 7 booleans (Mon–Sun), true if the user had at least one completed action on that day
  - Add `string week_background_media_id = 17;` to `WeeklyMetrics` — media ID for the week card background image

> **[AUDIT FIX]** Corrected proto field numbers from 18/19 to 16/17. `WeeklyMetrics` currently ends at field 15 (`co2_summary`). Proto field numbers must be sequential and unique within the message.

- **Server change:** `server/services/daily/metrics.go`
  - Compute `active_days` from the same completed-items data used for metrics: for each day (Mon–Sun), true if any item completed or action occurred on that day
  - Compute `week_background_media_id`: scan completed items for the first one with a non-empty background media ID; use that

- **New file:** `app/lib/presentation/widgets/daily/weekly_metrics_grid.dart`
  - 3+2 grid of white rounded metric cards (22px radius, subtle shadow)
  - Row 1: Quality minutes, Money saved, Time recovered
  - Row 2: CO₂ avoided, Actions count
  - Each card: large value (18pt, 700wt) + small uppercase label (9pt, 700wt, muted)
  - Color coding: coral for quality/actions, dark text for saved, sage for recovered/CO₂
  - Each card tappable → accepts `onMetricTap(String metricKey)` callback; `DailyScreen` wires this to open `MetricDetailScreen` with the appropriate data

- **Modify:** `app/lib/presentation/screens/daily/daily_screen.dart`
  - Replace the header/pills/breakdown area with: `WeeklyHeader` → `WeeklyDayCard` → `WeeklyMetricsGrid`
  - Remove `DailyMetricBreakdown` from the widget tree entirely

- **Remove:** `app/lib/presentation/widgets/daily/daily_metric_breakdown.dart` (no longer needed)
- **Remove:** `app/lib/presentation/widgets/daily/daily_stat_pills.dart` (replaced by grid)
- **Remove:** `app/lib/presentation/widgets/daily/daily_ring.dart` (ring no longer used)
- **Remove:** `app/lib/presentation/widgets/daily/daily_header.dart` (replaced by weekly_header)

- **Modify:** `app/lib/presentation/viewmodels/daily_view_model.dart`
  - Remove `expandedMetric` field entirely (already done in Phase 1)
  - Remove `expandedValue` field and `toggleValue()` — shared value is now always visible as a tap target, not an expandable panel
  - Run `flutter pub run build_runner build` after Freezed state changes to regenerate `daily_view_model.freezed.dart`

> **[AUDIT FIX]** Added build_runner step. `DailyState` uses Freezed — removing fields requires code regeneration or the app won't compile.

**Tests:**
- Widget test for `WeeklyDayCard`: renders 7 day circles, active days show checkmark, inactive show empty
- Widget test for `WeeklyMetricsGrid`: renders 5 metric cards with correct values
- Widget test: tapping a metric card triggers navigation
- Server test for `active_days` computation: verify correct days are flagged based on completed items
- Server test for `week_background_media_id`: picks first item with media

---

### Phase 3: Remove "Your Actions Today" Section

**Goal:** Remove the standalone "Your Actions Today" section. Action data is now accessible via the "ACTIONS" metric card.

**Changes:**

- **Modify:** `app/lib/presentation/screens/daily/daily_screen.dart`
  - Remove the `DailyActionsToday` widget and its surrounding section header from the scroll view
  - The "Needs Attention" section moves up to directly follow the metrics grid + friends row

- **Remove (or deprecate):** `app/lib/presentation/widgets/daily/daily_actions_today.dart`
  - Can be deleted if no other screen uses it. The action data is still served by the API and consumed by the metric detail screen.

- **Remove:** `app/lib/presentation/widgets/daily/daily_value_panel.dart`
  - The expandable value panel is replaced by the shared value detail screen (tap "SHARED VALUE ›" → `MetricDetailScreen`)

- **Modify:** `app/lib/presentation/widgets/daily/daily_shared_value.dart`
  - Either remove this file or repurpose it for the new `WeeklyHeader` shared value display. If `WeeklyHeader` inlines the shared value display, this file can be deleted.

**Tests:**
- Integration-level widget test: verify "Your Actions Today" no longer appears in the daily screen
- Verify tapping "ACTIONS" metric card shows the correct action items in the detail screen

---

### Phase 4: Polish and Consistency

**Goal:** Visual refinements and edge case handling.

**Changes:**

- **Empty states:**
  - Week card with no active days: show all circles as inactive (no checkmarks), card still renders
  - Metrics with zero values: show "0" (not hidden), tapping still opens detail screen with empty list + encouraging message
  - No background image: week card uses solid dark background (no image layer)

- **Animation:**
  - Metric card tap: `NavigationHelpers.pushWithSlide()` animation (right-to-left slide, previous screen stays still)
  - Shared value tap: same push animation
  - Consider `Hero` animation for the metric value (card → detail screen header) if it feels natural

- **Accessibility:**
  - Semantic labels on day circles ("Monday, active" / "Tuesday, no activity")
  - Metric cards have `Semantics` wrappers with descriptive labels
  - Detail screen item rows announce name, status, and value

- **Scroll position:**
  - After returning from a metric detail screen, restore scroll position (Flutter does this automatically with `Navigator.push`)
  - `onTabVisible()` still fires on pop for background refresh

- **Typography audit:**
  - Ensure serif font (Georgia / Libre Baskerville) is used for all large values per the reference
  - Ensure label sizing matches reference (9pt uppercase labels, 18pt values, 22pt detail titles)

- **Dark mode readiness:**
  - Use `AppColors` constants for all new colors (don't hard-code hex in widgets)
  - Week card already uses dark palette; ensure it works in both themes

**Tests:**
- Widget test: empty state renders correctly (no active days, zero metrics)
- Widget test: detail screen with empty items shows appropriate message
- Accessibility audit: TalkBack/VoiceOver reads correct labels

---

## Summary of Files Changed

### New Files
| File | Phase | Purpose |
|------|-------|---------|
| `app/lib/presentation/screens/daily/metric_detail_screen.dart` | 1 | Tabular metric detail (full-screen route) |
| `app/lib/presentation/models/metric_detail_item.dart` | 1 | Data class for detail screen items |
| `app/lib/presentation/widgets/daily/weekly_header.dart` | 2 | "THIS WEEK" + shared value header |
| `app/lib/presentation/widgets/daily/weekly_day_card.dart` | 2 | Dark card with day circles + background image |
| `app/lib/presentation/widgets/daily/weekly_metrics_grid.dart` | 2 | 3+2 grid of tappable metric cards |

### Modified Files
| File | Phase | Change |
|------|-------|--------|
| `app/lib/presentation/viewmodels/daily_view_model.dart` | 1–2 | Add detail item builders; remove `expandedMetric`, `expandedValue`, `toggleMetric()`, `toggleValue()` |
| `app/lib/presentation/screens/daily/daily_screen.dart` | 1–3 | Replace header/pills/breakdown/actions with new widgets; wire metric taps to detail screen |
| `proto/ripls/api/daily.proto` | 2 | Add `active_days`, `week_background_media_id` to `WeeklyMetrics` |
| `server/services/daily/metrics.go` | 2 | Compute active days and background media ID |

### Removed Files
| File | Phase | Reason |
|------|-------|--------|
| `app/lib/presentation/widgets/daily/daily_metric_breakdown.dart` | 2 | Replaced by `MetricDetailScreen` |
| `app/lib/presentation/widgets/daily/daily_stat_pills.dart` | 2 | Replaced by `WeeklyMetricsGrid` |
| `app/lib/presentation/widgets/daily/daily_ring.dart` | 2 | Ring replaced by flat grid card |
| `app/lib/presentation/widgets/daily/daily_header.dart` | 2 | Replaced by `WeeklyHeader` |
| `app/lib/presentation/widgets/daily/daily_actions_today.dart` | 3 | Actions moved to metric detail screen |
| `app/lib/presentation/widgets/daily/daily_value_panel.dart` | 3 | Replaced by shared value detail screen |
| `app/lib/presentation/widgets/daily/daily_shared_value.dart` | 3 | Inlined into `WeeklyHeader` |

---

## Key Server Files (Existing)

| File | Purpose |
|------|---------|
| [server/services/daily/metrics.go](../../server/services/daily/metrics.go) | Weekly metrics + breakdowns (modify in Phase 2) |
| [server/services/daily/actions.go](../../server/services/daily/actions.go) | `assembleActionsToday`, `assembleSharedInventory` (unchanged) |
| [server/services/daily/daily_view.go](../../server/services/daily/daily_view.go) | `GetDailyView` handler (unchanged) |

## Key Client Files (Existing)

| File | Purpose |
|------|---------|
| [app/lib/presentation/screens/daily/daily_screen.dart](../../app/lib/presentation/screens/daily/daily_screen.dart) | Screen assembly (modify in all phases) |
| [app/lib/presentation/viewmodels/daily_view_model.dart](../../app/lib/presentation/viewmodels/daily_view_model.dart) | State management (modify in Phases 1–2) |
| [app/lib/presentation/widgets/impact/impact_hero_section.dart](../../app/lib/presentation/widgets/impact/impact_hero_section.dart) | Reference for background image + overlay pattern |

---

## Documentation Updates

After implementation, update [docs/daily.md](../daily.md):
- Replace "Stat pills row" description with "Weekly metrics grid"
- Replace "Metric breakdown panel" with "Metric detail screen (pushed route)"
- Remove "Your Actions Today" section description
- Add "Weekly Day Card" section describing the dark hero card with day circles
- Update header description to reflect "THIS WEEK" + shared value layout
- Update Key Client Files table with new file names
