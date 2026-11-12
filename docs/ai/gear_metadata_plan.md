# Gear Metadata UX Improvements

## Overview

This plan improves how gear metadata (value, weight, brand, model, material, category) is presented and edited during gear creation and on the gear detail page. Today, AI-inferred metadata fields are invisible during creation and only appear on the item metrics screen in a read-only "Product Details" card with a stub correction UI that doesn't persist. This plan makes metadata visible and editable from the start, modeled after how experience creation handles AI-inferred location and time.

### Goals

1. **Creation-time visibility**: Show a summary chip of AI-inferred metadata on the gear preview modal, tappable to open a metadata editing sheet.
2. **Post-creation visibility**: Show metadata summary chips on the gear detail page, tappable to open the same editing sheet.
3. **Provenance-correct editing**: Fields edited by the user get USER provenance; unedited fields keep AI provenance.
4. **Impact screen cleanup**: Remove "Product Details" and "Social Context" sections from the gear metrics screen, keeping it focused on impact.

### Model: Experience Creation Flow

The experience preview modal (`experience_preview_modal.dart`) shows AI-inferred time and location as tappable pill-style selectors below the editable title and description fields. Each pill uses the same visual pattern: translucent container with icon, text, and chevron. Tapping opens a dedicated modal for correction. We follow this exact pattern for gear metadata.

## Design

### Metadata Summary Chip

A single chip (matching the existing `_buildInfoChip` pattern from `content_view_builders.dart`) summarizes the most important metadata in a truncated horizontal layout:

```
[auto_awesome icon] ~$130 · 3.2 kg · Coleman Sundome ▸
```

- Icon: `Icons.auto_awesome` (AI sparkle) when all values are AI-inferred; `Icons.edit` when any value has been user-edited.
- Text: Comma-separated summary of value, weight, and brand+model. Truncated with ellipsis when space runs out.
- Chevron: Right chevron indicating tappability.
- Tap: Opens the gear metadata editing sheet.

The chip is used in two places:
1. **GearPreviewModal** (creation) — below description and above location selector.
2. **GearContentView** (detail page) — below description, alongside location and source URL chips.

### Metadata Editing Sheet

A bottom sheet modal for viewing and editing all metadata fields. Fields:

| Field | Input Type | Display |
|-------|-----------|---------|
| Brand | Text field | "Coleman" |
| Model | Text field | "Sundome" |
| Estimated Value | Text field (USD) | "$130.00" |
| Weight | Text field + unit toggle (g/kg) | "3.2 kg" |
| Category | Text field | "Camping & Outdoors" |
| Material | Dropdown (MaterialCategory enum) | "Nylon" |

Each field shows an AI sparkle indicator if the value is AI-inferred, or a user-edit indicator if user-modified. The sheet has a "Done" button that closes and updates the values.

### Provenance Tracking

**During creation (GearPreviewModal)**:
- `GenGearState` gains new fields for user-edited metadata overrides (parallel to `editedName`/`editedDescription`).
- When building the `GearMetadata` for `SaveGearRequest`, each field checks whether the user edited it. Unedited fields get wrapped as `TrackedString(value: aiValue)` (server applies LLM provenance via `generation_mode`). Edited fields are the same — the server's update path handles provenance assignment based on `generation_mode` for inserts.
- This means creation-time metadata edits "just work" because the server applies LLM provenance to all fields on insert, and subsequent SaveGear updates with changed values will get USER provenance.

**After creation (GearContentView)**:
- The metadata editing sheet sends a SaveGear update with the full `GearMetadata` object.
- The server's existing update path (`trackedStringChanged`, `trackedEstimateChanged`) correctly assigns USER provenance to changed fields and preserves existing provenance on unchanged fields.
- The existing `saveChanges` method in `GearViewModel` is extended to accept optional `GearMetadata`.

### Impact Screen Cleanup

Remove two sections from `item_metrics_screen.dart`:
- `_buildProductDetails()` and its call site (gear-only "PRODUCT DETAILS" card).
- `_buildSocialContext()` and its call site (all item types "SOCIAL CONTEXT" card).

The impact screen retains: hero stats, "What Sharing This Means" card, and loan history.

## Implementation Plan

### Phase 1: Metadata Editing Sheet Widget

Create the reusable bottom sheet widget that both creation and detail views will use.

- [x] **1a**: Create `app/lib/presentation/widgets/gear/gear_metadata_sheet.dart`
  - Bottom sheet with form fields for all 6 metadata fields
  - Accepts initial values (from `DetectedGearItem` or `GearMetadata`)
  - Returns edited `GearMetadata` on "Done"
  - Material category dropdown using `GearMetadataFormatter.formatMaterialCategory()`
  - Weight field with g/kg toggle
  - Value field with "$" prefix
  - AI sparkle / user-edit indicators per field
- [x] **1b**: Create `app/lib/presentation/widgets/gear/gear_metadata_summary.dart`
  - A builder function or widget that composes the metadata summary text
  - Formats: value → weight → brand+model, truncated to fit
  - Used by both the creation chip and detail page chip

### Phase 2: Gear Creation Integration

Wire the metadata chip and editing sheet into the gear creation flow.

- [x] **2a**: Add metadata override fields to `GenGearState` in `gen_gear_view_model.dart`
  - New fields: `editedValueUsd`, `editedWeightGrams`, `editedBrand`, `editedModel`, `editedCategory`, `editedMaterialCategory`
  - New method: `updateMetadata(GearMetadata metadata)` — stores user edits
  - Update `createAndShareGear()` to merge edited metadata overrides when building the `GearMetadata` for `SaveGearRequest`
- [x] **2b**: Add metadata chip to `gear_preview_modal.dart`
  - New `_buildMetadataChip()` method below description, above location selector
  - Uses the same pill/container pattern as `_buildLocationSelector()`
  - Shows summary text from `gear_metadata_summary.dart`
  - Tap opens `GearMetadataSheet` with initial values from `detectedGear`
  - On return, calls `notifier.updateMetadata(editedMetadata)`
- [x] **2c**: Handle manual creation mode
  - When `detectedGear` is null (manual create), the chip shows "Add product details" placeholder
  - Tapping opens the sheet with empty fields
  - All manually entered fields will get LLM provenance on insert if generation_mode is set, or no provenance if manual

### Phase 3: Gear Detail Page Integration

Add metadata visibility and editing to the gear detail page.

- [x] **3a**: Extend `GearViewModel` / `GearState` in `gear_view_model.dart`
  - `GearState` already has `gearDetails` (`GetGearResponse`) which includes `metadata` and `valueEstimate`
  - Add `saveMetadata(GearMetadata metadata)` method that calls `gearRepository.saveGear()` with the updated metadata
  - Invalidate and reload gear details after save
- [x] **3b**: Extend `GearRepository` to pass metadata on save
  - `saveGear()` already accepts `metadata` parameter — verify it's wired correctly
  - Confirm round-trip: save with metadata → reload → metadata reflects changes
- [x] **3c**: Add metadata chip to `gear_content_view.dart`
  - New `_buildMetadataLine()` method in the read-mode content column
  - Position: after description, before or alongside location chip
  - Uses `gear_metadata_summary.dart` to format text from `GetGearResponse.metadata` and `GetGearResponse.valueEstimate`
  - Only shown for gear owner (non-owners see read-only version without chevron)
  - Tap opens `GearMetadataSheet` with current metadata values
  - On return, calls `viewModel.saveMetadata(editedMetadata)`
- [x] **3d**: Show metadata chip in edit mode too
  - When gear is in edit mode, the metadata chip remains visible and tappable
  - Metadata edits are saved independently from title/description edits (separate SaveGear call)

### Phase 4: Impact Screen Cleanup

Remove product details and social context from the impact screen.

- [x] **4a**: Remove `_buildProductDetails()` from `item_metrics_screen.dart`
  - Remove the method and its call site (lines ~167-170)
  - Remove `ProductDetailsCard` import if no longer used elsewhere
- [x] **4b**: Remove `_buildSocialContext()` from `item_metrics_screen.dart`
  - Remove the method, `_navigateToSocialAttribute()`, and the call site (lines ~173-176)
  - Keep `LoanSocialAttributeDisplay` if still used by loan history
- [x] **4c**: Clean up unused code
  - Remove `product_details_card.dart` if no longer imported anywhere
  - Remove `metadataMetrics` from `GearMetricData` if no longer consumed
  - Remove `GearMetadataFormatter` if no longer used (may still be needed by metadata summary)
  - Clean up unused imports in `item_metrics_screen.dart`
- [x] **4d**: Verify impact screen layout
  - Confirm the remaining sections (hero, sharing impact, history) flow well
  - Test with gear that has and doesn't have impact estimates

### Phase 5: Polish and Testing

- [x] **5a**: Unit tests for metadata summary formatting (31 tests)
  - `app/test/presentation/widgets/gear/gear_metadata_summary_test.dart`
  - `fromMetadata()`: null/empty, value formatting (<$1k, >=$1k, zero), weight (g/kg, zero), brand+model combos, all-parts joined, whitespace trimming
  - `fromDetectedGear()`: null, empty, value/weight/brand+model, all-parts
  - `hasUserEdits()`: null, no provenance, LLM provenance, USER provenance on each field type (brand, model, category, materialCategory, weightGrams, valueEstimate)
- [x] **5b**: Unit tests for ViewModel metadata methods (10 tests)
  - `GearNotifier.saveMetadata()` in `gear_view_model_test.dart`: success+reload, value estimate merge, null guard, error+rethrow+isSaving reset, isSaving during save
  - `GenGearNotifier.updateMetadata()` in `gen_gear_view_model_test.dart`: stores state, overwrites previous, without value estimate, initially null, reset clears
- [ ] **5c**: Verify provenance round-trip (manual / integration)
  - Create gear with AI metadata → verify LLM provenance on server
  - Edit metadata during creation → verify USER provenance on edited fields
  - Edit metadata from detail page → verify USER provenance on changed fields, existing provenance preserved on unchanged
- [ ] **5d**: UI polish
  - Verify chip styling matches existing location/time chips
  - Test on small screens (truncation)
  - Handle loading states (metadata not yet available)
  - Handle error states (save failure)

## Key Files

### Modified
| File | Changes |
|------|---------|
| `app/lib/presentation/viewmodels/gen_gear_view_model.dart` | Metadata override fields, `updateMetadata()` |
| `app/lib/presentation/screens/gear/gear_preview_modal.dart` | Metadata chip, sheet integration |
| `app/lib/presentation/viewmodels/gear_view_model.dart` | `saveMetadata()` method |
| `app/lib/presentation/screens/gear/gear_content_view.dart` | Metadata chip on detail page |
| `app/lib/presentation/screens/item/item_metrics_screen.dart` | Remove product details + social context |
| `app/lib/presentation/widgets/content/content_view_builders.dart` | New `buildMetadataChip()` builder (optional) |

### New
| File | Purpose |
|------|---------|
| `app/lib/presentation/widgets/gear/gear_metadata_sheet.dart` | Bottom sheet for editing all metadata fields |
| `app/lib/presentation/widgets/gear/gear_metadata_summary.dart` | Summary text formatting for chips |

### Potentially Removed
| File | Condition |
|------|-----------|
| `app/lib/presentation/widgets/impact/product_details_card.dart` | If not used elsewhere after Phase 4 |

## Open Questions (Resolved)

1. **Should non-owners see metadata on the detail page?** — Yes, as a read-only chip (no chevron, no tap action). This gives borrowers/requesters useful context about the item.
2. **Should embodied carbon be editable?** — No. It's computed from material + weight, so it updates automatically when those change. It can be shown as a read-only row in the metadata sheet for transparency.
3. **Value estimate in metadata vs. separate field** — `GetGearResponse` has `value_estimate` at the top level (field 14) separate from `metadata` (field 15). The metadata sheet should read from both and the summary chip should include value. The server already handles `ValueEstimate` updates in the metadata path.

## Architecture Audit

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`, `docs/server/architecture.md`

### Compliant

1. **ViewModel → Repository → Service layering**: Both `GenGearNotifier` and `GearNotifier` access data exclusively through `gearRepositoryProvider`. No screens or widgets access services directly. ✅
2. **Screens only interact with ViewModels**: `gear_preview_modal.dart` uses `ref.read(genGearProvider.notifier)` for mutations. `gear_content_view.dart` uses `ref.read(gearProvider(...).notifier)`. No service providers accessed from screens. ✅
3. **Cache invalidation via repository**: `GearRepository.saveGear()` already calls `invalidate(id)`, `refreshUserGear()`, `invalidateSearches()`, and the daily invalidation callback. `GearNotifier.saveMetadata()` doesn't need manual invalidation — it's handled by the repository. ✅
4. **Immutable Freezed state**: `GenGearState` uses `@freezed` with `copyWith()`. New `editedMetadata` field follows the same pattern as `editedName`/`editedDescription`. ✅
5. **Widget async exception pattern**: `_showMetadataSheet()` in both `gear_content_view.dart` and `gear_preview_modal.dart` is a modal bottom sheet (documented exception for widget async). Both check `mounted` after the await. ✅
6. **SafeNotifierMixin**: `GearNotifier` uses `SafeNotifierMixin` and checks `ref.mounted` after async gaps in `saveMetadata()`. ✅
7. **File organization**: New widget files (`gear_metadata_sheet.dart`, `gear_metadata_summary.dart`) are under `presentation/widgets/gear/`, following the directory structure convention. ✅
8. **No service dependencies in ViewModels**: Neither ViewModel accesses services directly — all data access goes through repository providers. ✅
9. **Server-side provenance**: The plan correctly delegates provenance assignment to the server's existing `trackedStringChanged`/`trackedEstimateChanged` helpers, keeping the client thin. ✅

### Issues Found and Fixed

1. **Testing gap** (violation of `docs/client/testing.md`): Phases 1-4 shipped new ViewModel methods (`saveMetadata`, `updateMetadata`) and a utility class (`GearMetadataSummary`) without any unit tests. Per testing.md, ViewModels require 90%+ coverage and "no new functionality should be added without test coverage." **Fixed**: Added 41 unit tests across 3 test files (Phase 5a/5b).

### Notes

- **2a plan vs. implementation mismatch (minor)**: The plan specified per-field overrides (`editedValueUsd`, `editedWeightGrams`, etc.) but the implementation correctly simplified to a single `GearMetadataEditResult? editedMetadata` field — this is better because it avoids state explosion (architecture doc weakness #3). No fix needed; the plan text is historical.
- **`content_view_builders.dart` change**: The plan listed adding a `buildMetadataChip()` builder, but implementation instead exposed the existing `_buildInfoChipOpt` as a public `buildInfoChip()` method — this is the correct approach since it reuses the existing pattern rather than creating a parallel one.

---

**Last Updated:** 2026-03-03
**Implementation Status:** Phases 1-4 complete. Phase 5a/5b complete (41 tests). Phase 5c/5d not started.
**Audit Status:** Testing gap found and fixed. Architecture compliant.