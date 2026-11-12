# Issue #1237: Integration tests wedge for minutes when LLM calls hang in RPC handlers

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1237
**Filed:** 2026-04-23 | **Last activity:** 2026-04-23
**Labels:** bug, P1
**Assignees:** unassigned
**Context docs:** none

> **Audit notes (2026-04-23):** Reviewed against `docs/server/architecture.md` and `CLAUDE.md` (client docs don't apply — no client changes). Four compliance refinements applied:
> 1. `server/ai/timeout.go` placement validated against the "Shared Functionality in Libraries" principle — `server/ai/` is the correct home because every call site lives outside that package and uses the `ai.Provider` interface.
> 2. Phase 2 logging guidance now explicitly enumerates the standard field names (`external_service`, `provider`, `operation`, `duration_ms`, `error`) from `CLAUDE.md`'s structured-logging section, and mandates `logging.LoggerWithContext(ctx)` as the entry point at each wrapped site.
> 3. Added an explicit "WARN is correct here, not a downgrade" note that reconciles the existing best-effort pattern with the user-memory guidance `feedback_keep_error_logs.md` (don't silence ERROR-level client/data bugs). Timeouts at an external-service boundary are the documented-ignored case that `CLAUDE.md`'s Fail-Fast rule explicitly allows.
> 4. `CallWithTimeout` doc comment spec added so it follows the Go convention `CallWithTimeout runs fn with a context bounded to timeout...` (function name + imperative sentence, per `CLAUDE.md`'s Go Code Comments rule).

## Problem Statement

Several RPC handlers call AI providers (`InferSocialAttributes`, `GenerateExperienceSuggestions`, `GenerateConversationSummary`, `CreateStory`) synchronously with the request's raw `ctx` — no `context.WithTimeout`, no deadline, no fallback. When an upstream LLM stalls, the RPC hangs for minutes, exhausts the 20-minute package test timeout, and takes down the shared Postgres testcontainer.

The issue cites nine CI runs between 2026-04-15 and 2026-04-22 where a single hung call consumed 5-10 minutes. Stack traces show the test client parked in `net/http.(*persistConn).roundTrip` while the server handler sits unresponsive waiting on an LLM that never responds.

The failure pattern is asymmetric: one test's hang can wedge subsequent tests in the same package (see run 24526126709, where `TestLoan_Example5_MultipleLoansCumulativeImpact` ran 598s and then *panicked* on a nil `ImpactEstimate` left behind by the earlier hang). Tests that complete successfully in 30s can be forced to 600s by a single upstream stall.

The pattern for the fix already exists in the codebase — `server/services/experience/needs_and_contributions.go:87` wraps a `GenerateExperienceSuggestions` call in `context.WithTimeout(ctx, 5*time.Second)`, and `server/services/request/lifecycle.go:22-29` defines `detachedContextWithTimeout`. The hot paths just don't use it.

## Context from Comments

No comments on the issue. The issue author has filed a detailed, concrete diagnosis with specific file/line references, nine reproducing CI runs, a proposed fix ranked against two alternatives, and a stated recommendation (Option 1 as same-day, Option 3 as follow-up). Treat the issue body as complete design context.

## Potential Solutions

### Option A: Bounded per-call `context.WithTimeout` at RPC-level call sites
- **Approach:** Wrap each RPC-level LLM call in `context.WithTimeout(ctx, 30*time.Second)` (or 15s for snappy paths, 30s where an end-user is actively waiting). On `context.DeadlineExceeded`, log a warning and fall through to the existing "LLM failed, use config defaults" error path — which already exists around every one of these call sites.
- **Pros:** Smallest diff with highest signal. Preserves existing graceful-fallback semantics (all affected sites already say `logger.WarnContext(ctx, "...failed, using config defaults", "error", err)` and continue). Completely contained to the RPC layer — no changes to providers, no changes to background jobs. Easiest to review and revert.
- **Cons:** Duplicates the `context.WithTimeout` boilerplate at each call site (5-8 sites). Each new LLM call site in the future would need to remember to add the wrap. Mitigate with a tiny `callAIWithTimeout(ctx, timeout, fn)` helper in a shared place (e.g., `server/ai/timeout.go`), but even without it the pattern is 2 lines and obvious.
- **Effort:** Small (~1 day).

### Option B: Push timeouts into the providers themselves
- **Approach:** Modify `provider_anthropic.go`, `provider_openai.go`, `provider_gemini.go` so every generation method applies a default per-call `context.WithTimeout` (e.g., 30s) before making the HTTP call. Optionally make the timeout configurable via provider config.
- **Pros:** Every caller gets a bounded deadline for free — no risk of a new call site forgetting the wrap. Cleanest long-term.
- **Cons:** Wider blast radius. Streaming RPCs (`GenerateExperienceFromTextStreaming`, `GenerateGearFromTextStreaming`) legitimately need longer budgets; a blanket provider-level timeout would need per-method overrides. Background jobs (`server/jobs/impact_backfill.go`) may also want different budgets than RPC handlers. Easier to break something than Option A.
- **Effort:** Medium (~2-3 days, mostly auditing call sites to pick the right timeout per method).

### Option C: Push the LLM calls into background goroutines via `detachedContextWithTimeout`
- **Approach:** Make the affected RPCs return immediately after persisting the config-default `ImpactEstimate`, then refine asynchronously in a goroutine using `detachedContextWithTimeout`. A later `GetExperience`/`GetRequest`/`GetTransfer` call would return the refined estimate.
- **Pros:** Best UX — RPC responses become sub-second regardless of LLM latency.
- **Cons:** Largest behavior change. The RPC response no longer carries the LLM-refined impact estimate, so clients that currently display the refined value at completion time need a re-fetch (or a push). The `ImpactEstimate`-over-time state machine becomes observable in ways tests currently assume against (see the nil-deref panic in `loan_test.go:894` — it's already fragile with sync refinement; going async without care would make it worse). Broader semantic change; deserves its own design doc.
- **Effort:** Large (~1 week, with care around async observability and test postconditions).

## Recommendation

**Option A.** It matches the issue author's recommendation and — crucially — it's the narrowest change that eliminates the entire P1 CI-flake class. Every one of the listed call sites is already written as "best effort, log-and-continue on error," so adding a deadline that triggers the same error path is behavior-preserving for success cases and behavior-improving for pathological cases.

Ship A as a same-day fix. Revisit C as a follow-up *only* if sub-second RPC latencies on completion become a UX goal beyond test stability. Option B is tempting but fights the grain of how streaming RPCs currently work and requires per-method timeout audits — the ROI vs. Option A's two lines per call site isn't there.

Add a tiny helper to DRY the pattern and make it hard to forget. Placement follows the architecture's "Shared Functionality in Libraries" principle: `server/ai/` is the correct home because every caller lives outside that package and already depends on the `ai.Provider` interface from there.

```go
// server/ai/timeout.go

// CallWithTimeout runs fn against a derived context bounded by timeout and
// returns fn's result. The derived context is always cancelled before return,
// so callers do not need to manage cancellation themselves. Intended for
// RPC-level AI calls where the surrounding handler already treats provider
// errors as best-effort (log and fall back to config defaults).
func CallWithTimeout[T any](ctx context.Context, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    return fn(ctx)
}
```

(Doc comment follows `CLAUDE.md`'s Go Code Comments rule — starts with the function name and an imperative sentence.)

Each call site becomes a single wrapped line. Future call sites get the pattern for free.

**Why WARN, not ERROR, is the correct log level at each wrapped site.** The user-memory guidance `feedback_keep_error_logs.md` says not to downgrade ERROR logs to silence alerts for real bugs. That rule targets client/data bugs — not external-service bounded timeouts. An LLM upstream exceeding its deadline is the *explicitly documented* case `CLAUDE.md`'s Fail-Fast principle allows: "Have an explicit comment explaining why it's safe to ignore" / "Be part of a documented fallback/retry strategy." Every one of these call sites already has the fallback strategy in place (fall through to `BuildExperienceImpactMetrics` / `BuildRequestImpactMetrics` / `BuildTransferImpactMetrics` with config defaults). We are preserving that pattern and making it bounded, not downgrading anything that was previously ERROR. If the timeout rate observed in prod (see Phase 4) starts to correlate with a code defect rather than upstream availability, that's when it graduates to ERROR.

## Open Questions

- **What timeout value(s)?** — Recommendation: **30s** for `InferSocialAttributes` and `GenerateExperienceSuggestions` (user-facing refinement at completion time — they're already async from the user's perspective, but still block the RPC response). **60s** for `CreateStory` because story generation currently fans out multiple LLM calls internally. Match the existing 5s timeout in `needs_and_contributions.go` for the lazy-suggestion path since that one is already on the user's critical path for the experience-view render. Re-examine if any legitimate successful call is observed exceeding these bounds.
- **Which error code on deadline?** — Recommendation: do nothing special. The existing call sites already handle `err != nil` by logging and continuing with the config default. `context.DeadlineExceeded` flows through that same branch. No new error handling needed at the call site; the fallback is already there.
- **Should we `errors.Is(err, context.DeadlineExceeded)` and log differently?** — Recommendation: yes — log at `WarnContext` with a distinct message like `"LLM call exceeded timeout budget, using config defaults"` so we can grep for it in prod logs and track whether 30s/60s are the right numbers. Keep the main "LLM social attribute inference failed" log as-is for non-timeout errors so the two failure modes are distinguishable.
- **Should this also cover `server/services/community/invitations.go:420` (`storyCreator.CreateStory`) and other `CreateStory` call sites?** — Recommendation: yes, audit and wrap every `storyCreator.CreateStory` call with the same pattern. They're all on RPC paths and all best-effort (log-and-continue on error). Include in scope of this change.
- **What about call sites that already have timeouts (e.g. `needs_and_contributions.go:87` uses 5s, and `lifecycle.go:194` uses `detachedContextWithTimeout(2*time.Minute)`)?** — Recommendation: leave them. 5s is intentionally tight for the lazy-suggestion path (it's on the view-experience render critical path). The `detachedContextWithTimeout` site is already fully async. Only wrap call sites that currently have no deadline.
- **Where should the helper live?** — Recommendation: `server/ai/timeout.go`. Keeps it colocated with the provider interface so every future AI call site naturally discovers it. Alternative: `server/services/internal/aitimeout` — not needed since the helper is a generic 5-line function. Put it in `ai/`.
- **Do we need an integration test for timeout behavior?** — Recommendation: a unit test with a fake provider that sleeps forever is enough. Integration-testing a real LLM's slow path is brittle. One unit test per wrapped call site is overkill; one unit test for `CallWithTimeout` plus a "does-it-compile" sweep is sufficient, since all the call sites are structurally identical.
- **Could the `detachedContextWithTimeout` function in `request/lifecycle.go:22-29` be promoted and shared?** — Recommendation: out of scope for this issue; it's a separate abstraction (detaches from parent cancellation, useful for goroutines continuing after RPC return). Option A uses the simpler in-request `context.WithTimeout`, not the detached variant.

## Implementation Plan

### Phase 1: Add the timeout helper
**Goal:** Establish the pattern so every wrapped call site is a one-liner.
**Tasks:**
- [ ] Create `server/ai/timeout.go` with the generic `CallWithTimeout[T any]` helper described above. Include a package doc comment describing when to use it (RPC-level AI calls) vs. when not to (streaming RPCs with already-large user-facing budgets, background jobs that manage their own deadlines).
- [ ] Add `server/ai/timeout_test.go` covering: (a) fast success passes through, (b) slow call gets `context.DeadlineExceeded`, (c) parent context cancellation propagates.
- [ ] Run `npm run build` to confirm lint passes.

**Validates:** Helper works in isolation. Clear pattern established for callers.

### Phase 2: Wrap the RPC-level AI calls listed in the issue
**Goal:** Every RPC handler call into `aiProvider.*` or `storyCreator.CreateStory` has a bounded deadline.
**Tasks:**
- [ ] `server/services/experience/lifecycle.go:214` — wrap `InferSocialAttributes` with 30s.
- [ ] `server/services/experience/lifecycle.go:673` — wrap `CreateStory` with 60s.
- [ ] `server/services/experience/save.go:119` — wrap `InferSocialAttributes` with 30s.
- [ ] `server/services/experience/save.go:156` — wrap `GenerateExperienceSuggestions` with 30s.
- [ ] `server/services/experience/save.go:295` — wrap `InferSocialAttributes` with 30s.
- [ ] `server/services/experience/summary.go:66` — wrap `GenerateExperienceCompletionSummary` with 30s.
- [ ] `server/services/request/lifecycle.go:390` — wrap `InferSocialAttributes` with 30s.
- [ ] `server/services/request/lifecycle.go:814` — wrap `CreateStory` with 60s.
- [ ] `server/services/request/gen.go:488` — wrap `GenerateConversationSummary` with 30s.
- [ ] `server/services/request/contributions.go:278` — wrap `GenerateRequestSuggestions` with 30s.
- [ ] `server/services/transfer/lifecycle.go:411` — wrap `InferSocialAttributes` with 30s.
- [ ] `server/services/transfer/lifecycle.go:750` — wrap `CreateStory` with 60s.
- [ ] `server/services/chat/summary.go:116` — wrap `GenerateConversationSummary` with 30s.
- [ ] `server/services/feed/nudges.go:97` — wrap `GenerateNudgeContent` with 30s.
- [ ] `server/services/community/invitations.go:420` — wrap `CreateStory` with 60s.
- [ ] For each wrap, when the error path fires, use `logging.LoggerWithContext(ctx)` (not `logger` captured outside the scope, not `logging.Default()`) and distinguish `errors.Is(err, context.DeadlineExceeded)` in the log message so we can grep for timeout hits in prod. Populate the standard fields listed in `CLAUDE.md`'s structured-logging section: `external_service` (the provider category, e.g. `"ai_provider"`), `provider` (`primary.Name()` if we can surface it, else omit), `operation` (the method being called, e.g. `"InferSocialAttributes"`), `duration_ms` (time since wrap start), and `error`. Keep the existing non-timeout warn log as-is so the two branches are distinguishable when searching logs.
- [ ] Do **not** wrap streaming RPCs (`*Streaming` methods), `convert_informal_time.go` (caller-driven timeout semantics), or the already-wrapped `needs_and_contributions.go:87` — the issue's scope is best-effort post-action refinement paths that currently have no deadline.
- [ ] Run `npm run build` — confirm every wrapped site compiles and lints.
- [ ] Run `go test ./server/...` (no integration tag) to confirm no unit test regressions.

**Validates:** Every RPC handler call into the AI layer returns within its timeout budget or flows through the existing fallback path. The 20-minute package test timeouts cited in the issue can no longer be caused by any of these call sites.

### Phase 3: Reproduce the hang locally and confirm the fix
**Goal:** Prove the fix closes the P1 flake class, not just that it compiles.
**Tasks:**
- [ ] Add a temporary test-only fake provider that `sleeps(10*time.Minute)` inside `InferSocialAttributes`. Wire it into one of the flaking tests (`TestEndToEnd_CompleteLoanLifecycle` is a good target — it's short and already in the issue's evidence list). Confirm that without the timeout fix the test hangs >30s, and with the fix the test completes in ~30s with the expected config-default `ImpactEstimate`.
- [ ] Revert the fake provider and sleep hack before committing.
- [ ] Run `go test -tags=integration ./server/services/experience/... ./server/services/request/... ./server/services/transfer/... ./server/services/chat/...` end-to-end to confirm no integration regression.

**Validates:** The change actually fixes the observed hang, not just the theory. A deliberate upstream stall is bounded at 30-60s per call rather than consuming the package timeout.

### Phase 4: Metrics and follow-up
**Goal:** Make it visible in prod when the timeout fires so we can tune the numbers.
**Tasks:**
- [ ] In the timeout-branch logging, ensure `external_service` / `provider` / `operation` / `duration_ms` fields are populated (per `CLAUDE.md`'s structured logging section).
- [ ] After the change ships to dev, inspect logs for a few days for `"LLM call exceeded timeout budget"` occurrences. If the rate is non-trivial on specific operations, open a follow-up to tune the per-operation timeout or pursue Option C for that specific call site.
- [ ] File a follow-up tracking issue referencing Option C (async refinement) as a potential future improvement if RPC-latency UX becomes a goal.

**Validates:** We know when timeouts fire in prod and can iterate on the values without guessing.

## Related Issues
- #1236 — Companion flake. #1236 addresses provider-selection drift in one specific integration test; #1237 (this one) addresses the structural cause of multi-minute hangs across many tests. Different fixes, related CI pain. Either can ship first; ideally #1237 lands first because it eliminates the root-cause pattern rather than one symptom.
- The "companion issue on pg container death" mentioned in the issue body — once hangs are bounded at 30-60s per call, pressure on the shared testcontainer should drop significantly, but isn't fully addressed by this change.
- PR #1232 CI run 24814282085 — the PR where #1236 was observed; also listed in #1237's evidence.
