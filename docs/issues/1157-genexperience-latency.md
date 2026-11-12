# Issue #1157: GenExperience / GenRequest perceived latency: stream media + parallelize Mapbox + stream AI tokens

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1157
**Filed:** 2026-04-17 | **Last activity:** 2026-04-21
**Labels:** (none) | **Assignees:** briangbrown
**Context docs:** none
**Eval log:** [docs/evals/1157-genexperience-latency.md](../evals/1157-genexperience-latency.md) — all before/after measurements, methodology, and end-to-end flow reference live here. Record new measurements as new dated entries (newest-first) after each phase lands.

## Audit Adjustments (2026-04-21)

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`, and project CLAUDE.md. Changes applied in-place:

- **Phase 1 scope correction — `Reasoning` field usage audit:** The original plan proposed dropping top-level `Reasoning` from *all* Gen* methods for consistency. Downstream audit showed this was too aggressive — one of the four generation types actually uses it:

  | Gen Type | Top-level `Reasoning` consumer | Action |
  |----------|--------------------------------|--------|
  | `ExperienceGeneration` | none (only `t.Logf` in integration test) | **Drop** |
  | `GearGeneration` | none | **Drop** |
  | `RequestGeneration` | none | **Drop** |
  | `CommunityGeneration` | `services/community/gen.go:83` → `GenCommunityResponse.reasoning` (proto field 5) → `community_creation_modal.dart:84` → community preview UI | **Keep** |
  | `ValueEstimate.Reasoning` (nested) | all four value-metric UIs | **Keep, bound via prompt** |
  | `SocialAttributeInference.*Reasoning` | `impact_metrics/estimator/social` | **Keep** (untouched) |

  Gear/Request/Experience proto responses have **no top-level reasoning field** — the Go structs populate a value that is read nowhere in production. Pure waste, safe to drop. Community's `reasoning` is a live UX contract; leave it alone. Token savings still apply for the three dropped paths.

- **Proto convention fix (Phase 4):** The original plan proposed reusing `GenExperienceRequest` for the new `StreamGenExperience` RPC. CLAUDE.md says "Never reuse RPC request or response message types across multiple RPCs." Plan now creates a dedicated `StreamGenExperienceRequest` with inline fields matching today's `GenExperienceRequest`. (Dedicated response-event type was already correct.)
- **i18n tasks added (Phase 4):** The streaming UI adds skeleton/empty states and per-event error messages. Per `docs/client/i18n.md`, every user-visible string goes through ARB files in both `app_en.arb` and `app_es.arb`, with `context.l10n.*` resolution in widgets (never in ViewModels). Explicit tasks now listed.
- **Async safety explicit (Phase 4):** Streaming subscription tasks now cite `ref.onDispose()` for cancellation, `SafeNotifierMixin.safeUpdateState()` for post-await state writes (already used in the existing file at `experience_creation_view_model.dart:183`), and the "regular Notifier with manual StreamSubscription" pattern from `docs/client/architecture.md#manual-stream-management` — not `StreamNotifier`, since this notifier has multiple concerns beyond one stream.
- **Disposal tests added (Phase 4):** `docs/client/architecture.md#disposal-testing` requires disposal tests for any async Notifier. Added as an explicit task.
- **File-split for `gen_ai.go` (Phase 4):** `server/services/experience/gen_ai.go` is already 526 lines. `docs/server/architecture.md#file-size-guidelines` targets < 500 (max < 1000). Adding a streaming state machine tips it over the target; plan now splits the new handler into `gen_ai_streaming.go`.
- **Test coverage gaps filled (Phase 3 and 4):** `docs/server/architecture.md` requires unit tests for all new code. Added unit-test tasks for the parallel-fan-out refactor (Phase 3) and the streaming JSON tokenizer + state machine (Phase 4, with table-driven tests).
- **Repository-caching clarification (Phase 4):** The `streamGenExperience` repository method is a cache-bypass pass-through — every generation is unique per prompt, so there is no `CacheService` layer. This matches `docs/client/caching.md`'s repository-owns-caching rule (repos decide what's cacheable; not everything needs to be). Note added.
- **Logging convention (Phase 3 & 4):** Log spans now explicitly use `logging.LoggerWithContext(ctx)` with standard field names (`operation`, `duration_ms`, `user_id`) per `docs/server/logging.md` — not ad-hoc `slog.Info` calls.

Everything else in the plan was already compliant: the MVVM Repository → ViewModel → Widget layering, async-first data access, `CachedMediaImage` for media (per project CLAUDE.md), shared-library boundaries (ai/ is a library, not a service), service-independence (no service-to-service calls introduced), and structured-error patterns.

## Problem Statement

`GenExperience` on a text prompt takes ~5.2s end-to-end, and the user sees a full-screen spinner for the entire duration. The measured breakdown on local dev (Android emulator, cold cache) is:

| Step | Duration | Share |
|------|----------|-------|
| Claude Haiku 4.5 (unary) | 4007 ms | 77% |
| Pexels stock video search/download/copy | 765 ms | 15% |
| Mapbox geocoding | 337 ms | 7% |
| Network + serialize + render | ~100 ms | 2% |

Two things stand out once you look closer:

1. The AI call **dominates wall-clock time, and output-token generation is the long pole within it.** ~350 output tokens at ~100 tok/s = ~3.5s of generation. The ~1500-token prompt only contributes ~500ms TTFT. This matters because prompt-input optimizations (caching, shrinking the static prompt) have a small ceiling; **output-schema optimizations have a large ceiling.**
2. Nothing about the current design masks the AI wait. Stock-media fetch and Mapbox geocoding run **serially after** the AI call, and the client doesn't see a response until all three complete. Even if the AI call stayed 4s, the user could be looking at a populated title/description at ~500ms if we streamed.

The issue's analysis (and the follow-up comments) converged on a clear priority ladder that mixes two kinds of wins:

- **Wall-clock wins:** trim output schema (biggest), prompt caching, shorter prompt, parallelize Mapbox + stock media.
- **Perceived-latency wins:** reorder output schema so Mapbox/media-fan-out fields stream earliest, and ultimately server-stream tokens to the client.

Target after full rollout: **~5.2s → ~2.5s actual wall-clock, ~500–800ms perceived time-to-first-content.**

## Context from Comments

Four substantial threads on the issue:

1. **Deep dive on the Claude long pole** (briangbrown) identified output tokens — not input — as the real bottleneck. Recommended: drop the `reasoning` field (~100–150 tokens of logged-but-never-shown text), drop/cap nested `value_estimate.reasoning` + `sources`, collapse confidence scores. Math: ~350 → ~180 output tokens ≈ **3.5s → 1.8s**. Also flushed out prompt-caching ROI (modest: ~250ms TTFT, only if call interval < 5 min cache TTL), a full streaming-end-to-end implementation plan (proto `StreamGenExperience` server-streaming RPC, streaming JSON tokenizer over `input_json_delta`, state-machine service layer that fires Mapbox + stock-media goroutines as soon as the relevant fields stream in, Flutter ViewModel consumes a stream and fills skeleton placeholders), and a recommended sequence.

2. **Schema field ordering** (briangbrown) pointed out that the current schema declares `reasoning` at position 3 and `location_query` at position 8. Since Claude respects tool-schema order and the streaming JSON arrives in schema order, this means the two fields that **trigger the longest-tail fan-outs** (Mapbox ~300ms, stock video ~800ms) don't complete until most of the call is already done. Reordering to put `title, location_query, search_keywords, date, ...` first and `reasoning` last is a **10-minute change with no contract impact** that fires fan-outs seconds sooner.

3. **Bryan** (maintainer): "Seems like we should remove reasoning field if we're not using it." Confirms the output-trim direction — `reasoning` comes out.

4. **Opus-rewritten prompts + eval harness** (briangbrown) proposed a meta-optimization: use Opus to rewrite `buildExperienceFromTextPrompt` (~1300 tokens → ~600–800) and related methods, gated by a golden-dataset eval (`server/ai/prompt_eval_test.go` behind `//go:build integration`, goldens in `server/ai/testdata/`, ~20–30 cases covering vague, explicit, contextual, past-tense, named-participants, etc.). The harness itself is reusable for future model upgrades and provider swaps.

Converged recommended sequence:
0. Reorder output schema (10 min, no contract change)
1. Trim output schema (drop `reasoning`, bound `value_estimate`)
2. Opus-rewritten prompt (gated by eval)
3. Prompt caching + cacheable/dynamic split
4. Parallelize Mapbox + stock media server-side
5. Streaming RPC end-to-end

## Potential Solutions

### Option A: Quick wins only (schema + prompt) — ~5.2s → ~3.2s actual

**Approach:** Ship phases 0–3 from the converged sequence. Reorder output schema, drop `reasoning` (confirmed by Bryan), shrink prompts (optionally Opus-rewritten + eval), split into cacheable + dynamic segments with `cache_control: {type: "ephemeral"}`. All server-side, no proto change, no client change.

- **Pros:** Biggest ROI per engineering hour. No contract change, no client work, single PR ships it. Half the latency gone.
- **Cons:** Users still stare at a ~3-second spinner. No perceived-latency win since the response is still unary.
- **Effort:** Small (1–2 days engineering + eval harness is the pacing item).

### Option B: Option A + parallelize Mapbox and stock media — ~3.2s → ~2.5s actual

**Approach:** Everything in A, plus rewrite the post-AI section of `gen_ai.go` so Mapbox `SearchPlaces` and `findAndStoreStockVideo/Image` run concurrently with `errgroup`. They're already independent (Mapbox uses `location_query`, stock media uses `search_keywords`).

- **Pros:** Another ~300ms with ~1 hour of work. Still server-only.
- **Cons:** Still no perceived-latency win. User still sees a single spinner → full response.
- **Effort:** Small additional increment on top of A.

### Option C: Full streaming end-to-end — ~2.5s actual, **~500–800ms perceived**

**Approach:** Everything in B, plus a new server-streaming RPC `StreamGenExperience` that emits `GenExperienceEvent` deltas: `title_delta`, `partial` (date/time/location_query), `geocoded` (after Mapbox join), `media` (after stock-media join), `final`. Provider layer adds `GenerateExperienceFromTextStreaming` using `client.Messages.NewStreaming` with a streaming JSON tokenizer over `input_json_delta` events. Service layer becomes a state machine that fires Mapbox + stock-media goroutines as soon as `location_query` / `search_keywords` stream in. Flutter client consumes the stream, opens the preview modal immediately, fills skeleton placeholders as events arrive.

- **Pros:** Transformative UX. `title` visible at ~400–600ms instead of 5s. Progressive field fill matches modern AI UX expectations.
- **Cons:** Largest refactor — proto contract, provider, service, repository, ViewModel, UI skeleton states, error handling for partial success, stream cancellation. Connect protocol streaming is already used in the codebase (`StreamCommunityEvents`, `StreamMessages`) so the pattern is proven, but this specific flow has per-field streaming JSON parsing which is novel.
- **Effort:** Large (est. 1–2 weeks). Best done after A+B land because those changes compound inside the streaming path.

## Recommendation

**Ship in order: A → B → C, each as its own PR.** This isn't one big plan — it's a ladder where each rung is independently valuable and the next one gets easier because of the previous one.

Reasoning:

- **A alone cuts the perceived problem in half** for a fraction of the engineering cost of C. That's where the ROI cliff is. Getting A shipped buys user relief while C is being scoped.
- **B is nearly free** once A's refactor is in flight; it's a 1-hour change that saves another ~300ms and removes serial-chain code smell from `gen_ai.go`.
- **C is the only option that changes the UX model**, from "spinner + response" to "progressive modal." But C's value is amplified by A+B: streaming a 4s call is less compelling than streaming a 1.8s call that can start firing Mapbox at 500ms.
- The eval harness from A's phase 2 is infrastructure that **outlives this issue** — every future prompt edit, model upgrade, or provider swap benefits from it. Build it properly once.

If capacity forces a choice, ship **A and B only** and defer C. Option A alone (even without the Opus rewrite) gets us to ~3.2s, which is a tolerable spinner duration. Streaming is the "great" version, not the "good enough" version.

## Open Questions

- **Do we keep the unary `GenExperience` RPC after shipping streaming, or replace it?**
  Recommendation: **keep both for one release cycle**, gate the client on a feature flag (`app/lib/core/config/`), and deprecate the unary version once streaming telemetry shows `client_first_event_ms` consistently < 1s and error rates are comparable. This matches the `StreamCommunityEvents` + `CommunityEvents` pattern already in the repo.

- **Remove `reasoning` entirely, or demote it to a debug-only field?**
  Recommendation: **remove entirely.** Bryan confirmed this on the issue, and the field is never read downstream — `ai.ExperienceGeneration.Reasoning` is populated and logged in `gen_ai.go` but never returned to the client and never indexed or searched. A `//go:build dev` debug variant adds complexity for no user value. If we later want LLM introspection for debugging, capture it via `logger.Debug` of the raw Anthropic response at the provider layer, not via the structured schema.

- **What call-frequency signal do we need before committing to prompt caching?**
  Recommendation: **add a lightweight structured-log counter for `GenExperience` calls per user per hour for one week** (via `logging.LoggerWithContext(ctx).Info("gen_experience_invoked", ...)`), then decide. If p50 inter-call interval > 5 min (likely, given gen_experience is a deliberate user gesture), default-TTL caching rarely hits and we'd need 1-hour paid caching. If that's the case, skip caching until we batch in more AI handlers that share the cacheable prefix. Don't build caching speculatively.

- **Should the Opus prompt rewrite be in this issue or a follow-up?**
  Recommendation: **follow-up issue, but file the eval-harness issue as a hard blocker.** The eval harness is infrastructure; the Opus rewrite is a one-time use of it. Splitting lets us land the harness (reviewable standalone) before debating prompt-quality tradeoffs. The harness PR is also a natural home for baseline golden captures.

- **What do we do for `GenRequest` and `GenExperienceFromImage` / `GenExperienceFromWebpage`?**
  Recommendation: **apply schema-reorder + `reasoning`-drop + caching to ALL Gen* methods in `provider_anthropic.go` in phase 1**, but defer streaming to Experience-from-Text initially. The other paths have different latency shapes (image mode includes image download, webpage mode includes HTTP fetch) and deserve their own measurement before streaming. Consistency of the output schema across methods is important regardless.

- **What happens on partial streaming failure — Claude succeeds but Mapbox fails?**
  Recommendation: **treat each field as independently cancellable/failable.** Emit a `GenExperienceEvent.error` variant that carries a reason code (e.g., `GEOCODE_FAILED`) and still emit the `final` event with the remaining fields populated. Client UI already handles "no geocoded location" today (falls back to `location_query` text). This matches the "fail fast on core, degrade on periphery" convention already in `gen_ai.go`.

## Implementation Plan

### Phase 1: Schema reorder + drop `reasoning` + bound value_estimate

**Goal:** Ship the highest-ROI, lowest-risk change that cuts wall-clock by ~1.5s with zero contract change.

**Tasks (revised after audit — `CommunityGeneration.Reasoning` is kept):**
- [ ] Reorder `outputSchema` in `server/ai/provider_anthropic.go:833-844` (Experience-from-text) to `title, location_query, search_keywords, date, time, time_confidence, value_estimate, mentioned_names, confidence` (drop `reasoning`).
- [ ] Remove `Reasoning` from `ExperienceGeneration`, `GearGeneration`, and `RequestGeneration` Go structs in `server/ai/provider.go`.
- [ ] Remove every `Reasoning: result.Reasoning` / `Reasoning: detection.Reasoning` assignment in `server/ai/provider_anthropic.go`, `provider_openai.go`, `provider_gemini.go`, `provider_mock.go` that targets those three struct types. (Leave `ValueEstimate.Reasoning` and `CommunityGeneration.Reasoning` alone.)
- [ ] Remove the `Reasoning string \`json:"reasoning"\`` field from every `outputSchema` struct in the three provider files for: Gear-from-text, Gear-from-webpage, Gear-from-image detection, Request-from-text, Request-from-image, Experience-from-text, Experience-from-image, Experience-from-webpage. **Do not touch** the Community-gen `outputSchema` — its `reasoning` is live contract.
- [ ] Remove the `reasoning` instruction from every prompt builder in `server/ai/prompts.go` that backs the dropped schemas (skip `buildCommunityPrompt` / equivalent). Specifically: remove the "Include brief reasoning explaining..." section in `buildExperienceFromTextPrompt:528-532`, the trailing "Return a JSON object with..." line at `prompts.go:620` (redundant with tool schema), and the equivalent in each sibling prompt.
- [ ] Bound `value_estimate.reasoning` via a prompt instruction ("≤ 10 words") and drop `value_estimate.sources` from the structured output (still logged server-side via raw response if needed).
- [ ] Lower `MaxTokens` from 4096 to 800 in all three providers' `GenerateExperienceFromText` / `GenerateGearFromText` / `GenerateRequestContent` call sites (keep community unchanged — its output is smaller anyway).
- [ ] Update `server/ai/experience_test.go` and `server/ai/provider_integration_test.go` to stop asserting/logging `generation.Reasoning` for the three dropped types.
- [ ] Update `server/services/experience/gen_ai.go` to stop copying `aiResponse.Reasoning` (currently assigned around gen_ai.go:892 in the provider — service layer doesn't read it, so this is mostly a no-op cleanup).
- [ ] Add a unit test that verifies schema field order for Experience-from-text is the intended order (fails if a future PR reverts or reorders).
- [ ] Run `npm run build` to lint + compile clean.

**Validates:** Re-run the Anthropic integration test harness documented in the baseline entry of [docs/evals/1157-genexperience-latency.md](../evals/1157-genexperience-latency.md) and add a post-Phase-1 entry there. Expect step 7 (Anthropic API call): ~3550 ms median → ~1800 ms median. Confirm no regression in output quality via manual spot-checks on the three example prompts from the issue (`"go play pickleball at 3rd shot in longmont"`, `"coffee sometime"`, `"board game night at my place friday 7pm"`).

### Phase 2: Eval harness + optional Opus prompt rewrite

**Goal:** Build reusable prompt evaluation infrastructure; opportunistically ship shorter prompts once they pass eval.

**Tasks:**
- [ ] Create `server/ai/testdata/experience_eval_goldens.json` with 20–30 hand-crafted goldens covering the distribution documented in the Opus-rewrite comment.
- [ ] Add `server/ai/prompt_eval_test.go` behind `//go:build integration` that runs each golden against the current prompt and checks per-field diffs (title soft-match, date/time/location_query exact, search_keywords Jaccard ≥ 0.6, value_estimate within 30%, mentioned_names exact set).
- [ ] Add `server/ai/cmd/prompt-eval/main.go` CLI for iterating on candidate prompts without editing code.
- [ ] (Optional, separate PR) Use Opus to rewrite `buildExperienceFromTextPrompt` with the goal of halving token count. Gate rollout on eval pass rate ≥ 95%.
- [ ] Wire the eval harness into a nightly CI integration job (don't block PRs — external API cost).

**Validates:** Harness passes on current prompt → baseline established. Harness detects a deliberate regression (e.g., remove timezone rule) → trust established. Record any quality-eval runs alongside the latency entries in [docs/evals/1157-genexperience-latency.md](../evals/1157-genexperience-latency.md) — even if timing is unchanged, track goldens pass rate and token-count deltas there.

### Phase 3: Parallelize Mapbox + stock media across Gen* handlers

**Goal:** Cut post-AI wall-clock by running the two serial I/O steps (Mapbox geocoding + stock-media fetch) concurrently in every Gen* handler that has both. Prompt caching was investigated and deferred — see Phase 3b Findings below.

**Revised task order (2026-04-21, after scope review):** The original plan listed tasks flat and assumed prompt caching would be a primary wall-clock win. After reviewing what each change affects vs. what the Phase 2 eval harness actually tests, the work is reordered so each step has a targeted baseline/measurement. The key insight: the eval harness tests step #7 (the AI call) only — it's the right regression canary for prompt caching but **not** the right tool for parallel-fan-out wall-clock (a service-handler-level change). Parallel fan-out is measured by a unit test with fake providers + controlled delays; prompt caching would be measured by `resp.Usage` token counts + `duration_ms` on the AI call.

**Phase 3b Findings (2026-04-21):** Prompt caching was implemented and tested against `claude-haiku-4-5` but did not activate — Anthropic returned `cache_creation_input_tokens: 0` and `cache_read_input_tokens: 0` on every call. [Anthropic's cache limitations docs](https://platform.claude.com/docs/en/build-with-claude/prompt-caching#cache-limitations) require a minimum cacheable prefix of **4096 tokens** for claude-haiku-4-5 / Opus 4.5+; our static system prompt is ~2100 tokens post-Phase-1, ~2000 tokens short. To meet the minimum we'd have to re-inflate the prompt with extra examples / edge cases, undoing ~half of Phase 1's schema+prompt trim win for a speculative ~100–200 ms cache-hit savings. Not worth the trade. **Phase 3b is deferred until either (a) we move to a model with a lower cache floor (Sonnet 4.5 = 1024 tokens), or (b) we naturally grow the prompt past 4096 tokens for other reasons.** The prep work (Phase 3 commit 1: `resp.Usage` logging in the Anthropic provider) stays shipped — the moment we do cross the threshold, `cache_*` fields go non-zero in prod logs with no additional plumbing.

**Scope expansion (2026-04-21):** Phase 3 originally targeted Experience only. Investigation showed Request (`GenRequest`) and Gear (`GenGear` text mode) use the identical serial Mapbox-then-stock pattern. Phase 3 now parallelizes all three. The shared concurrency primitive lives in a new `server/services/fanout` subpackage (promoted from a local helper in experience/). Community-gen has only a single post-AI I/O call (no fan-out opportunity); image/webpage modes have different shapes and are out of scope for this phase.

**Step 1 — Prep: add `resp.Usage` logging to Anthropic provider** (shipped on Phase 3a branch):
- [x] In `server/ai/provider_anthropic.go`, extend every `"anthropic API call completed"` log line with `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`. Zero values are fine pre-caching; in Phase 3b we confirmed the fields correctly surface cache activity (they stayed zero because the static prompt is under the 4096 threshold — that's the actual finding, not a plumbing issue). Standard field names per `docs/server/logging.md`.
- [x] Run eval harness against live Anthropic to capture pre-change baseline with token counts. See eval log 2026-04-21 Phase 3 prep baseline entry.

**Step 2 — Shared fanout helper** (shipped on Phase 3a branch):
- [x] Create `server/services/fanout` subpackage with `Branches` / `Results` / `Run` / `ProximityForGeocoding`. Pure concurrency primitive — takes closures, no storage or network dependencies. Unit tests cover parallel-execution (prove max not sum), nil-branch permutations, error permutations, and the proximity-bias ladder.

**Step 3 — Parallel fan-out across Gen\* handlers:**
- [x] **Experience**: `server/services/experience/gen_ai.go` — `GenExperience` runs Mapbox geocoding + stock-video-or-image fetch concurrently via `fanout.Run`. Helpers `fetchStockMediaFallback` and `geocodeViaMapbox` extracted to `gen_ai_fanout.go`.
- [x] **Request**: `server/services/request/gen.go` — `GenRequest` runs Mapbox geocoding + stock-image fetch concurrently. Helpers in `gen_fanout.go`. Note: Request's Mapbox client uses `SearchPlaces(...proximity, limit)` same as Experience.
- [x] **Gear**: `server/services/gear/gen_ai.go` — `GenGear` (text mode) runs Mapbox geocoding + stock-image fetch concurrently. Helpers in `gen_fanout.go`. Note: Gear's MapboxClient interface uses `Search(query, region)` instead of `SearchPlaces(query, proximity, limit)` — no proximity bias ladder. Gear uses the generated title as the stock-image query (Experience/Request use `search_keywords`).
- [x] Per-handler structured log span: `"post-AI fan-out completed"` with `fanout_duration_ms`, `ran_stock_*`, `ran_mapbox`, `*_hit` indicators. First-class observability for prod latency debugging.

**Step 4 — Deferred: prompt caching.** See Phase 3b Findings above. Not applicable on claude-haiku-4-5 until prompt grows past 4096 tokens or model changes.

**Validates:**
- Unit tests in `server/services/fanout/fanout_test.go` prove concurrency — two sleep closures (80 ms + 60 ms) return in ~80 ms, not 140 ms. Same test would fail with a serial implementation of `Run`.
- Per-handler logs now surface `fanout_duration_ms` in prod so latency wins are visible without redeploys.
- Eval harness (Phase 2) re-run on Phase 3a confirms AI-call behavior unchanged — fan-out is post-AI, so pass rates match prep baseline within normal LLM variance.
- Expected prod wall-clock savings on text-mode GenExperience: ~300 ms (cut serial ~1100 ms → concurrent ~800 ms). Similar for GenRequest and GenGear. End-to-end savings stacked on Phase 1's ~1600 ms: total ~5.2 s → ~3.3 s actual wall-clock across the three handlers.
- Add a Phase-3 entry to [docs/evals/1157-genexperience-latency.md](../evals/1157-genexperience-latency.md) with the Phase 3 prep baseline (token counts + durations) and notes on the Phase 3b deferral.

### Phase 4: Streaming end-to-end

**Goal:** Drop perceived latency to sub-second by streaming Claude tokens, Mapbox result, and media result to the client as they land. Apply to all three Gen\* handlers — Experience, Request, Gear — from the start, with shared primitives so each handler is a thin adapter over common streaming infrastructure, not a copy-paste.

**Branching:** Phase 4 builds directly on Phase 3's parallel fan-out. Branch 4a from `main` once Phase 3 is merged (branch name: `1157-phase4a-server-streaming`). Phase 4b branches from 4a post-merge; 4c from 4b post-merge.

**Streaming granularity note (Anthropic `input_json_delta`):** Deltas arrive ~5-40 chars each at ~20-80ms intervals — 15-50 events per second during generation. For the post-Phase-1 ~180-token response at ~100 tok/s, `title` closes at ~150ms, `location_query` at ~250-350ms, `search_keywords` at ~500-700ms. Firing Mapbox/Pexels off those key-closure events lets the fan-outs run ~1.0-1.3s earlier than today's "wait for full unary response" pattern. This is the wall-clock win that 4a's benchmark must confirm before 4b/4c commit to the proto + client work.

---

#### Phase 4a: Server-side token streaming, unary client response (all three handlers)

**Scope:** Internal-only. Server consumes streaming Claude output, fires Mapbox + Pexels as soon as trigger fields close, but still returns a single unary response to the client. No proto change, no client change. Benchmark gate decides whether 4b/4c proceed.

**Tasks — shared streaming library (`server/ai/`):**
- [ ] Add `StreamingJSONFields` tokenizer in `server/ai/streaming_json.go` — a pure state machine that consumes `input_json_delta` fragments and emits "key K completed with value V" events. Keys to watch are passed in by the caller (so Experience watches `title, location_query, search_keywords`; Request watches the same; Gear watches `title`). No Anthropic SDK coupling — just `([]byte) → []FieldEvent`.
- [ ] Table-driven unit tests in `server/ai/streaming_json_test.go`: well-formed JSON, out-of-order keys, truncated fragments, Unicode escapes, chunked mid-string, array values for `search_keywords`, object values (skipped). Pure byte-sequence input → pure event-sequence output.
- [ ] Extend `ai.Provider` with `GenerateExperienceFromTextStreaming`, `GenerateRequestFromTextStreaming`, `GenerateGearFromTextStreaming`. Each returns `(<-chan FieldEvent, <-chan FinalResult, error)` — **uniform signature across providers** so the service layer is provider-agnostic. Providers whose structured-output streaming is too coarse to win can legally emit all field events in one burst at end-of-call; the shape stays the same, the service code doesn't branch on "streaming vs unary provider."
- [ ] `AnthropicProvider`: use `client.Messages.NewStreaming` + the shared `StreamingJSONFields` tokenizer against `input_json_delta`. Per-token granularity; early-fire win applies.
- [ ] `OpenAIProvider`: use `stream=true` + the shared tokenizer against `tool_calls[i].function.arguments` deltas. Per-token granularity comparable to Anthropic; early-fire win applies. Same tokenizer — OpenAI emits the same "JSON fragments over time" shape, just wrapped in a different SDK envelope.
- [ ] `GeminiProvider`: Gemini's `streamGenerateContent` emits function-call arguments as one chunk near end-of-generation in practice (structured-output streaming is coarser than Anthropic/OpenAI). Implement streaming faithfully anyway — the tokenizer will see the single chunk, close all keys at once, and the `FieldEvent` channel emits all events in one burst just before `FinalResult`. Result: correct behavior, little/no early-fire win on Gemini. Documented limitation, not a workaround.
- [ ] `MockProvider`: scripted event timing for unit tests. Expose a helper to construct a mock that emits specific field events at specific delays.
- [ ] All four provider implementations share the same unit-test scaffolding — a provider-agnostic conformance test that verifies event ordering, terminal-event semantics, and cancellation.

**Tasks — shared streaming fan-out primitive (`server/services/fanout/`):**
- [ ] Extend the Phase-3 `fanout` package with `StreamingRun` — accepts a field-event channel and a map of `{trigger_key: branch_closure}`, fires each branch in its own goroutine the moment its trigger key closes, returns a `Results` struct compatible with Phase-3's `Run` once all branches complete. This is the shared library the user called for: Experience/Request/Gear all use it identically.
- [ ] Unit tests prove concurrency (two triggers close at 100ms + 200ms, branches each sleep 500ms → `StreamingRun` returns at ~700ms, not 1200ms), early-trigger handling (trigger closes before anyone subscribes), error propagation, context cancellation.

**Tasks — wire each handler to streaming fan-out:**
- [ ] `server/services/experience/gen_ai.go` — rewrite `GenExperience` (text mode) to use `GenerateExperienceFromTextStreaming` + `fanout.StreamingRun`. Mapbox fires on `location_query` close; Pexels fires on `search_keywords` close. Handler still builds and returns a unary `GenExperienceResponse` at the end.
- [ ] `server/services/request/gen.go` — same pattern for `GenRequest`.
- [ ] `server/services/gear/gen_ai.go` — same pattern for `GenGear` (text mode). Note: Gear triggers Pexels off `title` close (per Phase 3 notes, Gear uses title as the stock-image query, not `search_keywords`). Mapbox trigger key is the same location field as today.
- [ ] Structured logging per handler via `logging.LoggerWithContext(ctx)`: add `streaming_first_delta_ms`, `streaming_title_closed_ms`, `streaming_location_closed_ms`, `streaming_keywords_closed_ms`, `streaming_final_closed_ms` to the existing `"post-AI fan-out completed"` span. Standard field names per `docs/server/logging.md`.
- [ ] Unit tests per handler with a fake `ai.Provider` that emits a scripted field-event sequence + fake Mapbox/stock-media providers with controlled delays. Verify fan-outs fire on the right key-close events, partial-failure handling (Mapbox errors → unary response still returns with the remaining fields), context cancellation cleans up goroutines.

**Validates (Phase 4a benchmark gate):**
- Re-run `server/services/experience/gen_ai_fanout_benchmark_test.go` (Phase 3a's end-to-end benchmark) against the streaming implementation. Mirror to equivalent benchmarks for Request and Gear if they don't already exist.
- Expected: another ~1.0-1.3s wall-clock win per handler from firing Pexels/Mapbox ~1s earlier (off key-close events instead of full-response-close). Stacked on Phase 3's ~300ms: cumulative text-mode GenExperience ~3.3s → ~2.0-2.3s actual wall-clock.
- If the measured win is < 500ms, stop and investigate before committing to 4b. Possible causes: Anthropic batching deltas coarser than expected in prod, Pexels/Mapbox dominated by something other than kickoff time, schema ordering interaction with streaming.
- Add a Phase-4a entry to [docs/evals/1157-genexperience-latency.md](../evals/1157-genexperience-latency.md) with per-handler before/after timings and the streaming-key-close timestamps from the new log fields.

---

#### Phase 4b: Streaming proto + RPC surface (all three handlers)

**Scope:** Add streaming RPCs + server-side streaming handlers, sharing a common event envelope and handler-scaffolding library. No client changes yet.

**Tasks — proto (`proto/ripls/api/`):**
- [ ] Define a shared `GenStreamError` message in a common api proto (reason code + message) — used by all three event types.
- [ ] Add `rpc StreamGenExperience(StreamGenExperienceRequest) returns (stream GenExperienceEvent);` to `experience_service.proto`. Dedicated `StreamGenExperienceRequest` mirroring today's `GenExperienceRequest` (oneof text/media_id/website_url, location_id, lat/lng, current_time_unix_sec, timezone) per the CLAUDE.md "never reuse request/response types across RPCs" convention. `GenExperienceEvent.oneof event` carries: `string title_delta`, `PartialFields partial`, `GeocodedLocation geocoded`, `MediaReady media_ready`, `GenExperienceStreamFinal final`, `GenStreamError error`.
- [ ] Add `rpc StreamGenRequest(StreamGenRequestRequest) returns (stream GenRequestEvent);` with analogous dedicated request + event-oneof types.
- [ ] Add `rpc StreamGenGear(StreamGenGearRequest) returns (stream GenGearEvent);` with analogous dedicated request + event-oneof types. Gear's event oneof omits `geocoded` if Gear doesn't surface geocoded location today — verify against the current `GenGearResponse` shape.
- [ ] Run `npm run generate` and commit nothing generated — just confirm build.

**Tasks — shared streaming-handler library (`server/services/streaming/`):**
- [ ] New `server/services/streaming` subpackage with a `Handler[Req, Event]` generic helper that encapsulates the state-machine pattern: authenticate → open AI stream → route field-close events to either (a) emit an event directly to the client (e.g., `title_delta`) or (b) fire a fan-out branch whose completion emits an event (`geocoded`, `media_ready`) → on AI stream close, await remaining fan-outs → emit `final` → emit any terminal `error`. Per-handler code becomes a short adapter that supplies the field-to-event routing table and the final-builder closure.
- [ ] Unit tests for the shared helper: scripted provider + fake fan-out branches prove event ordering, partial-failure semantics (one branch errors → `error` event plus `final` with remaining fields), context cancellation, and abandoned-client cleanup.

**Tasks — per-handler streaming RPCs:**
- [ ] `server/services/experience/gen_ai_streaming.go` (new file — keeps `gen_ai.go` under the < 500-line target per `docs/server/architecture.md#file-size-guidelines`). Implements `StreamGenExperience` as a thin adapter over `streaming.Handler`.
- [ ] `server/services/request/gen_streaming.go` — same for `StreamGenRequest`.
- [ ] `server/services/gear/gen_ai_streaming.go` — same for `StreamGenGear`.
- [ ] Per-handler structured logging via `logging.LoggerWithContext(ctx)`: `claude_first_delta_ms`, `claude_stream_close_ms`, `mapbox_joined_ms`, `media_joined_ms`, `server_stream_close_ms`. Standard field names only; no PII.
- [ ] Unit tests per handler verify correct event routing against their respective event types. Shared helper's tests cover most semantics; per-handler tests focus on the routing table.
- [ ] Integration tests (behind `//go:build integration`) per handler — real Anthropic streaming endpoint, real event sequence. One canonical prompt per handler.

**Validates:** All three streaming RPCs pass unit + integration tests. `server_stream_close_ms` comparable to 4a's unary wall-clock (streaming doesn't add meaningful server overhead). No benchmark gate — 4c is where user-perceived latency is measured.

---

#### Phase 4c: Flutter streaming consumers (all three handlers) + benchmark gate

**Scope:** Subscribe to all three streaming RPCs from Flutter. Build skeleton UIs, ViewModels, disposal handling. Measure end-to-end perceived latency.

**Tasks — shared client streaming library (`app/lib/data/`):**
- [ ] New `app/lib/data/streaming/gen_stream_controller.dart` — a generic `GenStreamController<TEvent, TResponse>` that owns a `StreamSubscription`, exposes progress state, accumulates partial fields into a response builder, and handles `error`/`final` terminal events. Used by all three ViewModels; keeps per-ViewModel code to "map event to state" without re-implementing subscription lifecycle.
- [ ] Unit tests for the controller: subscription lifecycle, accumulator correctness, terminal-event handling, dispose-mid-stream safety.

**Tasks — repositories (`app/lib/data/repositories/`):**
- [ ] Add `Stream<GenExperienceEvent> streamGenExperience({...})` to `experience_repository.dart`. **Cache-bypass pass-through** — every generation is unique per prompt, no `CacheService` layer per `docs/client/caching.md`. Keep existing unary `genExperience` for back-compat.
- [ ] Add `Stream<GenRequestEvent> streamGenRequest({...})` to `request_repository.dart`. Same cache-bypass pattern.
- [ ] Add `Stream<GenGearEvent> streamGenGear({...})` to `gear_repository.dart`. Same cache-bypass pattern.

**Tasks — ViewModels (`app/lib/presentation/viewmodels/`):**
- [ ] Update `experience_creation_view_model.dart` to own a `GenStreamController<GenExperienceEvent, GenExperienceResponse>`. Regular `Notifier.autoDispose` (not `StreamNotifier`) per `docs/client/architecture.md#manual-stream-management` — these notifiers have multiple concerns beyond the stream. `ref.onDispose()` cancels the subscription; `safeUpdateState(...)` (mixin already used at `experience_creation_view_model.dart:183`) guards every post-await state write.
- [ ] Same pattern for the Request-creation ViewModel and the Gear-creation ViewModel.
- [ ] Disposal tests per ViewModel in `app/test/presentation/viewmodels/`: start a streaming call, dispose mid-stream, assert no throw and no state write after dispose. Required per `docs/client/architecture.md#disposal-testing`.

**Tasks — UI (`app/lib/presentation/`):**
- [ ] Open the preview modal immediately when generation starts (no full-screen spinner) in all three flows. Skeleton placeholders for title, time, location, and media.
- [ ] `CachedMediaImage` for streamed media once `media_ready` arrives, with stable `cacheKey` from `MediaUrl.cacheKey` per CLAUDE.md media-display rule.
- [ ] i18n — add shared ARB keys usable across Experience/Request/Gear streaming flows to **both** `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb`. Suggested: `genStreamingTitlePlaceholder`, `genStreamingLocationPlaceholder`, `genStreamingMediaPlaceholder`, `genStreamingGeocodeFailedNote`, `genStreamingCancelledMessage`. Resolve via `context.l10n.*` in widgets only (never in ViewModels) per `docs/client/i18n.md`.
- [ ] Run `npm run generate` so `flutter gen-l10n` picks up the new ARB keys.

**Tasks — config / rollout:**
- [ ] Gate streaming on a single client config flag (add to `app/lib/core/config/`) covering all three flows. Match existing env/feature-flag pattern. One release cycle of dual-path before deprecating unary `GenExperience`/`GenRequest`/`GenGear`.
- [ ] Integration tests per flow (Flutter side uses Mockito service mocks; server side behind `//go:build integration`).

**Validates (Phase 4c benchmark gate — end-to-end perceived latency):**
- Add per-ViewModel timing probes for `client_first_event_ms` (title-visible) and `client_final_event_ms` (fully-populated preview).
- Target: `client_first_event_ms` < 800ms on local dev for all three flows. Completion rates and error rates within ±2% of unary baseline.
- This is the first measurement covering all 13 steps of the end-to-end flow. Record per-event timings (`title_delta`, `geocoded`, `media_ready`, `final`) for each of Experience/Request/Gear in a single Phase-4c entry in [docs/evals/1157-genexperience-latency.md](../evals/1157-genexperience-latency.md). The perceived-latency story becomes quantified, not asserted.
- ARB keys render in both `en` and `es` locales. Disposal tests pass for all three ViewModels.

## Related Issues

- **#1143** — Proximity bias fix. Independent correctness issue; the latency measurements in this issue were captured on the #1143 branch but the work is orthogonal.
- **#1142** — Mapbox data capture. Expands the response fields Mapbox returns; doesn't change call timing.
- **#1146** — Mapbox confidence threshold. Independent correctness tuning; doesn't change call timing.
