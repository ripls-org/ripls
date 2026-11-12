# Map Improvements: Avatar Markers & Sheet-Aware Centering

> **Audit Status:** Reviewed against `docs/client/architecture.md`,
> `docs/client/caching.md`, and `docs/server/architecture.md` on 2026-03-14.
> See `[AUDIT FIX]` markers for changes made during review.

## Problems

### 1. Generic Ripple Markers

The discover map currently displays items as concentric colored circles
(ripple markers) — three `CircleAnnotation` layers per item with varying opacity.
These markers are visually distinctive but carry no information about *who* owns
the item. Users must tap a marker or browse the list to learn who is sharing.

**Impact:** Reduced discoverability. Users can't visually scan the map for items
from people they know or recognize.

### 2. Map Centering Ignores Bottom Sheet

Camera animations (`focusOnItem()`, `updateCameraForItems()`) center items in
the geometric middle of the full screen. The bottom sheet (`DiscoverBottomSheet`)
covers 20%–80% of the screen height, so centered items are frequently hidden
behind the sheet.

Specific issues:
- `focusOnItem()` centers on the item's lat/lng with no viewport padding
- `updateCameraForItems()` calculates bounds using geographic padding only
  (50% coordinate padding in `MapHelper.calculateCameraForLocations()`), with
  no awareness of the sheet
- `_centerOnUserLocation()` centers on the user's position with no padding
- The gallery overlay is hardcoded at `size.height * 0.21 + 8`, tightly coupled
  to the 0.20 sheet peek size

The Mapbox SDK `CameraOptions` supports a `padding` parameter (EdgeInsets) that
shifts the effective viewport center, but it is not used anywhere.

---

## Current Architecture

**Markers** (`discover_map_helper.dart`):
- One `CircleAnnotationManager` owns all markers
- Each item gets 3 `CircleAnnotation` objects (outer, middle, inner)
- Selection changes color via `_updateAnnotationColor()`
- Tap detection on the inner circle annotation

**Camera** (`discover_map_helper.dart` + `map_helper.dart`):
- `MapHelper.calculateCameraForLocations()` computes center + zoom from bounds
- `DiscoverMapHelper.updateCameraForItems()` wraps that with pitch=45
- `DiscoverMapHelper.focusOnItem()` does a simple center-on-point flyTo

**Data availability**: `DiscoverItem.user` provides the `User` proto which has
`media_id` (primary profile image) and `name`. This data is already loaded
for every item in the search results.

---

## Potential Solutions

### Problem 1: Avatar Markers

#### Option A: `PointAnnotation` with Rendered Image (Recommended)

Use Mapbox's `PointAnnotationManager` with `PointAnnotationOptions.image` to
display a pre-rendered avatar bitmap at each marker location.

**Approach:**
1. For each item, the ViewModel pre-resolves owner avatar URLs via
   `MediaRepository` and passes rendered image bytes to the map helper
   `[AUDIT FIX]` — helpers/utils must not call repositories directly;
   ViewModels are the only layer that calls Repositories
   (architecture.md rule 1)
2. Download and decode the image to a `ui.Image`
3. Render a circular-clipped avatar with a white border onto a `Canvas`
4. Encode to PNG bytes and pass as `PointAnnotationOptions.image`
5. Fall back to a colored circle with initials for users without avatars

**Pros:**
- Native Mapbox annotation — performant, hardware-accelerated, auto-clusters
- Tap handling via `PointAnnotationManager.tapEvents` (same pattern as current)
- No platform-specific code needed
- Clean migration path from current `CircleAnnotation` approach

**Cons:**
- Async image loading adds latency when markers first appear
- Must handle cache/placeholder while images load
- Selection state requires swapping the image (adding a colored border ring)

#### Option B: ViewAnnotation (Flutter Widget Overlay)

Use Mapbox `ViewAnnotation` API to place actual Flutter widgets on the map.

**Pros:**
- Full Flutter widget tree — can use `CachedMediaImage` directly
- Easiest to style consistently with the rest of the app

**Cons:**
- Performance: each ViewAnnotation is a native platform view, heavy at 20+ markers
- No built-in clustering support
- More complex lifecycle management (views tied to map lifecycle)
- mapbox_maps_flutter v2.3 ViewAnnotation API is less mature

#### Option C: Symbol Layer with SDF Icons

Use a Mapbox style layer with `icon-image` sourced from a GeoJSON data source.

**Pros:**
- Best performance for large numbers of markers
- Native clustering via Mapbox expressions

**Cons:**
- Most complex to implement
- Requires managing a dynamic image atlas
- Hard to update individual marker states (selection)
- Over-engineered for the current marker count (~10-50 items)

### Problem 2: Sheet-Aware Centering

#### Option A: Bottom Padding via `CameraOptions.padding` (Recommended)

Pass bottom padding to every `flyTo()` call to shift the effective viewport
center upward, away from the sheet.

**Approach:**
1. Track the current sheet fraction (0.20–0.80) in `DiscoverScreen`
2. Compute bottom padding as `screenHeight * sheetFraction`
3. Pass `padding: MbxEdgeInsets(bottom: paddingPx)` in every `CameraOptions`
4. Update padding when the sheet snaps to a new position

**Pros:**
- Uses the SDK's built-in mechanism — no manual coordinate math
- Works for both single-item focus and multi-item bounds
- Automatically adjusts zoom level for bounded views

**Cons:**
- Need to pipe screen dimensions and sheet fraction into `DiscoverMapHelper`

#### Option B: Offset the Center Coordinate

Manually shift the center latitude northward by an amount proportional to the
sheet coverage.

**Pros:**
- Simpler implementation — no API surface changes

**Cons:**
- Fragile: the pixel-to-coordinate offset varies with zoom level
- Doesn't work well for bounded camera (multi-item) views
- Would need recalculation at every zoom level

---

## Recommendations

1. **Avatar Markers**: Option A — `PointAnnotation` with rendered images.
   Native performance, clean tap handling, and a well-supported API path.

2. **Sheet-Aware Centering**: Option A — `CameraOptions.padding`.
   The SDK provides this exact mechanism; we should use it.

---

## Open Questions

1. **Avatar size on map**: What diameter should avatars be? 40px (current inner
   circle is 20pt radius) feels right, but should we test 32px or 48px?

2. **Selection indicator**: Currently, selected markers change color. With
   avatars, should selection be indicated by a colored ring around the avatar,
   a scale-up animation, or a different approach?

3. **Initials fallback**: For users without a profile photo, should the fallback
   be a colored circle with initials (matching the app's `CircleAvatar` pattern)
   or a generic person icon?

4. **Sheet position tracking**: Should the camera re-center when the user drags
   the sheet to a new snap point? Or only adjust padding for the next explicit
   focus/center action? Re-centering on every sheet drag could feel jittery.

5. **Clustering**: With avatar markers, should nearby items cluster into a count
   badge when zoomed out? `PointAnnotationManager` supports this, but it adds
   complexity. Could be a follow-up.

6. **Item type differentiation**: Currently all items use the same ripple color.
   With avatars, should gear/request/experience have different border colors or
   a small type badge? Or is the avatar alone sufficient?

---

## Phased Implementation Plan

### Phase 1: Sheet-Aware Camera Padding

**Goal:** Fix the centering problem so focused items are never hidden by the
bottom sheet.

**Files modified:**
- `app/lib/core/utils/discover_map_helper.dart` — accept bottom padding in
  `updateCameraForItems()`, `focusOnItem()`, and store it as state
- `app/lib/core/utils/map_helper.dart` — pass padding through
  `calculateCameraForLocations()` return value (or accept it as parameter)
- `app/lib/presentation/screens/discover/discover_screen.dart` — compute padding
  from sheet position and screen height, pass to map helper

**Changes:**
1. Add `setBottomPadding(double pixels)` method to `DiscoverMapHelper`
2. Include `padding: MbxEdgeInsets(bottom: _bottomPadding)` in all `CameraOptions`
   within `focusOnItem()`, `updateCameraForItems()`, and `_centerOnUserLocation()`
3. In `DiscoverScreen`, listen to `_sheetController` position changes and call
   `_mapHelper.setBottomPadding(screenHeight * sheetFraction)`
4. Set initial padding based on the sheet's initial size (0.60)

**Testing:**
- Unit tests for `DiscoverMapHelper` verifying padding is included in camera options
- Manual verification: select items near sheet edge, confirm they remain visible

---

### Phase 2: Avatar Image Rendering Utility

**Goal:** Build the image pipeline that converts a user's profile photo into a
circular bitmap suitable for map annotations.

**Files created:**
- `app/lib/core/utils/map_avatar_renderer.dart` — pure utility class (no
  repository access)

**Files modified:**
- `app/lib/presentation/viewmodels/discover_view_model.dart` — add avatar
  resolution logic that calls `MediaRepository`

`[AUDIT FIX]` The original plan had `MapAvatarRenderer` accept a
`MediaRepository` ref and fetch URLs itself. This violates architecture.md
rule 1: "ViewModels are the ONLY layer that calls Repositories." The renderer
is a utils class and must not access repositories. Instead, the ViewModel
resolves avatar URLs via `MediaRepository`, then passes the raw image URL
(or bytes) to the renderer.

**Changes:**
1. `MapAvatarRenderer.renderAvatar()` — pure async method that:
   - Takes an `imageUrl` (already resolved by ViewModel), and `size` (diameter
     in pixels)
   - Downloads and decodes the image to `ui.Image`
   - Paints onto a `Canvas`: circle clip + avatar image + white stroke border
   - Returns `Uint8List` PNG bytes
2. `MapAvatarRenderer.renderInitials()` — fallback method that:
   - Takes a name string and background color
   - Paints a colored circle with white initials text
   - Returns `Uint8List` PNG bytes
3. `DiscoverNotifier` (or a new `DiscoverMapNotifier`) resolves avatar URLs
   for all visible items via `MediaRepository.getMediaUrl()` and passes them
   to the renderer. The ViewModel owns this data flow.

`[AUDIT FIX]` The original plan added an in-memory `Map<String, Uint8List>`
cache inside the renderer utility. This violates caching.md rule: "Never
implement manual cache maps in ViewModels" (and by extension, utils). Avatar
URL resolution is already cached by `MediaRepository`. For the rendered
bitmap bytes, use a short-lived map scoped to a single `addItemMarkersToMap()`
call (same-owner dedup within one batch) rather than a persistent cache. If
persistent caching is needed later, it belongs in a repository, not a utility.

**Testing:**
- Unit tests verifying PNG bytes are produced for valid URL inputs
- Unit tests for initials fallback
- Unit tests for same-owner dedup within a single batch

---

### Phase 3: Replace Ripple Markers with Avatar Markers

**Goal:** Swap out `CircleAnnotation` ripple markers for `PointAnnotation`
avatar markers on the discover map.

**Files modified:**
- `app/lib/core/utils/discover_map_helper.dart` — major rework
- `app/lib/presentation/viewmodels/discover_view_model.dart` — avatar
  data resolution
- `app/lib/presentation/screens/discover/discover_screen.dart` — pass
  resolved avatar data to map helper

`[AUDIT FIX]` The original plan had `DiscoverMapHelper._createAvatarMarker()`
call `MapAvatarRenderer.renderAvatar()` with a `mediaId`, implying the helper
would resolve media URLs. Corrected: the ViewModel resolves avatar URLs via
`MediaRepository`, renders bitmaps via `MapAvatarRenderer`, and passes
`Map<String, Uint8List>` (userId → avatar bytes) to the helper. The helper
receives pre-rendered images only. This keeps the helper as a pure SDK
lifecycle utility (architecture.md: "Helpers/Utils: Pure utility functions").

**Changes:**
1. Replace `CircleAnnotationManager` with `PointAnnotationManager`
2. Change `addItemMarkersToMap()` signature to accept pre-rendered avatar
   bytes: `Map<String, Uint8List> avatarImages` (keyed by userId)
3. Replace `_createRippleMarker()` with `_createAvatarMarker()`:
   - Look up pre-rendered bytes from the passed-in map
   - Create `PointAnnotationOptions` with the image bytes and position
   - Store annotation → item mapping (same pattern as current)
4. Update `_selectAnnotation()` / `_updateAnnotationColor()`:
   - Re-render the avatar with a colored selection ring and swap the image
   - Or use `iconSize` scaling for a simpler selection indicator
5. Update `setupMarkerTapListener()` to use `PointAnnotationManager.tapEvents`
6. Remove `_annotationLayers` (no longer 3 circles per marker)
7. In ViewModel: add `_resolveAvatarImages()` that calls
   `MediaRepository.getMediaUrl()` for each unique `user.mediaId`, then
   `MapAvatarRenderer.renderAvatar()` to produce bytes. Screen passes these
   to `addItemMarkersToMap()`.

**Files deprecated:**
- `app/lib/presentation/widgets/ripple_map_marker.dart` — no longer needed
  (can be deleted if unused elsewhere)

**Testing:**
- Update existing `discover_map_helper_test.dart` tests
- New tests for avatar marker creation and selection (helper receives bytes,
  no repository mocking needed in helper tests)
- ViewModel tests: verify `MediaRepository` is called for avatar resolution
- Manual testing: verify markers display correct avatars, tap handling works,
  selection ring is visible

---

### Phase 4: Polish & Edge Cases

**Goal:** Handle edge cases, optimize performance, and clean up.

**Changes:**
1. **Loading race condition**: If search results arrive before avatar images are
   ready, show initials markers and swap in avatars as they load
2. **Same-owner grouping**: Multiple items from the same user should reuse the
   same rendered bitmap within a single marker batch (dedup in ViewModel's
   `_resolveAvatarImages()` by userId). `[AUDIT FIX]` — no persistent cache
   in the util; `MediaRepository` already caches the URL resolution. Bitmap
   dedup is per-batch only.
3. **Gallery overlay positioning**: Replace hardcoded `size.height * 0.21 + 8`
   with a value derived from the actual sheet peek size constant
4. **Memory cleanup**: ViewModel clears resolved avatar bytes when community
   changes (via existing community-change listener)
5. **Selection ring**: Fine-tune the visual treatment for selected avatar markers
   (ring color, thickness, optional scale animation)
6. `[AUDIT FIX]` **Image cache key alignment**: When resolving avatar URLs,
   use `ImageCacheKeys.thumbnail(mediaId)` for the disk-cached image fetch
   (avatars are small display < 200px per caching.md table). This ensures
   the rendered bitmap reuses the same disk-cached thumbnail that avatar
   chips use elsewhere in the app.

**Testing:**
- Manual testing across different screen sizes
- Verify no memory leaks on repeated community switches
- All existing discover tests still pass
