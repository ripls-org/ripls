# Issue #1961: Location is not updating in preview

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1961
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Audit Notes (2026-05-15)

The plan was reviewed against `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`,
`docs/client/accessibility.md`, and `docs/client/testing.md`. No
server changes are proposed, so `docs/server/*` audit lanes do not
apply.

Changes made during the audit:

1. **Flagged architecture violation.** The current widget
   (`_loadLocationDisplayText`) calls `LocationRepository` directly,
   which violates "Screens/Widgets never access repositories
   directly." Plan now elevates Phase 2 as the real fix and marks
   Phase 1 as throwaway-if-not-urgent.
2. **Async-safety added to Phase 2.** Making `updateLocation` async
   requires `ref.mounted` checks after the repo await — the plan now
   spells that out (and the two-step `copyWith` that gives the UI a
   loading affordance without flicker).
3. **i18n violation called out.** Hardcoded `'Loading location...'`
   / `'Tap to set location'` fail `npm run lint:dart:i18n`. Phase 2
   tasks now include adding ARB keys to `app_en.arb` and
   `app_es.arb` and running `npm run generate`.
4. **ViewModel state stores `Location`, not a pre-formatted
   string.** Aligns with i18n rule that "ViewModels expose typed
   state, widgets resolve to localized text in `build()`."
5. **Testing tier reordered.** Per `testing.md`, primary coverage
   is a ViewModel test with `ProviderContainer` + mocked
   `LocationRepository`; widget test is secondary. A
   disposal-safety case is added per `architecture.md` §"Disposal
   Testing".
6. **Caching scope clarified.** The location picker's
   `saveLocation` already populates the repo cache before
   returning, so no additional `LocationRepository.invalidate(...)`
   plumbing is required — noted to prevent a follow-on PR adding
   unnecessary invalidation.

## Problem Statement

When a user is in the request creation flow and uses the location picker
to change the location, the preview row in the request creation modal
keeps displaying the **previous** location name. Reopening the picker
correctly shows the new location, and submitting the request actually
stores the new location — so the underlying state is updated correctly.
Only the preview's text label is stale.

The bug is reported on iOS (app 0.20.1, OS 26.4.2) but the failing code
path is platform-independent Flutter logic.

## Context from Comments

The issue has no comments. The reporter is the user `user10@example.org`
(the codebase owner), and the report includes two screenshots showing the
stale preview vs. the correctly-updated picker.

Because there is no discussion, the description's framing is the only
signal — but the failure mode is precise enough that we don't need
clarification: the picker's return value reaches the ViewModel (proven
by the modal re-open and by the final submission), and the preview
widget that reads the location for display does not re-render with the
new value.

## Code Walkthrough

The preview lives in
[request_preview_modal.dart](app/lib/presentation/screens/request/request_preview_modal.dart).

Key state on the widget:

- [`String? _locationDisplayText`](app/lib/presentation/screens/request/request_preview_modal.dart#L44)
  — a **local widget field** holding the formatted location string
  used by the row.

Population flow:

- In `initState` via a post-frame callback, the widget calls
  [`_loadLocationDisplayText()`](app/lib/presentation/screens/request/request_preview_modal.dart#L82).
  That async method reads `genRequestProvider`, decides among three
  priorities (selected → geocoded → generated/primary), `await`s
  `LocationRepository.getLocation(...)` when needed, and finishes with
  `setState(() { _locationDisplayText = ... })`.
- When the user changes the location, `_showLocationPicker` awaits the
  picker, then calls
  [`ref.read(genRequestProvider.notifier).updateLocation(result)`](app/lib/presentation/screens/request/request_preview_modal.dart#L223)
  followed by
  [`await _loadLocationDisplayText()`](app/lib/presentation/screens/request/request_preview_modal.dart#L224).

### Architectural violation worth fixing in the same pass

`_loadLocationDisplayText` does
`ref.read(locationRepositoryProvider).getLocation(...)` directly from
the widget. That violates the rule in
[`docs/client/architecture.md`](docs/client/architecture.md) §"Key
Architectural Rules":

> **Screens/Widgets never access repositories directly … All data
> flows through ViewModels.**

Today this is the only thing keeping the bug somewhat working (the
widget reaches around the ViewModel to render). Option A below also
removes the violation; Option B preserves it. Audit-flagged: if we
ship Option B, we are knowingly carrying the violation forward — Phase
2 is the real fix and should not be deferred.

Rendering flow:

- [`_buildLocationSelector`](app/lib/presentation/screens/request/request_preview_modal.dart#L585)
  does `ref.watch(genRequestProvider)`, so it rebuilds on every
  ViewModel change.
- It calls
  [`_getLocationDisplayText(state)`](app/lib/presentation/screens/request/request_preview_modal.dart#L670-L684),
  which has this shape:

  ```dart
  if (state.selectedLocationId != null) {
    return _locationDisplayText ?? 'Loading location...';
  }
  if (state.geocodedLocation != null && state.geocodedLocation!.name.isNotEmpty) {
    return state.geocodedLocation!.name;
  }
  return _locationDisplayText ?? 'Tap to set location';
  ```

So `_buildLocationSelector` rebuilds when the ViewModel updates, but
the value it actually shows comes from a **non-reactive local field**
that is only refreshed by an asynchronous side-effect inside
`_showLocationPicker`. There is no `ref.listen` watching
`selectedLocationId`, no `FutureProvider.family`, nothing that ties
the displayed string to the state value it derives from.

The picker itself does the right thing: the picker view-model's
[`saveLocation`](app/lib/presentation/viewmodels/location_picker_view_model.dart#L90-L148)
creates the new location server-side, then calls
`getLocation(locationId)` so the LocationRepository's TTL cache
(`'location:<id>'`) is freshly populated *before* the picker returns.
So by the time `_loadLocationDisplayText()` runs on the preview side,
the repository cache already has the right data — meaning the failure
is in the preview's own code, not in caching or in the picker.

### Most likely failure modes

1. **Race / dispose:** `_loadLocationDisplayText` is `await`ed after
   `updateLocation`, but it does an async repo fetch. If the user
   navigates away or the modal is rebuilt before the `setState` line,
   the `if (!mounted) return;` short-circuits silently and the
   `_locationDisplayText` is never refreshed. The `mounted` checks
   throughout that method swallow these cases without setting state to
   anything (the user keeps seeing the old cached string).
2. **`updateLocation` fired from another path that doesn't refresh the
   text:** `updateLocation()` is a plain ViewModel setter. If any
   other path mutates `selectedLocationId` (today nothing else does,
   but it's a latent footgun), the preview will rebuild with the old
   `_locationDisplayText`.
3. **An exception inside `getLocation` falls through:** the
   `try/catch` around Priority 1 only logs and `continue`s — without
   `selectedLocationId != null` ever taking a different branch — so
   `_locationDisplayText` is never overwritten and the prior text
   persists.

All three converge on the same architectural smell: **the displayed
text is local widget state that mirrors ViewModel state, instead of
being derived from ViewModel state**.

## Potential Solutions

### Option A: Make the location text a derived ViewModel field

- **Approach:** Move the resolved display string into
  `GenRequestState` (e.g. add `Location? resolvedLocation` to
  `gen_request_view_model.dart` and populate it inside
  `updateLocation`, `_handleGeocoded`, and the AI-streaming path).
  Drop `_locationDisplayText` from the widget.
  `_getLocationDisplayText` reads it straight from `state` and
  formats with `LocationFormatter` at render time. Per
  [`docs/client/i18n.md`](docs/client/i18n.md), the widget — not the
  ViewModel — owns the localized fallback strings (`context.l10n`),
  so the ViewModel exposes the raw `Location?` and the widget chooses
  between `LocationFormatter.formatLocationNameShort(loc)` and an
  l10n key for "Tap to set location" / "Loading location".
- **Pros:** Single source of truth. The preview cannot show stale text
  because the rebuild and the value travel together in the same
  `state = state.copyWith(...)` call. Eliminates the dispose/race
  bullet entirely. Also fixes the architecture violation (the widget
  no longer touches `LocationRepository`) and the i18n violation
  (hardcoded English fallbacks become l10n keys).
- **Cons:** `updateLocation` becomes async (needs a repo fetch). The
  notifier must do the standard `ref.mounted` dance around the
  await — see
  [`docs/client/architecture.md`](docs/client/architecture.md)
  §"Async Safety in Notifiers". Streaming AI flow that sets
  `geocodedLocation` needs to populate `resolvedLocation` at the same
  time. Slightly larger blast radius.
- **Effort:** Medium.

### Option B: Add a `ref.listen` that triggers `_loadLocationDisplayText`

- **Approach:** Inside `build`, add
  `ref.listen<GenRequestState>(genRequestProvider, (prev, next) { if
  (prev?.selectedLocationId != next.selectedLocationId ||
  prev?.geocodedLocation != next.geocodedLocation ||
  prev?.generatedLocationId != next.generatedLocationId)
  _loadLocationDisplayText(); })`. Keep `_locationDisplayText` as
  local state, but force it to re-resolve whenever any input changes.
- **Pros:** Minimal diff. Works no matter who calls `updateLocation`.
  Doesn't require ViewModel surgery.
- **Cons:** Still has the dispose-mid-fetch silent-swallow problem
  unless we also make the `if (!mounted) return;` paths set the
  display text to a sentinel (e.g. the geocoded fallback) instead of
  bailing. Two sources of truth (state + local cache) remain. **Does
  not fix the architecture violation** — the widget still calls
  `LocationRepository` directly. Marked as "quick fix only", not a
  destination.
- **Effort:** Small.

### Option C: Switch the read to a `FutureProvider.family<Location, String>`

- **Approach:** Introduce a Riverpod `FutureProvider.family<Location,
  String>` that wraps `LocationRepository.getLocation(id)`. Replace
  `_locationDisplayText` with `ref.watch(locationByIdProvider(state.selectedLocationId!))`
  inside the builder, formatting the result inline.
- **Pros:** Idiomatic Riverpod. Reusable across other screens that
  display a location by ID (e.g. the loan flow, the gear detail
  screen). Auto-invalidates if we ever add a refresh provider.
- **Cons:** Larger change. The Location-by-ID provider doesn't exist
  yet, so we're introducing a new layer. Doesn't solve the
  `geocodedLocation` vs. `generatedLocationId` priority logic without
  more code.
- **Effort:** Medium-Large.

## Recommendation

**Option A.** The root cause is that the widget locally mirrors a
ViewModel value, and the bug will keep reappearing in other shapes
(initial render shows "Loading location...", another caller of
`updateLocation` skips the refresh, dispose-mid-fetch eats the
update) until the mirror is removed. Moving the display string into
`GenRequestState` aligns this screen with the rest of the
ViewModel — title, description, mediaId are all already in state —
and removes a class of bug rather than patching one instance.

If we want a faster ship, Option B is a defensible quick fix and can
land first as Phase 1; Option A would then graduate it to a permanent
fix. The plan below stages it that way.

## Open Questions

- **Does `updateLocation` also need to clear `geocodedLocation` when
  the user picks a location explicitly?** — Recommendation: yes.
  Today `geocodedLocation` is preserved (gen_request_view_model.dart:186-189),
  which means our display fallback chain still references an AI
  suggestion the user has explicitly overridden. Clearing it makes the
  state unambiguous and simplifies the display logic. Easy to do in
  the same `copyWith`.
- **Should the new state field be the formatted "short" name or the
  raw `Location` proto?** — Recommendation: store the raw `Location`
  (or its id+name pair) and format at render time. That way the
  formatter (`LocationFormatter.formatLocationNameShort`) can change
  without invalidating ViewModel data, and tests assert on the
  underlying object.
- **What happens on the AI streaming path that sets
  `geocodedLocation`?** — Recommendation: when the AI stream lands a
  `geocodedLocation`, set the display name to
  `geocodedLocation.name` in the same `copyWith`. Don't fire a repo
  fetch from the ViewModel — the geocoded result already has a name.
- **Do we need a regression test?** — Recommendation: yes, a widget
  test that pumps the preview modal, taps the location selector,
  has the mocked picker return a new locationId, and asserts the
  preview text changes. This is the kind of bug that re-emerges
  during refactors.

## Implementation Plan

### Phase 1 (optional — only if a same-day ship is needed): `ref.listen` quick fix

**Goal:** Stop the visible regression for users on the next build,
**knowing this leaves the architecture violation in place** (widget
still calls `LocationRepository` directly). Skip this phase and ship
Phase 2 if there is no urgency — Phase 1 is throwaway work otherwise.

**Tasks:**

- [ ] In [request_preview_modal.dart](app/lib/presentation/screens/request/request_preview_modal.dart),
      add a `ref.listen<GenRequestState>` inside `build` that watches
      for changes in `selectedLocationId`, `geocodedLocation`, and
      `generatedLocationId`, and calls `_loadLocationDisplayText()`
      when any of them change.
- [ ] In the existing `if (!mounted) return;` short-circuits inside
      `_loadLocationDisplayText`, fall back to the geocoded name or
      to clearing the cached string instead of silently returning, so
      a dispose-mid-fetch can't leave the user staring at stale text.
- [ ] Manually reproduce on the iOS simulator: launch request
      creation, let AI suggest a location, change it via the picker,
      confirm the preview row reflects the change.

**Validates:** That the user-visible bug is resolved, and that the
state→display sync gap is the actual root cause (not something
deeper in the picker save flow). Does **not** validate the
architecture is correct — that's Phase 2.

### Phase 2: Move location into `GenRequestState` (the real fix)

**Goal:** Eliminate the local mirror, restore the
ViewModel-owns-the-repository rule, and remove hardcoded English
fallback strings.

**Tasks:**

- [ ] Add `Location? resolvedLocation` to
      [GenRequestState](app/lib/presentation/viewmodels/gen_request_view_model.dart#L24-L92).
      Store the raw `Location` proto, not a pre-formatted string —
      keeps the formatter (and any future format changes) on the
      widget side and aligns with
      [`docs/client/i18n.md`](docs/client/i18n.md): "ViewModels
      expose typed state … widgets resolve to localized text in
      `build()`."
- [ ] Make `updateLocation` async. Inside it: set
      `state = state.copyWith(selectedLocationId: locationId,
      resolvedLocation: null)` first so the row immediately shows a
      "loading" affordance keyed off `selectedLocationId != null &&
      resolvedLocation == null`. Then `await
      _locationRepository.getLocation(locationId)`. After the await,
      guard with `if (!ref.mounted) return;` (per
      [`docs/client/architecture.md`](docs/client/architecture.md)
      §"Async Safety in Notifiers"), then write the second
      `copyWith` with `resolvedLocation: location`. Also clear
      `geocodedLocation` in the first `copyWith` (per the Open
      Questions answer above) so the fallback chain in
      `_getLocationDisplayText` becomes unambiguous.
- [ ] On the AI-streaming path that sets `geocodedLocation`, also
      populate `resolvedLocation` from the streaming payload if
      enough information is available, **or** leave it null and let
      the widget fall through to the `geocodedLocation.name` branch
      (no extra repo fetch — the geocoded result already has a
      name). Pick one path and document it inline.
- [ ] Delete `_locationDisplayText` and `_loadLocationDisplayText`
      from [request_preview_modal.dart](app/lib/presentation/screens/request/request_preview_modal.dart).
      The widget no longer reads `locationRepositoryProvider`.
- [ ] Rewrite `_getLocationDisplayText` to derive from `state` only:
      ```
      if (state.resolvedLocation != null) →
        LocationFormatter.formatLocationNameShort(state.resolvedLocation!)
      else if (state.selectedLocationId != null) →
        context.l10n.<a11yOrUiLoadingLocationKey>
      else if (state.geocodedLocation has a name) →
        state.geocodedLocation!.name
      else → context.l10n.<requestLocationTapToSetKey>
      ```
- [ ] Add the two new keys to both
      [app_en.arb](app/lib/l10n/app_en.arb) and
      [app_es.arb](app/lib/l10n/app_es.arb) (e.g.
      `requestLocationLoading`, `requestLocationTapToSet`). Run
      `npm run generate` to regenerate `AppLocalizations`. This is
      required by
      [`docs/client/i18n.md`](docs/client/i18n.md) — `npm run
      lint:dart:i18n` rejects raw user-visible strings in widgets.
- [ ] If Phase 1 shipped, remove the `ref.listen` it added — no
      longer needed.

**Validates:** Preview text is now a pure derivation of ViewModel
state; widget no longer touches a repository; hardcoded English
strings are gone.

### Phase 3: Regression tests

**Goal:** Catch a regression of this exact behavior in CI. Following
the testing tier hierarchy in
[`docs/client/testing.md`](docs/client/testing.md), the **primary**
test is a ViewModel-level unit test, with an optional widget test on
top.

**Tasks:**

- [ ] **Primary — ViewModel test.** Add a test under
      `app/test/presentation/viewmodels/gen_request_view_model_test.dart`
      that:
      - Sets up a `ProviderContainer` with a mocked
        `LocationRepository` (via `@GenerateMocks` + Mockito).
      - Calls `notifier.updateLocation('locA')` and asserts a
        loading-shape state (`selectedLocationId == 'locA'`,
        `resolvedLocation == null`) lands first.
      - Awaits the call and asserts `resolvedLocation` is the
        location returned by the mock, and `geocodedLocation` was
        cleared.
      - Calls `updateLocation('locB')` and verifies the new
        location replaces the prior `resolvedLocation`.
      - Includes a disposal-safety case per
        [`docs/client/architecture.md`](docs/client/architecture.md)
        §"Disposal Testing": dispose the container mid-fetch and
        confirm no throw.
- [ ] **Secondary — widget smoke test.** Add a widget test under
      `app/test/presentation/screens/request/` that pumps
      `RequestPreviewModal` with the mocked notifier producing state
      transitions, and asserts the rendered text changes. Keep this
      thin — the heavy lifting is in the ViewModel test per the
      testing-pyramid guidance ("test business logic, not widgets").
- [ ] Run `npm run test:app` and confirm green locally.

**Validates:** The fix is durable across future refactors of the
request creation flow.

## Related Issues

- None found in the open issue list referencing the request preview
  or location picker sync. (Worth a `gh issue list --search "location
  preview"` pass before starting Phase 1 to confirm no parallel work
  is in flight.)
