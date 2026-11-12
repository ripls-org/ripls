# Issue #804: Data Consent modal throws an exception

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/804
**Filed:** 2026-03-07 | **Last activity:** 2026-03-07
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/client/modals.md

## Problem Statement

The `ObservabilityConsentDialog` throws a `RenderFlex overflowed by 7.4 pixels on the bottom` exception. The error log also shows concurrent location permission requests being fired, suggesting the modal is triggered during app startup where other initialization (location permissions) is also happening.

The root cause, per the maintainer comment, is that the modal uses the **old gradient header design** (`ModalBuilders.buildGradientHeader`) which adds a tall orange gradient banner. This header consumes vertical space that pushes the three toggle items and footer buttons beyond the 55% height allocation, causing the overflow.

## Context from Comments

- **@tescobar (maintainer):** The source issue is the modal using the old design with an orange header. The fix is to read `docs/client/modals.md` and use the more common headerless approach. The gradient header is a legacy pattern used by only a few remaining modals (request_help_modal, request_participant_modal, and this consent dialog).

## Potential Solutions

### Option A: Replace gradient header with ModalHeader + drag handle

- **Approach:** Remove `ModalBuilders.buildGradientHeader()` and replace with the standard `ModalHeader` widget (centered title text) plus a drag handle indicator at the top. This is the pattern used by `RequestManagementModal` and other modern modals.
- **Pros:** Follows the documented "more common approach" from modals.md. Removes the tall orange gradient that wastes vertical space. Consistent with other modals.
- **Cons:** Visual change for users who have seen the old consent dialog.
- **Effort:** Small.

### Option B: Increase modal height factor

- **Approach:** Change `heightFactor` from 0.55 to 0.70 or higher to give the gradient header enough room.
- **Pros:** Minimal code change, preserves existing design.
- **Cons:** Doesn't address the maintainer's directive to stop using the old design. Modal would be unnecessarily tall.
- **Effort:** Small.

### Option C: Remove header entirely, use inline title

- **Approach:** Remove the header completely and place a simple `Text` title inside the content area with a drag handle above it.
- **Pros:** Maximum vertical space for content. Simplest possible layout.
- **Cons:** Slightly less structured than using `ModalHeader`.
- **Effort:** Small.

## Recommendation

**Option A** is the clear choice. The maintainer explicitly directed using the common headerless approach documented in `modals.md`. The `ModalHeader` widget already exists and provides consistent styling. This also slightly reduces the consent dialog's dependency on the gradient header infrastructure, which is a legacy pattern.

## Open Questions

- **Should the `isInitialPrompt` header distinction be preserved?** Recommendation: Yes. The `ModalHeader` title can still switch between "Data Consent" (initial) and "Privacy & Data" (settings), just without the orange gradient background.
- **Should the `onSave` button in the header be preserved for the settings (non-initial) flow?** Recommendation: Yes, but move it to the footer alongside or replacing the existing button layout. The settings flow currently shows a "Save" button in the gradient header; with the headerless approach, put a simple save action at the bottom.
- **Should the location permission race condition also be addressed?** Recommendation: No, that's a separate issue. The location permission errors in the log are coincidental — they happen during the same startup sequence but are not caused by this modal. File a separate issue if needed.

## Implementation Plan

### Phase 1: Replace gradient header with ModalHeader

**Goal:** Fix the overflow exception by removing the tall gradient header.

**Tasks:**
- [ ] In `consent_dialog.dart`, remove the `_buildHeader` method and `ModalBuilders.buildGradientHeader` call
- [ ] Add a drag handle widget at the top of the `Column` (same pattern as `ItemMetricDetailModal`)
- [ ] Add `ModalHeader(title: ...)` below the drag handle, using the existing `isInitialPrompt` ternary for title text
- [ ] For the non-initial-prompt (settings) flow, move the "Save" action from the header to the footer area, or add a simple save button at the bottom
- [ ] Remove the `modal_builders.dart` import if no longer needed
- [ ] Fix `mounted` → `context.mounted` in `_saveSettings()` (lines 84, 88) per modals.md best practices: "Use context.mounted instead of mounted for BuildContext checks after async operations"
- [ ] Update existing tests in `consent_dialog_test.dart` to reflect the new structure (title still present, gradient header gone)
- [ ] Verify the modal renders without overflow at 55% height on small screens

**Validates:** Modal opens without RenderFlex overflow. Visual design matches common modal pattern.

## Related Issues

- None identified.
