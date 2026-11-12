# Issue #2757: [ops:page] Dependency health check failed 2 consecutive times — `dep:ai backend:openai` recurrence

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2757
**Filed:** 2026-07-25 | **Last activity:** 2026-07-25
**Labels:** `auto-filed`, `claude-fix`, `ops`, `severity:page`, `dep:ai`, `backend:openai`, `gcp-incident:0.oanj0xuxqn0w` | **Assignees:** unassigned
**Context docs:** `docs/server/architecture.md` (⚓), `docs/server/conventions.md` (⚓), `docs/server/llm.md`, `docs/server/observability.md` (⚓), `docs/reviews/observability_review_prompt.md` (Areas to Review), `docs/reviews/error_handling_review_prompt.md` (Areas to Review) — auto-selected from `docs/llms.txt`; none passed as input.

## Problem Statement

The `Health Check Unhealthy - server` alert paged in prod for
`dependency=ai, backend=openai` — `OpenAIProvider.CheckHealth`
(`server/ai/provider_openai.go:452-475`) returned an error twice in a row
over the 10-minute alignment window, tripped the threshold of 2, then
auto-resolved. **This is the 2nd `dep:ai backend:openai` page in 15 days**
(prior: #2692 on 2026-07-10; both auto-closed by `ripls-alerts-bot` on
`state=closed` Pub/Sub events within minutes). The prior triage on
#2692 explicitly said: *"If a recurring pattern emerges (2+ pages within
a couple of weeks), open a follow-up with one of these durable
mitigations."* This is that trigger.

Two facts make this a **noise page**, not real degradation:

1. **OpenAI is the tertiary fallback** in the production AI chain
   (`server/ai/provider_factory.go:99-214`): primary = Gemini 3.1
   Flash-Lite (Vertex AI), fallback = Anthropic Claude Haiku 4.5, tertiary
   = OpenAI GPT-5-mini. `FallbackProvider.CheckHealth`
   (`server/ai/provider_fallback.go:440-460`) probes the primary and every
   fallback in sequence, so the alert fires when *any* leaf is unhealthy
   even if primary + secondary were healthy the whole time and no user
   request ever touched OpenAI. In practice OpenAI is reached only when
   both Gemini and Anthropic have already failed for the same request —
   near-zero traffic.
2. **`ai` is not in `criticalDependencies`** (`server/services/health/service.go:24-27`;
   only `database` and `storage` are), so the page did **not** pull the
   pod out of rotation via `/readyz`. Every fallback provider is
   redundant to the primary; the fallback chain already routes user Gen*
   calls around any single-provider failure. User-facing impact of the
   two OpenAI blips has been zero.

The OpenAI SDK is already configured with `option.WithMaxRetries(5)`
(`server/ai/provider_openai.go:94`), so each `Models.List` call the health
check makes has already survived (or exhausted) 5 SDK-internal retries with
exponential backoff before returning error — but the health check caps the
context at **10s** (`server/ai/provider_openai.go:454`), which is barely
enough for a couple of retries with backoff and gets cancelled long before
the SDK's 5-retry budget completes. That amplifies transient OpenAI-side
jitter, packet loss, or a single upstream 5xx into a `dependency_health_check_failed`
log, which the `dependency_health_failures_prod` log-based metric
(`terraform/modules/monitoring/main.tf:107-140`) counts and the
`Health Check Unhealthy - server` policy (`terraform/modules/monitoring/main.tf:376-437`)
pages on after 2 in ~10 minutes.

## Context from Comments

The issue itself has **zero comments** — it's a fresh auto-file from the
Cloud Monitoring pipeline. All actionable context lives on the twin
issue **#2692** (2026-07-10, same alert, same
`{dep:ai, backend:openai}` label pair), which I closed as blocked with a
detailed comment enumerating:

- The exact probe (`p.client.Models.List(ctx)` with a 10s ctx).
- OpenAI being tertiary in the fallback chain and not in
  `criticalDependencies`, so this page has no user-facing impact.
- Structural precedent — #1925 (Mailgun, 2026-05-14) and #2267 (Mapbox,
  2026-05-31) both auto-closed the same way and were treated as won't-fix.
- #2561 (Mapbox recurrence, 2026-06-26) was the first *recurrence* and
  got a targeted plan (#2564: "gate Mapbox health-check on active
  provider"). **That plan was closed unmerged**, so the mitigation
  itself is unshipped precedent — but the pattern (recurrence triggers a
  plan) is established.
- Three durable-mitigation candidates offered for the OpenAI case
  specifically: (a) single-retry wrapper around `OpenAIProvider.CheckHealth`,
  (b) bump `var.dependency_failure_threshold` from 2 to 3, or
  (c) reconsider whether OpenAI needs its own health check at all given
  its role.

**Maintainer signal:** #2692 was closed within minutes of the page —
consistent with the working assumption that these are transient blips
and the escalation is "wait for a recurrence, then plan." That has now
happened, so this plan follows the option-A/B/C shape from that
comment and picks a recommendation.

## Potential Solutions

### Option A: Gate the AI health check on primary-only, drop fallback leaves from `FallbackProvider.CheckHealth`

- **Approach:** Change `FallbackProvider.CheckHealth`
  (`server/ai/provider_fallback.go:440-460`) so the composite status
  reports only the **primary** provider's health (or an "at least one
  provider healthy" summary), and drop the leaf statuses for fallback
  providers. Fallbacks continue to be exercised on real requests via the
  existing `FallbackProvider.ClassifyUnifiedCreate` / `Gen*` retry loop
  — that's what actually matters — but we stop paging on a tertiary
  probe that has near-zero relation to user impact.
- **Pros:** Root-cause fix; the page cannot recur for `backend:openai`
  or `backend:anthropic` in the current prod chain because the metric
  won't be emitted for those backends. Symmetric with the `ai` label's
  not-critical status. Removes the false-positive class entirely, not
  case-by-case.
- **Cons:** Loses observability into fallback-provider health *when the
  primary is fine*. If Anthropic silently stops working we won't page
  on it via the health probe — we'd only discover on the first real
  request that fell through from Gemini. The daily digest (planned but
  not shipped, per `docs/server/observability.md:463`) would still cover
  it. Loses the per-backend log line that today's alert relies on.
- **Effort:** Small (~30 net lines: shrink `FallbackProvider.CheckHealth`
  + adapt `TestFallbackProvider_CheckHealth` in `provider_fallback_test.go`
  + update `docs/server/observability.md:441-447`).

### Option B: Bump the health-check context timeout so the SDK's built-in retries can actually run

- **Approach:** Extend the `context.WithTimeout` in each provider's
  `CheckHealth` from **10s** to **30s** so the OpenAI SDK's configured
  5-retry exponential-backoff budget (`server/ai/provider_openai.go:94`)
  has room to actually complete before we cancel it. Apply symmetrically
  to `AnthropicProvider.CheckHealth` (`server/ai/provider_anthropic.go:492`)
  and `GeminiProvider.CheckHealth` (`server/ai/provider_gemini.go:385`)
  since they have the same 10s cap and the same SDK-retry pattern.
- **Pros:** Fixes the amplification directly — jitter that the SDK
  already knows how to smooth over gets a chance to do so. Preserves
  per-backend observability. Symmetric across providers. No change to
  the alert threshold, so real persistent failures still page in ~10
  min. Uses the uptime-check timeout of 30s
  (`terraform/modules/monitoring/variables.tf:51-55`) as the natural
  ceiling — three 30s probes still fit in a single 30s uptime check because
  they run sequentially inside `Service.CheckHealth`, so `/health` could
  approach the uptime-check timeout on a worst-case all-providers-timing-out
  round. That means bumping to 30s is only safe if we shrink the per-provider
  budget or run probes in parallel; a **20s** per-provider ceiling with
  the current serial checker is the safe number.
- **Cons:** A worst-case cold `/health` still bounds at
  `3 × 20s = 60s > uptime_check_timeout=30s`, tripping the uptime alert
  instead of the dependency alert. Only truly safe with a parallelized
  `Service.CheckHealth` or a shorter budget per probe. Also masks
  real degradation lasting < 20s: if OpenAI is genuinely slow, we won't
  see it until it exceeds 20s. Symptomatic — treats the smoothing gap,
  not the "why is a tertiary probe pageable at all" question.
- **Effort:** Small (a few lines per provider + updated docstrings), but
  medium if we also parallelize `Service.CheckHealth` to preserve the
  uptime-check budget — which we should to avoid moving noise from one
  alert to another.

### Option C: Add a soft-vs-hard dependency severity so non-critical dependencies use a higher threshold

- **Approach:** Extend the health service with a per-registration
  severity concept (`RegisterCriticalFunc` vs `RegisterFunc`, or a
  `Severity` field on `Checker`), emit that severity as a `severity`
  label on `dependency_health_check_failed` log entries, extract it in
  the log-based metric (`terraform/modules/monitoring/main.tf:107-140`),
  and add a second alert policy that fires at `threshold=3` (or `4`) for
  `severity=soft` while the existing `threshold=2` policy fires only for
  `severity=hard`. Register `database`/`storage` as hard, everything else
  (email, geocoding, ai) as soft.
- **Pros:** Principled — encodes the existing informal `criticalDependencies`
  set (`server/services/health/service.go:24-27`) into the alerting layer,
  where it belongs. Applies once to email/mapbox/google_maps/ai and their
  future siblings, not per-provider. Preserves per-backend observability.
- **Cons:** Largest surface (service + proto/status + metric + alert +
  terraform + docs). Higher risk of regression across dev+prod alert
  wiring. Overshoots the reported problem — this is one recurring page
  on one backend, not a systemic alerting-taxonomy gap. Better done as a
  follow-up if a second class of alert (email or geocoding recurrence)
  appears.
- **Effort:** Medium-to-large.

## Recommendation

**Option A**, with **Option C explicitly deferred** to a follow-up if a
second `severity:page` recurs on a non-critical dependency's health
probe (email, geocoding, or a future fallback provider). Option B is
attractive but structurally weak because the current serial `Service.CheckHealth`
budget doesn't leave room to safely extend the per-provider timeout
without also parallelizing the checker or shrinking the per-provider cap
— which turns a "one-line raise" into a "restructure the checker" and
then only *partially* addresses the problem (jitter longer than 20s still
pages).

Option A directly matches the observability-review question the recurring
page is asking: **should a tertiary-fallback dependency page at all
when its probe fails but no user request touched it?** The `ai`
dependency's role in production is answered by
`FallbackProvider.ClassifyUnifiedCreate` (`server/ai/provider_fallback.go:325-434`):
if *any one* of {Gemini, Anthropic, OpenAI} works, users are fine. Paging
on any single-leaf probe failure is a poor signal-to-noise trade — the
alert as currently wired treats "one provider is momentarily off" as
equivalent to "AI generation is down for users", which the fallback
architecture explicitly designed away.

The right composite health signal for a fallback chain is **"is at least
one provider healthy?"**, not "are all providers healthy?" — and that's
the shape Option A takes. It preserves the primary's per-backend page (a
Gemini outage still pages instantly), keeps every provider's `CheckHealth`
implementation intact (they're still exercised by
`OpenAIProvider.CheckHealth`, `AnthropicProvider.CheckHealth`, etc. — just
not surfaced through the composite), and drops precisely the noise class
this issue is a symptom of.

**Effort:** small; **blast radius:** narrow (Go + one test file + one
docs section update); **rollback:** trivial (revert the composite change,
per-provider probes are unchanged).

## Open Questions

- **Should the composite return the primary's status alone, or a
  synthesized `Name: "ai"` status with the "at-least-one-healthy" rule?**
  — Recommendation: **synthesized composite**. Emit one
  `*health.Status{Name: "ai", Backend: "<primary>+fallback"}` whose
  `IsHealthy()` reflects `primary.Healthy || any(fallback.Healthy)`,
  and whose `Metadata` carries `primary_backend`, `primary_healthy`,
  `fallback_backends`, and `healthy_fallback_count` for debugging.
  This keeps the per-request `dependency_health_check_failed` log
  usable (still one log line per composite failure, with metadata),
  and only fires the alert when the whole chain is unhealthy — a real
  user-impact condition. A "primary-only" report on a failing primary
  with healthy fallbacks would still be `Healthy: false` (same page
  we want to suppress); the synthesized approach is the only one that
  correctly recognizes user-visible AI-availability.

- **Do we want a separate, lower-severity signal for individual
  fallback-provider health (so ops still notices Anthropic silently
  breaking)?** — Recommendation: **yes, but log-only, not
  metric-backed**. Emit a **WARN-level** `dependency_leaf_unhealthy`
  log per unhealthy leaf (see Phase 1). The `all_errors`
  log-based metric (`terraform/modules/monitoring/main.tf:151-155`)
  requires `severity>="ERROR"` so a WARN log will not fire the
  "Server Error Logged" alert; the `dependency_health_failures` metric
  filters on the exact `message="dependency_health_check_failed"`
  string so a `dependency_leaf_unhealthy` message will not fire the
  "Health Check Unhealthy" alert either. Ops can still grep for
  `dependency_leaf_unhealthy` on demand, the daily-digest pipeline
  (`docs/server/observability.md:463-518`) can surface it when built,
  and no change to Terraform is required. If ops finds themselves
  surprised by a silent leaf outage post-merge, promote it to a
  dashboard-only INFO metric as a follow-up.

- **Does `LoadBalancedProvider` (`server/ai/provider_loadbalanced.go`)
  need the same treatment?** — Recommendation: **yes, symmetric change**.
  `LoadBalancedProvider.CheckHealth` shares the same "every leaf paginates
  independently" pattern. Apply the same at-least-one-healthy composite
  there so the fix is uniform across the two composite providers. If
  scope pressure appears in review, keep it in this PR — it's ~10 more
  lines and the same test shape.

- **Do we need to touch the alert policy at all?** — Recommendation:
  **no**. The Terraform policy is already correctly shaped ("page when a
  backend is unhealthy") — the fix is that we should stop *reporting*
  fallback-leaf failures as backend-level unhealth. Zero terraform diff
  is a big win: no infra apply, no state churn, no dev/prod skew window.

## Implementation Plan

### Phase 1: Introduce the composite health status for `FallbackProvider`
**Goal:** Stop `dependency_health_check_failed` log entries from firing
for a fallback-leaf failure when the primary or another fallback is
healthy.

**Tasks:**
- [ ] In `server/ai/provider_fallback.go:437-460`, replace the current
  fan-out with a synthesized composite status. **Keep the leaf calls
  sequential** — the existing shape is sequential, `Service.CheckHealth`
  in `server/services/health/service.go:76` iterates checkers
  sequentially anyway, and parallelizing here would drag in the
  goroutine-panic-safety rules (`docs/server/architecture.md` §
  *Goroutine Safety*: every spawned goroutine needs a `recover`
  wrapper, enforced by `server/cmd/check-goroutines`) without a
  measurable latency win at the current three-leaf scale.
  - Run the primary's `CheckHealth`; capture its status(es).
  - Run each fallback's `CheckHealth` in order; capture each's
    status(es).
  - Return a single `*health.Status{Name: "ai", Backend: "fallback"}`
    whose `IsHealthy()` is `primary.IsHealthy() || any(fb.IsHealthy())`,
    `Backend` is `"<primary_backend>+fallback"`, `LatencyMs` is the sum
    across leaves, and `Metadata` includes
    `{primary_backend, primary_healthy, primary_error, fallback_backends,
    healthy_fallback_count, first_fallback_error}`.
  - When at least one leaf is unhealthy — regardless of composite
    health — emit one **WARN-level** log per unhealthy leaf with
    `message="dependency_leaf_unhealthy"`, `dependency="ai"`,
    `backend=<leaf name>`, `error=<leaf error>`. Rationale:
    `docs/server/conventions.md` § *Log levels* reserves WARN for
    *"unexpected but handled (retries, fallbacks, non-critical
    failures)"* — a leaf failure with composite healthy is exactly
    that. Do **not** use `message="dependency_health_check_failed"`
    for this leaf log; that message is reserved for the composite
    unhealthy case and powers the log-based metric
    (`terraform/modules/monitoring/main.tf:112-116`, exact-match on
    `jsonPayload.message`), so a new distinct message name keeps
    the metric definition untouched (zero-terraform-diff invariant
    from the *Open Questions* section).
  - When the composite is unhealthy (all leaves failed), the
    existing `Service.CheckHealth` (`server/services/health/service.go:122-141`)
    already emits `dependency_health_check_failed` at ERROR — no
    change needed there, the composite Status flows through the
    existing per-status log path.
- [ ] Update `TestFallbackProvider_CheckHealth` in
  `server/ai/provider_fallback_test.go` (and add cases if any are
  missing) to assert the new composite shape:
  - primary healthy → composite healthy (whatever fallbacks say)
  - primary unhealthy, any fallback healthy → composite healthy
  - all leaves unhealthy → composite unhealthy, metadata carries
    per-leaf errors
  - test that only **one** status is returned and its `Backend` reflects
    the composite (no leaf statuses appended)
- [ ] Apply the same composite shape to
  `LoadBalancedProvider.CheckHealth` in
  `server/ai/provider_loadbalanced.go` and its test in
  `server/ai/provider_loadbalanced_test.go` — same at-least-one-healthy
  rule, same metadata keys renamed for the load-balanced case
  (`primary_backend` → `weighted_backends`).

**Validates:** In dev, a probe run with one leaf forced to fail returns
`healthy: true` and does not emit `dependency_health_check_failed`.

### Phase 2: Update tests, docs, and cross-links
**Goal:** Codify the new behavior, refresh the docs the reader will hit,
close the tracking-issue loop.

**Tasks:**
- [ ] Refresh `docs/server/observability.md`:
  - `:441-447` ("AI Providers" bullet under "Current Implementations")
    — add a note that `FallbackProvider` and `LoadBalancedProvider` now
    report a single composite status per the at-least-one-healthy rule,
    with per-leaf detail in the log stream at INFO.
  - `:929-934` ("Health Check Unhealthy" section) — add a line noting
    that composite providers (fallback, load-balanced) suppress
    per-leaf pages by design; individual-leaf visibility lives in
    `dependency_leaf_status` INFO logs and is planned for the daily
    digest (§ 463).
- [ ] Refresh `docs/server/llm.md:51` ("FallbackProvider" bullet) to
  reference the new composite shape.
- [ ] Do **not** hand-edit `freshness:` stamps on the touched durable
  docs. Per `docs/context_map.md` § *Prose freshness*, stamps are
  *"Written only by the skill, never by hand. The skill advances the
  stamp on every verification run"* — the `claude_refresh_docs`
  nightly ratchet will advance them after this PR merges, using
  `ripls-doc-freshness` to re-verify the prose against the new code.
- [ ] Comment on #2692 to cross-link this issue and mark the recurring
  page as addressed by #2757.
- [ ] Comment on #2561 to cross-link — the mapbox equivalent stayed as
  an unshipped plan; if the pattern recurs, this PR is the shape to
  apply.

**Validates:** `npm run lint` (including `lint:context-map`) passes;
`npm run generate:context-map` is a no-op after the freshness bumps.

### Phase 3: Verify with integration test
**Goal:** Prove the fix under real API conditions.

**Tasks:**
- [ ] Extend `server/ai/health_check_integration_test.go` (build tag
  `integration`) with `TestFallbackProvider_CheckHealth_Composite_Live`:
  - Real Gemini + Anthropic providers + one deliberately-broken OpenAI
    provider (invalid API key).
  - Assert composite is healthy despite the OpenAI leaf being broken.
  - Assert no `dependency_health_check_failed` log line is captured
    (via `logging.NewTestLogger`).
- [ ] Run: `npm run generate && go test -tags=integration -timeout 20m ./server/ai/...`.

**Validates:** End-to-end contract: a real broken tertiary does not
trigger the metric that powers the page.

## Related Issues

- #2692 — Prior identical page (`dep:ai backend:openai`, 2026-07-10).
  Triaged as blocked pending recurrence; this issue is that recurrence.
- #2561 — Structural precedent: mapbox recurrence that took the plan
  path (#2564). Plan was closed unmerged; this PR is the first shipped
  instance of the recurrence-triggered mitigation pattern for a
  fallback dependency.
- #2267 / #1925 — First occurrences of the same alert class on Mapbox /
  Mailgun respectively. Both closed as transient.
- #2223 — "Delete Mapbox after Google Maps cutover settles." Adjacent
  cleanup; unrelated to the AI path.
- #929 — Precedent for de-registering a health check whose per-probe
  cost is disproportionate to its signal (Pexels/Unsplash `CheckHealth`
  is implemented but not registered, per
  `docs/server/observability.md:419-422`). This PR is the same rationale
  applied to composite fallback providers, one layer up.

---

## Audit notes (added by `/ripls-audit`)

**Docs consulted:** `docs/llms.txt`, `docs/server/architecture.md` (⚓
anchor, § *Goroutine Safety*), `docs/server/conventions.md` (⚓ anchor,
§ *Structured Logging* / § *Log levels*), `docs/server/observability.md`
(⚓ anchor, § *Dependency Health Checks* + § *Alerting*), `docs/server/llm.md`
(subsystem leaf), `docs/reviews/observability_review_prompt.md` (⚓, §
*Areas to Review* — Health Checks + Alerting Foundations),
`docs/reviews/error_handling_review_prompt.md` (⚓, § *Areas to Review* —
Retry & Recovery + Graceful Degradation), `docs/reviews/code_quality_review_prompt.md`
(⚓, § *Areas to Review* — TODO Hygiene + Strong Typing),
`docs/context_map.md` § *Prose freshness*.

**Fixes applied in-review:**
1. **Freshness stamps.** Original plan Phase 2 asked the implementer to
   hand-edit `freshness:` stamps on touched durable docs. Removed —
   `docs/context_map.md` § *Prose freshness* is explicit: stamps are
   *"Written only by the skill, never by hand"* and are advanced by
   `ripls-doc-freshness` via the nightly `claude_refresh_docs` ratchet.
   Replaced the task with the correct expectation (nightly ratchet will
   pick up).
2. **Goroutine safety.** Original Phase 1 proposed running fallback
   checks in an `errgroup.WithContext` group. `docs/server/architecture.md`
   § *Goroutine Safety* requires every server-spawned goroutine to be
   `recover`-wrapped (enforced by `server/cmd/check-goroutines`), and
   `errgroup.Go` does not recover; combined with the fact that today's
   implementation and `Service.CheckHealth` itself are both sequential
   at zero-observable-latency cost, changed Phase 1 to stay sequential.
3. **Log level & message name for leaf detail.** Original plan proposed
   an INFO-level `dependency_leaf_status` log. Two adjustments:
   (a) per `docs/server/conventions.md` § *Log levels*, WARN is the
   correct level for *"unexpected but handled … fallbacks, non-critical
   failures"* — bumped to WARN and renamed to `dependency_leaf_unhealthy`.
   (b) verified against `terraform/modules/monitoring/main.tf:107-140,151-155`
   that the new message name is safe: the `dependency_health_failures`
   metric filter is exact-match on `jsonPayload.message="dependency_health_check_failed"`
   and the `all_errors` metric filter is `severity>="ERROR"` — a WARN log
   with a distinct message name will not fire either alert. Made this
   invariant explicit in the plan so the implementer preserves it.
4. **Composite `Backend` label.** Tightened from `<primary_backend>+fallback`
   to `"<primary>+fallback"` and made explicit that `LatencyMs` is the
   sum (sequential) — was ambiguous ("max across leaves") in the original.
5. **Metric-safety open question.** Reworded the second open question
   to reference the exact-match and `severity>=ERROR` filters so the
   answer stands on the terraform code, not on a general claim.

No blockers found; plan is ready to commit.
