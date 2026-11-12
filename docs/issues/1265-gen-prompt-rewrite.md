# Issue #1265: Opus-driven prompt rewrite for Gen* methods (gated on Phase 2 eval)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1265
**Filed:** 2026-04-24 | **Last activity:** 2026-07-08
**Labels:** none (Issue Type: Feature, Priority: P4) | **Assignees:** unassigned
**Context docs:** auto-selected from `docs/llms.txt`: `docs/server/llm.md`, `docs/server/conventions.md`, `docs/server/architecture.md` (structure/testing sections), `docs/users/priorities.md`, `docs/server/private_llm_eval.md` (eval-suite context); plus `server/ai/README.md`, `server/ai/eval/README.md`, `docs/issues/1777-baseline-2026-05-09.md`, `docs/issues/1216-provider-model-defaults.md` as prior art. No context docs passed as input.

> **Directives from the requester (2026-07-08):** perform the rewrite with **Fable 5 in-session** (not Opus via API), iterate to **minimize latency and token count**, and **be conservative wherever eval coverage is too thin to confidently validate a trim**. Human co-work is available for eval-data curation — "it's worth the effort to get good evaluation data where we need it" — so golden expansion should be planned as a collaboration, not skipped for cost. Additionally: **fold in #1209** (search_keywords proper-noun bias) and close it with this rewrite if still relevant — assessment below.

## Audit Notes (2026-07-08)

Ran `/ripls-audit` against this plan. Scope: server-only (`server/ai/**` prompt strings, eval harness, golden data; no proto/RPC, storage, or client changes — `api_design` and `accessibility` dimensions not engaged). Fixes applied in-place:

- **Benchmark-tag compile blind spot** (testing): CI compiles `-tags=integration` but never `-tags=benchmark`, so eval-harness changes (negative assertion, report fields) can break `prompt_test.go` compilation invisibly. Added `go vet -tags=benchmark ./server/ai/...` to Phases 2 and 4 — the same trap `docs/server/conventions.md` documents for integration-tagged code.
- **PII in captured page bodies** (security): real event/product pages embed organizer names/emails/phones; added a scrub step to Phase 3 before goldens are committed.
- **Goldens `version` bump** (testing): `BaselineReport.GoldensVersion` is the field that keeps cross-run comparisons honest; Phase 3 now bumps it per expanded file.
- **Acceptance-gate granularity** (testing): "within 2 pp" is finer than single-assertion resolution on ~10-case suites; restated as "no more than one additional assertion failing" there.
- **Private-LLM baseline comparability** (architecture): `docs/server/private_llm_eval.md` benchmarks self-hosted models on these same prompts/goldens; the rewrite supersedes its 2026-05-19 baselines. Noted in Related Issues and the Phase 5 report task; the private models' documented weak fields (location, keyword breadth) reinforce the per-field gate.
- **Code-quality alignment**: no plan-phase labels in source comments (`lint:ephemeral-comments` fails the build); if `prompts.go` remains >1,000 lines post-trim, split by domain in a content-free follow-up commit (Phase 4).

Docs consulted: `docs/llms.txt`; anchors `docs/server/architecture.md`, `docs/server/conventions.md`, `docs/users/priorities.md`, `docs/users/personas.md` (skimmed); rubrics `docs/reviews/{testing,performance,code_quality,security}_review_prompt.md` ("Areas to Review"); leaf docs `docs/server/llm.md`, `docs/server/private_llm_eval.md`, `server/ai/README.md`, `server/ai/eval/README.md`; prior art `docs/issues/{1777-baseline-2026-05-09,1216-provider-model-defaults}.md`. `proto_conventions.md` not engaged (no proto surface).

## Problem Statement

`server/ai/prompts.go` (now 1,775 lines) holds every Gen* prompt, shared across the Gemini/Anthropic/OpenAI providers. Several prompts have accumulated redundant instruction layers, 3–4 verbose worked examples where 2 tight ones suffice, and closing "Return a JSON object with…" recitations that partially duplicate the structured-output schema. Every extra input token raises TTFT (prefill) and per-call cost on every Gen* request — the file's own header documents a measured case where ~200 extra input tokens slowed gpt-5-mini TTFT by 25% (~3s).

The issue proposed a strong-model rewrite pass gated on the eval harness. Since it was filed (April 2026), the landscape has changed materially — the plan below is grounded in the current state, not the issue text:

- **The eval harness matured well past "Phase 2"** (#1212 → #1777): nine suites (7 Gen* modes + community + unified-create classifier), per-(provider,model) pass-rate thresholds (`eval/threshold_test.go`), and machine-readable baseline reports with **median/P95 latency** via `-eval-report-out` (`eval/baseline_report.go`), archived under `docs/ai/eval/baselines/`.
- **Two prompts already got this treatment**: `buildGearDetectionPrompt` was rewritten ~35% shorter with a measured 4–5 pp pass-rate *gain* (Phase 6 of #1777) after its golden set was 10×'d to 49 cases, and `buildCommunityGenerationPrompt` was collapsed to keywords-only (#2547). Those are the template for how the remaining prompts should be handled — and evidence that trimming with adequate eval coverage is safe.
- **Production models changed** (#1216, closed): primary is `vertexai/gemini-3.1-flash-lite`, fallback `claude-haiku-4-5`, tertiary `gpt-5-mini`. Rewrites must be validated against these pairs, not just Anthropic.
- **Output schemas were consolidated** (#1226, closed), so schema-vs-prompt redundancy can now be assessed against a single source of truth per method.

**The central tension** — and the reason for the requester's caution — is that eval coverage is inversely correlated with prompt bloat:

| Prompt builder | ~lines | Suite | Golden cases | Trim confidence |
|---|---|---|---|---|
| `buildExperienceFromImagePrompt` | ~235 (worst bloat) | experience image | **2** | LOW |
| `buildGearFromWebpagePrompt` | ~160 | gear webpage | **3** | LOW |
| `buildExperienceFromTextPrompt` | ~150 | experience text | 26 | HIGH |
| `buildExperienceFromWebpagePrompt` | ~125 | experience webpage | **2** | LOW |
| `buildGearGenerationPrompt` | ~100 | gear text | 13 | MEDIUM-HIGH |
| `buildRequestGenerationPrompt` | ~65 | request text | 12 | MEDIUM-HIGH |
| `buildGearDetectionPrompt` | done (#1777) | gear image | 49 | — |
| `buildCommunityGenerationPrompt` | done (#2547) | community | 8 | — |

The biggest token wins sit exactly where a regression would currently be invisible. A 2-case suite cannot distinguish a safe trim from a broken one — passing 2/2 after deleting the flyer-vs-scene decision tree proves nothing.

Prompts with **no eval coverage at all** (conversation/experience summaries, stories, nudges, plan-tab suggestions, social attributes, informal-time parsing) are out of scope for rewriting; see Open Questions.

### #1209 (search_keywords proper-noun bias): mostly resolved, not verifiably closed

#1209 documented six 2026-04-21 baseline failures where Gen* prompts returned venue/city proper nouns ("3rd Shot", "Longmont", "Brainard Lake") as `search_keywords`, degrading stock-media search. Current state:

- The six problem goldens still exist (`explicit_pickleball_longmont`, `outdoor_camping_july4`, `contextual_yoga_rec`, and the three request cases), and the 2026-05-19 cloud baseline shows keyword fields passing at 86–100% across the production trio — the keyword sections were rewritten in the #1216-era commits (`7381011ee`, `a12117fcb`) with "MUST include the primary activity/item" plus "Skip personal names, specific dates/times, and hyper-niche identifiers" guidance, and the assertions moved from Jaccard to `must_contain` + `general_matches`.
- **But the harness has no negative assertion.** `must_contain`/`general_matches` prove the generic terms are *present*; a list like `["pickleball", "3rd Shot", "Longmont"]` still passes while still polluting the Pexels/Unsplash query. The issue's original success criterion (Jaccard ≥ 0.3) no longer exists, and the current guidance names "personal names" but not venue/city/place names explicitly.

So #1209's fix is ~shipped but its harm case is untested. This plan closes it properly in Phase 2: explicit venue/city anti-proper-noun guidance in the rewritten keyword sections, plus a small `search_keywords_must_not_contain` assertion in the harness set on the venue-heavy goldens — giving #1209 an unambiguous, durable close criterion under the current assertion scheme.

### Latency levers, ranked honestly

1. **Input tokens → TTFT.** Prefill cost scales with input size. This matters most for the streamed unified-create UX (user watches fields appear) and for slower providers. Expected win: ~1,300 → ~600–800 input tokens on the worst prompts (~40–50%), consistent with the gear-detection precedent.
2. **Output tokens → total wall-clock.** Generation is ~20–100 tok/s; output dominates end-to-end latency. Levers: tighter length caps that already exist (≤10-word reasoning, one-sentence description) plus removing any *requested-but-unused* output fields. Caveat: `value_estimate.reasoning` feeds Provenance records (`docs/provenance.md`) — it must stay. One concrete mismatch to reconcile: `buildGearGenerationPrompt` instructs "10. REASONING: 1-2 sentences" but its own closing field list (and possibly the schema) omits `reasoning` — either the instruction or the omission is wrong; both cost something.
3. **Prompt caching: not a lever here — close this thread from the issue.** The Anthropic minimum cacheable prefix for `claude-haiku-4-5` is **4,096 tokens**; these prompts are ~1,300 tokens *before* trimming and Anthropic is only the fallback provider. Additionally, per-request content (user input, `CURRENT TIME`) is interpolated near the top of most prompts, so there is no stable prefix. Restructuring static-content-first is cheap hygiene (helps Gemini implicit caching marginally) but must not gate or complicate the rewrite.

### Removing the schema-duplication tail — a per-provider caveat

The issue flags closing lines like `"Return a JSON object with: title, date …"` as pure redundancy (~40 tokens/call). That is true for OpenAI (native strict structured outputs) and Gemini (Genkit schema enforcement), but the Anthropic provider enforces schema **via prompting/forced-tool-use** — the tail may be load-bearing there. Removal is allowed only when the eval passes on the Anthropic pair specifically.

## Context from Comments

One comment: the 2026-04-24 triage pass (P4 icebox, "gated on Phase 2 eval; not actionable yet"). The gate has since been more than satisfied — the harness exists, has thresholds, latency reporting, and two successful precedent rewrites. No maintainer disagreements; the requester's 2026-07-08 directive (Fable in-session, latency+tokens, thin-eval caution) supersedes the issue's "Prompt Opus via API" mechanics but not its intent.

## Potential Solutions

### Option A: Full rewrite of all six remaining prompts now, against existing goldens
- **Approach:** Rewrite text + image + webpage prompts in one pass; gate everything on the current suites (26/13/12/2/2/3 cases).
- **Pros:** Fastest path to the full ~40–50% token reduction across all flows; one review cycle.
- **Cons:** Directly violates the thin-eval caution. The three most-bloated prompts would be validated by 2–3 cases each — statistically meaningless. A regression in flyer extraction or product-page price parsing would ship undetected and surface as user-visible bad generations.
- **Effort:** Medium.

### Option B: Coverage-first, staged rewrite (recommended)
- **Approach:** (1) Rewrite the three **text-mode** prompts now — 26/13/12 golden cases is real coverage and the eval-gated workflow is proven. (2) Expand the thin suites (experience-image 2→≥10, experience-webpage 2→≥10, gear-webpage 3→≥10) following the #1777 methodology and `eval/README.md` style notes — webpage goldens embed `page_title`/`page_description`/`page_body` inline, so new cases are cheap to author; image cases need curated flyer/scene images. (3) Only then rewrite the image/webpage prompts, gated on the expanded suites.
- **Pros:** Honors the caution exactly where it applies while still shipping the well-covered wins immediately. Golden expansion is a zero-production-risk PR with lasting value (it also hardens the #1216-style model-swap workflow). Matches the proven #1777 sequence: expand goldens → baseline → rewrite → gate.
- **Cons:** Three PRs instead of one; the biggest single prompt (`buildExperienceFromImagePrompt`) lands last. Experience-image golden curation requires sourcing ~10 flyer/scene images with hand-verified ground truth — the slowest part.
- **Effort:** Medium-large overall, but each stage is small and independently shippable.

### Option C: Text rewrite + mechanical-only trims on thin-coverage prompts
- **Approach:** Same step (1) as Option B; for image/webpage prompts do only human-verifiable mechanical trims (duplicate `VALUE_ESTIMATE` blocks, repeated location-rule expansions, schema-tail lines already covered by enforced schemas on Gemini/OpenAI) without restructuring, validated by the existing 2–3 cases as a smoke test. Skip golden expansion.
- **Pros:** Zero new test-data work; captures maybe half the token win on the bloated prompts at near-zero quality risk.
- **Cons:** Leaves the largest structural bloat (redundant example blocks, triple-stated extraction rules in the 235-line image prompt) in production indefinitely; "mechanical-only" discipline is hard to hold in review; still under-validated on Anthropic where the schema tail may matter.
- **Effort:** Small-medium.

## Recommendation

**Option B.** It is the only option consistent with the requester's directive: aggressive where the eval data supports it (text modes, 51 cases across three suites), and coverage-building before touching what it can't yet validate. The #1777 precedent shows the expand-goldens-then-rewrite loop works and even *improves* pass rates. If experience-image golden sourcing stalls, degrade that one prompt to Option C's mechanical-trim treatment rather than blocking the rest — the webpage suites (inline text, no images) have no such excuse.

Concretely, the rewrite pass itself (done by Fable in-session, per directive) follows the discipline already codified in the `prompts.go` header comment: ≤2 examples per steering point, no example overlap with golden inputs, prose never restating what an example shows, measure token delta per change, trim in the same diff that adds.

## Open Questions

- **What acceptance gate does a rewritten prompt need?** — Recommendation: on each gating (provider, model) pair, pass rate ≥ the pair's `modelThresholds` entry **and** within 2 pp of that pair's same-day pre-rewrite baseline (re-run baseline in the same session to control for drift) — for the smaller suites (~10 cases ≈ 50–80 assertions), read "2 pp" as "no more than one additional assertion failing", since single-assertion granularity is ~1.3–2 pp there; median latency and per-field pass rates not visibly regressed. Record both runs under `docs/ai/eval/baselines/` with `pre-rewrite`/`post-rewrite` labels, mirroring the `promptrev1` convention from #2057.
- **Which pairs gate adoption?** — Recommendation: the production trio — `gemini/vertexai/gemini-3.1-flash-lite` (primary), `anthropic/claude-haiku-4-5` (fallback), `openai/gpt-5-mini` (tertiary). Flash-Lite + Haiku are mandatory; treat gpt-5-mini as advisory (tertiary, and its `modelThresholds` entry doesn't exist — default 0.85 floor applies).
- **How do we measure the token reduction itself?** The baseline reports capture latency but not token counts. — Recommendation: don't plumb usage capture into the eval harness for this. Add a tiny default-suite unit test/dev helper that renders each prompt builder with fixed inputs and reports character counts; record before/after counts in the PR description and final baseline doc. Character delta ≈ token delta at the precision needed. This doubles as the **size-ratchet test** in Phase 5: per-prompt character budgets set just above the post-trim size, so future prompt growth fails CI without any paid API call — directly serving the issue's "re-run eval on any prompt edit going forward" intent at zero cost.
- **Nightly eval CI job (issue step 5)?** — Recommendation: **defer to a follow-up issue.** Every run costs paid API tokens across three providers, and this repo has already had one CI billing incident (#2626). The size-ratchet test plus the documented "re-run the suite when touching prompts.go" workflow (`server/ai/README.md`) covers the regression risk cheaply; a scheduled or label-triggered eval workflow can be designed separately with explicit cost controls.
- **Experience-image golden sourcing.** — Recommendation: ~10–12 cases split between flyer-type (event posters with explicit date/time/location text) and scene-type (photographed, like the existing hand-curated gear images), following `eval/README.md`'s attribution rules. The requester has offered co-work on eval data, which is the right split here: **Claude prepares the scaffolding** (case JSON skeletons with proposed `expected` blocks, a checklist of desired coverage — flyer/recurring-flyer/scene-food/scene-sports/scene-outdoors/unclear — and candidate public-domain flyer sources), **human supplies/verifies** the images and ground truth (photos, real flyers, sanity-check of dates/times a vision model should extract). Same collaboration applies to webpage-suite realism: humans can nominate real event/product pages worth capturing. Mechanical-only fallback for the image prompt remains the escape hatch only if curation stalls despite this.
- **Do untested prompts get touched at all?** — Recommendation: no rewrites. One exception to flag for reviewer decision: the shared `toneAndStyle` constant contains a typo ("parantheses") and is embedded in *every* generation prompt including the evaled ones — fix it in Phase 2 where the eval gates it. Summaries/stories/nudges/suggestions keep their current prompts; if we ever want to trim them, goldens come first (same principle).

## Implementation Plan

### Phase 1: Baselines + measurement scaffolding
**Goal:** Reproducible pre-rewrite reference for quality, latency, and size.
**Tasks:**
- [ ] Materialize eval credentials per `server/ai/README.md`; run all six target suites (`-mode=text` and the image/webpage suites) against the production trio with `-eval-report-out=docs/ai/eval/baselines/<date>_<sha>_pre-rewrite`.
- [ ] Add a default-suite prompt-size helper (renders each Gen* builder with fixed inputs, reports char counts); capture pre-rewrite sizes.
- [ ] Verify the `buildGearGenerationPrompt` REASONING-vs-schema mismatch against `schemas.go`/provider structs; note the resolution for Phase 2.
- [ ] Commit baselines + helper (no prompt changes).
**Validates:** Current pass rates/latency per pair are known and archived; any later regression is attributable.

### Phase 2: Rewrite text-mode prompts (experience, gear, request)
**Goal:** ~40–50% input-token reduction on the three highest-volume, best-covered prompts with zero eval regression.
**Tasks:**
- [ ] Rewrite `buildExperienceFromTextPrompt` (~1,300 → target ≤800 tokens): collapse 4 examples → 2 (disjoint from goldens), merge the triple-stated location guidance into the shared `locationExtractionRules` reference, deduplicate time-inference prose, keep `withWeekday` grounding and TONE SELECTION (past/future) logic intact.
- [ ] Rewrite `buildGearGenerationPrompt`: fold the 12-value `material_category` guidance to the compact form already proven in `buildGearDetectionPrompt`; resolve the REASONING mismatch; keep placeholder bans.
- [ ] Rewrite `buildRequestGenerationPrompt`: tighten value-estimate branch prose (the header comment documents its earlier trim — finish the job).
- [ ] Fix the `toneAndStyle` typo; attempt schema-tail removal, keeping it if the Anthropic pair regresses.
- [ ] **#1209 close-out:** extend the keyword guidance to name venue/city/place names explicitly (the current "personal names … hyper-niche identifiers" wording misses the documented failure mode); add a `search_keywords_must_not_contain` negative assertion to the harness (default-suite unit tests included) and set it on the venue-heavy goldens (`explicit_pickleball_longmont`, `outdoor_camping_july4`, `contextual_yoga_rec`, and the three request cases from #1209).
- [ ] Re-run the three text suites on the gating pairs; iterate (expect 2–3 rounds); save `post-rewrite-text` baseline reports; record char/token deltas.
- [ ] `npm run build` **plus** `go vet -tags=benchmark ./server/ai/...` — CI compiles neither `benchmark`- nor prompt-eval-tagged test code, so harness/assertion changes need an explicit local compile check (same trap as the `-tags=integration` rule in `docs/server/conventions.md`).
- [ ] PR with before/after table (pass rate, median/P95 latency, size per pair). Keep prompt-file comments free of plan-phase labels (`scripts/check_ephemeral_comments.js` fails the build on them) — reference `#1265`/`#1209` instead.
- [ ] On merge, close #1209 with a pointer to the negative assertions and the passing baseline.
**Validates:** The Fable-driven rewrite loop holds quality on well-covered prompts — the confidence basis for Phase 4 — and #1209's harm case becomes permanently tested rather than assumed fixed.

### Phase 3: Expand thin golden suites
**Goal:** Enough coverage on image/webpage modes to gate their rewrites (test-data-only PR, zero production risk).
**Tasks:**
- [ ] Draft the coverage checklist per suite and share with the requester (co-work kickoff — see Open Questions for the division of labor).
- [ ] `experience_webpage_goldens.json`: 2 → ≥10 (Eventbrite/Meetup/venue/recurring/ambiguous non-event pages; inline `page_body` text, no fetching; requester nominates real pages worth capturing).
- [ ] `gear_webpage_goldens.json`: 3 → ≥10 (product pages across price-visible/sale-price/review-article/spec-heavy variants; reuse `eval-seed-from-urls` where helpful).
- [ ] `experience_image_goldens.json`: 2 → ≥10 (flyers with explicit ground truth + scene photos — human-supplied/verified images, Claude-drafted case skeletons); respect attribution rules.
- [ ] Apply `eval/README.md` calibration style (broad `title_contains` synonym sets, `any` mode) to avoid the Haiku-vs-Gemini style trap fixed in #1800.
- [ ] Scrub PII from captured `page_body` text before committing (real event/product pages carry organizer names, emails, phone numbers — replace with plausible fictional values; ground truth is unaffected).
- [ ] Bump the `version` field in each expanded goldens file so `BaselineReport.GoldensVersion` distinguishes pre-/post-expansion runs (community goldens already model this at v2).
- [ ] Run expanded suites against the gating pairs; recalibrate `modelThresholds` if observed rates sit >3 pp from current gates; save `post-goldens` baselines.
**Validates:** Thin suites become discriminative (a broken trim now fails); refreshed baselines for Phase 4.

### Phase 4: Rewrite image/webpage prompts
**Goal:** The largest single reductions, now safely gated.
**Tasks:**
- [ ] Rewrite `buildExperienceFromImagePrompt` (~235 lines): unify the FLYER/SCENE branches' duplicated title/description/time/location requirements into one requirements block with per-branch deltas; 3 examples → 2; merge the stray duplicated section numbering (two "4." blocks) and the trailing VALUE_ESTIMATE block into the main flow.
- [ ] Rewrite `buildGearFromWebpagePrompt` (~160 lines): 3 examples → 2; collapse extraction prose that repeats `buildGearDetectionPrompt` field semantics.
- [ ] Rewrite `buildExperienceFromWebpagePrompt` (~125 lines): same treatment; fix its 5→7 section-numbering gap.
- [ ] Re-run expanded suites on gating pairs; iterate; save `post-rewrite-media` baselines; record deltas.
- [ ] `npm run build` + `go vet -tags=benchmark ./server/ai/...`; PR with before/after table. (If experience-image goldens were deferred: mechanical trims only for that builder, noted in the PR.)
- [ ] If `prompts.go` still exceeds the project's <1,000-line file-size preference after the trims, split by domain (the `prompts_unified.go` / `prompts_hero_card.go` precedent) in a mechanical follow-up commit — `git mv`-style history preservation doesn't apply to intra-file moves, so keep the split diff free of content edits.
**Validates:** Full-scope token reduction with regression protection everywhere it ships.

### Phase 5: Guardrails, docs, close-out
**Goal:** Lock in the wins; make future prompt growth visible without paid CI.
**Tasks:**
- [ ] Turn the Phase 1 size helper into a ratchet test: per-prompt char budgets set ~10% above post-trim size; growth past budget fails the default suite with a pointer to the eval workflow.
- [ ] Update `server/ai/README.md` (baseline-label conventions, ratchet test) and keep `docs/server/llm.md` prose true to the new prompt shapes (nightly doc-freshness will verify).
- [ ] Write the consolidated before/after report under `docs/ai/eval/baselines/` (per-pair pass rate, median/P95 latency, input-size deltas per prompt), noting that pre-rewrite baselines — including the 2026-05-19 private-LLM runs from #2057 — are superseded for cross-run comparison.
- [ ] File the follow-up issue for a scheduled/on-demand eval CI job with explicit cost controls; file cleanup issues for anything deferred (e.g. experience-image goldens if skipped).
- [ ] Close #1265 with the report linked.
**Validates:** Regression protection is permanent and free; documentation matches reality.

## Appendix: Phase 3 golden-expansion coverage checklist (co-work kickoff)

Target ≥10 cases per suite. Claude drafts case-JSON skeletons with proposed `expected` blocks; human supplies/verifies images, nominates real pages, and sanity-checks ground truth. Check off as cases land.

**experience_webpage (2 → ≥10)** — inline `page_body` text, PII-scrubbed:
- [ ] Ticketed concert/show page (explicit date+time+venue+price) *(existing: concert_event)*
- [ ] Recurring meetup page ("first Thursday of each month" — date resolution)
- [ ] Free community event (library/parks listing, no price)
- [ ] Multi-day festival (date range)
- [ ] Class/workshop with registration deadline vs event date (disambiguation)
- [ ] Online/virtual event (location_query must stay empty)
- [ ] Venue landing page with NO specific event (low confidence, empty date)
- [ ] Past-dated event page (date before CURRENT TIME)
- [ ] Sparse page: title + one date line only
- [ ] Non-event product/blog page (confidence floor case)

**gear_webpage (3 → ≥10)** — product pages across pricing shapes:
- [ ] Big-box product page, visible regular price *(existing: dewalt-style)*
- [ ] Sale price + strikethrough MSRP (prefer regular/MSRP)
- [ ] Review/roundup article, no direct price (estimate from type)
- [ ] Spec-heavy page with weight in lb/oz (unit conversion)
- [ ] Marketplace listing (used pricing)
- [ ] Bundle/kit listing (value of the bundle, not one part)
- [ ] Fabric/soft-goods item (material_category: fabric)
- [ ] Petrol tool page (material_category: petrol_tool)
- [ ] Page with price range ("from $199")
- [ ] Non-product page (confidence floor case)

**experience_image (2 → ≥10)** — human-supplied/verified images (co-work):
- [ ] Clean event flyer: explicit date, time, venue *(existing)*
- [ ] Recurring flyer ("Every Saturday 9am" — next-occurrence resolution)
- [ ] Flyer with partial info (date but no time)
- [ ] Dense/cluttered poster (multiple text blocks, one real event)
- [ ] Food/dinner scene *(existing)*
- [ ] Sports/recreation scene (court, field, equipment)
- [ ] Outdoor/nature scene (trailhead, park)
- [ ] Social gathering scene (game table, party setup)
- [ ] Ambiguous scene (should stay generic, UNKNOWN time)
- [ ] Non-event image (empty result / low confidence)

Rules for all: inputs disjoint from prompt examples; `expected` blocks use broad `title_contains` synonym pools + `any` mode; venue-carrying cases get `search_keywords_must_not_contain`; bump each file's `version`; scrub real names/emails/phones from captured text; image attribution per `eval/README.md`.

## Related Issues
- #1209 — search_keywords proper-noun bias (open, P3); mostly fixed by the #1216-era keyword rewrites but unverified — this plan closes it in Phase 2 via explicit venue/city guidance + a new `must_not_contain` assertion.
- #1157 — parent perceived-latency issue (shipped); origin of this track.
- #1212 — eval harness Phase 2 (merged); the original gate.
- #1777 — gear-image goldens 10× + gear-detection prompt rewrite (closed); the methodology precedent this plan replays for the remaining modes.
- #1216 — provider/model defaults re-evaluation (closed); defines the production trio used as gating pairs.
- #1226 — Gen* schema consolidation (closed); prerequisite for judging schema-vs-prompt redundancy.
- #2057 — private-LLM eval (closed); introduced the `promptrev1` baseline-label convention reused here. **Comparability caveat:** `docs/server/private_llm_eval.md` benchmarks self-hosted models against these same prompts and goldens; after this rewrite the 2026-05-19 private baselines are no longer comparable to fresh cloud runs — any future private-vs-cloud decision must re-run both sides on the post-rewrite prompts/goldens. The private models' documented weak spots (location-field omission, search-keyword breadth) are also the fields most sensitive to trimming — one more reason the per-field pass rates in the acceptance gate matter.
- #2547 — community generation reduced to keywords-only (merged); removed that prompt from this issue's scope.
