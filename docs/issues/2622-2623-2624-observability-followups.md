# Issues #2622 / #2623 / #2624: Observability follow-ups to #1613 — latency calibration, legacy-alert retirement, SLOs with burn-rate alerts

**Sources:**
- #2622 — [Calibrate per-RPC P95 latency thresholds from prod data](https://github.com/Type2Fun-Foundation/ripls/issues/2622)
- #2623 — [Evaluate retiring count-based rpc_errors alert after overlap soak](https://github.com/Type2Fun-Foundation/ripls/issues/2623)
- #2624 — [Cloud Monitoring Services + SLOs with burn-rate alerts on log-based metrics](https://github.com/Type2Fun-Foundation/ripls/issues/2624)

**Filed:** 2026-07-01 (all three, as #1613 follow-ups) | **Plan written:** 2026-07-05
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`, `docs/server/observability.md` (⚓ observability anchor), `docs/issues/1613-log-based-metrics.md` (parent plan), `terraform/modules/monitoring/{main,alerts,dashboards,log_metrics,variables,outputs}.tf`, `cloud_functions/src/gcpAlertsHandler.ts`, `server/middleware/logging.go`, observability review prompt "Areas to Review"
**UI investigation:** skipped — pure server/Terraform/Cloud Functions observability work; nothing here renders in the app.

## Status (2026-07-05): implemented in simplified form

Per direction ("simple steps to close these bugs, assuming more detailed
work will be needed when prod traffic ramps up"), all three issues were
implemented in one pass, deviating from the phased plan in two ways:

1. **#2623 retired early instead of waiting for the ~2026-07-31 soak end.**
   The soak's deliverable was an incident-history comparison, and at current
   traffic there is nothing to compare: zero prod 5xx since the metrics went
   live, zero firings of either RPC-error alert. The redundancy argument
   stands on its own — `all_errors` fires on any single ERROR log (superset
   of the legacy alert's ERROR-count signal), `rpc_error_rate` covers
   failing responses. Same reasoning as #1613's deliberately-skipped prod
   soak. The legacy policy, metric, outputs, and variables are removed.
2. **Fire-tests skipped** (threshold-lowering theater in dev): the
   log→metric→alert→incident→auto-file pipeline was already validated
   end-to-end organically in #1613. Accepted risk, consistent with the
   error-rate/pool alerts which also shipped without fire-tests. Burn-rate
   alerts are warn-severity, so a misfire is a daily-aggregated GitHub
   issue, not a page.

Everything else landed as planned: two-tier P95 latency (interactive 2000ms
/ slow-class 8000ms), `extractRPCMethod` tightened to `/ripls.api.`,
classifier patterns reconciled with real condition names (pinned in CI
tests), custom service + availability (99.5%/28d) + latency (99% under
2s/28d) SLOs with fast/slow burn-rate alerts, docs updated. Deferred to
traffic-ramp time: per-RPC threshold refinement, burn-rate threshold
calibration, promoting fast-burn to page, availability-denominator
weighting.

## Problem Statement

Issue #1613 (merged in PR #2619, applied to dev + prod on 2026-07-01) shipped
log-based SLI metrics, four dashboards, and three alerts. It deliberately
deferred three pieces of tuning/maturation work, each filed as its own issue:

1. **#2622 — latency-threshold calibration.** The P95 latency alert launched
   with one global threshold (2000ms sustained 15m, `Stream*` excluded). Dev
   validation had already shown pressure points (`GetHomeView` cold starts,
   `GenGear` AI generation, `UploadMedia`). Decide, from real prod data:
   raise the global threshold, exclude specific RPCs, or split into
   per-class policies — then apply to both envs. Also decide whether the P95
   condition name should gain a `/latency/i` classifier pattern in the
   GitHub auto-filer (it currently lands in the "unknown" severity bucket).
2. **#2623 — legacy error-alert retirement.** Two per-RPC error alerts run
   in parallel: the legacy count-based "RPC Errors Elevated" (>5 ERROR logs
   per `operation` in 5m) and the new fraction-based "RPC Error Rate High"
   (>5% 5xx per `rpc_method`, 10m, ~1/min floor). Per the #1613 migration
   constraint the legacy alert soaks alongside the new one for ~30 days
   (until ~2026-07-31), then we compare incident history and retire it,
   keep it, or document distinct roles.
3. **#2624 — proper SLO objects.** The metrics exist; the remaining #1520
   goal is Cloud Monitoring Service + SLO resources (availability +
   latency SLIs) with multi-window burn-rate alerts, in Terraform, sharing
   the same `local.slo_targets` variables the dashboards and alerts already
   use. Explicitly sequenced after #2622 (calibration sets the latency SLO
   threshold).

The three issues are one arc — calibrate → formalize as SLOs → retire the
redundant legacy alert — which is why this is a single combined plan.
Ordering constraint: #2622 → #2624 (stated in #2624); #2623 is time-gated
(soak ends ~2026-07-31) and independent of the other two.

## Context from Comments

All three issues have **zero comments**. That is expected rather than a
red flag here: they were self-filed from the #1613 implementation session as
scoped follow-ups, and their bodies are unusually specific (they name the
exact Terraform resources, variables, and decision criteria). The design
record is the parent plan `docs/issues/1613-log-based-metrics.md`.

## Research Findings

### Prod calibration data (2026-07-01 → 2026-07-05, ~4.5 days)

Pulled from `logging.googleapis.com/user/rpc_request_duration_prod` /
`rpc_requests_prod` via the Monitoring API (`ALIGN_DELTA` +
`REDUCE_PERCENTILE_XX`, whole-window alignment). Issue #2622 asks for ~1 week
of data; this is day 4.5 — directionally solid, worth a re-pull at
implementation time (the numbers below are coarse for low-n RPCs, where "P95"
is effectively the max sample interpolated within a histogram bucket).

**Non-streaming RPCs vs the 2000ms threshold:**

| RPC | P50 | P95 | P99 | n (requests) | Reading |
|---|---|---|---|---|---|
| `PortfolioService/GetHomeView` | 192ms | **2441ms** | **7859ms** | 93 | Fast median; tail is the in-request weather fetch, *not* cold starts (correction 2026-07-06, see below) |
| `ExperienceService/SaveExperience` | — | 2496ms | 2547ms | 2 | Too few samples to judge |
| `FeedbackService/SubmitFeedback` | — | 2368ms | 2522ms | 3 | Synchronous GitHub API call |
| `ExperienceService/CompleteExperience` | — | 2304ms | 2509ms | 4 | Completion side-effects |
| `HealthService/CheckHealth` | — | 1248ms | 1274ms | 8 | Dependency probes (Mailgun etc.) |
| `MediaService/AddMedia` | — | 1248ms | 1274ms | 1 | Upload path, single sample |
| `FeedService/GetFeed` | — | 1164ms | 1257ms | 65 | Healthy |
| everything else | — | <900ms | <1300ms | — | Comfortably under |

Streaming RPCs (`StreamMessages`, `StreamCommunityEvents` at 40960ms
bucket-top, `StreamGenUnifiedCreate` ~10s) confirm the `Stream*` exclusion is
load-bearing and correct.

**Dev P95s over the same window** (per-merge deploys reset the in-process
weather cache far more often than prod — see the 2026-07-06 correction below —
plus synthetic traffic):
`GetHomeView` 4736ms, `GenGear` **3904ms**, `GetCommunityImpactMetrics`
2496ms, `MarkRequestFulfilled` 2417ms, `GetFeed` 2330ms. `GenGear` had zero
prod traffic in the window — dev is the only calibration signal for the
AI-generation class, and it now reads ~3.9s (the issue's 1.99s observation
is stale).

**Key alert-behavior fact:** despite `GetHomeView`'s whole-window P95 sitting
*above* the 2000ms threshold, **the P95 alert has fired zero times in prod
since the 2026-07-01 apply.** This is the missing-data fix (#2620,
`EVALUATION_MISSING_DATA_INACTIVE`) working as designed: a lone cold-start
spike can't sustain the 15-minute condition when traffic is sparse, because
empty 5-minute windows reset it. The calibration pressure the issue
anticipated has largely not materialized — the day-one dev incident was the
pre-#2620 sparse-series bug, not a threshold problem.

**Error data for #2623:** prod served **zero 5xx responses** in the window
(statuses observed: 200/400/401/403/404 only). Neither RPC-error alert has
fired in prod since the Pub/Sub channel went live. The soak comparison, as of
today, is 0-vs-0.

**Prod emitters are fully live:** the v0.26.2 release (merged 2026-07-04)
shipped the `server/statslog` emitters to prod — `db_pool_utilization_prod`
is populating (P99 ≈ 0.05, healthy). All prod metrics and alerts are now
armed; nothing in this plan is blocked on a release.

### Discovered defect 1: scanner junk pollutes the `rpc_method` label space

`rpc_requests_prod` carries labels like `git/config`, `aws/credentials`,
`wp/wso.php`, `well-known/assetlinks.json` — internet scanner probes. Cause:
`extractRPCMethod` (`server/middleware/logging.go:118`) accepts any
two-segment path whose first segment contains a dot, and dot-file probes
(`/.git/config` → service segment `.git`) satisfy it. Real Connect paths are
always `/ripls.api.<Service>/<Method>`. Effects: label cardinality pollution
in every RPC metric, junk rows in dashboards, and junk series the SLO SLIs
(#2624) would ingest into their denominators. (`well-known/…` alone is ~41
requests in 4.5 days.) `extractRPCMethod` has exactly two call sites — the
logging middleware and the query-stats middleware — both of which want the
tightening, so the blast radius is contained. Cheap, worthwhile fix while
we're calibrating.

(Side note so the data table doesn't alarm anyone: scanner paths like
`/.git/config` show `status 200` because the server's catch-all route serves
the Flutter web `index.html` for unknown paths — verified by fetching it.
Nothing git-shaped is exposed; it's ordinary SPA fallback behavior.)

### Discovered defect 2: the auto-filer's severity classifier has drifted from the actual condition names

`cloud_functions/src/gcpAlertsHandler.ts` classifies page/warn by
substring-matching the **condition** display name. Two existing policies
don't match the pattern written for them:

| Policy | Condition display name | Intended pattern | Actual bucket |
|---|---|---|---|
| `all_errors` ("Server Error Logged - …") | "Service-level error logged (excluding middleware and infrastructure)" | `/server\s*error\s*logged/i` (warn) | **unknown** |
| legacy `rpc_errors` ("RPC Errors Elevated - …") | "RPC error count exceeded threshold" | `/rpc\s*errors/i` (warn) | **unknown** |

The patterns were evidently written against the *policy* names; the
classifier reads the *condition* name. This is not theoretical: `all_errors`
incidents file as per-incident `[ops:page]`-titled unknown-severity issues —
see the pairs #2600/#2601 and #2614/#2615 — instead of the intended
one-per-day warn aggregation. The docs table in
`docs/server/observability.md` ("Cloud Monitoring severity classification")
describes the intended, not actual, behavior. #2622's open question about a
`/latency/i` pattern should widen into reconciling this table wholesale.

(The new `rpc_error_rate` condition — "RPC errors exceed N% of requests" —
*does* match `/rpc\s*errors/i` correctly.)

### Consumers of the legacy `rpc_errors_{env}` metric (retirement blast radius)

Checked for #2623: the metric is consumed by (a) the legacy alert policy
itself, and (b) the module's `outputs.tf` maps (`alert_policy_ids.rpc_errors`,
`log_based_metric_names.rpc_errors`). Nothing else — the "RPC Traffic &
Errors" dashboard charts `rpc_requests` (the Terraform resource named
`google_monitoring_dashboard.rpc_errors` is just a resource name), the
auto-filer keys on display names not metrics, and the daily digest
(observability doc Part 3) is unimplemented. Retirement is clean; the env
roots pass no `rpc_error_threshold`/`rpc_error_window` overrides.

Signal-coverage note for the retirement decision: the legacy alert's unique
catch (#2623's "silent degradation" case — ERROR logs while returning 200s)
is *already* covered by `all_errors`, which fires on **any single** ERROR log
(rate-limited 1/5m) versus legacy's >5-per-operation-per-5m. The legacy
alert's only marginal value over `all_errors` + `rpc_error_rate` is the
per-`operation` label on the notification. It has not fired in prod since
auto-filing went live.

## Potential Solutions

The real fork is in #2622; #2623 and #2624 have essentially settled shapes
(documented under Recommendation) and their alternatives are noted inline.

### Option A (#2622): Raise the global threshold to ~3000ms
- **Approach:** Single knob turn: `rpc_p95_latency_threshold_ms = 3000`.
- **Pros:** Trivial; clears `GetHomeView`'s current P95 (2441ms) and every
  low-n ~2.5s write RPC.
- **Cons:** Doesn't actually cover the named pressure points — `GenGear`
  reads 3.9s in dev and `UploadMedia` legitimately runs seconds, so they'd
  still trip; meanwhile the 95% of RPCs that run <300ms lose 1s of
  sensitivity for no reason. Worst of both directions.
- **Effort:** Trivial.

### Option B (#2622): Two-tier thresholds — keep 2000ms interactive, add a slow-class carve-out (recommended)
- **Approach:** Keep the existing condition at 2000ms but exclude a
  small, evidence-based "slow class" regex (AI generation, media upload,
  synchronous third-party calls); add a second condition in the same policy
  for that class at 8000ms. Two conditions, one policy, `combiner = "OR"`,
  both named with "latency" for the classifier.
- **Pros:** Interactive RPCs keep the tight threshold; known-slow classes
  get honest headroom instead of exclusion-by-silence; membership is an
  explicit reviewed variable (default documented in Terraform) rather than
  a growing pile of one-off exceptions; maps directly onto the latency SLO
  filter #2624 needs.
- **Cons:** Slightly more Terraform than a knob turn; slow-class membership
  is a judgment call at current sample sizes.
- **Effort:** Small.

### Option C (#2622): Full per-RPC threshold map
- **Approach:** `for_each` over a map of `rpc_method → threshold_ms`,
  one condition or policy per entry.
- **Pros:** Maximum precision.
- **Cons:** Pure over-engineering at 4.5 days of data and ~100 low-traffic
  RPCs; every new RPC becomes a Terraform decision; the alert that pages you
  is the one you forgot to add. The data shows two natural classes, not a
  continuum.
- **Effort:** Medium, plus permanent upkeep.

## Recommendation

**Option B for #2622**, then **#2624 as specified in the issue** (custom
service + availability & latency SLOs + multi-window burn-rate alerts,
initially warn-severity, additive to the static alerts), then **#2623 at
soak end (~2026-07-31): retire the legacy policy and metric** unless July
produces a counterexample incident.

Specifics the data forces:

- **`GetHomeView` stays in the interactive 2000ms tier.** Its P50 is 192ms —
  the handler is fast; the P95/P99 tail (2441/7859ms) is slow in-request
  work, not a threshold problem. Excluding or accommodating the app's front
  door would hide exactly the signal we most want, and post-#2620 the alert
  has produced **zero** prod incidents, so there is no noise cost to keeping
  it. The tail itself is not an alert-calibration problem — file it
  separately. *(Correction 2026-07-06: this plan originally attributed the
  tail to "Cloud Run cold starts at low traffic" — that was wrong.
  `min_instances = 1` has been deployed in both envs since #1073
  (2026-04-09); the tail is the synchronous Open-Meteo weather fetch inside
  GetHomeView — ~2s per cache miss, up to the client timeout during provider
  brownouts. Diagnosis and fix:
  `docs/issues/2646-homeview-latency-tail.md`.)*
- **Slow class (initial membership):** `GearService/GenGear`,
  `MediaService/AddMedia`, `MediaService/AddMediaFromURL`,
  `FeedbackService/SubmitFeedback` — each independently evidenced
  (GenGear 3.9s dev P95; media upload/remote-fetch multi-second by design;
  SubmitFeedback makes a synchronous GitHub API call, 2.4s at n=3).
  Threshold 8000ms. Note: issue #2622 names `MediaService/UploadMedia`,
  which **does not exist** — the media-upload RPCs are `AddMedia` and
  `AddMediaFromURL` (`proto/ripls/api/media_service.proto`).
  `SaveExperience`/`CompleteExperience` (n≤4) stay interactive until real
  data argues otherwise — with sustained-15m + missing-data=INACTIVE they
  cannot page off single samples.
- **Fix both discovered defects as part of #2622**: the `extractRPCMethod`
  scanner-noise fix (require the `/ripls.api.` prefix) and the classifier
  reconciliation (`/latency/i` → warn; `/service.level\s*error/i` → warn;
  update the docs table). The latency pattern is #2622's own open question
  answered "yes, now" — without it every future latency incident files an
  unaggregated unknown-severity issue, and the `all_errors` mismatch is
  already producing duplicate issue spam today.
- **#2624 SLO shape:** one `google_monitoring_custom_service` per env;
  availability SLI = request-based `good_total_ratio` over
  `rpc_requests_{env}` (bad = `status =~ "5.."`), target **99.5% / 28d
  rolling**; latency SLI = `distribution_cut` over
  `rpc_request_duration_{env}` excluding `Stream*` and the slow class,
  threshold = the calibrated 2000ms, target **99% / 28d** (validated against
  a fresh distribution query before apply). Burn-rate alerts per SLO: fast
  (1h lookback) and slow (6h lookback) `select_slo_burn_rate` conditions,
  both **warn-severity initially** — promote fast-burn to page only after it
  proves signal at this traffic level. Static alerts stay: the error-rate
  alert's min-errors floor is load-bearing at low traffic (burn-rate math
  has no such guard), so the statics are the guarded floor and the SLOs are
  the strategic layer.
- **#2623 expected outcome:** retire the legacy policy **and** the
  `rpc_errors_{env}` metric (plus the two `outputs.tf` entries and the
  `rpc_error_threshold`/`rpc_error_window` variables), keeping `all_errors`
  (single-ERROR catch, now correctly warn-classified) and `rpc_error_rate`
  (5xx fraction) with documented roles. The decision is deliberately
  deferred to ~2026-07-31 with the evaluation procedure in Phase 3 — if July
  produces an incident the legacy alert caught and the others didn't, keep
  both and document roles instead.

## Open Questions

- **Slow-class membership and threshold (8000ms)?** — Recommendation: start
  with the four RPCs above at 8000ms. GenGear's dev P95 is 3.9s and AI
  generation latency is provider-variable; 8s gives 2× headroom while still
  catching a hung provider. Revisit membership in the #2623 soak review
  (same calendar checkpoint, zero extra process).
- **Should `GetHomeView`'s cold-start tail get its own issue?** —
  Recommendation: yes, file a small product/infra issue (P99 7.9s on the
  home view; options: `min_instances = 1` on prod Cloud Run vs. accepting
  it at current traffic). It is out of scope for all three issues here but
  the data that surfaced it lives in this plan.
- **Classifier fix: add patterns vs. rename conditions?** — Recommendation:
  add patterns in `gcpAlertsHandler.ts` (one-line each + unit tests). A
  condition rename in Terraform recreates alert-policy conditions and
  changes incident/issue titles for no benefit; matching reality in the
  classifier is cheaper and keeps history coherent. Leave the legacy
  `rpc_errors` condition mismatch unfixed — it has never fired via Pub/Sub
  and Phase 3 intends to delete it.
- **Same thresholds in dev and prod?** — Recommendation: yes, keep parity
  (one variable default, no per-env overrides). Dev's worse cold starts will
  occasionally email — that is a cheap ongoing rehearsal that the pipeline
  works, and dev has no Pub/Sub channel so there is no GitHub noise. Revisit
  only if dev emails become a nuisance.
- **SLO targets (99.5% availability, 99% under 2s)?** — Recommendation: as
  stated, but re-validate both against a fresh 7-day query at implementation
  time (a task in Phase 2, not a hand-wave): compute the actual 5xx fraction
  and the actual fraction of non-stream, non-slow-class requests under 2s,
  and set targets one comfortable notch below observed performance so the
  error budget is meaningful but not instantly consumed.
- **Availability SLI denominator is ~83% `ListCommunityEvents` polling.** A
  failure in a low-traffic RPC barely dents the service-level error budget.
  Accept? — Recommendation: yes for v1. The per-RPC static error alert (with
  its min-error floor) is precisely the compensating control for low-traffic
  RPCs; the SLO is the headline availability number, not the only tripwire.
  Note the caveat in the observability doc.
- **Do burn-rate alerts replace the static `rpc_error_rate`/P95 alerts
  (#2624's "replacing/steepening … where appropriate")?** — Recommendation:
  no replacement now. Keep both layers; reconsider retiring statics only if
  double-alerting is observed in practice (that review can piggyback on the
  #2623 checkpoint or the next observability review pass — do not pre-file
  an issue for a problem that may not materialize).

## Implementation Plan

### Phase 1 (#2622): Latency calibration + measurement hygiene — implement now

**Goal:** Latency alerting reflects the two real latency classes; the metric
label space and the auto-filer classification are trustworthy inputs for
Phase 2's SLOs.

**Tasks:**
- [ ] Re-pull per-RPC P95/P99 + request counts for prod (and dev `GenGear`)
      at implementation time (~day 7+ of data) and sanity-check the table in
      this plan; adjust slow-class membership only on clear evidence.
- [ ] Go: tighten `extractRPCMethod` (`server/middleware/logging.go`) to
      only match Connect paths — require the `/ripls.api.` prefix and return
      `<Service>/<Method>`; unit tests covering real RPC paths, scanner
      probes (`/.git/config`, `/.well-known/assetlinks.json`), and non-RPC
      paths. (Existing junk label values stop accruing points and age out of
      dashboard windows naturally; no metric change needed.)
- [ ] Terraform (`alerts.tf` + `variables.tf`): add
      `rpc_p95_slow_class_regex` (default covering
      `GearService/GenGear|MediaService/AddMedia|MediaService/AddMediaFromURL|FeedbackService/SubmitFeedback`)
      and `rpc_p95_slow_latency_threshold_ms` (default 8000). Split the P95
      policy into two conditions: interactive (existing filter + NOT
      slow-class regex, 2000ms) and slow-class (slow-class regex only,
      8000ms). Both condition display names include the word "latency";
      both keep `EVALUATION_MISSING_DATA_INACTIVE`, the 900s duration, and
      the documentation block (extended with the two-tier explanation).
      Update the `alerts.tf` header comment — it currently documents the
      *opposite* decision ("the P95 latency condition intentionally matches
      no pattern and lands in the 'unknown' bucket … until it proves
      signal"); this plan reverses that with data, and the comment must not
      outlive the decision it records.
- [ ] Cloud Function (`gcpAlertsHandler.ts` + `__tests__`): add
      `/latency/i` and `/service[-\s]?level\s*error/i` to `WARN_PATTERNS`,
      with unit tests asserting the *actual* Terraform condition display
      names classify as intended (pin the real strings in the tests so the
      next drift breaks CI). Deploy via `deploy_cloud_functions.yaml`.
      **Behavior change to state in the PR:** unknown-severity never adds
      `claude-fix`, so today's misclassified `all_errors` incidents don't
      trigger Claude plans; once reclassified to warn, the first firing per
      day will add `claude-fix` (the documented intent in
      `docs/server/observability.md`'s gating table, accidentally disabled
      by the drift). Slightly more Claude workflow spend, strictly less
      GitHub issue spam.
- [ ] `terraform fmt`/`validate`; apply to dev then prod with
      `-target=module.monitoring` (the dev Terraform state is shared across
      branches — a full apply from a feature branch can destroy other PRs'
      in-flight dev resources, so targeted applies are mandatory here; no
      new metrics ⇒ no 404-propagation retry needed; dashboard
      update-in-place flap is benign).
- [ ] Docs: update `docs/server/observability.md` — alert-policy table (two
      latency tiers), severity-classification table (new patterns, and
      correct the two rows that describe intended-not-actual behavior),
      and the metric-interpretation note that `rpc_method` now only ever
      contains `ripls.api` services.
- [ ] File the `GetHomeView` cold-start-tail issue (data from this plan;
      explicitly not a monitoring change).

**Validates:** The two-tier policy applies cleanly in both envs; scanner
paths stop minting new label values (confirm in Metrics Explorer after a
day); a synthetic latency incident in dev (temporarily lower the interactive
threshold below normal traffic, revert in the same session) files a
daily-aggregated `[ops:warn]` issue instead of an unknown-severity one.

### Phase 2 (#2624): Custom service + SLOs + burn-rate alerts — after Phase 1 lands

**Goal:** The #1520 endgame: real SLO objects with error budgets, and
burn-rate alerting layered (not swapped) onto the static alerts.

**Tasks:**
- [ ] Pre-apply validation queries (both envs): actual 28d-equivalent 5xx
      fraction and under-2s fraction (non-stream, non-slow-class) to confirm
      the 99.5% / 99% targets leave real but not hair-trigger error budgets;
      record the numbers in this doc.
- [ ] New `terraform/modules/monitoring/slos.tf`:
      `google_monitoring_custom_service` (`ripls-server-{env}`);
      availability SLO (`request_based_sli.good_total_ratio`: bad =
      `rpc_requests_{env}` filtered `status =~ "5.."`, total = all;
      `goal = var.slo_availability_target`, `rolling_period_days = 28`);
      latency SLO (`request_based_sli.distribution_cut` on
      `rpc_request_duration_{env}` excluding `Stream*` and
      `var.rpc_p95_slow_class_regex`; `range.max =
      var.rpc_p95_latency_threshold_ms`; `goal = var.slo_latency_target`).
      New variables `slo_availability_target` (0.995) and
      `slo_latency_target` (0.99); fold both into `local.slo_targets` so
      dashboard titles can reference them and nothing drifts.
- [ ] Burn-rate alert policies (one per SLO, two conditions each):
      `select_slo_burn_rate(<slo>, "3600s")` fast window and `"21600s"`
      slow window, thresholds ~14.4 and ~6 (standard multi-window values —
      tune against the validation queries). Severity WARNING; condition
      display names include "SLO burn rate"; add `/burn\s*rate/i` →
      warn to the classifier in the same PR. Documentation blocks explain
      budget math and point at the dashboards.
- [ ] Apply dev → validate → prod (`-target=module.monitoring`; SLO
      resources depend on existing metrics, so no propagation wait, but
      keep the retry habit if Terraform 404s on the new custom service).
- [ ] Dev fire-test one burn-rate alert: temporarily raise the dev
      availability target to an unattainable value (e.g. 0.9999) and induce
      a handful of 5xx responses (temporarily break a dev dependency, the
      same technique #1613 validation used), confirm the email fires;
      revert both in the same session.
- [ ] Docs: observability.md gains an "SLOs & error budgets" subsection
      (targets, windows, burn-rate thresholds, the
      polling-dominated-denominator caveat, statics-as-floor rationale).

**Validates:** SLO pages render error-budget burn in the console for both
envs; a synthetic burn fires the fast-window condition and auto-files a
warn-severity aggregated issue; static alerts and burn-rate alerts coexist
without duplicate paging (different severities, different condition names).

### Phase 3 (#2623): Legacy `rpc_errors` retirement decision — gated on soak end (~2026-07-31)

**Goal:** One deliberate keep/retire decision with the July evidence, then a
clean removal (or a documented two-alert steady state).

**Tasks:**
- [ ] Assemble the soak comparison (both envs, 2026-07-01 → 07-31): incident
      lists for "RPC Errors Elevated" and "RPC Error Rate High" from the
      Cloud Monitoring policies pages, plus auto-filed `[ops:*]` GitHub
      issues and ops@ email. Answer the issue's three questions: did the
      fraction alert miss anything that mattered; which was noisier; does
      the silent-degradation gap matter given `all_errors` covers any
      single ERROR log.
- [ ] **If retiring (expected):** remove
      `google_monitoring_alert_policy.rpc_errors` and
      `google_logging_metric.rpc_errors` from `main.tf`, the two
      `outputs.tf` entries, and the now-unused `rpc_error_threshold` /
      `rpc_error_window` variables; apply both envs; update
      observability.md (alert table, classifier table row, and the
      "additive during overlap" note in `alerts.tf`'s header comment).
- [ ] **If keeping:** document the two alerts' distinct roles in
      observability.md and fix the legacy condition's classifier mismatch
      (add `/rpc\s*error\s*count/i` → warn) so it stops filing
      unknown-severity issues.
- [ ] Close #2623 with the decision rationale and the incident-count
      comparison pasted in.

**Validates:** Exactly one error-alerting story per failure mode, and the
observability doc matches deployed reality.

## Related Issues
- #1613 — closed parent; PRs #2619/#2620/#2625 are the implementation this
  plan tunes. Plan doc: `docs/issues/1613-log-based-metrics.md`.
- #1520 — original observability goals; #2624 completes its last line item.
- #2620 — missing-data fix that already defused #2622's day-one hazard.
- #1148/#1149 — the alert→GitHub auto-filer whose classifier Phase 1 fixes;
  plan doc `docs/issues/1148-1149-alert-to-issue.md`.
- #2490 — crash-detection alerts; source of the module's naming/doc-block
  conventions the new conditions follow.

## Audit notes (2026-07-05)

Audited via `/ripls-audit`. Changes applied by the audit:

1. **Phantom RPC corrected.** Issue #2622 (and the first draft) named
   `MediaService/UploadMedia`, which does not exist in
   `proto/ripls/api/media_service.proto`. Slow-class membership corrected to
   `AddMedia` + `AddMediaFromURL`.
2. **Documented-decision reversal made explicit.** The `alerts.tf` header
   comment records "P95 lands in unknown until it proves signal"; Phase 1
   now includes updating that comment, since this plan reverses the
   decision (with the zero-incidents-since-#2620 evidence).
3. **`claude-fix` spend-behavior change flagged.** Reclassifying
   `all_errors` to warn re-enables first-firing-of-the-day `claude-fix`
   (documented intent, accidentally disabled by the classifier drift) —
   called out as a stated behavior change for the PR description.
4. **Blast-radius facts added:** `extractRPCMethod` has exactly two call
   sites (logging + query-stats middleware, both want the fix); no
   Terraform env root consumes the `module.monitoring` outputs slated for
   removal in Phase 3; `/.git/config` 200s are the SPA `index.html`
   fallback, not exposed content (fetched and verified).
5. **Private-memory reference replaced** with the inline explanation of the
   shared-dev-Terraform-state gotcha; a repo doc can't cite session memory.
6. Dev burn-rate fire-test procedure reworded to the concrete
   #1613-validated technique.

Docs consulted: `docs/llms.txt`; anchors `docs/server/observability.md`,
`docs/server/conventions.md` (comments/fail-fast/logging sections);
review-prompt "Areas to Review" for observability, error_handling, testing,
security, code_quality, performance, tech_debt; parent plan
`docs/issues/1613-log-based-metrics.md`; and the implementation surfaces
themselves (`terraform/modules/monitoring/{main,alerts,dashboards,log_metrics,variables,outputs}.tf`,
`cloud_functions/src/gcpAlertsHandler.ts`, `server/middleware/logging.go`,
`proto/ripls/api/media_service.proto`). `proto_conventions.md` was skipped —
no proto surface is touched; client/accessibility docs not engaged.
