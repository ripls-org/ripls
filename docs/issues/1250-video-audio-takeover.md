# Issue #1250: Audio is taking over but not playing

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1250
**Filed:** 2026-04-23 | **Last activity:** 2026-04-25
**Labels:** user-feedback | **Assignees:** tescobar | **Priority:** P2
**Context docs:** none

## Audit notes (`/ripls-audit`, 2026-05-20)

Reviewed against [docs/client/architecture.md](../client/architecture.md),
[docs/client/i18n.md](../client/i18n.md),
[docs/client/accessibility.md](../client/accessibility.md), and
[docs/client/testing.md](../client/testing.md). Server docs not applicable —
this is a pure client change with no RPC or storage impact.

Changes applied:

- **Accessibility primitives.** The extracted `VideoMuteToggleButton` MUST
  use `Tappable` with `semanticsLabel`, not `GestureDetector` / `InkWell` /
  `IconButton`. The existing community implementation already does this;
  the audit added explicit task language so the extraction can't regress.
  `scripts/check_a11y.js` enforces this via `avoid_raw_gesture_detector` —
  the allowlist is empty, so any new violation fails CI.
- **i18n keys reused.** Confirmed that `a11yMiscMute` and `a11yMiscUnmute`
  already exist in both `app_en.arb` and `app_es.arb` — no new strings
  needed and `npm run lint:dart:l10n-parity` already passes. Plan now
  states this explicitly so reviewers don't burn time on new key work.
- **ViewModel-first testing.** Per `docs/client/testing.md`, the primary
  test layer is `ProviderContainer` + Mockito at the ViewModel boundary.
  Plan now front-loads tests for `audio_session_manager` and the
  Experience `toggleMute()` wiring in Phase 2 rather than relegating all
  tests to a widget test in Phase 3.
- **Provider lifetime called out.** The session-scoped "any video
  unmuted" provider must NOT be `autoDispose` — feed swipes dispose
  per-screen providers, and unmute state must persist across them per
  the recommendation in Open Questions. Added the constraint to the
  Phase 2 task.
- **Helper, not controller.** Architecture doc forbids "controllers" in
  the presentation layer but allows helpers under `core/` for SDK
  lifecycle (e.g. `DiscoverMapHelper` for Mapbox). The `audio_session`
  glue belongs in `core/audio/` and is correctly framed as a helper —
  no rename needed, but added an explicit note to prevent reviewer
  confusion.
- **Build / lint verification.** Each phase now ends with explicit
  `flutter analyze`, `npm run lint:dart:a11y`, and
  `npm run lint:dart:i18n` so violations are caught locally rather than
  in CI.
- **Caching / data layer.** Not applicable. No repository or service
  changes; this work touches widgets, view models, and a new
  `core/audio/` helper. No cache key naming concerns.

## Problem Statement

When a user opens the feed and a feed item (specifically an experience) has a
video background, any audio playing from another app on the device (Spotify,
Apple Music, podcast app, etc.) is silenced — even though the feed video
itself plays inaudibly. Two distinct user-visible defects are mashed together
here, and the plan needs to address both:

1. **Audio session hijack (the actual bug).** The video plays muted
   ([VideoBackgroundHost.dart:55,82](app/lib/presentation/widgets/media/video_background_host.dart#L55))
   but the underlying iOS `AVAudioSession` category is still `playback`,
   which is the `video_player` plugin's default. The `playback` category
   interrupts other audio sources regardless of the player's volume. Setting
   `controller.setVolume(0.0)` does not change the category, so other apps
   get ducked/stopped for no audible reason.
2. **No way to unmute (the missing affordance).** Background videos default
   to muted everywhere
   ([video_cache_helper.dart:19](app/lib/core/utils/video_cache_helper.dart#L19)),
   which is correct. But for experience, request, and gear content views,
   there is no UI toggle to unmute. Only the community content view exposes
   a volume button
   ([community_content_view.dart:562-588](app/lib/presentation/screens/communities/community_content_view.dart#L562-L588));
   the others have `isMuted` state plumbed through but no button. The user
   wants explicit control: "videos should default to audio off and have a
   toggle."

The current branch is named `audio` but is identical to `main` — no work
has been started.

## Context from Comments

Only one comment, from `/ripls-triage` on 2026-04-24, setting priority to
P2 with rationale "User-reported: feed video silently hijacks audio without
playing. Affects all browsing users. Product lens." No further discussion,
no clarification from the author, no maintainer direction on scope. The
reporter is the assignee, which suggests this is internally owned rather
than awaiting external follow-up.

The absence of debate means two scope decisions are open:
- Does the fix apply only to experiences (the explicit report) or to all
  content types that show background videos (experience, request, gear,
  community, story)? The audio-session fix is global; the toggle UI
  decision is per-content-type. See Open Questions.
- Is muted-by-default the final behaviour, or do we eventually want the
  feed to play audio when the user has manually unmuted? See Open Questions.

## Potential Solutions

### Option A: Audio-session fix only (minimum bug fix)

- **Approach:** Add the `audio_session` package, configure the iOS audio
  session to `.ambient` (or `.soloAmbient`) at app startup so muted video
  playback no longer interrupts other apps. Do nothing about UI toggles.
- **Pros:** Smallest change. Eliminates the user's reported symptom — audio
  takeover stops. One file touched in app initialization plus pubspec.
- **Cons:** The user explicitly asked for a toggle. If they later unmute a
  video, `.ambient` won't mix correctly with other audio sources; we'd need
  a category swap on unmute. Leaves the existing inconsistency (community
  has a button, experience does not) in place.
- **Effort:** Small.

### Option B: Audio-session fix + experience-only toggle (scoped to the report)

- **Approach:** Option A plus add a volume toggle button to
  `ExperienceContentView` mirroring the community implementation. When a
  video is unmuted, swap the audio session to `.playback` (with
  `mixWithOthers: true` if we want to coexist) so audio is actually
  audible; swap back to `.ambient` when re-muted or when no video is
  unmuted on screen.
- **Pros:** Directly addresses the issue as written. Resolves the audio-
  session hijack and gives the user the requested toggle. Limits blast
  radius and review surface.
- **Cons:** Reinforces the inconsistency — request and gear content views
  still won't have a toggle. The category-swap logic adds modest
  complexity that needs to live somewhere central.
- **Effort:** Small-medium.

### Option C: Audio-session fix + toggle parity across all content types

- **Approach:** Option B, plus add the same volume toggle to
  `RequestContentView`, `GearContentView`, and decide whether
  `StoryContentView` (currently hardcoded muted) should get one too. Add
  the missing `toggleMute()` to `RequestNotifier`. Lift the volume button
  widget into a shared helper so the four call sites share one
  implementation.
- **Pros:** Removes the inconsistency in one pass. Future content types
  inherit the affordance by following the same pattern. The audio-session
  swap logic only needs to be written once.
- **Cons:** Larger diff, more screens to validate visually. Story is
  intentionally hardcoded muted today and the design intent there may not
  match (story autoplay + tap-to-advance UX is different from feed). Risks
  scope creep into a UX-consistency project.
- **Effort:** Medium.

## Recommendation

**Option B**, with the volume-button widget extracted to a reusable
component from the outset so Option C follow-ups are cheap.

Reasoning: the issue is filed against the experience feed and the audio-
hijack symptom is the real defect. Fixing that and adding the requested
toggle on experiences delivers the exact user ask. Pulling request/gear/
story in scope is tempting but each one needs visual placement decisions
(experience doesn't use `ContentOwnerRow`, so the community pattern
doesn't drop in directly) and they weren't in the report. Extract the
button widget to `app/lib/presentation/widgets/media/` so completing
Option C later is a few-line PR per content type rather than a
copy-paste exercise.

The dependency this hinges on: confirming `audio_session` is the right
package (it is the canonical Flutter plugin for `AVAudioSession` /
`AudioFocus` configuration and is widely used; no concerns) and that
swapping between `.ambient` and `.playback` on the fly works on iOS
without re-initializing controllers. The
[audio_session README](https://pub.dev/packages/audio_session) documents
exactly this flow.

## Open Questions

**Resolution (2026-05-20):** All six recommendations below are **accepted
as-decided** by the assignee. Implementation proceeds per the answers
shown.

- **Should the audio session swap from `.ambient` → `.playback` happen
  per-controller or app-wide?** — **Decided: app-wide swap.** Only one
  video can be audible at a time in this UI; track a single "any video
  unmuted" flag and configure the session accordingly. Per-controller
  plumbing duplicates what the OS already manages globally.
- **When unmuted, should we duck other apps (`.playback` exclusive) or
  mix (`.playback` with `mixWithOthers: true`)?** — **Decided: duck.**
  When the user explicitly taps unmute, they want to hear the video; if
  their music keeps playing over it, the unmute affordance feels broken.
  Matches Instagram / TikTok behaviour.
- **Should the toggle state persist across feed items or reset to muted
  when the feed advances?** — **Decided: persist for the session
  (in-memory only).** Once the user unmutes, subsequent videos play
  unmuted until they re-mute or kill the app. State lives in a
  non-`autoDispose` shared provider above the per-content view models.
- **Android parity — does Android also exhibit the audio-session
  hijack?** — **Decided: configure both platforms** via
  `audio_session`'s `AudioSessionConfiguration`. The package abstracts
  both platforms; doing iOS-only would re-introduce the same class of
  bug on Android later.
- **Where does the volume button live visually on
  `ExperienceContentView`?** — **Decided: top-right corner**, inside
  the existing `Stack` in `_buildContent`, at the same y-offset
  community uses (just below the status bar). Reuse the 32×32 circular
  black-translucent style from
  [community_content_view.dart:573-587](app/lib/presentation/screens/communities/community_content_view.dart#L573-L587).
- **Should muted toggle UI hide for non-video backgrounds?** —
  **Decided: yes**, mirror community's `editState.isVideo` guard.
  Show only when the background is actually a video.

## Implementation Plan

### Phase 1: Stop the audio-session hijack

**Goal:** Remove the reported symptom — other apps' audio is no longer
interrupted when a muted feed video appears.

**Tasks:**
- [ ] Add `audio_session` to `app/pubspec.yaml` using the existing
  caret-pin convention (`^x.y.z` — look up the current latest release on
  pub.dev rather than guessing a version range).
- [ ] Create `app/lib/core/audio/audio_session_manager.dart` exposing a
  small API: `configureForMutedPlayback()` (sets `.ambient`,
  `mixWithOthers: true`) and `configureForAudiblePlayback()` (sets
  `.playback`, duckOthers behaviour). This is a **helper** in the
  architecture sense — not a controller, not a ViewModel — analogous to
  `DiscoverMapHelper`. It owns an external-SDK lifecycle (iOS
  `AVAudioSession` / Android `AudioFocus` via the `audio_session`
  plugin) and exposes pure async methods.
- [ ] Add `app/lib/core/audio/README.md` covering purpose, when to add
  code here vs. elsewhere, and pointing at the `audio_session_manager`
  type. Required by the project's "every multi-file directory under
  `app/lib/` must have a README" rule (CLAUDE.md "Style Guidelines").
- [ ] Wire `configureForMutedPlayback()` once at app startup in
  `app/lib/main.dart` after Flutter binding initialization but before
  the first frame, so the session is correctly configured before any
  feed video controller initializes.
- [ ] Add a ViewModel-style unit test for the manager under
  `app/test/core/audio/audio_session_manager_test.dart` covering both
  configure calls. Mock the `audio_session` package boundary so the
  test runs without platform plugins. Pattern after the
  `ProviderContainer` + Mockito examples in `docs/client/testing.md`.
- [ ] Run `flutter analyze`, `npm run lint:dart:a11y`,
  `npm run lint:dart:i18n`, and `flutter test --reporter expanded`.
- [ ] Verify on a physical iOS device that playing music in Apple Music
  / Spotify continues uninterrupted when the feed loads an experience
  with a video background. Repro the exact scenario from the issue.

**Validates:** The user's reported bug is fixed even without any UI
changes. If music keeps playing while a muted feed video appears, the
audio-session category change is doing its job.

### Phase 2: Add the volume toggle to experience feed videos

**Goal:** Give users the requested ability to unmute experience
background videos, with the audio session correctly swapping when they
do.

**Tasks:**
- [ ] Extract the community volume button into a reusable
  `VideoMuteToggleButton` widget in
  `app/lib/presentation/widgets/media/video_mute_toggle_button.dart`.
  Inputs: `bool isMuted`, `VoidCallback onTap`. **Must use `Tappable`
  with `semanticsLabel` pulled from `context.l10n.a11yMiscMute` /
  `context.l10n.a11yMiscUnmute`** — never raw `GestureDetector` /
  `InkWell` / `IconButton`. Both l10n keys already exist in `app_en.arb`
  and `app_es.arb` (no new strings needed). Visual treatment (32×32
  circular, `Colors.black.withValues(alpha: 0.4)` background, white icon)
  copies the existing community widget verbatim. Replace the existing
  inline button in `community_content_view.dart` with the extracted
  widget to prove it covers the existing call site.
- [ ] Add the `VideoMuteToggleButton` to `ExperienceContentView` inside
  the existing `Stack`, positioned at top-right and gated on
  `state.isVideo`. Wire `onTap` to the existing
  [experience_view_model.dart:862](app/lib/presentation/viewmodels/experience_view_model.dart#L862)
  `toggleMute()` method.
- [ ] Add a session-scoped `videoUnmutedProvider`
  (`NotifierProvider<bool>` or `StateProvider<bool>` — single boolean
  doesn't need Freezed) at `app/lib/presentation/viewmodels/video_audio_view_model.dart`.
  **MUST NOT be `autoDispose`** — feed swipes dispose per-screen
  providers, but the user's "I want sound" decision must persist across
  swipes per the Open Questions recommendation. Any content view's
  `toggleMute()` updates this provider; `audio_session_manager`
  subscribes (via `ref.listen` from a root-level consumer set up in
  `main.dart`) and swaps categories.
- [ ] When a controller becomes unmuted, ensure its `setVolume(1.0)` is
  called *after* the audio session has swapped to `.playback`. The
  existing
  [VideoBackgroundHost.dart:52-56](app/lib/presentation/widgets/media/video_background_host.dart#L52-L56)
  reorders cleanly if the session manager exposes a `Future` to await.
- [ ] **ViewModel tests** (primary layer per `docs/client/testing.md`):
  - Verify `ExperienceNotifier.toggleMute()` flips `state.isMuted` and
    pushes the new value into `videoUnmutedProvider` (use
    `ProviderContainer` with a mocked `audio_session_manager`).
  - Verify `videoUnmutedProvider` → `audio_session_manager` wiring:
    flipping the provider triggers the expected
    `configureForAudiblePlayback()` / `configureForMutedPlayback()` call.
  - Disposal safety test for any new notifier per the "disposal testing"
    pattern in `docs/client/architecture.md` Appendix.
- [ ] **Widget smoke test** for `VideoMuteToggleButton` covering
  muted/unmuted icon swap, `onTap` invocation, and that the rendered
  `Semantics` exposes the correct `l10n` label. Wrap the test widget in
  the standard `localizationsDelegates` / `supportedLocales` boilerplate
  from `app/test/helpers/l10n_helpers.dart`.
- [ ] Run `flutter analyze`, `npm run lint:dart:a11y`,
  `npm run lint:dart:i18n`, `npm run lint:dart:l10n-parity`, and
  `flutter test --reporter expanded`.
- [ ] Manual QA: on iOS, tap unmute on an experience video while Spotify
  is playing — Spotify should duck/stop, video audio should be audible.
  Tap re-mute — Spotify resumes, video silenced. Swipe to the next feed
  item — verify behaviour matches the persistence decision in Open
  Questions.

**Validates:** Issue #1250 is closeable. The user can unmute, audio is
actually audible, and re-muting restores the no-interrupt state from
Phase 1.

### Phase 3 (optional follow-up, separate PR): Parity across content types

**Goal:** Add the same toggle to request and gear content views so the
affordance is consistent everywhere a video background appears in the
feed.

**Tasks:**
- [ ] Add `VideoMuteToggleButton` to `RequestContentView` and wire to
  request notifier. The notifier currently lacks `toggleMute()` — add
  it, mirroring the experience implementation. Add ViewModel tests for
  the new `toggleMute()` method.
- [ ] Add `VideoMuteToggleButton` to `GearContentView` and wire to
  [gear_view_model.dart:444](app/lib/presentation/viewmodels/gear_view_model.dart#L444)
  `toggleMute()`. Add ViewModel tests confirming Gear toggle pushes
  into `videoUnmutedProvider`.
- [ ] Decide separately whether `StoryContentView`'s hardcoded
  `isMuted = true` should get a toggle (story UX may differ — defer
  unless product confirms).
- [ ] Re-run `flutter analyze`, `npm run lint:dart:a11y`,
  `npm run lint:dart:i18n`, and `flutter test --reporter expanded`.

**Validates:** No content type in the feed has a silently-hijacked audio
session or a missing unmute toggle. Future content types follow the
established pattern.

## Related Issues

- None found via `gh issue list` for "audio" / "mute" / "video" keywords.
  If a story-mute design issue exists separately, link it from Phase 3
  before that phase starts.
