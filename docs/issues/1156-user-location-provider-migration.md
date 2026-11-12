# Issue #1156: Migrate remaining MapboxLocationService.getCurrentLocation() callers to userLocationProvider

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1156
**Filed:** 2026-04-17 | **Last activity:** 2026-04-17
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

PR #1143 introduced `userLocationProvider` (a `FutureProvider<Position?>` at
`app/lib/services/providers.dart:201`) plus `UserPositionResolver` and
`DeviceLocationService` as the single, session-cached path to the user's
device position. The provider invalidates on logout, after >10 min background,
and on pull-to-refresh. Direct callers of the legacy
`MapboxLocationService.getCurrentLocation()` method bypass all of that and
fire fresh `Geolocator.getCurrentPosition()` calls, which cost GPS wakeups,
take 100–300 ms each, and produced three duplicate fetches in a ~50-second
window during manual testing.

The acceptance criteria are (1) a single GPS fetch per cold-start session
under normal use, (2) `MapboxLocationService` reduced to Mapbox-API-only
methods (`searchAddresses`, `searchGeographicAreas`, `reverseGeocode`), and
(3) test-override ergonomics preserved for the search view model tests.

**Discrepancy worth flagging:** the issue body lists exactly two direct
callers (`search_view_model.dart` and `location_picker_view_model.dart`) but
codebase grep shows more:

| File | Line | How it calls GPS |
| --- | --- | --- |
| `app/lib/presentation/viewmodels/search_view_model.dart` | 42 | via `deviceLocationFetcherProvider` seam (initial search) |
| `app/lib/presentation/viewmodels/search_view_model.dart` | 592 | **direct** call in `_executeSearchWithLocation` (the issue missed this one) |
| `app/lib/presentation/viewmodels/location_picker_view_model.dart` | 167, 286 | **indirect** — goes through `LocationRepository.getCurrentLocation()`, which calls `MapboxLocationService.getCurrentLocation()` at `location_repository.dart:147` |
| `app/lib/data/repositories/location_repository.dart` | 147 | direct (GPS + reverse geocode combo) |
| `app/lib/core/utils/map_helper.dart` | 129 | direct (`enableUserLocation`) |
| `app/lib/core/utils/discover_map_helper.dart` | 263 | direct (center map when no results) |
| `app/lib/presentation/widgets/leaderboard/leaderboard_map_helper.dart` | 272 | direct (center leaderboard map) |
| `app/lib/presentation/screens/discover/discover_screen.dart` | 71, 82 | direct (load + center user puck) |

Deleting `MapboxLocationService.getCurrentLocation()` — which the issue's
acceptance criteria demand — requires all of these to be migrated, not just
the two named. The issue as written is therefore either a **scope that is too
narrow to satisfy its own acceptance criteria** or an **implicit ask to
migrate every caller**. The plan below assumes the latter and delivers the
full migration in a single PR (Option B).

## Context from Comments

No comments. The issue body is the entire specification. This is a single-
author cleanup follow-up to #1143 and was surfaced during manual testing of
that PR — low risk of surprise input from other reviewers. Flagging the
scope mismatch (above) is the only missing input.

## Potential Solutions

### Option A: Strict scope — migrate only the two named callers
- **Approach:** Change `search_view_model.dart` (lines 42 and 592) to use
  `ref.read(userLocationProvider.future)` and change
  `location_picker_view_model.dart` to read `userLocationProvider` instead
  of `_locationRepository.getCurrentLocation()` (calling
  `locationRepository.reverseGeocode(lat, lng)` on the result when an address
  is needed). Leave `MapboxLocationService.getCurrentLocation()` in place.
- **Pros:** Smallest diff. Matches the issue's Tasks checklist literally.
  Preserves the two-caller acceptance gain (deduplicating the search-path
  and location-picker GPS fetches).
- **Cons:** Fails the "MapboxLocationService contains only Mapbox API
  methods" acceptance criterion — the method stays, the `_permissionRequest`
  field stays, and 5+ other callers still bypass the cache. The session-
  level dedup benefit is partial; the duplicate-fetch pattern described
  in the issue can recur from the other callers.
- **Effort:** small.

### Option B: Full migration — every direct caller moves to userLocationProvider, delete legacy method
- **Approach:** Migrate every caller in the table above. For widgets that
  already have `ref` (via `ConsumerWidget` / `ConsumerStatefulWidget`),
  substitute `ref.read(userLocationProvider.future)` directly. For
  `map_helper.dart` / `discover_map_helper.dart` /
  `leaderboard_map_helper.dart` which are plain static helpers without
  `ref`, either (a) pass `Position?` in as an argument from the caller that
  does have `ref`, or (b) pass `ref` in and have the helper read the
  provider. Refactor `LocationRepository.getCurrentLocation()` into a pure
  `reverseGeocode(lat, lng)` path that callers combine with
  `userLocationProvider`. Delete
  `MapboxLocationService.getCurrentLocation()` and its
  `_permissionRequest` field.
- **Pros:** Fully satisfies the issue's acceptance criteria. One GPS fetch
  per session in all app flows. `MapboxLocationService` becomes a pure API
  wrapper. No future regression where a new caller picks the wrong method.
- **Cons:** Larger diff (~8 files + tests). Touches map helpers used in
  performance-sensitive map-load paths. The helper-refactor question
  ("inject `ref` vs. inject `Position?`") is a small architectural call
  that wants review.
- **Effort:** medium.

### Option C: Phased migration — ship Option A first, follow with Option B
- **Approach:** Phase 1 is exactly Option A, keeping the PR focused on the
  two callers the issue names and providing immediate observable win (fewer
  GPS fetches on cold start). Phase 2 migrates the remaining callers and
  deletes the legacy method. Phase 3 (trivial) deletes
  `deviceLocationFetcherProvider` and updates tests to the
  `userLocationProvider.overrideWith(...)` pattern.
- **Pros:** Each phase is a shippable, reviewable unit. If phase 2 hits a
  snag in a map-helper refactor, the phase-1 win is already merged.
  Matches the usual style of issue follow-ups in this repo.
- **Cons:** Two PRs instead of one — marginal overhead. Briefly leaves
  `MapboxLocationService.getCurrentLocation()` alive between phases, which
  is visibly inconsistent with the acceptance criteria until phase 2
  lands.
- **Effort:** medium (same total work as Option B, split).

## Recommendation

**Option B (single-PR full migration).** Reasoning: only a full migration
satisfies the issue's acceptance criteria — "MapboxLocationService class
contains only Mapbox API methods" requires deleting
`getCurrentLocation()`, which is only possible once every caller has
moved. The work is additive and low-risk: each caller migration is
independent, each falls back to the same (or better) behavior than
today, and the helper refactor (inject `Position?`) is a trivial
signature change with well-defined callers. Splitting into phases would
impose review-load savings at the cost of leaving the acceptance
criteria unmet between merges and leaving a confusingly half-migrated
view model in place. One PR is cleaner to review as a unit — reviewers
can confirm the grep-to-zero assertion in a single pass.

## Open Questions

- **The issue names only two callers but acceptance criteria require
  deleting the legacy method. Confirm we should expand scope to migrate
  all direct callers.** Recommendation: **yes, expand** — the acceptance
  criterion "`MapboxLocationService` class contains only Mapbox API
  methods" cannot be met otherwise. If we keep strict scope, rewrite that
  acceptance criterion to "the two named callers no longer call
  `getCurrentLocation` directly" and defer the deletion.

- **For plain static helpers (`map_helper.dart`,
  `discover_map_helper.dart`, `leaderboard_map_helper.dart`), inject
  `Position?` or inject `Ref`?** Recommendation: **inject `Position?`**
  from the caller. These helpers already take other parameters
  (`MapboxMap`, locations); adding `Position?` matches their style. The
  caller (a `ConsumerWidget` or `ConsumerState`) is the right place to
  read `userLocationProvider` — keeps helpers ref-free and easier to
  unit-test.

- **`LocationRepository.getCurrentLocation()` bundles GPS + reverse
  geocode. How do we split it?** Recommendation: **delete it; add
  `LocationRepository.reverseGeocode(double lat, double lng)` that wraps
  `MapboxLocationService.reverseGeocode`.** The two view-model callers
  (`useCurrentLocation`, `initializeDefaultCamera`) already have `ref` and
  read `userLocationProvider` to get the position. That leaves the
  repository with a single-responsibility method (reverse geocode) and
  keeps GPS acquisition in one place.

- **`search_view_model.dart:592` (`_executeSearchWithLocation`, direct
  GPS call) is not in the issue's task list. Migrate alongside the
  named callers?** Recommendation: **yes, migrate it.** It's in the
  same file as the provider being removed, it's a direct call to the
  exact method the issue targets, and leaving it would produce a
  weirdly half-migrated view model.

- **`userLocationProvider` may return `null`. The fallback behavior for
  each caller — what to display?** Recommendation: **match current fallback
  behavior per caller.** Search view model already falls back to Austin, TX
  (30.2672, -97.7431). Location picker already has an Austin fallback
  ladder. Discover screen's puck simply stays hidden. Preserve each.

## Implementation Plan

Single PR. Migrations grouped into logical sub-sections so reviewers can
walk the diff top-down. Each sub-section is a natural commit boundary if
we want a tidy commit sequence within the one PR.

### Phase 1: Full migration + deletion (single PR)

**Goal:** Migrate every caller of
`MapboxLocationService.getCurrentLocation()` to `userLocationProvider`,
delete the legacy method and its `_permissionRequest` field, and
confirm with grep + test suite + manual cold-start log check. Fully
satisfies all three acceptance criteria in one merge.

**Sub-section A — Repository-layer refactor (prerequisite).** Other
migrations depend on the new `reverseGeocode` method existing.

- [ ] `location_repository.dart` — add
      `Future<LocationResult> reverseGeocode(double lat, double lng)`
      that wraps `MapboxLocationService.reverseGeocode` with the same
      error-to-exception translation the deleted method used (throws
      on null result, rethrows on underlying error). No caching (per
      Finding 4 — matches existing behavior).
- [ ] `location_repository.dart` — delete `getCurrentLocation()`. Its
      only callers are the two view-model paths migrated in sub-
      section B.

**Sub-section B — View-model migrations (the two "named" callers + the
overlooked direct call in search).**

- [ ] `search_view_model.dart:42` — delete `deviceLocationFetcherProvider`.
- [ ] `search_view_model.dart:362` (`build()`) — replace
      `ref.read(deviceLocationFetcherProvider)` with an eager pre-read
      of `ref.read(userLocationProvider.future)` (same "resolve
      dependencies before async gap" pattern that's already in place).
- [ ] `_performInitialSearch` signature — swap
      `Future<geo.Position?> Function() locationFetcher` for a pre-
      resolved `Future<Position?> locationFuture`. Minimal diff;
      preserves the "no `ref.read` after async gap" invariant.
- [ ] `search_view_model.dart:592` (`_executeSearchWithLocation`) —
      replace `await MapboxLocationService.getCurrentLocation()` with
      `await ref.read(userLocationProvider.future)`. Keep the Austin
      fallback when the provider returns null. **Add an `if
      (!ref.mounted) return;` guard immediately after the await and
      before any `state =` assignment** (new async gap; see Finding 6).
- [ ] `location_picker_view_model.dart:159` (`useCurrentLocation`) —
      read `userLocationProvider` for the position, then call new
      `locationRepository.reverseGeocode(lat, lng)` for the address.
      On `null` position, route through the existing error message
      pathway.
- [ ] `location_picker_view_model.dart:246`
      (`initializeDefaultCamera`) — after the primary-residence
      branch, replace `_locationRepository.getCurrentLocation()` with
      `ref.read(userLocationProvider.future)` and use lat/lng directly
      for camera position. (No reverse geocode needed for camera
      centering.) **Preserve the existing ladder order**
      (residence → GPS → Austin). Do *not* collapse the whole method
      into a single `userLocationProvider` read: the resolver's order
      is GPS → residence, which is the opposite of what this picker
      wants ("where does this user live?" vs. "where is the user
      now?"). While editing this method, also simplify the residence
      branch by replacing the inline `userRepository.get(userId) →
      check primaryResidenceLocationId` code with
      `UserLocationInitializer.getPrimaryLocationIfSet(ref)` (added in
      commit 6af15d67 as the cheap, side-effect-free lookup) — then
      hand the returned ID to `_locationRepository.getLocation(...)`
      as before. Reduces duplication with the same helper used by the
      Gen* view models.

**Sub-section C — Screen and helper migrations (the callers the issue
didn't name).**

- [ ] `discover_screen.dart:71, 82` — make `_loadUserLocation` /
      `_centerOnUserLocation` read `userLocationProvider` instead of
      the static service. The screen is already a `ConsumerState`, so
      it has `ref`. **Drop the local `Position? _userPosition` field**
      and read `ref.read(userLocationProvider).asData?.value` each
      time — the provider is session-cached, and the widget-level
      cache is a redundant second source of truth (see Finding 5).
      Note: reading a provider directly from a screen is a documented
      deviation from "Screens only interact with ViewModels" —
      accepted here because the same pattern is already in
      `home_screen.dart:67` and `feed_screen.dart:187` for
      infrastructure providers (see Finding 2).
- [ ] `map_helper.dart:129` (`enableUserLocation`) — change signature
      to accept `Position?` from the caller. Update callers (likely
      `discover_screen.dart` / a map-init path; verify with a local
      callers search) to read `userLocationProvider` and pass the
      position in. Inject `Position?` not `Ref` (see Finding 3 — keeps
      helpers pure utility functions).
- [ ] `discover_map_helper.dart:263` — same pattern: accept
      `Position?` as a parameter, update callers.
- [ ] `leaderboard_map_helper.dart:272` — same pattern: accept
      `Position?` as a parameter, update callers.

**Sub-section D — Delete the legacy method.** Safe only after A/B/C
are complete.

- [ ] Delete `MapboxLocationService.getCurrentLocation()`.
- [ ] Delete `MapboxLocationService._permissionRequest` (the
      deduplication guard is already present on
      `DeviceLocationService`; nothing else needs it).
- [ ] Remove the `geolocator` import from
      `mapbox_location_service.dart` if no remaining references — only
      used for `Position?` in `searchAddresses` / `searchGeographicAreas`
      signatures, so it likely stays. Double-check.

**Sub-section E — Tests.**

- [ ] Update `search_view_model_test.dart` — four test setups
      currently override `deviceLocationFetcherProvider`; switch to
      `userLocationProvider.overrideWith((_) async => <Position?>)`.
- [ ] Update `location_picker_view_model_test.dart` — any test that
      mocked `_locationRepository.getCurrentLocation()` needs a
      `userLocationProvider.overrideWith(...)` plus a mock on the new
      `reverseGeocode(...)` repo method. Regenerate Mockito mocks
      (`flutter pub run build_runner build`) for every
      `*_test.mocks.dart` that currently has a
      `LocationRepository.getCurrentLocation()` stub.
- [ ] Verify existing disposal tests on `SearchNotifier` and
      `LocationPickerNotifier` still pass (per
      `docs/client/architecture.md` §"Disposal Testing"). If either is
      missing a disposal test, add one following the pattern in that
      section.
- [ ] Add a simple test (or extend an existing one) that confirms
      `map_helper.enableUserLocation(map, position: null)` handles the
      null case the same way the old GPS-returns-null path did — no
      regressions on map init when GPS is denied.

**Sub-section F — Docs, grep, manual verification.**

- [ ] Add a short note to the `userLocationProvider` doc comment in
      `app/lib/services/providers.dart` (and/or to the top of
      `app/lib/services/user_position_resolver.dart`) stating that
      (a) this is the single entry point for the user's device
      position and all callers must use it, and (b) this is a
      documented exception to the repository-only-calls-services
      architectural rule because GPS is transient device state, not a
      cacheable server resource (see Finding 1).
- [ ] `grep -r "MapboxLocationService\.getCurrentLocation" app/lib
      app/test` returns zero hits.
- [ ] `npm run build` + `npm run test:app` clean.
- [ ] Manual verification on a physical device (not just simulator):
      cold-start the app, exercise search + discover (list + map) +
      leaderboard + location picker, and confirm only one
      `DeviceLocationService: Got device location` log line per
      session — barring explicit invalidations (logout, pull-to-
      refresh on feed).

**Validates:**
- `MapboxLocationService` contains only
  `searchAddresses`, `searchGeographicAreas`, `reverseGeocode` →
  acceptance criterion 2.
- One GPS fetch per cold-start session under normal use →
  acceptance criterion 1.
- Search view model tests use `userLocationProvider.overrideWith` —
  same override ergonomics (just a different provider) →
  acceptance criterion 3.

## Related Issues

- **#1143** — introduced `userLocationProvider`, `UserPositionResolver`,
  and `DeviceLocationService`. This issue is its follow-up cleanup.

## Architecture Compliance Audit

*(Added 2026-04-17 by the audit skill. Reviewed against
`docs/client/architecture.md` and `docs/client/caching.md`.)*

The plan is architecturally sound but has several layering questions
worth surfacing so reviewers understand the precedents being applied.
None of the findings below change the recommended approach; they
document the *why* behind each choice so future readers can evaluate
edge cases consistently.

### Layer boundaries

**Finding 1 — GPS is a documented exception to "Repositories are the
ONLY layer that calls Services."** The strict rule in
`docs/client/architecture.md` is that view models never access services
directly. `userLocationProvider` routes through
`UserPositionResolver`, which calls `DeviceLocationService` (a service)
**and** reads `userRepository` and `locationRepository` (repositories) to
compute its fallback ladder. There is no `UserLocationRepository` in
front of the service. This is the pattern #1143 established and the plan
preserves it. Rationale to document inline in the code: device-GPS state
is not a cacheable server resource — it's transient device state — so
wrapping it in a Stash-backed repository would be a category error. The
provider itself plays the "cache + invalidation owner" role that a
repository would otherwise play.

**Action:** The doc-note task in sub-section F should explicitly state
that GPS reads are an exception to the repository-only-calls-services
rule, and why — otherwise future contributors will either copy the
pattern for non-GPS concerns or try to "correct" it back into a
repository.

**Finding 2 — Screen reads provider directly (sub-section C,
discover_screen).** `docs/client/architecture.md` says "Screens only
interact with ViewModels." After this migration, `DiscoverScreen` will
read `userLocationProvider` directly in `_loadUserLocation` and
`_centerOnUserLocation`. `home_screen.dart:67` and
`feed_screen.dart:187` already do this, so the plan is consistent with
established precedent — but it *is* a deviation from strict layering.
The informal convention in this codebase appears to be: infrastructure
providers (device GPS, auth state) can be read by screens; data
providers (gear, loans, users) must go through a view model.

**Action:** Keep the plan as written, with the inline note in sub-
section C's task flagging the deviation as intentional and matching
#1143's precedent.

**Finding 3 — Helpers should remain pure (sub-section C map helpers).**
`docs/client/architecture.md` explicitly positions helpers as "pure
utility functions (e.g., `DiscoverMapHelper` for Mapbox lifecycle)."
Injecting `Ref` into `map_helper.dart` / `discover_map_helper.dart` /
`leaderboard_map_helper.dart` would turn them from pure utilities into
quasi-view-models. The plan's recommendation to inject `Position?` as
a parameter (not `Ref`) is the architecturally correct choice for this
reason, not just an ergonomic preference.

### Caching

**Finding 4 — New `LocationRepository.reverseGeocode(lat, lng)` should
not be cached.** Per `docs/client/caching.md`, all cache keys follow
`'namespace:identifier'`. The existing
`MapboxLocationService.reverseGeocode` is uncached (direct HTTP to
Mapbox). The plan's new repo wrapper inherits that behavior. Reverse
geocoding a specific lat/lng *could* be cached (the answer is stable),
but the input space is continuous and cache hits would be rare in
practice — skipping the cache matches the existing behavior and keeps
the method's semantics identical to the deleted
`LocationRepository.getCurrentLocation()` minus the GPS step.

**Finding 5 — Manual per-widget caching of position (discover_screen).**
The discover screen currently keeps a local `Position? _userPosition`
field to avoid re-fetching. `docs/client/caching.md` says "Never
implement manual cache maps in ViewModels" — and the underlying
principle applies here, since `userLocationProvider` is already
session-cached. The plan (sub-section C) drops the local field and
reads `ref.read(userLocationProvider).asData?.value` each time —
single source of truth.

### Async safety

**Finding 6 — New async gaps need `ref.mounted` guards.**
`search_view_model.dart` already uses the "resolve deps eagerly, check
`ref.mounted` after each await" pattern documented in
`docs/client/architecture.md` §"Async Safety in Notifiers". Sub-
section B's migration of `_performInitialSearch` replaces one async
gap with another of the same shape, so the existing guards cover it.
Sub-section B's new call in `_executeSearchWithLocation` adds one
`await` and sets state after — needs a guard immediately before the
`state =` assignment. `location_picker_view_model.dart` uses
`autoDispose` and already has `ref.mounted` checks everywhere; the
migration preserves them.

**Action:** Sub-section B's task for `_executeSearchWithLocation`
already calls out the `if (!ref.mounted) return;` requirement inline.

**Finding 7 — Disposal tests.** `docs/client/architecture.md` §
"Disposal Testing" mandates disposal tests on all async notifiers.
`SearchNotifier` and `LocationPickerNotifier` both have existing async
flows that the plan modifies. Adding an explicit task to confirm
disposal tests exist and still pass.

### Summary

No blocking findings. The plan aligns with the architecture docs and
with the precedents set by PR #1143 and by `home_screen.dart` /
`feed_screen.dart`. All audit findings are reflected inline in the
relevant sub-section tasks.

### Revision note (Option B switch)

Revised 2026-04-17 per reviewer direction: collapsed the earlier
three-phase plan (Option C) into a single-PR Option B migration.
Scope of the work is unchanged; the single-PR structure uses sub-
sections (A–F) as internal commit boundaries. Audit findings 1–7
carry over without change — they describe architectural properties
of the target state, not the phasing path to get there.

## Implementation status (2026-04-17)

All six sub-sections shipped on branch
`issue-1156-user-location-migration`, six commits:

| Sub-section | Commit | Notes |
| --- | --- | --- |
| A — repo refactor | `7cb7703b` | Added `reverseGeocodeViaMapbox` (renamed from plan's `reverseGeocode` to avoid collision with the existing server-backed `LocationRepository.reverseGeocode`) |
| B — view models | `78526c60` | Search + location picker migrated; `deviceLocationFetcherProvider` deleted. 'Throws' tests rewritten against the null-return contract since `DeviceLocationService`/`UserPositionResolver` never throw. |
| C — screens & helpers | `b18f6d05` | `discover_screen` reads the provider; map helpers take `Position?` from callers. `LocationMapPreview` promoted to `ConsumerStatefulWidget`. |
| D — deletion | `5ad92598` | `MapboxLocationService.getCurrentLocation` + `_permissionRequest` gone; `LocationRepository.getCurrentLocation` gone. Mockito mocks regenerated. |
| F — docs | `d6d742f6` | `userLocationProvider` doc comment expanded with the architectural-exception rationale. |
| Gap-fill | `890addb3` | Added `LocationPickerNotifier` disposal test (`SearchNotifier` already had coverage in two places) and `MapHelper.enableUserLocation(map, position: null)` null-case test. |

- `flutter analyze` clean.
- `npm run test:app` — 3460 tests, all pass.
- `npm run build` — clean (no Go changes in this PR, ran as sanity check).
- `grep -r "MapboxLocationService\.getCurrentLocation" app/lib app/test` — zero hits.

## Manual verification (2026-04-17)

Ran on physical Pixel 8 (Android 16) against the dev backend with
`npm run start:app`. Key observations from a filtered
`adb logcat -s flutter:I` capture:

- **Zero `MapboxLocationService: Got current location`** lines during
  the session — the deleted method stayed deleted. ✓
- **`UserPositionResolver: Resolved user position via GPS`** fires
  through the new provider path. ✓
- Two `DeviceLocationService: Got device location` lines in the full
  session — the first on cold-start (real GPS: 40.14, -105.16,
  Boulder), the second after a pull-to-refresh on the feed. The plan's
  acceptance criterion explicitly allows the second ("barring explicit
  invalidations (logout, pull-to-refresh on feed)"). ✓
- All three acceptance criteria hold.

Pre-existing issues surfaced during verification, filed as follow-ups:

- **#1174** — Location permission popup doesn't appear on first cold-
  start after a revoke (Android timing race between `initState` and
  Activity resume). Pre-dates this PR — the old
  `MapboxLocationService.getCurrentLocation` call path had the same
  race. Recommended fix: wrap the initState provider read in
  `WidgetsBinding.instance.addPostFrameCallback`.
- **#1175** — Initial GPS fetch consistently times out at 3s on cold-
  start, so the initial search fires with Austin fallback coords even
  though real GPS arrives ~3s later. The iOS-documented Mapbox-vs-
  Geolocator lock race reproduces on Android too. Pre-dates this PR —
  the 3s timeout comment in `search_view_model.dart:383-388` was added
  for exactly this reason. Recommended fix: stack
  `Geolocator.getLastKnownPosition()` + early-warm in splash + lower
  accuracy for the initial fetch.

Neither blocks this PR. Both are recommendations to address in
follow-up PRs.
