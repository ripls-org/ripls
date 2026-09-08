import 'package:geolocator/geolocator.dart';
import 'package:logging/logging.dart';

final _log = Logger('DeviceLocationService');

/// Why the caller wants a position.
///
/// Every entry point into [DeviceLocationService] requires an intent so the
/// service can pick an accuracy / staleness tradeoff that matches what the
/// caller will actually do with the result. Without this, a single "get the
/// user's position" API has to compromise between speed and precision — see
/// issue #1175 for the cold-start race that motivated splitting by intent.
enum LocationIntent {
  /// The user will NOT see this position in the UI — it's used to rank,
  /// sort, or bias results (sort-by-nearest, proximity autocomplete, AI
  /// suggestion hints). Neighborhood-level accuracy (~100m) is fine, and a
  /// recent last-known fix from the OS is preferred for speed.
  proximityBias,

  /// The user WILL see this position in the UI as a pin, dot, or saved
  /// address. Stale or coarse is wrong here — e.g. a "use current location"
  /// save that ends up 100m away, or a "center on me" pin that jumps.
  /// Always requests a fresh high-accuracy fix.
  precisePin,
}

/// Maximum age a [Geolocator.getLastKnownPosition] fix may have and still be
/// returned for [LocationIntent.proximityBias]. Older than this, we fall
/// through to [Geolocator.getCurrentPosition] with medium accuracy so the
/// result is at least current-neighborhood correct.
const Duration _proximityLastKnownMaxAge = Duration(minutes: 5);

/// Outcome of a device location request — pairs an optional [Position] with
/// the final [LocationPermission] state observed during the call so callers
/// can make UX decisions without issuing another Geolocator call (which, on
/// Android, could show a second system prompt).
class LocationRequestResult {
  const LocationRequestResult({required this.permission, this.position});

  /// The GPS position if permission was granted and the fetch succeeded,
  /// null otherwise.
  final Position? position;

  /// The permission state at the end of the request. Useful for determining
  /// whether to show a settings-redirect dialog ([deniedForever]) or respect
  /// a fresh decline ([denied]).
  final LocationPermission permission;
}

/// Provider-agnostic device GPS location service using the geolocator package.
///
/// Handles permission checks, permission request deduplication, and position
/// fetching. Branches on [LocationIntent] so callers that only need proximity
/// biasing get a fast (possibly cached) fix, while callers that will display
/// the position in the UI get a fresh high-accuracy fix. No dependency on
/// any map provider (Mapbox, Google, etc.).
class DeviceLocationService {
  /// Guards against concurrent permission requests.
  ///
  /// Geolocator throws if requestPermission is called while a previous request
  /// is still pending. All concurrent callers await the same in-flight request.
  static Future<LocationPermission>? _permissionRequest;

  /// Get the current device GPS position WITHOUT prompting for permission.
  ///
  /// The [intent] controls the accuracy/staleness tradeoff:
  /// - [LocationIntent.proximityBias]: prefers a recent OS-cached fix (≤ 5
  ///   min old) for speed; falls through to [LocationAccuracy.medium] if no
  ///   recent cache. Sub-100ms in the fast path.
  /// - [LocationIntent.precisePin]: always fresh, [LocationAccuracy.high].
  ///
  /// Returns null when location services are disabled, permission is not
  /// already granted, or an error occurs. Never throws. Never shows the
  /// system permission dialog — use this from background/startup paths that
  /// should not pester the user.
  static Future<Position?> getCurrentPositionIfGranted({
    required LocationIntent intent,
  }) async {
    final result = await _fetchPosition(requestIfDenied: false, intent: intent);
    return result.position;
  }

  /// Request permission (if not yet granted) and get the current device GPS
  /// position, returning a [LocationRequestResult] so callers can observe the
  /// final permission state.
  ///
  /// Defaults to [LocationIntent.precisePin] because gesture-initiated
  /// location requests ("center on me", "use current location") are nearly
  /// always about showing or saving a precise position.
  ///
  /// Shows the system permission dialog when permission is `denied`. Never
  /// throws. Use this ONLY from user-initiated gesture paths where an OS
  /// prompt is expected.
  static Future<LocationRequestResult> requestAndGetCurrentPosition({
    LocationIntent intent = LocationIntent.precisePin,
  }) async {
    return _fetchPosition(requestIfDenied: true, intent: intent);
  }

  static Future<LocationRequestResult> _fetchPosition({
    required bool requestIfDenied,
    required LocationIntent intent,
  }) async {
    var permission = LocationPermission.unableToDetermine;
    try {
      if (!await Geolocator.isLocationServiceEnabled()) {
        _log.warning('Location services are disabled');
        return LocationRequestResult(permission: permission);
      }

      permission = await Geolocator.checkPermission();
      if (permission == LocationPermission.denied) {
        if (!requestIfDenied) {
          _log.fine('Location permission not granted; skipping prompt');
          return LocationRequestResult(permission: permission);
        }
        _permissionRequest ??= Geolocator.requestPermission().whenComplete(
          () => _permissionRequest = null,
        );
        permission = await _permissionRequest!;
        if (permission == LocationPermission.denied) {
          _log.warning('Location permissions are denied');
          return LocationRequestResult(permission: permission);
        }
      }

      if (permission == LocationPermission.deniedForever) {
        _log.warning('Location permissions are permanently denied');
        return LocationRequestResult(permission: permission);
      }

      if (intent == LocationIntent.proximityBias) {
        final lastKnown = await _recentLastKnownPosition();
        if (lastKnown != null) {
          _log.info(
            'Got device location: ${lastKnown.latitude}, ${lastKnown.longitude}'
            ' (intent=proximityBias, source=lastKnown)',
          );
          return LocationRequestResult(
            permission: permission,
            position: lastKnown,
          );
        }
      }

      final accuracy = intent == LocationIntent.proximityBias
          ? LocationAccuracy.medium
          : LocationAccuracy.high;
      final position = await Geolocator.getCurrentPosition(
        locationSettings: LocationSettings(accuracy: accuracy),
      );
      _log.info(
        'Got device location: ${position.latitude}, ${position.longitude}'
        ' (intent=${intent.name}, source=fresh, accuracy=${accuracy.name})',
      );
      return LocationRequestResult(permission: permission, position: position);
    } catch (e, stackTrace) {
      _log.severe('Error getting device location', e, stackTrace);
      return LocationRequestResult(permission: permission);
    }
  }

  /// Returns the OS-cached last-known position if it exists and is newer than
  /// [_proximityLastKnownMaxAge]. Returns null otherwise (no cache, or the
  /// cached fix is too old to trust for proximity biasing).
  static Future<Position?> _recentLastKnownPosition() async {
    final lastKnown = await Geolocator.getLastKnownPosition();
    if (lastKnown == null) {
      return null;
    }
    final age = DateTime.now().difference(lastKnown.timestamp);
    if (age > _proximityLastKnownMaxAge) {
      _log.fine(
        'Last-known position is stale (${age.inSeconds}s old, max '
        '${_proximityLastKnownMaxAge.inSeconds}s) — skipping',
      );
      return null;
    }
    return lastKnown;
  }
}
