# Issue #1083: [Feature Request] Upload multiple images

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1083
**Filed:** 2026-04-09 | **Last activity:** 2026-04-09
**Labels:** enhancement, user-feedback | **Assignees:** unassigned
**Context docs:** docs/client/media.md

## Problem Statement

Users can only pick and upload one image at a time through the media picker
bottom sheet. When adding photos to gear, experiences, or other content types,
each image requires a separate tap-through of the picker dialog (open sheet,
choose gallery, pick one image, wait for upload, repeat). This is tedious for
users who want to add several photos of the same item.

The request comes from a user on Android 16 (Pixel 9 Pro) running app v0.11.0.
The underlying `image_picker` Flutter package already supports multi-image
selection via `pickMultiImage()`, but the app only uses `pickImage()`.

## Context from Comments

No comments on this issue. The feature request is clear and self-contained —
the user wants multi-select in the gallery picker. No additional clarification
needed before planning.

## Current Architecture

The upload flow is:

1. **Picker UI:** `MediaPickerDialog` bottom sheet offers three options: Videos,
   Photos, Camera. Each triggers a single-file pick.
2. **Helper:** `MediaPickerHelper` wraps `ImagePicker` — methods
   `pickImageFromGallery()`, `pickVideoFromGallery()`, `pickImageFromCamera()`
   all return a single file path.
3. **Upload mixin:** `MediaUploadMixin` provides `pickAndUploadImageFromGallery()`
   etc., which pick one file then call `_setMediaFile()`.
4. **Notifiers:** `GearNotifier._setMediaFile()` and
   `ExperienceNotifier._setMediaFile()` create an optimistic placeholder, upload
   via `mediaRepository.addMedia()`, then save the updated media ID list to
   the server.
5. **Server API:** `AddMedia` RPC accepts a single file (`bytes + content_type`).
   No batch upload RPC exists.
6. **Association:** `SaveGearRequest` and `SaveExperienceRequest` both accept
   `repeated string media_ids` — the full list is replaced, not appended.

Key files:
- `app/lib/core/utils/media_picker_helper.dart` — ImagePicker wrapper
- `app/lib/core/utils/media_upload_mixin.dart` — reusable upload logic
- `app/lib/presentation/widgets/media/media_picker_dialog.dart` — bottom sheet UI
- `app/lib/presentation/viewmodels/gear_view_model.dart:562-704` — gear upload flow
- `app/lib/presentation/viewmodels/experience_view_model.dart:726-865` — experience upload flow
- `app/lib/presentation/screens/gear/gear_content_view.dart:1156-1172` — carousel add handler
- `app/lib/presentation/screens/experience/experience_content_view.dart:994-1028` — carousel add handler

## Potential Solutions

### Option A: Multi-select gallery with sequential uploads

- **Approach:** Add `pickMultipleImagesFromGallery()` to `MediaPickerHelper`
  using `ImagePicker.pickMultiImage()`. In the upload mixin and notifiers,
  loop through the returned file paths and upload each one sequentially via
  the existing `addMedia` RPC. Show a single progress indicator
  ("Uploading 3 of 5...") in the UI. The "Photos" option in the picker dialog
  opens the multi-select gallery; Camera and Video remain single-pick.
- **Pros:** Minimal server changes (none). Uses existing `addMedia` RPC.
  Reuses existing optimistic placeholder + state update patterns. The
  `image_picker` package already supports `pickMultiImage()`.
- **Cons:** Sequential uploads are slower than parallel. Each upload is a
  separate RPC call. If one upload fails mid-batch, partial state must be
  handled.
- **Effort:** Medium.

### Option B: Multi-select gallery with parallel uploads

- **Approach:** Same as Option A for picking, but upload all files concurrently
  using `Future.wait()` or a bounded concurrency pool. Create all optimistic
  placeholders at once, then resolve them as uploads complete.
- **Pros:** Faster total upload time for large batches. Better UX for users
  adding many photos.
- **Cons:** More complex error handling (partial failures). Higher memory usage
  (multiple file buffers in flight). Risk of server rate-limiting or timeout
  under load. More complex state management with multiple concurrent
  placeholder-to-real-item transitions.
- **Effort:** Medium-large.

### Option C: Server-side batch upload RPC

- **Approach:** Add a new `AddMediaBatch` RPC that accepts multiple files in
  one request. Client sends all files in a single call, server processes and
  returns all media IDs.
- **Pros:** Single round-trip. Atomic (all succeed or all fail). Simplifies
  client error handling.
- **Cons:** Large request payloads (multiple images in one protobuf message).
  Requires new server RPC, proto changes, and generated code updates. Timeout
  risk for large batches. More complex server-side processing. Overkill for
  the typical 2-5 image batch.
- **Effort:** Large.

## Recommendation

**Option A: Multi-select gallery with sequential uploads.** This is the
simplest approach that delivers the core user value — selecting multiple photos
at once without re-entering the picker each time. Sequential uploads are
acceptable because:

1. The bottleneck users feel is the *picking*, not the uploading. Even with
   sequential uploads, the user picks once and can do something else while
   uploads process.
2. Typical batch sizes are small (2-5 images for a gear item), so the
   sequential time penalty is minimal.
3. The existing `_setMediaFile()` pattern in notifiers already handles
   optimistic placeholders and error recovery for single uploads. Extending
   it to a loop is straightforward with well-defined failure semantics
   (stop on first error, keep already-uploaded items).
4. No server changes required.

If sequential speed becomes a user complaint later, upgrading to bounded
concurrency (Option B) is incremental — the multi-pick UI stays the same.

## Open Questions (Resolved)

- **Should multi-select apply only to gallery, or also offer a "take multiple
  photos" camera flow?** — **Decision: Gallery only.** The `image_picker`
  package doesn't support multi-capture from camera natively. Camera remains
  single-shot.

- **Should there be a maximum number of images per batch?** — **Decision:
  Yes, cap at 20 per batch pick.** This prevents accidental selection of
  hundreds of photos and keeps upload time reasonable. The cap is on the
  picker, not on total media per item (which is already governed by the server).

- **How should partial upload failures be handled?** — **Decision: Keep
  already-uploaded images, remove failed placeholders, and show a toast with
  the count of failures.** Don't retry automatically — let the user add the
  remaining images manually.

- **Should the "Photos" picker option always multi-select, or should we offer
  both single and multi?** — **Decision: Always use multi-select for the
  gallery option.** Single-select is a subset of multi-select (user just picks
  one). No need for two gallery options.

## Implementation Plan

### Phase 1: Multi-select picker infrastructure

**Goal:** Enable picking multiple images from gallery and uploading them
sequentially with progress feedback.

**Tasks:**
- [ ] Add `pickMultipleImagesFromGallery()` to `MediaPickerHelper` using
      `ImagePicker().pickMultiImage()` with same quality settings (1920x1080,
      85% quality). Return `List<String>` of file paths. Cap at 20 images.
- [ ] Add `pickAndUploadMultipleImagesFromGallery()` to `MediaUploadMixin`
      that calls the new picker method, then loops through each file path
      calling the existing upload flow. Add callbacks for batch progress
      (`onBatchUploadProgress(int completed, int total)`).
- [ ] Update `MediaPickerDialog` to call the multi-select picker for the
      "Photos" option instead of single-select. Camera and Video remain
      unchanged.
- [ ] Add upload progress state to relevant state classes (e.g.,
      `uploadProgress: String?`) and display it in the `MediaPickerButton`
      or an overlay.
- [ ] Add all new user-facing strings to both ARB files (`app_en.arb` and
      `app_es.arb`) — e.g., upload progress text, partial failure toast,
      batch selection label. Use ICU message format for the count placeholder.
      ViewModels must expose typed state (completed count, total count), not
      resolved strings — widgets resolve to localized text in `build()` via
      `context.l10n`.
- [ ] Handle partial failures: keep successful uploads, remove failed
      placeholders, show error toast with failure count.
- [ ] Add unit tests for `MediaPickerHelper.pickMultipleImagesFromGallery()`
      and `MediaUploadMixin` batch upload logic.

**Validates:** Users can select multiple photos in one picker session and
see them upload sequentially with progress feedback.

### Phase 2: Integrate with content notifiers

**Goal:** Wire multi-select upload into all content types that support
media addition.

**Tasks:**
- [ ] Update `GearNotifier` to use multi-image upload in its pick methods.
      Each uploaded image gets an optimistic placeholder, and `saveChanges()`
      is called once after all uploads complete (not after each individual
      upload) to batch the media ID list update into a single RPC.
- [ ] Update `ExperienceNotifier` with the same pattern — single
      `saveExperience()` call after all uploads complete.
- [ ] Update `GenGearNotifier` and `GenExperienceNotifier` (creation flows)
      to support multi-select during initial content creation.
- [ ] Refactor `CommunityPreviewModal` to use `MediaUploadMixin` instead of
      its current direct-upload pattern, then add multi-select support.
      Direct repository calls from the modal widget violate the architecture
      rule that ViewModels are the only layer that calls Repositories.
- [ ] Update `MediaCarousel._handleAddMedia()` to handle the case where
      multiple items are added at once (animate to last new item, refresh
      media list once after all uploads).
- [ ] Test end-to-end on Android and iOS gallery multi-select UX.

**Validates:** All content creation and editing flows support multi-image
selection from gallery. Media association RPCs are batched efficiently.

### Phase 3: Polish and edge cases

**Goal:** Handle edge cases and improve the batch upload UX.

**Tasks:**
- [ ] Add cancel-upload capability — if user navigates away during batch
      upload, cancel remaining uploads gracefully (already-uploaded images
      are kept). Use `ref.mounted` checks between sequential uploads in
      `autoDispose` notifiers to detect early disposal.
- [ ] Handle mixed media: if user somehow selects images through a
      multi-select but the app receives video files, filter them out and
      show a toast (the `pickMultiImage` API should only return images, but
      guard defensively).
- [ ] Test with large batches (10 images) on slow connections to verify
      timeout handling and progress UX.
- [ ] Verify memory usage with multiple large images loaded simultaneously
      — ensure files are read and uploaded one at a time, not all buffered.

**Validates:** Batch upload is robust under adverse conditions (slow
network, large files, navigation during upload).

## Related Issues

- None identified — this is a standalone feature request with no dependencies
  or blockers.

---

## Audit Notes

_Audited against: docs/client/architecture.md, docs/client/caching.md,
docs/client/i18n.md, docs/server/architecture.md_

**Changes made:**

1. **i18n compliance (Phase 1):** Added explicit task to add all new
   user-facing strings (upload progress, failure toast, batch labels) to
   both `app_en.arb` and `app_es.arb` using ICU message format. Added
   reminder that ViewModels must expose typed state (counts), not resolved
   strings — widgets resolve via `context.l10n` in `build()`.

2. **Architecture violation fix (Phase 2):** Changed the
   `CommunityPreviewModal` task from "use multi-select (this screen uses a
   direct upload pattern without the mixin)" to explicitly call out the
   architecture violation and require refactoring to `MediaUploadMixin`.
   The current direct `mediaRepository.addMedia()` call from the modal
   widget violates the rule that ViewModels are the only layer that calls
   Repositories (docs/client/architecture.md, rule 1).

3. **Async safety (Phase 3):** Added `ref.mounted` check guidance to the
   cancel-upload task. Sequential uploads in `autoDispose` notifiers must
   check `ref.mounted` between iterations to detect early disposal — this
   is the established pattern documented in the architecture guide.

**No issues found with:**

- **Caching strategy:** Upload flow correctly uses `mediaRepository.addMedia()`
  which doesn't cache write operations. Post-upload reads go through existing
  cached `getMedia()` / `getMediaUrl()` paths. No manual cache maps proposed.
- **Server/client boundary:** No server changes required. The existing
  `AddMedia` RPC handles one file per call, which is correct for sequential
  uploads. The `repeated media_ids` field on `SaveGearRequest` /
  `SaveExperienceRequest` already supports list replacement.
- **Repository pattern:** All data access flows through repositories.
  The `MediaUploadMixin` correctly delegates to `mediaRepository`.
- **State management:** Freezed state with `copyWith()` for progress updates.
  Notifier pattern with Riverpod providers. No `ChangeNotifier` or
  `StatefulWidget` for business logic.
