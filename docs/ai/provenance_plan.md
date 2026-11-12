# Provenance & Versioning for Metadata and Impact Fields

## Overview

This document describes a unified `Provenance` system that tracks **how, when, and by what version** each metadata and impact field was produced. This replaces the scattered provenance-like fields (`EstimationMethod`, `reasoning`, `sources`, `methodology_doc`) currently duplicated across `MoneySavings`, `PreventedEmissions`, `TimeSavings`, `CarbonEstimate`, and `ValueEstimate`.

### Why

Today, each impact sub-message carries its own `method`, `reasoning`, `sources`, and `methodology_doc` fields — four separate fields repeated in five different message types. Gear metadata fields (brand, category, material_category, weight, embodied_carbon) have **no** provenance tracking at all. We can't answer:

- Was this gear's brand set by the user or by an LLM?
- Which version of the GenGear prompt produced this material_category?
- Should we re-estimate this field when the algorithm improves?
- Was this value extracted from a product URL, detected from an image, or typed manually?

### Goals

1. **Unified provenance type**: One `Provenance` message used everywhere, replacing the scattered method/reasoning/sources/methodology_doc fields.
2. **Refresh control**: Given a field's provenance, decide whether to overwrite it when re-running estimation (e.g., never overwrite user-set fields; only re-estimate LLM fields when a newer algorithm version is available).
3. **Gear metadata tracking**: Know the origin of each gear field (brand, model, category, material_category, weight_grams, embodied_carbon, value_estimate).
4. **Impact estimate tracking**: Know which algorithm version produced each impact dimension's estimate, enabling selective re-estimation.
5. **Audit trail**: The client can display "How was this estimated?" using provenance data instead of ad-hoc reasoning strings.

### Design Decisions

- **Option 1 (typed wrappers)** — each polymorphic field gets a wrapper message that pairs its value with a `Provenance`. For example, gear's `string brand` becomes `TrackedString brand`. For fields that are already message types (CarbonEstimate, ValueEstimate), provenance is embedded directly.
- **Provenance replaces `EstimationMethod`** — the `ProvenanceSource` enum is a superset of `EstimationMethod`, covering both impact estimation methods and metadata sources.
- **Provenance replaces `reasoning`, `sources`, `methodology_doc`** — `reasoning` and `sources` become fields on `Provenance` itself. `methodology_doc` is removed entirely — it is implied by the source + version combination, and the server maps provenance to the appropriate doc at render time.
- **`ValueEstimate` gains Provenance** — its existing `confidence`, `reasoning`, `sources` fields are subsumed by `Provenance`.
- **`Estimate` stays bare** — the `Estimate` message (mean + stddev) remains a pure numerical type without provenance. Provenance belongs on the parent message that owns the estimate.

### Non-Goals

- Provenance on non-AI fields (gear name, description, media_ids, location_id) — these are always user-set.
- Historical provenance log (tracking all past values) — we only store the current provenance.
- Provenance on community-level aggregated metrics — aggregates are always computed from constituent transactions.

---

## Proto Design

### `Provenance` Message (in `common.proto`)

```protobuf
// common.proto (ripls.models package)

// ProvenanceSource indicates how a value was produced.
enum ProvenanceSource {
  PROVENANCE_SOURCE_UNSPECIFIED = 0;

  // User manually entered or corrected this value.
  PROVENANCE_SOURCE_USER = 1;

  // LLM inferred from a text prompt (e.g., "cordless drill").
  PROVENANCE_SOURCE_TEXT_VIA_LLM = 2;

  // LLM inferred from an uploaded image.
  PROVENANCE_SOURCE_IMAGE_VIA_LLM = 3;

  // LLM inferred from a fetched web page (URL mode).
  PROVENANCE_SOURCE_WEB_VIA_LLM = 4;

  // Computed by a deterministic formula (e.g., weight * emission factor).
  // The version field holds the formula name and revision.
  PROVENANCE_SOURCE_FORMULA = 5;

  // Derived from a configuration default value.
  PROVENANCE_SOURCE_CONFIG_DEFAULT = 6;
}

// Provenance records how, when, and by what version a value was produced.
// Used on metadata fields (gear brand, material_category, etc.) and impact
// estimate dimensions (money saved, emissions prevented, time saved).
message Provenance {
  // How this value was produced.
  ProvenanceSource source = 1;

  // Short identifier for the estimation method (e.g., "prevented_purchase",
  // "weight_material_carbon"). Used as a key for stddev lookup and display.
  string name = 2;

  // Version identifier for the algorithm or prompt that produced this value.
  // Examples: "genai_v3", "carbon_material_v1", "config_2026.1"
  // For FORMULA sources, holds the formula name: "weight_material_carbon_v1"
  int32 version = 3;

  // LLM-produced confidence score (0.0–1.0) when source is an LLM type.
  optional float confidence = 4;

  // Human-readable explanation of how the value was determined.
  // Replaces the per-message `reasoning` field.
  optional string reasoning = 5;

  // References used to produce this value (URLs, database names, study citations).
  // Replaces the per-message `sources` field.
  repeated string sources = 6;
}
```

The same `Provenance` and `ProvenanceSource` are mirrored in `ripls.api` (following the existing api/models separation pattern).

### Wrapper Messages for Scalar Fields

```protobuf
// In impact_estimate.proto (ripls.models package)

// TrackedString pairs a string value with its provenance.
message TrackedString {
  string value = 1;
  Provenance provenance = 2;
}

// TrackedMaterialCategory pairs a MaterialCategory value with its provenance.
message TrackedMaterialCategory {
  MaterialCategory value = 1;
  Provenance provenance = 2;
}
```

### Updated Gear Proto

```protobuf
message Gear {
  // ... fields 1-9 unchanged ...

  // Value estimate with provenance.
  // Provenance embedded directly in ValueEstimate (see below).
  ValueEstimate value_estimate = 10;

  // AI-detected product category (e.g., "Power Tools", "Camping & Outdoors").
  TrackedString category = 11;

  // AI-detected brand name (e.g., "DeWalt", "Coleman").
  TrackedString brand = 12;

  // AI-detected model identifier (e.g., "DCD771C2").
  TrackedString model = 13;

  // Material composition category for carbon estimation table lookup.
  TrackedMaterialCategory material_category = 14;

  // Estimated weight (used for carbon estimation).
  // Provenance on the parent TrackedEstimate (not on Estimate itself).
  TrackedEstimate weight_grams = 15;

  // Embodied carbon computed at GenGear time from material and weight.
  // Provenance embedded directly in CarbonEstimate (see below).
  CarbonEstimate embodied_carbon = 16;

  // ... field 20 unchanged ...
}
```

### Updated Message-Type Fields (Provenance Embedded)

For fields that are already message types, add `Provenance` directly rather than wrapping:

```protobuf
// TrackedEstimate pairs an Estimate with its provenance.
// Used where Estimate needs provenance tracking (e.g., gear weight_grams).
message TrackedEstimate {
  Estimate value = 1;
  Provenance provenance = 2;
}

// Estimate stays bare — pure numerical type.
message Estimate {
  float mean = 1;
  float stddev = 2;
}

// CarbonEstimate: replace `method` with `provenance`.
message CarbonEstimate {
  Estimate co2e_grams = 1;
  // EstimationMethod method = 2;  // REMOVED — replaced by provenance
  Provenance provenance = 2;       // NEW (reuses field number 2)
}

// ValueEstimate: replace confidence/reasoning/sources with provenance.
message ValueEstimate {
  float estimated_value_usd = 1;
  // float confidence = 2;          // REMOVED
  // string reasoning = 3;          // REMOVED
  // repeated string sources = 4;   // REMOVED
  Provenance provenance = 2;        // NEW (reuses field number 2)
}
```

**Wire compatibility note**: Reusing field numbers with different types is a **breaking wire change**. This is acceptable because:
- The `provenance` branch has not shipped to production yet.
- All existing data will be backfilled.
- If backward compat is needed, use new field numbers instead (e.g., `provenance = 5` on ValueEstimate).

### Updated Impact Estimate Messages

```protobuf
// MoneySavings: value + provenance only.
message MoneySavings {
  Estimate value_usd = 1;
  // string reasoning = 2;          // REMOVED
  // repeated string sources = 3;   // REMOVED
  // string methodology_doc = 4;    // REMOVED
  // EstimationMethod method = 5;   // REMOVED
  Provenance provenance = 2;        // NEW (replaces all four removed fields)
}

// PreventedEmissions: sub-estimates carry their own provenance;
// top-level provenance covers the aggregated reasoning.
message PreventedEmissions {
  CarbonEstimate manufacture_avoided_carbon = 1;
  CarbonEstimate waste_reduced_carbon = 2;
  // string reasoning = 3;          // REMOVED
  // repeated string sources = 4;   // REMOVED
  // string methodology_doc = 5;    // REMOVED
  // EstimationMethod method = 6;   // REMOVED
  Provenance provenance = 3;        // NEW
}

// TimeSavings: value + provenance only.
message TimeSavings {
  Estimate minutes = 1;
  // string reasoning = 2;          // REMOVED
  // repeated string sources = 3;   // REMOVED
  // string methodology_doc = 4;    // REMOVED
  // EstimationMethod method = 5;   // REMOVED
  Provenance provenance = 2;        // NEW
}
```

### Delete `EstimationMethod` Enum

The `EstimationMethod` enum is fully replaced by `ProvenanceSource` + `Provenance.version`:

| Old EstimationMethod | New Provenance |
|---------------------|----------------|
| `PRODUCT_LCA` | source=FORMULA, version="product_lca_v1" |
| `WEIGHT_MATERIAL` | source=FORMULA, version="weight_material_v1" |
| `CATEGORY_AVERAGE` | source=FORMULA, version="category_average_v1" |
| `SPEND_BASED` | source=FORMULA, version="spend_based_v1" |
| `LLM_INFERENCE` | source=TEXT_VIA_LLM (or IMAGE/WEB depending on input) |
| `PREVENTED_PURCHASE` | source=FORMULA, version="prevented_purchase_v1" |
| `SERVICE_VALUE` | source=CONFIG_DEFAULT, version="service_value_v1" |
| `COMMERCIAL_VALUE` | source=CONFIG_DEFAULT, version="commercial_value_v1" |
| `CONFIG_DEFAULT` | source=CONFIG_DEFAULT |
| `AI_ESTIMATED` | source=TEXT_VIA_LLM (or IMAGE/WEB) |

### Methodology Doc Resolution

With `methodology_doc` removed from proto messages, the server resolves the appropriate doc from provenance at render time:

```go
// MethodologyDocForProvenance returns the doc path for a given metric + provenance.
func MethodologyDocForProvenance(metric string, prov *Provenance) string {
    // Maps (metric, source+version) → doc path
    // e.g., ("money_saved", FORMULA, "prevented_purchase_v1") → "money_saved.md"
    // e.g., ("emissions_prevented", FORMULA, "weight_material_v1") → "emissions_prevented.md"
    switch metric {
    case "money_saved":
        return "money_saved.md"
    case "emissions_prevented":
        return "emissions_prevented.md"
    case "time_saved":
        return "time_saved.md"
    }
    return ""
}
```

This keeps the methodology doc mapping server-side and avoids storing redundant paths on every transaction.

---

## Refresh Logic

The provenance system enables principled refresh decisions:

```go
// ShouldRefresh returns true if a field should be re-estimated with a new algorithm.
// Looks up the current version from cfg by provenance name.
func ShouldRefresh(prov *models.Provenance, cfg *estimator.Config) bool {
    if prov == nil {
        return true // No provenance = always estimate
    }
    if prov.Source == models.ProvenanceSource_PROVENANCE_SOURCE_USER {
        return false // Never overwrite user-set values
    }
    currentVersion := cfg.ProvenanceVersion(prov.Name)
    if currentVersion == 0 {
        return false // Unknown name — cannot determine staleness
    }
    return prov.Version < currentVersion
}
```

### Version Scheme

Versions use a simple `name_vN` format:
- `genai_v1`, `genai_v2` — GenGear prompt versions
- `weight_material_v1` — carbon formula versions
- `prevented_purchase_v1` — savings formula versions
- `config_2026.1` — config default versions

When the GenGear prompt changes meaningfully, bump the version. The backfill job can selectively re-estimate fields where `version < current_version` and `source != USER`.

---

## Implementation Phases

### Phase 1: Proto Foundation

**Goal**: Define `Provenance`, `ProvenanceSource`, and wrapper types. Update all impact estimate messages. Delete `EstimationMethod`. Generate code, fix compilation.

**Session start**: Read this doc, then `proto/ripls/models/common.proto`, `proto/ripls/models/impact_estimate.proto`, `proto/ripls/api/impact_estimate.proto`.

- [x] **1a. Add Provenance and ProvenanceSource to common.proto**
  - Added `ProvenanceSource` enum and `Provenance` message to `proto/ripls/models/common.proto`
  - Mirrored in `proto/ripls/api/common.proto`
  - Run `npm run generate`

- [x] **1b. Add TrackedString, TrackedMaterialCategory, and TrackedEstimate**
  - Added `TrackedString`, `TrackedMaterialCategory`, `TrackedEstimate` to `proto/ripls/models/impact_estimate.proto`
  - Mirrored in `proto/ripls/api/impact_estimate.proto`

- [x] **1c. Add Provenance to CarbonEstimate**
  - Replaced `EstimationMethod method` with `Provenance provenance` on `CarbonEstimate` (field 2) in both api and models
  - Fixed Go/Dart compilation errors referencing `.Method` on `CarbonEstimate`

- [x] **1d. Replace scattered fields on MoneySavings, PreventedEmissions, TimeSavings**
  - MoneySavings: removed `reasoning`, `sources`, `methodology_doc`, `method`; added `Provenance provenance` (2)
  - PreventedEmissions: same pattern; added `Provenance provenance` (3)
  - TimeSavings: same pattern; added `Provenance provenance` (2)
  - Applied to both api and models protos; fixed Go/Dart compilation

- [x] **1e. Replace confidence/reasoning/sources on ValueEstimate with Provenance**
  - Removed `confidence`, `reasoning`, `sources` from ValueEstimate in both api and models
  - Added `Provenance provenance` (2); fixed all Go/Dart compilation errors

- [x] **1f. Delete EstimationMethod enum + update config**
  - Removed `EstimationMethod` from both api and models `impact_estimate.proto`
  - Changed `CarbonMethodStddev.method` from `EstimationMethod` to `string` in `estimator_config.proto`
  - Updated `config.textproto` to use string method names (e.g., `"weight_material_v1"`)
  - Updated `config.go` to use `map[string]float32` for method stddevs

- [x] **1g. Add confidence field to Provenance** (added during implementation)
  - Added `optional float confidence = 4` and made `reasoning` optional (field 5) in both api and models `common.proto`
  - Wired LLM-produced confidence through all services converting `ai.ValueEstimate` to proto
  - Fixed Dart test imports for `Estimate`/`CarbonEstimate` moved to `common.pb.dart`

**Tests pass**: `npm run generate` (clean), `go build ./...` (compiles), `go test ./...` (all pass), `flutter test` (all pass)

---

### Phase 2: Update Gear Storage Model

**Goal**: Change gear metadata fields from bare types to tracked types with provenance.

**Session start**: Read `proto/ripls/models/gear.proto`, `proto/ripls/api/gear_service.proto`, `server/services/gear/service.go`, `server/services/gear/gen_ai.go`.

- [x] **2a. Update gear.proto metadata fields to tracked types**
  - `string category = 11` → `TrackedString category = 11`
  - `string brand = 12` → `TrackedString brand = 12`
  - `string model = 13` → `TrackedString model = 13`
  - `MaterialCategory material_category = 14` → `TrackedMaterialCategory material_category = 14`
  - `Estimate weight_grams = 15` → `TrackedEstimate weight_grams = 15`
  - `CarbonEstimate embodied_carbon = 16` — already has Provenance from Phase 1c
  - `ValueEstimate value_estimate = 10` — already has Provenance from Phase 1e

- [x] **2b. Update API gear metadata proto**
  - Updated `GearMetadata` in `proto/ripls/api/gear_service.proto` to use tracked types
  - `DetectedGearItem` keeps bare types — provenance is assigned at SaveGear time

- [x] **2c. Fix Go server compilation**
  - `server/services/gear/service.go`: Added conversion helpers (convertAPITrackedStringToStorage, etc.) and accessor helpers (trackedStringValue, trackedEstimateMean, trackedMaterialCategoryValue)
  - `server/services/gear/service.go`: Updated SaveGear insert/update, GetGear conversion, convertStorageGearMetadataToAPI
  - `server/impact/builder.go`: Updated `gearWeightGrams()` to read `.Value.Mean` from TrackedEstimate
  - `server/jobs/impact_backfill.go`: Updated `gearNeedsEnrichment`, `applyGearGeneration`, `computeEmbodiedCarbon` for tracked types

- [x] **2d. Fix Go tests**
  - Updated `service_test.go`, `builder_test.go`, `impact_backfill_test.go`, `test_helpers.go` for tracked types

- [x] **2e. Fix Dart client compilation**
  - Updated `gear_metadata_formatter.dart` to read `.value` from tracked types
  - Updated `gen_gear_view_model.dart` to wrap DetectedGearItem bare fields into tracked types for GearMetadata
  - Updated `gear_metric_view_model.dart` for tracked field access
  - Updated `gear_metadata_formatter_test.dart`

**Tests pass**: `go test ./server/services/gear/...` ✓, `go test ./server/impact/...` ✓, `go test ./server/jobs/...` ✓, `flutter test` ✓

**Design note on DetectedGearItem**: GenGear returns AI suggestions that the user previews and may edit before SaveGear. DetectedGearItem uses bare types; provenance is assigned at SaveGear time based on which generation mode was used. SaveGear knows the source because the client passes it (or infers from existing fields like `source_url`). If the user edits a field before saving, the client can flag those fields as USER source.

---

### Phase 3: Update Impact Estimator Library

**Goal**: Update the estimator library to produce `Provenance` instead of scattered fields.

**Session start**: Read `server/impact/estimator/savings.go`, `server/impact/estimator/time.go`, `server/impact/estimator/gear_carbon.go`, `server/impact/estimator/carbon.go`.

- [x] **3a. Update savings.go**
  - All functions already populate Provenance with source, name, reasoning, sources (done during Phase 1)
  - Added `Version` from config-driven `provenance_versions` map via `cfg.ProvenanceVersion(name)`
  - `CalculateMoneySaved()`, `calculateGearEmissionsPrevented()`, `calculateRequestEmissionsPrevented()`, `calculateExperienceEmissionsPrevented()`, `CalculateTimeSaved()` all set version

- [x] **3b. Update time.go**
  - All functions already populate Provenance (done during Phase 1)
  - Added `Version` from config via `cfg.ProvenanceVersion(name)`
  - `EstimateGearTime()`, `EstimateRequestTime()`, `EstimateExperienceTime()` all set version

- [x] **3c. Update gear_carbon.go and carbon.go**
  - `EstimateGearCarbon()` already sets Provenance on CarbonEstimate (done during Phase 1)
  - Added `Version` from config via `cfg.ProvenanceVersion(name)`
  - `LookupMaterialCarbon()` / `LookupSpendCarbon()` return bare Estimate (provenance belongs on parent CarbonEstimate)

- [x] **3d. Update builder.go**
  - All builder functions already propagate Provenance (done during Phase 2)
  - `mergeValueSources()` merges provenance; `scaleMoneySaved()`/`scaleEmissionsPrevented()` preserve it
  - `gearCarbonToAPI()` converts provenance including version field
  - Top-level PreventedEmissions provenance now copies version from embodied carbon provenance

- [x] **3e. Update calculator.go**
  - `APIImpactToModels()` / `modelsImpactToAPI()` already convert Provenance (done during Phase 2)
  - `apiProvenanceToModels()` / `modelsProvenanceToAPI()` handle all fields including version

- [x] **3f. Add MethodologyDocForProvenance helper**
  - `MethodologyDocForProvenance(name, cfg)` in `server/impact/estimator/methodology.go`
  - Maps provenance name → methodology doc filename using config MethodologyDocs
  - Replaces the removed `methodology_doc` proto field; existing `GetMethodologyDoc` RPC unchanged (client still passes doc_path)

- [x] **3g. Update all estimator tests**
  - `savings_test.go`, `time_test.go`: added version assertions on all provenance checks
  - `config_test.go`: added `TestProvenanceVersionKnownName` and `TestProvenanceVersionUnknownReturnsZero`
  - `methodology_test.go`: added `TestMethodologyDocForProvenance` covering all names + unknown + empty
  - Test inputs updated to include Version (matching production EstimateGearCarbon output)

- [x] **3h. Add provenance_versions to estimator config**
  - Added `map<string, int32> provenance_versions` field to `EstimatorConfig` proto
  - Populated all 9 named sources at version 1 in `config.textproto`
  - Added `ProvenanceVersion(name) int32` accessor on `Config`

**Tests pass**: `go test ./server/impact/...` ✓, `go test ./server/storage/...` ✓, `flutter test` ✓

---

### Phase 4: Update Client Display

**Goal**: Update the Flutter client to read provenance instead of scattered fields.

**Session start**: Read `app/lib/core/utils/savings_formatter.dart`, `app/lib/core/utils/audit_trail_builder.dart`, `app/lib/presentation/widgets/item/item_metric_data.dart`.

- [x] **4a. Update savings_formatter.dart** (already done in Phase 1)
  - All three `_build*Detail()` methods already read `.provenance` with `provenanceDisplayName()` helper
  - `provenanceDisplayName()` maps provenance name strings to display names

- [x] **4b. Update audit_trail_builder.dart** (already done in Phase 1)
  - All three thread builders already read `p.reasoning`, `p.sources` from provenance

- [x] **4c. Update item_metric_data.dart** (already done in Phase 1)
  - `SavingsMetricDetail` and `ItemMetricValue` already have provenance-derived fields (methodName, reasoning, sources, methodologyDocPath)

- [x] **4d. Update gear_metadata_formatter.dart**
  - Replaced hardcoded reasoning/confidence/methodName on category, brand, model, material, weight metrics
  - Now reads provenance from TrackedString, TrackedMaterialCategory, TrackedEstimate types
  - Uses shared `SavingsFormatter.provenanceDisplayName()` (removed duplicate private `_provenanceDisplayName`)
  - Added `_provenanceConfidence()`, `_provenanceReasoning()`, `_provenanceSources()` helpers
  - Weight/embodied carbon fall back to stddev-based confidence when provenance has no confidence

- [x] **4e. Update gear_metric_view_model.dart**
  - VALUE metric now reads confidence, methodName, reasoning, sources from `ValueEstimate.provenance`

- [x] **4f. Update client tests**
  - `gear_metadata_formatter_test.dart`: added provenance extraction test group (8 tests covering all tracked types, fallback confidence, null provenance, empty fields)
  - `savings_formatter_test.dart`: already tested provenance reading (Phase 1)

**Tests pass**: `flutter test --reporter expanded` ✓ (2685 tests)

---

### Phase 5: Provenance-Aware GenGear & SaveGear

**Goal**: Set provenance on gear metadata fields based on the generation mode. Implement refresh logic.

**Session start**: Read `server/services/gear/gen_ai.go`, `server/services/gear/service.go`.

- [x] **5a. Config versions + GenGear version field**
  - Added 4 `provenance_versions` entries to `config.textproto`: `genai_from_text`, `genai_from_image`, `genai_from_web`, `genai_value_estimate`
  - Set `Version` on `ValueEstimate.Provenance` in all 3 GenGear modes (text, media, URL), guarded by nil check on `estimatorCfg`
  - Commit: "Add GenAI provenance versions to config and set version on ValueEstimate provenance"

- [x] **5b. SaveGear insert provenance + generation_mode**
  - Added `generation_mode` field to `SaveGearRequest` proto (field 8)
  - Server helpers: `generationModeToProvenanceName`, `buildMetadataProvenance`, `applyDefaultProvenance*` variants
  - Insert path applies LLM provenance to metadata fields lacking provenance
  - Client determines and passes generation mode from `DetectedGearItem.sourceUrl` + `inputMode`
  - Commit: "Add generation_mode to SaveGearRequest and apply LLM provenance on insert"

- [x] **5c. ShouldRefresh helper**
  - `server/impact/provenance.go`: `ShouldRefresh(prov, cfg)` — single function that looks up current version from config by provenance name
  - nil→true, USER→false, unknown name→false, otherwise compares stored vs current version
  - Tests: nil provenance, USER source, up-to-date/outdated version, unknown config name
  - Commit: "Add ShouldRefresh provenance helpers for staleness detection"
  - Follow-up: consolidated `ShouldRefreshWithConfig` into `ShouldRefresh` (no production callers for the raw-version variant)

- [x] **5d. SaveGear update USER provenance**
  - Update path detects changed metadata values via `trackedStringChanged`, `trackedMaterialCategoryChanged`, `trackedEstimateChanged`
  - Changed fields get `Provenance{Source: USER}`, unchanged fields keep existing provenance
  - Recomputes embodied carbon when material or weight changes
  - Commit: "Track user edits as USER provenance in SaveGear update path"

- [x] **5e. Provenance-aware backfill**
  - `gearNeedsEnrichment` checks provenance versions via `ShouldRefresh` in addition to missing fields
  - `applyGearGeneration` uses `ShouldRefresh` to skip user-set or up-to-date fields
  - `inferBackfillProvenanceName` derives mode from gear attributes (source_url → web, media_ids → image, else text)
  - Sets provenance on all updated metadata fields during backfill
  - Commit: "Make backfill provenance-aware: respect user edits, detect stale versions"

**Tests to pass**: `go test ./server/services/gear/...`, `go test ./server/impact/...`, `go test ./server/jobs/...`

---

### Phase 6: Integration Tests & Cleanup

**Goal**: End-to-end verification, cleanup, and documentation.

- [x] **6a. Update integration tests**
  - Added provenance assertions to all lifecycle tests (transfer, request, experience)
  - Verified TimeSaved.Provenance is set with CONFIG_DEFAULT source, non-empty name, version > 0
  - User-provenance retention already covered by existing tests:
    - `impact_backfill_test.go`: TestImpactBackfill_GearUserProvenancePreserved
    - `gear/service_test.go`: TestService_SaveGear_InsertProvenance + update tests
  - Commit: "Add provenance assertions to integration lifecycle tests"

- [x] **6b. Update impact_improvements_plan.md**
  - Added Phase 10 section referencing this provenance work
  - Updated status to "Phases 1-10 COMPLETE"
  - Commit: "Update impact plan with Phase 10: Provenance & Versioning"

- [x] **6c. Clean up dead code** (no-op)
  - Grep confirms no `EstimationMethod` or `methodDisplayName` references remain in `.go` or `.dart` files
  - Only references are in documentation (this file, impact_improvements_plan.md) describing the migration history

**Tests pass**: `go test ./server/services/transfer/... ./server/services/request/... ./server/services/experience/...` ✓

---

## File Inventory

### Proto Changes

| File | Changes |
|------|---------|
| `proto/ripls/models/common.proto` | Add `ProvenanceSource` enum, `Provenance` message |
| `proto/ripls/api/common.proto` (new or existing) | Mirror `ProvenanceSource`, `Provenance` for API |
| `proto/ripls/models/impact_estimate.proto` | Add `TrackedString`, `TrackedMaterialCategory`, `TrackedEstimate`; add `Provenance` to `CarbonEstimate`; replace scattered fields on `MoneySavings`, `PreventedEmissions`, `TimeSavings`; delete `EstimationMethod` |
| `proto/ripls/api/impact_estimate.proto` | Same changes as models |
| `proto/ripls/models/gear.proto` | Change `category`, `brand`, `model` to `TrackedString`; change `material_category` to `TrackedMaterialCategory`; change `weight_grams` to `TrackedEstimate` |
| `proto/ripls/api/gear_service.proto` | Update `GearMetadata` for tracked types |
| `proto/ripls/models/value.proto` | Replace `confidence`/`reasoning`/`sources` with `Provenance` |
| `proto/ripls/api/value.proto` | Same |

### Go Server Changes

| File | Changes |
|------|---------|
| `server/impact/estimator/savings.go` | Build Provenance instead of individual fields |
| `server/impact/estimator/time.go` | Build Provenance |
| `server/impact/estimator/gear_carbon.go` | Provenance on CarbonEstimate |
| `server/impact/estimator/carbon.go` | Return provenance info from lookups |
| `server/impact/builder.go` | Propagate Provenance in all builders |
| `server/impact/calculator.go` | Convert Provenance in api↔models functions |
| `server/services/gear/service.go` | Set/read tracked types, populate provenance |
| `server/services/gear/gen_ai.go` | Pass creation mode for provenance |
| `server/services/gear/stats.go` | Read tracked types |
| `server/services/impact/methodology.go` | Resolve methodology doc from provenance |
| `server/jobs/impact_backfill.go` | Read tracked types, provenance-aware refresh |
| `server/impact/provenance.go` (new) | `ShouldRefresh(prov, cfg)` — single staleness check helper |
| Tests: `savings_test.go`, `time_test.go`, `builder_test.go`, `calculator_test.go`, `gear_carbon_test.go`, `impact_backfill_test.go` | Assert Provenance fields |

### Dart Client Changes

| File | Changes |
|------|---------|
| `app/lib/core/utils/savings_formatter.dart` | Read `.provenance` instead of `.method`/`.reasoning`/etc. |
| `app/lib/core/utils/audit_trail_builder.dart` | Read `.provenance.reasoning`, `.provenance.sources` |
| `app/lib/presentation/widgets/item/item_metric_data.dart` | Update `SavingsMetricDetail` |
| `app/lib/presentation/screens/impact/audit_trail_screen.dart` | Use provenance for method display |
| Tests: `savings_formatter_test.dart` | Update for Provenance |

---

## Migration & Backward Compatibility

### Wire Breaks

This plan involves **intentional wire breaks** on field numbers:
- `CarbonEstimate.method` (field 2) → `CarbonEstimate.provenance` (field 2, different type)
- `ValueEstimate.confidence` (field 2) → `ValueEstimate.provenance` (field 2, different type)
- `MoneySavings.reasoning` (field 2) → `MoneySavings.provenance` (field 2, different type)
- `PreventedEmissions.reasoning` (field 3) → `PreventedEmissions.provenance` (field 3, different type)
- `TimeSavings.reasoning` (field 2) → `TimeSavings.provenance` (field 2, different type)
- `Gear.category` (field 11) → `Gear.category` (field 11, `string` → `TrackedString`)

These are acceptable because:
1. The system is pre-production (no deployed clients reading these wire formats).
2. All data will be backfilled with the new format.

If backward compatibility is ever needed, use `reserved` for old field numbers and assign new ones.

### Backfill

Existing records without provenance are treated as `source=UNSPECIFIED, version=""`. The `ShouldRefresh` function treats nil/empty provenance as "always refresh", so the backfill job will re-estimate all legacy records.

---

**Last Updated:** 2026-02-19
**Status:** Phases 1-6 COMPLETE
**Depends On:** [docs/ai/impact_improvements_plan.md](./impact_improvements_plan.md) (Phases 1-10 complete)
**Branch:** provenance
