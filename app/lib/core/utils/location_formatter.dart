import 'package:ripls/services/location_service.dart';

/// LocationFormatter provides utility methods for formatting location information
/// into human-readable strings with intelligent priority hierarchy.
class LocationFormatter {
  LocationFormatter._(); // Private constructor to prevent instantiation

  
  /// Formats a location name specifically for display in short spaces like chips.
  ///
  /// This variant prioritizes brevity while maintaining context:
  /// 1. Place name only (without city)
  /// 2. First street address line only
  /// 3. City/locality only
  /// 4. Region code
  /// 5. "Unknown" fallback
  static String formatLocationNameShort(Location location) {
    // Priority 1: Place name (most meaningful single identifier)
    if (location.name.isNotEmpty) {
      return location.name;
    }

    // Priority 2: Street address (specific location identifier)
    if (location.addressLines.isNotEmpty) {
      return location.addressLines.first;
    }

    // Priority 3: City/locality (broader area identifier)
    if (location.locality.isNotEmpty) {
      return location.locality;
    }

    // Priority 4: Region code
    if (location.regionCode.isNotEmpty) {
      return location.regionCode;
    }

    return 'Unknown';
  }

  /// Formats a location with complete details in a single-line format.
  ///
  /// Includes postal code, region code, and country when available.
  /// Suitable for display in location selection rows or detailed previews.
  ///
  /// Priority hierarchy:
  /// 1. Place name, street address, city, state/region, postal code
  /// 2. Street address, city, state/region, postal code
  /// 3. City, state/region, postal code (if no name/address)
  /// 4. Region code, postal code (fallback)
  /// 5. "Unknown Location" fallback
  ///
  /// Examples:
  /// - "Empower Field at Mile High, 1701 Bryant St, Denver, CO 80204"
  /// - "123 Main St, Austin, TX 78701"
  /// - "Austin, TX 78701"
  /// - "CO 80204"
  static String formatLocationDetailed(Location location) {
    final parts = <String>[];

    // Priority 1: Place name
    if (location.name.isNotEmpty) {
      parts.add(location.name);
    }

    // Priority 2: Street address (always add if available, even with place name)
    if (location.addressLines.isNotEmpty) {
      parts.add(location.addressLines.first);
    }

    // Add locality (city) - always include
    if (location.locality.isNotEmpty) {
      parts.add(location.locality);
    }

    // Add region code (state/province)
    if (location.regionCode.isNotEmpty) {
      parts.add(location.regionCode);
    }

    // Add postal code
    if (location.postalCode.isNotEmpty) {
      parts.add(location.postalCode);
    }

    return parts.isEmpty ? 'Unknown Location' : parts.join(', ');
  }

  /// Formats a LocationResult with complete details in a single-line format.
  ///
  /// This is a helper that handles LocationResult objects from the autocomplete
  /// service with the same priority logic as formatLocationDetailed().
  ///
  /// Priority hierarchy:
  /// 1. Place name (if different from street/locality), street address, city, state, postal
  /// 2. Street address, city, state, postal
  /// 3. City, state, postal
  ///
  /// The name field in LocationResult may contain the place name, street address,
  /// or locality depending on what was available during construction. We check
  /// for duplicates to avoid showing the same info twice.
  static String formatLocationResultDetailed(
    String name,
    String streetAddress,
    String locality,
    String regionCode,
    String postcode,
  ) {
    final parts = <String>[];

    // Priority 1: Place name (if it's not a street address or locality)
    if (name.isNotEmpty && name != streetAddress && name != locality) {
      parts.add(name);
    }

    // Priority 2: Street address (always add if available)
    if (streetAddress.isNotEmpty) {
      parts.add(streetAddress);
    }

    // Priority 3: City (always add)
    if (locality.isNotEmpty) {
      parts.add(locality);
    }

    // Priority 4: Region code
    if (regionCode.isNotEmpty) {
      parts.add(regionCode);
    }

    // Priority 5: Postal code
    if (postcode.isNotEmpty) {
      parts.add(postcode);
    }

    return parts.isEmpty ? 'Unknown Location' : parts.join(', ');
  }
}
