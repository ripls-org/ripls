# Impact Metrics: False Savings & Detail Screen Redesign

**Date:** 2026-04-02
**Status:** Draft
**Audit:** Passed with fixes (2026-04-02) — see `[AUDIT]` tags inline

---

## Problem 1: Listed Communities Show Non-Zero Savings with Zero Activity

### Symptoms

A new community with several items added but **zero** completed loans, giveaways, experiences, or requests shows:
- $176 saved
- 210 quality time minutes
- 117 kg CO2 avoided

### Root Cause

In `community_metrics_screen.dart` lines 260-276, the "Listed" stage (items exist but 0 completed transactions) computes **potential** values from library data and displays them as if they were actual savings:

```dart
final potentialMoney = isListed && metrics.totalValueUsd > 0
    ? _formatMoney(metrics.totalValueUsd * 0.075)   // $2350 * 0.075 = $176
    : data.costSavingsFormatted;
final potentialCo2 = isListed && metrics.totalValueUsd > 0
    ? _formatKg(metrics.totalValueUsd * 0.05)        // $2350 * 0.05 = 117 kg
    : data.carbonSavingsFormatted;
final potentialQt = isListed && metrics.gearCount > 0
    ? _formatRm((metrics.gearCount * 42).round())    // 5 * 42 = 210 min
    : data.qualityTimeFormatted;
```

The same logic is in `_heroValueFor()` (lines 1226-1257). These synthetic projections are shown without any visual indicator that they are projections, not actual tracked savings.

### Impact

Users see fabricated metrics that don't correspond to any actual sharing activity. This undermines trust in the entire metrics system.

---

## Problem 2: Metric Detail Screens Need Redesign

### Current State

The `MetricDetailScreen` shows:
- Large hero value + label
- Italic anchoring quote/hook (e.g., "The average American spends...")
- Trend card
- Chart
- Category tables (top items, top contributors)
- Research citation
- Methodology link

### Issues

1. **No description** of what the metric means (e.g., what "Library Value" represents)
2. **Library Value chart** only shows a single cumulative value line, missing an items count line/bar overlay
3. **Nudges and quotes** (anchoring hooks like "The average American spends...") appear on detail screens but should only be on the main community metrics screen
4. **Inconsistent layout** across metric detail screens - no standard template

### Desired Layout (per reference design `tmp/impact/inventory.png`)

Every detail metric screen should follow this standard template:
1. **Title** (e.g., "Library Value")
2. **Hero value** (e.g., "$2,350")
3. **Description card** explaining what the metric means
4. **Chart** showing trends over time
5. **Table** with item-level detail (tappable rows that navigate to the item)

For Library Value specifically:
- Description: Use `context.l10n.metricDetailLibraryValueDescription` — **`[AUDIT]` i18n: all user-visible description strings must be in ARB files per `docs/client/i18n.md`, not hardcoded Dart strings. Add keys to both `app_en.arb` and `app_es.arb`.**
- Chart: dual-axis with a **line for value** and a **bar/line for total items count** (labeled "Value" and "Items")
- Table: header row "Inventory — N Items — $X,XXX" with each item showing name, owner, date, and value

---

## Potential Solutions

### Problem 1: False Savings

**Option A: Show zeros for Listed stage**
Replace the synthetic potential values with actual zeros (from `data.costSavingsFormatted`, etc.). The Listed stage content area already shows projection/scenario cards explaining potential impact - those are the right place for "what could happen" messaging.

**Option B: Keep projections but label them clearly**
Add a "Potential" prefix or badge (e.g., "~$176 potential") and use a muted/dimmed style. This preserves the motivational intent but avoids deception.

**Option C: Show library value as the hero metric for Listed stage**
Since the community's only real metric is library value, show that as the hero instead of projected savings.

### Problem 2: Detail Screen Redesign

**Option A: Modify existing `MetricDetailScreen`**
Refactor the existing screen to follow the standard template: add a description card, remove nudges/quotes, enhance the chart for Library Value. Lowest effort since it reuses the existing screen.

**Option B: Create category-specific detail screens**
Build separate screens for Library Value, Money Saved, CO2, Quality Time. Each can have dimension-specific chart configurations and table formats. More code but cleaner separation.

---

## Recommendations

### Problem 1: Option A (show zeros)

Show actual zero values in the Listed stage. The existing scenario cards and projection content below the hero already communicate potential impact in the right context. Showing fabricated numbers in the hero/tabs is misleading regardless of labeling.

Specific changes:
- Remove the `isListed` potential-value branches in `_buildTabBar()` (lines 262-269)
- Remove the `isListed` potential-value branches in `_heroValueFor()` (lines 1240-1252)
- Both methods fall through to actual formatted values, which will be "$0" / "0 kg" / "0 rm"

### Problem 2: Option A (modify existing screen)

Refactor `MetricDetailScreen` to follow the standard template from the reference design. This applies to all detail screens (Library Value, Money Saved, CO2, Quality Time).

Specific changes:
1. **Add description card** below the hero value - a rounded card with explanatory text per dimension
2. **Remove the italic hook/quote** from detail screens (keep only on main metrics screen)
3. **Remove research citations** from detail screens (keep only on main metrics screen)
4. **Enhance Library Value chart** to show dual series (value line + items count bar/line)
5. **Standardize table headers** to show summary row (e.g., "Inventory - 5 Items - $2,350")
6. **Ensure item rows are tappable** and navigate to the gear detail screen

---

## Decisions

1. **Listed stage hero metric:** Show "$0 saved" — accurate zeros, no synthetic projections.
2. **Description text:** Client-owned for i18n safety. Add to l10n strings alongside existing `ImpactHook` patterns.
3. **Dual-axis chart:** Use whatever is easiest to implement in the existing `ImpactChartCard` widget (likely a secondary line series).
4. **Table navigation:** All rows navigate to the item (gear detail, request detail, etc.).
5. **Portfolio detail screen:** Same redesign applies — include in this effort.

---

## Phased Implementation Plan

### Phase 1: Fix False Savings (Critical Bug)

**Scope:** Remove synthetic potential values from Listed stage hero and tab bar.

**Files:**
- `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart`
  - `_buildTabBar()`: Remove `isListed` branches for potentialMoney, potentialCo2, potentialQt; always use actual formatted values
  - `_heroValueFor()`: Remove `isListed` branches; always use actual formatted values

**Verification:** Create a community with items but no transactions. Hero and tabs should show $0 / 0 kg / 0 rm. Existing Listed stage content (scenario cards, projections) still appears in the body.

**Risk:** Low. Only affects UI display in the Listed stage. No server changes. No data model changes.

### Phase 2: Standard Detail Screen Template

**Scope:** Establish the standard layout for all metric detail screens: title, hero value, description, chart, table.

**Files:**
- `app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart`
  - Remove italic hook/quote text widget
  - Remove `_buildCitation()` call and related citation widgets
  - Add description card widget below hero value (dimension-specific text)
  - Keep trend card, chart, tables, methodology link

**New widgets needed:**
- Description card widget (rounded card with paragraph text) - or reuse existing `MetricDescriptionCard` if suitable

**`[AUDIT]` i18n — add description strings to ARB files:**
- Add `metricDetailMoneyDescription`, `metricDetailEmissionsDescription`, `metricDetailQualityTimeDescription`, `metricDetailTimeDescription`, `metricDetailLibraryValueDescription` keys to both `app/lib/l10n/app_en.arb` and `app_es.arb`. The screen must use `context.l10n.*` — viewmodels must not resolve these strings (per `docs/client/i18n.md` and `docs/client/architecture.md`).

**`[AUDIT]` Theming — all colors must use `AppColors.xxx(context)`:**
- The description card must use `AppColors.cardBackground(context)` for its background, `AppColors.textPrimary(context)` / `AppColors.textSecondary(context)` for text — no hardcoded colors (per `docs/client/architecture.md` Colors and Theming section).

**Verification:** Open each dimension's detail screen. Should show: title, hero value, description card, chart, table. No quotes or citations.

### Phase 3: Library Value Detail Enhancement

**Scope:** Enhance the Library Value detail screen to match the reference design.

**Files:**
- `app/lib/presentation/widgets/impact/community/impact_chart_card.dart`
  - Extend to support dual-series data (line + bar/secondary line)
  - Add legend labels ("Value" and "Items")
- `app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart`
  - For LibraryValue category: pass both library value trend AND items count trend to the chart
  - Update table header to show "Inventory — N Items — $X,XXX" format
  - Ensure item rows are tappable (navigate to gear detail via `NavigationHelpers.pushToItemScreen`)

**Server changes (if needed):**
- `server/impact_metrics/metric_detail_series.go`: `computeLibraryValueTrend()` may already return value data; verify it also returns item count per bucket, or add a parallel series
- `proto/ripls/api/impact_service.proto`: May need a secondary series field on `TimeSeriesPoint` or a parallel `library_items_trend` field on the response
  - **`[AUDIT]` Proto rules: new field must use a dedicated message type (not reuse `TimeSeriesPoint` for a different semantic). Per CLAUDE.md, each RPC should have its own dedicated types. Adding a `library_item_count_trend` repeated field of `TimeSeriesPoint` on the existing response is acceptable since it's the same response message — but do NOT add a `secondary_value` field to `TimeSeriesPoint` itself, as that message is shared across dimensions.**
- **`[AUDIT]` Server: add unit tests for the new items-count series computation per `docs/server/architecture.md` testing requirements. Use table-driven tests.**
- **`[AUDIT]` Server: run `npm run build` after changes to verify lint + compilation (per CLAUDE.md Go Server Build Verification).**

**`[AUDIT]` Navigation — use `NavigationHelpers.pushToItemScreen()`:**
- Item rows must navigate via `NavigationHelpers.pushToItemScreen(context: context, itemId: id, itemType: type)` — never raw `Navigator.push(MaterialPageRoute(...))` (per `docs/client/architecture.md` Navigation section).

**Verification:** Open Library Value detail. Chart shows value line and items bar with legend. Table shows all inventory items sorted by value with tappable rows.

### Phase 4: Apply Template to All Dimensions

**Scope:** Ensure Money Saved, CO2, and Quality Time detail screens match the standard template.

**Files:**
- `app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart`
  - Add dimension-specific descriptions for Money, CO2, Quality Time
  - Standardize table headers per dimension
  - Ensure tappable rows navigate correctly (gear for loans/giveaways, request detail for requests, etc.)
- **`[AUDIT]` i18n: add ALL dimension description strings and table header strings to both `app_en.arb` and `app_es.arb`. Use ICU message format for parameterized strings (e.g., `"Inventory — {count} Items — {value}"` with placeholders declared in `@key` metadata).**

**Verification:** Each dimension detail screen follows the template: title, hero, description, chart, table with tappable rows.

### Phase 5: Portfolio Detail Parity

**Scope:** Apply the same redesign to `PortfolioMetricDetailScreen`.

**Files:**
- `app/lib/presentation/screens/portfolio/portfolio_metric_detail_screen.dart`
  - Remove nudges/quotes, add description card, standardize layout to match community detail screens
  - **`[AUDIT]` Reuse the same l10n keys and description card widget from Phases 2-4 — do not duplicate strings or widgets.**

---

## Audit Summary

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`

**Issues found and fixed inline (5):**

| # | Category | Finding | Phase |
|---|----------|---------|-------|
| 1 | **i18n** | Description text for Library Value was written as a hardcoded English string in the plan. All user-visible strings must go through ARB files and `context.l10n.*`. | 2, 3, 4 |
| 2 | **Theming** | Description card styling not specified. Must use `AppColors.xxx(context)` — no hardcoded colors. | 2 |
| 3 | **Navigation** | Table row navigation must use `NavigationHelpers.pushToItemScreen()`, not raw `Navigator.push()`. | 3 |
| 4 | **Proto design** | If adding an items-count series, must not mutate the shared `TimeSeriesPoint` message with a `secondary_value` field — add a parallel repeated field on the response instead. | 3 |
| 5 | **Server testing** | Server changes need unit tests (table-driven) and `npm run build` verification. | 3 |

**No issues found in:**
- Caching: Plan makes no caching changes. Impact data already flows through existing repository/viewmodel pattern. No manual cache maps introduced. Compliant.
- Architecture layers: All changes stay within correct layers — screens modify presentation, viewmodels untouched, no service-layer bypass. Compliant.
- Server architecture: No service-to-service dependencies introduced. Shared library pattern followed for `metric_detail_series.go`. Compliant.
