# services/location

The `location` service implements the LocationService RPC interface: saving and retrieving named places, geocoding text queries, extracting coordinates from photo EXIF data, computing geospatial proximity, managing per-user location history, and deduplicating nearby places.

## Key files

- `service.go` — service struct and constructor.
- `places.go` — `SaveLocation`, `GetLocation`, `ListLocations`.
- `geocoding.go` — `GeocodeText`: forward geocoding from a text query.
- `exif.go` — `ExtractLocation`: extracts coordinates from uploaded photo metadata.
- `proximity.go` — proximity queries (nearby places).
- `user_locations.go` — per-user location history management.
- `deduplication.go` — merges duplicate place entries for the same physical location.
- `regions.go` — administrative region assignment to saved places.

## Proximity RPC privacy contract

`GetNearbyUsers` and `GetNearbyGear` enforce the following invariants before returning any results:

1. **Community-scoped**: the caller must supply a `community_id` and be an active member (`auth.RequireMemberOfActiveCommunity`). Only entities owned by fellow members of the same community are returned.
2. **Coordinate quantization**: returned `latitude_deg` / `longitude_deg` are rounded to 3 decimal places (~110 m grid) so member home and gear locations cannot be pinpointed.
3. **Radius cap**: requests with `radius_meters > 50,000` are rejected with `CodeInvalidArgument`.
4. **Result cap**: at most 50 results are returned, sorted by distance ascending.
5. **Soft-deleted excluded**: both the entity and the community membership row must be active.

The storage primitive is `ProtoSQLStorage.QueryByProximityForCommunity` in `server/storage/protosql_spatial.go`.

## When to add code here vs. elsewhere

Location RPC handlers belong here. The geocoding API client belongs in `server/location`. Community-level geographic region computation (based on member home locations) belongs in `server/community`.
