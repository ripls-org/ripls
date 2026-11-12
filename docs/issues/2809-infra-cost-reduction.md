# Issue #2809: Infra cost review: $470/mo, ~70% avoidable (health-probe geocoding, AR image pileup, always-on dev, VPC connectors, SQL HA)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2809
**Filed:** 2026-07-29 | **Last activity:** 2026-07-29
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/server/observability.md` (health infra, uptime checks, alerting), `docs/server/conventions.md`, `docs/server/architecture.md` (outline), `terraform/environments/{dev,prod}/main.tf`, `terraform/modules/{containers,storage,metabase,monitoring}`, live billing data (billing account `01911A-DA95C0-B0C18F`, Jun 29 – Jul 28 2026) and Maps API metrics console

## Problem Statement

GCP spend is $469.63/mo and growing ~5%/mo, with dev ($246) costing more than
prod ($224). Four line items dominate, and the largest is self-inflicted:

1. **Geocoding API $209.72/mo (45% of the bill).** `GoogleMapsClient.CheckHealth`
   (`server/location/google.go:359`) issues a *billable* forward-geocode of
   "1600 Amphitheatre Parkway" every time it runs. It is registered as a health
   checker (`server/wiring.go:162`), and `/health` runs every registered checker
   on every request with no caching (`server/services/health/service.go`). Each
   env's Cloud Monitoring uptime check (300s period × 3 USA prober regions)
   therefore triggers one paid geocode every ~100s, 24/7, per environment:
   ~26K calls/mo/env ≈ the billed 51,943 calls. The Maps metrics console shows
   the matching constant ~0.01 qps drumbeat with no user shape.

   This is a regression from the #2188 Mapbox→Google cutover: the old Mapbox
   probe hit the *free* `/tokens/v2` endpoint (see observability.md "Current
   Implementations"), and #929 already established the precedent that paid
   probes must not sit on the uptime path (Pexels was deregistered for this).

2. **Artifact Registry $49.78/mo, compounding +20%/mo.** Every CI build pushes
   an image and nothing deletes them: `ripls-dev/images` is **521 GB**,
   `ripls-prod/images` 49 GB. No cleanup policies exist; the repos aren't even
   Terraform-managed (only their IAM grants are).

3. **Dev runs a 24/7 prod-shaped stack (~$100/mo idle).** `server-dev` has
   `min_instances=1` + `cpu_idle=false` (instance-based billing) = $57.87/mo at
   zero traffic. The dev uptime check would keep the instance warm even at
   `min_instances=0` (probe every ~100s < Cloud Run's ~15-min idle scale-down).
   Prod's always-on posture is justified (realtime streams, background delivery,
   digests — #2646); dev's is not (hermetic e2e runs locally/CI, dev digests off).

4. **Serverless VPC Access connectors $24/mo** (2 envs × 2 always-on e2-micros)
   — replaceable by Direct VPC egress (GA, no per-instance charge).

5. **Prod Cloud SQL REGIONAL (HA) premium $27/mo** — the only redundancy spend;
   decision needed on whether pre-launch scale justifies it.

Non-findings: network egress is $0 net ($8.64 gross, credit-offset); no LBs,
NAT, or reserved external IPs; Metabase already scales to zero; log retention
is default; media buckets are small.

## Context from Comments

No comments yet — the issue was filed today from this review. The findings are
grounded in live billing/console data rather than discussion. The one genuine
judgment call (SQL HA) is listed under Open Questions rather than assumed.

## Potential Solutions

### Option A: Fix each cost driver at its root (code + Terraform), keep monitoring semantics
- **Approach:** (1) Add per-checker TTL caching to the health service so deep
  dependency probes are computed at most once per TTL regardless of probe rate;
  (2) Terraform-manage the AR repos with cleanup policies; (3) dev server →
  `min_instances=0` + request-based billing + remove dev uptime check; (4)
  migrate Cloud Run/Metabase to Direct VPC egress and delete connectors; (5)
  decision-gated prod SQL ZONAL.
- **Pros:** Removes ~$320–350/mo (~70%) while keeping the health endpoint's
  diagnostic body, prod alerting, and rollback windows. Each piece is an
  independent, shippable increment. Aligns with the #929 precedent.
- **Cons:** Touches five surfaces (Go server, AR, Cloud Run config, VPC,
  Cloud SQL); needs staged Terraform applies.
- **Effort:** Medium overall; each phase is small.

### Option B: Deregister the google_maps health checker (mirror #929), plus the same infra fixes
- **Approach:** Instead of TTL caching, simply don't register
  `googleMapsClient.CheckHealth`, exactly as was done for Pexels.
- **Pros:** One-line server change; zero new machinery.
- **Cons:** Loses all visibility into the geocoding dependency (the #929 note
  shows this tradeoff was already accepted once, reluctantly). Doesn't protect
  the next paid checker — the same trap will recur (it already recurred once,
  in the #2188 cutover). Mailgun's GetDomain probe also runs ~26K/mo against
  provider quota for no reason.
- **Effort:** Small.

### Option C: Slow the uptime checks instead (e.g. 15-min period)
- **Approach:** Keep everything else, raise `uptime_check_period`.
- **Pros:** Terraform-only.
- **Cons:** Only divides the waste by 3 (~$140/mo still), slows outage
  detection for prod, and still keeps dev warm. Doesn't fix the structural
  problem that /health cost scales with probe rate.
- **Effort:** Trivial, but insufficient.

## Recommendation

**Option A.** The TTL cache is the root-cause fix (per the "fix root cause,
don't kludge" project norm): it makes `/health` cost-independent of probe rate
for *every* current and future checker, while preserving the response body and
alert semantics. Option B is the fallback if reviewers prefer zero new
machinery, but it forfeits visibility and doesn't prevent recurrence. Option C
is rejected as a kludge.

Phases below are ordered by savings-per-risk: the two low-risk items worth
~$245/mo ship first; the network cutover and the HA decision come last.

## Open Questions

- **Drop prod Cloud SQL HA (REGIONAL→ZONAL, −$27/mo)?** Gives up automatic
  zonal failover; backups (30d) + PITR (7d) are unaffected. — Recommendation:
  yes, until launch-scale traffic exists. The rest of the stack is
  single-region anyway, so the standby alone doesn't deliver meaningful
  availability today. Revisit alongside launch readiness.
- **AR retention window?** Deploys reference git-SHA tags; rollback via
  redeploying an older SHA. — Recommendation: keep the 15 most recent versions
  per repo and delete untagged layers after 7 days. 15 versions ≈ weeks of
  rollback headroom at current deploy cadence; adjust if the team wants more.
- **Dev cold starts acceptable?** `min_instances=0` means the first dev request
  after idle pays container start + entrypoint secret fetches (seconds).
  — Recommendation: yes; dev is manual-testing only. Anyone who needs a warm
  dev for a demo can temporarily bump min instances.
- **Health TTLs per checker?** — Recommendation: 15 min for paid/quota-bound
  external probes (google_maps/mapbox, email, github, ai), no cache for
  database/storage (they're free, local, and feed `/readyz` criticality).
  15 min keeps both envs' geocode volume (~6K/mo) inside the 10K free tier
  even with prod probes continuing at 300s.

## Implementation Plan

### Phase 1: TTL-cached health checkers (kills probe-driven Geocoding spend, ~$200/mo)
**Goal:** `/health` cost is independent of probe rate; geocoding drops into the free tier.
**Tasks:**
- [ ] Extend `server/services/health/service.go` with per-checker TTL support:
      `RegisterFuncCached(name string, ttl time.Duration, fn CheckFunc)` (or an
      options variant), storing last statuses + timestamp per checker;
      concurrent probes within the TTL reuse the cached result (guard with a
      mutex/singleflight so a slow upstream can't stampede).
- [ ] Cached results must be visibly stale-stamped: keep `latency_ms` from the
      real probe and add a `cached_age_ms` (or similar) detail so the /health
      body stays honest.
- [ ] Register external providers with 15-min TTL: `google_maps`, `mapbox`,
      `email`, `github` (`server/wiring.go`), `ai` (`server/main.go`).
      Leave `database`/`storage` uncached (criticality path for `/readyz`).
- [ ] Unit tests: cache hit within TTL (checker fn called once), refresh after
      expiry, error results also cached (don't hammer a failing paid API),
      concurrent access, uncached checkers unaffected.
- [ ] Update `docs/server/observability.md` Part 2 ("Current Implementations" —
      which also still describes the retired free Mapbox probe as current) with
      the TTL model and the cost rationale, referencing #929 and this issue.
- [ ] `npm run build` + `go test ./server/...`.
**Validates:** Maps metrics console flatlines to ~0 within a day of deploy;
health alerting still fires on real dependency failure (TTL only delays
detection by ≤15 min for non-critical deps, which the alert thresholds already
tolerate).

### Phase 2: Artifact Registry cleanup policies (~$45/mo, stops +20%/mo compounding)
**Goal:** Image storage bounded at a rollback window instead of growing forever.
**Tasks:**
- [ ] Import both `images` repos into their environment Terraform
      (`google_artifact_registry_repository`), keeping attributes verbatim so
      the import is a no-op apply (confirm each repo's actual location with
      `gcloud artifacts repositories describe` first — the IAM grants assume
      `var.region` but the repos were created out-of-band).
- [ ] Add `cleanup_policies`: keep 15 most-recent versions; delete untagged
      versions older than 7 days. Start with `cleanup_policy_dry_run = true`,
      inspect the candidate list, then flip to enforcing.
- [ ] Leave `gcf-artifacts` (0.4 GB, Cloud-Functions-managed) alone.
- [ ] Verify the deploy workflow (`build_and_deploy_container.yaml`) and prod
      rollback path still resolve their SHA tags after the first cleanup run.
**Validates:** Repo sizes drop to O(10 GB); the AR billing line falls to ~$2–5/mo
next cycle.

### Phase 3: Dev scale-to-zero (~$50/mo)
**Goal:** Dev costs scale with dev usage instead of wall-clock time.
**Tasks:**
- [ ] `terraform/modules/containers`: add a `cpu_idle` variable (default
      `false` to preserve prod's instance-based billing); dev env sets
      `cpu_idle = true` and `min_instances = 0`.
- [ ] `terraform/modules/monitoring`: add `enable_uptime_check` (default
      `true`) gating the uptime check config + uptime-failure alert; dev env
      sets `false`. Log-based alerts (#1613 SLI/SLOs, crash detection) remain —
      they're silent when nothing runs, active when dev is exercised.
- [ ] Confirm no dev-only background dependency exists (activity digest is
      already prod-only; realtime streams hold an active request so
      request-based billing still allocates CPU to them). Accept the fidelity
      caveat: post-request background work (e.g. notification settle windows,
      #2657) is CPU-throttled under `cpu_idle = true`, so dev exercises those
      paths with lower fidelity than prod — they remain fully covered by the
      local/CI integration and e2e stacks.
- [ ] Update `docs/server/observability.md` Part 4 (uptime checks run "every 5
      minutes" in both envs today) to state that dev has no uptime check and
      relies on log-based alerting.
- [ ] Staged apply per the shared-dev-state rule (see memory: feature-branch
      full applies can delete other PRs' dev resources — use `-target` from the
      branch, full apply only from merged main).
**Validates:** Dev Cloud Run line drops to ~$0 idle; a manual dev session works
end-to-end after a cold start.

### Phase 4: Direct VPC egress, delete connectors (~$24/mo)
**Goal:** Same private-DB reachability without per-instance connector cost.
**Tasks:**
- [ ] Add a dedicated egress subnet (/26) per env VPC in
      `terraform/modules/storage` (the existing `db_subnet` can be evaluated,
      but a dedicated subnet keeps concerns separate).
- [ ] Switch `vpc_access` in `modules/containers` and `modules/metabase` from
      `connector` to `network_interfaces` (network + subnet vars), keeping
      `egress = "PRIVATE_RANGES_ONLY"`.
- [ ] Roll out dev first: apply, verify DB connectivity + media/SQL paths, then
      prod. Keep connectors in place until both envs' new revisions are
      verified, then remove `google_vpc_access_connector` in a follow-up apply.
**Validates:** `Compute Engine` E2 line goes to ~$0; no connectivity
regressions in either env.

### Phase 5: Prod Cloud SQL REGIONAL → ZONAL (~$27/mo, decision-gated)
**Goal:** Stop paying the HA premium until scale justifies it (pending sign-off
on the open question).
**Tasks:**
- [ ] Flip `high_availability = false` for prod in
      `terraform/environments/prod/main.tf`.
- [ ] Schedule the apply in a low-traffic window (availability-type change
      restarts the instance, minutes of downtime).
- [ ] Note the revisit trigger in the launch checklist (re-enable HA when
      traffic/SLOs warrant).
**Validates:** SQL line drops to ~$53/mo total; backups/PITR unchanged.

### Phase 6: Hygiene + guardrails (~$4/mo + early warning)
**Goal:** Trim residual waste and make the next cost surprise visible in days,
not a quarter later.
**Tasks:**
- [ ] Prune superseded Secret Manager versions (keep latest 2 per secret) in
      both projects; document the practice in `docs/secrets.md`.
- [ ] Add a `google_billing_budget` (e.g. $300/mo with 50/80/100% alerts to
      ops@ripls.org) — catches the next "+20%/mo" line while it's small.
- [ ] Note the networking credit offset (−$8.64/mo) somewhere durable so its
      expiry isn't mistaken for a new cost.
**Validates:** Total bill lands ≈ $120–150/mo; a future anomaly emails ops
before it compounds.

## Related Issues
- #929 — precedent: Pexels health probe deregistered because probes burned quota; this plan generalizes that lesson with TTLs instead of losing visibility.
- #2188 — Google Maps cutover that (inadvertently) replaced Mapbox's free health probe with a billable geocode.
- #2646 — established that prod `min_instances=1` is required (realtime/background); dev has no equivalent justification.
- #1613 — log-based SLI/SLO alerting that keeps dev covered once its uptime check is removed.
- #2738 — prior storage-cost blowup (Actions artifacts) — same "unbounded accumulation" failure mode as the AR repos.

## Audit Notes (ripls-audit)

Changes applied by the audit:
- Phase 3: added the fidelity caveat that `cpu_idle = true` throttles
  post-request background work in dev (settle windows, #2657), and a task to
  update `docs/server/observability.md` Part 4, which currently states both
  envs run uptime checks every 5 minutes.
- Phase 2: added a location-verification step before the AR repo imports (the
  repos were created out-of-band; only their IAM grants are in Terraform).

Checked and found already compliant: cached-error behavior surfaces failures
rather than silently retrying (error-handling checklist); `health_check_failures`
log metric and the unhealthy-dependency alert still fire on cached-unhealthy
results, so alert semantics survive the TTL (observability checklist §4–5);
`/readyz` criticality path stays uncached; unit-test coverage enumerated for the
new health-service surface; Phase 6 secret pruning is safe because
`entrypoint.sh` and `secret_key_ref` both resolve `latest`; no proto/API surface
changes, so `proto_conventions.md` is not engaged.

Docs consulted: `docs/llms.txt`, `docs/server/conventions.md`,
`docs/server/observability.md` (Parts 2 & 4),
`docs/reviews/observability_review_prompt.md` ("Areas to Review"),
`docs/reviews/error_handling_review_prompt.md` ("Areas to Review"),
`terraform/environments/{dev,prod}/main.tf`,
`terraform/modules/{containers,storage,metabase,monitoring}`.

## Implementation Notes (2026-07-29)

All six phases implemented on the `infra_costs` branch and applied to both
environments the same day (Bryan approved applying the prod SQL restart
immediately — the maintenance window took ~5 minutes and the instance came
back healthy). Deviations and observations:

- **Health-checker TTL cache takes effect at the next server deploy** (the
  Cloud Run revisions run the image the CD pipeline last pushed). Until the
  PR merges and deploys, prod's uptime probes still trigger geocodes; dev's
  stopped immediately because its uptime check is gone.
- **Secret pruning was a near-no-op**: only one superseded version existed
  across both projects (dev `pexels-api-key/1`). The $3.45/mo Secret Manager
  line is ~30 secrets × 1–2 active versions × $0.06/replica — irreducible
  without deleting secrets. The keep-2-versions practice is documented in
  `docs/secrets.md` for future rotations; the projected ~$2/mo saving here
  was wrong.
- **Networking credit**: the $8.64/mo gross networking cost is currently
  fully offset by "Other savings" credits on the billing account. If a net
  Networking line appears in a future bill, that is the credit expiring —
  not a usage regression.
- **Billing budget**: $300/mo account-wide budget created (50/80/100%
  thresholds, default recipients = billing account admins). Required
  `user_project_override`/`billing_project` on the prod google provider —
  account-level APIs reject plain ADC calls without a quota project.
- **AR cleanup** policies are enforcing; deletion of the ~2,900 candidate
  versions happens on Artifact Registry's asynchronous schedule (roughly
  daily), so repo sizes and the billing line shrink over the following days.
- Pre-existing `SA1019` staticcheck findings (deprecated `Share*` RPCs in
  integration tests, from the #2526 merge) surface in local full-lint runs;
  untouched by this work.
