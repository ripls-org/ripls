import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers.dart';

Position _position({
  double latitude = 40.0150,
  double longitude = -105.2705,
  DateTime? timestamp,
}) {
  return Position(
    latitude: latitude,
    longitude: longitude,
    timestamp: timestamp ?? DateTime(2026),
    accuracy: 10,
    altitude: 1655,
    altitudeAccuracy: 0,
    heading: 0,
    headingAccuracy: 0,
    speed: 0,
    speedAccuracy: 0,
  );
}

void main() {
  group('userLocationProvider (family keyed on LocationIntent)', () {
    test('returns position when overridden with a fixed value', () async {
      final testPosition = _position();

      final container = ProviderContainer(
        overrides: [
          userLocationProvider.overrideWith(
            (ref, intent) async => testPosition,
          ),
        ],
      );
      addTearDown(container.dispose);

      final result = await container.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      expect(result, testPosition);
      expect(result?.latitude, 40.0150);
      expect(result?.longitude, -105.2705);
    });

    test('returns null when overridden with null', () async {
      final container = ProviderContainer(
        overrides: [
          userLocationProvider.overrideWith((ref, intent) async => null),
        ],
      );
      addTearDown(container.dispose);

      final result = await container.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      expect(result, isNull);
    });

    test('consumers fall back correctly when position is null', () async {
      final container = ProviderContainer(
        overrides: [
          userLocationProvider.overrideWith((ref, intent) async => null),
        ],
      );
      addTearDown(container.dispose);

      await container.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      final asyncValue =
          container.read(userLocationProvider(LocationIntent.proximityBias));
      expect(asyncValue.asData?.value, isNull);
    });

    test('ref.invalidate forces a re-fetch for a given intent', () async {
      var fetchCount = 0;
      final testPosition = _position();

      final container = ProviderContainer(
        overrides: [
          userLocationProvider.overrideWith((ref, intent) async {
            fetchCount++;
            return testPosition;
          }),
        ],
      );
      addTearDown(container.dispose);

      await container.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      expect(fetchCount, 1);

      container.invalidate(userLocationProvider(LocationIntent.proximityBias));
      await container.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      expect(fetchCount, 2);
    });

    test('disposal during in-flight fetch completes without throwing',
        () async {
      final container = ProviderContainer(
        overrides: [
          userLocationProvider.overrideWith((ref, intent) async {
            await Future.delayed(const Duration(milliseconds: 50));
            return null;
          }),
        ],
      );

      final future = container.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      container.dispose();

      await expectLater(future, completes);
    });

    test('each intent gets its own cached slot', () async {
      final proximityFixture = _position(latitude: 40, longitude: -105);
      final preciseFixture = _position(latitude: 37, longitude: -122);
      var proximityFetches = 0;
      var preciseFetches = 0;

      final container = ProviderContainer(
        overrides: [
          userLocationProvider.overrideWith((ref, intent) async {
            if (intent == LocationIntent.proximityBias) {
              proximityFetches++;
              return proximityFixture;
            }
            preciseFetches++;
            return preciseFixture;
          }),
        ],
      );
      addTearDown(container.dispose);

      final proximityResult = await container.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      final preciseResult = await container.read(
        userLocationProvider(LocationIntent.precisePin).future,
      );

      expect(proximityResult, proximityFixture);
      expect(preciseResult, preciseFixture);
      expect(proximityFetches, 1);
      expect(preciseFetches, 1);

      // Re-reading either intent should hit the cache, not re-fetch.
      await container.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      await container.read(
        userLocationProvider(LocationIntent.precisePin).future,
      );
      expect(proximityFetches, 1);
      expect(preciseFetches, 1);

      // Invalidating one intent must not force the other to re-fetch.
      container.invalidate(userLocationProvider(LocationIntent.proximityBias));
      await container.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      expect(proximityFetches, 2);
      expect(preciseFetches, 1);
    });
  });
}
