# Issue #1174: Location permission popup not shown on first cold-start (Android timing race)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1174
**Filed:** 2026-04-17 | **Last activity:** 2026-04-18
**Labels:** _(none)_ | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

On a fresh install or after revoking permissions, the Android OS permission
popup for location is never shown on the first cold-start. The app proceeds
as if location were denied: search, proximity sort, distance rendering, and
"center on me" all silently fall back to a default (Austin coords) or no-op
behavior. On the second cold-start the popup fires normally.

The issue description initially hypothesised an `initState`-vs-resumed-Activity
timing race. Fresh-install reproduction on 2026-04-18 showed a different root
cause: **FCM's notification permission request and Geolocator's location
permission request fire concurrently on cold-start, and Android's permission
queue can only service one at a time.** The losing request returns empty
`grantResults`, which Geolocator interprets as "user cancelled" — the user
never sees any dialog.

Evidence from the adb log on fresh launch:

```
I/flutter : FCMService: Initializing FCM service...
I/flutter : DiscoverScreen: 🔄 Community changed: ...
W/Activity   : Can request only one set of permissions at a time
I/Geolocator : The grantResults array is empty. This can happen when the
               user cancels the permission request
I/flutter : FCMService: Permission status: AuthorizationStatus.authorized
```

The deeper issue is not just that two permission prompts collide — it's that
the app asks for location permission **unsolicited, at app launch, without any
user intent**. That violates the platform "request-at-point-of-use" guideline
and is the reason two prompts can land at the same time in the first place.
The chosen fix therefore eliminates the unsolicited request entirely rather
than serialize it.

## Context from Comments

One comment exists (authored by briangbrown, 2026-04-18):

- Fresh-install repro confirmed the original symptom but rewrote the root
  cause as FCM/Geolocator collision.
- Explicitly rules out fix option 1 from the original issue (post-frame
  callback): the Activity is already resumed by the time both requests fire;
  the problem is concurrency, not ordering within the frame.
- Notes downstream impact on #1175 (3s GPS timeout may be a consequence, not
  Mapbox lock contention) and #1177 (center-on-me silent no-op).
- Proposed three fix options at the time (serialize, onboarding, retry).

Subsequent design discussion (2026-04-18, in-session): pivoted from "serialize
the two requests" to "defer the location request until a user gesture." This
fixes the race by construction and aligns with Android 13+ / iOS HIG
point-of-use guidance.

## Current code paths (pre-fix)

**FCM initialization (`app/lib/main.dart:225`)**

`_initializeFCM()` is called fire-and-forget from the app-startup microtask
while the splash is up, gated on `authState.isAuthenticated`. Inside
`FCMService.initialize()` at `app/lib/services/fcm_service.dart:60`,
`_requestPermission()` calls `FirebaseMessaging.requestPermission(...)` —
this is the first Android permission request in the queue.

**Location initialization (pre-fix)**

- `discover_screen.dart:59` — `initState` eagerly fires
  `unawaited(ref.read(userLocationProvider.future))`.
- `home_screen.dart:67` — `_onAddTap` (the `+` button) also calls
  `ref.read(userLocationProvider.future)`.

Both paths resolved through `UserPositionResolver.resolve` →
`DeviceLocationService.getCurrentPosition()`, which called
`Geolocator.requestPermission()` whenever the permission was `denied`. The
discover screen's `initState` fires this at app launch, racing FCM.

**Location consumers**

- **User-gesture paths** — `_centerOnUserLocation`
  (`discover_screen.dart:71`), `LocationPicker` "use current location"
  buttons, `_onAddTap` (`home_screen.dart:61`). Valid places to prompt.
- **Background paths** — `search_view_model.dart:357` (initial search
  proximity sort), `discover_screen.dart:350/385/417/297` (map camera),
  `leaderboard_screen.dart:108`, and all the `*_creation_view_model.dart`
  autofill reads. These use `.asData?.value` and already tolerate null via
  the `UserPositionResolver` fallback ladder (GPS → primary residence → null).

**Fallback ladder in `UserPositionResolver`**

1. Live GPS
2. User's primary residence (`user.primaryResidenceLocationId`)
3. null (unbiased)

**Brand-new-user reality:** For a fresh-install user, the residence rung is
also null (they haven't set one). The ladder collapses to `null` and the
`_buildMapView` hardcoded Boulder fallback renders until the user takes an
action that provides a location. That is acceptable for this fix — see
[Chosen approach](#chosen-approach-defer-prompt-until-gesture) below.

## Chosen approach: Defer prompt until gesture

**What changes:**

1. Split `DeviceLocationService.getCurrentPosition()` into two methods:
   - `getCurrentPositionIfGranted()` — checks permission via
     `Geolocator.checkPermission()`, returns null if not granted. **Never
     prompts.** Background consumers use this.
   - `requestAndGetCurrentPosition()` — prompts for permission if needed.
     **Only user-gesture paths use this.**
2. `UserPositionResolver.resolve` calls the non-prompting variant, so
   `userLocationProvider` never triggers a prompt.
3. Remove the eager `userLocationProvider.future` prewarm from
   `discover_screen.dart:initState`. The screen no longer asks for GPS
   unbidden.
4. `_centerOnUserLocation` in `discover_screen.dart` calls the prompting
   variant directly; on a successful grant, invalidates `userLocationProvider`
   so background consumers (search proximity, map camera) re-resolve with
   the fresh fix.
5. `_onAddTap` in `home_screen.dart` (the `+` button to create content) is a
   user gesture and also uses the prompting variant, with the same
   invalidation on success.

**Why this is better than serializing the requests:**

- **Fixes the race by construction.** No unbidden Geolocator request → no
  collision with FCM's request. Android's permission queue is never
  contested at startup.
- **Aligns with platform UX.** Android 13+ and iOS HIG both recommend
  requesting sensitive permissions at the point of use with a clear
  rationale from a user gesture.
- **Smaller permission fatigue.** Users who never center the map and never
  create content are never asked for GPS.
- **Self-correcting for #1175.** The 3 s timeout only fires for users who
  have granted. Fresh-install users fall back to residence/null immediately,
  no 3 s wait.
- **Directly addresses #1177.** `_centerOnUserLocation` is the prompt site,
  so the "silent no-op" case becomes "prompt appears → grant → map flies".
- **No new coordination primitive.** No `Completer`, no cross-service
  coupling, no gate to reset in tests.

**What we accept as a tradeoff:**

- **Brand-new users see an unbiased discover experience on first launch.**
  No GPS, no residence, no community location in the proto → map shows
  Boulder hardcode, search sorts by relevance rather than proximity. First
  location-triggering gesture fixes it. Strictly better than the Austin
  fallback or never-prompts-again current behavior.
- **Tracking community-centroid-based discovery for fresh-install users** is
  a separate initiative — requires adding location to the `Community` proto
  or a server-side IP hint. File separately.

## Alternatives considered and rejected

### Alternative: Serialize the two permission requests via an FCM-settled gate

- **Approach:** Add `Future<void> permissionSettled` on `FCMService`, have
  `UserPositionResolver.resolve` await it via a Riverpod provider before
  calling Geolocator.
- **Why rejected:** Fixes the race, but keeps the unsolicited location
  request at app launch — the upstream problem of "asking for GPS the user
  didn't ask for" is preserved. Also introduces cross-service coordination
  state (a `Completer` that must complete on every FCM failure path,
  including crashes), which is extra surface area. The deferred-prompt
  approach eliminates both problems with a comparable code diff.

### Alternative: Block splash until FCM permission resolves

- **Approach:** `await _initializeFCM()` in the startup microtask so FCM's
  prompt completes before discover mounts.
- **Why rejected:** Conflicts with `docs/client/startup.md` §Phase 5
  ("fire-and-forget service initialization"), and the 5 s splash max-duration
  cap would silently reintroduce the race if FCM ever took longer than that.

### Alternative: `permission_handler` batch pattern

- **Approach:** Use `[Permission.notification, Permission.location].request()`
  to queue both dialogs atomically on Android.
- **Why rejected (for now):** Fixes the race but keeps the unsolicited
  startup prompt (violates point-of-use guidance), adds a new dependency
  (`permission_handler`), and would require parity-checking iOS notification
  options that are currently set by `firebase_messaging.requestPermission`.
  Deferred-prompt approach is simpler and philosophically correct. Revisit if
  more permissions (camera, Bluetooth) enter scope — batch pattern scales
  well at that point.

### Alternative: Explicit permission onboarding flow

- **Approach:** New onboarding screens that explain each permission and
  drive the dialogs one at a time. Skip for returning users.
- **Why not now:** Bigger change. Requires new screens, ARB keys in
  `app_en.arb` and `app_es.arb` per `docs/client/i18n.md`, state tracking
  extension of `tutorial_view_model.dart`, and skip logic for re-grants.
  Consider as a follow-up to improve first-install conversion — **the
  deferred-prompt fix is a prerequisite**, since point-of-use is how the
  onboarding flow would hand off to actual system prompts anyway.

## Open Questions

- **Does the collision also happen on iOS?** — Recommendation: **no** — iOS
  doesn't share a single permission queue the way Android does, and location
  vs. notification prompts are independent. The deferred-prompt fix is a
  no-op on iOS (iOS already works), but ship the same code path so the
  architecture stays platform-uniform.

- **What about the Boulder hardcode in `_buildMapView`?** — Recommendation:
  **leave it for this PR**, file a follow-up. The MVP deferred-prompt path
  makes Boulder visible only to fresh-install users for a brief moment
  (until they interact or items load and the camera reframes). Replacing
  Boulder with something smarter (community centroid, IP geolocation) is a
  separate initiative.

- **Should we re-invalidate `userLocationProvider` after revocation (not
  just after grant)?** — Recommendation: **no action for this PR.** If a
  user revokes permission mid-session via system settings, the
  `userLocationProvider` cache would stay stale until the next session.
  Acceptable; the provider is session-cached by design. Revisit if it
  becomes a real pain point.

- **Should we add user-facing feedback if the user denies the prompt in
  `_centerOnUserLocation`?** — Recommendation: **yes, but in a follow-up**.
  Today `_centerOnUserLocation` silently bails when `position == null`.
  After this fix that state only happens when the user explicitly denied.
  Adding a snack bar or rationale dialog is the right UX but requires new
  ARB keys; file separately (relates to #1177).

- **Does `search_view_model`'s initial search still need its 3 s timeout?**
  — Recommendation: **yes, keep it for now, but re-measure in Phase 3**.
  With this fix, `userLocationProvider` resolves nearly instantly for
  denied/null cases (no prompt) and for returning users with a residence
  fallback. The 3 s timeout is only relevant if GPS itself is slow for
  granted users; that's #1175's territory.

- **Telemetry?** — Recommendation: **yes, minimal**. A single INFO log in
  `_centerOnUserLocation` and in `_onAddTap` recording whether the prompt
  resulted in grant vs. deny vs. already-granted. Useful for production
  dashboards and for #1175 follow-up. Add in Phase 2.

## Implementation Plan

### Phase 1: Deferred-prompt refactor — DONE (2026-04-18)

**Goal:** Eliminate the unsolicited Geolocator request at app launch; move
prompts to user-gesture sites.

**Delivered:**
- [x] Split `DeviceLocationService` into `getCurrentPositionIfGranted()`
      (checks only) and `requestAndGetCurrentPosition()` (prompts). Shared
      private `_fetchPosition({required bool requestIfDenied})` helper.
      `app/lib/services/device_location_service.dart`.
- [x] Update `UserPositionResolver.resolve` to call the non-prompting
      variant. `app/lib/services/user_position_resolver.dart`.
- [x] Remove eager `userLocationProvider.future` prewarm from
      `discover_screen.dart:initState`. Comment updated with a pointer to
      #1174.
- [x] Rewrite `_centerOnUserLocation` in `discover_screen.dart` to delegate
      to the new `requestLocationWithSettingsFallback` helper (see Phase 1b).
- [x] Update `_onAddTap` in `home_screen.dart` to prompt via the helper
      (fire-and-forget; the helper invalidates the provider on grant).
- [x] Migrate the location-picker modal's "use current location" button to
      the helper: new widget-side path (`_useCurrentLocation` in
      `location_picker_modal.dart`) calls the helper, then hands the
      resolved `Position` to a new ViewModel method
      `LocationPickerNotifier.applyDevicePosition(Position)`. The old
      `useCurrentLocation()` method that mixed permission + geocoding is
      removed — the ViewModel now has a pure reverse-geocode responsibility,
      no `BuildContext` dependency, preserving the MVVM boundary in
      `docs/client/architecture.md`.
- [x] Migrate the disposal test in
      `test/presentation/viewmodels/location_picker_view_model_test.dart`
      to the new `applyDevicePosition(Position)` API. All 3460 tests pass.
- [x] `flutter analyze` is clean across every changed file (3 pre-existing
      `MapRecorder` warnings in an auto-generated Mapbox mock are unrelated).

**Validates:** Manual repro on the Android emulator — uninstall
`org.ripls.app.dev`, relaunch, observe only the FCM notification prompt at
startup (no location prompt). Reach discover, tap "center on me", observe
location prompt, grant, map flies to position. All three gesture sites
(`center on me`, `+` button, location picker "use current location")
prompt on demand; none prompt at startup.

### Phase 1b: Settings-fallback helper — DONE (2026-04-18)

Shipped alongside Phase 1 once manual testing revealed that after a user
denies once, Android 13+ auto-promotes the permission to `deniedForever`
and the system won't show the prompt again — leaving the user with no
in-app way to re-enable beyond navigating to OS settings manually.

**Goal:** Offer a path forward when permission is `deniedForever` without
nagging users who just made an active choice.

**Delivered:**
- [x] New helper `app/lib/core/utils/location_permission_helper.dart` —
      `requestLocationWithSettingsFallback(BuildContext, WidgetRef)`.
      Single entry point used by all three gesture sites.
- [x] `DeviceLocationService.requestAndGetCurrentPosition()` now returns a
      new `LocationRequestResult { position, permission }` instead of a
      bare `Position?`. Lets the helper observe the authoritative
      permission state without a second `Geolocator.requestPermission()`
      call (which, per manual testing, can produce a **second system
      prompt back-to-back** on Android when the initial deny hasn't yet
      auto-promoted to `deniedForever`).
- [x] Timing heuristic: stopwatch the `requestAndGetCurrentPosition` call.
      If it took ≥200 ms, Android showed the system dialog and the user
      responded — respect that choice, suppress our settings dialog. If
      <200 ms, Android silently declined (state was already
      `deniedForever`) — show our dialog because the user otherwise has
      no visible feedback from the tap. Threshold validated manually: fresh
      user interaction measures ~2000–2500 ms, silent denials measure
      3–7 ms.
- [x] Settings dialog styled to match existing app dialogs per
      `experience_content_view.dart:1645-1674` pattern —
      `AppColors.surface` background, `AppColors.textPrimary` title,
      `AppColors.textSecondary` body + cancel.
- [x] `showDialog(useRootNavigator: true, ...)` so the dialog renders over
      any nested modal bottom sheet (e.g. the location picker modal).
- [x] `Geolocator.openAppSettings()` wired to the "Open Settings" action.
- [x] ARB keys added in both locales:
      `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb` —
      `commonOpenSettings`, `locationPermissionDeniedTitle`,
      `locationPermissionDeniedBody`. Regenerated via `flutter gen-l10n`.
- [x] Helper emits INFO log lines at each decision point
      (`Location granted`, `permission=... (service call took N ms)`,
      `System dialog appears to have been shown and declined this
      attempt; suppressing settings dialog to respect user choice`,
      `Showing open-settings dialog for pre-existing deniedForever state`,
      `Context unmounted before settings dialog could be shown`). Useful
      in future repros and production logs.

**Validates:** Manual repro with `adb shell pm reset-permissions
org.ripls.app.dev` followed by a `center on me` tap:
- First tap → system dialog appears → tap "Don't allow" → helper measures
  ~2245 ms → settings dialog suppressed.
- Second tap → no system dialog (state is now permanent) → helper
  measures 3–7 ms → our settings dialog appears.
- Tap "Open Settings" → Ripls backgrounds, OS app permissions page
  opens → user toggles location on → return → tap `center on me` →
  position granted, provider invalidated, map flies.

### Phase 2: Remaining test coverage and production verification

**Goal:** Lock in the fix against regression; confirm production behavior.
The disposal test migration above is done; the items below are still TODO
and can land incrementally.

**Tasks:**
- [ ] Unit test for `DeviceLocationService.getCurrentPositionIfGranted` —
      returns null when `Geolocator.checkPermission()` returns `denied`
      without calling `requestPermission`. Mock Geolocator and assert no
      prompt call was made.
- [ ] Unit test for `DeviceLocationService.requestAndGetCurrentPosition` —
      returns `LocationRequestResult` with `position` set when granted,
      `position: null` + `permission: denied` when user declines,
      `permission: deniedForever` when OS skips the dialog. Verify the
      existing `_permissionRequest` deduplication still works.
- [ ] Unit test for `UserPositionResolver.resolve` — returns residence
      fallback when GPS is not granted; returns null when neither GPS
      nor residence are available. Assert `Geolocator.requestPermission`
      is never called.
- [ ] Unit test for `requestLocationWithSettingsFallback` — three cases:
      granted (returns position, invalidates provider), fresh denial
      (stopwatch >200 ms, no dialog), pre-existing `deniedForever`
      (stopwatch <200 ms, dialog shown). Inject a fake clock or mock the
      service to force specific timings.
- [ ] Widget-level test (if feasible with the existing harness): mount
      `DiscoverScreen` with a fake `DeviceLocationService`; assert
      `requestPermission` is NOT called during `initState` or build, IS
      called when `_centerOnUserLocation` is invoked.
- [ ] Optional telemetry: add grant/deny/already-granted tagging to the
      helper's existing log lines so we can aggregate in production.
- [ ] Run `npm run test:app` cleanly. (Currently blocked by 3 pre-existing
      `MapRecorder` warnings in an auto-generated mock file that cause
      `flutter analyze` to exit 1 and short-circuit the npm script — file
      a separate cleanup issue.)

### Phase 3: Close-out and downstream re-measurement

**Goal:** Confirm the fix addresses #1174 end-to-end and informs the status
of #1175 and #1177.

**Tasks:**
- [x] Fresh-install repro on the Android emulator with the new build —
      confirmed on 2026-04-18 (process 5753 / 8214 logs):
    - No location prompt on first launch.
    - FCM notification prompt appears alone, authorized cleanly.
    - `W/Activity: Can request only one set of permissions at a time`
      does NOT appear in logcat.
    - Tapping "center on me" shows the location prompt and grants
      cleanly (Boulder emulator mock position resolved end-to-end).
- [ ] Repro the same flow on iOS Simulator. Use
      `xcrun simctl privacy booted reset location org.ripls.app.dev` to
      clear permission state between runs. Expected: iOS has no
      permission-queue collision to reproduce in the first place, but
      validate that deferred-prompt behavior and settings-fallback dialog
      still work (iOS `Geolocator.openAppSettings()` routes to the app's
      Settings panel).
- [ ] Re-run the cold-start timing capture from #1175 with the new build.
      If the 3 s GPS timeout no longer fires at startup (it shouldn't,
      because we don't call GPS at startup anymore), comment on #1175
      recommending we close it as fixed-by-1174. If the timeout fires
      after a gesture grant, the Mapbox-lock hypothesis still has legs
      and #1175 needs its own plan.
- [ ] Tap a pin and "center on me" per #1177's manual repro. If it now
      works end-to-end, comment on #1177 reducing its scope to:
      1. the leaderboard `updateCameraForCommunities` empty-path bug,
      2. user-facing feedback when the user explicitly denies the prompt,
      3. replacing the Boulder hardcode in `_buildMapView`.
- [ ] Merge the PR with a commit message referencing #1174 and summarizing
      the deferred-prompt approach.

**Validates:** All three issues are re-measured with up-to-date evidence
in their comment threads; first cold-start on Android no longer silently
leaves the user without a location prompt.

### Follow-ups to file separately (not in this PR)

- **Replace Boulder fallback in `_buildMapView`** with something that works
  for fresh-install users — candidates: community centroid (requires proto
  change), server-side IP hint, or a globe overview.
- **Add `location` (or `centroid`) field to `Community` proto** so communities
  have a geographic anchor point for discover and leaderboard maps.
- **Explicit permission onboarding flow** (old Option C) once the
  deferred-prompt shape is stable in production.
- **AlertDialog styling cleanup** — **filed as #1181**. This PR's new
  location permission dialog is the 26th `AlertDialog` call site in the
  app with independently-styled chrome; consolidating behind a shared
  helper is tracked there.
- **Pre-existing `flutter analyze` noise** — the `MapRecorder` experimental
  API warnings in the auto-generated
  `test/core/utils/map_helper_test.mocks.dart` cause the npm `test:app`
  script to short-circuit before `flutter test` runs. Unrelated to this
  work but should be cleaned up to unblock CI ergonomics.

## Related Issues

- **#1175** — Initial GPS fetch hits 3 s timeout on cold-start. Much of
  this is a downstream effect of the eager-prompt path we just deleted;
  the remaining Mapbox-lock hypothesis only matters after a user grants.
  Re-measure in Phase 3.
- **#1177** — Discover/leaderboard map (0,0) + "center on me" silent no-op.
  This PR turns the silent no-op into a visible permission prompt.
  Latent `leaderboard_map_helper` empty-communities bug is independent.
- **#1156** — Migration to `userLocationProvider` that surfaced these bugs
  during manual testing. Not a regression of #1156.
- **#1143** — Introduced `UserPositionResolver` / `DeviceLocationService`;
  this PR changes the Geolocator call convention on that scaffolding.

## Audit notes (2026-04-18)

Audited against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/startup.md`, and
`docs/server/architecture.md`.

- **No architectural conflicts.** Deferred-prompt keeps the existing
  fire-and-forget FCM init (per `docs/client/startup.md` §Phase 5), stays
  in the services layer (no changes to repositories or caching), and keeps
  `DeviceLocationService` as a static class.
- **Provider usage intact.** `userLocationProvider` remains a
  `FutureProvider<Position?>`. The new ingredient is `ref.invalidate` on
  user-gesture grant — a standard Riverpod pattern, documented in
  `docs/client/architecture.md` §Providers.
- **Async safety preserved.** `_centerOnUserLocation` and `_onAddTap`
  already include `if (!mounted) return;` guards after their awaits; the
  refactor keeps them. `UserPositionResolver.resolve` retains its existing
  `ref.mounted` checks.
- **Build command.** Verification is `npm run test:app` (generate +
  analyze + test) and `npm run lint:dart`. Do **not** run `npm run
  build:app` (full appbundle) for this bug fix.
- **i18n.** Three new user-facing strings shipped with Phase 1b
  (`commonOpenSettings`, `locationPermissionDeniedTitle`,
  `locationPermissionDeniedBody`), added to both `app_en.arb` and
  `app_es.arb` per `docs/client/i18n.md`.
- **No caching concerns.** `docs/client/caching.md` does not apply —
  service-layer coordination, not data access.
- **No server-side changes.** `docs/server/architecture.md` does not
  apply — fix is entirely client-side.

Plan is ready for Phase 2 (tests and telemetry). Phase 1 is in the
working tree awaiting its PR.
