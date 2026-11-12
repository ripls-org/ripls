# Portfolio Screen

## Problems

### 1. Daily/weekly metrics are scoped to a single community
The daily screen (`daily_screen.dart`) calls `GetDailyView` with a single `community_id`. Users who belong to multiple communities see only one community's weekly metrics (quality time, cost saved, time recovered, CO₂ avoided) at a time. There is no way to see aggregate personal impact across all communities.

### 2. No personal "home base" outside a community context
Everything in the app today is community-scoped — Feed, Discover, Metrics, Daily all require a selected community. There is no screen where users can see their own cross-community picture: all their stuff, their aggregate impact, and their consolidated daily rhythm.

### 3. Impact metrics are either community-level or per-item
The existing `CommunityMetricsScreen` shows community aggregates. The `UserMetricsScreen` exists but is accessed from a community member list (viewing another user's contributions). There is no first-class "my portfolio" metrics view showing:
- How much I've saved myself
- How much I've saved others
- How much my communities have saved in total

### 4. Calendar/weekly tab is redundant at community level
With a portfolio-level daily/weekly screen that aggregates all communities, the per-community Daily tab (index 3 in `home_screen.dart`) becomes redundant. Keeping it creates confusion about which weekly metrics a user should trust.

### 5. Sidebar "Your portfolio" card is a dead end
The sidebar redesign (see `tmp/portfolio/sidebar.md`) added a user card with "Your portfolio" subtitle, but tapping it is currently a no-op. Users see the affordance but can't act on it.

## Potential Solutions

### Option A: Portfolio as a new top-level screen (separate from HomeScreen)
Tapping the sidebar user card navigates to a brand-new `PortfolioScreen` that has its own `Scaffold`, bottom nav, and `IndexedStack` — independent of `HomeScreen`. The portfolio has three tabs (Daily, Items, Metrics). Navigating back returns to the community `HomeScreen`.

**Pros:** Clean separation, no interference with community screens, simple mental model.
**Cons:** Two parallel screen shells with similar structure (duplicate nav logic, floating header, scroll-to-top).

### Option B: Portfolio as a mode of HomeScreen
Reuse `HomeScreen` by adding a "portfolio mode" flag. When in portfolio mode, the bottom nav tabs switch to (Daily, Items, Metrics), the top bar shows the user's name, and data providers read "all communities" instead of `selectedCommunity`. Tapping a community in the sidebar exits portfolio mode.

**Pros:** Maximum code reuse for nav, header, sidebar.
**Cons:** Complex modal state in `HomeNotifier`, risk of bugs when toggling modes, conditionals scattered throughout.

### Option C: Portfolio reuses HomeScreen's shell with different tab configuration
Create a `PortfolioScreen` that composes the same building blocks as `HomeScreen` (floating header, animated bottom nav, sidebar) but with its own tab list and screen stack. The sidebar is shared via a provider. Navigation between community and portfolio is a top-level route push, not a mode toggle.

**Pros:** Reuses UI components without coupling state, clean navigation stack, sidebar stays consistent.
**Cons:** Some duplication of shell layout code (but small — mostly widget composition).

## Recommendation

**Option A** — Portfolio as a separate top-level screen. Reasoning:

1. The portfolio has fundamentally different data requirements (cross-community aggregation) from the community home (single community). Mixing these into one stateful widget creates unnecessary complexity.
2. The navigation model is simple: sidebar user card tap → push `PortfolioScreen`; sidebar community tap → pop back to `HomeScreen` (or push if already on portfolio). Back button always works.
3. The bottom nav widget and floating header are already extracted as reusable widgets. `PortfolioScreen` can compose them without duplicating logic.
4. This is the same pattern used by the reference sidebar JSX — the user card navigates to a distinct "user context."

To reduce duplication, extract any shared nav/header components into reusable widgets if not already extracted.

## Decisions

1. **Aggregate inbox view API** — Create a new `GetPortfolioInboxView` RPC that aggregates across all user communities. The existing `GetDailyView` RPC (community-scoped) will be removed since the inbox is now portfolio-level only. The "Daily" name was already outdated — the data is not daily-specific.

2. **Portfolio metrics scope** — Create new dedicated RPCs. The existing `GetUserImpactMetrics` covers "saved others" (impact of my sharing). A new `GetPortfolioMetrics` RPC will return three perspectives:
   - `saved_self` — impact on transactions where user is the *recipient* (borrowed, received help, attended)
   - `saved_others` — existing user impact calculation (impact of user's sharing on others)
   - `communities_total` — sum of impact across all user's communities

3. **Daily tab removal from community view** — Remove the Daily/Inbox tab from community bottom nav. Keep the Add button working exactly the same way it does now.

4. **Navigation between portfolio and community** — Stay in portfolio context. Push detail screens onto the portfolio navigator. The user can explicitly switch to a community via the sidebar.

5. **Profile items screen reuse** — Use `ProfileItemsScreen` as-is for now. Modifications deferred to a future PR.

## Architecture Audit

**Status: Issues found and fixed** — 6 violations corrected in the implementation plan below.

| Check | Result | Notes |
|-------|--------|-------|
| **Screens only interact with ViewModels** | Pass | Portfolio screens will use dedicated providers/view models |
| **ViewModels only call Repositories** | **FIXED** | Phases 4 and 5 originally had ViewModels calling services directly. Fixed: both go through repositories. |
| **Repositories are the ONLY layer that calls Services** | **FIXED** | Added `PortfolioRepository` (Phase 4) with inbox + metrics methods as required data layer. |
| **No manual cache maps** | Pass | All caching through repositories with CacheService namespaces |
| **Navigation helpers** | **FIXED** | Phase 3 originally used raw `Navigator.push()`. Fixed: uses `NavigationHelpers.pushWithSlide()` per architecture docs. |
| **Freezed state classes** | **FIXED** | Phases 4 and 5 now require Freezed state for all new ViewModels. |
| **Cache key naming convention** | **FIXED** | Phase 4 now specifies namespace and key patterns per caching.md. |
| **Cache invalidation mechanism** | **FIXED** | Phase 4 now specifies `dailyCacheInvalidationProvider` pattern per caching.md Pattern 7. |
| **Server boundary** | Pass | Proto changes follow API/models separation; new RPC has dedicated request/response types. |
| **Service independence** | Pass | `GetPortfolioMetrics` uses calculator library (shared lib), not other services. |
| **SQL efficiency** | Pass | Phase 1 now specifies batch query strategy to avoid N+1. |
| **autoDispose + ref.mounted** | Pass | Phase 7 tests include disposal safety tests per architecture.md. |

## Implementation Plan

### Phase 1 — Proto & server: portfolio inbox RPC (replaces DailyService)
**Files:** `proto/ripls/api/portfolio.proto` (new), `proto/ripls/api/daily.proto` (delete), `server/services/portfolio/` (new), `server/services/daily/` (delete)

- Create new `proto/ripls/api/portfolio.proto` with `PortfolioService` containing `GetPortfolioInboxView` RPC
- `GetPortfolioInboxViewRequest`: only `string timezone = 1` (no community_id — always aggregates all user communities, user ID from auth context)
- `GetPortfolioInboxViewResponse`: same shape as current `GetDailyViewResponse` but with `optional string community_name` on each item so the UI can show which community an item belongs to
- Delete `proto/ripls/api/daily.proto` and `server/services/daily/` — the community-scoped daily view is being removed entirely
- Create `server/services/portfolio/` with the inbox logic migrated from `server/services/daily/`:
  - Query user's communities via batch `QueryByField(ctx, "user_id", userID, &models.CommunityUser{})` — single query
  - Collect items across all communities using batch queries with `community_id IN (...)` rather than looping per community (SQL efficiency: never query in a loop)
  - Merge items across communities, deduplicating by conversation ID
  - Sum weekly metrics across communities (quality time, cost, time, CO₂ with quadrature propagation via `SumEstimates`)
  - Compute `active_days` as union across communities (OR each day's bool)
- Unregister `DailyService` from `server/main.go`, register new `PortfolioService`
- Unit tests for multi-community aggregation, including `storage.AssertMaxQueries` to prevent N+1 regression

### Phase 2 — Proto & server: portfolio metrics RPC
**Files:** `proto/ripls/api/portfolio.proto`, `server/services/portfolio/metrics.go` (new), `server/impact_metrics/calculator.go`

- Add `GetPortfolioMetrics` RPC to `PortfolioService` (same proto/service package as Phase 1)
- `GetPortfolioMetricsRequest`: no params (uses auth context for user ID)
- `GetPortfolioMetricsResponse`: three dedicated sections with own request/response types per CLAUDE.md conventions:
  - `saved_self` — impact on transactions where user is recipient (borrowed, received help, attended)
  - `saved_others` — existing `CalculateUserImpactSavings` (impact of user's sharing)
  - `communities_total` — sum of `CalculateImpactSavings` across user's communities
- Add `CalculateUserReceivedImpact(ctx, userID)` to calculator library (shared lib, not another service)
- Activity counts per section (items borrowed, items lent, community totals)
- Unit tests

### Phase 3 — Client: PortfolioScreen shell & navigation
**Files:** `app/lib/presentation/screens/portfolio/portfolio_screen.dart`, `app/lib/presentation/viewmodels/portfolio_view_model.dart`, `sidebar.dart`

- Create `PortfolioScreen` with `Scaffold`, `IndexedStack`, animated bottom nav (3 tabs)
- Tab 0: Calendar icon → `PortfolioInboxScreen` (reuses daily/inbox widgets)
- Tab 1: Grid icon → `ProfileItemsScreen` (existing, embedded)
- Tab 2: Chart icon → `PortfolioMetricsScreen` (new)
- **Persistent `FloatingHeader` across all three tabs** — rendered once in `PortfolioScreen` above the `IndexedStack`, not inside each child screen. Uses `FloatingHeader(title: userName, showMenuIcon: true)`. The hamburger menu tap calls `toggleSidebar()` to open the sidebar on any tab, allowing the user to navigate to communities from anywhere in the portfolio.
- Each child screen receives `topPadding: FloatingHeader.contentTop(context)` so content starts below the header (same pattern as `CommunityMetricsScreen.topPadding`)
- **[AUDIT FIX: Navigation]** Wire sidebar user card tap → `NavigationHelpers.pushWithSlide()` to `PortfolioScreen` (never raw `Navigator.push(MaterialPageRoute(...))` — per architecture docs, raw push animates both screens and flashes background)
- Wire sidebar community tap → pop portfolio if on stack, select community
- Create `portfolioNavigatorKeyProvider` in a new `portfolio_view_model.dart` (not in `home_view_model.dart` — portfolio is a separate screen context with its own providers)

### Phase 4 — Client: portfolio inbox screen
**Files:** `app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart`, `app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart`, `app/lib/data/repositories/portfolio_repository.dart` (new), `app/lib/services/portfolio_service.dart` (new)

- Create `PortfolioService` client wrapping the new `PortfolioService` Connect-Go service (calls `GetPortfolioInboxView`)
- **[AUDIT FIX: Repository pattern]** Create `PortfolioRepository` with `getInboxView(timezone)` method that calls `PortfolioService.getPortfolioInboxView()`. ViewModel must call repository, never the service directly (architecture rule: "Repositories are the ONLY layer that calls Services").
- **[AUDIT FIX: Cache key naming]** Cache key: `'portfolio:inbox'` in the `'portfolio'` namespace (follows `'namespace:identifier'` convention from caching.md)
- **[AUDIT FIX: Freezed state]** Create `PortfolioInboxState` as a `@freezed` class with `response`, `isLoading`, `errorMessage`, `unreadDeltas` fields. All state updates via `copyWith()` per architecture.md.
- Create `PortfolioInboxNotifier extends Notifier<PortfolioInboxState>` that reads `portfolioRepositoryProvider` via `ref.read()`
- Reuse existing daily widgets (`WeeklyHeader`, `WeeklyDayCard`, `WeeklyMetricsGrid`, `DailyItemRow`, etc.)
- Add community name badge/label to each `DailyItemRow` (uses new `community_name` field)
- Handle empty state (no communities)
- Delete `app/lib/services/daily_service.dart`, `app/lib/data/repositories/daily_repository.dart`, `app/lib/presentation/viewmodels/daily_view_model.dart`, `app/lib/presentation/screens/daily/daily_screen.dart` and related daily widgets (replaced by portfolio inbox)
- **[AUDIT FIX: Cache invalidation]** Follow caching.md Pattern 7 (Event-Driven Cache Invalidation): listen to `portfolioCacheInvalidationProvider` (new, replaces `dailyCacheInvalidationProvider`) in `build()`, debounce 300ms, cooldown 5s, `_refreshInPlace()` without showing loading spinner. Defer refresh when portfolio inbox tab is not active via `_hasPendingInvalidation` flag. Update all mutation repositories to notify this new provider instead of `dailyCacheInvalidationProvider`.

### Phase 5 — Client: portfolio metrics screen
**Files:** `app/lib/presentation/screens/portfolio/portfolio_metrics_screen.dart`, `app/lib/presentation/viewmodels/portfolio_metrics_view_model.dart`, `app/lib/data/repositories/portfolio_repository.dart`

- **[AUDIT FIX: Repository pattern]** Add `getPortfolioMetrics()` method to `PortfolioRepository` (created in Phase 4, calls `PortfolioService.getPortfolioMetrics()`). ViewModel reads from repository, never from service directly. Cache key: `'portfolio:metrics'` in the `'portfolio'` namespace.
- **[AUDIT FIX: Freezed state]** Create `PortfolioMetricsState` as a `@freezed` class with `savedSelf`, `savedOthers`, `communitiesTotal`, `selectedMetric`, `selectedSection`, `isLoading`, `errorMessage` fields. Use `AsyncNotifier` pattern (preferred per architecture.md) so `build()` handles lifecycle automatically.
- Create `PortfolioMetricsNotifier extends AsyncNotifier<PortfolioMetricsState>` that reads `portfolioRepositoryProvider`
- **Match `CommunityMetricsScreen` design pattern** — same visual structure:
  1. **`ImpactMetricsHeroSection`** (dark gradient background) with user profile image, user name, stat chips (total items, total loans, total communities), `extendToTop: true`, `customTopPadding` from `PortfolioScreen`
  2. **`ImpactTabBar`** (four metric tabs: Money Saved, CO₂ Avoided, Quality Time, Time Recovered) — reuse existing widget. Each tab shows the aggregated value; tapping selects the metric dimension
  3. **Content area** (warm white `Color(0xFFF8F5EE)` background) — changes based on selected tab, showing three collapsible perspective sections:
     - **What I've saved** (saved_self) — money not spent, time recovered, CO₂ avoided by borrowing/receiving
     - **What I've given** (saved_others) — impact of my sharing on others
     - **My communities' total impact** (communities_total) — combined community metrics
- Reuse shared impact widgets: `ImpactHeroValue`, `ImpactHook`, `BreakdownRow`, `InsightCard`, `StatPillRow`
- Each section tappable for audit trail detail (reuse existing audit trail screens)
- Use `ref.watch(portfolioMetricsProvider).when(data:, loading:, error:)` in the screen widget

### Phase 6 — Remove daily tab from community view
**Files:** `app/lib/presentation/screens/home/home_screen.dart`, `app/lib/presentation/viewmodels/home_view_model.dart`

**Note:** This phase has high blast radius — changing tab indices affects navigation throughout the app. Grep for `selectedIndex`, `bottomNavIndex`, and hardcoded tab index values before changing.

- Remove Daily tab (index 3) from community `HomeScreen` bottom nav
- Update tab indices: Feed (0), Discover (1), Metrics (2), Add (center button — keep working exactly as it does now)
- Remove `dailyNavigatorKeyProvider`, `dailyScrollControllerProvider` from community context
- Update badge wiring (unread counts move to portfolio inbox)
- Update `_logScreenViewEvent` tab names
- Grep for all references to old tab indices (especially hardcoded `3` for Daily, `2` for Metrics) and update
- Update conversation notification routing: currently routes to daily tab via `dailyNavigatorKeyProvider` — must now route to portfolio context

### Phase 7 — Polish & testing
- Flutter widget tests for `PortfolioScreen` tab switching
- Flutter tests for `PortfolioInboxNotifier` (multi-community merge, loading states, error handling)
- Flutter tests for `PortfolioMetricsNotifier` (three-section display, loading states, error handling)
- **Disposal safety tests** for both new Notifiers (required per architecture.md — start async, dispose mid-flight, verify no crash)
- Server integration tests for `GetPortfolioInboxView` (multi-community, single community, no communities)
- Server unit tests with `storage.AssertMaxQueries` to verify N+1 prevention
- Verify sidebar navigation flow (portfolio ↔ community switching)
- Verify deep links and conversation notification routing
- Verify all old `DailyService`/`daily_*` references are removed (dead code cleanup)
- Run `npm run build` and `flutter test --reporter expanded`

### Phase 8 — Persistent header, sidebar integration, and metrics screen polish
**Files:** `app/lib/presentation/screens/portfolio/portfolio_screen.dart`, `app/lib/presentation/screens/portfolio/portfolio_metrics_screen.dart`, `app/lib/presentation/widgets/floating_header.dart`, `sidebar.dart`

This phase ensures the floating header and sidebar work consistently across all portfolio tabs, and that the metrics screen matches the community metrics screen's polish level.

**Persistent floating header:**
- Verify `FloatingHeader` renders above `IndexedStack` in `PortfolioScreen` and persists across all tab switches (inbox, items, metrics) without rebuilding
- Header shows user name (from `authStateProvider`) and hamburger menu icon
- Hamburger menu tap opens sidebar overlay with slide-in animation (same behavior as community `HomeScreen`)
- Sidebar shows full community list — tapping a community pops portfolio and navigates to `HomeScreen` with that community selected
- Sidebar user card shows active highlight state when portfolio is the current context
- Hide/show header on scroll (same scroll-direction listener pattern as `HomeScreen`: `ScrollUpdateNotification` → `hideNav()`/`showNav()`)

**Metrics screen visual parity with `CommunityMetricsScreen`:**
- Hero section scrolls under the floating header (`extendToTop: true` + `customTopPadding`)
- `ImpactTabBar` sits immediately below the hero, pinned behavior matches community version
- Content area uses the same warm white background (`Color(0xFFF8F5EE)`)
- Pull-to-refresh via `RefreshIndicator` wrapping `CustomScrollView`
- Tab selection state persists across refreshes (stored in `PortfolioMetricsState.selectedMetric`)
- Each metric tab's content shows the three perspective sections (saved_self, saved_others, communities_total) with values appropriate to the selected dimension (e.g., money tab shows dollar amounts for all three perspectives)

**Items screen header integration:**
- `ProfileItemsScreen` receives `topPadding` param so content starts below the persistent floating header
- Scroll-to-top on re-tap of bottom nav tab (wire `scrollController` through `PortfolioScreen`)

**Testing:**
- Verify header visibility on all three tabs
- Verify sidebar opens from each tab and community navigation works
- Verify metrics screen tab selection, hero image, and pull-to-refresh
- Verify scroll-to-top on bottom nav re-tap for each tab

### Phase 9 — UX refinements (not yet implemented)

**Sidebar & community deselection:**
- When tapping "My Portfolio" in the sidebar, clear the selected community (`selectedCommunityProvider.notifier.clearSelection()` or equivalent). The portfolio is a distinct context — no community should appear selected in the sidebar while viewing the portfolio.

**Inbox screen changes:**
- Remove the "XXX shared value" display from the inbox weekly header — this information is now visible in the metrics tab, so it's redundant here.
- Replace the existing weekly metrics layout with a **horizontal carousel** (single row, horizontally scrollable) containing all metric chips: actions taken, items shared, quality time, cost saved, time recovered, CO₂ avoided. Each chip is a compact card showing the metric value and label. Use `ListView(scrollDirection: Axis.horizontal)` with fixed-height row.

**Items screen (`ProfileItemsScreen`) changes:**
- Remove the existing "My Items" header bar at the top of the screen — the persistent `FloatingHeader` from `PortfolioScreen` already provides the screen title context.
- Move the **search icon** and **edit/select icon** into the `FloatingHeader` trailing slot. When on the Items tab, the floating header should show these action icons on the right side.
- Search: tapping the search icon in the header toggles an inline search field (expanding into the header row, or a search bar that slides down below the header).
- Edit/select mode: tapping the edit icon in the header activates selection mode in `ProfileItemsScreen` (same bulk-edit functionality as today, just triggered from the header instead of the old screen-level header).
- `PortfolioScreen` passes the current tab index to `FloatingHeader` so it can conditionally render the correct trailing actions per tab (e.g., search+edit on Items tab only).

**Metrics screen (`PortfolioMetricsScreen`) changes:**
- Remove the user's name from the hero section — it already appears in the persistent floating header above.
- Replace the hero name area with **three filter chips**: `Full Impact` | `Given` | `Received`. These toggle the perspective:
  - **Full Impact** — total community impact across all user's communities (communities_total from `GetPortfolioMetrics`)
  - **Given** — impact of what the user has shared with others (saved_others)
  - **Received** — impact of what the user has received from others (saved_self)
- The selected chip filters all content below: the hero value, the `ImpactTabBar` values, and the content area all update to reflect the selected perspective.
- Adopt the **exact same metrics display format as `CommunityMetricsScreen`**: charts (`ImpactChartCard`), breakdowns by source type (loans, giveaways, experiences, requests via `BreakdownRow`), trend cards (`TrendCard`), insight cards (`InsightCard`), scenario projections (`ScenarioCard`), and the full stage-adaptive layout.
- Each metric card and breakdown row is tappable for **drill-down** into `CommunityMetricDetailScreen` / audit trail detail (reuse existing detail screens, passing the portfolio-level data instead of a single community's data).
- The `GetPortfolioMetrics` response may need to include time series, source breakdowns, and contributing factors (same shape as `GetCommunityMetricDetailResponse`) to support the chart and breakdown views. Evaluate whether to extend `GetPortfolioMetrics` or add a `GetPortfolioMetricDetail` RPC.

### Phase 10 — Full metrics parity with CommunityMetricsScreen (not yet implemented)

The portfolio metrics screen must be a visual clone of `CommunityMetricsScreen` — identical sections, charts, breakdowns, and drill-downs — but with data filtered by the perspective chips (Full Impact / Given / Received) and the user's profile image as the background.

**Background image:**
- Use the user's profile image (from `authStateProvider` → user media ID → `MediaRepository.getMediaUrl()`) as the `ImpactMetricsHeroSection` background image, instead of a community image.

**Perspective filter chips** (replaces user name in hero):
- `Full Impact` — communities_total (sum of all user's communities' impact)
- `Given` — saved_others (impact of user's sharing on others)
- `Received` — saved_self (impact of items user received/borrowed)
- Selected chip determines all data below. Default: `Full Impact`.

**Server support — `GetPortfolioMetricDetail` RPC:**
- Add `GetPortfolioMetricDetail` to `PortfolioService` (same pattern as `GetCommunityMetricDetail`)
- Request: `ImpactMetricDimension dimension`, `ImpactMetricPeriod period`, `PortfolioPerspective perspective` (enum: FULL_IMPACT, GIVEN, RECEIVED)
- Response: same shape as `GetCommunityMetricDetailResponse` — `total_value`, `formatted_total`, `cumulative_trend[]`, `source_breakdown[]`, `factors[]`, `top_items[]`, `top_contributors[]`, dimension-specific detail fields
- Server computes time series and breakdowns scoped to the selected perspective

**Per-tab sections (exact parity with `CommunityMetricsScreen`):**

The community screen has 5 stages (New, Listed, FirstTransaction, Active, Mature). The portfolio screen skips New/Listed (user always has data from at least one community) and uses a simplified stage model based on total completed transactions across all communities.

For each of the four metric tabs (Money, CO₂, Quality Time, Time), the content area shows:

**Money Saved tab (`IMPACT_METRIC_DIMENSION_MONEY`):**
| Section | Widget | Community version | Portfolio adaptation |
|---------|--------|-------------------|---------------------|
| Hero value | `ImpactHeroValue` | Community cost savings formatted | Perspective-filtered total |
| Hook text | `ImpactHook` | Community-specific hook | Portfolio-specific hook ("Your sharing has saved..." / "You've saved yourself..." / "Your communities have saved...") |
| Stat pills | `StatPillRow` | Library value, Utilization %, Actions count | Total items across communities, Utilization %, Total actions |
| Cumulative chart | `ImpactChartCard` (line) | "SAVINGS OVER TIME" from `GetCommunityMetricDetail` | Same chart from `GetPortfolioMetricDetail` |
| Trend card | `TrendCard` | "X loans completed across Y members..." | "X loans completed across Y communities..." |
| Breakdown by type | `BreakdownRow` | Loans (count, value) → tap drill-down | Loans (count, value) → tap drill-down |
| | `BreakdownRow` | Giveaways (count, value) → tap drill-down | Giveaways (count, value) → tap drill-down |
| | `BreakdownRow` | Requests (count, value) → tap drill-down | Requests (count, value) → tap drill-down |
| | `BreakdownRow` | Events (count, value) → tap drill-down | Events (count, value) → tap drill-down |
| Research insight | `InsightCard` | Citation from `ImpactCitations` | Same citations (reuse) |

**CO₂ Avoided tab (`IMPACT_METRIC_DIMENSION_EMISSIONS`):**
| Section | Widget | Community version | Portfolio adaptation |
|---------|--------|-------------------|---------------------|
| Hero value | `ImpactHeroValue` | Carbon savings formatted | Perspective-filtered total |
| Stat pills | `StatPillRow` | Total CO₂, Trees/yr equivalent, Cars equivalent | Same metrics aggregated across communities |
| Monthly chart | `ImpactChartCard` (bar) | "MONTHLY CO₂ AVOIDED" from detail | Same chart from `GetPortfolioMetricDetail` |
| QT sufficiency ring | — | Not shown for CO₂ | Not shown |
| Trend card | `TrendCard` | "X kg CO₂ avoided — equivalent to Y trees..." | Same, aggregated |
| Breakdown by source | `BreakdownRow` | Manufacture avoided (items shared, value) | Same, aggregated |
| | `BreakdownRow` | Waste avoided (end-of-life emissions, value) | Same, aggregated |
| Research insight | `InsightCard` | CO₂ citation | Same citation |

**Quality Time tab (`IMPACT_METRIC_DIMENSION_QUALITY_TIME`):**
| Section | Widget | Community version | Portfolio adaptation |
|---------|--------|-------------------|---------------------|
| Hero value | `ImpactHeroValue` | Quality time minutes formatted | Perspective-filtered total |
| Stat pills | `StatPillRow` | Quality time total, Per person, Members count | Quality time total, Per community avg, Communities count |
| Cumulative chart | `ImpactChartCard` (line) | "QUALITY TIME OVER TIME" from detail | Same chart from `GetPortfolioMetricDetail` |
| Sufficiency ring | `SufficiencyRing` | Weekly minutes / 52 | Same, aggregated weekly minutes |
| Trend card | `TrendCard` | "X interactions across Y members..." | "X interactions across Y communities..." |
| Breakdown by type | `BreakdownRow` | Loans (lending interactions, value) | Loans (lending interactions, value) |
| | `BreakdownRow` | Giveaways (completed, value) | Giveaways (completed, value) |
| | `BreakdownRow` | Requests (fulfilled, value) | Requests (fulfilled, value) |
| | `BreakdownRow` | Events (gatherings, value) | Events (gatherings, value) |
| Research insight | `InsightCard` | QT citation | Same citation |

**Time Recovered tab (`IMPACT_METRIC_DIMENSION_TIME`):**
| Section | Widget | Community version | Portfolio adaptation |
|---------|--------|-------------------|---------------------|
| Hero value | `ImpactHeroValue` | Time banked formatted | Perspective-filtered total |
| Stat pills | `StatPillRow` | Total time, Per person, Avg per loan | Total time, Per community avg, Avg per loan |
| Cumulative chart | `ImpactChartCard` (line) | "TIME RECOVERED OVER TIME" from detail | Same chart from `GetPortfolioMetricDetail` |
| Trend card | `TrendCard` | "X hours from Y loans..." | Same, aggregated |
| Breakdown by type | `BreakdownRow` | Loans (count, value) | Loans (count, value) |
| | `BreakdownRow` | Requests (fulfilled, value) | Requests (fulfilled, value) |
| Research insight | `InsightCard` | Time citation | Same citation |

**Drill-down behavior:**
- Tapping any `BreakdownRow` pushes `MetricDetailScreen` with portfolio-level data (same screen, different data source)
- Tapping stat pills with `onTap` pushes `CommunityMetricDetailScreen` or equivalent portfolio detail
- All drill-down screens reuse existing widgets; only the data provider changes

**Milestone cards:**
- Skip milestone cards in portfolio view (milestones are community-specific). May add portfolio-level milestones in a future phase.

**Widget reuse strategy:**
- All widgets (`ImpactMetricsHeroSection`, `ImpactTabBar`, `ImpactHeroValue`, `ImpactHook`, `StatPillRow`, `ImpactChartCard`, `TrendCard`, `BreakdownRow`, `SufficiencyRing`, `InsightCard`) are already parameterized and reusable. No widget changes needed — only different data passed in.
- The `_buildActiveOrMatureContent` layout pattern from `CommunityMetricsScreen` should be extracted into a shared builder or duplicated in `PortfolioMetricsScreen` with portfolio-specific data bindings.

### Phase 11: Auto-Hide Header and Footer on Scroll/Tap

**Goal:** The portfolio's FloatingHeader and bottom nav bar should auto-hide when the user scrolls down or taps the screen, and reappear when scrolling up or tapping again — matching the existing feed screen behavior.

**Existing pattern (HomeNotifier in `home_view_model.dart`):**
- `isNavVisible` boolean state field (defaults to `true`)
- `showNav()` — sets `isNavVisible = true`
- `hideNav()` — sets `isNavVisible = false`
- `toggleNav()` — flips `isNavVisible`
- Feed screen uses `GestureDetector(onTap: toggleNav)` for tap toggling
- `CommunityMetricsScreen` uses `NotificationListener<ScrollUpdateNotification>` with scroll delta thresholds: `delta > 10` → `hideNav()`, `delta < -10` → `showNav()`

**Changes:**

1. **`PortfolioViewModel` (`portfolio_view_model.dart`)**
   - Add `isNavVisible` field to the Freezed state class (default `true`)
   - Add `showNav()`, `hideNav()`, `toggleNav()` methods mirroring `HomeNotifier`

2. **`PortfolioScreen` (`portfolio_screen.dart`)**
   - Wrap the `IndexedStack` (or equivalent tab body) in a `GestureDetector` with `onTap: ref.read(portfolioViewModelProvider.notifier).toggleNav`
   - Wrap in `NotificationListener<ScrollUpdateNotification>` that calls `hideNav()` when `notification.scrollDelta > 10` and `showNav()` when `notification.scrollDelta < -10`
   - Animate the `FloatingHeader` in/out using an `AnimatedSlide` or `AnimatedPositioned` driven by `isNavVisible` (slide up to hide, slide down to show)
   - Animate the bottom `NavigationBar` similarly (slide down to hide, slide up to show)
   - Use the same animation duration and curve as the feed screen for consistency

3. **Each tab's scroll controller**
   - The `NotificationListener` captures scroll events from whichever tab is active (Inbox, Items, Metrics) since all are children of the listener
   - No per-tab scroll controller wiring needed — `NotificationListener` bubbles from any scrollable descendant

**Files modified:**
- `app/lib/presentation/viewmodels/portfolio_view_model.dart` — add nav visibility state + methods
- `app/lib/presentation/screens/portfolio/portfolio_screen.dart` — add GestureDetector, NotificationListener, animated header/footer

### Phase 12: Portfolio Metrics — Direction Breakdown and Full Activity Breakdown

**Goal:** Remove the Full Impact / Given / Received toggle chips from the portfolio metrics screen. The default (and only) view is full impact. Instead, add a **direction breakdown table** (Given, Received) above the existing **activity breakdown table** (Loans, Giveaways, Events, Requests). All rows and hero metrics are tappable to open metric detail screens, mirroring `CommunityMetricsScreen`.

**Existing pattern (`CommunityMetricsScreen` → `_buildBreakdownSection`):**
- Each tab builds a list of `BreakdownRow` widgets with `label`, `count`, `value`, and `onTap`
- `onTap` calls `MetricDetailScreen.push(context, communityId:, communityName:, dimension:, category:)` via `NavigationHelpers.pushWithSlide()`
- `sourceBreakdown` values come from `GetCommunityMetricDetail` RPC response (`detail.sourceBreakdown`)
- Rows are separated by `Divider(height: 1, color: Color(0xFFE4DFD4))`
- Community version already has rows for Loans, Giveaways, Requests, and Events per tab

**Changes:**

1. **Remove perspective toggle chips**
   - Delete the Full Impact / Given / Received chip bar from `PortfolioMetricsScreen`
   - Remove `perspective` field from `PortfolioMetricsState` (no longer needed)
   - The screen always shows full (aggregated) impact — no filter parameter sent to RPCs

2. **Add direction breakdown table (new, above activity breakdown)**
   - For each metric tab (Money, CO₂, Quality Time, Time), add a section with two `BreakdownRow` entries:

   | Row | Label | Count | Value | onTap |
   |-----|-------|-------|-------|-------|
   | 1 | Given | e.g. "12 transactions" | Formatted metric value given | Push `PortfolioMetricDetailScreen` with `direction: given` |
   | 2 | Received | e.g. "8 transactions" | Formatted metric value received | Push `PortfolioMetricDetailScreen` with `direction: received` |

   - Values come from `GetPortfolioMetricDetail` RPC response — add `direction_breakdown` repeated field to the response (label, count, formatted value)
   - Section header: use the same card styling as the activity breakdown (no explicit header text needed, just the card container)

3. **Ensure full activity breakdown rows per tab**
   - Mirror the community metrics pattern: each tab should show all four activity types where applicable:

   **Money Saved tab:**
   | Row | Label | Count source | Value source |
   |-----|-------|-------------|-------------|
   | Loans | `completedLoans` + " completed" | `sourceValue('Loans')` |
   | Giveaways | `completedGiveaways` + " completed" | `sourceValue('Giveaways')` |
   | Events | `pastEvents` + " completed" | `sourceValue('Events')` |
   | Requests | `fulfilledRequests` + " fulfilled" | `sourceValue('Requests')` |

   **CO₂ Avoided tab:**
   | Row | Label | Count source | Value source |
   |-----|-------|-------------|-------------|
   | Manufacture avoided | `gearCount` + " items shared" | `sourceValue('Manufacture avoided')` |
   | Waste avoided | "End-of-life emissions" | `sourceValue('Waste avoided')` |

   **Quality Time tab:**
   | Row | Label | Count source | Value source |
   |-----|-------|-------------|-------------|
   | Loans | `completedLoans` + " lending interactions" | `sourceValue('Loans')` |
   | Giveaways | `completedGiveaways` + " completed" | `sourceValue('Giveaways')` |
   | Events | `pastEvents` + " gatherings" | `sourceValue('Events')` |
   | Requests | `fulfilledRequests` + " fulfilled" | `sourceValue('Requests')` |

   **Time Recovered tab:**
   | Row | Label | Count source | Value source |
   |-----|-------|-------------|-------------|
   | Loans | `completedLoans` + " loans" | `sourceValue('Loans')` |
   | Requests | `fulfilledRequests` + " fulfilled" | `sourceValue('Requests')` |

   - Every `BreakdownRow` gets an `onTap` that calls the portfolio equivalent of `MetricDetailScreen.push()` with the appropriate `category` string
   - Rows with zero counts are hidden (same pattern as community version)

4. **Tappable hero metrics and stat pills**
   - Hero metric values (`ImpactHeroValue`) get an `onTap` that pushes `PortfolioMetricDetailScreen` for the selected dimension (no category filter — shows overall)
   - `StatPillRow` pills with `onTap` push the same detail screen
   - This mirrors how `CommunityMetricsScreen` wires taps on hero values and stat pills

5. **`PortfolioMetricDetailScreen`**
   - New screen analogous to `MetricDetailScreen` but takes `userId` instead of `communityId`
   - Accepts optional `direction` (given/received) and `category` (Loans/Giveaways/Events/Requests) filters
   - Calls `GetPortfolioMetricDetail` RPC with these filters
   - Pushed via `NavigationHelpers.pushWithSlide()`
   - Reuses the same detail widgets (`ImpactChartCard`, stat rows, etc.)

6. **Proto changes (`portfolio.proto`)**
   - Add `direction_breakdown` repeated field to `GetPortfolioMetricDetailResponse`:
     ```
     message DirectionBreakdown {
       string label = 1;           // "Given" or "Received"
       string count = 2;           // "12 transactions"
       string formatted_value = 3; // "$1,234"
     }
     repeated DirectionBreakdown direction_breakdown = 5;
     ```
   - Add optional `direction` and `category` filter fields to `GetPortfolioMetricDetailRequest`

**Layout order per tab (top to bottom):**
1. `ImpactHeroValue` (tappable → detail screen)
2. `StatPillRow` (tappable pills → detail screen)
3. `ImpactChartCard` (from detail data)
4. `SufficiencyRing` (QT tab only)
5. `TrendCard`
6. **Direction breakdown** — Given / Received `BreakdownRow` entries (new)
7. **Activity breakdown** — Loans / Giveaways / Events / Requests `BreakdownRow` entries
8. `InsightCard`

**Files modified:**
- `proto/ripls/api/portfolio.proto` — add direction/category filters to detail request, add `DirectionBreakdown` to detail response
- `server/services/portfolio/metrics.go` — compute direction breakdown values, support direction/category filtering
- `app/lib/presentation/screens/portfolio/portfolio_metrics_screen.dart` — remove toggle chips, add direction breakdown section, wire all `onTap` handlers
- `app/lib/presentation/screens/portfolio/portfolio_metric_detail_screen.dart` — new detail screen with direction/category filters
- `app/lib/presentation/viewmodels/portfolio_metrics_view_model.dart` — remove `perspective` state field
