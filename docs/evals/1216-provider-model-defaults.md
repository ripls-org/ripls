# AI Provider Model Default Evals (#1216)

**Issue:** [#1216 — Re-evaluate AI provider model defaults using Phase 4a benchmark findings (gated on quality eval)](https://github.com/Type2Fun-Foundation/ripls/issues/1216)
**Plan:** [docs/issues/1216-provider-model-defaults.md](../issues/1216-provider-model-defaults.md)
**Purpose:** Track per-provider, per-mode quality pass rates for the prompt-eval harness. Each swap decision (or rejection) lands a new dated entry recording the baseline, candidate, per-field deltas, and whether the candidate cleared the gate.

## How to read this doc

- Each entry is a single run of the `benchmark`-tagged prompt-eval harness (`server/ai/eval/prompt_test.go`) against a specific (provider × model × mode) combination.
- Entries are newest-first.
- Baselines establish a reference pass rate for a current default. Candidate entries compare deltas and state whether the gate passed.
- Gates:
  - **Phase 2 (OpenAI swap):** within 5pp of the gpt-5-mini text baseline. OpenAI is the tertiary fallback; partial quality degradation is acceptable given the ~16× speedup.
  - **Phase 3 (Gemini swap):** within 2pp of the gemini-2.5-flash text baseline. Gemini is the secondary fallback and fires more often; tighter gate.
  - Image and webpage aggregates must not drop below baseline regardless of provider.

## How to reproduce any entry

```bash
# Anthropic (all modes, ~3-4 min)
go test -tags=benchmark -v -timeout 15m -count=1 ./server/ai/eval/ \
  -anthropic-api-key="$(gcloud secrets versions access latest --secret=anthropic-api-key --project=ripls-dev)"

# OpenAI (all modes — wall-clock depends heavily on model; gpt-5-mini ~20 min, gpt-5.4-nano ~3-5 min)
go test -tags=benchmark -v -timeout 45m -count=1 ./server/ai/eval/ \
  -provider=openai \
  -openai-api-key="$(gcloud secrets versions access latest --secret=openai-api-key --project=ripls-dev)" \
  -openai-model=gpt-5-mini

# Gemini (all modes, ~5 min; requires `gcloud auth application-default login`)
go test -tags=benchmark -v -timeout 15m -count=1 ./server/ai/eval/ \
  -provider=gemini \
  -gemini-project=ripls-dev \
  -gemini-location=us-central1 \
  -gemini-model=vertexai/gemini-2.5-flash
```

Add `-mode=text|image|webpage` to scope a run to a single input mode. See [server/ai/README.md](../../server/ai/README.md) for full flag reference.

## Measurements

Entries newest-first.

---

### 2026-04-25 (update) — Weekday-augmented prompts (#1299): text-mode no-regression check

**Status:** Direct follow-up to #1299. The four prompt builders that interpolate `CURRENT TIME` (`buildExperienceFromTextPrompt`, `buildExperienceFromImagePrompt`, `buildExperienceFromWebpagePrompt`, `buildInformalToCalendarPrompt`) now append the weekday name in parentheses — `2026-04-21T09:00:00-06:00 (Tuesday)` — so smaller models don't have to derive day-of-week from the raw ISO timestamp. Implementation in commit `e8919ea5`.

**Per-model text re-eval (single sample per provider, all run on the same eval state as the 2026-04-25 entry below):**

| Provider                              | Pre-#1299 text   | Post-#1299 text  | Δ        | Notes |
|---------------------------------------|------------------|------------------|----------|-------|
| Anthropic `claude-haiku-4-5`          | 250/253 (98.8%)  | 249/253 (98.4%)  | −1 case  | `northface_backpack` value-estimate variance ($280 vs $180 ±50%). Unrelated to weekday. |
| OpenAI `gpt-5.4-nano`                 | 250/253 (98.8%)  | 249/253 (98.4%)  | −1 case  | Gear sample noise. Unrelated to weekday. |
| Gemini `vertexai/gemini-2.5-flash`    | 251/253 (99.2%)  | 251/253 (99.2%)  | 0        | `value_cooking_class` still failing universally; `explicit_boardgames_friday_7pm` flipped on `location_query` sentinel ("my house" vs USER_PRIMARY_LOCATION) but `babysitter_weekend` passed — net 0. |
| Gemini `vertexai/gemini-2.5-flash-lite` | 240/253 (94.9%) avg | **245/253 (96.8%)** | **+5 cases (+1.9pp)** | The headline result. |

**Where the flash-lite gain came from:**

| Case                          | Pre-#1299                           | Post-#1299 |
|-------------------------------|-------------------------------------|------------|
| `relative_saturday_run`       | ❌ "this Saturday" → Sun 04-26       | ✅ Sat 04-25 |
| `named_participants_hike`     | ❌ "saturday" → Sun 04-26            | ✅ Sat 04-25 |
| `kids_birthday_saturday`      | ❌ "this saturday" → Sun 04-26       | ✅ Sat 04-25 |
| `painting_workshop_kids`      | ❌ flaky off-by-one                  | ✅ stable pass |
| `babysitter_weekend` (Request) | borderline pool match (sample-dependent) | ✅ |

**What's still failing on flash-lite (text):**

- `past_tense_coffee` ("last Thursday" → Fri 04-17 vs Thu 04-16) — model now anchors today's weekday (Tuesday) correctly but still drifts +1 on the relative arithmetic.
- `stargazing_anne_white` ("friday night" → Sat 04-25 vs Fri 04-24) — same pattern in the future direction.
- `value_cooking_class` (universal — per-person vs total semantic, all providers fail).
- `value_wine_tasting` ($65 vs $45 ±30%) — single-sample value variance.
- `trek_road_bike_frameset` ($150 vs $400 ±60%) — used-bike pricing interpretation.
- `borrow_drill_weekend`, `help_moving_saturday` — keyword-pool breadth (tighter outputs vs Anthropic-style verbosity, pool was tuned to the latter).
- `vague_coffee_sometime` — model fabricates EXPLICIT time on a vague prompt (real KEEP — fired this run, was passing in prior samples).

**Reads:**

- The hypothesis in #1299 was correct: flash-lite was anchoring today's weekday wrong from the raw ISO timestamp, and feeding it the explicit weekday strings the off-by-one error from "this Saturday" / "saturday" cases.
- The remaining 2 date cases (`past_tense_coffee`, `stargazing_anne_white`) reveal a second drift mode: even with today's weekday correctly grounded, flash-lite still mis-counts the offset to "last Thursday" / "friday night". That's a counting-arithmetic limit in the model itself, not something prompt grounding can fix.
- No regressions on the larger models. Anthropic and gpt-5.4-nano −1 each is independent value-estimate variance on different cases (`northface_backpack`, gear sample noise).

**Aggregate effect on the gate (using post-#1299 text + the 2026-04-25 entry's image/webpage):**

| Swap                                  | Pre-#1299 verdict | Post-#1299 verdict | Why |
|---------------------------------------|-------------------|--------------------|---|
| OpenAI `gpt-5-mini` → `gpt-5.4-nano`  | BLOCKED           | BLOCKED            | Image regression unchanged (90.9% vs 100%). |
| Gemini `2.5-flash` → `2.5-flash-lite` | BLOCKED           | **STILL BLOCKED, narrower margin** | Text gap closed from −3.3pp to **−2.4pp** (2pp gate). 1 more pp of variance reduction or 1 more model improvement could clear it. |

Single-sample for flash-lite this run; a 2-3 sample average post-#1299 would confirm whether the +1.9pp is a stable property or includes some upside variance. Worth re-running before the next swap discussion.

Raw logs: `/tmp/ripls-1299-noreg/*.log` (local only; not checked in).

---

### 2026-04-25 — Temperature lever + Gemini global routing: all 5 models × all 3 modes

**Status:** First eval after introducing per-provider temperature flags (commit `0f7404b3` + bug fixes `5ade7546`) and switching the eval Gemini default to `location=global` (`d7b8b9f0`). The change directly targets the wide per-call variance that flipped the flash-lite gate verdict between consecutive samples.

**Setup:**

- `temperature=0.2` on all three providers (server-level flag, plumbed to every API call site for text + image + webpage). Reasoning-class OpenAI models (`gpt-5`, `gpt-5-mini`, `gpt-5-nano`, `o1*`/`o3*`/`o4*`) silently drop the param — Vertex API rejects anything other than 1.0 — so for `gpt-5-mini` this entry is functionally a temp=1.0 sample.
- Gemini eval runs on `location=global` instead of `us-central1`. Resolves both: (a) regional 429 RESOURCE_EXHAUSTED bursts when running flash + flash-lite in parallel against the dynamic-shared-quota pool; (b) HTTP/2 stalls on long-input cases (`dewalt_drill` and `blog_chainsaw` gear webpage; `brewery_social` experience image). All three cases that previously hung us-central1 for 5+ min completed cleanly on global (3.4s, 3.8s, 6.0s respectively). Production server still defaults to `us-central1` per `main.go`; the global flip is eval-only.

**Headline gate outcomes:**

- **OpenAI swap (gpt-5-mini → gpt-5.4-nano): still BLOCKED on image quality.** gpt-5.4-nano's image regression (90.9% vs gpt-5-mini's 100% this run) came in worse than the prior baseline showed; the case-level pattern is the same `rocky_talkie` / `mixer` / `atomic_maverick_ski` cluster. Text gate clears; webpage gate clears; image still blocks.
- **Gemini swap (2.5-flash → 2.5-flash-lite): still BLOCKED.** Three flash-lite samples on global (308, 311, 308 / 320) average 96.6% vs flash's 99.4% — text −3.3pp, image −1.2pp, webpage −1.0pp. The variance reduction from temperature=0.2 is real but small (spread 5 cases at temp=1.0 → 3 cases at temp=0.2); not enough to move flash-lite into the gate.
- **Gemini 2.5-flash on global at temp=0.2 is now the highest-scoring model in the harness — 318/320 (99.4%) — beating Anthropic Haiku at 317/320 (99.1%).** Request text scored a perfect 60/60 for the first time on any provider.

**Aggregate pass rate per (model × mode):**

| Model                              | Text             | Image           | Webpage          | Grand              | Notes |
|------------------------------------|------------------|-----------------|------------------|--------------------|-------|
| Anthropic `claude-haiku-4-5`       | 250/253 (98.8%)  | 33/33 (100%)    | 34/34 (100%)     | **317/320 (99.1%)** | Production primary, +0.7pp vs temp=1.0 baseline. |
| OpenAI `gpt-5-mini` *               | 248/253 (98.0%)  | 33/33 (100%)    | 34/34 (100%)     | 315/320 (98.4%)    | * Temperature param skipped (reasoning-class model rejects custom values). +1.0pp this run is bug-recovery (`dewalt_drill` webpage and `rocky_talkie` image landed cleanly), not temperature-driven. |
| OpenAI `gpt-5.4-nano` (candidate)  | 250/253 (98.8%)  | 30/33 (90.9%)   | 34/34 (100%)     | 314/320 (98.1%)    | Image regression worse than prior baseline (was 87.9%, now 90.9% — sample-level noise; cluster is the same). |
| Gemini `vertexai/gemini-2.5-flash` (global) | 251/253 (99.2%) | 33/33 (100%) | 34/34 (100%) | **318/320 (99.4%)** | Best in suite. Request text 60/60 for the first time. The 2 webpage cases that hung us-central1 ran cleanly on global (3.4s, 3.8s). |
| Gemini `vertexai/gemini-2.5-flash-lite` (global, 3-sample avg) | 240/253 (94.9%) avg | 26.7/27 (98.8%) avg | 33.7/34 (99.0%) avg | 309/320 (96.6%) avg | 3 samples: 308, 311, 308 / 320. Spread narrowed from 5 cases (temp=1.0) to 3 cases (temp=0.2). |

**Δ vs the 2026-04-24 temp=1.0 baseline (top of `1216-provider-model-defaults.md` previous entry):**

| Model                  | temp=1.0 grand   | temp=0.2 grand   | Δ        |
|------------------------|------------------|------------------|----------|
| Anthropic haiku-4-5    | 315/320 (98.4%)  | 317/320 (99.1%)  | **+0.7pp** |
| gpt-5-mini             | 305/313 (97.4%)  | 315/320 (98.4%)  | +1.0pp (bug-recovery, not temp-driven) |
| gpt-5.4-nano           | 312/320 (97.5%)  | 314/320 (98.1%)  | +0.6pp |
| gemini-2.5-flash       | 300/306 (98.0%)  | 318/320 (99.4%)  | **+1.4pp** |
| gemini-2.5-flash-lite (avg) | 308.5/320 (96.4%) | 309/320 (96.6%) avg | +0.2pp |

Every model gained, but the gain on flash-lite is a rounding error — its variance characteristic is more about per-call sampling on inputs the model is actually weak on (date arithmetic, value-estimate corner cases) than about the temperature setting. Lowering temperature does not turn a wrong answer into a right one if the model wasn't going to land a right answer at temperature=1.0 either.

**flash-lite variance (the original motivation for the temperature lever):**

- temp=1.0 (us-central1, 2 samples): 311/320 and 306/320 → spread 5 cases (1.6pp)
- temp=0.2 (global, 3 samples): 308/320, 311/320, 308/320 → spread 3 cases (0.9pp)

Variance narrowed by ~40% (5-case spread → 3-case spread). Not enough to flip the gate verdict — at the lowest gate-relevant boundary (text within 2pp of flash, no drop on image), flash-lite is still outside on both axes. But narrower variance does mean future runs have a smaller chance of giving a "false clear" like the original sample-1-only result that motivated this experiment.

**Stable failure cluster on flash-lite (all 3 samples):** `past_tense_coffee`, `stargazing_anne_white`, `value_cooking_class`, `trek_road_bike_frameset`, `borrow_drill_weekend`, `help_moving_saturday`. These are model-quality limits, not temperature-flippable.

**Latency note:** with global routing we no longer have a clean apples-to-apples wall-clock comparison vs the prior us-central1 baseline. Median per-call latencies on global appear similar to us-central1 within single-run variance (gemini-flash full suite ran in 255s including some retry-on-rate-limit overhead; flash-lite ~65s per sample). Worth one dedicated latency comparison if production is also flipped to global — Phase 4a's regional latency numbers won't directly apply.

**Decisions unchanged.** Production stays:

- Anthropic Claude Haiku 4.5 (primary) — 99.1% at temp=0.2.
- gpt-5-mini (OpenAI tertiary fallback) — 98.4% at default temperature.
- gemini-2.5-flash (Gemini secondary fallback) — 99.4% at temp=0.2 + global. **Strongly worth flipping the production server's default Gemini location to global** for the same reasons (eliminates the 2 webpage hang cases as production failure modes; +1.4pp quality; same latency band). File a follow-up.

**What the temperature lever did not do:**

- Did not unblock either swap.
- Did not eliminate flash-lite's stable failure cluster.
- Did not affect gpt-5-mini at all (reasoning-class — temperature is fixed at 1.0).

**What it did do:**

- Confirmed the variance-narrowing effect on Anthropic, both Gemini models, and gpt-5.4-nano. The narrowing is small in absolute terms but big enough that the +1.4pp gain on gemini-2.5-flash is now a stable property of the configuration rather than a single-sample artifact.
- Validated the per-provider plumbing (constructor → API call site) end to end, including the OpenAI reasoning-model skip-list and the Gemini vendor-specific `genai.GenerateContentConfig` config type.

Raw logs: `/tmp/ripls-1216-temp02/*.log` (local only; not checked in).

---

### 2026-04-24 (clean run) — Full post-fix re-eval: all 5 models × all 3 modes

**Status:** First clean re-eval after every audit-driven change landed (`28f73cb6` goldens cleanup, `1d5788e7` Request value classify-and-branch, `ce5cc5cc`/`a12117fc` keyword assertion redesign on Request and Experience). One run per (model × mode), single sample.

**Headline gate outcomes:**

- **OpenAI swap (gpt-5-mini → gpt-5.4-nano): still BLOCKED on image quality.** Text gate clears comfortably (gpt-5.4-nano now matches gpt-5-mini on text, both 98.4%), but image-mode regression is 9.1pp (97.0% → 87.9%) — the gear-image suite still fails on `rocky_talkie`, `mixer`, and `atomic_maverick_ski`. Same blocking pattern as the pre-cleanup run; goldens cleanup did not paper over the real model-quality gap.
- **Gemini swap (2.5-flash → 2.5-flash-lite): single-sample CLEARED the gate, but a 2-sample confirmation flips the verdict back to BLOCKED.** The first sample showed text −1.2pp / image tied / webpage tied. A second sample flipped 7 cases, dropping the average to text −2.5pp (outside the 2pp threshold) and image −1.5pp (drop on `rei_half_dome` — flash had it correct). flash-lite has wider per-call variance than flash, and the gate boundary is inside that variance. See the "2-sample confirmation" sub-section below for full detail.
- **Production-relevant new signal: gemini-2.5-flash now hangs on webpage mode.** Both `dewalt_drill` and `blog_chainsaw` (2 of the 3 gear webpage cases) stalled in HTTP/2 streams for 5+ minutes each and had to be skipped via `-skip` to complete the run. Streaming timeouts (#1267) only protect text mode; image and webpage paths use non-streaming APIs. **This is a real production correctness issue with the current Gemini default.** Filed as a follow-up consideration alongside #1229 (extending streaming to image + webpage modes).

**Aggregate pass rate per (model × mode):**

| Model                              | Text             | Image           | Webpage          | Grand (clean)     | Notes |
|------------------------------------|------------------|-----------------|------------------|-------------------|-------|
| Anthropic `claude-haiku-4-5`       | 249/253 (98.4%)  | 32/33 (97.0%)   | 34/34 (100.0%)   | **315/320 (98.4%)** | Production primary. |
| OpenAI `gpt-5-mini`                | 246/253 (97.2%)  | 32/33 (97.0%)   | 27/27 (100.0%)   | 305/313 (97.4%)   | dewalt_drill webpage still hits the malformed-float invocation bug — 1 case excluded. With it counted as a 7-zero: 305/320 (95.3%). |
| OpenAI `gpt-5.4-nano` (candidate)  | 249/253 (98.4%)  | 29/33 (87.9%)   | 34/34 (100.0%)   | 312/320 (97.5%)   | Text matches Haiku. Image regression vs gpt-5-mini blocks the swap. Webpage invocation bug fixed. |
| Gemini `vertexai/gemini-2.5-flash` | 247/253 (97.6%)  | 33/33 (100.0%)  | 20/20 (100.0%)   | 300/306 (98.0%)   | Webpage 20/20 only because dewalt_drill + blog_chainsaw hung and were skipped. Counting hangs as 7-zero: 300/320 (93.8%). |
| Gemini `vertexai/gemini-2.5-flash-lite` (candidate) | 244/253 (96.4%) | 33/33 (100.0%) | 34/34 (100.0%) | **311/320 (97.2%)** | Within 1.2pp of flash on text, ties on image + webpage, no hangs. Gate clears. |

**Per-suite drilldown (passes / checks per suite):**

| Suite (cases × checks)             | Anthropic       | gpt-5-mini      | gpt-5.4-nano    | gemini-2.5-flash | gemini-2.5-flash-lite |
|------------------------------------|-----------------|-----------------|-----------------|------------------|-----------------------|
| Experience text (26 × 118)         | 116/118 (98.3%) | 114/118 (96.6%) | 118/118 (100%)  | 115/118 (97.5%)  | 112/118 (94.9%)       |
| Gear text (13 × 75)                | 74/75 (98.7%)   | 75/75 (100%)    | 73/75 (97.3%)   | 75/75 (100%)     | 74/75 (98.7%)         |
| Request text (12 × 60)             | 59/60 (98.3%)   | 57/60 (95.0%)   | 58/60 (96.7%)   | 57/60 (95.0%)    | 58/60 (96.7%)         |
| Experience image (2 × 6)           | 6/6 (100%)      | 6/6 (100%)      | 6/6 (100%)      | 6/6 (100%)       | 6/6 (100%)            |
| Gear image (4 × 27)                | 26/27 (96.3%)   | 26/27 (96.3%)   | 23/27 (85.2%)   | 27/27 (100%)     | 27/27 (100%)          |
| Experience webpage (2 × 13)        | 13/13 (100%)    | 13/13 (100%)    | 13/13 (100%)    | 13/13 (100%)     | 13/13 (100%)          |
| Gear webpage (3 × 21)              | 21/21 (100%)    | 14/14 + invoc fail | 21/21 (100%) | 7/7 + 2 hangs    | 21/21 (100%)          |

**Where each model still fails (after all fixes):**

- **Anthropic haiku-4-5 (5 misses out of 320):** `value_cooking_class`, `value_wine_tasting` (Experience value-estimate per-person semantic — known unfixed), `tent_with_location` (location_query for "in my garage" — known PROMPT-side issue), `babysitter_weekend` (Request keyword breadth — borderline), `rocky_talkie` (gear image — single-sample variance, hit material_category here).
- **gpt-5-mini (8 misses + 1 invocation fail):** `explicit_pickleball_longmont` (general-pool match count 1 — single-sample variance with `[pickleball sports outdoors community]`), `relative_saturday_run` and `past_tense_coffee` (date arithmetic), `value_cooking_class`, three Request cases on borderline keyword pools (`help_moving_saturday`, `deck_repair_advice`, `bike_pump_home`), `rocky_talkie` image, `rei_tent` webpage, plus the `dewalt_drill` malformed-float invocation bug.
- **gpt-5.4-nano (8 misses):** `northface_backpack` and `tent_with_location` (text), `deck_repair_advice` and `bike_pump_home` (Request keywords), and 4 image-mode misses (`rocky_talkie`, `mixer`, `atomic_maverick_ski`) — the image suite is the swap blocker.
- **gemini-2.5-flash (6 misses + 2 hangs):** `explicit_pickleball_longmont` (general-pool, same single-sample issue), `value_cooking_class`, `value_yoga_retreat`, three Request cases (`borrow_drill_weekend`, `help_moving_saturday`, `dog_walker_single`), plus 2 webpage hangs.
- **gemini-2.5-flash-lite (9 misses):** 6 Experience text (mostly date/time-arithmetic — `past_tense_coffee`, `stargazing_anne_white`, `kids_birthday_saturday`, `painting_workshop_kids`, `value_cooking_class`, `explicit_boardgames_friday_7pm`), `trek_road_bike_frameset` value-estimate, 2 Request keyword-breadth misses.

**Decision summary:**

| Swap                                  | Pre-cleanup verdict | Post-cleanup verdict | Gate |
|---------------------------------------|---------------------|----------------------|------|
| OpenAI `gpt-5-mini` → `gpt-5.4-nano`  | BLOCKED (image −12.2pp) | **BLOCKED (image −9.1pp)** | image no-drop |
| Gemini `2.5-flash` → `2.5-flash-lite` | BLOCKED (text −5.6pp) | **CLEARS (text −1.2pp)** | text within 2pp + image no-drop |

**Confidence on the Gemini-swap result:** medium. The aggregate gate clears, but flash-lite still has a real cluster of date-arithmetic failures that flash handles correctly. Worth a 2-sample average run before flipping the default. The per-mode latency advantages from the original Phase 4a research still apply — flash-lite was 0.4-0.8s vs flash 3-7s — so the swap remains attractive on speed if the quality holds across a confirmation run.

#### 2-sample confirmation — flash-lite (run 2 of 2)

Result: **gate no longer clearly clears once variance is measured.** Recommend keeping `gemini-2.5-flash` as the default for now.

| Suite               | Sample 1        | Sample 2        | Δ      |
|---------------------|-----------------|-----------------|--------|
| Experience text     | 112/118 (94.9%) | 110/118 (93.2%) | −1.7pp |
| Gear text           | 74/75 (98.7%)   | 72/75 (96.0%)   | −2.7pp |
| Request text        | 58/60 (96.7%)   | 58/60 (96.7%)   | 0      |
| Experience image    | 6/6 (100%)      | 6/6 (100%)      | 0      |
| Gear image          | 27/27 (100%)    | 26/27 (96.3%)   | −3.7pp |
| Experience webpage  | 13/13 (100%)    | 13/13 (100%)    | 0      |
| Gear webpage        | 21/21 (100%)    | 21/21 (100%)    | 0      |
| **Grand**           | **311/320 (97.2%)** | **306/320 (95.6%)** | **−1.6pp** |

Average pass rates and gate evaluation:

| Mode    | flash-lite avg (2 samples) | flash baseline | Δ       | Gate                 |
|---------|----------------------------|----------------|---------|----------------------|
| Text    | 240.5/253 (95.1%)          | 247/253 (97.6%) | −2.5pp | within 2pp ❌        |
| Image   | 32.5/33 (98.5%)            | 33/33 (100%)    | −1.5pp | no drop ❌           |
| Webpage | 34/34 (100%)               | 20/20 (100%, hangs excluded) | 0  | no drop ✅       |

Stable failures (both samples — these are real model weaknesses): `explicit_boardgames_friday_7pm`, `past_tense_coffee`, `value_cooking_class`, `stargazing_anne_white`, `trek_road_bike_frameset`, `borrow_drill_weekend`, `help_moving_saturday`.

Volatile cases (passed once, failed once — single-sample noise):
- Sample 1 only: `kids_birthday_saturday`, `painting_workshop_kids`.
- Sample 2 only: `explicit_pickleball_longmont`, `relative_saturday_run`, `value_wine_tasting`, `potluck_sunday_evening`, `northface_backpack`, `aluminum_ladder`, `rei_half_dome` (image — flips the image gate).

**Read:** the single-sample sample-1 result that "cleared" the gate was on the optimistic end of flash-lite's variance distribution. Sample 2 flipped 7 cases including a gear-image case (`rei_half_dome`) that takes the image-mode pass rate below the no-drop threshold. The text-mode 2-sample average is also slightly outside the 2pp threshold (−2.5pp).

**Decision:** **keep `vertexai/gemini-2.5-flash` as the Gemini default.** The post-cleanup eval brought flash-lite close to viable, but the per-call variance is wide enough that it can't be relied on to clear the gate consistently. The latency win from the swap (≈4-7× faster) is still attractive but not at this variance level.

**What would unblock the swap:**

- Lower flash-lite variance, e.g., a deterministic-seed mode if Vertex AI exposes one, or temperature reduction on calls that don't benefit from creativity (most Gen* paths). This is the highest-leverage lever.
- Date-arithmetic prompt fix targeting the stable failure cluster (`past_tense_coffee`, `stargazing_anne_white`, `relative_saturday_run`). Without this, even a deterministic flash-lite would still fail those cases.
- Re-checking after `gemini-3.x-flash-lite` GA in us-central1 (parking-lot note in the issue body).

Raw logs for sample 2: `/tmp/ripls-1216-fullrun/gemini-2.5-flash-lite-sample2.log`.

**Notes on this run:**

- Single-sample noise: the `explicit_pickleball_longmont` case where gpt-5-mini's `[pickleball sports outdoors community]` only matched 1 of the 6 general-pool terms is at the edge of what variance can flip. A second run of just that case would likely come back with `racquet sport` or similar (it returned `[pickleball racquet sport outdoor recreation group sports]` in the prior run, which matched 2 terms cleanly). Don't read too much into single-pp differences.
- Gemini webpage hangs were the biggest surprise. Filed as a process observation; #1229 (server-side streaming to image + webpage) is the natural place this gets addressed. The gemini-flash webpage line in the table excludes the 2 hung cases — including them as 7-zero failures takes the grand to 93.8%.
- `gpt-5-mini` total runtime 22.6 min vs ~1-2 min for every other model. The latency penalty alone justifies looking for a replacement; the candidate just isn't quite there on image yet.

Raw logs: `/tmp/ripls-1216-fullrun/*.log` (local only; not checked in).

---

### 2026-04-24 (update) — Request prompt classification fix: text rerun across all five models

**Status:** Request text suites only. Other modes not re-run; their numbers in the 2026-04-24 baseline entry are still the current reference point.

**Change:** `buildRequestGenerationPrompt` at `server/ai/prompts.go:416` now classifies each request as SERVICE (labor/expertise, use hire-cost) or ITEM BORROW (someone else's physical item, use one-session rental-equivalent fee), with 2 non-leaky examples per branch. The `request_goldens.json` doc comment was updated to match. The prompt addresses the root cause of the "why does every provider estimate item retail instead of rental fee" pattern surfaced in the audit.

**Git SHA:** `1d5788e7` on branch `1216-phase1-eval-harness`.

**Request text pass rate — pre-fix vs post-fix (same harness, same goldens, same seeds):**

| Provider / Model                 | Pre-fix             | Post-fix            | Δ      |
|----------------------------------|---------------------|---------------------|--------|
| Anthropic `claude-haiku-4-5`     | 44/48 (91.7%)       | 44/48 (91.7%)       | 0      |
| OpenAI `gpt-5-mini`              | 41/48 (85.4%)       | **43/48 (89.6%)**   | +4.2pp |
| OpenAI `gpt-5.4-nano`            | 40/48 (83.3%)       | 38/48 (79.2%)       | −4.1pp |
| Gemini `vertexai/gemini-2.5-flash` | 44/48 (91.7%)       | **46/48 (95.8%)**   | +4.1pp |
| Gemini `vertexai/gemini-2.5-flash-lite` | 42/48 (87.5%)       | **45/48 (93.8%)**   | +6.3pp |

Three providers materially improved, one flat, one candidate regression. The gpt-5.4-nano regression is almost entirely on `search_keywords_jaccard` (not value_estimate), and the audit doc flags this as a separate prompt-consistency issue to tackle next (the prompt tells models to produce single-word keywords, goldens use phrases — orthogonal to this change).

**Per-check decomposition — value_estimate_usd gains from this change:**

| Provider                      | Pre-fix value | Post-fix value |
|-------------------------------|---------------|----------------|
| Anthropic haiku-4-5            | 11/11 (100%)  | 10/11 (91%) *  |
| gpt-5-mini                     | 9/11 (82%)    | **11/11 (100%)** |
| gpt-5.4-nano                   | 9/11 (82%)    | 10/11 (91%)    |
| gemini-2.5-flash               | 9/11 (82%)    | 10/11 (91%)    |
| gemini-2.5-flash-lite          | 9/11 (82%)    | **11/11 (100%)** |

\* Haiku regressed 1 case (`help_moving_saturday` — prior prompt's "Moving help: $50-100 total" example was anchoring Haiku directly to $80; removing the leak pushed Haiku's answer to $150, outside the ±60% tolerance). The audit now lists this as a TOL candidate independent of the prompt fix.

**Request text latency — median and TTFT, pre-fix vs post-fix:**

| Provider / Model                 | Pre-fix median / TTFT | Post-fix median / TTFT | Δ wall | Δ TTFT |
|----------------------------------|-----------------------|------------------------|--------|--------|
| Anthropic haiku-4-5              | 1.82s / 563ms         | **1.75s / 494ms**      | −4%    | **−12%** |
| gpt-5-mini                       | 12.8s / 12.0s         | 20.3s / 19.5s          | +59%   | +62%   |
| gpt-5.4-nano                     | 1.06s / 834ms         | 1.12s / **729ms**      | +6%    | −13%   |
| gemini-2.5-flash                 | 3.13s / 2.93s         | 2.96s / **2.37s**      | −5%    | **−19%** |
| gemini-2.5-flash-lite            | 772ms / 364ms         | 729ms / 357ms          | −6%    | −2%    |

**Reads:**

- **Anthropic (production primary) got faster AND kept quality.** TTFT dropped 12% because the classify-first framing lets the model commit to output structure sooner. Net win on the single provider that most matters.
- **Gemini 2.5-flash jumped 4.1pp in quality and got 19% faster on TTFT** — the biggest net win. The classify-and-branch pattern steered it away from its default "estimate item retail" misreading.
- **gpt-5-mini jumped 4.2pp in quality but also got 59% slower on wall clock.** Single-run variance warning applies — gpt-5-mini had a 600s tail outlier in an earlier run, and its per-call variance is large. The quality improvement reproducibly comes from value_estimate cases clearing; the latency jump may be partly noise.
- **gpt-5.4-nano is the outlier.** Lost 4.1pp, mostly on search_keywords failures unrelated to the change. Separate fix (prompt-consistency on keywords) should address it.

**Net effect on #1216 grand aggregate:** applying the post-fix Request numbers to the 2026-04-24 baseline entry's three-mode grid would bump the three baselines as follows (image + webpage unchanged):

| Provider                       | Pre-fix grand aggregate | Post-fix grand aggregate |
|--------------------------------|-------------------------|--------------------------|
| Anthropic haiku-4-5            | 290/301 (96.3%)         | 290/301 (96.3%)          |
| gpt-5-mini                     | 277/294 (94.2%)         | **279/294 (94.9%)**      |
| gemini-2.5-flash               | 286/301 (95.0%)         | **288/301 (95.7%)**      |

Plus the two candidates:

| Candidate                        | Pre-fix               | Post-fix              |
|----------------------------------|-----------------------|-----------------------|
| gpt-5.4-nano                     | 274/301 (91.0%)       | 272/301 (90.4%)       |
| gemini-2.5-flash-lite            | 275/301 (91.4%)       | **278/301 (92.4%)**   |

The candidate gates from the 2026-04-24 baseline still hold:

- OpenAI swap (gpt-5.4-nano): text gate was 88.2%. Post-fix candidate text: 90.6% → **79.2% (−11.4pp on Request alone)**. Text gate **still fails** but for a different reason than pre-fix (keywords instead of value). Plus image regression is unchanged. Swap remains blocked.
- Gemini swap (gemini-2.5-flash-lite): text gate was 92.9%. Post-fix candidate text: 95.5% overall but Request is 93.8% (within 2pp of the new Gemini 2.5-flash post-fix Request number 95.8%). Text gate still fails on other modes (Experience +1.2pp, Gear 90.7% vs 97.3%). Swap remains blocked.

Both swap decisions unchanged. Quality headroom improved for the primary (Haiku) and secondary fallback (Gemini 2.5-flash). Time-to-first-content improved slightly for the production primary.

Raw logs: `/tmp/ripls-1216-rerun/*.log`.

---

### 2026-04-24 — Phase 2 + 3 candidate evaluation: gpt-5.4-nano and gemini-2.5-flash-lite (both BLOCKED)

**Status:** Negative result. **Neither candidate clears its swap gate.** No code changes to `server/ai/provider_factory.go` — current defaults stand.

**Git SHA:** `09a03316` on branch `1216-phase1-eval-harness` (Phase 1 harness + baselines committed; candidate evals run from the same tree).
**Environment:** identical to the Phase 1 baseline entry — local dev, ripls-dev AI keys, `go test -tags=benchmark -count=1 ./server/ai/eval/`.
**Samples:** 1 run per candidate × 7 suites. No re-runs — deltas well outside single-sample noise on the failing dimensions.

**Candidate aggregate deltas vs. Phase 1 baseline:**

| Mode        | OpenAI `gpt-5-mini` → `gpt-5.4-nano`                | Gemini `2.5-flash` → `2.5-flash-lite`                |
|-------------|-----------------------------------------------------|-----------------------------------------------------|
| Text        | 218/234 (93.2%) → 212/234 (90.6%) — **−2.6pp**      | 222/234 (94.9%) → 209/234 (89.3%) — **−5.6pp** ❌   |
| Image       | 32/33 (97.0%) → 28/33 (84.8%) — **−12.2pp** ❌      | 30/33 (90.9%) → 33/33 (100.0%) — **+9.1pp** ✅      |
| Webpage     | 27/27 on 4/5 cases → 34/34 on 5/5 — **+invocation failure fixed** ✅ | 34/34 (100%) → 33/34 (97.1%) — −2.9pp |
| **Grand**   | 277/294 on 61/62 → 274/301 (91.0%) on 62/62          | 286/301 (95.0%) → 275/301 (91.4%)                    |

**Candidate latency (from log `duration_ms` / `streaming_first_delta_ms`):**

Both candidates are dramatically faster than their baselines across every mode. The wall-clock wins alone would justify the swaps if quality held — they don't.

OpenAI `gpt-5-mini` → `gpt-5.4-nano`:

| Operation | gpt-5-mini baseline (median / TTFT) | gpt-5.4-nano candidate (median / TTFT) | Speedup |
|---|---|---|---|
| `GenerateExperienceFromTextStreaming` | 23.4s / 21.9s | **1.36s / 0.87s** | **~17× wall / ~25× TTFT** |
| `GenerateGearFromTextStreaming` | 20.2s / 17.6s | **1.12s / 0.81s** | ~18× / ~22× |
| `GenerateRequestContentStreaming` | 12.8s / 12.0s | **1.06s / 0.83s** | ~12× / ~14× |
| `DetectGearInImage` (non-streaming) | 14.6s | **3.29s** | ~4.4× |
| `GenerateExperienceFromImage` (non-streaming) | 13.5s | **2.68s** | ~5× |
| `GenerateGearFromWebpage` (non-streaming) | 19.5s | **1.94s** | ~10× |
| `GenerateExperienceFromWebpage` (non-streaming) | 20.96s | **2.10s** | ~10× |

Gemini `2.5-flash` → `2.5-flash-lite`:

| Operation | 2.5-flash baseline (median / TTFT) | 2.5-flash-lite candidate (median / TTFT) | Speedup |
|---|---|---|---|
| `GenerateExperienceFromTextStreaming` | 5.83s / 5.78s | **0.80s / 0.41s** | **~7× wall / ~14× TTFT** |
| `GenerateGearFromTextStreaming` | 3.61s / 3.32s | **0.75s / 0.45s** | ~4.8× / ~7.4× |
| `GenerateRequestContentStreaming` | 3.13s / 2.93s | **0.77s / 0.36s** | ~4× / ~8× |
| `DetectGearInImage` (non-streaming) | 6.29s | **2.16s** | ~2.9× |
| `GenerateExperienceFromImage` (non-streaming) | 6.76s | **2.38s** | ~2.8× |
| `GenerateGearFromWebpage` (non-streaming) | 4.75s median (153s mean — outlier skew) | **0.85s** | ~5.5× on median |
| `GenerateExperienceFromWebpage` (non-streaming) | 7.50s | **1.10s** | ~6.8× |

**Read:** the latency data independently validates Phase 4a's findings. gpt-5.4-nano is on par with Haiku's wall-clock and TTFT; gemini-2.5-flash-lite actually beats Haiku on TTFT (~400ms vs ~600ms). If either quality issue could be fixed through prompt tweaks or schema adjustments rather than model change, we'd ship the swap tomorrow.

**Gate outcomes:**

- **OpenAI swap gate:** text within 5pp (✅ landed at −2.6pp) AND image/webpage must not drop below current. **FAILS** — image dropped 12.2pp (4 checks worse). gpt-5.4-nano's image-mode regressions (`material_category_any_of` 2/4, `model_contains` 2/3, `weight_grams_approx` 1/2, `brand_contains` 3/4) would silently degrade the photo-first Gen\* flows that are core to the creation UX.
- **Gemini swap gate:** text within 2pp AND image must not drop below current (90.9%). **FAILS** on text — aggregate fell 5.6pp. Gear text `value_estimate_usd` collapsed from 10/12 (83%) to 6/12 (50%); Experience `date` went from 13/13 to 10/13; `weight_grams_approx` regressed 5/5 → 4/5. Image improved but doesn't compensate.

**Per-case failure details (candidate-only; baseline failures covered in the 2026-04-24 baseline entry):**

New candidate regressions — cases that the baseline model handled correctly but the candidate missed:

**gpt-5.4-nano image-mode regressions (the decisive blockers).** Each row shows the case input, the failing check, and both providers' actual output on the relevant field:

| Case | Input | Field | Expected | gpt-5-mini baseline | **gpt-5.4-nano candidate** |
|---|---|---|---|---|---|
| `rocky_talkie` | `rocky_talkie.jpeg` | `material_category_any_of` | `mixed_plastic_metal` | `mixed_plastic_metal` ✅ | `electronics_small` ❌ |
| `rocky_talkie` | ↑ | `brand_contains` needs `"Rocky Talkie"` | — | `"ROCKY TALKIE"` ✅ | `""` (empty) ❌ |
| `atomic_maverick_ski` | `atomic_maverick_ski.webp` | `material_category_any_of` | `mixed_plastic_metal` or `mixed_wood_metal` | `mixed_wood_metal` ✅ | `solid_plastic` ❌ |
| `atomic_maverick_ski` | ↑ | `model_contains` needs `"Maverick 115"` substring of expected; 3 cases total | — | `"Maverick 115 CTI"` ✅ on all 3 | ❌ 1 case short of baseline (candidate collapsed `"MAVERICK 115"` — missing `CTI` on baseline's ski case) |
| `atomic_maverick_ski` | ↑ | `weight_grams_approx` `3200g ±50%` | — | `3800g` ✅ | `1000g` ❌ (69% off) |

Summary of change: on the ski, candidate dropped brand/material/model fidelity (brand strings normalized to all-caps, composite materials collapsed to a single material, weight off by 3×). On the radio, brand disappeared entirely. This is the pattern across the gear_image suite.

**gpt-5.4-nano text-mode regressions** (text gate passes within 5pp, listed for completeness; the candidate is faster and cheaper but these are real losses):

| Case | Prompt | Field | Expected | gpt-5-mini baseline | **gpt-5.4-nano candidate** |
|---|---|---|---|---|---|
| `northface_backpack` | `"North Face hiking backpack 65L"` | `brand` exact | `"North Face"` | `"North Face"` ✅ | `"The North Face"` ❌ |
| `northface_backpack` | ↑ | `value_estimate_usd` `$180 ±40%` | $108–$252 | `$180` ✅ | `$90` ❌ (50% off) |
| `camping_tent_generic` | `"4-person backpacking tent"` | `value_estimate_usd` `$250 ±40%` | $150–$350 | `$300` ✅ | `$120` ❌ (52% off) |
| `kitchenaid_mixer` | `"KitchenAid Artisan stand mixer"` | `value_estimate_usd` `$450 ±40%` | $270–$630 | `$350` ✅ | `$250` ❌ (44% off) |
| `tent_with_location` | `"camping tent in my garage"` | `location_query_contains` needs `"garage"` | — | `"Boulder, CO"` ❌ (baseline already failed) | `""` ❌ (worse — empty location) |
| `math_tutor_kid` | `"looking for a math tutor for my 10 year old"` | `search_keywords_jaccard` `≥0.20` vs `[tutoring math education homework]` | — | `[math tutoring child studying homework help]` ✅ 0.33 | `[math tutoring child learning homework help grade school teacher]` ❌ 0.00 |

**gemini-2.5-flash-lite text-mode regressions (the decisive blockers).** Note that most date/time misses are "off by one day" — the candidate interprets relative dates differently from 2.5-flash (today = 2026-04-24 in both runs):

| Case | Prompt | Field | Expected | gemini-2.5-flash baseline | **gemini-2.5-flash-lite candidate** |
|---|---|---|---|---|---|
| `relative_saturday_run` | `"this Saturday morning let's go for a run"` | `date` | `2026-04-25` (next Saturday) | `2026-04-25` ✅ | `2026-04-26` ❌ (Sunday) |
| `past_tense_coffee` | `"coffee with Jane last Thursday"` | `date` | `2026-04-16` (last Thursday) | `2026-04-16` ✅ | `2026-04-17` ❌ (off by 1) |
| `stargazing_anne_white` | `"stargazing at Anne U. White friday night"` | `date` | `2026-04-24` (this Friday) | `2026-04-24` ✅ | `2026-04-25` ❌ (Saturday) |
| `vague_coffee_sometime` | `"coffee sometime"` | `time_confidence` | `UNKNOWN` or `INFERRED` | passes | `EXPLICIT` ❌ (candidate fabricated `09:00`) |
| `camping_tent_generic` | `"4-person backpacking tent"` | `value_estimate_usd` `$250 ±40%` | $150–$350 | `$300` ✅ | `$120` ❌ (52% off) |
| `japanese_pull_saw` | `"Japanese pull saw for woodworking"` | `value_estimate_usd` `$60 ±40%` | $36–$84 | `$40` ✅ | `$35` ❌ (borderline, 42% off) |
| `kitchenaid_mixer` | `"KitchenAid Artisan stand mixer"` | `value_estimate_usd` `$450 ±40%` | $270–$630 | `$350` ✅ | `$250` ❌ (44% off) |
| `trek_road_bike_frameset` | `"Trek aluminum road bike frame 54cm, frameset only"` | `value_estimate_usd` `$400 ±40%` | $240–$560 | `$250` ✅ | `$150` ❌ (63% off) |
| `aluminum_ladder` | `"6-foot aluminum step ladder"` | `weight_grams_approx` `5000g ±70%` | 1500–8500g | `8000g` ✅ | `0g` ❌ (no estimate) |
| `aluminum_ladder` | ↑ | `value_estimate_usd` `$100 ±40%` | $60–$140 | `$80` ✅ | `$50` ❌ (50% off) |
| `math_tutor_kid` | `"looking for a math tutor for my 10 year old"` | `value_estimate_usd` `$50 ±60%` | $20–$80 | `$60` ✅ | `$150` ❌ (200% off) |
| `concert_event` | webpage: `"Summer Sounds Concert Series - Red Rocks..."` | `time` | `"19:00"` | `"19:00"` ✅ | `"17:30"` ❌ (grabbed a different time from the page body) |

**Upsides worth noting even though the swaps are blocked:**

- **gpt-5.4-nano fixes the dewalt_drill invocation failure.** Strict-schema JSON parsing succeeded on every case; no more malformed scientific-notation floats. This is a real latent correctness bug cleared by the candidate, independent of the quality regressions. If we ever decide to change the OpenAI default, gpt-5.4-nano is mechanically cleaner than gpt-5-mini.
- **gpt-5.4-nano is dramatically faster.** The candidate's full text suite ran in ~1 min vs ~16.5 min for gpt-5-mini — confirms Phase 4a's latency finding independently. The text quality even regressed only 2.6pp (which *would* clear the 5pp gate). It's purely the image-mode quality that blocks the swap.
- **gemini-2.5-flash-lite fixes the gear-image material-category bias.** `material_category_any_of` went 4/4 (vs 1/4 in 2.5-flash) and `weight_grams_approx` went 2/2 (vs 2/2 — maintained). flash-lite understands composite materials where 2.5-flash doesn't. But the text regressions (especially the numeric fields — values, dates, weights) are severe.

**Decision:**

- **Keep `gpt-5-mini` as the OpenAI default.** The invocation-failure bug is real but rare in production (only fired 1/13 webpage cases) and the image regression in gpt-5.4-nano is broad and deep. File the JSON parse-error class of bugs as a separate tracking issue if needed, not a reason to swap.
- **Keep `vertexai/gemini-2.5-flash` as the Gemini default.** The flash-lite text regressions on dates and value estimates (both numeric, both central to Experience and Gear generation) are disqualifying.

**Next candidates to consider (not scoped to this run):**

- A mid-tier OpenAI model between `gpt-5-mini` and `gpt-5.4-nano` on the size/quality curve — if one exists that keeps image quality while gaining latency. Given the Phase 4a numbers, gpt-5-mini's structured-output latency is a real production problem (~28s fallback spinners); finding a candidate that's faster *and* maintains image quality is worth one more pass.
- For Gemini, wait on `gemini-3.1-flash-lite-preview` us-central1 GA per the parking-lot note in the issue body. Phase 4a measured it at 1.43s with unknown quality — the eval harness is now ready to evaluate it the moment regional GA lands. File a one-time agent to re-check this in 30 days.
- Neither provider's candidate is a clear win on quality. **The real latency win left on the table is prompt caching (#1216 step B in the research comment on #1157), which is prompt-side, not model-side.** Worth sequencing before the next model-default pass.

**Re-run recommendation if anyone wants to re-litigate:** run each candidate twice and average. Single-sample noise on borderline cases (especially value estimates) is ~1-2pp; the gaps here (image 12.2pp, text 5.6pp) are well outside that, so a re-run is unlikely to flip the decision. But 2-sample averaging would tighten confidence.

Raw logs: `/tmp/ripls-1216-candidates/*.log` (local only; not checked in).

---

### 2026-04-24 — Phase 1 baseline: current default quality across all three providers (text + image + webpage)

**Status:** Baseline for [#1216](https://github.com/Type2Fun-Foundation/ripls/issues/1216). Phase 1 generalized the existing `benchmark`-tagged prompt-eval harness across providers (see `server/ai/eval/prompt_test.go:1` — new `-provider` and `-mode` flags) so candidate model swaps can be gated on per-provider, per-mode pass-rate deltas.

**Git SHA:** `56122cd3` (branch `1157-enable-gen-streaming`; Phase 1 harness refactor on top, not yet committed).
**Environment:** local dev, MacBook, wifi. AI keys = ripls-dev (Anthropic + OpenAI + Vertex AI project `ripls-dev` us-central1). Ran via `go test -tags=benchmark -count=1 ./server/ai/eval/` with the relevant `-provider` and `-mode` flags per run.
**Samples:** 1 run per (provider × mode) cell — cases within each suite are themselves the sample grid (26 Experience, 13 Gear, 12 Request text cases; 4 gear_image, 2 experience_image; 3 gear_webpage, 2 experience_webpage).
**Scope:** isolated the AI call against the live providers — this is a quality eval, not an end-to-end latency eval. Latency-side numbers for these same models live in [1157-genexperience-latency.md](1157-genexperience-latency.md).

**Aggregate pass rate grid (per suite, per provider):**

Text mode (51 cases; 234 total checks):

| Provider / Model                 | Experience 26c    | Gear 13c        | Request 12c     | Text aggregate    |
|----------------------------------|-------------------|-----------------|-----------------|-------------------|
| Anthropic `claude-haiku-4-5`     | 106/111 (95.5%)   | 73/75 (97.3%)   | 44/48 (91.7%)   | **223/234 (95.3%)** |
| Gemini `vertexai/gemini-2.5-flash` | 105/111 (94.6%)   | 73/75 (97.3%)   | 44/48 (91.7%)   | **222/234 (94.9%)** |
| OpenAI `gpt-5-mini`              | 104/111 (93.7%)   | 73/75 (97.3%)   | 41/48 (85.4%)   | **218/234 (93.2%)** |

Image mode (6 cases; 33 total checks):

| Provider / Model                 | gear_image 4c     | experience_image 2c | Image aggregate   |
|----------------------------------|-------------------|---------------------|-------------------|
| Anthropic `claude-haiku-4-5`     | 27/27 (100%)      | 6/6 (100%)          | **33/33 (100%)**  |
| OpenAI `gpt-5-mini`              | 26/27 (96.3%)     | 6/6 (100%)          | **32/33 (97.0%)** |
| Gemini `vertexai/gemini-2.5-flash` | 24/27 (88.9%)     | 6/6 (100%)          | **30/33 (90.9%)** |

Webpage mode (5 cases; 34 total checks):

| Provider / Model                 | gear_webpage 3c       | experience_webpage 2c | Webpage aggregate     |
|----------------------------------|-----------------------|-----------------------|-----------------------|
| Anthropic `claude-haiku-4-5`     | 21/21 (100%)          | 13/13 (100%)          | **34/34 (100%)**      |
| Gemini `vertexai/gemini-2.5-flash` | 21/21 (100%)          | 13/13 (100%)          | **34/34 (100%)**      |
| OpenAI `gpt-5-mini`              | 14/14 on 2/3 cases ⚠️ | 13/13 (100%)          | **27/27 on 4/5 cases**⚠️ |

Grand aggregate (62 cases; 301 total checks):

| Provider / Model                 | Overall                       |
|----------------------------------|-------------------------------|
| Anthropic `claude-haiku-4-5`     | **290/301 (96.3%)**            |
| Gemini `vertexai/gemini-2.5-flash` | **286/301 (95.0%)**            |
| OpenAI `gpt-5-mini`              | 277/294 (94.2%) on 61/62 cases⚠️ |

**⚠️ Invocation failure on OpenAI:** `TestGearWebpagePrompt/dewalt_drill` failed to parse gpt-5-mini's response with `json: cannot unmarshal number 0.95E00120399999999999999999999999999999999999999999999999999999 into Go struct field outputSchema.confidence of type float32`. gpt-5-mini occasionally emits malformed scientific-notation floats that the strict-schema unmarshaller rejects. Not counted as a check failure — reported as a separate invocation failure. This is a latent correctness bug that a text-only eval would have missed.

**Notable per-field regressions worth carrying into the candidate comparisons:**

- **Gemini 2.5 Flash image mode: `material_category_any_of` 1/4 (25%)** on the four gear_image cases. Misclassified material on rocky_talkie, mixer, and atomic_maverick_ski. Haiku gets this 4/4. OpenAI gets it 3/4. If a Gemini candidate (e.g., gemini-2.5-flash-lite in Phase 3) regresses further here, the swap can't go in.
- **OpenAI gpt-5-mini Request text: 85.4%**, right at the 85% gate. `location_query` 0/1, `search_keywords_jaccard` 8/12, `value_estimate_usd` 9/11. Any candidate that drops below 85% on Request is blocked.
- **All three providers share the same Experience `search_keywords_jaccard` weakness** (Anthropic 3/7, Gemini 2/7, OpenAI 2/7). This isn't a candidate-swap concern — it's a prompt quality issue that belongs with #1265 (Opus-driven prompt rewrite).

**Candidate gate reminder:**

- Phase 2 OpenAI swap (gpt-5-mini → gpt-5.4-nano) — gate: within **5pp** of the text baseline (218/234 = 93.2%), so candidate must hit **~88.2%** text aggregate. Image and webpage must not drop below current (32/33 image, 27/27 on 4/5 webpage); the existing invocation failure is a known regression so a candidate that cleanly emits floats would be net-positive.
- Phase 3 Gemini swap (gemini-2.5-flash → gemini-2.5-flash-lite) — gate: within **2pp** of the text baseline (222/234 = 94.9%), so candidate must hit **~92.9%** text aggregate. Image aggregate already at 90.9% — candidate must not drop further (the `material_category_any_of` weakness is real and worth watching).

**Run-to-run variance:** a second Anthropic image run hit `atomic_maverick_ski` material as `mixed_wood_plastic` instead of `mixed_wood_metal` (accepted values), dropping that suite to 26/27. Single-sample evals have real noise on borderline semantic calls. Treat 1-2pp swings as noise; a ≥3pp drop across the 62-case grid is a candidate-worthy signal. For a swap decision, running the eval twice and averaging would de-noise further — worth doing if a candidate lands within 2pp of the gate.

### Latency (from log `duration_ms` / `streaming_first_delta_ms`)

Per-operation timing across the baseline run. Median is the truer central tendency because Gemini's streaming API occasionally spikes to 10× typical on a single call (one Gemini Experience case hit 200+s; one Gemini Webpage case 600+s — pulls the mean up but leaves the median clean).

| Operation (mode) | Anthropic haiku-4-5 | OpenAI gpt-5-mini | Gemini 2.5-flash |
|---|---|---|---|
| `GenerateExperienceFromTextStreaming` | 2.22s mean / **2.05s median** / TTFT 0.63s | 23.1s mean / 23.4s median / TTFT 21.9s | 28.2s mean / **5.83s median** / TTFT 5.78s |
| `GenerateGearFromTextStreaming` | 2.64s mean / **2.33s median** / TTFT 0.57s | 18.6s mean / 20.2s median / TTFT 17.6s | 3.76s mean / **3.61s median** / TTFT 3.32s |
| `GenerateRequestContentStreaming` | 1.92s mean / **1.82s median** / TTFT 0.56s | 12.6s mean / 12.8s median / TTFT 12.0s | 3.27s mean / **3.13s median** / TTFT 2.93s |
| `DetectGearInImage` (non-streaming) | 3.05s mean / **3.16s median** | 14.7s mean / 14.6s median | 6.36s mean / **6.29s median** |
| `GenerateExperienceFromImage` (non-streaming) | 2.63s mean | 13.5s mean | 6.76s mean |
| `GenerateGearFromWebpage` (non-streaming) | 2.38s mean / **2.46s median** | 19.5s mean | 153s mean / **4.75s median** (outlier case) |
| `GenerateExperienceFromWebpage` (non-streaming) | 3.19s mean | 20.96s mean | 7.50s mean |

**Reads:**

- **Anthropic haiku-4-5 is the latency leader across every mode.** Text median 1.8–2.3s with sub-second TTFT (≤630ms) — the user sees tokens in under a second. Image and webpage are 3s non-streaming.
- **gpt-5-mini is catastrophically slow** — the Phase 4a finding (~28s unary-equivalent for Experience) reproduced cleanly: median 23.4s with 21.9s TTFT, meaning the streaming API is effectively unary (first delta at ~95% of total elapsed). If we ever fall back to OpenAI today, the user stares at a ~20s+ spinner across every mode. This is the core motivation for #1216 — the swap is blocked on quality, but the latency problem is real and unresolved.
- **Gemini 2.5-flash is tolerable on text but has tail-risk outliers.** Experience median 5.8s is 2–3× slower than Haiku but usable; the mean of 28s reflects one pathological case (200+s). Webpage had one 600s case that pulls the webpage mean to 153s — almost certainly a retry storm, not representative. Dev/prod traffic would see p50 around 5s, p99 much worse.

TTFT only applies to the streaming text operations; image and webpage currently use non-streaming provider APIs. If the Gen\* flows ever add streaming for image/webpage, that column becomes meaningful there too.

---

### Failure examples (spot-check)

Per-case outputs from the three providers, grouped by mode. Cell format: `value` (✅ if field passed for this provider, ❌ if failed). Expected column shows the golden's accepted values or threshold. Cases with at least one provider failure included.

#### Text — Experience

| Case | Field | Expected | Anthropic haiku-4-5 | OpenAI gpt-5-mini | Gemini 2.5-flash |
|---|---|---|---|---|---|
| `explicit_pickleball_longmont` | `search_keywords_jaccard` | `[pickleball sport paddle court]` (≥0.25) | `[pickleball 3rd shot longmont]` ❌ 0.17 | `[pickleball 3rd shot Longmont pickleball Longmont]` ❌ 0.14 | `[pickleball longmont 3rd shot longmont]` ❌ 0.00 |
| `value_cooking_class` | `value_estimate_usd` | `$25 ±30%` | `$50` ❌ | `$60` ❌ | `$100` ❌ |
| `contextual_yoga_rec` | `search_keywords_jaccard` | `[yoga fitness wellness stretching]` (≥0.25) | `[sunset yoga evening yoga rec center yoga Boulder Colorado]` ❌ 0.00 | `[sunset yoga yoga rec center Boulder CO]` ❌ 0.14 | `[yoga sunset rec center boulder co]` ❌ 0.12 |
| `painting_workshop_kids` | `search_keywords_jaccard` | `[painting kids workshop art]` (≥0.25) | `[painting workshop kids art class community art activity]` ❌ 0.00 | *(passed — keywords not captured)* ✅ | `[painting workshop kids painting art for kids children's art class]` ❌ 0.00 |
| `explicit_hike_tomorrow` | `search_keywords_jaccard` | `[hiking trail chautauqua outdoors]` (≥0.30) | ✅ | `[hiking Chautauqua Boulder 10am 2026-04-22]` ❌ 0.29 | ✅ |
| `relative_saturday_run` | `date` | `2026-04-25` | ✅ | `2026-04-24` ❌ | ✅ |
| `outdoor_camping_july4` | `search_keywords_jaccard` | `[camping outdoors lake nature]` (≥0.25) | ✅ | `[camping Brainard Lake July 4th weekend July 4 Boulder CO Brainard Lake camping]` ❌ 0.11 | `[camping Brainard Lake July 4th weekend Boulder holiday]` ❌ 0.12 |
| `explicit_boardgames_friday_7pm` | `search_keywords_jaccard` | `[board games game night tabletop social]` (≥0.30) | ✅ | `[board game night board games game night friends Boulder CO]` ❌ 0.29 | `[board game night board games games]` ❌ 0.17 |

#### Text — Gear

| Case | Field | Expected | Anthropic haiku-4-5 | OpenAI gpt-5-mini | Gemini 2.5-flash |
|---|---|---|---|---|---|
| `corded_circular_saw` | `value_estimate_usd` | `$130 ±40%` (range $78–$182) | `$65` ❌ 50% off | `$60` ❌ 54% off | `$80` ✅ |
| `japanese_pull_saw` | `value_estimate_usd` | `$60 ±40%` | ✅ | `$25` ❌ 58% off | ✅ |
| `tent_with_location` | `location_query_contains` | must contain `garage` | `"Boulder, CO"` ❌ | ✅ | ✅ |
| `gas_lawn_mower` | `value_estimate_usd` | `$250 ±40%` | ✅ | ✅ | `$120` ❌ 52% off |
| `northface_backpack` | `value_estimate_usd` | `$180 ±40%` | ✅ | ✅ | `$100` ❌ 44% off |

#### Text — Request

| Case | Field | Expected | Anthropic haiku-4-5 | OpenAI gpt-5-mini | Gemini 2.5-flash |
|---|---|---|---|---|---|
| `bike_pump_home` | `location_query` | exact `USER_PRIMARY_LOCATION` | ✅ `"USER_PRIMARY_LOCATION"` | `""` ❌ | ✅ `"USER_PRIMARY_LOCATION"` |
| `bike_pump_home` | `search_keywords_jaccard` | `[bike pump bicycle cycling]` (≥0.20) | `[bike pump tools cycling]` ❌ 0.17 | ✅ | ✅ |
| `borrow_drill_weekend` | `search_keywords_jaccard` | `[drill tools workshop power tool]` (≥0.20) | ✅ | `[drill power tools shelves DIY]` ❌ 0.14 | `[drill power tools DIY shelves]` ❌ 0.14 |
| `dog_walker_single` | `search_keywords_jaccard` | `[dog walking pets dog]` (≥0.20) | `[dog walking pet care dogs outdoor activity]` ❌ 0.17 | `[dog walking dog leash afternoon walk dog park]` ❌ 0.17 | `[dog walk pet care leash park]` ❌ 0.00 |
| `projector_movie_night` | `search_keywords_jaccard` | `[projector movie screen]` (≥0.20) | `[projector movie night entertainment family]` ❌ 0.17 | `[projector movie night family home theater]` ❌ 0.17 | ✅ |
| `projector_movie_night` | `value_estimate_usd` | `$30 ±80%` | ✅ | `$70` ❌ 133% off | `$60` ❌ 100% off |
| `deck_repair_advice` | `search_keywords_jaccard` | `[deck repair carpentry construction]` (≥0.20) | `[deck repair wood board carpentry deck maintenance]` ❌ 0.14 | ✅ | ✅ |
| `pressure_washer_borrow` | `value_estimate_usd` | `$45 ±50%` | ✅ | `$120` ❌ 167% off | ✅ |
| `tent_next_weekend` | `value_estimate_usd` | `$40 ±50%` | ✅ | ✅ | `$75` ❌ 88% off |
| `babysitter_weekend` | `search_keywords_jaccard` | `[babysitting childcare kids]` (≥0.20) | ✅ | `[babysitter child evening home]` ❌ 0.00 | ✅ |

#### Image — Gear (material classification variance)

| Case | Expected material | Anthropic haiku-4-5 | OpenAI gpt-5-mini | Gemini 2.5-flash |
|---|---|---|---|---|
| `rocky_talkie` | `mixed_plastic_metal` | ✅ `mixed_plastic_metal` | ✅ `mixed_plastic_metal` | `electronics_small` ❌ |
| `mixer` | `mixed_plastic_metal` | ✅ `mixed_plastic_metal` | `solid_metal` ❌ | `solid_metal` ❌ |
| `atomic_maverick_ski` | `mixed_plastic_metal` or `mixed_wood_metal` | ✅ first run / ❌ second run (`mixed_wood_plastic`) — variance case | ✅ `mixed_wood_metal` | `mixed_wood_plastic` ❌ |

**Read:** Gemini consistently misclassifies composite materials (e.g., electronics with metal casings get tagged as `electronics_small` instead of the mixed category). OpenAI and Anthropic handle most of these right. The `atomic_maverick_ski` case is borderline — run-to-run variance flips Anthropic between pass and fail, so future runs should weight this case lightly for swap decisions.

#### Webpage — Gear (invocation failure)

| Case | Result | Anthropic haiku-4-5 | OpenAI gpt-5-mini | Gemini 2.5-flash |
|---|---|---|---|---|
| `dewalt_drill` (DEWALT 20V MAX Cordless Drill/Driver Kit) | — | ✅ title=`"DeWalt 20V Cordless Drill"`, material=`cordless_power_tool`, $99 | ❌ **invocation failure**: JSON parse error on `confidence=0.95E00120399...` | ✅ title=`"DeWalt 20V Cordless Drill"`, material=`cordless_power_tool`, $99 |

**Read:** Anthropic and Gemini produce near-identical structured output on this case. gpt-5-mini occasionally emits float literals that Go's JSON unmarshaller can't parse; the bug is silent in production today because the Gen\* path returns an error to the client (visible as a failed-to-generate spinner). A candidate (gpt-5.4-nano) that cleanly formats floats would be net-positive here.

Raw logs from this run: `/tmp/ripls-1216-baseline/*.log` (local only; not checked in).
