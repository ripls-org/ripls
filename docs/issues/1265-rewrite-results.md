# #1265 Gen* prompt rewrite — consolidated results

**Issue:** [#1265](https://github.com/Type2Fun-Foundation/ripls/issues/1265) · also closes [#1209](https://github.com/Type2Fun-Foundation/ripls/issues/1209)
**Plan:** [`docs/issues/1265-gen-prompt-rewrite.md`](1265-gen-prompt-rewrite.md)
**Date:** 2026-07-08
**PRs:** #2670 (Phases 1–2), #2678 (Phase 3), #2679 (Phase 4), Phase 5 follow-up PR
**Baselines:** [`docs/ai/eval/baselines/2026-07-08_*`](../ai/eval/baselines/) — `pre-rewrite`, `post-rewrite-text*`, `expanded-*-goldens`, `post-rewrite-media`

## TL;DR

All six target Gen* prompts rewritten with **zero eval regression** — every
(suite × production-pair) gate finished at or above its pre-rewrite pass rate,
several at 100%. Template sizes dropped 26–58%; live Anthropic input tokens
dropped 16–31%; gpt-5-mini (tertiary fallback) median latency dropped 40–44%
on text suites and 19–29% on media suites. Golden coverage on the previously
thin suites grew from 2–3 cases to 10–11, and the expanded suites caught and
then verified fixes for four real prompt defects.

## Template and wire-token sizes

Template bytes from `TestPromptSizes` (fixed short inputs); wire tokens are
Anthropic per-call medians from the eval logs (includes schema/tool overhead;
webpage/image cases also carry page text or image tokens, and the media
suites' pre/post case mixes differ 2–3 vs 10–11 cases, so wire deltas there
are indicative, not exact).

| Template | Bytes pre | Bytes post | Δ | Wire input tokens (median) |
|---|---|---|---|---|
| experience_from_text | 7,455 | 4,540 | −39% | 3,039 → 2,172 (−29%) |
| gear_from_text | 5,542 | 3,243 | −41% | 2,381 → 1,741 (−27%); output −10% |
| request_from_text | 3,931 | 2,918 | −26% | 1,890 → 1,591 (−16%) |
| experience_from_image | 9,388 | 3,980 | −58% | 4,858 → 3,355 (−31%, mix-confounded) |
| gear_from_webpage | 6,976 | 3,963 | −43% | 3,177 → 2,224 (−30%) |
| experience_from_webpage | 6,889 | 4,643 | −33% | 2,956 → 2,303 (−22%) |

## Pass rates (production trio)

Text suites compare against the same 26/13/12-case goldens (plus the new
negative assertions); media suites compare calibration → post-rewrite on the
expanded v2 goldens.

| Suite | Pair | Pre | Post |
|---|---|---|---|
| Experience text | haiku / gpt-5-mini / flash-lite | 99.2 / 99.2 / 99.1 | **100 / 99.2 / 100** |
| Gear text | haiku / gpt-5-mini / flash-lite | 98.7 / 98.7 / 97.3 | 98.7 / 98.7 / **98.7** |
| Request text | all three | 100 | 100 |
| Experience image | haiku / gpt-5-mini / flash-lite | 98.3 / 94.8 / 100 | **100 / 98.3** / 100 |
| Experience webpage | haiku / gpt-5-mini / flash-lite | 98.2 / 100 / 100 | **100** / 100 / 100 |
| Gear webpage | haiku / gpt-5-mini / flash-lite | 95.5 / 97.0 / 97.0 | **98.5 / 100 / 100** |

gpt-5-mini median latency: gear text 18.9→10.8s, request text 17.9→10.6s,
gear webpage 17.7→12.7s, experience webpage 18.2→14.5s, experience image
17.4→14.1s. Haiku and Flash-Lite medians flat-to-better throughout.

## Defects found and fixed along the way

1. **Explicit prices overridden** (caught by round-1 text eval): the compacted
   market-rate anchors out-competed user-stated prices ("$25 per person" →
   $75–100). Fixed with a price-precedence rule; `value_cooking_class` and
   `value_wine_tasting` now pass.
2. **Region echo** (caught by new `location_query_empty` assertion): Haiku
   copied USER REGION into `location_query` on non-event pages. Fixed with an
   explicit rule; webpage suite 98.2→100%.
3. **Placeholder dates** (caught by new `date_empty` assertion): Haiku emitted
   the literal `"<UNKNOWN>"` as a date on date-less flyers. Fixed; image suite
   98.3→100%.
4. **Dead output field**: gear-from-text emitted `model` on the wire, then the
   receiver discarded it — nothing consumed it on any path. Field removed from
   schema and prompt (output tokens −10%).

Also removed: schema-absent REASONING instructions (gear/request text, gear
webpage), the `experiences`-array response format that contradicted the flat
schema from #1226, `sources` in examples the schema rejects, and three prompt
examples that near-verbatim leaked eval goldens.

## #1209 close-out

`search_keywords_must_not_contain` (negative assertion) now guards the eight
venue/person-carrying experience goldens; venue/city/business anti-proper-noun
guidance added to the experience and request keyword sections. 8/8 assertions
pass on all three providers. Request prompts contain no proper nouns, so no
request-side assertions were needed.

## Durable guardrails added

- `TestPromptSizes` budget ratchet (default suite, no API cost): each template
  capped ~10% above its post-rewrite size; raising a budget requires a passing
  eval run in the same commit.
- Golden suites at version 2 with 10–11 cases each on the previously thin
  modes; flyer fixtures under `server/test_data/flyers/` with provenance
  README.
- New harness assertions: `search_keywords_must_not_contain`, `date_empty`,
  `location_query_empty`, `max_confidence`.

## Caveats

- The 2026-05-19 baselines (including the #2057 private-LLM runs) are
  superseded for cross-run comparison: prompts, goldens, and assertion sets
  all changed. Any future private-vs-cloud decision must re-run both sides.
- `modelThresholds` was left untouched — observed rates sit ≥3 pp above every
  gate on the expanded suites.
- Transient infra noise during runs (Vertex 502 streak, Vertex OAuth timeout,
  gpt-5-mini streaming timeouts) is recorded per-run in the reports'
  `invocation_failures`; every such case re-passed on retry.
