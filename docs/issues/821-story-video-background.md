# Issue #821: Videos not showing up in stories

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/821
**Filed:** 2026-03-10 | **Last activity:** 2026-03-10
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

When a story is created in the feed for a completed experience with a video
background, the video does not render. The root cause is that
`StoryContentView` hardcodes `isVideo: false` when calling
`ContentViewBuilders.buildMediaBackground()` (line 74), so video media is
always rendered as a static image (which shows as a black frame or broken
image since the URL points to a video file).

The `StoryViewModel` already loads `MediaUrl` objects with `contentType`
correctly populated via `StoryRepository.loadMediaUrls()` →
`MediaRepository.getMediaUrl()`. The video detection data is available — it's
just not being used.

All other content views (Gear, Experience, Request, Community) correctly
detect video via `mediaUrl.contentType?.startsWith('video/')` and initialize a
`VideoPlayerController`. Stories are the only content type that skips this.

## Context from Comments

No comments on the issue. The bug is straightforward from code inspection.

## Affected Code

| File | Issue |
|------|-------|
| `app/lib/presentation/screens/story/story_content_view.dart:74` | Hardcodes `isVideo: false` |
| `app/lib/presentation/viewmodels/story_view_model.dart` | No `isVideo`, `videoController`, or `_videoController` fields |

## Potential Solutions

### Option A: Add video support to StoryViewModel + StoryContentView

- **Approach:** Follow the same pattern used by ExperienceNotifier and other
  content ViewModels: add `isVideo` and `videoController` to `StoryState`,
  detect video from `contentType` during media loading, initialize a
  `VideoPlayerController`, and pass it through to `buildMediaBackground()`.
- **Pros:** Consistent with all other content views. Uses proven, tested
  pattern. Full video lifecycle management (init, loop, mute, dispose).
- **Cons:** Requires adding a private `_videoController` field to the Notifier
  for safe disposal (standard pattern but adds some state management).
- **Effort:** Small.

### Option B: Detect video in StoryContentView widget only

- **Approach:** Check `contentType` in the widget and conditionally create a
  `VideoPlayerController` at the widget level, without modifying the ViewModel.
- **Pros:** Minimal changes to ViewModel.
- **Cons:** Violates architecture (business logic in widget). Video controller
  lifecycle management in a widget is error-prone. Doesn't follow the
  established pattern from other content views.
- **Effort:** Small, but architecturally wrong.

## Recommendation

**Option A** — add video support following the established content ViewModel
pattern. This is a small, well-understood change that mirrors exactly what
ExperienceNotifier, GearNotifier, and others already do. The pattern is:

1. Add `isVideo` + `videoController` to Freezed state
2. Keep a private `_videoController` in the Notifier for safe disposal
3. Detect video from `contentType` during media loading
4. Initialize `VideoPlayerController.networkUrl()`, set looping + muted
5. Register `ref.onDispose()` to dispose the controller
6. Pass `isVideo` + `videoController` to `buildMediaBackground()`

## Open Questions

- **Q: Should stories support the volume FAB?** — Recommendation: No, not
  initially. Stories have a 60% black overlay and are informational. The
  video is atmospheric background, not primary content. Add mute toggle
  later if users request it.
- **Q: Should stories use video caching (issue #811)?** — Recommendation:
  Yes, once #811 lands. For now, use `VideoPlayerController.networkUrl()`
  to match the current pattern in other ViewModels. When #811's
  `MediaRepository.getVideoFile()` is available, stories should adopt it
  along with all other content views.

## Implementation Plan

### Phase 1: Add video support to StoryViewModel

**Goal:** Detect video media and initialize a VideoPlayerController in the
story ViewModel.

**Tasks:**
- [ ] Add `isVideo` (bool, default false) and `videoController`
  (`VideoPlayerController?`) fields to `StoryState` Freezed class
- [ ] Add private `VideoPlayerController? _videoController` field to
  `StoryNotifier`
- [ ] Register `ref.onDispose()` in `build()` to dispose `_videoController`
- [ ] In `_loadStory()`, after loading media URLs, check
  `primaryMediaUrl.contentType?.startsWith('video/')` — if true:
  - Fetch the full-resolution URL via `mediaRepository.getFullMediaUrl(mediaId)`
    (not the thumbnail-preferred URL from `loadMediaUrls()`)
  - Create and initialize a `VideoPlayerController.networkUrl()` with the full URL
  - Check `ref.mounted` after `controller.initialize()` before updating
    `_videoController` or returning state with the controller
  - Set looping + muted, store in `_videoController`, include in returned state
- [ ] Run `flutter pub run build_runner build` to regenerate Freezed code

<!-- AUDIT FIX 1 (async safety): StoryNotifier is an AsyncNotifier.autoDispose
     family provider. Per architecture.md "Async Safety in Notifiers", methods
     that manually set state or fields after async gaps MUST check ref.mounted.
     controller.initialize() is an async gap — if the user scrolls past the
     story during init, autoDispose fires and the notifier is gone. Without
     the guard, _videoController would be set on a disposed notifier. -->

<!-- AUDIT FIX 2 (thumbnail vs full URL): StoryRepository.loadMediaUrls() calls
     MediaRepository.getMediaUrl() which prefers thumbnail URLs. For video
     playback, we need the full video URL (getFullMediaUrl()), not a potential
     thumbnail. Currently videos have no server-side thumbnails (media.md known
     limitation #4), so getMediaUrl() falls back to the full URL — but this is
     fragile. If video thumbnails are added (media.md future improvement #2),
     the story would get a still-frame JPEG URL instead of the video stream.
     Using getFullMediaUrl() explicitly matches the pattern in
     ExperienceNotifier._loadMediaFromServer(). -->

**Validates:** StoryState correctly reports `isVideo: true` and provides an
initialized `videoController` when the story's primary media is a video.

### Phase 2: Wire video into StoryContentView

**Goal:** Render video backgrounds in story feed items.

**Tasks:**
- [ ] Update `_buildContent()` in `StoryContentView` to pass `state.isVideo`
  and `state.videoController` to `ContentViewBuilders.buildMediaBackground()`
  instead of hardcoded `isVideo: false`
- [ ] Verify that `MediaBackground` renders the video correctly behind the
  60% black overlay
- [ ] Test with a completed experience that has a video background to confirm
  video plays in the feed story

**Validates:** Video backgrounds render and loop in story feed items.

### Phase 3: Tests

**Goal:** Verify video detection and lifecycle in story ViewModel.

**Tasks:**
- [ ] Add unit test: story with video media sets `isVideo: true` and
  `videoController` is not null
- [ ] Add unit test: story with image media sets `isVideo: false` and
  `videoController` is null
- [ ] Add unit test: story with no media has `isVideo: false`
- [ ] Add disposal safety test: disposing notifier during async video init
  completes without error

**Validates:** Video detection logic is correct and disposal is safe.

## Audit Notes

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/server/architecture.md`

**Issues found and fixed:**

1. **Missing `ref.mounted` check (architecture.md, Async Safety section):**
   `StoryNotifier` is an `AsyncNotifier.autoDispose.family` provider.
   `controller.initialize()` is an async gap — if the story scrolls off-screen
   during initialization, `autoDispose` fires and the notifier is disposed.
   Without a `ref.mounted` guard after `initialize()`, `_videoController` would
   be set on a disposed notifier. Added the required guard to Phase 1 tasks.

2. **Thumbnail vs full URL for video (media.md, Thumbnail vs Full Strategy):**
   `StoryRepository.loadMediaUrls()` calls `getMediaUrl()` which prefers
   thumbnails. For video playback, the full video URL is required. Currently
   videos have no thumbnails so this works by accident, but it will break when
   video thumbnail support is added (media.md future improvement #2). Fixed:
   Phase 1 now explicitly calls `getFullMediaUrl()` for video media, matching
   the pattern in `ExperienceNotifier._loadMediaFromServer()`.

**No issues with:**
- MVVM layering: video detection in ViewModel, not widget (architecture.md
  rules #1 and #3)
- Disposal pattern: private `_videoController` + `ref.onDispose()` matches
  established content ViewModel pattern (media.md, VideoPlayerController
  Lifecycle section)
- Repository access: data flows through `StoryRepository` → `MediaRepository`
  (caching.md rule #1: repositories own caching)
- No server-side changes needed: story creation already preserves media IDs
  including videos

## Related Issues

- #811 — Video not caching. Once that lands, story video loading should use
  `MediaRepository.getVideoFile()` instead of `VideoPlayerController.networkUrl()`.
