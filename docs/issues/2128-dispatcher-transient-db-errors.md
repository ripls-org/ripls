# Issue #2128: [ops:page] Service-level error logged (excluding middleware and infrastructure)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2128
**Filed:** 2026-05-23 | **Last activity:** 2026-05-23
**Labels:** auto-filed, ops, severity:unknown, gcp-incident:0.o86tcqdy9d11
**Assignees:** unassigned
**Context docs:** none

> **Audit (2026-05-23):** Plan reviewed against `docs/server/architecture.md`
> and `docs/server/observability.md` (client docs not applicable — this plan
> is server-only). Three changes applied:
>
> 1. **WARN vs ERROR mapping matches the observability log-level contract.**
>    `docs/server/observability.md:60-65` defines WARN as "unexpected but
>    handled (retries, fallbacks)" and ERROR as "failures requiring
>    attention." The plan's transient-streak classifier maps cleanly onto
>    this — confirmed and called out explicitly under *Implementation Plan*.
> 2. **`TransientStreak.Classify(nil)` reshaped as an explicit `Reset()`
>    method.** Returning a `slog.Level` for the success path conflated two
>    APIs: callers don't log on success, they just need to clear the
>    streak. Splitting it into `Classify(err) slog.Level` and `Reset()`
>    makes the contract obvious and avoids the trap of accidentally
>    logging INFO every tick.
> 3. **Per-call-site boilerplate factored into `errs.LogJobError`.** Per
>    the user's "factor to shared code as much as possible" steer, the
>    level switch is encapsulated so each background-job site becomes a
>    single line.
> 4. **New `server/errs/` directory will get a `README.md`** as required
>    by the project rule in `CLAUDE.md` ("every multi-file directory
>    under `server/` must have a `README.md`").
> 5. **Goroutine safety unchanged.** Every modified call site already
>    runs inside `logging.GoSafe` per `docs/server/architecture.md:919`;
>    this plan only changes the body of those goroutines, not their
>    construction.

## Problem Statement

The Cloud Monitoring "Server Error Logged" policy paged on-call at
2026-05-23T08:21:38Z. The log line that fired the alert was emitted from
the scheduled-notifications dispatcher in production:

```
message: scheduled-notification dispatch failed
error:   find due: FindDueScheduledNotifications:
         read tcp 169.254.8.1:40932->192.0.2.2:5432: read: connection reset by peer
```

This is a transient TCP-level reset of the connection between the Cloud Run
revision and the PostgreSQL proxy. The pool's idle connection was killed
out-of-band (Cloud SQL idle timeout, proxy restart, or transient network
blip), and the next query on that connection observed the reset. The
dispatcher runs every 60 seconds (`server/main.go:1231`), so the next tick
almost certainly succeeded — the system is already self-healing for this
class of error.

The bug is **not** that the connection got reset. The bug is that the
dispatcher treats every error path identically as ERROR-level
(`server/main.go:1259`), which fires the "Server Error Logged" alert
policy and pages on-call for a condition that requires no human action.
The same pattern exists for the reconciler at `server/main.go:1238` and
for the community-purge / activity-digest jobs (`server/main.go:1205,
1290, 1301`), so an identical page is one Cloud SQL hiccup away from
each of them.

This needs a fix that is targeted enough not to silence real database
outages (which absolutely should page), but loud enough to suppress the
single-tick blip class. Memory note
[[feedback_keep_error_logs]] applies: don't downgrade an ERROR that
exposes an upstream client/data bug. A TCP reset on an idle pooled
connection is not an upstream bug — it's expected behavior of a pooled
SQL client across cloud network paths.

## Context from Comments

The issue has no comments — it was auto-filed by the `/ripls-reviews`
pipeline from a Cloud Monitoring alert and awaits triage. Worth noting:

- The alert policy that fired is `Server Error Logged - server`, which
  matches ANY ERROR-level log from the `server` Cloud Run service that
  isn't filtered out as middleware/infrastructure. Adjusting the alert
  policy is one of the options below, but is brittle (regex on log
  text) compared to fixing the log level at the source.
- This is the first such incident from the dispatcher since the
  scheduled-notifications system shipped (per the absence of prior
  `gcp-incident:` issues referencing dispatch failure). The class of
  failure is uncommon, so a heavy fix (retry-with-backoff inside
  every storage call) would be over-engineering.

## Potential Solutions

### Option A: Downgrade single-tick transient errors to WARN at the dispatcher call site, with a single shared time-based escalator

**Approach:** Introduce a shared `server/errs/` package containing
*both* the transient-error predicate and a small classifier that
tracks how long the current run of transient failures has been going.
The predicate is extracted from the existing
`server/ai/errors.go::IsTransientError` (identical at the string
level). The classifier — call it `TransientStreak` — exposes one
method per error site: `Classify(err) slog.Level`. Internally it
records the timestamp of the first transient failure in the current
streak and returns WARN while `time.Since(firstTransient) <
5*time.Minute`, ERROR otherwise; any success or non-transient error
resets the streak. The same classifier instance is used at every
background-job call site — dispatcher, reconciler, community-purge,
activity-digest — with no per-job thresholds.

- **Pros:**
  - Targeted to the actual problem.
  - Reuses an existing, tested predicate.
  - Single-tick blips no longer page; sustained outages still do
    once they cross 5 minutes — uniform across every background job.
  - One shared helper, one shared test surface, one shared policy to
    document. The 5-minute threshold is the operator-facing knob.
  - No retry latency added inside the storage layer.
- **Cons:**
  - WARN logs are still visible in Cloud Logging — operators retain
    the signal, just without the page. (This is by design.)
  - For jobs with tick intervals longer than 5 minutes (the hourly
    activity-digest), the very first transient failure crosses the
    5-minute mark and immediately logs ERROR. That's arguably
    correct — an hourly job that fails has been failing for at least
    an hour — but it's worth calling out.
- **Effort:** Small (one new package + edits to ~5 call sites).

### Option B: Add inline retry with jittered backoff inside FindDueScheduledNotifications

**Approach:** Wrap the `s.db.QueryContext` call (and the row iteration)
in a 2- or 3-attempt retry loop with short exponential backoff (50ms,
200ms, 500ms with jitter), only retrying on errors matching
`IsTransientError`. The dispatcher tick effectively gets a tiny
self-heal window inside the 60-second tick.

- **Pros:**
  - The dispatcher succeeds even when one query fails, so the WARN/ERROR
    distinction matters less.
  - Hides single-blip failures from the log entirely (good if you trust
    the retry; arguably bad if you want visibility into pool health).
- **Cons:**
  - Adds complexity inside the storage layer — and the next person who
    sees a `connection reset` log will go looking for retry logic in
    storage, which sets an expectation that all storage calls retry
    (they don't, and we don't want them to without buy-in).
  - Doesn't help the reconciler, the community-purge job, or any
    future background job — each would need to opt in.
  - Doesn't solve the on-call page problem on its own; even with retry,
    we still need a way to say "all 3 attempts failed → still don't
    page on the first occurrence."
- **Effort:** Medium (storage method change + tests for retry behavior).

### Option C: Tighten the Cloud Monitoring alert policy

**Approach:** Change `Server Error Logged - server` to require N
occurrences in a Y-minute window before firing (e.g. 3 occurrences in
5 minutes), and/or add a log-text exclusion for known-transient
patterns.

- **Pros:**
  - Zero code change.
  - Catches sustained issues; ignores singletons.
- **Cons:**
  - Couples alert quality to the GCP terraform/console, where it's
    invisible to code review and easy to misconfigure.
  - Doesn't fix the underlying conflation in code (transient and
    real errors are still both ERROR-level, which is wrong on its own
    and matters for any other consumer of these logs — e.g. the
    daily error summary at `docs/server/observability.md:457-481`,
    which would still flag transient resets as "errors requiring
    attention").
  - Exclusion-by-regex is brittle: a typo in the regex re-enables the
    page; a new transient pattern (PostgreSQL pool exhaustion, DNS
    blip) needs a regex update; the exclusion has to live in
    `terraform/observability/`.
- **Effort:** Small (terraform change), but pays low dividends.

## Recommendation

**Option A**, with these shape decisions:

1. **One shared package, `server/errs/`**, exporting both
   `IsTransient(err) bool` (extracted from
   `server/ai/errors.go::IsTransientError`) and a
   `TransientStreak` type whose `Classify(err) slog.Level` method is
   the entire surface area used at job call sites. The existing AI
   call sites continue to import the same predicate (via a thin
   `server/ai/errors.go` shim that forwards to `errs.IsTransient`, so
   no rename ripples through `server/ai`).
2. **One uniform 5-minute escalation policy**, encoded inside
   `TransientStreak` as a single constant. No per-job tunable
   thresholds, no per-job counter logic. A user-visible 5-minute
   downtime budget is the contract; any background-job goroutine
   that wants in just instantiates a `TransientStreak` and calls
   `Classify`. The streak resets on success or on a non-transient
   error.
3. **All five existing background-job call sites** (dispatcher,
   reconciler, community-purge, daily activity-digest, weekly
   activity-digest) move to the same helper. Adding a new
   background job in the future is one line — instantiate a
   `TransientStreak`, call `Classify(err)` instead of hard-coding
   `slog.LevelError`.

Why not Option B alone: the page is the symptom, not the bug. The bug
is "we log ERROR for something that doesn't require human action."
Adding retry inside the storage layer is the right answer for code
paths where the failure breaks a user-facing RPC (an in-flight
`SaveGear` should retry rather than 500), but the dispatcher's
"failure" is just "wait 60 seconds." Free retry comes built-in.

Why not Option C alone: the alert tightening should arguably happen
too (a "3 in 5 minutes" threshold on `Server Error Logged` is
generally a good idea), but it's a backstop, not the fix. Code-level
classification is the durable answer because it travels with the code
and is visible in PR review.

## Open Questions

- **Where should the shared helper live?** — Recommendation: a new
  `server/errs/` package containing `transient.go` (the predicate)
  and `streak.go` (the `TransientStreak` classifier), with a
  `doc.go`. Predicate substrings ("connection reset", "i/o timeout",
  "503", "deadline exceeded") apply equally to PostgreSQL drivers and
  AI HTTP clients, so co-locating both consumers behind one package
  is the right grouping. `server/ai/errors.go` becomes a thin shim
  that forwards to `errs.IsTransient` so the existing AI call sites
  don't need to change.

- **Why 5 minutes, not some other window?** — Recommendation: 5
  minutes is the operational SLO for "noticed in time to matter."
  It's longer than every tick interval in the dispatcher loop (60s)
  and the reconciler loop (10min — see below), short enough that a
  real DB outage pages on-call before user impact compounds, and
  matches the rough "3 occurrences in 5 minutes" backstop already
  proposed for Cloud Monitoring (Option C). Encoding it as a single
  exported constant `errs.TransientPageAfter = 5 * time.Minute` makes
  the operator-facing knob obvious.

- **The reconciler tick is 10 minutes — does the 5-minute window
  make sense for it?** — Yes, deliberately. The reconciler's *first*
  transient failure starts the clock; the *second* failure, at
  ~10 minutes later, is already past the 5-minute threshold and
  logs ERROR. That's the right behavior: the reconciler being broken
  for two consecutive runs is operationally a 20-minute outage of
  the reconciliation guarantee, which warrants a page. The
  asymmetry is a feature, not a bug — we want longer-tick jobs to
  page faster (in *number of failures*) because each failure
  represents more downtime.

- **Should `TransientStreak` be safe for concurrent use?** —
  Recommendation: no. Each instance is owned by exactly one
  goroutine (one background-job loop). Adding a mutex would complicate
  tests and signal an API that doesn't match the intended usage.
  Document the single-goroutine contract in the type's doc comment.

- **Should the storage layer itself retry, or only the background-job
  callers?** — Recommendation: only the background-job callers, for
  now. Wrapping all storage calls in retry logic changes latency
  characteristics for RPC handlers in subtle ways and risks masking
  pool-exhaustion bugs. Background jobs are the right scope because
  (a) they're not user-facing, so a 60-second wait is free, and (b)
  they're long-running goroutines with natural retry cadence already
  built in (the ticker).

- **Should we also tighten the Cloud Monitoring alert policy?** —
  Recommendation: yes, but as a follow-up issue, not part of this
  one. Code-level classification is the durable fix; the alert policy
  change is defense-in-depth and lives in a different repo
  (`terraform/`). File a tracker, link it from this plan, and don't
  block this work on it.

- **Do we want a metric for transient-error rate?** — Recommendation:
  no new metric for now. The WARN logs are queryable in Cloud Logging
  and the WARN→ERROR transition is itself the signal. If we see this
  class of error spike in practice, add a counter then.

## Implementation Plan

### Phase 1: Shared `server/errs/` package — predicate + classifier
**Goal:** One package owns both "is this transient?" and "should this
streak page yet?", reused by every consumer.
**Tasks:**
- [ ] Create `server/errs/` package with `doc.go`, `transient.go`
  (predicate), `streak.go` (`TransientStreak` type), and
  `transient_test.go` + `streak_test.go`.
- [ ] Move the body of `server/ai/errors.go::IsTransientError` into
  `errs.IsTransient`. Seed `transient_test.go` with the existing AI
  test inputs plus PostgreSQL-flavored ones: `"read tcp ...:
  connection reset by peer"`, `"write tcp ...: broken pipe"`,
  `"dial tcp ...: i/o timeout"`, `"context deadline exceeded"`.
- [ ] Implement `TransientStreak`:
  - Exported constant `TransientPageAfter = 5 * time.Minute`.
  - Constructor `NewTransientStreak() *TransientStreak`.
  - Method `Classify(err error) slog.Level` (only called when
    `err != nil`):
    - non-transient `err` → `slog.LevelError`; resets the streak as
      a side effect (a real error supersedes any in-progress
      transient streak).
    - transient `err`, first of streak → record `firstTransientAt =
      time.Now()`, return `slog.LevelWarn`.
    - transient `err`, ongoing streak, `time.Since(firstTransientAt)
      < TransientPageAfter` → `slog.LevelWarn`.
    - transient `err`, ongoing streak, threshold crossed →
      `slog.LevelError` (streak continues; subsequent transient
      errors keep returning ERROR until `Reset()` is called).
  - Method `Reset()` — clears `firstTransientAt`. Called on each
    successful run. Explicit and separate from `Classify` because
    success-path callers don't log; they only need to clear state.
  - Single-goroutine contract documented on the type's doc comment.
    No mutex; the type is owned by exactly one goroutine.
  - The struct holds a `now func() time.Time` defaulting to
    `time.Now`, overridable in tests.
- [ ] Implement `errs.LogJobError(ctx context.Context, logger
  *slog.Logger, streak *TransientStreak, msg string, err error,
  kv ...any)`:
  - If `err == nil`, calls `streak.Reset()` and returns. (No
    success-path log emission — that stays the caller's choice.)
  - Otherwise calls `logger.Log(ctx, streak.Classify(err), msg,
    append([]any{"error", err}, kv...)...)`.
  - This is the **only** function that background-job call sites
    need to import. Encapsulates the level decision and the
    standard `error` field naming
    (`docs/server/observability.md:112`).
- [ ] Update `server/ai/errors.go::IsTransientError` to forward to
  `errs.IsTransient`. Preserve the exported name so `server/ai`
  callers don't change.
- [ ] Create `server/errs/README.md` covering purpose (transient
  error classification + escalation), key files (`transient.go`,
  `streak.go`), and when to add code here vs. an adjacent package.
  Style model: `server/ai/README.md`.
- [ ] `npm run build` and `go test ./server/errs/... ./server/ai/...`
  pass.
**Validates:** Predicate handles both AI-provider and SQL-driver error
strings; streak escalates from WARN→ERROR at exactly 5 minutes; reset
behavior is correct on both success and non-transient errors; the
single-line `LogJobError` API is what call sites actually consume.

### Phase 2: Adopt `errs.LogJobError` at every background-job call site
**Goal:** Every long-running background goroutine routes its error log
through the shared classifier. Each call site is one line.
**Tasks:**
- [ ] At the dispatcher goroutine (`server/main.go:1256-1273`):
  instantiate `streak := errs.NewTransientStreak()` outside the
  ticker loop; inside `runDispatch`, replace the existing
  `logger.Error("scheduled-notification dispatch failed", "error",
  err)` with `errs.LogJobError(ctx, logger, streak,
  "scheduled-notification dispatch failed", err)`. On a successful
  tick (no error), call `streak.Reset()` so the next isolated
  transient starts a fresh streak.
- [ ] Same pattern at the reconciler goroutine
  (`server/main.go:1234-1254`). One `TransientStreak` per goroutine,
  shared across the inner per-reconciler loop so the streak spans
  all reconcilers in a tick.
- [ ] Same pattern at the community-purge goroutine
  (`server/main.go:~1160-1210`) for both the purge and
  purge-reminder calls. One shared streak per goroutine.
- [ ] Same pattern at the daily activity-digest goroutine
  (`server/main.go:~1288-1307`).
- [ ] Same pattern at the weekly activity-digest goroutine
  (`server/main.go:~1309+`).
- [ ] Confirm none of these sites introduce new goroutines — all
  modifications happen inside existing `logging.GoSafe` blocks, per
  the goroutine-safety rule at `docs/server/architecture.md:919`.
- [ ] Update `docs/server/observability.md` near the log-level
  definitions (`docs/server/observability.md:60-65`) to document the
  policy: transient errors in background jobs log WARN for up to
  `errs.TransientPageAfter` (5 minutes); sustained transient errors
  or any non-transient error log ERROR. WARN/ERROR semantics map
  directly onto the existing definitions there
  (WARN = "unexpected but handled", ERROR = "failures requiring
  attention"). Name `errs.TransientPageAfter` as the constant to
  change if the SLO shifts.
- [ ] `npm run build`; spot-check locally by stopping the dev
  Postgres mid-tick and confirming the dispatcher logs WARN through
  the first few ticks, then escalates to ERROR after 5 minutes; then
  restart Postgres and confirm the next successful tick clears the
  streak (the next isolated transient logs WARN again, not ERROR).
**Validates:** A single transient blip no longer pages on any
background job; a 5+ minute outage pages on every background job
uniformly. New background jobs adopt the policy with one constructor
call + one `errs.LogJobError` call.

### Phase 3: Backstop alert-policy tightening (tracker only)
**Goal:** Defense-in-depth at the Cloud Monitoring layer, tracked
separately so this issue can close.
**Tasks:**
- [ ] File a follow-up issue against `terraform/` (or whichever repo
  owns `Server Error Logged - server`) proposing a "3 occurrences in
  5 minutes" threshold and/or a log-text exclusion for known transient
  patterns. Cross-reference this plan and #2128.
- [ ] Link the new issue from the bottom of this plan under "Related
  Issues" once filed.
**Validates:** The alerting weakness is not lost even though we're
choosing not to fix it in this change set.

## Related Issues

- #625 — Scheduled notifications design (the system whose dispatcher
  surfaced this page). The original plan covered functional behavior
  but not operational alerting policy; this fills that gap.
- #1622 — Notification dispatcher deleted-guard. Sibling work in the
  same dispatcher; useful precedent for adding classification logic
  in the same code path.
