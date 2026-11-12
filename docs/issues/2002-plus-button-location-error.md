# Issue #2002: Hit the plus button and immediately got an location error

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2002
**Filed:** 2026-05-17 | **Last activity:** 2026-05-17
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a Ripls user on iOS has location turned off (either app permission set to "Never" or system location services disabled), tapping the global **+** (create) button surfaces a modal "Location access needed / Location is disabled for Ripls. Open Settings to enable it and try again." dialog before the user has done anything that requires location. The user wrote: *"It makes me sad when i cant use your incredible app when i have location turned off"* — they were trying to create content and got blocked by what reads as a permission error, even though the `+` flow has actions that do not need location at all (Invite User, Request Something inside a community, and the unified-create modal's text-only path).

The dialog originates from `app/lib/core/utils/location_permission_helper.dart:69-102`, called eagerly from `_HomeScreenState._onAddTap` (`app/lib/presentation/screens/home/home_screen.dart:73`). The helper invokes `DeviceLocationService.requestAndGetCurrentPosition()`, and when the resulting permission state is `deniedForever` and the call returned quickly (no system dialog was shown, because iOS will not re-prompt after a permanent denial), it shows the in-app "Open Settings" alert. This was an intentional fallback for the deniedForever case but is too aggressive: the user did not invoke a location-requiring action, they only tapped `+`.

The root cause is that `_onAddTap` opportunistically prompts for location (and shows the settings-redirect dialog) regardless of whether the user is about to take a location-requiring action.

## Context from Comments

Two comments, both from maintainer:

- **"Can we have a more graceful degradation when location permission has been disallowed?"** — this is the requested direction. Bryan is asking for the `+` flow (and presumably any other path the user takes after `+`) to work without nagging when location has been refused.
- **"@claude"** — invocation of this triage workflow.

The reporter (user13@example.org) is on iOS 26.2.1, iPhone, app 0.21.1 (build 6979), PROD. The screenshot URL embedded in the issue body is a presigned GCS link from Ripls' feedback system; it's not viewable from this environment but the description plus Bryan's framing make the failure mode clear.

There is no disagreement and no maintainer direction on *how* to degrade — only that we should.

## Potential Solutions

### Option A: Silent eager fetch — never prompt or pop a dialog from `+`

- **Approach:** Add a new sibling function in `app/lib/core/utils/location_permission_helper.dart` — `warmupUserLocationSilent(WidgetRef ref)` — that calls `DeviceLocationService.getCurrentPositionIfGranted(intent: LocationIntent.proximityBias)`, invalidates both `userLocationProvider(LocationIntent.proximityBias)` and `userLocationProvider(LocationIntent.precisePin)` on success (mirroring the existing helper at `location_permission_helper.dart:36-37`), and returns `void`. It never prompts and never shows a dialog. Replace the `unawaited(requestLocationWithSettingsFallback(context, ref))` call at `home_screen.dart:73` with `unawaited(warmupUserLocationSilent(ref))`. Move the prompting + settings-redirect responsibility entirely to user-gesture sites that *need* location: the "Use current location" chip in the location picker (`location_picker_modal.dart:311`) and "center on me" on the discover map (`discover_screen.dart:95`). Both of those already call `requestLocationWithSettingsFallback` and already represent the user explicitly asking for location.

  Putting the warmup logic in `location_permission_helper.dart` (alongside the existing `requestLocationWithSettingsFallback`) instead of inlining it in `_onAddTap` keeps `home_screen.dart` thin, gives the new function a single test target, and groups all "permission-aware location entry points" in one file — consistent with the helpers/utils pattern called out in `docs/client/architecture.md` ("no controllers" — utility classes/helpers handle non-state concerns).
- **Pros:**
  - Trivial, low-risk change — one line in `_onAddTap`, the helper itself is unchanged.
  - Eliminates the false-error UX entirely for the `+` path.
  - Autofill convenience is preserved when permission *is* granted.
  - Other call sites (discover map, location picker) keep their current behavior; the user who explicitly asks for location still gets the settings nudge.
- **Cons:**
  - Loses one of the iOS first-prompt opportunities. If the user never taps "Use current location" or "center on me", we never prompt for location at all, and the autofill won't kick in until they do. For users who land on `+` first, location-aware autofill in the unified-create modal degrades silently.
  - On Android, where a single "Deny" auto-promotes to `deniedForever`, this means we now never re-prompt from `+`. (We already don't re-prompt; we only pop the in-app settings dialog. So practical impact is small.)
- **Effort:** Small.

### Option B: Add a `suppressSettingsDialog` flag to the helper

- **Approach:** Extend `requestLocationWithSettingsFallback(context, ref, {bool suppressSettingsDialog = false})` so callers can opt out of the in-app alert while still triggering the system prompt on first-time `denied`. Pass `suppressSettingsDialog: true` from `home_screen.dart:73`. The helper's `deniedForever` branch returns null silently when the flag is set. The other two call sites keep the default (`false`) so they still nudge to Settings.
- **Pros:**
  - Preserves the iOS first-time prompt-on-`+` behavior — users who haven't decided yet still get the system dialog, and a grant wires up autofill immediately.
  - Targeted, additive change.
- **Cons:**
  - Slightly more API surface; future callers have to make the same decision.
  - The first-time prompt on a non-location action is itself debatable UX — Bryan's "graceful degradation" framing reads as "don't ask unless I need it," which favors A.
- **Effort:** Small.

### Option C: Defer the location decision to inside the unified-create modal

- **Approach:** Remove the location call from `_onAddTap` entirely. Move the opportunistic permission request into the location row of the unified-create modal — only the moment the user opens that row or taps the location field. Keep the existing helper unchanged.
- **Pros:**
  - Best alignment with "ask when relevant." Users who never set a location are never prompted from creation flows.
- **Cons:**
  - Larger surface area: needs changes to `unified_create_modal.dart` plus the legacy `PlusButtonModal` flow paths (gear / experience / request / invite). Several entry points.
  - Loses the pre-warm: today, location resolves while the user reads the modal; deferring to the location row makes the resolution feel slower.
- **Effort:** Medium.

## Recommendation

**Option B.** Per reviewer direction (maintainer): "We still want first request on +, but just a graceful degradation after that if they say no. We shouldn't keep bugging them." Extend `requestLocationWithSettingsFallback` with a `suppressSettingsDialog` flag and pass `true` from the `+` site. The first-time path is unchanged — when permission state is `denied`, the system prompt still fires from the `+` tap and a grant still wires up autofill immediately. Once the user has refused (state becomes `deniedForever`), the helper now returns silently from this call site instead of showing the in-app "Open Settings" alert. The two gesture sites that *do* represent an explicit user request for location ("Use current location" in the picker, "center on me" on the discover map) keep the default (`suppressSettingsDialog: false`), so their settings-redirect dialog still appears — that's the path where the dialog is the correct UX because the user just asked for location and there's nowhere else to take them.

Option A is rejected because it drops the first-time prompt entirely, which the reviewer explicitly wants to preserve.

Option C is deferred — too many entry points for what is fundamentally a "stop showing the dialog after refusal" problem.

## Open Questions

- **Should the `+` button still prompt for location the very first time (when permission state is `denied`, not `deniedForever`)?** — Resolved by reviewer: **Yes.** Bryan: "We still want first request on +." Option B preserves this — the system prompt fires on first tap; only the in-app settings-redirect dialog (which only appears in the `deniedForever` branch) is suppressed.
- **Do we want any indication in the unified-create modal that location is not currently available?** — Recommendation: **No additional UI in this fix.** The location row already supports unset state, and creation succeeds with `locationId: null` (verified in `unified_create_save_actions.dart`). Adding an inline "Location off — tap to enable" hint is a reasonable follow-up but is a separate UX choice and not what the user complained about.
- **Should the discover map's "center on me" still pop the settings dialog?** — Recommendation: **Yes, unchanged.** That button is the user explicitly asking the map to find them; the settings dialog is the only path forward and is expected. Same for the location picker's "Use current location."
- **Are there analytics events around the location prompt we should preserve / re-target?** — Recommendation: keep the existing `_log.info` lines in the helper; no new analytics needed for this fix. If we later want to measure how often `+` results in a granted autofill, add an event in a follow-up.
- **Does the same eager prompt fire from any other surface (e.g. splash, profile)?** — Recommendation: **No change scope creep.** Grep confirms only three callers of `requestLocationWithSettingsFallback`; only `home_screen.dart` is the offending one. The other two stay on the default (`suppressSettingsDialog: false`).

## Implementation Plan

### Phase 1: Add `suppressSettingsDialog` flag and pass it from the `+` call site

**Goal:** The `+` button still triggers the system permission prompt the first time (when state is `denied`), but no longer shows the in-app "Open Settings" alert once the user has refused (state `deniedForever`). The two explicit-gesture call sites are unchanged.

**Tasks:**
- [ ] In `app/lib/core/utils/location_permission_helper.dart`, extend `requestLocationWithSettingsFallback`'s signature:

  ```dart
  Future<Position?> requestLocationWithSettingsFallback(
    BuildContext context,
    WidgetRef ref, {
    bool suppressSettingsDialog = false,
  }) async { ... }
  ```

  Inside the `deniedForever` branch (currently `location_permission_helper.dart:48-107`), short-circuit before the `showDialog<bool>` call when `suppressSettingsDialog == true`:

  ```dart
  if (suppressSettingsDialog) {
    _log.info('deniedForever state observed; suppressing settings dialog '
        'per caller (passive entry point)');
    return null;
  }
  ```

  Place this check *after* the existing logging and `context.mounted` guard but *before* the dialog is shown. All other branches (granted → invalidate providers; `denied` → silent; `deniedForever` with a long-enough service-call duration → silent because the system dialog handled it) are unchanged. The system-prompt path on a fresh `denied` state continues to work because `DeviceLocationService.requestAndGetCurrentPosition` is called unconditionally at the top of the function.

- [ ] Update the helper's doc comment to mention the flag: a one-line addition like *"Pass `suppressSettingsDialog: true` from passive entry points (e.g. the global `+` tap) where the system permission prompt is still wanted on first use, but the in-app settings-redirect alert should not appear after the user has already refused."*

- [ ] In `app/lib/presentation/screens/home/home_screen.dart`, change line 73 from

  ```dart
  unawaited(requestLocationWithSettingsFallback(context, ref));
  ```

  to

  ```dart
  unawaited(
    requestLocationWithSettingsFallback(
      context,
      ref,
      suppressSettingsDialog: true,
    ),
  );
  ```

- [ ] Update the rationale comment above the call (currently `home_screen.dart:67-72`) to read approximately:

  ```
  // The + button is a user gesture — a valid point to prompt for location
  // the first time (the OS system dialog fires on `denied`, and a grant
  // wires up autofill immediately). But once the user has refused and
  // permission is `deniedForever`, the in-app "Open Settings" alert would
  // be a surprise here: the + flow has multiple actions that don't need
  // location at all (Invite User, Request Something), so we shouldn't keep
  // pestering. `suppressSettingsDialog: true` keeps the first-time prompt
  // and silences the repeat dialog (#2002).
  ```

- [ ] No import changes are required — the helper symbol is unchanged.

**Validates:** A user with `denied` state taps `+` and sees the OS system dialog (unchanged). A user with `deniedForever` state taps `+` and sees the create modal with no alert overlay. A user who already granted location sees the same autofill behavior as before.

### Phase 2: Tests — unit-first per `docs/client/testing.md`

**Goal:** Cover the regression at the cheapest reliable layer, and lock in that the explicit-gesture call sites still surface the settings dialog.

**Tasks:**
- [ ] Add unit tests for `requestLocationWithSettingsFallback` in `app/test/core/utils/location_permission_helper_test.dart` (create the file if it doesn't yet exist) using `ProviderContainer` + the existing `Geolocator` stubbing pattern from `app/test/services/device_location_service_test.dart`. Stub out the dialog by routing through a test harness that fails on any `showDialog` invocation, or by wrapping the call in a `MaterialApp` test widget and asserting on the rendered widget tree. Cover the new flag specifically:
  - **`suppressSettingsDialog: true`, `LocationPermission.deniedForever`, fast service call (no system dialog)** → returns null silently, NO `AlertDialog` is shown. (This is the specific state #2002 was hitting.)
  - **`suppressSettingsDialog: false` (default), `LocationPermission.deniedForever`, fast service call** → the existing dialog is shown (regression guard for the location picker / discover map paths).
  - **`suppressSettingsDialog: true`, permission granted** → returns the position; both `userLocationProvider(LocationIntent.proximityBias)` and `userLocationProvider(LocationIntent.precisePin)` are invalidated (autofill still warms).
  - **`suppressSettingsDialog: true`, fresh `LocationPermission.denied`** → returns null silently (no in-app dialog regardless of the flag; this branch was already silent).
  - **`suppressSettingsDialog: true`, `deniedForever` with elapsed time ≥ `systemDialogInteractionThresholdMs`** → returns null silently (covered by existing branch; flag is a no-op here, but assert nothing regresses).
- [ ] Confirm the existing tests in `app/test/services/device_location_service_test.dart` still pass — `DeviceLocationService` itself is unchanged.
- [ ] Confirm `app/test/presentation/widgets/plus_button_modal_test.dart` still passes — it does not exercise the location flow, but verifies the modal still opens cleanly.
- [ ] (Optional, low value) A widget test of `HomeScreen` is **not** added; per `docs/client/testing.md`, business logic belongs to viewmodels/utils and that's where the regression is best caught.

**Validates:** CI gate prevents (a) the in-app settings dialog reappearing from `+`, and (b) the dialog accidentally being suppressed at the gesture sites.

### Phase 3: Manual verification on a real iOS device with location off

**Goal:** Confirm both the user's reported reproduction is fixed *and* the first-time prompt is preserved.

**Tasks:**
- [ ] iOS Settings → Privacy & Security → Location Services → Ripls → reset to "Ask Next Time Or When I Share" (so permission state is `denied`, not `deniedForever`). Reinstall or use Reset Location & Privacy if needed to get a clean state.
- [ ] Launch the app, tap `+`. Expected: iOS system permission dialog appears (this is the "first request on +" the reviewer wants preserved).
- [ ] Tap "Don't Allow." Tap `+` again. Expected: unified-create modal opens, **no in-app alert appears.** This is the fix.
- [ ] Inside the modal, tap the location field → "Use current location." Expected: the existing in-app "Open Settings" dialog appears (this path is unchanged — explicit user gesture asking for location).
- [ ] Repeat with iOS Settings → Privacy & Security → Location Services → off globally. Expected: no alert from `+`; explicit "Use current location" still pops its settings dialog.
- [ ] Repeat after granting location. Expected: autofill location appears in the modal as today.
- [ ] Repeat on Android: deny location once → state auto-promotes to `deniedForever` → tap `+` and confirm no dialog appears. Tap "Use current location" inside the picker and confirm the existing settings dialog still appears.

**Validates:** The fix resolves the reported bug on both platforms while preserving the first-time prompt on `+`.

## Audit Notes (auto-applied)

Applied after a `/ripls-audit` pass against `docs/client/{architecture,i18n,accessibility,testing}.md` and `docs/server/observability.md`:

- **Architecture (helpers/utils, not controllers):** the change keeps all permission-handling logic inside `location_permission_helper.dart` and adds a single named parameter to the existing function rather than inlining a new branch in `_onAddTap`. Keeps `home_screen.dart` thin and aligns with the "Helpers/Utils handle non-state concerns" rule in `docs/client/architecture.md`.
- **No new viewmodel needed:** this fix is purely a permission/utility concern with no persistent state, so a `Notifier` would be overkill.
- **i18n:** no ARB additions/removals. The dialog strings (`locationPermissionDeniedTitle` / `locationPermissionDeniedBody`) remain in use by `location_picker_modal.dart:311` and `discover_screen.dart:95`. Do not delete the keys.
- **Accessibility:** the fix removes an unsolicited dialog at the `+` site; no new interactive widgets or labels are introduced, so the `a11y` lint rules are not triggered. The other two call sites' dialogs are unchanged.
- **Testing strategy:** unit tests target the modified helper (the testable unit), parameterized over the new `suppressSettingsDialog` flag — covers both the suppressed and default branches. No widget test of `HomeScreen`. Matches `docs/client/testing.md`: "Test business logic, not Flutter widgets."
- **Server observability:** not applicable — client-only change.
- **Logging:** the suppressed branch adds a single `_log.info` line describing why the dialog was skipped, consistent with the helper's existing logging style.

## Related Issues

- None directly linked. The location prompting helper was last touched as part of the unified-create work referenced in `docs/design/unified-create.md`; this fix narrows where that helper is invoked but does not change the helper itself.
