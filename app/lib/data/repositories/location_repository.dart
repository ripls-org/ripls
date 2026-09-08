import 'package:geolocator/geolocator.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/gen/ripls/api/location_service.pb.dart';
import 'package:ripls/services/location_search_service.dart';
import 'package:ripls/services/location_service.dart';

final _log = Logger('LocationRepository');

/// Repository for location data with transparent caching.
///
/// LocationRepository wraps two services:
/// - LocationService (RPC) for server-side location storage with caching
/// - LocationSearchService (third-party geocoder) for autocomplete and
///   reverse-geocode pass-throughs (no caching — inputs are continuous)
///
/// ViewModels call this repository; widgets do not access services
/// directly (`docs/client/architecture.md`).
class LocationRepository {
  final CacheService _cache;
  final LocationService _service;
  final LocationSearchService _searchService;

  LocationRepository(
    CacheManager cacheManager,
    this._service,
    this._searchService,
  ) : _cache = CacheService(cacheManager, 'location');

  /// Gets a location by ID with caching.
  Future<Location> get(String locationId) async {
    // Get location from service (already converted to Location)
    final location = await _cache.get(
      key: locationId,
      fetch: () => _service.getLocation(locationId),
    );

    return location;
  }

  /// Gets a location by ID with caching.
  ///
  /// This is a convenience wrapper around get() that returns
  /// the shared Location type.
  Future<Location> getLocation(String locationId) async {
    return get(locationId);
  }

  /// Saves a new location and invalidates relevant caches.
  ///
  /// Returns the ID of the saved location.
  /// Automatically adds the location to the authenticated user's location list.
  Future<String> saveLocation({
    required double latitudeDeg,
    required double longitudeDeg,
    required String regionCode,
    required String postalCode,
    required String locality,
    List<String>? addressLines,
    String? name,
    String? neighborhood,
    String? county,
    String? administrativeArea,
    String? externalPlaceId,
    String? externalPlaceProvider,
    String? userId,
  }) async {
    final locationId = await _service.saveLocation(
      latitudeDeg: latitudeDeg,
      longitudeDeg: longitudeDeg,
      regionCode: regionCode,
      postalCode: postalCode,
      locality: locality,
      addressLines: addressLines,
      name: name,
      neighborhood: neighborhood,
      county: county,
      administrativeArea: administrativeArea,
      externalPlaceId: externalPlaceId,
      externalPlaceProvider: externalPlaceProvider,
    );

    // Invalidate caches after mutation
    // Invalidate the user locations cache since a new location was added
    await _cache.invalidate('user_locations');

    return locationId;
  }

  /// Deletes a location by ID and invalidates caches.
  ///
  /// Returns true if the location was deleted successfully.
  Future<bool> deleteLocation(String locationId, {String? userId}) async {
    final deleted = await _service.deleteLocation(locationId);

    // Invalidate caches after mutation
    await invalidate(locationId);
    // Invalidate the user locations cache since a location was deleted
    await _cache.invalidate('user_locations');

    return deleted;
  }

  /// Geocodes an address to coordinates.
  ///
  /// This is a pass-through to the service with no caching since addresses
  /// are already validated/cached on the server side.
  Future<GeocodeAddressResponse> geocodeAddress({
    required String regionCode,
    required String postalCode,
    required String locality,
    List<String>? addressLines,
  }) async {
    return _service.geocodeAddress(
      regionCode: regionCode,
      postalCode: postalCode,
      locality: locality,
      addressLines: addressLines,
    );
  }

  /// Reverse geocodes coordinates to an address.
  ///
  /// This is a pass-through to the service with no caching since coordinates
  /// can map to many possible addresses depending on precision.
  Future<ReverseGeocodeResponse> reverseGeocode({
    required double latitudeDeg,
    required double longitudeDeg,
  }) async {
    return _service.reverseGeocode(
      latitudeDeg: latitudeDeg,
      longitudeDeg: longitudeDeg,
    );
  }

  /// Reverse geocodes coordinates via the third-party search service and
  /// returns a [LocationResult]. Distinct from [reverseGeocode], which
  /// calls the server's LocationService RPC and returns a
  /// [ReverseGeocodeResponse].
  ///
  /// Throws if the search service returns no result.
  ///
  /// No caching: reverse-geocode inputs are continuous coordinates, so
  /// cache hits would be rare.
  Future<LocationResult> reverseGeocodeViaSearchService(
    double latitudeDeg,
    double longitudeDeg,
  ) async {
    final result = await _searchService.reverseGeocode(
      latitudeDeg,
      longitudeDeg,
    );
    if (result == null) {
      _log.warning(
        'Reverse geocode returned no result for '
        '($latitudeDeg, $longitudeDeg)',
      );
      throw Exception('Unable to determine address for your location.');
    }
    return result;
  }

  /// Searches for addresses, POIs, places, and localities matching [query].
  /// Pass-through to the configured [LocationSearchService] (no caching
  /// — autocomplete queries are typed character-by-character and rarely
  /// repeat).
  ///
  /// Suggestions may be incomplete on providers that bill per detail
  /// fetch — call [resolveSuggestion] before persisting any selection.
  /// See [LocationSearchService] for the session-token contract.
  Future<List<LocationResult>> searchAddresses(
    String query, {
    Position? proximity,
    String? sessionToken,
  }) {
    return _searchService.searchAddresses(
      query,
      proximity: proximity,
      sessionToken: sessionToken,
    );
  }

  /// Searches for geographic areas (place, locality, district, region,
  /// country) matching [query]. Pass-through; no caching.
  Future<List<LocationResult>> searchGeographicAreas(
    String query, {
    Position? proximity,
    String? sessionToken,
  }) {
    return _searchService.searchGeographicAreas(
      query,
      proximity: proximity,
      sessionToken: sessionToken,
    );
  }

  /// Resolves a [LocationResult] suggestion to a fully-populated record.
  /// Pass-through to the configured [LocationSearchService].
  ///
  /// Mapbox returns the input unchanged; Google fetches Place Details.
  /// Pass the same [sessionToken] that produced the suggestion to keep
  /// the autocomplete + details pair on one billable session.
  Future<LocationResult?> resolveSuggestion(
    LocationResult suggestion, {
    String? sessionToken,
  }) {
    return _searchService.resolveSuggestion(
      suggestion,
      sessionToken: sessionToken,
    );
  }

  /// Invalidates a specific location entry.
  Future<void> invalidate(String locationId) async {
    await _cache.invalidate(locationId);
  }

  /// Invalidates all cached locations.
  Future<void> invalidateAll() async {
    await _cache.invalidateAll();
  }

  /// Searches for regions by name with caching.
  ///
  /// Results are cached per query+type combination.
  /// Use [invalidateRegionSearchCache] to force a refresh.
  Future<List<RegionItem>> searchRegions({
    required String query,
    String? regionType,
  }) async {
    final type = regionType ?? 'all';
    return _cache.get(
      key: 'regions:search:$query:$type',
      fetch: () => _service.searchRegions(
        query: query,
        regionType: regionType,
      ),
    );
  }

  /// Gets a specific region by ID with caching.
  ///
  /// Use [invalidateRegion] to force a refresh.
  Future<RegionItem> getRegion(String regionId) async {
    return _cache.get(
      key: 'region:$regionId',
      fetch: () => _service.getRegion(regionId),
    );
  }

  /// Invalidates all cached region search results.
  ///
  /// Call this when region data changes (e.g., new communities created in regions).
  Future<void> invalidateRegionSearchCache() async {
    await _cache.invalidatePattern('regions:search:');
  }

  /// Invalidates a specific region's cached data.
  ///
  /// Call this when a region's details change.
  Future<void> invalidateRegion(String regionId) async {
    await _cache.invalidate('region:$regionId');
  }

  /// Gets the authenticated user's locations sorted by last_used_at (most recent first).
  ///
  /// Use [refreshUserLocations] to force a refresh.
  Future<List<UserLocationWithDetails>> getUserLocations({int? limit}) async {
    return _cache.get(
      key: 'user_locations',
      fetch: () => _service.getUserLocations(limit: limit),
    );
  }

  /// Adds a location to the authenticated user's location list.
  ///
  /// Creates or updates the user-location association and refreshes the cache.
  Future<void> addUserLocation(String locationId) async {
    await _service.addUserLocation(locationId);
    await _cache.invalidate('user_locations');
  }

  /// Removes a location from the authenticated user's location list.
  ///
  /// Soft-deletes the user-location association and refreshes the cache.
  Future<void> removeUserLocation(String locationId) async {
    await _service.removeUserLocation(locationId);
    await _cache.invalidate('user_locations');
  }

  /// Updates the last_used_at timestamp for a location.
  ///
  /// Call this when the user selects a location for gear/request/experience.
  /// This bubbles the location to the top of the user's location list.
  Future<void> updateLocationLastUsed(String locationId) async {
    await _service.updateLocationLastUsed(locationId);
    await _cache.invalidate('user_locations');
  }

  /// Invalidates the user locations cache to force a refresh on next load.
  ///
  /// Call this when you want to ensure fresh data from the server.
  Future<void> invalidateUserLocationsCache() async {
    await _cache.invalidate('user_locations');
  }
}
