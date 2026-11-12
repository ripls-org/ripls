# Impact 3.0 — Metrics Screen Consistency & Quality

## Overview

Simplify and unify the three metrics screens (Community, Portfolio, User) so
they share consistent behavior: all metrics tappable, all detail views useful,
all insight text actually insightful. Also fix the user-avatar navigation so
tapping another user's avatar shows the proper `UserMetricsScreen` (not a stale
or incomplete view).

**Screens in scope:**

| Screen | File | Current state |
|--------|------|---------------|
| Community metrics | `community_metrics_screen.dart` | 4-tab bar (Money, CO₂, QT, Time); mixed tappability on stat pills; insight text repeats numbers |
| Portfolio metrics | `portfolio_metrics_screen.dart` | 4-tab bar; all pills tappable; insight text repeats numbers |
| User metrics | `user_metrics_screen.dart` | 4 metric cards; some open `MetricDrillDownScreen`; no tab bar |

**Reference docs:** [impact_metrics.md](../impact_metrics.md),
[impact2.md](./impact2.md)

---

## Problems

### P1 — Fourth tab ("Time Recovered") adds clutter without value

The community and portfolio screens show a fourth tab for Time Saved. This
metric is the weakest of the four — it's a rough estimate of shopping time
avoided and overlaps conceptually with Quality Time. Displaying it as a
co-equal tab dilutes the stronger metrics. The plan is to eventually fold time
savings into Quality Time, but that work isn't ready yet.

### P2 — Inconsistent tappability across metrics

Some stat pills and metric values open detail views; others are dead taps.
Users can't predict which elements are interactive.

**Community screen gaps:**
- Hero value: NOT tappable (portfolio hero IS tappable)
- Chart: NOT tappable on any screen
- Trend card: NOT tappable on any screen

Note: CO₂ pills, QT "Per person" pill, and Money "Utilized" pill are
already tappable in the community screen (fixed in prior work). The
remaining gap is the hero value.

**User metrics screen gaps:**
- Only 4 metric cards; no stat pills or breakdowns at all
- Cards open `MetricDrillDownScreen` which shows formula components — useful
  but inconsistent with community/portfolio detail screens

### P3 — Actions detail shows wrong content

When tapping the "Actions" stat pill on the community screen, the detail view
(`MetricDetailScreen` with `category: 'Actions'`) shows a summary table with
action type counts and formatted totals (e.g., "42 completed — $1,250"). It
does NOT show the individual actions themselves. Users expect to see a list of
actual sharing actions (loans, giveaways, requests, events) with their
associated impact values (dollar savings, CO₂ avoided, etc.).

### P4 — Insight blurbs repeat the numbers

The trend card text and portfolio hook text just restate what's already visible:

- *"65 loans completed across 5 members. Your circle is 16% through its shared
  library's savings potential."* — The loan count and member count are already
  shown as stat pills.
- *"2500 kg of CO₂ avoided — equivalent to 125 trees planted for a year."* —
  The CO₂ value is the hero number and trees is a stat pill.
- Portfolio: *"$12,450 saved across all your communities."* — This IS the hero
  value.

These blurbs take up screen real estate without adding information. They should
surface something the user can't already see: trends, comparisons, benchmarks,
or actionable observations.

### P5 — User avatar tap shows incomplete metrics view

When a user taps another user's avatar (in chat, content views, request cards),
it opens `UserScreen` → `UserMetricsScreen`. This screen shows 4 metric cards
but lacks the tab bar, stat pills, charts, breakdowns, and insight sections
that the community and portfolio screens have. It feels like viewing a different
app. The user metrics screen should match the community/portfolio pattern.

---

## Proposed Solutions

### S1 — Remove Time tab from community and portfolio screens

Remove the 4th tab (Time Recovered / `IMPACT_METRIC_DIMENSION_TIME`) from
`ImpactTabBar` and both screens. The tab bar becomes 3 tabs: Money, CO₂,
Quality Time.

**Files affected:**
- `impact_tab_bar.dart` — Remove time tab, adjust layout to 3-column
- `community_metrics_screen.dart` — Remove all time-dimension cases in
  `_activePills`, `_trendText`, `_buildContentForDimension`, chart builders
- `portfolio_metrics_screen.dart` — Remove time-dimension cases in
  `_pillsFor`, `_portfolioHookText`, chart/breakdown builders
- `community_impact_view_model.dart` — Keep time data available (still
  computed server-side) but don't surface it in tab bar
- `portfolio_view_model.dart` — Same

**Not affected:** Server code, protos, user metrics screen (no tab bar
currently). Time data remains in the API response for future QT integration.

### S2 — Make all stat pills and hero values tappable

Every stat pill on every dimension opens a relevant detail view. Every hero
value is tappable. The detail views use the existing `MetricDetailScreen`
(community) and `PortfolioMetricDetailScreen` (portfolio) infrastructure.

**Community screen changes:**
- Hero value: Wrap in `GestureDetector` → open `CommunityMetricDetailScreen`
  for current dimension (matches portfolio behavior)
- CO₂ pills: All 3 tappable → open `CommunityMetricDetailScreen` for emissions
  dimension
- Money "% Utilized" pill: Tappable → open detail with category
  `'Utilization'`
- Time pills (if retained during transition): All tappable → open detail
- QT "Per person" pill: Tappable → open detail

**User metrics screen changes:** (see S5 for full redesign)

### S3 — Fix Actions detail to show individual actions

Replace the summary table in `MetricDetailScreen` (category `'Actions'`) with
a scrollable list of individual sharing actions. Each row shows:

- Action type icon (loan, giveaway, request, event)
- Item/event name
- Counterparty (borrower/recipient/helper)
- Date
- Impact values: dollar savings, CO₂ avoided (per-transaction from
  `ImpactEstimate`)

**Data source:** The server already stores `ImpactEstimate` on each
transaction. The `GetCommunityMetricDetail` RPC returns `detail_items` with
per-transaction breakdowns. The client needs to render these as individual
action rows instead of aggregating them into a summary table.

**Server changes:** May need a new RPC or parameter on
`GetCommunityMetricDetail` to return individual transaction-level items with
names, dates, and counterparties. Currently the detail RPC returns aggregated
breakdowns by source type, not individual transactions.

**New RPC option:** Add `GetCommunityActions` RPC that returns paginated
transaction-level items with:
- Transaction type (loan, giveaway, request, event)
- Item/event name
- Participants
- Date
- Per-transaction `ImpactEstimate`

Alternatively, add a `list_items: bool` parameter to
`GetCommunityMetricDetail` to switch between aggregated and item-level
responses.

### S4 — Make insight blurbs actually insightful

Replace the current trend card text (which repeats visible numbers) with
computed insights that surface non-obvious information. Categories of useful
insights:

**Trend-based insights** (requires historical data):
- *"Sharing picked up 40% this month compared to last."*
- *"Your most active week was March 10–16 with 12 transactions."*
- *"CO₂ savings have doubled since January."*

**Comparative insights** (requires benchmark data):
- *"Your community saves 3× more per member than average."*
- *"You're in the top 20% of communities by sharing frequency."*

**Behavioral insights** (derivable from existing data):
- *"Your top 3 items account for 60% of total savings."*
- *"Gear loans generate 4× more savings per transaction than events."*
- *"Most sharing happens on weekends — Saturday is your busiest day."*
- *"5 items haven't been shared yet. Listing them could unlock $X in
  potential savings."*

**Seasonal/contextual insights:**
- *"Spring is peak sharing season — last year your community was 2× more
  active in April."*

**Recommendation:** Start with behavioral insights (top items, day patterns,
idle items) and trend-based insights (period-over-period deltas). Skip
comparative insights initially — they require cross-community benchmarking.

**Insight generation is fully server-side.** The server computes
previous-period metrics internally, queries for top items and activity
patterns, ranks candidate insights, and returns 1–2 insight strings per
metric dimension in the RPC response. This keeps logic centralized and lets
us iterate on insight quality without app releases. The client simply
displays the strings.

**i18n note:** Server-generated insight strings are returned in English only
for now. When multi-locale support is needed, the server should return
structured insight data (template key + parameters) and the client resolves
to localized text via `context.l10n` ARB keys. This avoids hardcoding
user-visible strings on the server long-term.

### S5 — Redesign UserMetricsScreen to match community/portfolio pattern

The user metrics screen should adopt the same visual structure as the
community and portfolio screens:

- **Hero section:** User name, metadata, stat chips (items shared, loans,
  borrows, giveaways, events, requests)
- **3-tab bar:** Money, CO₂, Quality Time (same `ImpactTabBar` widget)
- **Stat pills:** 3 per dimension, all tappable
- **Chart:** Trend over time
- **Breakdown rows:** By source (Loans, Giveaways, Requests, Events), tappable
- **Insight card:** Research citation

**Data:** The `GetUserImpactMetrics` RPC already returns aggregated metrics.
May need to add:
- Source breakdown (by transaction type)
- Chart data (time-series)
- Period-over-period for insights

**View model changes:** `UserImpactViewModel` needs to expose the same data
shape as `CommunityImpactViewModel` — stat pill values, chart points,
breakdown rows, insight text.

This also fixes P5 (avatar tap) because `UserScreen` already renders
`UserMetricsScreen` — making that screen consistent automatically fixes the
navigation.

### S6 — User avatar navigation (already correct, just needs S5)

The current flow is: avatar tap → `UserScreen` → `UserMetricsScreen`. This
path is correct. The problem is that `UserMetricsScreen` is too sparse. Once
S5 lands, the avatar tap experience is fixed for free.

---

## Decisions

1. **Actions list pagination:** Paginate with 25-item pages, most recent first.

2. **Insight generation — server-side.** The server computes and returns
   insight text strings. This keeps the logic centralized, avoids duplicating
   metric analysis on the client, and lets us iterate on insight quality
   without app releases.

3. **User metrics — own vs. others:** Show the same high-level structure
   (hero, tabs, stat pills, chart, breakdowns by source type) for all users.
   Do NOT allow drilling into another user's individual transaction details —
   only show aggregates. The client disables detail navigation when viewing
   someone else's profile.

4. **Tab bar widget reuse:** Yes — extract a shared `MetricTabBar` widget
   used by all three screens.

5. **Time metric data in QT:** No changes yet. Keep QT focused on social
   connection. Time integration is a separate future effort.

6. **Methodology link placement:** Yes — add to all detail views for
   consistency and transparency.

---

## Phased Implementation Plan

### Phase 1 — Remove Time tab (community + portfolio)

**Goal:** Simplify both screens from 4 tabs to 3.

**Changes:**
- `impact_tab_bar.dart`: Remove the Time tab. Update layout from 4-column to
  3-column. Update any dimension-to-index mapping.
- `community_metrics_screen.dart`: Remove all `IMPACT_METRIC_DIMENSION_TIME`
  switch cases in `_activePills`, `_listedPills`, `_trendText`,
  `_buildContentForDimension`, chart builders. Remove Time-specific stat pill
  definitions.
- `portfolio_metrics_screen.dart`: Remove Time dimension from `_pillsFor`,
  `_portfolioHookText`, chart builder, breakdown builder.
- `metric_selector_bar.dart`: If used elsewhere, remove Time option.
- Update tests for community and portfolio screens that reference the Time tab.

**Verification:** `flutter test --reporter expanded` — all existing tests pass
with Time tab removed.

### Phase 2 — Make hero values tappable (community + portfolio)

**Goal:** Hero value opens a detail view on all screens.

**Changes:**
- `community_metrics_screen.dart`:
  - Wrap hero value in `GestureDetector` → `CommunityMetricDetailScreen`
    for current dimension (matches portfolio behavior which already does
    this).
- All stat pills are already tappable on community and portfolio screens
  (CO₂, QT "Per person", Money "Utilized" — confirmed in current code).
- Add widget tests for the hero value tap handler.

**Verification:** Tap hero value on community screen → detail screen opens.

### Phase 3 — Fix Actions detail view

**Goal:** Show individual sharing actions with per-transaction impact values.

**Server changes:**
- Add `GetCommunityActions` RPC to `ImpactMetricsService` in
  `proto/ripls/api/impact_metrics_service.proto`. Each RPC gets its own
  dedicated request/response types (per CLAUDE.md: never reuse RPC
  request/response messages across RPCs):
  ```protobuf
  message GetCommunityActionsRequest {
    string community_id = 1;
    optional string page_token = 2;
    int32 page_size = 3;
  }
  message GetCommunityActionsResponse {
    repeated ActionItem items = 1;
    optional string next_page_token = 2;
  }
  message ActionItem {
    string transaction_id = 1;
    string transaction_type = 2;
    string item_name = 3;
    string counterparty_name = 4;
    optional string counterparty_user_id = 5;
    int64 completed_at_unix_sec = 6;
    ImpactEstimate impact = 7;
  }
  ```
- Pagination: `page_token` / `next_page_token` pattern, 25 items per page.
- Server handler in `server/services/impact_metrics/actions.go` (separate
  file per server architecture: short, focused files).
- Query must use `LIMIT` and avoid N+1 (batch-load item names and
  counterparty names via `IN` clauses, not per-row queries).

**Client changes:**
- New service method: `ImpactMetricsService.getCommunityActions()`.
- New repository method: `ImpactMetricsRepository.getCommunityActions()`
  with cache key `'impact:actions:$communityId'`.
- New viewmodel (or extend existing): exposes actions list state. The
  screen reads actions via the viewmodel, not the repository directly
  (per architecture rule: screens only interact with viewmodels).
- `metric_detail_screen.dart`: When `category == 'Actions'`, replace
  `_buildActionsTable()` with a `ListView` of `ActionItemRow` widgets.
- New widget: `action_item_row.dart` — shows type icon, item name,
  counterparty, date, primary impact value (dollar savings).
- Rows are tappable → navigate to the item/event screen via
  `NavigationHelpers.pushToItemScreen()`.

**Verification:** Tap Actions pill → see list of individual transactions with
dollar values.

### Phase 4 — Insight text overhaul

**Goal:** Replace number-repeating blurbs with genuinely insightful text.
Insight generation is fully server-side so we can iterate without app releases.

**Server changes:**
- New insight generator in `server/impact_metrics/insights.go`:
  - Computes previous-period metrics (prior 30-day window) internally
  - Queries top items by total impact, busiest day of week, idle item count
  - Ranks candidate insights by "interestingness" (largest delta, most
    surprising pattern)
  - Returns 1–2 insight strings per metric dimension
  - All queries must use `LIMIT` and batch patterns (no N+1).
  - Use structured logging: `logging.LoggerWithContext(ctx).With("operation",
    "GenerateInsights")`.
- Add `repeated string insights` field to the existing
  `GetCommunityImpactMetricsResponse` and `GetUserImpactMetricsResponse`
  messages (new fields on existing responses — do NOT create new RPCs for
  this, since the insights are part of the same metrics fetch).
- Insight templates (deterministic, not AI-generated):
  1. Large trend delta: *"Sharing is up 40% this month."*
  2. Top item concentration: *"Your tent accounts for 45% of total savings."*
  3. Day pattern: *"Saturdays are your community's sharing day."*
  4. Idle items: *"8 items haven't been shared yet — they could unlock
     $2,400."*
  5. Fallback: *"Your community has saved the equivalent of 12 trees this
     year."* (anchoring comparison, used only when no trend data exists)
- Unit tests for insight generation with various metric scenarios.

**Client changes:**
- `community_metrics_screen.dart`: Replace `_trendText()` with server-provided
  insight string from response.
- `portfolio_metrics_screen.dart`: Replace `_portfolioHookText()` with
  server-provided insight string.
- `impact_hook.dart`: Keep hero hook text (anchoring comparisons are useful
  and distinct from trend insights). These are not the problem — the trend
  card text is.
- Remove client-side trend text generation logic.
- Update viewmodels to expose insight strings from repository data. Screens
  read insight text from the viewmodel, never from the repository directly.
- Update tests to verify insight strings from server are displayed.

**i18n note:** Initially the server returns English strings and the client
displays them directly. When Spanish locale support is needed for insights,
migrate to structured insight data (template key + parameters) with ARB
keys in `app_en.arb` / `app_es.arb`. See S4 note above.

### Phase 5 — Redesign UserMetricsScreen

**Goal:** User metrics screen matches community/portfolio visual structure.

**Server changes:**
- Extend `GetUserImpactMetricsResponse` with new fields (adding fields to
  an existing response is wire-compatible):
  - Source breakdown (loans, giveaways, requests, events)
  - Time-series chart data
  - `repeated string insights` (server-generated, same as Phase 4)
- Server handler logic in a new file `server/services/impact_metrics/user_metrics.go`
  (keep files short and focused per server architecture).
- The RPC must enforce the own-vs-others privacy boundary: when the
  requesting user differs from the target user, omit transaction-level
  detail fields from the response. The client should not be the only
  enforcement point.

**Client changes:**
- `user_metrics_screen.dart`: Full rewrite to match the portfolio metrics
  screen exactly. The user metrics screen (whether viewing your own or
  another user's profile) should be visually identical to the portfolio
  metrics screen:
  - Hero section (user info + stat chips)
  - 3-tab bar: SAVED, CO₂, SHARED (same `MetricTabBar` widget)
  - Hero value labels: "saved by sharing instead of buying", "CO₂ avoided
    by sharing", "minutes of quality time shared"
  - Stat pills must match the canonical set (see Metrics Reference in
    `docs/impact_metrics.md`):
    - Money: LIBRARY, UTILIZED, ACTIONS
    - CO₂: TOTAL CO₂, TREES/YR, CARS EQUIV
    - QT: QUALITY TIME, PER PERSON, MEMBERS
  - Chart (line/bar depending on dimension)
  - Source breakdown rows (Loans, Giveaways, Requests, Events)
  - Insight card (server-provided insight string)
  - **Own profile:** All pills and breakdown rows are tappable, opening
    detail screens with drill-down.
  - **Other user's profile:** Exact same layout and content, but NO
    tap handlers on pills or breakdown rows. Display-only, no drill-down.
  - Navigation: detail screens pushed via `NavigationHelpers.pushScreen()`
    with `SwipeToCloseMixin` (per architecture navigation standards).
- `user_impact_view_model.dart`: Extend to expose stat pill values, chart
  data, breakdown rows — same shape as community view model. The viewmodel
  is the only layer that calls the repository; the screen reads state via
  `ref.watch()` and triggers actions via `ref.read().notifier`.
- Delete or repurpose `MetricDrillDownScreen` (formula-component view) — may
  keep as a secondary drill-down from the detail screen.
- Add tests for new user metrics screen layout and interactions.
- Add i18n keys for any new user-visible strings in `app_en.arb` and
  `app_es.arb`.

**Verification:** Tap user avatar → see full metrics screen with tabs, pills,
charts, breakdowns matching portfolio UX exactly.

### Phase 6 — Shared MetricTabBar widget

**Goal:** Extract a reusable tab bar widget used by all three screens.

**Changes:**
- New widget: `metric_tab_bar.dart` — 3-column tab bar accepting metric
  labels, values, colors, and `onSelected` callback.
- Refactor `impact_tab_bar.dart` to use (or replace with) `MetricTabBar`.
- Refactor portfolio screen's inline tab builder to use `MetricTabBar`.
- Wire into user metrics screen.
- Update all tests.

**Note:** This phase can be done earlier (during Phase 1) or later as cleanup.
Listed last because it's a refactor, not a user-facing change.

### Phase 7 — Consistency fixes and regressions

**Goal:** Ensure all three metrics screens use identical tab labels, hero
labels, stat pills, and drill-down content. Fix the portfolio inbox metric
ring order. Fix detail screen content.

**Problem:** The three screens have drifted:
- User metrics screen pills don't match community/portfolio (e.g., Money
  tab shows MONEY SAVED / LIBRARY / ACTIONS instead of LIBRARY / UTILIZED /
  ACTIONS; QT tab shows ACTIONS instead of PER PERSON).
- Hero value labels must always show the dimension name: "saved by sharing
  instead of buying", "CO₂ avoided by sharing", "minutes of quality time
  shared".
- Portfolio inbox metric rings are ordered Social Time → Money Saved → CO₂
  Avoided, which doesn't match the tab bar order (SAVED → CO₂ → SHARED).

**Canonical metric definitions:** See the Metrics Reference section added to
`docs/impact_metrics.md`. All screens must conform to this reference.

**Changes:**

1. **Stat pills — align all three screens to the canonical set:**

   | Tab | Pill 1 | Pill 2 | Pill 3 |
   |-----|--------|--------|--------|
   | SAVED (Money) | LIBRARY (library value) | UTILIZED (utilization %) | ACTIONS (total action count) |
   | CO₂ (Emissions) | TOTAL CO₂ (kg) | PER ITEM (kg / item count) | TREES/YR (kg / 20) |
   | SHARED (QT) | QUALITY TIME (total rm) | PROBLEMS SOLVED (loans + requests + giveaways completed) | TIME SAVED (hours saved) |

   - `user_metrics_screen.dart`: Change Money pill 1 from "MONEY SAVED" →
     "LIBRARY"; replace QT pills 2–3 with PROBLEMS SOLVED / TIME SAVED.
   - `portfolio_metrics_screen.dart`: Change Money pill 1 from "MONEY
     SAVED" → "LIBRARY"; replace QT pills 2–3 with PROBLEMS SOLVED / TIME SAVED.
   - `community_metrics_screen.dart`: Replace QT pills 2–3 with PROBLEMS
     SOLVED / TIME SAVED.

2. **Hero value labels — verify all three screens use `ImpactHook.labelFor()`:**
   - Community: already uses it.
   - Portfolio: already uses it.
   - User: verify it uses `ImpactHook.labelFor()` and not custom strings.

3. **Portfolio inbox metric ring order:**
   - `metric_rings_row.dart`: Reorder from
     `[social_time, money_saved, co2_avoided]` to
     `[money_saved, co2_avoided, social_time]` to match tab bar order
     (SAVED → CO₂ → SHARED).

4. **Library Value detail screen content:**
   - `metric_detail_screen.dart` (category `'LibraryValue'`): Currently
     shows items sorted by dollar value with name, sharer, loan count, and
     value. This is correct — verify it remains a full list of library items
     with dollar values.

5. **Actions detail screen content:**
   - Already fixed in Phase 3 to show individual sharing actions with
     per-transaction impact values. Verify each action row shows: type icon,
     item name, counterparty, date, and the associated dollar savings value.

6. **Update tests** for any pill label changes and ring reordering.

**Verification:** All three metrics screens show identical tab labels, hero
labels, and stat pill labels. Portfolio inbox rings match tab order. Detail
screens show correct content per the Metrics Reference.

### Phase 8 — Secondary pill drill-downs always match the secondary metric

**Goal:** Every secondary pill (pill 2 and pill 3) opens a detail screen whose
title and hero value correspond to that specific metric. Tapping TREES/YR must
show a trees screen. Tapping PROBLEMS SOLVED must show problems, not generic QT
minutes. Currently several pills open the wrong (aggregate) detail screen.

#### Invariant

> A pill labeled X must open a screen titled X whose hero value is the same
> number shown on the pill.

#### Pills that currently violate this (and their fixes)

**PER ITEM** → currently opens `CommunityMetricDetailScreen` for emissions
(shows total CO₂, not per-item). Fix: new `PerItemCO2DetailScreen`:
- Title: "CO₂ per Item"
- Hero value: `totalCO₂kg / itemCount` formatted as kg
- Content: library items ranked by their individual embodied carbon estimate
  (highest CO₂ first). Each row: item name, category, kg CO₂, sharer name.
  Tappable → item screen. Methodology link at bottom.
- New file: `app/lib/presentation/screens/impact_metrics/per_item_co2_detail_screen.dart`
- No new server RPC — gear `carbonEstimate.co2eGrams` already in the
  `GetCommunityMetricDetail` response's `topItems`.

**TREES/YR** → currently opens generic emissions aggregate. Fix: new
`TreesDetailScreen` (full spec in Phase 9).

**PROBLEMS SOLVED** → new pill (replacing PER PERSON). New `ProblemsSolvedDetailScreen`:
- Title: "Problems Solved"
- Hero value: total count (loans completed + requests fulfilled + giveaways
  completed)
- Content: type breakdown chips (N loans · N requests · N giveaways), then
  paginated list of completed actions (item name, helper name, date). Tappable
  → item screen.
- New file: `app/lib/presentation/screens/impact_metrics/problems_solved_detail_screen.dart`
- Server: the count is derivable from existing `sourceBreakdown` in
  `GetCommunityMetricDetail` (no new RPC). The item list reuses the
  `GetCommunityActions` RPC from Phase 3, filtered to completed transactions.

**TIME SAVED** → new pill (replacing MEMBERS on QT tab). Reuses the data
from the removed Time tab dimension:
- Title: "Time Saved"
- Hero value: total hours saved, formatted (e.g., "168 hrs")
- Content: line chart over time, source breakdown (Loans / Giveaways /
  Requests / Events), methodology link.
- New file: `app/lib/presentation/screens/impact_metrics/time_saved_detail_screen.dart`
- Data: `timeBankedMinutes` already in the `GetCommunityImpactMetrics`
  response. The Time dimension was removed from the tab bar but the data
  still flows — this screen surfaces it as a detail, not a top-level tab.

#### Pills that are already correct (no change needed)

| Pill | Detail screen title | Hero value |
|------|---------------------|------------|
| LIBRARY | "Library Value" | total library $ | ✓ |
| UTILIZED | "Utilization" | utilization % | ✓ |
| ACTIONS | "Actions" | total action count | ✓ |
| TOTAL CO₂ | "CO₂ Avoided" | total kg | ✓ |
| QUALITY TIME | "Quality Time" | total rm | ✓ |

#### ARB keys to add

```
impactPillProblemsSolved  →  "PROBLEMS SOLVED"
impactPillTimeSaved       →  "TIME SAVED"
```

Remove: `impactPillPerPerson`, `impactPillMembers`, `impactPillCarsEquiv`.

#### Test requirement

Add a widget test `pill_drill_down_invariant_test.dart` that:
1. For each screen (community, portfolio, user), for each pill (pill 2 and
   pill 3 on each tab), simulates a tap.
2. Asserts the detail screen that opens has a title matching the pill label
   and a hero value matching the pill value.
3. Test is parameterized over all 9 secondary pills (3 tabs × 3 pills,
   minus pill 1 which is the aggregate and already tested elsewhere).

**Verification:** Tap every secondary pill → detail screen title matches pill
label, hero value matches pill value. All 9 invariant tests pass.

---

### Phase 9 — CO₂ detail: playful Trees screen + replace CARS EQUIV pill

**Goal:** Make the Trees/yr pill open a fun, engaging detail screen with
real-world comparisons. Replace the CARS EQUIV pill with a more relatable
secondary metric.

#### Part A — Trees detail screen

When the user taps the TREES/YR pill, open a dedicated detail screen (not
the generic `CommunityMetricDetailScreen`) with:

1. **Tree growth chart:** Longitudinal line chart showing cumulative
   trees-equivalent over time (one data point per completed transaction,
   y-axis = cumulative kg / 20). This makes the tree count feel alive and
   growing.

2. **"Your forest is..." comparison card:** A static lookup table maps the
   community's current tree count to a real-world landmark. Show an
   illustration/icon, the landmark name, and a short description. The
   comparisons escalate playfully:

   | Trees | Comparison |
   |-------|-----------|
   | < 1 | "A seedling — every share plants roots." |
   | 1 | "A single oak in your front yard." |
   | 5 | "A row of street trees on your block." |
   | 10 | "A backyard orchard." |
   | 25 | "A community garden's tree canopy." |
   | 50 | "A small city park." |
   | 100 | "A neighborhood woodland." |
   | 250 | "A section of Golden Gate Park." |
   | 500 | "A small nature reserve." |
   | 1,000 | "A forest the size of 14 football fields." |
   | 2,500 | "A stretch of Muir Woods." |
   | 5,000 | "A corner of Yellowstone." |
   | 10,000 | "A patch of the Amazon rainforest." |

   Lookup picks the largest threshold ≤ the community's tree count.

3. **Other CO₂ equivalences section:** A list of additional comparisons
   computed from the community's total CO₂ avoided (kg). Each row shows an
   icon, the equivalence value, and a label:

   - **Miles not driven** — `kg / 0.4` (avg passenger car emits ~0.4
     kg CO₂/mile, EPA). E.g., "3,200 miles not driven — that's LA to New
     York."
   - **Cow burps avoided** — `kg / 6.8` days of cow methane
     (~2,500 kg CO₂e/year per cow ÷ 365). E.g., "Spared 147 days of cow
     burps."
   - **Flights not taken** — `kg / 250` (one-way domestic flight ≈ 250 kg
     CO₂ per passenger, ICCT). E.g., "2.4 cross-country flights skipped."
   - **Cheeseburgers not made** — `kg / 3.6` (one cheeseburger ≈ 3.6 kg
     CO₂e, Poore & Nemecek 2018). E.g., "167 cheeseburgers worth of
     emissions."
   - **Smartphones charged** — `kg * 125` (1 kg CO₂ ≈ 125 full charges,
     EPA). E.g., "75,000 phone charges."

   All conversion factors are deterministic constants defined client-side
   (no server changes). Citations stored alongside for methodology link.

4. **Methodology link** at the bottom.

**New file:** `app/lib/presentation/screens/impact_metrics/trees_detail_screen.dart`

**Data:** Uses the existing `CommunityImpactMetrics` (or `UserImpactMetrics`)
response — `carbonSavingsGrams` and chart data are already available. No new
server RPC needed.

#### Part B — Replace CARS EQUIV with PER ITEM, move TREES to pill 3

**Decision:** Remove CARS EQUIV. Replace with PER ITEM (`totalCO₂ / itemCount`)
as pill 2, move TREES/YR to pill 3. PER ITEM shows the average carbon
efficiency per shared item — actionable signal about library composition
rather than an abstract car equivalence.

**New CO₂ pill order:**

| Pill 1 | Pill 2 | Pill 3 |
|--------|--------|--------|
| **TOTAL CO₂** — total kg avoided | **PER ITEM** — kg / item count | **TREES/YR** — kg / 20 |

**Changes:**
- `community_metrics_screen.dart`, `portfolio_metrics_screen.dart`,
  `user_metrics_screen.dart`: Update emissions pill definitions — replace
  CARS EQUIV with PER ITEM, reorder so TREES/YR is pill 3.
- Add ARB key `impactPillPerItem` ("PER ITEM") to `app_en.arb` and
  `app_es.arb`. Remove `impactPillCarsEquiv`.
- PER ITEM pill taps to new `PerItemCO2DetailScreen` (see Phase 8).
- TREES/YR pill taps to the new `TreesDetailScreen` (Part A, this phase).
- `docs/impact_metrics.md` Metrics Reference already updated.

---

## Recommendations

1. **Phase 1–2 first** — quick wins that improve UX immediately with no server
   changes.
2. **Phase 4 before Phase 3** — insight text is a bigger UX improvement than
   the actions list, and the server changes for insights (previous period
   metrics) are simpler than the actions list RPC.
3. **Phase 5 last** — the user metrics redesign is the largest change and
   benefits from having the shared patterns established in Phases 1–4.
4. **Keep Time metric data flowing** — don't remove Time from server responses
   or protos. Just hide the tab. Future QT integration will use it.
5. **Don't over-engineer insights** — start with 4–5 deterministic templates
   and a simple priority ranking. Avoid AI-generated insight text for now.
6. **Test each phase independently** — each phase should leave all tests
   passing before moving to the next.

---

## Audit Notes

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, and `docs/server/architecture.md` on 2026-03-31.
Changes made in-place:

1. **Proto RPC reuse (CLAUDE.md violation).** Phase 3 originally proposed
   adding `list_items: bool` to `GetCommunityMetricDetail` or reusing its
   response type. Fixed: `GetCommunityActions` now has its own dedicated
   request/response messages per the rule "never reuse RPC request or
   response message types across multiple RPCs."

2. **Client layer violations (architecture.md).** Phase 3 had the screen
   reading actions directly without specifying a viewmodel or repository.
   Fixed: added explicit service → repository → viewmodel → screen data flow
   for the actions list. Screens only interact with viewmodels.

3. **N+1 and LIMIT (CLAUDE.md SQL efficiency).** Phases 3 and 4 involve new
   server queries. Added explicit callouts to use batch `IN` queries (not
   per-row), and `LIMIT` on all new queries.

4. **Navigation pattern (architecture.md).** Phase 5 did not specify which
   navigation helper to use for detail screen pushes. Fixed: detail screens
   use `NavigationHelpers.pushScreen()` + `SwipeToCloseMixin`, and action
   rows use `NavigationHelpers.pushToItemScreen()`.

5. **i18n compliance (i18n.md).** Server-side insight strings are
   user-visible text returned as raw English strings, which violates the
   principle that user-visible strings belong in ARB files. Added i18n notes
   to S4 and Phase 4 documenting the tradeoff: English-only is acceptable
   for the initial implementation, but the migration path to structured
   insight data + ARB keys is documented for when Spanish locale support is
   needed. Phase 5 adds a reminder to create ARB keys for new UI strings.

6. **Structured logging (CLAUDE.md).** Phase 4 server changes now specify
   using `logging.LoggerWithContext(ctx)` per logging standards.

7. **Server file organization (server architecture.md).** Phase 5 server
   changes specify a new focused file (`user_metrics.go`) rather than
   appending to an existing file, per the "short, focused files" principle.

8. **Server-side privacy enforcement.** Phase 5 originally only enforced
   own-vs-others privacy on the client (disabling tap handlers). Added
   server-side enforcement: the RPC omits transaction-level detail when the
   requesting user differs from the target user. Defense in depth.

9. **Metrics consistency (Phase 7).** Added Phase 7 to align all three
   screens to a canonical pill/label set defined in `docs/impact_metrics.md`.
   User metrics screen pills were drifting (MONEY SAVED instead of LIBRARY,
   ACTIONS instead of PER PERSON on QT tab). Portfolio inbox ring order was
   wrong. Added Metrics Reference section to `docs/impact_metrics.md` as the
   single source of truth for all screen labels and drill-down content.

---

**Last Updated:** 2026-03-31
