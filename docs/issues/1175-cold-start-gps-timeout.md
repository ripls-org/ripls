# Issue #1175: Initial GPS fetch consistently hits 3s timeout on cold-start; search fires with Austin fallback

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1175
**Filed:** 2026-04-17 | **Last activity:** 2026-04-21
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

On cold-start of the Flutter app on Android (reproduces on Pixel 8 emulator, Android 16), the very first `SearchViewModel` run on Discover fires its RPC before `userLocationProvider` can resolve a real GPS fix. Because the view model caps the location wait at a 3-second timeout (`search_view_model.dart:383`), it falls back to Austin coordinates and renders ~108 proximity-biased results. The real device position arrives ~1.4 seconds *after* the timeout — too late to influence the first-impression search.

After the two-comment thread and the merge of #1182, scope has narrowed considerably:

- **Fresh-install path is fixed.** Scenario A in the 2026-04-21 re-verification shows `getCurrentPositionIfGranted()` now returns `null` immediately (no timeout warning) and the Austin fallback is used as designed until the user makes a gesture that prompts for permission. The earlier hypothesis that FCM/Geolocator permission collision (#1174) was partly responsible no longer applies to fresh-install.
- **Returning-user path still reproduces.** Scenario B shows a clean 3s timeout → Austin fallback → GPS lands 1.4s late. With permission already granted, `Geolocator.getCurrentPosition(accuracy: high)` in `device_location_service.dart:87-91` takes ~4.5s from cold-start — consistent with Mapbox-vs-Geolocator lock contention (the iOS-documented race now confirmed on Android) rather than a permission artifact.

So the live bug is: **returning users on cold-start see the wrong city's results for their first search**, every session, reliably.

## Who Is Actually Asking For Location On Cold-Start

Tracing from code rather than from inference: **the cold-start GPS fetch is triggered by the Discover tab being eagerly constructed off-screen, not by Feed, and not by anything the user can see.**

The chain:

1. `home_screen.dart:454-477` builds all bottom-nav tabs inside an `IndexedStack`. `IndexedStack` mounts every child at construction time; the visible child is just the one at `state.selectedIndex`. Default tab is Feed (index 0) per `home_screen.dart:53`, but the Discover tab at index 2 is mounted simultaneously.
2. The Discover subtree contains widgets that `ref.watch(searchProvider)` (e.g. `discover_bottom_sheet.dart:111`, `discover_screen.dart:444`, `discover_screen.dart:482`). Any `ref.watch` on `searchProvider` causes Riverpod to build `SearchNotifier`.
3. `SearchNotifier.build()` (`search_view_model.dart:300-374`) calls `ref.read(userLocationProvider.future)` at line 357 and hands the future to `_performInitialSearch`. `userLocationProvider` is a `FutureProvider` whose first read triggers `UserPositionResolver.resolve()` → `DeviceLocationService.getCurrentPositionIfGranted()` → `Geolocator.getCurrentPosition(accuracy: high)`.
4. The 3s timeout at `search_view_model.dart:424` races against that GPS call. On cold-start, GPS loses; Austin fallback coordinates are used; an RPC fires with Austin coords; ~108 Austin-biased results are cached in `searchProvider`.
5. `SearchNotifier.build()` at `search_view_model.dart:306` calls `ref.keepAlive()`, so those results stick. `searchCacheInvalidationProvider` (see the listener at line 309) does not re-fire when `userLocationProvider` later resolves — GPS lands 1.4s too late, but there is no mechanism that invalidates the search cache on GPS resolution.
6. The user is still on Feed at this point and sees nothing. When they eventually tap over to Discover, the already-cached Austin-biased results render, even though the device GPS has long since resolved to their real location.

### Ruling out other candidates

- **Feed:** `feed_screen.dart` only references `userLocationProvider` at line 187, inside `_onRefresh()` (pull-to-refresh). It does not `ref.read` or `ref.watch` the provider on startup, and it does not pass location to the server on its RPC. PR #1180 (server-only change to feed sort order) cannot be involved. The user's hypothesis ("is the feed computing location and dropping it, or passing it to the server which drops it?") is ruled out by code inspection: the feed never touches location at startup.
- **FCM:** `FCMService: Permission status: AuthorizationStatus.authorized` in Scenario A is push-notification permission, not location.
- **HomeScreen `_onAddTap`:** `home_screen.dart:69` calls `requestLocationWithSettingsFallback`, but it's gated on the + button gesture — not firing during cold-start.
- **Mapbox `MapWidget` in Discover:** still present inside the Discover subtree (also eagerly mounted). Could still contribute to the `getCurrentPosition` slowdown via native-side lock contention, but is not itself producing any log line — the Mapbox-race framing remains inferential.

### Historical context — why is Discover eagerly built?

Searching closed issues for the Austin/discover lineage:

- **#444 "Location improvements"** (closed 2026-01-20) — established the `GPS → primary residence → Austin` fallback ladder. That's where "Austin, TX (30.2672, -97.7431)" came from.
- **#803 "The discover_screen still sometimes does not load"** (closed 2026-03-08) — the reason `IndexedStack` + `keepAlive()` + three redundant initial-search mechanisms exist. Its plan at `docs/issues/803-discover-initial-load.md:93-101` explicitly documents the pattern: DiscoverScreen is mounted immediately at startup, and `SearchNotifier` is kept alive, because the previous zero-state-empty-on-first-visit bug was caused by the initial search being dropped when its provider was disposed and re-created. The fix guaranteed the initial search fires at startup and its results survive tab switching.
- **#1177 "Discover/leaderboard map briefly renders at (0,0) over ocean"** (open) — related but different symptom (Mapbox default null-island camera), same root cause neighborhood.

So the eager-build of Discover is **not accidental** — it's load-bearing behavior from the fix to #803. A naive "just lazy-build Discover" proposal would regress #803. Any defer-initial-search variant needs to preserve #803's guarantee that the first time the user lands on Discover they see populated results, not a blank zero-state.

## Context from Comments

Two comments, both by the issue author (Brian, MEMBER):

- **2026-04-18 comment — false-lead hypothesis, then superseded.** First suggested the 3s timeout might be a downstream effect of the FCM/Geolocator permission collision in #1174. That was plausible on fresh-install but didn't explain the original trace showing GPS successfully landing 3.7s late — a permission-denied path would produce no `Got device location` log at all.

- **2026-04-21 comment — definitive re-verification after #1182 merged.** Clean split between Scenario A (fresh-install, fixed) and Scenario B (returning user with permission granted, still broken). Explicitly states: "The '3s warning in Scenario B is pure Mapbox-vs-Geolocator lock contention, not a permission artifact." Narrows the recommended fix to **option #1** (`getLastKnownPosition()` first) with **option #3** (lower accuracy) as a complement. Options #2 (eager warm), #4 (raise timeout), and #5 (retry) are either deferred or rejected.

The author is also the assignee and has committed to implementing option #1 as the next step. That is load-bearing direction, not a drive-by suggestion.

## Potential Solutions

The issue body already ranks five fix options. Post-comments, the short list is:

### Option A: `getLastKnownPosition()` first, then fall through to `getCurrentPosition()`

- **Approach:** In `DeviceLocationService.getCurrentPositionIfGranted()` (`device_location_service.dart:40-43`), before calling `Geolocator.getCurrentPosition()`, try `Geolocator.getLastKnownPosition()`. If the OS returns a cached fix, return it immediately. Otherwise, fall through to the current high-accuracy path. Applies inside `_fetchPosition` after the permission checks at `device_location_service.dart:82-85` succeed.
- **Pros:** Returning users (the scoped-down repro) get sub-100ms resolution of the OS-cached fix — no lock contention, because we never call `getCurrentPosition()` in the hot path. Standalone change, ~10 lines, no architectural ripple. Fresh-install behavior is unaffected because the OS has no last-known fix to return.
- **Cons:** Returned fix may be stale (minutes to hours old). For proximity-biased search that's fine (5–100m error is already the target accuracy), but a user who walks across town between sessions would get yesterday's neighborhood. Minor. `getLastKnownPosition()` also returns `null` if the OS has no cached fix — still covers the main repro because a returning user who granted permission recently will almost always have one.
- **Effort:** Small. One method change, one new unit test case.

### Option B: Lower accuracy on the initial fetch to `LocationAccuracy.medium`

- **Approach:** Either change the default in `_fetchPosition` to `medium`, or add an `accuracy` parameter so `getCurrentPositionIfGranted()` uses `medium` while `requestAndGetCurrentPosition()` (gesture path) keeps `high`. `LocationAccuracy.medium` settles faster because it can return a WiFi/cell fix without waiting for GPS.
- **Pros:** Reduces the worst-case hot-path latency even when `getLastKnownPosition()` returns null (fresh install + brand new device). Plenty accurate for sort-by-nearest.
- **Cons:** Behavior change for any other caller that reads `userLocationProvider` and expects high accuracy. Needs inspection of the 19 call sites (most are proximity-bias, which don't care, but `requestAndGetCurrentPosition` is invoked from address-save flows where precision matters). Risk is low but not zero.
- **Effort:** Small.

### Option C: Stacked Option A + Option B

- **Approach:** Ship A alone first. Measure with a follow-up repro whether B is still needed. If A alone closes the bug (expected, since the returning-user repro has a cached fix available), defer B.
- **Pros:** Matches the 2026-04-21 comment's direction ("A is the fix; B is optional once A lands"). Smallest shippable increment goes first.
- **Cons:** Two PRs instead of one if B turns out to be needed. Accepting this cost for the ability to measure A in isolation.
- **Effort:** Small + small.

### Option D: Defer the Discover initial search until Discover is visible

- **Approach:** Attack the root cause exposed by the code trace above. `SearchNotifier.build()` currently fires `_performInitialSearch` the moment a community is selected, regardless of whether the user has ever looked at the Discover tab. Change the trigger: watch `homeProvider.selectedIndex` (or a smaller derived `isDiscoverVisible` provider) and only fire the initial search once Discover first becomes visible. Keep `ref.keepAlive()` so results persist after the user switches away. An equivalent formulation is to observe tab visibility from DiscoverScreen itself and call a `primeInitialSearch()` method on the notifier.
- **Pros:** By the time the user taps over to Discover (any realistic time after cold-start), `userLocationProvider` has long since resolved. The race goes away entirely — no Austin fallback, no stale cache, no need for `getLastKnownPosition()` or lower accuracy. Also eliminates wasted work on cold-starts where the user never visits Discover (most sessions, probably).
- **Cons:** Has to preserve #803's zero-state guarantee: the first view of Discover must show populated results, not blank. If the initial search fires only *on first visible frame* of Discover, there's a latency window (the search RPC takes 35-140ms in the traces) where the user sees a loading state. That's probably fine — it's a brief skeleton, not a blank — but needs verification. Also needs care around the three-mechanism redundancy documented in `docs/issues/803-discover-initial-load.md:84-137`; changing the primary trigger interacts with the fallback `_triggerSearch()` paths in `discover_screen.dart`.
- **Effort:** Medium. Not a one-liner — needs a new tab-visibility signal, touches `SearchNotifier.build()`, and requires re-verifying the #803 zero-state test (`app/test/presentation/viewmodels/search_initial_load_test.dart`) still passes.

### Option E: Caller-intent API — `LocationIntent.proximityBias` vs `LocationIntent.precisePin`

- **Approach:** Introduce an explicit `LocationIntent` enum that every caller of `userLocationProvider` / `DeviceLocationService` must supply. The service branches on intent:

  ```dart
  enum LocationIntent {
    /// Proximity bias — the user does NOT see their exact position in the UI;
    /// the fix is used to rank/sort/bias results. Neighborhood accuracy is
    /// fine. Callers: Discover search, autocomplete proximity, request/
    /// experience creation suggestion bias.
    proximityBias,

    /// Precise pin — the user WILL see their exact position in the UI (blue
    /// dot on map, address-level precision). Stale or coarse is wrong here.
    /// Callers: Discover map center/pin, leaderboard map pin, location
    /// picker, "use current location" save.
    precisePin,
  }
  ```

  - `proximityBias`: Try `getLastKnownPosition()` first; if non-null AND `position.timestamp` is within a bounded freshness window (e.g. ≤ 5 min), return it. Otherwise fall through to `getCurrentPosition(accuracy: medium)`.
  - `precisePin`: Always `getCurrentPosition(accuracy: high)`. No last-known shortcut. No stale-tolerance.

  The provider becomes a Riverpod family, so each intent gets its own session cache entry:

  ```dart
  final userLocationProvider = FutureProvider.family<Position?, LocationIntent>(
    (ref, intent) => UserPositionResolver.resolve(ref, intent),
  );
  ```

  Callers update from `ref.read(userLocationProvider.future)` to
  `ref.read(userLocationProvider(LocationIntent.proximityBias).future)` etc.
- **Pros:**
  - Semantically honest: intent is always explicit at call sites, so reviewers can audit "does this path actually need precision?" instead of guessing.
  - The cold-start race is resolved asymmetrically, matching each caller's actual needs: proximity callers get sub-100ms (from last-known) and a usable result even if GPS is slow; precise callers get the correct fresh fix without any stale shortcut compromising the user-visible pin.
  - Each intent gets its own session cache, so invalidation is targeted. `ref.invalidate(userLocationProvider(LocationIntent.precisePin))` after a gesture doesn't force the proximity cache to re-fetch.
  - Obviates most of Option D's urgency: even with Discover eagerly built, the proximity-bias initial search returns instantly (fast path) and the precise-pin map renders when GPS lands (as progressive enhancement).
- **Cons:**
  - Larger blast radius. All 19 `userLocationProvider` call sites need a decision and a migration. Splitting the provider changes the documented "single entry point" contract at `providers.dart:192-198`.
  - More surface area to test: staleness window behavior, the medium-vs-high accuracy split, family-provider invalidation semantics.
  - Requires discipline: future contributors must pick an intent correctly. Worth a lint hint or doc comment on the provider to nudge.
- **Effort:** Medium. The service and provider changes are mechanical; the audit of 19 call sites is where the time goes. Each site needs a one-line decision (proximity vs precise) plus a 1-line code change. A few are ambiguous — `home_screen.dart`'s invalidation, or the `location_autocomplete_field` proximity bias when combined with an already-picked location — and need a judgment call.

## Recommendation

**Option E (caller-intent API).** Options A–D were the stepwise progression of thinking; Option E is the synthesis. It subsumes A (last-known), B (lower accuracy), and C (stacked) into the `proximityBias` branch, and removes most of the urgency behind D (defer) because the proximity callers stop waiting on GPS entirely.

The decisive argument is that the underlying bug isn't really "GPS is slow at cold-start" — it's "we've been treating `userLocationProvider` as a single thing when callers actually want two different things." Discover's proximity-bias search wants a fast, coarse, maybe-stale answer; Discover's map pin wants a precise, fresh one. Forcing both through one high-accuracy, no-last-known, session-cached provider means either path compromises the other: optimize for speed and the map pin becomes stale; optimize for precision and the search races a 3s timeout. Splitting the intent resolves both without tradeoffs.

Practical sequencing: ship E in one or two PRs (service/provider change + call-site migration), not as a multi-quarter effort — the call-site audit is mechanical. Option D becomes a nice-to-have rather than a must-do.

**If a smaller fix is preferred**, the fallback is Option A with the A2 refinement (staleness-gated last-known, ≤ 5 min) — one method change, no call-site migration. That closes the reported symptom but leaves the precision/proximity ambiguity unresolved, so the next similar bug is still latent.

Open risk to watch during validation of E: if a user's OS last-known fix is `null` or older than the staleness window on cold-start (rare — right after a device restart, or after long inactivity), the proximity path still falls through to `getCurrentPosition(medium)`. The 3s timeout in `search_view_model.dart:383` remains as a safety net for that case. Medium accuracy should make even the slow path land inside 3s reliably, but this is worth measuring.

## Open Questions

- **What staleness window should `proximityBias` accept for last-known fixes?** — Recommendation: 5 minutes. Long enough to cover the vast majority of returning-user cold-starts (app backgrounded, user returning within a short gap), short enough that it won't return yesterday's city. Make it a named constant in `DeviceLocationService` so future tuning is obvious: `static const _proximityLastKnownMaxAge = Duration(minutes: 5);`. Revisit if field data shows users regularly return after > 5 min and the fall-through is hitting the slow path too often.

- **Which medium accuracy do we choose for `proximityBias` fall-through?** — Recommendation: `LocationAccuracy.medium` (~100m). `.low` (~500m) is tempting for speed but would produce visible mis-ranking of results within the same neighborhood, which is the exact UX problem this plan is trying to fix. Medium settles fast (WiFi/cell fix, no GPS satellite wait) and is well within the sort-by-nearest tolerance.

- **Family provider vs two named providers?** — Recommendation: family (`FutureProvider.family<Position?, LocationIntent>`). Idiomatic Riverpod, scales if a third intent is ever added, and keeps invalidation targeted. Two named providers (`proximityPositionProvider` + `precisePositionProvider`) would also work and produces slightly shorter call sites, but the family pattern makes the intent a first-class parameter and keeps the architectural note at `providers.dart:192-198` coherent (one owner, multiple cached slots).

- **Should the proximity cache be served by a recent precise fix if one exists?** — Recommendation: no, at least not in the first pass. A precise session fix is strictly "better" than last-known for proximity purposes, and it would be technically correct to prefer it — but the cross-intent dependency complicates invalidation. Keep the two caches independent. Revisit if tests show wasteful double-fetches in real usage.

- **What happens on cold-start if something on Discover requests `precisePin` via `ref.watch` but the user hasn't granted precision permission?** — Recommendation: same behavior as today — returns null, the pin simply doesn't render. `precisePin` must not trigger a system permission prompt from a passive watch; prompting stays gated on a gesture (`requestAndGetCurrentPosition`). Document this clearly in the enum doc comment so future contributors don't wire up a surprise prompt.

- **Caller audit: how do we ensure every existing call site picks an intent?** — Recommendation: make the migration mechanical by deleting the old `userLocationProvider` signature in the same PR. Every call site then fails to compile until it picks an intent; the compiler is the checklist. Avoid a flag-day soft migration — any temporary default would almost certainly ossify.

- **Where exactly should the branching live — inside `DeviceLocationService` or one level up in `UserPositionResolver`?** — Recommendation: inside `DeviceLocationService`. Geolocator-specific knowledge (that the OS caches fixes, that accuracy is a knob on `getCurrentPosition`) belongs in the device-level service, consistent with the existing file-comment split ("device GPS" service vs. "ladder resolver"). `UserPositionResolver` threads `LocationIntent` through unchanged to the primary-residence fallback (which ignores intent — it's always a coarse synthetic position already).

- **Platform differences in `Geolocator.getLastKnownPosition()`?** — Recommendation: treat iOS as less reliable than Android and confirm via a manual iOS repro during implementation. Android's Fused Location Provider consistently returns a non-null last-known fix when permission is granted; iOS's Core Location is more conservative (may return null if the app has been backgrounded for a while, even with permission). The code handles null by falling through to `getCurrentPosition(medium)`, so behavior is safe — but the win on iOS may be smaller, which is worth knowing before claiming the race is fully closed on iOS.

- **Unit-test strategy?** — Recommendation: focused tests around the new intent-branching logic (staleness comparison, accuracy selection, fall-through on expired last-known, family-provider cache isolation). If `DeviceLocationService` doesn't already have a seam to substitute the Geolocator static calls, add one — the new branching makes this worth a small, localized refactor rather than continuing to rely on platform-channel round-trips in tests.

## Implementation Plan

Three phases delivering Option E. Phase 1 introduces the new types and the branching logic without touching any call site. Phase 2 migrates the call sites. Phase 3 is optional cleanup.

### Phase 1: Introduce `LocationIntent` and branch inside `DeviceLocationService`

**Goal:** Land the new API surface and its implementation without any call-site changes yet. The new provider exists in parallel with the old one so the app keeps working.

**Tasks:**
- [ ] Add `LocationIntent` enum (`proximityBias`, `precisePin`) to `app/lib/services/device_location_service.dart` (or a new `location_intent.dart` if it's cleaner). Write doc comments that explicitly describe each intent's accuracy/staleness/permission-prompt semantics, including the rule that `precisePin` must never cause a prompt from a passive watch.
- [ ] Thread `LocationIntent intent` through `DeviceLocationService.getCurrentPositionIfGranted({required LocationIntent intent})` and `DeviceLocationService.requestAndGetCurrentPosition({LocationIntent intent = LocationIntent.precisePin})`. Update `_fetchPosition` to accept the intent.
- [ ] Implement the branching in `_fetchPosition`:
  - `proximityBias`: call `Geolocator.getLastKnownPosition()`. If non-null AND `position.timestamp` is within `_proximityLastKnownMaxAge` (new constant, 5 minutes), return it. Else call `Geolocator.getCurrentPosition(accuracy: medium)`.
  - `precisePin`: call `Geolocator.getCurrentPosition(accuracy: high)`. No last-known shortcut.
- [ ] Add structured logging: use the existing `_log.info(...)` sink and include the intent + whether the fix came from last-known vs fresh. Keep the "Got device location: <lat>, <lng>" prefix so log consumers don't break; append intent/source as structured fields.
- [ ] Update `UserPositionResolver.resolve(Ref ref)` to `UserPositionResolver.resolve(Ref ref, LocationIntent intent)`. Thread the intent through to `DeviceLocationService`. The primary-residence fallback is intent-independent (already coarse) — leave it unchanged.
- [ ] Convert `userLocationProvider` in `app/lib/services/providers.dart` from `FutureProvider<Position?>` to `FutureProvider.family<Position?, LocationIntent>`. Update the architectural note at `providers.dart:192-198` to document the per-intent cache semantics.
- [ ] Unit tests for `DeviceLocationService`: staleness-gated last-known (within window vs expired), accuracy selection per intent, fall-through on null last-known, family-provider cache isolation (two intents → two independent cached values).
- [ ] Do NOT migrate call sites in this phase. Temporarily export a deprecated shim (`@Deprecated('Use userLocationProvider(LocationIntent.…).future')` wrapper) only if unavoidable; otherwise Phase 1 becomes Phase 1+2.

**Validates:** That the new API behaves correctly at the service/provider level in isolation, with test coverage before we commit every call site to it.

### Phase 2: Migrate all call sites

**Goal:** Pick an intent for every existing reader/writer of `userLocationProvider`. Delete the old signature.

**Tasks:**
- [ ] Enumerate the 19 call sites (`grep userLocationProvider app/lib`). For each, pick an intent based on the rule: *does the user see this position in the UI as a pin/dot/address?* Yes → `precisePin`. No (ranking/biasing/filtering) → `proximityBias`.
  - Likely `proximityBias`: `search_view_model.dart`, `location_autocomplete_field.dart`, `gen_request_view_model.dart`, `request_creation_view_model.dart`, `experience_creation_view_model.dart`, any leaderboard ranking logic.
  - Likely `precisePin`: `discover_screen.dart` (map center + pin), `leaderboard_map_helper.dart` (pin), `location_picker_view_model.dart` / `location_modal_widgets.dart` (picker map + "use current" save), `map_helper.dart`, `location_permission_helper.dart`.
  - Mutation/invalidation sites (`feed_screen.dart:187`, `home_screen.dart:69`, permission-grant success in `location_permission_helper.dart`): invalidate both intents (`ref.invalidate(userLocationProvider(LocationIntent.proximityBias))` + `…(precisePin)`). Users who explicitly refresh want fresh for both.
- [ ] Delete the old `userLocationProvider` signature in the same PR. Rely on the compiler to flag missed sites.
- [ ] Update tests in `app/test/services/user_location_provider_test.dart`, `app/test/presentation/viewmodels/search_view_model_test.dart`, `app/test/presentation/viewmodels/search_initial_load_test.dart` to pass intents explicitly. Verify #803's zero-state guarantee (initial search still fires at startup) is preserved.
- [ ] Manually repro Scenario B from issue #1175 on Pixel 8 emulator: uninstall → cold-start → grant via center-on-me → force-stop → cold-start from launcher. Confirm no 3s timeout warning, and initial search fires with last-known coords (`proximityBias` path).
- [ ] Also verify on iOS simulator. `getLastKnownPosition()` may return null on iOS; expected outcome is either fast path or fall-through to `medium` accuracy (still should land well inside the 3s timeout). Capture behavior for the PR description.
- [ ] Verify on Discover that the map pin (`precisePin`) renders correctly after GPS lands — progressive enhancement, not racing against a timeout. It may briefly show no pin on cold-start, which is acceptable (better than a pin at Austin).
- [ ] Run `npm run test:app` to verify no regressions. Run `flutter analyze` to catch missed call sites.

**Validates:** That intent assignment is correct across the app, that the #1175 symptom is closed, and that #803's zero-state guarantee is preserved.

### Phase 3 (optional cleanup): Update the `_locationTimeout` doc comment

**Goal:** After Phases 1+2 land, the 3s timeout in `search_view_model.dart:377-383` is a safety net, not the primary mitigation. The doc comment still frames the race as iOS-Mapbox-specific; update it to say the proximity-bias path now handles this through staleness-gated last-known and medium accuracy, and the timeout only fires in unusual cases (device just rebooted + permission just granted). Small change, not critical, ship opportunistically.

## Related Issues

- **#1156** — `userLocationProvider` consolidation that surfaced the Android manifestation of this race during manual verification.
- **#1174** — FCM/Geolocator permission collision; previously implicated as a cause of the 3s timeout on fresh-install. Now ruled out for returning-user cold-start; #1182 closed this on the fresh-install side.
- **#1177** — Related downstream consequence (map center-on-me no-op when `userLocationProvider` never resolves). Separate bug; not blocked by this fix.
- **#1143** — Original `UserPositionResolver` / `DeviceLocationService` introduction.
- **#1182** — Merged fix for #1174; referenced in the 2026-04-21 re-verification.
- **#803** — "The discover_screen still sometimes does not load" (closed 2026-03-08). The fix introduced the eager-build + `keepAlive` + three-mechanism redundancy that makes Discover fire its initial search at cold-start regardless of tab visibility. Option D must preserve its zero-state guarantee. See `docs/issues/803-discover-initial-load.md`.
- **#444** — "Location improvements" (closed 2026-01-20). Established the `GPS → primary residence → Austin` fallback ladder. Source of the Austin coordinates that appear in the symptom.

## Audit Findings (2026-04-21)

Audited against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, and `docs/server/architecture.md`. Summary:

- **Repository/Service boundary (architecture §Key Rules 1–2):** Compliant. The change lives inside `DeviceLocationService` and does not cross the ViewModel→Service boundary. The existing architectural exemption for `userLocationProvider` (documented at `providers.dart:208-215`: GPS is transient device state, not a Stash-cacheable resource) already covers this pattern — the plan explicitly cites and respects it rather than proposing a new repository layer for GPS.
- **Async-first (architecture §Core Principle 1):** Compliant. `getLastKnownPosition()` is awaited like every other call in `_fetchPosition`; no sync cache access is introduced.
- **No manual cache maps (caching §Key Rule 1):** Compliant. Session caching continues to happen in the `userLocationProvider` `FutureProvider` as today. No ViewModel-level cache is added.
- **Cache invalidation semantics (caching):** Unchanged. `ref.invalidate(userLocationProvider)` continues to force a re-fetch through the same ladder; a stale-but-good last-known fix becomes the new session value, which is the intended behavior.
- **i18n:** Not applicable — no user-visible strings are added.
- **Server architecture:** Not applicable — pure client change.

Changes applied during audit:

1. **Added iOS verification task to Phase 1.** The root-cause analysis rests on Android reproduction, but the underlying race was originally documented on iOS (see `search_view_model.dart:383`). A Phase 1 that only verifies Android would leave iOS as a blind spot.
2. **Added an Open Question on platform differences in `Geolocator.getLastKnownPosition()`.** iOS Core Location is known to return `null` more often than Android's Fused Location Provider, which affects the expected win size on iOS. Noting this keeps the PR description honest if the iOS result differs from Android.
3. **Tightened Phase 1 logging task.** Called out that the new info log must use the existing `_log.info(...)` sink with a format consistent with the current "Got device location" message, so structured-logging consumers (if any) don't break.

No architectural violations were found; the changes above are reinforcements, not corrections.
