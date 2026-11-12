# Issue #1328: [api_design] PortfolioService encodes view-model formatting into the API contract

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1328
**Filed:** 2026-04-25 | **Last activity:** 2026-04-27
**Labels:** auto-filed, review:api_design | **Assignees:** unassigned
**Context docs:** `docs/reviews/reports/api_design_review.md`,
`docs/client/i18n.md`, `docs/client/architecture.md`,
`docs/server/architecture.md`

## Audit Notes

Audited against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, and `docs/server/architecture.md`. Updates applied:

- **i18n is already set up.** The project ships English + Spanish ARB
  bundles (`app/lib/l10n/app_{en,es}.arb`), enforces no-hardcoded-strings
  via `npm run lint:dart:i18n`, and forbids viewmodels from resolving
  strings (widgets call `context.l10n.<key>` in `build()`). ARB
  additions are required, not optional, and key prefixes follow the
  `portfolio*` / `impact*` / `metricDetail*` / `communityMetrics*`
  convention from `docs/client/i18n.md`.
- **Architecture boundary preserved.** Per
  `docs/client/architecture.md`, viewmodels hold business logic and
  expose typed state; widgets resolve display. The plan keeps existing
  consumers (mostly widgets) reading proto fields directly from
  responses provided by viewmodels — that already aligns with the
  architecture and does not introduce a new repository or cache.
  Caching layer (`docs/client/caching.md`) is unaffected: this issue
  changes the *shape* of cached values, not where caching lives.
- **Server architecture aligned.** Per
  `docs/server/architecture.md`, services depend on shared libraries.
  This PR deletes server formatter helpers locally inside
  `server/services/portfolio/` and `server/services/impact_metrics/`
  rather than promoting them to a shared package — they are
  view-model concerns and should not become a server-wide library.
- **Wire-compat trade-off accepted.** Per reviewer comment 2026-04-27,
  the team prefers a single big-bang PR over the phased dual-spec
  alternative. The plan therefore relies on a coordinated app-then-server
  release, with a minimum-supported-client-version bump as the
  pre-merge gate (Step 7).

## Problem Statement

`PortfolioService` and `ImpactService` are server-rendered view-models. Their
RPC responses ship dozens of pre-formatted display strings — currency strings
(`"$1,247"`), short relative-time labels (`"12m"`, `"2h"`, `"3d"`), date and
week labels (`"Mon, Feb 23"`, `"This week · Feb 17 – 23"`), CO₂ strings
(`"0.8kg"`), section titles and subtitles (`"What I've saved"`), encouragement
copy (`"Over halfway there!"`), and theme tokens (`action_button_style:
"coral" / "sage" / "coral_soft"`).

The Flutter app already has a complete formatter stack — `ValueHelpers`
(`formatSavingsMoney(cents)`, `formatCO2Grams(grams)`, `formatTimeMinutes`),
`SavingsFormatter`, and `DateTimeFormatter` — that produces the same strings
the server now hand-rolls. Routing the data through pre-formatted server
strings:

- blocks any client-side i18n (locale, currency, date format are all baked
  into Go `fmt.Sprintf` calls and English literals like `"Tomorrow"`,
  `"Later This Week"`);
- prevents alternate clients (web, watch, widget, email digest) from
  participating without re-implementing the Flutter UI in Go;
- forces a server deploy for any copy or format A/B test;
- bloats wire size — every action carries duplicate copies of values
  available raw elsewhere in the same response;
- hides the actual numeric value from the wire entirely in some places
  (e.g. `RankedUser.formatted_value`, `DirectionBreakdown.formatted_value`
  expose only a string), so even the existing client cannot do arithmetic
  on them.

The pattern is established but not uniform: a few messages already carry
both forms (`WeeklyMetrics.money_saved_cents_goal` next to `cost_saved`;
`MoneySavedDetail.current_usd` next to `library_value`), proving the
raw shape is feasible. The fix is to converge the entire surface on raw
numerics in one PR.

The maintainer comment from `@tescobar` ("It's legit, though some of this
might get obsoleted. But I have been going through and been removing any
text strings from the server since that's not i18n compatible") confirms
direction and acknowledges some fields may be retired entirely as the UI
evolves — the plan should leave room for both outcomes.

## Context from Comments

- **`maintainer`** (2026-04-25): tagged `@tescobar` for input.
- **`@tescobar`** (2026-04-25): "It's legit, though some of this might get
  obsoleted. But I have been going through and been removing any text
  strings from the server since that's not i18n compatible." → confirms
  the direction (server should not return display strings) and signals
  that some pre-formatted fields are candidates for outright removal,
  not parity.
- **`maintainer`** (2026-04-27): pinged `@claude` to plan/execute.

No disagreement; the recommendation is endorsed. The only nuance is that
a subset of fields may be cheaper to delete than to swap for a raw
counterpart — the plan triages each field into "replace with raw" vs.
"delete outright" during the Step 1 inventory.

A second piece of guidance was added in the plan PR (`#1481`,
2026-04-27) when reviewer `maintainer` requested a single big-bang
PR rather than the phased dual-spec sequence originally drafted. The
recommendation below now executes Option A.

## Potential Solutions

### Option A: Big-bang replacement — single PR (recommended)

- **Approach:** In one PR, delete every pre-formatted string field from
  `portfolio.proto` and `impact_service.proto`, replace with raw numerics
  (`*_cents`, `*_grams`, `*_minutes`, `*_unix_sec`, theme enums), delete
  pure-copy fields outright, delete the server formatter helpers in the
  same change, and migrate every Flutter consumer to read the raw fields
  and resolve copy from `app/lib/l10n/app_{en,es}.arb`. Reserve the old
  field numbers and names so the slot can never be reused.
- **Pros:** Final shape lands in one reviewable diff; no transitional
  dual-spec state in the proto, no zombie formatter helpers, no follow-up
  cleanup PRs. Each per-field "delete vs. replace with raw" decision is
  made once and committed once. Reviewers see proto, server, and client
  changes side-by-side and can verify the migration is complete.
- **Cons:** This is a wire-breaking change. Any deployed Flutter client
  on a version older than the one shipped with this PR will see missing
  fields (default zero values in Dart) once the server deploys, breaking
  portfolio + impact UIs. Mitigation: app release coordinates with the
  server deploy — TestFlight + Play Store store-update prompt rolls the
  client first, server deploy follows, and we set a minimum supported
  client version that excludes anything that predates the cutoff.
  Larger PR (~40 proto fields touched, ~25+ Flutter consumers, multiple
  ARB key additions) but mechanical.
- **Effort:** Large single PR; ~5–7 days end-to-end.

### Option B: Additive parity then deprecation

- **Approach:** Phase 1 — add raw numeric counterparts beside every
  formatted string field, leaving the formatted fields intact and
  populated. Phase 2 — migrate every Flutter consumer call-site off the
  formatted fields to the raw fields. Phase 3 — mark formatted fields
  `[deprecated = true]`. Phase 4 (deferred, behind a minimum-client-version
  gate) — remove the deprecated string fields and server formatters.
- **Pros:** Wire-compatible with old clients at every step. Each phase
  is independently shippable.
- **Cons:** Lives in a transitional dual-spec state for several PRs.
  Server keeps computing formatted strings (and carrying the formatter
  helpers and tests for them) until Phase 4. Reviewer prefers a single
  PR (see PR comment 2026-04-27); maintaining four sequential PRs adds
  process overhead the team does not want here.
- **Effort:** Medium per phase, large total; ~3–5 weeks elapsed.

### Option C: Wrap formatted strings in a structured type (`FormattedValue`)

- **Approach:** Add a `FormattedValue { double raw = 1; string formatted = 2;
  string unit = 3; }` message and replace every `string formatted_*` with
  `FormattedValue`. Each call-site gets both forms in one field.
- **Pros:** Self-documenting; can't have a raw without its formatted twin
  or vice versa.
- **Cons:** Doesn't actually solve the problem — the server still produces
  the formatted string and clients still use it for display in practice.
  Doesn't let the client choose its own locale. Adds a wrapper layer
  without removing the bad data.
- **Effort:** Medium, but doesn't reach the goal.

## Recommendation

**Option A, single big-bang PR.** This is the maintainer's stated
preference (reviewer comment 2026-04-27: "I'd rather do this in a single
'big bang' PR") and it produces the cleanest end state — one diff, no
transitional shape in the proto, no leftover server formatters or
deprecated fields to clean up later. The wire-compatibility cost is
real but manageable: this project ships to TestFlight and the Play
Store with a minimum-supported-client-version gate, and the app release
that includes the new client code is shipped before the server deploy
that ships the new wire format.

Option B was the safer phased path and is preserved here as a fallback
if reviewer preference changes. Option C was rejected because it papers
over the design problem instead of solving it.

The plan below executes Option A. The pre-merge checklist (release
coordination + minimum-version bump) is non-negotiable — the PR cannot
deploy until the bumped minimum client version is live in TestFlight
and Play Store.

## Open Questions

- **Q1: Should fields like `encouragement_headline` / `encouragement_subtitle`
  / section `title` / `subtitle` get raw counterparts at all, or be deleted
  outright?**
  Recommendation: **delete outright in this PR.** These are pure copy strings
  (`"Over halfway there!"`, `"What I've saved"`) with no numeric counterpart.
  The client already knows what perspective it requested; English copy
  belongs in `app/lib/l10n/app_en.arb` (and `app_es.arb`) per
  `docs/client/i18n.md`. **Encouragement copy that depends on progress
  thresholds** (e.g. "Over halfway there!" vs. "Almost there!" vs. "Keep
  going!") becomes a client-side switch on the raw `quality_time_minutes`
  / `quality_time_minutes_goal` ratio — no server logic needed.

- **Q2: Should theme tokens (`action_button_style: "coral" / "sage" /
  "coral_soft"`) become an enum or be removed?**
  Recommendation: **remove.** A grep of `app/lib/` shows zero readers of
  `actionButtonStyle` outside generated proto code. Reserve the field
  number and name in this PR; the field is dropped from the wire.

- **Q3: For week labels (`week_label`, `week_start_label`, `week_end_label`,
  `date_label`, `day_abbrev`, `date_label` per `InboxDay`), what raw shape
  replaces them?**
  Recommendation: ship a single `int64 week_start_unix_sec` (Monday 00:00
  in the request timezone) and `int64 week_end_unix_sec` (Sunday 23:59:59);
  the client formats `"Feb 17 – 23"` and `"Mon"` from those plus the
  request's IANA `timezone`. Existing `InboxDay.date_number` (already a
  raw `int32`) covers the day-of-month case.

- **Q4: How do we handle `MetricLineItem.amount` / `note` and
  `RecentActivity.value` / `FactorStat.value` / `SubsidiaryMetric.formatted`
  — fields whose meaning is dimension-dependent (could be cents, grams,
  minutes, or a count)?**
  Recommendation: add a sibling `double raw_value = N;` plus an
  `ImpactMetricDimension dimension = N;` to the message (or inherit
  dimension from the enclosing response where it's already specified).
  The client picks the right `ValueHelpers.format*` based on dimension.

- **Q5: Do we need to gate any of this on a minimum client version?**
  Recommendation: **yes, this is the merge-blocking pre-condition.**
  Because Option A is wire-breaking, the PR cannot deploy to prod until
  the new app version is live in TestFlight + Play Store and the server's
  minimum-supported-client-version is bumped to that build (or a build
  carrying these client changes). Confirm via app analytics that older
  versions sit below the project's drop threshold before merge.

- **Q6: Server-side formatter helpers (`formatTimeAgo`,
  `formatDimensionValue`, `formatMetricValue`, `formatDuration`,
  `formatInventoryValue`) — keep, delete, or move?**
  Recommendation: **delete in this PR** alongside the field removals.
  No migration to a shared package needed — they exist only to populate
  the strings being removed. Their tests delete with them.

- **Q7: How do we handle field renumbering in the proto?**
  Recommendation: keep raw fields on **new field numbers**, never on the
  number a deleted string field used. Mark every deleted field as both
  `reserved <number>;` and `reserved "<name>";`. This prevents any
  accidental wire-compat hazard if the slot is later reused, and makes
  the diff explicit about what is gone.

## Implementation Plan

This is a single coordinated PR covering proto, server, Flutter, and
docs. Tasks are grouped by area for review legibility, but they ship
together. The PR cannot merge until the **pre-merge gate** at the end
of this section is satisfied.

### Step 1: Inventory + classification

**Goal:** Every pre-formatted display string field in `portfolio.proto`
and `impact_service.proto` is classified as either "replace with raw
field" or "delete outright" with a rationale.

- [ ] Produce a per-field inventory in a sub-doc
      (`docs/issues/1328-inventory.md`) listing every `string formatted*`,
      `*_label`, `*_summary`, theme-token, and copy-string field in both
      protos. For each: target raw type (`*_cents`, `*_grams`, `*_minutes`,
      `*_unix_sec`, enum, count) **or** "delete-only" with reason.
      Rough buckets: numeric-with-formatter → raw counterpart; pure
      English copy → delete and replace at consumer with ARB key;
      theme tokens → delete (no readers).

### Step 2: Proto edits — `proto/ripls/api/portfolio.proto`

**Goal:** Replace formatted fields with raw fields on new field numbers;
reserve all deleted field numbers and names.

- [ ] On each affected message, add the raw field on a new (next
      unused) field number; keep the old formatted field temporarily
      in the diff to keep the change reviewable, then in the same PR
      delete it and add `reserved <number>; reserved "<name>";`. Note
      that some messages already carry a raw field (`InboxDay.date_number`,
      `MoneySavedDetail.current_usd`, `EmissionsPreventedDetail.current_kg`,
      `GetTimeToSolveDetailResponse.median_minutes`,
      `SolvedRequest.solve_minutes`, `SourceBreakdown.value`,
      `SubsidiaryMetric.value`); for those, only the formatted twin needs
      removing. Specific edits:
      - `GetPortfolioInboxViewResponse`: replace `string shared_value` →
        `int64 shared_value_cents`.
      - `PortfolioMetricsSection`: replace `cost_saved` /
        `time_recovered` / `co2_avoided` / `cost_summary` / `time_summary`
        / `co2_summary` / `title` / `subtitle` → `int64 cost_saved_cents`,
        `int32 time_recovered_minutes`, `int32 co2_avoided_grams`. Copy
        strings (`title`, `subtitle`, `*_summary`) are deleted, not
        replaced — rendered from ARB on the client.
      - `DailyAction`: replace `time_ago` (use existing `unix_sec`),
        `cost_saved` / `time_recovered` / `co2_avoided` →
        `int64 cost_saved_cents`, `int32 quality_time_minutes_delta`,
        `int32 co2_avoided_grams`.
      - `WeeklyMetrics`: replace `cost_saved` / `time_recovered` /
        `co2_avoided` → raw `*_cents` / `*_minutes` / `*_grams`; replace
        `week_label` / `week_start_label` / `week_end_label` →
        `int64 week_start_unix_sec`, `int64 week_end_unix_sec`.
      - `SharedItem`: replace `value` → `int64 value_cents`.
      - `MetricLineItem`: replace `amount` (and `note` where it carries
        a numeric) → `double raw_amount` plus
        `ImpactMetricDimension dimension`.
      - `GetPortfolioMetricDetailResponse`: replace `formatted_total` →
        `double total_value_raw`.
      - `DirectionBreakdown`: replace `count` (string) and
        `formatted_value` → `int32 transaction_count`, `double raw_value`.
      - `RankedUser`: replace `formatted_value` → `double raw_value`.
      - `RankedUserCommunity`: replace `formatted_value` /
        `your_value` / `library_value` → `double raw_value`,
        `int64 your_value_cents`, `int64 library_value_cents`.
      - `CommunityCardStat`: replace `community_value` / `your_value` →
        `double raw_community_value`, `double raw_your_value`.
      - `InboxDay`: delete `day_abbrev` / `date_label` (already has raw
        `date_number`, `day_index`).
      - `DailyItem.action_button_style`: delete and `reserved`.
      - `DailyGroup.label`: delete (rendered from ARB).
      - Encouragement copy fields (`encouragement_headline`,
        `encouragement_subtitle`): delete; client computes copy from
        `quality_time_minutes` / `quality_time_minutes_goal` ratio.

### Step 3: Proto edits — `proto/ripls/api/impact_service.proto`

- [ ] Apply the same pattern. Specific edits:
      - `GetCommunityMetricDetailResponse`: replace `formatted_total` →
        `double total_value_raw`.
      - `SourceBreakdown`: delete `formatted` (raw `value` already
        present); add `string raw_unit` only if dimension cannot be
        inherited from the enclosing response.
      - `TopItem`: replace `item_value` / `cumulative_saved` →
        `int64 item_value_cents`, `double cumulative_saved_raw`.
      - `TopContributor`: replace `formatted_value` → `double raw_value`.
      - `RecentActivity`: replace `value` (string) → `double raw_value`
        plus `ImpactMetricDimension dimension`.
      - `FactorStat`: replace `value` (string) → `double raw_value` plus
        `ImpactMetricDimension dimension`.
      - `MoneySavedDetail`: add `int64 library_value_cents`; delete
        `summary_text`.
      - `TimeSavedDetail`: add `int32 avg_per_loan_minutes`; delete
        `speed_comparison`.
      - `EmissionsPreventedDetail`: delete `equivalence`.
      - `QualityTimeDetail`: delete `equivalence`.
      - `SubsidiaryMetric`: delete `formatted` (raw `value` already
        present); add `ImpactMetricDimension dimension` per Q4.
      - `UtilizationItem`: replace `value` / `savings` →
        `int64 value_cents`, `int64 savings_cents`.
      - `RedundancyGroup`: replace `total_value` → `int64 total_value_cents`.
      - `GetTimeToSolveDetailResponse`: delete `formatted_median`.
      - `SolvedRequest`: delete `formatted_solve_time`.
      - `RankedCommunity`: replace `formatted_value` → `double raw_value`.
      - `LeaderboardMember`: replace `formatted_value` →
        `double raw_value`.
      - `UserDirectionBreakdown`: replace `count` (string) and
        `formatted_value` → `int32 transaction_count`, `double raw_value`.
      - `UserRankedCommunity`: replace `formatted_value` →
        `double raw_value`.
      - `UserImpactTransaction`: replace `formatted_value` →
        `double raw_value`.
      - `GetUserPercentileDetailResponse.this_user_value`: replace →
        `double this_user_raw_value`.
      - `GetCommunityPercentileDetailResponse.this_community_value`:
        replace → `double this_community_raw_value`.
      - `GetUserCommunityImpactDetailResponse.formatted_total`: replace →
        `double total_value_raw`.
      - `GetCommunityUtilizationResponse`: delete `formatted_utilization`
        (raw `utilization_rate` already present); replace
        `formatted_library_value` → `int64 library_value_cents`.
      - `CommunityComparison.ranking_title` / `ranking_detail`: delete.

### Step 4: Server changes (`server/services/portfolio/`, `server/services/impact_metrics/`)

- [ ] Run `npm run generate`. Update every handler that previously
      populated a deleted field to populate its raw replacement instead
      from the same source value that fed the deleted formatter call.
- [ ] **Delete** the formatter helpers (`formatTimeAgo`,
      `formatDimensionValue`, `formatMetricValue`, `formatDuration`,
      `formatInventoryValue`, week-label builders) and their tests.
      Verify with grep that no callers remain. Per CLAUDE.md, do not
      leave the helpers behind as zombies.
- [ ] Server tests: under `server/services/portfolio/*_test.go` and
      `server/services/impact_metrics/*_test.go`, refresh fixtures so
      every response message asserts on raw fields only. Add at least
      one round-trip test per touched response confirming the raw
      values match the source numbers.
- [ ] Run `npm run build` and `go test ./...` (and the integration
      build per CLAUDE.md if any of the touched code reaches integration
      callers): green required.

### Step 5: Flutter changes (`app/lib/`)

**Goal:** Every consumer of a removed field reads the raw counterpart
and resolves copy via `context.l10n`.

- [ ] Audit & migrate consumers:
      `app/lib/presentation/screens/portfolio/*.dart`,
      `app/lib/presentation/screens/impact_metrics/*.dart`,
      `app/lib/presentation/widgets/daily/*.dart`,
      `app/lib/presentation/widgets/impact/*.dart`,
      `app/lib/presentation/models/impact_metrics_content_adapters.dart`,
      `app/lib/presentation/models/drill_down_data.dart` /
      `drill_down_data.freezed.dart`,
      `app/lib/presentation/widgets/content/content_impact_tiles.dart`,
      `app/lib/presentation/widgets/magazine_thumbnail.dart`,
      `app/lib/core/utils/drill_down_builder.dart`.
- [ ] For each consumer, replace the formatted-field read with
      `ValueHelpers.formatSavingsMoney(msg.costSavedCents)` /
      `formatCO2Grams(msg.co2AvoidedGrams)` /
      `formatTimeMinutes(msg.timeRecoveredMinutes.toDouble())`.
- [ ] Replace server-rendered date/week labels with
      `DateTimeFormatter.formatDayAndDate(msg.weekStartUnixSec)` and a
      new `DateTimeFormatter.formatWeekRange(startUnixSec, endUnixSec)`
      helper (added to `app/lib/core/utils/date_time_formatter.dart`).
- [ ] Replace `time_ago` reads with the existing `timeago` package
      already used elsewhere (`timeago_short_messages.dart`).
- [ ] For each deleted English copy field, add an ARB key. Required
      steps per `docs/client/i18n.md`:
      1. Add the key to `app/lib/l10n/app_en.arb` with `@key` metadata
         (placeholders declared) **and** `app/lib/l10n/app_es.arb`.
      2. Use existing key prefixes: `portfolio*`, `portfolioDetail*`,
         `impact*`, `communityMetrics*`, `metricDetail*`.
      3. Use ICU `{count, plural, ...}` and `{name}` placeholders for
         strings that interpolate a number or person.
      4. Resolve in widgets via `context.l10n.<key>`. Viewmodels must
         not resolve strings (architecture rule in `docs/client/i18n.md`).
      5. Run `npm run generate` (invokes `flutter gen-l10n`) and
         `npm run lint:dart:i18n` to confirm no hardcoded strings.
- [ ] For dimension-dependent values (`SubsidiaryMetric`,
      `MetricLineItem`, `RecentActivity`, `FactorStat`), add a Dart
      helper `ImpactValueFormatter.format(double rawValue,
      ImpactMetricDimension dim, AppLocalizations l10n)` under
      `app/lib/core/utils/` that dispatches to the existing
      `ValueHelpers.format*` based on dimension. Keeps the switch in
      one place rather than scattering it across consumers.
- [ ] Encouragement-copy switch: a small utility in
      `app/lib/core/utils/` returns the right localized headline /
      subtitle key based on the `quality_time_minutes` /
      `quality_time_minutes_goal` ratio (e.g. <0.25, 0.25–0.5, 0.5–0.9,
      >=0.9). Widget reads `context.l10n.<key>`.
- [ ] Update Flutter widget tests under `app/test/presentation/` to
      feed raw fields and assert the rendered string still matches.
      Widget tests must wire `localizationsDelegates` and
      `supportedLocales` per `app/test/helpers/l10n_helpers.dart`.
- [ ] Run `cd app && flutter test --reporter expanded`.

### Step 6: Docs + lint

- [ ] Add a moratorium note to `docs/proto_conventions.md`: "Display-only
      string fields (`formatted_*`, `*_label`, copy strings, theme
      tokens) are not allowed in `portfolio.proto` /
      `impact_service.proto`. Add a raw numeric (`*_cents` / `*_grams` /
      `*_minutes` / `*_unix_sec`) or an enum and let the client format."
- [ ] Add a lint check (script under `scripts/`, wired into
      `npm run lint:proto`) that fails if a new field whose name matches
      `^formatted_` or ends in `_label` is added to either proto file.
- [ ] Update `docs/reviews/reports/api_design_review.md` to mark Issue
      #3 / Issue #7 resolved.

### Step 7: Pre-merge release gate (blocks merge)

This Option A plan is wire-breaking. The following must be true before
merging the PR to `main` and deploying the server:

- [ ] App build that contains the Step 5 Flutter changes is shipped to
      TestFlight and Play Store and has had time to roll out.
- [ ] Server's minimum-supported-client-version is bumped to a build
      that contains the Step 5 changes (or the cutoff is set to a build
      that the team agrees is acceptable to drop).
- [ ] App analytics confirm the share of traffic on pre-cutoff builds
      is below the project's drop threshold.
- [ ] Coordinated deploy: client store update goes out first; server
      deploys after.

**Validates:** `portfolio.proto` and `impact_service.proto` carry only
raw numerics, IDs, and enums. `npm run build`, `go test ./...`,
`npm run test:app`, `npm run lint:dart:i18n`, and `npm run lint:proto`
all green. Issue #1328 closes when the PR ships and the deploy
completes.

## Related Issues

- **Review report `docs/reviews/reports/api_design_review.md` §3** — the
  source of this issue. Lists `PortfolioService` pre-formatted-string
  pattern as the single largest i18n risk in the codebase.
- **Issue #7 (historical)** — same pattern was first flagged on
  `DailyService`; that service was replaced by `PortfolioService` and
  the problem grew ~4×, prompting this re-filing at High severity.
- **`docs/proto_conventions.md`** — receives a new subsection in Step 6
  formalizing the moratorium on display-only string fields in these
  two protos.
