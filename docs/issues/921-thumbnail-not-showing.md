# Issue #921: [Bug Report] image thumbnail does not show

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/921
**Filed:** 2026-03-27 | **Last activity:** 2026-03-27
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** docs/client/media.md

## Problem Statement

In the gear content view, thumbnail images in the Details tab carousel (`ContentMediaCarousel`) fail to display, while the full-resolution background image (the "media screen") renders correctly. The user sees an empty placeholder or error state instead of the 72×72 thumbnail strips that should appear when switching to the Details tab.

This is a presigned URL expiry / stash cache TTL mismatch. The server generates GCS presigned URLs that expire after **15 minutes**, but the `GetMediaResponse` containing those URLs is cached in the stash metadata cache for **30 minutes** (the default `CACHE_TTL_MINUTES`). During the 15–30 minute window, the stash cache returns a valid-looking response with expired presigned URLs. When `CachedNetworkImage` can't find the image bytes in its disk cache, it tries to download from the expired URL, gets a 403, and shows the error widget.

The full-resolution background (`MediaBackground`, key `media_$mediaId`) is more likely to be in the `CachedNetworkImage` disk cache—it loads on every content view render—so it usually survives the stale-URL window. The thumbnail strip (key `thumbnail_$mediaId`) is a separate `CachedNetworkImage` entry that only populates when the user opens the Details tab, making it more likely to be absent from disk cache and thus exposed to the stale-URL failure.

**Existing partial mitigation:** `FeedNotifier.refresh()` (and `DiscoverNotifier.refresh()`) already clears `media:*` from stash on pull-to-refresh, forcing fresh presigned URLs to be fetched. This means the bug only manifests when the user does *not* refresh — i.e., during normal scrolling in an active session. The fix must address the non-refresh case.

## Context from Comments

No comments. The report was filed by user10@example.org via the in-app feedback system on iOS (v0.8.0, DEV environment). No screenshots are interpretable (GCS feedback-system URLs), but the text description is unambiguous.

## Potential Solutions

### Option A: Reduce the stash TTL for `MediaRepository` below presigned URL expiry

Add a `MediaRepository`-specific short TTL (e.g. 10 minutes) so the stash entry expires before the presigned URL does. On re-access, a fresh `GetMedia` RPC is made, returning fresh presigned URLs.

- **Approach:** Override the TTL used for `media:*` keys in `CacheManager`/`CacheService`. This requires either (a) a per-call TTL on `CacheService.get()`, or (b) a separate `CacheManager` instance for `MediaRepository` configured with a shorter TTL.
- **Pros:** Fixes the root cause; no behavioral change for callers; presigned URLs are always fresher than their expiry.
- **Cons:** Requires extending the `CacheManager` interface to accept per-entry TTL, touching `StashCacheManager` and `HybridCacheManager`; more infrastructure churn than the problem warrants.
- **Effort:** Medium.

### Option B: Extend presigned URL expiry on the server to match cache TTL

Change the server's `GetSignedURL` call from `15*time.Minute` to `35*time.Minute` (a little more than the 30-min cache TTL).

- **Approach:** One-line change in `server/services/media/service.go` (two lines: `url` and `thumbnailUrl`).
- **Pros:** Simplest code change; no client-side infrastructure changes; fixes the mismatch at the source.
- **Cons:** Wider URL validity window is a minor security tradeoff (a leaked URL stays valid longer). Acceptable for a non-sensitive media use case. Doesn't fix the conceptual mismatch—if `CACHE_TTL_MINUTES` is later raised, the bug reappears.
- **Effort:** Small.

### Option C: Invalidate the media stash cache entry after presigned URL expiry (proactive refresh)

Track when the cached `GetMediaResponse` was stored, and force-invalidate the stash entry after 12 minutes regardless of the global TTL. On next access a fresh RPC is made.

- **Approach:** Add a `cacheTimestamp` field or use the stash entry's stored-at time. Compare elapsed time in `MediaRepository.get()` before returning.
- **Pros:** Correct at the data layer; doesn't require server changes.
- **Cons:** Considerably more complex; the stash library may not expose entry timestamps cleanly; requires new logic in `MediaRepository`.
- **Effort:** Large.

## Recommendation

**Option B** — extend the server presigned URL expiry to 35 minutes.

This is the simplest, most targeted fix: one two-line change on the server aligns the URL validity period with the cache TTL. The security tradeoff (URL stays valid 35 min vs 15 min) is negligible for user media in a controlled-access app. It also documents the relationship between the two constants, so future changes to `CACHE_TTL_MINUTES` remain visible near the relevant server code.

After shipping Option B, add a code comment next to the presigned URL expiry in `service.go` noting that it must be greater than the client-side `CACHE_TTL_MINUTES`. If a longer-term architectural change is wanted (per-entry TTL on the cache manager), Option A can be pursued separately without urgency.

## Decisions

- **Presigned URL expiry and cache TTL must be explicitly linked.** Use a named constant `presignedURLExpiry = 35 * time.Minute` in `service.go` with a comment noting it must exceed `CACHE_TTL_MINUTES` (default 30).
- **35 minutes is the correct expiry.** 30-min cache TTL + 5-min buffer.
- **All content types are affected.** Experiences, requests, and communities use the same stash-cached `GetMediaResponse` path. Phase 1 fixes the server for all; Phase 2 covers the client-side parallel loading for all three view models.

## Implementation Plan

### Phase 1: Server — extend presigned URL expiry
**Goal:** Eliminate the stale-URL window so cached `GetMediaResponse` always contains valid URLs.

**Tasks:**
- [ ] In `server/services/media/service.go`, change the `GetSignedURL` expiry from `15*time.Minute` to `35*time.Minute` for both the full URL (line ~106) and the thumbnail URL (line ~123).
- [ ] Add a comment above the constant explaining the client cache TTL dependency.
- [ ] Run `npm run build` to verify the server compiles and passes lint.
- [ ] Run existing media service tests (`go test ./server/services/media/...`) to verify no regressions.

**Validates:** Presigned URLs are now valid for longer than the stash cache TTL, so cached responses always carry fresh-enough URLs.

### Phase 2: Client — parallelize media loading in `_loadAllMediaFromServer`
**Goal:** Fix a pre-existing N+1 sequential-await loop that violates the parallel-loading pattern in caching docs (Pattern 4) and compounds the stale-URL window by stretching the total load time.

**Tasks:**
- [ ] In `gear_view_model.dart`, convert `_loadAllMediaFromServer`'s sequential `for (final mediaId in mediaIds) { await ... }` loop to a single `Future.wait(mediaIds.map(...))` call. Each item should catch its own error and return `null` on failure, with the null entries filtered before calling `safeUpdateState`.
- [ ] Apply the same parallel-loading fix to `_loadAllMediaFromServer` (or equivalent) in `experience_view_model.dart` and `request_view_model.dart`.
- [ ] In `_MediaThumbnailCell` (`content_shared_widgets.dart:469`), add an inline comment confirming `cacheKey: 'thumbnail_$mediaId'` is intentional — a separate `CachedNetworkImage` disk cache entry from the full-image key `media_$mediaId` used by `MediaBackground`. The distinction is architecturally correct, just undocumented.

**Validates:** Media items in the carousel load in parallel rather than serially; total load time drops proportionally; the existing cache key convention is documented.

### Phase 3: Observability — add debug logging for stale URL detection (optional)
**Goal:** Make future URL expiry issues easier to diagnose.

**Tasks:**
- [ ] In `_loadAllMediaFromServer`, add a debug-level log entry when a thumbnail URL is empty (indicating no thumbnail was generated), so it can be distinguished from a load failure in crash logs.
- [ ] Verify existing `_log.warning` in the catch block is sufficient for failures.

**Validates:** Future media loading failures produce actionable log output.

## Related Issues

- #811 — Video caching (same media repository, different cache path for video bytes)
- #847 — Video thumbnails (thumbnail generation on server side)
