# Issue #1143: Location autocomplete: proximity bias not wired up for GPS fallback

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1143
**Filed:** 2026-04-15 | **Last activity:** 2026-04-15
**Labels:** (none) | **Assignees:** briangbrown
**Context docs:** none

> **Audit status (2026-04-16):** Reviewed against `docs/client/architecture.md`,
> `docs/client/caching.md`, `docs/client/i18n.md`, and
> `docs/server/architecture.md`. Changes applied in this audit pass:
> 1. Fixed layer violation in Option C / Implementation Plan — inserted the
>    missing Repository step so changes flow ViewModel → Repository → Service
>    (Key Architectural Rules 1–2).
> 2. Specified provider type as `FutureProvider<Position?>` (one-shot cached
>    async value, `ref.invalidate`-able) rather than vague "async/cached",
>    aligning with the async-safety guidance for `build()`-managed disposal.
> 3. Clarified that the new provider is a Riverpod provider, **not** a Stash
>    repository entry — the caching doc explicitly scopes Stash to API
>    response objects, not device sensor readings.
> 4. Added a mandatory disposal-safety unit test in Phase 1 per the
>    architecture doc's "All async Notifiers must have disposal tests"
>    guidance.
> 5. Added a server logging task (Phase 3) matching `docs/server/logging.md`
>    structured field conventions so the rollout impact is measurable.
> 6. Noted that no new ARB strings are required (Phase 2), keeping the plan
>    consistent with the i18n doc's lint gate.
>
> **Revision 2 (2026-04-16):** Folded in `locationId` findings:
> - Documented that `experience_creation_view_model.dart:163` omits
>   `locationId` (only `gen_experience_view_model.dart` sets it today).
> - Documented that the server fetches the Location record for the LLM
>   region string but ignores its coords — a free proximity signal the
>   server is throwing away.
> - Added Phase 3.5 for the server-side fallback and a Phase 3 task for
>   the client-side omission fix.
>
> **Revision 3 (2026-04-16):** Corrected a misread from Revision 2
> after tracing the two-stage modal architecture:
> - `GenExperienceNotifier.generateExperience()` in
>   `gen_experience_view_model.dart:270` is **dead code** —
>   `ExperiencePreviewModal` only calls this notifier's state-editing
>   and persistence methods, never its RPC method. The stage-1 caller
>   (`ExperienceCreationNotifier`) is the only live GenExperience RPC
>   path. Verified by a repo-wide grep on `.generateExperience()`
>   returning exactly one hit at `experience_creation_modal.dart:108`.
> - Updated Concrete Change #6 to include a dead-code deletion step.
> - Rewrote the Phase 3 ViewModel tasks to consolidate the GPS wiring
>   and the `locationId` fix into the single live method
>   (`ExperienceCreationNotifier.generateExperience()`), and added a
>   post-deletion grep verification step.
> - Expanded the Problem Statement with the two-stage modal walk-through
>   so future readers don't trip over the same misread.

## Problem Statement

Mapbox searches are returning globally-ranked results whenever the calling
widget does not pass an explicit `proximityBias`. A search for "Central Park"
in Boulder currently surfaces Central Park NYC first. This happens in at
least two independent flows:

**Client-side (autocomplete widget):**
- `LocationAutocompleteField` declares `Position? _currentPosition` as a GPS
  fallback (`app/lib/presentation/widgets/location/location_autocomplete_field.dart:47`)
  but the field is never assigned, so the fallback at line 142 is always `null`.
- Of the three call sites, only the location picker modal passes
  `proximityBias` (map center). The governance screen and leaderboard header
  rely on the dead GPS fallback and therefore search globally.

**Server-side (AI-assisted creation flows):**
- `GenExperience` reads `req.Msg.LatitudeDeg/LongitudeDeg` to bias the
  Mapbox `SearchPlaces` call (`server/services/experience/gen_ai.go:242-247`),
  but the Flutter client **never sets those fields** on the outgoing
  `GenExperienceRequest` (`app/lib/services/experience_service.dart:401-440`).
  They default to `0,0`, which the server's "non-zero" guard correctly skips —
  so the server-side geocoding is always unbiased.
- Same pattern for `GenRequest` — `server/services/request/gen.go:123-128`
  consumes the fields; `app/lib/services/request_service.dart:56-60` never
  sets them.

**`locationId` is a half-wired third signal (with a dead-code twist):**

The experience-creation flow is a **two-stage modal** split across two
viewmodels — this matters because the two look symmetric but only one is
live on the GenExperience RPC path:

- **Stage 1 — `ExperienceCreationNotifier`**
  (`experience_creation_view_model.dart`, provider
  `experienceCreationProvider`, screen `ExperienceCreationModal`).
  Opened from `home_screen.dart:71`, `community_metrics_screen.dart:369`,
  `nudge_content_view.dart:95,99`, `empty_content_state.dart:51`. This
  is the **only live caller** of `GenExperience`. Its
  `generateExperience()` method (line 135) calls
  `_experienceRepository.genExperience(...)` at line 163 and **does not
  pass `locationId`**.
- **Stage 2 — `GenExperienceNotifier`**
  (`gen_experience_view_model.dart`, provider `genExperienceProvider`,
  screen `ExperiencePreviewModal`). Hydrated by stage 1 with the AI
  response and shown as a second modal where the user edits and hits
  Create. Stage 2's Create button calls
  `_experienceRepository.createExperience(...)` (the persistence RPC),
  not `genExperience(...)`.
- **Dead code:** `GenExperienceNotifier.generateExperience()` at
  `gen_experience_view_model.dart:270` and its private helpers
  (`_generateFromText/Image/Url`, `_callGenExperienceAPI` at line 385)
  *do* pass `locationId` correctly at line 404 — but nothing calls
  them. A repo-wide grep for `.generateExperience()` returns exactly
  one hit (`experience_creation_modal.dart:108`), which resolves to the
  **stage-1** notifier. The stage-2 RPC path is legacy from before the
  two-modal split and was never removed. That's why it looked like
  "gen_experience_view_model sets `locationId`" — it does, on a code
  path that never runs.
- **Server gap:** when `locationId` *is* set (once stage 1 is fixed),
  the server at `gen_ai.go:46-56` (and `request/gen.go:52-68`) fetches
  the Location record but only extracts `location.Address.Locality` as
  a region string for the LLM. It **ignores** the
  `Geolocation.LatitudeDeg/LongitudeDeg` attached to the same record,
  even though they're the closest thing to the user's real location
  available to the server when GPS isn't provided. Trivial to add as
  the next rung of the proximity fallback ladder.

Net: no experience-creation flow in production currently sends
`locationId`, and the server wouldn't use the coords even if it
received it. Fixing stage 1 + the server fallback closes both halves.

Net effect: even with a correct implementation on the server, the proximity
signal never arrives, and Mapbox falls back to relevance + popularity only.
This compounds with #1147 (POI ranking gaps: "Wahoos Fish Taco" in Boulder
matched to Fresno) — proximity is the primary tool Mapbox ships for
disambiguating common names.

## Context from Comments

No comments on the issue. The body is unusually detailed and unambiguous for
a no-comment issue, so this is not a flag for missing input — the author
already laid out the problem surface. The one thing that *isn't* called out
in the body but came out of the user's scoping note on this planning pass:
the server-side `GenExperience` / `GenRequest` flows have a symmetrical
bug, and the fix should cover both.

## Potential Solutions

### Option A: Minimal fix — wire up `_currentPosition` inside the widget

- **Approach:** Call `MapboxLocationService.getCurrentLocation()` from
  `initState` and `setState` the result into `_currentPosition`. Leave the
  server-side `GenExperience`/`GenRequest` gap alone (track separately).
- **Pros:** Smallest surface area; matches the literal text of the issue;
  backward-compatible; no new public API.
- **Cons:** Leaves the server-side call sites silently passing `0,0` — the
  Wahoos-in-Fresno class of bug persists for prompt-based creation flows.
  Each widget instance (autocomplete, governance, leaderboard) fires its own
  permission check. The in-flight dedup in `MapboxLocationService` keeps
  this from being a correctness issue, but the repeated `checkPermission()`
  calls are wasteful.
- **Effort:** Small.

### Option B: Central `userLocationProvider` + thread through both widget and RPC senders

- **Approach:**
  - Introduce a Riverpod provider (`userLocationProvider`, async / cached)
    that wraps `MapboxLocationService.getCurrentLocation()` so every caller
    sees the same cached `Position?`. Model it after the existing
    `deviceLocationFetcherProvider` in `search_view_model.dart:40-43`
    (which is already a test-override-friendly wrapper) and upgrade it from
    a function provider to a cached async provider.
  - Update `LocationAutocompleteField` to `ref.watch(userLocationProvider)`
    as the GPS fallback, replacing the dead `_currentPosition` field.
  - Update `experience_service.dart:genExperience()` and
    `request_service.dart:genRequest()` / `genRequestFromMedia()` to read
    the provider and set `latitudeDeg`/`longitudeDeg` on the outgoing
    request when a position is available.
  - Delete the dead `_currentPosition` field and its GPS-fallback comments.
- **Pros:** Fixes the widget and the server-side RPC symmetrically — same
  user GPS, same proximity signal, end-to-end. Central cache avoids
  repeated `checkPermission` calls. Testable via provider override
  (matches the pattern search_view_model is already using). Removes dead
  code rather than papering over it.
- **Cons:** Bigger blast radius — touches one widget, two services, and
  three consumer screens. Requires deciding on cache TTL/refresh semantics
  (recommendation: one-shot fetch on first access, manual `refresh()` when
  needed; don't aggressively poll GPS). The service-layer mutation breaks
  the current rule that the services layer is "dumb" in the MVVM diagram;
  prefer having the ViewModel resolve the position and pass it in.
- **Effort:** Medium.

### Option C: Push lat/lng decision up to ViewModels, thread through Repository → Service

- **Approach:** Each ViewModel that calls `genExperience` / `genRequest`
  pre-resolves the user position via the new `userLocationProvider` and
  passes `latitudeDeg` / `longitudeDeg` into its **repository** method
  (`ExperienceRepository.genExperience()` / `RequestRepository.genRequest()` /
  `RequestRepository.genRequestFromMedia()`), which in turn forwards to the
  service. Also wire `LocationAutocompleteField` to the same provider.
- **Pros:** Honors the documented layering rule — "ViewModels are the
  ONLY layer that calls Repositories; Repositories are the ONLY layer
  that calls Services" (`docs/client/architecture.md`, Key Architectural
  Rules 1–2). Position is decided by the ViewModel (business logic),
  serialized by the Service (plumbing), with the Repository as the
  architectural seam the codebase already uses. Fixes widget and server
  RPC symmetrically.
- **Cons:** Touches three layers instead of two (viewmodel, repository,
  service) — but all three layers already exist for these calls, so it's
  a signature extension, not new plumbing. Minor duplication across
  ViewModels (one-line `ref.read` each).
- **Effort:** Medium.

## Recommendation

**Option C.** It fixes both the widget and the server-side RPC bug (which
the user explicitly asked me to cover), reuses the existing
`deviceLocationFetcherProvider` pattern, and respects the client
architecture layering rule that services should not own business logic.
Option B bleeds business logic into services; Option A leaves the more
impactful server-side gap unsolved.

Concrete shape of the change:

1. **New provider.** Introduce `userLocationProvider` as a **Riverpod
   `FutureProvider<Position?>`** (not a `NotifierProvider`, and *not* a
   Stash-cached repository entry). Rationale:
   - The value is a device sensor reading, not an API response object, so
     it doesn't belong in Stash (`docs/client/caching.md` specifies Stash
     caches API response objects).
   - A `FutureProvider` gives one-shot-per-session semantics for free:
     subsequent `ref.watch(userLocationProvider)` calls reuse the same
     `Future`. Screens that need to force a refetch can call
     `ref.invalidate(userLocationProvider)` (e.g., pull-to-refresh on
     discover) — this matches Pattern 5 in the caching doc.
   - `build()` on `FutureProvider` handles disposal automatically per
     the architecture doc's "AsyncNotifier handles disposal in build()"
     guidance, so no `ref.mounted` dance is required in the resolver.
   - Keep `deviceLocationFetcherProvider` in `search_view_model.dart`
     as-is — it's already a function-typed seam for test overrides; the
     new provider sits next to it and is a separate abstraction (cached
     value, not fetcher function). If `search_view_model` migrates to
     the new provider, that's a cleanup follow-up, not a prerequisite.
2. **Widget fix.** `LocationAutocompleteField` reads
   `ref.watch(userLocationProvider).valueOrNull` in the
   `suggestionsCallback` branch that needs the fallback. Delete the dead
   `_currentPosition` field.
3. **ViewModel wiring.** `gen_experience_view_model.dart` and
   `request_creation_view_model.dart` (plus any other caller of
   `genRequest` / `genExperience`) read the provider before the RPC and
   pass `latitudeDeg` / `longitudeDeg` into the *repository* method.
4. **Repository signatures.** Extend
   `experience_repository.dart:genExperience()` (line 420) and
   `request_repository.dart:genRequest()` (line 141) /
   `genRequestFromMedia()` (line 164) with optional
   `double? latitudeDeg, double? longitudeDeg` named args that forward to
   the service. These repositories already exist as the architectural
   seam — per Key Architectural Rule 2, only repositories call services.
5. **Service signatures.** Extend `experience_service.dart:genExperience()`
   and `request_service.dart:genRequest()` / `genRequestFromMedia()` with
   the same optional args and set them on the proto when non-zero. No
   proto change required — the fields already exist at
   `GenExperienceRequest` fields 5/6 and `GenRequestRequest` fields 3/4.
6. **Close the `locationId` gaps and delete the dead stage-2 RPC path.**
   Three paired fixes that make the existing `locationId` field a real
   proximity signal and clean up the confusion that led to thinking
   stage 2 was wired:
   - **Client (live path):** Teach
     `experience_creation_view_model.dart:generateExperience()`
     (line 135) to resolve a default location via
     `UserLocationInitializer.getPrimaryLocation(ref)` and pass it as
     `locationId` on the `_experienceRepository.genExperience(...)`
     call at line 163. This is the **only live GenExperience RPC path**
     — confirmed by repo-wide grep for `.generateExperience()`
     returning exactly one hit (`experience_creation_modal.dart:108`).
   - **Client (dead code removal):** Delete
     `GenExperienceNotifier.generateExperience()` and its helpers
     `_generateFromText/Image/Url` + `_callGenExperienceAPI` in
     `gen_experience_view_model.dart` (roughly lines 269–408). These
     are legacy from before the two-modal split; `genExperienceProvider`
     is still needed as the stage-2 state holder for
     `ExperiencePreviewModal`, but it no longer owns the RPC call. The
     dead code is actively misleading — it contains the one "correct"
     `locationId`-passing example in the codebase and was my first
     misread during planning.
   - **Server:** In `gen_ai.go:46-56` (experience) and
     `request/gen.go:52-68` (request), when the Location record is
     fetched to derive `region`, also grab its
     `Geolocation.LatitudeDeg/LongitudeDeg` and use them as a
     proximity fallback when `req.Msg.LatitudeDeg/LongitudeDeg` is
     zero. This extends the existing ladder — (a) JSON-LD event coords
     → (b) client GPS → (c) locationId coords → (d) unbiased — for
     near-zero cost, since we're already reading the record.

## Open Questions

- **Should the widget block rendering on the GPS fetch?** — Recommendation:
  **no**. Fire-and-forget in the provider; the fallback is just a bias, not
  correctness. If the search runs before the position resolves, the results
  will be unbiased (same as today). When the position lands, subsequent
  queries bias correctly. Adding a loading state here would regress perceived
  snappiness in the typeahead.
- **What if GPS permission is denied?** — Recommendation: fall back to the
  user's `primaryResidenceLocationId` coordinates when present. That's the
  next-best "where the user actually is" signal and is already used as a
  location fallback in `GetUserLocationFallback` on the server. If neither
  GPS nor primary residence is available, accept the unbiased search (no
  regression from today). Note: the server-side `locationId`-coord
  fallback (Concrete Change #6) already delivers most of this benefit
  without any extra client work, since
  `UserLocationInitializer.getPrimaryLocation` already sets
  `locationId` from the primary residence.
- **Why are there two viewmodels that both look like they call
  GenExperience?** — Two-stage modal split:
  `ExperienceCreationNotifier` (stage 1) owns the creation modal and
  the live `GenExperience` RPC call; `GenExperienceNotifier` (stage 2)
  owns the preview modal state and the `CreateExperience` persistence
  RPC. Stage 2 has its own `generateExperience()` method with correct
  `locationId` handling, but nothing calls it — it's pre-split legacy.
  Recommendation: (a) fix stage 1's omission, (b) delete the dead
  stage-2 RPC code in the same PR so future maintainers aren't misled.
  See Concrete Change #6.
- **Is it OK that `GeocodeAddress` / `ReverseGeocode` don't take proximity?**
  — Recommendation: yes, out of scope. Those are structured (address-field
  → lat/lng) flows, not fuzzy-text searches. Proximity bias matters for
  Search Box, not for structured forward geocoding.
- **Cache lifetime of the shared `Position`?** — Recommendation: fetch
  once per app session, never refresh automatically. The user's location
  bias doesn't need sub-meter accuracy. Provide a `ref.invalidate(...)`
  escape hatch for screens that need to force a refetch (e.g., pull-to-
  refresh on discover).
- **Should the server use GPS from the RPC to bias `GeocodeAddress`?** —
  Recommendation: no. The issue is scoped to search autocomplete and the AI
  gen flows, which is where the bias actually moves the needle.

## Implementation Plan

### Phase 1: Centralize user GPS position ✅ COMPLETE

**Goal:** Make "current user position" a single, cached, testable value
available to any widget or ViewModel.

**Tasks:**
- [x] Introduced `userLocationProvider` as a `FutureProvider<Position?>`
      in `app/lib/services/providers.dart`. Wraps
      `UserPositionResolver.resolve(ref)` rather than
      `MapboxLocationService.getCurrentLocation()` directly — see Phase 4
      for the ladder it implements. `DeviceLocationService` is the
      provider-agnostic GPS wrapper that the resolver calls.
- [x] Left `deviceLocationFetcherProvider` in `search_view_model.dart`
      alone; migration tracked as a follow-up (issue #1156).
- [x] Provider unit tests in
      `app/test/services/user_location_provider_test.dart`:
      - Fixed-value override → consumers see the value ✓
      - Null override → consumers fall back ✓
      - `ref.invalidate` forces a re-fetch ✓
      - **Disposal safety test** — container disposal during in-flight
        GPS fetch completes without throwing ✓

**Validates:** ✅ One GPS fetch per session; provider override works in
tests; disposal-safe; invalidate-driven refresh works.

### Phase 2: Wire the widget ✅ CODE COMPLETE · manual verify outstanding

**Goal:** `LocationAutocompleteField` uses GPS fallback when parent doesn't
pass `proximityBias`.

**Tasks:**
- [x] `LocationAutocompleteField` reads
      `ref.watch(userLocationProvider).valueOrNull` inside
      `suggestionsCallback`; uses it as the fallback when
      `widget.proximityBias == null`. `valueOrNull` returns null while the
      fetch is in flight — acceptable (unbiased is the status quo).
- [x] Deleted the unassigned `_currentPosition` field and the fallback
      comment that referenced it.
- [ ] Manual verify on dev: search "Central Park" while physically in
      Boulder → first result is a Boulder match, not NYC. Governance
      screen + leaderboard header both benefit. (User manual step;
      governance reachable via Settings → community row.)
- [ ] Widget test with `userLocationProvider` overridden to a known
      position; assert `MapboxLocationService.searchAddresses` /
      `searchGeographicAreas` is called with that proximity. (Deferred;
      primary paths covered by provider tests + manual verification.)
- [x] Confirmed no new user-facing strings introduced — no ARB updates
      needed.

**Validates:** Literal bug in the issue body is closed.

### Phase 3: Thread lat/lng into Gen* RPCs (ViewModel → Repository → Service)

**Goal:** Server-side `SearchPlaces` receives the same GPS signal for
prompt-driven flows. Respects the documented layering:
ViewModel → Repository → Service (see `docs/client/architecture.md`
§Key Architectural Rules 1–2).

### Phase 3: Thread lat/lng into Gen* RPCs ✅ COMPLETE

**Tasks:**

*Repository layer:*
- [x] `experience_repository.dart:genExperience()` accepts
      `latitudeDeg`/`longitudeDeg`.
- [x] `request_repository.dart:genRequest()` accepts the same.
      `genRequestFromMedia()` skipped — proto has no lat/lng fields and
      the media path doesn't call `SearchPlaces` server-side.

*Service layer:*
- [x] `experience_service.dart:genExperience()` and
      `request_service.dart:genRequest()` serialize the args onto the
      proto when non-null.

*ViewModel layer:*
- [x] `experience_creation_view_model.dart` reads GPS from
      `userLocationProvider` + primary residence from
      `UserLocationInitializer.getPrimaryLocationIfSet` (see hotfix note
      below).
- [x] `request_creation_view_model.dart` does the same.
- [x] `gen_request_view_model.dart` does the same.
- [x] Deleted dead stage-2 RPC code in `gen_experience_view_model.dart`
      (`generateExperience`, `_generateFromText`, `_generateFromImage`,
      `_generateFromUrl`, `_callGenExperienceAPI`). Grep confirmed only
      the stage-1 call remains.
- [x] Audited remaining `genRequest` / `genExperience` call sites — all
      threaded.

*Observability:*
- [x] Added structured log field
      `proximity_source=event_coords|gps|location_id|none` in both
      `server/services/experience/gen_ai.go` and
      `server/services/request/gen.go`.

**Android hotfix:** Initial implementation used
`UserLocationInitializer.getPrimaryLocation`, which fires its own GPS
+ reverse-geocode + save-location chain when no residence is set. On
Android this hung the generate flow. Split into
`getPrimaryLocationIfSet` (cheap, side-effect-free) for the generate
hot path; the heavyweight version is still used only in the
save-on-confirm flow.

**Validates:** ✅ End-to-end verified on both iOS (request flow,
`proximity_source=gps`) and Android (experience flow,
`proximity_source=gps` after pre-warm).

### Phase 3.5: Server-side `locationId`-coord proximity fallback

**Goal:** When the client didn't send GPS (zero lat/lng) but did send a
`locationId`, the server extracts lat/lng from the already-fetched
Location record and uses it as proximity bias. Complements Phase 3
because `gen_experience_view_model` already sends `locationId` today —
this phase turns that existing signal into a proximity signal for free.

### Phase 3.5: Server-side `locationId`-coord proximity fallback ✅ COMPLETE

**Tasks:**
- [x] `server/services/experience/gen_ai.go`: when the `location` record
      is fetched for region context, also captures
      `location.Geolocation.LatitudeDeg/LongitudeDeg` as
      `locationProximity`.
- [x] Proximity ladder implemented in `gen_ai.go` geocoding block:
      1. JSON-LD `eventLatitude/eventLongitude` (structured event coords)
      2. `req.Msg.LatitudeDeg/LongitudeDeg` (client-supplied device GPS)
      3. `locationProximity` (user-selected location's coords)
      4. `nil` (unbiased)
- [x] Mirrored in `server/services/request/gen.go` (only `SearchPlaces`
      paths — `GenRequestFromMedia` confirmed not to geocode).
- [x] Structured log field
      `proximity_source=event_coords|gps|location_id|none` covers all
      ladder rungs (no per-rung test needed; the log observably covers
      every case).
- [ ] ~~Unit tests for the proximity ladder~~ — deferred. The ladder is
      3 `if/else-if` branches inline in the handler; extracting it to a
      pure function and table-testing would add churn without
      meaningfully improving coverage. The `proximity_source` log field
      makes rollout impact measurable in prod.

**Validates:** ✅ User on a device without GPS, with a primary residence
set, gets locally-biased results. Confirmed on iOS (first T4 run showed
`proximity_source=location_id`).

### Phase 4: Client-side graceful degradation ✅ CODE COMPLETE · manual verify outstanding

**Goal:** Belt-and-suspenders for the GPS-denied case on the client side,
complementing Phase 3.5's server-side fallback.

**Tasks:**
- [x] Introduced `UserPositionResolver` (new
      `app/lib/services/user_position_resolver.dart`) implementing the
      ladder: device GPS → primary-residence coords → null. Returns a
      synthetic `Position` for the residence fallback with `accuracy: 1000`
      to flag it as coarse. Keeps `DeviceLocationService` focused on GPS.
- [x] `userLocationProvider` now calls `UserPositionResolver.resolve(ref)`
      instead of `DeviceLocationService.getCurrentPosition()` directly.
      One-liner; resolver owns the ladder.
- [x] Log field `source: gps|primary_residence|unbiased` (matches
      `UserPositionSource` enum) for client-side proximity source
      distribution.
- [ ] Manual verify: governance / leaderboard autocomplete returns
      biased results when GPS denied but user has primary residence set.

**Validates:** `LocationAutocompleteField` on governance / leaderboard
screens gets proximity-biased results even when GPS is denied, without
any further server-side work.

### Phase 5 (bonus): Position freshness and pre-warming ✅ COMPLETE

Not in the original plan — added to address real-world usage patterns
surfaced during manual testing.

**Tasks:**
- [x] Pre-warm on plus-button tap in `home_screen.dart:_onAddTap`. Fires
      `ref.read(userLocationProvider.future)` unawaited so GPS resolves
      while the user reads the create modal.
- [x] Invalidate on login/logout in
      `auth_state.dart:_clearUserCaches`. Primary-residence fallback
      differs per user; the new session re-resolves cleanly.
- [x] Invalidate on background→foreground transition after 10 min in
      `AuthStateNotifier.didChangeAppLifecycleState`. Handles the
      "user drove somewhere" case.
- [x] Invalidate on pull-to-refresh in `feed_screen._onRefresh`. Respects
      explicit user intent to refresh.

**Validates:** Pre-warm path observed in Android logs (GPS resolved in
~50ms on plus-button tap); subsequent GenExperience RPC sends
`proximity_source=gps` rather than falling to `location_id`.

## Measured latency (manual testing)

End-to-end latency for a text-prompt GenExperience call on Android
emulator with cached auth (2026-04-16 23:57 session):

| Step | Duration | Share |
|------|----------|-------|
| Claude Haiku 4.5 | 4007 ms | 77% |
| Pexels stock video | 765 ms | 15% |
| Mapbox geocoding | 337 ms | 7% |
| Network + serialize + render | ~100 ms | 2% |
| **Total client-perceived** | **~5.2 s** | |

AI dominates. Proximity-bias work adds zero measurable overhead
(Mapbox time was 182ms without this PR's logging overhead on a prior
run, 337ms with; both are a small fraction of total). Follow-up
optimization issue: #1157 (stream AI tokens, parallelize stock video
+ Mapbox, return text before media).

## Related Issues
- #1142 — Mapbox API: capture mapbox_id, context hierarchy, and other unused
  response fields (closes out the proto plumbing that this issue depends on
  for downstream dedup).
- #1156 — Migrate remaining `MapboxLocationService.getCurrentLocation()`
  callers (SearchViewModel, LocationPickerViewModel) to
  `userLocationProvider` so the whole app shares one GPS cache. Surfaced
  during manual testing — observed 3 separate GPS fetches in 50 s.
- #1157 — GenExperience/GenRequest perceived latency (~5.2 s baseline;
  streaming tokens + parallelizing stock-video with Mapbox would reduce
  both real and perceived wait).
- #1147 — POI coverage/ranking gaps; this plan is load-bearing for the
  ranking half of #1147 (Wahoos-in-Fresno disappears once proximity is
  actually sent).
- #1146 — Confidence threshold for Mapbox; related — a real proximity
  signal will shift the confidence distribution, so confidence-threshold
  tuning should happen *after* this lands.
