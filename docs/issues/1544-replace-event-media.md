# Issue #1544: Replace media in event

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1544
**Filed:** 2026-04-28 | **Last activity:** 2026-04-30
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When creating a new event, the user can tap the pencil ("Replace background")
overlay on the AI-generated preview image to pick a different photo. The new
photo uploads successfully — proven by the fact that after tapping
**Create event** the experience is created with the replacement image. But
inside the preview modal itself the background never updates: the user still
sees the AI-generated image, even after their replacement upload finishes.
Because the user sees no feedback, they assume the operation failed and try
again or give up.

The bug is in the preview / create flow, **not** the existing-experience
edit flow. The preview lives in
`app/lib/presentation/screens/experience/experience_preview_modal.dart`,
which renders the background from
`genExperienceProvider.state.selectedMediaIds.first`.

## Context from Comments

- **maintainer** reproduced the bug and gave a precise repro:
  > create a new event → in the event details screen, click pencil over
  > "replace background" → select another image and confirm → background is
  > not replaced in preview, but after hitting "create event" it shows
  > correctly with the replacement image.
- The reproducer uses photo replacement (image → image). The user did not
  test video replacement, but the underlying state-handling bug is broader
  than the image case (see Recommendation).
- No prior comments propose a fix. maintainer pinged @claude to handle it.

## Root cause

In `experience_preview_modal.dart` the background widget tree is:

```dart
final firstMediaId = effectiveMediaIds.isNotEmpty ? effectiveMediaIds.first : null;
...
if (firstMediaId != null) _buildBackground(state, firstMediaId),
```

and `_buildBackground` is:

```dart
Widget _buildBackground(GenExperienceState state, String mediaId) {
  final controller = state.previewVideoController;
  if (controller != null && controller.value.isInitialized) {
    return Positioned.fill(child: ...VideoPlayer(controller)...);   // (A)
  }
  return _BackgroundMediaImage(key: ValueKey(mediaId), mediaId: mediaId); // (B)
}
```

Two separate problems combine to produce the user-visible bug:

1. **Stale video controller.** `previewVideoController` is set once by
   `_loadPreviewVideoIfNeeded`, called only from `initializeFromGenResponse`
   (`gen_experience_view_model.dart:450-452`). When the user replaces media
   via `pickAndUploadMultipleImagesFromGallery` /
   `pickAndUploadVideoFromGallery` / `pickAndUploadImageFromCamera` (provided
   by `MediaUploadMixin`), the resulting `onMediaUploaded` /
   `onBatchUploadCompleted` callbacks update `selectedMediaIds` but **never
   touch `previewVideoController`** (`gen_experience_view_model.dart:166-217`).
   So if the AI generated a video as the suggested media, branch (A) keeps
   firing with the old controller and the original video keeps playing —
   regardless of what the user picked.

2. **`FutureBuilder` future churn on every rebuild in
   `BackgroundMediaImage`.** `app/lib/presentation/widgets/media/background_media_image.dart:51-52`:

   ```dart
   FutureBuilder<MediaUrl>(
     future: getMediaUrl(),   // new Future every build
     ...
   )
   ```

   `_BackgroundMediaImage` does have `key: ValueKey(mediaId)`, so when the
   media id changes the entire subtree is rebuilt with a fresh element and a
   correct new URL. But: the closure passed to `FutureBuilder.future` is
   re-invoked on every rebuild, and unrelated provider notifications (e.g.
   the streaming-generation listener at lines 334-353, or any state copy
   that fires while the upload is in flight) cause the widget to rebuild
   with the **same** `mediaId` but a new `Future`. Each rebuild discards the
   previous snapshot and re-issues the request, which in the image-replace
   case can race with another notifier emission and produce visible
   flicker / stale states. The fix should either memoize the future or
   key the FutureBuilder by `mediaId` so it only re-fetches when the id
   actually changes.

The combined effect explains the bug: with a video-mode AI-generated
suggestion, the user always sees the AI clip; with an image-mode
suggestion, the FutureBuilder thrashes and the user sees the old image
flash back rather than the new one resolve. Either way the new
`selectedMediaIds.first` is correct in state — which is why the experience
is correctly created when the user finally taps **Create event**.

## Potential Solutions

### Option A: Fix viewmodel + widget; minimal surface (recommended)

- **Approach:**
  1. In `gen_experience_view_model.dart`, refactor `onMediaUploaded` and
     `onBatchUploadCompleted` (and `onBatchUploadPartialFailure`) to call a
     new private method `_replacePreviewMedia(List<String> mediaIds)` that:
     (a) **synchronously** disposes any existing `_previewVideoController`,
     (b) sets state in a single `copyWith` call (`selectedMediaIds`,
     `previewVideoController: null`, `previewIsMuted: true`), so the
     widget tree rebuilds the image branch with the new id immediately —
     no flicker through the old video controller; (c) **asynchronously**
     kicks off `_loadPreviewVideoIfNeeded(mediaIds)` (fire-and-forget, the
     helper is content-type-checked so it's a no-op for image media).
     This matches the existing video-controller lifecycle pattern in
     `docs/client/media.md` ("private field + state copy" — Notifier owns
     the field; state holds the read-only mirror).
  2. In `app/lib/presentation/widgets/media/background_media_image.dart`,
     stabilize the future. Pass an explicit `mediaId` parameter and use
     `FutureBuilder<MediaUrl>(key: ValueKey('bg-$mediaId'), ...)` so the
     FutureBuilder is rebuilt fresh exactly when `mediaId` changes and
     not on unrelated parent rebuilds. This is preferable to a
     `StatefulWidget` + closure-identity check because the call sites
     (`_BackgroundMediaImage` etc.) construct a new closure every parent
     rebuild — closure identity is unreliable. Update the four call
     sites (`experience_preview_modal.dart`,
     `gear_preview_modal.dart`, `community_preview_modal.dart`,
     `request_preview_modal.dart`) to pass the `mediaId` they already
     have. The existing `getMediaUrl` parameter stays for the closure
     that does the actual fetch.
  3. Keep `_buildBackground` defensively guarded: only return branch (A)
     if `previewVideoController != null` **and**
     `previewVideoController.value.isInitialized`. The current condition
     already does this; no shape change needed once Phase 1 keeps the
     controller in sync.
- **Pros:** Targets root cause in both layers. Small diff. Tests are
  straightforward (viewmodel unit test for state transitions, widget test
  for FutureBuilder stability).
- **Cons:** Touches both a viewmodel and a shared widget — `BackgroundMediaImage`
  is also used by gear / community / request preview modals
  (verified via grep), so the widget change must be regression-tested
  across all four. The widget signature change (adding `mediaId`) is
  source-incompatible at the four call sites, which all need a
  one-line edit. Sibling viewmodels share the same `MediaUploadMixin`
  pattern and have the same controller-staleness vulnerability — this
  PR scopes the **viewmodel** fix to experience only (the only flow
  with a reported bug and a video controller path); the widget fix
  benefits all four because it's at the shared widget.
- **Effort:** Small-to-medium.

### Option B: Push controller management into MediaUploadMixin

- **Approach:** Add an optional `onMediaIdsChanged(List<String>)` hook to
  `MediaUploadMixin` (default no-op) and have it called automatically
  inside `_setMediaFile` / `_uploadMultipleMediaFiles` after every
  successful upload. The experience viewmodel overrides it to do video
  controller lifecycle management; gear/community/request override it to
  invalidate their own thumbnails.
- **Pros:** Fixes the bug class for all four preview viewmodels in one
  pass. Cleaner abstraction.
- **Cons:** Larger refactor, riskier — mixin is consumed by four
  viewmodels with subtly different state shapes (e.g. some always use
  single-element media lists, some use multi). Harder to land safely
  in a single PR. The user's reported bug only proves the experience
  flow needs fixing; speculative refactor of three more is YAGNI.
- **Effort:** Medium.

### Option C: Widget-only fix — drive video player from media id, not controller

- **Approach:** Remove `previewVideoController` from state entirely and
  always render `_BackgroundMediaImage`. Have a separate
  `_BackgroundMediaVideo(mediaId)` widget that creates and owns its own
  `VideoPlayerController` keyed on `mediaId`. `_buildBackground` selects
  image vs. video by inspecting the media's `contentType`.
- **Pros:** No global controller state; lifecycle is automatic.
- **Cons:** Significant rewrite of preview-video plumbing. The current
  controller is created from a cached file (`createCachedVideoController`)
  to share bytes with the post-creation experience screen — moving that
  logic into a per-widget controller would either lose that optimization
  or require new caching plumbing. Out of proportion to the bug.
- **Effort:** Large.

## Recommendation

**Option A.** It targets the precise root cause in both layers
(viewmodel-side controller leak and widget-side future churn), is the
smallest fix, and the test surface is bounded. The widespread mixin
issue is real but is best addressed in a separate, focused refactor
once this user-visible bug is unblocked. Filing a follow-up issue is
part of this plan (see Phase 3).

## Open Questions

- **Should the fix also dispose the controller when the user clears
  media (e.g. via a future "remove background" gesture)?**
  Recommendation: yes — handle it inside `_replacePreviewMedia([])` so
  any future clear-media path is automatically correct. No new code path
  needed today.
- **Does FCM/realtime invalidate the `media:<id>` cache entry of the
  newly-uploaded media in a way that re-triggers the FutureBuilder?**
  Recommendation: not relevant to this fix. The FutureBuilder churn is
  caused by widget rebuilds, not cache invalidation. Memoizing the
  future is the correct fix regardless.
- **Should the same fix be applied to gear / community / request
  preview modals as part of this PR?**
  Recommendation: no — open a follow-up tracking issue. The mixin-level
  refactor (Option B) is the right place to handle it; doing it in
  this PR risks scope creep and a larger blast radius for a fix that
  unblocks a single reported bug.

## Implementation Plan

### Phase 1: Viewmodel — manage video controller lifecycle on media replace

**Goal:** Ensure `previewVideoController` always reflects
`selectedMediaIds.first`.

**Tasks:**
- [ ] In `app/lib/presentation/viewmodels/gen_experience_view_model.dart`,
  add a private `void _replacePreviewMedia(List<String> mediaIds)`
  method (synchronous in shape; it kicks off async work but does not
  return a Future to the caller) that:
  - Synchronously disposes `_previewVideoController` if set; nulls the
    field.
  - Calls `state = state.copyWith(selectedMediaIds: mediaIds, previewVideoController: null, previewIsMuted: true)`.
    Doing this in one assignment (rather than two) means widget
    rebuilds see consistent state — the old video branch never
    flashes after the new image arrives.
  - If `mediaIds.isNotEmpty`, calls
    `unawaited(_loadPreviewVideoIfNeeded(mediaIds))` — the helper
    already short-circuits on non-video content type and already does
    `if (!ref.mounted) return;` after its internal awaits, so no
    additional safety check is needed here.
- [ ] Refactor `onMediaUploaded(String mediaId)` so it both calls
  `_replacePreviewMedia([mediaId])` **and** sets `isUploadingMedia: false`.
  Since `_replacePreviewMedia` does its own `state = state.copyWith(...)`,
  fold the `isUploadingMedia: false` into the helper's copyWith **or**
  into a follow-up assignment after the helper returns — pick whichever
  reads cleaner during implementation; both are correct because the
  Notifier runs single-threaded.
- [ ] Refactor `onBatchUploadCompleted(List<String> mediaIds)` similarly,
  preserving the existing `batchUpload*: null` resets.
- [ ] Refactor `onBatchUploadPartialFailure(List<String> uploadedIds, int failedCount)`
  to call the same helper with `uploadedIds`, preserving the existing
  `batchUploadFailedCount: failedCount` field.
- [ ] Add unit tests in
  `app/test/presentation/viewmodels/gen_experience_view_model_test.dart`
  using the existing `ProviderContainer` + Mockito setup:
  - Test 1: After `initializeFromGenResponse` with a video media id
    sets a controller (mock `MediaRepository.getFullMediaUrl` to return
    `contentType: 'video/mp4'`), calling `onMediaUploaded(newImageId)`
    clears `previewVideoController` from state and updates
    `selectedMediaIds` to `[newImageId]`.
  - Test 2: After image → image replacement, `selectedMediaIds.first`
    is the new id and `previewVideoController` stays null. Verify
    `MediaRepository.getFullMediaUrl` is called exactly once for the
    new media id (the controller-init path runs but exits early on
    image content type).
  - Test 3: `onBatchUploadPartialFailure` path also replaces media
    correctly and preserves `batchUploadFailedCount`.
  - Test 4 (disposal safety): Start a media replace where
    `_loadPreviewVideoIfNeeded` is mid-flight; dispose the
    `ProviderContainer` before the future resolves; assert no exception.

**Validates:** The viewmodel never hands a stale controller to the
preview branch (A); covers both video → image and image → image; and
disposal during the async controller-init path is safe.

### Phase 2: Widget — stabilize FutureBuilder

**Goal:** Eliminate redundant `getMediaUrl()` re-issues on unrelated
rebuilds across all four preview modals.

**Tasks:**
- [ ] Modify `BackgroundMediaImage` in
  `app/lib/presentation/widgets/media/background_media_image.dart`:
  - Add a required `String mediaId` constructor parameter.
  - Inside `build`, return
    `FutureBuilder<MediaUrl>(key: ValueKey('bg-$mediaId'), future: getMediaUrl(), ...)`.
    The `ValueKey` makes Flutter discard the FutureBuilder element on
    `mediaId` change (and only on `mediaId` change), so the future is
    re-issued exactly once per real id change. Same id → same element
    → snapshot is preserved across unrelated parent rebuilds.
  - Update the existing `CachedMediaImage(key: ValueKey(mediaUrl.mediaId), ...)`
    inside the success branch — this stays correct.
- [ ] Update the four call sites to pass `mediaId`:
  - `app/lib/presentation/screens/experience/experience_preview_modal.dart`
    (`_BackgroundMediaImage` already holds `mediaId`; forward it).
  - `app/lib/presentation/screens/gear/gear_preview_modal.dart`
    (search for `BackgroundMediaImage(`).
  - `app/lib/presentation/screens/communities/community_preview_modal.dart`.
  - `app/lib/presentation/screens/request/request_preview_modal.dart`.
  - Confirm via `grep -rn "BackgroundMediaImage(" app/lib/` that no
    other call sites exist before pushing.
- [ ] No change needed inside `_buildBackground` in
  `experience_preview_modal.dart` once Phase 1 keeps the controller in
  sync — the existing guard
  `controller != null && controller.value.isInitialized` is sufficient.
- [ ] Add or extend a widget test in
  `app/test/presentation/widgets/media/` (use an existing file there if
  one already covers `BackgroundMediaImage`; otherwise create
  `background_media_image_test.dart`):
  - Test A — same `mediaId`, multiple parent rebuilds: build a host
    widget that drives unrelated state changes; pump three frames;
    assert the `getMediaUrl` callback is invoked at most once.
  - Test B — `mediaId` change: rebuild with a new `mediaId`; assert
    the callback fires again with the new id.

**Validates:** Same `mediaId` → at most one URL fetch; new `mediaId` →
exactly one new URL fetch. Eliminates the flicker / stale-state path
for all four preview modals (the user-reported one and three siblings
that share the same widget).

### Phase 3: Manual verification + follow-up

**Goal:** Confirm the user-visible flow is fixed and capture related
work without scope-creeping this PR.

**Tasks:**
- [ ] Manual repro on Android emulator: run the app, create a new event
  with a text or image prompt, wait for AI suggestion to land, tap
  "Replace background", pick a different photo, and verify the preview
  updates immediately. Repeat with video → image and image → video
  permutations.
- [ ] Run `cd app && flutter test --reporter expanded` to confirm
  no regressions across viewmodel and widget tests.
- [ ] Run `npm run lint:dart:size` and `dart analyze` (or
  `flutter analyze`).
- [ ] File a follow-up issue: "Apply Option B media-replace fix to
  gear / community / request preview viewmodels" — link to this plan
  and to the user-feedback bug. Do not implement it in this PR.

**Validates:** End-to-end fix lands cleanly; the broader pattern is
tracked for separate work.

## Related Issues

- None directly. Future follow-up issue (Phase 3) will track the same
  fix pattern in `gen_gear_view_model.dart`,
  `gen_community_view_model.dart`, and `gen_request_view_model.dart`.

## Audit notes (ripls-audit pass — 2026-04-30)

Adjustments applied after reading
`docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/media.md`, `docs/client/testing.md`,
`docs/client/accessibility.md`, `docs/server/architecture.md`,
`docs/server/observability.md`:

- **Closure-identity check is unreliable** — the original Phase 2
  proposed a `StatefulWidget` + `oldWidget.getMediaUrl != widget.getMediaUrl`
  check. Each `_BackgroundMediaImage` rebuild constructs a fresh closure
  with the same captured `mediaId`, so closure identity always differs
  and the memoization wouldn't help. Switched to passing `mediaId`
  explicitly and using `ValueKey('bg-$mediaId')` on the `FutureBuilder`,
  which is the same pattern `docs/client/media.md` uses for
  `CachedMediaImage` cache keys (stable id, not URL).
- **Synchronous state assignment** — `_replacePreviewMedia` now sets
  `selectedMediaIds`, `previewVideoController: null`, and
  `previewIsMuted: true` in a single `copyWith`, not a sequence. This
  matches `docs/client/architecture.md`'s "state updates through
  `copyWith()`" rule and prevents an intermediate frame where the old
  controller and new id coexist.
- **Disposal-during-async test added** — `docs/client/architecture.md`
  ("All async Notifiers must have disposal tests") and
  `docs/client/testing.md` both require this; Test 4 was added to
  Phase 1.
- **Video lifecycle alignment** — confirmed the plan keeps the
  "private field on Notifier + read-only mirror in Freezed state"
  pattern from `docs/client/media.md` (§ "VideoPlayerController
  Lifecycle"). `_previewVideoController` stays a `Notifier` field;
  state's `previewVideoController` is reset to `null` on replace.
- **Widget fix scope** — verified `BackgroundMediaImage` is consumed
  by four preview modals (experience, gear, community, request).
  Phase 2's signature change touches all four, but the behavioral fix
  benefits all four uniformly, which is what we want. Sibling
  *viewmodel* fixes are still deferred to a follow-up so this PR's
  blast radius stays bounded.
- **No accessibility / i18n / observability impact** — no new
  user-visible strings, no new interactive widgets, no server-side
  changes; `docs/client/accessibility.md`,
  `docs/client/i18n.md`, and `docs/server/observability.md` rules are
  unaffected.
