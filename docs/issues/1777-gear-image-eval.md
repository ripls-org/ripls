# Issue #1777: eval: 10x the image-mode gear goldens with human-curated ground truth, drop low-signal fields

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1777
**Filed:** 2026-05-07T19:22:08Z | **Last activity:** 2026-05-07T19:42:51Z
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** none

> **Update 2026-05-08:** The maintainer's original direction was to seed cases from Amazon Berkeley Objects (ABO). A trial ingest revealed ABO is, by design, Amazon's house-brand catalog — 88% of the sampled items were "AmazonBasics," with the remainder also being Amazon-owned brands. That makes brand-detection assertions degenerate (a model can pass 88% of brand checks by always answering "AmazonBasics" without seeing the image), and `model_contains` mostly resolves to internal SKU codes vision models won't reproduce. The plan pivoted to **interactive ingest from manufacturer product URLs**: a curated list of ~30–50 URLs across diverse brands gives us the brand variety ABO can't, and Schema.org Product JSON-LD on every modern e-commerce page yields the same structured fields we needed (title, brand, model, weight, USD price, hero image). The ABO ingest script was deleted; `server/cmd/eval-seed-from-urls` replaces it. Sections below have been updated; the legacy ABO-specific text appears as struck-through context in §2 and §codebase-research for future reference.

## Problem Statement

The image-mode gear-detection eval at `server/ai/eval/testdata/gear_image_goldens.json` has only 4 cases, all clean catalog-style photos of well-known branded products (REI Half Dome tent, KitchenAid mixer, Rocky Talkie radio, Atomic Maverick ski). Combined with wide tolerances (50–60% on weight and value), low confidence floors (0.5), `title_contains_mode: "any"` over multiple substrings, and a single global pass threshold of 0.85 (`prompt_test.go:68`), the suite is saturated — every credible vision model passes, so it can't rank providers or detect prompt regressions.

The original issue body proposed expanding to ~40 cases across 8 categories (including 8 deliberately tricky/failure-prone shots), adding a model-proposal helper script, dropping low-signal `category_contains_any_of` assertions, and tightening tolerances. The maintainer's follow-up comment narrowed scope:

1. Seed ~50 cases from **Amazon Berkeley Objects (ABO)** only — skip the hand-shot tricky bucket and skip the `eval_propose_golden.go` model-proposal helper.
2. **Leave `category_contains_any_of` alone** — no need to drop or downweight.
3. **Test across many models per provider, not just one default model** — this is a new requirement not present in the original body.

The remaining structural change is the per-(provider, model) threshold gate, which the comment did not contradict and which is needed regardless of golden-set size in order to compare models in a single invocation.

## Context from Comments

There is one comment, from the issue author (maintainer, MEMBER), posted ~20 min after filing:

> I think we should start just by scraping ~50 examples from ABO that have golden data. We can skip the other model-proposal/hand-labeling script for now. We can leave category alone/no need to downweight.
>
> Let's make sure we can test across many models, not just the default model per-provider.

This narrows scope sharply. Out:
- The hand-shot tricky bucket of 8 distribution-shift cases.
- Wikimedia Commons / press kits / Pexels as imagery sources.
- The `scripts/eval_propose_golden.go` model-proposal helper.
- Dropping or restructuring the `category_contains_any_of` assertion.
- Tolerance tightening as primary work (may still happen as natural fallout when ABO ground truth supplies tighter values).

In, beyond what the body already covered:
- Multi-model support per provider. The harness today takes one `-anthropic-model` / `-openai-model` / `-gemini-model` value per invocation; we need to be able to run the same suite against, e.g., Haiku, Sonnet, and Opus in one go.

## Codebase research

**Current eval harness** (`server/ai/eval/`):
- `prompt_test.go` is gated behind `//go:build benchmark` and skipped in CI / default `go test`. Each `Test*Prompt` function reads goldens, picks one provider via `-provider`, loops cases, records `Check`s into a single `Tally`, and gates on the global `promptEvalMinPassRate = 0.85`.
- `eval.go` defines `Tally` (per-field counters), `Report` (lines + total/overall rate), and the assertion helpers (`containsCheck`, `numericWithinTolerance`, etc.).
- `gear_image.go` defines `GearDetectionGoldens` / `GearDetectionCase` / `GearDetectionExpected` and `EvaluateGearDetection` which produces one `Check` per asserted field.

**Provider construction:**
- `NewAnthropicProvider(apiKey, model, temperature)` — one provider, one model.
- `NewOpenAIProvider(apiKey, model, temperature)` — same.
- `NewGeminiProvider(ctx, model, project, location, temperature)` — same; project-scoped credentials.

To run multiple models per provider in one invocation, we must construct one `ai.Provider` per (provider type, model) pair.

**Image fixtures:** today live directly in `server/test_data/` alongside non-eval test data (`books_exif.jpg`, `experience.jpg`, etc.). 4 image files for the current 4 cases. ABO will add ~50; a subdirectory keeps them organized.

**ABO dataset facts** (from public docs):
- Released by Amazon under CC BY 4.0.
- "Listings" file: `abo-listings.tar.gz` — JSON-Lines metadata per item (brand, model, dimensions, weight, retail price when available, color, country, item-name in multiple locales).
- "Images" file: `abo-images-original.tar` — full-resolution photos keyed by `image_id`.
- Categories are coarse but include enough for our purposes: home appliances, lawn/garden, sports/outdoors, tools (light), electronics, kitchen.
- Not every item has weight or retail price populated — filtering for items that do is part of the ingest script's job.

**Out-of-scope confirmation:**
- `category_contains_any_of` stays per the comment. The original concern (free-text category isn't load-bearing) is real but doesn't block the eval expansion; can revisit later.
- The 8 tricky distribution-shift cases are dropped from this issue. We accept that the resulting eval will reward catalog-photography-shaped uploads more than real user uploads. If that turns out to matter empirically, a follow-up issue can add hand-shot cases later.

## Potential Solutions

### Option A: Sequential phases — harness first, then ABO ingest

- **Approach:** Phase 1 ships the multi-model + per-(provider, model) threshold harness with the current 4 cases. Phase 2 ships the ABO ingest script and the ~50 new cases. Phase 3 (post-merge) tunes thresholds based on observed pass rates.
- **Pros:** Each phase is independently mergeable and verifiable. Phase 1 immediately lets us start ranking models on the existing 4 cases — useful even before ABO lands. ABO ingest script can be developed without touching the harness.
- **Cons:** Two passes through the eval setup. Phase 1's discriminative value is limited until Phase 2 lands (4 cases is too few to rank confidently).
- **Effort:** medium total; small Phase 1, medium Phase 2.

### Option B: All-in-one PR

- **Approach:** Land harness changes + ABO ingest + ~50 new cases together.
- **Pros:** Single review pass, everything works on day one.
- **Cons:** Large PR, harder to review. Mixes infrastructure (harness refactor) with content (ABO data). Bisects worse if something regresses.
- **Effort:** medium.

### Option C: ABO ingest first, harness changes second

- **Approach:** Phase 1 lands ~50 ABO-seeded cases against the current single-model-per-provider harness with the current 0.85 gate. Phase 2 retrofits multi-model support afterwards.
- **Pros:** Discriminative value lands earliest — the existing harness is already running, so 50 cases on it immediately gives useful per-provider signal.
- **Cons:** Without the per-pair threshold map, an expanded suite that fails a single weak provider blocks every benchmark run. We'd need to set the global gate low to accommodate the weakest model, which defeats the discrimination goal. Also delivers the "test many models" requirement last, which is the maintainer's explicit ask.
- **Effort:** medium.

## Recommendation

**Option B — single PR landing the multi-model harness, the ABO ingest script, and the ~50 seeded cases together.** Per maintainer direction. The work is well-scoped enough that a single review pass is feasible, and shipping it together avoids an interim state where the harness can run multi-model but the goldens are too sparse to discriminate.

Tradeoffs we accept:
- The PR is larger than ideal (~harness refactor + ingest tool + ~50-case JSON expansion + ~10MB of fixture images). Mitigation: structure the commits so the harness change, the ingest script, and the goldens addition are separable in `git log` even though they ship in one PR.
- Bisecting a future regression to either the harness change or the goldens change is harder. Mitigation: keep the multi-model harness commits first in the series so the suite is exercised against the old 4 cases before the new ones land in a later commit.
- Threshold tuning still happens *after* this PR merges (we won't have the variance data to tune until the suite has run a few times). Initial thresholds default to 0.85 across the board; a follow-up PR tunes them.

## Open Questions

- **Multi-model CLI flag syntax** — Recommendation: change each `-{provider}-model` from `flag.String` to a comma-separated list parsed via a small `flag.Var` implementation, named `-{provider}-models` (plural). Default for each provider is its current single value to preserve existing invocations. Example: `-anthropic-models=claude-haiku-4-5,claude-sonnet-4-5,claude-opus-4-7`. Comma-separated keeps the recipe block in `server/ai/README.md` short; repeated flags would force every recipe to grow.

- **Threshold map storage** — Recommendation: a package-level `var modelThresholds = map[string]float64{ ... }` in `prompt_test.go`, keyed by `"{provider}/{model}"` (e.g. `"anthropic/claude-haiku-4-5"`). Lookup falls back to a documented default of `0.85`. A small map keeps the change reviewable; we can always lift it to JSON later if it grows past ~20 entries. Initial values: leave every key absent (i.e., everything defaults to 0.85) and tune in the follow-up after Phase 2's data lands.

- **Per-pair test attribution** — Recommendation: each `Test*Prompt` becomes `t.Run(providerName, func(t *testing.T) { for model := range models { t.Run(model, ...) } })`. This produces failure paths like `TestGearImagePrompt/anthropic/claude-haiku-4-5/rocky_talkie/title_contains` that are easy to grep in CI logs.

- **Where to put ABO images** — Recommendation: `server/test_data/abo/<abo_item_id>.<ext>`. Subdirectory keeps the 50 ABO photos separate from the existing 4 hand-curated fixtures and from non-eval test data. The image-file path stored in each golden case becomes `abo/<id>.<ext>`. Update `gear_image.go`'s test loader if it currently assumes flat paths (it doesn't — `filepath.Join(testDataDir, c.ImageFile)` already supports subdirectories).

- **ABO image size and repo cost** — Recommendation: resize ABO images on ingest to ≤1024 px on the long edge (preserving aspect ratio) before committing. ABO originals can be 2000+ px. 50 images at ~150–250 KB each = ~10 MB total — acceptable for a test-fixture set, comparable to existing `experience.jpg` etc. Document the resize step in the ingest script so future seeds match.

- **CC BY 4.0 attribution** — Recommendation: add an `abo_item_id` field to each ABO-sourced golden case (e.g., `"abo_item_id": "B07X1YH..."`), and add a credits paragraph to `server/ai/eval/README.md` and `server/test_data/README.md` (the latter already exists per the IDE-opened-file context). The item ID acts as the attribution link back to ABO and as the ground-truth provenance pointer.

- **ABO data fetch mechanics** — Recommendation: don't vendor the ABO listings tarball into the repo; instead, the ingest script downloads `abo-listings.tar.gz` from the public S3 URL into a temp dir on each run, filters JSON lines, fetches the chosen images, and writes them under `server/test_data/abo/`. This keeps the repo small. The script is invoked manually (not in CI), so a re-fetch on rerun is fine.

- **Selecting which 50 ABO items** — Recommendation: filter for items that have at least *brand*, *item_weight*, and *retail_price_in_lcyusd* populated (so we can assert all three high-signal fields), then proportionally sample across the categories present in our current 4 cases plus power-tools, kitchen, sports/outdoors. Drop categories where ABO has fewer than 5 items meeting the filter. The ingest script logs the resulting category breakdown so we can spot-check before committing.

- **Cost of a full benchmark run** — Recommendation: a 50-case run × 6 models (2 per provider) is ~300 image API calls. At rough current pricing this is single-dollar territory, well within tolerance for a manual benchmark. We don't need to optimize. The `-mode=image` flag already lets developers iterate cheaply on a single mode.

- **What if ABO is missing weight or retail price for the items we want?** — Recommendation: when a field is absent in ABO metadata, omit the corresponding assertion from that case's `expected` block (the harness already treats missing assertions as "not checked"). Don't fall back to model-proposed values — the whole point of ABO is third-party ground truth. Document the empty-field policy in the ingest script's README section.

- **How do we keep the ingest reproducible?** — Recommendation: the ingest script accepts an optional `-seed=<int>` flag and uses it for any random sampling; the chosen 50 items' IDs are recorded in a manifest committed alongside the goldens. Re-running ingest with the same seed reproduces the same 50 items. Future expansion just bumps the count and rerun.

## Implementation Plan

Single PR. Within the PR, structure commits so the work is reviewable in stages — harness first (still useful against the existing 4 cases), then ingest tooling, then the goldens expansion. This keeps `git log` readable and makes future bisects tractable even though everything ships together.

Recommended commit sequence inside the PR:

### Commit 1: Multi-model + per-(provider, model) harness

**Goal:** Restructure the eval harness so a single invocation can run all 7 suites against multiple (provider, model) pairs, with independent per-pair thresholds and skip-on-missing-credentials behavior. Standalone-correct against the existing goldens.

**Tasks:**

- [ ] Replace each `-{provider}-model` `flag.String` with a `-{provider}-models` flag-Var that parses a comma-separated list. Defaults preserve the current single-value behavior so existing recipes still work.
- [ ] Introduce a `providerModel` struct `{ Name string; Model string; Provider ai.Provider }` and a `providersForEval(t)` helper that returns one entry per (provider, model) pair whose API-key credentials are set. Pairs with missing credentials are silently omitted (not test failures).
- [ ] Refactor each `Test*Prompt` function to wrap the existing case loop in `for _, pm := range providersForEval(t) { t.Run(pm.Name+"/"+pm.Model, ...) }`. Each pair gets its own `Tally` and report.
- [ ] Add `var modelThresholds = map[string]float64{}` keyed by `"{provider}/{model}"`. Add `func thresholdFor(key string) float64` returning the map value or `defaultPromptEvalPassRate = 0.85` fallback. Document the defaulting in a comment.
- [ ] Update `reportAndGate` to take the (provider, model) key and gate against `thresholdFor(key)` instead of the single global constant. Log the threshold in the report header.
- [ ] Delete `promptEvalMinPassRate` constant.
- [ ] Repurpose `-provider` flag as a filter — empty/`all` (new default) runs every credentialed pair; an explicit value restricts to that provider only. Preserves the iterate-on-one-provider workflow.
- [ ] Update `server/ai/README.md` benchmark recipes for the new flag names and behavior.
- [ ] Add a per-pair section to `Report` so logs clearly delineate where one pair ends and the next begins.
- [ ] **Split `prompt_test.go` to stay under the 1,000-line ceiling.** Today it sits at 567 lines; the multi-model refactor (`providerModel`, multi-provider construction, flag-Var parser, threshold map) will push it well past the limit set by `docs/server/architecture.md` (target <500, max <1,000). Suggested split: `prompt_test.go` keeps only the `Test*Prompt` functions; `provider_setup_test.go` holds flag definitions, the comma-separated flag-Var, `providerModel`, `providersForEval`, and the per-provider constructors; `threshold_test.go` holds `modelThresholds`, `thresholdFor`, and `defaultPromptEvalPassRate`; `report_test.go` holds `reportAndGate`, `requireMode`, and `retryOnRateLimit`. All files keep the `//go:build benchmark` tag so they remain CI-skipped.
- [ ] **Add unit tests (no `benchmark` tag) for the new helpers.** Per `docs/server/architecture.md` §1.4 ("no new functionality without test coverage") and the existing pattern in `gear_image_test.go` (which unit-tests `EvaluateGearDetection` without a build tag). Cover: the comma-separated flag-Var (parses, deduplicates, rejects malformed input); `thresholdFor` (returns map value, falls back to default, handles missing key); `providersForEval` filtering (skips uncredentialed providers, returns expected pair count). These tests run in default `go test` and don't need API keys.

**Validates at this commit boundary:** the harness still runs cleanly against the existing 4 image + 13 text gear cases and produces one report per (provider, model) pair instead of one global report. Default `go test ./server/ai/eval/` passes (new unit tests included). `go build -tags=benchmark ./server/ai/eval/` succeeds.

### Commit 2: ABO ingest script

**Goal:** Add the tool that turns ABO listings into proposed golden cases, without yet adding any cases.

**Tasks:**

- [ ] Add `scripts/eval_seed_from_abo.go`. Responsibilities:
  - Download `abo-listings.tar.gz` to a temp dir if not already present (cache by checksum).
  - Parse JSON-Lines, filter for items with brand, item_weight, and retail_price set.
  - Sample N items per target category (power tools, hand tools, kitchen/appliances, camping/outdoor, sports/fitness, lawn/garden, electronics) with reproducible seed.
  - Download chosen images, resize to ≤1024px long edge, write to `server/test_data/abo/<abo_item_id>.<ext>`.
  - Emit a JSON block per case to stdout (or a target file) with `id`, `tags`, `image_file`, `mime_type`, `abo_item_id`, and an `expected` block populated from ABO metadata: `title_contains` (from item-name tokens), `brand_contains`, `weight_grams_approx`, `value_estimate_usd`, plus `min_confidence: 0.5` and `has_description: true`. Material category and tolerances use defaults; reviewer tightens.
  - Write a manifest (`server/ai/eval/testdata/gear_image_abo_manifest.json`) recording the seed, the ABO listings checksum, and the chosen item IDs for reproducibility.
- [ ] **Fail-fast error handling throughout.** Per `CLAUDE.md` ("never silently ignore errors or use warn-and-continue patterns"), every download / parse / image-decode error returns from the script with a non-zero exit code; do not skip-and-continue past a bad item silently. Logging the failure plus the offending item ID is fine; the next run with a different seed will work around it.
- [ ] **If using concurrent image downloads, every `go func()` needs a recover defer.** Per `docs/server/architecture.md` "Goroutine Safety". The CI checker (`server/cmd/check-goroutines`) exempts `_test.go` files but not `scripts/`. Either keep downloads serial (recommended given N ≈ 50 — wall-clock cost is small) or wrap each goroutine with the inline-recover pattern from `server/services/streaming/sender.go`. Document the choice in the script's package comment.
- [ ] Document the ingest workflow in `server/ai/eval/README.md`: when to run, how to add categories, how to re-seed.
- [ ] Add a credits paragraph to `server/ai/eval/README.md` and `server/test_data/README.md` covering CC BY 4.0 attribution and the `abo_item_id` field as the attribution pointer.

**Validates at this commit boundary:** running the script produces a JSON block of proposed cases plus a populated `server/test_data/abo/` directory, but neither the goldens file nor the harness's case count has changed. `npm run build` passes (the script lives under `scripts/`, but if it's a Go package it gets compiled by `go build ./...` which is part of `npm run build`).

### Commit 3: Run ingest, hand-review, append ~50 cases

**Goal:** Land the goldens expansion. This is the only commit that changes `gear_image_goldens.json` and `server/test_data/abo/`.

**Tasks:**

- [ ] Run the ingest script with a recorded seed. Hand-review the proposed cases (sanity-check titles, brands, that the chosen image actually shows the item, no obvious miscategorizations). Reject items that don't look like recognizable consumer gear; rerun with a different seed if rejection rate is high.
- [ ] Append the accepted ~50 cases to `gear_image_goldens.json`. Keep the 4 existing hand-curated cases.
- [ ] Commit `server/test_data/abo/` images and the populated manifest.
- [ ] Verify the harness reads an `image_file: "abo/<id>.<ext>"` path correctly. Update `gear_image_test.go` if a path-format assumption breaks (it shouldn't — `filepath.Join(testDataDir, c.ImageFile)` already supports subdirectories).

**Validates at this commit boundary:** `TestGearImagePrompt` runs ~54 cases per (provider, model) pair. A single benchmark invocation against multiple models produces a report with measurable spread between them.

### Verification before merge

- [ ] `npm run build` passes (golangci-lint + `go build ./...`).
- [ ] `go build -tags=benchmark ./server/ai/eval/` passes — `npm run build` does not exercise the `benchmark` build tag, and the harness lives behind it.
- [ ] `go test ./server/ai/eval/` passes (default suite — exercises the new no-tag unit tests for the flag-Var, threshold map, and `providersForEval` helpers).
- [ ] One end-to-end manual benchmark run against at least two providers, e.g. `go test -tags=benchmark -timeout 30m -run TestGearImagePrompt ./server/ai/eval/ -anthropic-models=claude-haiku-4-5,claude-sonnet-4-5 -anthropic-api-key=...`, confirming each (provider, model) pair produces its own report section and gates on its own threshold (defaulting to 0.85).

### Follow-up phases (each its own PR after this one lands)

Three follow-ups are queued. Each depends on the expanded golden set and the multi-model harness landing first; the order below is the recommended sequencing.

#### Follow-up 1: threshold tuning

Run the full benchmark suite three times against each (provider, model) pair to characterize variance, then set each pair's threshold to roughly `mean - 1.5 × stddev` rounded to the nearest 0.01. Document the run that established each threshold in a comment next to the `modelThresholds` map entry. Validate the gate by deliberately reverting a known prompt improvement and confirming the threshold trips on the affected pair.

#### Follow-up 2: mine ABO for discriminative cases

Once threshold tuning has produced a baseline run across multiple models, the same per-pair tally data we use to gate the suite tells us which cases are *discriminative* and which are *saturated*. A discriminative case is one where models disagree — e.g. four out of six pairs pass every assertion but two pairs fail two checks each. A saturated case is one where every pair passes every check (currently true of all 4 hand-curated cases). The discriminative cases are the ones worth keeping and oversampling; the saturated ones add wall-clock and cost without ranking signal.

The follow-up:

1. Add an opt-in `-stats-out` flag to the harness that, when set, writes a JSON record per (case, pair) with the per-field check results plus the case's tags. A run with N pairs × M cases produces N×M records.
2. Add `scripts/eval_score_discrimination` (or a sibling under `server/cmd/`) that ingests the stats JSON and produces a per-case discrimination score: e.g. the variance across pairs of the per-case overall pass rate, weighted up by the number of pairs sampled. Output sorted: most discriminative first, fully-saturated cases at the bottom.
3. Use the report to drive a *targeted* second-pass ABO ingest: for the top-K discriminative `product_type` buckets, sample more items in the same buckets (`-product-types=...` flag on `eval-seed-from-abo`) and append them to the goldens. For the bottom-K (everyone-passes) buckets, mark them `tags: ["saturated"]` in the goldens so we can keep them as smoke tests but exclude from the headline pass-rate by default.
4. Document the iterate-on-discrimination workflow in `server/ai/eval/README.md` so prompt revisions can re-run the analysis and catch newly-saturated buckets.

The end state is a self-correcting golden set: as models get better and saturate cases, the discrimination signal points us at where to add more.

#### Follow-up 3: per-pair latency in the eval report

The harness logs `duration_ms` per API call (provider JSON logs) and Go's test framework prints per-subtest durations, but the `Tally` / `Report` doesn't aggregate timing — so the per-pair eval report has pass rates but no median/p95 latency. This matters because reasoning-class models (Opus 4.7, gpt-5.5) are 10–30× slower than their non-reasoning siblings, and a model that's marginally more accurate but 30× slower may not be the right choice.

The follow-up:

1. Extend `Tally` (in `server/ai/eval/eval.go`) with a `durations []time.Duration` slice that subtests append to alongside their `Check` recordings.
2. Surface median + p95 + max in `Report.Lines`. Format like `latency: median 2.8s, p95 6.4s, max 9.1s`.
3. The per-call JSON logs continue to be the source of truth for ad-hoc analysis; the report just summarizes for the in-loop benchmark workflow.
4. Update threshold-tuning recipe (Follow-up 1) to consider median latency alongside pass rate when picking pair thresholds.

Out of scope: a strict latency gate (e.g., "fail if median > 10s"). Pass-rate gates are the primary correctness signal; latency is an information-only column for now.

#### Follow-up 4: enumerate the latest models per provider

Today the `-{provider}-models` flag defaults to a single model name baked into `provider_setup_test.go`. To get the broadest possible cross-model view in a single benchmark run, we want the default to be a curated list of "latest models across sizes" per provider — e.g. Anthropic's current Haiku + Sonnet + Opus, OpenAI's current nano + mini + large, Gemini's current Flash + Flash-Lite + Pro. The list rotates as model lines update, so we need a maintenance story.

Two viable shapes:

- **Curated list in code with a documented refresh cadence.** Add a `currentModelDefaults` map (keyed by provider name) listing the small/medium/large model IDs we consider "current," with a top-of-file comment specifying the review cadence (e.g. quarterly) and a CHANGELOG note in `server/ai/eval/README.md` whenever the list rotates. Default `-{provider}-models` reads from this map. Simple, no runtime cost, but requires human upkeep.
- **Runtime enumeration via each provider's models-list API.** Anthropic has `GET /v1/models`, OpenAI has `GET /v1/models`, Vertex has `models.list`. We could fetch at test-start and pick the latest small/medium/large by a heuristic (model name prefix + release date). Always fresh, but adds a network call + a credential dependency just to list, and the "size tier" classification is a heuristic the API doesn't expose directly.

Recommendation: **start with the curated list** (Option A) since it's simpler and our model-rotation cadence is slow enough that monthly review is fine. Revisit Option B if the curated list goes stale faster than expected.

The follow-up:

1. Add `currentModelDefaults` map in `provider_setup_test.go`, keyed by provider name, valued by an ordered list of model IDs covering small/medium/large of the current generation. Include a comment naming the canonical refresh source (Anthropic docs page, OpenAI API reference, Vertex AI model catalog).
2. Change each `-{provider}-models` default from a single value to the corresponding entry in `currentModelDefaults`.
3. Add a `scripts/check_eval_model_defaults` linter (or a no-op test that just logs) that fetches each provider's current model list and warns if the defaults are missing a model that's been released for >90 days. Optional but reduces drift.
4. Update `server/ai/README.md` recipes to mention that the default benchmark run now sweeps multiple models per provider, with a corresponding cost/wall-clock note (`50 cases × 9 pairs ≈ 450 calls per image suite, ~$1–2 per full run`).
5. Coordinate with Follow-up 2: when discriminative-mining picks up new ABO cases, immediately re-run the benchmark against the broader default model set so threshold-tuning data stays per-pair-current.

## Related Issues

- #1157 — original prompt-eval harness motivation (referenced from `server/ai/README.md`); this issue extends the harness pattern to multi-model.
- (none filed yet) — follow-up for hand-shot tricky/distribution-shift cases, deferred per the maintainer's comment. Worth filing if the ABO-only set turns out to over-represent catalog-style photography in a way that diverges from real user uploads.
- (none filed yet) — follow-up for revisiting `category_contains_any_of` assertions if `Gear.category` becomes load-bearing in the UI (e.g., filtering, search). Deferred per the maintainer's comment.

## Audit notes

Audited against `docs/server/architecture.md` and `docs/server/observability.md`. The plan's scope is server-side only: a benchmark-tag-gated test harness in `server/ai/eval/` and a developer-only ingest script under `scripts/`. None of the work ships in the running server binary or in CI's default test pass, so most runtime-observability conventions (structured logging via `logging.LoggerWithContext`, request-ID propagation, sensitive-data masking, `connecterr.Internal`) do not apply. Storage-layer and Connect-handler conventions also don't apply. Client-side docs (caching, i18n, accessibility, ViewModel testing) are out of scope.

Findings folded into the plan above:

- **File-size ceiling.** `prompt_test.go` is 567 lines today; the multi-model refactor would push it over the 1,000-line ceiling that `docs/server/architecture.md` and CLAUDE.md set. Added an explicit task in Commit 1 to split the file into `prompt_test.go` (Test\* functions only) + `provider_setup_test.go` + `threshold_test.go` + `report_test.go`, all keeping the `//go:build benchmark` tag.
- **Test coverage for new helpers.** Per the architecture doc's "no new functionality without test coverage" rule, added a Commit 1 task to write no-build-tag unit tests covering the comma-separated flag-Var, `thresholdFor` defaulting, and `providersForEval` credential-skip filtering. These run in default `go test` without API keys, mirroring `gear_image_test.go`'s existing pattern.
- **Goroutine safety in the script.** Per the goroutine-safety section: any concurrent download in the ingest script must use the recover-defer pattern from `server/services/streaming/sender.go`, since `scripts/` is not exempted by `server/cmd/check-goroutines`. Added a Commit 2 task; recommendation is to keep downloads serial since N≈50 makes parallelism unnecessary.
- **Fail-fast error handling in the script.** Per CLAUDE.md, the ingest script must not warn-and-continue past errors. Added a Commit 2 task to enforce non-zero exit on download/parse/decode failures.
- **Build verification.** Added a "Verification before merge" sub-section with explicit `npm run build`, `go build -tags=benchmark`, and `go test` steps, plus a manual end-to-end benchmark run to confirm the per-(provider, model) gating behavior end-to-end.

No contradictions with existing architecture decisions surfaced. The Connect-Go service-independence rules and storage-layer conventions are untouched by this work.
