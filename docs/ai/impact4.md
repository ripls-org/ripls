# Impact 4.0 — Leaderboards, Percentiles & Metric Refinements

## Overview

Polish the community metrics screen with three categories of changes:

1. **Leaderboards** — Add per-tab leaderboards showing top community members
   by impact, with tappable user drill-downs.
2. **Percentile rankings** — Add a cross-community percentile metric to Money
   and CO₂ tabs, plus a new one on Quality Time.
3. **Metric refinements** — Swap, remove, and add secondary metrics on each
   tab; move the "Your forest is" card to the main CO₂ tab; redesign the
   header layout; clean up detail screen tables.

**Screens in scope:** `community_metrics_screen.dart` and all its detail
screens.

**Reference docs:** [impact_metrics.md](../impact_metrics.md),
[impact3.md](./impact3.md)

---

## Problems

### P1 — No per-person impact visibility within a community

Users can see aggregate community metrics but can't tell who is contributing
the most to money saved, CO₂ avoided, or quality time. There's no way to
celebrate top contributors or understand individual impact patterns within the
community context.

### P2 — No cross-community benchmarking on individual tabs

The existing `CommunityComparison` radar chart compares 5 activity dimensions
but doesn't answer the simple question: "How does our community rank against
others in money saved / CO₂ / quality time?" Users want a percentile or
ranking for each specific metric dimension.

### P3 — Money tab "Actions" pill is redundant

The Actions count (loans + giveaways + requests) duplicates information already
visible in the breakdown rows below the chart. The pill slot would be better
used for the percentile ranking.

### P4 — CO₂ tab shows Manufacture/Waste breakdown instead of activity sources

The CO₂ breakdown rows show "Manufacture Avoided" and "Waste Avoided" — the
two components of the carbon formula. Users don't think in these terms. They
think in terms of what they _did_ (loans, giveaways, requests, events). The
Money and QT tabs already use the activity-source breakdown pattern.

### P5 — CO₂ "Per Item" and "Total CO₂" pills are low-value

"Per Item" is a ratio that doesn't drive behavior. "Total CO₂" just repeats
the hero value. Better secondary metrics: the CO₂ _potential_ of the full
library (motivates listing more items) and a percentile ranking.

### P6 — "Your forest is" comparison is buried in the Trees/yr detail

The playful forest landmark comparison is one of the most engaging elements
but requires tapping through to the Trees/yr detail screen to see it. It
should be visible on the main CO₂ tab.

### P7 — Quality Time secondary metrics don't tell a useful story

Current QT pills are: Quality Time (repeats hero), Problems Solved, Time
Saved. Better metrics: hours per person (engagement depth), time-to-solve
(community responsiveness), and percentile (benchmarking).

### P8 — Header label wastes vertical space

The hero label ("saved", "CO₂ avoided", "quality time") sits on its own line
below the value. Moving it to the same line as the value (to the right) saves
vertical space and looks cleaner.

### P9 — Unselected tab labels are hard to read

The unselected tab values and labels use 30% opacity white text. The labels
are particularly hard to read at that opacity. They need to be lighter gray
rather than transparent white.

### P10 — Detail screen tables show unnecessary icons and carets

The item/contributor tables in detail screens show icons and chevron carets
that add visual clutter without helping navigation (the whole row is
tappable). Clean, minimal table rows are easier to scan.

---

## Proposed Solutions

### S1 — Per-tab leaderboards

Add a leaderboard section below the breakdown rows on each tab. The
leaderboard shows the top community members ranked by the tab's primary
metric:

| Tab | Ranking metric |
|-----|---------------|
| SAVED | Total money saved (sum of cost savings from all their participations) |
| CO₂ AVOIDED | Total CO₂ avoided (sum of carbon savings from all their participations) |
| QUALITY TIME | Total quality time created (sum of QT minutes from all their participations) |

"Participations" includes all roles: owned items that were lent/given,
items borrowed/received, requests helped/fulfilled, events attended/hosted.

**Leaderboard display:**
- Not shown inline on the main metrics tab. Accessed via a dedicated
  leaderboard detail screen (e.g., tappable "Top Contributors" section
  header or row).
- Detail screen shows top 10 rows, each with: rank number, avatar, name,
  formatted value.
- Tappable row → opens a **user community impact detail screen** scoped to
  that person and this community.

**User community impact detail screen:**
- Hero: user name + avatar + total value for this metric in this community
- Longitudinal line chart (cumulative over time)
- Table of items/transactions that contributed, reverse chronological (most
  recent at top). Each row: item/event name, type (loan/giveaway/request/
  event), role (owner/borrower/helper/attendee), date, impact value.
- No pagination needed initially (most users won't have hundreds of
  transactions in a single community).

**Server changes:**
- New RPC: `GetCommunityLeaderboard` — returns top N members ranked by a
  specific dimension, with their total impact value for that dimension in
  this community. Includes the calling user's own rank even if not in
  top N.
- New RPC: `GetUserCommunityImpactDetail` — returns a user's impact
  breakdown within a specific community: time series + transaction list.

**Client changes:**
- New widget: `impact_leaderboard.dart` — renders the ranked list.
  Avatars use `CachedMediaImage` with `ImageCacheKeys.thumbnail(mediaId)`.
- New screen: `user_community_impact_screen.dart` — detail drill-down.
  Must use `SwipeToCloseMixin` and be pushed via
  `NavigationHelpers.pushScreen()`. Uses `autoDispose` provider with
  `ref.mounted` guards after async gaps.
- New viewmodel: `community_leaderboard_view_model.dart` —
  `AsyncNotifierProvider.autoDispose.family` keyed by
  `(communityId, dimension)`. The viewmodel calls the repository; the
  screen reads state via `ref.watch()` only.
- New repository method: `ImpactMetricsRepository.getCommunityLeaderboard()`
  with cache key `'impact:leaderboard:$communityId:$dimension'`.
- New repository method: `ImpactMetricsRepository.getUserCommunityImpactDetail()`
  with cache key `'impact:user_detail:$communityId:$userId:$dimension'`.
- Leaderboard data fetched when tab content loads.

### S2 — Percentile ranking metric

Add a "PERCENTILE" stat pill to each tab that shows where this community
ranks among all communities for that specific dimension. E.g., "92nd" means
this community is in the top 8% for money saved.

**Detailed view (tappable):**
- Hero: percentile value (e.g., "92nd")
- Chart: position over time (line chart showing percentile per month)
- Table: top 10 communities for this dimension + this community's position
  (highlighted). Each row: rank, community name, formatted value. No icons,
  no carets.

**Server changes:**
- New field on `GetCommunityImpactMetricsResponse`: `percentile_money`,
  `percentile_co2`, `percentile_quality_time` (int32, 0–100).
- New RPC: `GetCommunityPercentileDetail` — returns historical percentile
  data points + top 10 communities list for a given dimension.
- Computation: query all communities' aggregate metric for the dimension,
  sort, find this community's position. Cache with a reasonable TTL (hourly
  or daily) since percentiles don't change rapidly.

**Client changes:**
- New detail screen: `percentile_detail_screen.dart` — pushed via
  `NavigationHelpers.pushWithSlide()` (utility detail screen, no
  swipe-to-close needed). Uses `autoDispose` provider with `ref.mounted`
  guards.
- New viewmodel: `percentile_detail_view_model.dart` —
  `AsyncNotifierProvider.autoDispose` that loads detail via repository.
- New repository method:
  `ImpactMetricsRepository.getCommunityPercentileDetail()` with cache key
  `'impact:percentile_detail:$communityId:$dimension'`.
- Pill taps to the percentile detail screen for the current tab's dimension.

### S3 — Money tab: remove Actions, add Percentile

**New Money pills:**

| Pill 1 | Pill 2 | Pill 3 |
|--------|--------|--------|
| **LIBRARY** — library value ($) | **UTILIZED** — utilization % | **PERCENTILE** — cross-community rank |

Remove the Actions pill and its detail screen navigation from the Money tab.
The actions list remains accessible via the breakdown rows.

### S4 — CO₂ tab: activity breakdown, forest card, new pills

**Changes:**
1. Replace Manufacture Avoided / Waste Avoided breakdown rows with Loans /
   Giveaways / Requests / Events breakdown (matching Money and QT tabs).
2. Move the "Your forest is..." landmark card from `TreesDetailScreen` into
   the main CO₂ tab, positioned below the chart.
3. Remove "Total CO₂" pill (repeats hero) and "Per Item" pill.
4. Add "Potential" pill: sum of all library items' potential CO₂ savings if
   each were shared once. Ordered by most to least in the detail view.
5. Add "Percentile" pill.

**New CO₂ pills:**

| Pill 1 | Pill 2 | Pill 3 |
|--------|--------|--------|
| **POTENTIAL** — total library CO₂ potential (kg) | **TREES/YR** — kg / 20 | **PERCENTILE** — cross-community rank |

**Potential detail view:**
- Hero: total potential CO₂ savings (kg)
- Table: all library items ranked by potential CO₂ savings (most to least).
  Each row: item name, sharer name, potential kg. No icons, no carets.
- Methodology link.

**Server changes:**
- Compute potential CO₂: sum of `CarbonEstimate.co2e_grams` for all gear in
  the community. Already stored on gear records.
- Return in `GetCommunityImpactMetricsResponse` as a new field.
- Activity-source breakdown for CO₂ is already computable from stored
  `ImpactEstimate` records grouped by transaction type.

### S5 — Quality Time tab: new secondary metrics

**New QT pills:**

| Pill 1 | Pill 2 | Pill 3 |
|--------|--------|--------|
| **HRS/PERSON** — avg QT per total member | **TIME-TO-SOLVE** — median request→resolution time | **PERCENTILE** — cross-community rank |

**Hrs/Person detail view:**
- Hero: average hours per person
- Breakdown by the 7 QT attributes: duration, modality, group_size,
  tie_strength, reciprocity, novelty, vulnerability. Each row shows the
  attribute name and its contribution/average value.
- No icons, no carets.

**Time-to-Solve detail view:**
- Hero: formatted average time (e.g., "4.2 hrs")
- Chart: time-to-solve trend over time
- Table: recent requests with their individual solve times, reverse
  chronological. Each row: request title, helper name, solve time, date.
- No icons, no carets.

**Server changes:**
- Hrs/Person: total QT minutes / total member count (all community
  members, not just active ones). Already computable.
- Time-to-Solve: for each fulfilled request (requests only, not loans),
  compute time from creation to fulfillment. Return median. New
  computation in calculator or metric detail handler.
- Attribute breakdown for hrs/person detail: aggregate each of the 7
  `QualityTimeAttributes` dimensions across all transactions.

### S6 — Header layout: label inline with value

Move the dimension label ("saved", "CO₂ avoided", "quality time") to the
same line as the hero value, positioned to its right. For the currently
selected tab, the label font should match the value (serif, same weight).

**Changes to `impact_hero_value.dart`:**
- Lay out value + label in a `Row` instead of stacked `Column`
- Label uses same font family as value (Lora/Libre Baskerville)
- Label is smaller than value but on the same baseline
- Hook text remains below on its own line

### S7 — Unselected tab styling

**Changes to `impact_tab_bar.dart`:**
- Selected tab: white text for value, white text for label (current behavior)
- Unselected tab: keep current dimmed value, but make the label a lighter
  gray instead of 30% opacity white. Add a new semantic color to `AppColors`
  (e.g., `AppColors.tabLabelInactive(context)`) that returns a warm light
  gray in both light and dark themes. The goal is legibility without
  competing with the active tab.
- **Do NOT use hardcoded `Color(0xFF...)` values** — all colors must go
  through `AppColors` helpers per `architecture.md` theming rules.

### S8 — Clean up detail screen tables

Remove icons and chevron carets from all table rows in detail screens. The
row remains tappable (when applicable) but without visual clutter.

**Files affected:**
- `metric_detail_screen.dart` — remove chevron icons from `_DetailRowWidget`
- `per_item_co2_detail_screen.dart` — remove chevrons
- `problems_solved_detail_screen.dart` — remove type icons from rows
- `time_saved_detail_screen.dart` — remove icons if present
- `trees_detail_screen.dart` — equivalence rows can keep their icons (these
  are illustrative, not navigation affordances)
- All new detail screens (percentile, potential, hrs/person, time-to-solve,
  user community impact) — build without icons/carets from the start

---

## Recommendations

1. **Phase the leaderboard server work carefully.** The two new RPCs
   (`GetCommunityLeaderboard` and `GetUserCommunityImpactDetail`) need
   efficient queries. Compute leaderboard from stored `ImpactEstimate`
   records grouped by user, not by re-aggregating from raw transactions.
   Use `LIMIT` on all queries.

2. **Cache percentiles aggressively.** Cross-community percentile
   computation touches all communities. Compute once per dimension per day
   (or hourly) and cache server-side. Return cached values in the metrics
   response.

3. **Do design changes (S6–S8) first.** Header layout, tab styling, and
   table cleanup are pure client changes with no server dependencies. Ship
   them first for an immediate visual improvement.

4. **Keep the "Your forest is" card as a shared widget.** Extract it from
   `TreesDetailScreen` into a standalone `ForestComparisonCard` widget so
   it can be used on both the main CO₂ tab and the Trees detail screen.

5. **Time-to-Solve uses median, not mean.** A few very slow resolutions
   would skew a mean. Median is more representative of typical community
   responsiveness.

6. **Leaderboard shows "You" label for the current user.** If the current
   user appears in the leaderboard, highlight their row (similar to how
   `CommunityCompareV2` highlights "Your Circle").

---

## Decisions

1. **Leaderboard size:** No leaderboard on the main tab. Show top 10 in the
   leaderboard detail screen (accessed via a "Top Contributors" row or
   section header tap).

2. **Percentile privacy:** All communities are visible in the top 10 list.
   Community names are already public in the leaderboard/discovery screens.

3. **Time-to-Solve scope:** Requests only. "Time to solve" semantically maps
   to "someone asked for help and got it." Loans are excluded.

4. **Hrs/Person denominator:** Total community members (not just active
   members). This gives a conservative number that reflects overall community
   engagement depth.

5. **Forest card placement:** Below the chart, above the breakdown rows. It's
   a visual highlight that anchors the numbers.

---

## Phased Implementation Plan

### Phase 1 — Design changes (client only)

**Goal:** Ship header layout, tab styling, and table cleanup with no server
changes.

**Changes:**

1. **Hero label inline** (`impact_hero_value.dart`):
   - Replace vertical `Column(value, label)` with a `Row`
   - Value: current 48px Lora serif
   - Label: 18px Lora serif, same color, vertically aligned to baseline of
     value
   - Hook text stays below on its own line (unchanged)
   - Handle overflow: if value + label don't fit, allow label to wrap to
     next line (use `Wrap` widget)

2. **Unselected tab labels** (`impact_tab_bar.dart`):
   - Selected tab: value and label both white (no change)
   - Unselected tab value: keep current 30% opacity
   - Unselected tab label: add `AppColors.tabLabelInactive(context)` semantic
     color (warm light gray, theme-aware for light/dark mode) — legible but
     clearly secondary. Do NOT use hardcoded hex `Color(0xFF...)`.
   - Selected tab label font: match the value font (Lora serif, same weight)

3. **Remove icons and carets from detail tables**:
   - `metric_detail_screen.dart`: Remove `Icons.chevron_right` from
     `_DetailRowWidget`. Keep tap handlers.
   - `per_item_co2_detail_screen.dart`: Remove chevrons from item rows
   - `problems_solved_detail_screen.dart`: Remove type icons from action rows
   - `time_saved_detail_screen.dart`: Remove icons if present
   - `trees_detail_screen.dart`: Keep equivalence row icons (illustrative)

**Verification:** Visual inspection + `flutter test --reporter expanded`.

### Phase 2 — CO₂ tab restructure (client + server)

**Goal:** Activity-source breakdown, forest card on main tab, new pills.

**Server changes:**
- Add `co2_potential_grams` (float) field to
  `GetCommunityImpactMetricsResponse` — sum of all gear `CarbonEstimate`
  co2e_grams in the community.
- Activity-source breakdown for CO₂: group `ImpactEstimate.prevented_emissions`
  by transaction type (loan/giveaway/request/event) in the metric detail
  handler. Return as `SourceBreakdown[]` with type labels.

**Client changes:**
1. **Breakdown rows**: Replace "Manufacture Avoided" / "Waste Avoided" with
   "Loans" / "Giveaways" / "Requests" / "Events" on the CO₂ tab. Reuse
   the same `_buildBreakdownRows` pattern from Money/QT tabs.

2. **Forest card on main tab**: Extract the "Your forest is..." card from
   `TreesDetailScreen` into a reusable `ForestComparisonCard` widget in
   `app/lib/presentation/widgets/impact/forest_comparison_card.dart`. Embed
   it on the CO₂ tab between the chart and the breakdown rows. Also use it
   in `TreesDetailScreen` to avoid duplication.

3. **New pills** (POTENTIAL, TREES/YR, PERCENTILE):
   - POTENTIAL: `co2PotentialGrams / 1000` formatted as kg. Taps to new
     `Co2PotentialDetailScreen`.
   - TREES/YR: unchanged, taps to `TreesDetailScreen` (which still keeps
     the forest card + equivalences).
   - PERCENTILE: placeholder "(coming)" — wired in Phase 4.

4. **Co2PotentialDetailScreen** (new file):
   - Hero: total potential kg
   - Label: "potential CO₂ savings from your library"
   - Table: all library items ranked by potential CO₂ (most to least).
     Each row: item name, sharer name, potential kg formatted. No icons,
     no carets. Tappable → item screen via
     `NavigationHelpers.pushToItemScreen()`.
   - Methodology link at bottom.
   - Pushed via `NavigationHelpers.pushWithSlide()` (utility detail screen).
   - Uses `autoDispose` provider with `ref.mounted` guards.

5. **New repository method:**
   `ImpactMetricsRepository.getCo2Potential()` with cache key
   `'impact:co2_potential:$communityId'`.

6. Remove old CO₂ pills (Total CO₂, Per Item) and their tap handlers.

**Verification:** CO₂ tab shows forest card, activity breakdown rows,
new pills. `flutter test --reporter expanded`.

### Phase 3 — Money and QT tab metric changes (client + server)

**Goal:** New secondary metrics for Money (percentile placeholder) and QT
(hrs/person, time-to-solve, percentile placeholder).

**Server changes:**

New computation logic in `server/impact_metrics/calculator.go` (or a new
`time_to_solve.go` if it exceeds reasonable size). New RPC handler in
`server/services/impact_metrics/time_to_solve.go`. Use
`logging.LoggerWithContext(ctx)` for structured logging. Median computation
must fetch only fulfilled requests (not all requests) using WHERE clause.
Use `LIMIT` on the recent requests list in the detail response.

- Add `quality_time_per_person_minutes` (float) to
  `GetCommunityImpactMetricsResponse` — total QT minutes / total member
  count (all community members).
- Add `time_to_solve_minutes` (float) to response — median minutes from
  request creation to fulfillment for all fulfilled requests in the
  community.
- New RPC: `GetTimeToSolveDetail` with dedicated request/response types —
  returns time series of solve times + recent requests with individual
  solve times.
- Add QT attribute aggregation to metric detail handler for hrs/person
  breakdown: average values for duration, modality, group_size,
  tie_strength, reciprocity, novelty, vulnerability across all transactions.

**Client changes:**

1. **Money tab**: Remove Actions pill. Add PERCENTILE pill (placeholder
   "(coming)" until Phase 4). Pill order: LIBRARY, UTILIZED, PERCENTILE.

2. **QT tab**: Replace all 3 pills:
   - HRS/PERSON: formatted hours per person. Taps to new
     `HrsPerPersonDetailScreen`.
   - TIME-TO-SOLVE: formatted time. Taps to new
     `TimeToSolveDetailScreen`.
   - PERCENTILE: placeholder "(coming)" until Phase 4.

3. **HrsPerPersonDetailScreen** (new file):
   - Hero: average hours per person
   - Label: "average quality time per community member"
   - Breakdown table: 7 rows for QT attributes (duration, modality,
     group_size, tie_strength, reciprocity, novelty, vulnerability). Each
     row: attribute name, average value or distribution. No icons, no carets.
   - Methodology link.
   - Pushed via `NavigationHelpers.pushWithSlide()`.

4. **TimeToSolveDetailScreen** (new file):
   - Hero: median time formatted (e.g., "4.2 hrs")
   - Label: "median time from request to resolution"
   - Chart: time-to-solve trend over time (line chart)
   - Table: recent fulfilled requests, reverse chronological. Each row:
     request title, helper name, solve time, date. No icons, no carets.
   - Pushed via `NavigationHelpers.pushWithSlide()`.
   - New viewmodel: `time_to_solve_view_model.dart` —
     `AsyncNotifierProvider.autoDispose` with `ref.mounted` guards.
   - New repository method:
     `ImpactMetricsRepository.getTimeToSolveDetail()` with cache key
     `'impact:time_to_solve:$communityId'`.

5. Remove old QT pills (Quality Time, Problems Solved, Time Saved) and
   their detail screen navigation from the QT tab.

**Verification:** Money tab shows LIBRARY/UTILIZED/PERCENTILE. QT tab shows
HRS/PERSON/TIME-TO-SOLVE/PERCENTILE. `flutter test --reporter expanded`.

### Phase 4 — Percentile system (server + client)

**Goal:** Wire up the percentile pill on all three tabs with real data and
detail screens.

**Server changes:**

Server handler in `server/services/impact_metrics/percentile.go`. Use
`logging.LoggerWithContext(ctx)` for structured logging. The cross-community
query must use `LIMIT` and avoid loading full metric responses for all
communities — query only the aggregate field needed for ranking.

- New computation: for each dimension (money, co2, quality_time), query all
  communities' aggregate metric, sort, compute this community's percentile
  (0–100). Cache server-side with a 1-hour TTL.
- Add `percentile_money`, `percentile_co2`, `percentile_quality_time`
  (int32) fields to `GetCommunityImpactMetricsResponse`.
- New RPC: `GetCommunityPercentileDetail` with dedicated request/response
  types:
  ```protobuf
  message GetCommunityPercentileDetailRequest {
    string community_id = 1;
    ImpactMetricDimension dimension = 2;
  }
  message GetCommunityPercentileDetailResponse {
    int32 current_percentile = 1;
    repeated PercentileDataPoint history = 2;
    repeated RankedCommunity top_communities = 3;
    int32 this_community_rank = 4;
    string this_community_value = 5;
  }
  message PercentileDataPoint {
    int64 timestamp_unix_sec = 1;
    int32 percentile = 2;
  }
  message RankedCommunity {
    int32 rank = 1;
    string community_name = 2;
    string formatted_value = 3;
    bool is_this_community = 4;
  }
  ```
- Percentile history: store monthly snapshots so the detail chart can show
  position over time. Run a monthly job or compute on-demand from historical
  metric snapshots.

**Client changes:**
- New detail screen: `PercentileDetailScreen` (new file):
  - Hero: ordinal percentile (e.g., "92nd")
  - Label: "among all communities" + dimension name
  - Chart: percentile over time (line chart, y-axis 0–100)
  - Table: top 10 communities + this community's position (highlighted
    row). Each row: rank, community name, formatted metric value. No icons,
    no carets.
- Wire PERCENTILE pill on all 3 tabs to `PercentileDetailScreen` with the
  appropriate dimension.
- Replace "(coming)" placeholder with actual percentile value.

**Verification:** All three tabs show live percentile. Tap → detail screen
with chart and top 10 list. `flutter test --reporter expanded`.

### Phase 5 — Leaderboard (server + client)

**Goal:** Per-tab leaderboards with tappable user drill-downs.

**Server changes:**

Server handlers go in new focused files per server architecture (short,
focused files < 500 lines): `server/services/impact_metrics/leaderboard.go`
and `server/services/impact_metrics/user_impact_detail.go`. Use
`logging.LoggerWithContext(ctx).With("operation", "GetCommunityLeaderboard")`
for structured logging. All queries must use `LIMIT` and batch patterns
(no N+1). Auth: `auth.RequireAuth(ctx)` at start of every handler.

- New RPC: `GetCommunityLeaderboard` with dedicated types:
  ```protobuf
  message GetCommunityLeaderboardRequest {
    string community_id = 1;
    ImpactMetricDimension dimension = 2;
    int32 limit = 3;
  }
  message GetCommunityLeaderboardResponse {
    repeated LeaderboardMember members = 1;
    optional LeaderboardMember calling_user = 2;
  }
  message LeaderboardMember {
    int32 rank = 1;
    string user_id = 2;
    string display_name = 3;
    optional string media_id = 4;
    string formatted_value = 5;
  }
  ```
- Computation: for each user in the community, sum their `ImpactEstimate`
  values for the given dimension across all transactions where they
  participated (as owner, borrower, helper, or attendee). Use a single
  aggregation query with GROUP BY user_id, not N+1.
- Include the calling user's rank even if they're not in the top N.

- New RPC: `GetUserCommunityImpactDetail` with dedicated types:
  ```protobuf
  message GetUserCommunityImpactDetailRequest {
    string community_id = 1;
    string user_id = 2;
    ImpactMetricDimension dimension = 3;
  }
  message GetUserCommunityImpactDetailResponse {
    string display_name = 1;
    optional string media_id = 2;
    string formatted_total = 3;
    repeated TimeSeriesPoint cumulative_trend = 4;
    repeated UserImpactTransaction transactions = 5;
  }
  message UserImpactTransaction {
    string transaction_id = 1;
    string item_name = 2;
    string transaction_type = 3;
    string role = 4;
    int64 completed_at_unix_sec = 5;
    string formatted_value = 6;
  }
  ```

**Client changes:**
1. New screen: `community_leaderboard_screen.dart`
   - Title: "Top Contributors"
   - 10 rows (or fewer if community is small). Each row: rank badge, avatar
     (CachedMediaImage with `ImageCacheKeys.thumbnail(mediaId)`), name,
     formatted value on the right.
   - If calling user is not in top 10, show a divider + "You" row with
     their rank highlighted.
   - Entire row tappable → `UserCommunityImpactScreen`.
   - Pushed via `NavigationHelpers.pushWithSlide()` (utility list screen).

2. New tappable section header on each tab: "Top Contributors >" below the
   breakdown rows. Taps to `CommunityLeaderboardScreen` for the current
   dimension.

3. New screen: `user_community_impact_screen.dart`
   - Hero: user avatar (CachedMediaImage, thumbnail cache key) + name +
     total formatted value.
   - Chart: cumulative impact over time (line chart).
   - Table: transactions in reverse chronological order. Each row: item
     name, type pill, role label, date, value. No icons, no carets.
   - Must use `SwipeToCloseMixin` and be pushed via
     `NavigationHelpers.pushScreen()` (primary content screen).
   - Uses `autoDispose` provider with `ref.mounted` guards after async.

4. New viewmodel: `community_leaderboard_view_model.dart` —
   `AsyncNotifierProvider.autoDispose.family` keyed by
   `(communityId, dimension)`. Calls repository, never service directly.
   New viewmodel: `user_community_impact_view_model.dart` —
   `AsyncNotifierProvider.autoDispose` for the detail screen.

**Verification:** Each tab shows top 5 contributors. Tap user → detail
screen with chart and transaction list. `flutter test --reporter expanded`.

### Phase 6 — Tests and cleanup

**Goal:** Comprehensive test coverage for all new features.

**Server tests:**
- Percentile computation: verify ordering, edge cases (1 community, tied
  values)
- Leaderboard: verify ranking, multiple participation roles counted,
  calling user inclusion
- Time-to-solve: verify median computation, edge cases (no fulfilled
  requests)
- CO₂ potential: verify sum computation

**Client tests:**
- Phase 1 design changes: hero label layout, tab styling, table rows
  without icons
- Percentile pill + detail screen: tap handler, chart, top 10 table
- Leaderboard widget: rendering, tap navigation, "You" row
- CO₂ forest card on main tab
- New detail screens: HrsPerPerson, TimeToSolve, Co2Potential
- Pill drill-down invariant: verify all new pills open correct detail
  screens

**Cleanup:**
- Remove dead code from removed pills (Actions on Money, Total CO₂,
  Per Item, Quality Time/Problems Solved/Time Saved on QT)
- Delete unused detail screens or screen code paths
- **i18n:** Add new ARB keys to BOTH `app_en.arb` AND `app_es.arb`:
  `impactPillPercentile`, `impactPillPotential`, `impactPillHrsPerPerson`,
  `impactPillTimeToSolve`, `impactLeaderboardTitle`,
  `impactPercentileLabel`, `impactCo2PotentialLabel`,
  `impactHrsPerPersonLabel`, `impactTimeToSolveLabel`.
  Remove deprecated keys: `impactPillActions`, `impactPillTotalCo2`,
  `impactPillPerItem`, `impactPillQualityTime`, `impactPillProblemsSolved`,
  `impactPillTimeSaved`.
  All user-visible strings in new screens must use `context.l10n.*` — no
  hardcoded strings in Dart. Viewmodels must expose typed state (enums,
  data objects), not resolved strings.

---

## Updated Metrics Reference

After all phases, the canonical secondary metrics are:

### Stat Pills (3 per tab)

| Tab | Pill 1 | Pill 2 | Pill 3 |
|-----|--------|--------|--------|
| SAVED | **LIBRARY** — total library value ($) | **UTILIZED** — utilization % | **PERCENTILE** — cross-community rank |
| CO₂ AVOIDED | **POTENTIAL** — total library CO₂ potential (kg) | **TREES/YR** — kg / 20 | **PERCENTILE** — cross-community rank |
| QUALITY TIME | **HRS/PERSON** — avg QT per total member | **TIME-TO-SOLVE** — median request→resolution time | **PERCENTILE** — cross-community rank |

### Pill Drill-Down Content

| Pill | Taps to | Content shown |
|------|---------|---------------|
| LIBRARY | `MetricDetailScreen` category `'LibraryValue'` | Library items sorted by dollar value descending. Rows: item name, sharer name, loan count, dollar value. Tappable → item screen. |
| UTILIZED | `UtilizationDetailScreen` | Utilization over time chart, most utilized items, untapped potential, redundancy groups. |
| POTENTIAL | `Co2PotentialDetailScreen` | Library items ranked by potential CO₂ savings (most to least). Rows: item name, sharer name, potential kg. Methodology link. |
| TREES/YR | `TreesDetailScreen` | Tree growth chart, "Your forest is..." landmark card, CO₂ equivalences. |
| HRS/PERSON | `HrsPerPersonDetailScreen` | Average hours per person. Breakdown by 7 QT attributes: duration, modality, group_size, tie_strength, reciprocity, novelty, vulnerability. |
| TIME-TO-SOLVE | `TimeToSolveDetailScreen` | Median solve time. Trend chart. Recent fulfilled requests with individual solve times. |
| PERCENTILE (all tabs) | `PercentileDetailScreen` | Percentile over time chart. Top 10 communities + this community's position (highlighted). |

### Source Breakdown Rows

| Tab | Rows shown |
|-----|-----------|
| SAVED | Loans, Giveaways, Requests, Events |
| CO₂ AVOIDED | Loans, Giveaways, Requests, Events |
| QUALITY TIME | Loans, Giveaways, Requests, Events |

### Additional Tab Content

| Tab | Extra content (between chart and breakdown) |
|-----|---------------------------------------------|
| CO₂ AVOIDED | "Your forest is..." landmark comparison card |

### Leaderboard (tappable section header, all tabs)

"Top Contributors >" section header below breakdown rows on each tab. Taps
to a dedicated leaderboard screen showing top 10 members by tab metric.
Each member row tappable → user community impact detail (longitudinal chart
+ transaction list).

### Detail Screen Table Style (all screens)

- Rows show text only: name, value, optional secondary text
- No icons, no chevron carets
- Tappable rows navigate without visual arrow affordance
- Trees equivalence rows keep their illustrative icons (exception)

---

## Audit Notes

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, and `docs/server/architecture.md` on 2026-04-01.
Changes made in-place:

1. **Hardcoded colors (architecture.md violation).** S7 and Phase 1 originally
   specified `Color(0xFFB0A898)` for unselected tab labels. Fixed: must use a
   new `AppColors.tabLabelInactive(context)` semantic color helper, never
   hardcoded hex values. Architecture doc: "Do NOT use hardcoded hex
   `Color(0xFF...)` values for UI elements."

2. **Missing client data layer (architecture.md + caching.md violations).**
   S1, S2, Phases 2–5 originally specified new screens and viewmodels but
   omitted repository methods and cache keys. Fixed: added explicit
   `ImpactMetricsRepository` methods with cache key patterns
   (`'impact:leaderboard:$communityId:$dimension'`,
   `'impact:percentile_detail:$communityId:$dimension'`, etc.) for every new
   data fetch. Architecture rule: "Repositories are the ONLY layer that calls
   Services" and "ViewModels are the ONLY layer that calls Repositories."

3. **Missing navigation patterns (architecture.md violation).** New screens
   did not specify whether to use `pushScreen()` + `SwipeToCloseMixin` or
   `pushWithSlide()`. Fixed: `UserCommunityImpactScreen` uses
   `pushScreen()` + `SwipeToCloseMixin` (primary content screen);
   utility detail screens (Percentile, Co2Potential, HrsPerPerson,
   TimeToSolve) use `pushWithSlide()`.

4. **Missing async safety (architecture.md violation).** New viewmodels and
   screens did not specify `autoDispose` or `ref.mounted` guard requirements.
   Fixed: all new viewmodels use `AsyncNotifierProvider.autoDispose` with
   explicit `ref.mounted` checks after async gaps.

5. **Missing i18n compliance (i18n.md violation).** Phase 6 cleanup only said
   "update ARB keys" without specifying both locales or listing specific keys.
   Fixed: explicit list of new ARB keys to add to BOTH `app_en.arb` and
   `app_es.arb`, deprecated keys to remove, and reminder that viewmodels
   expose typed state (not resolved strings).

6. **Server file organization (server architecture.md).** Phases 3–5 did not
   specify file locations for new server code. Fixed: added explicit file
   paths (`leaderboard.go`, `user_impact_detail.go`, `percentile.go`,
   `time_to_solve.go`) per the "short, focused files" principle.

7. **Server structured logging (CLAUDE.md).** Phases 3–5 server changes did
   not mention logging. Fixed: added `logging.LoggerWithContext(ctx)` pattern
   to each phase's server changes.

8. **SQL efficiency (CLAUDE.md).** Phase 4 percentile computation queries all
   communities but did not specify query constraints. Fixed: added callout to
   query only the aggregate field needed for ranking (not full metric
   responses), use `LIMIT`, and `WHERE` clauses for filtering.

9. **Missing CachedMediaImage usage (CLAUDE.md media display rules).**
   Leaderboard avatars and user detail hero did not specify image caching
   strategy. Fixed: added `CachedMediaImage` with
   `ImageCacheKeys.thumbnail(mediaId)` for avatars per media display rules.

---

**Last Updated:** 2026-04-01
