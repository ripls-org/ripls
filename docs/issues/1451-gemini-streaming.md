# Issue #1451: Gemini streaming: flatten image-mode schemas and address structured-output buffering

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1451
**Filed:** 2026-04-27 | **Last activity:** 2026-04-27
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/evals/1157-genexperience-latency.md` (eval data referenced inline in the issue)

## Problem Statement

Gemini's image-mode streaming methods are functionally correct but
deliver near-zero wall-clock win versus their unary equivalents, while
Anthropic on the same code path delivers the ~830 ms early-fire win
the streaming architecture was designed for. Two compounding causes:

1. **Wrapped image-mode schemas (Gemini-specific for Experience).**
   `GeminiProvider.GenerateExperienceFromImageStreaming` wraps the
   result in `experiences[]`, and `DetectGearInImageStreaming` wraps
   in `gear_item`. The `StreamingJSONFields` parser only watches
   top-level keys, so `title` / `location_query` / `search_keywords`
   nested inside `experiences[0]` or `gear_item.title` never fire
   `FieldEvent`s mid-stream. Anthropic and OpenAI keep the
   experience-from-image schema flat, so they don't have this wrap on
   that method — but `DetectGearInImage` is wrapped on all three
   providers, so flattening that path is broader than just Gemini.

2. **Structured-output streaming buffers JSON until ~90% of elapsed
   time on `gemini-2.5-flash` (the current default).** Per the
   2026-04-22 eval entry: first delta lands ~88-93% into the call,
   so even with a flat schema there's almost no time left to fire
   Mapbox/Pexels concurrently. The same eval shows
   `gemini-2.5-flash-lite` already streams per-token (first delta
   ~28% in, full early-fire benefit) and is in our prod region today.
   The buffering is **model-specific**, not provider-code-specific.

The streaming-contract code path is correct; both fixes are about
the inputs the parser sees and the model's native streaming behavior.

## Context from Comments

The issue has **no comments**. The body itself is unusually
self-contained: it cites the 2026-04-22 Phase 4a benchmark with a
results table, ranks fix options, and recommends a two-PR rollout.
Treat the body's recommendation as the authoritative starting point;
this plan re-validates each step against current main and surfaces
the things the body left implicit.

## Codebase verification (2026-04-28)

Confirmed against current main:

- **Default Gemini model is `vertexai/gemini-2.5-flash`** —
  `server/ai/provider_factory.go:120` and `server/ai/provider_gemini.go:37`.
- **Gemini Experience-image schema wraps in `experiences[]`** —
  `server/ai/provider_gemini_streaming.go:289-365`. The doc-comment
  at line 289 already calls out the exact two limits this issue
  addresses, with a "flattening… is out of scope here" disclaimer
  pointing to a follow-up — i.e. this issue.
- **Gemini DetectGear streaming wraps in `gear_item`** —
  `server/ai/provider_gemini_streaming.go:246-285`. Same wrap, same
  reason, and the same comment about "out of scope here."
- **Anthropic `DetectGearInImageStreaming` ALSO wraps in `gear_item`** —
  `server/ai/provider_anthropic_streaming.go` (`outputSchema { GearItem
  *GearDetection }`). The issue body says "All three providers wrap
  `DetectGearInImage` in `gear_item`, so the gear case is uniform" —
  confirmed. **This means the gear-side schema flatten can't be
  Gemini-only without diverging providers; the choice is "flatten on
  all three" vs "leave gear_item wrap alone."**
- **Anthropic `GenerateExperienceFromImageStreaming` is flat** —
  `server/ai/provider_anthropic_streaming.go` schema has top-level
  `Title`, `Description`, etc. So the experiences[] wrap is
  Gemini-only and flattening it on Gemini just brings parity.
- **Goldens exist** at `server/ai/eval/testdata/{experience_image,
  gear_image}_goldens.json` for the eval-gate step.
- **Phase 3a Gear merged** (PR #1471) — server-side
  `streamGenGearFromImage` only emits a title FieldEvent when one
  fires. Today, since gear_item wraps on every provider, no events
  fire and only the terminal final lands. So the gear-image
  user-visible streaming win is currently zero across all providers,
  not just Gemini. Flattening gear_item on all three providers
  unblocks that across the board.

## Potential Solutions

### Option A: Recommended — model swap first, then flatten schemas (issue body's recommendation)

- **Approach:**
  1. **PR-1**: swap default Gemini model from `gemini-2.5-flash` to
     `gemini-2.5-flash-lite`. Gate on eval-harness pass-rate within
     ~5% of current model on existing goldens.
  2. **PR-2** (after ~1 week of prod observation on PR-1): flatten
     the `experiences[]` wrap on `GeminiProvider.GenerateExperienceFromImageStreaming`
     so the parser sees top-level `title` / `location_query` /
     `search_keywords` and FieldEvents fire mid-stream as Gemini
     emits per-token.
  3. **PR-3** (optional, broader scope): flatten `gear_item` wrap
     across **all three providers** for `DetectGearInImageStreaming`.
     Without this, gear-image streaming stays terminal-only on every
     provider (not Gemini-specific). With it, gear-image gets
     mid-stream title events on Anthropic + OpenAI + lite-Gemini.
- **Pros:**
  - Each PR is independently revertable.
  - Eval-gated, so quality regressions are caught before prod.
  - Sequencing PR-1 before PR-2 means PR-2's measurable benefit
    isn't masked by buffering — easier to verify the schema flatten
    actually unlocks early-fire.
  - PR-3 is the natural extension that closes the gear-image
    streaming gap referenced in PR #1471's Phase 3a wrap-up.
- **Cons:**
  - Three PRs is more bureaucracy than one.
  - The PR-2 "wait a week of prod observation" gate is a
    soft-schedule item that can slip if no one watches.
- **Effort:** small × 3.

### Option B: Single PR that ships swap + Gemini-only flatten + gear flatten across providers

- **Approach:** one PR that does all of (A.1), (A.2), and (A.3) at
  once. Eval-harness pass on all four affected schemas before merge.
- **Pros:**
  - Single review and single deploy.
  - Closes the entire Gemini-streaming gap and the gear-image
    streaming gap in one shot.
- **Cons:**
  - Reverts are coarser. If the model swap regresses on quality,
    rolling back also rolls back the schema flatten which may have
    been independently fine.
  - Bigger surface area for the eval-gate; harder to attribute
    quality changes to model swap vs. schema change.
- **Effort:** medium (combined eval coordination).

### Option C: Defer indefinitely

- **Approach:** Mark this as P4/icebox. Anthropic remains the
  primary; Gemini is a fallback path where users already accept
  degraded latency. The streaming-contract code is correct.
- **Pros:** zero cost.
- **Cons:** gear-image streaming stays at "title-only at final"
  across **all** providers because of the cross-provider gear_item
  wrap — this is a user-visible gap that the #1456 audit explicitly
  flagged for a future fix.
- **Effort:** zero.

## Recommendation

**Option B — single PR, eval-gated.** Per the engineer's call: ship
the model swap and both schema flattens together, with the eval
benchmark suite as the gate for both quality regressions and
streaming-correctness checks. Justification:

- The benefit of separating PRs is **independent revertability**.
  In practice the eval harness is a strong enough signal that we
  catch regressions before merge, not after, so the independent
  revertability has limited value here. If something does regress
  in prod after merge, a single revert reverts everything — same
  outcome as reverting two PRs in sequence, just one click.
- The benefit of combining is **one eval cycle, one review,
  one deploy**. Eval runs are not free (API spend + wall time);
  running them once across the combined change is materially
  cheaper than running twice.
- The schema flattens depend on the model swap to demonstrate the
  early-fire win. Splitting means PR-2's win is not measurable until
  PR-1 is in prod, which lengthens the validation cycle.
- Per the engineer's decision: combine and rely on the eval suite
  as both correctness gate AND regression gate. Proceeding on that
  basis.

## Open Questions

- **Is `gemini-2.5-flash-lite` a true drop-in for `gemini-2.5-flash`
  on every method we use, or only on the streaming methods
  benchmarked?** — Recommendation: run the full eval harness on lite
  before the swap (`go test -tags=benchmark ./server/ai/eval/...
  -gemini-project=ripls-dev -gemini-model=vertexai/gemini-2.5-flash-lite`);
  pass-rate within 5% of `gemini-2.5-flash` on every golden category
  (experience text/image/webpage, gear text/image/webpage, request
  text/image, community, story, suggestions, social attributes).
  If any category regresses materially, **do not ship the global
  swap**. Per-method model override capability doesn't exist today;
  adding it would be its own work item, not part of this plan.
- **Does flattening `experiences[]` to a single experience object
  regress detection quality?** — Recommendation: the wrap historically
  existed because Gemini's structured output reliability was better
  on collection schemas. Test on Vertex 2.5-flash-lite via the
  experience-image goldens (~30 cases). If pass-rate drops more than
  one case versus the wrapped baseline, keep the wrap and accept the
  no-mid-stream-events tradeoff. The 2026-04-22 eval was on
  `gemini-2.5-flash` — re-run the wrapped-vs-flat comparison on lite.
- **Should `DetectGearInImage`'s `gear_item` wrap be removed on
  Anthropic and OpenAI, or only Gemini?** — Recommendation: remove
  on all three. The wrap was added to mirror the unary path, but the
  unary path on Anthropic + OpenAI is now a drain wrapper around the
  streaming method (post-#1452), so flattening the streaming schema
  flattens the unary path along with it. Eval-gate on
  `gear_image_goldens.json` per provider. The benefit (mid-stream
  title events for gear-image on every provider, including the
  primary Anthropic) outweighs the small risk.
- **What if eval shows lite-quality is fine but the schema flatten
  regresses?** — Recommendation: revert the schema flatten only and
  ship the model swap alone. The model swap delivers the buffering
  fix on its own, and the schema flatten can be re-attempted later
  with more eval iteration. The combined-PR shape doesn't preclude
  partial rollback — just `git revert` the schema-flatten commits
  before merge if eval surfaces a regression.
- **Where does the per-PR benchmark live?** — Recommendation:
  append two entries to `docs/evals/1157-genexperience-latency.md`,
  one per PR, matching the format of the 2026-04-22 Phase 4a entry.
  Re-use that doc's benchmarking script so results are
  apples-to-apples.

## Audit findings (2026-04-28)

Reviewed against `docs/server/architecture.md` and current code. Plan
adjustments below were applied in-place after the initial draft:

- **Eval-test build tag.** The eval harness in `server/ai/eval/` uses
  `//go:build benchmark` (see `prompt_test.go`), not `integration`.
  Updated phase tasks to specify `go test -tags=benchmark
  ./server/ai/eval/...` plus the required API-key flags
  (`-anthropic-api-key`, `-openai-api-key`, `-gemini-project`)
  rather than the integration-tag form used elsewhere.
- **No per-method model override exists today.** The factory exposes
  `GeminiModelOverride` / `OpenAIModelOverride` /
  `AnthropicModelOverride` as **global** per-provider overrides, not
  per-method. The original draft suggested falling back to a
  per-method override on quality regression — that path doesn't
  exist and would require its own PR. Rewrote the PR-1 fallback to
  be honest: if any eval category regresses materially, **do not
  ship the swap**. Filing per-method override capability is out of
  scope here.
- **Drain-wrapper claim verified.** Confirmed all three providers'
  unary `DetectGearInImage` are now drain wrappers around the
  streaming method (`provider_anthropic.go`, `provider_openai.go`,
  `provider_gemini.go`). Flattening the streaming schema does
  cascade through the unary path with no extra change required.
- **File-size guidelines.** Arch doc says <500 ideal, <1000 max.
  Current sizes: `provider_anthropic_streaming.go` 749,
  `provider_openai_streaming.go` 846, `provider_gemini_streaming.go`
  591. Flattening shrinks each by ~10-20 lines (removes wrapper
  type + unwrap glue), so PR-2 nudges files in the right direction;
  no split needed.
- **AI provider is an interface-based dependency** (Pattern 2 in
  arch guide). Plan respects this — no `Provider` interface
  changes, only changes inside leaf-provider implementations and
  one service-layer field-event arm.
- **Service Independence.** Plan touches `server/ai/` and
  `server/services/gear/gen_ai_streaming.go`. The gear service
  change is a single `case ai.StreamFieldTitle` arm and does not
  cross other service boundaries.
- **i18n / client architecture / caching.** Reviewed —
  not applicable; this is a server-only change with no
  user-facing strings, no caching surface, and no client-API
  contract change. The streaming wire format is unchanged: the
  response oneof shape stays the same; clients just receive an
  earlier `title` event for the affected flows.

## Implementation Plan

### Phase 1: Single combined PR — model swap + schema flattens, eval-gated

**Goal:** In one PR, swap the default Gemini model to
`gemini-2.5-flash-lite`, flatten the `experiences[]` wrap on Gemini
Experience-image streaming, flatten the `gear_item` wrap on
`DetectGearInImageStreaming` for all three providers, and wire the
new mid-stream events through the gear service handler. Eval
benchmark suite is the merge gate — both for quality (golden
pass-rate) and streaming correctness (first-delta timing,
mid-stream FieldEvent ordering).

**Tasks:**

**Model swap**
- [ ] Change `defaultVertexModel` in
      `server/ai/provider_factory.go:120` to
      `vertexai/gemini-2.5-flash-lite`.
- [ ] Change the fallback default in
      `server/ai/provider_gemini.go:37` to match.

**Gemini Experience-image schema flatten**
- [ ] In `server/ai/provider_gemini_streaming.go:306-365`, replace
      `outputSchema { Experiences []experienceItem }` with a flat
      schema mirroring the Anthropic
      `GenerateExperienceFromImageStreaming` shape (top-level
      `title`, `description`, `confidence`, `search_keywords`,
      `date`, `time`, `time_confidence`, `location_query`,
      `value_estimate`).
- [ ] Update the result-handling block to read fields directly
      instead of `result.Experiences[0]`. Drop the "no experiences
      detected in image" error branch (replaced by an empty-title
      check on the flat result).
- [ ] Update the doc-comment at line 289 to remove the
      "flattening… is out of scope here" disclaimer and document
      the new flat schema as the streaming source of truth.

**`DetectGearInImage` schema flatten — all three providers**
- [ ] `server/ai/provider_anthropic_streaming.go`:
      `DetectGearInImageStreaming` — replace `outputSchema { GearItem
      *GearDetection }` with a flat schema mirroring the
      `GearDetection` field set, marshal directly into
      `*GearDetection` for the terminal final.
- [ ] `server/ai/provider_openai_streaming.go`: same flatten.
- [ ] `server/ai/provider_gemini_streaming.go:259`: same flatten.
      Drop the `result.GearItem` indirection.
- [ ] Verify `provider_{anthropic,openai,gemini}.go` unary
      `DetectGearInImage` are unchanged drain wrappers. They
      should not need updates because they read
      `final.Result` (a `*GearDetection`) which is now produced
      directly from the flat schema. **Note:** confirmed in the
      audit step that all three are drain-wrappers; this is a
      regression-canary check, not a code change.

**Service-layer wiring**
- [ ] In `server/services/gear/gen_ai_streaming.go`,
      `streamGenGearFromImage` already has a
      `case ai.StreamFieldTitle` arm but it never fires today
      because of the `gear_item` wrap. After the schema flatten,
      verify the arm is correct (it is) and add a `case
      ai.StreamFieldDescription` arm to emit description
      mid-stream too — same pattern as the gear webpage handler.
      This closes the wrap-up note from PR #1502 about
      gear-image being the one remaining terminal-only flow.

**Eval benchmark — combined gate**
- [ ] Run the full eval suite on the combined change:
      ```
      go test -tags=benchmark ./server/ai/eval/... \
        -anthropic-api-key=$ANTHROPIC_API_KEY \
        -openai-api-key=$OPENAI_API_KEY \
        -gemini-project=ripls-dev
      ```
      All categories, all providers. The same run gates both quality
      (golden pass-rate must be within 5% of pre-PR baseline) and
      streaming-correctness (first-delta timing, mid-stream
      FieldEvent ordering on the affected schemas).
- [ ] Compare results to the 2026-04-22 Phase 4a baseline
      already in `docs/evals/1157-genexperience-latency.md`. If
      either Gemini-image quality regresses materially or
      first-delta timing fails to drop from ~89% → ~28% on Gemini
      lite, **do not ship**. Either revert the model swap and ship
      schema flatten alone, or revert both.
- [ ] Append a new dated entry to
      `docs/evals/1157-genexperience-latency.md` with the
      side-by-side numbers (lite vs. previous flash, flat vs.
      wrapped) for each affected schema.

**Unit tests**
- [ ] Existing `TestStreamGenGearFromImage_EventOrdering` in
      `server/services/gear/gen_ai_streaming_test.go` already
      scripts a title FieldEvent into the mock — verify it still
      passes (mock provider behavior is unchanged; only the
      real-provider schemas changed).
- [ ] Add a new mock-stream test case that scripts both `title`
      and `description` FieldEvents and asserts the gear-image
      handler emits both on the wire before final.
- [ ] Update `provider_{anthropic,openai,gemini}_streaming_test.go`
      (or add tests if missing) to confirm
      `DetectGearInImageStreaming`'s flat schema parses correctly
      and produces a non-nil `*GearDetection` on the terminal
      final.

**Build**
- [ ] `npm run build` clean.
- [ ] `go build -tags=integration ./...` clean.
- [ ] `go test ./server/ai/...` (unit tests, no benchmark tag)
      clean.

**Validates:**

- Quality is preserved on lite (eval pass-rate within tolerance).
- Streaming correctness: first-delta timing improves on Gemini
  (~89% → ~28% of total call), and mid-stream FieldEvents fire on
  the formerly-wrapped paths.
- Gear-image gets mid-stream title + description events on all
  three providers — closes the last terminal-only Gen* flow.
- Single revert, single deploy, single eval cycle.

## Related Issues

- **#1229** — Image+webpage streaming. PR #1452 (Phase 1) landed the
  streaming contract that this issue addresses the Gemini-specific
  gap of. PR #1471 (Phase 3a Gear) wired the image-mode handler
  expecting mid-stream events that don't fire today; PR-2 of this
  plan is what makes them fire.
- **#1456** — Gen* preview modal streaming UX gaps (description
  mid-stream + skeleton coverage). Closed by PR #1502. The closing
  note explicitly flagged gear-image as the one remaining
  terminal-only flow because of the wrapped-schema issue tracked
  here. PR-2 of this plan is the resolution.
- **#1216** — Re-evaluate AI provider model defaults using Phase 4a
  benchmark findings. PR-1 of this plan is essentially a tactical
  subset of #1216 specifically for Gemini, and the eval-harness
  artifact produced by PR-1 should be referenced from #1216.
- **#1157** — Parent latency-optimization issue.
- **`docs/evals/1157-genexperience-latency.md`** — 2026-04-22
  Phase 4a entries (the source-of-truth benchmark this plan
  references; PR-1 and PR-2 each append a new entry).
