# Issue #1957: Background image changes during create

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1957
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** user-feedback | **Assignees:** tescobar
**Context docs:** none (referenced internally: `docs/design/unified-create.md`)

## Problem Statement

In the unified-create flow, when a user starts creation by capturing a photo (in-app camera) or picking from the gallery, the photo is uploaded and shown as the hero background on the preview card. The preview card has a type selector that lets the user flip between Event / Gear / Request. Each flip re-issues the AI stream with `force_type`, and the new stream emits a fresh `UnifiedCreateEventMedia` event carrying server-chosen stock imagery for that type. The reducer applies that media event and **overwrites the user's original photo** with stock imagery.

The intended contract (already partially implemented) is: if the user supplied the media, the AI's media suggestions must not clobber it. The reducer at [unified_create_view_model.dart:360-362](app/lib/presentation/viewmodels/unified_create_view_model.dart#L360-L362) already guards with `state.userEditedFields.contains(UserEditedField.mediaIds)` — but the user-supplied media path never sets that flag, so the guard is dead code for camera/gallery starts.

## Context from Comments

No comments on the issue. The author (Thomas, the project owner) submitted via the in-app feedback channel on 2026-05-16 against app version 0.20.1 on iOS. The report is unambiguous and includes the desired contract: "If the user supplied a background image, only the user should ever replace it." No further clarification is needed — this is a straightforward bug report from a maintainer.

## Root Cause

In [`UnifiedCreateViewModel._uploadAndStartStream`](app/lib/presentation/viewmodels/unified_create_view_model.dart#L210-L246) the user's captured / picked media is set into state at [line 235](app/lib/presentation/viewmodels/unified_create_view_model.dart#L235):

```dart
state = state.copyWith(mediaId: mediaId, mediaIds: [mediaId]);
```

No `UserEditedField.mediaIds` is added. So when `flipType` cancels and re-dispatches the stream and a `UnifiedCreateEventMedia` arrives, the reducer guard at [line 361](app/lib/presentation/viewmodels/unified_create_view_model.dart#L361) is False and the user's photo is replaced.

The contrasting paths already do the right thing: [`_replaceFromPicker`](app/lib/presentation/viewmodels/unified_create_view_model.dart#L137-L152) (the "replace background" dialog) explicitly sets `UserEditedField.mediaIds` after a manual replace — so manually replacing the photo *is* respected on flipType. Only the initial camera/gallery entry is unprotected.

## Potential Solutions

### Option A: Mark `UserEditedField.mediaIds` on user-supplied entry (recommended)

- **Approach:** In `_uploadAndStartStream`, when assigning the uploaded media into state, also add `UserEditedField.mediaIds` to `userEditedFields`. One line change at [unified_create_view_model.dart:235](app/lib/presentation/viewmodels/unified_create_view_model.dart#L235):
  ```dart
  state = state.copyWith(
    mediaId: mediaId,
    mediaIds: [mediaId],
    userEditedFields: {...state.userEditedFields, UserEditedField.mediaIds},
  );
  ```
  Reuses the exact same protection mechanism that `_replaceFromPicker` already relies on.
- **Pros:** Smallest possible change. Symmetric with the already-correct `_replaceFromPicker` path. No new concepts in the state shape. Text-input and URL-input flows (which have no user-supplied media) are unaffected — they never call `_uploadAndStartStream`.
- **Cons:** Couples "this field was edited" with "this field was user-provided at start." Reading the enum name `UserEditedField`, both feel like "user touched this field, don't clobber it," so the semantic overlap is acceptable.
- **Effort:** Small (one production line + 1-2 test cases).

### Option B: New `UserProvidedField` concept distinct from `UserEditedField`

- **Approach:** Add a parallel `userProvidedFields` set and check both in the reducer guards.
- **Pros:** Cleaner semantic separation if we later need to distinguish "supplied at start" from "edited mid-flow" (e.g., for analytics).
- **Cons:** New state surface, every guard needs to check both sets, every test needs to consider the new dimension. No current consumer needs the distinction. Pure over-engineering for this bug.
- **Effort:** Medium.

### Option C: Drop the reducer guard and rework the server stream to never emit media when input is image mode

- **Approach:** Push the rule server-side: if the inbound `StreamGenUnifiedCreate` request includes a user-uploaded `media_id`, skip the stock-imagery fan-out and don't emit `UnifiedCreateEventMedia` at all.
- **Pros:** Saves a round-trip of unused stock-image generation; no client-side guard needed.
- **Cons:** Cross-cutting change (server + proto + client + tests). Doesn't fix the bug for non-image input modes that later get a photo via "replace background" — those still need the client-side guard. The client guard already exists and works; this proposal duplicates the contract on both sides.
- **Effort:** Large.

## Recommendation

**Option A.** This is a one-line bug fix that uses a protection mechanism already wired through the entire reducer. Options B and C are speculative over-builds. The fix should land alongside a regression test that asserts: after `captureAndStartFromCamera` (and `pickAndStartFromGallery`), a subsequent `UnifiedCreateEventMedia` event from the stream does **not** replace `state.mediaIds`.

## Open Questions

- **Should `mediaId` (singular, the user-uploaded id) also be preserved across flipType, not just `mediaIds`?** — Recommendation: yes. `mediaId` is set in `_uploadAndStartStream` and is the canonical record of "the asset the user gave us at start." Nothing in `flipType` or the reducer touches it today, so no change is needed beyond confirming it's still correct on inspection. Add a one-line assertion in the regression test.

- **Should the "replace background" affordance be visually distinguished after the user has supplied a photo (e.g., "Your photo" badge)?** — Recommendation: no, out of scope for this bug. File a follow-up issue if UX wants it; the current bug is purely behavioral. The user explicitly said "only the user should ever replace it," not "show that the user supplied it."

- **Does the legacy gear flow (`GenGearViewModel` / `gen_gear_streaming_actions.dart`) have the same bug?** — Recommendation: investigate as a follow-up. The unified-create flow is the one the user is reporting against (it is the modal with type tabs). The legacy single-type `GearPreviewModal` does not have type-flipping tabs, so the same code path doesn't exist there — but the exploration agent flagged `gen_gear_streaming_actions.dart` lines 264-278 unconditionally overwriting `selectedMediaIds` on stream completion. If that flow is still reachable in 0.20.1, it deserves its own ticket. Not part of this fix.

## Implementation Plan

### Phase 1: Fix and regression test
**Goal:** User-supplied photos survive type flips on the preview card.
**Tasks:**
- [ ] Update [`_uploadAndStartStream`](app/lib/presentation/viewmodels/unified_create_view_model.dart#L210-L246) to add `UserEditedField.mediaIds` to `userEditedFields` when assigning the uploaded media into state.
- [ ] Add a regression test in [unified_create_view_model_test.dart](app/test/presentation/viewmodels/unified_create_view_model_test.dart) following the file's established fixtures (`_FakeMediaRepository` for media uploads, `_ScriptedActions` for the stream, `_drain()` to flush microtasks, `ProviderContainer` + `addTearDown(container.dispose)`). The scenario:
  1. Stub `_FakeMediaRepository.stubReturns('user-media-123')`.
  2. Use a first `_ScriptedActions` that emits a `Type` + `Final` for GEAR — no media event.
  3. Call `vm.captureAndStartFromCamera('/tmp/x.jpg')`; drain; assert `state.mediaIds == ['user-media-123']` and `state.userEditedFields` contains `UserEditedField.mediaIds`.
  4. `vm.attachActions(secondScript)` where `secondScript` emits `Type`(EVENT), `Media(['stock-1'])`, `Final`(EVENT) — matching the re-attach pattern at [unified_create_view_model_test.dart:196](app/test/presentation/viewmodels/unified_create_view_model_test.dart#L196).
  5. Call `vm.flipType(EVENT)`; drain; assert `state.mediaIds == ['user-media-123']` (not `['stock-1']`).
  6. Repeat the assertion path for `pickAndStartFromGallery` via a second test — the gallery picker is impossible to fake without overriding `MediaPickerHelper` static methods, so this case can be covered by directly exercising `_uploadAndStartStream`'s effect through the existing `captureAndStartFromCamera` path (both call the same private helper); document this in the test comment rather than duplicating coverage.
- [ ] Run `npm run test:app` (which invokes `flutter test --reporter expanded` per CLAUDE.md) and confirm the existing `captureAndStartFromCamera uploads and transitions to preview` test still passes — its assertions on `mediaId`/`mediaIds` are unaffected by the `userEditedFields` change.
**Validates:** The contract from the issue — "if the user supplied a background image, only the user should ever replace it" — holds for the in-app camera and gallery entry points, end to end through one type flip.

### Phase 2: Verify on device (manual)
**Goal:** Confirm the reported scenario is fixed on iOS 0.20.x.
**Tasks:**
- [ ] Build and run on iOS simulator. Capture a photo to start unified-create. Flip type tabs. Confirm photo persists.
- [ ] Repeat with gallery pick.
- [ ] Sanity-check that flipping type still updates title, description, type-specific fields per the existing flipType contract.
- [ ] Sanity-check that the manual "replace background" affordance still works after flipType (i.e., the guard correctly allows *user* replacement, only blocks *stream* replacement).
**Validates:** No UI regression from the state-shape change.

## Related Issues

- None directly linked. See Open Questions §3 for a potential follow-up on the legacy `GenGearViewModel` path. The design doc `docs/design/unified-create.md` (§ Decisions on flipType and userEditedFields) is the relevant reference for the protection contract.

## Audit Notes (`/ripls-audit`)

Audited against `docs/client/architecture.md` and `docs/client/testing.md`. Other reference docs (`caching.md`, `i18n.md`, `accessibility.md`, server docs) were inspected and are not load-bearing for this change — no cache invalidation, no user-visible strings (the `mediaIds` field is purely state-shape), no a11y surface change, no server change.

**Compliance check:**
- **MVVM boundaries** (architecture.md §1): ✅ Fix lives entirely in the ViewModel layer; no Repository or Service changes; no Widget reaches into state directly. The `mediaRepositoryProvider` is read via `ref.read`, matching the established pattern.
- **Immutable state via copyWith** (architecture.md §2): ✅ The fix uses a single additional `copyWith` field (`userEditedFields: {...state.userEditedFields, UserEditedField.mediaIds}`) — identical pattern to the existing `_replaceFromPicker` call site at [unified_create_view_model.dart:144-147](app/lib/presentation/viewmodels/unified_create_view_model.dart#L144-L147).
- **No manual caches / no controllers** (architecture.md §4-5): ✅ No new caching, no controller class.
- **No over-engineering** (CLAUDE.md "Don't add features beyond what the task requires"): ✅ Option B (parallel `userProvidedFields` set) explicitly rejected. Option C (server-side change) explicitly rejected. The recommended fix is one line.
- **ViewModel-first testing** (testing.md §1): ✅ Regression test runs against the `UnifiedCreateViewModel` via `ProviderContainer`, not against a widget tree.
- **Test fixtures** (testing.md §Mock Generation): ⚠️ Departure noted — `unified_create_view_model_test.dart` uses hand-rolled fakes (`_FakeMediaRepository extends Fake`, `_ScriptedActions`) instead of Mockito-generated mocks. This is deliberate per the file's existing comment (line 14-16) and the new test must follow the file's local convention, not the global `testing.md` Mockito recommendation. Phase 1 task list reflects this.
- **Container disposal** (testing.md §Pitfall 1): ✅ Plan calls out `addTearDown(container.dispose)`.
- **Async safety / `ref.mounted`** (architecture.md §Async Safety): ✅ The fix does not introduce new awaits; the existing `if (!ref.mounted) return;` guard at [line 234](app/lib/presentation/viewmodels/unified_create_view_model.dart#L234) already covers the post-upload state assignment.
- **Test runner flag** (CLAUDE.md): ✅ Plan specifies `npm run test:app` which wraps `flutter test --reporter expanded`.

**No changes required to the recommendation or implementation plan beyond the Phase 1 test-detail sharpening already applied.**
