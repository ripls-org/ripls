import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:ripls/services/google_location_search_service.dart';
import 'package:ripls/services/location_search_service.dart';

/// boulder is a fixed Position used as the proximity hint in tests that
/// exercise the bias path.
final boulder = Position(
  latitude: 40.0150,
  longitude: -105.2705,
  timestamp: DateTime(2026, 5, 27),
  accuracy: 0,
  altitude: 0,
  altitudeAccuracy: 0,
  heading: 0,
  headingAccuracy: 0,
  speed: 0,
  speedAccuracy: 0,
);

/// makeClient wires a GoogleLocationSearchService whose HTTP layer returns
/// a fixed JSON response. The captured request is exposed to the test via
/// the onRequest callback so assertions can inspect URI, method, body, and
/// headers.
GoogleLocationSearchService makeClient(
  String jsonBody, {
  void Function(http.Request req)? onRequest,
  int statusCode = 200,
}) {
  final mock = MockClient((req) async {
    onRequest?.call(req);
    return http.Response(jsonBody, statusCode,
        headers: {'content-type': 'application/json'});
  });
  return GoogleLocationSearchService(httpClient: mock, apiKey: 'test-key');
}

void main() {
  group('GoogleLocationSearchService.searchAddresses', () {
    test('returns unhydrated suggestions from Places Autocomplete (New)',
        () async {
      const body = '''
      {
        "suggestions": [
          {
            "placePrediction": {
              "placeId": "ChIJZilkerPark",
              "text": {"text": "Zilker Park, Austin, TX, USA"},
              "structuredFormat": {
                "mainText": {"text": "Zilker Park"},
                "secondaryText": {"text": "Austin, TX, USA"}
              },
              "types": ["park", "establishment", "point_of_interest"]
            }
          }
        ]
      }
      ''';

      http.Request? captured;
      final service = makeClient(body, onRequest: (r) => captured = r);
      final results = await service.searchAddresses('zilker park');

      expect(results, hasLength(1));
      final r = results.first;
      expect(r.name, 'Zilker Park');
      expect(r.fullName, contains('Austin'));
      expect(r.type, 'POI');
      expect(r.externalPlaceId, 'ChIJZilkerPark');
      expect(r.externalPlaceProvider, googleMapsProvider);
      // Lazy resolution: coordinates and address fields are blank until
      // resolveSuggestion() runs.
      expect(r.latitude, 0.0);
      expect(r.longitude, 0.0);
      expect(r.streetAddress, isEmpty);
      expect(r.locality, isEmpty);

      // Verify the wire call shape: POST to the Places host with the key
      // in the X-Goog-Api-Key header.
      expect(captured, isNotNull);
      expect(captured!.method, 'POST');
      expect(
        captured!.url.toString(),
        startsWith('https://places.googleapis.com/v1/places:autocomplete'),
      );
      expect(captured!.headers['X-Goog-Api-Key'], 'test-key');
      final decoded = json.decode(captured!.body) as Map<String, dynamic>;
      expect(decoded['input'], 'zilker park');
    });

    test('empty query returns empty without an HTTP call', () async {
      var called = false;
      final service = makeClient('{}', onRequest: (_) => called = true);
      final results = await service.searchAddresses('   ');
      expect(results, isEmpty);
      expect(called, isFalse);
    });

    test('adds locationBias circle when proximity supplied', () async {
      http.Request? captured;
      final service = makeClient(
        '{"suggestions": []}',
        onRequest: (r) => captured = r,
      );
      await service.searchAddresses('cafe', proximity: boulder);

      expect(captured, isNotNull);
      final decoded = json.decode(captured!.body) as Map<String, dynamic>;
      expect(decoded['locationBias'], isA<Map<String, dynamic>>());
      final bias = decoded['locationBias'] as Map<String, dynamic>;
      expect(bias['circle'], isA<Map<String, dynamic>>());
      final center = (bias['circle']
              as Map<String, dynamic>)['center'] as Map<String, dynamic>;
      expect(center['latitude'], boulder.latitude);
      expect(center['longitude'], boulder.longitude);
    });

    test('threads session token into the request body', () async {
      http.Request? captured;
      final service = makeClient(
        '{"suggestions": []}',
        onRequest: (r) => captured = r,
      );
      await service.searchAddresses('cafe', sessionToken: 'abc-123');

      final decoded = json.decode(captured!.body) as Map<String, dynamic>;
      expect(decoded['sessionToken'], 'abc-123');
    });

    test('HTTP error returns empty list (no throw)', () async {
      final service = makeClient('upstream error', statusCode: 500);
      final results = await service.searchAddresses('cafe');
      expect(results, isEmpty);
    });
  });

  group('GoogleLocationSearchService.resolveSuggestion', () {
    LocationResult googleSuggestion() => const LocationResult(
          name: 'Zilker Park',
          fullName: 'Zilker Park, Austin, TX, USA',
          type: 'POI',
          latitude: 0,
          longitude: 0,
          streetAddress: '',
          locality: '',
          region: '',
          postcode: '',
          regionCode: '',
          externalPlaceId: 'ChIJZilkerPark',
          externalPlaceProvider: googleMapsProvider,
        );

    test('hydrates from Place Details (New) with field mask + session token',
        () async {
      const body = '''
      {
        "id": "ChIJZilkerPark",
        "displayName": {"text": "Zilker Park"},
        "formattedAddress": "2207 Lou Neff Rd, Austin, TX 78746, USA",
        "location": {"latitude": 30.2669, "longitude": -97.7728},
        "types": ["park", "establishment", "point_of_interest"],
        "addressComponents": [
          {"longText": "2207", "shortText": "2207", "types": ["street_number"]},
          {"longText": "Lou Neff Road", "shortText": "Lou Neff Rd", "types": ["route"]},
          {"longText": "Austin", "shortText": "Austin", "types": ["locality"]},
          {"longText": "Texas", "shortText": "TX", "types": ["administrative_area_level_1"]},
          {"longText": "United States", "shortText": "US", "types": ["country"]}
        ]
      }
      ''';

      http.Request? captured;
      final service = makeClient(body, onRequest: (r) => captured = r);
      final hydrated = await service.resolveSuggestion(
        googleSuggestion(),
        sessionToken: 'sess-1',
      );

      expect(hydrated, isNotNull);
      expect(hydrated!.latitude, 30.2669);
      expect(hydrated.longitude, -97.7728);
      expect(hydrated.streetAddress, '2207 Lou Neff Road');
      expect(hydrated.locality, 'Austin');
      expect(hydrated.regionCode, 'TX');

      expect(captured, isNotNull);
      expect(captured!.method, 'GET');
      expect(
        captured!.url.toString(),
        startsWith(
          'https://places.googleapis.com/v1/places/ChIJZilkerPark',
        ),
      );
      expect(captured!.url.queryParameters['sessionToken'], 'sess-1');
      expect(captured!.headers['X-Goog-Api-Key'], 'test-key');
      expect(captured!.headers['X-Goog-FieldMask'], isNotNull);
      expect(
        captured!.headers['X-Goog-FieldMask']!,
        contains('addressComponents'),
      );
    });

    test('already-hydrated suggestion skips the HTTP call', () async {
      var called = false;
      final service = makeClient('{}', onRequest: (_) => called = true);
      final hydrated = await service.resolveSuggestion(
        const LocationResult(
          name: 'Boulder',
          fullName: 'Boulder, CO, USA',
          type: 'City',
          latitude: 40.0150,
          longitude: -105.2705,
          streetAddress: '',
          locality: 'Boulder',
          region: 'Colorado',
          postcode: '',
          regionCode: 'CO',
          externalPlaceId: 'ChIJBoulder',
          externalPlaceProvider: googleMapsProvider,
        ),
      );
      expect(hydrated, isNotNull);
      expect(hydrated!.latitude, 40.0150);
      expect(called, isFalse, reason: 'No HTTP call for hydrated suggestion');
    });

    test('non-Google suggestion returns input unchanged without HTTP', () async {
      var called = false;
      final service = makeClient('{}', onRequest: (_) => called = true);
      final input = const LocationResult(
        name: 'Some place',
        fullName: 'Some place',
        type: 'POI',
        latitude: 0,
        longitude: 0,
        streetAddress: '',
        locality: '',
        region: '',
        postcode: '',
        regionCode: '',
        externalPlaceId: 'mapbox-id',
        externalPlaceProvider: 'mapbox',
      );
      final out = await service.resolveSuggestion(input);
      expect(out, same(input));
      expect(called, isFalse);
    });

    test('HTTP error returns null (no throw)', () async {
      final service = makeClient('upstream', statusCode: 500);
      final out = await service.resolveSuggestion(googleSuggestion());
      expect(out, isNull);
    });
  });

  group('GoogleLocationSearchService.searchGeographicAreas', () {
    test('sends area-only result_type filter', () async {
      http.Request? captured;
      final service = makeClient(
        '{"status": "OK", "results": []}',
        onRequest: (r) => captured = r,
      );
      await service.searchGeographicAreas('Boulder');
      expect(captured, isNotNull);
      final resultType = captured!.url.queryParameters['result_type'];
      expect(resultType, isNotNull);
      expect(resultType, contains('locality'));
      expect(resultType, contains('administrative_area_level_1'));
      expect(resultType, contains('country'));
      // Geographic-only filter must not include street_address / route /
      // POI types.
      expect(resultType, isNot(contains('street_address')));
      expect(resultType, isNot(contains('route')));
    });
  });

  group('GoogleLocationSearchService.reverseGeocode', () {
    test('uses result_type filter and returns address', () async {
      const body = '''
      {
        "status": "OK",
        "results": [{
          "formatted_address": "1200 Block Pearl St, Boulder, CO 80302, USA",
          "geometry": {"location": {"lat": 40.0177, "lng": -105.2799}},
          "place_id": "ChIJ_pearl",
          "types": ["route"],
          "address_components": [
            {"long_name": "Pearl Street", "short_name": "Pearl St", "types": ["route"]},
            {"long_name": "Boulder", "short_name": "Boulder", "types": ["locality", "political"]},
            {"long_name": "Colorado", "short_name": "CO", "types": ["administrative_area_level_1", "political"]}
          ]
        }]
      }
      ''';

      http.Request? captured;
      final service = makeClient(body, onRequest: (r) => captured = r);
      final result = await service.reverseGeocode(40.0177, -105.2799);

      expect(result, isNotNull);
      expect(captured!.url.queryParameters['result_type'],
          contains('street_address'));
      expect(result!.locality, 'Boulder');
      expect(result.regionCode, 'CO');
    });

    test('Plus-Code-only response returns null', () async {
      const body = '''
      {
        "status": "OK",
        "results": [{
          "formatted_address": "H629+25 Keystone, CO, USA",
          "geometry": {"location": {"lat": 39.6, "lng": -105.9}},
          "place_id": "GhIJ_pluscode",
          "types": ["plus_code"],
          "address_components": []
        }]
      }
      ''';
      final service = makeClient(body);
      final result = await service.reverseGeocode(39.6, -105.9);
      expect(result, isNull);
    });

    test('ZERO_RESULTS returns null', () async {
      final service = makeClient('{"status": "ZERO_RESULTS", "results": []}');
      final result = await service.reverseGeocode(0, 0);
      expect(result, isNull);
    });

    test('HTTP error returns null (no throw)', () async {
      final service = makeClient('upstream', statusCode: 500);
      final result = await service.reverseGeocode(40, -105);
      expect(result, isNull);
    });
  });

  group('GoogleLocationSearchService no application-restriction headers', () {
    tearDown(() {
      debugDefaultTargetPlatformOverride = null;
    });

    // The client key (google-maps-api-key-client-places) is API-restricted
    // but NOT application-restricted, so no per-platform request headers
    // are sent. Sending the colon-form X-Android-Cert was what broke
    // Android autocomplete in prod (#2246); the whole mechanism is gone.
    for (final platform in [TargetPlatform.android, TargetPlatform.iOS]) {
      test('no platform restriction headers on $platform', () async {
        debugDefaultTargetPlatformOverride = platform;

        http.Request? captured;
        final service = makeClient(
          '{"suggestions": []}',
          onRequest: (r) => captured = r,
        );
        await service.searchAddresses('cafe');
        expect(captured!.headers.containsKey('X-Android-Package'), isFalse);
        expect(captured!.headers.containsKey('X-Android-Cert'), isFalse);
        expect(
          captured!.headers.containsKey('X-Ios-Bundle-Identifier'),
          isFalse,
        );
        // The API key still rides on the authenticated Places call.
        expect(captured!.headers['X-Goog-Api-Key'], 'test-key');
      });
    }
  });
}
