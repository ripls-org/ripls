# Issue #1958: Directions app doesn't open

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1958
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Audit Notes

**Audited against:** `docs/client/architecture.md`, `docs/client/i18n.md`,
`docs/client/accessibility.md`, `docs/client/observability.md`,
`docs/client/testing.md`.

**Findings:**

1. **Logging primitive corrected:** original plan said "developer.log (or
   logging equivalent)". The canonical primitive per
   `docs/client/observability.md` is `ObservableLogger.named('Foo')`, which
   gives PII redaction via `LogRedactor` and auto-records breadcrumbs at
   warning/error level (flushed to Crashlytics on the next crash). Phase 1
   now specifies `ObservableLogger.named('OpenDirections').warning(...)`.
2. **i18n: convention, not lint:** original plan implied CI would catch the
   hardcoded `'Could not open directions app'` toast. It won't —
   `scripts/check_i18n.js` only inspects `Text('...')` and `tooltip: '...'`
   with a narrow keyword shortlist (Cancel, Save, Delete, …), so the
   `ToastHelper.showError(context, '...')` arg slips through. Per
   `docs/client/i18n.md` ("Strings belong in ARB files, not Dart code") the
   localization is still the correct fix; the plan now frames it that way and
   spells out the required `@key` metadata block.
3. **Pure helper boundary tightened:** `docs/client/testing.md` directs us to
   "test business logic, not Flutter widgets." Phase 2's URL-builder helper
   now explicitly accepts plain arguments (`isIOS`, `latitudeDeg`,
   `longitudeDeg`, `name`) — no `BuildContext`, no `Platform` — so the test
   runs without any Flutter or platform setup. `openDirections` stays as the
   `BuildContext`-aware launcher that composes the helper.
4. **A11y + existing `context.mounted` pattern OK:** the
   `actionSemanticsLabel: context.l10n.contentRowsGetDirections` chain on the
   call sites already satisfies `docs/client/accessibility.md`. The existing
   `if (context.mounted)` guards around the toast (lines 90 and 95) follow
   the "Caller is responsible for `mounted` checks" contract and stay.
5. **Server docs (architecture / observability) not applicable:** this is a
   pure-client manifest + UI fix; no server changes are in scope.

## Problem Statement

On Android, tapping the directions icon on a location row does not launch the
device's maps app. Instead, the user sees a toast at the bottom reading
"Could not open directions app" (confirmed by the user on a Pixel 8 / Android
16, prod build 6957, app v0.21.0). iOS is unaffected.

The toast text comes from `openDirections()` in
`app/lib/presentation/widgets/location/location_picker_helpers.dart:91,96`,
which fires when either `canLaunchUrl()` returns false or `launchUrl()`
throws. The Android branch builds a `geo:` URI; the iOS branch builds an
`http://maps.apple.com/` URL.

The almost-certain root cause is Android 11+ **package visibility**: as of
API 30, an app must declare in its manifest which intents/packages it wants
to query, or `canLaunchUrl()` returns false even when a maps app is
installed. The project's
`app/android/app/src/main/AndroidManifest.xml:69-78` declares a `<queries>`
block, but only for `PROCESS_TEXT` and calendar `INSERT` intents — there
is **no entry for the `geo` scheme**. So `canLaunchUrl(Uri.parse('geo:...'))`
returns false on every modern Android device regardless of whether Google
Maps is installed, and the user gets the toast.

A second, smaller contributor: when only a place name is available the URI
is built as `geo:0,0?q=<encoded name>` (line 79). Even with the `<queries>`
fix this works for Google Maps but is less reliable than a
`https://www.google.com/maps/search/?api=1&query=...` fallback — and any
device whose default maps app isn't Google Maps may still fail to resolve
the name-only form. Worth fixing in the same pass.

## Context from Comments

There is one comment: maintainer `tescobar` confirmed on 2026-05-16 that this
is "Android only — works on iOS." That matches the root cause: iOS does not
have Android's `<queries>` package-visibility model and the
`http://maps.apple.com/` URL is universally launchable on iOS.

The current branch (`directions_link`, no commits ahead of `main`) appears to
be where someone intended to do this work but hasn't started yet. The most
recent direction-related commit on `main` is `2c9d2974d` "Always show
directions button" — which made the failing UI path more reachable, plausibly
explaining why the bug is now visible in the wild.

## Potential Solutions

### Option A: Add `geo` to the manifest `<queries>` block (minimal fix)
- **Approach:** Append a `<intent>` for `action.VIEW` + `data android:scheme="geo"`
  inside the existing `<queries>` block in `AndroidManifest.xml`. No Dart
  changes required.
- **Pros:** One-line manifest change; smallest possible diff; matches the
  pattern documented by `url_launcher` for Android 11+.
- **Cons:** Doesn't address the name-only fallback being weaker on non-Google
  default map apps. Doesn't address the swallowed exception path
  (`catch (e)` at line 94) — failures are still silent for telemetry.
- **Effort:** Small (manifest line + a manual smoke test on the emulator).

### Option B: Manifest fix + URL hardening + better failure signal (recommended)
- **Approach:** Do Option A, **plus**:
  1. Replace the name-only Android fallback with the canonical Google Maps
     web URL: `https://www.google.com/maps/search/?api=1&query=<encoded>`.
     This launches in whichever app handles `https`-mapped Google Maps
     intent filters; if none, it cleanly falls through to the browser.
     Also declare an `<intent>` for `https` scheme in `<queries>` (already
     implicitly covered by the existing app-link intent filters but worth
     auditing).
  2. Log the failure via `ObservableLogger.named('OpenDirections').warning(...)`
     with `{platform, uri, hasCoords, hasName}` and the exception, so the
     next crash report includes a breadcrumb explaining which URI was
     attempted. Use `warning` (not `error`) — the user sees a toast, no
     uncaught throw, and `RpcErrorHandler`-style crash reporting is not
     appropriate. Per `docs/client/observability.md` PII is auto-redacted by
     `LogRedactor`; the place name in the URI passes through but is not PII.
  3. Drop the now-redundant `canLaunchUrl` gate where it doesn't add value
     — `launchUrl` already throws on failure, and `canLaunchUrl` is what's
     misleadingly returning false here. Keep a single try/catch.
- **Pros:** Fixes the root cause and the latent reliability issues around it;
  gives us breadcrumbs the next time a similar report comes in.
- **Cons:** Slightly bigger diff; requires touching Dart logic, not just the
  manifest.
- **Effort:** Small-to-medium (≈30 lines + a unit test for the URL builder).

### Option C: Switch to a native plugin (`map_launcher` or similar)
- **Approach:** Replace the hand-rolled URL building with a package that
  enumerates installed maps apps and launches them via platform channels.
- **Pros:** Most robust for users with non-default maps (Waze, OsmAnd,
  Organic Maps); offers a picker UX.
- **Cons:** New dependency for what is a one-tap convenience; adds maintenance
  surface; still requires `<queries>` declarations for each supported app.
  Overkill for a single feedback report.
- **Effort:** Medium (dep add, integration, picker UX decisions, tests).

## Recommendation

**Option B.** The manifest entry is the actual fix; bundling the name-fallback
hardening and adding a log line costs almost nothing and prevents the
next-most-likely failure mode (default maps app isn't Google Maps, or coords
are missing) from surfacing as the same opaque toast. Option C is correct in
the long run but premature for one prod report — revisit if we see this
pattern repeat with Waze/OsmAnd users.

## Open Questions

- **Should the manifest also declare an `<intent>` for `https` + Google Maps
  host, or rely on the implicit visibility from the existing app-link intent
  filter?** Recommendation: add an explicit `<queries>` entry for
  `action.VIEW` + `data scheme="https" host="www.google.com"`. App-link
  intent filters give visibility *to* the app from outside, not visibility
  *from* the app outward; the explicit query is cheap insurance.
- **Should the toast text become a localized ARB string instead of the
  hardcoded `'Could not open directions app'` it is today?** Recommendation:
  yes — per `docs/client/i18n.md` every user-visible string lives in ARB.
  Add `locationPickerDirectionsLaunchFailed` to `app_en.arb` with a
  `@locationPickerDirectionsLaunchFailed` metadata block (`description`:
  "Toast shown when the device cannot open the maps app for directions"),
  add the Spanish counterpart to `app_es.arb`, then use
  `context.l10n.locationPickerDirectionsLaunchFailed`. Note: the existing
  literal does not currently trip `npm run lint:dart:i18n` — that linter's
  pattern set is narrow — but fixing it is still required by convention.
- **Do we want analytics on the failure?** Recommendation: not yet — the
  `ObservableLogger.warning` breadcrumb is enough to triage future user
  reports via Crashlytics; adding a typed event class in `events.dart` and
  a Metabase chart for a now-rare error path is over-instrumentation.

## Implementation Plan

### Phase 1: Manifest + URL fix
**Goal:** Ship the actual bug fix so Android users can launch directions.
**Tasks:**
- [ ] Add `<intent><action android:name="android.intent.action.VIEW"/><data android:scheme="geo"/></intent>` to the `<queries>` block in `app/android/app/src/main/AndroidManifest.xml`.
- [ ] Add a parallel `<intent>` for `action.VIEW` + `scheme="https"` + `host="www.google.com"` in the same block.
- [ ] In `openDirections()`, change the Android name-only branch to build `https://www.google.com/maps/search/?api=1&query=<encoded>` instead of `geo:0,0?q=...`. Keep the coords branch on `geo:` (it works once the manifest is fixed and Apple Maps deep-linking on iOS already mirrors it).
- [ ] Replace the no-coords, no-name branch (`geo:0,0?q=`) with an early `ToastHelper.showError` — building a maps URL with nothing to search is a programming error and should signal it, not open the maps app to an empty query.
- [ ] Inside the `catch` (and before the `canLaunchUrl == false` toast), call `ObservableLogger.named('OpenDirections').warning('Could not launch directions URL', {'platform': Platform.isIOS ? 'ios' : 'android', 'uri': uri.toString(), 'has_coords': hasCoords, 'has_name': hasName}, error)`.
- [ ] Add ARB key `locationPickerDirectionsLaunchFailed` to `app_en.arb` (with `@locationPickerDirectionsLaunchFailed` metadata describing the toast) and `app_es.arb`. Replace both `ToastHelper.showError(context, '...')` literals with `ToastHelper.showError(context, context.l10n.locationPickerDirectionsLaunchFailed)`.
- [ ] Run `npm run generate` so `AppLocalizations` picks up the new key, then `npm run lint:dart` and `flutter test --reporter expanded`.
**Validates:** Manual smoke test on an Android emulator (`npm run start:app:android`) — tap directions on an experience, gear, request, and location-picker location row; confirm Google Maps opens in each case.

### Phase 2: Test coverage
**Goal:** Lock in the URL-builder behavior so a future refactor doesn't regress it.
**Tasks:**
- [ ] Extract URL construction from `openDirections()` into a pure helper with the signature `String? buildDirectionsUrl({required bool isIOS, double? latitudeDeg, double? longitudeDeg, String? name})` — explicitly **no** `BuildContext`, no `Location` proto type, no `Platform` lookup. Return `null` for the no-coords-no-name case so the launcher can short-circuit to the failure toast. This satisfies `docs/client/testing.md` ("test business logic, not Flutter widgets") — no widget tree or platform setup needed.
- [ ] `openDirections` keeps `BuildContext`, calls the helper, runs `canLaunchUrl` / `launchUrl`, owns logging and toasts.
- [ ] Add `app/test/presentation/widgets/location/location_picker_helpers_test.dart` covering: iOS coords, iOS name-only, iOS empty; Android coords (asserts `geo:lat,lng?q=lat,lng`), Android name-only (asserts `https://www.google.com/maps/search/?api=1&query=...` with encoding), Android empty (asserts `null`).
**Validates:** `flutter test --reporter expanded app/test/presentation/widgets/location/` passes; the helper's contract is now executable documentation.

### Phase 3: PR + release
**Goal:** Get the fix to users.
**Tasks:**
- [ ] Open PR from `directions_link` → `main` referencing #1958.
- [ ] After merge, cut a patch release (v0.21.2) following the release skill — this is a user-visible prod fix on a feature that was just released.
**Validates:** The next round of user feedback no longer mentions the directions error.

## Related Issues

- `2c9d2974d` "Always show directions button" — the recent commit that made
  the directions affordance more prominent, increasing the visibility of this
  pre-existing Android bug. Not a separate issue but worth noting in the PR
  description.
- #1174 "android-perm-collision" — prior Android manifest/permissions fix;
  precedent that the manifest is the right place for this class of fix.
