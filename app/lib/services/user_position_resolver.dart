import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart';
import 'package:logging/logging.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('UserPositionResolver');

/// Source used to resolve the user's current position, for telemetry.
enum UserPositionSource {
  /// Live GPS fix from the device.
  gps,

  /// Fallback: the authenticated user's primary residence location.
  primaryResidence,

  /// No position available (GPS denied / unavailable and no primary residence).
  unbiased,
}

/// Resolves a best-effort position for the authenticated user.
///
/// Ladder:
///   1. Device GPS (via DeviceLocationService).
///   2. User's primary residence location (looked up via repositories).
///   3. null (unbiased).
///
/// Never throws; any error results in returning null so callers can fall back
/// to unbiased geocoding rather than fail loudly.
class UserPositionResolver {
  UserPositionResolver._();

  /// Resolves a [Position] for the authenticated user under the given
  /// [intent].
  ///
  /// The [intent] is passed through to [DeviceLocationService] so proximity
  /// callers get a fast (possibly OS-cached) fix and precise callers get a
  /// fresh high-accuracy fix. The primary-residence fallback is already a
  /// coarse synthetic position, so it is intent-independent.
  ///
  /// Returns null if no position can be determined (e.g. GPS denied and no
  /// primary residence on file).
  static Future<Position?> resolve(Ref ref, LocationIntent intent) async {
    final gps = await DeviceLocationService.getCurrentPositionIfGranted(
      intent: intent,
    );
    if (gps != null) {
      _log.info('Resolved user position via GPS',
          {'source': UserPositionSource.gps.name, 'intent': intent.name});
      return gps;
    }

    if (!ref.mounted) return null;

    final residence = await _fromPrimaryResidence(ref);
    if (residence != null) {
      _log.info('Resolved user position via primary residence', {
        'source': UserPositionSource.primaryResidence.name,
        'intent': intent.name,
      });
      return residence;
    }

    _log.info('No user position available',
        {'source': UserPositionSource.unbiased.name, 'intent': intent.name});
    return null;
  }

  /// Looks up the authenticated user's primary residence and converts it to
  /// a synthetic [Position]. Returns null if the user isn't authenticated,
  /// has no primary residence, or the location can't be loaded.
  static Future<Position?> _fromPrimaryResidence(Ref ref) async {
    try {
      final authState = ref.read(authStateProvider);
      final userId = authState.user?.id;
      if (userId == null || userId.isEmpty) {
        return null;
      }

      final userRepository = ref.read(userRepositoryProvider);
      final user = await userRepository.get(userId);
      if (!ref.mounted) return null;

      final locationId = user.primaryResidenceLocationId;
      if (locationId.isEmpty) {
        return null;
      }

      final locationRepository = ref.read(locationRepositoryProvider);
      final location = await locationRepository.get(locationId);
      if (!ref.mounted) return null;

      // Build a synthetic Position. Accuracy is set to a large value (1km) to
      // signal that this is a coarse fallback, not a real GPS fix.
      return Position(
        latitude: location.latitudeDeg,
        longitude: location.longitudeDeg,
        timestamp: DateTime.now(),
        accuracy: 1000,
        altitude: 0,
        altitudeAccuracy: 0,
        heading: 0,
        headingAccuracy: 0,
        speed: 0,
        speedAccuracy: 0,
      );
    } catch (e, stackTrace) {
      _log.warning('Primary residence position lookup failed',
          e, stackTrace);
      return null;
    }
  }
}
