import 'dart:convert';
import 'package:geolocator/geolocator.dart';
import 'package:http/http.dart' as http;
import 'package:logging/logging.dart';
import '../core/config/environment.dart';
import 'location_search_service.dart';

final _log = Logger('GoogleLocationSearchService');

/// Canonical value for [LocationResult.externalPlaceProvider] when the
/// place came from the Google Maps Platform APIs.
const String googleMapsProvider = 'google_maps';

/// Google-backed implementation of [LocationSearchService].
///
/// Search uses **Places Autocomplete (New)** so POI queries like
/// "Zilker Park" resolve. Autocomplete suggestions return only the place
/// id and display text; full coordinates and address components are
/// fetched lazily via **Place Details (New)** in [resolveSuggestion].
/// Pickers thread the same session token through every autocomplete
/// keystroke and the final details call, which collapses the per-session
/// bill to a single $5/1k Place Details charge regardless of how many
/// autocomplete keystrokes fired.
///
/// Geographic-area search ([searchGeographicAreas]) stays on the
/// Geocoding API: admin-area types resolve well there and the response
/// already carries the coordinate, so the resolveSuggestion follow-up
/// isn't needed.
///
/// Mirrors the server-side `GoogleMapsClient` mitigations on the
/// reverse-geocode path: a `result_type` filter prefers real addresses
/// over nearby businesses, and Plus-Code-only results are dropped.
class GoogleLocationSearchService implements LocationSearchService {
  static const String _geocodingUrl =
      'https://maps.googleapis.com/maps/api/geocode/json';
  static const String _autocompleteUrl =
      'https://places.googleapis.com/v1/places:autocomplete';
  static const String _placeDetailsBaseUrl =
      'https://places.googleapis.com/v1/places/';

  /// Field mask scoping Place Details billing to the cheap "Basic" SKU.
  /// Adding atmosphere or contact fields would jump every call into a
  /// higher tier — keep this list minimal and matched to the server-side
  /// `placeDetailsFieldMask` constant in `server/location/google.go`.
  static const String _placeDetailsFieldMask =
      'id,displayName,formattedAddress,location,types,addressComponents';

  /// Result types preferred for reverse geocoding. Pipe-separated; the
  /// Geocoding API treats this as OR. Excludes pure Plus Codes and
  /// de-prioritizes establishment / point_of_interest matches.
  static const String _reverseResultTypes =
      'street_address|route|premise|subpremise|locality';

  /// Approximate ~50 km half-side for the autocomplete proximity circle.
  /// Matches the server-side `DefaultBoundRadiusM` so client/server agree
  /// on what "near" means.
  static const double _proximityRadiusMeters = 50000;

  final http.Client _http;
  final String _apiKey;

  GoogleLocationSearchService({http.Client? httpClient, String? apiKey})
      : _http = httpClient ?? http.Client(),
        _apiKey = apiKey ?? Environment.googlePlacesApiKey;

  @override
  Future<List<LocationResult>> searchAddresses(
    String query, {
    Position? proximity,
    String? sessionToken,
  }) async {
    try {
      if (query.trim().isEmpty) {
        return [];
      }

      final body = <String, dynamic>{'input': query};
      if (proximity != null) {
        body['locationBias'] = {
          'circle': {
            'center': {
              'latitude': proximity.latitude,
              'longitude': proximity.longitude,
            },
            'radius': _proximityRadiusMeters,
          },
        };
      }
      if (sessionToken != null && sessionToken.isNotEmpty) {
        body['sessionToken'] = sessionToken;
      }

      _log.info('Google autocomplete: $query');

      final response = await _http.post(
        Uri.parse(_autocompleteUrl),
        headers: {
          'Content-Type': 'application/json',
          'X-Goog-Api-Key': _apiKey,
        },
        body: json.encode(body),
      );

      if (response.statusCode != 200) {
        _log.severe(
          'Failed to autocomplete: ${response.statusCode}\n'
          'Response body: ${response.body}',
        );
        return [];
      }

      final data = json.decode(response.body) as Map<String, dynamic>;
      final suggestions = (data['suggestions'] as List? ?? [])
          .cast<Map<String, dynamic>>();
      final results = <LocationResult>[];
      for (final s in suggestions) {
        final pred = s['placePrediction'] as Map<String, dynamic>?;
        if (pred == null) {
          continue;
        }
        final placeId = pred['placeId'] as String? ?? '';
        if (placeId.isEmpty) {
          continue;
        }
        results.add(_suggestionFromPrediction(pred, placeId));
      }
      _log.info('Found ${results.length} autocomplete suggestions');
      return results;
    } catch (e, stackTrace) {
      _log.severe('Error in autocomplete', e, stackTrace);
      return [];
    }
  }

  @override
  Future<List<LocationResult>> searchGeographicAreas(
    String query, {
    Position? proximity,
    String? sessionToken,
  }) async {
    try {
      if (query.trim().isEmpty) {
        return [];
      }

      final params = <String, String>{
        'address': query,
        'key': _apiKey,
        // Geocoding API result_type for area-only queries excludes street
        // addresses and POIs.
        'result_type':
            'locality|sublocality|administrative_area_level_1|administrative_area_level_2|country|postal_code',
      };

      // Google's Geocoding API takes a ~50km bounds box as the closest
      // analogue to a proximity hint.
      if (proximity != null) {
        const halfDeg = 0.45; // ~50km at most latitudes
        final sw =
            '${proximity.latitude - halfDeg},${proximity.longitude - halfDeg}';
        final ne =
            '${proximity.latitude + halfDeg},${proximity.longitude + halfDeg}';
        params['bounds'] = '$sw|$ne';
      }

      _log.info('Google searching for areas: $query');

      final uri = Uri.parse(_geocodingUrl).replace(queryParameters: params);
      final response = await _http.get(uri);

      if (response.statusCode != 200) {
        _log.severe(
          'Failed to search areas: ${response.statusCode}\n'
          'Response body: ${response.body}',
        );
        return [];
      }

      final data = json.decode(response.body) as Map<String, dynamic>;
      final status = data['status'] as String? ?? '';
      if (status != 'OK' && status != 'ZERO_RESULTS') {
        _log.severe('Google Geocoding API status: $status');
        return [];
      }

      final results = (data['results'] as List? ?? [])
          .cast<Map<String, dynamic>>()
          .where((r) => !_isPlusCodeOnly(_typesOf(r)))
          .map(_resultFromGeocoding)
          .toList();

      _log.info('Found ${results.length} area results');
      return results;
    } catch (e, stackTrace) {
      _log.severe('Error searching areas', e, stackTrace);
      return [];
    }
  }

  @override
  Future<LocationResult?> resolveSuggestion(
    LocationResult suggestion, {
    String? sessionToken,
  }) async {
    final placeId = suggestion.externalPlaceId;
    if (suggestion.externalPlaceProvider != googleMapsProvider ||
        placeId.isEmpty) {
      // Not a Google suggestion (or no place id to resolve) — caller
      // expects a hydrated result, return whatever they passed in.
      return suggestion;
    }
    if (suggestion.latitude != 0.0 || suggestion.longitude != 0.0) {
      // Already hydrated. Geographic-area searches go through Geocoding
      // (not Autocomplete) and return populated coordinates inline;
      // skipping the Place Details round-trip saves the call entirely.
      return suggestion;
    }
    try {
      final params = <String, String>{
        if (sessionToken != null && sessionToken.isNotEmpty)
          'sessionToken': sessionToken,
      };
      final uri = Uri.parse('$_placeDetailsBaseUrl${Uri.encodeComponent(placeId)}')
          .replace(queryParameters: params.isEmpty ? null : params);

      _log.info('Google place details: $placeId');

      final response = await _http.get(
        uri,
        headers: {
          'X-Goog-Api-Key': _apiKey,
          'X-Goog-FieldMask': _placeDetailsFieldMask,
        },
      );

      if (response.statusCode != 200) {
        _log.severe(
          'Failed to fetch place details: ${response.statusCode}\n'
          'Response body: ${response.body}',
        );
        return null;
      }

      final data = json.decode(response.body) as Map<String, dynamic>;
      return _resultFromPlaceDetails(data, fallbackName: suggestion.name);
    } catch (e, stackTrace) {
      _log.severe('Error fetching place details', e, stackTrace);
      return null;
    }
  }

  @override
  Future<LocationResult?> reverseGeocode(
    double latitude,
    double longitude,
  ) async {
    try {
      final params = <String, String>{
        'latlng': '$latitude,$longitude',
        'result_type': _reverseResultTypes,
        'key': _apiKey,
      };

      _log.info('Google reverse geocoding: $latitude, $longitude');

      final uri = Uri.parse(_geocodingUrl).replace(queryParameters: params);
      final response = await _http.get(uri);

      if (response.statusCode != 200) {
        _log.severe(
          'Failed to reverse geocode: ${response.statusCode}\n'
          'Response body: ${response.body}',
        );
        return null;
      }

      final data = json.decode(response.body) as Map<String, dynamic>;
      final status = data['status'] as String? ?? '';
      if (status != 'OK' && status != 'ZERO_RESULTS') {
        _log.severe('Google Geocoding API status: $status');
        return null;
      }

      final raw = (data['results'] as List? ?? []).cast<Map<String, dynamic>>();
      final best = raw.cast<Map<String, dynamic>?>().firstWhere(
            (r) => r != null && !_isPlusCodeOnly(_typesOf(r)),
            orElse: () => null,
          );
      if (best == null) {
        _log.warning('No usable reverse-geocode result (all Plus Codes or empty)');
        return null;
      }

      final result = _resultFromGeocoding(best);
      _log.info('Reverse geocoded to: ${result.fullName}');
      return result;
    } catch (e, stackTrace) {
      _log.severe('Error reverse geocoding', e, stackTrace);
      return null;
    }
  }

  /// _suggestionFromPrediction builds a partially-populated [LocationResult]
  /// from a Places Autocomplete (New) `placePrediction`. The coordinate and
  /// admin-hierarchy fields are left empty (zero / blank) — they get filled
  /// in by [resolveSuggestion] when the user picks this suggestion.
  static LocationResult _suggestionFromPrediction(
    Map<String, dynamic> pred,
    String placeId,
  ) {
    final text = pred['text'] as Map<String, dynamic>?;
    final structured = pred['structuredFormat'] as Map<String, dynamic>?;
    final mainText =
        (structured?['mainText'] as Map<String, dynamic>?)?['text'] as String?;
    final secondaryText =
        (structured?['secondaryText'] as Map<String, dynamic>?)?['text']
            as String?;
    final fullText = text?['text'] as String? ?? '';

    final displayName = mainText ?? fullText;
    final fullName = secondaryText == null || secondaryText.isEmpty
        ? (fullText.isNotEmpty ? fullText : displayName)
        : (mainText != null && mainText.isNotEmpty
            ? '$mainText, $secondaryText'
            : fullText.isNotEmpty
                ? fullText
                : secondaryText);

    final types = (pred['types'] as List? ?? const []).cast<String>();
    return LocationResult(
      name: displayName,
      fullName: fullName,
      type: _typeForResult(types),
      latitude: 0,
      longitude: 0,
      streetAddress: '',
      locality: '',
      region: '',
      postcode: '',
      regionCode: '',
      externalPlaceId: placeId,
      externalPlaceProvider: googleMapsProvider,
    );
  }

  /// _resultFromPlaceDetails builds a fully-populated [LocationResult] from
  /// a Place Details (New) response. Falls back to [fallbackName] for the
  /// display name when the response omits displayName (e.g. for pure-address
  /// places).
  static LocationResult _resultFromPlaceDetails(
    Map<String, dynamic> d, {
    String fallbackName = '',
  }) {
    final location = d['location'] as Map<String, dynamic>? ?? const {};
    final lat = (location['latitude'] as num?)?.toDouble() ?? 0.0;
    final lng = (location['longitude'] as num?)?.toDouble() ?? 0.0;
    final displayName =
        (d['displayName'] as Map<String, dynamic>?)?['text'] as String? ?? '';
    final formattedAddress = d['formattedAddress'] as String? ?? '';
    final placeId = d['id'] as String? ?? '';
    final types = (d['types'] as List? ?? const []).cast<String>();

    String streetNumber = '';
    String route = '';
    String locality = '';
    String region = '';
    String postcode = '';
    String regionCode = '';
    String country = '';
    String countryCode = '';

    for (final c in (d['addressComponents'] as List? ?? const [])
        .cast<Map<String, dynamic>>()) {
      final ctypes = (c['types'] as List? ?? const []).cast<String>();
      final long = c['longText'] as String? ?? '';
      final short = c['shortText'] as String? ?? '';
      if (ctypes.contains('street_number')) {
        streetNumber = long;
      } else if (ctypes.contains('route')) {
        route = long;
      } else if (ctypes.contains('locality')) {
        locality = long;
      } else if (ctypes.contains('postal_code')) {
        postcode = long;
      } else if (ctypes.contains('administrative_area_level_1')) {
        region = long;
        regionCode = short.toUpperCase();
      } else if (ctypes.contains('country')) {
        country = long;
        countryCode = short.toUpperCase();
      }
    }
    if (regionCode.isEmpty) {
      regionCode = countryCode;
    }

    final streetAddress =
        [streetNumber, route].where((s) => s.isNotEmpty).join(' ');

    String name;
    if (displayName.isNotEmpty) {
      name = displayName;
    } else if (streetAddress.isNotEmpty) {
      name = streetAddress;
    } else if (locality.isNotEmpty) {
      name = locality;
    } else if (formattedAddress.isNotEmpty) {
      name = formattedAddress;
    } else if (fallbackName.isNotEmpty) {
      name = fallbackName;
    } else {
      name = country;
    }

    final fullName =
        formattedAddress.isNotEmpty ? formattedAddress : name;

    return LocationResult(
      name: name,
      fullName: fullName,
      type: _typeForResult(types),
      latitude: lat,
      longitude: lng,
      streetAddress: streetAddress,
      locality: locality,
      region: region,
      postcode: postcode,
      regionCode: regionCode,
      externalPlaceId: placeId,
      externalPlaceProvider: placeId.isNotEmpty ? googleMapsProvider : '',
    );
  }

  /// _resultFromGeocoding builds a LocationResult from a Google Geocoding
  /// API result entry. Pulls administrative-hierarchy fields from
  /// address_components by walking the type tags.
  static LocationResult _resultFromGeocoding(Map<String, dynamic> r) {
    final geometry = r['geometry'] as Map<String, dynamic>? ?? {};
    final location = geometry['location'] as Map<String, dynamic>? ?? {};
    final lat = (location['lat'] as num?)?.toDouble() ?? 0.0;
    final lng = (location['lng'] as num?)?.toDouble() ?? 0.0;

    final formattedAddress = r['formatted_address'] as String? ?? '';
    final placeId = r['place_id'] as String? ?? '';
    final types = _typesOf(r);

    String streetNumber = '';
    String route = '';
    String locality = '';
    String region = '';
    String postcode = '';
    String regionCode = '';
    String country = '';
    String countryCode = '';

    for (final c in (r['address_components'] as List? ?? []).cast<Map<String, dynamic>>()) {
      final ctypes = (c['types'] as List? ?? []).cast<String>();
      final long = c['long_name'] as String? ?? '';
      final short = c['short_name'] as String? ?? '';

      if (ctypes.contains('street_number')) {
        streetNumber = long;
      } else if (ctypes.contains('route')) {
        route = long;
      } else if (ctypes.contains('locality')) {
        locality = long;
      } else if (ctypes.contains('postal_code')) {
        postcode = long;
      } else if (ctypes.contains('administrative_area_level_1')) {
        region = long;
        regionCode = short.toUpperCase();
      } else if (ctypes.contains('country')) {
        country = long;
        countryCode = short.toUpperCase();
      }
    }

    if (regionCode.isEmpty) {
      regionCode = countryCode;
    }

    final streetAddress = [streetNumber, route].where((s) => s.isNotEmpty).join(' ');

    String name;
    if (streetAddress.isNotEmpty) {
      name = streetAddress;
    } else if (locality.isNotEmpty) {
      name = locality;
    } else if (formattedAddress.isNotEmpty) {
      name = formattedAddress;
    } else {
      name = country;
    }

    // Build a display-friendly name. Geographic-area results have no
    // street_address component, so the formatted address is already a
    // good display string.
    final fullName = formattedAddress.isNotEmpty ? formattedAddress : name;

    return LocationResult(
      name: name,
      fullName: fullName,
      type: _typeForResult(types),
      latitude: lat,
      longitude: lng,
      streetAddress: streetAddress,
      locality: locality,
      region: region,
      postcode: postcode,
      regionCode: regionCode,
      externalPlaceId: placeId,
      externalPlaceProvider: placeId.isNotEmpty ? googleMapsProvider : '',
    );
  }

  /// _typesOf extracts the types array from a Geocoding result, defaulting
  /// to an empty list when missing.
  static List<String> _typesOf(Map<String, dynamic> result) {
    final t = result['types'];
    if (t is List) {
      return t.cast<String>();
    }
    return const [];
  }

  /// _isPlusCodeOnly reports whether the type set consists exclusively of
  /// plus_code-related entries. Plus Codes are opaque strings users can't
  /// act on, so we drop them per the parity audit (#2188).
  static bool _isPlusCodeOnly(List<String> types) {
    if (types.isEmpty) {
      return false;
    }
    for (final t in types) {
      if (t != 'plus_code' && t != 'compound_plus_code' && t != 'global_plus_code') {
        return false;
      }
    }
    return true;
  }

  /// _typeForResult maps a Google types array onto the LocationResult.Type
  /// taxonomy callers expect (Address, POI, City, Locality, District,
  /// Region, Country, Place).
  static String _typeForResult(List<String> types) {
    for (final t in types) {
      switch (t) {
        case 'street_address':
        case 'premise':
        case 'subpremise':
        case 'route':
          return 'Address';
        case 'establishment':
        case 'point_of_interest':
          return 'POI';
        case 'locality':
          return 'City';
        case 'sublocality':
        case 'neighborhood':
          return 'Locality';
        case 'administrative_area_level_1':
          return 'Region';
        case 'administrative_area_level_2':
          return 'District';
        case 'country':
          return 'Country';
      }
    }
    return 'Place';
  }
}
