# Impact 5.0 — Portfolio Metrics Screen Redesign

## Overview

Redesign the portfolio metrics screen to match the community metrics screen's
design and content, scoped to the individual user across all their communities.
Where the community screen shows a people leaderboard, the portfolio screen
shows the user's communities ranked by impact.

**Screens in scope:** `portfolio_metrics_screen.dart` and its detail screens.

**Reference docs:** [impact_metrics.md](../impact_metrics.md),
[impact4.md](./impact4.md)

---

## Problems

### P1 — Portfolio pills don't match community pills

The community metrics screen (after Impact 4.0) uses these pills:

| Tab | Community pills |
|-----|----------------|
| SAVED | LIBRARY, UTILIZED, PERCENTILE |
| CO₂ AVOIDED | POTENTIAL, TREES/YR, PERCENTILE |
| QUALITY TIME | HRS/PERSON, TIME-TO-SOLVE, PERCENTILE |

The portfolio screen still uses the pre-Impact 4.0 pills:

| Tab | Portfolio pills (current) |
|-----|-------------------------|
| SAVED | LIBRARY, UTILIZED, ACTIONS |
| CO₂ AVOIDED | TOTAL CO₂, PER ITEM, TREES/YR |
| QUALITY TIME | QUALITY TIME, PROBLEMS SOLVED, TIME SAVED |

Users see different metrics for the same tab depending on which screen they're
on. This creates confusion about what the canonical metrics are.

### P2 — No percentile ranking for the user

The community screen shows where a community ranks among all communities.
The portfolio screen has no equivalent — the user can't see how they rank
among all users for money saved, CO₂ avoided, or quality time.

### P3 — No community breakdown per metric tab

The community screen shows a "Top Contributors" section header that taps to
a leaderboard of members. The portfolio equivalent would be a breakdown of
the user's communities ranked by that tab's metric — showing which of their
communities drives the most impact. This doesn't exist. The current
direction breakdown (Given/Received) is useful but doesn't answer "which
community contributes the most?"

### P4 — CO₂ tab missing forest card and activity breakdown

The community CO₂ tab shows:
1. Activity-source breakdown (Loans/Giveaways/Requests/Events)
2. "Your forest is..." landmark card between chart and breakdown

The portfolio CO₂ tab shows neither. It has the old Manufacture/Waste
breakdown pattern.

### P5 — Hero label layout doesn't match

The community screen shows the hero label inline with the value (same line,
to the right, serif font). The portfolio screen shows the label below the
value.

### P6 — Tab bar styling doesn't match

The community screen uses updated tab styling with `AppColors.tabLabelInactive`
for unselected labels and serif font for the selected label. The portfolio
screen uses the older styling.

### P7 — Detail screen tables still have icons and carets

The community detail screens had icons and carets removed in Impact 4.0. The
portfolio detail screens still show them.

### P8 — QT tab missing updated secondary metrics

The community QT tab shows hrs/person and time-to-solve as secondary metrics.
The portfolio QT tab shows problems-solved and time-saved — metrics that were
replaced on the community screen.

---

## Proposed Solutions

### S1 — Align portfolio pills to community canonical set

Replace all portfolio pills to match the community screen exactly:

| Tab | Pill 1 | Pill 2 | Pill 3 |
|-----|--------|--------|--------|
| SAVED | **LIBRARY** — total library value ($) | **UTILIZED** — utilization % | **PERCENTILE** — cross-user rank |
| CO₂ AVOIDED | **POTENTIAL** — total library CO₂ potential (kg) | **TREES/YR** — kg / 20 | **PERCENTILE** — cross-user rank |
| QUALITY TIME | **HRS/WEEK** — avg QT hours per active week | **TIME-TO-SOLVE** — median request→resolution time across communities | **PERCENTILE** — cross-user rank |

**Pill semantics for portfolio context:**
- LIBRARY: total value of all items the user owns across all communities
- UTILIZED: utilization rate across the user's owned items
- POTENTIAL: total CO₂ potential of user's owned items
- TREES/YR: total CO₂ avoided / 20 across all communities
- HRS/WEEK: total QT minutes / number of active weeks (weeks with ≥1
  QT-generating interaction). Reflects personal engagement cadence.
- TIME-TO-SOLVE: median time from request creation to resolution for requests
  the user was involved in (as requester or helper)
- PERCENTILE: where this user ranks among all users for the tab's metric

**Drill-down screens:** Pill taps navigate to the same detail screens used by
the community screen. The screens accept data for rendering — they don't need
to know if the source is community or portfolio. Where a community-specific
screen doesn't apply (e.g., `Co2PotentialDetailScreen` showing community
library items), create a portfolio variant or parameterize the existing screen.

### S2 — User percentile system

Add a percentile ranking for the user across all users, per dimension. This
mirrors the community percentile but ranks users instead of communities.

**Display:** Same as community — ordinal percentile value (e.g., "85th") on
the pill, taps to a detail screen showing:
- Percentile over time chart
- Top 10 users for this dimension + this user's position (highlighted)

**Server changes:**
- New RPC: `GetUserPercentileDetail` — computes percentile by aggregating
  each user's total impact for the given dimension across all their
  communities, sorting, and finding this user's position. Handler in a new
  focused file `server/services/portfolio/percentile.go` (or
  `server/services/impact_metrics/user_percentile.go`). Use
  `logging.LoggerWithContext(ctx)` for structured logging.
  `auth.RequireAuth(ctx)` at start. The aggregation query must use GROUP BY
  with a single pass, not per-user queries. Use `LIMIT` on the ranking.
- New fields on `GetPortfolioMetricsResponse`: `percentile_money`,
  `percentile_co2`, `percentile_quality_time` (int32, 0–100).
- Cache aggressively (hourly or daily) — percentile computation touches all
  users.

### S3 — Community ranking per tab (replaces people leaderboard)

Add a "Your Communities" section header on each tab that taps to a detail
screen showing the user's communities ranked by that tab's metric:

| Tab | Ranking metric |
|-----|---------------|
| SAVED | Total money saved in that community (user's contributions) |
| CO₂ AVOIDED | Total CO₂ avoided in that community (user's contributions) |
| QUALITY TIME | Total quality time in that community (user's contributions) |

**Community ranking detail screen:**
- Title: "Your Communities"
- 10 rows (or fewer). Each row: rank, community avatar/name, formatted value.
- Tappable row → opens a filtered portfolio view for that community (showing
  the user's impact within that specific community via
  `PortfolioMetricDetailScreen` with a community filter).

This is the portfolio analog of the community leaderboard. The community
screen asks "who in this community contributes most?" The portfolio screen
asks "which of my communities generates the most impact?"

**Server changes:**
- New RPC: `GetUserCommunityRanking` — for the calling user, compute their
  impact per community for a given dimension. Return ranked list. Handler
  in `server/services/impact_metrics/user_community_ranking.go`. Use
  `logging.LoggerWithContext(ctx)` for structured logging.
  `auth.RequireAuth(ctx)` at start. Use a single GROUP BY query, not
  per-community queries. `LIMIT` on the result. Batch-fetch community
  names and media IDs via `storage.GetByIDs`.
- This uses the same per-user-per-community aggregation already done by
  `GetCommunityLeaderboard` but inverted: instead of "all users in one
  community," it's "all communities for one user."

### S4 — CO₂ tab: forest card + activity breakdown

Add the forest comparison card and switch to activity-source breakdown on the
portfolio CO₂ tab, matching the community screen.

1. **Forest card:** Reuse `ForestComparisonCard` widget. Compute tree count
   from the user's total CO₂ across all communities. Place below chart,
   above breakdown rows.
2. **Breakdown rows:** Switch from Manufacture Avoided / Waste Avoided to
   Loans / Giveaways / Requests / Events. The data is already available in
   `GetPortfolioMetricDetailResponse.source_breakdown`.

### S5 — Shared widgets: hero, tab bar, tables

Reuse the community screen's updated widgets on the portfolio screen:

1. **Hero layout:** Use `ImpactHeroValue` (or the same `Wrap`-based
   value+label layout) for inline label.
2. **Tab bar:** Use `ImpactTabBar` (shared) with the updated
   `AppColors.tabLabelInactive` styling and serif selected label.
3. **Detail tables:** Remove icons and chevron carets from portfolio detail
   screen tables, matching the community cleanup.

If the community widgets are already parameterized to accept data (not
hardcoded to community context), reuse them directly. If they assume a
community ID, parameterize them or create thin portfolio wrappers.

### S6 — QT tab secondary metrics

Replace portfolio QT pills with hrs/week and time-to-solve:

- **HRS/WEEK** for portfolio: total QT minutes across all user's communities
  / number of active weeks (weeks with ≥1 QT-generating interaction).
  This differs from the community screen's HRS/PERSON — the portfolio
  metric reflects personal engagement cadence rather than per-member depth.
- **TIME-TO-SOLVE** for portfolio: median solve time for requests the user
  participated in (as requester or helper) across all communities.

**Detail screens:** HrsWeek detail shows the same 7 QT attribute breakdown
as the community HrsPerPerson screen but with a weekly time-series chart
instead. TimeToSolve detail shows the same trend chart + recent requests
table, parameterized for portfolio context.

---

## Recommendations

1. **Share widgets aggressively.** The community screen widgets
   (`ImpactHeroValue`, `ImpactTabBar`, `StatPillRow`, `BreakdownRow`,
   `ForestComparisonCard`) should be parameterized to accept data rather
   than being coupled to community-specific providers. If they already
   accept data props, just reuse them. If they read community providers
   internally, refactor to accept data props.

2. **Keep the direction breakdown.** The Given/Received breakdown is unique
   to the portfolio and valuable. Keep it alongside the source breakdown
   (Loans/Giveaways/Requests/Events). Show direction breakdown first, then
   source breakdown — this matches the current layout order.

3. **Reuse detail screens where possible.** `PercentileDetailScreen`,
   `TreesDetailScreen`, `Co2PotentialDetailScreen`, `HrsPerPersonDetailScreen`,
   `TimeToSolveDetailScreen` should accept data/params rather than fetching
   community-specific data internally. If they currently read a community
   provider, parameterize them to accept either community or portfolio data.

4. **Percentile ranks users, not communities.** The portfolio percentile
   answers "how do I rank among all users" — distinct from the community
   percentile which answers "how does my community rank among all
   communities." The detail screen should show top users (not communities).

5. **Phase server work to unblock client early.** Add the new aggregate
   fields to `GetPortfolioMetricsResponse` first (percentiles, potential
   CO₂, hrs/week, time-to-solve). The detail RPCs can come in a later
   phase while the pills show values immediately.

6. **Forest card tree count for portfolio.** Sum the user's total CO₂
   across all communities. This is already available in the portfolio
   metrics response as `co2_avoided`. Convert to trees (kg / 20) and pass
   to `ForestComparisonCard`.

---

## Decisions

1. **Portfolio QT pill 1 is HRS/WEEK, not HRS/PERSON.** The community screen
   uses HRS/PERSON (total QT / total members). For the portfolio, the more
   meaningful metric is the user's average quality time per week — it
   reflects personal engagement cadence. Computation: total QT minutes /
   number of active weeks (weeks with at least one QT-generating
   interaction).

2. **TIME-TO-SOLVE scope:** Both requests the user created and requests the
   user helped resolve. The portfolio reflects total impact — helping resolve
   requests counts.

3. **Community ranking tap target:** Opens a filtered portfolio view for that
   community (showing the user's impact within that specific community), not
   the community metrics screen. This keeps the user in their portfolio
   context and answers "what did *I* contribute to this community?"

4. **Percentile anonymization:** Other users are shown with location-based
   anonymous names derived from their community locations (e.g., "Someone in
   Portland", "Someone in Austin"). The calling user's row shows their real
   name. The server generates these anonymous labels from the user's primary
   community location — no real names are exposed.

---

## Phased Implementation Plan

### Phase 1 — Visual alignment (client only)

**Goal:** Match the portfolio screen's visual design to the community screen
with no server changes. Use existing data.

**Changes:**

1. **Hero layout inline** (`portfolio_metrics_screen.dart`):
   - Reuse the `Wrap`-based value+label pattern from `ImpactHeroValue`.
   - If `ImpactHeroValue` is already a standalone widget that accepts value,
     label, and hook text as params, use it directly. If it reads community
     providers internally, extract the layout into a shared widget or
     parameterize it.

2. **Tab bar styling**:
   - Use `ImpactTabBar` if it's a reusable widget. If it's coupled to
     community state, refactor to accept tab data as props (labels, values,
     accent colors, selected index, onSelect callback).
   - Unselected labels use `AppColors.tabLabelInactive(context)`.
   - Selected label uses serif font matching the value.

3. **Remove icons and carets from portfolio detail tables**:
   - `portfolio_metric_detail_screen.dart`: Remove chevrons from table rows.
   - Any portfolio-specific detail screens: remove icons and carets.

4. **CO₂ breakdown rows**: Switch from Manufacture Avoided / Waste Avoided
   to activity-source breakdown (Loans / Giveaways / Requests / Events).
   The `source_breakdown` data already exists in
   `GetPortfolioMetricDetailResponse`.

5. **Forest card on CO₂ tab**: Compute tree count from existing
   `co2_avoided` field. Reuse `ForestComparisonCard` widget. Place below
   chart, above direction breakdown.

**Verification:** Visual inspection + `flutter test --reporter expanded`.

### Phase 2 — Pill alignment (client + server)

**Goal:** Replace old pills with the Impact 4.0 canonical set. Server adds
the new aggregate fields needed for the new pills.

**Server changes:**

Add new fields to `GetPortfolioMetricsResponse` (wire-compatible — adding
fields to existing response):
- `co2_potential_grams` (float) — sum of `CarbonEstimate.co2e_grams` for all
  gear owned by the user across all communities.
- `quality_time_per_week_minutes` (float) — total QT minutes / number of
  active weeks (weeks with ≥1 QT-generating interaction).
- `active_weeks` (int32) — count of weeks with at least one QT-generating
  interaction (needed for HRS/WEEK computation and detail display).
- `time_to_solve_minutes` (float) — median minutes from request creation to
  fulfillment for all requests the user participated in (as requester or
  helper).
- `utilization_rate` (float) — if not already present, utilization percentage
  across all user's owned items.
- `library_value_formatted` (string) — total library value formatted.

Server handler updates in `server/services/portfolio/metrics.go` (or a new
`portfolio_aggregates.go` file if the existing file is large). Use
`logging.LoggerWithContext(ctx)` for structured logging. All queries must use
`LIMIT` and batch patterns. Push filtering to SQL (WHERE clauses, not
post-fetch Go filtering).

**Client changes:**

1. **Money tab pills**: Replace ACTIONS with PERCENTILE (placeholder
   "(coming)" until Phase 3).
   - LIBRARY: existing `library_value_formatted` or computed from data
   - UTILIZED: `utilization_rate` from response
   - PERCENTILE: placeholder

2. **CO₂ tab pills**: Replace TOTAL CO₂ and PER ITEM with POTENTIAL and
   PERCENTILE.
   - POTENTIAL: `co2_potential_grams / 1000` formatted as kg. Taps to
     `Co2PotentialDetailScreen` (parameterized for portfolio context —
     shows user's owned items, not community items).
   - TREES/YR: existing computation (total CO₂ / 20)
   - PERCENTILE: placeholder

3. **QT tab pills**: Replace QUALITY TIME, PROBLEMS SOLVED, TIME SAVED with
   HRS/WEEK, TIME-TO-SOLVE, PERCENTILE.
   - HRS/WEEK: `quality_time_per_week_minutes / 60` formatted as hours.
     Taps to new `HrsPerWeekDetailScreen`.
   - TIME-TO-SOLVE: `time_to_solve_minutes` formatted. Taps to
     `TimeToSolveDetailScreen` (parameterized for portfolio).
   - PERCENTILE: placeholder

4. **Repository layer** (`PortfolioRepository`):
   - Aggregate pill values come from the existing `getPortfolioMetrics()`
     response (cache key `'portfolio:metrics'`). No new methods needed for
     these.
   - For pill drill-down detail screens that need portfolio-specific item
     lists or breakdowns, add new repository methods with explicit cache
     keys:
     - `getPortfolioCo2Potential()` → `'portfolio:co2_potential'`
     - `getPortfolioHrsPerWeekDetail()` → `'portfolio:hrs_per_week'`
     - `getPortfolioTimeToSolveDetail()` → `'portfolio:time_to_solve'`

5. **Detail screen parameterization**: `Co2PotentialDetailScreen` and
   `TimeToSolveDetailScreen` currently accept a `communityId` parameter.
   Refactor to accept either a `communityId` OR a portfolio flag. The
   screen renders the same UI regardless of data source. Each screen reads
   state from a viewmodel via `ref.watch()` — the viewmodel calls the
   appropriate repository method (community or portfolio) based on context.
   Screens never call repositories directly (architecture.md rule 1).
   `HrsPerWeekDetailScreen` is a new portfolio-only screen (not a
   parameterization of `HrsPerPersonDetailScreen`) since the metric and
   chart axis differ.

**i18n:** Add new ARB keys to both `app_en.arb` and `app_es.arb` for any
new portfolio-specific labels. Reuse existing pill label keys where
identical to community (e.g., `impactPillPercentile` is the same key).

**Verification:** All three tabs show updated pills with correct values.
`flutter test --reporter expanded`.

### Phase 3 — User percentile (server + client)

**Goal:** Wire up the PERCENTILE pill on all three tabs with real data.

**Server changes:**

New handler in `server/services/impact_metrics/user_percentile.go` (or
`server/services/portfolio/percentile.go` — keep in the service that owns
the proto). Use `logging.LoggerWithContext(ctx)` for structured logging.

- Add `percentile_money`, `percentile_co2`, `percentile_quality_time`
  (int32, 0–100) fields to `GetPortfolioMetricsResponse`.
- Computation: for each user, aggregate their total impact for the given
  dimension across all communities. Sort all users, find this user's
  position. Cache server-side with a 1-hour TTL.
- The aggregation query must use GROUP BY user_id with a single pass over
  `ImpactEstimate` records, not per-user queries. Use `LIMIT` on the
  final ranking.

- New RPC: `GetUserPercentileDetail` with dedicated request/response types:
  ```protobuf
  message GetUserPercentileDetailRequest {
    ImpactMetricDimension dimension = 1;
  }
  message GetUserPercentileDetailResponse {
    int32 current_percentile = 1;
    repeated PercentileDataPoint history = 2;
    repeated RankedUser top_users = 3;
    int32 this_user_rank = 4;
    string this_user_value = 5;
  }
  message RankedUser {
    int32 rank = 1;
    string display_name = 2;
    string formatted_value = 3;
    bool is_this_user = 4;
  }
  ```
  Note: `RankedUser.display_name` is anonymized for other users using
  location-based names derived from their primary community location
  (e.g., "Someone in Portland", "Someone in Austin"). The server resolves
  each user's primary community, looks up its location, and generates the
  label. Only the calling user's row shows their real name. If a user has
  no community location, fall back to "Someone nearby."

**Client changes:**

- New service method: `ImpactMetricsService.getUserPercentileDetail()` —
  wraps the `GetUserPercentileDetail` RPC.
- New repository method:
  `ImpactMetricsRepository.getUserPercentileDetail()` with cache key
  `'impact:user_percentile:$dimension'`.
- New viewmodel: `user_percentile_detail_view_model.dart` —
  `AsyncNotifierProvider.autoDispose` with `ref.mounted` guards. Calls
  repository via `ref.read()`, never service directly.
- New detail screen: `user_percentile_detail_screen.dart` — same layout as
  `PercentileDetailScreen` but shows users instead of communities. Pushed
  via `NavigationHelpers.pushWithSlide()`. Screen reads state via
  `ref.watch()` only.
- Wire PERCENTILE pill on all 3 portfolio tabs to
  `UserPercentileDetailScreen`.
- Replace "(coming)" placeholder with actual percentile value from
  `GetPortfolioMetricsResponse`.

**Verification:** All three tabs show live percentile. Tap → detail screen
with chart and top 10 list. `flutter test --reporter expanded`.

### Phase 4 — Community ranking (server + client)

**Goal:** Add "Your Communities" section on each tab that taps to a ranked
list of the user's communities by impact.

**Server changes:**

New handler in `server/services/impact_metrics/user_community_ranking.go`.
Use `logging.LoggerWithContext(ctx)` for structured logging.

- New RPC: `GetUserCommunityRanking` with dedicated types:
  ```protobuf
  message GetUserCommunityRankingRequest {
    ImpactMetricDimension dimension = 1;
  }
  message GetUserCommunityRankingResponse {
    repeated RankedUserCommunity communities = 1;
  }
  message RankedUserCommunity {
    int32 rank = 1;
    string community_id = 2;
    string community_name = 3;
    optional string media_id = 4;
    string formatted_value = 5;
  }
  ```
- Computation: for the calling user, aggregate their `ImpactEstimate` values
  per community for the given dimension. Rank by total value descending.
  Use a single GROUP BY query, not per-community queries. Use `LIMIT`.
- Batch-fetch community names and media IDs via `storage.GetByIDs`.

**Client changes:**

1. New tappable section header on each portfolio tab: "Your Communities >"
   below the source breakdown rows (and direction breakdown if shown). Taps
   to `UserCommunityRankingScreen`.

2. New screen: `user_community_ranking_screen.dart`
   - Title: "Your Communities"
   - Rows: rank, community avatar (`CachedMediaImage` with
     `ImageCacheKeys.thumbnail(mediaId)`), community name, formatted value.
   - Tappable row → `NavigationHelpers.pushScreen()` to a filtered
     portfolio detail view scoped to that community (showing the user's
     impact within that specific community — uses
     `PortfolioMetricDetailScreen` with a community filter). This keeps
     the user in their portfolio context.
   - This ranking screen itself is pushed via
     `NavigationHelpers.pushWithSlide()` (utility list screen — no
     swipe-to-close needed).
   - Screen reads state via `ref.watch()` from the viewmodel; never calls
     repository directly.

3. New viewmodel: `user_community_ranking_view_model.dart` —
   `AsyncNotifierProvider.autoDispose.family` keyed by dimension. Calls
   repository, never service directly.

4. New service method: `ImpactMetricsService.getUserCommunityRanking()` —
   wraps the `GetUserCommunityRanking` RPC.

5. New repository method:
   `ImpactMetricsRepository.getUserCommunityRanking()` with cache key
   `'impact:user_community_ranking:$dimension'`.

**Verification:** Each tab shows "Your Communities >" section header. Tap →
ranked list of communities. Tap community → community metrics screen.
`flutter test --reporter expanded`.

### Phase 5 — Detail screen parameterization + tests

**Goal:** Ensure all pill drill-down detail screens work correctly in the
portfolio context. Add comprehensive tests.

**Client changes:**

1. **Parameterize existing detail screens** to work for both community and
   portfolio contexts. Each screen that currently reads a community-specific
   provider needs an alternative code path for portfolio data:
   - `Co2PotentialDetailScreen`: Parameterize to accept either communityId
     or portfolio flag. Shows the user's owned items across all communities,
     ranked by potential CO₂.
   - `HrsPerWeekDetailScreen`: New portfolio-only screen. Shows weekly QT
     trend chart (x-axis: weeks, y-axis: hours) + 7 QT attribute breakdown.
     Not a parameterization of `HrsPerPersonDetailScreen` since the metric
     (hours/week vs hours/person) and chart axis differ.
   - `TimeToSolveDetailScreen`: Parameterize to accept portfolio flag. Shows
     requests the user participated in (created or helped) across all
     communities with solve times.
   - `TreesDetailScreen`: Already parameterized by CO₂ value (no community
     dependency). No changes needed.

2. **Portfolio-specific detail viewmodels** where needed:
   `AsyncNotifierProvider.autoDispose` with `ref.mounted` guards. Each
   viewmodel calls the appropriate repository (portfolio or impact metrics,
   depending on which service owns the RPC). Screens read state via
   `ref.watch()` only — they never call repositories or services directly.

3. **Pill drill-down invariant test**: Add a widget test that verifies every
   portfolio pill taps to the correct detail screen with matching title and
   hero value.

**Server changes:**

Add new RPCs or extend existing ones if the portfolio detail screens need
data not currently in `GetPortfolioMetricDetailResponse`:
- CO₂ potential item list: may need a new RPC to return the user's owned
  items with their `CarbonEstimate` values. Handler in a new focused file
  (e.g., `server/services/portfolio/co2_potential.go`).
- Time-to-solve detail: may need a new RPC returning recent requests with
  individual solve times for the portfolio context. Handler in a new file
  (e.g., `server/services/portfolio/time_to_solve.go`).
- Hrs/person attribute breakdown: may need aggregate QT attribute values
  across all user's communities.

Each new RPC gets its own dedicated request/response types per CLAUDE.md.
Handler files stay short and focused (< 500 lines). Use
`logging.LoggerWithContext(ctx)` for structured logging.
`auth.RequireAuth(ctx)` at start. All queries use `LIMIT` and batch
patterns (no N+1).

**i18n:** Verify all new portfolio-specific labels have ARB keys in both
`app_en.arb` and `app_es.arb`. Viewmodels expose typed state, not resolved
strings.

**Tests:**

Server tests:
- Portfolio aggregate computation (potential CO₂, hrs/person, time-to-solve)
- User percentile: ordering, edge cases (1 user, tied values)
- Community ranking: correct per-community aggregation, calling user data
- All queries use `storage.AssertMaxQueries` to prevent N+1 regressions

Client tests:
- Visual alignment: hero layout, tab styling, table rows without icons
- Pill values match expected computations
- Pill tap → correct detail screen (invariant test)
- Community ranking screen: rendering, tap navigation
- Percentile detail screen: chart, anonymized user list
- Forest card on CO₂ tab
- Direction + source breakdown coexistence

**Verification:** All portfolio pills tap to correct detail screens with
matching data. All tests pass. `flutter test --reporter expanded` and
`npm run build`.

---

## Updated Portfolio Metrics Reference

After all phases, the portfolio metrics screen matches the community screen
design with these portfolio-specific differences:

### Stat Pills (3 per tab — identical labels to community)

| Tab | Pill 1 | Pill 2 | Pill 3 |
|-----|--------|--------|--------|
| SAVED | **LIBRARY** — total library value ($) | **UTILIZED** — utilization % | **PERCENTILE** — cross-user rank |
| CO₂ AVOIDED | **POTENTIAL** — total library CO₂ potential (kg) | **TREES/YR** — kg / 20 | **PERCENTILE** — cross-user rank |
| QUALITY TIME | **HRS/WEEK** — avg QT hours per active week | **TIME-TO-SOLVE** — median request→resolution time | **PERCENTILE** — cross-user rank |

### Pill Drill-Down Content

Same detail screens as community, parameterized for portfolio context. The
screen layout is identical; only the data source differs.

### Source Breakdown Rows

Same as community: Loans, Giveaways, Requests, Events.

### Direction Breakdown (portfolio-specific, above source breakdown)

Given (impact generated for others) and Received (impact from borrowing/
receiving). Tappable → `PortfolioMetricDetailScreen` filtered by perspective.

### Additional Tab Content

| Tab | Extra content (between chart and breakdown) |
|-----|---------------------------------------------|
| CO₂ AVOIDED | "Your forest is..." landmark comparison card |

### Community Ranking (tappable section header, all tabs)

"Your Communities >" section header below breakdown rows on each tab. Taps
to a ranked list of the user's communities by that tab's metric. Each
community row tappable → filtered portfolio view for that community (showing
the user's impact within that specific community).

### Percentile

Ranks users (not communities). Detail screen shows top 10 users with
location-based anonymous names (e.g., "Someone in Portland") + this user's
highlighted position with real name.

### Detail Screen Table Style

Same as community: no icons, no chevron carets. Tappable rows navigate
without visual arrow affordance.

---

## Audit Notes

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, and `docs/server/architecture.md` on 2026-04-01.
Changes made in-place:

1. **Missing client data layer (architecture.md + caching.md violations).**
   S2, S3, and Phase 2 originally lacked explicit service methods, repository
   methods, and cache keys for new data fetches. Fixed: added explicit
   service methods (wrapping RPCs), repository methods with namespaced cache
   keys (`'portfolio:co2_potential'`, `'impact:user_percentile:$dimension'`,
   `'impact:user_community_ranking:$dimension'`), and the correct
   service → repository → viewmodel → screen data flow. Architecture rules:
   "Repositories are the ONLY layer that calls Services" and "ViewModels are
   the ONLY layer that calls Repositories."

2. **Screen/repository boundary (architecture.md violation).** Phase 5
   detail viewmodels originally said "calls the portfolio repository, not
   the impact metrics repository" — but the choice depends on which service
   owns the RPC, not a blanket rule. Fixed: clarified that viewmodels call
   the appropriate repository. Added explicit rule that screens read state
   via `ref.watch()` only and never call repositories or services directly.

3. **Missing server auth, logging, and SQL patterns (CLAUDE.md + server
   architecture.md).** S2 and S3 server changes originally lacked
   `auth.RequireAuth(ctx)`, `logging.LoggerWithContext(ctx)`, `LIMIT`, and
   batch query callouts. Fixed: added all four to every server change
   section. Phase 5 server changes now also specify structured logging,
   auth, and query constraints.

4. **Missing server file organization (server architecture.md).** S2 and S3
   did not specify handler file locations. Fixed: added explicit paths
   (`server/services/portfolio/percentile.go`,
   `server/services/impact_metrics/user_community_ranking.go`,
   `server/services/portfolio/co2_potential.go`,
   `server/services/portfolio/time_to_solve.go`). Files stay < 500 lines.

5. **Navigation pattern ambiguity (architecture.md).** Phase 4 community
   ranking screen had both `pushScreen()` and `pushWithSlide()` mentioned
   without clarifying which applies where. Fixed: the ranking screen itself
   is pushed via `pushWithSlide()` (utility list); tapping a community row
   uses `pushScreen()` (primary content screen with `SwipeToCloseMixin`).

6. **Missing CachedMediaImage spec (CLAUDE.md media display).** Phase 4
   community ranking rows already specified `CachedMediaImage` with
   `ImageCacheKeys.thumbnail(mediaId)` — compliant, no change needed.

7. **i18n compliance (i18n.md).** Phases 2 and 5 already specify adding ARB
   keys to both `app_en.arb` and `app_es.arb`, and that viewmodels expose
   typed state not resolved strings — compliant, no change needed.

---

**Last Updated:** 2026-04-01
