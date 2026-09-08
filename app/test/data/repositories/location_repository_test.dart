import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/location_service.pb.dart';
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/services/location_search_service.dart';
import 'package:ripls/services/location_service.dart';

import 'location_repository_test.mocks.dart';

@GenerateMocks([LocationService, LocationSearchService])
void main() {
  group('LocationRepository', () {
    late LocationRepository repository;
    late MockLocationService mockService;
    late MockLocationSearchService mockSearchService;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockLocationService();
      mockSearchService = MockLocationSearchService();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = LocationRepository(cacheManager, mockService, mockSearchService);
    });

    tearDown(() async {
      await cacheManager.dispose();
    });

    group('getLocation', () {
      test('fetches location from service', () async {
        const locationId = 'loc123';
        final mockLocation = Location(
          id: locationId,
          locality: 'San Francisco',
          regionCode: 'US',
          postalCode: '94102',
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        when(mockService.getLocation(locationId))
            .thenAnswer((_) async => mockLocation);

        final result = await repository.getLocation(locationId);

        expect(result.id, locationId);
        expect(result.locality, 'San Francisco');
        expect(result.latitudeDeg, 37.7749);
        verify(mockService.getLocation(locationId)).called(1);
      });

      test('caches location data', () async {
        const locationId = 'loc123';
        final mockLocation = Location(
          id: locationId,
          locality: 'San Francisco',
        );

        when(mockService.getLocation(locationId))
            .thenAnswer((_) async => mockLocation);

        // First call - should fetch
        await repository.getLocation(locationId);

        // Second call - should use cache
        await repository.getLocation(locationId);

        // Service should only be called once
        verify(mockService.getLocation(locationId)).called(1);
      });
    });

    group('saveLocation', () {
      test('saves location and returns ID', () async {
        const locationId = 'loc123';

        when(mockService.saveLocation(
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          regionCode: anyNamed('regionCode'),
          postalCode: anyNamed('postalCode'),
          locality: anyNamed('locality'),
          addressLines: anyNamed('addressLines'),
        )).thenAnswer((_) async => locationId);

        final result = await repository.saveLocation(
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
          regionCode: 'US',
          postalCode: '94102',
          locality: 'San Francisco',
        );

        expect(result, locationId);
        verify(mockService.saveLocation(
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
          regionCode: 'US',
          postalCode: '94102',
          locality: 'San Francisco',
          addressLines: anyNamed('addressLines'),
        )).called(1);
      });

      test('saves location successfully', () async {
        const locationId = 'loc123';
        const userId = 'user123';

        when(mockService.saveLocation(
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          regionCode: anyNamed('regionCode'),
          postalCode: anyNamed('postalCode'),
          locality: anyNamed('locality'),
          addressLines: anyNamed('addressLines'),
        )).thenAnswer((_) async => locationId);

        // Save location
        await repository.saveLocation(
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
          regionCode: 'US',
          postalCode: '94102',
          locality: 'San Francisco',
          userId: userId,
        );

        verify(mockService.saveLocation(
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
          regionCode: 'US',
          postalCode: '94102',
          locality: 'San Francisco',
          addressLines: anyNamed('addressLines'),
        )).called(1);
      });
    });

    group('deleteLocation', () {
      test('deletes location and returns success', () async {
        const locationId = 'loc123';

        when(mockService.deleteLocation(locationId))
            .thenAnswer((_) async => true);

        final result = await repository.deleteLocation(locationId);

        expect(result, true);
        verify(mockService.deleteLocation(locationId)).called(1);
      });

      test('deletes location and invalidates caches', () async {
        const locationId = 'loc123';
        final mockLocation = Location(id: locationId);
        final mockUserLocations = [
          UserLocationWithDetails(
            locationId: 'loc1',
            lastUsedAtUnixSec: Int64(123456),
            location: Location(id: 'loc1'),
          )
        ];

        when(mockService.getLocation(locationId))
            .thenAnswer((_) async => mockLocation);
        when(mockService.deleteLocation(locationId))
            .thenAnswer((_) async => true);
        when(mockService.getUserLocations())
            .thenAnswer((_) async => mockUserLocations);

        // Populate caches
        await repository.getLocation(locationId);
        await repository.getUserLocations();

        // Delete location (should invalidate caches)
        await repository.deleteLocation(locationId);

        // Fetch again - should call service (caches were invalidated)
        await repository.getLocation(locationId);
        await repository.getUserLocations();

        // Services should be called twice (once before delete, once after)
        verify(mockService.getLocation(locationId)).called(2);
        verify(mockService.getUserLocations()).called(2);
      });
    });

    group('geocodeAddress', () {
      test('geocodes address without caching', () async {
        final mockResponse = GeocodeAddressResponse(
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        when(mockService.geocodeAddress(
          regionCode: anyNamed('regionCode'),
          postalCode: anyNamed('postalCode'),
          locality: anyNamed('locality'),
          addressLines: anyNamed('addressLines'),
        )).thenAnswer((_) async => mockResponse);

        final result = await repository.geocodeAddress(
          regionCode: 'US',
          postalCode: '94102',
          locality: 'San Francisco',
        );

        expect(result.latitudeDeg, 37.7749);
        expect(result.longitudeDeg, -122.4194);
        verify(mockService.geocodeAddress(
          regionCode: 'US',
          postalCode: '94102',
          locality: 'San Francisco',
          addressLines: anyNamed('addressLines'),
        )).called(1);
      });
    });

    group('reverseGeocode', () {
      test('reverse geocodes coordinates without caching', () async {
        final mockResponse = ReverseGeocodeResponse(
          regionCode: 'US',
          postalCode: '94102',
          locality: 'San Francisco',
        );

        when(mockService.reverseGeocode(
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
        )).thenAnswer((_) async => mockResponse);

        final result = await repository.reverseGeocode(
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        expect(result.locality, 'San Francisco');
        expect(result.postalCode, '94102');
        verify(mockService.reverseGeocode(
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        )).called(1);
      });
    });
  });
}
