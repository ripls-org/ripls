# Issue #1216: Re-evaluate AI provider model defaults using Phase 4a benchmark findings (gated on quality eval)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1216
**Filed:** 2026-04-22 | **Last activity:** 2026-04-24
**Labels:** enhancement, P2 | **Assignees:** unassigned
**Context docs:** none

## Audit Notes (2026-04-24)

Ran `/ripls-audit` against `docs/server/architecture.md`, `docs/server/llm.md`, and the client reference docs. This is a server-only change (no proto, no client, no storage schema) so the client docs are out of scope. Findings from the audit:

- **Hardcoded model defaults are duplicated across several sites**, not just `provider_factory.go:91-95`. The initial plan only pointed at the const block and the `createProvider` fallbacks. Phase 2 and Phase 3 task lists now enumerate every site that needs updating (see Phase 2/3 below).
- **`docs/server/llm.md` references the current default model names in multiple places** (lines 28–30, 66–68, 78–80, 84–86). Any default swap requires a doc update in the same PR to keep the LLM doc truthful. Phase 2 and Phase 3 now include an explicit doc-update task.
- **Testing convention alignment confirmed.** The plan extends the existing `benchmark`-tag-gated test harness (`server/ai/eval/prompt_test.go:1`), which matches CLAUDE.md's guidance that tests hitting external AI APIs must be tag-gated. No new pattern introduced.
- **No architectural principles violated.** The eval harness is a shared library per `docs/server/architecture.md` §3 patterns — no new cross-service dependencies, no service-to-service calls, no new storage surface.

## Problem Statement

Phase 4a benchmarking for #1157 surfaced two concrete model-default swaps where the candidate is materially faster than the current default and deployable in our prod region today:

- **OpenAI** `gpt-5-mini` (27.56s median) → `gpt-5.4-nano` (1.69s) — **~16× speedup**. Headline finding: gpt-5-mini's structured-output streaming is effectively unary (first delta at ~93% of total elapsed), so Phase 4a's early-fire fan-out has nothing to overlap with. Falling back to OpenAI today means a ~28s spinner for the user.
- **Gemini** `gemini-2.5-flash` (5.03s) → `gemini-2.5-flash-lite` (1.20s) — **~4× speedup**.
- **Anthropic** `claude-haiku-4-5` stays — no faster alternative measured, and Haiku is the established quality baseline.
- **Gemini 3.1 Flash-Lite Preview** is parking-lot: faster than every current default but `global`-only, which adds cross-region latency and changes data-residency posture.

The swaps are latency wins, not correctness changes, but smaller/newer models can regress output quality in subtle ways. The gate is an eval-harness pass against the existing goldens (`server/ai/eval/testdata/experience_goldens.json`, plus Gear and Request goldens) within ~2pp of the current default's baseline — same bar we'd apply to any prompt edit.

One implementation wrinkle that isn't called out in the issue: the existing eval harness (`server/ai/eval/prompt_test.go`) is hardcoded to Anthropic. `newAnthropicProviderForEval` takes an `-anthropic-api-key` flag and constructs `claude-haiku-4-5` directly. To use the harness to validate candidate OpenAI and Gemini models we first need to generalize it across providers. That's a small but real piece of infra work that sits in front of the swap PRs.

## Context from Comments

One comment (my own, 2026-04-24) adds external research on the three providers' small-model defaults for streaming + structured JSON:

- Benchmarks the same three candidates from different angles: TTFT, output tok/s, blended $/M. Confirms `gemini-2.5-flash-lite` as the fastest and cheapest of the three, with `claude-haiku-4-5` as the established quality baseline and `gpt-5.4-nano` in the middle on both axes.
- **New config gotchas** the issue body doesn't cover:
  - **OpenAI**: `gpt-5.4-nano` is reasoning-capable and defaults to `reasoning.effort: "none"`. Overriding to `low`/`medium`/`high` degrades TTFT to 3–5s. The 16× speedup depends on *not* touching this.
  - **OpenAI**: parallel tool calls unsupported at `none`/`minimal` effort. Our Gen\* flows use single-tool tool-use so this is fine, but worth verifying.
  - **Anthropic**: extended thinking off by default — do not enable.
  - **Gemini**: no effort knob; non-reasoning by default.
- **Streaming + strict JSON gotcha**: partial JSON during the stream is unparseable until completion. Our server already handles this via the streaming fan-out architecture (Phase 4a/b/c from #1157) — emit field-level events as the streaming JSON parser completes keys. So this is more "make sure we don't break the existing pattern during the swap" than a new design concern.
- **Migration rule**: strict schema constraints can hurt semantic accuracy (the format constraint fights the model's natural distribution). Haiku is unusually well-tuned for strict JSON — worth bearing in mind if we ever evaluate swapping the Anthropic primary. For the OpenAI/Gemini fallbacks we're actually looking at here, Haiku's tuning isn't directly relevant; we just need the candidates to match their own current defaults' quality, not Haiku's.

No disagreements or unresolved debates in the thread.

## Potential Solutions

### Option A: Generalize eval harness, run evals, swap both

**Approach:** Extend `server/ai/eval/prompt_test.go` to accept any of the three providers (add `newOpenAIProviderForEval`, `newGeminiProviderForEval` alongside the existing Anthropic helper; wire `-openai-api-key`, `-gemini-project`, and `-model` flags). Run the full goldens suite against each current default to establish a baseline, then against each candidate. For each candidate that passes within the tolerance band, update `server/ai/provider_factory.go:91-95` constants. Record every eval run (pass and fail) in `docs/evals/1216-provider-model-defaults.md` so negative results aren't lost.

- **Pros:** Data-driven, matches the issue's gate, builds reusable infra that pays off for future model swaps and #1265 (Opus prompt rewrite). Each swap is a small, reviewable PR.
- **Cons:** The eval-harness generalization is real work (a day, not an hour). Spends credits on three providers' evals.
- **Effort:** Medium.

### Option B: Swap OpenAI only, skip the Gemini and harness work

**Approach:** Just update `defaultOpenAIModel` to `gpt-5.4-nano` in `provider_factory.go`. Skip the eval gate on the theory that OpenAI is our tertiary fallback — partial quality degradation is acceptable to cap the ~28s spinner pain.

- **Pros:** Fast. Addresses the biggest user-visible pain (the 28s spinner) immediately. Very low blast radius since OpenAI sits behind two other providers.
- **Cons:** Sets a bad precedent of untested model swaps. Doesn't address Gemini, which is the more-likely-to-fire fallback. No reusable infra for #1265 or for future model decisions. If `gpt-5.4-nano` silently regresses on, say, `location_query` extraction, we won't know until users see bad pins.
- **Effort:** Small.

### Option C: Invert — build harness first as its own PR, then two swap PRs gated on it

**Approach:** Same ingredients as Option A, but sequenced as three independently-reviewable PRs instead of one: (1) eval-harness provider generalization, (2) OpenAI swap, (3) Gemini swap. PR 1 includes the baseline eval runs and a short docs section in `server/ai/README.md` explaining how to invoke the harness against any provider.

- **Pros:** Same outcome as A with better review ergonomics. PR 1 is reusable infrastructure that stands on its own merit (unblocks #1265, future model upgrades). PRs 2 and 3 become ~10-line diffs plus an eval-log entry — easy to review. Any swap can be reverted without touching the others.
- **Cons:** Three PRs instead of one (more ceremony). The harness PR has no user-facing impact on its own.
- **Effort:** Medium (same total work as A; slightly more overhead from PR splits).

## Recommendation

**Option C.** The PR-split pays for itself: the harness generalization is the sort of infra change that wants a focused review, and splitting it out means each model swap is a trivial diff plus a pointer to an eval-log entry. The infra is also genuinely reusable — both #1265 (Opus prompt rewrites) and the "Gemini 3.x when GA lands in us-central1" parking-lot item will want this same harness, and #1216's acceptance criteria ("If we decide NOT to swap any of them, that decision is recorded") implicitly assumes you can easily re-run the eval against alternatives.

Option A is fine if the reviewer would rather land it in a single bundled PR — the work and risk are identical. Option B is tempting for the OpenAI-only pain but I wouldn't: once you've built the harness for OpenAI you've already done 80% of the Gemini work, and skipping the gate normalizes untested swaps.

## Open Questions

- **Should the eval harness run against dev or prod provider keys?** — Recommendation: dev keys for AI providers, prod keys for Mapbox/Pexels to match the Phase 4a benchmark convention (`gen_ai_fanout_benchmark_test.go:12-19`). The eval only exercises the AI call, so dev keys are correct; keeping the convention aligned lets us reuse env-setup instructions.
- **What pass-rate tolerance do we use for each candidate?** — Recommendation: follow the issue — ~5pp for OpenAI (tertiary fallback, partial degradation acceptable given the 16× speedup), ~2pp for Gemini (secondary fallback; fires more often). Record both thresholds in `docs/evals/1216-provider-model-defaults.md` as the decision policy so future candidates don't re-litigate it.
- **Do we run evals on all three goldens suites (experience, gear, request) or just experience?** — Recommendation: all three. The Phase 4a benchmark measured `GenExperience` but the defaults swap affects every Gen\* method. Running all three is maybe 3× the cost but catches per-flow regressions that experience-only wouldn't see (e.g., `gpt-5.4-nano` might handle `location_query` well but botch Gear's search-keywords generation).
- **Is #1215 a prerequisite?** — Recommendation: no. The issue itself says it's not a hard blocker, and the constants in `provider_factory.go` are the source of truth either way. Mention #1215 in each swap PR as a "would-enable-env-flip rollback" note but don't gate on it.
- **Do we keep the image + webpage eval suites (gear_image, experience_image, gear_webpage, experience_webpage) in scope?** — **Decision: yes, in scope.** A default-model swap affects every Gen\* method including image and webpage paths, so the gate has to validate those too — a candidate that regresses on `DetectGearInImage` or `GenerateExperienceFromImage` would silently break the photo-first flows even if its text metrics looked fine. The per-candidate eval-time overhead is small (image+webpage adds only 11 cases to the 51 text cases) and the regression coverage is non-optional. This supersedes the earlier "text-only" scoping recorded in the initial draft.

## Implementation Plan

### Phase 1: Generalize eval harness across providers (all modes)

**Goal:** Enable running the existing goldens against any of the three providers via CLI flags — across text, image, and webpage modes — with a documented invocation recipe.

**Tasks:**
- [x] Add `-openai-api-key`, `-openai-model`, `-gemini-project`, `-gemini-location`, `-gemini-model`, `-anthropic-model` flags to `server/ai/eval/prompt_test.go` (extending the existing `-anthropic-api-key` pattern at line 35).
- [x] Add `newOpenAIProviderForEval(t)` and `newGeminiProviderForEval(t)` helpers mirroring `newAnthropicProviderForEval` at line 368. Make the existing helper read `-anthropic-model` so we can eval non-default Anthropic models in the future too.
- [x] Introduce a `-provider` flag (anthropic|openai|gemini) that routes **every** `Test*Prompt` test through a dispatcher — text, image, and webpage. Default remains `anthropic` so existing invocations don't break.
- [x] Add a `-mode` flag (all|text|image|webpage, default `all`) so iteration loops can run a single mode without writing a regex. `requireMode` skips tests whose declared mode doesn't match.
- [x] Document the new invocation recipes in `server/ai/README.md` — one block per provider, plus mode-filtered examples for faster iteration.
- [x] Run the harness against each current default (`claude-haiku-4-5`, `gpt-5-mini`, `gemini-2.5-flash`) **across all seven suites** (3 text + 2 image + 2 webpage). Record results as a new dated entry in `docs/evals/1216-provider-model-defaults.md` — provider, model, per-suite pass rate, per-field pass rate. *Entry dated 2026-04-24.*

**Validates:** Infrastructure works for all modes, and we have a measured baseline to compare candidates against — including image and webpage regressions that a text-only eval would miss.

### Phase 2: OpenAI swap (gpt-5-mini → gpt-5.4-nano) — **BLOCKED**

**Goal:** Land the 16× speedup on the OpenAI fallback path with eval backing.

**Tasks:**
- [x] Run the harness against `gpt-5.4-nano` across all three text goldens suites (experience, gear, request). Verify `reasoning.effort` is not overridden in `server/ai/provider_openai.go` — the config note from the external research comment says the default `none` is load-bearing.
- [x] Add an entry to `docs/evals/1216-provider-model-defaults.md` with the swap decision, per-field deltas, and the PR link. *Entry dated 2026-04-24.*
- [x] Candidate failed the gate: record the failure in the eval log with per-field deltas and open a follow-up issue. Do not swap.

**Outcome:** Text within gate (−2.6pp, under 5pp threshold), but **image mode regressed 12.2pp** (32/33 → 28/33). Specific image failures: `material_category_any_of` 2/4 (was 3/4), `model_contains` 2/3 (was 3/3), `brand_contains` 3/4 (was 4/4), `weight_grams_approx` 1/2 (was 2/2). Photo-first Gen\* flows would silently degrade.

**Upsides not acted on:** candidate fixes the `dewalt_drill` JSON-parse invocation failure (malformed scientific-notation float) and runs ~16× faster on text. Not enough to offset the image regression.

**Not done:** code changes deliberately not made. `gpt-5-mini` remains the OpenAI default. File a separate issue if the parse-error bug on gpt-5-mini proves to bite production traffic often enough to warrant the image-quality tradeoff.

### Phase 3: Gemini swap (gemini-2.5-flash → gemini-2.5-flash-lite) — **BLOCKED**

**Goal:** Land the 4× speedup on the Gemini secondary-fallback path with eval backing.

**Tasks:**
- [x] Run the harness against `vertexai/gemini-2.5-flash-lite` across all three text goldens suites.
- [x] Add an entry to `docs/evals/1216-provider-model-defaults.md` with the swap decision and per-field deltas. *Entry dated 2026-04-24.*
- [ ] (Stretch) Run the harness against `vertexai/gemini-3.1-flash-lite-preview` via `global` location and record the result. No swap regardless of outcome — this is just parking-lot data for when us-central1 GA lands.

**Outcome:** Text regressed **5.6pp** (94.9% → 89.3%), well outside the 2pp gate. Decisive regressions concentrated on numeric fields: Gear `value_estimate_usd` 6/12 (was 10/12), Experience `date` 10/13 (was 13/13), Gear `weight_grams_approx` 4/5 (was 5/5), plus `time` and `time_confidence` misses. These are central to the Experience and Gear creation UX — not acceptable losses.

**Upside not acted on:** image mode actually improved from 90.9% → 100% — `gemini-2.5-flash-lite` fixes the composite-material bias that `2.5-flash` has on gear images. But the numeric-field text regression dominates.

**Not done:** code changes deliberately not made. `vertexai/gemini-2.5-flash` remains the Gemini default.

### Follow-up direction (not a phase in this issue)

With both candidates blocked, the biggest latency win left to pursue is **prompt caching + cacheable/dynamic prompt split** — the "step B" track outlined in the #1157 external-research comment. It's prompt-side, not model-side, so it sidesteps the quality/latency tradeoff that killed these candidates. Worth sequencing before the next model-default re-evaluation. Also file a one-time agent to re-run the eval against `gemini-3.1-flash-lite-preview` once Vertex AI GA lands it in us-central1 — the harness is ready for that comparison today.

## Related Issues

- #1157 — parent perceived-latency issue (closed); Phase 4a benchmark data that motivates this swap
- #1212 — Phase 2 eval harness (closed); the infra this plan extends
- #1215 — wire AI model env vars end-to-end; not a blocker, but complementary — lets a future swap ship as an env-var flip on Cloud Run instead of a code change
- #1265 — Opus-driven prompt rewrite; same eval-harness extension unblocks that work too (prompt rewrite per-provider evaluations)
- #1226 — consolidate Gen\* JSON output schemas; orthogonal but touches the same schema code that provider swaps will exercise
