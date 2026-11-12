# Issue #847: Need to generate thumbnails for videos

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/847
**Filed:** 2026-03-15 | **Last activity:** 2026-03-15
**Labels:** none | **Assignees:** tescobar
**Context docs:** docs/client/media.md

## Problem Statement

Video items in the Discover feed (and likely other list views) throw "Invalid image data" exceptions. The root cause is a two-part failure:

1. **Server**: Video thumbnail generation via ffmpeg exists (`generateVideoThumbnail()` in `server/storage/media.go`) but failures are intentionally non-fatal — the upload succeeds with an empty `ThumbnailStorageUrl`. This means some videos in production have no thumbnail.

2. **Client**: When no thumbnail exists, `MediaRepository.getMediaUrl()` falls back to the full URL. For videos, this means `CachedNetworkImage` downloads video bytes and attempts to decode them as an image, producing "Exception: Invalid image data" for every affected item.

The result is a cascade of errors on the Discover screen whenever video items appear without thumbnails.

## Context from Comments

No comments on this issue. The error logs in the description are clear enough to diagnose — multiple item IDs failing with the same `ImageDescriptor.encoded` → `Invalid image data` stack trace.

## Potential Solutions

### Option A: Client-side guard + server reliability improvements

- **Approach:** Fix the client to never attempt image decoding on video URLs without thumbnails (show a placeholder instead). Separately, improve server-side ffmpeg reliability so thumbnails are generated more consistently.
- **Pros:** Fixes the immediate crashes. Defensive client code prevents future regressions. Server improvements reduce the frequency of missing thumbnails.
- **Cons:** Two-layer fix touches both client and server.
- **Effort:** Medium.

### Option B: Client-side only — generate thumbnails on-device

- **Approach:** When the client detects a video with no thumbnail, download the video and extract a frame locally using a package like `video_thumbnail`.
- **Pros:** No server changes needed.
- **Cons:** Expensive on-device operation (downloading full video just for a thumbnail), poor UX (slow), battery drain, doesn't fix the root cause. Every client re-does the work.
- **Effort:** Medium, but wasteful.

### Option C: Server-side only — make thumbnail generation mandatory for videos

- **Approach:** Make video upload fail if ffmpeg thumbnail generation fails, forcing a retry or rejecting the upload.
- **Pros:** Guarantees every video has a thumbnail.
- **Cons:** Breaks uploads when ffmpeg has transient failures. Doesn't fix existing videos without thumbnails. Doesn't protect the client from future edge cases.
- **Effort:** Small, but fragile.

## Recommendation

**Option A** — fix both sides. The client must be resilient to missing thumbnails regardless of server improvements (defense in depth). The server should also be more reliable at generating thumbnails so the placeholder is rarely seen.

## Open Questions

- **Are there existing videos in production without thumbnails?** — Recommendation: Yes, almost certainly (the logs show multiple affected items). Phase 1 should include a backfill mechanism or at minimum handle existing data gracefully.
- **Should we add a "video" badge/icon overlay on video thumbnails in list views?** — Recommendation: Yes, a small play icon overlay would improve UX by signaling video content before tap. Defer to Phase 3.
- **Should ffmpeg failure block video upload?** — Recommendation: No. Keep it non-fatal but add better logging/alerting so we notice when it fails.

## Implementation Plan

### Phase 1: Client-side guard — prevent "Invalid image data" crashes
**Goal:** Stop the error cascade immediately. Video items without thumbnails show a graceful placeholder instead of crashing.
**Tasks:**
- [ ] In `MediaRepository.getMediaUrl()`, check `contentType`. If the content type starts with `video/` and `thumbnailUrl` is empty, return a `MediaUrl` with an empty URL (not the full video URL). This keeps the decision in the repository layer where it belongs — ViewModels and widgets should not need to know about video/image content type logic for thumbnail selection. [AUDIT: Repository is the correct layer per architecture.md rule "Repositories own caching - ViewModels don't" and media.md data flow]
- [ ] In `CachedMediaImage`, handle the case where `imageUrl` is empty — show the default placeholder widget instead of attempting a network load. This is purely a UI-level guard (display a placeholder), not business logic. [AUDIT: Widget-level placeholder display is acceptable per architecture.md UI Structure rules]
- [ ] In `DiscoverViewModel` (and similar VMs that resolve thumbnails), handle the "no displayable image" case gracefully — log at debug level, not warning. ViewModels should call `MediaRepository.getMediaUrl()` as they already do; no new service or repository calls needed. [AUDIT: Compliant — VMs call repositories, not services directly]
- [ ] Add unit tests for `MediaRepository.getMediaUrl()` when content type is video and thumbnail is missing — mock `MediaService` with Mockito per testing patterns in `client_testing.md`
- [ ] Add widget test for `CachedMediaImage` with empty URL
**Validates:** No more "Invalid image data" errors in logs when video items appear in feeds.

### Phase 2: Server-side reliability — improve ffmpeg thumbnail generation
**Goal:** Reduce the frequency of missing video thumbnails so the placeholder from Phase 1 is rarely seen.
**Tasks:**
- [ ] Add structured logging to `generateVideoThumbnail()` using `logging.LoggerWithContext(ctx)` — log the ffmpeg command, exit code, and stderr on failure. [AUDIT: Must use structured logging per CLAUDE.md — never `log.Printf` or `fmt.Printf`]
- [ ] Add a retry (1 attempt) for ffmpeg thumbnail generation with a different timestamp (e.g., 0s instead of 1s) in case the video is shorter than 1 second
- [ ] Add a backfill mechanism for existing videos that have `ThumbnailStorageUrl` empty and `ContentType` starting with `video/`. Use `storage.ProtoSQLStorage` query methods (e.g., `QueryByField`) to find affected media records — do not write raw SQL in service code. [AUDIT: Compliant with server architecture.md storage layer rules]
- [ ] Add unit test for `generateVideoThumbnail()` with a short test video — use table-driven tests per server testing conventions
**Validates:** ffmpeg failures are logged with enough detail to diagnose, and retry handles the common "video too short" edge case.

### Phase 3: Client-side polish — video indicator overlay
**Goal:** Users can distinguish video items from image items in list views.
**Tasks:**
- [ ] Add a small play icon overlay to `CachedMediaImage` (or a wrapper) when `contentType` indicates video. Any overlay colors or backgrounds must use `AppColors` helpers for theme-awareness (light/dark mode). [AUDIT: Required per architecture.md Colors and Theming rules — no hardcoded colors]
- [ ] Apply the overlay in Discover gear grid, event carousel, and other list views that display mixed media. The `contentType` information flows through `MediaUrl` from the repository — widgets read it from ViewModel state, not from repositories directly. [AUDIT: Compliant with architecture.md rule "Screens only interact with ViewModels"]
- [ ] Update `docs/client/media.md` Known Limitations section — remove item #4 (no server-side video thumbnails) and add notes about the new behavior
**Validates:** Video items are visually distinct in feeds, and the media documentation reflects current state.

### Audit Summary
**Reviewed against:** `docs/client/architecture.md`, `docs/client/caching.md`, `docs/server/architecture.md`
**Findings:**
- All client changes route through the correct layers (Repository for data logic, ViewModel for state, Widget for display)
- No manual cache maps introduced in ViewModels — relies on existing `MediaRepository` caching
- Server changes stay within the storage library and use structured logging
- Image cache key strategy (`ImageCacheKeys.thumbnail()` / `.full()`) is preserved — no new cache key patterns needed
- Theme-aware colors required for any new UI elements (Phase 3 overlay)

## Related Issues
- #811 — Video caching (completed, established `VideoCacheManager` and `MediaRepository.getVideoFile()`)
- #821 — Story video background (completed, established video detection pattern via `contentType`)
