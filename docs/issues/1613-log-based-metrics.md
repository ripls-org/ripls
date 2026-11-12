# Issue #1613: [observability] Replace Prometheus pipeline with log-based metrics for SLI/SLO and alerting

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1613
**Filed:** 2026-04-30 | **Last activity:** 2026-05-20
**Labels:** review:observability | **Assignees:** unassigned
**Priority:** P2 (Task) — per 2026-05-19 triage
**Context docs:** `docs/llms.txt`, `docs/server/architecture.md` (anchor), `docs/server/conventions.md` (anchor), `docs/server/observability.md`, `docs/server/metrics.md`, `docs/server/profiling.md`, `docs/reviews/reports/observability_review.md`, review-prompt "Areas to Review" for observability, error_handling, testing, security, code_quality, performance

## Problem Statement

The project's observability goals — percentile-aware per-RPC latency, alertable
error rates, SLI/SLO burn-rate alerting, and DB-pool / Go-runtime capacity
signals — were originally planned as a Prometheus pipeline (#1585/#1586/#1587,
attempted in PR #1612). That plan hit a hard wall: Google Cloud Monitoring has
no Prometheus remote-write endpoint, so writing Prometheus metrics from Cloud
Run requires new infrastructure (sidecar collector or a bespoke v3
`timeSeries.create` writer). PR #1612 was closed unmerged.

Meanwhile, the structured-logging migration means every HTTP request already
emits a JSON log line (`message="http request"` in
`server/middleware/logging.go`) carrying `duration_ms`, `rpc_method`, `status`,
`request_id`, and `user_id`. Cloud Monitoring **log-based metrics** can build
counters and percentile-capable DISTRIBUTION histograms directly from these
lines — one observability backend, zero new infrastructure, zero new Go
dependencies. This issue adds the log-based metrics, the periodic
pool/runtime stats log emitters they need, four dashboards, and three alerts.

### Corrections to the issue spec (from codebase research)

The issue was filed 2026-04-30; the code has moved and a few of its technical
assumptions don't hold. The plan below incorporates these fixes:

1. **Log field names.** The `"http request"` middleware line has **no
   `operation` or `code` fields**. The actual fields are `rpc_method`
   (`GearService/SaveGear`, empty for non-RPC paths) and `status` (HTTP status
   int) — see `server/middleware/logging.go:38-50`. All label extractors must
   use `jsonPayload.rpc_method` / `jsonPayload.status`. (`operation` exists
   only on service-level log lines, which is what the existing
   `rpc_errors_{env}` metric extracts.)
2. **Error-rate fraction needs a single log source.** The issue proposes
   `rpc_errors_{env} / rpc_requests_{env}`, but `rpc_errors_{env}` is labeled
   by `operation` (service log lines) while a request counter from middleware
   lines would be labeled by `rpc_method` — the label values don't join
   (`SaveGear` vs `GearService/SaveGear`). Instead: **one counter**
   `rpc_requests_{env}` labeled `(rpc_method, status)` built from the
   middleware line; the error fraction is derived at query time by filtering
   `status =~ "5.."` over the same metric.
3. **Log-based metrics cannot be gauges.** Cloud Logging supports only DELTA
   COUNTER (count of matching entries) and DELTA DISTRIBUTION (extracted
   values). Pool "gauges" become **DISTRIBUTION metrics with
   `value_extractor`**, plotted/alerted with percentile aligners. For the
   saturation alert, the server logs a precomputed `db_pool_utilization`
   (in_use ÷ open) so the alert is a simple threshold, not a cross-metric
   ratio.
4. **Slow-query logs are a censored distribution — but an uncensored
   per-request DB signal already exists.** The storage layer logs only queries
   ≥ 100 ms (`slowQueryThreshold`, `server/storage/instrumented_db.go:68-93`),
   and that line has **no `query_kind` field** today. Logging every query is
   not viable (a single load-test run executes ~17.5k queries). However, the
   QueryStats middleware (`server/middleware/query_stats.go`) already emits a
   per-request `"request db stats"` INFO line with `db_queries` and
   `db_duration_ms` (total DB time for the request) — uncensored, one line per
   request. So the DB latency story is two metrics: `db_request_duration_{env}`
   (per-request total DB time, from `"request db stats"`, needs `rpc_method`
   added to that line — it currently carries only `path`) and
   `db_slow_query_duration_{env}` (per-query tail detail, honestly named — it
   answers "how many slow queries, how slow", not "overall DB P95" — with
   `query_kind` added to the slow-query line).
5. **Pool publisher location.** The ticker is now at `server/main.go:1286-1305`
   (not 899-916), calling `ProtoSQLStorage.PublishPoolStats()`
   (`server/storage/protosql.go:757-767`).
6. **The out-of-scope cleanup issue is #1614, not #1615.** #1615 is an
   unrelated version-bump PR. Also note #1614's premise ("unused /metrics
   endpoint") has drifted: `server/metrics` now also feeds the rate-limit
   soak counters and off-app notification counters — see Related Issues.
7. **Streaming RPCs would poison a global P95 alert.** All seven streaming
   RPCs are named `Stream*` (`StreamMessages`, `StreamGenUnifiedCreate`, …);
   their `duration_ms` is the whole stream lifetime (minutes). The latency
   alert must exclude `rpc_method` matching `Stream`.
8. **Alert display names feed the GitHub auto-filer.**
   `cloud_functions/src/gcpAlertsHandler.ts` classifies page-vs-warn severity
   by substring-matching the **condition** display name. New alert conditions
   must be named deliberately (see Phase 4).

## Context from Comments

One comment: the 2026-05-19 triage pass set Issue Type = Task, Priority = P2
("meaningful improvement, not closed-beta blocking"). No design discussion or
disagreement in comments — the issue body itself is the design record, written
after the Prometheus approach failed. The three predecessor issues
(#1585/#1586/#1587) and parent #1520 are closed; PR #1612 (closed, unmerged)
contains the dashboard widget inventory this issue says to reuse.

## Potential Solutions

### Option A: Log-based metrics on existing structured logs (the issue's proposal)
- **Approach:** Terraform `google_logging_metric` resources over the existing
  `"http request"` / `"slow query"` lines plus two new periodic INFO stats
  lines (DB pool, Go runtime); dashboards and alerts on
  `logging.googleapis.com/user/*` metrics.
- **Pros:** Zero new infrastructure or Go deps; single backend (Cloud
  Monitoring) for metrics, dashboards, alerts, and eventual SLOs; builds on
  logs that are already emitted and battle-tested; native
  `ALIGN_PERCENTILE_95` support; all original #1520 goals reachable.
- **Cons:** Log-based metrics have no gauge kind (worked around with
  DISTRIBUTIONs); DB latency is censored at the 100 ms slow-query threshold;
  metric data starts at metric-creation time (no backfill from old logs);
  per-instance signals (pool stats) blur across Cloud Run instances unless
  aligned with percentile aligners.
- **Effort:** Medium (small Go change + ~400 lines of Terraform + validation).

### Option B: Revive Prometheus via OTel Collector sidecar
- **Approach:** Run Google's OTel Collector as a Cloud Run sidecar container
  scraping `/metrics` and writing to Managed Prometheus.
- **Pros:** True gauges and full-fidelity histograms; the `server/metrics`
  package (merged in #1589, still maintained) becomes fully load-bearing;
  PromQL everywhere.
- **Cons:** New always-on sidecar per service (cost, config surface, another
  image to patch); a second metrics pipeline to operate; contradicts the
  "minimal dependencies" project preference; the issue explicitly evaluated
  and rejected this for value already obtainable from logs.
- **Effort:** Medium-large, plus permanent operational surface.

### Option C: Bespoke writer against Cloud Monitoring v3 `timeSeries.create`
- **Approach:** Rewrite `remotewrite.go` from PR #1612 to translate Prometheus
  registry samples into v3 API calls from a server goroutine.
- **Pros:** No sidecar; keeps the in-process Prometheus registry as source of
  truth.
- **Cons:** Custom metric-translation code we own forever (rate limits, label
  mapping, DISTRIBUTION bucket translation are all nontrivial); the v3 write
  path costs per-series ingestion just like log-based metrics; highest-risk
  option for the least differentiated value.
- **Effort:** Large.

## Recommendation

**Option A**, as the issue proposes — with the eight corrections above baked
in. The decisive facts: the structured logs already carry every needed value;
Cloud Monitoring dashboards/alerts/SLOs consume log-based metrics natively;
and both alternatives buy fidelity we don't currently need (true gauges,
sub-100ms DB percentiles) at the cost of new infrastructure. The one real
fidelity loss — censored DB latency — is acceptable because the slow-query
signal ("count and severity of queries over 100 ms") is exactly what the DB
health dashboard is for, and Cloud SQL's own `database/*` metrics (already
alerted on in `terraform/modules/monitoring/main.tf`) cover instance-level
capacity.

## Open Questions

- **Should the error-rate alert count 4xx as errors?** — Recommendation: no.
  Alert on `status =~ "5.."` only; 4xx is dominated by client behavior
  (expired tokens, not-found) and would false-page. The dashboard shows both
  classes since the `status` label preserves them. This matches the existing
  Prometheus `ripls_connect_rpcs_total` guidance of separating RPC-aware error
  rate from raw 4xx/5xx.
- **Keep the issue's `db_query_duration` name despite the 100 ms censoring?**
  — Recommendation: rename to `db_slow_query_duration_{env}` (and a companion
  interpretation note in `docs/server/observability.md`). A metric named
  "query duration" that silently omits 99% of queries will mislead the next
  reader into believing dashboard P95s are overall DB latency. The uncensored
  headline metric is `db_request_duration_{env}` from the existing
  `"request db stats"` line (correction #4), which the issue's spec predates.
- **Where do the new Go stats emitters live?** — Recommendation: a small
  `server/statslog` package exposing pure functions
  (`PoolStatsAttrs(sql.DBStats) []any`, `RuntimeStatsAttrs(prev, cur) []any`)
  with unit tests, wired to tickers in `server/main.go` next to the existing
  pool publisher (which already ticks at 15s — extend that loop rather than
  adding a second DB ticker). Pure-function shape satisfies the "unit tests
  for each new library" rule without needing fake clocks. New multi-file
  package ⇒ needs `doc.go` and `README.md` per repo conventions.
- **How to validate that alerts fire (the issue's synthetic-injection step)?**
  — Recommendation: don't ship fault-injection code. Validate by temporarily
  lowering thresholds in dev Terraform (P95 threshold below observed normal;
  error-fraction threshold to 0 with any natural 5xx, or one manufactured
  `CodeInternal` via a temporarily broken dev dependency) and confirming the
  incident + email + (prod-only) GitHub auto-file path. Restore thresholds in
  the same PR cycle. A dev-only latency-injection flag is reusable but is new
  attack/config surface and can be a follow-up if threshold-lowering proves
  awkward.
- **New Terraform module or the existing `monitoring` module?** —
  Recommendation: extend `terraform/modules/monitoring/` with new files
  `log_metrics.tf`, `dashboards.tf`, and `alerts.tf` — the exact file names
  `docs/server/observability.md`'s Phase 5 roadmap already prescribes. The
  `metrics` module from PR #1612 never merged, all existing log-based
  metrics/alerts already live in `monitoring`, and both env roots already
  instantiate it.
- **Initial P95 latency threshold?** — Recommendation: global 2s over
  non-streaming RPCs, WARN severity, sustained 15m. Calibrate per-RPC (and
  consider excluding `MediaService/UploadMedia`, which legitimately runs
  seconds) after a week of dev data, as the issue already anticipates.
- **Do the new alerts join the GitHub auto-filer's page/warn taxonomy?** —
  Recommendation: yes, deliberately. Name conditions so
  `gcpAlertsHandler.ts` classifies: error-rate condition contains "RPC
  errors" (→ warn, daily-aggregated), pool-saturation condition contains
  "Database" (→ page, consistent with existing `database_*` policies). Let
  the P95 latency alert land in the "unknown" bucket initially (files an
  issue, no `claude-fix`); add a `/latency/i` → warn pattern to the Cloud
  Function only if it proves noisy to triage manually.

## Implementation Plan

### Phase 1: Go stats emitters
**Goal:** Every signal the Terraform metrics need exists as a structured log
line in both environments.
**Tasks:**
- [x] New `server/statslog` package (`doc.go`, `statslog.go`,
      `statslog_test.go`, `README.md`): pure attr-builder functions for pool
      stats and runtime stats.
- [x] Pool stats: extend the existing 15s ticker in `server/main.go:1286-1305`
      to also emit `logger.Info("db pool stats", ...)` with `db_pool_open`,
      `db_pool_in_use`, `db_pool_idle`, `db_pool_wait_count`,
      `db_pool_wait_seconds`, and precomputed `db_pool_utilization`
      (0 when `open == 0`). Keep calling `PublishPoolStats()` (Prometheus
      gauges stay until #1614 decides their fate). Needs a
      `ProtoSQLStorage.PoolStats() sql.DBStats` accessor or equivalent.
- [x] Runtime stats: new 30s ticker goroutine spawned via `logging.GoSafe`
      (the repo's mandatory panic-safe pattern — `server/cmd/check-goroutines`
      fails the build on bare `go func()` without a recover), with a
      `select` on `ctx.Done()` so it terminates on shutdown like the pool
      ticker. Emits `logger.Info("go runtime stats", ...)` with
      `go_goroutines` (`runtime.NumGoroutine()`), `heap_inuse_bytes`, and
      `gc_pause_ms` (mean pause since last tick from
      `MemStats.PauseTotalNs`/`NumGC` deltas; 0 when no GC ran).
      `runtime.ReadMemStats` at 30s cadence is negligible overhead.
- [x] Add `query_kind` to the existing slow-query WARN log in
      `server/storage/instrumented_db.go:89` (the `queryKind()` helper is
      already there).
- [x] Add `rpc_method` to the `"request db stats"` line in
      `server/middleware/query_stats.go` (reuse `extractRPCMethod`, same as
      the logging middleware) so the per-request DB duration metric can be
      labeled consistently with the RPC metrics.
- [x] Unit tests for attr builders and the `query_kind` log field;
      `npm run build` (lint + compile) and `go test ./server/...`.
**Validates:** Log lines appear locally (`--log-format=text`) and in dev Cloud
Logging with the exact `jsonPayload` field names the metric filters will
reference. INFO level keeps them out of the `all_errors` alert path.

### Phase 2: Log-based metrics (Terraform)
**Goal:** All time series exist and accumulate data in dev (log-based metrics
have no backfill — creating them early maximizes calibration data for
Phase 4 thresholds).
**Tasks:**
- [x] New `terraform/modules/monitoring/log_metrics.tf` (implementation also
      added a `db_slow_queries_{env}` DELTA counter next to the duration
      distribution — distributions can't be rate-aligned, so slow-query
      *volume* needs its own counter for the dashboard):
  - `rpc_requests_{env}` — DELTA counter; filter
    `jsonPayload.message="http request"` AND `jsonPayload.rpc_method!=""`;
    labels `rpc_method`, `status` (via `EXTRACT(...)`).
  - `rpc_request_duration_{env}` — DELTA DISTRIBUTION;
    `value_extractor = "EXTRACT(jsonPayload.duration_ms)"`; label
    `rpc_method`; exponential buckets ~5 ms → ~20 s (scale 5, growth 2,
    13 finite buckets); same filter as above.
  - `db_request_duration_{env}` — DELTA DISTRIBUTION on
    `jsonPayload.message="request db stats"`; extractor on `db_duration_ms`;
    label `rpc_method`. This is the **uncensored** per-request DB latency
    headline (see correction #4); buckets like `rpc_request_duration`.
  - `db_slow_query_duration_{env}` — DELTA DISTRIBUTION on
    `jsonPayload.message="slow query"`; extractor on `duration_ms`; label
    `query_kind`; exponential buckets 100 ms → ~13 s. Tail detail only.
  - `db_pool_in_use_{env}`, `db_pool_open_{env}`, `db_pool_idle_{env}`,
    `db_pool_utilization_{env}` — DELTA DISTRIBUTIONs with value extractors
    on the `"db pool stats"` line (utilization is the alerting metric; the
    raw three are for dashboards; `wait_count`/`wait_seconds` stay log-only
    until someone needs them charted).
  - `go_goroutines_{env}`, `go_heap_inuse_bytes_{env}`,
    `go_gc_pause_ms_{env}` — DELTA DISTRIBUTIONs on the
    `"go runtime stats"` line.
  - Gauge-like distributions need value-appropriate buckets: linear 0–50
    step 1 for pool counts, linear 0–1 step 0.05 for utilization,
    exponential ~1 MiB → ~4 GiB for heap bytes, exponential 0.1 ms → ~1 s
    for GC pause, linear/exponential to ~10k for goroutines.
- [x] `terraform validate` + `terraform fmt -check` pass for dev and prod
      (`-backend=false` init).
- [x] Apply to dev (targeted `-target=module.monitoring` to avoid unrelated
      root-level drift pending in another PR); drove synthetic RPC traffic +
      organic dev traffic; all 12 metrics return points in Metrics Explorer.
      Note: alert-policy creation 404s until brand-new log-based metrics
      propagate (~10 min) — apply twice or wait.
**Validates:** `metric.type="logging.googleapis.com/user/rpc_request_duration_dev"`
returns points in Metrics Explorer; `ALIGN_PERCENTILE_95` produces sane
values; pool/runtime metrics tick at 15s/30s.

### Phase 3: Dashboards (Terraform)
**Goal:** The four dashboards from PR #1612's `dashboards.tf`, rebuilt on
log-based metric filters instead of PromQL.
**Tasks:**
- [x] `terraform/modules/monitoring/dashboards.tf` with
      `google_monitoring_dashboard` resources:
  - Per-RPC latency (P50/P95/P99 via percentile aligners, grouped by
    `metric.label.rpc_method`).
  - Per-RPC request & error rate (rate-aligned `rpc_requests_{env}`, split
    by status class; error-fraction chart).
  - DB health (per-request DB time P95 by `rpc_method`; pool
    open/in-use/idle + utilization; slow-query rate and P95 by
    `query_kind`; complements the existing Cloud SQL `database_*` alerts).
  - Go runtime (goroutines, heap in-use, GC pause).
- [x] `local.slo_targets` map; pin targets in widget titles (per the issue) —
      sourced from the same variables that drive the alert thresholds.
- [x] Apply to dev; screenshot-verified all four dashboards render live data
      (2026-07-01).
**Validates:** Dashboards answer the original #1520 questions ("P95 of
CompleteTransfer over the last week") without PromQL or new infra.

### Phase 4: Alerts, validation, prod rollout
**Goal:** The three alert policies fire correctly and route to the existing
channels; prod gets everything after a dev soak.
**Tasks:**
- [x] `rpc_error_rate_spike` (severity ERROR): per-`rpc_method` fraction
      `status=~"5.."` / all > 5% sustained 10m, via
      `condition_prometheus_query_language` (already used in this module for
      `database_memory`; log-based metrics are queryable as
      `logging_googleapis_com:user_rpc_requests_{env}`). Guard against
      low-traffic noise (e.g., require numerator rate > ~1 error/min or
      document the tradeoff). Condition display name contains "RPC errors"
      (auto-filer → warn).
- [x] `rpc_p95_latency` (severity WARN): `ALIGN_PERCENTILE_95` over
      `rpc_request_duration_{env}` > 2s sustained 15m, excluding
      `metric.label.rpc_method` matching `Stream` (all streaming RPCs are
      `Stream*`-named). Per-RPC calibration follow-up after a week.
- [x] `db_pool_saturation` (severity ERROR): `db_pool_utilization_{env}`
      `ALIGN_PERCENTILE_99` > 0.9 sustained 10m (P99 ≈ worst instance when
      Cloud Run runs multiple). Condition display name contains "Database"
      (auto-filer → page, consistent with existing `database_*` policies).
- [x] All three attach `local.notification_channel_ids` (email everywhere +
      Pub/Sub→GitHub in prod), `documentation` blocks with triage links, and
      auto-close, matching the existing policy style.
- [~] Dev validation: the P95 alert validated itself end-to-end on day one —
      a single `PortfolioService/GetHomeView` cold start (3.06s) opened a
      WARNING incident with correct `rpc_method` label and correlated logs,
      and confirmed the `Stream*` exclusion works (an 840s stream series did
      NOT fire). It also exposed a sparse-series bug: missing data after one
      violating point sustained the condition for the full duration window —
      fixed with `evaluation_missing_data = "EVALUATION_MISSING_DATA_INACTIVE"`
      on both threshold alerts. Error-rate and pool-saturation alerts not yet
      fire-tested (threshold-lowering still an option during soak).
- [x] ~~Soak in dev 1–2 weeks; then `terraform apply` in prod.~~ **Accelerated
      (2026-07-01, deliberate decision):** the soak's only real purpose was
      alert-noise calibration — the metrics/dashboards are passive and the
      missing-data fix (PR #2620) removed the main false-positive hazard.
      Prod applied same day as dev; since log-based metrics have no
      backfill, early prod apply *maximizes* the calibration data #2622
      needs. RPC metrics populate immediately in prod (the `http request`
      line predates this work); DB-per-request/pool/runtime metrics stay
      empty (and their alerts inert via missing-data=INACTIVE) until the
      next prod release ships the Go emitters.
- [x] Docs: update `docs/server/observability.md` Part 4 (new metrics + alert
      tables, the `db_slow_query_duration` censoring caveat) and
      `docs/server/metrics.md`'s "Relationship to Logging" section (the
      Prometheus-alert sentence is now obsolete — log-based is the alerting
      path). The doc-freshness ratchet will also flag these.
- [x] File follow-up issues (repo rule: deferred work gets a tracked issue,
      not an untracked promise): **#2622** per-RPC P95 threshold calibration
      from prod data, **#2623** evaluate retiring the count-based
      `rpc_errors_{env}` alert after the ~30-day overlap, **#2624** Cloud
      Monitoring Services + SLO objects with burn-rate alerts.
- [x] Comment on #1614 that its "unused /metrics endpoint" premise has
      drifted (rate-limit soak + off-app counters now use `server/metrics`)
      and needs re-scoping — posted, and the endpoint half of #1614 was
      folded into this PR (see scope-addition note below).
**Validates:** End-to-end: log line → metric → dashboard → alert → email +
auto-filed GitHub issue. After ~30 days of overlap, a separate cleanup issue
can evaluate retiring the old count-based `rpc_errors_{env}` alert in favor of
the fraction alert (explicitly out of scope here, per the issue).

### Scope addition during implementation: `/metrics` removal (#1614, complete)

At the user's request **all of #1614** was folded into this work, in two
stages. PR #2619 removed the endpoint half: the `/metrics` route,
`metricsAuthMiddleware`, the `--metrics-auth-token(-file)` flags + non-dev
fail-fast, and the secret plumbing (entrypoint fetch/flag,
`run_server_with_sm_secrets.sh`, Dockerfile/Terraform comment lists,
secrets/rotation docs). PR #2625 then deleted the rest: the
`server/metrics` package, the HTTP/Connect metrics middleware,
`PublishPoolStats` + the `recordQuery` histogram, every counter increment
site (rate-limit, off-app notifications, SMS webhook), and the
`prometheus/client_golang` dependency. Tests that asserted on counters
(rate-limit soak modes, off-app simulation suppression) were rewritten to
assert on the parallel structured log lines — which are the production
observables anyway. One counter-only branch (off-app email
`suppressed_disabled`) gained a log line for parity. `docs/server/metrics.md`
was deleted. The Secret Manager `metrics-auth-token` entries stay (unused,
harmless; deletion optional per #1614).

### Out of scope (unchanged from the issue)
- Cloud Monitoring Services + SLO objects (small follow-up once these metrics
  exist — filed as #2624).
- Existing log-based alerts (`rpc_errors_{env}`, `health_check_unhealthy`,
  `all_errors`) stay live; everything here is additive.

## Related Issues
- #1520 — closed parent; its SLI/SLO and alerting goals are what this issue
  finishes.
- #1585 / #1586 / #1587 — closed predecessors (Prometheus scrape / dashboards
  / alerts), replaced by this issue.
- PR #1612 (closed, unmerged) — contains the `dashboards.tf` widget inventory
  and `local.slo_targets` shape to port in Phase 3.
- PR #1589 (merged) — the `/metrics` endpoint and `server/metrics` package;
  its `DBQueryDuration`/pool gauges stay untouched by this work.
- #1614 — "remove unused /metrics endpoint" cleanup (the issue body's "#1615"
  reference is a typo). **Its premise has drifted:** `server/metrics` now also
  backs `ripls_ratelimit_rejections_total` (rate-limit soak mode) and
  `ripls_off_app_notifications_total`. Once this issue's pool metrics land,
  recommend re-scoping #1614 rather than executing it as written — worth a
  comment on that issue when this plan is approved.
- #2490 — crash/process-exit alerts; established the module's current
  log-based-alert patterns (naming, rate limits, documentation blocks) this
  work should match.
