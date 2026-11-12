# Issue #2188: Migrate maps tile/viewports and POI search, geocoding, reverse geocoding to Google Maps

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2188
**Filed:** 2026-05-26 | **Last activity:** 2026-05-26
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none
**Audit pass:** `/ripls-audit` — `docs/client/{architecture,caching,i18n,accessibility,testing}.md`,
`docs/server/{architecture,observability}.md`. Audit notes embedded inline as **[AUDIT]** callouts.

## Status (2026-05-27)

Phases 0–4 are done on branch `google_maps` and submitted as
**PR #2218** (https://github.com/Type2Fun-Foundation/ripls/pull/2218).
Dev Cloud Run service has already been flipped to `MAP_PROVIDER=google`
via a terraform apply earlier on this branch; once #2218 merges, the
deploy-on-push workflow rebuilds the dev image and the dev environment
runs Google end-to-end (server + Dart client + map widgets).

Phase 5 (prod cutover) waits on a healthy week on dev. Phase 6 (Mapbox
removal) waits on Phase 5. A handful of Phase 3 follow-ups are tracked
in the "Phase 3 itself still needs to do" list further down — none
of them block the Phase 5 cutover.

| Phase | Status | Notes |
|---|---|---|
| 0 — Pricing + parity audit + provisioning | ✅ **Done** | Audit run, both providers compared, prod + dev keys provisioned and partly restricted, budget alerts in place. Surfaced typed-proximity finding baked into Phase 3. |
| 1 — Server `location.Provider` interface | ✅ **Done** | Interface defined; 4 services migrated; dead gear MapboxClient deleted; `npm run build` + integration-tag build + unit tests all green. |
| 2 — Client `LocationSearchService` abstraction | ✅ **Done** | Abstract class + Riverpod provider + repository pass-throughs + widget→viewmodel→repository routing fix for the pre-existing layer violation. 25 touched-package tests pass. |
| 3 — Google implementations + cutover-ready | ✅ **Done (main path)** | Server `*GoogleMapsClient`, typed proximity, `--map-provider` selector, dev terraform flipped, Dart `GoogleLocationSearchService` + provider selector, env.dev.json/local.json switched to google. Follow-ups still open: Cloud NAT + prod-key IP restriction, server-side Place Details + reverse-geocode caches, session-token Autocomplete, per-API quota caps, `google-maps-api-key-local` dev convenience key. |
| 4 — Map widget migration | ✅ **Done** | `google_maps_flutter ^2.10.0` added; per-platform SDK init (iOS method channel, Android manifest meta-data via Gradle env var, web index.html template). `LocationMapPreview` + discover stack rewritten on GoogleMap. `map_helper.dart`, `ripple_map_marker.dart`, `web_map_placeholder.dart` deleted. Dark map style asset at `app/assets/map_styles/google_dark.json`. Verified on web, iOS sim, Android emulator. |
| 5 — Cutover | ⬜ **Next** (after PR merge + dev burns in) | Flip prod terraform `map_provider = "google"` + flip `env.prod.json` `MAP_PROVIDER` to `google` + apply. Per-API quota caps applied before traffic shift. Watch error logs + budget for one week. |
| 6 — Remove Mapbox | ⬜ Not started | Once Phase 5 cutover is healthy. Delete `mapbox_maps_flutter` from `pubspec.yaml`, `MapboxOptions.setAccessToken` from `main.dart`, `MapboxLocationService` + the Mapbox server client + integration tests, `--mapbox-access-token` flag + the `mapbox-access-token` GSM secret + terraform IAM binding + `mapboxAccessToken` config field + `MAPBOX_ACCESS_TOKEN` from `release_app.yaml` / `test_web_smoke.yaml` / `build_and_deploy_container.yaml` / `package.json` start/build scripts / `app/fastlane/Fastfile`'s `mapbox_dart_define`. Drop `Environment.mapboxAccessToken` + the `MAPBOX_ACCESS_TOKEN` env entries from `app/env.{local,dev,prod}.json`. Sweep proto comments per CLAUDE.md (`location_service.proto:114,118,122,374`) and rewrite Mapbox references in `docs/secrets.md`, `docs/secret-rotation.md`, `docs/server/observability.md`, `docs/ai/map.md`, `docs/ai/filters.md`, `server/location/README.md`. Remove the `--map-provider` flag itself (single provider now) and `Environment.mapProvider`. Drop the discover_filter_sheet l10n strings `webMapPlaceholderBody` and `a11yMapsNotAvailable` from both `app_en.arb` and `app_es.arb` (web no longer special-cased). Close #2156 and #2027. |

### What's in PR #2218 (commits on this branch in topological order)

Phase 3 server:
- `4d8c4e744` typed proximity widening in `location.Provider`
- `00f154ef2` `*GoogleMapsClient` HTTP implementation + integration test
- `dd0c9fcb0` `--map-provider` selector + dev terraform → google (applied)

Phase 3 client:
- `5b362340c` `GoogleLocationSearchService` + provider selector + env defines

Phase 4 foundation:
- `a62d0eb35` `google_maps_flutter` pubspec + per-platform SDK init + dark JSON
- `bccbe656a` `LocationMapPreview` rewrite on `GoogleMap`
- `1233a0379` iOS Dart→AppDelegate method-channel GMS init
- `66abd10ee` revert noise-only xcconfig comments
- `e90bb6be9` fix iOS Swift compile (`applicationRegistrar.messenger()`, explicit closure types)

Phase 4 widgets:
- `1b041c35a` discover stack migrated; `map_helper.dart`, `ripple_map_marker.dart`, `web_map_placeholder.dart` deleted
- `9dd7821cf` shrink discover markers (`BitmapDescriptor.bytes` width:40) + skip NA→zoom flight

Build / infra / ergonomics:
- `acdf53d26` local `start:server` flipped to `--map-provider=google`
- `396b1ce65` BSD-sed escape fix in `build:web:*` `GOOGLE_MAPS_API_KEY` substitution
- `2a255cd4f` + `bc845248e` `start:app:local:{ios,android}` per-platform Maps keys
- `399454e4e` plan: add `google-maps-api-key-local` dev convenience key follow-up
- `49fbf9561` Kotlin Gradle Plugin 2.1.0 → 2.3.21
- `cdfd5fb0b` migrate Kotlin DSL: `kotlinOptions` → `kotlin { compilerOptions }`
- `5c24bde60` Gradle wrapper 8.12 → 8.14.5; AGP 8.9.1 → 8.13.2
- `de04d371a` + `2ad276293` + `750e25aa6` force plugin subprojects to compileSdk 36 (`afterEvaluate` with `state.executed` guard for force-evaluated subprojects via `evaluationDependsOn(":app")`)

### What's intentionally still on Mapbox (to keep rollback viable through Phase 5)

- `main.dart` still calls `MapboxOptions.setAccessToken` — kept until Phase 6 removes the dep.
- `mapbox_maps_flutter` still in `pubspec.yaml` — same reason.
- `--mapbox-access-token` flag, `mapbox-access-token` GSM secret + terraform binding, `MAPBOX_ACCESS_TOKEN` env-var plumbing through every workflow + `start:server` + `build:web:*` + `start:app:*` + Fastlane — all kept as the rollback surface.
- Prod `env.prod.json` keeps `MAP_PROVIDER=mapbox`. Prod terraform keeps `map_provider = "mapbox"`. Phase 5 flips both.

### What still needs picking up before / during Phase 5

These are the Phase 3 follow-ups the migration plan calls out that
didn't land in #2218. They don't block the cutover but are best done
in the dev burn-in window so prod inherits them:

- **Per-API daily quota caps** on prod (and looser caps on dev). The
  provisioning runbook (`docs/issues/2188-google-maps-provisioning.md`)
  has the `gcloud alpha services quota update` form; the values are
  the Phase 5 prereq.
- **Cloud NAT + prod-key IP restriction.** `google_compute_router` +
  `google_compute_router_nat` + `google_compute_address` in
  `terraform/environments/prod/`, flip Cloud Run egress to
  `ALL_TRAFFIC`, apply `--allowed-ips=<static-ip>` to the prod
  server key.
- **Server-side Place Details cache** (keyed by placeId) + **reverse-
  geocode cache** (keyed by rounded coordinate) — both lower the
  bill against the Phase 3 audit's projected volume.
- ~~**Session-token-backed Autocomplete** on the client side.~~ Landed
  2026-05-27. `LocationSearchService` widened with optional `sessionToken`
  on the search methods and a new `resolveSuggestion(LocationResult,
  sessionToken)` that fetches Place Details lazily on selection. Picker
  + governance viewmodels mint a UUID per session, thread it through
  every autocomplete + the final Place Details, and rotate after each
  resolve. Side benefit (the actual reason this moved out of follow-up
  scope): the previous Geocoding-only client couldn't return POIs, so
  "Zilker Park" returned no results in the picker. Places Autocomplete
  fixes that functional gap, not just the cost story.
- **`google-maps-api-key-local` dev convenience key** that whitelists
  all three client Maps SDK API targets at once. Once provisioned,
  collapse `start:app:local:{ios,android}` back into one script.
- **Marker scale-on-select regression** in the discover screen — the
  Mapbox helper scaled selected markers 1.0× → 1.35×. Google's
  `Marker` has no dynamic size; the gallery overlay below the map
  already communicates selection, so the helper records selection
  state but doesn't visually swap the icon. Adding a scaled
  `BitmapDescriptor` render is the fix.

### Open Xcode side-effects not in the PR

The working tree has a few unstaged iOS files (Podfile.lock,
Runner.xcodeproj/project.pbxproj, Runner.xcscheme, two `swiftpm/`
dirs) that Xcode wrote when the iOS build ran. They aren't in the
PR — verify whether any are meaningful before committing in a
follow-up.

**Infrastructure already in place from Phase 0** (so Phase 3 doesn't have
to redo this):

- GCP API keys per environment per platform (8 keys total: dev/prod ×
  server/web/android/ios), API-restricted to the right Maps Platform
  services.
- GSM secrets in both projects: `google-maps-api-key-{server,web,android,ios}`.
- Terraform IAM bindings: Cloud Run runtime SA → server key;
  GitHub Actions SA (prod only) → web/android/ios keys.
- Server flag plumbing: `--google-maps-api-key-server` /
  `--google-maps-api-key-server-file`. Optional fetch in
  `server/entrypoint.sh` (empty secret → feature disabled).
- Application restrictions on prod keys: web (referrer
  `https://ripls.app/*`), iOS (bundle `org.ripls.app`), Android (both
  Firebase-registered SHA-1s + `org.ripls.app`). Server-key IP
  restriction deferred — needs Cloud NAT (a Phase 3 prereq).
- Budget alerts on the billing account: $50/month dev, $500/month
  prod, scoped to the five Maps Platform APIs, emails at 50/90/100%.
- Per-API daily quota caps: **not** applied — deferred to Phase 3
  cutover work (set caps once we have real volume data, otherwise we
  risk a real outage on cutover).
- Local dev path: `scripts/run_server_with_sm_secrets.sh` knows about
  `google-maps-api-key-server` and tolerates the secret being absent
  (so a local `npm run start:server` works whether or not the
  developer has fetched the key).

**Phase 0 audit outcomes worth re-reading before Phase 3 starts:**

- `docs/issues/2188-parity-audit.md` — the substantive findings table.
- The Phase 3 "Typed proximity semantics" subsection below — the
  single biggest design decision the audit surfaced.

## Problem Statement

The Ripls app uses Mapbox for every map-related concern: native map rendering
(iOS + Android, no web), forward and reverse geocoding, POI search (Search
Box API), and tile styles. This has two real problems:

1. **No web parity.** `mapbox_maps_flutter: 2.19.1` has no web implementation.
   Today the web bundle hides every screen that mounts a `MapWidget`; once
   the rest of the web app lands (issue #2157), each of those screens
   breaks on web. This is the original scope of issue #2156, which the
   present issue subsumes.
2. **POI + geocoder quality.** Google's POI dataset and forward/reverse
   geocoder are materially better than Mapbox's for the queries Ripls users
   actually run — small businesses, parks, events, international
   addresses. Migrating fixes a felt quality gap independent of the web
   story.

The author's follow-up comment narrows the scope from the original issue
body: **the client map widget will not be wrapped behind a neutral
abstraction**. The widget surface (camera, annotation managers, style URIs,
event streams) is too provider-specific to wrap cleanly. The widget call
sites will switch from `package:mapbox_maps_flutter` to
`package:google_maps_flutter` directly. Abstraction layers are still
required for the two boundaries where the surface *is* small enough to be
neutral: the server's geocoding / POI search client, and the client-side
location search service.

## Context from Comments

The single comment on the issue (from the author) is load-bearing:

> "We don't actually need to abstract the client map widget, it is too
> extensive and specific of an interface to abstract to common
> functionality. We'll just use the google_maps_flutter package directly
> without wrapping."

That changes the plan in the issue body. The widget surface is dropped
from the "abstract first, then implement" approach and becomes a direct
substitution. The abstraction boundary now applies only to:

- **Server**: geocoding / POI search behind a `location.Provider`-style
  interface.
- **Client (non-widget)**: location autocomplete / reverse geocode behind a
  `LocationSearchService`-style interface.

The "treat this migration as the first of many" framing from the issue
body still stands for those two surfaces. The widget surface is treated
as a per-call-site migration with no abstraction commitment, because the
cost/benefit is bad there.

## Codebase Reality Check

A few facts from the current code that the issue body either skipped or
got slightly wrong:

- **Removed call site.** The issue body (inherited from #2156) lists
  `app/lib/presentation/screens/leaderboard/leaderboard_screen.dart` and
  `app/lib/presentation/widgets/leaderboard/leaderboard_map_helper.dart`.
  Neither file exists anymore — the cross-community leaderboard was
  ripped out in #2102 (commit `f1eddeca8`). The current widget-side
  Mapbox call-site list is **6 files**, not 8:
  - `app/lib/main.dart` (Mapbox access-token init)
  - `app/lib/core/utils/map_helper.dart`
  - `app/lib/core/utils/discover_map_helper.dart`
  - `app/lib/presentation/screens/discover/discover_screen.dart`
  - `app/lib/presentation/widgets/ripple_map_marker.dart`
  - `app/lib/presentation/widgets/discover/discover_filter_sheet.dart`
    (style URIs only — `MapboxStyles.STANDARD/OUTDOORS/SATELLITE/DARK`)
  - `app/lib/presentation/widgets/location/location_modal_widgets.dart`
- **Server abstraction is partly done.** `server/location.PlaceResult`
  is already provider-neutral (Name, FullAddress, Type, Coordinates,
  ExternalID/ExternalProvider, full admin hierarchy). The shape doesn't
  need to change — we just need the wire-decoder types (`mapboxFeature`,
  `searchBoxResponse`, `searchBoxContext`, `mapboxResponse`,
  `mapboxTokenResponse`) to live behind an interface so the Google
  implementation can replace them.
- **Two services already define a per-service Mapbox interface.**
  `services/experience/service.go` and `services/gear/service.go` each
  declare a `MapboxClient interface { ... }` with the subset of methods
  they use. `services/location/service.go` and
  `services/request/service.go` still hold a concrete
  `*location.MapboxClient`. The migration should unify on **one**
  `location.Provider` interface in the `location` package and have all
  four services depend on it.
- **Dead wiring in gear.** `services/gear/service.go` declares
  `MapboxClient interface { Search(ctx, query, region) ([]location.PlaceResult, error) }`,
  but the production `*location.MapboxClient` doesn't have a `Search`
  method, and `main.go` never calls `gearService.SetMapboxClient(...)`.
  So gear's geocoding path is dead. This is an existing bug that
  `geocodeViaMapbox` always returns nil in prod. Worth fixing as part of
  this work (either delete the dead branch or fold it into the unified
  interface and wire it up).
- **`MapboxLocationService` is static-only.** The client service is a
  class with only static methods — `MapboxLocationService.searchAddresses(...)`,
  `.searchGeographicAreas(...)`, `.reverseGeocode(...)`. Callers
  reference the type directly. Abstracting this requires moving to
  instance methods + DI (Riverpod provider, since the app already uses
  Riverpod).
- **Proto layer is mostly clean already.** The `external_place_provider`
  field is a real data-model concern (it identifies which vendor issued
  the opaque `external_place_id`) and stays. Three field-doc comments
  name Mapbox where they shouldn't, per CLAUDE.md's proto-comments rule:
  - `proto/ripls/api/location_service.proto:114, 118, 122` ("extracted
    from Mapbox context") → "extracted from geocoding context" or
    similar.
  - `proto/ripls/api/location_service.proto:374` ("based on Mapbox
    feature type") → "based on the geocoder's place-type taxonomy".
  - The `(e.g., "mapbox", "google")` examples in the `provider` field
    comments are arguably fine since the field's *value* is canonically
    a vendor name — leave them.
- **Infrastructure surface.** Mapbox token lives at:
  - Server: `--mapbox-access-token` flag, `cfg.mapboxAccessToken`,
    GSM secret `mapbox-access-token` (dev + prod).
  - Client: `String.fromEnvironment('MAPBOX_ACCESS_TOKEN')` injected
    via Flutter build, sourced from GSM in
    `.github/workflows/release_app.yaml` (two locations) and
    `.github/workflows/test_web_smoke.yaml`.
  - Terraform: `terraform/environments/dev/main.tf`,
    `terraform/environments/prod/main.tf`,
    `terraform/modules/containers/main.tf`,
    `terraform/modules/containers/variables.tf`.
- **Existing data.** Locations already saved with
  `external_place_provider="mapbox"` stay readable forever — the value
  is opaque storage. They just won't be re-resolved against Mapbox once
  the client is gone.

## [AUDIT] Architectural Compliance Notes

The plan was reviewed against the reference architecture docs. The
substantive findings, with the fixes folded into the Implementation Plan
below:

1. **Client architecture: widgets must not call services directly.**
   `docs/client/architecture.md` is strict that **ViewModels are the
   only layer that calls Repositories** and **Repositories are the only
   layer that calls Services**. Today the static `MapboxLocationService`
   is called directly from `governance_screen.dart`,
   `location_picker_modal.dart`, and `location_autocomplete_field.dart` —
   that is an existing architecture violation, predating this work. The
   correct shape is: the new `LocationSearchService` is consumed *only*
   by `LocationRepository`, which exposes async methods to ViewModels.
   Widgets call ViewModels; ViewModels call the repository; the
   repository calls the service. Phase 2 has been updated to take the
   migration as a chance to fix this rather than preserve the
   violation. The widget-level call sites move to repository methods on
   `LocationRepository`, accessed via ViewModels.
2. **Client caching: the `LocationSearchService` should not be exposed
   as a top-level Riverpod provider that widgets/viewmodels read.** Per
   `docs/client/caching.md` rule 1, ViewModels never access services
   directly. The service must live behind `LocationRepository`; only
   the repository depends on the service-level provider. The plan
   originally said viewmodels would consume `locationSearchServiceProvider`
   directly — that's removed.
3. **Server architecture: gear's dead `MapboxClient` interface should be
   deleted, not preserved.** `docs/server/architecture.md` favors a
   single shared library interface (`location.Provider`) over per-service
   interfaces that duplicate it. The phase-1 task now explicitly says
   delete the gear-specific interface and its dead `geocodeViaMapbox`
   path; if gear ever wants geocoding back, it depends on
   `location.Provider` directly.
4. **Server error wrapping: `GoogleMapsClient` errors must use
   `connecterr.Internal`.** Per `docs/server/architecture.md` and
   CLAUDE.md, bare `connect.NewError(connect.CodeInternal, err)` is
   gated by CI. Any new error path the Google client surfaces (network,
   decode, quota) must wrap via `connecterr.Internal(ctx, op, err, kv...)`
   so raw API bodies don't leak to the wire.
5. **Server observability: `external_service` and health-check
   `Backend` must use the canonical vendor names.** Per
   `docs/server/observability.md` field-name conventions,
   `external_service` is the existing key with documented values like
   `"mapbox"`, `"openai"`. The new client uses `external_service="google_maps"`
   (or `"google"`; pick one and use it consistently across logs and the
   health-status `Backend` field). Logging in `GoogleMapsClient` must
   go through `logging.LoggerWithContext(ctx)` with the standard fields
   (`operation`, `duration_ms`, `error`); plain `log.Printf` and
   `fmt.Printf` are not acceptable.
6. **Server testing: a `google_integration_test.go` mirroring
   `mapbox_integration_test.go` is required.** The existing pattern
   uses `//go:build integration` and exercises the real API against a
   test token. The plan now requires the same for the Google
   implementation so it gets the same CI gating Mapbox has today.
7. **Client testing: disposal tests + mock regeneration.**
   `docs/client/testing.md` requires disposal tests for any async
   notifier touched, and Mockito mocks must be regenerated after
   service-interface changes. The `LocationRepository` already has
   tests (`app/test/data/repositories/`); Phase 2 must update them to
   mock `LocationSearchService` and re-run `flutter pub run build_runner
   build`. ViewModels that consume the repository need their `.mocks.dart`
   regenerated.
8. **Client i18n: any user-visible map UI strings must use
   `context.l10n.*`.** The discover-filter labels
   (`discoverFilterMapDefault`/`Outdoor`/`Satellite`/`Dark`) already
   live in ARB files and stay neutral. If Phase 4 introduces any new
   user-facing strings (e.g., "Style not supported on this platform"
   fallback), they go in `app_en.arb` and `app_es.arb` together; the
   parity gate (`npm run lint:dart:l10n-parity`) blocks otherwise.
9. **Client accessibility: any new interactive map UI must use the
   `widgets/accessibility/` primitives.** If Phase 4 adds a map style
   toggle, marker tap surfaces, or any new tappable controls, they
   compose from `Tappable` / `IconAction` / `Toggle` with required
   `semanticsLabel`s pulled from `context.l10n`. The `scripts/check_a11y.js`
   allowlist is empty — violations fail CI.
10. **Proto comments: vendor names are banned in `.proto` files (per
    CLAUDE.md) but allowed in READMEs and `docs/`.** The plan already
    distinguishes these. When implementing `GoogleMapsClient`, any new
    proto fields or enum values it would motivate must use
    capability-first names (e.g. `GEN_STREAM_ERROR_CODE_GEOCODING_FAILED`,
    not `..._GOOGLE_FAILED`).

## Potential Solutions

### Option A: Single-step cutover (Mapbox out, Google in)

- **Approach:** One large diff that introduces `google_maps_flutter`,
  rewrites every widget call site, swaps the client `LocationService`
  implementation, swaps the server `location.Provider` implementation,
  and removes Mapbox in the same PR.
- **Pros:** No long-lived dual-provider state. One review, one rollback
  point.
- **Cons:** Huge blast radius. Map UI regressions on three platforms
  land at once. Any quality regression in Google's geocoder vs.
  Mapbox's lands at once too. Hard to bisect if something breaks.
- **Effort:** Large.

### Option B: Phased migration with abstractions first (Recommended)

- **Approach:** Land the server interface and client search abstraction
  in their own PRs with Mapbox still behind them (no behavior change).
  Then add Google implementations behind the same interfaces, plus a
  `--map-provider=google|mapbox` config flag (server) and a Riverpod
  provider override (client). Then switch widget call sites file-by-file
  to `google_maps_flutter`. Then cut over via config. Finally delete
  Mapbox.
- **Pros:** Every step is independently reviewable, testable, and
  reversible. Behavior-preserving refactor PRs are easy to merge.
  Provider config flag lets dev validate Google end-to-end against prod
  Mapbox in parallel without committing. Widget migration can land
  per-screen, surfacing regressions on the smallest possible surface.
- **Cons:** Longer wall-clock to "Mapbox is gone" (likely 4-6 PRs over
  a few weeks). Dual-provider state exists for the duration of the
  migration; need test coverage for both.
- **Effort:** Large, but distributed.

### Option C: Use `google_maps_flutter` for web only, keep Mapbox on mobile

- **Approach:** Conditional import: `mapbox_maps_flutter` on
  iOS/Android, `google_maps_flutter` on web. Server keeps Mapbox.
- **Pros:** Minimum-cost fix for #2156 (the original problem).
- **Cons:** Two SDKs to maintain forever; two style systems; doesn't
  solve the POI / geocoder quality problem; per-platform map UX will
  diverge subtly. Defeats the explicit goal of "even support across
  iOS, Android, and Web." Server still gets the worse geocoder.
- **Effort:** Medium up front, ongoing maintenance tax.

## Recommendation

**Option B (phased migration with abstractions first).** It matches the
issue author's framing — abstractions are load-bearing on the two
surfaces where the cost is reasonable (server, client search), and the
widget surface migrates directly without an abstraction commitment.
Phasing turns one scary PR into a series of small reversible ones, and
the dual-provider window lets us A/B-validate Google against Mapbox on
real queries before deleting anything.

Two tradeoffs in this recommendation:

- **Worth the abstraction tax.** Two new interfaces (server-side
  `location.Provider`, client-side `LocationSearchService`) are small —
  4 methods each. They're already half-built (per-service interfaces in
  experience + gear, neutral `PlaceResult` and `LocationResult` DTOs).
  The work to finish them is small and pays for itself the first time we
  ever want to A/B-test a provider or swap one out.
- **Worth the dual-provider window.** Keeping both implementations
  compiled simultaneously costs a few weeks of build-time bloat and a
  little CI complexity. We get back the ability to cut over via a
  config flag without redeploying, which makes the "Google's geocoder
  is worse than Mapbox's for use case X" surprise survivable instead of
  catastrophic.

## Open Questions

- **Q1: Where does the server `Provider` interface live?** —
  **Recommendation:** in the existing `server/location` package, as
  `location.Provider`. Keep `MapboxClient` as `*location.MapboxClient`
  (already there), add `*location.GoogleMapsClient` next to it, both
  implement the interface. Don't create a new package — the package
  boundary is fine; the dependency direction (`services/*` → `location`)
  is already right. The doc.go and README need updating to drop the
  "uses Mapbox" framing.
- **Q2: How is the server provider selected?** —
  **Recommendation:** a new CLI flag `--map-provider=mapbox|google`
  (default `mapbox` initially, flipped to `google` once parity is
  validated). Coexists with `--mapbox-access-token` and a new
  `--google-maps-api-key` / `--google-maps-api-key-file`. Both clients
  can be constructed if both tokens exist; main.go picks one based on
  the flag. After Phase 5, the flag and the Mapbox key go away.
- **Q3: How are the client-side static methods migrated?** —
  **Recommendation:** convert `MapboxLocationService` from static
  methods to an instance class implementing a new abstract
  `LocationSearchService`. Expose it through a Riverpod provider
  (`locationSearchServiceProvider`), **consumed only by
  `LocationRepository`** — not by viewmodels or widgets. Add a
  `GoogleLocationSearchService` alongside. Per
  `docs/client/architecture.md` and `docs/client/caching.md`,
  ViewModels call repositories and repositories call services; nothing
  else calls services. The existing widget-level static calls in
  `governance_screen.dart`, `location_picker_modal.dart`, and
  `location_autocomplete_field.dart` get rerouted through
  `LocationRepository` methods (e.g. `searchAddresses`,
  `searchGeographicAreas`, `reverseGeocode`) called from new or
  existing ViewModels. The migration is the right moment to fix this
  pre-existing architecture violation rather than carry it forward.
- **Q4: How is the client-side provider selected?** —
  **Recommendation:** a compile-time constant in `Environment` (already
  the pattern for `mapboxAccessToken`). Add `Environment.mapProvider`
  with values `'mapbox'` or `'google'`; the Riverpod provider switches
  on it. Production builds set it via `--dart-define=MAP_PROVIDER=...`
  in `release_app.yaml`. Default is `mapbox` until parity validation
  completes.
- **Q5: Map widget initialization differs per platform — what's
  needed?** — **Recommendation:** plan for all three:
  - **Android**: `<meta-data android:name="com.google.android.geo.API_KEY"
    android:value="..."/>` in
    `app/android/app/src/main/AndroidManifest.xml`. Key injected via
    `--dart-define` is not enough; the manifest needs the value at build
    time, so the release workflow needs to template the manifest (or use
    `manifestPlaceholders` with a Gradle property fed from the
    workflow).
  - **iOS**: `GMSServices.provideAPIKey(...)` in
    `app/ios/Runner/AppDelegate.swift`. Same key, threaded from
    `--dart-define` or an Info.plist entry.
  - **Web**: `<script src="https://maps.googleapis.com/maps/api/js?key=..."></script>`
    in `app/web/index.html`. The key is a public client-side key (must
    have HTTP referrer restrictions set in the GCP console).
- **Q6: Should we keep one Google API key or three?** —
  **Recommendation:** one key per environment (dev, prod), restricted
  to the APIs we use (Maps SDK for Android, Maps SDK for iOS, Maps
  JavaScript API, Places API, Geocoding API). Apply application
  restrictions: HTTP referrers for the web key, package-name+SHA1 for
  Android, bundle-ID for iOS. The same key string can ship to all three
  platforms if restrictions are set correctly; this is simpler than
  three keys. **Server** uses a separate key (no application
  restrictions; IP-restricted to Cloud Run egress IPs if possible).
- **Q7: Pricing risk.** — **Recommendation:** before Phase 3 cutover,
  run the migration script over a week of representative production
  query volume against Google Places Autocomplete + Geocoding API and
  Maps SDK loads, then compare the projected Google bill against the
  current Mapbox spend. Set a budget alert at the projected number +
  20%. Google's per-request pricing is higher than Mapbox's; the
  difference is unlikely to be material at our current volume but is
  not zero, and the failure mode (waking up to a surprise four-figure
  invoice) is recoverable but unpleasant.
- **Q8: What happens to existing rows with
  `external_place_provider="mapbox"`?** — **Recommendation:** nothing.
  They keep their value forever; we won't re-resolve them. The provider
  field is exactly there for this reason. Future writes (post-cutover)
  will set `external_place_provider="google"`. Both values are valid
  storage and both clients gracefully fall through to coordinate-based
  matching when they don't recognize the provider on lookup.
- **Q9: Mapbox style URIs (`STANDARD`/`OUTDOORS`/`SATELLITE`/`DARK`) in
  `discover_filter_sheet.dart` — Google equivalents?** —
  **Recommendation:** Google has `MapType.normal`, `MapType.terrain`,
  `MapType.satellite`, `MapType.hybrid`, and supports custom JSON
  styles for dark mode. Map `STANDARD → normal`, `OUTDOORS → terrain`,
  `SATELLITE → satellite`, `DARK → normal + dark-style JSON`. The dark
  JSON style needs to be authored and added under `app/assets/`. The
  l10n strings (`discoverFilterMapDefault` etc.) keep their existing
  labels — they're user-facing and provider-neutral already.
- **Q10: iOS minimum version.** — **Recommendation:** verify the
  current iOS Podfile / project sets the minimum to iOS 14 or later
  (required by `google_maps_flutter_ios`). If not, bump it as part of
  Phase 2 and validate that no other plugin objects.
- **Q11: Do we need the `RippleMapMarker` aesthetic in Google Maps?** —
  **Recommendation:** for parity, yes — `google_maps_flutter` supports
  `Circle` overlays which can be layered just like Mapbox's
  `CircleAnnotation`. Rewrite `ripple_map_marker.dart` against Google's
  `Circle` type with the same three-layer geometry. The aesthetic
  intent stays the same; the API call changes.

## Implementation Plan

### Phase 0: Pricing + parity audit + provisioning — ✅ Done
**Goal:** know before we build whether Google's geocoder/POI quality
beats Mapbox's on representative Ripls queries, and whether the price is
acceptable.
**Tasks:**
- [x] **Provisioning (deferred from Phase 3, pulled forward at user
      request):** GCP project APIs enabled, 4 application-restricted
      keys per environment (server/web/android/ios), GSM secrets,
      terraform IAM bindings, server flag plumbing, budget alerts.
      All CLI-driven via `scripts/provision_google_maps_2188.sh`.
      Runbook: `docs/issues/2188-google-maps-provisioning.md`.
- [x] Build the parity-audit harness covering Mapbox Search Box vs.
      Google Places Autocomplete (New) + Place Details + Geocoding,
      with both `locationBias` and `locationRestriction` modes for
      proximity-aware queries. `scripts/parity_audit_2188.js`.
- [x] Build a 50-query starter sample with US POIs, international
      landmarks, fuzzy autocomplete, sparse-coverage reverse
      coordinates. `docs/issues/2188-parity-queries.json`.
- [x] Run the harness against ripls-dev keys; record findings.
      Total cost ~$0.56 across two runs.
- [x] Write up the audit. `docs/issues/2188-parity-audit.md` with
      methodology, cost model, side-by-side findings, decision +
      mitigations.
- [ ] Production-log query sampling. **Not done** — the
      representative 50-query sample was used instead. Pulling real
      production volume from Cloud Logging stays as a Phase 3 task
      (needed to size the cost projection and the per-API quota caps).
- [ ] Final cost projection. **Partly blocked** on the line above.
      Audit doc has order-of-magnitude estimates and the per-call cost
      shape; final dollar number needs production volume.

**Validates:** the migration is worth doing on quality grounds, and
won't blow up the budget. **Outcome:** proceed with mitigations — see
Phase 3's "Typed proximity semantics" section and the audit doc's
"Required mitigations before cutover" list for the specific quality
fixes Phase 3 must implement.

### Phase 1: Server `location.Provider` interface (Mapbox-only behind it) — ✅ Done
**Goal:** every server caller depends on the interface; behavior is
unchanged.
**Tasks:**
- [x] Define `location.Provider` in `server/location/provider.go` with
      methods: `ForwardGeocode`, `SearchPlaces`, `ReverseGeocode`,
      `CheckHealth`. Signatures lifted verbatim from current
      `MapboxClient`. **(Note: signatures will widen in Phase 3 — see
      "Typed proximity semantics" below.)**
- [x] Make `*MapboxClient` satisfy `Provider` (compile-time
      `var _ Provider = (*MapboxClient)(nil)` in `provider.go`).
- [x] Refactor `server/services/location/service.go` to hold
      `location.Provider` instead of `*location.MapboxClient`. Field
      renamed to `locationProvider`.
- [x] Refactor `server/services/request/service.go` similarly. Internal
      helper `geocodeViaMapbox` renamed to `geocodePlaceQuery`. Setter
      `SetMapboxClient` renamed to `SetLocationProvider`. `main.go`
      wiring updated.
- [x] Replace the per-service `MapboxClient` interface in
      `server/services/experience/service.go` with `location.Provider`
      (was a strict subset). Test mock (`mockLocationProvider`) updated
      to satisfy the full interface (gained `CheckHealth`).
- [x] **[AUDIT]** Delete the dead `MapboxClient` interface and
      `geocodeViaMapbox` / `MapboxFn` branch in
      `server/services/gear/{service,gen_fanout,gen_ai,gen_ai_streaming}.go`.
      Production never wired the gear service to a mapbox client; all
      branches were unreachable.
- [x] Update `server/location/README.md` and `doc.go` to describe the
      package as the geocoding-provider abstraction (capability-first).
- [x] Compile-time interface assertion in `provider.go` (in place of a
      separate unit test).
- [x] `npm run build` passes; `go build -tags=integration ./server/...`
      passes; unit tests pass on all five touched packages.

**Validates:** ✅ all callers compile against `Provider`, integration
tests pass unchanged, no behavior change. Commits: `888216caf` (plan
+ interface), `dc45b8714` (services/location), `72096284e`
(services/request), `77398d946` (services/experience), `955ce2f92`
(gear dead-code removal), `1da01b365` (docs).

### Phase 2: Client `LocationSearchService` abstraction (Mapbox-only behind it) — ✅ Done
**Goal:** every Dart caller of geocoding/POI search goes through
`LocationRepository`, which depends on an abstract
`LocationSearchService`; behavior is unchanged; the existing
widget-direct-to-service violations are fixed in passing.
**Tasks:**
- [x] Define `LocationSearchService` abstract class in
      `app/lib/services/location_search_service.dart`. Provider-neutral
      `LocationResult` DTO moved here from `mapbox_location_service.dart`
      and re-exported there for back-compat.
- [x] Convert `MapboxLocationService` from static methods to instance
      methods that satisfy `LocationSearchService`. Now takes an
      injectable `http.Client` + access token for testability.
- [x] Add `locationSearchServiceProvider` Riverpod provider returning
      the Mapbox instance (default). Consumed only by
      `LocationRepository`'s provider — never by viewmodels or widgets.
- [x] **[AUDIT]** Add three repository methods to
      `LocationRepository`: `searchAddresses`, `searchGeographicAreas`,
      and renamed `reverseGeocodeViaMapbox` → `reverseGeocodeViaSearchService`.
      All pass-throughs (no caching, per existing pattern).
- [x] **[AUDIT]** Migrate widget-level callers to go through
      ViewModels → Repository:
      - `location_picker_modal._onMapTap` now calls
        `ref.read(locationPickerProvider.notifier).reverseGeocodeForLatLng(...)`.
      - `LocationAutocompleteField` redesigned to take a typed
        `LocationSearchCallback` parameter; the widget no longer imports
        the service. Each caller wires the callback to its viewmodel's
        `searchAddresses` / `searchGeographicAreas` method.
      - `governance_screen` wires `GovernanceNotifier.searchGeographicAreas`
        into both autocomplete sites.
- [x] Viewmodel-level updates: `LocationPickerNotifier` gained
      `reverseGeocodeForLatLng`, `searchAddresses`, `searchGeographicAreas`.
      `GovernanceNotifier` gained the two search methods.
- [x] **[AUDIT]** Regenerated Mockito mocks via build_runner;
      `location_picker_view_model_test` updated to mock the renamed
      method.
- [x] **[AUDIT]** Disposal test for `applyDevicePosition` already
      exists in `location_picker_view_model_test.dart` and was updated;
      passes.
- [x] All tests pass: 25 tests across `location_repository_test.dart`,
      `location_picker_view_model_test.dart`,
      `governance_view_model_test.dart`. `flutter analyze` clean (3
      pre-existing info-level lints unrelated to this work).

**Validates:** ✅ client-side abstraction shape is right; viewmodels
compile against the repository (not the service); layer-violation
fixed; tests mock the service cleanly. Commit: `d0badb545`.

### Phase 3: Add Google implementations behind both interfaces — 🟡 Next
**Goal:** Google Maps is available end-to-end behind the same
abstractions, selectable via config. Mapbox is still the default.

**Pre-completed by Phase 0** (don't re-do):

- ✅ GCP API keys per env per platform (8 keys total) + GSM secrets +
  terraform IAM bindings + API restrictions + application restrictions
  on prod web/iOS/Android.
- ✅ `--google-maps-api-key-server` flag plumbed in `server/main.go`,
  fetched optionally by `server/entrypoint.sh` and
  `scripts/run_server_with_sm_secrets.sh`. Currently unused (no
  `GoogleMapsClient` exists yet); flips on automatically when Phase 3
  ships the client and wires it through `main.go`.
- ✅ Budget alerts on the billing account (dev $50/mo, prod $500/mo).

**Phase 3 itself still needs to do:**

- Build `*GoogleMapsClient` satisfying `location.Provider`
- Widen the `Provider` interface to typed proximity (see below)
- Implement the audit-derived quality mitigations
- Add the `--map-provider` selector flag
- Apply per-API quota caps (deferred from Phase 0 pending volume data)
- Provision Cloud NAT static IP and apply the prod server-key IP
  restriction
- **Provision a `google-maps-api-key-local` dev convenience key** that
  whitelists all three client Maps SDK API targets at once
  (`maps-ios-backend`, `maps-android-backend`, `maps-backend`) plus
  `places` and `geocoding`. Today the dev keys are split per-platform
  (because that's what Phase 0 mirrored from prod), so developers
  must remember to use `npm run start:app:local:ios` vs
  `start:app:local:android` — each fetches a different key, and
  picking the wrong one fails authorization with a blank tan map. A
  single local-only key in ripls-dev removes the per-platform friction
  without affecting prod (which keeps per-platform keys for proper
  application restrictions). Once provisioned, fold the two
  `start:app:local:{ios,android}` scripts back into a single
  `start:app:local:sim` that fetches `google-maps-api-key-local`.

#### Typed proximity semantics (revised after Phase 0 audit)

The Phase 0 parity audit surfaced that **proximity isn't a single
concept** — different call sites want different things, and Google's
API treats `locationBias` (soft) and `locationRestriction` (hard, but
not strictly hard) differently than Mapbox's single `proximity`
parameter does.

The original Phase 1 `Provider` interface signed up callers to a
single proximity coordinate:

```go
SearchPlaces(ctx, query string, proximity *models.Geolocation, limit int) ([]PlaceResult, error)
```

Phase 3 widens this to a typed proximity request, so each call site
declares its semantic intent:

```go
type ProximitySemantics int
const (
    ProximityNone   ProximitySemantics = iota
    ProximityHint               // soft bias; out-of-radius results allowed
    ProximityBound              // hard restriction + post-filter
)

type ProximityRequest struct {
    Coord     *models.Geolocation // nil for ProximityNone
    RadiusM   int                 // 0 → provider-default
    Semantics ProximitySemantics
}

type Provider interface {
    ForwardGeocode(ctx, *models.Address) (*models.Geolocation, error)
    SearchPlaces(ctx, query string, proximity ProximityRequest, limit int) ([]PlaceResult, error)
    ReverseGeocode(ctx, *models.Geolocation) (*PlaceResult, error)
    CheckHealth(ctx) ([]*health.Status, error)
}
```

**Per-call-site mapping** (revised 2026-05-27 — see "Source-based
proximity selection" below). The AI flow's original "Bound everywhere"
rule dropped named distant POIs ("Zilker Park" from a Boulder user)
because the 50 km circle filtered them out at the server. The new rule
routes Bound only when we have a high-confidence reference (event
coords parsed from JSON-LD); GPS and community-fallback coords drop to
Hint:

| Call site | Semantic | Rationale |
|---|---|---|
| `experience/location_extract.go` — AI-extracted segments | `ProximityHint` (when GPS supplied) | Bulk-paste candidates may include explicit named-but-distant POIs the user typed; dropping them is worse UX than admitting an out-of-area "park" → "Parker, CO" mis-resolve. |
| `experience/gen_ai*.go` — AI-generated `location_query` for events | Bound only with `event_coords` source (JSON-LD webpage event); Hint for `gps` / `location_id` | When the source is a webpage's structured event location, dropping out-of-radius matches is correct. When the source is the user's GPS or community fallback, soft-bias only. |
| `request/gen*.go` — AI-generated `location_query` for help requests | Same source-based rule as `experience/gen_ai*.go` | Help requests have no `event_coords` source, so this collapses to Hint when proximity is supplied. |
| `location_picker_modal` — viewport-aware autocomplete | `ProximityBound` from viewport center, larger radius | User is looking at a part of the map and typing. Out-of-viewport results are actively confusing UX. |
| Generic `location_autocomplete_field` with cached GPS | `ProximityHint` | User may legitimately type a global address ("1 Infinite Loop", "Bondi Beach"). Bias toward local but don't drop globally-popular matches. |
| `discover_screen` viewport search | `ProximityBound` to viewport | Same as location_picker. |
| `governance_view_model` region picker | `ProximityNone` (most flows) | The user is configuring a community that may be anywhere; no proximity context. |

**Source-based proximity selection** (2026-05-27). The
`fanout.ProximityForGeocoding` ladder returns a source label
(`"event_coords" | "gps" | "location_id" | "none"`) alongside the
chosen coordinate. `location.ProximityRequestFromSource` maps that
label to semantics: `event_coords → Bound`, `gps | location_id → Hint`,
anything else → `None`. The AI-flow callers (`experience/gen_ai.go`,
`experience/gen_ai_streaming.go`, `request/gen.go`,
`request/gen_streaming.go`) call the helper inline and pass the typed
request through `geocodePlaceQuery` instead of a bare `*Geolocation`.

**Implementations:**

- `MapboxClient.SearchPlaces` maps both `ProximityHint` and
  `ProximityBound` to the Mapbox `proximity` query parameter — Mapbox's
  bias is strong enough that the semantic distinction is mostly moot
  there, and we can post-filter for `ProximityBound` cases to match
  the strict-locality contract.
- `GoogleMapsClient.SearchPlaces`:
  - `ProximityHint` → `locationBias.circle`
  - `ProximityBound` → `locationRestriction.circle` **AND** server-
    side distance check on the resolved Place Details centroid. Drop
    results whose centroid is > 1.5× the requested radius (the audit
    showed Google's restriction is loose; the post-filter catches
    cases like "park" → Parker, CO at 70 km when 50 km was requested).
  - `ProximityNone` → no location parameter.

**Tests carry over.** The integration tests in
`server/services/experience/location_test.go` etc. specify product
behavior ("user in Boulder typing 'The Cup' resolves to The Cup
Boulder"), not API parameter values. Passing them on Google requires
the `ProximityBound` semantic + post-filter; passing them on Mapbox is
unchanged.

#### Implementation tasks

**Tasks:**
- [x] Widen `location.Provider.SearchPlaces` to take `ProximityRequest`.
      Update `*MapboxClient` to map both `Hint` and `Bound` semantics
      onto its existing `proximity` parameter; add the post-filter
      pass for `ProximityBound`. Done — `ProximityRequest` /
      `ProximitySemantics` live in `provider.go`; haversine
      post-filter in `geometry.go`; `MapboxClient.SearchPlaces` uses
      both. `DefaultBoundRadiusM` = 50 km;
      `BoundRadiusExpansionFactor` = 1.5×.
- [x] Update every existing call site (server-side) to pass an
      explicit `ProximityRequest` per the table above. Done — all
      three AI flows (`experience/location_extract`,
      `experience/gen_ai_fanout`, `request/gen_fanout`) construct a
      `ProximityBound` request when the caller supplied coordinates,
      `ProximityNone` otherwise. Default radius applies (no per-call-
      site override yet; revisit if dev validation surfaces tuning
      needs).
- [ ] Add `google_maps_flutter: ^X.Y.Z` to `app/pubspec.yaml` (pick
      latest stable). `google_maps_flutter_web` is bundled.
- [x] HTTP-direct (no Go SDK dependency) — chose to match Mapbox's
      hand-rolled client style; keeps the package's dependency graph
      flat and avoids the SDK's transitive auth/transport surface.
- [x] Implement `server/location/google.go` (`*GoogleMapsClient`)
      satisfying `Provider`. Uses **Places Autocomplete (New)** for
      search (POST with `locationBias` for `ProximityHint` /
      `locationRestriction` for `ProximityBound`), **Place Details
      (New)** to resolve coordinates per suggestion (with the
      `placeDetailsFieldMask` constant scoped to the cheap Basic SKU),
      **Geocoding API** for forward + reverse, and a Geocoding call
      against `1600 Amphitheatre Parkway` for `CheckHealth`. Returns
      `ExternalProvider="google_maps"` on `PlaceResult`. All logging
      goes through `logging.LoggerWithContext(ctx)` with the standard
      `external_service`, `operation`, `duration_ms`, `error` fields;
      the `external_service` value and the `health.Status.Backend`
      match (`google_maps`). Connect-handler wrapping via
      `connecterr.Internal` happens at call-site service layers (the
      client returns plain errors; CLAUDE.md gate is at the handler).
- [x] **[FROM AUDIT]** Filter Plus Codes from reverse-geocode results.
      `firstNonPlusCodeResult` + `isPlusCodeOnly` walks the results
      list and returns the first non-Plus-Code match; if every result
      is Plus-Code-only, returns no-result (covered by
      `TestGoogleReverseGeocode_PlusCodeOnly_Skipped` and the
      address-after-pluscode test).
- [x] **[FROM AUDIT]** Use the Geocoding API `result_type` filter.
      `ReverseGeocode` sets
      `result_type=street_address|route|premise|subpremise|locality`,
      which excludes establishment / point_of_interest matches at the
      API level. Integration test on Pearl Street confirms a real
      street address comes back (not the nearby Boulder Dushanbe Tea
      House).
- [ ] **[FROM AUDIT]** Plan server-side Place Details cache (keyed by
      placeId) so confirmed-saved places never re-pay the $5/1k
      Place Details rate. Same shape as the existing
      `protosql.GetByID` cache for other entities.
- [ ] **[FROM AUDIT]** Plan server-side reverse-geocode cache keyed by
      rounded coordinate (6 decimals).
- [ ] **[FROM AUDIT]** Add Google Maps Autocomplete session tokens
      from the client (UUID per session, threaded into both
      Autocomplete and Place Details calls). Drops per-session cost
      from N×($2.83 + $5.00) to 1×$5.00 within a session.
- [x] **[AUDIT]** Added `server/location/google_integration_test.go`
      with `//go:build integration`, mirroring
      `mapbox_integration_test.go`. Covers ForwardGeocode (Googleplex),
      SearchPlaces (no proximity / Hint Austin / Bound Boulder with
      30km radius), ReverseGeocode (Pearl Street), and CheckHealth
      (valid + invalid key). Verified end-to-end against ripls-dev key
      — the Bound post-filter and Plus-Code filter both behave as
      designed on live data.
- [x] **(Phase 0)** `--google-maps-api-key-server` flag + GSM secret +
      terraform IAM bindings + entrypoint plumbing. Done. The flag is
      compiled in but unused until `*GoogleMapsClient` exists; the
      Phase 3 implementation reads the value via the existing
      `cfg.googleMapsAPIKeyServer` field.
- [x] Add `--map-provider=mapbox|google` flag (default `mapbox`).
      Done — flag parses + validates at startup; main.go constructs
      both clients when both credentials are present (so each
      registers CheckHealth), then picks the active one per the
      flag. Refuses to start if `--map-provider=google` but
      `--google-maps-api-key-server` is empty (and vice versa).
      `MAP_PROVIDER` env var threads through `entrypoint.sh`.
      Dev terraform now sets `map_provider = "google"` (per user
      request, 2026-05-27) so dev exercises Google end-to-end ahead
      of the prod cutover; prod stays on `mapbox`.
- [ ] **Cloud NAT static IP + prod server-key IP restriction.**
      Deferred from Phase 0. Add `google_compute_router` +
      `google_compute_router_nat` + `google_compute_address` to
      `terraform/environments/prod/`, flip the `vpc_access.egress` on
      the Cloud Run service from `PRIVATE_RANGES_ONLY` to `ALL_TRAFFIC`,
      then apply `--allowed-ips=<static-ip>` to the prod server key.
      Phase 0 left this undone because Phase 3 hadn't yet started
      calling Google from the server.
- [ ] **Per-API quota caps.** Deferred from Phase 0 — set tight caps
      on prod (and somewhat looser on dev) after Phase 3's local
      smoke tests give us a feel for real per-call volume. The
      provisioning runbook has the `gcloud alpha services quota
      update` form documented; values to be decided at this point.
- [x] Implement `app/lib/services/google_location_search_service.dart`
      satisfying `LocationSearchService`. Done — uses the Geocoding API
      for searchAddresses / searchGeographicAreas / reverseGeocode (one
      call per method, matching Mapbox's shape and sidestepping the
      Autocomplete + Place Details N+1 cost trap). Plus-Code-only
      results are dropped, `result_type` filter is applied to
      searchGeographicAreas (admin-area types) and reverseGeocode
      (address-like types). 11 unit tests via `package:http/testing`
      MockClient. **Follow-up** (separate task in this phase):
      session-token-backed Autocomplete + Place Details for cheaper
      per-keystroke pricing — needs a small interface widening
      (the service needs to know about the picker session lifecycle
      so the same token threads through autocomplete and details).
- [x] Add `Environment.mapProvider` + switch
      `locationSearchServiceProvider` on it. Done — `Environment.mapProvider`
      reads `--dart-define=MAP_PROVIDER=...` (default `mapbox`); the
      provider returns `GoogleLocationSearchService` for `google`,
      `MapboxLocationService` otherwise. `env.dev.json` and
      `env.local.json` now set `MAP_PROVIDER=google`; `env.prod.json`
      explicit `mapbox` until Phase 5 cutover.
- [x] Add `Environment.googleMapsApiKey`
      (`String.fromEnvironment('GOOGLE_MAPS_API_KEY')`). Done.
      `package.json` build:web:{local,dev,prod} scripts now fetch
      `google-maps-api-key-web` from the matching GSM project and
      thread it via `--dart-define`. iOS/Android lanes in
      `app/fastlane/Fastfile` will need the same threading in Phase 4
      when the map widget SDK init lands.
- [ ] Configure Google Maps SDK init per platform:
      - Android: template
        `app/android/app/src/main/AndroidManifest.xml` to read
        `manifestPlaceholders['GOOGLE_MAPS_API_KEY']` from a Gradle
        property; thread the property through `release_app.yaml`.
      - iOS: call `GMSServices.provideAPIKey(...)` in
        `AppDelegate.swift`, reading from Info.plist; thread Info.plist
        value through `release_app.yaml`.
      - Web: add `<script src="https://maps.googleapis.com/maps/api/js?key=..."></script>`
        to `app/web/index.html`, templated in the web build step of
        `test_web_smoke.yaml` / production web release workflow.
- [ ] Run the parity audit's representative queries against the new
      Google implementations end-to-end in dev; compare results to
      what Phase 0 saw raw. They should match within rounding.
**Validates:** Google can answer every request the Mapbox client can,
end-to-end on all three platforms, while production stays on Mapbox.

### Phase 4: Migrate map widget call sites to `google_maps_flutter`
**Goal:** every map widget in the app uses `google_maps_flutter`.
Mapbox `MapWidget` is gone from the UI tree.

**Foundation landed (separate commit):** `google_maps_flutter ^2.10.0` in
`pubspec.yaml`; per-platform SDK init (Android manifest meta-data with
manifestPlaceholders + Gradle property; iOS `GMSServices.provideAPIKey`
in AppDelegate reading from Info.plist via xcconfig; web index.html
template + build-time sed substitution); dark map style JSON at
`app/assets/map_styles/google_dark.json`. The SDK init is a no-op
until a `GoogleMap` widget mounts, so the existing Mapbox widget code
continues to work unchanged. Fastlane Android/iOS lanes need
`google-maps-api-key-{android,ios}` GSM-fetch wiring before prod
release builds can flip to MAP_PROVIDER=google in env.prod.json
(deferred until Phase 5 cutover).
**Tasks:**
- [ ] Rewrite `app/lib/core/utils/map_helper.dart`:
      `MapboxMap` → `GoogleMapController`, `CameraOptions` →
      `CameraPosition`, `Point`/`Position` → `LatLng`,
      `MbxEdgeInsets` → padding, `flyTo` → `animateCamera`,
      `LocationComponentSettings` → `myLocationEnabled` /
      `myLocationButtonEnabled` flags on `GoogleMap`.
- [ ] Rewrite `app/lib/core/utils/discover_map_helper.dart`:
      `PointAnnotationManager` → `Marker` set passed to `GoogleMap`,
      `CircleAnnotationManager` → `Circle` set, tap events →
      per-marker `onTap` callbacks, style switching → `mapType` +
      style JSON.
- [ ] Rewrite `app/lib/presentation/widgets/ripple_map_marker.dart`:
      `CircleAnnotation` × 3 → three `Circle` instances added to the
      map's `circles` set.
- [ ] Rewrite `app/lib/presentation/screens/discover/discover_screen.dart`:
      `MapWidget(...)` → `GoogleMap(...)`. Drop the
      `kIsWeb`-conditional placeholder since web now works.
- [ ] Rewrite `app/lib/presentation/widgets/discover/discover_filter_sheet.dart`:
      `MapboxStyles.{STANDARD,OUTDOORS,SATELLITE,DARK}` → `MapType` enum
      values + a dark-mode `mapStyle` JSON asset.
- [ ] Rewrite `app/lib/presentation/widgets/location/location_modal_widgets.dart`:
      same pattern as discover.
- [ ] Author `app/assets/map_styles/google_dark.json` (dark map JSON
      style) and reference it from `discover_filter_sheet.dart`.
- [ ] Replace the Mapbox init in `app/lib/main.dart`
      (`MapboxOptions.setAccessToken(...)`) with the Google Maps
      no-op (key comes from manifest/Info.plist/script tag).
- [ ] Delete `app/lib/presentation/widgets/map/web_map_placeholder.dart`
      and its call sites (web is no longer special-cased).
- [ ] **[AUDIT]** Any new tappable map UI (style toggle, marker
      tap targets at the Flutter level, etc.) must compose from the
      `widgets/accessibility/` primitives — `Tappable`, `IconAction`,
      `Toggle` — with `semanticsLabel` pulled from `context.l10n.*`.
      The `scripts/check_a11y.js` allowlist is empty; CI rejects new
      raw `GestureDetector` / `InkWell` / `IconButton` outside the
      primitives. Existing labels (`discoverFilterMapDefault`,
      `Outdoor`, `Satellite`, `Dark`) stay; if any new strings are
      needed, add to both `app_en.arb` and `app_es.arb` to satisfy
      `npm run lint:dart:l10n-parity`.
- [ ] Run the app on iOS simulator, Android emulator, and web. Smoke
      test discover, location picker, location modal. Verify markers,
      camera, taps, style switching all work.
**Validates:** end-to-end map UX works on all three platforms with
Google. Closes #2156's underlying problem. Production is still on
Mapbox for the server-side calls — server cutover is Phase 5.

### Phase 5: Cutover and observability
**Goal:** Google is the production default; Mapbox is still wired but
unused.

**Tasks:**

- [ ] Set `--map-provider=google` on the dev Cloud Run service. Watch
      `external_service="google"` log volume and error rate.
- [ ] Set `MAP_PROVIDER=google` for the next dev app build (TestFlight,
      Android internal). Smoke test for ~1 week.
- [ ] Flip prod server `--map-provider=google`. Monitor health checks,
      `mapboxClient.CheckHealth` should go quiet; `googleMapsClient.CheckHealth`
      should be green.
- [ ] Flip prod client `MAP_PROVIDER=google` in `release_app.yaml`. Ship
      the next prod app build.
- [ ] Watch error logs and the budget alert for one full week. If
      anything regresses, flip back via the flag (no redeploy needed
      server-side; client redeploy with `MAP_PROVIDER=mapbox` to roll
      back).
**Validates:** Google handles real production traffic without quality
or cost regression.

### Phase 6: Remove Mapbox
**Goal:** the codebase has one map provider. Mapbox is gone.

**Tasks:**

- [ ] Delete `server/location/mapbox.go` and its tests
      (`mapbox_test.go`, `mapbox_integration_test.go`).
- [ ] Delete `app/lib/services/mapbox_location_service.dart`.
- [ ] Remove `mapbox_maps_flutter` from `app/pubspec.yaml`.
- [ ] Remove `--mapbox-access-token` flag, `mapboxAccessToken` config,
      health check registration from `server/main.go`.
- [ ] Remove `MAPBOX_ACCESS_TOKEN` from `app/lib/core/config/environment.dart`,
      `app/lib/main.dart`, `release_app.yaml`, `test_web_smoke.yaml`,
      `build_and_deploy_container.yaml`.
- [ ] Remove the `--map-provider` flag (single provider now). Drop
      `Environment.mapProvider`.
- [ ] Remove the `mapbox-access-token` GSM secret in dev and prod
      (Terraform: drop from `terraform/environments/{dev,prod}/main.tf`
      and `terraform/modules/containers/variables.tf`). Run `terraform
      apply` per environment.
- [ ] Sweep proto comments per CLAUDE.md:
      `location_service.proto:114,118,122,374` lose the word "Mapbox".
      Provider-example fields (`(e.g., "mapbox", "google")`) — leave
      since they're a real enumeration of valid values.
- [ ] Sweep `docs/` and READMEs: rewrite "Mapbox" → "Google Maps" in
      `docs/secrets.md`, `docs/secret-rotation.md`, `docs/server/observability.md`,
      `docs/ai/map.md`, `docs/ai/filters.md`,
      `server/location/README.md`, `app/lib/data/repositories/README.md`
      (if it exists). Vendor names are allowed in these per CLAUDE.md.
- [ ] Close #2156 with a comment pointing at the merged PR(s).
- [ ] Update `docs/ai/map.md`'s "current architecture" section to
      reflect Google.
**Validates:** `grep -rn mapbox` returns only historical
artifacts (commit messages, eval docs); the running code knows nothing
about Mapbox.

## Related Issues

- **#2156** — Web map rendering broken because `mapbox_maps_flutter` has
  no web implementation. Subsumed by this issue; closes when Phase 4
  ships (widget call sites work on web).
- **#2157** — Full web app rollout. Currently blocked on #2156; unblocks
  the moment Phase 4 ships.
- **#2027** — Mapbox `onMapCreated` race condition. Becomes obsolete
  once the widget migrates to `google_maps_flutter` (Phase 4); close it
  out as part of Phase 6.
- **#2102** — Removed the cross-community leaderboard (already merged).
  The reason the issue body's call-site list is shorter than it claims.
