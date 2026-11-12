# Issue #1516: [code_quality] Material-category enum exists but Go uses stringly-typed values

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1516
**Filed:** 2026-04-28 | **Last activity:** 2026-04-30
**Labels:** auto-filed, review:code_quality | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`proto/ripls/{api,models}/common.proto` define a `MaterialCategory` enum with
13 values (UNSPECIFIED + 12 real categories). Server Go code only uses the
typed enum on the *storage* side: `models.MaterialCategory` is the value type
of `models.TrackedMaterialCategory`, and `impact_metrics/estimator` indexes
emission factors by it.

On the AI-input side it is still a string. There are **two independent,
hand-maintained mappings** that must change in lockstep with the proto:

1. `server/ai/provider_openai.go:25-86` — `validateGearMetadata` keeps a
   `validCategories map[string]bool` plus a string-keyed default-fallback
   switch.
2. `server/services/gear/service.go:1348-1379` — `ParseMaterialCategory`
   keeps a 12-arm `switch` translating the AI's lowercase strings to
   `api.MaterialCategory` enum values.

`server/ai/{provider.go, schemas.go}` carry the value as
`MaterialCategory string` on `GearDetection` and `GearGeneration` plus the
streaming wire DTOs (`gearDetectionOutput`, `gearFromTextOutput`,
`gearFromWebpageOutput`). The eval framework
(`server/ai/eval/{gear,gear_image,gear_webpage}.go`) compares the string
field against an `[]string` allow-list with `strings.EqualFold`. The mock
provider hardcodes string values like `"mixed_plastic_metal"` in
`server/ai/provider_mock.go`. Sanitization
(`server/ai/sanitize.go:49,60`) is run on the string field but does not
validate against the enum set.

The cost: adding a new enum value requires editing N source files
(`provider_openai.go` validator map and default-fallback switch,
`gear/service.go` `ParseMaterialCategory`, `provider_mock.go` test
fixtures, plus the eval golden JSON files). Errors surface only at
runtime when an unknown string falls through to UNSPECIFIED. The two
hand-maintained mappings are also redundant — they encode the same
enum-value↔JSON-string pairing twice.

## Context from Comments

The only substantive comments on the issue are the auto-triage entry
(2026-04-28: Type=Task, Priority=P3) and a `@claude` mention. There are
no clarifications or maintainer disagreements to resolve. The issue body
itself prescribes the fix:

> **Recommendation:** convert at the AI JSON boundary only; use
> `models.MaterialCategory` everywhere else in Go. Add a unit test
> asserting every proto enum value has a JSON-string mapping.

This plan honors that recommendation, but flags one subtlety in Open
Questions: the AI-side code currently sits on `api.MaterialCategory` at
the conversion seam (`ParseMaterialCategory` returns `api.MaterialCategory`
because `api.DetectedGearItem` carries it), not `models.MaterialCategory`.
We resolve this by anchoring on `api.MaterialCategory` for the AI →
service path (it is what the response struct already wants) and let the
existing `convertAPITrackedMaterialCategoryToStorage` handle the
storage hop.

## Potential Solutions

### Option A: Single canonical helper, keep AI struct fields as `string`
- **Approach:**
  - Add `server/ai/material_category.go` with two functions:
    - `MaterialCategoryFromJSON(s string) api.MaterialCategory` — the
      lookup AI input ⇒ enum.
    - `MaterialCategoryJSON(c api.MaterialCategory) string` — the inverse
      enum ⇒ lowercase JSON token (e.g.
      `MATERIAL_CATEGORY_SOLID_METAL` → `"solid_metal"`).
    - Both are derived **once** from `api.MaterialCategory_value` /
      `_name` at package-init time by stripping the
      `MATERIAL_CATEGORY_` prefix and lowercasing — no hand-maintained
      table.
  - Replace `validateGearMetadata`'s `validCategories` map with
    `MaterialCategoryFromJSON(materialCategory) != UNSPECIFIED`. Keep
    the title/category-driven default-fallback switch (it is product
    heuristic, not a proto-bound table).
  - Replace `gear.ParseMaterialCategory`'s 12-arm switch with a
    one-line call to `ai.MaterialCategoryFromJSON`. Keep it as a
    re-export shim or inline-and-delete (recommend inline-and-delete:
    its callers can call `ai.MaterialCategoryFromJSON` directly since
    they already import `ai`).
  - Add `server/ai/material_category_test.go`:
    - `TestEveryMaterialCategoryHasJSONMapping`: iterates
      `api.MaterialCategory_name`, requires every non-UNSPECIFIED
      value round-trips through
      `MaterialCategoryJSON`/`MaterialCategoryFromJSON`.
    - `TestMaterialCategoryFromJSONUnknownReturnsUnspecified`: drift
      guard for unrecognized strings.
  - Update mock provider, sanitize, and eval to call the helper for
    validation. Eval keeps its `[]string` golden format (it is JSON
    anyway).
  - **Tests added/updated:** mapping round-trip test
    (`server/ai/material_category_test.go`), updated assertions in
    `server/services/gear/gen_ai_test.go` (no behavior change, but the
    helper is exercised), no new golden JSON.
- **Pros:**
  - Eliminates both hand-maintained tables in one shot. The single
    canonical mapping is reflection-derived, so adding a new enum
    value Just Works.
  - Minimal blast radius on call sites: AI struct types still carry
    `string`, so AI providers (OpenAI/Gemini/Anthropic streaming
    code) need zero changes other than the validator-map deletion.
  - Closes the issue's exact ask ("unit test asserting every proto
    enum value has a JSON-string mapping") with a one-file test.
- **Cons:**
  - The AI struct fields remain stringly typed. A future contributor
    could still introduce a new untyped string compare. (Mitigated
    by the central helper being the *only* path that produces those
    strings.)
- **Effort:** Small.

### Option B: Type the AI struct fields as `api.MaterialCategory`
- **Approach:**
  - Change `GearDetection.MaterialCategory` and
    `GearGeneration.MaterialCategory` from `string` to
    `api.MaterialCategory`.
  - Implement `UnmarshalJSON` on a small `MaterialCategoryJSON` wrapper
    (or on the parent struct) that converts the JSON
    `"solid_metal"` token to the enum on the way in, and back on the
    way out for the JSON schema generator.
  - Remove `ParseMaterialCategory` entirely; AI struct already holds
    the typed value.
  - Touch every provider/streaming/mock/sanitize/eval file that
    reads the field, plus the JSON-schema generator
    (`generateSchema[T]`) and the streaming JSON parser
    (`pkg/streamparse` if it inspects the schema).
- **Pros:**
  - Enforces type safety at the boundary itself — no future
    untyped-string compare possible.
  - Self-documenting struct definitions.
- **Cons:**
  - Large blast radius: streaming wire DTOs
    (`gearDetectionOutput` etc.) currently use `string` so OpenAI's
    structured-output schema reflects to `"type": "string"`. Switching
    to a Go enum type changes the schema reflector's output and may
    break OpenAI Strict-mode compatibility unless we keep two struct
    layers (wire DTO with string + post-converted struct with enum).
  - The JSON-schema generator (`invopop/jsonschema`) does not know
    about proto enums; we'd need a custom `JSONSchemaAlias` or
    enum-list generator to keep the LLM-facing schema correct.
  - Eval golden files compare against `[]string`; they would need
    a converter or the field would need a custom `MarshalJSON`
    that emits the lowercase token (effectively re-introducing
    Option A's helper internally).
- **Effort:** Medium-to-large.

### Option C: Move the canonical mapping into proto via custom options
- **Approach:** Add a custom `(json_token)` enum-value option in the
  proto and generate the mapping. Or: change the proto JSON name via
  `[json_name = "solid_metal"]` and use proto JSON marshalling.
- **Pros:** Mapping lives in the schema itself.
- **Cons:** Requires `buf` plugin / build wiring changes; adds proto
  toolchain risk far out of scale for the bug.
- **Effort:** Large.

## Recommendation

**Option A.** It exactly matches the issue body's recommendation
("convert at the AI JSON boundary only"), eliminates both
hand-maintained tables in one helper, and adds the requested
round-trip test. The reflection-derived mapping means adding an enum
value tomorrow needs only the `.proto` edit — no other Go changes.
Option B is over-investment given the helper in Option A is the same
function we'd write for B's `UnmarshalJSON` anyway, and B risks
breaking the OpenAI Strict-mode JSON schema. Option C is far out of
scope.

## Open Questions

- **Should the helper return `api.MaterialCategory` or
  `models.MaterialCategory`?** — Recommendation: `api.MaterialCategory`.
  All AI-side callers ultimately produce an `api.DetectedGearItem`
  whose `MaterialCategory` field is `api.MaterialCategory` (see
  `server/services/gear/gen_ai_streaming.go:485`). The
  storage type is reached via the existing
  `convertAPITrackedMaterialCategoryToStorage` shim
  (`server/services/gear/service.go:1126`). The issue body says
  `models.MaterialCategory` but the `api.` and `models.` enums are
  numerically identical and the conversion already exists, so
  anchoring on `api.` matches the existing seam without changing
  the convention.
- **Where does the helper live?** — Recommendation:
  `server/ai/material_category.go` (new file, in package `ai`).
  Callers in `server/services/gear/` and `server/jobs/` already
  import `server/ai`, so no new import edges. Co-locating the
  helper next to `sanitize.go` and `provider.go` keeps boundary
  conversion logic together.
- **Should `validateGearMetadata`'s default-fallback switch
  ("power tool" → mixed_plastic_metal etc.) be removed?** —
  Recommendation: leave it. It is product heuristic — what to do
  when the LLM returned an empty/unknown value — not a proto
  binding. The validator-map check (the part that goes stale on
  enum change) is what we replace.
- **Should `gear.ParseMaterialCategory` be kept as a thin shim
  or deleted?** — Recommendation: delete it. All four call sites
  (`server/services/gear/{detect_url,gen_ai,gen_ai_streaming}.go`,
  `server/jobs/impact_backfill.go`) already import
  `server/ai`, and the inline call
  `ai.MaterialCategoryFromJSON(generation.MaterialCategory)`
  reads as well as the helper. Keeping a shim doubles the surface
  for no benefit.
- **Should the eval framework
  (`server/ai/eval/gear*.go`) switch from `EqualFold` string
  compare to enum compare?** — Recommendation: keep its current
  `[]string` golden format; have it call the new helper to
  validate that each `material_category_any_of` entry parses to a
  known enum. Add a one-time test at load that fails the suite if
  a golden JSON contains an unknown token. This catches typos in
  the goldens without forcing schema changes.

## Implementation Plan

### Phase 1: Canonical helper + round-trip test
**Goal:** Single source of truth for AI-string ↔ enum mapping plus
the requested enforcement test.
**Tasks:**
- [ ] Create `server/ai/material_category.go`:
  - Build `materialCategoryByJSON map[string]api.MaterialCategory`
    and `materialCategoryToJSON map[api.MaterialCategory]string` in
    `init()` from `api.MaterialCategory_name` by stripping the
    `MATERIAL_CATEGORY_` prefix and lowercasing.
  - Export `MaterialCategoryFromJSON(s string) api.MaterialCategory`
    (returns UNSPECIFIED for empty/unknown after trimming +
    lowercasing).
  - Export `MaterialCategoryJSON(c api.MaterialCategory) string`
    (returns "" for UNSPECIFIED).
  - Add `// Package ai ...` already exists in `doc.go`; no new
    package comment needed.
- [ ] Create `server/ai/material_category_test.go`:
  - `TestEveryMaterialCategoryHasJSONMapping`: iterate
    `api.MaterialCategory_name`, assert every non-zero value has a
    non-empty `MaterialCategoryJSON` mapping that round-trips.
  - `TestMaterialCategoryFromJSONUnknown`: empty / "unknown" /
    "BogusValue" all return UNSPECIFIED.
  - `TestMaterialCategoryFromJSONLowercases`: "Solid_Metal" parses
    correctly (defensive trim + lower).
- [ ] Run `npm run build` and `go test ./server/ai/...`.
**Validates:** Mapping is correct and locked to the proto enum.

### Phase 2: Replace hand-maintained tables
**Goal:** Both validator map and switch deleted; all callers go
through the helper.
**Tasks:**
- [ ] `server/ai/provider_openai.go:25-86`:
  - Delete `validCategories map[string]bool` (lines 28-33).
  - Replace `validMaterial == "" || !validCategories[validMaterial]`
    with `MaterialCategoryFromJSON(materialCategory) ==
    api.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED`.
  - Keep the title/category default-fallback switch as-is — it is
    product heuristic, not enum-bound.
- [ ] `server/services/gear/service.go:1348-1379`:
  - Delete `ParseMaterialCategory`.
- [ ] Update the four call sites
  (`server/services/gear/{detect_url.go:82, gen_ai.go:167,361,475,
  gen_ai_streaming.go:474,517}` and
  `server/jobs/impact_backfill.go:316`) to call
  `ai.MaterialCategoryFromJSON(...)` directly.
- [ ] Update `server/ai/sanitize.go:49,60` to also clear values
  that don't parse to a known enum (defensive — keeps
  unknown-token data from leaking into AI struct fields). Add a
  test case in `server/ai/sanitize_test.go`.
- [ ] Update `server/ai/eval/{gear.go:113-126,
  gear_image.go:99-111, gear_webpage.go:90-100}` so the
  string compare path additionally validates each
  `material_category_any_of` entry resolves through
  `MaterialCategoryFromJSON`. Surface mismatch as a test failure
  (typo guard for goldens).
- [ ] Run `npm run build` then
  `go test -tags=integration -timeout 20m ./server/ai/...
  ./server/services/gear/... ./server/jobs/...`.
**Validates:** Build is green; existing tests still pass; no
hand-maintained material-category list remains.

### Phase 3: Documentation + lint guard (optional, light)
**Goal:** Make it discoverable and future-proof.
**Tasks:**
- [ ] Update `server/ai/README.md` with a "Material category
  mapping" subsection pointing at `material_category.go` as the
  single source of truth.
- [ ] Optional `scripts/check_material_category.sh` CI gate that
  greps for `case "solid_metal"` or
  `map[string]bool{...material...}` patterns outside
  `server/ai/material_category.go` and fails build. Skip if the
  team prefers test-only enforcement.
- [ ] Run `npm run build` one final time.
**Validates:** Future contributors are nudged to the helper, not
back into the per-file table pattern.

## Related Issues

- None directly. The proto enum was introduced as part of the
  carbon-impact estimator workstream (see
  `server/impact_metrics/estimator/`), which already uses the
  typed `models.MaterialCategory` and is the canonical example
  of the typed-enum pattern this plan extends to the AI-input
  side.

## Audit notes (`/ripls-audit`)

Reviewed against `docs/server/architecture.md` and
`docs/server/observability.md`:

- **Service Independence:** Helper lives in `server/ai/` (shared
  library), not in a service. Callers in `server/services/gear/`
  and `server/jobs/` already import `server/ai`; no new
  service-to-service edges. ✓
- **Shared Functionality in Libraries:** `server/ai` is the
  correct location — it owns the AI ↔ Go boundary. ✓
- **Proto Convention (api vs models):** Helper anchors on
  `api.MaterialCategory` to match the existing seam at
  `api.DetectedGearItem.MaterialCategory`. The `api.` and
  `models.` enums are numerically identical and the existing
  `convertAPITrackedMaterialCategoryToStorage` shim handles
  the storage hop, so there is no convention violation. ✓
- **Testing Strategy:** New tests follow table-driven unit-test
  pattern from `docs/server/architecture.md` §Testing Strategy
  → Unit Tests, located alongside implementation
  (`server/ai/material_category_test.go`). ✓
- **Observability:** No new logging surfaces (the helper is
  pure conversion). The default-fallback in
  `validateGearMetadata` already logs at `WARN` via
  `logging.Logger`, which we preserve. ✓
- **Error Handling — Fail Fast:** `MaterialCategoryFromJSON`
  returns UNSPECIFIED on unknown input rather than erroring,
  matching the existing contract (callers already handle
  UNSPECIFIED). This is documented behavior, not a silent
  swallow. ✓
