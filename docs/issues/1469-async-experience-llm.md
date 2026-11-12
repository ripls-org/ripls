# Issue #1469: Experience creation blocks on 2 sequential AI calls (~3s post-create wall time)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1469
**Filed:** 2026-04-27 | **Last activity:** 2026-04-27
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`Experience.SaveExperience` (in `server/services/experience/save.go`, the create branch lines 117–196) blocks the response on two sequential synchronous LLM calls before returning to the client:

1. `aiProvider.InferSocialAttributes` — refines `ImpactEstimate` with vulnerability/duration; on success runs a `storage.Update`.
2. `aiProvider.GenerateExperienceSuggestions` — produces category-specific suggestion chips; on success runs a second `storage.Update`.

Each call has a 30s timeout; in steady state both succeed but together add ~3 seconds of wall time between the synchronous DB insert and the response. The user sees that wait directly: the preview modal stays in a loading state until the response returns.

Both calls are explicitly best-effort — failure falls back to config defaults / empty suggestions, and there is already a lazy synchronous fallback in `ListExperienceNeedsAndContributions` (`needs_and_contributions.go:80-107`) that fills suggestions on first read if missing. Neither belongs on the create critical path.

The peer `Request` service already solved the same problem: `server/services/request/lifecycle.go:179-224` runs `GenerateRequestSuggestions` in a detached goroutine after the synchronous insert and returns immediately. `InferSocialAttributes` for requests runs at fulfillment, not creation. `Gear.SaveGear` makes no AI calls in the create path. Experience is the outlier.

## Context from Comments

One comment from collaborator `@briangbrown`: "@claude write a plan." That is the trigger for this document. No other discussion, clarifications, or pushback on the proposed fix.

The issue body itself is unusually detailed: it identifies the exact lines, ranks three fix options, names a recommendation (Option 1 — detached goroutine, mirroring Request), and lists acceptance criteria. Treat the issue body as the design brief.

## Potential Solutions

### Option A: Detached goroutines, mirroring `Request.SubmitRequest`
- **Approach:** After the synchronous insert + initial config-default `ImpactEstimate` is persisted, kick off the two LLM calls in the background using the same pattern as `request/lifecycle.go:184-223`:
  - `go func(...)` with `defer recover()` and a stack-trace log on panic
  - `detachedContextWithTimeout(ctx, 2*time.Minute)` to preserve `request_id`, `user_id`, etc., while severing parent cancellation
  - On success, reload the experience with `storage.GetByID` (to avoid clobbering concurrent updates), apply the new fields, and `storage.Update`.
  - Two separate goroutines so one slow call doesn't delay the other's persistence.
- **Pros:** Matches the existing pattern in the codebase, so the team already understands the failure modes (panic recovery, log-and-drop). Eliminates the full ~3s from user-visible latency. Lazy-fill safety net in `needs_and_contributions.go` already covers the edge case where the goroutine hasn't finished when the user opens the Plan tab. Each goroutine's persistence is independent, so a slow `InferSocialAttributes` doesn't delay a fast `GenerateExperienceSuggestions`.
- **Cons:** Two `storage.GetByID` + `storage.Update` round-trips per create instead of in-process struct mutation, because we need to re-read and merge to avoid clobbering. Tests that asserted suggestions land in the response synchronously must be rewritten to poll storage with a deadline. There is a small window where a client reading the experience immediately sees the config-default `ImpactEstimate` until inference finishes (~1–2s later) — acceptable per the issue.
- **Effort:** Medium.

### Option B: Parallelize the two calls with `errgroup`
- **Approach:** Run both LLM calls concurrently with `golang.org/x/sync/errgroup` (or a hand-rolled `sync.WaitGroup`) and a single shared 30s timeout. Cuts wall-clock from `t1 + t2` to `max(t1, t2)`.
- **Pros:** Smaller diff. Preserves the current contract that suggestions and refined impact estimate are present in the response. No test changes needed beyond ordering assumptions.
- **Cons:** User still blocks ~1.5s on whichever LLM is slower. Doesn't move the work off the critical path, so any future regression (slower model, retry, etc.) reappears as user-visible latency. Doesn't match the established Request pattern, so we'd carry two parallel-but-different idioms in the codebase.
- **Effort:** Small.

### Option C: Tighten the 30s timeout to ~5s
- **Approach:** Drop both `ai.CallWithTimeout(ctx, 30*stdtime.Second, …)` budgets to `5*stdtime.Second` so the worst-case create latency is bounded.
- **Pros:** Trivial change. Bounds tail latency.
- **Cons:** Doesn't fix the typical-case 3s wait — typical LLM latency is 1–2s per call, well under 5s. Increases the rate at which the LLM falls back to config defaults under transient slowness, since the 30s budget is currently absorbing slow runs.
- **Effort:** Trivial.

## Recommendation

**Option A.** It matches the existing `Request` pattern, fully addresses the user-visible latency named in the issue title, and has the safety net (lazy fill in `needs_and_contributions.go`) already in place. The cost — two additional `GetByID` + `Update` round-trips on creation — is paid in the background after the response and does not affect user-visible latency.

The trade-off the issue calls out is acceptable: clients reading the experience in the ~1–2s window after create see the config-default `ImpactEstimate` and empty suggestions; both are filled in shortly after. The preview modal already showed the AI-generated content during streaming, so the post-create polish is a quiet upgrade rather than a missing feature.

Option B is a tempting smaller fix but accepts permanent ~1.5s blocking on the critical path. Option C is a backstop, not a fix.

**Scope note:** server-only. No proto changes, no `app/lib/` changes. The Flutter client already handles empty `Suggestions` and a config-default `ImpactEstimate` correctly — the lazy-fill in `ListExperienceNeedsAndContributions` is the existing proof. The user-visible win is purely a faster response from `SaveExperience`.

## Open Questions

- **Should the two LLM calls share one goroutine or run in two parallel goroutines?** — Recommendation: **two parallel goroutines.** The two calls are independent (`InferSocialAttributes` updates `ImpactEstimate`; `GenerateExperienceSuggestions` updates `Suggestions` + `CategoryHint`). Running them in parallel halves background latency and makes each persist as soon as it's ready, so a fast suggestion result isn't stuck behind a slow inference. Each goroutine reloads the experience with `storage.GetByID` immediately before its `Update` to avoid the two writers clobbering each other's fields.
- **What detached-context timeout?** — Recommendation: **2 minutes**, mirroring `request/lifecycle.go:196`. Covers the 30s LLM budget plus DB round-trips with comfortable headroom; matches the existing convention.
- **How should tests verify async persistence?** — Recommendation: extend the existing AI mock with a "done" channel (or use `sync.WaitGroup`) so tests can deterministically wait for the goroutine to finish before asserting on stored state. Avoid `time.Sleep`-based polling — flaky in CI. The mock infrastructure in `server/ai/mock_provider.go` is the natural place to add a `WaitForCalls(n int, timeout time.Duration)` helper if one doesn't already exist.
- **Does the `update` branch (existing-experience path, `save.go:228-353`) also need this treatment?** — Recommendation: **out of scope.** The issue is specifically about creation latency. The update branch only re-runs `InferSocialAttributes` when `textChanged`, which is rare; addressing it would require its own design conversation about whether updates should optimistically return the unchanged estimate. Park as a follow-up if metrics show update latency is also a problem.
- **Should we delete the now-redundant `ai.CallWithTimeout(ctx, 30*stdtime.Second, …)` wrapper from the create path now that the goroutine has its own 2-minute detached context?** — Recommendation: **keep an LLM-specific timeout inside the goroutine** (e.g. 30s as a `context.WithTimeout` inside the detached context). The 2-minute budget is the outer cap for total goroutine work including reload/update; the inner 30s is the LLM-call timeout that decides when to give up and log. This mirrors the layered-timeout convention in `request/lifecycle.go:196-199` (where the `GenerateRequestSuggestions` call inherits the 2-min context) but is slightly tighter because we have two LLM calls per goroutine.
- **Should `detachedContextWithTimeout` be hoisted to a shared package now that it'll have two callers?** — Recommendation: **defer.** The architecture doc (`docs/server/architecture.md` §"Shared Functionality in Libraries") says shared functionality across services belongs in a library, and that's the right end state. But the helper is 6 lines and the cost of an extra refactor PR vs. one focused latency fix isn't worth bundling. The plan is to **copy** for now and **file a follow-up** to hoist (e.g. to `server/services/internal/async.go` or as a method on a future `async` package) once a third caller appears or a reviewer prefers the lift.

## Implementation Plan

### Phase 1: Extract the create-path async work

**Goal:** Move both LLM calls off the critical path of `SaveExperience` (create branch only) without changing storage schema or external behavior beyond the documented latency improvement.

**Tasks:**
- [ ] Add a package-level `detachedContextWithTimeout` helper in `server/services/experience/save.go` (or a new `server/services/experience/async.go`) — copy verbatim from `server/services/request/lifecycle.go:24-31`. (If the team would prefer hoisting to a shared location like `server/services/internal/async.go`, raise that as a separate refactor PR — don't bundle it.)
- [ ] In `save.go`, replace lines 117–196 (the two synchronous LLM blocks in the create branch) with two `go func(...)` invocations launched **after** the watch upsert and **before** the response is built. Each goroutine:
  - Defers a `recover()` that logs the panic with `request_id`, `experience_id`, and `string(debug.Stack())` (mirror `request/lifecycle.go:155-165`).
  - Calls `detachedContextWithTimeout(ctx, 2*time.Minute)` and defers cancel.
  - Builds an async logger via `logging.LoggerWithContext(asyncCtx).With("experience_id", id, "operation", "<op-name>")` where `<op-name>` is `InferSocialAttributes` or `GenerateExperienceSuggestions`. Standard observability fields (`request_id`, `user_id`) propagate automatically through the detached context per `docs/server/observability.md`.
  - Calls the LLM with an inner `context.WithTimeout(asyncCtx, 30*time.Second)` so the LLM-call deadline stays the same as today. On a deadline-exceeded error specifically, log at `Warn` with `external_service: "ai_provider"`, `operation`, `duration_ms`, `error` (preserve today's structured-logging keys from `save.go:128-132` and `save.go:175-179`).
  - On success, reloads the experience with `storage.GetByID`, applies the new fields (`ImpactEstimate` for inference goroutine; `Suggestions` + `CategoryHint` for suggestion goroutine), and calls `storage.Update`. Reload-then-write avoids the inference goroutine clobbering suggestions written by the other goroutine, and vice versa.
  - On failure, logs at `Warn` and returns. No retry.
- [ ] Capture by-value into the goroutine: `id`, `expStored.Name`, `expStored.Description`, `expStored.Category`, `valueUSD`, `req.Msg.SocialContext` (cloned), and the user-set duration extracted via `extractExperienceDurationMinutes(expStored)`. Do **not** capture the `expStored` pointer or `ctx` directly — they become stale or canceled before the goroutine runs.
- [ ] Preserve the gating conditions: skip both goroutines when `clock.IsSimulated(ctx)`, when `s.aiProvider == nil`, or when both `Name` and `Description` are empty. The inference goroutine additionally requires `s.estimatorCfg != nil` (matches today's `save.go:120` guard).
- [ ] Leave the synchronous initial-`ImpactEstimate` block (`save.go:101-107`) untouched: the config-default impact estimate is what the response returns, and what clients see during the brief async window.
- [ ] Leave the **update** branch (`save.go:228-353`) untouched in this issue.

**Validates:** After this phase, a manual create of an experience via the dev server should return in ~50–100ms instead of ~3s, and a `GetExperience` call ~2s later should show the LLM-refined `ImpactEstimate` and the `Suggestions` slice populated.

### Phase 2: Test alignment

**Goal:** Make the existing test suite green and add coverage for the new async behavior.

**Tasks:**
- [ ] **Update** `TestSaveExperience_SuggestionsGeneratedOnCreate` in `server/services/experience/save_test.go:604-648`:
  - The response will no longer contain suggestions synchronously. Replace the assertions on `resp.Msg.Experience.Suggestions` with a deterministic wait on the AI mock (channel signal or counter) followed by a `storage.GetByID` re-read of the persisted experience.
  - Keep the assertion that the persisted `Suggestions` slice has 3 entries and `CategoryHint == "outdoor hike"` — that is the durable contract.
- [ ] **Update** `TestSaveExperience_SuggestionFailureDoesNotBlockCreate` (`save_test.go:652-680`): the test already only asserts that the response succeeds and that suggestions are empty in the response. Both still hold. Add a deterministic wait so the test doesn't race with the goroutine writing a warning log into the test logger after `t.Cleanup` runs — that produces flaky `panic: Log in goroutine after test has completed` failures.
- [ ] **Update** `TestSaveExperience_LLMFallbackOnFailure` in `server/services/experience/qt_test.go:133-156`: same shape — wait for the inference goroutine to settle so `service.SetAIProvider` cleanup doesn't race with an in-flight call.
- [ ] **Verify untouched**: `TestSaveExperience_UsesUserDurationAtCreation` (`qt_test.go:35-74`) and `TestSaveExperience_RecalculatesQTOnUpdate` (`qt_test.go:78-128`) — neither configures a mock AI provider, so the inference goroutine is a no-op for them. The user-set 90-minute / 180-minute duration still flows through the synchronous `BuildExperienceImpactMetrics` path.
- [ ] **Verify untouched**: tests in `server/services/experience/needs_and_contributions_test.go` that exercise the lazy-fill safety net — the lazy fill remains correct as a fallback.
- [ ] **Add** `TestSaveExperience_SuggestionsAndImpactPersistedAsync` (new): a single end-to-end test that creates an experience with a mock AI provider that returns both inference and suggestions, waits for both goroutines via a `sync.WaitGroup` exposed by the mock, and asserts that `storage.GetByID` reflects both the LLM-refined `ImpactEstimate` and the `Suggestions` slice. Pin the assertion to the persisted state, not the response.
- [ ] **Helper**: if the existing AI mock in `server/ai/mock_provider.go` doesn't already support deterministic completion signaling, add a minimal helper there (e.g. a `Done()` channel that closes after each method returns). Keep the helper additive — don't change the existing `Func` field signatures.

**Validates:** `go test -tags=integration -timeout 20m ./server/services/experience/...` passes locally and in CI without flakes across at least 5 consecutive runs (`go test -count=5`).

### Phase 3: Verification & docs

**Goal:** Confirm the change behaves as intended end-to-end and document the new contract.

**Tasks:**
- [ ] Run `npm run build` to clear `golangci-lint` (gofumpt, godot, revive) and the regular Go build.
- [ ] Run `go build -tags=integration ./...` to catch any signature drift in integration-tagged tests.
- [ ] Manual smoke test on local dev: create an experience via the Flutter app or `curl`, verify the response returns in <200ms, and verify a follow-up `GetExperience` ~2s later shows refined `ImpactEstimate` and populated `Suggestions`.
- [ ] Add a one-paragraph note to `server/services/experience/README.md` explaining that `SaveExperience` (create branch) returns immediately and that LLM-derived fields (`Suggestions`, `CategoryHint`, refined `ImpactEstimate`) are filled in asynchronously, with the lazy-fill in `ListExperienceNeedsAndContributions` as the safety net. Reference the same pattern in `request/lifecycle.go` so future readers can find precedent.
- [ ] No proto changes.
- [ ] No client changes — the client already tolerates an empty `Suggestions` slice (the lazy-fill path proves that) and the `ImpactEstimate` is read from a single field so the eventual upgrade is invisible to UI code.

**Validates:** Issue acceptance criteria met:
1. `SaveExperience` returns immediately after the synchronous DB insert and impact-default population. ✓
2. `InferSocialAttributes` and `GenerateExperienceSuggestions` run in detached goroutines with panic recovery. ✓
3. Each goroutine persists its result via `storage.Update`. ✓
4. No regression in tests; existing impact-estimator tests already tolerate config-default fallback. ✓

## Related Issues

- **No GitHub-linked predecessors** are referenced from #1469 itself. The relevant historical context is the request-side migration — `server/services/request/lifecycle.go:184-223` is the established pattern this plan mirrors.
- **Follow-ups to file (out of scope here):**
  - Apply the same async pattern to the `SaveExperience` **update** branch (`save.go:228-353`) when `textChanged` is true.
  - Hoist `detachedContextWithTimeout` to a shared location (e.g. `server/services/internal/async.go`) so request, experience, and any future caller share one implementation. See `docs/server/architecture.md` §"Shared Functionality in Libraries".

## Audit notes (2026-04-27)

Plan reviewed against `docs/server/architecture.md` and `docs/client/architecture.md`. Key checks:

- **Service independence (server arch §1):** ✓ no service-to-service calls introduced.
- **Shared functionality in libraries (server arch §2):** flagged — the plan duplicates a 6-line helper rather than hoisting it. Justified for now, with a follow-up filed in *Related Issues* above.
- **Short, focused files (server arch §3):** ✓ — `save.go` net change is ≤ 0 lines; option to factor goroutines into `async.go` if reviewer prefers.
- **Test coverage (server arch §4):** ✓ — Phase 2 covers updated and added tests; calls out integration-tag impact.
- **Observability (server arch §observability):** ✓ — explicit log fields (`operation`, `external_service`, `duration_ms`, `experience_id`) and panic stack-trace; consistent with current `save.go` and `request/lifecycle.go`.
- **Server/client boundary:** ✓ — server-only change. Client already tolerates empty `Suggestions` and config-default `ImpactEstimate`.
- **Storage layer (server arch §storage):** ✓ — uses `storage.GetByID` + `storage.Update`; no raw SQL; reload-then-write protects against the two goroutines clobbering one another.
