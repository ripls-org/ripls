---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Community impact metrics — activity counts, per-transaction savings (money, carbon, time, quality time) from a config-driven estimator with uncertainty propagation, quadrature aggregation, provenance tracking, and methodology docs.
  globs: [server/impact_metrics/**, app/lib/presentation/screens/impact_metrics/**, app/lib/presentation/widgets/impact/**, proto/ripls/api/impact_estimate.proto]
  triggers: [impact, savings, estimator, money-saved, emissions-prevented, time-saved, quality-time, uncertainty, quadrature, provenance, methodology]
  lens: [domain, server, client]
  domain: impact
freshness:
  verified_commit: "f263dbd79"
  verified_on: "2026-06-17"
---
# Community Impact Metrics

## Overview

The Ripls impact system measures community health, activity, and the tangible benefits of sharing. It combines **activity counts**, **estimated savings** (cost, carbon, time, quality time), and **engagement trends** into a single Community Impact Card. Savings are computed per-transaction using a deterministic estimator library with uncertainty propagation, then aggregated at query time using quadrature.

The system is organized in two layers:

1. **Estimator library** (`server/impact_metrics/estimator/`) -- config-driven functions that produce `Estimate` values (mean + stddev) for individual savings dimensions.
2. **Builder + calculator** (`server/impact_metrics/builder.go`, `calculator.go`) -- compose estimator functions into full `ImpactEstimate` messages and aggregate them across a community.

Key design decisions:
- **Impact on transactions, not items**: `ImpactEstimate` is stored on Transfer, Request, and Experience records. Gear stores `ValueEstimate` (replacement value) and `CarbonEstimate` (embodied carbon) as metadata for the estimator.
- **Uncertainty throughout**: All numerical estimates use `Estimate` (mean + stddev). Aggregation uses quadrature propagation.
- **Declarative config**: All assumptions, emission factors, and thresholds live in a checked-in `config.textproto` -- auditable and reviewable.
- **Provenance tracking**: Every computed value carries a `Provenance` record (source, name, version, reasoning, citations) enabling staleness detection, user-edit preservation, and audit trail display. See [provenance.md](./provenance.md).
- **Methodology transparency**: Each estimation method has a full specification doc under [docs/impact_metrics/](./impact_metrics/) (formulas, parameters, uncertainty model, research references). The in-app `MethodologyDocScreen` is currently a placeholder ("coming soon"); the earlier server-rendered `GetMethodologyDoc` RPC has been removed.

## Activity Metrics

Simple counts computed by `CalculateActivityCounts()` in [calculator.go](../server/impact_metrics/calculator.go).

| Metric | Definition | Calculation |
|--------|-----------|-------------|
| **People** | Active community members | COUNT of CommunityUser records |
| **Items** | Gear + requests + events | Gear Count + Open/Fulfilled Requests + Upcoming/Past Events |
| **Loans** | Active + completed gear loans | COUNT of Transfer records (type=LOAN, not soft-deleted) |
| **Giveaways** | Completed permanent transfers | COUNT of completed Transfer records (type=GIVEAWAY) |
| **Help Offered** | Fulfilled requests | COUNT of Request records (state=FULFILLED) |
| **Events** | Past + upcoming experiences | COUNT of Experience records |

Client: `totalSharingCount` and metric getters in [community_impact_view_model.dart](../app/lib/presentation/viewmodels/community_impact_view_model.dart).

### Problems Handled (X of Y)

A derived "problems handled" fraction surfaced on the Workshop overview as
**X of Y** — how many community requests reached completion, over the universe
of requests raised. Both sides split into the same three buckets:

| Bucket | Handled (X) | Potential (Y) |
| --- | --- | --- |
| Loans | LOAN transfers in `COMPLETED` | all non-cancelled, non-deleted LOAN transfers (giveaways excluded) |
| Requests | requests in `FULFILLED` | all non-cancelled, non-deleted requests |
| Needs | planning needs claimed at least once (a `PlanningContribution.from_need_id` references them) | all non-deleted planning needs posted in the community |

Each Handled bucket is a subset of its Potential bucket, so X ≤ Y. Both come
wholesale from **`Calculator.CalculateProblemsCounts`** (the needs side via
`Calculator.CommunityNeedData`) — the single source of truth, so the headline
fraction and the deep-dive can never drift apart. Narrower than the broad
`acts_count`. The aggregate counts are carried on the Workshop brief
(`BriefPayload.problems_solved_count` = X, `problems_potential_count` = Y); the
per-entry breakdown comes from **`GetCommunityProblemsSolvedDetail`** (handler
[problems_solved.go](../server/services/impact_metrics/problems_solved.go),
client repo `getProblemsSolvedDetail`), rendered by
[`WorkshopProblemsSolvedDetailScreen`](../app/lib/presentation/screens/workshop/workshop_problems_solved_detail_screen.dart).
The deep-dive lists the handled entries, each tappable to its underlying gear /
request / need-parent via the item's `content_type`. See
[workshop.md](workshop.md).

## Savings Metrics

Savings quantify the tangible value generated through sharing. Each dimension is computed per-transaction by [builder.go](../server/impact_metrics/builder.go), stored on the transaction record as `ImpactEstimate`, and aggregated at query time by `CalculateImpactSavings()` in [calculator.go](../server/impact_metrics/calculator.go).

All savings values are `Estimate` messages carrying mean + stddev for rigorous uncertainty propagation. Each metric has a full specification document with formulas, parameters, uncertainty model, and research references.

### Money (Cost Savings)

Dollars saved by sharing instead of purchasing. Formula: `item_value_usd * prevented_purchase_rate`.

**Full specification:** [money_saved.md](./impact_metrics/money_saved.md)

### Emissions Prevented (CO2)

Kilograms of CO2e avoided through sharing (manufacturing avoided + waste reduced).

**Full specification:** [emissions_prevented.md](./impact_metrics/emissions_prevented.md)

### Time Saved

Hours saved through community interactions instead of shopping, researching, or doing tasks alone.

**Full specification:** [time_saved.md](./impact_metrics/time_saved.md)

### Quality Time (Social Connection)

A composite score representing the relational value of sharing interactions. One QT minute approximates one minute of quality in-person social contact, computed from seven weighted attributes (duration, modality, group size, tie strength, reciprocity, novelty, vulnerability).

**Full specification:** [quality_time.md](./impact_metrics/quality_time.md)

### Uncertainty Model

Each estimate carries a standard deviation reflecting confidence level. Aggregation uses `SumEstimates()` with quadrature: `stddev_total = sqrt(sum(stddev_i^2))`. See [uncertainty.go](../server/impact_metrics/estimator/uncertainty.go). Per-metric uncertainty details are documented in each specification doc.

## Trends Metrics

### Heartbeat

**Definition:** Median response time from request creation to first offer received.

**Calculation:** For each request with offers, compute `first_offer_time - request_created_time`, then take the median. Excludes withdrawn offers. Returns 0 if no data; UI shows "--".

**Display:** `~30 mins` / `~2.5 hrs` / `~3.2 days`

**Implementation:** `CalculateHeartbeat()` in [calculator.go](../server/impact_metrics/calculator.go)

## Architecture

### Proto Types

**`ImpactEstimate`** ([impact_estimate.proto](../proto/ripls/api/impact_estimate.proto)):
- `MoneySavings`: `value_usd` (Estimate), `provenance` (Provenance)
- `PreventedEmissions`: `manufacture_avoided_carbon` + `waste_reduced_carbon` (CarbonEstimate), `provenance` (Provenance)
- `TimeSavings`: `minutes` (Estimate), `provenance` (Provenance)
- `QualityTimeEstimate`: `quality_time_minutes` (Estimate), `belonging_minutes` (Estimate), `trust_credits` (Estimate), `provenance` (Provenance), `attributes` (QualityTimeAttributes)

**`QualityTimeAttributes`**: Per-transaction attribute values stored for display in loan history -- `estimated_duration_minutes`, `modality`, `group_size`, `tie_strength`, `reciprocity`, `novelty`, `vulnerability` (enum tiers, not numeric weights).

**`ConnectionContext`**: Builder input (not persisted) -- `prior_interaction_count`, `mutual_connection_count`, `distinct_contacts_this_week`. Resolved at build time from transaction history and community graph.

**`Estimate`**: `mean` (float) + `stddev` (float) -- pure numerical type, no provenance.

**`CarbonEstimate`**: `co2e_grams` (Estimate) + `provenance` (Provenance)

**`Provenance`**: `source` (ProvenanceSource enum), `name` (string), `version` (int32), optional `confidence` (float), optional `reasoning` (string), `sources` (repeated string). Defined in [common.proto](../proto/ripls/models/common.proto). See [provenance.md](./provenance.md) for full design.

**Storage:** `ImpactEstimate` stored on Transfer (field 13), Request (field 15), Experience (field 16) in both API and models protos.

### Data Flow

1. **Item creation** (SaveGear): `EstimateGearCarbon()` computes `CarbonEstimate` stored on gear. `ValueEstimate` generated by AI during GenGear. Both carry `Provenance` recording the generation mode and algorithm version.
2. **Transaction creation** (StartLoan, SubmitRequest, SaveExperience): Builder function creates `ImpactEstimate` from gear metadata + config. `ConnectionContext` is resolved for the participants and passed to the builder for Quality Time computation. Each savings dimension gets `Provenance` with the estimation method name and version. Converted to models via `APIImpactToModels()` and persisted.
3. **Transaction completion** (CompleteTransfer, MarkRequestFulfilled, CompleteExperience): ImpactEstimate re-computed with final data (actual attendee count, etc.) and persisted with updated provenance.
4. **Query** (GetCommunityImpactMetrics RPC): `CalculateImpactSavings()` reads stored ImpactEstimates from all completed transactions, aggregates with quadrature.
5. **Backfill**: Startup job in [impact_backfill.go](../server/jobs/impact_backfill.go) populates ImpactEstimate on pre-existing transactions. Provenance-aware: respects user-edited fields and skips up-to-date algorithm versions via `ShouldRefresh()`.

### Server Components

| Component | File | Purpose |
|-----------|------|---------|
| Estimator library | [server/impact_metrics/estimator/](../server/impact_metrics/estimator/) | Config-driven estimation functions for cost, carbon, time, quality time, uncertainty |
| Quality Time estimator | [social.go](../server/impact_metrics/estimator/social.go) | Quality Time scoring: attribute classification and composite computation |
| Config | [config.textproto](../server/impact_metrics/estimator/config.textproto) | All assumptions, emission factors, thresholds, social attribute weights |
| Methodology specs | [docs/impact_metrics/](./impact_metrics/) | Per-dimension specification docs explaining calculations (`money_saved.md`, `emissions_prevented.md`, `time_saved.md`, `quality_time.md`) |
| Builder | [builder.go](../server/impact_metrics/builder.go) | Composes estimator functions into `ImpactEstimate` per transaction type |
| ConnectionContext resolver | [social_context.go](../server/impact_metrics/social_context.go) | Resolves prior interaction count, mutual connections, weekly contacts for QT computation |
| Calculator | [calculator.go](../server/impact_metrics/calculator.go) | Aggregates metrics across a community, type conversions, subsidiary metric computation; `CalculateProblemsCounts` / `CommunityNeedData` (the "problems handled" X-of-Y buckets, incl. claimed planning needs) |
| Metric detail calculator | [metric_detail_calculator.go](../server/impact_metrics/metric_detail_calculator.go) | Computes detailed breakdowns per dimension including social sufficiency, categories, subsidiary metrics |
| Impact metrics service | [service.go](../server/services/impact_metrics/service.go) | RPC dispatch (`GetCommunityImpactMetrics`, `GetUserImpactMetrics`, `GetCommunityMetricDetail`, `GetCommunityProblemsSolvedDetail`, …); per-RPC logic in sibling files (e.g. [metrics.go](../server/services/impact_metrics/metrics.go), [metric_detail.go](../server/services/impact_metrics/metric_detail.go), [problems_solved.go](../server/services/impact_metrics/problems_solved.go)) |
| Backfill job | [impact_backfill.go](../server/jobs/impact_backfill.go) | Populates IE on transactions created before impact was wired in (includes QT backfill) |
| Provenance helpers | [provenance.go](../server/impact_metrics/provenance.go) | `ShouldRefresh()` staleness check for provenance-aware re-estimation |

### Client Components (Impact 2.0)

**Services & Repositories:**
| Component | File | Purpose |
|-----------|------|---------|
| Impact metrics service | [impact_service.dart](../app/lib/services/impact_service.dart) | RPC client for GetCommunityImpactMetrics, GetUserImpactMetrics, and the metric-detail RPCs |
| Impact metrics repository | [impact_repository.dart](../app/lib/data/repositories/impact_repository.dart) | Caching layer for community/user metrics |

**ViewModels:**
| Component | File | Purpose |
|-----------|------|---------|
| Community VM | [community_impact_view_model.dart](../app/lib/presentation/viewmodels/community_impact_view_model.dart) | AsyncNotifier for community-level aggregated metrics |
| Profile/User VM | [profile_metrics_view_model.dart](../app/lib/presentation/viewmodels/profile_metrics_view_model.dart) | `ProfileImpactMetricsViewModel` AsyncNotifier for user-level aggregated metrics (data shape in [profile_metrics_data.dart](../app/lib/presentation/viewmodels/profile_metrics_data.dart)) |
| Gear VM | [gear_metric_view_model.dart](../app/lib/presentation/viewmodels/gear_metric_view_model.dart) | AsyncNotifier for per-item gear metrics |
| Experience VM | [experience_metric_view_model.dart](../app/lib/presentation/viewmodels/experience_metric_view_model.dart) | AsyncNotifier for per-experience metrics |
| Request VM | [request_metric_view_model.dart](../app/lib/presentation/viewmodels/request_metric_view_model.dart) | AsyncNotifier for per-request metrics |
| Impact draft notifier | [impact_draft_notifier.dart](../app/lib/presentation/viewmodels/impact_draft_notifier.dart) | Drives the `DraftImpactEstimate` flow for editable creation-time estimates (state in [impact_draft_state.dart](../app/lib/presentation/viewmodels/impact_draft_state.dart)) |

**Screens:**
| Component | File | Purpose |
|-----------|------|---------|
| Workshop metric screens | `screens/workshop/workshop_{money,time,co2,problems_solved}_detail_screen.dart` | Per-dimension community impact, each rendering its series into `ImpactChartCard` |
| Item screen | [item_metrics_screen.dart](../app/lib/presentation/screens/item/item_metrics_screen.dart) | Full-screen gear impact with cumulative bar, loan history |
| User screen (personal impact) | [user_screen.dart](../app/lib/presentation/screens/users/user_screen.dart) | Viewer-facing profile; personal impact rendered via [profile_metric_rows.dart](../app/lib/presentation/widgets/profile_section/profile_metric_rows.dart) driven by `ProfileImpactMetricsViewModel` |
| Metric drill-down | [metric_drill_down_screen.dart](../app/lib/presentation/screens/impact_metrics/metric_drill_down_screen.dart) | Per-dimension drill-down with confidence indicator, formula visualization, references |
| Audit trail | [audit_trail_screen.dart](../app/lib/presentation/screens/impact_metrics/audit_trail_screen.dart) | Per-attribute provenance breakdown built directly from an `ImpactEstimate` proto, rendered as `AuditTrailStep` rows |

**Shared Widgets:**
| Component | File | Purpose |
|-----------|------|---------|
| Impact hero | [impact_hero_section.dart](../app/lib/presentation/widgets/impact/impact_hero_section.dart) | Dark gradient hero with metadata, description, stat chips |
| Audit trail step | [audit_trail_step.dart](../app/lib/presentation/widgets/impact/audit_trail_step.dart) | Single provenance step with AI/user icon and reasoning bubble |
| Stat chip | [stat_chip.dart](../app/lib/presentation/widgets/impact/stat_chip.dart) | Translucent pill button for hero section stats |
| Formula visualization | [formula_visualization.dart](../app/lib/presentation/widgets/impact/formula_visualization.dart) / [formula_component_card.dart](../app/lib/presentation/widgets/impact/formula_component_card.dart) | Visual breakdown of a metric's calculation formula |
| Reference list | [reference_list.dart](../app/lib/presentation/widgets/impact/reference_list.dart) | Methodology sources shown in detail views |
| Loan history | [loan_history_card.dart](../app/lib/presentation/widgets/impact/loan_history_card.dart) | Per-loan entry with borrower, dates, status, metrics, expandable social attributes |
| Sharing impact | [sharing_impact_card.dart](../app/lib/presentation/widgets/impact/sharing_impact_card.dart) | Per-transaction impact display including QT with teal accent |
| Equivalence ladder | [equivalence/equivalence_ladder_card.dart](../app/lib/presentation/widgets/impact/equivalence/equivalence_ladder_card.dart) | Real-world analogue card ("$X is like …") driven by per-dimension ladder data (`kMoneyPaycheckLadder`, `kCo2DailyLifeLadder`, `kTimeHealthLadder`) |

**Data Models:**
| Component | File | Purpose |
|-----------|------|---------|
| Profile metrics data | [profile_metrics_data.dart](../app/lib/presentation/viewmodels/profile_metrics_data.dart) | Freezed model holding the aggregated user/profile impact metrics the `ProfileImpactMetricsViewModel` exposes |
| Impact draft state | [impact_draft_state.dart](../app/lib/presentation/viewmodels/impact_draft_state.dart) | Freezed state for the editable creation-time draft estimate flow |

The audit trail no longer has a dedicated freezed model — `AuditTrailScreen` builds its `AuditTrailStep` rows directly from the `ImpactEstimate` proto's per-attribute provenance.

**Utilities:**
| Component | File | Purpose |
|-----------|------|---------|
| Savings formatter | [savings_formatter.dart](../app/lib/core/utils/savings_formatter.dart) | Converts ImpactEstimate proto to display strings |

## Display Structure

All three metric screens (Community, User, Item) follow a consistent two-part
visual pattern. The illustrative mock below shows the original four-dimension
layout; the **canonical, current** tab/hero/pill/drill-down spec — including the
Impact 3.0 reduction to three visible tabs (Time Recovered removed) — is the
"[Metrics Screen Reference](#metrics-screen-reference)" section below. Where the
two differ, the Metrics Screen Reference wins.

### Common Pattern

```
┌──────────────────────────────────────────────┐
│  DARK HERO SECTION (gradient background)     │
│  ──────────────────────────────────────       │
│  Name / Title (large serif)                  │
│  Metadata line (members · location · date)   │
│  Description (truncated to 3 lines)          │
│  [Stat chips: items, loans, giveaways...]    │
│                                              │
│  [Item only: Cumulative impact bar]          │
├──────────────────────────────────────────────┤
│  LIGHT CARD SECTION (warm white bg)          │
│  ──────────────────────────────────────       │
│  Italic framing text                         │
│                                              │
│  ┌──────────────────────────────────────┐    │
│  │ [$] $250           [●] [>]          │    │
│  │     Saved                            │    │
│  └──────────────────────────────────────┘    │
│  ┌──────────────────────────────────────┐    │
│  │ [⏱] 2 hrs          [●] [>]          │    │
│  │     Recovered                        │    │
│  └──────────────────────────────────────┘    │
│  ┌──────────────────────────────────────┐    │
│  │ [☁] 4.1kg          [●] [>]          │    │
│  │     CO₂ Avoided                      │    │
│  └──────────────────────────────────────┘    │
│  ┌──────────────────────────────────────┐    │
│  │ [👥] 1,842rm       [●] [>]          │    │
│  │      Shared                          │    │
│  └──────────────────────────────────────┘    │
│                                              │
│  [Item only: Loan History section]           │
└──────────────────────────────────────────────┘
```

### Audit Trail Screen

`AuditTrailScreen` ([audit_trail_screen.dart](../app/lib/presentation/screens/impact_metrics/audit_trail_screen.dart))
shows the per-attribute provenance breakdown for an `ImpactEstimate`, grouped by
dimension (Quality Time, Money, Emissions). Each attribute is one `AuditTrailStep`
row: the attribute label on the left, and on the right the formatted value plus a
source badge.

```
┌──────────────────────────────────────────────┐
│  [<] How we calculated this                  │
├──────────────────────────────────────────────┤
│  MONEY SAVED                                 │
│  Item value              $250        [AI]    │
│  Prevented-purchase rate 70%   [CALCULATED]  │
│  Money saved             $175        [AI]    │
├──────────────────────────────────────────────┤
│  QUALITY TIME                                │
│  Duration                30 min   [DEFAULT]  │
│  Modality                In person   [AI]    │
│  …                                           │
└──────────────────────────────────────────────┘
```

Each row's source badge is derived from the attribute's `Provenance.source`:
USER (coral), AI/LLM (green), CALCULATED/FORMULA, or DEFAULT. There is no
step-timeline, counterfactual comparison card, or suggest-correction form on this
screen — those Impact 2.0 surfaces were removed. The richer "How we got here"
reasoning and formula breakdowns now live on the per-dimension drill-down screens
([metric_drill_down_screen.dart](../app/lib/presentation/screens/impact_metrics/metric_drill_down_screen.dart)
with `formula_visualization.dart` and `reference_list.dart`).

## Testing

### Server Tests
- [calculator_test.go](../server/impact_metrics/calculator_test.go) -- Activity counts, heartbeat, savings aggregation with quadrature
- [builder_test.go](../server/impact_metrics/builder_test.go) -- 11 tests for all builder functions and scaling
- [impact_backfill_test.go](../server/jobs/impact_backfill_test.go) -- 8 tests for backfill job (empty DB, idempotency, missing gear, etc.)
- [social_test.go](../server/impact_metrics/estimator/social_test.go) -- Quality Time scoring: attribute classification, composite computation, boundary values, quadrature uncertainty propagation
- Estimator library tests: savings (incl. provenance assertions), carbon, time, uncertainty
- Lifecycle tests ([transfer](../server/services/transfer/lifecycle_test.go), [request](../server/services/request/lifecycle_test.go), [experience](../server/services/experience/lifecycle_test.go)) -- provenance populated with correct source, non-empty name, version > 0

### Integration Tests
- [loan_test.go, giveaway_test.go, request_test.go, experience_test.go](../server/integration_tests/) -- Impact postconditions on all workflow examples: positive IE on completed transactions, zero impact on cancelled/withdrawn, cumulative scaling, community aggregation

### Client Tests
- [community_impact_view_model_test.dart](../app/test/presentation/viewmodels/community_impact_view_model_test.dart) -- Display formatting, disposal safety, computed metric properties
- [profile_metrics_view_model_test.dart](../app/test/presentation/viewmodels/profile_metrics_view_model_test.dart) -- `ProfileImpactMetricsViewModel`: data loading, repository fan-out, refresh
- [savings_formatter_test.dart](../app/test/core/utils/savings_formatter_test.dart) -- Formatting, detail objects, method display names, confidence calculation, QT formatting
- [loan_history_card_test.dart](../app/test/presentation/widgets/impact/loan_history_card_test.dart) -- Collapsed/expanded states, social attribute display rows

## Future Work

### LLM-Assisted Request/Experience Estimation
Currently requests and experiences use flat config defaults for all three savings dimensions. Planned improvements:
- Classify request type (borrow, repair, service, advice) for type-specific carbon estimation
- Estimate carbon-relevant parameters for requests (what item is being repaired? what transportation is avoided?)
- Use LLM hints for experience duration and value based on description
- See TODOs in [savings.go](../server/impact_metrics/estimator/savings.go) and [config.textproto](../server/impact_metrics/estimator/config.textproto)

### Refine Estimates at Transaction Completion
- On loan return: re-estimate if gear metadata has changed since loan start
- On request fulfillment: refine with fulfillment details (actual labor time, materials used)
- On experience conclusion: refine with actual attendance and duration
- Currently creation-time estimates are sufficient; refinement improves accuracy for repeat events

### Category Carbon Database Integration
- Integrate an external API (e.g., Climatiq, Icebreaker One) for category-level carbon averages
- The primary path is material-weight table lookup from GenGear metadata, with spend-based EEIO as fallback
- A richer category database would improve the fallback path

### Uncertainty-Aware Display Formatting
- Low uncertainty: "24 kg CO2", medium: "~24 kg CO2", high: "est. 24 kg CO2"
- Confidence is shown in per-metric drill-down screens (derived from stddev/mean); hero values still display as point estimates

### LLM-Assisted Quality Time Enrichment
Duration and vulnerability attributes currently use config defaults. Planned improvements:
- Infer handoff duration from item type ("camping gear → 30 min walkthrough")
- Classify vulnerability from item value, request sensitivity, and hosting context
- Compare config-default vs LLM-inferred cohorts to validate enrichment accuracy
- See the Quality Time spec: [quality_time.md](./impact_metrics/quality_time.md)

## Metrics Screen Reference

This section is the canonical definition of tab labels, hero labels, stat
pills, and drill-down content for all metrics screens. All three screens
(Community, Portfolio, User) must conform to this reference. Any deviation is
a regression.

### Tab Bar (3 tabs, left to right)

| Tab | Label | Dimension enum |
|-----|-------|----------------|
| 1 | SAVED | `IMPACT_METRIC_DIMENSION_MONEY` |
| 2 | QUALITY TIME | `IMPACT_METRIC_DIMENSION_QUALITY_TIME` |
| 3 | CO2 AVOIDED | `IMPACT_METRIC_DIMENSION_EMISSIONS` |

The Time Recovered tab has been removed from the tab bar (Impact 3.0). Time
data is still computed server-side for future QT integration.

### Hero Value (large number above tabs)

| Tab | Hero value | Hero label (inline, right of value) |
|-----|-----------|--------------------------------------|
| SAVED | Formatted cost savings (e.g., "$59.8K") | "saved" |
| CO2 AVOIDED | Formatted CO₂ (e.g., "14K kg") | "CO2 avoided" |
| QUALITY TIME | Formatted quality time (e.g., "567 rm") | "quality time" |

Hero label is displayed on the same line as the hero value, to its right,
in matching serif font. Hero values are generated by
`ImpactHook.labelFor(dimension)`. All screens must use this method — no
custom label strings.

### Stat Pills (3 per tab, below tab bar)

Pills differ between the **Community** screen and the **Portfolio / User** screens.

#### Community Screen Pills

| Tab | Pill 1 | Pill 2 | Pill 3 |
|-----|--------|--------|--------|
| SAVED | **LIBRARY** — total library value ($) | **UTILIZED** — utilization % | **PERCENTILE** — cross-community rank |
| CO₂ AVOIDED | **POTENTIAL** — total library CO₂ potential (kg) | **TREES/YR** — kg / 20 | **PERCENTILE** — cross-community rank |
| QUALITY TIME | **HRS/PERSON** — avg QT per total member | **TIME-TO-SOLVE** — median request→resolution time | **PERCENTILE** — cross-community rank |

#### Portfolio / User Screen Pills

| Tab | Pill 1 | Pill 2 | Pill 3 |
|-----|--------|--------|--------|
| SAVED | **GIVEN** — cost savings the user generated for others by sharing | **RECEIVED** — cost savings the user earned by borrowing | **LIBRARY** — total library value ($) |
| CO₂ AVOIDED | **GIVEN** — CO₂ the user avoided for others through their sharing | **RECEIVED** — CO₂ the user avoided by borrowing from others | **TREES/YR** — total kg / 20 |
| QUALITY TIME | **CREATED** — QT from events/items the user created and participated in | **JOINED** — QT from others' events/items the user joined | **HRS/WEEK** — avg quality-time hours per active week |

ARB keys (community): `impactPillLibrary`, `impactPillUtilized`, `impactPillPercentile`,
`impactPillPotentialCo2`, `impactPillTreesYr`, `impactPillHrsPerPerson`,
`impactPillTimeToSolve`.

ARB keys (portfolio/user): `impactPillGiven`, `impactPillReceived`, `impactPillLibrary`,
`impactPillTreesYr`, `impactPillCreated`, `impactPillJoined`, `impactPillHrsWeek`.

All pills are tappable on all screens (except when viewing another user's
profile, where pills are display-only).

### Pill Drill-Down Content

#### Community Screen Drill-Downs

| Pill | Taps to | Content shown |
|------|---------|---------------|
| LIBRARY | `MetricDetailScreen` category `'LibraryValue'` | Scrollable list of all library items sorted by dollar value descending. Each row: item name, sharer name, loan count, dollar value. Rows with gearId are tappable → item screen. |
| UTILIZED | `UtilizationDetailScreen` | Utilization over time chart, most utilized items (ranked by loan-days/days-listed; items with 0% utilization are hidden — shows "Nothing yet" if all are zero), untapped potential (high-value low-utilization items). |
| POTENTIAL | `Co2PotentialDetailScreen` | Title: "CO₂ Potential". Hero: total potential kg. Library items ranked by potential CO₂ savings (most to least). Each row: item name, sharer name, potential kg. Methodology link. |
| TREES/YR | `TreesDetailScreen` | Title: "Trees Planted". Hero: tree count. Tree growth chart (cumulative over time), "Your forest is..." landmark comparison card, other CO₂ equivalences (miles, flights, burgers, cow burps, phone charges). |
| HRS/PERSON | `HrsPerPersonDetailScreen` | Title: "Hours per Person". Hero: avg hours. Breakdown by 7 QT attributes: duration, modality, group_size, tie_strength, reciprocity, novelty, vulnerability. Methodology link. |
| TIME-TO-SOLVE | `TimeToSolveDetailScreen` | Title: "Time to Solve". Hero: median time. Trend chart. Recent fulfilled requests with solve times, reverse chronological. |
| PERCENTILE — community | `PercentileDetailScreen` | Percentile over time chart (y-axis 0–100). Top 10 communities + this community's position (highlighted row). |

#### Portfolio / User Screen Drill-Downs

All portfolio/user metric pills open `PortfolioMetricDetailScreen` with the
appropriate `dimension` and `perspective` parameters, except TREES/YR which
opens `TreesDetailScreen`. Each detail screen shows:
1. **Hero** — the exact metric value for that pill (e.g. Given CO₂ kg, Joined QT mins).
2. **Growth chart** — cumulative trend for that dimension + perspective over time.
3. **Line items** — top transactions that contributed to that metric, most impactful first.

| Pill | Perspective | Line items shown |
|------|------------|-----------------|
| SAVED · GIVEN | `PORTFOLIO_PERSPECTIVE_GIVEN` | Transfers where the user lent or gave items to others |
| SAVED · RECEIVED | `PORTFOLIO_PERSPECTIVE_RECEIVED` | Transfers where the user borrowed something |
| SAVED · LIBRARY | `PORTFOLIO_PERSPECTIVE_FULL_IMPACT` + category `'library'` | All library items sorted by estimated value descending |
| CO₂ · GIVEN | `PORTFOLIO_PERSPECTIVE_GIVEN` | Transfers/transactions where the user's sharing avoided CO₂ for others |
| CO₂ · RECEIVED | `PORTFOLIO_PERSPECTIVE_RECEIVED` | Transfers where the user borrowed, avoiding a purchase |
| CO₂ · TREES/YR | `TreesDetailScreen` | Tree count, forest landmark, CO₂ equivalences |
| QT · CREATED | `PORTFOLIO_PERSPECTIVE_GIVEN` | Events/experiences the user created and participated in |
| QT · JOINED | `PORTFOLIO_PERSPECTIVE_RECEIVED` | Events/experiences created by others that the user joined |
| QT · HRS/WEEK | `PORTFOLIO_PERSPECTIVE_FULL_IMPACT` | Full quality-time detail (all transactions) |

### Source Breakdown Rows (below chart)

| Tab | Rows shown | Taps to |
|-----|-----------|---------|
| SAVED | Loans, Giveaways, Requests, Events | `MetricDetailScreen` filtered by source category |
| CO₂ AVOIDED | Loans, Giveaways, Requests, Events | `MetricDetailScreen` filtered by source category |
| QUALITY TIME | Loans, Giveaways, Requests, Events | `MetricDetailScreen` filtered by source category |

Row identity is the typed `SourceBreakdown.source_type`
(`ImpactSourceType`, #2827) — clients key ordering and aggregation on the
enum and render the row label from the `metricDetailSource*` ARB keys at
build time. The server no longer emits a `SourceBreakdown.label` at all —
#2835 removed it and reserved the field number.

### Contextual Equivalence Cards (all stages, per tab)

Each tab shows a contextual real-world-analogue card immediately below the stat
pills, giving the current metric value a tangible comparison. These are all
rendered by the single `EquivalenceLadderCard`
([equivalence/equivalence_ladder_card.dart](../app/lib/presentation/widgets/impact/equivalence/equivalence_ladder_card.dart)),
parameterized by a per-dimension ladder constant and an `EquivalenceMetric`. The
card resolves the highest tier the value reaches via `resolveEquivalence()`
([equivalence_resolver.dart](../app/lib/presentation/widgets/impact/equivalence/equivalence_resolver.dart)).

| Tab | Ladder | `EquivalenceMetric` | Analogue |
|-----|--------|---------------------|----------|
| SAVED | `kMoneyPaycheckLadder` ([money_paycheck_ladder.dart](../app/lib/presentation/widgets/impact/equivalence/money_paycheck_ladder.dart)) | `moneySaved` | "$X saved is like …" — 17 ascending money tiers |
| CO₂ AVOIDED | `kCo2DailyLifeLadder` ([co2_daily_life_ladder.dart](../app/lib/presentation/widgets/impact/equivalence/co2_daily_life_ladder.dart)) | `co2Avoided` | "X kg CO₂ avoided is like …" — 17 ascending daily-life tiers |
| QUALITY TIME | `kTimeHealthLadder` ([time_health_ladder.dart](../app/lib/presentation/widgets/impact/equivalence/time_health_ladder.dart)) | `timeTogether` | "X of quality time is like …" — 16 ascending time/health tiers |

Each ladder is a list of `EquivalenceTier`s ([equivalence_tier.dart](../app/lib/presentation/widgets/impact/equivalence/equivalence_tier.dart))
carrying `labelKey` / `copyKey` / `sourceNameKey` ARB keys (resolved at the widget
layer via `context.l10n`) plus a raw `sourceUrl`. When the value is below the
lowest tier the resolver returns a fallback (`equivalenceFallbackHeadline` + a
per-metric fallback body). All copy is i18n'd via `equivalence*` ARB keys (e.g.
`equivalenceMoneyPaycheckLabel*`, `equivalenceCo2DailyLifeLabel*`,
`equivalenceTimeHealthLabel*`, `equivalenceFallbackHeadline`). The same card is
reused on workshop detail screens and the profile impact rows; the
`EquivalenceSurface` parameter (workshop detail / profile row / community stage)
dedups the `tier_unlocked` analytics event.

### Leaderboard (tappable section header, all tabs)

"Top Contributors >" section header below breakdown rows on each tab. Taps
to a dedicated leaderboard detail screen showing top 10 community members
ranked by the tab's primary metric (money saved, CO₂ avoided, or quality
time). Includes all participation roles (owned, borrowed, helped, attended).
Each member row tappable → user community impact detail screen (longitudinal
chart + transaction list in reverse chronological order).

### Community Lifecycle Stage UX

The community metrics screen adapts its content based on the community's lifecycle stage, determined client-side from `CommunityImpactMetrics`.

| Stage | Condition | Key UX |
|-------|-----------|--------|
| **New** | 0 transactions, ≤1 member | Tab-specific "Your community is born!" welcome card + single MetricDescriptionCard for the active tab + get-started next steps |
| **Listed** | ≥1 item listed, 0 completed transactions | Comparison card + scenario projection card + tab-specific next-step card |
| **First Transaction** | Exactly 1 completed transaction | Comparison card + progress bar toward first milestone + what's-ahead scenario card |
| **Active** | 2–19 completed transactions | Full view with chart, trend card, milestone cards, breakdown, leaderboard |
| **Mature** | 20+ completed transactions | Full view (same as Active, milestones hidden after dismissal) |

#### New-community stage: tab-specific welcome cards

Each tab shows a distinct "Your community is born!" `WelcomeCard` with copy and a CTA tailored to that dimension:

| Tab | Copy focus | CTA | Action |
|-----|-----------|-----|--------|
| SAVED | Start listing shareable items | "Add your first item" | Opens the blank gear-creation flow via `openBlankCreateGear()` |
| CO₂ AVOIDED | Ask your circle for help before buying | "Add a help request" | Opens `RequestCreationModal` |
| QUALITY TIME | Share upcoming events so your circle can join | "Add your first event" | Opens `ExperienceCreationModal` |

The `WelcomeCard` widget accepts `title`, `description`, `buttonLabel`, `onButtonTap`, and `gradientColors` — the gradient and button color match each tab's accent.

#### New-community stage: "What your circle will build" section

Shows a single `MetricDescriptionCard` for the currently selected tab (money, CO₂, or quality time), not all three at once.

#### Listed stage: tab-specific next-step cards

The call-to-action at the bottom of the listed stage is tab-specific:

| Tab | Next step | Action |
|-----|-----------|--------|
| SAVED | "Start your first loan" | No navigation (informational) |
| CO₂ AVOIDED | "Post a help request" | Opens `RequestCreationModal` |
| QUALITY TIME | "Share your first event" | Opens `ExperienceCreationModal` |

### Detail Screen Table Style

All detail screen tables use clean rows without icons or chevron carets.
Rows are tappable where applicable but without visual arrow affordances.
Exception: Trees equivalence rows keep their illustrative icons.

### Invariant: Secondary pill drill-down must match the pill

Every secondary pill (pill 2 and pill 3) must open a detail screen whose
title and hero value correspond to that specific metric — not the generic
aggregate detail for the whole dimension. Tapping TREES/YR must show a trees
screen, not the total CO₂ screen. A widget test verifies this for every pill
on every screen.

### Portfolio Inbox Metric Rings

Order must match tab bar: **Saved → CO2 Avoided → Quality Time**
(previously was Social Time → Money Saved → CO₂ Avoided).

### User Metrics Screen (own vs. others)

- **Own profile:** Identical to Portfolio metrics screen. All pills and
  breakdown rows tappable with full drill-down.
- **Other user's profile:** Identical layout and content to Portfolio metrics
  screen, but all pills and breakdown rows are display-only (no tap handlers,
  no drill-down navigation). Server enforces this by omitting transaction-level
  detail from the response.

## Related Documentation

- [provenance.md](./provenance.md) -- Provenance system design: tracked types, version-based staleness, refresh logic
- [impact_metrics/quality_time.md](./impact_metrics/quality_time.md) -- Quality Time (fourth metric): seven attributes, sufficiency model, subsidiary metrics
- [client/architecture.md](./client/architecture.md) -- MVVM patterns
- [client/caching.md](./client/caching.md) -- Repository caching strategy

### Per-Metric Specifications

- [money_saved.md](./impact_metrics/money_saved.md) -- Money Saved: formula, parameters, uncertainty, research references
- [emissions_prevented.md](./impact_metrics/emissions_prevented.md) -- Emissions Prevented: material/spend factors, two-component formula
- [time_saved.md](./impact_metrics/time_saved.md) -- Time Saved: defaults, AI hints, attendee scaling
- [quality_time.md](./impact_metrics/quality_time.md) -- Quality Time: seven attributes, sufficiency model, subsidiary metrics

---

**Last Updated:** 2026-05-05
**Implementation Status:**
- **Backend:** Complete. Transaction-level ImpactEstimate with quadrature aggregation, estimator library, backfill. Provenance on all impact dimensions and gear metadata. (The server-rendered methodology-doc RPC was removed; methodology specs live under `docs/impact_metrics/`.)
- **Frontend:** Complete. Three full-screen metric views (Community, User/profile, Item), per-attribute audit trail screen, per-dimension drill-down screens, equivalence-ladder cards, shared widget library.
- **Quality Time (Fourth Metric):** Complete. Proto definitions, estimator library, config, ConnectionContext resolver, builder integration, community/item/user metric screens with detail view, per-loan social attributes, subsidiary metrics. See [quality_time.md](./impact_metrics/quality_time.md) for the dimension spec.
- **Impact 3.0:** In progress. See [impact3.md](./ai/impact3.md) for plan. Time tab removed, metrics consistency and insight overhaul planned.
- **Impact 4.0:** Planned. See [impact4.md](./ai/impact4.md) for plan. Leaderboards, percentile rankings, metric refinements, design polish.
**Architecture:** Transaction-level ImpactEstimate storage with community/user-level aggregation. Three visible dimensions: cost, carbon, quality time (time saved computed but not displayed as a tab). Provenance tracking on all computed values. Client displays via dedicated screens with audit trail drill-downs.
