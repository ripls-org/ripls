import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart' show Position;
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart' show Location;
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/presentation/viewmodels/location_picker_view_model.dart';
import 'package:ripls/services/mapbox_location_service.dart';
import 'package:ripls/services/providers.dart';

import 'location_picker_view_model_test.mocks.dart';

@GenerateMocks([LocationRepository])
void main() {
  group('LocationPickerNotifier', () {
    late MockLocationRepository mockLocationRepository;
    late ProviderContainer container;

    setUp(() {
      mockLocationRepository = MockLocationRepository();

      container = ProviderContainer(
        overrides: [
          locationRepositoryProvider.overrideWithValue(mockLocationRepository),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    group('saveLocation', () {
      test('preserves POI name when saving location', () async {
        // Arrange
        const testLocationId = 'test-location-id';
        const testUserId = 'test-user-id';
        const poiName = 'North Boulder Park';

        final locationResult = LocationResult(
          name: poiName,
          fullName: 'North Boulder Park, Boulder, CO 80304',
          type: 'place',
          latitude: 40.027598,
          longitude: -105.286678,
          streetAddress: '',
          locality: 'Boulder',
          region: '',
          postcode: '80304',
          regionCode: 'CO',
        );

        final savedLocation = Location(
          id: testLocationId,
          name: poiName,
          latitudeDeg: 40.027598,
          longitudeDeg: -105.286678,
          locality: 'Boulder',
          regionCode: 'CO',
          postalCode: '80304',
        );

        // Mock the repository calls
        when(mockLocationRepository.saveLocation(
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          regionCode: anyNamed('regionCode'),
          postalCode: anyNamed('postalCode'),
          locality: anyNamed('locality'),
          addressLines: anyNamed('addressLines'),
          name: anyNamed('name'),
          userId: anyNamed('userId'),
        )).thenAnswer((_) async => testLocationId);

        when(mockLocationRepository.getLocation(testLocationId))
            .thenAnswer((_) async => savedLocation);

        // Act
        final notifier = container.read(locationPickerProvider.notifier);
        await notifier.saveLocation(
          selectedLocation: locationResult,
          userId: testUserId,
        );

        // Assert - Verify that name parameter was passed to saveLocation
        final verification = verify(mockLocationRepository.saveLocation(
          latitudeDeg: captureAnyNamed('latitudeDeg'),
          longitudeDeg: captureAnyNamed('longitudeDeg'),
          regionCode: captureAnyNamed('regionCode'),
          postalCode: captureAnyNamed('postalCode'),
          locality: captureAnyNamed('locality'),
          addressLines: captureAnyNamed('addressLines'),
          name: captureAnyNamed('name'),
          userId: captureAnyNamed('userId'),
        ));

        verification.called(1);

        // Verify the captured name parameter
        final capturedName = verification.captured[6] as String?;
        expect(capturedName, equals(poiName),
            reason: 'POI name should be preserved when saving location');

        // Verify other parameters
        expect(verification.captured[0], equals(40.027598)); // latitude
        expect(verification.captured[1], equals(-105.286678)); // longitude
        expect(verification.captured[2], equals('CO')); // regionCode
        expect(verification.captured[3], equals('80304')); // postalCode
        expect(verification.captured[4], equals('Boulder')); // locality
        expect(verification.captured[7], equals(testUserId)); // userId
      });

      test('passes null for name when LocationResult name is empty', () async {
        // Arrange
        const testLocationId = 'test-location-id';
        const testUserId = 'test-user-id';

        final locationResult = LocationResult(
          name: '', // Empty name
          fullName: 'Boulder, CO 80304',
          type: 'place',
          latitude: 40.027598,
          longitude: -105.286678,
          streetAddress: '',
          locality: 'Boulder',
          region: '',
          postcode: '80304',
          regionCode: 'CO',
        );

        final savedLocation = Location(
          id: testLocationId,
          latitudeDeg: 40.027598,
          longitudeDeg: -105.286678,
          locality: 'Boulder',
          regionCode: 'CO',
          postalCode: '80304',
        );

        when(mockLocationRepository.saveLocation(
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          regionCode: anyNamed('regionCode'),
          postalCode: anyNamed('postalCode'),
          locality: anyNamed('locality'),
          addressLines: anyNamed('addressLines'),
          name: anyNamed('name'),
          userId: anyNamed('userId'),
        )).thenAnswer((_) async => testLocationId);

        when(mockLocationRepository.getLocation(testLocationId))
            .thenAnswer((_) async => savedLocation);

        // Act
        final notifier = container.read(locationPickerProvider.notifier);
        await notifier.saveLocation(
          selectedLocation: locationResult,
          userId: testUserId,
        );

        // Assert - Verify that name parameter is null when empty
        final verification = verify(mockLocationRepository.saveLocation(
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          regionCode: anyNamed('regionCode'),
          postalCode: anyNamed('postalCode'),
          locality: anyNamed('locality'),
          addressLines: anyNamed('addressLines'),
          name: captureAnyNamed('name'),
          userId: anyNamed('userId'),
        ));

        verification.called(1);

        final capturedName = verification.captured[0] as String?;
        expect(capturedName, isNull,
            reason: 'Name should be null when LocationResult name is empty');
      });

      test('saves location with full address details', () async {
        // Arrange
        const testLocationId = 'test-location-id';
        const testUserId = 'test-user-id';
        const streetAddress = '1701 Bryant St';

        final locationResult = LocationResult(
          name: 'Empower Field at Mile High',
          fullName: 'Empower Field at Mile High, 1701 Bryant St, Denver, CO 80204',
          type: 'place',
          latitude: 39.7439,
          longitude: -105.0201,
          streetAddress: streetAddress,
          locality: 'Denver',
          region: 'Colorado',
          postcode: '80204',
          regionCode: 'CO',
        );

        final savedLocation = Location(
          id: testLocationId,
          name: 'Empower Field at Mile High',
          latitudeDeg: 39.7439,
          longitudeDeg: -105.0201,
          addressLines: [streetAddress],
          locality: 'Denver',
          regionCode: 'CO',
          postalCode: '80204',
        );

        when(mockLocationRepository.saveLocation(
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          regionCode: anyNamed('regionCode'),
          postalCode: anyNamed('postalCode'),
          locality: anyNamed('locality'),
          addressLines: anyNamed('addressLines'),
          name: anyNamed('name'),
          userId: anyNamed('userId'),
        )).thenAnswer((_) async => testLocationId);

        when(mockLocationRepository.getLocation(testLocationId))
            .thenAnswer((_) async => savedLocation);

        // Act
        final notifier = container.read(locationPickerProvider.notifier);
        await notifier.saveLocation(
          selectedLocation: locationResult,
          userId: testUserId,
        );

        // Assert
        final verification = verify(mockLocationRepository.saveLocation(
          latitudeDeg: captureAnyNamed('latitudeDeg'),
          longitudeDeg: captureAnyNamed('longitudeDeg'),
          regionCode: captureAnyNamed('regionCode'),
          postalCode: captureAnyNamed('postalCode'),
          locality: captureAnyNamed('locality'),
          addressLines: captureAnyNamed('addressLines'),
          name: captureAnyNamed('name'),
          userId: captureAnyNamed('userId'),
        ));

        verification.called(1);

        expect(verification.captured[0], equals(39.7439)); // latitude
        expect(verification.captured[1], equals(-105.0201)); // longitude
        expect(verification.captured[2], equals('CO')); // regionCode
        expect(verification.captured[3], equals('80204')); // postalCode
        expect(verification.captured[4], equals('Denver')); // locality
        expect(verification.captured[5], equals([streetAddress])); // addressLines
        expect(verification.captured[6], equals('Empower Field at Mile High')); // name
        expect(verification.captured[7], equals(testUserId)); // userId
      });

      test('passes null for addressLines when street address is empty', () async {
        // Arrange
        const testLocationId = 'test-location-id';
        const testUserId = 'test-user-id';

        final locationResult = LocationResult(
          name: 'Denver',
          fullName: 'Denver, CO 80204',
          type: 'place',
          latitude: 39.7392,
          longitude: -104.9903,
          streetAddress: '', // Empty street address
          locality: 'Denver',
          region: '',
          postcode: '80204',
          regionCode: 'CO',
        );

        final savedLocation = Location(
          id: testLocationId,
          name: 'Denver',
          latitudeDeg: 39.7392,
          longitudeDeg: -104.9903,
          locality: 'Denver',
          regionCode: 'CO',
          postalCode: '80204',
        );

        when(mockLocationRepository.saveLocation(
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          regionCode: anyNamed('regionCode'),
          postalCode: anyNamed('postalCode'),
          locality: anyNamed('locality'),
          addressLines: anyNamed('addressLines'),
          name: anyNamed('name'),
          userId: anyNamed('userId'),
        )).thenAnswer((_) async => testLocationId);

        when(mockLocationRepository.getLocation(testLocationId))
            .thenAnswer((_) async => savedLocation);

        // Act
        final notifier = container.read(locationPickerProvider.notifier);
        await notifier.saveLocation(
          selectedLocation: locationResult,
          userId: testUserId,
        );

        // Assert
        final verification = verify(mockLocationRepository.saveLocation(
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          regionCode: anyNamed('regionCode'),
          postalCode: anyNamed('postalCode'),
          locality: anyNamed('locality'),
          addressLines: captureAnyNamed('addressLines'),
          name: anyNamed('name'),
          userId: anyNamed('userId'),
        ));

        verification.called(1);

        final capturedAddressLines = verification.captured[0] as List<String>?;
        expect(capturedAddressLines, isNull,
            reason: 'addressLines should be null when street address is empty');
      });
    });

    group('loadLocationById', () {
      test('loads location by ID from saved database location', () async {
        // Arrange - This tests the "saved location" path where modal receives initialLocationId
        const testLocationId = 'test-location-id';
        const poiName = 'Red Rocks Amphitheatre';

        final savedLocation = Location(
          id: testLocationId,
          name: poiName,
          latitudeDeg: 39.6654,
          longitudeDeg: -105.2054,
          addressLines: ['18300 W Alameda Pkwy'],
          locality: 'Morrison',
          regionCode: 'CO',
          postalCode: '80465',
        );

        when(mockLocationRepository.getLocation(testLocationId))
            .thenAnswer((_) async => savedLocation);

        // Act
        final notifier = container.read(locationPickerProvider.notifier);
        await notifier.loadLocationById(testLocationId);

        // Assert - Verify the location was loaded by ID
        verify(mockLocationRepository.getLocation(testLocationId)).called(1);

        final state = container.read(locationPickerProvider);
        expect(state.currentLocation, equals(savedLocation));
        expect(state.currentLocation?.name, equals(poiName));
        expect(state.currentLocation?.latitudeDeg, equals(39.6654));
        expect(state.currentLocation?.longitudeDeg, equals(-105.2054));
      });

      test('handles location not found error', () async {
        // Arrange
        const testLocationId = 'non-existent-id';

        when(mockLocationRepository.getLocation(testLocationId))
            .thenThrow(Exception('Location not found'));

        // Act
        final notifier = container.read(locationPickerProvider.notifier);
        await notifier.loadLocationById(testLocationId);

        // Assert - Verify error is captured in state, not thrown
        final state = container.read(locationPickerProvider);
        expect(state.error, isNotNull);
        expect(state.isLoadingLocation, isFalse);
      });
    });

    group('applyDevicePosition', () {
      Position testPosition() => Position(
            latitude: 40.015,
            longitude: -105.2705,
            timestamp: DateTime(2026, 4, 18),
            accuracy: 10,
            altitude: 1655,
            altitudeAccuracy: 0,
            heading: 0,
            headingAccuracy: 0,
            speed: 0,
            speedAccuracy: 0,
          );

      test('reverse-geocodes the position and writes to state on success',
          () async {
        final expected = LocationResult(
          name: 'Pearl Street Mall',
          fullName: 'Pearl Street Mall, Boulder, CO',
          type: 'place',
          latitude: 40.017,
          longitude: -105.28,
          streetAddress: '',
          locality: 'Boulder',
          region: 'CO',
          postcode: '80302',
          regionCode: 'CO',
        );
        when(mockLocationRepository.reverseGeocodeViaSearchService(any, any))
            .thenAnswer((_) async => expected);

        final notifier = container.read(locationPickerProvider.notifier);
        await notifier.applyDevicePosition(testPosition());

        final state = container.read(locationPickerProvider);
        expect(state.currentLocationResult, expected);
        expect(state.isLoadingCurrentLocation, isFalse);
        expect(state.error, isNull);
        verify(mockLocationRepository.reverseGeocodeViaSearchService(40.015, -105.2705))
            .called(1);
      });

      test(
          'sets errorMessage and clears loading when reverse-geocode throws',
          () async {
        when(mockLocationRepository.reverseGeocodeViaSearchService(any, any))
            .thenThrow(Exception('network down'));

        final notifier = container.read(locationPickerProvider.notifier);
        await notifier.applyDevicePosition(testPosition());

        final state = container.read(locationPickerProvider);
        expect(state.currentLocationResult, isNull);
        expect(state.isLoadingCurrentLocation, isFalse);
        expect(state.error, isNotNull);
      });

      test('sets isLoadingCurrentLocation true while in flight', () async {
        final completer = Completer<LocationResult>();
        when(mockLocationRepository.reverseGeocodeViaSearchService(any, any))
            .thenAnswer((_) => completer.future);

        final notifier = container.read(locationPickerProvider.notifier);
        final future = notifier.applyDevicePosition(testPosition());

        // Let the state update microtask land.
        await Future.microtask(() {});
        expect(
          container.read(locationPickerProvider).isLoadingCurrentLocation,
          isTrue,
        );

        completer.complete(
          LocationResult(
            name: 'Test',
            fullName: 'Test, CO',
            type: 'place',
            latitude: 40,
            longitude: -105,
            streetAddress: '',
            locality: 'Test',
            region: 'CO',
            postcode: '',
            regionCode: 'CO',
          ),
        );
        await future;

        expect(
          container.read(locationPickerProvider).isLoadingCurrentLocation,
          isFalse,
        );
      });
    });

    group('disposal', () {
      test(
        'disposal during applyDevicePosition completes without throwing',
        () async {
          // Per docs/client/architecture.md §"Disposal Testing": start an
          // async flow, dispose the container mid-flight, and verify the
          // in-flight work settles without exceptions.
          //
          // `applyDevicePosition` takes a Position (produced upstream by the
          // permission-aware helper in a widget) and reverse-geocodes it via
          // the repository. The only async gap inside the method is the
          // reverse-geocode call, which is what we dispose during here.
          final reverseGeocodeCompleter = Completer<LocationResult>();

          when(mockLocationRepository.reverseGeocodeViaSearchService(any, any))
              .thenAnswer((_) => reverseGeocodeCompleter.future);

          final disposalContainer = ProviderContainer(
            overrides: [
              locationRepositoryProvider
                  .overrideWithValue(mockLocationRepository),
            ],
          );
          // Keep the provider alive across the async gap.
          disposalContainer.listen(locationPickerProvider, (_, _) {});

          // Trigger the async flow; don't await.
          final notifier =
              disposalContainer.read(locationPickerProvider.notifier);
          final position = Position(
            latitude: 40,
            longitude: -105,
            timestamp: DateTime.now(),
            accuracy: 10,
            altitude: 0,
            altitudeAccuracy: 0,
            heading: 0,
            headingAccuracy: 0,
            speed: 0,
            speedAccuracy: 0,
          );
          final future = notifier.applyDevicePosition(position);

          // Let the reverse-geocode await register.
          await Future.microtask(() {});

          // Dispose mid-flight. Complete the pending future afterwards —
          // post-disposal state writes must be no-ops, not throws.
          disposalContainer.dispose();
          reverseGeocodeCompleter.complete(
            LocationResult(
              name: 'Test',
              fullName: 'Test, CO',
              type: 'place',
              latitude: 40,
              longitude: -105,
              streetAddress: '',
              locality: 'Test',
              region: 'CO',
              postcode: '',
              regionCode: 'CO',
            ),
          );

          await expectLater(future, completes);
        },
      );
    });
  });
}
