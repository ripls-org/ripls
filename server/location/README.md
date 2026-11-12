# Location

The `location` package is the geocoding-provider abstraction used by
server-side services. A single `Provider` interface fronts the geocoding
client; the only implementation is Google Maps Platform via
`GoogleMapsClient` (#2188). The package also includes helpers for mapping
coordinates to administrative regions (city, state, country) and the
user-location fallback used when no location can be extracted.

## Key files

- `provider.go` — `Provider` interface: `ForwardGeocode`, `SearchPlaces`,
  `ReverseGeocode`, `CheckHealth`. The shape all services depend on. Also
  defines the provider-neutral `PlaceResult` and the `ProximityRequest`
  semantics (none / hint / bound).
- `google.go` — `GoogleMapsClient`: place search via Places API (New) and
  forward / reverse geocoding via the Geocoding API. Satisfies `Provider`
  (verified by a compile-time assertion at the bottom of `provider.go`).
  `server/wiring.go` constructs it from `--google-maps-api-key-server`,
  which is required outside dev mode.
- `geometry.go` — haversine distance and the `ProximityBound` post-filter.
- `geocoded_location.go` — converts a `PlaceResult` (the provider-neutral
  result type) into the `api.GeocodedLocation` proto returned to clients.
- `regions.go` — maps a `models.Location` to its administrative region
  hierarchy (neighborhood, city, county, state, country).
- `state_codes.go` — US state / territory name → USPS code lookup.
- `fallback.go` — fallback strategies when geocoding returns no result.

Stored rows from before the #2188 cutover may carry
`external_place_provider = "mapbox"`; nothing in this package produces
that value any more.

## When to add code here vs. elsewhere

New geocoding or coordinate utilities belong here. A new provider backend
would live as a sibling file that also satisfies `Provider`. RPC handlers
for location-related endpoints (save place, list places, EXIF-based
location extraction from uploads — `server/services/location/exif.go`)
belong in `server/services/location`. Community region computation from
member locations belongs in `server/community`.
