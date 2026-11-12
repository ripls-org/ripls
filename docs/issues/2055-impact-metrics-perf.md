# Issue #2055: perf: GetCommunityImpactMetrics is 10+ seconds, blocks community page render

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2055
**Filed:** 2026-05-19T20:03:11Z | **Last activity:** 2026-05-20T17:48:44Z
**Labels:** none (triage comment adds Bug + P1) | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

`GetCommunityImpactMetrics` consistently takes **10–11 seconds** on dev, observed across three back-to-back community openings during a Workshop carousel → community detail smoke test. The community-detail screen renders a blank cream background with a centered spinner until the RPC returns; every other RPC on the same flow (`GetCommunity`, `ListCommunityEvents`, `ListMyTransfers`, …) returns in 50–270 ms, so the spinner is concentrated entirely on this one call.

`GetCommunityMetricDetail` shows the same pathology at half the magnitude (~4.3 s) on the metric-drilldown path.

The client side caches the response — `CacheAudit` shows `impact:community:<id>:metrics_response` hits after the first open — so the 10 s pain is only on **cold** load. But every navigation to a community the user hasn't opened in the current session (or recently, given the 5-minute `CACHE_TTL_MINUTES` on dev) hits it. That makes the symptom a P1 closed-beta blocker: a user tapping into a community sees nothing useful for 10 s on first contact, and again every few minutes for communities they revisit.

The issue body's "likely culprits" ranking proposes four candidates: per-period unbounded scans, N+1 across communities or members, synchronous cold AI dependency, missing server-side cache. Code reading after filing the issue points clearly at culprit #2 (N+1 across communities) as the dominant cost, with culprits #1 and #4 as accelerants.

## Context from Comments

One comment, from `briangbrown` (2026-05-20):

> ## Triage update — 2026-05-19
>
> - Issue Type: (none) → Bug
> - Priority: (unset) → P1
>
> Engineering/product driver per triage doc § P1 — closed-beta blocker or P1 carry-forward.

No disagreement or alternative angles in the discussion. The maintainer (issue author) is the same person who triaged it, so direction is single-voice.

## Root Cause (from codebase walk)

The dominant 10 s cost is `computePercentiles` in [`server/services/impact_metrics/percentile.go:206`](../../server/services/impact_metrics/percentile.go):

```go
allMsgs, err := store.ListAll(ctx, &models.Community{})  // every community on the platform
...
for _, comm := range allCommunities {
    if comm.Deleted != nil { continue }
    calc := impactlib.NewCalculator(store, cfg)
    savings, err := calc.CalculateImpactSavings(ctx, comm.Id)  // 6 SELECTs per call
    ...
}
```

`CalculateImpactSavings` ([`server/impact_metrics/calculator.go:235`](../../server/impact_metrics/calculator.go)) issues **six queries** per invocation:

1. `QueryByField[Transfer](community_id)`
2. `GetByIDs[Gear]` (for fallback impact estimates)
3. `QueryByField[CommunityRequest](community_id)`
4. `GetByIDs[Request]`
5. `QueryByField[CommunityExperience](community_id)`
6. `GetByIDs[Experience]`

With **N communities on the platform**, `computePercentiles` issues **6 × N + 1** queries per `GetCommunityImpactMetrics` call. On dev with the prod-shaped fixture set (~100 communities), that's ~600 SELECTs against shared connection-pool capacity. At ~15 ms each (typical for these batch fetches once warm) that's 9 s — matching the observed 10–11 s with overhead.

Secondary contributors compound the issue:

- **Within one community's metrics call, the same tables are queried multiple times** without any per-request memoization across `CalculateActivityCounts`, `CalculateImpactSavings`, `CalculateTotalValue`, `CalculateCo2Potential`, `CalculateTimeToSolveMedian`. `CommunityGear`, `Transfer`, `CommunityRequest`, `CommunityExperience` all get re-fetched 2–4 times in the same handler call. Within a single community's calc, this is ~20 extra queries.
- **`GenerateInsights`** ([`server/impact_metrics/insights.go`](../../server/impact_metrics/insights.go)) queries `Transfer` **four more times** — once in each of `computeTrendInsight`, `computeConcentrationInsight`, `computeDayPatternInsight`, `computeIdleItemsInsight`. None share the upstream calculator's already-fetched slice.
- **No server-side cache.** Every call recomputes from scratch even when nothing has changed since the previous call.

The issue's culprit #3 (cold AI / external dependency) is **not** the cause — `GenerateInsights` is pure Go aggregation; no AI call is made on this path.

## Potential Solutions

### Option A: Precomputed per-community denominators, live numerator (recommended)
- **Approach:** Add a `community_impact_metrics_snapshot` table (`models.CommunityImpactSnapshot` Proto-SQL row) with one row per community: `{ community_id, money_total_usd, co2_total_grams, qt_total_minutes, updated_at_unix_sec }`. **Crucially, the snapshot does *not* store percentile ranks** — only the per-community totals that feed into the denominator.

  At request time, `GetCommunityImpactMetrics`:
  1. Computes the **calling community's** totals live from authoritative tables (`Transfer`, `CommunityRequest` → `Request`, `CommunityExperience` → `Experience`, etc.). Always fresh.
  2. `storage.ListAll` of the snapshot table to fetch every other community's precomputed totals (the denominator).
  3. Counts how many snapshot rows have `money_total_usd < this_community_money_total` for each dimension → percentile.
  4. **Upserts the calling community's freshly-computed totals into the snapshot row as a side effect** — so the table is self-healing: every active community refreshes itself every time someone opens it.

  Backstop: a small periodic job sweeps communities missing from the snapshot (newly-created ones nobody has opened yet) and inserts a row. Can run once an hour or even daily — the snapshot tolerates being incomplete because the only consequence is "community X is missing from the denominator pool of N," which moves percentiles by ~1/N.
- **Pros:**
  - **The user's own numbers are always fresh.** Numerator is computed live from authoritative tables; only the denominator (other communities' totals) can drift, and ranks move slowly because aggregate totals do.
  - **Eliminates `N × 6` at request time** — the denominator read is one `ListAll` over a small fixed-shape table (~50 bytes per row). 10k communities = ~50 ms one query instead of 60s of fan-out.
  - **Self-healing without aggressive ticks** — every modal-open refreshes that community's row. Active communities stay fresh as a side effect of being looked at; dormant communities don't matter (their totals don't change).
  - Simpler data model than precomputed ranks: one row, four scalars. No background recomputation of ranks; ranks fall out of the in-memory reduce pass.
  - No "snapshot drift" warning case — there's nothing to be stale about for the user's own view.
- **Cons:**
  - New schema (one Proto-SQL table). Routine in this codebase but still surface area.
  - Snapshot upsert on every request adds ~5–10 ms write. Acceptable; this is what makes it self-healing.
  - Brand-new community → has no snapshot row until first modal open → gets `null` percentile for that one call (or a backstop-job insertion). Acceptable; new communities don't have meaningful percentile data anyway.
- **Effort:** small–medium.

**Speed estimate (with Phase 1 + Option A both in place):**

| Step | Queries | Latency |
|------|---------|---------|
| `auth.RequireActiveCommunity` | 1 | ~5 ms |
| Live calc — calling community's `CommunityDataset` load | ~8 (one per source table) | ~80–120 ms |
| Aggregations on preloaded dataset (`CalculateActivityCounts`, `…ImpactSavings`, `…TotalValue`, `…Co2Potential`, `…TimeToSolveMedian`, `GenerateInsights`) | 0 | ~5 ms in-memory |
| `storage.ListAll(CommunityImpactSnapshot)` | 1 | ~10–50 ms (linear in N rows of ~50 bytes) |
| In-memory percentile reduce (count-below) | 0 | ~50 µs |
| Upsert calling community's snapshot row | 1 | ~5–10 ms |
| **Total** | **~11** | **~110–190 ms** |

That's a **~50–100× improvement** over today's 10 s on dev and **5–10×** over today's 1–2 s on prod-small. Most importantly, **scale-invariant**: the only N-dependent step (`ListAll`) reads tiny fixed-shape rows, so the wall-time stays in the same band whether the platform has 30 communities or 30,000.

### Option B: Per-request memoization + push aggregations into SQL
- **Approach:** Introduce a request-scoped `ImpactDataCache` shared across `CalculateActivityCounts`, `CalculateImpactSavings`, `CalculateTotalValue`, `CalculateCo2Potential`, `CalculateTimeToSolveMedian`, and the insight functions. Fetch `Transfer`, `CommunityGear`, `CommunityRequest`, `Request`, `CommunityExperience`, `Experience` once at the top of `getCommunityImpactMetrics` and pass the slices/maps into each calculator function. For `computePercentiles`, replace the per-community in-memory fold with a single PostgreSQL aggregate query that returns `(community_id, sum_money, sum_co2, sum_qt)` across all communities in one SELECT.
- **Pros:**
  - Same-call redundancy disappears (~20 queries dropped per handler call).
  - `computePercentiles` collapses from `N × 6` to one SQL round-trip with `GROUP BY community_id`. PostgreSQL is the right tool for this aggregation.
  - No new schema, no staleness window — every call sees authoritative data.
  - The pattern (preload + push-down) is already CLAUDE.md §SQL Efficiency house style.
- **Cons:**
  - The aggregate SQL is non-trivial. `ImpactEstimate` is a Proto field on Transfer/Request/Experience, stored as JSONB in our Proto-SQL layout. Reaching into a nested JSONB structure for `money_saved.value_usd.mean` across millions of rows means either (a) using PostgreSQL JSONB operators (slower than columnar but acceptable) or (b) projecting the relevant scalars into denormalized columns at write time. The latter touches every write path that mutates these protos — large blast radius.
  - Still O(N) communities in the percentile SQL, just one query instead of N×6. Fine until we have 10k+ communities; at that scale Option A's snapshot approach wins.
- **Effort:** medium (without denormalization) → large (with denormalization).

### Option C: Server-side response cache
- **Approach:** Wrap `GetCommunityImpactMetrics` with an in-process LRU keyed on `(community_id)` with a 30–60 s TTL. Invalidate on relevant `CommunityEvent` types (`TRANSFER_COMPLETED`, `REQUEST_FULFILLED`, `EXPERIENCE_COMPLETED`, `GEAR_SHARED`, member join/leave). Cold first call is still 10 s; every subsequent call within the TTL window is sub-millisecond.
- **Pros:**
  - Lowest implementation cost. No schema, no background job, no SQL rewrite.
  - Mitigates the repeat-visit case the client cache doesn't cover (cross-user reuse — same community opened by user A and user B in quick succession).
- **Cons:**
  - **First-visit user still waits 10 s.** Does nothing for the carousel-cold case which is the reported symptom.
  - Cache-invalidation surface needs every event type that affects any of the 6 query results to be wired in or risk stale data.
  - In-process LRU is per-instance; with horizontal scaling, hit rate degrades as instance count goes up.
- **Effort:** small.

### Option D: Drop percentiles from the cold-load response, fetch on demand
- **Approach:** Remove `PercentileMoney/Co2/QualityTime` from `GetCommunityImpactMetricsResponse`. Render the community page without those numbers initially. The metric-detail screen already calls `GetCommunityPercentileDetail` separately ([`server/services/impact_metrics/percentile.go:21`](../../server/services/impact_metrics/percentile.go)); make that the only place percentiles are computed. Community-detail shows percentiles only on drill-down.
- **Pros:**
  - Removes the 10 s blocker entirely.
  - Doesn't require any backend optimization to ship.
- **Cons:**
  - Product regression: percentile is a top-of-screen surface today. Removing it changes the page's visual hierarchy.
  - The detail-screen percentile path is itself slow (it's the same `N × 6` pattern reused, hence the 4.3 s `GetCommunityMetricDetail`). So we'd need Option A or B eventually anyway.
  - Feels like papering over the root cause.
- **Effort:** small.

## Recommendation

**Option A (precomputed denominators, live numerator)** as the durable fix, with **Option B's per-request memoization** as a Phase 1 quick-win that lands first. Order matters:

1. **Phase 1 — Per-request memoization (Option B partial).** Removes ~20 redundant queries per handler call and is largely mechanical. On dev (N≈200) drops `GetCommunityImpactMetrics` from ~10 s to ~3–5 s (the residual cost is still `N × 6` for percentiles). On prod-small (N≈30) this alone gets the handler down to ~200–400 ms — likely good enough until prod scales further. Low risk; no schema change; no staleness window.
2. **Phase 2 — Precomputed-denominator snapshot (Option A).** Makes the handler **scale-invariant**: ~110–190 ms regardless of platform size, because the only N-dependent step reads tiny fixed-shape rows. The user's own numbers stay live; only the denominator (other communities' totals) ages, and aggregate totals move slowly enough that minute-to-hour-scale staleness is invisible. Self-healing — every modal-open refreshes that community's row.
3. **Phase 3 — Defensive server-side cache (Option C).** Belt-and-suspenders for the cross-user repeat-visit case and for hot communities (every workshop carousel community is hot for that user's session). Probably skippable if Phase 2 lands.

Given the user/prod observation that non-power users on prod are already at 1–2 s, **Phase 1 alone may be enough to ship and close the P1**. Phase 2 is the preventative measure for when prod's community count grows past the point where `N × 6` stops fitting in the latency budget. Don't gate Phase 1 on Phase 2; ship Phase 1 first, observe, then decide on Phase 2.

Option D is not recommended: it sacrifices the product surface to avoid the engineering work, which isn't that bad once Phase 1 lands.

A note on the **observability** that lets us **verify** the fix:
- Wrap the new handler in `storage.WithQueryStats(ctx)` and emit a `Debug` log line with `query_count` per request. This is the standard pattern in `docs/server/observability.md`.
- Add an `AssertMaxQueries` test in `server/services/impact_metrics/` to lock in the post-fix query budget — exactly the pattern called out in CLAUDE.md §SQL Efficiency.

## Open Questions

- **Do we need a periodic backstop job at all?** — Recommendation: yes, but tiny and infrequent. Every modal-open self-heals the calling community's snapshot row, so active communities stay current as a side effect. The backstop only matters for brand-new communities that haven't been opened. A daily sweep that inserts rows for any missing community is enough; cadence is not load-bearing.
- **Where should the snapshot live in proto/models?** — Recommendation: new file `proto/ripls/models/community_impact_snapshot.proto`. Independent enough from `community.proto` to live alone, and it keeps the snapshot type out of the `Community` message itself (which would force a backfill of every existing row).
- **Should the snapshot table be append-only (immutable, time-series) or upserted (one row per community)?** — Recommendation: upserted, `(community_id)` primary key. We only need the current value; history isn't surfaced anywhere.
- **Does the per-request memoization need to be a typed struct, or just an `ImpactDataCache` map?** — Recommendation: typed struct (`type CommunityDataset struct { Transfers []*Transfer; CommunityGear []*CommunityGear; … }`). Makes the contract explicit and lets the test files type-check the cache shape.
- **What if the calling community's snapshot row is missing on first open?** — Recommendation: compute the percentile from the partial denominator anyway (N is the count of rows present, not the count of communities on the platform). The upsert at the end of the request fills it in for next time. This is the simplest correct behaviour and degrades gracefully.
- **Should `GetCommunityMetricDetail` (the 4.3 s call) be part of this issue's scope?** — Recommendation: yes for Phase 1 (per-request memoization) since it shares calculator code paths; carve out a separate follow-up for any detail-specific optimization. The shared-helper refactor in Phase 1 is the natural place to fix both.

## Audit Findings (`/ripls-audit` 2026-05-20)

Cross-checked against `docs/server/architecture.md`, `docs/server/observability.md`, `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/testing.md`, `docs/client/i18n.md`, `docs/client/accessibility.md`. The following adjustments folded into the Implementation Plan below:

1. **Background-job pattern is not Cloud Scheduler.** Existing jobs (`server/jobs/community_purge.go`, `server/jobs/activity_digest.go`) expose `Run(ctx) error` and are driven from `server/main.go` via `time.NewTicker(interval)` inside a `logging.GoSafe(ctx, "name", func() { ... })` goroutine. The snapshot job must follow this shape; the goroutine wrapper is CI-enforced by `server/cmd/check-goroutines` per `architecture.md` §Goroutine Safety.
2. **Library/service boundary.** `server/impact_metrics/` is the shared library (no RPC handlers) and `server/services/impact_metrics/` is the service implementation. Per `architecture.md` §1 (Service Independence) and §Pattern 1 (Standalone Libraries), `CommunityDataset` and its loader must live in the shared library — the new background job must consume it through that library, not through the service package.
3. **Goroutine safety in main.go.** Phase 2's ticker loop must be wrapped in `logging.GoSafe`. Every `go func()` in `server/` without a recover defer fails the `check-goroutines` lint.
4. **Error wrapping.** Per `architecture.md` §Error wrapping (and `scripts/check_codeinternal.js`), the handler stays on `connecterr.Internal(ctx, "GetCommunityImpactMetrics", err, kv...)`. Don't regress to bare `connect.NewError(connect.CodeInternal, err)` — it leaks raw storage errors to the wire.
5. **Structured logging field names.** Per `observability.md` §Standard Field Names: the new log lines use `operation` for the function name and entity-specific keys like `community_id`. The `query_count` debug field is fine — it's not an entity ID. Use `logging.LoggerWithContext(ctx)` consistently; never `log.Printf`. Levels: `Debug` for query-count instrumentation, `Info` for "snapshot tick complete", `Warn` for "snapshot drift > 2× refresh interval", `Error` only for actual system failures.
6. **Authorization preserved on the snapshot read path.** The current handler calls `auth.RequireActiveCommunity(ctx, store, communityID)` before computing anything (`server/services/impact_metrics/metrics.go:31`). The snapshot-first path must do the same — reading the precomputed row does not loosen authorization.
7. **No raw SQL in services.** Per `architecture.md` §Storage Best Practices, all persistence flows through `storage.ProtoSQLStorage`. The Option B "aggregate query across all communities" would require adding a helper to `server/storage/` rather than embedding SQL in `server/impact_metrics/`. Phase 1's preferred shape — `LoadAllCommunitiesDatasets` calling `QueryByField` once per source table without a community filter — keeps the work inside the existing Proto-SQL primitives.
8. **Proto-field conventions.** Per CLAUDE.md §Proto Field Conventions: `community_id` is the PK and stays required; all value fields (totals, ranks, `updated_at_unix_sec`) are `optional`. Per CLAUDE.md §Proto Comments: snapshot proto comments describe the data, not the storage table, refresh cadence, vendor names, or server pipeline mechanics — those belong in the README / this plan.
9. **Client side unchanged.** The proposed RPC response shape stays identical (`api.GetCommunityImpactMetricsResponse`), so `CommunityImpactMetricsNotifier` (`app/lib/presentation/viewmodels/community_impact_view_model.dart`) and `impactMetricsRepositoryProvider` need no changes. The existing client cache (TTL 30 min default, 5 min on dev per `CACHE_TTL_MINUTES`) keeps working; per `docs/client/caching.md` §Repositories own caching, no ViewModel changes are required.
10. **Client cache-invalidation event is independent.** `impactCacheInvalidationProvider` fires after experience completion / request fulfillment to drop the client cache. The server-side snapshot is *its own* materialization on a separate cadence — the client invalidation event doesn't need to trigger server snapshot refresh.
11. **Testing layer convention.** Per `architecture.md` §Testing Strategy: unit tests live alongside implementation (`*_test.go`). Add the `AssertMaxQueries` regression test to a new `server/services/impact_metrics/service_query_count_test.go` (focused) or extend the existing `service_test.go` — both are acceptable. Snapshot-job tests go in `server/jobs/community_impact_snapshot_test.go` alongside the implementation.
12. **i18n / a11y unaffected.** No new user-visible strings, no new interactive widgets — `docs/client/i18n.md` and `docs/client/accessibility.md` have no requirements to flag here.

## Implementation Plan

### Phase 1: Per-request memoization & redundant-query elimination

**Goal:** Drop `GetCommunityImpactMetrics` to ~3–5 s by removing same-call query duplication. No schema changes, no behavior changes.

**Tasks:**
- [ ] In **`server/impact_metrics/` (shared library, not the service package)**, introduce `CommunityDataset` struct: holds preloaded `Transfers`, `CommunityGear`, `Gear` map, `CommunityRequests`, `Request` map, `CommunityExperiences`, `Experience` map, `CommunityUsers`, `CommunityEvents`. Place in a new `dataset.go` alongside `calculator.go` (per `architecture.md` §File Organization, short focused files; current `calculator.go` is already 978 lines).
- [ ] Add `func LoadCommunityDataset(ctx, store, communityID) (*CommunityDataset, error)` that issues exactly one `storage.QueryByField` per source table. No raw SQL — uses existing Proto-SQL primitives per `architecture.md` §Storage Best Practices. Wire it into `getCommunityImpactMetrics` at the top of the handler, after the existing `auth.RequireActiveCommunity` check.
- [ ] Refactor `CalculateActivityCounts`, `CalculateImpactSavings`, `CalculateTotalValue`, `CalculateCo2Potential`, `CalculateTimeToSolveMedian` to accept a `*CommunityDataset` instead of querying themselves. Keep existing per-query helpers as private fallbacks (or delete entirely once callers are migrated).
- [ ] Refactor `GenerateInsights` (in `server/impact_metrics/insights.go`) so all four insight helpers (`computeTrendInsight`, `computeConcentrationInsight`, `computeDayPatternInsight`, `computeIdleItemsInsight`) accept the preloaded `[]*Transfer` slice instead of re-querying. Drop the four `store.QueryByField(ctx, "community_id", communityID, &models.Transfer{})` calls.
- [ ] In `computePercentiles` (`server/services/impact_metrics/percentile.go:206`), replace the per-community `calc.CalculateImpactSavings` loop with one batched dataset-load + reduce pass. Concretely: `LoadAllCommunitiesDatasets(ctx)` issues `QueryByField` once per source table (no community filter), partitions in memory by `community_id`, and feeds per-community datasets into `CalculateImpactSavings`. Same logic, ~6 queries total instead of `6 × N`.
- [ ] Keep all error returns on the `connecterr.Internal(ctx, "GetCommunityImpactMetrics", err, "detail", ...)` path (no regression to bare `connect.NewError(connect.CodeInternal, …)` — `scripts/check_codeinternal.js` gate).
- [ ] Add `storage.WithQueryStats` instrumentation: in `getCommunityImpactMetrics`, attach `WithQueryStats(ctx)` then `GetQueryStats(ctx)` at the end of the handler and emit a `Debug` log line via `logging.LoggerWithContext(ctx).DebugContext(ctx, "metrics query stats", "operation", "GetCommunityImpactMetrics", "community_id", communityID, "query_count", stats.Count, "duration_ms", elapsed)`. Field names follow `observability.md` §Standard Field Names.
- [ ] Add a regression test `server/services/impact_metrics/service_query_count_test.go` (or extend `service_test.go`) using `storage.AssertMaxQueries(t, ctx, 12, fn)` that locks the budget at ≤ 12 queries per `GetCommunityImpactMetrics` call.

**Validates:** Query count drops from ~600 to ~12 (one batch per source table + a few targeted `GetByIDs`). Wall-time drops measurably; we'll observe the new floor before moving to Phase 2.

### Phase 1.5: Extend dataset preload to `GetCommunityMetricDetail`

**Goal:** `GetCommunityMetricDetail` is on the same hot path (tapped from the community-detail screen to drill into a metric) and was unchanged by Phase 1. Post-Phase-1 measurement on the populated dev fixture shows **1081 queries / 251 ms** for this RPC — driven by:

- `loadFulfilledRequests` (`metric_detail_calculator.go:357`) iterates `CommunityRequest` and issues one `GetByID(Request)` per row — ~190 queries on this fixture.
- `loadCompletedExperiences` (`metric_detail_calculator.go:396`) does the same for experiences — ~210 queries.
- `transferImpact` fallback (`metric_detail_calculator.go:696`) issues one `GetByID(Gear)` per transfer that lacks an inline `ImpactEstimate` — up to ~350 queries.
- `computeMoneyDetail` (`metric_detail_calculator.go:792`) issues one `GetByID(Gear)` per `CommunityGear` — ~146 queries.
- `EntityCache.GetGear` calls in `computeTopItems` / `computeTopContributors` / `computeFactors` / `metric_detail_comparison.go` — another ~100+ on this fixture.

At Cloud SQL latencies (~40–50 ms/query), 1081 × 40 ms = **~40 s** — meaning the issue's reported 4.3 s was likely truncated by a client-side timeout. On a real prod-shape dataset this RPC would be deeply broken.

**Tasks:**
- [ ] Add `(*MetricDetailCalculator).ComputeMetricDetailFromDataset(ctx, ds *CommunityDataset, dimension, period)` that accepts a preloaded dataset. The existing `ComputeMetricDetail` becomes a thin wrapper that loads a dataset then delegates.
- [ ] Replace `loadCompletedTransfers`, `loadFulfilledRequests`, `loadCompletedExperiences` with pure filters over `ds.Transfers` / `ds.CommunityRequests` + `ds.Requests` / `ds.CommunityExperiences` + `ds.Experiences`. Eliminates ~400 per-row `GetByID` calls in one stroke.
- [ ] Add `transferImpactFromDataset(transfer, gearMap)` that reads fallback gear from a preloaded map. Update `computeSourceBreakdown`, `computeTimeSeries`, `computeMonthlyBars`, `computeRecentItems` to thread the gear map through. Eliminates per-transfer `GetByID(Gear)` calls.
- [ ] Replace `computeMoneyDetail`'s per-`CommunityGear` `GetByID(Gear)` loop with a single pass over `ds.CommunityGear` + lookup in `ds.Gear`. Same shape for `computeFactors`, `computeLibraryItems`, `metric_detail_comparison.go`.
- [ ] Add `NewEntityCacheFromDataset(storage, ds)` constructor that prepopulates the cache from `ds.Gear`. Update the two `NewEntityCache(c.storage)` call sites at `metric_detail_calculator.go:128, 302` to use it. Existing helpers that call `cache.GetGear(t.GearId)` keep working — the lookups become map hits instead of DB round-trips.
- [ ] Keep `auth.RequireActiveCommunity` at the top of `getCommunityMetricDetail` — the dataset-preload path does not loosen authorization.
- [ ] Out of scope for this phase: cross-community paths in `metric_detail_calculator.go:738/755/799/820/835` (portfolio queries, cross-community comparison). They're a separate optimization and the percentile-style batched-load pattern fits them later.
- [ ] Add regression test `server/services/impact_metrics/service_query_count_test.go::TestGetCommunityMetricDetail_QueryBudget` that asserts ≤ 25 queries on a community with 200+ rows in each junction table, with 60+ communities on the platform. Mirrors the existing Phase 1 budget test.

**Validates:** `GetCommunityMetricDetail` query count drops from ~1081 to ≤ 25 on the populated fixture, and stays there as community count grows. Wall-time on dev fixture should drop from ~250 ms to under ~50 ms.

### Phase 2: Precomputed-denominator snapshot

**Goal:** Make `GetCommunityImpactMetrics` scale-invariant. Cold-and-warm both return in ~110–190 ms regardless of platform community count, because the only N-dependent step reads tiny fixed-shape rows.

**Tasks:**
- [ ] Define `proto/ripls/models/community_impact_snapshot.proto`:
  - `string community_id = 1;` (PK — **not** `optional`, PKs are always present)
  - `optional float money_total_usd = 2;` `optional float co2_total_grams = 3;` `optional float qt_total_minutes = 4;`
  - `optional int64 updated_at_unix_sec = 5;`
  - **Note: no precomputed ranks.** Ranks are derived at request time from the in-memory reduce over all snapshot rows. Storing them would force snapshot recomputation across all communities every time any one community's totals change, defeating the "slow-moving denominator" property that makes this design work.
  - Proto comments describe meaning, not mechanism, per CLAUDE.md §Proto Comments. E.g. `money_total_usd`: "Cumulative dollar-value of completed transfers, fulfilled requests, and completed experiences attributed to this community." Don't reference refresh cadence, the snapshot table, or "self-healing on modal open" — that's internal documentation.
- [ ] Add Proto-SQL storage registration so the table is created (match how existing models register; e.g. follow the pattern in `server/storage/migrations.go` or wherever `models.Community` is registered). PK on `community_id`; secondary index on `updated_at_unix_sec` for the backstop sweep.
- [ ] In `server/impact_metrics/` (shared library), add `CommunityTotals` struct + `func ComputeCommunityTotals(dataset *CommunityDataset) CommunityTotals` that derives `{ money_total_usd, co2_total_grams, qt_total_minutes }` from a preloaded dataset. Pure function, no DB access.
- [ ] In `server/services/impact_metrics/`, replace `computePercentiles` (`percentile.go:206`) with `computePercentilesFromSnapshot(ctx, store, thisTotals)`:
  1. `storage.ListAll[*models.CommunityImpactSnapshot]` — read every row (small fixed-shape).
  2. In-memory: count rows where `money_total_usd < thisTotals.MoneyUsd` for each dimension; percentile = count/total × 100. The calling community is guaranteed to be in the count (its own row gets upserted before the read on subsequent visits; on first visit it's missing and we count from N-of-others, which is the correct partial-denominator behaviour).
- [ ] In `getCommunityImpactMetrics` (`metrics.go`), after the live-calc path returns:
  - Build `thisTotals` from the already-computed `ImpactSavingsResult`.
  - **Upsert the calling community's snapshot row** via `storage.Update[*models.CommunityImpactSnapshot]` (or insert if missing) with `updated_at_unix_sec = now`. Idempotent — naturally safe under concurrent requests since both compute the same value; last writer wins.
  - Call `computePercentilesFromSnapshot` and stitch the percentiles into the response.
- [ ] Implement `server/jobs/community_impact_snapshot_backfill.go`: a tiny job exposing `func (j *CommunityImpactSnapshotBackfillJob) Run(ctx context.Context) error` that finds active communities missing a snapshot row (or with `updated_at_unix_sec` older than a generous threshold, e.g. 7 days) and computes one. Use `server/jobs/community_purge.go` as the structural model. **Goal: catch brand-new communities nobody has opened, not to refresh active data** — active data is refreshed by request-time upserts.
- [ ] Wire the backfill job into `server/main.go` matching the existing pattern (around lines 1140–1340): one-shot first run, then a `time.NewTicker(24 * time.Hour)` loop. **Use `logging.GoSafe(ctx, "community-impact-snapshot-backfill", func() { ... })`** — bare `go func()` is rejected by `server/cmd/check-goroutines`.
- [ ] Keep `auth.RequireActiveCommunity` at the top of the handler — the snapshot-read path does not loosen authorization.
- [ ] Add a unit test `server/jobs/community_impact_snapshot_backfill_test.go`: fixture with 3 communities, two with snapshots and one without; run one tick; assert the missing community now has a row. Use the standard testcontainers setup per `architecture.md` §Testing Strategy.
- [ ] Add an `AssertMaxQueries` test for `GetCommunityImpactMetrics` post-Phase-2: budget ≤ 12 (auth, ~8 dataset queries, snapshot ListAll, snapshot upsert).
- [ ] Add an integration test exercising the self-healing property: open community A's metrics, verify its snapshot row was created; mutate A's data; open A's metrics again; verify the row's totals updated.

**Validates:** `GetCommunityImpactMetrics` ≤ 200 ms p95 on dev fixture and stays there regardless of community count. Confirm self-healing: a freshly-created community's first metrics call returns successfully (partial denominator), and its row appears in the snapshot for the next caller.

### Phase 3: Defensive server-side response cache (optional)

**Goal:** Catch the cross-user repeat-visit pattern (e.g. a community trending in the Workshop carousel; first user warms the snapshot, second user gets the response without recomputing the per-user activity flags).

**Tasks:**
- [ ] Add an in-process LRU keyed on `(community_id, user_id)` with 60 s TTL. Use `golang.org/x/sync/singleflight` for stampede protection + a small ttlmap. Place the cache in the **service package** (`server/services/impact_metrics/`) — it's a service-implementation concern, not a library primitive, so it stays out of `server/impact_metrics/`.
- [ ] Wire it into `getCommunityImpactMetrics` after `auth.RequireActiveCommunity` but before the snapshot/dataset load. **Auth check must run on every request; the cache key includes `user_id` so cross-user leakage is impossible by construction.**
- [ ] Add invalidation on the same `CommunityEvent` types relevant to the snapshot. Hook into the existing `community_event_bus` (`server/community_event_bus`) — `architecture.md` §Pattern 4 (Domain Event Buses) — by subscribing the cache invalidator.
- [ ] Test: hit the same `(community_id, user_id)` twice in quick succession; assert second call is a cache hit (0 queries) via `WithQueryStats`.

**Validates:** Repeat-open of the same community within 60 s by the same user is free server-side. Belt-and-suspenders for the cold-cache window after a snapshot invalidation.

## Related Issues

- None tracked yet — this is the first perf issue logged against the impact metrics path. Worth filing a follow-up if Phase 1 surfaces additional `GetCommunityMetricDetail`-specific costs not covered by the shared `CommunityDataset` refactor.

## Audit Trail

`/ripls-audit` run 2026-05-20 added the "Audit Findings" section above and tightened the Implementation Plan in these ways:
- Phase 1: named the file under `server/impact_metrics/` for `CommunityDataset` (vs the service package), preserved `auth.RequireActiveCommunity` and `connecterr.Internal`, fixed log field names + level (`Debug`, `operation`, `community_id`, `query_count`, `duration_ms`).
- Phase 2: rewrote the job tasks against the actual `Run(ctx) error` + `time.NewTicker` + `logging.GoSafe` pattern used by every existing job (was generic "Cloud Scheduler-shaped"); pinned proto field shapes (PK required, value fields `optional`) and proto-comment rules (no internal mechanics in proto comments); preserved the snapshot-read auth check.
- Phase 3: clarified library/service placement, kept auth on every request, wired cache invalidation through the existing `community_event_bus`.

**Rework 2026-05-20 (post-audit):** Restructured Option A around the user's decomposition insight: store only the slow-moving *denominator* (per-community totals) in the snapshot table, compute the calling community's *numerator* live every request, and self-heal the snapshot via an upsert at the end of each call. This:
- Eliminates the "snapshot drift" warning case — the user's own numbers are always fresh.
- Replaces the "background tick refreshes every community" loop with a "request-time upserts the calling community + tiny daily backfill for never-opened communities" pattern. Much cheaper.
- Drops precomputed-ranks from the snapshot schema — ranks fall out of the in-memory reduce at request time.
- Added an explicit speed-estimate table: ~110–190 ms total, ~11 queries, scale-invariant in community count.
- Surfaced the prod observation (non-power users at 1–2 s) as a recommendation to potentially ship Phase 1 alone and defer Phase 2 as a preventative measure.
