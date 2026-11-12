# Issue #2225: Limit gear genAI description to one sentence; the rest are useless

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2225
**Filed:** 2026-05-27 | **Last activity:** 2026-05-27
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When the server uses an LLM to invent gear details from an uploaded photo,
the resulting `description` field consistently follows a two-part shape:

1. A crisp opening sentence that names the item and its primary use ("Compact,
   lightweight cordless drill with high-performance motor."). This sentence is
   genuinely useful — it gives the item an identity that the title + structured
   fields don't, and reads well on the gear card.
2. One or more follow-on sentences that are largely fluff — restating obvious
   visual aspects of the photo ("It has a black handle and red trigger."),
   inventing soft marketing copy ("Perfect for weekend projects!"), or
   paraphrasing structured fields the user can already see ("Weighs about 3.6
   lbs."). These sentences add no information beyond what the title, brand,
   model, weight, and image already convey, and they make the gear card feel
   AI-generated rather than authored.

The issue author asks that we constrain the AI to emit only the crisp first
sentence and drop the rest.

**Scope decision (from issue discussion):** this change targets *purely
AI-generated* description text — i.e. image mode. Descriptions that are
lifted or paraphrased from a real product page (webpage mode) and
descriptions sourced from the structured product-spec lookup are
manufacturer-authored, higher-signal text and stay as-is. The "fluff" pattern
above is an image-mode artifact: with no source text to lean on, the model
fills space by narrating the photo.

## Context from Comments

The issue has no comments. The description is short but unambiguous, and the
"first sentence is good, the rest is fluff" framing matches what we see
empirically in production gear cards.

## Codebase Findings

Three paths can populate `Gear.description` from AI; only one is in scope:

- **Text mode** (`server/services/gear/gen_ai.go:135`,
  `gearFromTextOutput` in `server/ai/schemas.go:188`) — the description is the
  user's prompt verbatim. The schema deliberately omits a description field
  and `generation.Description = prompt` stamps the user text in. **Out of
  scope, no change.**
- **Image mode** (`buildGearDetectionPrompt` in `server/ai/prompts.go:81`,
  `gearDetectionOutput` in `schemas.go:167`) — asks the model for
  `~300 chars, max 500` of "factual specs you can observe — size, condition,
  visible accessories, technical specs from labels." **In scope.**
- **Webpage mode** (`buildGearFromWebpagePrompt` in `prompts.go:1337`,
  `gearFromWebpageOutput` in `schemas.go:206`) — asks for `~200 chars, max
  400`, but the AI is paraphrasing real product-page text, not inventing.
  **Out of scope, no change.**

Adjacent behavior worth noting:

- **Product-spec override in image mode** (`server/services/gear/gen_ai.go:317`)
  — when the structured product-spec lookup returns a description and it's
  longer than the AI's, it *replaces* the AI's. Once the AI emits a single
  short sentence, the longer-wins comparison will reliably pick the
  manufacturer-authored spec text when one exists — which is exactly the
  behavior we want (real source beats AI invention). **No change needed.**
- **Streaming surface** (`server/services/gear/gen_ai_streaming.go:189,326,460`)
  — image and webpage modes emit a `Description` event mid-stream as the field
  arrives from the LLM. Shorter image-mode descriptions mean fewer mid-stream
  emits; the protocol and the webpage emit are unaffected.

## Approach

Tighten the image-mode prompt only — the simplest, most idiomatic change.

Rewrite the `description` block in `buildGearDetectionPrompt`
(`server/ai/prompts.go:99`) to require a single crisp sentence that names the
item and its primary use; drop the "factual specs you can observe — size,
condition, visible accessories, technical specs from labels" framing (that
phrasing is precisely what produces the photo-narration fluff); shrink the
size cue from `~300 chars, max 500` to `target ~80 chars, max 160`. Update
the schema doc comment on `gearDetectionOutput` (`server/ai/schemas.go:167`)
to match.

Pros: smallest possible change. Saves output tokens. No client changes.
Matches how the codebase already steers length and shape for every other
AI-generated field (title ~20 chars, reasoning ≤10 words, value reasoning
≤10 words) — and those targets hold up in practice. Webpage mode and the
product-spec override are untouched, so any time the description comes from
a real product source it keeps the manufacturer-authored richness.

Cons: frontier models occasionally ignore length hints. We accept that risk
rather than adding a server-side trim layer; if production gear cards
regress, a defensive trim can be added later as a small follow-up.

## Open Questions

- **What's the right target length for the new one-sentence description?**
  Recommendation: `target ~80 chars, max 160`. Long enough for "Compact
  cordless drill with high-performance motor and two-battery kit." (62 chars)
  and "Easy-setup dome tent with weatherproof floor and ventilation for
  four." (70 chars); short enough to force the model to pick one beat.

- **What about descriptions that already exist on previously-saved gear?**
  Recommendation: leave them alone. Migration churn isn't worth it for a
  cosmetic change, and users who want shorter descriptions can edit
  individual items. Note this explicitly in the PR so reviewers don't
  expect a backfill.

- **Do we need to update any documentation under `docs/server/` or `docs/ai/`
  that describes AI gear-generation output shape?**
  Recommendation: grep before the PR. If any reader-facing doc spells out a
  longer image-mode description shape, update it; otherwise skip.

## Implementation Plan

Single-phase change. All edits are in image-mode code paths.

### Phase 1: Tighten the image-mode prompt
**Goal:** Ship the user-visible improvement in a single small PR.
**Tasks:**
- [ ] Rewrite the `description` block in `buildGearDetectionPrompt`
  (`server/ai/prompts.go:99`) to require one crisp sentence that names
  the item and its primary use. Drop the "factual specs you can observe
  — size, condition, visible accessories, technical specs from labels"
  framing — that wording is what licenses the photo narration. Change
  the size cue to `target ~80 chars, max 160`.
- [ ] Update the schema doc comment on `gearDetectionOutput`
  (`server/ai/schemas.go:167`) to reflect the one-sentence shape, so a
  future reader of the schema doesn't go re-add length to the prompt.
- [ ] Refresh image-mode test fixtures so any multi-sentence
  `description` values become single sentences at the new length. Grep
  `server/services/gear/` (notably `gen_ai_test.go` and any aitest
  fixtures) and `server/ai/aitest/` for image-mode description strings;
  leave webpage-mode and text-mode fixtures alone.
- [ ] Inspect — but expect to leave unchanged —
  `server/ai/eval/testdata/gear_image_goldens.json`. The image-mode
  evaluator (`server/ai/eval/gear_image.go`) only asserts
  `has_description: true` on each case (a boolean non-empty check, not
  a length or content assertion), so a one-sentence description still
  satisfies the goldens. Touch only if a case ever grows a stricter
  assertion before this PR lands.
- [ ] Grep `docs/server/` and `docs/ai/` for any reader-facing doc that
  pins image-mode description length; update if found.
- [ ] Run `npm run build` (golangci-lint + `go build ./...`); run
  `go test ./server/services/gear/... ./server/ai/...`; if any
  `integration`-tagged test asserts on description content, also run
  `go test -tags=integration ./...`.
**Validates:** New gear created via photo upload in dev produces a
single crisp sentence on the gear card. Webpage-mode descriptions still
read as paraphrased product copy (regression check). The product-spec
override path naturally prefers the manufacturer text whenever a spec
lookup hits — confirm by uploading a photo of a well-known item (e.g.
DeWalt DCD771C2) and observing the longer spec description survive.

## Audit Notes (architectural compliance)

Verified against `docs/server/architecture.md`, `docs/server/observability.md`,
and the provenance scheme tracked in `server/impact_metrics/estimator/config.textproto`.

- **Scope vs. shared-library boundary.** All edits live in `server/ai/`,
  the shared AI library, and never leak into service-to-service coupling
  — image-mode callers (`server/services/gear/gen_ai.go`) consume the
  same `gearDetectionOutput` shape they do today. No new abstraction,
  no new dependency injection, no new file split. Compliant with the
  "shared functionality in libraries" pattern (server architecture §
  2).
- **Provenance version (`genai_from_image`) does NOT need bumping.**
  The provenance-tracked gear fields are `category`, `brand`, `model`,
  `material_category`, and `weight_grams`; `description` is not a
  tracked-provenance field, so this prompt edit doesn't change anything
  the `ShouldRefresh` staleness check looks at. Bumping
  `provenance_versions { key: "genai_from_image" value: 1 }` in
  `server/impact_metrics/estimator/config.textproto` would force a
  re-generation of every AI-classified gear's *tracked* metadata for
  no semantic gain. Explicitly leave the version at `1`.
- **Observability is unaffected.** The image-mode handler already logs
  through `logging.LoggerWithContext(ctx)` with the standard
  `operation`, `user_id`, `user_email`, `gear_id` fields; the prompt
  change emits no new log lines and changes no field semantics. No
  observability work required.
- **Streaming protocol is unchanged.** `gen_ai_streaming.go` still
  emits `Description` events as the field arrives from the LLM
  (`server/services/gear/gen_ai_streaming.go:326`); shorter content
  just means fewer mid-stream emits before the field closes. No client
  changes; no a11y considerations (no UI labels affected).
- **Testing follows the local-unit-test preference** (server
  architecture § 1.4): all assertions live next to the code they cover
  (`server/ai/`, `server/services/gear/`). No new integration test is
  needed for a prompt-content change; the `integration` re-run line
  above is a guard against accidentally tripping an existing
  description assertion behind that build tag.

## Related Issues

- #1002 (`docs/issues/1002-gear-from-url.md`) — the URL/webpage pathway,
  explicitly out of scope for this issue. Useful background for why
  webpage-mode descriptions are kept as-is.
