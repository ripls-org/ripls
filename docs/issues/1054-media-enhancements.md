# Issue #1054: Media enhancements

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1054
**Filed:** 2026-04-07 | **Last activity:** 2026-04-07
**Labels:** none | **Assignees:** tescobar
**Context docs:** docs/client/media.md

## Problem Statement

The media experience in content views (particularly the experience content view)
needs several enhancements to the media strip and the full-screen media carousel.
The current implementation has a basic media strip with 48x48 thumbnails and a
full-screen carousel, but lacks several features: contributor name display in
the carousel, video attribution overlays, a visual indicator for the primary
(background) image, an add-media button in the carousel, a download-all
button, and owner edit mode (reorder + delete) in the carousel.

Most of these features either already exist in other parts of the codebase or
have partial implementations that need to be extended.

## Context from Comments

No comments on this issue. The requirements are clearly specified in the issue
body. No ambiguities to resolve.

## Existing Implementation Analysis

**Media Strip** (`inline_conversation_view.dart:713-806`):
- 48x48 thumbnails in a horizontal ListView
- Shows `attribution.creatorName` or `uploaderFirstName` below each thumbnail
- Tapping opens `MediaCarousel.show()` at the selected index
- No add button currently in the strip (add functionality exists elsewhere)

**Media Carousel** (`media_carousel.dart`):
- Full-page push via `NavigationHelpers.pushScreen()`
- 80x80 thumbnail strip at the bottom
- White border on the *currently viewed* item (not first/primary item)
- Attribution overlay on images via `AttributionWidget` (stock photos only)
- Edit mode already exists: `_isEditMode` toggle with `ReorderableGridView`
- Delete with X button in edit mode already exists
- Add button (plus icon) already exists at end of thumbnail row
- `_handleAddMedia()`, `_handleDeleteMedia()`, `_handleReorder()` all implemented
- No download functionality

**Experience ViewModel** (`experience_view_model.dart`):
- `deleteMedia()` (lines 849-899) and `reorderMedia()` (lines 901-926) already implemented
- Media upload via `_setMediaFile()` (lines 747-847) with optimistic placeholders
- `pickImageFromGallery()`, `pickImageFromCamera()`, `pickVideoFromGallery()` all exist

**Available packages:**
- `gal: ^2.3.0` - save media to device gallery (already in pubspec.yaml)
- `cached_network_image` - for downloading network images
- `video_player` - for video playback
- `share_plus` - for sharing

**MediaItemData** (`media_item_data.dart:23-38`):
- Already has `attribution`, `uploaderFirstName`, `thumbnailUrl` fields

## Potential Solutions

### Option A: Enhance existing MediaCarousel and media strip incrementally

- **Approach:** Modify the existing `MediaCarousel` widget and
  `InlineConversationView` media strip in place. Add contributor labels to
  carousel thumbnails, first-item border styling, download button to carousel
  header, and wire up the already-existing edit/reorder/delete/add capabilities
  from the experience content view.
- **Pros:** Minimal new code; leverages all existing edit mode, reorder, and
  delete implementations. Low risk of regressions.
- **Cons:** The MediaCarousel file is already 1067 lines and will grow further.
- **Effort:** Medium.

### Option B: Create a dedicated MediaScreen that wraps MediaCarousel

- **Approach:** Create a new `media_screen.dart` that acts as the container for
  viewing media, with a header bar (back + download), the main image/video
  display, and the MediaCarousel embedded as the thumbnail strip. Move some
  layout logic out of MediaCarousel into this new screen.
- **Pros:** Better separation of concerns; MediaCarousel stays focused on
  carousel behavior; MediaScreen owns the header and download button.
- **Cons:** More refactoring; risk of breaking existing callers of
  `MediaCarousel.show()`.
- **Effort:** Large.

### Option C: Enhance MediaCarousel with extracted helper methods

- **Approach:** Same as Option A but proactively extract methods and sub-widgets
  from MediaCarousel to keep it under 1000 lines (e.g., extract the
  `_FullscreenImageViewer`, the thumbnail strip, and the header bar into
  separate files).
- **Pros:** Keeps code maintainable while reusing existing infrastructure.
- **Cons:** Slightly more work than Option A due to extraction.
- **Effort:** Medium.

## Recommendation

**Option A** - enhance the existing components incrementally. The MediaCarousel
already has edit mode, reorder, delete, and add functionality built in. The
issue is primarily about wiring these features from the experience content view
and adding a few new visual elements (contributor labels in carousel, first-item
border, download button, video attribution overlay). Creating a new screen
(Option B) is over-engineering for what are mostly wiring and styling changes.
If the file grows too large, extraction can happen as a separate cleanup.

## Decisions

- **Download behavior:** Download all media immediately on tap with a progress
  indicator/snackbar. No confirmation dialog.
- **Download destination:** Save to device photo gallery using the `gal` package
  (already in pubspec.yaml).
- **Contributor names in carousel:** Use the same `creatorName` /
  `uploaderFirstName` fallback logic from `inline_conversation_view.dart:727-730`.
- **First-image border:** Use a theme-aware accent/primary border
  (`AppColors.primary(context)`) on index 0 that persists regardless of
  selection. Currently-selected item keeps its white 3px border. When the first
  item is also selected, use the white border (selected takes precedence).
- **Add button:** Use the existing `onAddMedia` callback pattern already
  implemented in `MediaCarousel`.

## Implementation Plan

### Phase 1: Contributor names in carousel thumbnails and video attribution overlay
**Goal:** Show contributor/attribution names on carousel thumbnails (matching
the media strip behavior) and add attribution overlay on videos.
**Tasks:**
- [ ] Add contributor name label below each thumbnail in `_buildThumbnail()` and `_buildThumbnailForGrid()` in `media_carousel.dart`, using the same `creatorName` / `uploaderFirstName` fallback logic from `inline_conversation_view.dart:727-730`
- [ ] Add `uploaderFirstName` field to the `MediaItem` class in `media_carousel.dart` (currently only has `attribution`)
- [ ] Update all callers that create `MediaItem` objects to pass `uploaderFirstName` from `MediaItemData`
- [ ] Add attribution text overlay on video items in the carousel main view (same positioning as the existing image attribution overlay at lines 351-376)
- [ ] **[i18n]** Any new static label text (e.g., fallback placeholder for missing contributor names) must be added to `app_en.arb` and `app_es.arb` and accessed via `context.l10n`. Dynamic contributor names are data, not i18n strings.
**Validates:** Contributor names appear correctly for both stock photos and user uploads in carousel and on video full-view.

### Phase 2: First-image border indicator
**Goal:** Visually distinguish the first image (background image) in both the
media strip and the carousel thumbnail row.
**Tasks:**
- [ ] **[Theming]** In `media_carousel.dart` `_buildThumbnail()`, add a distinct border to index 0 using a theme-aware color from `AppColors` (e.g., `AppColors.primary(context)` or a new semantic color if needed — never hardcoded hex or `Colors.*` constants). The border must persist regardless of which item is currently selected.
- [ ] In `media_carousel.dart` `_buildThumbnailForGrid()`, apply the same first-item border in edit mode
- [ ] In `inline_conversation_view.dart` `_buildMediaStrip()`, add the same first-item border treatment to the first thumbnail
- [ ] Ensure the currently-selected white border and the first-item border can coexist (e.g., first item shows accent/primary border when not selected, white border when selected)
**Validates:** Users can identify which image is the background image at a glance.

### Phase 3: Add-media button in carousel
**Goal:** Allow any user to add media from the carousel view.
**Tasks:**
- [ ] Ensure the add button (plus icon) in `_buildAddThumbnail()` and `_buildAddThumbnailForGrid()` is visible when `canAddMedia` is true - this already exists
- [ ] Wire `canAddMedia: true` and `onAddMedia` callback when opening the carousel from the experience content view (via `InlineConversationView` or `ContentViewHelpers`)
- [ ] Ensure `onAddMedia` triggers the experience's media picker flow (`pickImageFromGallery` / `pickImageFromCamera` / `pickVideoFromGallery`)
- [ ] Verify the plus button matches thumbnail size and appears after the last item
**Validates:** Users can add images/video from the carousel and see them appended.

### Phase 4: Download button
**Goal:** Add a download button to the carousel header that downloads all media
to the device gallery.
**Tasks:**
- [ ] Add a download icon button to the right side of the carousel's top header bar (lines 251-284 of `media_carousel.dart`), next to the existing back button
- [ ] **[Architecture]** Add an `onDownloadAll` callback parameter to `MediaCarousel`. The widget must NOT call repositories or perform network I/O directly (violates "ViewModels are the ONLY layer that calls Repositories"). The download button invokes the callback; all download logic lives in the ViewModel.
- [ ] **[Architecture]** Implement `downloadAllMedia()` in `ExperienceNotifier` (ViewModel layer). This method iterates over all media items, uses `MediaRepository` to download images (via `flutter_cache_manager`) and videos (via `getVideoFile()`), and saves each to the device gallery using the `gal` package (`Gal.putImage()` / `Gal.putVideo()`).
- [ ] **[i18n]** Add ARB entries for download progress and completion snackbar messages (both `app_en.arb` and `app_es.arb`). Show progress/completion via snackbar in the widget after the ViewModel callback completes.
- [ ] Wire the `onDownloadAll` callback when opening the carousel from the experience content view, connecting to `ExperienceNotifier.downloadAllMedia()`
**Validates:** Tapping download saves all media items to the device photo gallery.

### Phase 5: Owner edit mode (reorder + delete)
**Goal:** Enable long-press on thumbnails to enter edit mode for the experience
owner.
**Tasks:**
- [ ] The edit mode already exists in `MediaCarousel` (`_isEditMode`, `_handleDeleteMedia`, `_handleReorder`, `_buildReorderableGrid`) - verify it works end-to-end
- [ ] Wire `onDeleteMedia` and `onReorderMedia` callbacks when opening the carousel from the experience content view, connecting to `ExperienceNotifier.deleteMedia()` and `ExperienceNotifier.reorderMedia()`
- [ ] Add long-press gesture on thumbnails to toggle edit mode (currently toggled via an Edit button in the header bar) - add `onLongPress` to `_buildThumbnail()` GestureDetector
- [ ] Verify that in edit mode: thumbnails show X delete button, thumbnails are draggable to reorder, first item gets the background-image border after reorder
- [ ] Ensure only the experience owner sees edit capabilities (check `isOwner` or similar flag before passing callbacks)
**Validates:** Owner can long-press to enter edit mode, drag to reorder, delete with X, and the first item always shows the background-image border.

## Related Issues
- None referenced in the issue or comments.

## Audit Notes

**Audited against:** docs/client/architecture.md, docs/client/caching.md,
docs/client/i18n.md, docs/server/architecture.md

**Changes made during audit:**

1. **Phase 1 — i18n:** Added reminder that any new static label text must use
   ARB files and `context.l10n`. Dynamic data (contributor names) is exempt.
2. **Phase 2 — Theming:** Changed "gold/accent border" to require
   `AppColors.xxx(context)` theme-aware colors. Hardcoded colors break dark
   mode.
3. **Phase 4 — Architecture (critical):** Original plan had download logic
   (`_handleDownloadAll`, `MediaRepository.getVideoFile()`, `Gal.putImage()`)
   executing directly in the `MediaCarousel` widget. This violates the rule
   that Screens/Widgets never access repositories directly. Moved all download
   logic to `ExperienceNotifier` (ViewModel layer) with the widget receiving
   an `onDownloadAll` callback. Added ARB entries requirement for snackbar
   messages.
4. **No server-side changes needed.** This issue is entirely client-side.
   Caching patterns are unaffected — existing `MediaRepository` caching
   continues to work for all media access.
