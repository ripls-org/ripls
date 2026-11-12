# Issue #1009: User screen is wonky

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1009
**Filed:** 2026-04-03 | **Last activity:** 2026-04-03
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

The user screen (shown when tapping on an item owner) has two problems:
1. It displays **old metrics** — the data model and UI were built before the portfolio metrics screen matured, so it shows a simpler, less rich set of metrics.
2. It shows **floating chips in the header area** that shouldn't be there — the `ImpactMetricsHeroSection` is passed `chips: const []` but the hero section still renders structural elements that look broken.

The issue asks to rebuild the user screen from scratch so it mirrors the portfolio metrics screen content, but in a **read-only mode** (no drill-down navigation to detail screens). The key concern is keeping the two screens in sync over time — any future changes to portfolio metrics should automatically apply to the user screen.

## Context from Comments

No comments on this issue. The requirements are clear from the description: show the same content as portfolio metrics, keep it DRY, and prevent detail-screen navigation.

## Potential Solutions

### Option A: Extract shared content widget

Extract the body of `PortfolioMetricsScreen` into a reusable `ImpactMetricsContent` widget that accepts a `readOnly` flag. When `readOnly` is true, all `onTap` callbacks on stat pills, breakdown rows, hero value, and communities section are null. The user screen and portfolio screen both compose this widget, each providing their own data source.

- **Approach:** Create `ImpactMetricsContent` widget that renders hero + tabs + stat pills + comparison cards + chart + sufficiency ring + trend card + direction breakdown + source breakdown + communities section. Accept all data as parameters (or a unified state object). Portfolio screen passes its own providers + `readOnly: false`; user screen maps `UserImpactData` into the same shape and passes `readOnly: true`.
- **Pros:** Maximum code sharing — single widget renders both screens. Future portfolio UI changes automatically apply to user screen. Clear separation between data loading and presentation.
- **Cons:** Requires a data mapping layer to transform `UserImpactData` into the shape `ImpactMetricsContent` expects, since the two APIs return different response structures. Some portfolio-specific sections (communities ranking, direction breakdown) may not have data on the user side, requiring conditional rendering. The shared widget could accumulate complexity.
- **Effort:** Medium.

### Option B: Reuse PortfolioMetricsScreen directly with a userId parameter

Extend `PortfolioMetricsScreen` to accept an optional `userId` parameter. When set, it fetches portfolio metrics for that user (via a new or extended API) instead of the authenticated user. Add a `readOnly` mode that disables all navigation.

- **Approach:** Add `userId` and `readOnly` parameters to `PortfolioMetricsScreen`. Create a new provider variant (or extend existing ones) that fetches metrics for a specific user. The user screen becomes a thin wrapper around `PortfolioMetricsScreen(userId: userId, readOnly: true)`.
- **Pros:** Zero widget duplication — it's literally the same screen. Any change to portfolio is automatically reflected in user view.
- **Cons:** Requires a **new server API** (`GetPortfolioMetrics` currently only works for the authenticated user). The portfolio viewmodel/providers need significant refactoring to support parameterization by userId. Mixing "own portfolio" and "other user's portfolio" concerns in one screen can create confusing conditional logic. Larger server-side effort.
- **Effort:** Large.

### Option C: Rebuild UserMetricsScreen to match portfolio layout using shared widgets

Keep `UserMetricsScreen` as a separate screen but rebuild its `_buildActiveContent` method to include all the visual sections from portfolio metrics (comparison cards, sufficiency ring, direction breakdown, etc.), using the same shared widgets. Expand the `GetUserImpactMetrics` API to return the additional data needed (direction breakdown, comparison card data, trend data for all dimensions).

- **Approach:** Enrich `GetUserImpactMetricsResponse` with additional fields that portfolio already has (direction breakdown, CO2/QT trends, comparison data, sufficiency data). Update `UserImpactData` and `UserImpactMetricsNotifier` to handle the new fields. Rebuild `UserMetricsScreen._buildActiveContent` to render the same sections as portfolio, using the same shared widgets (`ForestComparisonCard`, `MealComparisonCard`, `WalkComparisonCard`, `SufficiencyRing`, etc.) but without `onTap` callbacks.
- **Pros:** Each screen stays independent, so changes to one don't risk breaking the other. No data mapping layer needed — the data comes in the right shape from the server.
- **Cons:** The two screens can **drift out of sync** — if someone adds a section to portfolio, they have to remember to add it to user screen too. This is the exact problem the issue warns about. More server-side work to expose the additional data.
- **Effort:** Medium-Large.

## Recommendation

**Option A: Extract shared content widget.** This is the best balance of DRY code and manageable complexity:

1. It solves the sync problem directly — one widget renders both views, so future changes to the content layout apply to both screens automatically.
2. It doesn't require new server APIs — the existing `GetUserImpactMetrics` response already has the core data (metrics, source breakdowns, trends, insights). We just need a lightweight adapter to map `UserImpactData` into the shape the shared widget expects.
3. It keeps the two screens' data loading independent — portfolio uses its own providers, user screen uses `userImpactProvider`. Only the presentation is shared.
4. The `readOnly` flag is a clean, minimal way to control navigation behavior.

The data mapping gap is real but manageable: portfolio has some data the user API doesn't return (direction breakdown, communities ranking, comparison card details, sufficiency ring data). For the user screen, these sections can be conditionally hidden when the data isn't available, or we can incrementally extend the user API to return them.

## Open Questions (Resolved)

- **Should sections missing data be hidden or shown with empty state?** — **Decision: Show all sections.** Do not hide sections when data is missing; render them with an empty/zero state instead. This keeps the layout consistent between portfolio and user screens and makes it obvious when data is available vs not.

- **Should the user screen show comparison cards (forest, meal, walk)?** — **Decision: Yes.** The user API already returns the raw metrics (carbon savings grams, cost savings USD, quality time minutes) needed to compute these.

- **Should we keep the user profile header (name, location, description, background image)?** — **Decision: Yes, separate header.** The shared content widget covers the content below the hero and tab bar only. Each screen owns its own hero section — user screen shows user identity (name, location, description, background image), portfolio has its own hero.

- **What about the settings icon for own profile?** — **Decision: Keep it.** The user screen retains its own AppBar with back button and conditional settings icon. Only the content body is shared.

## Implementation Plan

### Phase 1: Extract shared content widget

**Goal:** Create a reusable widget that renders the metrics content body (everything below the tab bar), used by both screens.

**Tasks:**
- [ ] Create `ImpactMetricsContentData` as a **Freezed** presentation model in `app/lib/presentation/models/impact_metrics_content_data.dart`. This is a UI-layer data class (not a viewmodel) — it carries pre-formatted display values and optional sections. Include all fields needed to render: stat pills (as `List<StatPill>`), comparison card values (cost USD, CO2 kg, QT minutes), chart data points, trend text, direction breakdown (optional), source breakdown, communities ranking (optional), sufficiency ring weekly minutes (optional). Use `@Default` with zero/empty values so all sections always render (no hidden sections — show empty/zero state when data is unavailable). <!-- AUDIT: Freezed required per architecture.md §2; placed in presentation/models/ per directory structure. -->
- [ ] Create `ImpactMetricsContent` widget in `app/lib/presentation/widgets/impact/` that renders the full content body: stat pills → comparison cards → chart → sufficiency ring → trend card → direction breakdown → source breakdown → communities section. Accept `ImpactMetricsContentData`, `selectedMetric`, and **navigation callbacks** as nullable `VoidCallback?` parameters (e.g., `onHeroTap`, `onStatPillTap(int)`, `onDirectionTap(PortfolioPerspective)`, `onSourceTap(String)`). When callbacks are null, elements render without tap handlers (read-only). The widget must not access any providers or repositories — it is purely presentational. <!-- AUDIT: Widgets must not access repositories (architecture.md §1). Using callback params instead of readOnly flag avoids provider coupling. -->
- [ ] **Metric selection state**: The shared widget accepts `selectedMetric` and an `onMetricSelected` callback as parameters. The *caller* owns the metric selection state — portfolio screen delegates to its viewmodel, user screen uses local `setState`. The shared widget does not manage this state internally. <!-- AUDIT: Added — the two screens use different state management for tab selection. Making this explicit avoids a design ambiguity. -->
- [ ] Add adapter method to `UserImpactData` (or a standalone function) that converts `UserImpactData` → `ImpactMetricsContentData`, mapping available fields and leaving optional sections null.
- [ ] Add adapter method for portfolio that converts `PortfolioMetricsState` + `PortfolioMetricDetailState` → `ImpactMetricsContentData`.
- [ ] Ensure all user-visible strings in the shared widget use `context.l10n.*` from ARB files — no hardcoded strings. Reuse existing `impact*` and `portfolio*` i18n keys where possible; add new keys to both `app_en.arb` and `app_es.arb` if needed. <!-- AUDIT: i18n compliance per i18n.md — all user-visible strings must go through ARB. -->

**Validates:** The unified data model can represent both portfolio and user data without loss.

### Phase 2: Integrate into both screens

**Goal:** Replace duplicated content rendering code in both screens with the shared widget.

**Tasks:**
- [ ] Refactor `PortfolioMetricsScreen._buildActiveContent` to use `ImpactMetricsContent`, passing navigation callbacks that use `NavigationHelpers.pushScreen()` to open detail screens. <!-- AUDIT: Navigation must use NavigationHelpers per architecture.md Navigation section. -->
- [ ] Refactor `UserMetricsScreen._buildActiveContent` to use `ImpactMetricsContent` with all navigation callbacks set to `null` (read-only).
- [ ] Fix the hero section in user screen — remove floating chips artifact, ensure clean layout.
- [ ] Verify user screen shows comparison cards (forest/meal/walk) using data from `UserImpactData`.
- [ ] Add unit tests for data adapter methods (both portfolio and user adapters). Test with `ProviderContainer` for viewmodel-level tests. <!-- AUDIT: Test pattern per architecture.md §Testing. -->
- [ ] Verify portfolio screen behavior is unchanged (regression test).

**Validates:** Both screens render correctly, portfolio has full interactivity, user screen is read-only.

### Phase 3: Enrich user screen data (follow-up)

**Goal:** Incrementally add portfolio-exclusive sections to the user screen by extending the server API.

**Tasks:**
- [ ] Extend `GetUserImpactMetricsResponse` with direction breakdown fields (optional).
- [ ] Extend `GetUserImpactMetricsResponse` with CO2 and quality time trend data (already defined in proto fields 11-12 but may not be populated by server).
- [ ] Server-side: populate `co2_trend` and `quality_time_trend` fields in the impact service handler. Run `npm run build` to verify Go build passes. <!-- AUDIT: Go changes require npm run build verification per CLAUDE.md. -->
- [ ] Update user data adapter to pass through new fields to `ImpactMetricsContentData`.
- [ ] Verify new sections appear on user screen when data is available.

**Validates:** User screen shows progressively richer data as the API expands, without UI code changes.

## Audit Notes

_Audited against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`._

**Changes made:**

1. **Freezed requirement** — `ImpactMetricsContentData` must use `@freezed` per architecture.md §2 (Riverpod 3.0 Notifier Pattern). Updated Phase 1 task.
2. **File placement** — Moved `ImpactMetricsContentData` to `app/lib/presentation/models/` per the directory structure in architecture.md. It's a presentation-layer data class, not a widget.
3. **No provider access in shared widget** — architecture.md §1 ("Screens only interact with ViewModels") and the general principle that widgets are presentation-only. Changed from `readOnly` boolean flag to nullable callback parameters. This keeps the shared widget purely presentational with zero Riverpod coupling.
4. **Metric selection state ownership** — Added explicit task: the shared widget receives `selectedMetric` + `onMetricSelected` callback. Portfolio delegates to its viewmodel; user screen uses local `setState`. This was an ambiguity in the original plan since the two screens manage this state differently.
5. **i18n compliance** — Added Phase 1 task requiring all strings to use `context.l10n.*` per i18n.md. Existing `impact*`/`portfolio*` keys should be reused.
6. **Navigation pattern** — Phase 2 callbacks must use `NavigationHelpers.pushScreen()` per architecture.md Navigation section.
7. **Go build verification** — Phase 3 server changes require `npm run build` per CLAUDE.md.

**No caching issues found** — the plan doesn't introduce new caching; data flows through existing repositories (`impactMetricsRepositoryProvider`, `portfolioRepositoryProvider`). No manual cache maps are created in viewmodels.

**No server architecture issues found** — Phase 3's proto extension is additive (optional fields) and doesn't change service boundaries.

## Related Issues

- No directly related issues found.
