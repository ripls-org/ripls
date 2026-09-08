import 'dart:math';

/// Formats distance in meters to a human-readable string.
class DistanceFormatter {
  /// Formats distance in meters to a human-readable string.
  ///
  /// For distances >= 0.1 miles (528 feet):
  /// - Returns miles with 1 decimal place (e.g., "0.1 mi", "1.2 mi")
  ///
  /// For distances < 0.1 miles:
  /// - Returns feet (e.g., "500 ft", "250 ft")
  ///
  /// Returns null if distanceMeters is null or negative.
  static String? format(double? distanceMeters) {
    if (distanceMeters == null || distanceMeters < 0) {
      return null;
    }

    // Convert meters to feet (1 meter = 3.28084 feet)
    final feet = distanceMeters * 3.28084;

    // If less than 0.1 miles (528 feet), show in feet
    if (feet < 528) {
      return '${feet.round()} ft';
    }

    // Convert to miles
    final miles = feet / 5280;

    // Show 1 decimal place for miles
    return '${miles.toStringAsFixed(1)} mi';
  }

  /// Formats distance with "away" suffix (e.g., "500 ft away", "1.2 mi away")
  static String? formatWithAway(double? distanceMeters) {
    final formatted = format(distanceMeters);
    return formatted != null ? '$formatted away' : null;
  }

  /// Calculates the distance in meters between two geographic coordinates using the Haversine formula.
  ///
  /// Returns the great circle distance in meters.
  /// Returns null if any coordinate is null or if coordinates are invalid.
  static double? calculateDistance({
    required double? lat1,
    required double? lon1,
    required double? lat2,
    required double? lon2,
  }) {
    if (lat1 == null || lon1 == null || lat2 == null || lon2 == null) {
      return null;
    }

    // Check for invalid coordinates (0,0 means no location set)
    if ((lat1 == 0 && lon1 == 0) || (lat2 == 0 && lon2 == 0)) {
      return null;
    }

    // Earth's radius in meters
    const double earthRadiusMeters = 6371000;

    // Convert degrees to radians
    final lat1Rad = lat1 * pi / 180;
    final lat2Rad = lat2 * pi / 180;
    final deltaLat = (lat2 - lat1) * pi / 180;
    final deltaLon = (lon2 - lon1) * pi / 180;

    // Haversine formula
    final a =
        sin(deltaLat / 2) * sin(deltaLat / 2) +
        cos(lat1Rad) * cos(lat2Rad) * sin(deltaLon / 2) * sin(deltaLon / 2);
    final c = 2 * atan2(sqrt(a), sqrt(1 - a));

    return earthRadiusMeters * c;
  }
}
