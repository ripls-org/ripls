# Issue #1833: Viewing photo gives error

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1833
**Filed:** 2026-05-10 | **Last activity:** 2026-05-10
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none
**Audit pass:** 2026-05-13 (see "Audit Updates" below)

## Audit Updates (2026-05-13)

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/server/architecture.md`, and
`docs/server/observability.md`. Material changes:

1. **MVVM compliance.** Initial draft had `CarouselImageViewer` (a widget)
   reading `MediaRepository` via `FutureBuilder`. That violates the rule
   "ViewModels are the ONLY layer that calls Repositories. Screens/Widgets
   never access repositories directly"
   (`docs/client/architecture.md:47–51`). Phase 1 is rewritten to introduce a
   small `AsyncNotifier`/`FutureProvider.family` keyed by `mediaId` so the
   viewer widget consumes `AsyncValue<MediaUrl>` and the repository call
   stays in the data layer.
2. **Drop the new `forceRefresh` API.** Initial draft added a
   `forceRefresh: true` parameter to `MediaRepository.getFullMediaUrl`. The
   documented pattern for forcing a refetch is `repository.invalidate(key)`
   then a normal call (`docs/client/caching.md:107–121`). No new API
   surface; the error-retry path invalidates first and then calls
   `ref.invalidateSelf()` on the family provider.
3. **Reuse `MediaUrl.cacheKey`, don't recompute.** Initial draft passed
   `ImageCacheKeys.full(item.mediaId)` directly to `CachedNetworkImage`.
   The repository already returns `MediaUrl.cacheKey`
   (`app/lib/data/repositories/media_url.dart:39`), which is the single
   source of truth recommended by `docs/client/caching.md:347–352`. The
   viewer reads `cacheKey` off the resolved `MediaUrl`.
4. **Use `CachedMediaImage`, not raw `CachedNetworkImage`.**
   `docs/client/accessibility.md:39` enforces
   `require_semantic_label_on_cached_media_image`, and CLAUDE.md states
   "Always use CachedMediaImage for user-uploaded media." The carousel main
   view today uses raw `CachedNetworkImage` (`carousel_image_viewer.dart:41`)
   — migrating during this fix removes one source of drift. Same for
   `FullscreenImageViewer` and `CarouselThumbnailStrip`'s inner image.
5. **AsyncValue.when() over FutureBuilder.** Per
   `docs/client/architecture.md:670–710`, `AsyncNotifier` + `.when()` is
   the recommended async pattern. Replaces the draft's bare `FutureBuilder`.
6. **i18n for any new visible string.** No new ARB keys are required as long
   as we keep the existing error glyph behaviour. If the error path adds a
   user-visible label (e.g. a "Retry" button, a screen-reader announcement),
   it goes in `app_en.arb`/`app_es.arb` with an `a11y*` prefix
   (`docs/client/i18n.md:23–28`). The plan now calls out the
   `a11yMediaLoadError` / `a11yMediaRetry` keys explicitly.
7. **Accessibility announcement on error.** The error glyph currently has
   no semantics label. `docs/client/accessibility.md` recommends a
   `LiveRegion` or `SemanticAnnouncer.announce` for status content that
   appears in place; Phase 1 adds a `Semantics(label: ..., liveRegion: true)`
   wrapper for the error state.
8. **Test layering.** Initial draft proposed a widget test for
   `CarouselImageViewer`. Per `docs/client/testing.md:9–11`, the primary
   focus is ViewModels with `ProviderContainer` + Mockito. Phase 3 is
   rewritten around testing the new provider/notifier with a mocked
   `MediaRepository`, plus a thin widget test that the viewer renders
   the loading / data / error branches of `AsyncValue`.
9. **No server-side changes needed.** Confirmed the bug is fully resolvable
   client-side; server `presignedURLExpiry` (35 min) is correct against the
   30-min metadata cache TTL. The earlier "Option C: tighten headroom"
   remains discarded.

## Problem Statement

A user (Eric Dreher, iOS 26.3.1, app v0.19.2 PROD) tapped a photo attached to
a chat message and the carousel main view rendered the error glyph
(`Icons.error_outline`) instead of the image. The screenshot shows the
thumbnail strip at the bottom of the carousel rendering the same photo
correctly while the main viewport above shows the exclamation icon — i.e. the
thumbnail's bytes are on disk, but the main view's `CachedNetworkImage` failed
to load from the network.

The chat message timestamp in the screenshot reads "1h ago." That matters,
because server-side presigned URLs in `server/services/media/service.go:108`
have a 35-minute lifetime — well under the elapsed time since upload, and
right at the edge of (or past) the 30-minute client metadata cache TTL.

There are no comments on the issue. Reproduction is the photo-and-screenshot
that was filed via the in-app feedback flow.

## Context from Comments

None — the issue was filed via the feedback widget and has no follow-up.
That's a minor flag: there is no clarification on whether the bug is
intermittent, whether retrying via pull-to-refresh fixes it, or whether other
users have reproduced. Best to treat the screenshot as the full evidence and
infer mechanism from code.

## Code Path Walkthrough

1. **Chat row renders.** `MessageMediaAttachments`
   (`app/lib/presentation/widgets/chat/message_list/message_media_attachments.dart:37`)
   builds a `FutureBuilder` on `mediaRepositoryProvider.getMediaUrl(mediaId)`.
   That call hits `MediaRepository.getMediaUrl`
   (`app/lib/data/repositories/media_repository.dart:43`), which fetches
   `GetMediaResponse` through the in-memory metadata cache (30-min TTL via
   `CACHE_TTL_MINUTES`) and returns a `MediaUrl` whose `.url` is the
   **thumbnail** URL if present, otherwise the full URL.
2. **Inline thumbnail loads.** A `CachedNetworkImage` renders the thumbnail
   (line 81) with `cacheKey: ImageCacheKeys.thumbnail(mediaId)`
   (`media:thumb:{mediaId}`). On success the bytes are stored on disk by
   `flutter_cache_manager` under that key.
3. **User taps.** The tap handler calls `MediaCarousel.show` with
   `getMediaItems: () => [MediaItem(url: mediaUrl.url, …, mediaId: mediaId)]`
   (line 67). The closure captures the **`mediaUrl.url` from the
   `FutureBuilder` snapshot**, i.e. the URL that was returned to the chat
   when the row last built — not a freshly-fetched URL at tap time.
4. **Carousel mounts.** `MediaCarousel._buildMediaItem`
   (`app/lib/presentation/widgets/media/media_carousel.dart:357`) renders
   `CarouselImageViewer` with `item.url` (still the thumbnail URL) and the
   thumbnail strip with the same `item.url` value
   (`_mediaItemsAsData()` at line 389).
5. **Two paths diverge:**
   - **Thumbnail strip** uses `cacheKey: ImageCacheKeys.thumbnail(mediaId)`
     (`carousel_thumbnail_strip.dart:167`). Disk cache **hit** from step 2,
     so the bytes display without a network call. ✅
   - **Main view** uses `cacheKey: ImageCacheKeys.full(item.mediaId)`
     (`carousel_image_viewer.dart:43`), `media:full:{mediaId}`. Disk cache
     **miss**, so `CachedNetworkImage` fetches `item.url`. If the URL is
     expired (or returns a 403 from GCS), the load fails and the `errorWidget`
     renders the exclamation icon. ❌

The screenshot is precisely the state at step 5b. Both behaviors are reachable
with the same code, the same `MediaItem`, the same URL — divergence is purely
"cache hit vs. cache miss + network."

## Why the URL is likely expired

Server presigned URL TTL is **35 minutes** (`server/services/media/service.go:108`,
`presignedURLExpiry = 35 * time.Minute`). The comment above that constant says it
"must exceed the client-side stash cache TTL (default `CACHE_TTL_MINUTES = 30 min`)."

So:

- T+0: `getMediaUrl` hits the server, gets a URL valid until T+35.
- T+0 to T+30: cache hits return the same URL.
- T+30: stash cache evicts the entry; next `getMediaUrl` re-fetches.

The 5-minute slack between the 30-min cache TTL and the 35-min URL lifetime
is supposed to be enough headroom, **but it only works for the URL that was
freshly fetched.** Two things can erode it:

- **Closure capture lag.** `MessageMediaAttachments` captures `mediaUrl.url`
  inside the tap closure (step 3 above). If the user scrolled past the
  message, kept the app open, and tapped it 25–30 minutes later, the closure
  still holds the original URL — and the metadata cache entry that produced it
  is now 25–30 minutes old too. Open the carousel a few minutes later, fetch
  the main view, and you're at T+~35 (or worse — if the metadata cache hit
  was already an older entry, T+~35+) which is past the URL's expiry.
- **Backgrounding.** iOS may suspend the app for an arbitrary interval. When
  it resumes, the chat row re-uses the cached `MediaUrl` until the
  `FutureBuilder` rebuilds; the URL embedded in any in-flight closure or
  saved state can be much older than the rebuild suggests.

Result: the main view's first fetch goes out to GCS, GCS returns 403/expired,
`CachedNetworkImage` stores the failure and renders `errorWidget`. The
thumbnail strip is unaffected because its bytes are already on disk.

## Why the strip's cache key doesn't help the main view

`docs/client/media.md:9` says "cache keys based on stable media IDs allow
image caches to persist across URL refreshes." True — but the strip and the
main view are **separate cache entries** (`media:thumb:…` vs. `media:full:…`).
A persisted thumbnail under one key does not satisfy a first-time fetch under
the other key. The carousel is hitting the URL-expiration problem from a
cold cache, and there is currently no retry path that refreshes the
underlying presigned URL when the fetch fails.

There is also a small content bug in passing: the carousel main view's
cacheKey says "full" but the URL it loads is the **thumbnail URL**
(`MessageMediaAttachments` only ever had the thumbnail URL to pass in). So
when the carousel does work, the main view shows the thumbnail-resolution
image stretched to fullscreen. Worth fixing alongside the expiration issue,
since the natural fix touches the same code path.

## Potential Solutions

### Option A: Re-resolve the URL inside the carousel via a ViewModel (recommended)

- **Approach:** Make `CarouselImageViewer` (and `FullscreenImageViewer`,
  same code path on the tap-to-zoom layer) take a `mediaId` instead of a
  baked-in URL. Introduce a small ViewModel/provider that owns the
  repository call — concretely, a
  `FutureProvider.autoDispose.family<MediaUrl, String>` (or an
  equivalent `AsyncNotifier` if more state is needed) that calls
  `MediaRepository.getFullMediaUrl(mediaId)`. The widget consumes
  `AsyncValue<MediaUrl>` via `.when(...)` and uses the resolved
  `MediaUrl.url` and `MediaUrl.cacheKey` for `CachedMediaImage`. On
  error, the retry path calls `mediaRepository.invalidate(mediaId)`
  followed by `ref.invalidateSelf()` on the family provider — that
  forces a fresh `getFullMediaUrl` round-trip with a server-generated
  presigned URL.
  - Keep an optional URL field on `MediaItem` as a fallback for the
    small handful of callers that have only a URL (uploads in flight,
    attribution-only surfaces) — the viewer prefers `mediaId` and
    falls back to the URL when `mediaId` is null.
  - Switch the main view's data source from the thumbnail URL to the
    **full** URL via `getFullMediaUrl`, so the `media:full:` cache key
    actually holds full-resolution bytes. Closes the silent content
    bug noted above.
  - Replace the raw `CachedNetworkImage` calls in
    `carousel_image_viewer.dart` with `CachedMediaImage` and pass
    `semanticsLabel: context.l10n.a11yViewMediaContent` (new key) so
    the lint `require_semantic_label_on_cached_media_image` passes
    and the image is announced meaningfully.
- **Pros:** Fixes the root cause (stale captured URL), recovers from
  metadata-cache staleness, aligns the carousel with the
  "stable cache key per media ID" pattern, and respects the MVVM rule
  that widgets never call repositories directly
  (`docs/client/architecture.md:47–51`). Minimal new abstractions —
  the repository already has `getFullMediaUrl`, `MediaUrl` already
  exposes `cacheKey`, and a `FutureProvider.family` is a one-liner.
- **Cons:** A few extra widgets get their own provider subscription,
  which adds a frame of latency on cold opens. Mitigated because
  `getFullMediaUrl` hits the in-memory metadata cache on the warm
  path. Some chat call sites pass URLs without a `mediaId`; those
  retain the URL-direct fallback.
- **Effort:** Small to medium. Touches three widget files
  (`carousel_image_viewer.dart`, `media_carousel.dart`, the
  `MessageMediaAttachments` carousel-open path), adds one provider
  file (`carousel_media_url_provider.dart`), and adds two ARB keys.

### Option B: Always invalidate-and-retry on error

- **Approach:** Leave the URL-capture flow alone, but add an `errorWidget`
  that calls `mediaRepository.invalidate(mediaId)` and triggers a one-shot
  rebuild. On retry, the metadata cache miss forces a fresh
  `getMediaUrl(mediaId)` round-trip, which gives a new URL.
- **Pros:** Tiny diff. Doesn't change the data flow.
- **Cons:** Treats the symptom, not the cause. Every expired-URL view
  burns one round-trip before showing the image — visible to the user as a
  brief error glyph that flickers into success, which is a worse UX than
  just doing the right thing up front. Also leaves the "main view uses
  thumbnail URL" content bug unfixed.
- **Effort:** Small.

### Option C: Tighten the headroom between URL expiry and cache TTL

- **Approach:** Bump server `presignedURLExpiry` from 35 min to e.g. 2 h, or
  drop client `CACHE_TTL_MINUTES` from 30 min to 10 min, so closure-capture
  lag is far less likely to land in the expired window.
- **Pros:** Trivially small server constant change.
- **Cons:** Doesn't actually fix the bug; only widens the lucky window. Any
  user who backgrounds the app long enough still hits the same failure.
  Longer-lived signed URLs are also a small security regression (they're
  bearer credentials for the bucket). Shorter cache TTLs increase
  background metadata RPC volume.
- **Effort:** Small but misguided as a sole fix.

## Recommendation

**Option A.** It addresses both the captured-URL staleness *and* the
silent content bug (showing thumbnail bytes in the "full" viewer), and
matches the pattern the rest of the codebase already follows —
`CachedMediaImage` with `mediaUrl.cacheKey` and `MediaRepository` calls
keyed on `mediaId`. Option B (in-error invalidate-and-retry) folds into
Option A as the error-handling branch; neither B nor C alone is the right
shape of fix.

Option A as audited also respects the MVVM rule that widgets never call
repositories directly — the repository call moves into a
`FutureProvider.family` ViewModel that the viewer widgets consume via
`AsyncValue.when()`.

## Open Questions

- **Should the carousel always show the full image, or should it
  preferentially reuse the cached thumbnail bytes for the first frame and
  swap to full when ready?** — *Recommendation:* Fetch the full URL
  directly. The thumbnail is small (typically 200–400px wide); the visible
  upgrade-in-place flicker is more jarring than the extra few hundred ms of
  network. Reusing the thumbnail bytes for an instant placeholder is a
  worthwhile *future* polish, but it should be the `placeholder:` callback,
  not the primary `imageUrl:`.
- **How aggressively should we invalidate on error?** — *Recommendation:*
  Invalidate once per provider instance, not in a loop. Track retry count
  inside an `AsyncNotifier` (or keep an `int` family-keyed counter in
  `ref.read`-only state) so a transient 5xx doesn't burn through retries.
  Cap at 1 retry — after that, surface the error.
- **Are there other call sites passing captured presigned URLs into long-lived
  closures?** — *Recommendation:* Audit `MediaCarousel.show` call sites
  during this work (`community_content_view.dart`,
  `inline_conversation_attachments.dart`, `content_view_helpers.dart`,
  etc.). If any pass `MediaItem(url: …, mediaId: …)` from a captured
  snapshot, they have the same latent bug — fix them all by routing the
  carousel through `mediaId` directly.
- **Should we extend `getFullMediaUrl` with a `forceRefresh` flag?** —
  *Recommendation (revised after audit):* **No.** The documented pattern
  (`docs/client/caching.md:107–121`) is `repository.invalidate(key)` then
  a normal call. The error-retry path becomes:
  `await mediaRepository.invalidate(mediaId); ref.invalidateSelf();`
  which forces a fresh metadata fetch through the existing API. No new
  surface, and consistent with how `LoanRepository` handles mutations.

## Implementation Plan

### Phase 1: Carousel viewer takes `mediaId`, resolves URL via a ViewModel
**Goal:** Fix the immediate bug (expired URL on photo viewing) and the silent
"thumbnail-stretched-to-fullscreen" content issue, while staying inside the
MVVM rules.
**Tasks:**
- [ ] Add a new `app/lib/presentation/viewmodels/carousel_media_url_provider.dart` exposing `FutureProvider.autoDispose.family<MediaUrl, String>` (keyed by `mediaId`) that calls `MediaRepository.getFullMediaUrl(mediaId)`. The repository call lives in the ViewModel layer; widgets only consume the provider.
- [ ] Update `CarouselImageViewer` (`app/lib/presentation/widgets/media/carousel/carousel_image_viewer.dart:14`) to take `String mediaId` (optionally plus a URL fallback). In `build`, `ref.watch(carouselMediaUrlProvider(mediaId)).when(...)` and render `CachedMediaImage(imageUrl: mediaUrl.url, cacheKey: mediaUrl.cacheKey, semanticsLabel: context.l10n.a11yViewMediaContent)`.
- [ ] Update `FullscreenImageViewer` similarly (same file, line 67).
- [ ] On error, render a retry-capable widget that on tap calls `await mediaRepository.invalidate(mediaId); ref.invalidateSelf();` via a small notifier or `ConsumerStatefulWidget` retry-count guard. Cap at 1 automatic retry; surface the error (with `Semantics(label: context.l10n.a11yMediaLoadError, liveRegion: true)`) afterwards.
- [ ] Update `MediaCarousel._buildMediaItem` and `_showFullscreenImage` (`media_carousel.dart:357`, `:368`) to pass `mediaId` instead of `url`.
- [ ] For `MediaItem` entries that have no `mediaId` (uploads in flight, etc.), retain the existing URL-direct rendering path so local-file display still works.
- [ ] Add ARB keys `a11yViewMediaContent`, `a11yMediaLoadError`, `a11yMediaRetry` to both `app_en.arb` and `app_es.arb` with `@key` metadata, then `npm run generate`.
**Validates:** Repro from the issue (open photo in chat ≥35 min after the metadata cache last refreshed) succeeds. Manual check that a freshly-uploaded photo still works without regression. Lint `npm run lint:dart:a11y` and `npm run lint:dart:i18n` pass.

### Phase 2: Audit other carousel entry points
**Goal:** Catch every place that captures a presigned URL into a long-lived
closure passed to `MediaCarousel.show` or related viewers.
**Tasks:**
- [ ] Grep `MediaCarousel.show(`, `MediaItem(`, and `FullscreenImageViewer(`. Files of interest from the initial sweep: `app/lib/presentation/screens/communities/community_content_view.dart`, `app/lib/presentation/widgets/content/inline_conversation_view.dart`, `app/lib/presentation/widgets/content/content_view_helpers.dart`, `app/lib/presentation/widgets/content/inline_conversation_attachments.dart`.
- [ ] For each call site, confirm it routes a `mediaId` through to the new viewer signature.
- [ ] Add a lightweight assertion or lint comment that `MediaItem` constructed for the carousel should carry a `mediaId` whenever one is available.
**Validates:** A code search confirms no remaining call sites embed captured presigned URLs without a corresponding `mediaId`.

### Phase 3: Tests + telemetry
**Goal:** Prevent regression and surface the failure mode if it recurs.
**Tasks:**
- [ ] **Provider tests** in `app/test/presentation/viewmodels/carousel_media_url_provider_test.dart`: use `ProviderContainer` with an overridden `mediaRepositoryProvider` (Mockito `@GenerateMocks([MediaRepository])`). Assert: (a) provider returns the `MediaUrl` from `getFullMediaUrl`, (b) `ref.invalidate` followed by re-read calls `getFullMediaUrl` again, (c) errors propagate as `AsyncError`. Pattern follows `docs/client/testing.md:38–87`.
- [ ] **Repository test** in `app/test/data/repositories/media_repository_test.dart`: assert `invalidate(mediaId)` followed by `getFullMediaUrl(mediaId)` re-hits the underlying `MediaService` (no `forceRefresh` API needed — verifies the canonical `invalidate-then-call` pattern works).
- [ ] **Thin widget test** for `CarouselImageViewer` covering the three `AsyncValue` branches (loading spinner, image rendered, error glyph + retry semantics label). Mock `mediaRepositoryProvider` via `ProviderScope` override.
- [ ] **Disposal-safety test** per `architecture.md:809–826`: start `getFullMediaUrl`, dispose the `ProviderContainer`, and assert no crash. Required for autoDispose providers.
- [ ] Add a structured Dart log via the existing client logger (Dart `package:logging`) when the error-retry path fires, with the `mediaId` field so post-mortem feedback reports show whether the URL-expiration recovery activated. Do not log presigned URLs — they are bearer credentials.
**Validates:** CI keeps the fix from regressing; if it does come back, the retry log appears in app diagnostics instead of waiting for another feedback report.

## Related Issues
- #1197 (closed) — Same root cause for inbox experience thumbnails — fixed by adopting `CachedMediaImage` + stable cache keys. Confirms the project already understands the URL-expiration class of bug; this issue is the same class applied to the chat-photo carousel main view.
- #1154 (open) — Feed card stuck on spinner after `refreshInPlace` invalidates the media cache. Related in that it's another media-cache vs. URL-lifetime interaction; worth re-reading after Phase 1 to see if the fix here suggests anything for that bug.
- #1529 (open) — Per-resource auth check on `GetMedia` is missing. Unrelated to this bug but relevant if Phase 1 adds a force-refresh path that bumps server-side `GetMedia` traffic.
