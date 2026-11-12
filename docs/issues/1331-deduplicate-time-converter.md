# Issue #1331: [architecture] Deduplicate convertTimeModelsToAPI across 4 services

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1331
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** auto-filed, review:architecture | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The function `convertTimeModelsToAPI` (and variants thereof) is copy-pasted
across four service packages — `experience`, `search`, `feed`, and `community`
— with subtle but real divergences between each copy. Because the copies have
drifted, bugs in one copy don't get fixed in the others, and new fields added
to the proto may only be wired in some of the copies.

Concrete divergences found across the four copies:

| Field | experience | search | feed | community |
|---|---|---|---|---|
| Specific.DurationMinutes | ✓ | ✓ | ✗ | ✗ |
| Specific.IsAllDay | ✗ | ✗ | ✗ | ✗ |
| Range.DurationMinutes | ✓ | ✓ | ✗ | ✓ |
| Range.IsAllDay | ✗ | ✗ | ✗ | ✓ |
| Default (unknown type) | TBD | nil | TBD | nil (verified) |

Both `DurationMinutes` and `IsAllDay` exist in both the models and API protos
for `SpecificTime` and `TimeRange`. The `feed` copy missing `DurationMinutes`
is a display bug: durations won't show up for feed experiences.

The issue body mentions three locations (experience, search, feed) but missed
the fourth copy in `community/shadow_users.go`.

## Context from Comments

Two comments from `maintainer` are just `@claude` pings — no additional
technical direction. The issue body itself provides the actionable recommendation:
add a shared `ConvertTimeModelsToAPI` to `server/services/` and delete the copies.

## Potential Solutions

### Option A: Add to existing `server/services/` package (new file)

- **Approach:** Create `server/services/experience_converters.go` in the
  `services` package with an exported `ConvertTimeModelsToAPI` function.
  Replace all four local copies with calls to it. All four sub-packages
  already import `type2fun.org/ripls/server/services`, so no new import
  cycles are introduced.
- **Pros:** Consistent with the existing pattern (`user_util.go` also lives here
  and provides shared model-to-API conversions). No new packages. Three of the
  four callers (`experience`, `search`, `feed`) already import
  `type2fun.org/ripls/server/services`, so only `community` needs a new import
  edge added. Easy to find.
- **Cons:** The `services` package is already the "dumping ground" for
  shared utilities; this adds to that. The `community` package does not currently
  import `services`, so one new import edge is unavoidable with this option.
- **Effort:** Small.

### Option B: New `server/services/converters/` package

- **Approach:** Create a dedicated converters sub-package. All four service
  packages would need a new import.
- **Pros:** Clear separation of concerns; converter logic has its own home.
- **Cons:** New package for what is currently one function; all four callers
  need a new import added. Overhead not justified until there are many converters.
- **Effort:** Small (but slightly more scaffolding).

### Option C: Add to `roundtrip_helpers.go` (as the issue suggests)

- **Approach:** Put the function directly in the existing `roundtrip_helpers.go`
  file.
- **Cons:** `roundtrip_helpers.go` currently only contains `AssertFieldRoundTrip`,
  a test-time generic helper. Adding a production models-to-API converter there
  would break the file's conceptual coherence. The file also imports `testing`,
  which is fine in test packages but mixing it with production code in one file
  is awkward.
- **Effort:** Small, but conceptually messy.

## Recommendation

**Option A**: add `ConvertTimeModelsToAPI` to a new file
`server/services/experience_converters.go` in the `services` package.

This follows the established pattern of `user_util.go` (shared model-to-API
helpers in the parent `services` package), incurs no new import edges, and
keeps all cross-service converters co-located.

The canonical implementation must:
1. Copy all fields present in both models and API protos: `DurationMinutes`
   and `IsAllDay` for both `SpecificTime` and `TimeRange`.
2. Take the address of local copies of `StartUnixSec`/`EndUnixSec` (not struct
   field addresses), matching the community pattern.
3. Return a TBD value for the `default` case (unknown oneof type) — more
   defensive than returning `nil`, consistent with the experience and feed
   copies which are the most complete.

The `community/shadow_users.go` copy (`shadowConvertTimeModelsToAPI`) should
also be replaced, bringing the total to four call-site updates.

## Open Questions

- **Which default return value is canonical — TBD or nil?**
  Recommendation: TBD. If a persisted experience has an unrecognized time type
  (e.g., from a future proto version), returning TBD displays gracefully rather
  than hiding the time field entirely. Two of four copies already use TBD.

- **Should `IsAllDay` be copied for `SpecificTime`?**
  Recommendation: Yes. The field exists in both models and API protos.
  All four current copies omit it, which is a latent bug: all-day specific
  times will silently lose their all-day flag on read.

- **Should we add nil-safety guards for `v.Specific` and `v.Range` sub-fields?**
  Recommendation: No. Proto3 guarantees the inner field is non-nil when the
  oneof case matches. Adding nil checks would be defensive code for a case
  that cannot happen and would obscure the main logic.
  Note: the `community` copy (`shadowConvertTimeModelsToAPI`) does contain these
  nil guards (`if v.Specific == nil { return nil }`), but they are not present
  in any other copy and should not be carried over to the canonical function.

## Implementation Plan

### Phase 1: Create the canonical shared function

**Goal:** Add `ConvertTimeModelsToAPI` to `server/services/experience_converters.go`
with complete field mapping and unit tests.

**Tasks:**
- [ ] Create `server/services/experience_converters.go` with exported
      `ConvertTimeModelsToAPI(*models.ExperienceTime) *api.ExperienceTime`
- [ ] Include all fields: `DurationMinutes` and `IsAllDay` for both Specific
      and Range cases; TBD for the default case
- [ ] Create `server/services/experience_converters_test.go` with table-driven
      tests covering all four oneof cases (Specific, Range, TBD, nil input)
      and verifying all fields including `DurationMinutes` and `IsAllDay`

**Validates:** The canonical function behaves correctly in isolation before
any callers are changed.

### Phase 2: Replace the four local copies

**Goal:** All four service packages use the shared function; no local copies remain.

**Tasks:**
- [ ] `server/services/experience/service.go`: replace `convertTimeModelsToAPI`
      definition and all call sites (lines 228, 422)
- [ ] `server/services/experience/time_proposals.go`: replace call at line 426
      (same local function; the definition removal in service.go covers it)
- [ ] `server/services/search/service.go`: replace `convertTimeModelsToAPI`
      definition and call sites (lines 367, 622, 649)
- [ ] `server/services/feed/service.go`: replace `convertExperienceTimeModelsToAPI`
      definition and call site (lines 1014, 1097)
- [ ] `server/services/community/shadow_users.go`: replace
      `shadowConvertTimeModelsToAPI` definition and call site (lines 325, 374);
      add `"type2fun.org/ripls/server/services"` to the import block — this
      package does not currently import the parent `services` package

**Validates:** `npm run build` passes; `go test -tags=integration ./...` green.

## Related Issues

- #1277 — similar duplicate-function cleanup in a different service
- #1273 — same category of server-side architecture cleanup
