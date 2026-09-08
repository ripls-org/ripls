import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart';
import 'package:logging/logging.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('UserLocationInitializer');

/// Utility for initializing user location in view models and screens.
///
/// This utility provides a centralized way to get a user's default location
/// for gear, requests, and other location-aware features. It handles:
/// 1. Fetching user's primary residence location if set
/// 2. Detecting device location if no primary residence exists
/// 3. Reverse geocoding to get address details
/// 4. Saving the location and setting it as primary residence
///
/// This ensures consistent location initialization behavior across the app.
class UserLocationInitializer {
  UserLocationInitializer._();

  /// Cheap, side-effect-free lookup: returns the user's primary residence
  /// location ID if one is already set, otherwise null.
  ///
  /// Safe to call from hot paths like RPC request builders — does NOT trigger
  /// GPS fetches, reverse geocoding, or writes. Use [getPrimaryLocation] for
  /// the heavyweight onboarding flow that initializes a residence when none
  /// is set.
  static Future<String?> getPrimaryLocationIfSet(Ref ref) async {
    try {
      final authState = ref.read(authStateProvider);
      final userId = authState.user?.id;
      if (userId == null || userId.isEmpty) return null;

      final userRepository = ref.read(userRepositoryProvider);
      final user = await userRepository.get(userId);
      if (!ref.mounted) return null;

      return user.primaryResidenceLocationId.isNotEmpty
          ? user.primaryResidenceLocationId
          : null;
    } catch (e) {
      _log.warning('Failed to read primary residence: $e');
      return null;
    }
  }

  /// Initialize default location from user's primary residence or device location.
  ///
  /// Returns the user's primary residence location ID if set.
  /// If no primary residence exists, attempts to detect device location,
  /// reverse geocode it to get address, save it, and set it as primary residence.
  /// Returns null if location detection fails or permission is denied.
  ///
  /// **This is a heavyweight operation** — may perform GPS lookup, reverse
  /// geocoding, and a SaveLocation RPC when no residence is set. Don't call
  /// it in hot paths like RPC request builders; use [getPrimaryLocationIfSet]
  /// there and invoke this one from a background/onboarding flow.
  ///
  /// Example usage:
  /// ```dart
  /// final locationId = await UserLocationInitializer.getPrimaryLocation(ref);
  /// if (locationId != null) {
  ///   updateLocation(locationId);
  /// }
  /// ```
  static Future<String?> getPrimaryLocation(Ref ref) async {
    try {
      // Try to get user's primary residence location
      final authState = ref.read(authStateProvider);

      if (authState.user?.id == null) {
        return null;
      }

      final userRepository = ref.read(userRepositoryProvider);
      final user = await userRepository.get(authState.user!.id);

      // Check if ref is still mounted after async operation
      if (!ref.mounted) {
        return null;
      }

      if (user.primaryResidenceLocationId.isNotEmpty) {
        return user.primaryResidenceLocationId;
      }

      // No primary residence - try to detect device location and set it as primary
      final hasPermission = await _checkLocationPermission();

      // Check if ref is still mounted after async operation
      if (!ref.mounted) {
        return null;
      }

      if (!hasPermission) {
        return null;
      }

      // Get device GPS coordinates
      final position = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(
          accuracy: LocationAccuracy.medium,
        ),
      );

      // Check if ref is still mounted after async operation
      if (!ref.mounted) {
        return null;
      }

      // Reverse geocode to get address from coordinates
      final locationService = ref.read(locationServiceProvider);

      // Try to reverse geocode, but continue even if it fails
      String? regionCode;
      String? postalCode;
      String? locality;
      List<String>? addressLines;

      try {
        final address = await locationService.reverseGeocode(
          latitudeDeg: position.latitude,
          longitudeDeg: position.longitude,
        );

        // Check if ref is still mounted after async operation
        if (!ref.mounted) {
          return null;
        }

        regionCode = address.regionCode;
        postalCode = address.postalCode;
        locality = address.locality;
        addressLines = address.addressLines;
      } catch (e) {
        _log.warning('Reverse geocoding failed: $e - saving location with coordinates only');
        // Continue without address details - we'll save just the coordinates
      }

      // Check if ref is still mounted
      if (!ref.mounted) {
        return null;
      }

      // Save location (with or without address details)
      final locationId = await locationService.saveLocation(
        latitudeDeg: position.latitude,
        longitudeDeg: position.longitude,
        regionCode: regionCode ?? '',
        postalCode: postalCode ?? '',
        locality: locality ?? '',
        addressLines: addressLines ?? [],
      );

      // Check if ref is still mounted after async operation
      if (!ref.mounted) {
        return null;
      }

      // NOTE: No need to call saveUser() here - the server's SaveLocation RPC
      // automatically adds the location to the user's list (as primary if none exists,
      // or to other locations if primary already set)

      return locationId;
    } catch (e, stackTrace) {
      _log.warning('Failed to initialize default location: $e', e, stackTrace);
      return null;
    }
  }

  /// Check if location permission is granted.
  ///
  /// If permission is denied, this method will request permission.
  /// Returns true if permission is granted (whileInUse or always).
  static Future<bool> _checkLocationPermission() async {
    try {
      final permission = await Geolocator.checkPermission();
      if (permission == LocationPermission.denied) {
        final newPermission = await Geolocator.requestPermission();
        return newPermission == LocationPermission.whileInUse ||
            newPermission == LocationPermission.always;
      }
      return permission == LocationPermission.whileInUse ||
          permission == LocationPermission.always;
    } catch (e) {
      _log.warning('Failed to check location permission: $e');
      return false;
    }
  }
}
