# Location

The `location` package is the geocoding-provider abstraction used by
server-side services. A single `Provider` interface fronts pluggable
implementations (today: Mapbox via `MapboxClient`; planned: Google Maps).
The package also includes EXIF-based coordinate extraction from photos and
helpers for mapping coordinates to administrative regions (city, state,
country).

## Key files

- `provider.go` — `Provider` interface: `ForwardGeocode`, `SearchPlaces`,
  `ReverseGeocode`, `CheckHealth`. The shape all services depend on.
- `mapbox.go` — `MapboxClient`: forward and reverse geocoding via the
  Mapbox Search Box and Geocoding APIs. Satisfies `Provider` (verified by
  a compile-time assertion at the bottom of `provider.go`).
- `exif.go` — extracts GPS coordinates from image EXIF metadata.
- `geocoded_location.go` — converts a `PlaceResult` (the provider-neutral
  result type) into the `api.GeocodedLocation` proto returned to clients.
- `regions.go` — maps a `models.Location` to its administrative region
  hierarchy (neighborhood, city, county, state, country).
- `fallback.go` — fallback strategies when geocoding returns no result.

## When to add code here vs. elsewhere

New geocoding or coordinate utilities belong here. New provider backends
(e.g. a Google Maps implementation) live as a sibling file (`google.go`)
that also satisfies `Provider`. RPC handlers for location-related
endpoints (save place, list places, EXIF-based location extraction from
uploads) belong in `server/services/location`. Community region
computation from member locations belongs in `server/community`.
