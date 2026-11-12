# Issue #1236: Chat summary integration test hardcodes Gemini, causes flaky CI on Vertex 429

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1236
**Filed:** 2026-04-23 | **Last activity:** 2026-04-23
**Labels:** (none) | **Assignees:** maintainer
**Context docs:** none

> **Audit notes (2026-04-23):** Reviewed against `docs/server/architecture.md` (client docs don't apply — no client changes). Three compliance fixes applied:
> 1. Phase 1 helper location now explicitly requires `//go:build integration` as the first line of any new helper file so it only compiles under the integration tag (matches the existing `summary_integration_test.go`).
> 2. Helper logging uses `t.Logf` rather than `logging.Default()` — architecture's structured-logging guidance is for RPC handlers, not tests; `t.Logf` surfaces under `go test -v` which is the useful place for this diagnostic.
> 3. Added explicit open-question discussion of how the 30s per-call budget interacts with the `FallbackProvider` chain composed by `NewAllProvidersWithFallback` — confirmed it's fine for fast-return 429s (which is the actual failure mode targeted), and cross-referenced to #1237 where slow/hung upstream handling belongs.

## Problem Statement

`TestGenConversationSummary_Integration` and `TestGenConversationSummary_Integration_MultipleParticipants` in `server/services/chat/summary_integration_test.go` hard-wire the Gemini provider via Vertex AI:

```go
aiProvider, err := ai.NewGeminiProvider(ctx, "", vertexProject, "us-central1")
service.SetAIProvider(aiProvider)
```

Two problems stem from this:

1. **Architectural drift from prod.** Production uses `ai.NewAllProvidersWithFallback` (`server/main.go:290`) with Anthropic Haiku as primary and Gemini/OpenAI as fallbacks. The integration test does not exercise that path, so it no longer reflects the configuration prod ships.
2. **CI flake.** When Vertex's per-project Gemini quota is exhausted, the test burns its full ~600s timeout retrying 429 `RESOURCE_EXHAUSTED` errors and fails — unrelated to the PR under test. Seen on PR #1232 CI run 24814282085.

A single-provider test has a single point of failure; a multi-provider test (matching prod) survives any one provider being rate-limited.

## Context from Comments

No comments on the issue. The issue author (also the assignee) has already framed the problem, cited the failing run, and ranked three fix options. Treat the filed body as the complete design context.

## Potential Solutions

### Option A: Match production via `NewAllProvidersWithFallback`
- **Approach:** Replace the direct `ai.NewGeminiProvider(...)` calls with `ai.NewAllProvidersWithFallback(ctx, ai.AllProvidersConfig{...})`, reading the same env vars `initializeAIProvider` reads: `VERTEX_AI_PROJECT`, `VERTEX_AI_LOCATION`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, and the model override knobs. Skip the test if *no* provider is configured (rather than gating solely on `VERTEX_AI_PROJECT`). Extract a helper (e.g. `newIntegrationAIProvider(t, ctx) ai.Provider`) and share it across both integration tests in the file.
- **Pros:** Test drives the exact code path prod runs, so the integration test starts catching issues in the fallback plumbing, not just Gemini. Surviving one provider being quota-throttled is built in — Haiku is primary, Gemini is fallback, OpenAI is tertiary. Removes an implicit Gemini-only assumption from the test.
- **Cons:** Slightly more setup to read multiple env vars. CI needs at least one AI provider env var set (already true — Anthropic runs in `gen_ai_streaming_integration_test.go` and elsewhere). Test assertions that assumed Gemini-style output may need a loosening pass — but existing assertions already note "relaxed checks since AI output varies," so this should be low-risk.
- **Effort:** Small.

### Option B: Switch primary to Anthropic, keep a local fallback chain in the test
- **Approach:** Still build the provider manually in the test (not via `NewAllProvidersWithFallback`), but use `NewFallbackProvider(anthropic, []Provider{gemini})` so Anthropic leads and Gemini takes over on failures. Skip if neither is configured.
- **Pros:** Works around the CI flake without touching `main.go`'s provider wiring. Easier to mentally reason about inside the test file.
- **Cons:** Still architecturally drifts from prod — we'd be maintaining a second, similar provider-composition path in the test package. Every future provider change would need to be mirrored in two places. Doesn't exercise `NewAllProvidersWithFallback` at all in integration tests.
- **Effort:** Small.

### Option C: Shorter per-call timeout + treat 429 as `t.Skip`
- **Approach:** Wrap the `GenConversationSummary` call in a `context.WithTimeout(ctx, 30*time.Second)`. If the returned error is a quota/rate-limit error (HTTP 429, `RESOURCE_EXHAUSTED`), call `t.Skip`. Otherwise fail as today.
- **Pros:** Minimal diff. Fixes the symptom.
- **Cons:** Doesn't address the architectural drift called out in the issue. A skip-on-429 masks real regressions (e.g., a genuine retry bug). Still pinned to Gemini. Two failure modes papered over with one band-aid.
- **Effort:** Tiny.

## Recommendation

**Option A + the defensive pieces of Option C.** Use `NewAllProvidersWithFallback` as the primary fix, and additionally:

1. Wrap the `GenConversationSummary` call in `context.WithTimeout(ctx, 30*time.Second)` so any single LLM call can't consume the whole 10-minute test timeout.
2. If the call returns a rate-limit / quota error after the fallback chain has had its shot (string-match on "429", "rate limit", "resource exhausted", "quota" — the same pattern `server/ai/test_helpers_test.go:29-32` already uses), call `t.Skip` rather than `t.Fatal`.

Rationale for combining:
- Option A alone satisfies the issue's three acceptance criteria: drives the prod code path, survives a single-provider 429, still exercises real AI when quota is available.
- Adding the 30s timeout bounds the worst case even if the fallback chain itself stalls (e.g., both Anthropic and Gemini slow to respond for unrelated reasons). Without this, a network blip could still consume minutes before failing.
- Treating a post-fallback 429 as `t.Skip` rather than `t.Fatal` means that if *every* configured provider is quota-exhausted at once (rare, but possible on CI accounts under load), we get a yellow skip rather than a red fail. A skip is honest: we couldn't run the test, it's not a code defect.

Option B leaves a lurking drift problem that will resurface the next time provider wiring changes. Plain Option C ranks the test's symptoms over its purpose, but the timeout + skip machinery is still useful as belt-and-suspenders on top of A.

Factor the provider construction into a small `testProviderFromEnv(t, ctx)` helper, and factor the run-with-timeout-and-quota-skip into a `runWithAIQuotaSkip` helper, so the second test (`TestGenConversationSummary_Integration_MultipleParticipants`) stops duplicating both construction and error-handling boilerplate.

## Open Questions

- **Which env vars should the helper require as a minimum skip condition?** — Recommendation: skip the test if *none* of `VERTEX_AI_PROJECT`, `ANTHROPIC_API_KEY`, or `OPENAI_API_KEY` is set. Log which providers ended up configured via `t.Logf` (not `logging.Default()`) so the output surfaces under `go test -v` rather than getting routed through slog JSON. This mirrors the "only providers with API keys set will be included" behavior of `NewAllProvidersWithFallback` and preserves "still exercises real AI when quota is available."
- **Should the helper live in this test file or somewhere shareable?** — Recommendation: start local (`summary_integration_test.go` helper) to avoid cross-package coupling. If a second integration test in a different package needs the same wiring, promote to `server/ai/testutil` or similar at that point. YAGNI for now — the other integration tests that already use real providers (`impact_backfill_integration_test.go`, `gen_ai_integration_test.go`) each pick their own provider mix intentionally.
- **Should the hardcoded model strings in the existing test move to match `NewAllProvidersWithFallback` defaults?** — Recommendation: yes, but implicitly — by not passing model overrides in the helper, the test picks up whatever `NewAllProvidersWithFallback` deems canonical. Removes one more place to keep in sync.
- **Do the existing summary assertions still hold across providers?** — Recommendation: the assertions are already "summary non-empty, >20 chars, message/participant counts exact." Those should hold for any capable model. Leave them alone; if Anthropic's output shape breaks the `len(summary) < 20` check in practice, loosen at that point.
- **Is 30s the right per-call timeout, and how does it interact with the fallback chain?** — Recommendation: 30s is a reasonable starting point but the interaction deserves a callout. `NewAllProvidersWithFallback` composes providers into a sequential `FallbackProvider` (Anthropic → Gemini → OpenAI). If the primary stalls for the full 30s before erroring, fallbacks never get their shot. For the specific failure mode the issue targets — a fast-return 429 — this is fine: the 429 arrives in ~1s, the fallback runs inside the remaining budget, we still succeed within 30s. For slow/hung primaries it's less fine, but slow-hung upstreams are addressed by issue #1237 at the RPC layer, not here. If empirically we see chain runs exceeding 30s in CI, either bump to 60s or push per-provider timeouts into the provider layer (part of #1237's Option B). Anthropic Haiku averages ~4s; Gemini Flash similar; OpenAI gpt-5-mini a bit slower.
- **How do we detect "quota error" robustly?** — Recommendation: reuse the string-match pattern already established in `server/ai/test_helpers_test.go:29-32` — lowercase the error message and look for `"429"`, `"rate limit"`, `"resource exhausted"`, or `"quota"`. Not elegant, but it works across all three providers' error surfaces today. If we later add typed rate-limit errors to the `ai` package, migrate the check at that point.
- **Should the quota-skip wrap the *whole* test run or just the `GenConversationSummary` call?** — Recommendation: just the RPC call. Setup errors (storage insert failures, provider construction errors) should still fail loudly — a quota-skip there would mask real bugs.
- **Should we also short-circuit on `context.DeadlineExceeded`?** — Recommendation: no. If the 30s timeout fires, that's a real regression worth a red test; skipping on timeout would hide a stuck provider. Keep `t.Fatal` for deadline exceeded.

## Implementation Plan

### Phase 1: Swap to production provider wiring
**Goal:** Test uses `ai.NewAllProvidersWithFallback` with the same env vars `initializeAIProvider` reads; CI no longer fails on a single-provider 429 as long as another provider is configured.
**Tasks:**
- [ ] Add a `testProviderFromEnv(t *testing.T, ctx context.Context) ai.Provider` helper in `server/services/chat/summary_integration_test.go` (or a new `integration_helpers_test.go` in the same package — **must carry `//go:build integration` as its first line**, matching the existing file so the helper is only compiled when the integration tag is set). It reads `VERTEX_AI_PROJECT`, `VERTEX_AI_LOCATION`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, and model override env vars; calls `ai.NewAllProvidersWithFallback`; `t.Skip`s if no provider is configured; `t.Fatal`s on construction error; uses `t.Logf` (not `logging.Default()` or `fmt.Printf`) to surface which providers were configured so the info shows up in `go test -v` output.
- [ ] Replace lines 22-25 (the `VERTEX_AI_PROJECT`-only skip) and lines 156-157 (the `NewGeminiProvider` call) in `TestGenConversationSummary_Integration` with `aiProvider := testProviderFromEnv(t, ctx)`.
- [ ] Replace lines 210-213 and lines 296-297 in `TestGenConversationSummary_Integration_MultipleParticipants` with the same helper call.
- [ ] Run `go test -tags=integration -run TestGenConversationSummary_Integration ./server/services/chat/...` locally with `ANTHROPIC_API_KEY` set (and Vertex unset) to confirm the fallback path works when Gemini is absent.
- [ ] Run the same command with both Anthropic and Vertex set to confirm the primary path runs.
- [ ] Run `npm run build` to confirm lint passes.

**Validates:** The failing-CI scenario (Gemini quota exhausted, Anthropic available) now passes. The architectural-drift scenario (prod uses `NewAllProvidersWithFallback`, test didn't) is closed.

### Phase 2: Add per-call timeout and quota-skip
**Goal:** Any single `GenConversationSummary` call is bounded at 30s; if every provider in the chain is quota-throttled, the test skips rather than fails.
**Tasks:**
- [ ] Add an `isAIQuotaError(err error) bool` helper alongside `testProviderFromEnv` that lowercases the error message and matches on `"429"`, `"rate limit"`, `"resource exhausted"`, `"quota"`. Keep the predicate focused — do not match on generic `"unavailable"` or `"timeout"` (those should stay as failures).
- [ ] In both integration tests, replace the direct `ctx = withAuth(user1ID)` → `service.GenConversationSummary(ctx, req)` pair with a pattern that wraps the RPC call in `context.WithTimeout(ctx, 30*time.Second)` (on top of the auth ctx) and, on error, checks `isAIQuotaError(err)` → `t.Skipf("all AI providers quota-throttled: %v", err)` before falling through to `t.Fatal`. A small helper like `callSummaryOrSkip(t, ctx, service, req)` keeps each test readable.
- [ ] Ensure the 30s timeout is `defer cancel()`-ed properly so leaked contexts don't accumulate across subtests.
- [ ] Verify (by temporarily pointing Anthropic at an invalid key + unsetting Vertex) that a simulated 429 triggers a skip, not a fail. Revert the forced-failure before committing.
- [ ] Run `npm run build` again to confirm lint passes with the new helpers.

**Validates:** The test cannot consume the full package-level go-test timeout on a single hanging call. Simultaneous quota exhaustion across all configured providers produces a skip (yellow) rather than a failure (red) — accurately reporting "we couldn't run the test," not "the code is broken."

### Phase 3: Sanity-check that no other integration test repeats the pattern
**Goal:** Prevent the same drift from recurring elsewhere.
**Tasks:**
- [ ] `grep -rn "NewGeminiProvider\b\|NewAnthropicProvider\b" server/services/*/` under the `integration` build tag. For each hit, note whether the test genuinely *needs* a single provider (e.g., provider-specific regression tests in `server/ai/provider_integration_test.go` and `server/ai/time_parsing_integration_test.go` legitimately do) or whether it should move to `NewAllProvidersWithFallback`.
- [ ] File a lightweight follow-up issue listing any additional integration tests that should migrate. Do not fold into this PR; this issue is explicitly about the chat summary test.

**Validates:** We have a paper trail of which integration tests are still single-provider and whether that's intentional. Scope of this PR stays tight.

## Related Issues
- #1237 — Integration tests wedge for minutes when LLM calls hang. Companion issue: #1237 addresses *hangs* (RPC handlers with no per-call timeout), while this issue addresses *provider-selection drift from prod*. Both contribute to the same CI pain, but the fixes are independent — one adds timeouts at call sites, the other fixes test construction. They can ship in either order.
- PR #1232 — The CI run (24814282085) where this flake was first reported.
