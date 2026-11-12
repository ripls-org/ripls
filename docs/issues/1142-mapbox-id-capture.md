# Issue #1142: Mapbox API: capture mapbox_id, context hierarchy, and other unused response fields

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1142
**Filed:** 2026-04-15 | **Last activity:** 2026-04-16
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

We call three Mapbox APIs (Search Box v1, Geocoding v6 forward, Geocoding v6
reverse) but discard significant data from the responses. The most severe
consequence is that `mapbox_feature_id` — the canonical identifier used for
location deduplication — is never captured. This breaks location
deduplication entirely: every location save hits the 50-meter coordinate
proximity fallback, causing genuinely different places in urban areas to
merge into one record.

**Empirical evidence:** Queried prod and dev databases on 2026-04-15.
**0% of location records have a `mapbox_feature_id`.** Every location stored
since the column was added in PR #532 (January 2026) is dedup-blind.

The dedup code itself is correctly designed (`deduplication.go` tries
`mapbox_feature_id` first, falls back to 50m radius). The bug is an
**incomplete implementation** — the server-side dedup primary path was
wired up, but Mapbox response parsing and the client were never updated to
populate the ID.

### Additional gaps

Beyond `mapbox_feature_id`:
- Context hierarchy (neighborhood, county, administrative area) is parsed
  on the client but discarded by the server
- `Locality`, `RegionCode`, `PostalCode` are always empty in
  `GeocodedLocation` returned by `GenExperience`
- Forward geocoding discards everything except coordinates
- Synthetic confidence scores (position-based) instead of real Mapbox
  relevance ranking
- POI categories, routable_point, external_ids all captured by nobody

## Context from Comments

Three comments (all self-authored during investigation) document:

1. **Why dedup matters:** Locations are first-class entities with stable
   UUIDs referenced by experiences, gear, requests, and user locations.
   "Central Park" chosen by 1000 users should be one record, not 1000.
2. **Empirical confirmation:** 0% of prod/dev locations have
   `mapbox_feature_id`.
3. **User-facing impact:** The 50m fallback silently merges different
   places in urban areas. User B's experience at a cafe 30m from a jazz
   club gets attached to the jazz club's Location record, showing the
   wrong name on the experience. Also privacy-sensitive — home addresses
   custom-named ("Brian's House") get inherited by any experience created
   within 50m.

No external commenter input.

## Current Chain Break

| Step | File | Problem |
|------|------|---------|
| Mapbox response parsed | `server/location/mapbox.go:168-183` | `searchBoxResponse` struct doesn't capture `mapbox_id` from the JSON |
| PlaceResult returned | `server/location/mapbox.go:156-165` | `PlaceResult` struct has no `ID` field |
| Server geocodes location | `server/services/experience/gen_ai.go:259-267` | Builds `GeocodedLocation` without any feature ID |
| API proto | `proto/ripls/api/location.proto:12-20` | `GeocodedLocation` message has no `mapbox_feature_id` field |
| Client parses results | `app/lib/services/mapbox_location_service.dart` | `LocationResult` class has no `mapboxId` field |
| Client saves location | `app/lib/presentation/viewmodels/location_picker_view_model.dart:85-108` | `saveLocation()` call doesn't pass `mapboxFeatureId` |

**Already in place (but using Mapbox-specific names — will be renamed in
Phase 2):**
- `models.Location.mapbox_feature_id` (field 7) — storage schema
- `api.SaveLocationRequest.mapbox_feature_id` (field 11) — RPC input
- `location_repository.dart saveLocation(mapboxFeatureId:)` — parameter
- `server/services/location/deduplication.go findLocationByMapboxFeatureID` — query function

Since **zero rows are populated**, these fields will be renamed to
provider-agnostic names (`external_place_id` + `external_place_provider`)
with no data migration required. See Recommendation below.

## Potential Solutions

### Option A: Fix the primary gap (mapbox_id only)
- **Approach:** Add `mapbox_id` to `searchBoxResponse`, add `ID` to
  `PlaceResult`. Add `mapbox_feature_id` to `GeocodedLocation` API proto.
  Pass it through `gen_ai.go` when building the response. On the client,
  add `mapboxId` to `LocationResult`, extract it from the Search Box API
  JSON response, and pass it to `saveLocation()`. Reverse geocoding adds
  the ID to its response too.
- **Pros:** Fixes the highest-impact bug (dedup broken for all users).
  Minimal surface area — narrow, testable change.
- **Cons:** Doesn't address the other gaps (empty locality/region_code,
  synthetic confidence, etc.). Those remain as follow-ups.
- **Effort:** Small-medium.

### Option B: Fix primary gap + context hierarchy
- **Approach:** Option A plus parse the `context` map in `SearchPlaces`,
  extract `neighborhood`, `county`, `administrative_area`, `locality`,
  `region_code`, `postal_code`, and populate `PlaceResult` + `GeocodedLocation`.
  Client already does similar parsing (Dart), so server just mirrors it.
- **Pros:** Fixes both the dedup bug and the empty-display-fields bug in
  one pass. `GeocodedLocation.Locality`/`RegionCode`/`PostalCode` finally
  get populated, which makes location previews and confirmation screens
  work correctly.
- **Cons:** Larger change, more test surface area. Some extra care needed
  for different context formats Mapbox returns for different feature
  types.
- **Effort:** Medium.

### Option C: Fix everything (primary + context + forward geocode + minor gaps)
- **Approach:** Option B plus return full place data from `ForwardGeocode`
  (not just coordinates), use Mapbox's real relevance ranking instead of
  synthetic confidence, capture `poi_category` / `routable_point` /
  `external_ids`.
- **Pros:** One clean pass, no follow-up issues to remember.
- **Cons:** Scope creep. `ForwardGeocode` is only used by structured
  address geocoding (minor path). Real relevance scores vs synthetic
  isn't currently causing user-visible issues. POI categories have no
  consumer yet.
- **Effort:** Medium-large.

## Recommendation

**Option B, with provider-agnostic naming from day one.**

The dedup bug is the load-bearing fix, but the empty
`Locality`/`RegionCode`/`PostalCode` fields in `GeocodedLocation` are a
known documented bug (see comments in `gen_ai.go:263-265`) that blocks
proper location display. They're solved by the same `context` parsing
work, so bundling them avoids a second round-trip through the same code.

Options that add fields no consumer uses (Option C's extras) should be
deferred to separate issues when a consumer actually materializes.

### Provider-agnostic naming (important design decision)

Since **zero locations currently have a persisted Mapbox ID**, we have a
free opportunity to design the field for provider flexibility. If we ever
switch geocoding providers (Google Maps, OpenStreetMap, HERE, etc.) or
support multiple providers, Mapbox-specific field names become a
liability requiring a migration.

**Proposed schema change:** Rather than using `mapbox_feature_id`, use
two fields:

```proto
// On models.Location, api.GeocodedLocation, api.SaveLocationRequest:
optional string external_place_id = 7;        // opaque provider-specific ID
optional string external_place_provider = 8;  // "mapbox", "google", "osm", etc.
```

This keeps queries clean (`WHERE external_place_provider = 'mapbox'`
to find all Mapbox-sourced locations for re-geocoding) without tying
the schema to a vendor name. A two-field approach is preferred over a
prefixed single field (`"mapbox:poi.12345"`) because it enforces
consistency via schema rather than string convention, and provider-
filtered queries don't need `LIKE` patterns.

**Renames through the stack:**
- `models.Location.mapbox_feature_id` → `external_place_id` + `external_place_provider`
- `api.SaveLocationRequest.mapbox_feature_id` → `external_place_id` + `external_place_provider`
- `api.GeocodedLocation` adds `external_place_id` + `external_place_provider`
- `server/location/PlaceResult.ID` → `ExternalID` (and add `ExternalProvider`)
- `deduplication.go findLocationByMapboxFeatureID` → `findLocationByExternalID(id, provider)`
- `location_repository.dart saveLocation(mapboxFeatureId:)` → `saveLocation(externalPlaceId:, externalPlaceProvider:)`
- `LocationResult.mapboxId` (proposed) → `externalPlaceId` + `externalPlaceProvider`

**Implementation writes `provider = "mapbox"` today.** All Mapbox paths
populate `external_place_provider = "mapbox"`. A future provider just
sets it to `"google"` or `"osm"`. No migration needed when switching —
just start writing new values alongside the existing ones.

**Storage migration cost:** Because zero rows have `mapbox_feature_id`
populated, we can drop the old column entirely or leave it as a no-op.
The proto-SQL storage layer will auto-create the new columns on startup.

**What about the internal abstraction?** Consider wrapping `MapboxClient`
behind a `location.PlaceProvider` interface in a follow-up issue (matches
the existing `ai.Provider` pattern in `server/ai/provider.go`). Not in
scope here — this plan is about data model correctness; swapping the
implementation is a separate concern.

### Backfill question

**Should existing locations be backfilled with `mapbox_feature_id`?** No.
Reverse-geocoding existing coordinates to find Mapbox IDs is lossy (the
nearest POI to a coordinate isn't necessarily the original place). Accept
that existing locations will continue using the 50m fallback; new
locations going forward will dedup correctly. Document this in a release
note. The dedup broken-ness is already baked into history; fixing forward
is the right tradeoff.

A data-cleanup script could be a future issue if the merged-location
situation proves problematic.

## Open Questions

- **Should the client send `mapbox_feature_id` when it matches a result
  from an older search that doesn't have one?** — Recommendation: Yes,
  send empty string. Server will fall back to proximity. This is the
  same as today's behavior, but new saves that do have an ID get proper
  dedup.

- **Does `GenExperience`'s JSON-LD path (from #1134) need a
  `mapbox_feature_id`?** — Recommendation: Yes. The webpage flow geocodes
  via `SearchPlaces` just like text/media paths, so it naturally gets an
  ID once `PlaceResult` carries one. No extra work needed — just verify
  in a test.

- **Reverse geocoding — should we also capture mapbox_id there?** —
  Recommendation: Yes. The "current location" chip in the location picker
  calls reverse geocoding. Without mapbox_id there too, picking "current
  location" still hits the 50m fallback. Same struct change applies.

- **What about the existing 0% of prod locations without mapbox_id?** —
  Recommendation: Leave as-is (see Backfill question above). Document in
  release notes that new locations dedup correctly; historical locations
  may contain merged places.

- **Should we remove the 50m fallback once all new saves have mapbox_id?**
  — Recommendation: No, keep it. Covers cases where the user entered a
  custom name without Mapbox lookup (e.g., typed a location manually
  without picking from autocomplete). The fallback is a correctness
  safety net, just not the primary path it's been forced to be.

## Implementation Plan

### Phase 1: Server-side — capture external ID and context ✅ COMPLETE

**Goal:** Server-side Mapbox response parsing captures the fields needed
to support dedup and populate display data. Field naming is
provider-agnostic from day one.

**Tasks:**
- [x] Add `ExternalID` and `ExternalProvider` fields to `PlaceResult`
  struct (`server/location/mapbox.go`). In Mapbox paths, always set
  `ExternalProvider = "mapbox"`.
- [x] Add `MapboxID string` (JSON tag `mapbox_id`) to the
  `searchBoxResponse.Features[].Properties` struct (this is the raw
  vendor field — internal to the Mapbox adapter)
- [x] Populate `PlaceResult.ExternalID` from `properties.mapbox_id` in
  `SearchPlaces`
- [x] Parse `Properties.Context` in `SearchPlaces` — extract `place`
  (locality), `region` + `region_code`, `postcode`, `neighborhood`,
  `county` sub-fields
- [x] Add `Locality`, `RegionCode`, `PostalCode`, `Neighborhood`,
  `County`, `AdministrativeArea` fields to `PlaceResult`
- [x] Do the same capture for `ReverseGeocode` (change its return type
  to a full `PlaceResult` or similar struct)
- [x] Add unit tests with fixture JSON covering POI, Address, Place, and
  Locality feature types
- [x] `npm run build` passes
- [x] **Manually verified** via temporary debug log: real Mapbox calls
  return populated `external_id`, `locality`, `region_code`,
  `postal_code`, `administrative_area` for a POI match

**Validates:** Server can extract the data but doesn't yet expose it
via API — all existing clients continue working unchanged.

### Phase 2: Proto rename + expose via API + GenExperience / GenRequest / GenGear ✅ COMPLETE

**Goal:** Rename the storage field to be provider-agnostic, add it to
the response API, and populate it from `GenExperience` / `GenRequest` /
`GenGear`.

**Tasks:**
- [x] Rename `models.Location.mapbox_feature_id` (field 7) →
  `external_place_id` and add `external_place_provider` (new field 8)
  in `proto/ripls/models/location.proto`
- [x] Rename `api.SaveLocationRequest.mapbox_feature_id` (field 11) →
  `external_place_id` and add `external_place_provider` (new field 12)
  in `proto/ripls/api/location_service.proto`
- [x] Add `external_place_id` (field 8) and `external_place_provider`
  (field 9) to `GeocodedLocation` in `proto/ripls/api/location.proto`
  (also added neighborhood/county/administrative_area, fields 10-12)
- [x] Run `npm run generate`
- [x] Rename `deduplication.go findLocationByMapboxFeatureID` →
  `findLocationByExternalPlaceID(ctx, externalID, externalProvider)`
  and update the SQL query to match on both fields via `QueryByFields`
- [x] Update `places.go SaveLocation` to pass both fields through
- [x] Update `gen_ai.go` (experience) to populate
  `GeocodedLocation.ExternalPlaceId`, `ExternalPlaceProvider`,
  `Locality`, `RegionCode`, `PostalCode`, `Neighborhood`, `County`,
  `AdministrativeArea` from the enriched `PlaceResult`
- [x] Same change in `server/services/gear/gen_ai.go`
- [x] Same change in `server/services/request/gen.go`
- [x] Remove the `"Not available in PlaceResult"` comments at
  `gen_ai.go:263-265`
- [x] Update existing server tests that reference `mapbox_feature_id`
- [x] Unit tests verify `GeocodedLocation.ExternalPlaceId` flows through
  (tests: `TestService_GenExperience_LocationExtraction`)
- [x] Added structured log field `dedup_path=external_id|proximity|new`
  for monitoring in `findOrCreateLocation`

**Validates:** Server emits the data through the API. Client can ignore
it until Phase 3, so rollout stays backward-compatible.

### Phase 3: Client — extract from Mapbox response, forward to SaveLocation ✅ COMPLETE

**Goal:** Client captures external place ID from Search Box API responses
and sends it back to the server when saving locations. Field names on
the client use the provider-agnostic scheme (`externalPlaceId` +
`externalPlaceProvider`).

**Tasks:**
- [x] Add `externalPlaceId` and `externalPlaceProvider` fields to
  `LocationResult` in `app/lib/services/mapbox_location_service.dart`.
- [x] Add `mapboxProvider = 'mapbox'` constant to mirror server
  `providerMapbox` — avoids hardcoded strings across call sites.
- [x] Extract `properties.mapbox_id` in `searchAddresses()` and
  `searchGeographicAreas()` and store as `externalPlaceId`
- [x] Also capture in `reverseGeocode()` response
- [x] Rename `location_repository.dart saveLocation(mapboxFeatureId:)`
  → `saveLocation(externalPlaceId:, externalPlaceProvider:)` and
  forward to the service
- [x] Rename in `location_service.dart`
- [x] Update `location_picker_view_model.dart saveLocation()` call to
  pass both fields
- [x] Update `gen_experience_view_model.dart` — when building
  `SaveLocationRequest` from cached `GeocodedLocation`, pass both
  fields (plus neighborhood, county, administrative_area which were
  also previously dropped)
- [x] Same for `gen_request_view_model.dart` and `gen_gear_view_model.dart`
- [x] All 3441 Flutter tests pass after mock regeneration
- [x] **Manual end-to-end verified** on local dev: created experience
  with location "Longmont" → saved with
  `external_place_id=dXJuOm1ieHBsYzpDMnFJN0E`,
  `external_place_provider=mapbox`. Created second experience at same
  place → **single Location row** (dedup via external_place_id
  confirmed working, log shows `dedup_path=external_id`).

**Validates:** End-to-end flow. New locations going forward have
`external_place_id` populated. Dedup works correctly for new saves.

### Phase 3.5: Storage cleanup (optional, post-Phase 3 verification) ✅ COMPLETE

**Goal:** Drop the orphaned `mapbox_feature_id` column from the `location`
table once the new fields are populated and verified.

**Background:** The proto-SQL storage layer auto-adds new columns
(`external_place_id`, `external_place_provider`) via
`ALTER TABLE ADD COLUMN` on startup but does not drop removed columns.
After the Phase 2 rename, `mapbox_feature_id` is orphaned — no code
writes or reads it. It's harmless (empty column, no index, ~1 byte per
row for NULL bitmap) but can be dropped cleanly since it was always NULL
in prod and dev (verified on 2026-04-15, 0% populated).

**Tasks:**
- [x] After Phase 4 monitoring confirms new locations are saving with
  `external_place_id` populated, run on dev:
  ```sql
  ALTER TABLE location DROP COLUMN mapbox_feature_id;
  ```
- [x] Verify server didn't re-add the column (confirmed via
  `information_schema.columns` query — 0 rows for `mapbox_feature_id`,
  2 rows for `external_place_id` + `external_place_provider`)
- [x] Run the same on prod
- [x] Document in issue comment that cleanup is complete

**Skip conditions:** If prod ever gained rows with populated
`mapbox_feature_id` (e.g., via a hotfix backport), pause and decide
whether to backfill `external_place_id` first.

**Pre-flight check gotcha:** Use `mapbox_feature_id != ''`, not
`mapbox_feature_id IS NOT NULL`. Proto-SQL storage writes empty strings
for unset optional scalar fields, so `IS NOT NULL` returns every row
(including 152 empty-string "populated" rows on dev on 2026-04-16 that
caused a false alarm). The correct check:
```sql
SELECT COUNT(*) FILTER (WHERE mapbox_feature_id != '') FROM location;
```
Should be 0 before drop.

**Also required before prod drop:** Phase 4 verification that new
locations saved via the deployed Phase 3 client actually populate
`external_place_id` (not just write empty strings like the old column
did). Without this, dropping the old column on prod leaves no fallback
if Phase 3 has a bug.

**Validates:** Schema matches proto cleanly; no orphan columns lingering.

### Phase 4: Verification and monitoring ✅ COMPLETE (initial verification)

**Goal:** Confirm the fix works in the wild and dedup hits the primary
path.

**Tasks:**
- [x] Deploy to dev
- [x] Create an experience and verify external_place_id + provider are
  populated on the resulting Location row (verified 2026-04-16:
  "Juniper Goods" saved with `external_place_id = dXJuOm1ieHBvaTo...`
  and `external_place_provider = mapbox`)
- [x] Add structured log line in `findOrCreateLocation` distinguishing
  "found via external_id" vs "found via proximity" vs "created new"
  (shipped in Phase 2 as `dedup_path` field)
- [x] Deploy to prod
- [ ] Query prod a week later: confirm new locations have
  `external_place_id` populated (with `external_place_provider = 'mapbox'`),
  and the ratio of proximity-fallback hits is low.
- [ ] Additional manual dedup tests (nearby-but-different places,
  reusing same place) — defer until real prod usage accumulates.

**Validates:** Bug is fixed in the wild, monitoring confirms the primary
path is actually being hit.

## Audit Notes

1. **[AUDIT] Client architecture compliance.** The plan updates
   `location_picker_view_model.dart` to pass `mapboxFeatureId` to
   `_locationRepository.saveLocation()`, which matches the client
   architecture: ViewModels → Repositories → Services (never skip a
   layer). The repository layer already accepts `mapboxFeatureId` and
   handles cache invalidation in `saveLocation()`. No changes needed to
   repository or service signatures — they're already wired through.

2. **[AUDIT] No caching key changes needed.** The `LocationRepository`
   cache namespace is `'location'` with `location:{id}` keys. Adding
   `mapbox_feature_id` to the saved proto doesn't change cache keys or
   invalidation logic — the existing `invalidate*()` pattern handles
   the new field transparently. Verified in
   `app/lib/data/repositories/location_repository.dart`.

3. **[AUDIT] No i18n impact.** No user-visible strings are added. All
   changes are field plumbing. No ARB updates needed.

4. **[AUDIT] Server architecture — no new service dependencies.** All
   server changes stay within the `server/location` shared library
   (Pattern 1 in architecture.md — shared library, not service-to-
   service). `GenExperience`, `GenRequest`, `GenGear` already depend
   on `MapboxClient`; they just read additional fields from the same
   `PlaceResult` struct.

5. **[AUDIT] Proto conventions — single place to define, flowed to many
   RPCs.** `GeocodedLocation` is a shared API message used by
   `GenExperienceResponse`, `GenRequestResponse`, and
   `GenGearResponse`. Adding `mapbox_feature_id` to `GeocodedLocation`
   propagates to all three RPCs in one change. This is the correct use
   of shared types in the API layer (they're field-level shared data,
   not request/response reuse — which CLAUDE.md prohibits).

6. **[AUDIT] Backward compatibility of proto change.** Adding
   `string mapbox_feature_id = 8` to `GeocodedLocation` is a
   backward-compatible proto3 change (new optional field; old clients
   ignore it; the server treats missing values as empty string). No
   migration needed. The storage layer already has the column (added
   in PR #532).

7. **[AUDIT] Ordering of phases matters for rollout.** Phase 1 and
   Phase 2 must ship before Phase 3 client work is useful. However,
   Phase 2 is safe to ship before Phase 3 because old clients just
   ignore the new `mapbox_feature_id` field in `GeocodedLocation`.
   This means you can ship server first, verify the response includes
   the ID via logs, then ship the client. Standard backward-compatible
   rollout.

8. **[AUDIT] Dedup fallback should stay.** The plan correctly keeps
   the 50m proximity fallback in `deduplication.go`. This is the right
   call — users can create a Location without going through Mapbox
   autocomplete (e.g., typing a free-form address), and those still
   need dedup via coordinates. The change is that the fallback becomes
   actually-a-fallback rather than the only path.

9. **[AUDIT] Phase 4 monitoring is underspecified.** The plan
   recommends logging "found via mapbox_id vs proximity vs created
   new" — this should be a structured log field with a consistent
   name (e.g., `dedup_path=mapbox_id|proximity|new`) so it can be
   queried in Cloud Logging / Metabase. Per CLAUDE.md, use
   `logging.LoggerWithContext(ctx)` and standard field names.

## Related Issues

- #1134 — Location search resolves wrong place from JSON-LD structured
  data. Same underlying code path — fix here compounds with the JSON-LD
  override to give the best-possible geocoding for event URLs.
- #1143 — Location autocomplete: proximity bias not wired up for GPS
  fallback. Independent; could be done in parallel.
- #1144 — Document proto API/models conventions. This issue is an
  example of the class of bug that a sync checklist would catch.
