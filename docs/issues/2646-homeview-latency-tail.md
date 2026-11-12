# Issue #2646: GetHomeView cold-start latency tail: P50 192ms but P95 2.4s / P99 7.9s in prod

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2646
**Filed:** 2026-07-06 | **Last activity:** 2026-07-06
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`; `docs/weather.md` (governs the subsystem at fault);
`docs/issues/2622-2623-2624-observability-followups.md` (the calibration data that
filed this issue); `docs/server/observability.md` topics via that plan;
`docs/server/architecture.md` + `docs/server/conventions.md` (server anchors, skimmed
for the background-work and testing conventions); review-prompt "Areas to Review" for
performance and error_handling. `proto_conventions.md` engaged but no proto surface
changes. Primary evidence is not a doc: prod Cloud Logging forensics and the live
Cloud Run scaling config (reproduced below).
**UI investigation:** skipped — the misbehavior is server-side request latency,
diagnosed directly from prod request traces; the hermetic e2e environment cannot
reproduce prod's Open-Meteo latency/brownout characteristics or the in-process
cache population pattern.

## Problem Statement

**The issue's diagnosis is wrong, and the user's instinct was right.** Issue #2646
says the P95/P99 tail is "Cloud Run cold starts: at current traffic prod regularly
scales to zero" and recommends `min_instances = 1` as the eventual fix. But:

- `min_instances = 1` has been in Terraform for **both** environments since
  2026-04-09 (commit `000aa1094`, "Configure always-on servers with 2Gi memory",
  #1073): `terraform/environments/prod/main.tf:340`, `dev/main.tf:214`.
- It is **live**: `gcloud run services describe` shows
  `autoscaling.knative.dev/minScale: '1'` on both `server` (ripls-prod) and
  `server-dev` (ripls-dev). Prod does not scale to zero and has not for months.

The real cause, established from prod request traces (2026-06-28 → 07-05, all 13
GetHomeView requests ≥1500ms):

1. **The slow requests happen on warm instances.** The two P99-defining requests
   (8254ms at 07-03 23:34 and 8154ms at 07-04 00:12) ran **38 minutes apart on the
   same instance**, which had been serving since 07-02. Container boot is not
   involved anywhere in the tail.

2. **The tail is the synchronous Open-Meteo weather fetch inside `GetHomeView`.**
   `fetchHomeForecast` (`server/services/portfolio/home_weather.go:25`) calls
   `CachedProvider.DailyWeather` on the request path. On a cache miss the provider
   makes **two sequential external HTTP calls** (forecast API for days 0–16, then
   archive API for climate normals out to day 46 —
   `server/weather/open_meteo.go:68-81`), measured at **~1.9–2.0s** per miss in
   prod traces. Requests that hit the cache complete in ~150–220ms — interleaved
   in the same hour on the same instance.

3. **The 8-second P99 requests are Open-Meteo brownouts.** Both carry
   `weather skipped: provider error … Client.Timeout exceeded while awaiting
   headers` — the fetch hung until it hit the timeout budget
   (`weatherHTTPTimeout = 8s`, `open_meteo.go:21`). Two aggravators:
   - **No negative caching**: a failed fetch writes nothing to the cache
     (`cached.go:73-76`), so every request during a brownout re-pays up to 8s.
   - **Midnight cache-key rollover**: the cache key embeds the from/to dates
     (`cached.go:62`), so the entry expires at local midnight regardless of TTL —
     the 00:12 request was a forced re-fetch minutes after the 23:34 one failed.

4. **Cache misses are common at current traffic** because the cache is in-process,
   per ~0.1° grid place, 3h TTL, and date-keyed. Each distinct viewer place pays a
   fresh ~2s fetch every 3 hours, after every deploy (new instance ⇒ empty cache;
   prod deployed 5+ times inside the 07-01→07-05 measurement window), and after
   every midnight rollover. At ~19 requests/day, a handful of misses *is* the P95.

5. **Cleared suspect:** nudge regeneration (LLM + stock imagery + media copy)
   appears mid-trace in slow requests but runs in `logging.GoSafe` goroutines
   (`feed_nudges.go:164`, `InboxNudge`) that carry the request ID without blocking
   the response. The per-request DB time is also innocent: the three slow 07-05
   requests spent 166–429ms in the DB out of 1924–3475ms total.

Dev's worse tail (P95 4.7s) is the same mechanism amplified: dev deploys on every
merge to main, so its in-process weather cache is nuked many times a day.

Direction from the user (via `/ripls-issue`): *"we definitely want the home screen
loading to be faster"* — so the issue's "option 3, accept until traffic ramps" is
overridden; fix it now. The fix is in the weather path, not in Cloud Run scaling.

## Context from Comments

The issue has zero comments (self-filed from the #2622 calibration session). The
user's `/ripls-issue` invocation supplies the missing discussion: they believed
`min_instances` was already 1 for prod (correct) and want the home screen faster
(this plan). The "cold starts" claim in the issue body — and in the research
findings table of `docs/issues/2622-2623-2624-observability-followups.md` — was an
unverified inference from the P50/P95 shape; neither checked the deployed scaling
config or traced a slow request. This plan should close with the issue body
corrected (a comment with the real diagnosis) so the wrong claim doesn't propagate
into future calibration work.

## Potential Solutions

### Option A: `min_instances = 1` on prod (the issue's recommendation)
- **Approach:** What the issue proposes as the eventual fix.
- **Pros:** None — it is already deployed, in both environments, since April.
- **Cons:** A no-op that would cost investigation time chasing why nothing
  improved. Rejected on evidence.
- **Effort:** n/a.

### Option B: Take the weather fetch off the request's critical path (recommended)
- **Approach:** Four coordinated changes, all inside `server/weather` +
  `home_weather.go`, no proto or client changes:
  1. **Stale-while-revalidate in `CachedProvider`**: on TTL expiry (or date
     rollover) with a previous entry present for the place, return the stale data
     immediately and refresh in a background goroutine (`logging.GoSafe`,
     `context.WithoutCancel` — the established pattern). Bound served staleness
     (~48h) so a dead refresh path degrades to "no weather", not ancient weather.
     Single-flight the refresh via `golang.org/x/sync/singleflight` — already a
     direct dependency, so no new dep — so concurrent misses don't stampede.
  2. **Negative caching / failure backoff**: on provider error, record the failure
     for the key and don't retry for a few minutes. A brownout then costs one
     bounded fetch per backoff window instead of 8s per request. Real fetch
     failures keep the existing WARN (`weather skipped: provider error`) — the
     documented best-effort exception to fail-fast, and deliberately not ERROR,
     since the `all_errors` alert fires on any single ERROR log and a third-party
     brownout with a graceful fallback should not page. Backoff-suppressed
     fetches log at Debug so the suppression is visible without spam.
  3. **Parallelize the forecast + archive calls** in `OpenMeteoProvider
     .DailyWeather` — they are independent ranges joined afterward. Halves the
     true-miss cost (~2s → ~1.2s). Lower `weatherHTTPTimeout` from 8s to 4s.
  4. **Bounded first-fetch wait in `fetchHomeForecast`**: when there is no cached
     entry at all (first request for a place on a fresh instance), wait for the
     fetch only up to a budget (~1200ms); past it, return the view without
     weather (the contract is already best-effort) while the fetch completes in
     the background and warms the cache for the next request. The abandoned
     fetch must run under `logging.GoSafe` with `context.WithoutCancel` —
     the Connect handler's context is cancelled when the RPC returns, and
     without detaching, the abandoned fetch dies and the cache never warms.
- **Pros:** Kills every observed slow-request class: steady-state misses (SWR
  serves stale instantly), brownouts (negative cache + shorter timeout + budget),
  post-deploy cold caches (budget caps the wait; second view is warm). Benefits
  every `weather.Provider` caller — the event forecast, hourly strip, and calendar
  weather share the same `CachedProvider`. Scope is two packages; the weather doc
  explicitly names `CachedProvider` as the swappable seam.
- **Cons:** Slightly stale glyphs (up to hours) after TTL expiry — harmless for
  five-bucket sky conditions the product already labels "Typical" beyond the
  forecast horizon. First-ever view of a place on a fresh instance may render
  without weather for one load.
- **Effort:** Small-medium (mostly `server/weather/cached.go` + tests).

### Option C: Shared/persistent weather cache with a background refresher
- **Approach:** What `docs/weather.md` sketches as the multi-instance upgrade:
  back `CachedProvider` with Postgres (or Redis), plus a background job that
  proactively refreshes known viewer places, so instances never fetch in-request.
- **Pros:** Cache survives deploys and is shared across instances (max 2 today);
  eliminates even the first-view miss.
- **Cons:** New table + job + reconciliation for a dataset that is trivially
  refetchable; the in-process cache is the documented cost lever and is fine at
  ≤2 instances. Doesn't remove the need for Option B's brownout handling anyway
  (the refresher still talks to Open-Meteo). Premature at current traffic.
- **Effort:** Medium.

## Recommendation

**Option B.** Every slow request traced maps to one of the three classes it fixes,
and it needs no new infrastructure, no proto changes, and no client changes. If
traffic ramps to where instance count grows and post-deploy weatherless first
loads become visible, Option C layers cleanly on top later (same `Provider`
interface).

Also part of the recommendation, because this issue exists due to a diagnosis
error: add **per-phase duration attributes** to the `home view assembled` log line
(at minimum `weather_duration_ms`; ideally `fetch_duration_ms` for the
fetchAll+extras block too — matching the `duration_ms`/`db_duration_ms` naming
convention) so the next latency question is answerable from one log line instead
of a forensic session, and **correct the record** — comment on #2646 with the
real diagnosis and fix the "cold starts" claims in the #2622 plan doc's findings
table.

Explicitly *not* recommended: touching Cloud Run scaling (already correct),
excluding GetHomeView from the 2000ms latency tier (the #2622 calibration decision
stands — after this fix its P95 should sit honestly under the threshold), or
changing the weather provider (commercial-licensing question, tracked separately
in `docs/weather.md`).

## Open Questions

- **Sync-wait budget for a true first fetch?** — Recommendation: 1200ms. With the
  parallelized fetch (~1.2s typical miss) most first fetches still complete in
  time, and worst case total stays under the 2000ms interactive alert tier.
  Tune from the new `weather_ms` data after a week.
- **Serve-stale ceiling for SWR?** — Recommendation: 48h. A two-day-old daily
  forecast is still glyph-accurate; beyond that, prefer showing nothing (matches
  the best-effort contract). The date-keyed cache needs the SWR lookup to fall
  back to the previous day-span's entry for the same grid point — key the stale
  lookup by place, not by exact span.
- **Failure-backoff duration?** — Recommendation: 5 minutes per cache key, stored
  alongside the entry (no new machinery). During the observed 40-minute brownout
  that's ≤8 bounded retries instead of every-request stalls.
- **Should the experience-service weather surfaces get the request-path budget
  too?** — Recommendation: not in this issue. They inherit SWR, negative caching,
  and the faster miss automatically via the shared `CachedProvider`;
  `GetExperienceDayWeather` is a lazy per-day tap (user-initiated, small blast
  radius). File a follow-up only if their latency data says so.
- **Re-check dev after Phase 2?** — Recommendation: yes; dev is the natural
  validation environment for the post-deploy cold-cache class because every merge
  exercises it. Expect dev GetHomeView P95 to drop from ~4.7s to near its P50.

## Implementation Plan

### Phase 1: Cache hardening in `server/weather`
**Goal:** A cache miss or provider failure can no longer cost multiple seconds on
any caller's request path.
**Tasks:**
- [x] `CachedProvider`: stale-while-revalidate — keep the last good entry per
      grid point (daily and hourly), serve it when the fresh entry is expired or
      date-rolled (≤48h stale ceiling), refresh via `logging.GoSafe` +
      `context.WithoutCancel`, single-flighted per key.
- [x] `CachedProvider`: negative cache — on provider error, record failure time
      for the key; skip re-fetch (serve stale or nothing) for 5 minutes.
- [x] `OpenMeteoProvider.DailyWeather`: issue the forecast and archive range
      fetches concurrently; join and sort as today. Drop `weatherHTTPTimeout`
      8s → 4s.
- [x] Unit tests (`server/weather`, fake provider + injected clock — the
      existing `CachedProvider.now` hook): stale served on expiry while refresh
      runs; refresh single-flight; stale ceiling honored; failure backoff
      suppresses refetch then allows it; parallel fetch merges identically to
      sequential; no test calls the live API (existing rule). Run the package
      tests under `go test -race` — SWR + singleflight is new concurrency.
**Validates:** The steady-state miss and brownout classes are gone at the
subsystem level, provably in tests, before touching the RPC.

### Phase 2: Request-path budget + observability in the portfolio service
**Goal:** GetHomeView never waits more than ~1.2s for weather, and the wait is
measurable.
**Tasks:**
- [x] `fetchHomeForecast`: run the provider call so the request waits at most
      ~1200ms (launch via `logging.GoSafe` with `context.WithoutCancel`, select
      on a result channel; on budget expiry return nil while the fetch completes
      in the background and populates the cache). Keep the existing best-effort
      logging taxonomy (`weather skipped: …`).
- [x] Add `weather_duration_ms` (and `fetch_duration_ms` for the fetchAll+extras
      block) to the `home view assembled` log line in `GetHomeView`.
- [x] Tests: budget expiry returns nil forecast + still-successful view; fast
      provider path unchanged; background population observable via fake provider.
- [x] Run the standard server verification (`npm run build` = golangci-lint +
      `go build ./...`, plus `go build -tags=integration ./...` and the full
      `go test ./server/...` suite, all green locally; changed packages also
      pass under `-race`). Note: `go test -race ./server/services/experience/`
      exposes a pre-existing data race in `ai.MockProvider.ParseInformalTime`
      (unrelated to this change; CI runs without `-race`) — tracked as #2658.
**Validates:** Worst-case GetHomeView latency is structurally capped near
P50 + budget, independent of Open-Meteo's behavior.

### Phase 3: Verify in prod, correct the record, close
**Goal:** Evidence the tail is gone; no wrong diagnosis left behind.
**Tasks:**
- [ ] After ~1 week of prod data post-deploy, re-pull GetHomeView P50/P95/P99
      (same Monitoring API query as the #2622 calibration) for prod and dev;
      paste the before/after table into this doc.
- [ ] Comment on #2646 with the corrected diagnosis (min instances were already 1;
      the tail was the synchronous weather fetch) and close if numbers hold.
- [x] Fix the "Cloud Run cold starts" / "regularly scales to zero" claims in
      `docs/issues/2622-2623-2624-observability-followups.md` (findings table and
      recommendation bullet) with a pointer here.
- [x] Update `docs/weather.md` "Caching is the cost lever" section: SWR semantics,
      stale ceiling, negative caching, request-path budget (+ refresh its
      `freshness:` stamp).
**Validates:** P95 under the 2000ms interactive tier without any alert or SLO
carve-out for the app's front door — the outcome #2622 wanted but couldn't have.

## Related Issues
- #2622 / #2624 — the latency calibration and SLOs that surfaced this; GetHomeView
  deliberately stayed in the 2000ms tier expecting this issue to fix the tail.
- #2514 — the weather substrate this plan modifies (`docs/weather.md`).
- #2435 — the Home view (v4.1) whose RPC this is.
- #1073 — where `min_instances = 1` actually shipped (2026-04-09), disproving the
  issue's premise.

## Audit notes (2026-07-06)

Audited via `/ripls-audit`. Changes applied by the audit:

1. **Log field names aligned to convention.** `weather_ms`/`fetch_ms` renamed to
   `weather_duration_ms`/`fetch_duration_ms`, matching the documented
   `duration_ms`/`db_duration_ms` standard-field pattern
   (`docs/server/conventions.md` → Structured Logging).
2. **Abandoned-fetch context made explicit.** The Phase 2 budget-expiry path now
   states `logging.GoSafe` + `context.WithoutCancel` — the Connect handler ctx
   is cancelled at RPC completion, so without detaching, the abandoned fetch
   dies and the cache never warms (verified `GoSafe` exists,
   `server/logging/logger.go:270`, and that `feed_nudges.go` establishes the
   pattern).
3. **Singleflight without a new dependency.** `golang.org/x/sync v0.21.0` is
   already a direct go.mod dependency; the plan now names
   `x/sync/singleflight` instead of leaving room to hand-roll or add a dep
   (minimal-dependencies principle).
4. **WARN-not-ERROR decision recorded.** Weather-provider failures stay WARN:
   they are the documented best-effort exception to fail-fast (weather.md and
   `home_weather.go` both state the contract), and the `all_errors` alert fires
   on any single ERROR log — logging brownouts at ERROR would auto-file issues
   for a third-party hiccup that degrades gracefully. Backoff-suppressed fetches
   log at Debug so suppression is observable.
5. **Race detector required for Phase 1 tests** — SWR + singleflight is new
   concurrency in `CachedProvider`; also pinned the tests to the existing
   injected-clock hook (`CachedProvider.now`).

Checked and passed without changes: outbound-HTTP security (Open-Meteo hosts are
constant, so plain `http.Client` is the documented-correct choice — no
`safehttp` needed); no proto surface (api_design not engaged); no SQL changes
(SQL-efficiency rules untouched); `client/calendar.md` and `client/inbox.md`
only claim weather is best-effort, which this plan preserves, so weather.md is
the only durable doc needing a Phase 3 update; issues-dir docs are excluded
from the context map, so editing the #2622 plan doc needs no frontmatter work.

Docs consulted: `docs/llms.txt`; anchors `docs/server/architecture.md` (structure
skim), `docs/server/conventions.md` (error handling, outbound HTTP, SQL
efficiency, structured logging), `docs/weather.md` (full), `proto_conventions.md`
(engaged, no proto changes); review-prompt "Areas to Review" for performance,
error_handling, observability; `docs/issues/2622-2623-2624-observability-followups.md`;
implementation surfaces `server/weather/{cached,open_meteo}.go`,
`server/services/portfolio/{home_view,home_weather}.go`,
`server/services/feed/feed_nudges.go`, `server/middleware/{logging,query_stats}.go`,
`server/logging/logger.go`, `terraform/modules/containers/main.tf`,
`terraform/environments/{dev,prod}/main.tf`, `go.mod`; and live state via
`gcloud run services describe` + prod Cloud Logging traces.
