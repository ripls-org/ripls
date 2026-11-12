# Issue #1980: Creation modal: use swipe-down/tap-off dismiss like other modals (keep X only on camera viewport)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1980
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Audit Notes (post-`/ripls-audit`)

Architectural-compliance fixes made after running `/ripls-audit`:

- **`avoid_raw_gesture_detector` is enforced with an empty allowlist.** Option B as originally written would have introduced a raw `GestureDetector` for tap-off dismiss in `app/lib/presentation/screens/` — that's a lint failure on `npm run lint:dart:a11y`. Option B now requires `Tappable` (which mandates a `semanticsLabel`), or `Listener` for the drag-only portion. The recommendation has been adjusted to reflect this added cost.
- **`NavigationHelpers` + `SwipeToCloseMixin` aren't a clean fit for the camera-as-separate-route variant of Option A.** Those primitives target primary content screens, and a sub-state of a modal is a poor match. Recommendation has been narrowed to Option A approach 2 (`OverflowBox` from a single route) as the lower-friction path, with Option B as fallback.
- **X-in-camera-mode semantics aligned with the issue text.** The user wrote "tap an X in the upper right corner" as an alternative to swipe-down. Both should dismiss the modal (today's behavior). Removed the conflicting open question about X = "exit camera state" — only the Android system back button still warrants a separate decision.
- **l10n + a11y label requirements made explicit.** Any new dismiss affordance needs an `a11y`-prefixed ARB key in both `app_en.arb` and `app_es.arb` per `docs/client/i18n.md` and `docs/client/accessibility.md`.
- **`accessibleDuration` callout added** for any new animations introduced by this work, per the a11y primitives table.
- **Testing guidance reframed.** `docs/client/testing.md` is ViewModel-first, but this issue is purely UI/dismiss behavior with no ViewModel logic change. Phase 1 widget tests fall under the "Widget Async Exceptions" precedent and require l10n setup via `app/test/helpers/l10n_helpers.dart`.
- **Lint gates checklist expanded** to include `lint:dart:a11y`, `lint:dart:i18n`, and `lint:nav` (the latter only matters if the camera-as-route variant is pursued).

## Problem Statement

The unified create modal (the sheet that opens when the user taps the "+"
FAB on the home screen) is the only modal in the app that closes via an
explicit X button. Every other bottom sheet — `ExperienceCreationModal`,
`RequestCreationModal`, `TimePollProposeModal`, etc. — dismisses on
swipe-down or tap-off, with no close affordance in the corner. The
inconsistency is jarring.

The user wants the creation modal aligned to the standard behavior, with
one exception: when the live-camera viewport is showing (Image input
mode), the X stays in the upper right corner because the camera fills
the screen and there is no off-modal area to tap, and a drag handle
would be obscured by camera UI.

A wrinkle the issue doesn't call out but matters for implementation: the
unified create modal currently fills the entire screen height
(`SizedBox(height: media.size.height)` in
[unified_create_modal.dart:78-79](app/lib/presentation/screens/create/unified_create_modal.dart#L78-L79)).
With a full-bleed sheet, "tap-off" has nothing to land on — the modal
covers every pixel — so simply removing the X isn't enough; the layout
also has to leave room for the barrier tap target, or wrap the
non-content area in a gesture handler. The other modals don't have this
problem because their content sizes to its children and the barrier
remains tappable above them.

## Context from Comments

There are no comments on the issue. The description is unambiguous, but
the lack of discussion means a couple of design calls (drag handle vs.
no handle, exact tap-off area, accessibility back-button behavior on
Android) need to be made during planning rather than read off the
thread.

## Potential Solutions

### Option A: Resize the sheet + rely on `showAccessibleModal` defaults
- **Approach:** Stop forcing the modal to full screen height. Let the
  content (input drawer + preview card) size itself, the way
  `ExperienceCreationModal` and `RequestCreationModal` do. Remove the
  X for non-camera states. `showAccessibleModal` already defaults
  `isDismissible: true` and `enableDrag: true`, so swipe-down and
  barrier-tap-to-dismiss start working as soon as there's an exposed
  barrier and a top edge for the drag to grab. Keep the X mounted only
  while `state.stage == input && state.inputMode == image`.
- **Pros:** Matches the existing pattern in the codebase exactly — same
  helper, same flags, same behavior. Cheapest path to "consistent with
  other modals." No new gesture handling, no custom drag region.
- **Cons:** Requires laying out the unified create modal's stack
  (backdrop blur, hero media background, glass drawer, preview card)
  inside a sized child instead of `Stack` filling the screen. The
  full-bleed hero image in preview state and the camera viewport in
  image mode both currently assume "I own the whole screen" via
  `Positioned.fill`; they need to keep that visual effect while the
  outer container is shorter than the screen — likely via
  `OverflowBox` or by promoting the camera/hero into the route's
  barrier area through a different layout, which adds non-trivial
  layout work.
- **Effort:** Medium.

### Option B: Keep the modal full-screen, wrap the non-content margin in a dismiss gesture
- **Approach:** Leave the `SizedBox(height: media.size.height)` and the
  full-bleed visual treatment alone. Add a transparent dismiss zone
  over the area above the drawer/preview card. **Critical lint
  constraint:** `scripts/check_a11y.js` enforces
  `avoid_raw_gesture_detector` with an empty allowlist
  (`docs/client/accessibility.md` lint table) — so the tap-off zone
  cannot be a raw `GestureDetector`. Use `Tappable` (from
  `widgets/accessibility/`) which mandates a `semanticsLabel`, e.g. a
  new `a11yDismissModal` ARB key. The vertical-drag portion can use
  `Listener` since it's not a tap. Remove the X for non-camera
  states; keep it (and ensure the dismiss zone is unmounted) when the
  camera layer is active.
- **Pros:** Preserves the current full-bleed hero/camera visuals
  without layout surgery. Lets us tune the tap-off zone precisely
  (e.g. exclude the "Replace background" button hit area).
- **Cons:** Reinvents what Flutter's modal barrier already does, just
  inside the sheet. Drift risk: if `showAccessibleModal` ever changes
  defaults or a future modal designer expects standard barrier
  semantics, this one-off won't pick that up. Adds a new
  `a11yDismissModal` l10n key (en + es) and screen-reader semantics
  for a transparent area that visually appears to be "off" the modal.
- **Effort:** Small-to-medium.

### Option C: Hybrid — barrier dismiss + explicit drag handle, keep full screen
- **Approach:** Same as Option B for tap-off (transparent gesture
  overlay), but for swipe-down, render a small drag handle pill at the
  top of the modal and only treat drags that originate on the handle
  as a dismiss gesture (the rest of the modal area remains free for
  drawer/preview interactions). Camera state hides the handle and
  re-introduces the X.
- **Pros:** Visible affordance for swipe-down (current creation modal
  has none — even with `enableDrag: true`, users don't know they can
  swipe because there's no handle), which the other modals also lack
  but probably shouldn't. Could be promoted to all modals later.
- **Cons:** Introduces a UI element (drag pill) that none of the other
  modals have, so it diverges from the stated goal of "match the
  others" in a different direction. May need a design call.
- **Effort:** Medium.

## Recommendation

**Option A, approach 2: resize the sheet and use `OverflowBox` to
preserve full-bleed visuals where needed.** Reuse the existing
`showAccessibleModal` defaults (`isDismissible: true`,
`enableDrag: true`) instead of reinventing barrier semantics inside
one modal. This stays homogeneous with `ExperienceCreationModal`,
`RequestCreationModal`, and `TimePollProposeModal`.

Why approach 2 over approach 1 (pushing the camera as a separate
route): `docs/client/architecture.md` mandates `NavigationHelpers`
(`pushScreen` + `SwipeToCloseMixin`) for in-app navigation, and
`npm run lint:nav` enforces it. Those primitives target primary
content screens. Promoting the camera viewport — a sub-state of a
modal flow whose state lives in `unifiedCreateViewModel` — into a
push-screen route would be a poor fit: it would either require
plumbing the provider across the route boundary or reworking the
state model. `OverflowBox` from a single sized sheet is the lower-
friction path.

The cost is the layout work to keep the full-bleed hero (preview
state) and the camera viewport (image input mode) bleeding past the
sheet bounds when the outer container is no longer the full screen.
`OverflowBox` can fight `MediaQuery`/`SafeArea` reads, and the
existing `mergeForBottomSheet` plumbing for keyboard insets (issue
#1933) must keep working. Verify before merging.

If `OverflowBox` turns out to be more trouble than it's worth, fall
back to **Option B** — but plan for the `Tappable` constraint:
adding the `a11yDismissModal` l10n key (en + es) and accepting that
a transparent screen-area announcing itself as "Dismiss modal" is a
weaker UX than letting Flutter's native barrier handle it.

## Open Questions

- **Should we add a visible drag-handle pill, or just rely on the
  invisible swipe gesture?** — Recommendation: no handle, to match the
  other modals exactly (none of them show a handle either). If we want
  handles, that's a separate cross-app decision, not a one-off here.
- **Android system back button in camera mode — should it close the
  camera layer back into the drawer first, or dismiss the whole
  modal like swipe-down does?** — Recommendation: dismiss the whole
  modal, matching the X and swipe-down semantics. Treating back as
  "close camera, return to text tab" is a *different* UX choice the
  user can make later without redesigning this work. Confirm with the
  user; if they want the sub-state behavior, it's a single
  `WillPopScope` / `PopScope` away.
- **Tap-off in the preview stage with a full-bleed hero image: is the
  hero considered "the modal" (tap does nothing) or "background"
  (tap dismisses)?** — Recommendation: hero is part of the preview
  card visual, so it should not dismiss — match the modal-content
  hit area to the preview card itself and let any visible barrier
  area handle dismissal. Otherwise users tapping the image to inspect
  it would accidentally lose their draft.
- **Accessibility: with the X removed, does the modal still announce
  a way to dismiss itself to screen-reader users?** — Recommendation:
  yes; `showModalBottomSheet`'s barrier already exposes a "Dismiss"
  semantics action via Flutter's `ModalBarrier`. Verify with VoiceOver
  + TalkBack before merge; if it doesn't, add an explicit
  `Semantics(onDismiss: ...)` wrapper around the sheet content. (Note:
  under Option B's `Tappable` dismiss zone, a new `a11yDismissModal`
  ARB key is required in `app_en.arb` + `app_es.arb` —
  `docs/client/i18n.md` requires both locales before
  `npm run generate`.)
- **Does the `mergeForBottomSheet` keyboard-inset plumbing still work
  when the sheet is sized-to-content instead of full-screen?** —
  Recommendation: should be fine — `viewInsets.bottom` is propagated
  from the inner `MediaQuery`, which doesn't depend on the sheet
  height. But verify on iOS + Android with the keyboard open in text
  mode before merging; this is the kind of subtle regression that
  caused #1933 in the first place.

## Implementation Plan

### Phase 1: Behavior parity for non-camera states
**Goal:** Non-camera states (text input, url input, preview) dismiss
on swipe-down and tap-off, with no X button. Camera state is unchanged.
**Tasks:**
- [ ] Default to Option A approach 2 (resize sheet + `OverflowBox`
      for full-bleed visuals). Fall back to Option B (`Tappable`
      dismiss zone — *not* raw `GestureDetector`, per the
      `avoid_raw_gesture_detector` lint rule) if `OverflowBox`
      conflicts with `MediaQuery` reads.
- [ ] Resize the modal stack in
      [unified_create_modal.dart:78-79](app/lib/presentation/screens/create/unified_create_modal.dart#L78-L79)
      so the barrier above the sheet is tappable. Wrap the hero
      background and any full-bleed visual in `OverflowBox` to bleed
      past the new sheet bounds where the design demands it.
- [ ] Gate the existing `_GlassIconButton` (X) so it renders only when
      `state.stage == CreateStage.input &&
      state.inputMode == CreateInputMode.image`, in
      [unified_create_modal.dart:128-138](app/lib/presentation/screens/create/unified_create_modal.dart#L128-L138).
- [ ] Make `showAccessibleModal`'s dismiss flags explicit at the call
      site in
      [unified_create_modal.dart:60-70](app/lib/presentation/screens/create/unified_create_modal.dart#L60-L70)
      (`isDismissible: true`, `enableDrag: true`). They default to true
      in [show_accessible_modal.dart:21-22](app/lib/presentation/widgets/accessibility/show_accessible_modal.dart#L21-L22),
      but being explicit documents intent.
- [ ] If Option B fallback is taken: add an `a11yDismissModal` key to
      `app_en.arb` *and* `app_es.arb` (with `@key` metadata), then
      `npm run generate`. The dismiss zone uses `Tappable` with that
      semantics label — never a raw `GestureDetector`.
- [ ] If any new animation duration is added (e.g., a custom fade for
      the resized sheet), wrap it in `accessibleDuration(context, ...)`
      per the a11y primitives table. `prefer_accessible_duration` lint
      will fail otherwise.
- [ ] **Widget tests** — this is one of the documented widget-test
      cases (UI dismiss behavior cannot be exercised via
      `ProviderContainer` alone). Use `app/test/helpers/l10n_helpers.dart`
      for the `MaterialApp` wrapper. No ViewModel logic changes, so no
      ViewModel test additions are expected.
  - [ ] Open the modal in text mode, simulate a vertical drag from the
        top edge, assert the modal is gone.
  - [ ] Open the modal in text mode, tap the barrier (or `Tappable`
        dismiss zone under Option B), assert the modal is gone.
  - [ ] Open the modal, switch to image (camera) mode, assert the X
        button is present and `find.byType(_GlassIconButton)` returns
        a hit.
  - [ ] Open the modal, switch to image mode, simulate a swipe-down,
        assert the modal is gone (verifies swipe-down still works
        beneath the camera layer's gesture handlers).

**Validates:** That the new dismiss behavior works in text/url/preview
states and that the X correctly survives only in camera mode.

### Phase 2: Camera-mode behavior
**Goal:** In camera (image input) mode, swipe-down dismisses the modal
and the X in the upper right also dismisses the modal (matching today's
X behavior and swipe-down semantics). Tap-off is intentionally not
supported because the camera viewport fills the screen.
**Tasks:**
- [ ] Confirm the camera layer
      ([unified_create_camera_layer.dart](app/lib/presentation/widgets/creation/unified_create_camera_layer.dart))
      doesn't intercept the swipe-down gesture. Its tap-to-focus and
      shutter handlers shouldn't compete with a vertical drag, but
      verify on device (iOS + Android emulator at minimum).
- [ ] Keep the existing X behavior: `Navigator.of(context).pop()`,
      which closes the whole modal. This matches what swipe-down does.
- [ ] Add a widget test for camera mode: tapping the X dismisses the
      modal entirely.

**Validates:** The X-as-camera-only exception is wired correctly and
no swipe-down regression slipped in from the camera's gesture handlers.

### Phase 3: Accessibility + polish
**Goal:** Screen-reader users can dismiss without the X, and the
implementation matches the visual style of other modals.
**Tasks:**
- [ ] Manually verify VoiceOver (iOS) and TalkBack (Android) announce a
      dismiss action for the creation modal in non-camera states. If
      not, wrap the sheet root with explicit `Semantics(onDismiss: ...)`.
- [ ] Verify Android system back button behavior matches the open
      question's recommendation (back dismisses the modal in all
      states). Confirm no regression of the X's behavior in camera
      mode.
- [ ] The `a11yClose` ARB key stays — the camera-state X still uses
      it. No ARB removal needed.
- [ ] Run the full lint suite locally and ensure all pass:
  - [ ] `npm run lint:dart:a11y` — catches any raw `GestureDetector` /
        missing `semanticsLabel` slip-ups. The allowlist is empty;
        new violations *will* fail CI.
  - [ ] `npm run lint:dart:i18n` — catches hardcoded user-facing
        strings if the Option B path was taken and `a11yDismissModal`
        wasn't routed through `context.l10n`.
  - [ ] `npm run lint:nav` — only relevant if any nested navigation
        was introduced; if you stuck to a single sheet, this is
        free-pass.
  - [ ] `npm run lint:dart:size` — the file is ~360 lines today, well
        under the 1000-line gate, but check after changes.
  - [ ] `npm run lint:ephemeral-comments` — don't leave "Phase 1" or
        issue-tracking phase markers in code comments.
- [ ] `npm run generate` after any ARB changes (Option B path only).
- [ ] Smoke-test on both iOS simulator and Android emulator, including
      the keyboard-open path in text mode (regression guard for
      #1933's `mergeForBottomSheet` plumbing).

**Validates:** No accessibility regression and all code-style gates
are green.

## Related Issues
- #1933 — Referenced in the existing `mergeForBottomSheet` comment in
  `UnifiedCreateModal.show`. The keyboard-inset plumbing it added must
  keep working after the layout changes in this plan; any resize work
  in Phase 1 needs to preserve that fix.
