import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/distance_formatter.dart';

void main() {
  group('DistanceFormatter', () {
    group('format', () {
      test('formats distances < 528 feet as feet', () {
        // 100 meters = ~328 feet
        expect(DistanceFormatter.format(100), '328 ft');

        // 50 meters = ~164 feet
        expect(DistanceFormatter.format(50), '164 ft');

        // 160 meters = ~525 feet (just under 0.1 miles)
        expect(DistanceFormatter.format(160), '525 ft');
      });

      test('formats distances >= 528 feet as miles with 1 decimal', () {
        // 161 meters = ~528 feet = 0.1 miles
        expect(DistanceFormatter.format(161), '0.1 mi');

        // 1609 meters = 1 mile
        expect(DistanceFormatter.format(1609), '1.0 mi');

        // 3218 meters = 2 miles
        expect(DistanceFormatter.format(3218), '2.0 mi');

        // 1931 meters = ~1.2 miles
        expect(DistanceFormatter.format(1931), '1.2 mi');

        // 8046 meters = ~5 miles
        expect(DistanceFormatter.format(8046), '5.0 mi');
      });

      test('handles edge case at 0.1 mile boundary', () {
        // 160.9 meters = ~527.9 feet (just under)
        expect(DistanceFormatter.format(160.9), '528 ft');

        // 161.5 meters = ~530 feet (just over)
        expect(DistanceFormatter.format(161.5), '0.1 mi');
      });

      test('returns null for null distance', () {
        expect(DistanceFormatter.format(null), null);
      });

      test('returns null for negative distance', () {
        expect(DistanceFormatter.format(-100), null);
        expect(DistanceFormatter.format(-0.1), null);
      });

      test('handles zero distance', () {
        expect(DistanceFormatter.format(0), '0 ft');
      });

      test('handles very small distances', () {
        // 1 meter = ~3 feet
        expect(DistanceFormatter.format(1), '3 ft');

        // 0.5 meters = ~2 feet
        expect(DistanceFormatter.format(0.5), '2 ft');
      });

      test('handles very large distances', () {
        // 160934 meters = ~100 miles
        expect(DistanceFormatter.format(160934), '100.0 mi');

        // 1609340 meters = ~1000 miles
        expect(DistanceFormatter.format(1609340), '1000.0 mi');
      });
    });

    group('formatWithAway', () {
      test('adds "away" suffix to formatted distance', () {
        expect(DistanceFormatter.formatWithAway(100), '328 ft away');
        expect(DistanceFormatter.formatWithAway(1609), '1.0 mi away');
        expect(DistanceFormatter.formatWithAway(3218), '2.0 mi away');
      });

      test('returns null for null distance', () {
        expect(DistanceFormatter.formatWithAway(null), null);
      });

      test('returns null for negative distance', () {
        expect(DistanceFormatter.formatWithAway(-100), null);
      });
    });

    group('calculateDistance', () {
      test('calculates distance between two points', () {
        // Austin to Dallas (approx 182 miles = 293,000 meters)
        final austinLat = 30.2672;
        final austinLon = -97.7431;
        final dallasLat = 32.7767;
        final dallasLon = -96.7970;

        final distance = DistanceFormatter.calculateDistance(
          lat1: austinLat,
          lon1: austinLon,
          lat2: dallasLat,
          lon2: dallasLon,
        );

        // Should be approximately 293 km (allow 2% margin)
        expect(distance, isNotNull);
        expect(distance!, greaterThan(287000)); // 293k - 2%
        expect(distance, lessThan(299000)); // 293k + 2%
      });

      test('calculates zero distance for same location', () {
        final lat = 30.2672;
        final lon = -97.7431;

        final distance = DistanceFormatter.calculateDistance(
          lat1: lat,
          lon1: lon,
          lat2: lat,
          lon2: lon,
        );

        expect(distance, isNotNull);
        expect(distance!, lessThan(0.01)); // Essentially zero
      });

      test('calculates short distances accurately', () {
        // Two points ~100 meters apart in Austin
        final lat1 = 30.2672;
        final lon1 = -97.7431;
        final lat2 = 30.2681; // ~0.0009 degrees north
        final lon2 = -97.7431;

        final distance = DistanceFormatter.calculateDistance(
          lat1: lat1,
          lon1: lon1,
          lat2: lat2,
          lon2: lon2,
        );

        // Should be approximately 100 meters (allow 10% margin for small distances)
        expect(distance, isNotNull);
        expect(distance!, greaterThan(90));
        expect(distance, lessThan(110));
      });

      test('returns null when any coordinate is null', () {
        expect(
          DistanceFormatter.calculateDistance(
            lat1: null,
            lon1: -97.7431,
            lat2: 32.7767,
            lon2: -96.7970,
          ),
          null,
        );

        expect(
          DistanceFormatter.calculateDistance(
            lat1: 30.2672,
            lon1: null,
            lat2: 32.7767,
            lon2: -96.7970,
          ),
          null,
        );

        expect(
          DistanceFormatter.calculateDistance(
            lat1: 30.2672,
            lon1: -97.7431,
            lat2: null,
            lon2: -96.7970,
          ),
          null,
        );

        expect(
          DistanceFormatter.calculateDistance(
            lat1: 30.2672,
            lon1: -97.7431,
            lat2: 32.7767,
            lon2: null,
          ),
          null,
        );
      });

      test('returns null for (0,0) coordinates', () {
        // (0,0) means no location set
        expect(
          DistanceFormatter.calculateDistance(
            lat1: 0,
            lon1: 0,
            lat2: 32.7767,
            lon2: -96.7970,
          ),
          null,
        );

        expect(
          DistanceFormatter.calculateDistance(
            lat1: 30.2672,
            lon1: -97.7431,
            lat2: 0,
            lon2: 0,
          ),
          null,
        );

        expect(
          DistanceFormatter.calculateDistance(
            lat1: 0,
            lon1: 0,
            lat2: 0,
            lon2: 0,
          ),
          null,
        );
      });

      test('handles cross-hemisphere calculations', () {
        // New York to London (approx 5,570 km)
        final nyLat = 40.7128;
        final nyLon = -74.0060;
        final londonLat = 51.5074;
        final londonLon = -0.1278;

        final distance = DistanceFormatter.calculateDistance(
          lat1: nyLat,
          lon1: nyLon,
          lat2: londonLat,
          lon2: londonLon,
        );

        // Should be approximately 5,570 km (allow 5% margin)
        expect(distance, isNotNull);
        expect(distance!, greaterThan(5290000)); // 5570k - 5%
        expect(distance, lessThan(5850000)); // 5570k + 5%
      });
    });
  });
}
