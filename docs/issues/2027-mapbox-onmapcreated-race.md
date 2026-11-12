---
issue: 2027
title: "Location picker map flashes to Austin then stays there: didUpdateWidget race against Mapbox onMapCreated"
---

# Issue #2027: Location picker map flashes to Austin then stays there: didUpdateWidget race against Mapbox onMapCreated

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2027
**Filed:** 2026-05-18 | **Last activity:** 2026-05-18
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

> **Audit pass (2026-05-18):** Reviewed against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/testing.md](../client/testing.md), and the proto/server conventions in [CLAUDE.md](../../CLAUDE.md). Updates appear as **\[Audit]** callouts and as a new "Compliance Requirements" section. Server docs were read but do not apply — this is a client-only widget-internal fix with no proto/RPC/repository changes.

## Problem Statement

Opening the location picker for a surface that does not pre-populate a location (e.g. the create flow → Set location, or a TBD-location experience) intermittently leaves the map centered on the Austin hardcoded fallback (`30.2672, -97.7431`) instead of the user's GPS or residence. Outcome flips between opens of the same flow — even back-to-back, on the same device, with GPS permission granted and the network warm.

The bug is a Flutter–native-view race inside `LocationMapPreview` (`app/lib/presentation/widgets/location/location_modal_widgets.dart`). On a "fast modal, slow Mapbox" open the camera correction is dropped on the floor: `didUpdateWidget` fires before `_onMapCreated`, sees `_mapboxMap == null`, logs a warning, and returns — the new coordinates are never re-applied once the Mapbox controller arrives.

## Context from Comments

The repro evidence on the issue is a 3-open logcat trace where every open hit `Using GPS location for default camera position` but the two Austin opens *also* emitted

```
WARNING: Map coordinates changed but mapboxMap is null - cannot animate
```

…and the home-rendering open did not. The warning is a 1:1 marker for the buggy outcome.

The issue author attached a video ("Broken" comment) showing the alternating behavior. No other comments — this is a fresh, self-contained report; the analysis in the issue body is the only source of clarification beyond the logs.

## Codebase Findings

What exists today, anchored to real files:

- `app/lib/presentation/widgets/location/location_modal_widgets.dart:50-79` — `_LocationMapPreviewState`. `didUpdateWidget` (L55-73) consults `_mapboxMap`:
  - non-null + coords changed → `_flyToPosition`
  - null + coords changed → `_log.warning('Map coordinates changed but mapboxMap is null - cannot animate')` and silently drop the change
- `app/lib/presentation/widgets/location/location_modal_widgets.dart:129-135` — `_onMapCreated` only stores the controller in `setState(() { _mapboxMap = mapboxMap; })`. It does **not** check whether `widget.latitude/longitude` has advanced past the initial `cameraOptions`.
- `app/lib/presentation/widgets/location/location_modal_widgets.dart:81-101` — `_flyToPosition` issues `MapboxMap.flyTo` with a 500ms animation. Safe to call on a freshly-created controller once it exists (other call sites already do).
- `app/lib/presentation/widgets/location/location_modal_widgets.dart:137-183` — `_onStyleLoaded` runs *after* `_onMapCreated`. It enables the user-location blue dot, sets the basemap theme, and (if `widget.onCameraChanged != null`) calls `_notifyCameraPosition()`. Reading `_mapboxMap` here is always safe.
- `app/lib/presentation/widgets/location/location_modal_widgets.dart:241` — `key: const ValueKey('location_picker_map')`. Intentional to keep the same `MapWidget` element across rebuilds inside one modal; not a cross-modal sharing mechanism.
- `app/lib/presentation/widgets/location/location_picker_modal.dart:843-874` — `_buildMapPreview` priority chain. When every higher-priority source is null on the first build, it lands on the hardcoded fallback `30.2672 / -97.7431`. The "ultimate fallback" comment at L853, L858 already acknowledges this is meant to be rare.
- `app/lib/presentation/viewmodels/location_picker_view_model.dart:258-321` — `initializeDefaultCamera`. Async (awaits user-repository fetch + `userLocationProvider`). Publishes the chosen coordinate into `state.defaultCameraLatitude/Longitude`.
- `app/lib/core/utils/location_picker_helper.dart:19` — `hasSavedLocationId` (introduced in #1991 / PR #2014). Already routes the empty-string `locationId == ""` case into the same code path as `null`, so `initializeDefaultCamera` *does* fire for TBD experiences. The current bug is downstream of that fix.

**Sequence on a buggy open (anchored in real timestamps from the issue):**
1. `t=0ms` — `LocationPickerHelper.showLocationPicker` called. Modal pushed.
2. `t≈0ms` — Modal's `initState` postFrameCallback fires, kicks off `initializeDefaultCamera` (async).
3. `t≈0ms` — Modal builds. `state.defaultCameraLatitude` is still `null`. `_buildMapPreview` falls through every priority and resolves `lat,lng = 30.2672, -97.7431`.
4. `t≈0ms` — `LocationMapPreview` mounted with `widget.latitude/longitude = (Austin)`. `MapWidget` constructed with `cameraOptions: Point(Austin)`. Native view begins initializing.
5. `t≈16ms` — `initializeDefaultCamera` finishes, `state.defaultCameraLatitude/Longitude` becomes GPS. Modal rebuilds. `LocationMapPreview` receives `widget.latitude/longitude = (home)`.
6. `t≈16ms` — `didUpdateWidget` fires. `oldWidget.latitude (Austin) != widget.latitude (home)`. But `_mapboxMap` is still null because `_onMapCreated` hasn't fired. **Warning logged, change dropped.**
7. `t≈40ms` — `_onMapCreated` fires. `_mapboxMap` set. No replay of pending coords. Map stays on Austin.
8. `t≈140ms` — `_onStyleLoaded`. Blue dot enabled, theme set. Camera still on Austin.

On a non-buggy open (3rd in the trace) `_onMapCreated` raced ahead of step 5 — by the time `didUpdateWidget` fires, `_mapboxMap` is already non-null and `_flyToPosition` succeeds.

## Potential Solutions

### Option A — Replay pending coordinates in `_onMapCreated` (recommended in issue body)
- **Approach:** After `_onMapCreated` sets `_mapboxMap`, immediately call `_flyToPosition(widget.latitude, widget.longitude)`. If the initial `cameraOptions` and `widget.lat/lng` are already equal this is a no-op (flyTo to current position). If they diverge — the buggy case — the map snaps/flies to the correct camera.
- **Pros:** Single-place, single-line behavioral fix. The race becomes a non-event because whichever async wins first, the camera ends up correct. Mapbox supports `flyTo` on the freshly-created controller (other code paths already issue flyTo before style-load). The "Map coordinates changed but mapboxMap is null" warning becomes unreachable in normal operation, which itself is a good test signal.
- **Cons:** On the buggy code path, you may see Austin for the duration of one frame plus the 500ms flyTo animation back to home. That's still a regression from "instant correct" but a massive improvement over "stuck on Austin forever". Hidden behind the style-load loading state in practice.
- **Effort:** Trivial.

### Option B — Defer building the `MapWidget` until camera is decided
- **Approach:** Render a glass-skeleton placeholder in `_buildMapPreview` while `state.isLoadingDefaultCamera == true`. Mount the `MapWidget` only once a camera is chosen. The `cameraOptions` passed into `MapWidget` is then guaranteed correct on first build.
- **Pros:** No Austin frame ever. The platform view is created exactly once, with correct coordinates. Side-benefit: avoids the wasted Mapbox style-load cycle for a discarded camera.
- **Cons:** Wider change (state machine adds a "waiting for camera" gate; `isLoadingDefaultCamera` is currently set but never read for gating). Adds a perceptible "loading" beat on the first open even when GPS is cached and the answer is instant — eliminating the snappy "instant home" experience for the lucky case.
- **Effort:** Small. The skeleton widget pattern is already used in `_buildSavedLocationChips` (L745-751).

### Option C — Lift camera resolution out of the modal (eager warm-up)
- **Approach:** Resolve `userLocationProvider(LocationIntent.precisePin)` and the user's residence eagerly elsewhere — e.g. when the create flow opens, or on app start — so by the time the location picker is invoked the providers are already populated. The picker then reads them synchronously on first build and bypasses the race entirely.
- **Pros:** Best perceived latency on first open. Doesn't change the modal at all — just removes the timing window in which the race can fire.
- **Cons:** Widest blast radius (touches create flow, app boot). Doesn't *fix* the underlying race in `LocationMapPreview` — only makes it statistically less likely. A future surface that opens the modal without warming the providers re-introduces the bug.
- **Effort:** Medium.

## Recommendation

**Option A.** The cause is unambiguous (silently-dropped state update) and the fix is one block of code in one widget. It closes the bug categorically; the residual "one-frame Austin flash plus 500ms flyTo on the buggy timing path" is acceptable both visually and as a UX guarantee (correct end state, every time). The `Map coordinates changed but mapboxMap is null` warning becomes a meaningful signal again — its appearance under normal conditions should fail tests, and the absence of it is provable by the fix.

If the residual flash is objectionable in practice (we'll know after the fix-video step), layer Option B as a follow-up — gate the `MapWidget` on `!state.isLoadingDefaultCamera`. That sits cleanly on top of Option A; the helpers don't conflict.

Do **not** ship Option C as the fix for #2027. It's a perceived-latency improvement worth pursuing separately, but it doesn't fix the bug.

## Open Questions

- **Should the post-`_onMapCreated` replay be unconditional or guarded?** — Recommendation: **unconditional.** A flyTo to the current camera position is effectively a no-op (Mapbox treats it as a 500ms animation that doesn't visibly move), and the guard cost is more code than the benefit. The `_mapboxMap == null` warning logic in `didUpdateWidget` can stay (defensive logging), but should become unreachable in practice — that's a good signal.
- **Do we need to also call `_updateMarker()` from `_onMapCreated`?** — Recommendation: **no, not in this PR.** Markers are rendered inside `_onStyleLoaded` (L166-176) when `!widget.showUserLocationMarker`, after the style is ready. That path is correct. Mixing marker re-creation into `_onMapCreated` would invite a different race (creating an annotation manager before the style loads); the existing flow handles it.
- **\[Audit] Will `_onStyleLoaded`'s work clobber the replay flyTo?** — Recommendation: **no.** `_onStyleLoaded` calls `MapHelper.enableUserLocation` (`app/lib/core/utils/map_helper.dart:124-153`), which only writes `LocationComponentSettings` (puck/pulse) — it does not call `setCamera` or `flyTo`. The other style-load work (`setStyleImportConfigProperties`, annotation manager) is camera-neutral. So the replay flyTo from `_onMapCreated` is safe — its target survives style-load.
- **What about the `key: const ValueKey('location_picker_map')` retaining native state across modal opens?** — Recommendation: **leave alone.** The key is scoped inside `_LocationPickerModalState`'s build subtree; it does not survive modal disposal, so it cannot cause cross-modal camera leakage. The race is purely intra-open.
- **Should we tighten `_buildMapPreview`'s Austin fallback to a more local default (e.g. user's last-known camera) to soften the misfire even without Option A?** — Recommendation: **no.** "Austin" is a deliberately recognizable fallback; if we soften it we hide the next regression behind a plausible-looking but wrong location. Fix the bug, keep the fallback obviously wrong.
- **Test coverage?** — Recommendation: per [docs/client/testing.md](../client/testing.md) ("test business logic, not Flutter widgets"), this is borderline — the bug lives in a widget's controller-arrival lifecycle. Two pragmatic options: (a) widget test with a mock `MapboxMap` controller asserting that `flyTo` is invoked with the latest `widget.latitude/longitude` after `_onMapCreated` runs, or (b) plain unit test of a tiny extracted helper that captures "do I need to replay coords?" given (old props, new props, controller-just-arrived). **\[Audit] Pick (a)** — observing behavior end-to-end matches testing.md's "test behavior, not internals" rule. Do **not** refactor `_flyToPosition` to expose its target just for testing; that's the exact "make internals public for the test" anti-pattern testing.md warns against. If the mock-controller test is too cumbersome, file the test gap as a follow-up rather than ship a brittle test.

## Compliance Requirements

**\[Audit]** — derived from `docs/client/*` and `CLAUDE.md`. These apply to the implementation phase below.

### Architecture (MVVM + Riverpod)
- **Pure widget-internal change.** `_LocationMapPreviewState` already owns the `MapboxMap` controller as private state; the fix is one extra method call inside an existing `_onMapCreated` handler. No new ViewModel, no new provider, no repository touch. Compliant with the MVVM rule at [client/architecture.md](../client/architecture.md) (widgets never call services; ViewModels are the only layer that calls repositories).
- **No new controllers, no manual cache maps.** Confirmed.
- **`setState` after async.** `_onMapCreated` is already `async`; the existing `setState(() { _mapboxMap = mapboxMap; })` runs synchronously after `await`-less work, and the proposed `await _flyToPosition(...)` after it follows the same pattern. The widget'"'"'s `mounted` check is not strictly required here because `_onMapCreated` is invoked by the platform view *while the widget is in the tree* — but adding `if (!mounted) return;` after the setState is a defensive nicety worth keeping.

### Caching
- **N/A.** No data-layer change. The bug is in widget lifecycle ordering, not data freshness.

### i18n
- **No new ARB keys.** The single string change in the plan is a `_log.warning(...)` message — a server/dev log line, not user-facing. Per [client/i18n.md](../client/i18n.md) and `scripts/check_i18n.js`, only user-visible strings need ARB keys. **\[Audit]** Verify by running `npm run lint:dart:i18n` after the change.

### Accessibility
- **No new tap surfaces, semantics labels, or modal entry points.** No new `GestureDetector` / `InkWell` / `IconButton`. No `CachedMediaImage`. No `Duration(...)` literal. The change is entirely below the widget'"'"'s public interface; existing a11y in `LocationPickerModal` (the parent) is unaffected.
- **`prefer_accessible_modal`, `avoid_color_only_status`, etc.** All N/A. **\[Audit]** Run `npm run lint:dart:a11y` after the change to confirm the empty allowlist stays empty.
- **Reduce-motion.** The 500ms `flyTo` animation is the only motion in this widget. Mapbox'"'"'s `MapAnimationOptions(duration: 500)` is not wrapped in `accessibleDuration` today — that'"'"'s a pre-existing gap, not a regression introduced here. If we want to wire it up, file a follow-up; it'"'"'s out of scope for #2027.

### Observability
- **No new analytics events.** The bug is a UI race; no business signal.
- **Existing `_log.warning(...)` becomes meaningful.** With the replay in place, that warning should be effectively unreachable under normal conditions. It stays as a defensive log line and gains diagnostic value: future appearances are a signal that the replay path itself failed, not the original race. **\[Audit]** The plan'"'"'s suggestion to soften the message text is fine but optional.

### Testing (per [docs/client/testing.md](../client/testing.md))
- **Behavior, not internals.** The plan'"'"'s "make `_flyToPosition` observable" sub-task is an anti-pattern; replaced in the Open Questions section above with the mock-controller widget-test approach.
- **No ViewModel change → no ViewModel test required.** The existing `location_picker_view_model_test.dart` is unaffected.

## Implementation Plan

### Phase 1: Replay pending camera coords in `_onMapCreated`
**Goal:** Eliminate the silent drop in `didUpdateWidget` when `_mapboxMap` is null — bug closed.

**Tasks:**
- [ ] In `app/lib/presentation/widgets/location/location_modal_widgets.dart:129-135`, after `setState(() { _mapboxMap = mapboxMap; })`, call `await _flyToPosition(widget.latitude, widget.longitude);`. Order matters — set first so subsequent paths see the controller, then flyTo. Add `if (!mounted) return;` between the setState and the flyTo as a defensive guard against widget disposal during the platform-view callback.
- [ ] Update the `didUpdateWidget` warning text at `location_modal_widgets.dart:71` to clarify the new invariant — e.g. `Map coordinates changed but mapboxMap is null - will replay when _onMapCreated fires`. Optional but useful for future debuggers.
- [ ] Add a widget test in `app/test/presentation/widgets/location/location_map_preview_test.dart` (create if missing) following the pattern outlined in Open Questions option (a): mount `LocationMapPreview` with a mock `MapboxMap` controller, simulate the prop-change-before-`_onMapCreated` sequence, assert that `flyTo` is invoked with the latest `(lat, lng)` after the controller arrives. Do **not** make `_flyToPosition` testable by widening its visibility — the test observes Mapbox SDK calls on the mock, not internal state.
- [ ] Manually verify on Pixel: open create flow → Set location 10× rapidly. Map lands on GPS/residence every time. Capture a fix video for the PR description.
- [ ] Grep logcat for `Map coordinates changed but mapboxMap is null` during the manual run — confirm it does not appear under normal conditions (it may still appear if you scroll-dismiss the modal mid-Mapbox-load, which is acceptable).
- [ ] Run `npm run lint:dart`, `npm run lint:dart:i18n`, `npm run lint:dart:a11y` locally and confirm green.

**Validates:** The exact log line `Using GPS location for default camera position` is followed by a correctly-centered map regardless of which async wins. The bug is closed.

### Phase 2 (optional follow-up, file separately if needed): Skeleton placeholder for first-open polish
**Goal:** Remove the one-frame Austin flash even on the slow-Mapbox timing.

**Tasks:**
- [ ] Read `state.isLoadingDefaultCamera` in `_buildMapPreview`. When true and no other source has populated coordinates, render a glass-style skeleton placeholder instead of the `MapWidget`.
- [ ] Validate that gating doesn't introduce a perceived delay on the fast-path (GPS cached).

**Validates:** No visible Austin frame on the buggy timing. Pure visual polish — defer if Phase 1 already looks clean in the fix video.

## Related Issues

- #1991 / PR #2014 — fixed the empty-string `locationId` guard. Made this race observable on TBD experiences and create flows; previously the picker's misbehavior was masked by `initializeDefaultCamera` never even firing for those surfaces.
- #2018 — orthogonal: `initializeDefaultCamera` still prefers residence over GPS for non-profile surfaces. Not relevant to this fix.
