import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:plugin_platform_interface/plugin_platform_interface.dart';
import 'package:ripls/core/utils/location_permission_helper.dart';

import '../../helpers/l10n_helpers.dart';

class _FakeGeolocator extends GeolocatorPlatform
    with MockPlatformInterfaceMixin {
  _FakeGeolocator({
    required this.serviceEnabled,
    required this.initialPermission,
    this.requestedPermission,
    this.position,
    this.checkPermissionDelay = Duration.zero,
  });

  bool serviceEnabled;
  LocationPermission initialPermission;
  LocationPermission? requestedPermission;
  Position? position;
  Duration checkPermissionDelay;

  @override
  Future<bool> isLocationServiceEnabled() async => serviceEnabled;

  @override
  Future<LocationPermission> checkPermission() async {
    if (checkPermissionDelay > Duration.zero) {
      await Future.delayed(checkPermissionDelay);
    }
    return initialPermission;
  }

  @override
  Future<LocationPermission> requestPermission() async {
    final resolved = requestedPermission ?? initialPermission;
    initialPermission = resolved;
    return resolved;
  }

  @override
  Future<Position?> getLastKnownPosition({
    bool forceLocationManager = false,
  }) async =>
      null;

  @override
  Future<Position> getCurrentPosition({
    LocationSettings? locationSettings,
  }) async {
    final p = position;
    if (p == null) throw LocationServiceDisabledException();
    return p;
  }
}

Position _boulderPosition() => Position(
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

/// Pumps a [ProviderScope]+[MaterialApp] tree and calls [fn] via a button tap.
/// Returns the value [fn] produced (null if [fn] is still awaiting, e.g. an
/// open dialog).
Future<Position?> _pump(
  WidgetTester tester,
  _FakeGeolocator fake,
  Future<Position?> Function(BuildContext, WidgetRef) fn, {
  bool withLocalizations = false,
}) async {
  GeolocatorPlatform.instance = fake;
  Position? result;
  final consumer = Consumer(
    builder: (context, ref, _) => ElevatedButton(
      onPressed: () async {
        result = await fn(context, ref);
      },
      child: const Text('tap'),
    ),
  );
  await tester.pumpWidget(
    ProviderScope(
      child: withLocalizations
          ? localizedApp(consumer)
          : MaterialApp(home: consumer),
    ),
  );
  await tester.tap(find.text('tap'));
  await tester.pumpAndSettle();
  return result;
}

void main() {
  group('requestLocationWithSettingsFallback', () {
    group('suppressSettingsDialog: true', () {
      testWidgets(
        'deniedForever with fast call returns null without showing a dialog',
        (tester) async {
          final result = await _pump(
            tester,
            _FakeGeolocator(
              serviceEnabled: true,
              initialPermission: LocationPermission.deniedForever,
            ),
            (context, ref) => requestLocationWithSettingsFallback(
              context,
              ref,
              suppressSettingsDialog: true,
            ),
          );

          expect(result, isNull);
          expect(find.byType(AlertDialog), findsNothing);
        },
      );

      testWidgets(
        'granted returns the position without showing a dialog',
        (tester) async {
          final result = await _pump(
            tester,
            _FakeGeolocator(
              serviceEnabled: true,
              initialPermission: LocationPermission.whileInUse,
              position: _boulderPosition(),
            ),
            (context, ref) => requestLocationWithSettingsFallback(
              context,
              ref,
              suppressSettingsDialog: true,
            ),
          );

          expect(result, isNotNull);
          expect(result?.latitude, 40.015);
          expect(find.byType(AlertDialog), findsNothing);
        },
      );

      testWidgets(
        'fresh denied returns null without a dialog',
        (tester) async {
          final result = await _pump(
            tester,
            _FakeGeolocator(
              serviceEnabled: true,
              initialPermission: LocationPermission.denied,
              requestedPermission: LocationPermission.denied,
            ),
            (context, ref) => requestLocationWithSettingsFallback(
              context,
              ref,
              suppressSettingsDialog: true,
            ),
          );

          expect(result, isNull);
          expect(find.byType(AlertDialog), findsNothing);
        },
      );

      testWidgets(
        'deniedForever with slow call (elapsed >= threshold) returns null via '
        'the existing elapsed-time branch — suppressSettingsDialog is a no-op',
        (tester) async {
          // The stopwatch-elapsed check fires before the suppressSettingsDialog
          // check, so the flag makes no difference here. Asserts no regression.
          final fake = _FakeGeolocator(
            serviceEnabled: true,
            initialPermission: LocationPermission.deniedForever,
            checkPermissionDelay: const Duration(milliseconds: 250),
          );
          Position? result;
          await tester.runAsync(() async {
            GeolocatorPlatform.instance = fake;
            final consumer = Consumer(
              builder: (context, ref, _) => ElevatedButton(
                onPressed: () async {
                  result = await requestLocationWithSettingsFallback(
                    context,
                    ref,
                    suppressSettingsDialog: true,
                  );
                },
                child: const Text('tap'),
              ),
            );
            await tester.pumpWidget(
              ProviderScope(child: MaterialApp(home: consumer)),
            );
            await tester.tap(find.text('tap'));
            // Allow real wall-clock time to pass so Stopwatch reads >= 200ms.
            await Future.delayed(const Duration(milliseconds: 350));
          });
          await tester.pumpAndSettle();

          expect(result, isNull);
          expect(find.byType(AlertDialog), findsNothing);
        },
      );
    });

    group('suppressSettingsDialog: false (default)', () {
      testWidgets(
        'deniedForever with fast call shows the open-settings dialog',
        (tester) async {
          // Regression guard: explicit-gesture call sites (location picker,
          // discover map) must still see the settings-redirect dialog.
          await _pump(
            tester,
            _FakeGeolocator(
              serviceEnabled: true,
              initialPermission: LocationPermission.deniedForever,
            ),
            (context, ref) =>
                requestLocationWithSettingsFallback(context, ref),
            withLocalizations: true,
          );

          expect(find.byType(AlertDialog), findsOneWidget);
        },
      );
    });
  });
}
