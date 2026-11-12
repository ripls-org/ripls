# Issue #811: Video not caching

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/811
**Filed:** 2026-03-09 | **Last activity:** 2026-03-09
**Labels:** none | **Assignees:** tescobar
**Context docs:** docs/client/media.md

## Problem Statement

The server is being hammered with HTTP 206 (range) requests for the same video
resources. Videos play via `VideoPlayerController.networkUrl()` which streams
directly from presigned GCS URLs with no local file caching. Every loop
iteration, seek, or ViewModel recreation causes fresh HTTP range requests to
the server.

Images are well-served by `CachedNetworkImage` + `flutter_cache_manager` with
stable cache keys, but there is no equivalent caching layer for video. The
`video_player` plugin has no built-in caching — it streams on every play.

One workaround already exists in `ConversationViewModel` (lines 770-822): it
downloads the entire video via `http.get()` to a temp file, then uses
`VideoPlayerController.file()`. This approach works but is isolated to one
ViewModel and not generalized.

## Context from Comments

No comments on the issue. The problem is straightforward from server logs
showing repeated 206 requests for the same media.

## Affected Code

8 locations create `VideoPlayerController.networkUrl()` without caching:

| ViewModel / Widget | File |
|--------------------|------|
| GearNotifier | `app/lib/presentation/viewmodels/gear_view_model.dart` |
| ExperienceNotifier | `app/lib/presentation/viewmodels/experience_view_model.dart` |
| CommunityNotifier | `app/lib/presentation/viewmodels/community_view_model.dart` |
| CommunityEditNotifier | `app/lib/presentation/viewmodels/community_edit_view_model.dart` |
| RequestNotifier | `app/lib/presentation/viewmodels/request_view_model.dart` |
| GenExperienceNotifier | `app/lib/presentation/viewmodels/gen_experience_view_model.dart` |
| MediaCarousel | `app/lib/presentation/widgets/media/media_carousel.dart` |
| ConversationNotifier | `app/lib/presentation/viewmodels/conversation_view_model.dart` (has workaround) |

## Potential Solutions

### Option A: Centralized video cache utility (download-to-file)

- **Approach:** Extract the ConversationViewModel workaround into a shared
  utility that downloads videos to a managed cache directory, keyed by media ID.
  All ViewModels call `VideoCacheManager.getFile(mediaId, url)` which returns a
  local `File`, then use `VideoPlayerController.file()`.
- **Pros:** Simple, proven pattern (already works in ConversationViewModel). No
  new packages. Full control over cache directory, eviction, and size limits.
  Works with presigned URLs since the file is downloaded once and reused.
- **Cons:** First play requires full download before playback starts (no
  progressive streaming). Large videos cause a visible loading delay.
- **Effort:** Medium.

### Option B: flutter_cache_manager with custom HttpFileService

- **Approach:** Use `flutter_cache_manager` (already a transitive dependency
  via `cached_network_image`) to manage video file caching. Create a
  `VideoCacheManager` singleton extending `CacheManager` with video-specific
  config (max cache size, stalePeriod). ViewModels call
  `cacheManager.getSingleFile(url, key: mediaId)` to get a cached `File`.
- **Pros:** Leverages existing dependency. Built-in LRU eviction, max size
  limits, and stale period management. Returns a `Stream<FileResponse>` that
  can show progress while downloading. Well-tested library.
- **Cons:** Still full download before playback (no streaming). Requires
  careful key management to avoid presigned URL collisions.
- **Effort:** Medium.

### Option C: video_player_cache or similar streaming cache package

- **Approach:** Add a package like `flutter_video_cacher` that provides a local
  HTTP proxy server, caching video bytes as they stream. The video player
  connects to `localhost:port` instead of the remote URL.
- **Pros:** Progressive playback — video starts immediately while caching.
  Best UX for large videos.
- **Cons:** Adds a new dependency with potential maintenance/compatibility
  risks. Local proxy server adds complexity. May have issues with presigned
  URLs that expire mid-stream. Less mature ecosystem.
- **Effort:** Medium-large.

## Recommendation

**Option B (flutter_cache_manager)** is the best balance of reliability,
simplicity, and capability:

1. It's already a transitive dependency — no new packages to add.
2. Built-in LRU eviction and max cache size prevent unbounded disk growth.
3. Cache keys based on media ID solve the presigned URL expiry problem.
4. The `getSingleFile()` API returns a `File` that works directly with
   `VideoPlayerController.file()`.
5. The `getFileStream()` API can provide download progress for a loading
   indicator if needed.

The download-before-play tradeoff is acceptable because:
- Most videos in the app are short background loops (a few seconds).
- After first download, subsequent plays are instant from cache.
- A loading indicator during first download is better than hammering the server.

## Open Questions

- **Q: What's the typical video file size?** — Recommendation: Check server
  logs or GCS bucket stats. If videos are consistently < 10MB, download-first
  is fine. If some are > 50MB, consider Option C for progressive playback.
- **Q: Should the cache survive app restarts?** — Recommendation: Yes. Use
  `flutter_cache_manager`'s default disk cache which persists across restarts.
  Set `stalePeriod` to 7 days to auto-evict old videos.
- **Q: Max cache size for videos?** — Recommendation: 200MB cap. Videos are
  larger than images, so a reasonable limit prevents excessive disk use.
- **Q: Should the ConversationViewModel workaround be migrated?** —
  Recommendation: Yes, replace it with the shared utility for consistency.

## Implementation Plan

### Phase 1: Video file cache in data layer

**Goal:** Add video file caching to the data layer, exposed through
MediaRepository (per architecture rule: repositories own caching, ViewModels
don't access cache managers directly).

<!-- AUDIT FIX: Original plan had ViewModels calling VideoCacheManager directly.
     Per client/architecture.md rule #1 ("ViewModels are the ONLY layer that
     calls Repositories") and rule #2 ("Repositories are the ONLY layer that
     calls Services"), video file caching must be accessed through
     MediaRepository, not a standalone cache manager. -->

**Tasks:**
- [ ] Create `app/lib/data/cache/video_cache_manager.dart` — singleton
  `VideoCacheManager` extending `flutter_cache_manager`'s `CacheManager` with
  config: max 200MB, stalePeriod 7 days, key `'videoCache'`. This is a
  **file-level disk cache** (like `CachedNetworkImage`'s cache for image bytes),
  separate from the Stash-based metadata cache
- [ ] Add `getVideoFile(String mediaId, String url)` to `MediaRepository` that
  uses `VideoCacheManager` internally, keyed by `'video_$mediaId'` (stable ID,
  not the expiring presigned URL). Returns a local `File`
- [ ] Add `evictVideo(String mediaId)` to `MediaRepository` for manual cache
  clearing
- [ ] Wire `VideoCacheManager` into `MediaRepository` via Riverpod provider
  (same pattern as `CacheService` injection)
- [ ] Write unit tests for `MediaRepository.getVideoFile()` — cache hit/miss
  scenarios, stable key generation, eviction

**Validates:** Video files are cached on disk, keyed by stable media ID.
Repository owns the caching logic.

### Phase 2: Integrate into ViewModels via MediaRepository

**Goal:** Replace all `VideoPlayerController.networkUrl()` calls with cached
file playback, accessed through the repository layer.

<!-- AUDIT FIX: Original plan had a helper method that accessed VideoCacheManager
     directly. Updated to go through MediaRepository, consistent with how image
     loading already works (ViewModels call mediaRepository.getFullMediaUrl(),
     not CachedNetworkImage's cache directly). -->

**Tasks:**
- [ ] Create a shared helper in `video_cache_helper.dart`:
  `Future<VideoPlayerController> createCachedVideoController(MediaRepository repo, String mediaId, String url)`
  that calls `repo.getVideoFile(mediaId, url)`, creates
  `VideoPlayerController.file()`, initializes, sets looping + muted defaults
- [ ] Update `GearNotifier._loadMediaFromServer()` to use cached controller
- [ ] Update `ExperienceNotifier._loadMediaFromServer()` to use cached controller
- [ ] Update `CommunityNotifier._loadMediaFromServer()` to use cached controller
- [ ] Update `CommunityEditNotifier._loadMediaFromServer()` to use cached controller
- [ ] Update `RequestNotifier._loadMediaFromServer()` to use cached controller
- [ ] Update `GenExperienceNotifier` preview video to use cached controller
- [ ] Update `MediaCarousel._initializeVideo()` to use cached controller
- [ ] Replace ConversationViewModel's manual temp-file workaround with the
  shared helper (removes ad-hoc `http.get()` + temp file management)
- [ ] Add loading indicator in MediaBackground while video downloads on first play

**Validates:** Server 206 requests drop dramatically. Videos play from cache on
repeat views. All content types (gear, experience, request, community) benefit.

### Phase 3: Cache management and cleanup

**Goal:** Ensure video cache is well-behaved on user devices.

**Tasks:**
- [ ] Add video cache size to any existing storage/settings diagnostics
- [ ] Add cache clear option if the app has a settings/storage screen
- [ ] Test cache behavior across app restart, URL expiry, and LRU eviction
- [ ] Monitor server logs to verify 206 request reduction

**Validates:** Cache doesn't grow unbounded. Users can manage storage. Server
load is reduced.

## Audit Notes

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/server/architecture.md`

**Issues found and fixed:**

1. **Repository layer bypass (architecture.md rules #1 and #2):** The original
   plan had ViewModels calling `VideoCacheManager` directly. Per architecture
   rules, ViewModels access data only through repositories, and repositories
   own all caching. Fixed: video file caching is now accessed through
   `MediaRepository.getVideoFile()`, with `VideoCacheManager` as an internal
   implementation detail of the data layer.

2. **Cache system distinction (caching.md "Image Cache Keys" section):** The
   plan correctly identifies `flutter_cache_manager` for video byte caching
   (disk-based, file-level) vs Stash for metadata caching (memory-based,
   object-level). Added explicit callout that `VideoCacheManager` is a
   file-level disk cache, analogous to how `CachedNetworkImage` handles image
   bytes — not a replacement for or addition to `StashCacheManager`.

3. **No server-side changes needed:** Confirmed this is purely a client-side
   caching issue. The server already serves video via presigned URLs correctly.
   No changes to `server/storage/media.go` or any RPC service required.

**No issues with:**
- Cache key naming convention (`video_$mediaId`) follows established pattern
- Riverpod provider wiring matches existing repository injection pattern
- Helper utility pattern matches existing `video_mute_helper.dart` approach
- Test strategy aligns with repository test patterns in `caching.md`

## Related Issues

- None referenced.
