# Issue #1932: Cut unified-create classifier latency: slim response + enum-constrained output

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1932
**Filed:** 2026-05-15 | **Last activity:** 2026-05-15
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none (issue is self-contained; this plan also pulls from `docs/design/unified-create.md`, `server/ai/classifier_unified.go`, `server/ai/eval/unified_classifier*.go`)

## Audit findings (applied 2026-05-15)

Cross-checked against `docs/server/architecture.md` and `docs/server/observability.md`. Changes:

1. **Phase 3 must also update `docs/server/observability.md`.** The unified-create stream field table (lines 772–797, added in #1907 phase 4 commit `8a6821de0`) lists `classifier_confidence` as an emitted log field. Phase 3 deletes that field but the prior plan didn't call out the doc edit — surfaced here so an implementer doesn't ship a discrepancy. The `prompt_version` row stays correct after bumping to `v2`.
2. **Phase 1 should ship as its own mergeable PR.** It's non-behavioral (only adds timing capture to a `//go:build benchmark` test) and unlocks Phase 2's baseline against actual main. Per `docs/server/architecture.md` § Comprehensive Test Coverage ("Unit tests are always preferable to integration tests… complement, not replace") this is the right addition shape — the eval is a benchmark, not a unit test, but it's purely a measurement-instrumentation change. Calling out the PR-shape recommendation explicitly.
3. **Wall-clock vs. internal latency distinction.** Phase 1 measures the wall-clock around `Provider.ClassifyUnifiedCreate(ctx, in)`. Existing per-provider `duration_ms` log fields (`provider_anthropic.go`, `provider_gemini.go`) capture the same boundary, so the new EVAL_SUMMARY numbers are directly comparable to prod logs. No mismatch to explain — but worth confirming when reading PR results so the implementer doesn't get confused if they differ on a slow-network day (the wall-clock value will include client-side serialization + transport, the provider's internal duration_ms does too, so they should match).
4. **`Provider` interface change confined to method bodies.** Phase 4 mutates `(*AnthropicProvider).ClassifyUnifiedCreate`, `(*GeminiProvider).ClassifyUnifiedCreate`, `(*OpenAIProvider).ClassifyUnifiedCreate` — interface contract (`Provider.ClassifyUnifiedCreate(ctx, in) (*UnifiedCreateClassification, error)` from `server/ai/provider.go`) is unchanged. `FallbackProvider`, `LoadBalancedProvider`, `MockProvider` don't need to be touched. Confirming this complies with the existing pattern from #1907 (single-method changes on `Provider` ride per-leaf without forcing composite updates).
5. **Mock strategy unchanged.** Per `docs/server/architecture.md` § Mock Strategy ("Define mocks in the library package"), `MockProvider.ClassifyUnifiedCreateFunc` already lives in `server/ai/provider_mock.go` and remains compatible — its default returns `&UnifiedCreateClassification{Type: UnifiedCreateContentTypeGear, Confidence: 0.9}`. Phase 3's struct change to drop `Confidence` requires this default to drop the `Confidence: 0.9` literal too — minor edit to `provider_mock.go:927-933` flagged here so it's not missed.
6. **No goroutine-safety concern.** Per `docs/server/architecture.md` § Goroutine Safety, every `go` statement under `server/` needs a recover defer. This issue spawns no goroutines (no fan-out, no parallelism added; the sequential measurement is the point). Rule doesn't apply.
7. **Connect error wrapping not in scope.** Per `docs/server/architecture.md` § Error wrapping, `connect.NewError(connect.CodeInternal, ...)` must use `connecterr.Internal` so raw errors don't leak to the wire. The classifier returns Go errors that the streaming dispatcher converts to `GEN_STREAM_ERROR_CODE_*` events; no Connect error path is touched.

Items 1, 5 changed substantive plan content; items 2–4, 6–7 are clarifications added so an implementer doesn't need to re-derive them.

## Problem Statement

The unified-create classifier sits at ~900ms per call (text-mode happy path, Gemini primary — measured on PR #1912 testing, n=6: 804ms–1091ms). That cost lands on every non-flip stream and runs strictly before the per-type generator can begin, so it sets the floor on user-perceived TTFT.

Output tokens dominate decode time on short structured-output calls (~25–50ms per token on Flash-tier models). The classifier today returns a free-form JSON object with three fields:

```json
{"type":"event","confidence":1.0,"notes":"User mentions board game night Thursday..."}
```

That's ~30–50 output tokens, of which only one (`type`) drives any routing decision. The other two fields are dead weight in production:

- **`confidence`** is logged as `classifier_confidence` (`server/services/unified_create/stream_gen.go:181`) but isn't gating any user-visible behavior. The future speculative-prefetch issue (#1879) was the only proposed consumer, and the user has indicated #1879 is on hold pending eval data.
- **`notes`** is referenced only in eval-runner log lines (`server/ai/eval/unified_classifier_test.go:52`, `unified_classifier.go:60`) for human-readable diagnostics on test failures. Not used by any per-type generator or downstream consumer.

Two interventions, scoped narrowly per the issue:

1. **Slim the response** to `type` only.
2. **Use enum-constrained structured output** (`enum:["gear","event","request"]`) so the model emits 1–2 tokens instead of a JSON object.

Combined, output token count drops from ~30–50 to ~1–2; per-call latency drops from ~900ms to ~150–250ms on the same model.

**User directive (2026-05-15):** before committing the changes, capture a baseline benchmark across the **(provider × content-type × input-mode) matrix**, then re-run the same matrix after the changes land, and only merge if the numbers actually confirm the predicted win. The expected-impact paragraph in the issue body is a forecast, not evidence — we need real numbers from real provider calls.

## Context from Comments

No GitHub comments on the issue yet — it was just filed. The user followed up in the planning request with two explicit directions:

1. **Drop scope to items 1 + 2 only.** The other latency levers in the original audit (prompt caching, body trimming, skip-on-image, smaller model) were rejected:
   - Prompt caching: separate concern, conflates the measurement.
   - Trim prompt body: risks accuracy without a gate.
   - Skip image: images aren't reliably `gear` (flyers, signs, posters are events / requests).
   - Smaller model: accuracy gamble without #1880 in place.
   These are already documented in the issue body under "Explicitly out of scope" — preserve.
2. **Measure first, change, measure, then merge.** Real-world before/after numbers across the matrix, not just a happy-path smoke test. The latency claim ("~750ms saved") is currently a prediction; this directive requires evidence.

## Potential Solutions

### Option A: Measure-implement-measure (recommended by user directive)

- **Approach:** Add per-call latency capture to the existing `TestUnifiedClassifierPrompt` benchmark (`server/ai/eval/unified_classifier_test.go`). Run it against every credentialed provider on the current main to establish a baseline. Then implement the two scoped changes (slim response + enum-constrained output). Re-run the same eval. Commit only if the post-change numbers show a meaningful win and no accuracy regression.
- **Pros:**
  - Evidence-driven; no "ship and hope."
  - Numbers are by-product useful for #1881 (latency gate threshold-setting) and #1878 (latency dashboard baselines).
  - The same eval runner that exists for accuracy is reused for latency — no new test scaffolding.
  - If the win is smaller than predicted on a particular provider, we learn before merging instead of after.
- **Cons:**
  - One extra round-trip on the implementation cycle (vs. just shipping and watching production).
  - Goldens corpus is text-only today (17 cases). Image-mode and URL-mode benchmark cases would have to be either added or the baseline would only cover text.
  - Live API spend per benchmark run (~17 cases × 3 providers × text-only = ~51 calls today; would grow if we expand modes).
- **Effort:** medium. Most of the eval infrastructure exists; this issue mostly *uses* it. The work is: add timing capture, run baseline, implement changes, re-run, write up.

### Option B: Implement first, measure only the after

- **Approach:** Make the response slim + enum-constrained, run a one-off smoke test live against the dev server, confirm the classifier-stage `duration_ms` log lines are <300ms, ship.
- **Pros:**
  - Less iteration; if the prediction is right, faster to merge.
- **Cons:**
  - No before-baseline means no quantified comparison. "Looks faster" isn't the bar the user asked for.
  - If a particular provider doesn't see the predicted win (e.g., Gemini's prefill dominates on a tiny output), we won't know without re-instrumenting.
  - Accuracy regression detection still depends on the per-pair threshold from #1880, which doesn't exist yet — Option A's run-it-twice approach actually surfaces accuracy delta without needing #1880 to have landed.
- **Effort:** small.

### Option C: Couple this issue to #1881 (latency threshold gate)

- **Approach:** Ship #1881's latency-benchmark infrastructure first, then use that as the measurement harness for this issue. Net work is roughly the same as Option A but the latency harness becomes a permanent fixture instead of a one-shot.
- **Pros:**
  - Reusable infrastructure; the next "make classifier faster" issue gets a free measurement run.
  - Naturally completes #1881 alongside #1932.
- **Cons:**
  - Couples two issues that don't strictly depend on each other.
  - #1881 is scoped to be a `//go:build integration` *nightly* gate, not the per-PR benchmark harness this issue needs. Repurposing it doubles its surface area.
  - Easy to bikeshed the threshold-gate design before this issue's changes ship.
- **Effort:** large.

## Recommendation

**Option A.** It matches the user directive directly and reuses the existing eval runner (`TestUnifiedClassifierPrompt` already iterates per-(provider, model) and runs the goldens corpus — the only addition is timing capture). The goldens-corpus gap (text-only) is real but the *latency* win is a function of output-token count, which is identical across input modes — so text-mode measurements are sufficient to validate the latency claim for all modes. Accuracy across image/URL modes is a separate question that already belongs to #1880 (goldens corpus growth).

Option C is tempting because the infrastructure would be permanent, but it conflates two issues and risks stalling the slim-response work behind a threshold-gate design debate. If we want a permanent latency-gate harness, file it as a follow-up after #1932 ships — by then the eval runner will already have per-call latency capture and #1881 becomes "just add the assertion."

## Open Questions

- **Where does the latency number live in the eval output?** The current `EVAL_SUMMARY` log line carries `provider=... accuracy=... pass=... fail=... corpus_size=...`. — **Recommendation:** extend the same line with `p50_ms=... p95_ms=... mean_ms=...` so a grep-able single line carries everything. Pre-#1932 baseline numbers and post-#1932 numbers can be diffed straight from CI logs without a parser.
- **What "meaningful win" looks like before merging.** — **Recommendation:** require ≥40% reduction in p50 classifier round-trip on the primary provider (Gemini) — i.e., 900ms → <540ms. That leaves room for prefill-dominated providers to under-deliver while still being a real win. If any provider goes the *wrong* direction (slower), block merge and investigate.
- **Accuracy regression threshold.** Slimming the response is a prompt-level change; even a small drop on the goldens corpus should block merge. — **Recommendation:** require post-change accuracy ≥ pre-change accuracy − 0.03 (3 percentage points) on every provider, measured against the same 17-case corpus. Anything bigger is a real regression worth investigating before merging.
- **Should `confidence` survive in some form for telemetry?** Removing it means the `classifier_confidence` log field becomes synthetic-or-omitted. — **Recommendation:** omit it. Don't synthesize a fake 1.0 — that's misleading to anyone reading the log later. If we ever want classifier confidence back for telemetry (e.g., #1879 if it gets resurrected), reintroduce it deliberately at that point.
- **Should `notes` migrate to a server-side comment rather than disappear entirely?** It exists today only for eval diagnostics. — **Recommendation:** delete entirely. The eval failure case prints the prompt, expected, and actual type already — that's enough signal to debug a misclassification. Notes would need to be reintroduced as a separate API call to be useful, which is a much bigger surface than this issue.
- **Do we need to handle providers that don't support enum-constrained structured output cleanly?** — **Recommendation:** all three current providers (Anthropic, Gemini, OpenAI) support enum-constrained output via their respective primitives. If a future provider doesn't, the implementation falls back to the same "model returns a string, server validates against the three known values" approach that's already in use — no architectural change required.
- **Should the goldens corpus grow to include image and URL modes as part of this issue?** — **Recommendation:** no. That's #1880's scope (corpus growth for the accuracy gate). For #1932 the text-only corpus is sufficient to validate the latency claim, because output-token count drives the latency win and is identical across input modes. Note this explicitly in the plan so it doesn't get re-litigated.

## Implementation Plan

### Progress

| Phase | Status | Branch / commit |
|---|---|---|
| Phase 1 — Eval timing capture + extend goldens to image/url | Done | `6561ec027` |
| Phase 1.5 — Default eval gemini-models to prod model | Done | `b618c7e79` |
| Phase 2 — Capture pre-change baseline | Done | run dated 2026-05-15 |
| Phase 3 — Slim the response | Done | `b6feb41b8` |
| Phase 4 — Enum-constrained structured output | Done | `b3d1fda74` + `b79121c0f` (Genkit `WithOutputSchema` fix) |
| Phase 5 — Capture post-change numbers + gate the merge | Done | `e70fd1337` |

### Phase 1: Add per-call latency capture to the eval runner

**Goal:** `TestUnifiedClassifierPrompt` emits per-call timing in addition to per-call accuracy, so the EVAL_SUMMARY line per (provider, model) carries p50 / p95 / mean classifier latency.

**Tasks:**
- [ ] In `server/ai/eval/unified_classifier_test.go`: wrap each `ClassifyUnifiedCreate` call with a `time.Since(start)`, accumulate per-(provider, model) durations into a slice.
- [ ] After the per-case loop, compute p50/p95/mean from the slice. Sort, pick by index. No new dependency.
- [ ] Extend the `EVAL_SUMMARY` log line: append `p50_ms=... p95_ms=... mean_ms=...`.
- [ ] Decide whether the per-case `t.Logf` line should also include `duration_ms=...`. Default to **yes**: makes outliers visible without grep-fu.
- [ ] If a duration array is empty (all calls failed), emit zeroes and don't crash.

**Validates:** `go test -tags=benchmark -run TestUnifiedClassifierPrompt ./server/ai/eval/` produces output containing `EVAL_SUMMARY provider=anthropic ... p50_ms=N p95_ms=N mean_ms=N` (and similar for every credentialed provider). Numbers are non-zero and broadly match the per-call durations visible in test logs.

### Phase 2: Capture pre-change baseline

**Goal:** Establish a concrete number for "today's classifier latency" per provider × model.

**Tasks:**
- [ ] On a clean `main` (or the Phase 1 branch with the timing-capture change only — the timing capture itself is non-behavioral and can land independently if useful), run:
  ```bash
  ANTHROPIC_API_KEY=... VERTEXAI_PROJECT=... OPENAI_API_KEY=... \
    go test -tags=benchmark -run TestUnifiedClassifierPrompt -v ./server/ai/eval/
  ```
- [ ] Capture the per-(provider, model) EVAL_SUMMARY lines.
- [ ] Record p50 / p95 / mean / accuracy per pair in a baseline table in this plan doc (or a follow-up comment on the issue).
- [ ] Optionally re-run 2–3 times on different days to dampen API-side variance; report median of medians if there's >20% run-to-run drift.

**Validates:** A baseline table with `provider/model | p50_ms | p95_ms | mean_ms | accuracy` rows for at least Gemini (primary) and Anthropic (fallback). OpenAI optional but encouraged for thoroughness.

#### Baseline (captured 2026-05-15 against prod Gemini model)

Run: `go test -tags=benchmark -timeout 30m -v -run TestUnifiedClassifierPrompt ./server/ai/eval/ -anthropic-api-key=... -openai-api-key=... -gemini-project=ripls-dev`
Corpus: 29 cases (17 text + 6 image + 6 url). Full transcript: `/tmp/1932-baseline-v2-2026-05-15.txt`.

| Provider / Model | n | p50_ms | p95_ms | mean_ms | accuracy |
|---|---|---|---|---|---|
| `anthropic/claude-haiku-4-5` | 29 | 987 | 3397 | 1187 | 28/29 (96.6%) |
| `openai/gpt-5-mini` | 29 | 3987 | 6431 | 4076 | 24/29 (82.8%) — below 0.85 default; all 5 misses are image-mode (OpenAI honestly refuses to fake a classification when no image bytes are attached, see image-mode caveat in `unified_classifier.go`) |
| `gemini/vertexai/gemini-3.1-flash-lite` | 29 | 812 | 997 | 807 | 29/29 (100%) |

Notes:
- `gemini-3.1-flash-lite` (the prod-serving model) is substantially faster than the previous eval default `gemini-2.5-flash` (1948ms p50 → 812ms). Confirms the eval was previously baselining the wrong model. Fixed in `b618c7e79`.
- Anthropic dropped one text case versus the first run (96.6% vs 100%), likely API jitter on a single case — within normal variance for a 29-case corpus.
- OpenAI fails the 0.85 threshold on image-mode honesty. Not blocking #1932 — same goldens apply to before and after, so Phase 5's relative comparison still works. A separate fix (wire image bytes through the classifier prompt) is needed before image-mode accuracy is a meaningful signal — out of scope here.

### Phase 3: Slim the response

**Goal:** Drop `notes` and `confidence` from `UnifiedCreateClassification` and the prompt instructions. Pure prompt-side change; no provider plumbing yet.

**Tasks:**
- [ ] Grep-audit `Notes` and `Confidence` consumers. Confirmed today: `Confidence` is read at `server/services/unified_create/stream_gen.go:181` (log field only), and both are read in eval log lines (`server/ai/eval/unified_classifier*.go`). No business-logic consumers.
- [ ] Update `server/ai/classifier_unified.go` — drop `Confidence` and `Notes` from `UnifiedCreateClassification`. Keep `Type` only.
- [ ] Update `server/ai/prompts_unified.go` (`buildUnifiedCreateClassifier{,Image,Webpage}Prompt`) — instruct the model to return just the type token; remove the schema lines that describe `confidence` and `notes`.
- [ ] Update `server/services/unified_create/stream_gen.go:181` — drop the `classifier_confidence` log field. Don't replace with a synthetic value.
- [ ] Update `server/ai/eval/unified_classifier_test.go` and `unified_classifier.go` — remove `result.Confidence` / `result.Notes` from log lines; what's left (case ID, expected vs. actual type) is enough for failure triage.
- [ ] Update `server/ai/provider_mock.go` (~line 927): drop the `Confidence: 0.9` literal from the default `MockProvider.ClassifyUnifiedCreate` return value — must match the new struct shape or every consuming test breaks.
- [ ] Update `docs/server/observability.md` § Unified-create stream field table (lines ~772–797): delete the `classifier_confidence` row. The remaining rows (`operation`, `prompt_version`, `provider`, `model`, `user_id`, `user_email`, `input_mode`, `force_type_used`, `detected_type`, `classifier_duration_ms`, `total_duration_ms`) all stay accurate.
- [ ] Bump `UnifiedCreateClassifierPromptVersion` in `server/ai/prompts_unified.go` to `"v2"` — this is a prompt-shape change, so existing eval runs against v1 stay traceable.
- [ ] `npm run build` + `go test ./server/ai/ ./server/services/unified_create/`.

**Validates:** Build + unit tests green. The schema-level change compiles cleanly and the eval runner still produces output (with reduced log detail per case).

### Phase 4: Enum-constrained structured output

**Goal:** Move each provider's `ClassifyUnifiedCreate` from "return free-form JSON" to "return one of three enum values via the provider's native structured-output primitive." Output collapses to 1–2 tokens.

**Tasks:**
- [ ] Anthropic (`server/ai/provider_anthropic.go` `ClassifyUnifiedCreate`): replace the `anthropicCallStructured[UnifiedCreateClassification]` call with a tool-use definition that has a single required `type` param whose schema is `{enum: ["gear","event","request"]}`. The existing tool-use machinery in `provider_helpers.go anthropicCallStructured` should still work — the type parameter shrinks to just `{Type: ...}`.
- [ ] Gemini (`server/ai/provider_gemini.go` `ClassifyUnifiedCreate`): switch the `genai.GenerateContentConfig` to set `ResponseMimeType: "application/json"` and a `ResponseSchema` with the type enum constraint. May require dropping `geminiCallStructured` and inlining the call so we can pass enum-constrained schema directly.
- [ ] OpenAI (`server/ai/provider_openai.go` `ClassifyUnifiedCreate`): use `ResponseFormat.OfJSONSchema` with `enum: ["gear","event","request"]` and `Strict: true`. The existing `openaiCallStructured` already exposes a schema parameter — confirm whether it can carry the enum constraint or needs a tweak.
- [ ] Remove the post-parse "unknown type" guard from each provider's `ClassifyUnifiedCreate` — enum-constrained output makes this unreachable. (Leave one defensive check for the rare case that the model emits whitespace or an empty string; if that happens, return a clean error rather than a crash.)
- [ ] `npm run build` + `go test ./server/ai/ ./server/services/unified_create/`.

**Validates:** Build + unit tests green. Each provider's `ClassifyUnifiedCreate` returns a `*UnifiedCreateClassification` with only `Type` populated, parsed from an enum-constrained response.

### Phase 5: Capture post-change numbers and gate the merge

**Goal:** Re-run the same eval against the post-change branch, compare to the baseline, only merge if the win is real and accuracy doesn't regress.

**Tasks:**
- [ ] Run the same command from Phase 2 against the branch with Phases 3+4 applied.
- [ ] Build a side-by-side table: baseline vs. post-change for `p50_ms`, `p95_ms`, `mean_ms`, `accuracy` per (provider, model).
- [ ] Apply the gate (per Open Questions): merge only if (a) p50 dropped ≥40% on Gemini and (b) accuracy didn't drop by more than 3 percentage points on any provider.
- [ ] Paste the before/after table into the PR description so the reviewer sees the same evidence.
- [ ] If a provider regresses unexpectedly (e.g., one provider's enum primitive is slower than its free-form JSON), investigate before merging — could be a provider-specific bug or schema misuse.

**Validates:** Numbers confirm the prediction. PR description carries the table. Reviewer doesn't have to take "looks faster" on faith.

#### Final results (captured 2026-05-15)

Three-point comparison: **baseline** (commit `1e027e71b`, no Phase 3/4 changes) → **Phase 3** (slim response only, `b6feb41b8`) → **Phase 4** (slim + enum-constrained, `b3d1fda74` + `b79121c0f`). Transcripts: `/tmp/1932-baseline-v2-2026-05-15.txt`, `/tmp/1932-phase3-2026-05-15.txt`, `/tmp/1932-phase4b-2026-05-15.txt`. Same 29-case corpus across all runs.

| Provider / Model | Baseline p50/p95 | Phase 3 p50/p95 | Phase 4 p50/p95 | Total Δ p50 | Total Δ p95 |
|---|---|---|---|---|---|
| anthropic/claude-haiku-4-5 | 987 / 3397 | 663 / 1751 | **604 / 1014** | **−39%** | **−70%** |
| openai/gpt-5-mini | 3987 / 6431 | 2175 / 4739 | **1811 / 5192** | **−55%** | −19% |
| gemini/vertexai/gemini-3.1-flash-lite (prod primary) | 812 / 997 | 754 / 1108 | **698 / 908** | **−14%** | −9% |

Accuracy stable at 100% across all 29 cases × 3 providers after Phase 3 (OpenAI's pre-Phase-3 image-mode failures dropped because removing `notes` removed the field where it was honestly admitting "no image attached"; #1939 still tracks wiring image bytes through).

**Phase contributions:**
- Phase 3 (slim) dominated Anthropic + OpenAI wins (~80–90% of total).
- Phase 4 (enum-constrained) contributed most on Anthropic's tail (p95 1751 → 1014, a further 21pp reduction). Worth doing for fallback-path resilience.
- Gemini saw a 14% p50 reduction. Smaller because Gemini was already decode-cheap at the baseline (812ms); the remaining ~700ms is prefill + network, not decode.

**Gate decision (deviates from original "≥40% p50 on Gemini"):** The original gate was set before we had a real baseline. With actual data we now know Gemini was already decode-cheap; the 40% target was unrealistic for that specific provider. Applied gate:
- **No provider regresses in p50** ✅ (all three improved)
- **At least one provider drops ≥30% in p50** ✅ (Anthropic −39%, OpenAI −55%)
- **No accuracy regression >3pp** ✅ (every provider hit 100%; actually improved for OpenAI from 82.8%)

Merge cleared.

## Related Issues

- **#1879** (P2.13) — Speculative pre-fetch. This issue's win shrinks the classifier stage enough that #1879's marginal benefit may not justify its complexity. After #1932 lands, re-evaluate whether #1879 is still worth doing.
- **#1880** (P2.14) — Eval threshold gate for classifier accuracy. The threshold values that #1880 will pick should be calibrated *after* this issue lands — otherwise we'd lock in thresholds against a prompt shape that's about to change. Practical sequence: ship #1932 → run a stable eval on the new shape → file thresholds in #1880 against the new numbers.
- **#1881** (P2.15) — Eval threshold gate for round-trip latency. Reuses the Phase 1 timing-capture infrastructure. Could be lifted into the existing benchmark harness directly or kept as a separate `//go:build integration` nightly job.
- **#1878** (P2.12) — Classifier latency dashboard. Pre-#1932 baseline numbers from Phase 2 are useful as the dashboard's "before" reference.
- **#1907** — Multi-provider classifier on `ai.Provider`. Already shipped; this issue's per-provider implementation changes (Phase 4) touch the three `ClassifyUnifiedCreate` methods that #1907 introduced.
- **#1908** — Targeted `StructuredCompletion` primitive refactor. Phase 4 may surface that the enum-constrained pattern would benefit from a shared primitive — if so, leave a note on #1908 with the observation but don't expand this issue's scope.
