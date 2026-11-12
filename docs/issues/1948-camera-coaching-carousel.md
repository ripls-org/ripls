# Issue #1948: create: open unified-create to camera with coaching carousel (v2 design)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1948
**Filed:** 2026-05-15 | **Last activity:** 2026-05-15
**Labels:** _none_ | **Assignees:** unassigned
**Context docs:** [docs/cowork/App Design/bottom-drawer-standalone-v2.html](../cowork/App%20Design/bottom-drawer-standalone-v2.html), [docs/concepts/create/camera.png](../concepts/create/camera.png)

> **Audit pass — 2026-05-15.** Plan reviewed against [docs/client/architecture.md](../client/architecture.md), [docs/client/i18n.md](../client/i18n.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/testing.md](../client/testing.md), and [docs/client/caching.md](../client/caching.md). Server docs (observability, server architecture) intentionally not consulted — this work has zero server, repository, or RPC surface. Audit-driven changes are summarized in the "Audit notes" callouts below each affected section. The most consequential adjustments: (a) viewfinder corner brackets must not hardcode `Colors.white` — use the `AppColors` token family the rest of the modal already uses; (b) accessibility primitives are *not* optional — every interactive widget on the camera screen routes through `Tappable` / `IconAction`, every duration through `accessibleDuration(context, …)`, and the cycling nudge text must be wrapped in `LiveRegion`; (c) test strategy splits into a ViewModel test (default `inputMode`, `captureAndStartFromCamera`, disposal safety — `ProviderContainer`, no widgets) and a thinner widget test for the carousel UI; (d) the new method is async on a non-`autoDispose` provider, so `ref.mounted` is *not* required — but logging via `_log` *is*.

## Problem Statement

The unified-create modal currently opens to the Text tab. Even after the user taps Image, the panel shown is just two large icon buttons — Camera and Gallery — each of which delegates to the OS native picker (`ImagePicker.pickImage`). There is no in-app live camera preview anywhere in the unified-create flow.

The v2 mock at [docs/cowork/App Design/bottom-drawer-standalone-v2.html](../cowork/App%20Design/bottom-drawer-standalone-v2.html) collapses the two-tap path (Image tab → Camera button → OS picker → shoot) into a single tap (FAB → live preview → shutter). It also adds a coaching strip — an eyebrow label "CAPTURE ANYTHING", a 4-thumbnail carousel that cycles every 3 s, and a nudge sentence under the carousel — so first-time users immediately learn what kinds of things the camera is good at capturing (gear, clothes, food, event flyers).

The non-obvious part: this is **not just a UI swap**. The current `pickAndStartFromImage()` flow at [unified_create_view_model.dart:186-221](../../app/lib/presentation/viewmodels/unified_create_view_model.dart#L186-L221) calls `MediaPickerHelper.pickImageFromCamera()`, which under the hood is `ImagePicker.pickImage(source: ImageSource.camera)` — the OS native camera UI, presented as a separate modal route. The new design requires an **embedded live `CameraController` preview** inside the unified-create modal, exactly like [create_gear_modal.dart:309-315](../../app/lib/presentation/screens/gear/create_gear_modal.dart#L309-L315) already does. So the work is: rip out the OS-picker path for the Camera entry, host a live `CameraViewport` behind the input drawer, and add a new "capture" code path that calls `CameraController.takePicture()` and then enters the existing upload-and-stream flow.

## Context from Comments

The issue has no comments — it was filed in the same session as this plan. All requirements are spelled out in the issue body's "What changes", "Out of scope", and "Acceptance criteria" sections; there's no maintainer back-and-forth to reconcile. Ambiguities therefore live entirely in the body, primarily around how the live-camera plumbing interacts with the existing OS-picker path. See Open Questions below.

## Existing Implementation Analysis

**Default state.** [unified_create_state.dart:78](../../app/lib/presentation/viewmodels/unified_create_state.dart#L78) sets `@Default(CreateInputMode.text)` on the freezed factory. `reset()` at [unified_create_view_model.dart:72-80](../../app/lib/presentation/viewmodels/unified_create_view_model.dart#L72-L80) re-constructs a fresh `UnifiedCreateState()` on every modal open and then seeds `selectedCommunityIds`; it inherits the freezed default.

**Image panel today.** [unified_create_input_drawer.dart:326-355](../../app/lib/presentation/screens/create/unified_create_input_drawer.dart#L326-L355) renders a `Row` of two `_IconAction` cards (Camera, Gallery). Tapping either calls `vm.pickAndStartFromImage(fromCamera: …)` which routes to the OS picker, uploads, and transitions to preview. There is no in-modal preview state.

**CameraViewport reuse.** [camera_viewport.dart](../../app/lib/presentation/widgets/creation/camera_viewport.dart) (309 lines) already encapsulates camera-permission errors, no-camera-available, loading, live preview, gallery picker, and a captured-image preview-with-clear-X. It exposes:
- `selectedImagePath`, `onImageCaptured`, `onImageCleared` for the file-based flow
- `onCameraReady(CameraController?)` so the host can hold a reference and call `takePicture()`
- `instructionText` (passed to `CameraInstructionOverlay`) — single string, fades after 3 s
- Gallery button currently rendered at [camera_viewport.dart:200-202](../../app/lib/presentation/widgets/creation/camera_viewport.dart#L200-L202) in the lower-RIGHT

`create_gear_modal.dart` is the only existing consumer and is the working reference for "host a CameraViewport + own the CameraController + capture on shutter tap".

**Cycling-hint pattern.** [unified_create_input_drawer.dart:146-177](../../app/lib/presentation/screens/create/unified_create_input_drawer.dart#L146-L177) (Text panel) and [unified_create_input_drawer.dart:247-277](../../app/lib/presentation/screens/create/unified_create_input_drawer.dart#L247-L277) (URL panel) already do the exact "shuffle list, `Timer.periodic(3s)`, modulo wrap, `if (!mounted) return`, cancel in dispose" pattern the carousel needs. Mirror it.

**Theme tokens** (per user's standing feedback memory — reuse existing tokens, never invent per-section coral/cream constants). [app_colors.dart:590-730](../../app/lib/core/theme/app_colors.dart) defines `modalSurface` (white @ 15%), `modalBorder` (white @ 30%), `modalTextPrimary` (white), `modalBackdropBlurSigma` (4.0) — exactly the translucent-glass-over-content vocabulary the carousel needs. Note `CameraInstructionOverlay` at [camera_instruction_overlay.dart:60](../../app/lib/presentation/widgets/creation/camera_instruction_overlay.dart#L60) currently hardcodes `Colors.white.withValues(alpha: 0.9)` — that's an existing theme-token bypass that the new carousel must not replicate.

> **Audit note (architecture).** [docs/client/architecture.md § Colors and Theming](../client/architecture.md) is explicit: "Don't use hardcoded hex `Color(0xFF...)` values for UI elements — add a named helper to `AppColors` if a new semantic color is needed." The corollary for `Colors.white` is in the same section: hardcoded white is invisible in light mode and harsh in dark mode. That applies even to white-on-camera overlays — the viewfinder corner brackets must read through `AppColors.modalTextPrimary` (which resolves to pure white) rather than the literal `Colors.white`. If a translucent variant is needed (e.g. 85 % alpha for the corners), use `AppColors.modalTextPrimary.withValues(alpha: 0.85)` rather than `Colors.white.withValues(alpha: 0.85)`. The two render identically today but the indirection is what lets a future theme reskin re-tune the value in one place.

**Accessibility primitives.** Per [docs/client/accessibility.md](../client/accessibility.md), every interactive widget added by this work must route through the primitives in [app/lib/presentation/widgets/accessibility/](../../app/lib/presentation/widgets/accessibility/), not raw `GestureDetector` or `IconButton`. Concretely:
- Each thumbnail in the carousel is a `Tappable` with a `semanticsLabel` (uses a `Toggle` pattern is overkill — the thumbnails are coaching examples, not toggles).
- The shutter button is a `Tappable` (or `IconAction` if it ends up icon-only — it isn't in the mock; the mock shows a filled white circle, which is a `Tappable` with a custom-painted child).
- Every `Duration(milliseconds: ...)` on `AnimatedContainer`, `AnimatedSwitcher`, etc. is wrapped in `accessibleDuration(context, …)`. The `prefer_accessible_duration` lint rule (in `scripts/check_a11y.js`) will fail CI otherwise.
- The cycling nudge text is wrapped in `LiveRegion` so VoiceOver / TalkBack re-announces it on each tick. Without `LiveRegion`, the carousel cycles silently for screen-reader users.
- `Timer.periodic` itself is not animation — `accessibleDuration` does not apply to the 3 s tick interval. Reduce-motion users will still see the cycle (consistent with the existing text-panel cycling pattern, which makes the same call).

**ARB conventions.** [app_en.arb:6046-6142](../../app/lib/l10n/app_en.arb) follows `unifiedCreate{Component}` naming with per-string `@description` blocks. There is **no** existing `unifiedCreateCamera*Coaching*` namespace — these strings will be new in both `app_en.arb` and `app_es.arb`.

> **Audit note (i18n).** [docs/client/i18n.md](../client/i18n.md) calls out two things this plan now incorporates: (1) **Each new string needs both locales** — Spanish translations must be actual translations, not English copies. Don't ship `app_es.arb` with the English strings as placeholders; the `nullable-getter: false` config will not catch this, but reviewers will. (2) **Accessibility-visible strings use the `a11y*` prefix.** The four thumbnail buttons each carry a `semanticsLabel`, which is accessibility-visible — those keys belong in the `a11y*` namespace (e.g. `a11yUnifiedCreateCoachingGear`), not under `unifiedCreate*`. The visible coaching text (eyebrow, nudge) stays under `unifiedCreate*` because it's regular UI content. The `require_l10n_for_semantic_labels` lint rule in `scripts/check_a11y.js` will fail CI if any `semanticsLabel:` argument is a raw string. **Net key count for this issue: 9 new strings × 2 locales = 18 ARB entries** (5 `unifiedCreate*` + 4 `a11y*`).

**Modal lifecycle.** [unified_create_modal.dart:34-62](../../app/lib/presentation/screens/create/unified_create_modal.dart#L34-L62) calls `reset()` before pushing the bottom sheet via `showAccessibleModal`. No pre-flight camera permission check exists — the OS picker handles its own permissions today. Switching to a live preview means **the modal itself now triggers the camera permission prompt** on first open. `CameraViewport` already handles the denied case (renders an error message and still allows gallery selection), so no new permissions plumbing is strictly required — but the first-launch UX needs to be verified on both platforms.

**Tests.** Reference [unified_create_input_drawer_test.dart](../../app/test/presentation/screens/create/unified_create_input_drawer_test.dart) uses a `_FakeVm` override of `unifiedCreateViewModelProvider` plus a `MaterialApp` with the standard l10n delegates. The carousel cycling test pumps `Duration(seconds: 4)` to cross the 3 s tick boundary.

> **Audit note (testing).** [docs/client/testing.md](../client/testing.md) is explicit that ViewModels are the primary test target ("Test business logic, not Flutter widgets"). That means the test split for this work is:
> - **ViewModel tests** (`app/test/presentation/viewmodels/unified_create_view_model_test.dart`): default `inputMode == CreateInputMode.image` after `reset()`, `captureAndStartFromCamera(path)` happy-path + error-path, disposal during in-flight upload. Use `ProviderContainer` + Mockito-mocked `MediaRepository`. No `MaterialApp`, no widgets, no l10n setup.
> - **Widget tests** (`app/test/presentation/widgets/creation/camera_coaching_carousel_test.dart` and `unified_create_input_drawer_test.dart` extensions): carousel cycling + tap-to-lock, eyebrow / nudge rendering, gallery-button position. Pump the widget in isolation, not the full modal — the real `CameraViewport` requires hardware and is not under test here.
>
> The doc also requires a disposal test for async Notifiers ("All async Notifiers **must** have disposal tests"). The new `captureAndStartFromCamera` is async — disposal during in-flight upload must complete without throwing. This is a `ProviderContainer.dispose()` test, not a widget test.

**Async safety.** `unifiedCreateViewModelProvider` is a regular `NotifierProvider`, **not** `autoDispose`. The comment in [unified_create_modal.dart:36](../../app/lib/presentation/screens/create/unified_create_modal.dart#L36) (`"Reset state before each open since the provider isn't autoDispose"`) confirms this. Per [docs/client/architecture.md § Async Safety](../client/architecture.md), `ref.mounted` checks after async gaps are required *for `autoDispose` providers*. This provider is not autoDispose, so the existing `pickAndStartFromImage` method does not add those checks — and the new `captureAndStartFromCamera` should follow the same precedent. Reviewers may ask; the answer is documented here.

**Logging.** [unified_create_view_model.dart:1-30](../../app/lib/presentation/viewmodels/unified_create_view_model.dart) initialises a `_log` field (Dart's `logging` package). The existing `pickAndStartFromImage` does `_log.warning('image upload + dispatch failed', e, s)` on error. The new `captureAndStartFromCamera` should mirror this with `_log.info('camera capture: $path')` on success and `_log.warning('capture upload + dispatch failed', e, s)` on error. Don't introduce a new logger.

## Potential Solutions

### Option A: Embed `CameraViewport` directly in the input drawer's Image panel
- **Approach:** Keep all camera UI inside `UnifiedCreateInputDrawer`. When `inputMode == image`, the drawer's `AnimatedSize` expands to fill the available space above the tab strip and renders the `CameraViewport` + coaching carousel inside the same `GlassSurface` container that hosts the Text and URL panels. The drawer owns the `CameraController` and calls `takePicture()` on shutter tap.
- **Pros:** Single widget tree — same scope as `Text` and `URL` panels. No layout coordination between modal and drawer. Disposal is trivially correct because everything lives under `_UnifiedCreateInputDrawerState`.
- **Cons:** Breaks the mock's visual hierarchy. The mock shows the camera **full-bleed behind** the drawer, with the drawer collapsed to a thin tab strip (≈80 px) at the bottom — the camera is not contained inside a glass card, it's the background. Reproducing that inside an `AnimatedSize`-bounded drawer would require fighting the existing layout (drawer would have to expand to nearly full screen, which conflicts with how Text/URL panels size themselves).
- **Effort:** Medium (3-4 days). Most of the cost is layout fights, not feature work.

### Option B: Camera as a sibling layer in `UnifiedCreateModal`, drawer collapses
- **Approach:** Add a new `_CameraLayer` sibling to the existing drawer/preview in [unified_create_modal.dart:71-198](../../app/lib/presentation/screens/create/unified_create_modal.dart). When `stage == input && inputMode == image`, the modal stacks `_CameraLayer` (full-bleed `CameraViewport` + viewfinder corners + coaching carousel + shutter button) underneath the input drawer; the drawer simultaneously shrinks to "tab strip only" (no panel content) via the existing `AnimatedSize` in [unified_create_input_drawer.dart:60-103](../../app/lib/presentation/screens/create/unified_create_input_drawer.dart#L60-L103). Camera controller and capture logic live in `_CameraLayer`'s state — not the view model.
- **Pros:** Matches the v2 mock layout one-to-one (camera fills the screen, glass tab strip floats above it). Reuses the existing drawer collapse pattern. Clean separation: the drawer keeps owning Text/URL inputs; the new camera concern lives in its own component. Disposal of `CameraController` is bounded by `_CameraLayer` lifecycle, which mounts only when Image is selected.
- **Cons:** The view model no longer owns the camera state — capture is triggered from the widget tree, then it calls back into the VM to upload and stream. That callback wiring adds one indirection (widget → vm). Same indirection pattern is used by `create_gear_modal.dart`, so it's familiar but not free.
- **Effort:** Medium (3-4 days).

### Option C: New dedicated `UnifiedCreateCameraScreen` route, push instead of inline
- **Approach:** Keep the existing drawer Image panel as a fallback. When the modal opens, if `inputMode == image` is the default, immediately push a separate full-screen `UnifiedCreateCameraScreen` route that contains the camera + carousel. The screen pops back into the modal preview stage after capture.
- **Pros:** Cleanest separation — camera lives in its own widget file, its own route, its own state. No coordination with the drawer.
- **Cons:** Defeats the "instant camera on FAB tap" intent: the user sees the modal flash and then a screen push transition. Mock is unambiguous that the camera is *the modal*, not a screen on top of it. Also leaves dead code in the input drawer (Camera/Gallery icon buttons would still need to exist for the case where the user navigates back to Image from Text/URL).
- **Effort:** Small (2 days) — but ships a different product than what the mock specifies.

## Recommendation

**Option B.** It is the only option that matches the mock's visual hierarchy (camera full-bleed, drawer floats above as a tab strip) without fighting the existing `AnimatedSize` layout. Two tradeoffs I'm explicit about:

1. **Camera state lives in widget state, not the view model.** That's a deliberate choice — the `CameraController` is a hardware resource whose lifecycle tracks the widget, not the post being composed. `create_gear_modal.dart` already does this; I am not inventing a pattern.
2. **The drawer learns a new "collapsed to tab strip" rendering mode.** That means the existing `switch (state.inputMode)` in [unified_create_input_drawer.dart:85-97](../../app/lib/presentation/screens/create/unified_create_input_drawer.dart#L85-L97) grows a `CreateInputMode.image => SizedBox.shrink()` arm (no body content when Image is selected — the camera fills behind). The drawer becomes structurally smaller for Image mode, which the existing `AnimatedSize` handles cleanly.

If during implementation it turns out the existing modal's `Stack` order makes the camera-behind-drawer arrangement awkward (z-index fights with `_GlassIconButton`, the close button at the top-right), fall back to Option A and accept the layout-fight cost — but Option B should work on first pass.

## Open Questions

Each question below is something the implementer needs answered before starting. I've included a recommendation as a starting point.

- **Does the live camera start as soon as the modal opens, or only when the user is on the Image tab?** — **Recommendation:** Only when on Image. Mounting the camera-layer conditionally on `inputMode == image` means the `CameraController` is created lazily, the OS camera permission prompt only fires when the user is in a context where it makes sense, and switching to Text/URL releases the camera. Since Image is now the default, the practical effect is "camera starts immediately on FAB tap" — but if the user begins on Image, switches to Text, then switches back to Image, the camera tears down and re-initializes. That's the simplest correct lifecycle.

- **What happens to `pickAndStartFromImage(fromCamera: true)` — does it become dead code?** — **Recommendation:** Delete it. After this change there is no remaining call site for the OS-camera path inside unified-create. Keep `pickAndStartFromImage(fromCamera: false)` for the gallery button. Search for any other callers before deleting — `MediaPickerHelper.pickImageFromCamera()` likely still has callers in `create_gear_modal.dart` and the legacy creation modals, so do not remove the helper, only the unified-create entry point.

- **Where does the in-app shutter capture handler live, and what does it call?** — **Recommendation:** Add a new method `captureAndStartFromCamera(String filePath)` to `UnifiedCreateViewModel`. It does exactly what `pickAndStartFromImage(fromCamera: false)` does today after the picker returns a path — set streaming state, upload via `mediaRepository.addMedia()`, then `_dispatchStream()`. The widget layer calls `await _cameraController.takePicture()` to get the path, then forwards it to the VM. Don't bury `takePicture()` inside the VM — `CameraController` is a UI-layer concern.

- **What are the four nudge strings and the eyebrow label exactly?** — **Recommendation:** Use the v2 mock's strings verbatim so the implementation matches the design without a separate copy review:
  - Eyebrow: "Capture anything"
  - Nudge 1 (gear thumb): "Try gear, games, or anything you'd lend"
  - Nudge 2 (clothes thumb): "Lay clothes flat and shoot straight down"
  - Nudge 3 (food thumb): "Snap what you can't finish on your own"
  - Nudge 4 (event flyer thumb): "Get the whole flyer in the frame"

  All five go into `app_en.arb` + `app_es.arb` with `@description` blocks. The eyebrow stays in caps via the widget's `TextStyle` (`letterSpacing` + uppercase) so localizers don't have to ship per-locale caps.

- **What thumbnail images does the carousel use?** — **Recommendation:** Bundle four small (≈120 px) JPEG assets under `app/assets/images/create/coaching/` (`gear.jpg`, `clothes.jpg`, `food.jpg`, `flyer.jpg`) and reference them via `AssetImage`. The mock uses Unsplash URLs, but the app should not depend on network availability for an onboarding moment. Image sourcing can be a follow-up issue if assets aren't ready — temporarily fall back to colored placeholders that match the AppColors palette.

- **Does the carousel's auto-cycle pause when the user navigates away from Image?** — **Recommendation:** Yes. The carousel widget mounts only while `inputMode == image && stage == input`, so its `Timer.periodic` is cancelled automatically by `dispose()` when the user switches tabs or moves to preview. The "tap a thumbnail to lock" feature already kills the timer; no extra state coordination needed.

- **How does the carousel render at small screen heights (e.g. SE-class device, 568 pt tall)?** — **Recommendation:** The carousel sits 200 px above the bottom in the mock, which leaves room above the shutter (≈100 px) and the drawer's tab strip (≈80 px). On a 568-pt-tall screen with the tab strip plus shutter plus carousel, vertical space is tight. Make the carousel's vertical position responsive — anchor it 24 dp above the shutter using a `Column` with `MainAxisAlignment.end`, rather than an absolute `bottom: 200`. The mock's `bottom-[200px]` is for a fixed-size phone frame; the Flutter version should be flexible.

- **Should the gallery picker still close the modal and re-open it (current behavior)?** — **Recommendation:** No. The gallery flow today triggers `ImageSource.gallery` which presents an OS modal route; on dismiss, the unified-create modal is still mounted underneath. That keeps working. The only thing that changes is the button's *position* (lower-left, not lower-right) and the icon's container styling to match the mock's translucent glass square.

- **What's the test strategy for the live camera, given that widget tests don't have a real camera?** — **Recommendation:** Split into ViewModel tests (no widgets) and a thin widget test for the carousel. Per [docs/client/testing.md](../client/testing.md), the primary target is ViewModels. ViewModel tests use `ProviderContainer` with a Mockito-mocked `MediaRepository`; widget tests pump the `CameraCoachingCarousel` in isolation. **Do not** try to test the full modal with a stubbed `CameraViewport` — that's reaching into the hardware integration layer where flake risk is high. Tests cover: (a) `inputMode` defaults to Image after `reset()` (VM test), (b) `captureAndStartFromCamera(path)` happy path: state transitions to preview, `addMedia` is called once, `_dispatchStream` fires (VM test), (c) `captureAndStartFromCamera(path)` error path: state's `errorMessage` is set, `streaming` is false (VM test), (d) disposal during in-flight upload completes without throwing (VM test, `container.dispose()` mid-future), (e) carousel cycles every 3 s (widget test, pump 4 s), (f) tapping a thumbnail locks the index (widget test, pump 6 s after tap), (g) gallery button is on the left half of the viewport (widget test, find by `Key('camera-gallery-button')`, check `Offset.dx < screenWidth / 2`).

- **What if the device doesn't have a camera at all (older simulator, disabled hardware)?** — **Recommendation:** `CameraViewport` already renders an error state with "Use the gallery button to select a photo" ([camera_viewport.dart:251-258](../../app/lib/presentation/widgets/creation/camera_viewport.dart#L251-L258)). The carousel should still render on top of that error state — the coaching is content-agnostic. The shutter button hides when there's no live preview; gallery button stays visible.

- **Who owns the `CameraController` lifecycle and how does it handle app backgrounding?** — **Recommendation:** `_CameraLayer` (the new widget in `unified_create_modal.dart`) owns the controller via `onCameraReady`, stores it in widget state, and disposes it in `dispose()`. For app-lifecycle (background/resume), look at how `create_gear_modal.dart` handles `AppLifecycleState.paused/resumed` today — if it pauses + resumes the camera, lift the same pattern into `_CameraLayer`. If `create_gear_modal.dart` doesn't, the existing UX accepts whatever the `camera` package does by default. Don't invent a new lifecycle layer in this PR; match the existing one. Grep for `WidgetsBindingObserver` + `didChangeAppLifecycleState` in `create_gear_modal.dart` before implementing.

- **Where does `_CameraLayer` actually live — same file as `UnifiedCreateModal` or its own file?** — **Recommendation:** Own file: `app/lib/presentation/widgets/creation/unified_create_camera_layer.dart`. Reason: `unified_create_modal.dart` is currently 332 lines; adding a ~150-line `_CameraLayer` puts it close to the 1000-line CI cap and mixes camera-hardware concerns into the modal's layout file. Putting `_CameraLayer` next to `camera_viewport.dart` and `camera_coaching_carousel.dart` under `widgets/creation/` keeps the camera surface in one place. The modal file just imports and uses it.

- **Does the carousel image fade-cross-dissolve when cycling, or does it just hard-switch?** — **Recommendation:** The mock fades the nudge **text** (200 ms `fadeIn` keyframe in the v2 HTML) but not the thumbnails — the thumbnails' size/border are what animate (active vs inactive). Mirror that: text uses `AnimatedSwitcher` with a 300 ms fade transition; thumbnails use `AnimatedContainer(duration: 500 ms)` for size, border, and opacity. The v2 timings in the HTML (`duration-500` Tailwind) are a good starting point.

## Implementation Plan

Four phases. Each lands a meaningful slice the user can interact with; nothing is theoretical or "set up for later".

### Phase 1: Default to Image mode + embedded camera in the modal
**Goal:** Tapping the FAB shows a live camera preview behind the drawer's tab strip. Shutter captures and enters the preview stage. Gallery still works. No coaching carousel yet.

**Tasks:**
- [ ] Change [unified_create_state.dart:78](../../app/lib/presentation/viewmodels/unified_create_state.dart#L78) `@Default(CreateInputMode.text)` → `@Default(CreateInputMode.image)`. Run `flutter pub run build_runner build --delete-conflicting-outputs` to regenerate the `.freezed.dart` file.
- [ ] Add `captureAndStartFromCamera(String filePath)` to `UnifiedCreateViewModel`. Body mirrors lines 199-212 of `pickAndStartFromImage`: state transition to preview/streaming, `mediaRepo.addMedia`, `_dispatchStream`. Receives the path directly instead of calling `MediaPickerHelper`. Add `_log.info('camera capture: $filePath')` at entry and `_log.warning('capture upload + dispatch failed', e, s)` in the `catch` — match the existing logging style. **Do not** add `ref.mounted` checks; the provider is not autoDispose (see existing precedent in `pickAndStartFromImage`).
- [ ] Create `app/lib/presentation/widgets/creation/unified_create_camera_layer.dart` for the new `_CameraLayer` widget. (Why a separate file: keeps `unified_create_modal.dart` under the 1000-line CI cap and groups camera concerns with `camera_viewport.dart` / the upcoming `camera_coaching_carousel.dart`.) The widget mounts only when `state.stage == CreateStage.input && state.inputMode == CreateInputMode.image`. It hosts a full-bleed `CameraViewport` with `onCameraReady` capturing the controller into local state, a shutter `Tappable` (not a raw `GestureDetector`) centered above the drawer (≈100 dp from the bottom) with `semanticsLabel: context.l10n.a11yUnifiedCreateShutter`, and wires the shutter tap to `controller.takePicture()` then `vm.captureAndStartFromCamera(file.path)`. Dispose the camera controller in `dispose()`.
- [ ] Verify whether `create_gear_modal.dart` registers a `WidgetsBindingObserver` for `didChangeAppLifecycleState` and replicate the same pattern in `_CameraLayer` if so. Don't invent a new lifecycle policy — match existing.
- [ ] Update [unified_create_input_drawer.dart:85-97](../../app/lib/presentation/screens/create/unified_create_input_drawer.dart#L85-L97) `switch` so `CreateInputMode.image => const SizedBox.shrink()`. Delete the `_ImagePanel` and `_IconAction` classes — both become dead code. Re-run `npm run lint:ephemeral-comments` and `npm run lint:dart:size` to confirm.
- [ ] Delete the `fromCamera: true` branch logic in `pickAndStartFromImage` — rename the method to `pickAndStartFromGallery()` since it now only does one thing. Grep for callers and fix. The `MediaPickerHelper.pickImageFromCamera()` static helper has other callers (e.g. `create_gear_modal.dart`) — do **not** delete it; only remove the unified-create entry.

**Validates:** First-tap camera works end-to-end. Confirms the embedded camera plumbing is correct before any visual polish lands.

### Phase 2: Visual polish — viewfinder corners, gallery button position, ARB strings
**Goal:** The camera screen matches the v2 mock visually except for the carousel.

**Tasks:**
- [ ] Add a `_ViewfinderCorners` widget to [camera_viewport.dart](../../app/lib/presentation/widgets/creation/camera_viewport.dart). Four `Positioned` corner brackets (24×24, 2 px border, color `AppColors.modalTextPrimary.withValues(alpha: 0.85)` — **not** literal `Colors.white`; see Audit note in Existing Implementation Analysis), only the relevant two edges visible per corner. Mount it whenever `_cameraController != null && _cameraController!.value.isInitialized` (same condition as the existing instruction overlay).
- [ ] Move `_buildGalleryButton` in [camera_viewport.dart:200-202](../../app/lib/presentation/widgets/creation/camera_viewport.dart#L200-L202) from `right: 16` to `left: 16`. Add a stable `Key('camera-gallery-button')` so the widget test in Phase 4 can find it by key. Update the bottom offset so it sits next to the shutter (current value: 135 — keep it; verify visually). Update the container styling to a translucent white-on-glass square (matches the mock's `bg-white/15 border border-white/30 rounded-2xl`); reuse `AppColors.modalSurface` + `AppColors.modalBorder` rather than hardcoding `Colors.black54`. The `Tappable` wrapping already provides the `semanticsLabel` from `context.l10n.a11yMiscOpenGallery` — leave that alone.
- [ ] Add to [app_en.arb](../../app/lib/l10n/app_en.arb) (and [app_es.arb](../../app/lib/l10n/app_es.arb)) the **9 new keys** with `@description` blocks (5 visible + 4 a11y; see Audit note in Existing Implementation Analysis). Spanish translations must be actual translations, not English copies:
  - **Visible UI** (`unifiedCreate*` prefix):
    - `unifiedCreateCameraCoachingEyebrow` — EN: "Capture anything" / ES: "Captura lo que sea"
    - `unifiedCreateCameraCoachingGear` — EN: "Try gear, games, or anything you'd lend" / ES: "Prueba con equipo, juegos o cualquier cosa que prestarías"
    - `unifiedCreateCameraCoachingClothes` — EN: "Lay clothes flat and shoot straight down" / ES: "Coloca la ropa plana y fotografía desde arriba"
    - `unifiedCreateCameraCoachingFood` — EN: "Snap what you can't finish on your own" / ES: "Fotografía lo que no puedes terminar tú solo"
    - `unifiedCreateCameraCoachingFlyer` — EN: "Get the whole flyer in the frame" / ES: "Captura el folleto completo en el encuadre"
  - **Screen-reader labels** (`a11y*` prefix — required by `require_l10n_for_semantic_labels` lint):
    - `a11yUnifiedCreateShutter` — EN: "Take photo" / ES: "Tomar foto"
    - `a11yUnifiedCreateCoachingGear` — EN: "Coaching example: gear" / ES: "Ejemplo: equipo"
    - `a11yUnifiedCreateCoachingClothes` — EN: "Coaching example: clothes" / ES: "Ejemplo: ropa"
    - `a11yUnifiedCreateCoachingFood` — EN: "Coaching example: food" / ES: "Ejemplo: comida"
    - `a11yUnifiedCreateCoachingFlyer` — EN: "Coaching example: event flyer" / ES: "Ejemplo: folleto de evento"

  Spanish strings above are starting drafts; verify with a fluent speaker (the project's existing Spanish strings give the tone — informal `tú`, not `usted`) before merge.
- [ ] Run `npm run generate` to refresh `app_localizations*.dart`. Verify `npm run lint:dart:i18n` and `npm run lint:dart:a11y` pass.

**Validates:** The screen matches the mock's chrome (corners, gallery in lower-left) and localizable copy is in place — even though the carousel widget itself isn't built yet, the strings exist and `flutter analyze` is clean.

### Phase 3: Coaching carousel
**Goal:** The "CAPTURE ANYTHING" eyebrow + 4-thumbnail carousel + cycling nudge text — the marquee feature in the issue title.

**Tasks:**
- [ ] Bundle 4 thumbnail JPEGs in `app/assets/images/create/coaching/` (`gear.jpg`, `clothes.jpg`, `food.jpg`, `flyer.jpg`, ≈120 px wide, 60–80 KB each). Add the directory to `pubspec.yaml` `flutter.assets`.
- [ ] Create `app/lib/presentation/widgets/creation/camera_coaching_carousel.dart` (under 250 lines, single file). Stateful widget that:
  - Holds `_currentIndex`, `_userLocked`, `Timer? _cycleTimer`.
  - Initializes `_cycleTimer = Timer.periodic(const Duration(seconds: 3), …)` in `initState`; cancels in `dispose`. Skip the tick when `_userLocked`. Guard with `if (!mounted) return;` before `setState` (matches the existing `_TextPanelState` pattern).
  - Renders a `Column`: eyebrow text (10 pt, letter-spacing 0.16em, uppercase via `TextStyle`), 4-thumb `Row` with active/inactive sizing via `AnimatedContainer` (active: 64×64, 2 px white border, opacity 1; inactive: 50×50, 1 px white-35 % border, opacity 0.55), `AnimatedSwitcher` for the nudge text below.
  - Each thumbnail wraps in `Tappable` (NOT a raw `GestureDetector` / `InkWell` — the `avoid_raw_gesture_detector` lint rule will fail CI otherwise) with its corresponding `a11yUnifiedCreateCoachingGear` / `*Clothes` / `*Food` / `*Flyer` string as `semanticsLabel`. Tapping sets `_userLocked = true`, cancels the timer, and updates `_currentIndex`.
  - The nudge `Text` is wrapped in `LiveRegion` (from `app/lib/presentation/widgets/accessibility/`) so VoiceOver / TalkBack re-announces it on every tick.
  - **Every `Duration(...)` argument on `AnimatedContainer` / `AnimatedSwitcher` is wrapped in `accessibleDuration(context, ...)`.** The `prefer_accessible_duration` lint rule (`scripts/check_a11y.js`) will fail CI on raw `Duration` arguments.
  - Uses `AppColors.modalTextPrimary` for text, `AppColors.modalTextPrimary.withValues(alpha: 0.95)` for the active ring, and `AppColors.modalTextPrimary.withValues(alpha: 0.35)` for inactive rings. No `Colors.white` literals. No new constants in `AppColors`.
  - Reads the four nudge strings + eyebrow from `context.l10n`.
- [ ] Mount the carousel inside `_CameraLayer`, anchored 24 dp above the shutter (not absolute-positioned — use a `Column` with `MainAxisAlignment.end` and bottom padding).

**Validates:** The named-key addition from the issue title is live, matches the mock, and passes `npm run lint:dart:a11y` cleanly.

### Phase 4: Tests, accessibility audit, end-to-end QA
**Goal:** The feature can ship with confidence. Regression protection in place. Test layering follows [docs/client/testing.md](../client/testing.md) — ViewModel tests are the primary target, widget tests are surgical.

**Tasks:**
- [ ] **ViewModel tests** in `app/test/presentation/viewmodels/unified_create_view_model_test.dart` (using `ProviderContainer`, Mockito-mocked `MediaRepository`, no `MaterialApp`):
  - `inputMode` defaults to `CreateInputMode.image` after `reset()` on a fresh `ProviderContainer`.
  - `captureAndStartFromCamera('/tmp/x.jpg')` happy path: state transitions to `CreateStage.preview` with `streaming: true`, `mediaRepo.addMedia` called once with that path, `_dispatchStream` fires.
  - `captureAndStartFromCamera` error path: `mediaRepo.addMedia` throws; state ends with `streaming: false`, `errorMessage` populated, `selectorEnabled: true`.
  - Disposal safety: start `captureAndStartFromCamera`, immediately `container.dispose()`, assert the future completes without throwing (matches the disposal-test recipe in [docs/client/architecture.md § Disposal Testing](../client/architecture.md)).
- [ ] **Widget test** in `app/test/presentation/widgets/creation/camera_coaching_carousel_test.dart`:
  - Pumps the carousel in isolation under a `MaterialApp` with l10n delegates (use the existing `app/test/helpers/l10n_helpers.dart`).
  - First-tick assertion: eyebrow + nudge 1 visible.
  - Pump `Duration(seconds: 4)`: nudge 2 visible.
  - Tap thumbnail 3, pump `Duration(seconds: 4)`: nudge 3 still visible (locked, no cycle).
  - Each thumbnail is a `Tappable` (`find.byType(Tappable)` count = 4 for the thumbnail row).
- [ ] **Widget test** in `app/test/presentation/widgets/creation/camera_viewport_test.dart`: gallery button is on the left half of the viewport (find by `Key('camera-gallery-button')`, assert `Offset.dx < screenWidth / 2`). Don't bother with `RenderBox.localToGlobal` math — `find.byKey` + `tester.getCenter()` is cleaner.
- [ ] **Lint pass:** `npm run lint:dart:a11y` (a11y rules), `npm run lint:dart:i18n` (l10n rules), `npm run lint:dart:size` (file size cap), `npm run lint:ephemeral-comments` (phase-marker ban).
- [ ] **Manual QA on a physical device** (one iOS, one Android) for: first-time camera permission prompt, denied-permission gallery fallback, switching Text → Image → Text without controller leaks (verify in Xcode Instruments / Android Profiler), VoiceOver / TalkBack announces the eyebrow + active nudge via the `LiveRegion`, reduce-motion (Settings → Accessibility) honors the `accessibleDuration(...)`-wrapped animation timings (animations collapse to instant), backgrounding the app while on Image tab releases the camera and re-acquires on resume.
- [ ] Update the bottom-line acceptance criteria from the issue body and check each box in the PR description.

**Validates:** Feature ships with regression protection. Memory and lifecycle correctness verified manually because widget tests can't fully exercise the real camera hardware. The disposal safety test is what prevents the Notifier-disposed-during-async-upload class of crashes ([docs/client/architecture.md § Async Safety](../client/architecture.md)).

## Related Issues

- #1934 — Modal-button WCAG contrast on the frosted glass surface. Relevant because the new shutter / gallery button styling lands on the same glass surface vocabulary; verify the white-on-translucent stays above 3:1 contrast on a worst-case bright background.
- #1939 — "Wire image bytes through the unified-create classifier prompt (image-mode classification is currently a default-to-gear coin flip)". The new design fires the classifier on every shutter tap — if #1939 is unmerged when this lands, classification accuracy will be the same as today. They are independent but compounding: ship this UI without waiting for #1939; the UX wins are independent of classifier accuracy.
- #1946 — Heritage Sage accent migration. Touches the modal glass surface tokens but does not change their alpha values; this work uses those tokens through helpers, so it inherits any color shift cleanly. Confirm the gallery-button styling reads from `AppColors` helpers, not literals, so #1946 cascades correctly.
