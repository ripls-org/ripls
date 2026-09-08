---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client geospatial system — Discover map, location picker, and profile locations; provider-agnostic geocoding behind LocationSearchService (Mapbox/Google), Haversine distance, avatar markers, and intent-driven GPS.
  globs: [app/lib/services/location_search_service.dart, app/lib/services/mapbox_location_service.dart, app/lib/services/google_location_search_service.dart, app/lib/services/device_location_service.dart, app/lib/core/utils/discover_map_helper.dart, app/lib/core/utils/map_avatar_renderer.dart, app/lib/presentation/widgets/location/**, app/lib/data/repositories/location_repository.dart]
  triggers: [geospatial, map, location, geocoding, discover, gps, marker, mapbox, google-maps, haversine, place-search]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Client Geospatial System

## Table of Contents

1. [Overview](#overview)
2. [Architecture Principles](#architecture-principles)
3. [Map Data Flow](#map-data-flow)
   - [LOAD Flow](#load-flow-displaying-items-on-map)
   - [SELECT Flow](#select-flow-tapping-an-item-marker)
4. [Three-Layer Architecture](#three-layer-architecture)
   - [DeviceLocationService](#layer-1-devicelocationservice-gps)
   - [LocationSearchService](#layer-2-locationsearchservice-geocoding--place-search)
   - [LocationService & Repository](#layer-3-locationservice--locationrepository-rpc--caching)
   - [DiscoverMapHelper](#layer-4-discovermaphelper-map-state-management)
5. [Profile Locations](#profile-locations)
6. [AI-Powered Location Extraction](#ai-powered-location-extraction)
7. [Server-Side Location APIs](#server-side-location-apis)
8. [User Interactions](#user-interactions-with-maps)
9. [Testing](#testing)
10. [Architecture Strengths & Weaknesses](#architecture-strengths)
11. [Geographic Regions](#geographic-regions)
12. [Shared Location Components](#shared-location-components)
13. [Appendix: Resources & Components](#appendix)

---

## Overview

The Ripls Client provides comprehensive geospatial functionality using the [google_maps_flutter](https://pub.dev/packages/google_maps_flutter) plugin, enabling users to discover nearby items, manage personal locations, and set pickup/return locations for gear. The system includes three main geospatial features:

1. **Discover Map:** Full-screen interactive Google Map showing community gear, requests, and experiences as avatar/thumbnail markers, paired with a draggable bottom sheet of results
2. **Location Picker:** Shared full-screen modal for selecting and editing locations across all creation flows (gear, requests, experiences)
3. **Profile Locations:** User's saved places (home, work, etc.) for reuse across items and personal reference

Geocoding and place search are provider-agnostic behind the [LocationSearchService](../app/lib/services/location_search_service.dart) abstraction. Two implementations ship today — [MapboxLocationService](../app/lib/services/mapbox_location_service.dart) (legacy default) and [GoogleLocationSearchService](../app/lib/services/google_location_search_service.dart) (#2188). The active provider is selected at build time via the `MAP_PROVIDER` `--dart-define` (`'mapbox'` or `'google'`). The map widget itself is always Google Maps, independent of the geocoding provider.

The system combines client-side and server-side geospatial capabilities: client-side distance calculations (Haversine formula) for real-time UI, server-side spatial filtering for proximity queries, proximity-biased search, AI-powered location extraction, and fallback handling for unavailable GPS services.

## Architecture Principles

**Client-Side Distance Calculation:** Distance between user and items is calculated locally using the Haversine great circle formula, eliminating server round-trips for real-time proximity display.

**Map State Isolation:** [DiscoverMapHelper](../app/lib/core/utils/discover_map_helper.dart) encapsulates all map-related state (controller, markers, selection, style, sheet-driven padding) so screen logic stays free of Google-SDK details.

**Avatar / Thumbnail Markers:** Markers are pre-rendered as circular PNG bitmaps via [MapAvatarRenderer](../app/lib/core/utils/map_avatar_renderer.dart) (gear thumbnails, user avatars, or initials fallback) and attached as `BitmapDescriptor.bytes`. The pre-migration Mapbox "ripple" marker design has been deferred — see `docs/ai/map.md` for the motivation.

**Provider-Agnostic Geocoding:** Viewmodels and widgets depend only on the [LocationSearchService](../app/lib/services/location_search_service.dart) interface and the [LocationRepository](../app/lib/data/repositories/location_repository.dart) that wraps it. Swapping providers is a single Riverpod-binding change in [location_providers.dart](../app/lib/services/providers/location_providers.dart).

**Session Tokens for Billing:** On Google, every picker session mints one UUID session token and threads it through every autocomplete keystroke plus the final Place Details call. This collapses the per-session bill into a single $5/1k Place Details charge. Mapbox ignores the token; the abstraction tolerates either behavior.

**Intent-Driven GPS Accuracy:** [DeviceLocationService](../app/lib/services/device_location_service.dart) splits requests by [LocationIntent](../app/lib/services/device_location_service.dart) (`proximityBias` for search ranking, `precisePin` for UI display) so background callers get fast OS-cached fixes while UI-facing callers always get a fresh high-accuracy fix.

**No-Prompt-On-Mount:** Permission is never requested on screen mount (#1174). The first user gesture that needs GPS (tapping "center on me", "use current location", etc.) is the only prompt site.

**Fallback Camera:** When no items and no user position are available, Discover mounts on a contiguous-US camera (39.5°N, 98.5°W, zoom 2.5) so the empty state still reads as a map and cues "tap center-on-me to start."

**Auto-Focus on Selection:** When an item is selected — from the bottom sheet or by marker tap — the camera smoothly animates to center on that item. Camera moves apply a bottom padding equal to the current sheet height so the focused item lands above the sheet.

**Coordinate Filtering:** Items with zero coordinates (`(0, 0)`) are silently skipped from the marker set while remaining visible in the bottom-sheet list.

## Map Data Flow

### LOAD Flow (Displaying Items on Map)

**Example: User opens Discover and the map mounts**

```
DiscoverScreen mounts
    ↓
Read userLocationProvider (cached, no prompt on mount)
    ├─ DeviceLocationService.getCurrentPositionIfGranted(intent)
    ├─ Geolocator: services enabled? permission granted?
    └─ Return Position or null (no prompt if denied)
    ↓
Compute initial camera
    ├─ Already-loaded items? → DiscoverMapHelper.initialCameraForItems()
    ├─ User position?         → center on user @ zoom 12
    └─ Otherwise              → contiguous-US fallback (39.5, -98.5, zoom 2.5)
    ↓
Mount GoogleMap widget with computed camera
    ├─ myLocationEnabled when user position present
    ↓
onMapCreated → DiscoverMapHelper.onMapCreated(controller)
    ├─ Store controller
    └─ Apply current style (dark JSON when applicable)
    ↓
ViewModel pre-renders avatar / thumbnail PNGs for all items
    └─ MapAvatarRenderer.renderThumbnail / renderInitials
    ↓
DiscoverMapHelper.buildMarkers(items, thumbnailImages)
    ├─ Skip items with (0, 0) coords or no rendered bitmap
    └─ Create Marker per item with BitmapDescriptor.bytes
    ↓
GoogleMap rebuilds with new marker Set
    ↓
DiscoverMapHelper.updateCameraForItems()
    ├─ animateCamera(newLatLngBounds) for multiple items
    ├─ animateCamera(newCameraPosition) for one item
    └─ Centered on user position when no items
    ↓
Bottom sheet renders the same items in DiscoverBottomSheet
```

### SELECT Flow (Tapping an Item Marker)

**Example: User taps an avatar marker to view item details**

```
User taps Marker on map
    ↓
Marker.onTap callback fires (captured at buildMarkers time)
    ↓
DiscoverMapHelper._handleMarkerTap(item)
    ├─ Record _selectedItemId
    └─ Invoke onMarkerTapped → DiscoverNotifier.selectItem(item)
    ↓
Screen collapses bottom sheet to peek size
    ↓
DiscoverMapHelper.focusOnItem(item)
    └─ animateCamera to (item.lat, item.lon) @ zoom 14, tilt 45°
    ↓
DiscoverGalleryOverlay surfaces the selected item above the sheet
    ↓
User taps overlay → navigation to Gear / Request / Experience screen
```

**Key Points:**
- Marker `onTap` callbacks are captured into each `Marker` at `buildMarkers` time, so the screen must call `buildMarkers` whenever the callback or items change.
- The Mapbox 1.35× selected-marker scale has no direct Google equivalent — selection is communicated by the gallery overlay below the map instead. `_selectedItemId` is tracked for a future scaled-`BitmapDescriptor` implementation.
- Camera moves apply `DiscoverMapHelper.padding` (driven by `setBottomPadding`) so focused items land above the bottom sheet, not behind it.
- Tapping empty map area clears the selection and toggles the bottom navigation.

---

## Three-Layer Architecture

### Layer 1: DeviceLocationService (GPS)

**Purpose:** Provider-agnostic device GPS via the geolocator plugin.

**File:** [app/lib/services/device_location_service.dart](../app/lib/services/device_location_service.dart)

**Public surface:**
- `getCurrentPositionIfGranted({intent})` — non-prompting variant for background / mount paths.
- `requestAndGetCurrentPosition({intent})` — prompts on `denied`; returns a `LocationRequestResult` carrying both the position and the final permission state so callers don't fire a second Geolocator call to inspect permissions.

**`LocationIntent`:**
- `proximityBias` — caller will not show the position; medium accuracy and a ≤5-min last-known fix are preferred for sub-100ms responses.
- `precisePin` — caller will show the position as a pin / dot / saved address; always a fresh high-accuracy fix.

**Permission and concurrency guarantees:**
- Geolocator throws if `requestPermission` runs while a previous request is pending. The service de-dupes concurrent callers onto a single in-flight request (`_permissionRequest`).
- Never throws. Returns `null` (or a `LocationRequestResult` with no position) on disabled services, denied permission, or unexpected errors.

**Riverpod surface:** Callers read `userLocationProvider(intent)` (in [services/providers/](../app/lib/services/providers/)) rather than touching `DeviceLocationService` directly. The provider exposes a session cache so the same `intent` doesn't refetch within a session.

### Layer 2: LocationSearchService (Geocoding & Place Search)

**Purpose:** Third-party geocoder for autocomplete, area search, suggestion resolution, and reverse geocoding.

**Files:**
- Abstraction: [app/lib/services/location_search_service.dart](../app/lib/services/location_search_service.dart)
- Mapbox impl: [app/lib/services/mapbox_location_service.dart](../app/lib/services/mapbox_location_service.dart)
- Google impl: [app/lib/services/google_location_search_service.dart](../app/lib/services/google_location_search_service.dart)
- Binding: [app/lib/services/providers/location_providers.dart](../app/lib/services/providers/location_providers.dart)

**Interface methods:**
- `searchAddresses(query, {proximity, sessionToken})` — POIs, addresses, places, localities.
- `searchGeographicAreas(query, {proximity, sessionToken})` — admin-area-only results (locality, district, region, country). Always fully populated.
- `resolveSuggestion(suggestion, {sessionToken})` — fetches missing coordinate + admin fields. No-op on Mapbox (which returns full results inline); a Place Details (New) call on Google.
- `reverseGeocode(latitude, longitude)` — single best-match for a coordinate.

**Provider selection:** [locationSearchServiceProvider](../app/lib/services/providers/location_providers.dart) switches on `Environment.mapProvider` (`MAP_PROVIDER` `--dart-define`, default `'mapbox'`). An empty matching credential surfaces as an error at first call rather than at app start.

**`LocationResult` DTO:** Provider-neutral. Carries `name`, `fullName`, `type`, lat/lon, address components, plus an opaque `externalPlaceId` and the matching `externalPlaceProvider` (`'mapbox'` / `'google_maps'`) for cross-session deduplication on save.

**Google-specific behavior:**
- **Autocomplete + Details split.** Google's Places Autocomplete (New) returns only display text and a place id; the coordinate and admin hierarchy are fetched lazily by `resolveSuggestion` via Place Details (New). The field mask is pinned to the cheap "Basic" SKU — see the matching server constant in `server/location/google.go`.
- **Plus Code filtering.** Plus-Code-only reverse-geocode and area results are dropped — they're opaque strings users can't act on (#2188).
- **`result_type` biasing** prefers real addresses over nearby businesses on the reverse-geocode path.
- **Two API keys.** The map widget reads the per-platform `GOOGLE_MAPS_API_KEY` (application-restricted, wired into Android manifest / iOS `GMSServices.provideAPIKey` / web JS loader). The REST geocoder reads `GOOGLE_MAPS_PLACES_KEY` (API-restricted, no per-platform restrictions). Do **not** cross the keys — application restrictions don't validate on the REST path.

**Mapbox-specific behavior:**
- Search Box API (`/search/searchbox/v1/forward`) for forward search with proximity bias.
- Geocoding API (`/search/geocode/v6/reverse`) for reverse.
- `resolveSuggestion` is a pass-through — results are already complete.
- `sessionToken` is silently ignored.

### Layer 3: LocationService & LocationRepository (RPC + Caching)

**Purpose:** Server-backed location persistence with transparent caching.

**Files:**
- [app/lib/services/location_service.dart](../app/lib/services/location_service.dart)
- [app/lib/data/repositories/location_repository.dart](../app/lib/data/repositories/location_repository.dart)

**LocationService RPC methods:** `listUserLocations`, `saveLocation`, `getLocation`, `geocodeAddress`, `reverseGeocode`, `deleteLocation`.

**LocationRepository:** Wraps both `LocationService` and `LocationSearchService`. Geocode / autocomplete / reverse-geocode pass-throughs do **not** cache — the input space is continuous so the hit rate would be near zero. Only `Location` records (RPC results) are cached.

**Cache namespace:** `'location'`; item keys: `'location:$itemId'`; list key: `'user_locations'`. Mutations (`saveLocation`, `deleteLocation`) invalidate the list key automatically.

### Layer 4: DiscoverMapHelper (Map State Management)

**Purpose:** Encapsulates the discover screen's Google Maps state — controller, marker set, selection, active style, and the bottom padding driven by the bottom sheet.

**File:** [app/lib/core/utils/discover_map_helper.dart](../app/lib/core/utils/discover_map_helper.dart)

**Style management:**
- Active style stored as a neutral identifier (`mapStyleStandard` / `mapStyleOutdoor` / `mapStyleSatellite` / `mapStyleDark`) so it round-trips through `SearchState.filters.mapStyle`.
- `mapType` getter translates to Google's `MapType` enum: standard / dark → `MapType.normal`, outdoor → `MapType.terrain`, satellite → `MapType.hybrid`.
- Dark style is `MapType.normal` plus the JSON at `assets/map_styles/google_dark.json` applied via `controller.setMapStyle(...)`.
- `changeStyle(style)` swaps at runtime; the screen rebuilds the `GoogleMap` widget so the new `mapType` flows through.

**Marker management:**
- `buildMarkers({items, thumbnailImages})` produces a fresh `Set<Marker>`. Items without coords or without a pre-rendered thumbnail are skipped. Each marker uses `BitmapDescriptor.bytes(bytes, width: 40)` to size the 112-physical-px PNG to a sensible logical size, and anchors at `(0.5, 1.0)` so the pin tip sits on the coordinate.
- The marker `onTap` callback is captured at build time. The screen must re-run `buildMarkers` whenever `onMarkerTapped` changes.
- `selectAnnotationForItem(item)` and `clearSelection()` track `_selectedItemId` for the future scaled-bitmap implementation but do not currently re-render the marker set.

**Camera control:**
- `setBottomPadding(pixels)` updates the `EdgeInsets.only(bottom: …)` returned by `padding`, applied to all camera moves so focused items land above the bottom sheet.
- `updateCameraForItems(items, {userPosition})` — animates to `newLatLngBounds` for multiple items, `newCameraPosition` for one, or centers on the user when items are empty.
- `focusOnItem({item, zoom})` — smooth `animateCamera` to a single coordinate at zoom 14, tilt 45°.
- `initialCameraForItems(items)` (static) — synchronous best-effort camera for `GoogleMap.initialCameraPosition` so the widget mounts already framed; the post-mount `animateCamera` call refines using the actual map size.

**Lifecycle:**
- `onMapCreated(controller)` — store controller, apply current style.
- `clearAnnotationState()` — wipe markers + selection while preserving the controller (used on community switch before the screen rebuilds the `GoogleMap`).
- `resetState()` / `dispose()` — drop everything.

---

## Profile Locations

### ProfileLocationsScreen

**Purpose:** Manage the user's saved-location list sorted by recency with primary location support.

**File:** [profile_locations_screen.dart](../app/lib/presentation/screens/profile/profile_locations_screen.dart)

**Architecture:**
- User locations are tracked via [UserLocation](../proto/ripls/models/user_location.proto) associations.
- Each association carries the location id and a last-used timestamp.
- Server-side ordering: primary location first, then by `last_used_at_unix_sec` descending.
- Deletion is a hard delete of the `UserLocation` association — the `Location` entity itself stays for other users.

**Key features:** list view sorted by recency, primary pinned at top, "set as primary residence", delete-association, pull-to-refresh, cache invalidation before load.

**User flows:**

**View locations:**
1. Navigate to "My Locations" from profile.
2. `ProfileLocationsViewModel` loads via [GetUserLocations](../server/services/location/user_locations.go).
3. Primary location appears first, others sorted by recency; last-used timestamp is displayed on each card.
4. Tap a location to view details or get directions.

**Set primary location:**
1. Tap location card → "Set as Primary Residence".
2. Updates `User.primary_residence_location_id` via [UserService.saveUser](../server/services/user/user.go).
3. List refreshes with new primary at top.

**Delete location:**
1. Tap delete icon → confirm dialog.
2. [RemoveUserLocation](../server/services/location/user_locations.go) hard-deletes the association.
3. List refreshes.

**Integration:**
- Locations are added to the user's list automatically by the server when `SaveLocation` succeeds — client code must **not** call `saveUser()` after a save.
- Last-used timestamp updates when a location is selected for gear / experiences.

---

## AI-Powered Location Extraction

### Overview

The system extracts locations from natural-language input across all creation flows (gear, experiences, requests) using AI and the geocoding provider. Location handling follows a unified **generate-then-preview** pattern: geocoded locations are displayed in the picker without a database save until the user confirms creation.

### Pattern

1. User provides input with an optional location reference (e.g., "hiking tomorrow morning at Chautauqua").
2. AI extracts the location query string ("Chautauqua").
3. Server geocodes via the configured provider (Mapbox or Google) through `server/location/`.
4. Server returns a `GeocodedLocation` (coordinates, name, address components) in the preview response.
5. Client caches the geocoded data in preview state.
6. The location picker displays the geocoded location without a save.
7. On confirm, the final save creates a `Location` record from the cached geocoded data.
8. Location is added to the user's list automatically by the server.

**Architectural principles:**
- **Generate-then-preview:** `Gen*` RPCs return geocoded data but don't save `Location` records.
- **Lazy persistence:** Locations are saved only when the user confirms creation.
- **Graceful degradation:** Falls back to the user's primary residence if extraction / geocoding fails.
- **No orphaned records:** Cancelled previews don't create database records.

### Unified Location Picker Integration

[LocationPickerModal](../app/lib/presentation/widgets/location/location_picker_modal.dart) accepts either:
- `initialLocationId` (`String`) — a pre-saved location id, or
- `initialGeocodedLocation` (`GeocodedLocation` proto) — unsaved geocoded data from the AI path.

Display priority:
1. If `initialGeocodedLocation` is set, display it on the map immediately.
2. Otherwise load the location by id from the database.
3. Fallback hierarchy: primary residence → current GPS → null (no centered location).

Call sites must go through [LocationPickerHelper.showLocationPicker](../app/lib/core/utils/location_picker_helper.dart) — the helper pre-loads the location synchronously to avoid a fallback-camera flash before the real coordinates arrive.

### Server-Side Location Management

All services route through `server/location/` for geocoding. When `SaveLocation` succeeds the server:
- Makes the new location `User.primary_residence_location_id` if none exists.
- Otherwise appends to `User.other_location_ids`.
- De-duplicates against the existing list.

Client code must **not** call `saveUser()` after saving a location — doing so will overwrite the server's list management.

### Proto Messages

**`GeocodedLocation` (API-level):** name, coordinates, address components; no id or timestamps; reusable across gear, experiences, and requests; intentionally separate from the `Location` storage model so preview data never collides with persisted state.

---

## Server-Side Location APIs

### LocationService RPCs

**Entry point:** `LocationService` in [server/services/location/service.go](../server/services/location/service.go)

1. **ListUserLocations** — fetch all saved locations for the user.
2. **SaveLocation** — persist new location with **automatic location-list management** (see above). See [server/services/location/places.go:68-110](../server/services/location/places.go#L68-L110).
3. **GetLocation** — fetch by id with authorization.
4. **GeocodeAddress** — address → coordinates via the active provider.
5. **ReverseGeocode** — coordinates → address via the active provider.
6. **DeleteLocation** — remove a saved location.
7. **GetNearbyUsers** — users within a radius of a query point.
8. **GetNearbyGear** — gear within a radius of a query point.

The proximity RPCs use two-phase filtering via `QueryByProximityForCommunity` in [server/storage/protosql_spatial.go](../server/storage/protosql_spatial.go): a SQL `BETWEEN` bounding-box filter on indexed lat/lon columns (scoped to the community's active members), followed by precise Haversine distance.

### Provider Library

The server abstracts geocoding behind `location.Provider` in [server/location/provider.go](../server/location/provider.go) with two implementations: `MapboxClient` and `GoogleMapsClient`. Both are wired through the same `ForwardGeocode` / `ReverseGeocode` / `SearchPlaces` surface so the client choice (`MAP_PROVIDER`) and the server choice can be configured independently.

**Why both client and server distance?**
- **Client-side (Haversine):** Real-time distance display, instant sorting without server round-trips, works offline.
- **Server-side (Proximity queries):** Initial filter on the wire, reduced payload, location-based search, server-side authorization.

---

## User Interactions with Maps

### Interaction 1: Mount the discover map

1. The map fills the screen.
2. Initial camera is the items' bounds, the user's position, or the contiguous-US fallback (in that priority order).
3. The user-location blue dot is shown only when a session-cached GPS position exists (no prompt on mount).

### Interaction 2: Select an item from the bottom sheet

1. User taps an item row in `DiscoverBottomSheet`.
2. `DiscoverNotifier.selectItem(item)` updates state.
3. `DiscoverMapHelper.focusOnItem(item)` animates the camera, applying the current sheet-driven bottom padding.
4. The gallery overlay rises above the sheet to show the selected item's media.

### Interaction 3: Tap a marker

1. `Marker.onTap` fires → `DiscoverMapHelper._handleMarkerTap(item)`.
2. `_selectedItemId` is recorded and `onMarkerTapped` notifies the screen.
3. Screen collapses the bottom sheet to the peek size and pushes the gallery overlay.

### Interaction 4: Tap empty map area

1. Toggle the bottom navigation visibility.
2. If an item is selected, clear it (`DiscoverNotifier.clearSelectedItem()` + `DiscoverMapHelper.clearSelection()`).

### Interaction 5: Center on me

1. User-initiated gesture (the only permission-prompt site on this screen, per #1174).
2. `DeviceLocationService.requestAndGetCurrentPosition(intent: precisePin)`.
3. Camera animates to the user's coordinates.

### Interaction 6: Switch communities

1. `DiscoverNotifier` loads the new community's items.
2. Screen calls `DiscoverMapHelper.clearAnnotationState()` and rebuilds the `GoogleMap` with a fresh initial camera.
3. `buildMarkers` re-fires for the new item set.

### Interaction 7: Search

1. Search bar (inside `DiscoverBottomSheet`) includes the user's position as a proximity bias.
2. Results stream into the same bottom-sheet list and marker set.
3. Camera repositions to fit the filtered marker bounds.

### Interaction 8: User-location updates

1. Google Maps SDK updates the blue dot from the OS.
2. Distance calculations recompute reactively (Haversine on the client).

---

## Testing

### Client-Side Tests

**Device location** ([app/test/services/device_location_service_test.dart](../app/test/services/device_location_service_test.dart)) — permission states, intent-driven accuracy choice, concurrent-permission-request de-duplication.

**Location search (Google)** ([app/test/services/google_location_search_service_test.dart](../app/test/services/google_location_search_service_test.dart)) — autocomplete + Place Details split, area search, reverse geocode, Plus Code filtering, session token passthrough.

**Location repository** ([app/test/data/repositories/location_repository_test.dart](../app/test/data/repositories/location_repository_test.dart)) — service delegation, cache hit behavior, list caching, cache invalidation after mutations.

**Location picker** ([app/test/presentation/widgets/location/location_picker_modal_test.dart](../app/test/presentation/widgets/location/location_picker_modal_test.dart), [location_picker_view_model_test.dart](../app/test/presentation/viewmodels/location_picker_view_model_test.dart)) — `locationId` vs `geocodedLocation` paths, save / cancel, fallback hierarchy.

**Profile locations viewmodel** ([app/test/presentation/viewmodels/profile_locations_view_model_test.dart](../app/test/presentation/viewmodels/profile_locations_view_model_test.dart)) — load, set primary, delete, refresh.

**Permission helpers** — [location_permission_helper_test.dart](../app/test/core/utils/location_permission_helper_test.dart).

---

## Architecture Strengths

1. **Provider-agnostic geocoding** behind `LocationSearchService` — swap Mapbox / Google with one Riverpod binding (#2188).
2. **Intent-driven GPS** in `DeviceLocationService` — fast OS-cached fix for proximity bias, fresh high-accuracy fix for UI display.
3. **Single Google API key per role** — `GOOGLE_MAPS_API_KEY` for the per-platform map widget; `GOOGLE_MAPS_PLACES_KEY` for the REST geocoder. No cross-platform restriction headaches.
4. **Session-token billing on Google** — a picker session collapses to one billable Place Details charge regardless of keystroke count.
5. **Avatar / thumbnail markers** — items are visually identifiable on the map without an extra tap.
6. **Sheet-aware camera padding** — focused items land above the bottom sheet, not behind it.
7. **No prompt on mount** (#1174) — permission asked only on user gesture.
8. **Generate-then-preview** — geocoded data flows through the picker without a database save until the user confirms.
9. **Automatic location-list management** — server adds saved locations to the user's list, eliminating a client-side `saveUser` round-trip.
10. **Client-side Haversine distance** — no server round-trip for real-time distance display.
11. **Decoupled map state** — `DiscoverMapHelper` keeps Google SDK details out of the screen.

## Architecture Weaknesses

1. **No selected-marker visual scale.** The pre-migration Mapbox 1.35× scale on selection has no direct Google equivalent; selection is currently communicated by the gallery overlay only. A scaled `BitmapDescriptor` implementation is deferred.
2. **No ripple aesthetic on markers.** Google's `Circle` is geographic, not pixel-sized, and scales with zoom — so the pre-migration ripple design was dropped. See `docs/ai/map.md`.
3. **No marker clustering** (yet). Heavy item sets create visual clutter; clustering would need a custom layer or the `google_maps_cluster_manager` package.
4. **No offline maps.** Google Maps tiles require connectivity.
5. **No category / availability filtering on the map** beyond search.
6. **Single-key REST geocoder.** `GOOGLE_MAPS_PLACES_KEY` is API-restricted but not application-restricted — a leak is more impactful than the per-platform map keys. Rotation is via Secret Manager (`google-maps-api-key-client-places`).
7. **Plus Code filtering is best-effort.** Reverse-geocode results occasionally surface non-Plus-Code-only entries that are still poor matches; the `result_type` bias mitigates but doesn't eliminate this.
8. **Inefficient community switch** — clearing annotation state recreates all markers. Diff-based updates would be faster but haven't been needed at current item counts.

---

# APPENDIX

## Resources

### Key Internal Files

**Client-Side:**

*Discover & map view:*
- [discover_screen.dart](../app/lib/presentation/screens/discover/discover_screen.dart) — full-screen map + bottom sheet
- [discover_map_helper.dart](../app/lib/core/utils/discover_map_helper.dart) — Google Maps state management
- [discover_bottom_sheet.dart](../app/lib/presentation/widgets/discover/discover_bottom_sheet.dart) — draggable results sheet
- [discover_gallery_overlay.dart](../app/lib/presentation/widgets/discover/discover_gallery_overlay.dart) — selected-item media overlay
- [map_avatar_renderer.dart](../app/lib/core/utils/map_avatar_renderer.dart) — circular PNG renderer for marker bitmaps

*Location picker & profile:*
- [location_picker_modal.dart](../app/lib/presentation/widgets/location/location_picker_modal.dart) — shared full-screen picker
- [location_picker_helper.dart](../app/lib/core/utils/location_picker_helper.dart) — helper that pre-loads location before opening the modal
- [location_modal_widgets.dart](../app/lib/presentation/widgets/location/location_modal_widgets.dart) — `LocationMapPreview` and other reusable UI
- [location_autocomplete_field.dart](../app/lib/presentation/widgets/location/location_autocomplete_field.dart) — autocomplete text field
- [profile_locations_screen.dart](../app/lib/presentation/screens/profile/profile_locations_screen.dart) — saved-places list

*Core services & utilities:*
- [device_location_service.dart](../app/lib/services/device_location_service.dart) — GPS (provider-agnostic)
- [location_search_service.dart](../app/lib/services/location_search_service.dart) — geocoder abstraction
- [mapbox_location_service.dart](../app/lib/services/mapbox_location_service.dart) — Mapbox implementation (legacy default)
- [google_location_search_service.dart](../app/lib/services/google_location_search_service.dart) — Google implementation (#2188)
- [location_service.dart](../app/lib/services/location_service.dart) — RPC client
- [location_repository.dart](../app/lib/data/repositories/location_repository.dart) — caching layer
- [location_providers.dart](../app/lib/services/providers/location_providers.dart) — Riverpod bindings
- [location_permission_helper.dart](../app/lib/core/utils/location_permission_helper.dart) — permission UX helpers
- [distance_formatter.dart](../app/lib/core/utils/distance_formatter.dart) — Haversine + formatting

**Server-Side:**
- [service.go](../server/services/location/service.go) — `LocationService` RPC implementation
- [provider.go](../server/location/provider.go) — geocoder abstraction
- [mapbox.go](../server/location/mapbox.go) — Mapbox implementation
- [google.go](../server/location/google.go) — Google implementation

**Proto definitions:**
- [location.proto](../proto/ripls/models/location.proto) — `Location` storage model
- [location_service.proto](../proto/ripls/api/location_service.proto) — API contract (`Location`, `GeocodedLocation`)

### Map Provider Configuration

| `--dart-define`              | Used by                                                | Notes                                                          |
| ---------------------------- | ------------------------------------------------------ | -------------------------------------------------------------- |
| `GOOGLE_MAPS_API_KEY`        | Native map widget (Android manifest / iOS / web JS)    | Application-restricted per platform. Not for REST calls.       |
| `GOOGLE_MAPS_PLACES_KEY`     | `GoogleLocationSearchService` REST                     | API-restricted (Places New + Geocoding). No app restrictions.  |
| `MAPBOX_ACCESS_TOKEN`        | `MapboxLocationService` (and the legacy map widget)    | Required when `MAP_PROVIDER=mapbox`.                           |
| `MAP_PROVIDER`               | `locationSearchServiceProvider`                        | `'mapbox'` (default) or `'google'`. Selects the geocoder only. |

### External Dependencies

**Map widget:**
- [google_maps_flutter](https://pub.dev/packages/google_maps_flutter) — official Google Maps Flutter plugin
- [mapbox_maps_flutter](https://pub.dev/packages/mapbox_maps_flutter) — legacy, still pinned (only `MapboxOptions.setAccessToken` is referenced from `main.dart`)

**Geolocation:**
- [geolocator](https://pub.dev/packages/geolocator) — cross-platform GPS, permissions, last-known fix

**Provider APIs:**
- **Google:** Places API (New) — Autocomplete + Place Details; Geocoding API — area search + reverse.
- **Mapbox:** Search Box API — forward search; Geocoding API v6 — reverse.

---

## Map Components

### LocationMapPreview

**File:** [location_modal_widgets.dart](../app/lib/presentation/widgets/location/location_modal_widgets.dart)

Configurable Google Maps preview widget. Default 200 px height, zoom 16, marker is a `BitmapDescriptor.defaultMarkerWithHue(...)` derived from `AppColors.primary`. (A custom ripple-style `BitmapDescriptor` is deferred — see #2188.)

**Key behaviors:**
- Smooth `flyTo`-style updates via `didUpdateWidget`: when latitude/longitude change, animate the camera instead of recreating the widget.
- Optional `markers` (`Set<Marker>`) + `fitToMarkers`: when supplied, the explicit set replaces the default single marker and, with 2+ points, the camera fits all markers' bounds (with edge padding) on creation and whenever the set changes. Used by the location panel (#2291) to plot every poll proposal at once; the blue user dot still renders separately via `myLocationEnabled`. Because a `GoogleMap` is a platform view screen readers can't introspect, the panel's vote/address list below the map is the accessible source of truth and the map carries only a short `Semantics` label — never make a marker-only interaction load-bearing.
- Optional `interactive` (default `false`): adds an `EagerGestureRecognizer` so the map claims pan / pinch-zoom over a parent that competes for drags — required inside the location panel (the morph surface owns a swipe-to-close `GestureDetector`) and the embedded picker. Leave `false` for read-only previews on a scrolling page, where the page should win the drag.
- **Lettered poll pins.** The location panel renders each poll option's pin as a teardrop bitmap with its A / B / C letter via [`LocationPinRenderer`](../../app/lib/core/utils/location_pin_renderer.dart) (Canvas → PNG → `BitmapDescriptor.bytes`, anchored at the tip). Pins are rendered lazily and cached by letter + leader state; a plain colored marker shows until the bitmap lands. Letters track the option-row order (`_visibleProposals`) so a coordinate-less proposal still consumes its letter and pins stay aligned with the list.
- Pre-controller-ready coordinate replays — if coords change before `onMapCreated`, the new coords are re-applied on creation.
- Dark style is applied lazily via `setMapStyle(_darkStyleJson)` when `Theme.of(context).brightness == Brightness.dark`.
- Optional `onMapTap` callback returns the tapped coordinate (used by the picker modal for tap-to-select with reverse geocoding).
- Optional `onCameraChanged` callback fires on `onCameraIdle` with the centre of the visible region (used to bias autocomplete proximity toward what's on screen, not just GPS).

### MapAvatarRenderer

**File:** [map_avatar_renderer.dart](../app/lib/core/utils/map_avatar_renderer.dart)

Pure async utility (no repositories). Methods return `Uint8List` PNG bitmaps suitable for `BitmapDescriptor.bytes`.

- `renderAvatar({imageUrl, size, selected})` — downloads an image, clips to a circle with a white border.
- `renderThumbnail({imageUrl, size, selected})` — same shape, for item thumbnails.
- `renderInitials({initials, color, size, selected})` — fallback when no image is available.

The selected variant uses a thicker (`4.5 px` vs `3 px`) white border. The discover viewmodel pre-renders these and hands the bitmap map to `DiscoverMapHelper.buildMarkers`.

---

## Distance Calculation

### DistanceFormatter

**File:** [distance_formatter.dart](../app/lib/core/utils/distance_formatter.dart)

- `calculateDistance(lat1, lon1, lat2, lon2)` — Haversine, returns metres (Earth radius `6_371_000 m`).
- `format(distanceInMeters)` — `< 528 ft` (≈ 160.9344 m) displays as feet (e.g. `"500 ft"`); larger displays as miles with one decimal (e.g. `"1.2 miles"`).

Client-side computation eliminates the server round-trip for proximity sorting and label updates.

---

## UI Integration Patterns

### Pattern 1: Map initialization sequence

Build `GoogleMap` with `initialCameraPosition` from `DiscoverMapHelper.initialCameraForItems(items)`, then in `onMapCreated` call `helper.onMapCreated(controller)`, set the sheet-driven bottom padding, and re-run `buildMarkers`. Never call `setMapStyle` before the controller is available.

**Example:** [discover_screen.dart:293-348](../app/lib/presentation/screens/discover/discover_screen.dart#L293-L348)

### Pattern 2: Marker-tap wiring

Marker callbacks are captured at build time, so the screen must re-run `helper.buildMarkers(...)` after changing `helper.onMarkerTapped`. Build markers inside the state's `build` (or memoize per items / callback) — never mutate a `Marker` after it's been added to a `Set`.

**Example:** [discover_screen.dart](../app/lib/presentation/screens/discover/discover_screen.dart), [discover_map_helper.dart:114-147](../app/lib/core/utils/discover_map_helper.dart#L114-L147)

### Pattern 3: Sheet-aware camera padding

The bottom sheet drives `_mapHelper.setBottomPadding(screenHeight * sheetSize)` on `addListener`. The helper's `padding` getter feeds `GoogleMap.padding`, so every camera move offsets for the sheet automatically.

**Example:** [discover_screen.dart:82-90](../app/lib/presentation/screens/discover/discover_screen.dart#L82-L90)

### Pattern 4: Dual location input

`LocationPickerModal` accepts `initialLocationId` (saved) or `initialGeocodedLocation` (AI-extracted but unsaved). Always open via `LocationPickerHelper.showLocationPicker(...)` so the helper can pre-load the saved location synchronously and avoid a fallback-camera flash.

**Example:** [location_picker_helper.dart](../app/lib/core/utils/location_picker_helper.dart), [experience_content_view.dart](../app/lib/presentation/screens/experience/experience_content_view.dart)

### Pattern 5: Provider-aware test setup

When a test depends on geocoding, override `locationSearchServiceProvider` with a fake — never construct `GoogleLocationSearchService` / `MapboxLocationService` directly. The two providers behave differently around `resolveSuggestion`, so a fake-based test stays portable across both.

---

## Geographic Regions

### Overview

The system automatically extracts and tracks geographic regions from user and community locations to enable regional discovery and community organization. Regions are hierarchical (neighborhood → city → county → state → country) and are computed from member primary-residence addresses.

### Region Hierarchy

From most specific to most general:

1. **Neighborhood** — local areas within cities (e.g., "Capitol Hill")
2. **City** — municipalities and localities (e.g., "Boulder", "Denver")
3. **County** — administrative subdivisions (e.g., "Boulder County")
4. **State** — states, provinces, territories (e.g., "Colorado", "CO")
5. **Country** — nations (e.g., "United States", "US")

Each region stores a hierarchical path (comma-separated ids) enabling efficient ancestor / descendant queries.

### Automatic Region Computation

**Triggers:** community creation (from the creator's location), new member joins, member updates primary residence.

**Server implementation:** [server/services/community/regions.go:141-297](../server/services/community/regions.go#L141-L297)

**Process:**
1. Extract location from each member's `primary_residence_location_id`.
2. Extract regions from address components (neighborhood, city, county, state).
3. Create or find `Region` records via `GetOrCreateRegion`.
4. Count members per region and calculate percentages.
5. Create `CommunityRegion` associations linking communities to regions.
6. Skip recomputation if a manual override exists.

**Example:** community with 6 members in Boulder and 4 in Denver → Boulder (60%), Denver (40%), Colorado (100%).

### Manual Region Override

Community creators can manually set a single region override, replacing all auto-computed regions.

**Server RPC:** `SetCommunityRegionOverride` in [server/services/community/regions.go:67-139](../server/services/community/regions.go#L67-L139)

**Client:** [governance_screen.dart](../app/lib/presentation/screens/governance/governance_screen.dart)

### Region Deduplication

**Server library:** [server/location/regions.go](../server/location/regions.go) — `GetOrCreateRegion` matches case-insensitively with whitespace normalisation before inserting.

### Global Region Discovery

1. User enters a region name in an autocomplete field.
2. Server searches the `Region` table by name (case-insensitive).
3. Results show region-type icons and the community count.
4. User selects a region to view associated communities.

### Region Display Names

**Server library:** `GetRegionDisplayName` in [server/location/regions.go:102-129](../server/location/regions.go#L102-L129)

Regions are displayed with parent context using state codes when available:
- "Boulder, CO" (city with state parent)
- "Capitol Hill, Denver" (neighborhood with city parent)
- "Colorado" (state with no parent)

---

## Shared Location Components

The system provides reusable UI components for consistent location management across all features.

**File:** [location_modal_widgets.dart](../app/lib/presentation/widgets/location/location_modal_widgets.dart)

### LocationMapPreview

(See [Map Components](#locationmappreview) above.) Used in the location picker, location-details cards, and any read-only map preview.

### LocationPickerModal

**Purpose:** Shared modal for selecting / editing locations across all features (gear, requests, experiences).

**Files:**
- Widget: [location_picker_modal.dart](../app/lib/presentation/widgets/location/location_picker_modal.dart)
- ViewModel: [location_picker_view_model.dart](../app/lib/presentation/viewmodels/location_picker_view_model.dart)
- Helper: [location_picker_helper.dart](../app/lib/core/utils/location_picker_helper.dart)

**Key features:**
- Full-screen interactive Google Map with proximity-biased search.
- Saved-location chips for quick selection.
- Ownership-based edit permissions.
- Optional "Get Directions" button (opens native Maps via `url_launcher`).
- Tap-to-select with reverse geocoding.
- Change detection (Save / Cancel only enabled when changes are made).
- Integrates with automatic server-side location-list management.

**Architecture:**
- Pre-loading via `LocationPickerHelper` avoids the fallback-camera flash before the real coordinates arrive.
- Smooth `animateCamera` updates (not widget recreation) for coordinate changes.
- The user-location blue dot is shown alongside the selected-location marker.
- Proximity-biased search uses the visible map centre, not just the GPS position, so panning the map refines the autocomplete results.

**Critical patterns:**
- Use `LocationPickerHelper.showLocationPicker()` — never call `LocationPickerModal.show()` directly.
- The viewmodel must **not** call `saveUser()` after saving a location — the server manages the user's list automatically.
- Always thread the picker's session token through every autocomplete call and the final selection (`resolveSuggestion`) so Google billing collapses to one Place Details charge.

### LocationDetailsCard & LocationDisplayMap

- **LocationDetailsCard** — locality, full address, coordinates (monospace, 6 decimals); edit button if the user is authorized.
- **LocationDisplayMap** — read-only Google Maps view of a saved location at zoom 16, used in detail views.

### Location Architecture Patterns

- **Dual map presentation.** Blue dot (user) plus a marker (selected location) coexist on the same map.
- **Authorization.** `isOwner` checks gate edit UI; viewers see read-only details.
- **Optimization.** User-location is cached per `LocationIntent` to avoid repeated GPS fetches; autocomplete is debounced.
- **Automatic location-list management.** `SaveLocation` on the server adds the location to the user's list. Calling `saveUser()` client-side will override the server logic. See [user_location_initializer.dart](../app/lib/core/utils/user_location_initializer.dart), [create_place_modal.dart](../app/lib/presentation/screens/locations/create_place_modal.dart), and [location_picker_view_model.dart](../app/lib/presentation/viewmodels/location_picker_view_model.dart).
- **Directions integration.** Platform-specific URLs (Apple Maps / Google Maps) launched via `url_launcher`.

---

**Last Updated:** 2026-05-31
**Architecture Version:** 2.0 (Google Maps migration)
**Status:** Production-ready

**Recent Changes:**
- Migrated the map widget from `mapbox_maps_flutter` to `google_maps_flutter`. The pre-existing ripple-marker aesthetic and 1.35× selected-marker scale are deferred — markers are now circular avatar / thumbnail bitmaps rendered via [MapAvatarRenderer](../app/lib/core/utils/map_avatar_renderer.dart).
- Introduced the provider-agnostic [LocationSearchService](../app/lib/services/location_search_service.dart) with Mapbox and Google implementations selected at build time by `MAP_PROVIDER` (#2188).
- Split GPS access into the provider-agnostic [DeviceLocationService](../app/lib/services/device_location_service.dart) with `LocationIntent` for accuracy / staleness tradeoffs (#1175).
- Discover screen is now a full-screen map plus a draggable bottom sheet (no list/map toggle). The fallback camera is the contiguous US (39.5, -98.5, zoom 2.5).
- No-prompt-on-mount (#1174): the first user gesture is the only permission-prompt site on Discover.