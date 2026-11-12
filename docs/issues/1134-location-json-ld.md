# Issue #1134: Location search resolves wrong place from JSON-LD structured data

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1134
**Filed:** 2026-04-15 | **Last activity:** 2026-04-15
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

When creating an experience from an Eventbrite URL, the location resolves to
"Boulder County Jail" instead of "Boulder Airport" despite the page's JSON-LD
markup containing the correct location name and street address (`3393 Airport
Rd, Boulder, CO 80302`).

The root cause is that JSON-LD structured location data is only used as a
**fallback** when the LLM fails to extract any location. Since the LLM
usually does extract a `location_query` from the page body text, the
structured data is ignored — even though it's more reliable. The LLM's
extracted query may differ from the structured data, and Mapbox then
geocodes it to the wrong POI.

This is the same pattern as the time parsing bug (#990): structured data
should override LLM extraction, not be a fallback.

## Context from Comments

No comments. The issue body already contains a thorough root cause analysis
from the investigation that filed it.

## Current Location Flow

The experience creation location pipeline:

1. **LLM extracts `location_query`** — a free-text string (e.g., "Boulder
   Airport" or "the airport in Boulder")
2. **Mapbox Search Box API v1** geocodes the string against POI, address,
   place, and locality databases
3. **First result auto-selected** — highest-relevance match used with no
   confidence threshold or user confirmation
4. **JSON-LD override (current, broken)** — only fills `LocationQuery` if
   the LLM returned empty:
   ```go
   if ev.Location != "" && aiResponse.LocationQuery == "" {
       aiResponse.LocationQuery = ev.Location
   }
   ```

Key observations:
- The Eventbrite JSON-LD contains both a place name (`Boulder Airport`) and
  a street address (`3393 Airport Rd, Boulder, CO 80302`). The extraction
  function combines them: `"Boulder Airport, 3393 Airport Rd, Boulder, CO
  80302, Boulder, CO"`.
- Some JSON-LD includes geo coordinates (Luma, AllEvents) but the
  `StructuredEventData` struct doesn't capture them.
- Geo coordinates from JSON-LD could be passed as proximity bias to Mapbox
  for even more accurate geocoding.
- `GeocodedLocation` fields `Locality`, `RegionCode`, `PostalCode` are left
  empty in `gen_ai.go` even though Mapbox returns context data. This is a
  separate minor issue.

## Potential Solutions

### Option A: Override LocationQuery with structured data (one-liner)
- **Approach:** Remove the `&& aiResponse.LocationQuery == ""` condition so
  structured JSON-LD location always overrides LLM extraction. Same pattern
  used for the time override in PR #1132.
- **Pros:** Minimal change, directly fixes the bug, proven pattern.
- **Cons:** Doesn't use geo coordinates from JSON-LD even when available.
  Doesn't improve geocoding for non-JSON-LD pages.
- **Effort:** Small.

### Option B: Override + pass geo coordinates as proximity
- **Approach:** Option A plus: extract `geo.latitude`/`geo.longitude` from
  JSON-LD into `StructuredEventData`, and when available, pass them as
  proximity bias to the Mapbox `SearchPlaces` call. This gives Mapbox both
  the location name/address AND the coordinates.
- **Pros:** Near-perfect geocoding for structured pages. Even if the name
  is ambiguous, the coordinates disambiguate.
- **Cons:** Slightly more work (struct change, extraction code, passing
  proximity). For pages with coordinates, the Mapbox search is almost
  redundant — we already have the location.
- **Effort:** Small-medium.

### Option C: Skip Mapbox entirely when JSON-LD has coordinates
- **Approach:** When JSON-LD provides both a location name and geo
  coordinates, build the `GeocodedLocation` directly from the structured
  data without calling Mapbox at all. Use the Place name as the display
  name, the coordinates as lat/lng, and the PostalAddress fields for the
  address. Only fall back to Mapbox when coordinates are missing.
- **Pros:** Eliminates the Mapbox call entirely for structured pages —
  faster, cheaper, and 100% accurate. No chance of Mapbox mismatching.
- **Cons:** Doesn't get Mapbox's `feature_id` for location deduplication.
  Address formatting may differ from Mapbox's normalized format. Need to
  populate `Locality`/`RegionCode`/`PostalCode` from JSON-LD's
  `PostalAddress` fields.
- **Effort:** Medium.

## Recommendation

**Option B.** The one-liner fix (Option A) solves the immediate bug but
leaves accuracy on the table. Option B adds minimal complexity while
providing excellent geocoding: the structured location text gives Mapbox the
right name and address, and the coordinates as proximity bias eliminate
ambiguity. This covers the Eventbrite case and improves results for Luma,
AllEvents, and any other platform that includes geo data.

Option C is tempting but the lack of `mapbox_feature_id` hurts location
deduplication, and the extra code to parse PostalAddress fields into the
right format adds complexity for a marginal gain over Option B.

## Open Questions

- **Should we also extract geo coordinates from JSON-LD for non-location
  purposes (e.g., map display)?** — Recommendation: Yes, add `Latitude`
  and `Longitude` fields to `StructuredEventData`. They're already
  present in many event pages (Luma, AllEvents) and provide value beyond
  just geocoding — the client could display a map pin without needing a
  separate geocoding call.

- **Should we add a confidence threshold for Mapbox results?** — 
  Recommendation: Not in this PR. It's a broader change that affects all
  location geocoding, not just JSON-LD pages. File as a separate issue
  if desired.

- **Does the `extractJSONLDLocation` function return a good Mapbox
  query string?** — The current format is `"Place Name, streetAddress,
  addressLocality, addressRegion"` (e.g., `"Boulder Airport, 3393
  Airport Rd, Boulder, CO 80302, Boulder, CO"`). This is a good geocoding
  query. However, some JSON-LD has redundant data (streetAddress already
  contains city/state). No action needed — Mapbox handles redundancy fine.

## Implementation Plan

### Phase 1: Fix location override and add geo coordinate extraction
**Goal:** Structured JSON-LD location always overrides LLM extraction,
and geo coordinates from JSON-LD are used as proximity bias for Mapbox.

**Tasks:**
- [ ] Change the location override in `gen_ai.go` from fallback to
  always-override when structured event location is present (remove
  `&& aiResponse.LocationQuery == ""`)
- [ ] Add `Latitude` and `Longitude` (float64 pointers) to
  `StructuredEventData` in `webfetch/fetcher.go`
- [ ] Extract `geo.latitude` and `geo.longitude` from JSON-LD Place
  objects in `buildEventData` / `extractJSONLDLocation`
- [ ] In `gen_ai.go`, when `pageContent.Event` has geo coordinates,
  pass them as proximity bias to `SearchPlaces` (overriding user's
  lat/lng if structured coordinates are available)
- [ ] Add unit tests for geo coordinate extraction from JSON-LD
  (Luma and AllEvents test cases already have coordinates in the markup)
- [ ] Add a test in `location_test.go` verifying that structured
  location overrides LLM-extracted location
- [ ] Run `npm run build` to verify lint and compilation

**Validates:** The Eventbrite "Boulder Airport" case resolves correctly.
Pages with geo coordinates get proximity bias for better Mapbox matching.

## Audit Notes

1. **Server architecture compliance:** All changes stay within the existing
   `webfetch` and `services/experience` packages. No new service
   dependencies introduced. The `StructuredEventData` struct change is in
   the shared `webfetch` library, which is the correct place for page
   content extraction. Follows Pattern 2 (Interface-Based Dependencies)
   — the Mapbox client is already injected via interface.

2. **No client-side changes needed.** The fix is entirely server-side.
   The `GeocodedLocation` returned to the client will simply have the
   correct data. No caching, i18n, or UI changes required.

3. **No proto changes needed.** The `GenExperienceResponse` already
   returns `GeocodedLocation` and `LocationQuery`. The fix changes which
   values populate those fields, not the fields themselves.

4. **Testing approach is sound.** The existing `location_test.go` already
   has a `mockMapboxClient` and tests for location extraction. Adding a
   test for structured-data override follows the established pattern.
   The JSON-LD real-world tests in `jsonld_realworld_test.go` already
   verify coordinate data exists in test fixtures (Luma, AllEvents).

5. **Geo coordinate extraction scope.** The plan proposes changing
   `extractJSONLDLocation` to also return coordinates. However, the
   function currently returns a single string. A cleaner approach is to
   extract geo coordinates in `buildEventData` (which already parses
   the full JSON-LD object) and add them as fields on
   `StructuredEventData`, keeping `extractJSONLDLocation` focused on
   the location string. The plan's task list already suggests this
   (`buildEventData` / `extractJSONLDLocation`) but should be clarified:
   geo extraction belongs in `buildEventData`, not `extractJSONLDLocation`.

6. **[AUDIT FIX] Proximity override safety.** The plan says "overriding
   user's lat/lng if structured coordinates are available." This is
   correct for the Mapbox proximity bias parameter, but the code should
   only use structured coordinates for proximity bias (not replace the
   user's coordinates in the request, which may be used elsewhere).
   The current code already scopes proximity to the `SearchPlaces` call,
   so this is safe as-is.

## Related Issues
- #990 — Experience creation GenAI does not reliably parse time (same
  structured-vs-LLM override pattern, fixed in PR #1132)
