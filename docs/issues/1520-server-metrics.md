# Issue #1520: [observability] No server metrics collection blocks alerting and SLI/SLO

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1520
**Filed:** 2026-04-28 | **Last activity:** 2026-04-28
**Labels:** auto-filed, review:observability | **Assignees:** unassigned
**Priority:** P1 (assigned by `/ripls-triage` 2026-04-28)
**Context docs:** docs/reviews/reports/observability_review.md, docs/server/observability.md, docs/server/architecture.md

## Audit Findings (`/ripls-audit`, 2026-04-29)

The plan was audited against `docs/server/observability.md`,
`docs/server/architecture.md`, and the existing instrumentation in
`server/middleware/`, `server/storage/`, and `server/main.go`. Three
adjustments were folded in below; reasoning is captured here so the
intent is auditable:

1. **Metric name prefix.** The existing observability doc's "Future
   Enhancements" sketch uses a `ripls_` prefix
   (`ripls_http_request_duration_seconds`,
   `ripls_active_transfers_current`). The plan now matches: every
   exported metric uses the `ripls_` prefix to keep namespace ownership
   unambiguous when GMP merges metrics from sibling services.
2. **Label name alignment.** The existing log middleware emits the
   structured field as `rpc_method` (`server/middleware/logging.go:48`),
   while existing log-based Cloud Monitoring metrics use the field name
   `operation` (`docs/server/observability.md` Part 4). Keeping
   `rpc_method` as the Prometheus label avoids a name change to the
   middleware contract; the divergence from `operation` is a
   *log-based-metric* concern that the GMP migration follow-up can
   address by either renaming the log field or aliasing in the alert
   policy.
3. **Coexistence with existing log-based alerting.** The observability
   doc documents existing log-based metrics (`rpc_errors_{env}`,
   `health_check_failures_{env}`) and alert policies that ship to
   `support@ripls.org`. The plan's Phase 4 follow-up issues now
   explicitly call out the migration strategy (Prometheus *augments*,
   then *replaces* — alerts stay live throughout) so we don't end up
   with two parallel alerting paths or, worse, lose alerting during the
   cut-over.

Other observations that were checked and found to need no change:

- **Logging conventions.** All new log lines from the metrics package /
  middleware will use `logging.LoggerWithContext(ctx)` and the standard
  field names (`request_id`, `error`, `duration_ms`). Already implicit
  in the existing middleware that the new metrics middleware will sit
  next to.
- **Service independence.** `server/metrics/` is a shared library, not
  a service, and is consumed by middleware and `instrumented_db.go`.
  No service-to-service coupling introduced.
- **Health endpoint behavior.** The plan correctly does not touch
  `/health` (which intentionally returns 200) or readiness; those are
  separate review findings.
- **Sensitive-data masking.** Metrics emit no PII — labels are bounded
  enums (`rpc_method`, HTTP status code, `query_kind`). No
  `MaskEmail`/`MaskToken` work needed.

## Problem Statement

The server emits structured logs but no time-series metrics. There is no
`server/metrics/` package, no `/metrics` endpoint, and no Prometheus or
OpenTelemetry instrumentation anywhere in the codebase (`grep -r
"prometheus\|otel.Tracer\|otel.Meter" server/` returns zero hits). The only
quantitative signal exported today is per-request DB query counts logged at
request end (`server/middleware/query_stats.go`) and per-request HTTP
status/duration/bytes (`server/middleware/logging.go:38-50`) — both as log
fields, not as aggregatable counters or histograms.

Without time-series metrics:

- We cannot detect gradual performance degradation (e.g. P95 latency creeping
  up over a week).
- We cannot define an alertable error rate by RPC method × Connect error
  code; finding #5 in the review depends on this.
- SLI/SLO measurement is impossible — Cloud Logging log-based metrics can
  approximate but cannot match histogram fidelity for P95/P99.
- Capacity planning has no per-RPC throughput data.

This is greenfield instrumentation, not a refactor. The natural integration
points are already in place: the request middleware already extracts
`rpc_method` and captures `status`, and `InstrumentedDB` already wraps every
SQL call. The Critical rating reflects that *no* alerting or SLO work can
proceed until this lands — it unblocks findings #5 (error-rate alerting),
#9 (alert policies), and #10 (SLI/SLO dashboards) from the same review.

## Context from Comments

The issue has one comment — the `/ripls-triage` automated update on
2026-04-28 setting Type to Task and Priority to P1, citing "blocks all
alerting and SLI/SLO". No human discussion or revisions.

The motivating review (`docs/reviews/reports/observability_review.md`) is
the authoritative source. The "Recommendations / High Priority" section
estimates this as a 2–3 day implementation. The review explicitly suggests:

- `github.com/prometheus/client_golang` (not OTel meter SDK).
- Instrument middleware with `http_request_duration_seconds{rpc_method,code}`
  + `http_requests_total{rpc_method,code}` + `rpc_errors_total{rpc_method,code}`.
- Add a DB latency histogram in `server/storage/instrumented_db.go`.
- Expose `/metrics` in dev mode or behind a shared secret in production.
- Reuse `responseWriter.status` and `extractRPCMethod` for label values.

There is no maintainer disagreement to resolve — the recommendation is
internally consistent with the wider observability roadmap.

## Codebase Context

**Existing instrumentation surfaces (where metrics will plug in):**

- `server/middleware/logging.go:23-73` — wraps every HTTP request, already
  computes `duration`, captures `wrapped.status`, calls `extractRPCMethod`,
  reads `request_id` and `user_id` from context. Adding a histogram +
  counter here is two lines per signal.
- `server/middleware/query_stats.go:15-41` — already attaches a per-request
  query counter via `storage.WithQueryStats(ctx)`.
- `server/storage/instrumented_db.go:62-77` — `recordQuery` is the single
  funnel for every `QueryContext`/`ExecContext`/`QueryRowContext`. Adding a
  histogram observation here covers all SQL traffic.
- `server/storage/instrumented_db.go:25` — `Stats() sql.DBStats` already
  surfaces pool stats (`OpenConnections`, `InUse`, `Idle`, `WaitCount`,
  `WaitDuration`); a collector goroutine can publish these as gauges.

**Mux / routing:**

- `server/main.go:976` — pprof is mounted on the app mux only in dev mode
  (`mux.Handle("/debug/pprof/", http.DefaultServeMux)`). `/metrics` needs
  the same gating *plus* a defense-in-depth posture: never route through
  `http.DefaultServeMux` (this is finding #4 in the same review). Register
  promhttp handler on the app mux directly.
- `server/main.go:990, 994` — `/health` and `/readyz` are precedent for
  unauthenticated GET handlers on the app mux.

**Configuration:**

- `server/main.go:142, 185, 241` — `devMode` flag + struct field already
  exist and are widely used. A new `metricsAuthToken` config field
  (env-driven) would parallel the existing patterns.

**Dependencies:**

- `go.sum:291` shows `github.com/prometheus/client_model` is already a
  *transitive* dep (pulled in by something else — likely OTel indirectly).
  `client_golang` (the registry + collector library) is not yet a direct
  dep. Adding it is `go get github.com/prometheus/client_golang/prometheus`
  + `go mod tidy`.

**Style precedent:**

- `server/middleware/logging.go:118-132` shows label normalization style:
  the path → RPC label conversion already strips the `ripls.api.` package
  prefix to keep cardinality bounded. Same pattern applies to metric
  labels.
- The repo prohibits `log.Printf`/`fmt.Printf` for logging; equivalent
  rule for metrics: every counter/histogram lives in `server/metrics/` and
  is registered on a single shared registry, not the global default.

## Potential Solutions

### Option A: Prometheus client_golang with `/metrics` endpoint (review's recommendation)

- **Approach:** Add `github.com/prometheus/client_golang/prometheus` and
  `prometheus/promhttp`. Create `server/metrics/` package with:
  - A non-global `*prometheus.Registry` (no use of
    `prometheus.DefaultRegisterer`).
  - Pre-registered Go runtime + process collectors
    (`collectors.NewGoCollector()`, `collectors.NewProcessCollector(...)`)
    on the package registry.
  - Public histogram/counter vars: `HTTPRequestDuration`,
    `HTTPRequestsTotal`, `RPCErrorsTotal`, `DBQueryDuration`, plus pool
    gauges.
  - A `Handler()` returning `promhttp.HandlerFor(registry, opts)`.

  Update `server/middleware/logging.go` to observe the histogram + increment
  counters at the same place it logs. Update
  `server/storage/instrumented_db.go:recordQuery` to observe DB duration.
  Spawn a goroutine in `main.go` that polls `db.Stats()` every 15s and
  updates pool gauges.

  Mount `/metrics` on the app mux (never `DefaultServeMux`), gated:
  - In `cfg.devMode`: open.
  - Otherwise: require `Authorization: Bearer <token>` matching
    `METRICS_AUTH_TOKEN` env var. Refuse to start in non-dev mode if env
    var is unset *and* an explicit `--allow-public-metrics` flag is not
    given.

- **Pros:** Matches the review's explicit recommendation. Prometheus is the
  standard in the Cloud Run ecosystem; Google Managed Prometheus (GMP) can
  scrape `/metrics` directly. Synchronous instrumentation is easy to
  reason about. No new SDK dependency tree to manage.
- **Cons:** Requires solving the scrape problem in production: GMP needs
  either a sidecar or a public scrape config. Auth-gated `/metrics` is
  workable but not all scrapers support bearer tokens equally cleanly.
- **Effort:** Medium (~2–3 days as the review estimates).

### Option B: OpenTelemetry Meter SDK + OTLP exporter

- **Approach:** Use `go.opentelemetry.io/otel/metric` instead of Prometheus
  directly. Wire an OTLP exporter that ships metrics to Google Cloud
  Monitoring (or any OTLP receiver). Same instrumentation points, but the
  SDK + exporter live where the registry would have lived.
- **Pros:** Aligns with finding #6 (distributed tracing) — both metrics
  and traces use the same OTel runtime. Push-based: no scrape endpoint to
  authenticate. GCM has native OTLP ingestion.
- **Cons:** Heavier dependency tree (`otel/sdk/metric`,
  `otel/exporters/otlp/otlpmetric/otlpmetricgrpc`, `otel/sdk/resource`).
  Push-based exporter has its own failure modes (exporter goroutines, batch
  sizes, dropped data on shutdown). The review *explicitly* recommended
  Prometheus, presumably because the maintainer wants the simpler model
  first — overriding that without discussion is not warranted.
- **Effort:** Medium-large (~4–5 days; SDK setup + exporter wiring + CI
  validation that no metrics are dropped at shutdown).

### Option C: Cloud Logging log-based metrics only

- **Approach:** Skip server-side aggregation; rely on Cloud Logging's
  log-based metrics feature to derive counters/histograms from existing
  structured log fields (status, duration_ms, rpc_method).
- **Pros:** Zero code change. Uses logs we already emit.
- **Cons:** Histogram fidelity is poor (Cloud Logging buckets durations
  coarsely). High cost at volume. Cannot expose runtime metrics (Go GC,
  goroutine count, DB pool state) — these aren't logged. Doesn't satisfy
  the issue's "set up error rate alerts, measure SLIs/SLOs" goal at
  required precision. Locks the project to GCP.
- **Effort:** Small (configuration only) but doesn't actually solve the
  problem.

## Recommendation

**Go with Option A (Prometheus client_golang).** The review's recommendation
is well-founded: simpler dependency surface than OTel, push the tracing
work to a follow-up that can adopt OTel cleanly when finding #6 lands. The
auth gating is straightforward and the GMP scrape path is supported.

Two specific design choices worth flagging:

1. **Use a non-global `*prometheus.Registry`**, not
   `prometheus.DefaultRegisterer`. Two reasons: (a) tests can construct
   isolated registries without polluting global state, mirroring the
   pattern already used in `server/middleware` where loggers are passed
   in rather than read from globals; (b) defense in depth against a
   future dependency registering metrics on the default registry that
   conflict with ours.
2. **Auth gating in non-dev mode is mandatory at startup, not optional.**
   Specifically: in non-dev mode, if `METRICS_AUTH_TOKEN` is unset, the
   server should refuse to start (fail-fast pattern from CLAUDE.md).
   Public unauthenticated `/metrics` exposes goroutine counts, GC stats,
   and request rates that are reconnaissance-useful; the project's stance
   on the JWT secret (finding #3) is the same posture and we should
   apply it consistently.

The work is independent of finding #2 (`/readyz`), finding #4 (pprof
hardening), and finding #6 (tracing) — but it shares the "register on app
mux only, never DefaultServeMux" rule from finding #4, so the
implementation will follow that constraint by construction.

## Open Questions

- **Where should `/metrics` be scraped from in production?**
  Recommendation: defer the scrape config to a follow-up (Terraform
  change). For Phase 1, ship `/metrics` behind `METRICS_AUTH_TOKEN` and
  validate locally with `curl -H "Authorization: Bearer ..."`. The review
  doesn't prescribe GMP vs. self-hosted; that's a deployment decision for
  the operator.

- **What histogram bucket boundaries should we use for HTTP duration?**
  Recommendation: use `prometheus.ExponentialBuckets(0.005, 2, 12)` (5ms
  → ~10s in 12 buckets). Matches typical Connect-RPC latency profile —
  most calls finish in tens of milliseconds; AI-bound calls can run for
  seconds. Avoid the default Prometheus buckets (5ms–10s linear) — too
  coarse for our P50, too narrow for AI streaming.

- **Should DB query histograms be labeled by query type or table?**
  Recommendation: **no labels** beyond optional `query_kind`
  (`select`/`insert`/`update`/`delete`/`other`) parsed from the leading
  SQL keyword. Per-table or per-statement labels explode cardinality.
  `query_kind` is bounded at 5 values and gives us the breakdown we
  actually need (read vs. write latency).

- **Should we emit business metrics
  (`gear_created_total`, `transfer_completed_total`)?**
  Recommendation: defer to a separate issue (review #11 follow-up). This
  issue is scoped to infrastructure metrics; mixing business counters in
  expands review/PR surface and they can be added incrementally without
  touching the registry plumbing.

- **Should we wire the metrics middleware around or inside the existing
  logging middleware?**
  Recommendation: wrap the logging middleware in a separate
  `Metrics(reg)` middleware, placed at the same nesting level. Keeps the
  two concerns independent and avoids changing logging signatures.

- **Should existing `slowQueryThreshold` warn-level logging stay?**
  Recommendation: keep it. Logs surface the slow query *text* for triage;
  the histogram captures the distribution. Different audiences, different
  cardinality. No conflict.

## Implementation Plan

### Phase 1: Core registry + HTTP request metrics

**Goal:** A working `/metrics` endpoint exposing HTTP request latency and
counts, gated for production access. This is the minimum for any alerting
work to begin.

**Tasks:**
- [ ] Add `github.com/prometheus/client_golang/prometheus` and
  `.../promhttp` to `go.mod`; run `go mod tidy`.
- [ ] Create `server/metrics/` package with `doc.go` and `README.md`.
- [ ] Define `Registry` (non-global `*prometheus.Registry`),
  pre-register `collectors.NewGoCollector()` and
  `collectors.NewProcessCollector()`.
- [ ] Define `ripls_http_request_duration_seconds`
  (`HistogramVec` with labels `rpc_method`, `code`; buckets per the
  recommendation above).
- [ ] Define `ripls_http_requests_total` (`CounterVec` with same labels).
- [ ] Define `ripls_rpc_errors_total` (`CounterVec` with same labels —
  tracks 4xx + 5xx for now; Connect error code mapping in Phase 5).
- [ ] Add `Handler()` returning `promhttp.HandlerFor(Registry, opts)`.
- [ ] Add `server/middleware/metrics.go` with `Metrics(reg)` middleware
  that observes the histogram and increments counters at request end.
  Reuse `extractRPCMethod` and `responseWriter.status`.
- [ ] Wire `Metrics` middleware into the chain in `server/main.go`,
  alongside `Logging`.
- [ ] Add `METRICS_AUTH_TOKEN` env var read at startup. In non-dev mode
  fail-fast if unset.
- [ ] Add `/metrics` route on app mux in `server/main.go`. Wrap with
  bearer-token check in non-dev mode; open in dev mode.
- [ ] Unit tests in `server/metrics/metrics_test.go` (registry isolation,
  no panic on duplicate registration, label cardinality bounded).
- [ ] Integration test in `server/middleware/metrics_test.go` (scrape
  endpoint after a fake request, assert metric line is present).
- [ ] Update `server/middleware/README.md` and add `server/metrics/README.md`.

**Validates:** Confirms the registry/exposition pipeline end-to-end and
gives the alerting work a concrete signal to consume. P95 latency of any
RPC is now queryable.

### Phase 2: DB instrumentation

**Goal:** Per-query histogram + pool gauges so DB-bound regressions are
detectable independently of HTTP-level latency.

**Tasks:**
- [ ] Add `ripls_db_query_duration_seconds` (`HistogramVec` with label
  `query_kind`, same exponential bucket family).
- [ ] Add pool gauges: `ripls_db_pool_open`, `ripls_db_pool_in_use`,
  `ripls_db_pool_idle`, `ripls_db_pool_wait_count`,
  `ripls_db_pool_wait_seconds`.
- [ ] In `server/storage/instrumented_db.go:recordQuery`, classify the
  query (parse leading keyword) and observe the histogram.
- [ ] In `server/main.go`, spawn a goroutine that polls `db.Stats()`
  every 15s and updates the pool gauges. Stop on context cancel.
- [ ] Unit test that `recordQuery` observes the histogram with the
  expected `query_kind` label for select/insert/update/delete/other.
- [ ] Integration test (uses `storage.SetupTestStorage`) that runs a
  handful of queries and asserts the histogram has nonzero observations
  for the right `query_kind` values.

**Validates:** DB latency distribution and pool saturation are visible
on the metrics endpoint. Validates that the registry survives a
realistic test load without panic.

### Phase 3: Documentation + operator guidance

**Goal:** Make the new endpoint operationally usable; document the auth
contract and example PromQL queries.

**Tasks:**
- [ ] Add `docs/server/metrics.md` covering: exposed metrics, label
  semantics, scrape configuration sketch (GMP / generic Prometheus),
  auth contract, and example PromQL queries (P95 by RPC, error rate by
  RPC, DB pool saturation alert template).
- [ ] Update `docs/server/logging.md` to cross-link to the new
  metrics doc and clarify the logs-vs-metrics split.
- [ ] Add an entry in the top-level `README.md` development section for
  hitting `/metrics` locally.
- [ ] Note `METRICS_AUTH_TOKEN` in any deployment/runbook docs that
  enumerate env vars.

**Validates:** A reviewer landing in the repo cold can hit the endpoint
and write a useful query without reading source.

### Phase 4: File follow-up issues for GMP scrape + dashboards

**Goal:** Hand the production-viewing work off to dedicated issues so #1520
can close once the endpoint is live and locally verifiable, without
blocking on Terraform/operator decisions.

**Tasks:**
- [x] File issue: **"Wire Google Managed Prometheus to scrape
  `/metrics`"** — #1585.
- [x] File issue: **"Build SLI/SLO dashboards in Cloud Monitoring"** —
  #1586. Closes review finding #10.
- [x] File issue: **"Define alert policies on metrics"** — #1587.
  Closes review finding #9. **Migration note:** the existing log-based
  alert policies (`rpc_errors_{env}`, `health_check_failures_{env}`,
  "Server Error Logged" — all in `terraform/modules/monitoring/`) must
  stay live throughout; new Prometheus-based alerts run *in addition*,
  and the log-based ones are only removed once the Prometheus
  equivalents have proven themselves over a sustained period.
- [x] File issue: **"Emit business metrics
  (`gear_created_total`, `transfer_completed_total`, ...)"** — #1588.
  Closes review finding #11; consumes the `server/metrics/` registry.
  Out of scope for #1520 itself.
- [x] Cross-link the four follow-ups in this plan document and in the
  closing comment of #1520 (comment posted 2026-04-29).

**Validates:** The hand-off is explicit. No "we'll set up scraping
later" rotting in a TODO; each downstream concern has an owner-able
ticket.

### Phase 5: RPC error rate by Connect error code (closes review finding #5)

**Goal:** Replace the Phase 1 placeholder `RPCErrorsTotal{rpc_method,code}`
(HTTP code) with a Connect-aware version labeled by Connect error code,
satisfying review finding #5.

**Tasks:**
- [ ] Add a Connect interceptor (`server/middleware/connect_metrics.go`
  or in the metrics package) that inspects the returned `connect.Error`
  and increments `ripls_rpc_errors_total{rpc_method,connect_code}`.
- [ ] Decide whether to replace the HTTP-status-coded counter from Phase 1
  or keep both (recommendation: keep both — `code=4xx/5xx` is useful for
  non-Connect routes like `/health`, `/readyz`, `/metrics` itself).
- [ ] Wire the interceptor into every Connect service handler
  registration in `server/main.go` (already a long block — add a helper
  to avoid touching every line).
- [ ] Integration test: trigger a `connect.NewError(connect.CodeNotFound,
  ...)` and assert the counter increments with `connect_code="not_found"`.

**Validates:** Closes review finding #5. Alert rules of the form "error
rate of `SaveGear` for `connect_code=internal` over 1m" become
expressible.

## Related Issues

- **Review finding #4** (pprof on `DefaultServeMux`) — same review.
  This plan's "register `/metrics` on app mux only" constraint is the
  flip side of the pprof hardening. Should be resolved at the same time
  or in adjacent PRs to avoid one regressing the other.
- **Review finding #5** (no error rate tracking by RPC × Connect code)
  — directly closed by Phase 5 of this plan.
- **Review finding #6** (no distributed tracing) — adjacent but
  independent. OTel for tracing can land later without disturbing the
  Prometheus metrics path; the two coexist cleanly.
- **Review finding #9, #10** (alerting + SLO dashboards) — both
  unblocked by this issue. They are separate workstreams; track in
  follow-up issues once `/metrics` is live.
- **Review finding #11** (business metrics:
  `gear_created_total`, `transfer_completed_total`) — explicitly out
  of scope for this issue; filed as a follow-up in Phase 4 that
  consumes the registry from `server/metrics/`.
