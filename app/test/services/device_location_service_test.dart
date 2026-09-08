import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:plugin_platform_interface/plugin_platform_interface.dart';
import 'package:ripls/services/device_location_service.dart';

/// Fake [GeolocatorPlatform] that records calls and lets tests control the
/// permission, last-known, and fresh position outcomes deterministically.
class FakeGeolocatorPlatform extends GeolocatorPlatform
    with MockPlatformInterfaceMixin {
  FakeGeolocatorPlatform({
    required this.serviceEnabled,
    required this.initialPermission,
    this.requestedPermission,
    this.position,
    this.lastKnownPosition,
  });

  bool serviceEnabled;
  LocationPermission initialPermission;
  LocationPermission? requestedPermission;
  Position? position;
  Position? lastKnownPosition;

  int checkPermissionCalls = 0;
  int requestPermissionCalls = 0;
  int getCurrentPositionCalls = 0;
  int getLastKnownPositionCalls = 0;
  LocationAccuracy? lastRequestedAccuracy;

  @override
  Future<bool> isLocationServiceEnabled() async => serviceEnabled;

  @override
  Future<LocationPermission> checkPermission() async {
    checkPermissionCalls++;
    return initialPermission;
  }

  @override
  Future<LocationPermission> requestPermission() async {
    requestPermissionCalls++;
    final resolved = requestedPermission ?? initialPermission;
    initialPermission = resolved;
    return resolved;
  }

  @override
  Future<Position?> getLastKnownPosition({
    bool forceLocationManager = false,
  }) async {
    getLastKnownPositionCalls++;
    return lastKnownPosition;
  }

  @override
  Future<Position> getCurrentPosition({LocationSettings? locationSettings}) async {
    getCurrentPositionCalls++;
    lastRequestedAccuracy = locationSettings?.accuracy;
    final p = position;
    if (p == null) {
      throw LocationServiceDisabledException();
    }
    return p;
  }
}

Position _boulderPosition({DateTime? timestamp}) => Position(
      latitude: 40.015,
      longitude: -105.2705,
      timestamp: timestamp ?? DateTime(2026, 4, 18),
      accuracy: 10,
      altitude: 1655,
      altitudeAccuracy: 0,
      heading: 0,
      headingAccuracy: 0,
      speed: 0,
      speedAccuracy: 0,
    );

Position _austinPosition({DateTime? timestamp}) => Position(
      latitude: 30.2672,
      longitude: -97.7431,
      timestamp: timestamp ?? DateTime(2026, 4, 18),
      accuracy: 10,
      altitude: 150,
      altitudeAccuracy: 0,
      heading: 0,
      headingAccuracy: 0,
      speed: 0,
      speedAccuracy: 0,
    );

void main() {
  late FakeGeolocatorPlatform fake;

  setUp(() {
    fake = FakeGeolocatorPlatform(
      serviceEnabled: true,
      initialPermission: LocationPermission.denied,
    );
    GeolocatorPlatform.instance = fake;
  });

  group('getCurrentPositionIfGranted (proximityBias)', () {
    test('returns null when permission is denied and never prompts', () async {
      fake.initialPermission = LocationPermission.denied;

      final result = await DeviceLocationService.getCurrentPositionIfGranted(
        intent: LocationIntent.proximityBias,
      );

      expect(result, isNull);
      expect(fake.checkPermissionCalls, 1);
      expect(fake.requestPermissionCalls, 0,
          reason: 'must not show the system prompt from a non-prompting path');
      expect(fake.getCurrentPositionCalls, 0);
      expect(fake.getLastKnownPositionCalls, 0);
    });

    test('returns null when permission is deniedForever without prompting',
        () async {
      fake.initialPermission = LocationPermission.deniedForever;

      final result = await DeviceLocationService.getCurrentPositionIfGranted(
        intent: LocationIntent.proximityBias,
      );

      expect(result, isNull);
      expect(fake.requestPermissionCalls, 0);
    });

    test('returns null when services are disabled', () async {
      fake.serviceEnabled = false;
      fake.initialPermission = LocationPermission.whileInUse;

      final result = await DeviceLocationService.getCurrentPositionIfGranted(
        intent: LocationIntent.proximityBias,
      );

      expect(result, isNull);
      expect(fake.checkPermissionCalls, 0,
          reason: 'service-enabled gate short-circuits before permission');
    });

    test(
        'prefers a fresh last-known fix and skips the getCurrentPosition call',
        () async {
      fake.initialPermission = LocationPermission.whileInUse;
      fake.lastKnownPosition = _boulderPosition(timestamp: DateTime.now());
      fake.position = _austinPosition();

      final result = await DeviceLocationService.getCurrentPositionIfGranted(
        intent: LocationIntent.proximityBias,
      );

      expect(result, isNotNull);
      expect(result?.latitude, 40.015,
          reason: 'should return the last-known Boulder fix, not the fresh '
              'Austin fallback');
      expect(fake.getLastKnownPositionCalls, 1);
      expect(fake.getCurrentPositionCalls, 0,
          reason:
              'a usable last-known fix must short-circuit the expensive call');
    });

    test('falls through to medium-accuracy getCurrentPosition when last-known '
        'is older than the staleness window', () async {
      fake.initialPermission = LocationPermission.whileInUse;
      fake.lastKnownPosition = _boulderPosition(
        timestamp: DateTime.now().subtract(const Duration(hours: 2)),
      );
      fake.position = _austinPosition();

      final result = await DeviceLocationService.getCurrentPositionIfGranted(
        intent: LocationIntent.proximityBias,
      );

      expect(result, isNotNull);
      expect(result?.latitude, 30.2672,
          reason: 'stale last-known should be ignored, fresh position used');
      expect(fake.getLastKnownPositionCalls, 1);
      expect(fake.getCurrentPositionCalls, 1);
      expect(fake.lastRequestedAccuracy, LocationAccuracy.medium,
          reason:
              'proximityBias should use medium accuracy on the slow path');
    });

    test('falls through to getCurrentPosition when last-known is null',
        () async {
      fake.initialPermission = LocationPermission.whileInUse;
      fake.lastKnownPosition = null;
      fake.position = _boulderPosition();

      final result = await DeviceLocationService.getCurrentPositionIfGranted(
        intent: LocationIntent.proximityBias,
      );

      expect(result, isNotNull);
      expect(fake.getLastKnownPositionCalls, 1);
      expect(fake.getCurrentPositionCalls, 1);
      expect(fake.lastRequestedAccuracy, LocationAccuracy.medium);
    });
  });

  group('getCurrentPositionIfGranted (precisePin)', () {
    test('always calls getCurrentPosition with high accuracy, ignoring '
        'last-known', () async {
      fake.initialPermission = LocationPermission.whileInUse;
      fake.lastKnownPosition = _boulderPosition(timestamp: DateTime.now());
      fake.position = _austinPosition();

      final result = await DeviceLocationService.getCurrentPositionIfGranted(
        intent: LocationIntent.precisePin,
      );

      expect(result, isNotNull);
      expect(result?.latitude, 30.2672,
          reason: 'precisePin must not shortcut via last-known');
      expect(fake.getLastKnownPositionCalls, 0,
          reason: 'precisePin must not consult last-known at all');
      expect(fake.getCurrentPositionCalls, 1);
      expect(fake.lastRequestedAccuracy, LocationAccuracy.high);
    });

    test('returns null when permission is denied', () async {
      fake.initialPermission = LocationPermission.denied;

      final result = await DeviceLocationService.getCurrentPositionIfGranted(
        intent: LocationIntent.precisePin,
      );

      expect(result, isNull);
      expect(fake.requestPermissionCalls, 0);
      expect(fake.getCurrentPositionCalls, 0);
    });
  });

  group('requestAndGetCurrentPosition', () {
    test('defaults to precisePin (high accuracy) when caller omits intent',
        () async {
      fake.initialPermission = LocationPermission.whileInUse;
      fake.position = _boulderPosition();

      final result =
          await DeviceLocationService.requestAndGetCurrentPosition();

      expect(result.position, isNotNull);
      expect(fake.lastRequestedAccuracy, LocationAccuracy.high);
      expect(fake.getLastKnownPositionCalls, 0);
    });

    test('returns granted result with position when user grants', () async {
      fake.initialPermission = LocationPermission.denied;
      fake.requestedPermission = LocationPermission.whileInUse;
      fake.position = _boulderPosition();

      final result =
          await DeviceLocationService.requestAndGetCurrentPosition();

      expect(result.position, isNotNull);
      expect(result.position?.latitude, 40.015);
      expect(result.permission, LocationPermission.whileInUse);
      expect(fake.requestPermissionCalls, 1);
    });

    test('returns denied result with null position when user declines',
        () async {
      fake.initialPermission = LocationPermission.denied;
      fake.requestedPermission = LocationPermission.denied;

      final result =
          await DeviceLocationService.requestAndGetCurrentPosition();

      expect(result.position, isNull);
      expect(result.permission, LocationPermission.denied);
      expect(fake.requestPermissionCalls, 1);
      expect(fake.getCurrentPositionCalls, 0);
    });

    test(
        'returns deniedForever result when a denial auto-promotes to permanent',
        () async {
      fake.initialPermission = LocationPermission.denied;
      fake.requestedPermission = LocationPermission.deniedForever;

      final result =
          await DeviceLocationService.requestAndGetCurrentPosition();

      expect(result.position, isNull);
      expect(result.permission, LocationPermission.deniedForever);
      expect(fake.requestPermissionCalls, 1);
    });

    test('skips requestPermission when already granted', () async {
      fake.initialPermission = LocationPermission.whileInUse;
      fake.position = _boulderPosition();

      final result =
          await DeviceLocationService.requestAndGetCurrentPosition();

      expect(result.position, isNotNull);
      expect(result.permission, LocationPermission.whileInUse);
      expect(fake.requestPermissionCalls, 0,
          reason:
              'already-granted state should not trigger another system prompt');
    });

    test('returns unresolved result when services are disabled', () async {
      fake.serviceEnabled = false;

      final result =
          await DeviceLocationService.requestAndGetCurrentPosition();

      expect(result.position, isNull);
      expect(fake.requestPermissionCalls, 0);
    });

    test('returns result without throwing when Geolocator throws', () async {
      fake.initialPermission = LocationPermission.whileInUse;
      fake.position = null;

      final result =
          await DeviceLocationService.requestAndGetCurrentPosition();

      expect(result.position, isNull,
          reason: 'errors are swallowed; callers never see exceptions');
      expect(result.permission, LocationPermission.whileInUse);
    });
  });
}
