import 'dart:convert';
import 'package:geolocator/geolocator.dart';
import 'package:http/http.dart' as http;
import 'package:logging/logging.dart';
import '../core/config/environment.dart';
import 'location_search_service.dart';

// Re-export LocationResult so existing call sites that import this file
// continue to compile. New code should import location_search_service.dart
// directly.
export 'location_search_service.dart' show LocationResult;

final _log = Logger('MapboxLocationService');

/// Canonical value for [LocationResult.externalPlaceProvider] when the
/// place came from the Mapbox APIs.
const String mapboxProvider = 'mapbox';

/// Mapbox-backed implementation of [LocationSearchService] using the
/// Mapbox Search Box API for autocomplete and the Mapbox Geocoding API
/// for reverse geocoding.
class MapboxLocationService implements LocationSearchService {
  static const String _baseUrl =
      'https://api.mapbox.com/search/searchbox/v1/forward';
  static const String _reverseUrl =
      'https://api.mapbox.com/search/geocode/v6/reverse';

  final http.Client _http;
  final String _accessToken;

  MapboxLocationService({http.Client? httpClient, String? accessToken})
      : _http = httpClient ?? http.Client(),
        _accessToken = accessToken ?? Environment.mapboxAccessToken;

  /// The `features` array of a Mapbox Search/Geocoding response body, as
  /// feature objects. Mapbox omits keys rather than nulling them, so every
  /// field read downstream stays nullable.
  static List<Map<String, dynamic>> _features(String body) {
    final data = json.decode(body) as Map<String, dynamic>;
    final features = data['features'] as List<dynamic>? ?? const [];
    return features.cast<Map<String, dynamic>>();
  }

  /// Reads `context[key][field]` out of a feature's `context` object, which
  /// is a map of loosely-typed sub-objects (`place`, `region`, `country`, …).
  /// Null when either level is missing or is not the shape Mapbox documents.
  static String? _contextField(
    Map<String, dynamic>? context,
    String key,
    String field,
  ) {
    final entry = context?[key];
    return entry is Map<String, dynamic> ? entry[field] as String? : null;
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
        'q': query,
        'access_token': _accessToken,
        'types': 'place,locality,district,region,country',
        'limit': '10',
        'auto_complete': 'true',
      };

      if (proximity != null) {
        params['proximity'] = '${proximity.longitude},${proximity.latitude}';
        _log.info(
          'Searching for geographic areas near: ${proximity.latitude}, ${proximity.longitude}',
        );
      }

      _log.info('Searching for geographic areas: $query');

      final uri = Uri.parse(_baseUrl).replace(queryParameters: params);
      final response = await _http.get(uri);

      if (response.statusCode != 200) {
        _log.severe(
          'Failed to search geographic areas: ${response.statusCode}\n'
          'Response body: ${response.body}\n'
          'URL: $uri',
        );
        return [];
      }

      final features = _features(response.body);

      _log.info('Found ${features.length} geographic area results');

      final results = features.map((feature) {
        final properties = feature['properties'] as Map<String, dynamic>;
        final geometry = feature['geometry'] as Map<String, dynamic>;
        final coordinates = geometry['coordinates'] as List;

        final featureType = properties['feature_type'] as String? ?? 'place';
        String type = 'Place';
        if (featureType == 'place' || featureType == 'city') {
          type = 'City';
        } else if (featureType == 'locality') {
          type = 'Locality';
        } else if (featureType == 'district' || featureType == 'neighborhood') {
          type = 'District';
        } else if (featureType == 'region') {
          type = 'Region';
        } else if (featureType == 'country') {
          type = 'Country';
        }

        final name = properties['name'] as String? ?? '';
        final apiFullAddress = properties['full_address'] as String? ??
                           properties['place_formatted'] as String? ??
                           '';

        final mapboxId = properties['mapbox_id'] as String? ?? '';

        final context = properties['context'] as Map<String, dynamic>?;
        String locality = '';
        String region = '';
        String postcode = '';
        String regionCode = '';
        String country = '';

        if (context != null) {
          locality = _contextField(context, 'place', 'name') ??
              _contextField(context, 'locality', 'name') ??
              '';
          region = _contextField(context, 'region', 'name') ?? '';
          postcode = _contextField(context, 'postcode', 'name') ?? '';
          regionCode = _contextField(context, 'country', 'country_code') ?? '';
          country = _contextField(context, 'country', 'name') ?? '';
          if (regionCode.isNotEmpty) {
            regionCode = regionCode.toUpperCase();
          }
        }

        // For city/place/locality type features, the name IS the locality
        if ((type == 'City' || type == 'Locality' || type == 'Place') && locality.isEmpty) {
          locality = name;
        }

        // Build a complete display name for geographic areas
        // Format: "Boulder, Colorado, United States"
        final displayParts = <String>[];
        if (name.isNotEmpty) {
          displayParts.add(name);
        }
        if (region.isNotEmpty) {
          displayParts.add(region);
        }
        if (country.isNotEmpty) {
          displayParts.add(country);
        }
        final fullAddress = displayParts.isNotEmpty
            ? displayParts.join(', ')
            : (apiFullAddress.isNotEmpty ? apiFullAddress : name);

        final result = LocationResult(
          name: name,
          fullName: fullAddress,
          type: type,
          latitude: coordinates[1] as double,
          longitude: coordinates[0] as double,
          streetAddress: '', // No street addresses for geographic areas
          locality: locality,
          region: region,
          postcode: postcode,
          regionCode: regionCode,
          externalPlaceId: mapboxId,
          externalPlaceProvider: mapboxId.isNotEmpty ? mapboxProvider : '',
        );

        _log.info(
          'Geographic area: ${result.name} (${result.type}) - ${result.locality}, ${result.region}',
        );

        return result;
      }).toList();

      return results;
    } catch (e, stackTrace) {
      _log.severe('Error searching geographic areas', e, stackTrace);
      return [];
    }
  }

  @override
  Future<LocationResult?> reverseGeocode(
    double latitude,
    double longitude,
  ) async {
    try {
      final params = <String, String>{
        'longitude': longitude.toString(),
        'latitude': latitude.toString(),
        'access_token': _accessToken,
        'types': 'place,address,locality,neighborhood,street',
        'limit': '1',
      };

      _log.info('Reverse geocoding: $latitude, $longitude');

      final uri = Uri.parse(_reverseUrl).replace(queryParameters: params);
      final response = await _http.get(uri);

      if (response.statusCode != 200) {
        _log.severe(
          'Failed to reverse geocode: ${response.statusCode}\n'
          'Response body: ${response.body}\n'
          'URL: $uri',
        );
        return null;
      }

      final features = _features(response.body);

      if (features.isEmpty) {
        _log.warning('No reverse geocode results found');
        return null;
      }

      final feature = features[0];
      final properties = feature['properties'] as Map<String, dynamic>;
      final geometry = feature['geometry'] as Map<String, dynamic>;
      final coordinates = geometry['coordinates'] as List;

      _log.info('Reverse geocode response - feature_type: ${properties['feature_type']}, '
          'name: ${properties['name']}, '
          'full_address: ${properties['full_address']}, '
          'place_formatted: ${properties['place_formatted']}');

      final featureType = properties['feature_type'] as String? ?? 'address';
      String type = 'Address';
      if (featureType == 'place') {
        type = 'Place';
      } else if (featureType == 'locality') {
        type = 'Locality';
      } else if (featureType == 'neighborhood') {
        type = 'Neighborhood';
      } else if (featureType == 'street') {
        type = 'Street';
      }

      final name = properties['name'] as String? ?? '';
      final fullAddress = properties['full_address'] as String? ??
                         properties['place_formatted'] as String? ??
                         name;

      final mapboxId = properties['mapbox_id'] as String? ?? '';

      final context = properties['context'] as Map<String, dynamic>?;
      final String streetAddress = properties['address'] as String? ?? '';
      String locality = '';
      String region = '';
      String postcode = '';
      String regionCode = '';

      if (context != null) {
        locality = _contextField(context, 'place', 'name') ??
            _contextField(context, 'locality', 'name') ??
            '';
        region = _contextField(context, 'region', 'name') ?? '';
        postcode = _contextField(context, 'postcode', 'name') ?? '';
        regionCode = _contextField(context, 'country', 'country_code') ?? '';
        if (regionCode.isNotEmpty) {
          regionCode = regionCode.toUpperCase();
        }
      }

      final result = LocationResult(
        name: name,
        fullName: fullAddress,
        type: type,
        latitude: coordinates[1] as double,
        longitude: coordinates[0] as double,
        streetAddress: streetAddress,
        locality: locality,
        region: region,
        postcode: postcode,
        regionCode: regionCode,
        externalPlaceId: mapboxId,
        externalPlaceProvider: mapboxId.isNotEmpty ? mapboxProvider : '',
      );

      _log.info('Reverse geocoded to: ${result.fullName}');
      return result;
    } catch (e, stackTrace) {
      _log.severe('Error reverse geocoding', e, stackTrace);
      return null;
    }
  }

  @override
  Future<LocationResult?> resolveSuggestion(
    LocationResult suggestion, {
    String? sessionToken,
  }) async {
    // Mapbox suggestions already carry full coordinate + address fields,
    // so no second-leg fetch is needed. Returned verbatim.
    return suggestion;
  }

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

      final params = <String, String>{
        'q': query,
        'access_token': _accessToken,
        'types': 'poi,address,place,locality',
        'limit': '10',
        'auto_complete': 'true',
      };

      if (proximity != null) {
        params['proximity'] = '${proximity.longitude},${proximity.latitude}';
        _log.info(
          'Searching for addresses near: ${proximity.latitude}, ${proximity.longitude}',
        );
      }

      _log.info('Searching for addresses: $query');

      final uri = Uri.parse(_baseUrl).replace(queryParameters: params);
      final response = await _http.get(uri);

      if (response.statusCode != 200) {
        _log.severe(
          'Failed to search addresses: ${response.statusCode}\n'
          'Response body: ${response.body}\n'
          'URL: $uri',
        );
        return [];
      }

      final features = _features(response.body);

      _log.info('Found ${features.length} results');

      final results = features.map((feature) {
        final properties = feature['properties'] as Map<String, dynamic>;
        final geometry = feature['geometry'] as Map<String, dynamic>;
        final coordinates = geometry['coordinates'] as List;

        final featureType = properties['feature_type'] as String? ?? 'place';
        String type = 'Place';
        if (featureType == 'address') {
          type = 'Address';
        } else if (featureType == 'poi') {
          type = 'POI';
        } else if (featureType == 'place' || featureType == 'city') {
          type = 'City';
        } else if (featureType == 'locality') {
          type = 'Locality';
        }

        final name = properties['name'] as String? ?? '';
        final fullAddress = properties['full_address'] as String? ??
                           properties['place_formatted'] as String? ??
                           name;

        final mapboxId = properties['mapbox_id'] as String? ?? '';

        final context = properties['context'] as Map<String, dynamic>?;
        final String streetAddress = properties['address'] as String? ?? '';
        String locality = '';
        String region = '';
        String postcode = '';
        String regionCode = '';

        if (context != null) {
          locality = _contextField(context, 'place', 'name') ??
              _contextField(context, 'locality', 'name') ??
              '';
          region = _contextField(context, 'region', 'name') ?? '';
          postcode = _contextField(context, 'postcode', 'name') ?? '';
          regionCode = _contextField(context, 'country', 'country_code') ?? '';
          if (regionCode.isNotEmpty) {
            regionCode = regionCode.toUpperCase();
          }
        }

        final result = LocationResult(
          name: name,
          fullName: fullAddress,
          type: type,
          latitude: coordinates[1] as double,
          longitude: coordinates[0] as double,
          streetAddress: streetAddress,
          locality: locality,
          region: region,
          postcode: postcode,
          regionCode: regionCode,
          externalPlaceId: mapboxId,
          externalPlaceProvider: mapboxId.isNotEmpty ? mapboxProvider : '',
        );

        _log.info(
          'Result: ${result.name} (${result.type}) - ${result.locality}, ${result.region} '
          '(${result.latitude}, ${result.longitude})',
        );

        return result;
      }).toList();

      return results;
    } catch (e, stackTrace) {
      _log.severe('Error searching addresses', e, stackTrace);
      return [];
    }
  }
}
