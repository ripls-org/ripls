import 'package:geolocator/geolocator.dart';

/// LocationSearchService is the client-side abstraction for geocoding and
/// place-search operations against an external provider (Mapbox today,
/// Google Maps next). Implementations wrap a single provider's HTTP API.
///
/// Callers must depend on this interface, not a concrete implementation.
/// Per `docs/client/architecture.md`, this service is consumed only by
/// [LocationRepository] — viewmodels and widgets do not access it
/// directly.
///
/// **Session tokens.** The [sessionToken] parameter on the search methods
/// (and the matching [resolveSuggestion] follow-up) lets a picker session
/// bundle all of its autocomplete keystrokes plus one selection-time
/// detail fetch under a single billable session on providers that
/// support it (Google Maps Platform). Pickers should mint a UUID when
/// the search UI opens, thread it through every search call, pass the
/// same token to [resolveSuggestion] when the user picks a result, and
/// discard the token when the picker closes. Providers that don't
/// recognise session tokens (Mapbox) ignore the parameter.
abstract class LocationSearchService {
  /// Searches for addresses, POIs, places, and localities matching [query].
  /// Returns up to ten suggestions sorted by relevance.
  ///
  /// Suggestions may be **incomplete** — providers that bill per detail
  /// fetch (Google) return only the displayable text + an opaque
  /// [LocationResult.externalPlaceId]. Callers must call
  /// [resolveSuggestion] before persisting or otherwise relying on the
  /// coordinate / address fields. Providers that always return full
  /// results (Mapbox) populate every field up front, and
  /// [resolveSuggestion] is a no-op pass-through there.
  Future<List<LocationResult>> searchAddresses(
    String query, {
    Position? proximity,
    String? sessionToken,
  });

  /// Searches for geographic areas only (place, locality, district,
  /// region, country) matching [query]. Excludes POIs and street
  /// addresses. Use for discovery / filtering flows where the user is
  /// selecting an area rather than a specific address.
  ///
  /// Returns are fully populated (coordinate + admin hierarchy) — area
  /// queries don't have the POI N+1 cost profile that drives lazy
  /// resolution on [searchAddresses], so the result is usable as-is
  /// without a follow-up [resolveSuggestion] call.
  Future<List<LocationResult>> searchGeographicAreas(
    String query, {
    Position? proximity,
    String? sessionToken,
  });

  /// Resolves a search [suggestion] (typically from [searchAddresses]) to
  /// a fully-populated [LocationResult] with coordinate and address
  /// components.
  ///
  /// Providers that return incomplete suggestions (Google) fetch the
  /// remaining detail fields here; providers that return full results
  /// (Mapbox) return the suggestion unchanged.
  ///
  /// Pass the same [sessionToken] used for the originating search to
  /// keep the autocomplete + details pair billable as a single session.
  /// Returns null when the provider has no detail for the suggestion.
  Future<LocationResult?> resolveSuggestion(
    LocationResult suggestion, {
    String? sessionToken,
  });

  /// Converts coordinates to a single best-match [LocationResult].
  /// Returns null when the provider returns no result.
  Future<LocationResult?> reverseGeocode(double latitude, double longitude);
}

/// LocationResult is the provider-neutral DTO returned from
/// [LocationSearchService]. Implementations populate every field they can
/// from the underlying provider's response; absent fields are empty
/// strings.
class LocationResult {
  final String name;
  final String fullName;
  final String type;
  final double latitude;
  final double longitude;
  final String streetAddress;
  final String locality;
  final String region;
  final String postcode;
  final String regionCode;

  /// Provider-specific opaque identifier for this place (empty if the
  /// geocoder didn't return one). Paired with [externalPlaceProvider],
  /// used for cross-session deduplication when saving the location.
  final String externalPlaceId;

  /// The geocoding provider that issued [externalPlaceId] (e.g., "mapbox",
  /// "google"). Empty when [externalPlaceId] is empty.
  final String externalPlaceProvider;

  const LocationResult({
    required this.name,
    required this.fullName,
    required this.type,
    required this.latitude,
    required this.longitude,
    required this.streetAddress,
    required this.locality,
    required this.region,
    required this.postcode,
    required this.regionCode,
    this.externalPlaceId = '',
    this.externalPlaceProvider = '',
  });

  @override
  String toString() {
    return 'LocationResult(name: $name, fullName: $fullName, type: $type, '
        'lat: $latitude, lng: $longitude, streetAddress: $streetAddress, '
        'locality: $locality, region: $region, postcode: $postcode, '
        'regionCode: $regionCode, externalPlaceId: $externalPlaceId, '
        'externalPlaceProvider: $externalPlaceProvider)';
  }
}
