# Issue #1776: Gear details missing on details tab

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1776
**Filed:** 2026-05-07 | **Last activity:** 2026-05-07
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

On the gear screen Details tab, owner-side metadata fields render with empty
placeholder text ("Brand name", "$0", "0 g") even though the AI-detected
metadata for brand, value, and weight is present on the gear. In addition,
the Model field is **never** displayed in the Details tab — neither for
owners nor for non-owners — even though `GearMetadata.model` is a populated
proto field that other surfaces (e.g. `GearMetadataSummary.fromMetadata`,
`GearMetadataFormatter.formatMetadata`) already render.

The user filed this as "make and model" appearing empty in the Details
tab, with a screenshot from Android v0.19.2 showing the issue.

## Context from Comments

The issue has only one comment: a `@claude` mention from the maintainer
(`maintainer`) requesting automated triage. There are no clarifying
comments or workarounds. The reporter is the project lead, so the
screenshot is taken on a piece of gear they own — which matches the
"owner viewing their own gear" code path.

## Root Cause

Two distinct bugs combine in `app/lib/presentation/screens/gear/widgets/gear_details_pane.dart`:

### Bug 1 — uninitialized edit buffers leak into the read-only owner view

In `gear_content_view.dart` the parent state declares:

```dart
String _editingBrand = '';
String _editingValueUsd = '';
String _editingWeightGrams = '';
String _editingWebsite = '';
bool _hasInitializedMetadata = false;
```

These buffers are seeded from metadata only inside `_buildDetailsPane` and
only when the user is in edit mode:

```dart
if (state.isEditing && !_hasInitializedMetadata) {
  _hasInitializedMetadata = true;
  _editingBrand = metadata?.brand.value ?? '';
  // ...
}
```

But `gear_details_pane.dart` calls the editable rendering branch for **all
owners**, regardless of `isEditing`:

```dart
if (state.isOwner)
  _buildDetailRowsEdit(context, state, metadata)
else
  _buildDetailRows(state, gear, metadata, valueEstimate),
```

`_buildDetailRowsEdit` then passes the (still-empty) `editingBrand` etc.
into `_buildEditableRow`, which builds a `TextFormField` with
`initialValue: value`. The owner therefore sees empty text fields with
hint placeholders ("Brand name", "$0", "0 g") rather than the actual
metadata. Material is the lone exception because it is rendered as a
read-only row that reads `metadata.materialCategory` directly.

### Bug 2 — Model is not rendered anywhere on the Details tab

`_buildDetailRows` (non-owner read-only) renders Brand, Value, Material,
Weight. `_buildDetailRowsEdit` (owner editable) renders Brand, Value,
Material (read-only), Weight. Neither path renders `metadata.model`,
even though `GearMetadata.model` is a `TrackedString` populated by the
AI metadata pipeline and rendered by `GearMetadataSummary.fromMetadata`
as part of the brand-and-model summary chip elsewhere in the app.

## Potential Solutions

### Option A — Show editable form only when `state.isEditing`; add Model row to both views

- **Approach:**
  1. In `gear_details_pane.dart`, change the branch to
     `state.isOwner && state.isEditing` for the editable rows, falling
     back to `_buildDetailRows` otherwise.
  2. Add a `Model` row to `_buildDetailRows` between Brand and Value, and
     an editable `_editingModel` buffer + `Model` row in
     `_buildDetailRowsEdit` between Brand and Value.
  3. Wire `_editingModel` into `gear_content_view.dart` alongside the
     existing brand/value/weight buffers and into the metadata save path
     in `_scheduleMetadataSave` / `saveChanges`.
- **Pros:**
  - Matches the user's mental model: not editing = see your data; tap
    Edit = see editable fields. Mirrors the way `_buildInfoCardEdit`
    already gates on `state.isEditing`.
  - Naturally hides the empty-placeholder bug — if you're not editing,
    you never see a `TextFormField`.
  - Adds Model coverage for both owners and non-owners in one pass.
- **Cons:**
  - Owners lose inline tap-to-edit for individual fields; they have to
    enter edit mode first. This is consistent with how the rest of the
    Details pane works (`_buildInfoCardEdit` is already gated this way),
    but it does change owner UX subtly.
  - Need a small migration of `_editingModel` through the metadata save
    debouncer.
- **Effort:** Small.

### Option B — Always seed edit buffers when metadata is available; keep editable rows for all owners; add Model row

- **Approach:**
  1. Move the `_editingBrand = metadata?.brand.value ?? ''` seeding out
     of the `state.isEditing` guard so the buffers track metadata for
     any owner viewing the screen, not only in edit mode.
  2. Keep the existing "owners always see editable rows" behavior.
  3. Add a Model row + `_editingModel` buffer to the editable view, and
     a Model row to the non-owner read-only view.
- **Pros:**
  - Preserves the current always-editable owner UX.
  - One-line change to fix the empty placeholders.
- **Cons:**
  - The "always editable" behavior is itself questionable UX — owners
    see a constantly-active text field even when they're just viewing.
    Doesn't match the explicit `toggleEditMode` action the menu exposes.
  - `TextFormField.initialValue` is only consulted on field
    construction, so subsequent metadata updates would still not flow
    into the displayed value without a key-based rebuild — the seeding
    path needs careful handling to avoid clobbering in-progress edits.
  - Doesn't address the inconsistency that `_buildInfoCardEdit` is
    already correctly gated by `state.isEditing` while the rows are not.
- **Effort:** Small, but more state-management complexity.

### Option C — Render Model in non-owner view only; leave owner path alone

- **Approach:** Just add a Model row to `_buildDetailRows`. Don't touch
  the owner editable path or the empty-buffer bug.
- **Pros:** Minimal blast radius; addresses the literal "Model is
  missing" half of the bug.
- **Cons:** Doesn't fix the empty-make-and-value-and-weight problem the
  user actually reported. Bryan is the project lead and is almost
  certainly viewing his own gear, hitting the owner code path.
- **Effort:** Trivial.

## Recommendation

**Option A.** It fixes both bugs with the smallest surface change, brings
the Details tab into line with the gating pattern already established by
`_buildInfoCardEdit`, and matches the explicit edit-mode toggle that the
action menu exposes via `toggleEditMode`. The owner UX change (no
inline tap-to-edit) is a feature, not a regression — the rest of the
pane already works that way.

## Open Questions

- **Should Model also appear in `_buildInfoCardEdit` (the location/website
  card) or only in the rows card?** — Recommendation: only the rows
  card. Model is a product attribute alongside Brand/Value/Weight, not a
  free-form field like website. Keep `_buildInfoCardEdit` focused on
  location + sourceUrl as it is today.
- **Display order: Brand-Model-Value-Material-Weight, or
  Brand-Value-Material-Weight-Model?** — Recommendation: Brand-Model
  side-by-side at the top. Brand and Model are conceptually paired
  (e.g. "DeWalt DCD771C2") and `GearMetadataSummary._formatBrandModel`
  already treats them as a pair.
- **Should non-owners with an empty Brand/Model still see the row, or
  should we keep the existing `if (brand.isNotEmpty)` filter?** —
  Recommendation: keep the filter — empty rows add visual noise. Apply
  the same `if (model.isNotEmpty)` filter for the Model row.
- **Save path: should `_editingModel` participate in the
  debounced metadata save in `_scheduleMetadataSave`?** —
  Recommendation: yes, mirror `_editingBrand`. The metadata save build
  in `gen_gear_view_model.dart` already constructs `metadata.model`
  when calling save, so the existing save plumbing supports it; we just
  need to thread the new buffer through.
- **Should we opportunistically migrate the existing hardcoded labels
  ('Brand', 'Value', 'Material', 'Weight', 'Brand name', '$0', '0 g',
  'Add location', 'https://') to ARB keys while we're touching the
  file?** — Recommendation: **yes**. `gearDetailMaterial`,
  `gearDetailWeight`, `gearDetailEstimatedValue` already exist in
  `app/lib/l10n/app_en.arb` and `app_es.arb`, so reusing them is free.
  We need to add new ARB keys for `gearDetailBrand`, `gearDetailModel`,
  and their hint-text counterparts anyway — migrating the rest in the
  same PR keeps the file consistent and avoids leaving the new Model
  row as the lone non-localized label in a file otherwise full of
  them. CI runs `npm run lint:dart:i18n` and the `require_l10n_for_*`
  rules; touching the file is a natural moment to fix the violations
  rather than compound them. (Per `docs/client/i18n.md`: "Strings
  belong in ARB files, not Dart code.")

## Implementation Plan

### Phase 1: Gate editable rows on edit mode and seed buffers correctly

**Goal:** Owners not in edit mode see their actual brand/value/weight
values instead of empty hints.
**Tasks:**
- [ ] In `app/lib/presentation/screens/gear/widgets/gear_details_pane.dart`,
      change `if (state.isOwner)` to `if (state.isOwner && state.isEditing)`
      so the editable rows only render in edit mode.
- [ ] Verify `_buildDetailRows` renders correctly for owners not in
      edit mode (it already reads metadata directly, so no other
      changes are needed for the read-only path).
- [ ] Add a widget test under
      `app/test/presentation/screens/gear/` that exercises the
      Details tab as an owner with metadata set and `isEditing: false`,
      asserting that the brand value is visible (not the "Brand name"
      hint). The test must wrap `MaterialApp` with
      `AppLocalizations.localizationsDelegates` and
      `AppLocalizations.supportedLocales` per
      `app/test/helpers/l10n_helpers.dart`. Use a real
      `ProviderContainer` with a fake/overridden gear repository so the
      ViewModel exposes a populated `gearDetails` —
      `docs/client/testing.md` recommends ViewModel-first testing, but
      this is a UI-rendering bug, so a widget test is the right tool.
**Validates:** The reported empty-fields bug is gone for owners.

### Phase 2: Add Model field and migrate labels to ARB

**Goal:** Model is rendered in the Details tab whenever it's set,
matching the brand/value/weight treatment, and all labels in the file
read from `context.l10n.*` instead of being hardcoded.
**Tasks:**
- [ ] Add new keys to both `app/lib/l10n/app_en.arb` and
      `app/lib/l10n/app_es.arb` (every new string needs both locales —
      see `docs/client/i18n.md`):
  - `gearDetailBrand` ("Brand" / "Marca")
  - `gearDetailModel` ("Model" / "Modelo")
  - `gearDetailBrandHint` ("Brand name" / "Nombre de marca")
  - `gearDetailModelHint` ("Model name" / "Nombre de modelo")
  - `gearDetailValueHint` ("$0" — same in es; bare currency)
  - `gearDetailWeightHint` ("0 g" — same in es; SI unit)
  - `gearDetailAddLocation` ("Add location" / "Agregar ubicación")
  - Each new key needs a `@key` metadata block with a `description`.
- [ ] Run `npm run generate` to produce the updated
      `AppLocalizations`. Verify the new getters appear in
      `.dart_tool/flutter_gen/`.
- [ ] In `_buildDetailRows`, insert a
      `if (model.isNotEmpty) (context.l10n.gearDetailModel, model)`
      row immediately after Brand. Pull `model` from
      `metadata?.model.value ?? ''`. Replace the other hardcoded
      labels in this method with `context.l10n.gearDetailBrand`,
      `context.l10n.gearDetailEstimatedValue` (already exists),
      `context.l10n.gearDetailMaterial` (already exists),
      `context.l10n.gearDetailWeight` (already exists). Note that
      `_buildDetailRows` does not currently take a `BuildContext`;
      thread one in.
- [ ] In `_buildDetailRowsEdit`, insert a `_buildEditableRow` for Model
      between the Brand and Value rows, with an `editingModel`
      parameter, an `onModelChanged` callback, and the new
      `gearDetailModelHint` hint string. Migrate the surrounding
      labels and hints to ARB keys at the same time
      (`gearDetailBrand`, `gearDetailBrandHint`,
      `gearDetailEstimatedValue`, `gearDetailValueHint`,
      `gearDetailMaterial`, `gearDetailWeight`,
      `gearDetailWeightHint`).
- [ ] In `_buildInfoCardEdit`, replace the hardcoded `'Add location'`
      with `context.l10n.gearDetailAddLocation`. Leave the
      `'https://'` URL hint as-is — it's a URL scheme literal, not
      a translatable string.
- [ ] In `app/lib/presentation/screens/gear/gear_content_view.dart`:
  - Add `String _editingModel = '';` alongside the existing buffers.
  - Seed `_editingModel = metadata?.model.value ?? '';` inside the
    existing `state.isEditing && !_hasInitializedMetadata` block.
  - Pass `editingModel: _editingModel` and
    `onModelChanged: (v) { setState(() => _editingModel = v); _scheduleMetadataSave(); }`
    to `GearDetailsPane`.
  - Find the metadata-save path that currently reads `_editingBrand`
    and write `_editingModel` into the saved `GearMetadata.model`
    `TrackedString` the same way (set value, mark provenance as
    `PROVENANCE_SOURCE_USER` if `_editingModel != metadata?.model.value`).
    Verify by inspecting `gen_gear_view_model.dart:435`
    where `metadata.model = TrackedString(...)` is already wired.
- [ ] Update or add tests in `app/test/presentation/screens/gear/`
      to cover:
  - Read-only Details renders Model when `metadata.model.value` is set.
  - Editable Details exposes a Model row that is editable.
  - Saving a changed Model value calls `saveGear` with the updated
    `metadata.model.value` and provenance set to USER.
  - Widget tests must wrap their `MaterialApp` with `localizationsDelegates`
    and `supportedLocales` per `app/test/helpers/l10n_helpers.dart`.
**Validates:** Model is visible everywhere brand-value-weight already
appears, all labels are localized, and editing Model persists.

### Phase 3: Manual smoke test on Android

**Goal:** Confirm the user-reported screenshot scenario is fixed end to end.
**Tasks:**
- [ ] Run `flutter test --reporter expanded` in `app/` and confirm the new
      tests pass alongside the existing suite.
- [ ] Launch the app against an emulator using the
      `ripls-app-logs` skill, open a piece of owned gear that has AI-
      detected brand and model, switch to the Details tab without
      entering edit mode, and verify:
  - Brand row shows the real value (not "Brand name").
  - Model row shows the real value (new row).
  - Value row shows the real value (not "$0").
  - Weight row shows the real value (not "0 g").
- [ ] Toggle edit mode via the action menu and confirm all four rows
      are editable, including Model, with their current values
      pre-populated.
**Validates:** The fix matches the user-reported scenario from the
screenshot.

## Related Issues

- None directly. The closest neighbors are the Phase 4 RPC error work and
  the gear creation flow (`gear_preview_modal.dart`), which already calls
  `GearMetadataSummary.fromDetectedGear` and renders both brand and model
  — that surface is unaffected by this bug because it builds its own
  summary string and doesn't go through `_buildDetailRows`.

## Audit Notes

`ripls-audit` was run against this plan. Findings folded back into the plan:

- **i18n:** `docs/client/i18n.md` requires that "Strings belong in ARB
  files, not Dart code." The current `gear_details_pane.dart` violates
  this with hardcoded labels ('Brand', 'Value', 'Material', 'Weight',
  'Brand name', '$0', '0 g', 'Add location'). Phase 2 was expanded to
  add new ARB keys (`gearDetailBrand`, `gearDetailModel`,
  `gearDetailBrandHint`, `gearDetailModelHint`, `gearDetailValueHint`,
  `gearDetailWeightHint`, `gearDetailAddLocation`) in both `app_en.arb`
  and `app_es.arb`, reuse the existing
  `gearDetailMaterial`/`gearDetailWeight`/`gearDetailEstimatedValue`
  keys, and migrate the rest of the hardcoded strings in the file in
  the same PR. The new test must wrap `MaterialApp` with
  `AppLocalizations.localizationsDelegates` per
  `app/test/helpers/l10n_helpers.dart`.
- **Architecture (MVVM):** No new repositories or services are needed —
  the fix lives entirely in the presentation layer. The existing
  metadata save path (`gen_gear_view_model.dart:435`) already supports
  the model field; we just thread `_editingModel` through to it. No
  caching changes; the `GearRepository` already returns the populated
  `metadata.model` since the proto is fully wired.
- **Testing:** Widget testing is unusual in this codebase (the testing
  doc emphasizes ViewModel-first), but the bug is specifically about UI
  rendering, so a widget test is the appropriate tool. ViewModel-side
  coverage for the model save path goes through
  `gear_view_model_test.dart` patterns.
- **Accessibility:** No new a11y primitives are needed. The existing
  `_buildEditableRow` uses raw `TextFormField`, matching the surrounding
  pattern; introducing `Tappable`/`IconAction` here is out of scope.
- **Theming:** The existing file uses a mix of raw
  `Colors.black.withValues(...)` and `AppColors.overlayFieldText()`.
  Per `docs/client/architecture.md` the colors should all be theme-aware,
  but reworking the theme of this file is out of scope; the new Model
  row simply matches the surrounding pattern.
