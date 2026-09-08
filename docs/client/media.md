---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client media system — presigned URLs with stable media-ID cache keys, smart thumbnail/full-image selection, multi-tier caching, and widget-owned VideoPlayerController lifecycle for full-screen video backgrounds.
  globs: [app/lib/presentation/widgets/media/**, app/lib/data/repositories/media_repository.dart, app/lib/presentation/widgets/chat/cached_media_image.dart]
  triggers: [media, presigned-url, cached-media-image, thumbnail, video-player, cache-key, video-background]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "5c2d3e59c"
  verified_on: "2026-07-07"
---
# Client Media System

## Overview

The Ripls Flutter app uses a sophisticated media system with multi-tier caching, presigned URLs, and smart thumbnail/full-image selection. Media flows from server-generated presigned URLs through repositories and cache layers to UI widgets with minimal overhead.

## Architecture Principles

**Presigned URLs with Stable Cache Keys:** Media URLs expire 35 minutes after generation (`presignedURLExpiry` in `server/services/media/service.go`), and the client metadata cache holds them for up to 30 minutes (`CACHE_TTL_MINUTES`). Cache keys based on stable media IDs allow image bytes to persist across URL refreshes — but stable keys only help on bytes-already-cached paths. See [Presigned URL Lifecycle and Closure Capture](#presigned-url-lifecycle-and-closure-capture) below for the failure mode this creates and the resolve-at-mount-time pattern that fixes it.

**Smart URL Selection:** Repositories automatically choose between thumbnail and full-resolution URLs based on context (list views use thumbnails, detail views use full images).

**Multi-Tier Caching:** Stash-based memory caching for metadata (media objects) with LRU eviction, plus CachedNetworkImage disk cache for image bytes.

## Media Data Flow

### Complete Request Flow

```
User opens discover screen
    ↓
DiscoverGearCard displays gear item
    ↓
DiscoverViewModel.loadMediaUrl(mediaId)
    ↓
MediaRepository.getMediaUrl(mediaId)
    ↓
StashCacheManager (via CacheService)
    ├─ Memory cache hit? → Return cached Media object
    └─ Memory cache miss? → Fetch from MediaService
        ↓
        MediaService.getMedia(mediaId) [gRPC]
        ↓
        Server returns: {url: full, thumbnailUrl: thumb, ...}
        ↓
        Cache Media object in memory
    ↓
Extract best URL: thumbnailUrl.isNotEmpty ? thumbnailUrl : url
    ↓
Return MediaUrl(url: thumbURL, isThumbnail: true, mediaId: mediaId)
    ↓
ViewModel updates state: state.copyWith(mediaUrls: {..., itemId: thumbURL})
    ↓
Widget reads from state: ref.watch(discoverProvider).mediaUrls[itemId]
    ↓
CachedMediaImage(imageUrl: thumbURL, cacheKey: 'media_mediaId')
    ↓
CachedNetworkImage loads from:
    ├─ DartFileCache disk cache (keyed by 'cache_media_mediaId')
    ├─ Or download from presigned URL
    └─ Display 110x130 thumbnail
```

---

## Video Playback

### Overview

All five content types — Gear, Experience, Request, Community, and Story — support video as the full-screen background. When a ViewModel loads media and detects a video content type, it initializes a `VideoPlayerController` and passes it to `MediaBackground`. Videos loop silently by default, and a volume FAB lets users opt in to audio (except Stories, which are purely atmospheric).

### Video Detection

Video type is determined from the `contentType` field on `MediaUrl`, which is populated from `GetMediaResponse.contentType` in the repository layer. This is reliable for both server-loaded media and locally uploaded files:

```dart
final isVideo = mediaUrl.contentType?.startsWith('video/') == true;
```

URL-based heuristics (checking for `.mp4` or `"video"` in the path) are not used because presigned GCS URLs don't reliably contain this information.

### VideoPlayerController Lifecycle

**Widgets own the controller, not ViewModels.** ViewModels expose URL/identity state (`mediaPath`, `mediaId`, `isVideo`, `isMuted`); a dedicated widget — [`VideoBackgroundHost`](../../app/lib/presentation/widgets/media/video_background_host.dart) — creates the `VideoPlayerController` in its `State`, syncs volume from props, and disposes in `State.dispose()`. Because Flutter guarantees `dispose()` runs *after* the widget is unmounted, the platform-level AVFoundation player can never be torn down while a `VideoPlayer` widget is still in the tree.

This pattern replaces the older "ViewModel owns the controller" approach, which created a race against Riverpod's autoDispose: when an autoDispose provider fired `onDispose` during a widget update pass (e.g. switching communities in the sidebar), the platform player was removed mid-frame and `_VideoPlayerState.build` crashed with `StateError: No active player with ID N`. Widget-owned controllers eliminate that race by tying the controller's lifetime to the widget element's lifetime.

**ViewModel state:**
- `String? mediaPath` — URL for image; full video URL for video
- `String? mediaId` — stable cache key
- `bool isVideo` — discriminator
- `bool isMuted` — authoritative mute state (host syncs to controller)
- `String? backgroundThumbnailUrl` — static frame shown while host loads the controller

**ViewModel pattern:**
```dart
Future<void> _loadMediaFromServer(String mediaId) async {
  final mediaUrl = await mediaRepository.getFullMediaUrl(mediaId);
  final isVideo = mediaUrl.contentType?.startsWith('video/') == true;
  state = state.copyWith(
    mediaPath: mediaUrl.url,
    mediaId: mediaId,
    isVideo: isVideo,
  );
}

// Mute is just state — the host applies it to its controller.
void toggleMute() {
  state = state.copyWith(isMuted: !state.isMuted);
}
```

No `_videoController` field. No `ref.onDispose` for video. No `createCachedVideoController` calls in the ViewModel.

**Widget pattern:**
```dart
VideoBackgroundHost(
  mediaPath: state.mediaPath,
  mediaId: state.mediaId,
  isVideo: state.isVideo,
  thumbnailUrl: state.backgroundThumbnailUrl,
  isMuted: state.isMuted,
)
```

The host calls `createCachedVideoController()` internally, mounts/remounts the controller when `mediaId` or `isVideo` changes, applies `setVolume(...)` when `isMuted` flips, and disposes the controller in its `State.dispose()`.

**Volume FAB visibility check:** gate on `state.isVideo` only — no longer on `videoController?.value.isInitialized`. The host owns the loading state; the FAB just toggles `state.isMuted`, and the host applies it whenever the controller becomes (or already is) initialized.

**Out-of-scope ViewModels:** `community_edit_view_model.dart` and `gen_experience_view_model.dart` are content creation/edit screens that still hold their own controllers. They aren't on the feed crash path. Migrate them to the host pattern when convenient.

### Video Caching

Videos are cached on-device to avoid repeated HTTP range requests to the server. The caching system is analogous to how `CachedNetworkImage` handles image bytes — a separate disk-based file cache, distinct from the Stash metadata cache.

**How it works:**
- `MediaRepository.getVideoFile(mediaId, url)` downloads the video to local disk on first request, keyed by stable media ID (`video_$mediaId`). Subsequent calls return the cached file without a network request, even after the presigned URL rotates
- The shared helper `createCachedVideoController()` ([video_cache_helper.dart](../app/lib/core/utils/video_cache_helper.dart)) wraps this: it calls `getVideoFile()`, creates a `VideoPlayerController.file()`, initializes it, and sets looping + muted defaults
- All content ViewModels use this helper for server-loaded video. Local uploads bypass it since the file is already on-device

**Cache internals:**
- Backed by `VideoCacheManager` ([video_cache_manager.dart](../app/lib/data/cache/video_cache_manager.dart)), a singleton extending `flutter_cache_manager`'s `CacheManager`
- Max 100 cached objects, 7-day stale period, LRU eviction
- `MediaRepository.evictVideo(mediaId)` removes a specific cached video
- `VideoCacheManager` is an internal implementation detail of the data layer — ViewModels access video caching only through `MediaRepository`

### Default Mute Behavior

All video backgrounds start muted (`setVolume(0.0)`) to avoid surprising users. A volume FAB appears on each content view when video is active, allowing opt-in audio.

### Volume FAB

Each content view conditionally renders a volume floating action button when `state.isVideo == true` and the controller is initialized:

```dart
if (state.isVideo && state.videoController?.value.isInitialized == true)
  FloatingActionConfig(
    icon: state.isMuted ? Icons.volume_off : Icons.volume_up,
    onPressed: () => ref.read(viewModelProvider.notifier).toggleMute(),
  ),
```

`isMuted` is tracked in ViewModel state. Each ViewModel's `toggleMute()` delegates to the shared `toggleVideoMute()` helper:

```dart
// app/lib/core/utils/video_mute_helper.dart
void toggleVideoMute({
  required VideoPlayerController? controller,
  required bool currentlyMuted,
  required void Function(bool isMuted) updateState,
}) {
  if (controller == null || !controller.value.isInitialized) return;
  final newMuted = !currentlyMuted;
  controller.setVolume(newMuted ? 0.0 : 1.0);
  updateState(newMuted);
}
```

### Video in MediaCarousel

The `MediaCarousel` widget already supports video via `MediaItemData.isVideo`. Each page in the carousel that contains a video gets its own per-page `VideoPlayerController`. Tapping a video in the carousel plays/pauses it. `isVideo` on `MediaItemData` is set from `mediaUrl.contentType` for reliable detection.

---

## Media Carousel Screen

**File:** [app/lib/presentation/widgets/media/media_carousel.dart](../../app/lib/presentation/widgets/media/media_carousel.dart)

The `MediaCarousel` is a full-screen media viewer pushed onto the navigation stack. It handles images, videos, multiple items, upload, delete, reorder, and save-to-gallery.

### Entry Point

The carousel is always opened via the static `MediaCarousel.show()` method, never constructed directly:

```dart
MediaCarousel.show(
  context: context,
  getMediaItems: () => widget.mediaItems.map((m) => MediaItem(...)).toList(),
  initialIndex: i,
  canAddMedia: widget.canAddMedia,
  onAddMedia: widget.onAddMedia,       // null → hides add button
  onDeleteMedia: widget.onDeleteMedia, // null → hides delete button
  onSaveMedia: widget.onSaveMedia,
  onReorderMedia: widget.onReorderMedia,
  onDownloadAll: widget.onDownloadAll,
);
```

`getMediaItems` is a **closure** — it is called each time the carousel needs to refresh its list (after add, delete, or reorder). The closure reads from the calling widget's live `widget.mediaItems` prop, so it always reflects the latest Riverpod state once the parent has rebuilt.

**Important:** `onClose` is always a no-op. The carousel pops itself using its own live `context` via `performClose()` (a `SwipeToCloseMixin` override), avoiding stale-context crashes from async slide animations.

### Layout

The carousel uses `SwipeToCloseMixin` to support a swipe-down-to-dismiss gesture. The body is a `Scaffold(backgroundColor: Colors.black)` — the `Scaffold` ancestor is required to prevent Flutter's root `DefaultTextStyle` (which has `TextDecoration.underline`) from leaking decoration to all descendant `Text` widgets.

The layout is a `Stack` with two layers:

1. **Content column** (bottom layer): header spacer → `PageView` (fills remaining space) → context bar → thumbnail strip
2. **Header overlay** (top layer): floats over the content, overlapping the top of the `PageView`

### PageView

Each page displays one `MediaItem`. Images use `CachedMediaImage` whose URL is resolved at mount time by watching `fullMediaUrlProvider(mediaId)` (a `FutureProvider.autoDispose.family<MediaUrl, String>`) — the widget renders `AsyncValue.when()` for loading / data / error. The provider returns a fresh full-resolution `MediaUrl` from `MediaRepository.getFullMediaUrl(mediaId)`, and the widget passes `mediaUrl.url` + `mediaUrl.cacheKey` (the stable `media:full:{mediaId}` key) into `CachedMediaImage`. Tapping opens `FullscreenImageViewer` (same provider-driven path) pushed as a transparent route. The fallback path — `MediaItem` without a `mediaId` (locally-staged uploads) — renders `item.url` directly with no re-resolution.

Videos use a per-page `VideoPlayerController` initialized lazily via `createCachedVideoController()` as the user pages to them. Before the controller is created, `_initializeCurrentVideo` calls `mediaRepository.getFullMediaUrl(mediaId)` to get a fresh presigned URL — the on-disk `VideoFileCache` ignores the URL on a cache hit, so the warm-path cost is one in-memory metadata-cache lookup. On a cold cache (first-time view) this avoids the same expired-URL failure as the image path.

An "add media" page is appended at the end when `canAddMedia` is true, showing an upload prompt.

### Header

A fixed 56px bar overlay at the top showing:
- **Back arrow** — calls `handleClose()` (the mixin's swipe-close entry point)
- **Item count** — "N items" centered
- **Right action** — "Save All" pill (calls `onDownloadAll`) in normal mode; "Done" text button in edit/reorder mode

### Context Bar

Sits between the `PageView` and the thumbnail strip. Updates on every page change to reflect the currently viewed item:

- **Attribution present** (stock photo): renders `AttributionWidget(compact: true)` showing "📷 CreatorName on Provider" with tappable links that open the creator profile URL and platform URL via `url_launcher`. The `AttributionWidget` is the same component used in content info modals.
- **No attribution** (user upload): shows `uploaderFirstName` in the accent color as plain text.
- **Delete button** (trash icon): shown only when `onDeleteMedia != null` and the item has a non-empty `mediaId`. Prompts a confirmation dialog before calling the callback.
- **Save button**: shown when `onSaveMedia != null`. Calls `onSaveMedia(mediaId)` to download the current item to the device photo gallery via `Gal`.

### Thumbnail Strip

A horizontal scrollable row of 60×60px thumbnails at the bottom of the screen. The selected thumbnail has a 2.5px accent-colored border and full opacity; unselected thumbnails are 55% opacity. Below each thumbnail is a 9px label showing the attribution creator name (if present) or the uploader's first name — matching the media strip beneath the chat input.

Each thumbnail resolves its URL through `mediaObjectProvider(mediaId)` (thumbnail-preferred `MediaUrl`) and renders via `CachedMediaImage` with `mediaUrl.cacheKey` — the stable `media:thumb:{mediaId}` key. This is the only correct way to render a thumbnail for video items: `MediaItem.url` on a video holds the **video** URL, which `CachedNetworkImage` cannot decode as an image and which leaves the tile blank. The `mediaObjectProvider` returns the server's `thumbnailUrl` for both photo and video items, falling back to the full URL for photos without a thumbnail and to an empty URL for videos without one (CachedMediaImage handles the empty case by rendering a placeholder). For locally-staged uploads with no `mediaId` yet, the strip falls back to `item.url` directly.

**Normal mode:** `SingleChildScrollView` wrapping a `Row`. Thumbnails are tappable to navigate the `PageView`. Long-pressing any thumbnail (when `onDeleteMedia != null`) activates edit/reorder mode.

**Edit/reorder mode:** Switches to `ReorderableListView(scrollDirection: Axis.horizontal)` with `buildDefaultDragHandles: false`. Each thumbnail is wrapped in `ReorderableDragStartListener` so the entire thumbnail acts as the drag handle. Drag-and-drop calls `onReorderMedia(newMediaIds)` and updates local state optimistically; on error it reverts via `_refreshMediaItems()`. The header shows a "Done" button to exit edit mode.

When `canAddMedia` is true, a "+" button appears at the end of the strip in both modes. It is the `footer` of the `ReorderableListView` (so it is never included in drag-and-drop) and is shown as a `Column` sibling of the thumbnail to align it with the label row height.

### Adding Media

The "+" button and the add-media page both call `_handleAddMedia()`:

1. Sets `_isAddingMedia = true` (shows spinner, disables the button).
2. Awaits `widget.onAddMedia()` — this is `_showCarouselMediaPicker()` on the parent `ExperienceContentView`, which opens a bottom sheet for camera/gallery selection, then awaits the upload future from `ExperienceNotifier`.
3. Waits **two post-frame callbacks** before calling `_refreshMediaItems()`. This is necessary because Riverpod schedules widget rebuilds for the next frame; one callback fires at the end of the frame the rebuild is scheduled, and the second fires after the rebuild completes. Without this, `getMediaItems()` would still read the pre-upload `widget.mediaItems`.
4. Waits **one more post-frame callback** after `_refreshMediaItems()` before calling `animateToPage(N)`. The `PageView` needs one frame to rebuild with the new `itemCount` before it accepts a page index equal to `N`.

### Refreshing After Mutations

All mutations (add, delete, reorder) call `_refreshMediaItems()` which re-invokes `widget.getMediaItems()` and calls `setState`. This is the single source of truth update path — the carousel never mutates `_mediaItems` directly on success (only optimistically for reorder, reverting on error).

### State Isolation

`_mediaItems` is a local copy (mutable `List<MediaItem>`) initialized from `widget.getMediaItems()` in `initState`. This means:
- The carousel can optimistically mutate the list for reorder feedback.
- Parent widget rebuilds (unrelated to media) do not change the displayed items until `_refreshMediaItems()` is explicitly called.
- `widget.getMediaItems()` always reads the *current* `widget.mediaItems` from the parent closure (not a snapshot captured at open time), so it reflects the latest Riverpod state after a rebuild.

---

## Thumbnail vs Full Image Strategy

### When to Use Thumbnails

**List Views (Discover, Search):**
- Small display size (110x130px)
- Many images loaded simultaneously
- Bandwidth efficiency critical
- Use: `MediaRepository.getMediaUrl()` → thumbnail preferred

**Carousels and Galleries:**
- Medium display size (full width, scrollable)
- Multiple images in memory
- Fast loading more important than max quality
- Use: `MediaRepository.getMediaUrl()` → thumbnail preferred

### When to Use Full Images

**Detail View Backgrounds:**
- Large display area (full screen)
- Single image at a time
- Quality critical for user experience
- Use: `MediaRepository.getFullMediaUrl()` → always full

**Image Editing or Zooming:**
- User interaction with image
- Need full resolution for quality
- Use: `MediaRepository.getFullMediaUrl()` → always full

### Implementation

**GearViewModel Example ([gear_view_model.dart:175-247](../app/lib/presentation/viewmodels/gear_view_model.dart#L175-L247)):**

```dart
// Carousel: Load all media with thumbnails
Future<void> _loadAllMediaFromServer(List<String> mediaIds) async {
  for (final mediaId in mediaIds) {
    final mediaUrl = await mediaRepository.getMediaUrl(mediaId);  // Thumbnail
    allMedia.add(MediaItemData(id: mediaId, url: mediaUrl.url, ...));
  }
  state = state.copyWith(allMediaItems: allMedia);
}

// Background: Load first media at full resolution
Future<void> _loadMediaFromServer(String mediaId) async {
  final mediaUrl = await mediaRepository.getFullMediaUrl(mediaId);  // Full
  state = state.copyWith(mediaPath: mediaUrl.url, mediaId: mediaId);
}
```

### The four media URL methods — decision matrix

`MediaRepository` exposes four ways to resolve a media id to a renderable
URL, each paired with a Riverpod family provider. The right choice depends
on the surface size and on whether the media might be a video.

| Method / Provider | Image media | Video media | Use for |
|---|---|---|---|
| [`getMediaUrl`](../../app/lib/data/repositories/media_repository.dart#L43) / [`mediaUrlProvider`](../../app/lib/services/providers/media_providers.dart) | Thumbnail (or full if no thumb) | Poster JPEG, or **empty URL** if no thumb exists | Default. Avatars, list rows, dropdown items, anything <200px. |
| [`mediaObjectProvider`](../../app/lib/services/providers/media_providers.dart) | Same as `getMediaUrl` but also exposes `contentType` | Same | Same as above when the caller needs `contentType` (e.g. to show a play badge on a video thumbnail). |
| [`getHeroMediaUrl`](../../app/lib/data/repositories/media_repository.dart#L87) / [`heroMediaUrlProvider`](../../app/lib/services/providers/media_providers.dart) | **Full-resolution URL** | **Poster JPEG** (never the raw video URL) | Full-bleed background, community-hero / preview-hero surfaces. Sharp at large sizes; safe for video media (returns the poster, not the MP4). |
| [`getFullMediaUrl`](../../app/lib/data/repositories/media_repository.dart#L69) / [`fullMediaUrlProvider`](../../app/lib/services/providers/media_providers.dart) | **Full-resolution URL** | The **raw video URL** — `CachedNetworkImage` / `NetworkImage` can't decode it | Surfaces that are guaranteed image-only: fullscreen viewers, share previews, image-editing flows. |

#### Cache-key pairing

`MediaUrl.cacheKey` automatically selects between `media:thumb:{mediaId}`
and `media:full:{mediaId}` based on the `isThumbnail` flag the repo set.
Always pass it alongside `mediaUrl.url` when rendering through
`CachedMediaImage` / `CachedNetworkImage`:

```dart
final mediaUrl = ref.watch(heroMediaUrlProvider(mediaId)).value;
if (mediaUrl != null && mediaUrl.url.isNotEmpty) {
  return CachedMediaImage(
    imageUrl: mediaUrl.url,
    cacheKey: mediaUrl.cacheKey, // auto-picks thumb vs full key
    semanticsLabel: 'Hero image for ${community.name}',
  );
}
```

Mixing keys is dangerous: per [`docs/client/caching.md`](caching.md)
§ Image Cache Keys, sharing a cache key between a 400×400 thumbnail and
a 4000×4000 full-resolution image causes `CachedNetworkImage` to serve
the cached thumbnail when the full is requested, producing blurry
backgrounds. `MediaUrl.cacheKey` keeps the two namespaces distinct by
construction; just use it.

#### The video-hero bug class

Before #2064 several hero-shaped surfaces called `getFullMediaUrl` directly. For
image community media that worked. For video community media it
returned the raw MP4 URL, which then fed `NetworkImage` /
`CachedNetworkImage` — neither can decode video bytes, so the hero
rendered broken. Any hero surface that might show video media should
prefer `getHeroMediaUrl` / `heroMediaUrlProvider`.

---

## Server-Side Media Processing

### Thumbnail Generation

**File:** [server/storage/media.go:64-83](../../server/storage/media.go#L64-L83)

**Process:**

1. **Decode Image:**
   ```go
   img, _, err := image.Decode(bytes.NewReader(data))
   ```

2. **Resize with High-Quality Filter:**
   ```go
   thumbnail := imaging.Fit(img, 400, 400, imaging.Lanczos)
   ```

3. **Encode as JPEG:**
   ```go
   imaging.Encode(&buf, thumbnail, imaging.JPEG, imaging.JPEGQuality(85))
   ```

**Generation Criteria ([media.go:32-62](../../server/storage/media.go#L32-L62)):**

Thumbnails are generated **only if**:
- Content type is image (JPEG, PNG, WebP, HEIC)
- File size > 500 KB **OR** dimensions > 1000px

**Small/Low-Res Images:**
- No thumbnail generated
- `thumbnailUrl` field remains empty
- Client falls back to full URL

**Why These Thresholds:**
- Avoid generating thumbnails larger than original
- Skip processing for already-small images
- Save storage space (thumbnails cost money)
- 400px target covers most mobile screens

### Upload Flow

**File:** [server/storage/media.go:87-162](../../server/storage/media.go#L87-L162)

```
Client uploads image data
    ↓
StoreMedia(data, contentType, filename, ...)
    ↓
Insert metadata into SQL (generates media ID)
    ↓
Upload full image to bucket storage
    key: "users/{userId}/media/{mediaId}"
    ↓
shouldGenerateThumbnail(data, contentType)?
    ├─ Yes → Generate 400px JPEG thumbnail
    │        Upload to bucket storage
    │        key: "users/{userId}/media/{mediaId}_thumb"
    │        Update SQL with thumbnail storage URL
    │
    └─ No  → Skip thumbnail generation
    ↓
Return media ID
```

**Storage URLs:**
- Full image: `gs://bucket/users/{userId}/media/{mediaId}`
- Thumbnail: `gs://bucket/users/{userId}/media/{mediaId}_thumb`

**Presigned URLs:**
- Generated on-demand in `GetMedia` RPC
- Expiry: 35 minutes (`presignedURLExpiry` in `server/services/media/service.go`)
- Separate URLs for full and thumbnail

---

## Cache Key Naming Convention

### Repository-Level Keys (Metadata)

**Format:** `namespace:item_id` or `namespace:list_type:filter`

**Examples:**
- Media object: `media:media123`
- User object: `user:user456`
- Gear list: `gear:user:list`
- Loan list: `loan:received:gear:gear789`

**Implementation ([base_repository.dart:34-57](../app/lib/data/repositories/base_repository.dart#L34-L57)):**

```dart
// Single item
String _itemCacheKey(String id) => '$_namespace:$id';

// List with filters
String _listCacheKey(Map<String, dynamic> filters) {
  return '$_namespace:list:${_filtersToKey(filters)}';
}
```

### Image-Level Keys (Image Bytes)

**Standard Format:** `media_${mediaId}`

**Rules:**
- Always use stable media IDs (never URLs)
- Passed to `MediaBackground` via `mediaId` parameter
- Passed to `CachedMediaImage` via `cacheKey` parameter
- Persists across presigned URL refreshes

**Examples:**
```dart
// MediaBackground (automatic)
MediaBackground(
  mediaPath: url,
  mediaId: 'media123',  // → Generates 'media_media123'
)

// CachedMediaImage (explicit)
CachedMediaImage(
  imageUrl: url,
  cacheKey: 'media_media123',  // → Manual but same format
)
```

**Internal Cache Key:** `cache_media_media123` (flutter_cache_manager adds prefix)

**Why Stable IDs?**
- Presigned URLs expire 35 minutes after generation (`presignedURLExpiry`)
- Media IDs are permanent
- Cache survives URL regeneration
- Better cache hit rates across app restarts

**Important — stable keys are necessary but not sufficient:** they preserve already-cached bytes across URL rotation, but a *cold* cache key still requires a network fetch with a live URL. The carousel main view and its thumbnail strip use *different* cache keys (`media:full:{id}` vs. `media:thumb:{id}`), so the strip caching a thumbnail does nothing for the main view's first byte fetch. See the next section.

---

## Presigned URL Lifecycle and Closure Capture

**Context (#1833):** server-issued presigned URLs are valid for 35 minutes (`server/services/media/service.go:108`). The client metadata cache (`CACHE_TTL_MINUTES`, default 30 min in dev / 60 min in prod) holds `GetMediaResponse` for up to that TTL. The 5-minute headroom in dev is meant to absorb in-flight requests, **not** long-lived UI state.

**The failure mode that motivated this section:**

```
T+0   chat row builds, FutureBuilder resolves getMediaUrl(mediaId)
      → server issues URL valid until T+35
      → MediaItem(url: <URL>, mediaId: <id>) captured into tap closure
      → inline thumbnail bytes downloaded to flutter_cache_manager
        under 'media:thumb:{mediaId}'

T+25  user taps the photo
      → MediaCarousel.show(getMediaItems: () => [<captured MediaItem>])
      → Carousel main view: cache key 'media:full:{mediaId}' is cold,
        fetches from <captured URL>  → still valid → renders
      → Thumbnail strip: cache hit on 'media:thumb:{mediaId}' → renders

T+40  user backgrounds and reopens; closure still holds the same URL
      → main-view fetch under 'media:full:{mediaId}' → 403/expired
      → exclamation glyph (errorWidget)
      → strip still works because bytes were on disk
```

The Sentry-style telltale: the strip renders correctly while the main view shows the error glyph, on the same `MediaItem`, with the same URL — divergence is purely "cache hit vs. cache miss + expired network."

**Rules for any new media-rendering surface:**

1. **Never capture a presigned URL into a closure that may outlive a few minutes.** Tap handlers, `getMediaItems` closures, navigation arguments — anything that may be invoked an arbitrary time after construction — must re-resolve the URL at consumer-mount time, not pass through a captured one.
2. **Re-resolve via a Riverpod family provider keyed on `mediaId`.** Use `fullMediaUrlProvider(mediaId)` for full-resolution surfaces (carousel main view, fullscreen viewer) and `mediaObjectProvider(mediaId)` for thumbnail-preferred surfaces (carousel strip, list cards). Both are `FutureProvider.autoDispose.family` so the result drops when the consuming widget unmounts.
3. **Widgets never call `MediaRepository` directly.** The provider call lives in the ViewModel layer; widgets consume `AsyncValue<MediaUrl>` via `.when()`. This follows the MVVM rule in `docs/client/architecture.md`.
4. **Use `MediaUrl.cacheKey` from the repository return value.** Don't recompute `ImageCacheKeys.thumbnail(...)` / `.full(...)` at call sites — `mediaUrl.cacheKey` already picks the right one based on `isThumbnail` and is a single source of truth.
5. **Always wrap with `CachedMediaImage`, never raw `CachedNetworkImage`.** Enforced by `require_semantic_label_on_cached_media_image`. Pass an l10n-resolved `semanticsLabel`.

**Error recovery — invalidate, don't add a `forceRefresh` flag.** When a byte-fetch fails (most likely an expired URL slipping past the 5-min headroom), the recovery path is:

```dart
await ref.read(mediaRepositoryProvider).invalidate(mediaId);  // drop stale metadata
ref.invalidate(fullMediaUrlProvider(mediaId));                // re-trigger the future
```

The next provider read goes back through `MediaRepository.get`, which finds the cache empty and issues a fresh `GetMedia` RPC — guaranteed-fresh presigned URL. Cap automatic retries to 1 per provider instance to prevent loops on permanent errors, and expose a manual retry button (announced via `Semantics(liveRegion: true)`) for the user to escalate.

**Video-specific note.** `VideoFileCache` keys on `video_{mediaId}` and stores bytes on disk for 7 days. Once a video has played once, the URL is irrelevant — subsequent plays serve from disk. The expired-URL failure only bites on the *first* play after a URL expires, so the symptom is rarer than the image case but the resolution is identical (resolve a fresh URL right before `createCachedVideoController`).

**Things `MediaItem.url` is *not* safe for** (because it's a captured snapshot):
- Driving the main `CachedMediaImage` on a surface that may live longer than ~5 minutes.
- Rendering an image-shaped thumbnail of a video — `MediaItem.url` for video is the **video** URL, not an image. Use `mediaObjectProvider(mediaId)` instead.

**Things `MediaItem.url` *is* safe for:**
- A fallback when `MediaItem.mediaId` is absent (locally-staged uploads in flight).
- Video controller initialization on a captured URL when `mediaId` is also null (uploads in flight).
- Short-lived render passes that complete before the URL expires (the inline thumbnail in a chat row is fine — it pre-warms the disk cache, and on the next render the bytes are already local).

---

## Performance Characteristics

### Cache Hit Rates

**Expected Performance:**
- **Memory cache hit (Stash):** <1ms (instant)
- **Network fetch:** 100-500ms (depends on connection)
- **Image cache hit:** <10ms (DartFileCache disk read)
- **Image download:** 200-2000ms (depends on size and connection)

### Cache Effectiveness

**Thumbnail URLs (environment-configured TTL, default 30 min):**
- Hit rate: ~90% for active browsing session
- Miss causes: metadata-cache TTL expiry, app restart, cache eviction (LRU at 1000 entries)
- Mitigation: AccessedExpiryPolicy refreshes TTL on each access; if a downstream byte-fetch fails because the cached URL is past the 35-min presigned lifetime, the consumer invalidates the metadata entry and re-reads via `ref.invalidateSelf()` on its family provider (see [Presigned URL Lifecycle and Closure Capture](#presigned-url-lifecycle-and-closure-capture)).

**Image Bytes (no TTL, LRU eviction):**
- Hit rate: ~95% for recently viewed images
- Miss causes: LRU eviction (disk full), cache clear
- Mitigation: Stable cache keys preserve across URL refreshes

### Bandwidth Usage

**Discover Screen (20 items):**
- With thumbnails: ~20 × 50 KB = 1 MB
- Without thumbnails: ~20 × 500 KB = 10 MB
- **10x bandwidth savings**

**Gear Detail View (1 item):**
- Background: 500 KB (full resolution)
- Carousel (5 images): ~5 × 50 KB = 250 KB (thumbnails)
- Total: ~750 KB per gear view

---

## Common Patterns

### Pattern 1: FutureBuilder with ViewModel Helper (Recommended)

**Implementation Example:**

**ViewModel:** [discover_view_model.dart:234-235](../app/lib/presentation/viewmodels/discover_view_model.dart#L234-L235)
- Provides `getMediaUrl()` wrapper via [MediaHelpers](../app/lib/core/utils/media_helpers.dart)
- No media URL state needed in ViewModel
- MediaRepository handles all caching

**Widget:** [discover_thumbnail_item.dart:236-259](../app/lib/presentation/screens/discover/discover_thumbnail_item.dart#L236-L259)
- FutureBuilder loads media URLs on-demand
- MediaRepository cache provides sub-millisecond cache hits
- Automatic loading/error states

**Similar implementations:**
- [discover_request_card.dart:156-177](../app/lib/presentation/screens/discover/discover_request_card.dart#L156-L177)
- [inbox_request_card.dart:113-131](../app/lib/presentation/screens/inbox/inbox_request_card.dart#L113-L131)
- [community_preview_modal.dart:248-264](../app/lib/presentation/screens/communities/community_preview_modal.dart#L248-L264)

### Pattern 2: Load Full Image in Detail

**Implementation Example:**

**ViewModel:** [gear_view_model.dart:209-227](../app/lib/presentation/viewmodels/gear_view_model.dart#L209-L227)
- Uses `mediaRepository.getFullMediaUrl()` for full-resolution images
- Sets `mediaPath` in state for `MediaBackground` widget
- Used for detail views where quality matters

### Pattern 3: Preload Media for Smooth UX

**Implementation:**

All multi-media content types (gear, experience) use shared [MediaItemData](../app/lib/core/models/media_item_data.dart) class to preload media URLs during ViewModel initialization. The pattern:

- ViewModels store `List<MediaItemData> allMediaItems` in state with preloaded thumbnail URLs
- `_loadAllMediaFromServer()` fetches URLs via `MediaRepository.getMediaUrl()` for carousel display
- `_loadMediaFromServer()` fetches full-resolution URL via `MediaRepository.getFullMediaUrl()` for background
- Content views read from `state.allMediaItems` synchronously (no async in build methods)
- MediaRepository caches all fetched URLs for instant subsequent access

**Reference:** [gear_view_model.dart](../app/lib/presentation/viewmodels/gear_view_model.dart), [experience_view_model.dart](../app/lib/presentation/viewmodels/experience_view_model.dart)

### Pattern 4: Refresh Expired URLs

**Implementation:**

Media URLs automatically refresh via the repository caching layer. The StashCacheManager uses environment-configured TTL (default 30 minutes) with AccessedExpiryPolicy, which:
- Automatically fetches fresh URLs when cache expires
- Refreshes TTL on each access to keep frequently-used URLs cached
- No manual refresh needed in most cases

For manual refresh, call `mediaRepository.invalidate(mediaId)` to drop the cache entry and force a fresh fetch on the next read.

---

## Known Limitations

### 1. No Progressive Image Loading

**Issue:** Large images don't show low-res preview first.

**Impact:** Blank space while downloading full image.

**Workaround:** Use thumbnails for list views.

**Potential Fix:** Add BlurHash or progressive JPEG support.

### 2. Cache Size Not Configurable

**Issue:** Image cache grows unbounded until disk full.

**Impact:** May consume excessive storage on device.

**Workaround:** User can clear app data.

**Potential Fix:** Add max cache size setting in `CachedNetworkImage`.

### 3. No Offline Support for Media

**Issue:** Media URLs require network to refresh after expiry.

**Impact:** Images disappear after TTL expires offline (default 30 minutes, or until app restart).

**Workaround:** AccessedExpiryPolicy refreshes TTL on access, extending cache lifetime during active use.

**Potential Fix:** Store original media URLs (non-presigned) for offline use, or add disk persistence in Phase 2.

### 4. Video Thumbnails May Be Missing for Older Uploads

**Issue:** Videos uploaded before server-side ffmpeg thumbnail generation was stabilised may lack a thumbnail.

**Impact:** Those video items show a blank placeholder in carousels and feed cards instead of a preview still. The client guards against "Invalid image data" errors by returning an empty URL for videos without thumbnails (see `MediaRepository.getMediaUrl()`).

**Workaround:** None currently wired up. The server's `BackfillVideoThumbnails()` function existed for this but was deleted as dead code (#2643) since no reports of missing thumbnails ever surfaced; see [Video Thumbnail Backfill](#2-video-thumbnail-backfill) below.

**Status:** Server retries ffmpeg at 0s if the 1s extraction fails (handles short videos). Client shows a neutral placeholder for videos without thumbnails, and a play-icon overlay for videos that do have a thumbnail.

---

## Best Practices

### Do's

✅ **DO:**
- Use `MediaRepository.getMediaUrl()` for lists and thumbnails
- Use `MediaRepository.getFullMediaUrl()` for detail views and backgrounds
- Use `MediaBackground` for all full-screen immersive backgrounds
- Use `CachedMediaImage` for thumbnails, avatars, and inline images
- **Always pass `mediaId` to MediaBackground** (critical for caching)
- Use FutureBuilder pattern with ViewModel helper methods for on-demand media loading
- Provide `getMediaUrl()` wrapper in ViewModels via MediaHelpers utility
- Use stable cache keys (`'media_${mediaId}'`) not URLs
- Test cache hit/miss scenarios in repository tests
- Preload media for smooth navigation (when needed)
- Let FutureBuilder handle loading and error states automatically
- Use `mediaUrl.contentType?.startsWith('video/')` for video detection (not URL inspection)
- Wrap video backgrounds in `VideoBackgroundHost` — it owns the `VideoPlayerController` lifecycle via the widget's own `State.dispose()`
- Keep `isMuted` in ViewModel state as the source of truth; the host syncs it to the controller via `setVolume()` on each `didUpdateWidget`
- Gate the volume FAB on `state.isVideo` only — the host handles controller readiness internally

### Don'ts

❌ **DON'T:**
- Directly call `MediaService` from UI (use repositories)
- Use full URLs for list views (wastes bandwidth)
- Use URLs as cache keys (they expire)
- Use `CachedMediaImage` for full-screen backgrounds (use `MediaBackground`)
- Forget to pass `mediaId` to MediaBackground (cache optimization lost)
- Forget to handle empty `thumbnailUrl` (fall back to `url`)
- Skip error handling (network failures are common)
- Load all full-resolution images in lists (memory/bandwidth issue)
- Manually manage media URL state in ViewModels (use FutureBuilder instead)
- Call repositories directly from widgets (use ViewModel wrappers)
- Use URL heuristics to detect video content type (GCS presigned URLs are unreliable; use `contentType`)
- Store `VideoPlayerController` in ViewModel state OR as a private field on the Notifier — Riverpod's autoDispose timing races with Flutter's widget update pass and crashes `_VideoPlayerState.build` with `StateError: No active player with ID N`. Use `VideoBackgroundHost` so the controller lives in widget `State` and is disposed by Flutter's lifecycle
- Call `createCachedVideoController()` from a ViewModel — the host calls it internally
- Use `VideoPlayerController.networkUrl()` for server-loaded video — `createCachedVideoController()` (used by `VideoBackgroundHost`) caches to disk and avoids repeated HTTP range requests
- Access `VideoCacheManager` directly from ViewModels or widgets — go through `MediaRepository.getVideoFile()` (repository owns caching)

---


## Testing Media System

### Repository Tests

**File:** [media_repository_test.dart](../app/test/data/repositories/media_repository_test.dart)

**Test Coverage:**
- `getMediaUrl()` returns thumbnail when available, falls back to full URL
- `getFullMediaUrl()` always returns full-resolution URL
- Proper MediaUrl wrapper with correct `isThumbnail` flag
- Cache integration via BaseRepository

### Cache Tests

**File:** [stash_cache_manager_test.dart](../app/test/data/cache/stash_cache_manager_test.dart)

**Test Coverage:**
- Cache hit/miss scenarios
- TTL expiration behavior
- Pattern-based cache invalidation
- Environment-configured TTL settings
- AccessedExpiryPolicy refresh behavior

### ViewModel Tests

**Files:**
- [gear_view_model_test.dart](../app/test/presentation/viewmodels/gear_view_model_test.dart)
- [discover_view_model_test.dart](../app/test/presentation/viewmodels/discover_view_model_test.dart)

**Test Pattern:**
- Mock MediaRepository with Mockito
- Verify `getMediaUrl()` called for thumbnails (list views)
- Verify `getFullMediaUrl()` called for backgrounds (detail views)
- Assert state updates correctly with loaded URLs
- Test error handling and loading states

---


## Architecture Strengths

### ✅ Strengths

1. **Clear Separation of Concerns:** Repository handles data, ViewModels handle logic, widgets handle display
2. **Efficient Caching:** Multi-tier cache minimizes network requests
3. **Bandwidth Optimization:** Smart thumbnail/full image selection saves 90% bandwidth
4. **Stable Cache Keys:** Images persist across URL refreshes
5. **Type Safety:** Protobuf ensures consistent data models
6. **Reusable Components:** Shared widgets reduce code duplication
7. **Testable:** Repository and ViewModel patterns enable comprehensive testing
8. **Automatic URL Refresh:** Stale-while-revalidate prevents expired URLs
9. **MVVM Architecture:** ViewModels provide thin wrappers maintaining proper layering
10. **FutureBuilder Pattern:** On-demand loading with automatic state management eliminates manual caching code
11. **Full Video Support:** All content view backgrounds (Gear, Experience, Request, Community, Story) support looping video with mute control
12. **Widget-Owned Controller Lifecycle:** `VideoBackgroundHost` ties `VideoPlayerController` to widget `State.dispose()`, eliminating the autoDispose-vs-widget-update race that previously crashed feeds during community switches
13. **Video Caching:** Videos are cached on-device via `VideoCacheManager`, eliminating repeated HTTP range requests on replay and ViewModel recreation

## Architecture Weaknesses

### ⚠️ Weaknesses

1. **Complex Flow:** Many layers between server and display
2. **No Progressive Loading:** Blank space while images load
3. **Limited Offline Support:** Media disappears after 30 minutes offline (default TTL)
4. **Cache Size Unbounded:** May consume excessive storage
5. **Video Thumbnails May Be Missing for Old Uploads:** Videos without thumbnails show a blank placeholder; play-icon overlay distinguishes video items that have a thumbnail
6. **URL Expiry Complexity:** Requires careful TTL tuning

## Future Improvements

### 1. Progressive Image Loading

**Priority:** Medium

**Approach:** Generate BlurHash or low-res preview during upload

**Benefit:** Instant preview while full image loads

**Implementation:**
- Add `blurhash` field to Media proto
- Generate blurhash server-side
- Display blurhash in `CachedMediaImage` placeholder

### 2. Video Thumbnail Backfill

**Status:** Dropped. Thumbnails have been generated at upload since stable ffmpeg support landed (2026-03), so only videos older than that could lack one, and no reports ever surfaced. The `storage.BackfillVideoThumbnails` implementation sat unwired in `server/storage/media.go` until it was deleted as dead code (#2643); recover it from git history if a backfill is ever actually needed.

### 3. Lazy Loading Optimization

**Priority:** Low

**Approach:** Only load media URLs for visible items in scrollable lists

**Benefit:** Reduce initial network requests and memory usage

**Implementation:**
- Use visibility detectors or viewport-based loading
- Load media URLs only when items scroll into view
- Combine with FutureBuilder pattern for automatic caching

### 4. Offline Media Access

**Priority:** Low

**Approach:** Store non-presigned URLs for offline fallback

**Benefit:** Images persist longer offline

**Trade-off:** Requires authentication headers in image requests

### 5. Cache Size Limits

**Priority:** Low

**Approach:** Configure max cache size in `CachedNetworkImage`

**Benefit:** Prevent excessive storage usage

**Implementation:**
- Add `maxCacheSize` to app settings
- Configure `flutter_cache_manager` with limit

### 6. Media Analytics

**Priority:** Low

**Approach:** Track cache hit rates, load times, bandwidth usage

**Benefit:** Identify performance bottlenecks

**Implementation:**
- Add telemetry to `MediaRepository` and `CachedMediaImage`
- Send metrics to analytics service

### 7. Preloading Strategy

**Priority:** Low

**Approach:** Intelligent prefetching of likely-to-be-viewed media

**Benefit:** Smoother user experience with instant media display

**Implementation:**
- Prefetch next/previous items in carousels during idle time
- Prefetch detail view media when user hovers on list items
- Use heuristics to predict user navigation patterns

---

## Resources

### External Documentation

- **CachedNetworkImage:** https://pub.dev/packages/cached_network_image
- **Flutter Cache Manager:** https://pub.dev/packages/flutter_cache_manager
- **Presigned URLs (GCS):** https://cloud.google.com/storage/docs/access-control/signed-urls
- **EXIF Orientation:** https://www.impulseadventure.com/photo/exif-orientation.html

### Internal Files

**Client-Side:**
- [media_repository.dart](../app/lib/data/repositories/media_repository.dart) - Data access layer with caching
- [media_url.dart](../app/lib/data/repositories/media_url.dart) - URL wrapper
- [media_helpers.dart](../app/lib/core/utils/media_helpers.dart) - Shared utilities for ViewModels
- [stash_cache_manager.dart](../app/lib/data/cache/stash_cache_manager.dart) - Stash-based cache infrastructure
- [cache_service.dart](../app/lib/data/cache/cache_service.dart) - Namespace wrapper for cache
- [cached_media_image.dart](../app/lib/presentation/widgets/cached_media_image.dart) - Image widget
- [media_carousel.dart](../app/lib/presentation/widgets/media/media_carousel.dart) - Full-screen viewer
- [media_background.dart](../app/lib/presentation/widgets/media/media_background.dart) - Background display (image and video) — `StatelessWidget`, given a `VideoPlayerController?` to render
- [video_background_host.dart](../app/lib/presentation/widgets/media/video_background_host.dart) - Owns the `VideoPlayerController` lifecycle for content view backgrounds; wraps `MediaBackground`
- [video_mute_helper.dart](../app/lib/core/utils/video_mute_helper.dart) - Legacy mute toggle helper (used only by `community_edit_view_model.dart` and `gen_experience_view_model.dart`; the 5 main content ViewModels use `VideoBackgroundHost` instead)
- [video_cache_helper.dart](../app/lib/core/utils/video_cache_helper.dart) - Shared helper to create a cached, initialized VideoPlayerController
- [video_cache_manager.dart](../app/lib/data/cache/video_cache_manager.dart) - Disk-based video file cache (flutter_cache_manager)
- ViewModels ([discover_view_model.dart](../app/lib/presentation/viewmodels/discover_view_model.dart), [home_view_model.dart](../app/lib/presentation/viewmodels/home_view_model.dart)) - Thin wrappers for MVVM layering

**Server-Side:**
- [media.go](../server/storage/media.go) - Thumbnail generation & storage
- [media_service.proto](../proto/ripls/api/media_service.proto) - API contract
- [media.proto](../proto/ripls/models/media.proto) - Data model

**Related Docs:**
- [client_architecture.md](./client_architecture.md) - Overall MVVM architecture
- [client_caching.md](./client_caching.md) - Stash-based caching architecture
- [client_testing.md](./client_testing.md) - Testing patterns

---

## Appendix: Layer Breakdown

### 1. Data Layer: Media Repository

**File:** [app/lib/data/repositories/media_repository.dart](../app/lib/data/repositories/media_repository.dart)

**Purpose:** Centralized data access for media objects with transparent caching.

**Key Methods:**

```dart
// Get best available URL (thumbnail preferred)
Future<MediaUrl> getMediaUrl(String mediaId) async {
  final media = await getById(mediaId);
  return MediaUrl(
    url: media.thumbnailUrl.isNotEmpty ? media.thumbnailUrl : media.url,
    isThumbnail: media.thumbnailUrl.isNotEmpty,
    mediaId: mediaId,
  );
}

// Get full-resolution URL (for detail views)
Future<MediaUrl> getFullMediaUrl(String mediaId) async {
  final media = await getById(mediaId);
  return MediaUrl(
    url: media.url,
    isThumbnail: false,
    mediaId: mediaId,
  );
}
```

**Cache Policy:**
- **TTL:** Environment-configured (CACHE_TTL_MINUTES, default 30 minutes)
- **Persistent:** false (memory-only in Phase 1)
- **Eviction:** LRU (Least Recently Used)

**Why These Settings:**
- Global TTL ensures media URLs refresh before presigned URL expiration (35 minutes)
- Memory-only provides fast access without disk I/O
- LRU eviction keeps hot media in cache
- Phase 1 limitation: Repository-specified TTL (10 minutes) is ignored

**Inheritance:** Extends `BaseRepository<Media>` ([base_repository.dart:15-98](../app/lib/data/repositories/base_repository.dart#L15-L98))

---

### 2. Data Models: MediaUrl

**File:** [app/lib/data/repositories/media_url.dart](../app/lib/data/repositories/media_url.dart)

**Purpose:** Value object wrapping media URLs with metadata.

```dart
class MediaUrl {
  final String url;            // Presigned URL (thumbnail or full)
  final bool isThumbnail;      // Whether this is thumbnail or full image
  final String mediaId;        // Stable media ID for cache key
  final String? contentType;   // MIME type from GetMediaResponse (e.g. "video/mp4")

  String get cacheKey => isThumbnail
      ? ImageCacheKeys.thumbnail(mediaId)!
      : ImageCacheKeys.full(mediaId)!;
}
```

**Why This Design:**
- Stable `cacheKey` persists across URL refreshes (uses `ImageCacheKeys` utility to avoid collisions between thumbnail and full-resolution entries)
- `isThumbnail` flag enables logging and debugging
- `contentType` from the server is the authoritative source for video detection — URL heuristics are unreliable for presigned GCS URLs
- Encapsulates URL selection logic
- Type-safe wrapper prevents URL misuse

---

### 3. Service Layer: Media Service

**File:** [app/lib/services/media_service.dart](../app/lib/services/media_service.dart)

**Purpose:** gRPC client for media API (via Connect protocol).

**Key Operations:**
- `getMedia(mediaId)` - Fetch media metadata with presigned URLs
- `uploadMedia(data, contentType, ...)` - Upload new media
- `deleteMedia(mediaId)` - Delete media

**API Contract:** [proto/ripls/api/media_service.proto](../proto/ripls/api/media_service.proto)

```proto
message GetMediaResponse {
  string id = 1;
  string content_type = 2;
  string url = 3;              // Full-resolution presigned URL (35 min expiry)
  string thumbnail_url = 6;    // Thumbnail presigned URL (35 min expiry)
  string filename = 4;
  string description = 5;
}
```

**URL Generation (Server-Side):**
- Full image: Presigned URL to full-resolution object in GCS/S3
- Thumbnail: Presigned URL to 400px JPEG thumbnail (if exists)
- Expiry: 35 minutes (`presignedURLExpiry`)

---

### 4. Caching Infrastructure

#### StashCacheManager

**File:** [app/lib/data/cache/stash_cache_manager.dart](../app/lib/data/cache/stash_cache_manager.dart)

**Purpose:** Memory-based cache using Stash library with LRU eviction and environment-configured TTL.

**Architecture:**

```
┌─────────────────────────────────────────┐
│         StashCacheManager               │
│  ┌───────────────────────────────────┐  │
│  │   Memory Cache (Stash)            │  │  ← LRU eviction
│  │   - Max 1000 entries              │  │
│  │   - Environment-configured TTL    │  │
│  │   - AccessedExpiryPolicy          │  │
│  │     (refreshes on access)         │  │
│  └───────────────────────────────────┘  │
│              ↓ cache miss                │
│  ┌───────────────────────────────────┐  │
│  │   Network Fetch                   │  │  ← Fallback
│  │   - fetchFunction()               │  │
│  │   - Cache result on success       │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

**Cache Lookup Flow:**

1. **Memory Cache Check (Stash):**
   - Key exists and not expired? → Return immediately
   - Key missing or expired? → Continue to network

2. **Network Fetch:**
   - Call `fetchFunction(key)`
   - Cache result in memory with TTL
   - Return fetched value

**Key Features:**
- **Async-only:** No synchronous cache access (getSync returns null)
- **LRU Eviction:** Least recently used entries removed when max capacity reached
- **Environment-configured TTL:** Set via CACHE_TTL_MINUTES in env.*.json
- **Pattern Clearing:** Can invalidate by key pattern (e.g., 'media:*')
- **Auto-initialization:** Initializes on first use

**Cache Configuration:**
```dart
// Global TTL from environment
final ttlMinutes = int.tryParse(env.get('CACHE_TTL_MINUTES', '30')) ?? 30;
final ttl = Duration(minutes: ttlMinutes);

// Set to 0 to disable caching
if (ttlMinutes == 0) {
  // Always fetches fresh data
}
```

**Why This Pattern:**
- Battle-tested Stash library handles cache internals
- LRU eviction keeps hot media in memory
- Async-first eliminates UI blocking
- Environment-based TTL allows different policies per environment

#### Cache Policy

**File:** [app/lib/data/cache/cache_policy.dart](../app/lib/data/cache/cache_policy.dart)

**Purpose:** Defines cache behavior configuration (Phase 1: simplified for forward compatibility).

```dart
class CachePolicy {
  final Duration? ttl;       // Time to live (ignored in Phase 1)
  final bool persistent;     // Disk persistence (ignored in Phase 1)
}
```

**Phase 1 Behavior:**

StashCacheManager uses a **global environment-configured TTL** for all cached data:
- Environment variable: `CACHE_TTL_MINUTES` (default: 30)
- Repository-specified TTL values are **ignored**
- All data types share the same TTL

**Media Cache Policy Declaration:**
```dart
static const _defaultPolicy = CachePolicy(
  ttl: Duration(minutes: 10),  // Ignored in Phase 1
  persistent: false,           // Ignored in Phase 1
);
```

**Phase 2 Plans:**

Per-entry TTL support may be added with disk persistence. The intended target TTLs:

| Data Type | Target TTL | Rationale |
|-----------|------------|-----------|
| **Media URLs** | 10 min | Refresh before 35-minute presigned URL expiry |
| **Users** | 30 min | Stable data, infrequent changes |
| **Gear Items** | 30 min | Moderate staleness acceptable |
| **Loans** | 10 min | More frequent updates expected |

---

### 5. Presentation Layer

#### ViewModel-Based Media Loading

**Pattern:** ViewModels manage media URL loading with proper state management.

**Implementation Examples:**

**DiscoverViewModel** ([discover_view_model.dart](../app/lib/presentation/viewmodels/discover_view_model.dart)):
- **State** ([lines 14-27](../app/lib/presentation/viewmodels/discover_view_model.dart#L14-L27)): Includes `mediaUrls` map for caching
- **loadMediaUrl()** ([lines 235-257](../app/lib/presentation/viewmodels/discover_view_model.dart#L235-L257)): Fetches and caches media URLs
- **Usage**: [discover_gear_card.dart:118](../app/lib/presentation/screens/discover/discover_gear_card.dart#L118)

**GearViewModel** ([gear_view_model.dart](../app/lib/presentation/viewmodels/gear_view_model.dart)):
- **Full Images** ([lines 209-227](../app/lib/presentation/viewmodels/gear_view_model.dart#L209-L227)): `getFullMediaUrl()` for backgrounds
- **Carousel** ([lines 175-204](../app/lib/presentation/viewmodels/gear_view_model.dart#L175-L204)): Preloads multiple thumbnails

**Key Features:**
- Centralized state management in ViewModel
- Automatic caching prevents redundant fetches
- Testable logic separated from UI
- No widget-level state for media URLs
- Consistent pattern across all screens

---

#### CachedMediaImage Widget

**File:** [app/lib/presentation/widgets/cached_media_image.dart](../app/lib/presentation/widgets/cached_media_image.dart)

**Purpose:** Wrapper around `CachedNetworkImage` with app-specific defaults.

**Features:**
- Stable cache keys (media ID, not URL)
- Default placeholders and error widgets
- Zero fade duration (instant display from cache)
- Optional border radius

**Usage:**

```dart
CachedMediaImage(
  imageUrl: mediaUrl.url,
  cacheKey: 'media_${mediaId}',
  width: 110,
  height: 130,
  fit: BoxFit.cover,
  borderRadius: BorderRadius.circular(8),
)
```

**Internal Implementation ([cached_media_image.dart:32-52](../app/lib/presentation/widgets/cached_media_image.dart#L32-L52)):**

```dart
final image = CachedNetworkImage(
  imageUrl: imageUrl,
  cacheKey: cacheKey,        // Stable key (not URL!)
  fit: fit ?? BoxFit.cover,
  width: width,
  height: height,
  placeholder: (context, url) => placeholder ?? _buildDefaultPlaceholder(context),
  errorWidget: (context, url, error) => errorWidget ?? _buildDefaultErrorWidget(context),
  fadeInDuration: Duration.zero,  // Instant from cache
  fadeOutDuration: Duration.zero,
);
```

**Why Stable Cache Keys:**
- Presigned URLs change every 35 minutes
- `CachedNetworkImage` uses URL as default cache key
- Changing URLs would invalidate cache every 10 minutes
- Stable key (`media_mediaId`) persists across URL refreshes

**Disk Cache (DartFileCache):**
- Managed by `flutter_cache_manager` package
- Separate from HybridCacheManager (images vs metadata)
- Key format: `cache_media_mediaId`
- No TTL (evicted by LRU when disk full)

---

#### MediaCarousel Widget

**File:** [app/lib/presentation/widgets/media/media_carousel.dart](../app/lib/presentation/widgets/media/media_carousel.dart)

**Purpose:** Full-screen swipeable carousel for viewing all media.

**Key Features:**
- Horizontal `PageView` with media items
- Supports images and videos
- Thumbnail URLs for better performance
- Pinch-to-zoom with `PhotoView`

**Data Source:**
```dart
// GearViewModel loads all media
await _loadAllMediaFromServer(gear.mediaIds);

// Each media item
MediaItemData(
  id: mediaId,
  url: mediaUrl.url,        // Thumbnail URL preferred
  contentType: contentType,
  isVideo: isVideo,
)
```

**Image Rendering ([media_carousel.dart:263-278](../app/lib/presentation/widgets/media/media_carousel.dart#L263-L278)):**

```dart
PhotoView(
  imageProvider: CachedNetworkImageProvider(
    item.url,
    cacheKey: item.mediaId != null ? 'media_${item.mediaId}' : null,
  ),
  minScale: PhotoViewComputedScale.contained,
  maxScale: PhotoViewComputedScale.covered * 3,
  heroAttributes: PhotoViewHeroAttributes(tag: item.id),
)
```

**Why Thumbnails in Carousel:**
- Faster loading (400px vs 4000px)
- Lower bandwidth consumption
- Acceptable quality for mobile screens
- User can tap for full-resolution if needed (future feature)

---

#### MediaBackground Widget

**File:** [app/lib/presentation/widgets/media/media_background.dart](../app/lib/presentation/widgets/media/media_background.dart)

**Purpose:** Full-screen immersive backgrounds for content views (gear, request, community creation).

**Key Features:**
- Supports images and videos with automatic VideoPlayer integration
- Handles URLs and local file paths automatically
- No fade animations (`fadeInDuration: Duration.zero`)
- Stable cache keys via `mediaId` parameter
- Black background container with error handling

**Usage:**

```dart
MediaBackground(
  mediaPath: state.mediaPath,     // URL or file path
  mediaId: state.mediaId,         // CRITICAL: Always pass for optimal caching
  isVideo: state.isVideo,
  videoController: state.videoController,
)
```

**Cache Key Generation:**
- Automatically creates `'media_${mediaId}'` when `mediaId` provided
- Falls back to URL-based caching if `mediaId` is null
- Stable keys persist across presigned URL refreshes

**Video Support:**
```dart
// For video playback
MediaBackground(
  mediaPath: videoUrl,
  mediaId: mediaId,
  isVideo: true,
  videoController: videoController,  // Initialized VideoPlayerController
)
```

**Why Full Resolution:**
- Large display area (entire screen)
- Quality critical for immersive experience
- Single media item (not a list)
- User focused on content detail

---

**Last Updated:** 2026-04-30
**Architecture Version:** 2.6 (Riverpod 3.0 + MVVM + FutureBuilder Pattern with MediaHelpers + Video Support + Video Caching + Widget-Owned VideoPlayerController via VideoBackgroundHost)
