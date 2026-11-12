# Issue #1226: Consolidate Gen* JSON output schemas into a single source of truth per method

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1226
**Filed:** 2026-04-22 | **Last activity:** 2026-04-25
**Labels:** none (Triage: Type=Task, Priority=P3)
**Assignees:** briangbrown
**Context docs:** none

## Problem Statement

Each Gen* AI method declares its JSON output schema independently in
each provider implementation — typically as a `type outputSchema
struct { ... }` block inside the method body, with `json:"..."` tags
that name the wire fields. After today's audit (post-#1507):

- **46 inline `outputSchema` structs** across the six provider files
  (`provider_anthropic.go`, `provider_anthropic_streaming.go`,
  `provider_openai.go`, `provider_openai_streaming.go`,
  `provider_gemini.go`, `provider_gemini_streaming.go`).
- A separate `valueEstimateSchema` is re-declared **11 times**
  across Anthropic + OpenAI + Gemini (1 in `provider_anthropic.go`,
  2 in `provider_gemini_streaming.go`, 8 in
  `provider_openai_streaming.go`) — a slim variant of the canonical
  `ValueEstimate` (no `Sources []string`).
- The `StreamFieldKey` constants in `server/ai/streaming_provider.go`
  are a **fifth copy** of these wire names. A typo there silently
  breaks streaming fan-out — the parser doesn't fire the watched key,
  the early-fire branch never starts, and the unary-path behavior
  shows up with no error.
- Every prompt-builder in `server/ai/prompts.go` names the same
  fields a third or fourth time in natural language.

PR #1507 just demonstrated the feasibility of consolidation by using
the canonical `GearDetection` struct directly as the schema for
`DetectGearInImageStreaming` across all three providers (no inline
duplicate). That pattern works and cuts ~30 lines of duplication
per-provider for that one method.

## Context from Comments

- **Triage 2026-04-25 (maintainer):** Type=Task, Priority=P3.
  "Engineering lens — DRY refactor, code quality." No technical
  pushback or scope adjustment.

The issue body itself proposes three options ranked, with the
author's recommendation: ship (B) as a cheap safety net and file (A)
as the real fix. Given that PR #1507 has now delivered a partial
proof-of-concept of (A) — using `GearDetection` directly as the
schema across all three providers' detect-gear streaming — the
"real fix" half of the recommendation is no longer speculative.

## Codebase verification (2026-04-28, post-#1507)

- **`generateToolSchema[T any]()`** already exists in
  `server/ai/provider_anthropic.go` — reflects a Go struct into the
  Anthropic tool-input schema format. Used by every Anthropic method.
- **`generateSchema[T any]()`** already exists in
  `server/ai/provider_openai.go` — same idea for OpenAI's
  `ResponseFormatJSONSchema`. Used by every OpenAI structured-output
  method.
- **Genkit** uses Go reflection on the input struct directly; no
  helper is needed (provider_gemini_streaming.go already passes
  `outputSchema{}` to `streamStructuredOutput`).
- The infrastructure for "feed a Go struct, get a provider-shaped
  schema" already exists. The missing piece is a **canonical
  per-method struct definition** that all three providers can share.
- **Existing canonical types** in `server/ai/provider.go`:
  `ExperienceGeneration`, `GearGeneration`, `RequestGeneration`,
  `CommunityGeneration`, `GearDetection`, `ValueEstimate`. These are
  receiver structs (what `Provider.Generate*` returns). Some are
  already used as schemas (PR #1507), but most aren't — the inline
  per-method schemas frequently differ from the canonical types in
  small ways (text-mode experience has no `Description` field;
  webpage-mode experience has no `SceneType`; etc.).
- **Watch-key constants** (`server/ai/streaming_provider.go`) —
  `StreamFieldTitle`, `StreamFieldDescription`, etc. — are
  hand-aligned with the JSON tags in the inline schemas. No
  compile-time check ties them together.

The canonical types are close to the schemas but not identical. A
clean consolidation must reconcile this drift — either by broadening
the canonical types, or by introducing per-method schema structs that
are explicitly the source of truth.

## Potential Solutions

### Option A: Per-method canonical schema structs in `server/ai/schemas.go`

- **Approach:** Create one shared struct per (method × mode) tuple
  in `server/ai/schemas.go` (e.g., `experienceFromTextOutput`,
  `experienceFromImageOutput`, `gearFromTextOutput`,
  `gearFromWebpageOutput`, `requestFromTextOutput`,
  `requestFromImageOutput`, `gearDetectionOutput` — already done as
  `GearDetection` in #1507). Each struct has the `json:"..."` tags
  that are the wire names. All three providers' unary + streaming
  methods import that struct and pass it through the existing
  `generateToolSchema[T]()` / `generateSchema[T]()` reflectors. Add
  one more reflection helper that derives the `StreamFieldKey` watch
  list from a struct's `json` tags so the watched keys can't drift.
- **Pros:**
  - Eliminates 46 inline schema structs + ~8 inline
    `valueEstimateSchema` copies.
  - Adding a field requires touching one struct, not 6-8 files
    per provider.
  - Watched-keys list is mechanically derived from the struct tags,
    so a typo in one location can't silently break streaming fan-out.
  - Existing infra (`generateToolSchema`, `generateSchema`) already
    works for the Go-struct → provider-schema step. No new
    framework needed.
  - PR #1507 already shipped this pattern for `GearDetection`; the
    rest of the methods just follow suit.
- **Cons:**
  - Not all canonical types match all per-method schemas exactly
    (e.g., text-mode Experience has no `Description` in the schema
    today). Each consolidation requires deciding either to broaden
    the schema (slightly more LLM output tokens, possibly negligible)
    or to introduce a method-specific struct. Both are fine; the
    decision needs to happen per-method.
  - Schema-reflection + watch-key-from-tags helper is one more
    reflection helper to maintain. Small surface area but real.
- **Effort:** medium. ~1 file per method-group + reviewing the
  per-method schema for each provider to confirm equivalence + an
  init-time test that the watched-keys list matches the struct tags.

### Option B: Assertion test only (the issue body's "cheap interim")

- **Approach:** Add a test in `server/ai` that uses Go reflection to
  walk each provider's inline `outputSchema` for each Gen method,
  collect the `json` tags, and assert each `StreamFieldKey` and each
  `*StreamingKeys` entry appears as a top-level tag in every
  expected method. Catches drift at `go test` time. Doesn't fix the
  duplication.
- **Pros:**
  - Closes the silent-failure mode (typo → fan-out doesn't fire,
    no error) immediately.
  - Tiny diff. No structural change to providers.
- **Cons:**
  - Doesn't reduce duplication. Adding a new field still requires
    editing 6-8 files per provider.
  - Reflection-on-inline-types in tests is awkward — the
    `outputSchema` types are scoped inside method bodies and aren't
    importable from tests, so the test would need either to scrape
    the source files or to be re-implemented per provider.
- **Effort:** small.

### Option C: Status quo + comment

- **Approach:** Leave duplication in place; add a doc-comment in
  `streaming_provider.go` explaining the implicit coupling between
  `StreamFieldKey` constants and provider schemas.
- **Pros:** Zero implementation cost.
- **Cons:** Does nothing. Issue body explicitly calls this the
  "weakest option."
- **Effort:** trivial.

## Recommendation

**Option A**, building on PR #1507's proof-of-concept. The
infrastructure (`generateToolSchema`, `generateSchema`, Genkit
reflection) already exists. PR #1507 already demonstrated the
pattern works for `DetectGearInImage` across all three providers
without quality regression. The remaining methods just follow the
same template.

Skip Option B as a separate intermediate step — Option A's
init-time watch-keys-from-tags helper closes the same silent-failure
mode that B's assertion test would close, with no extra cost. The
issue body's recommendation to ship B + file A reflected the state
**before** #1507 demonstrated A was tractable. With the partial-A
already in main, going straight to full-A is the right call.

Sequencing inside Option A: do this **per method-group** to keep
each PR small and revertible:

1. Experience (text + image + webpage)
2. Gear (text + webpage; image already done in #1507)
3. Request (text + image)
4. Other Gen methods that aren't streaming (Community, Suggestions,
   Story, Nudge, etc. — lowest priority since the fan-out drift
   risk doesn't apply)

Each phase is one PR, eval-gated on the same suite that gates PR
#1507.

## Open Questions

- **Should the schema struct be the same as the canonical receiver
  type (e.g., `ExperienceGeneration`), or a method-specific
  schema-only struct?** — Recommendation: **method-specific
  schema-only structs in `server/ai/schemas.go`.** The canonical
  types in `provider.go` are receiver-side and may include fields
  the LLM doesn't generate (computed at call-site, e.g.,
  `MentionedNames` is a slice that may be `nil` rather than
  required). Schemas-only structs let us be explicit about what the
  LLM is asked to produce vs. what the receiver carries afterward.
  PR #1507 used `GearDetection` directly because it happens to be
  receiver+schema-equivalent; not all methods are.
- **What about the `valueEstimateSchema` slim variant — promote
  `ValueEstimate` (with `Sources []string`) everywhere, or keep a
  slim form?** — Recommendation: **keep a slim
  `valueEstimateOutput` in `schemas.go`** (no `Sources`) because the
  LLMs we currently use don't reliably populate `sources` and
  including it nudges them to fabricate URLs. Receiver still uses
  the canonical `ValueEstimate` with `Sources` (empty slice from
  schema → empty slice on receiver, no information loss).
- **Where should the watch-keys-from-tags helper live?** —
  Recommendation: `server/ai/streaming_provider.go` next to the
  existing `streamFieldKeysToStrings` helper. One function:
  `experienceStreamingKeysFromSchema[T]() []StreamFieldKey` that
  reflects on `T`, reads each top-level `json:"..."` tag, and
  filters to the keys that match a known `StreamFieldKey` constant.
  Used at package init time to populate `experienceStreamingKeys`,
  `gearStreamingKeys`, `requestStreamingKeys` from the canonical
  schema struct rather than hand-listed.
- **Does this change anything wire-visible?** — Recommendation: **it
  must not.** The wire field names are the contract with each
  provider's API. Any consolidation must produce JSON-equivalent
  output. The eval suite is the gate that confirms this.

## Audit findings (2026-04-28)

Reviewed against `docs/server/architecture.md` and the post-#1507 codebase. Plan adjustments below were applied where appropriate; nothing material had to change because this is a pure server-side refactor that doesn't cross any architectural boundary.

- **Inline schema count corrected.** Initial draft said "8 inline `valueEstimateSchema` copies"; actual count is **11** (1 in `provider_anthropic.go`, 2 in `provider_gemini_streaming.go`, 8 in `provider_openai_streaming.go`). The 46 inline `outputSchema` count is accurate. Updated below.
- **Reflection infrastructure confirmed.** `generateToolSchema[T]()` (Anthropic) and `generateSchema[T]()` (OpenAI) both already exist and are used by every provider method that needs a JSON-schema. Genkit reflects the input struct directly. No new framework needed; the plan's claim is accurate.
- **`GearDetection` direct-as-schema usage in #1507 confirmed.** Verified in `provider_anthropic_streaming.go`, `provider_openai_streaming.go`, and `provider_gemini_streaming.go` — all three use `GearDetection` (canonical type) as the schema for `DetectGearInImageStreaming`. The proof-of-concept is real.
- **Service Independence (Pattern 2 in arch guide).** Plan only touches `server/ai/`. The `Provider` interface signature doesn't change; only the internal schema declarations move from inline to a shared file. Independent of every service in `server/services/`.
- **File-size guidelines.** Arch doc says <500 ideal, <1000 max. The new `server/ai/schemas.go` will be one struct per (method × mode) — roughly 7–10 structs, ~12 lines each plus doc comments and the slim `valueEstimateOutput`. Estimated ~250–350 lines; comfortably under the 500 ideal.
- **`provider_*.go` files shrink** — each method body loses its inline schema struct (~10 lines) plus its inline `valueEstimateSchema` (~5 lines) when present. Nudges the existing 700–850 line files back toward the ideal.
- **Wire compatibility.** The refactor must be byte-equivalent on the wire. The eval gate (≥85% pass-rate per provider per suite, used for #1451) is sufficient evidence — same input, same field names, same JSON output structure. No proto change, no client visibility.
- **i18n / client architecture / client caching.** Reviewed — not applicable. No user-facing strings, no client RPC contract change, no caching surface.
- **Migration ordering robustness.** Each phase migrates one method-group at a time. If any phase's eval-gate run regresses, that phase reverts independently without affecting the prior phase. Safer than a single big-bang refactor.

## Implementation Plan

### Phase 1: Foundations + Experience consolidation

**Goal:** Land `server/ai/schemas.go` with the canonical
per-method-group structs and the watch-keys-from-tags helper. Migrate
all Experience streaming methods (text + image + webpage) across all
three providers off inline schemas and onto the shared structs.

**Tasks:**

- [ ] Create `server/ai/schemas.go` with:
  - `experienceFromTextOutput`
  - `experienceFromImageOutput`
  - `experienceFromWebpageOutput`
  - `valueEstimateOutput` (slim variant of canonical `ValueEstimate`,
    no `Sources`)
  - Doc comment explaining: these are wire-only schemas; receiver
    types live in `provider.go`.
- [ ] Add reflection helper in `streaming_provider.go`:
  ```go
  // streamingKeysFromSchema returns the StreamFieldKey constants
  // matching the top-level json tags of T, in declaration order.
  // Panics at init time if T has any json tag that doesn't match a
  // known StreamFieldKey constant — keeps wire names + watch-keys
  // in lockstep.
  func streamingKeysFromSchema[T any]() []StreamFieldKey
  ```
- [ ] Replace `experienceStreamingKeys` declaration in
  `streaming_provider.go` with `streamingKeysFromSchema[experienceFromTextOutput]()`.
- [ ] Migrate all six Experience methods (3 providers ×
  unary-as-drain + streaming) to use the shared struct via
  `generateToolSchema[experienceFromTextOutput]()` /
  `generateSchema[experienceFromTextOutput]()` /
  `streamStructuredOutput(... experienceFromTextOutput{})`.
- [ ] Run the existing eval gate (same as PR #1507): all three
  providers, all suites, ≥85% pass-rate.
- [ ] Append result entry to
  `docs/evals/1157-genexperience-latency.md` matching the 2026-04-22
  Phase 4a format.
- [ ] `npm run build` + `go build -tags=integration ./...` clean.

**Validates:** Foundations work; experience methods are
schema-de-duplicated; watch-keys-from-tags helper prevents the
silent-failure drift. Each subsequent phase is mechanical.

### Phase 2: Gear consolidation

**Goal:** Migrate Gear text and webpage methods to shared schemas.
Image is already done (PR #1507 used `GearDetection` directly — keep
that or move it to `gearDetectionOutput` for consistency).

**Tasks:**

- [ ] Add `gearFromTextOutput` and `gearFromWebpageOutput` structs to
  `schemas.go`.
- [ ] Decide whether to introduce a `gearDetectionOutput` schema-only
  struct or keep using `GearDetection` directly (was #1507's choice).
  Recommendation: keep using `GearDetection` since it's already
  wire-aligned and was deliberately chosen in #1507.
- [ ] Replace `gearStreamingKeys` with
  `streamingKeysFromSchema[gearFromTextOutput]()`.
- [ ] Migrate Gear text + webpage methods on all three providers.
- [ ] Eval gate.

**Validates:** Same shape as Phase 1, second domain.

### Phase 3: Request consolidation

**Goal:** Migrate Request text + image methods.

**Tasks:**

- [ ] Add `requestFromTextOutput` and `requestFromImageOutput`
  structs.
- [ ] Replace `requestStreamingKeys` with
  `streamingKeysFromSchema[requestFromTextOutput]()`.
- [ ] Migrate Request methods on all three providers.
- [ ] Eval gate.

**Validates:** All four streaming-aware Gen methods are now
schema-de-duplicated.

### Phase 4 (optional): Non-streaming Gen methods

**Goal:** Consolidate the remaining unary-only methods
(`GenerateCommunityContent`, `GenerateExperienceSuggestions`,
`GenerateRequestSuggestions`, `GenerateStoryContent`,
`GenerateNudgeContent`, `GenerateConversationSummary`,
`GenerateExperienceCompletionSummary`, `InferSocialAttributes`,
`ParseInformalTime`).

**Tasks:**

- [ ] One struct per method in `schemas.go`.
- [ ] Migrate unary methods. No streaming surface, so no watch-keys
  involvement.
- [ ] Eval gate (only the suites that touch these methods).

**Validates:** Full schema consolidation; `provider_*.go` files no
longer contain inline `outputSchema` blocks for any Gen method. Phase
4 is optional because the original issue's risk model
(silent-failure on streaming watch-key drift) doesn't apply to
unary-only methods — they fail loudly. Worth doing for code-quality
reasons but not urgent.

## Related Issues

- **#1218 / PR #1212** — introduced `StreamFieldKey` and the
  experiences/gear/request streaming-keys lists. The fifth copy of
  the wire names that this issue calls out.
- **#1451 / PR #1507** — flattened image-mode schemas and used
  `GearDetection` directly as the schema across all three providers.
  Proof-of-concept for Option A; this plan extends the same pattern
  to the remaining methods.
- **#1216** — re-evaluate AI provider model defaults using Phase 4a
  benchmark findings. Independent of this work but eval-suite-adjacent
  — Phase 1 of this plan should re-run the same suite #1216 uses.
- **#1157** — parent latency-optimization issue.
- **`docs/evals/1157-genexperience-latency.md`** — eval gate runs
  append entries here; the 2026-04-22 Phase 4a format and the
  2026-04-27 #1451 entry are both reference layouts.
