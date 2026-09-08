import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('LocationPermissionHelper');

/// Prompts for location permission at a user-gesture site, returning the
/// resolved [Position] when available.
///
/// If the permission is (or becomes) permanently denied — common after a
/// single "deny" on Android 11+ which auto-promotes the state to
/// `deniedForever` and no longer shows the system dialog — this shows an
/// in-app dialog that offers to open the OS app settings so the user can
/// enable location manually.
///
/// Pass [suppressSettingsDialog] as `true` from passive entry points (e.g.
/// the global `+` tap) where the system permission prompt is still wanted on
/// first use, but the in-app settings-redirect alert should not appear after
/// the user has already refused.
///
/// On a successful grant, invalidates [userLocationProvider] so background
/// consumers (search proximity, map camera, creation autofill) pick up the
/// live GPS fix.
///
/// Returns null when location is unavailable for any reason (denied,
/// permanently denied, services off, error). Never throws.
Future<Position?> requestLocationWithSettingsFallback(
  BuildContext context,
  WidgetRef ref, {
  bool suppressSettingsDialog = false,
}) async {
  final stopwatch = Stopwatch()..start();
  final result = await DeviceLocationService.requestAndGetCurrentPosition();
  stopwatch.stop();
  if (result.position != null) {
    _log.info('Location granted; invalidating userLocationProvider');
    ref.invalidate(userLocationProvider(LocationIntent.proximityBias));
    ref.invalidate(userLocationProvider(LocationIntent.precisePin));
    return result.position;
  }

  _log.info('Location null; permission=${result.permission} '
      '(service call took ${stopwatch.elapsedMilliseconds} ms)');

  // Only the `deniedForever` state is a candidate for showing our settings
  // dialog. A fresh `denied` means the user just tapped "Don't allow" in
  // the system prompt — respect that and stay silent. `unableToDetermine`
  // (services disabled, error) is also silent.
  if (result.permission != LocationPermission.deniedForever) {
    return null;
  }

  // If the service call took long enough for the system dialog to have been
  // shown and for the user to respond, the user just actively said no in
  // this gesture — respect it. If it returned near-instantly, no system
  // dialog was shown (state was already deniedForever), so the user has no
  // visible feedback from the gesture without our dialog.
  const systemDialogInteractionThresholdMs = 200;
  if (stopwatch.elapsedMilliseconds >= systemDialogInteractionThresholdMs) {
    _log.info('System dialog appears to have been shown and declined this '
        'attempt; suppressing settings dialog to respect user choice');
    return null;
  }

  if (suppressSettingsDialog) {
    _log.info('deniedForever state observed; suppressing settings dialog '
        'per caller (passive entry point)');
    return null;
  }

  if (!context.mounted) {
    _log.warning('Context unmounted before settings dialog could be shown');
    return null;
  }
  _log.info('Showing open-settings dialog for pre-existing deniedForever state');
  final shouldOpenSettings = await showDialog<bool>(
    context: context,
    // Use the root navigator so the dialog displays above any modal bottom
    // sheet or nested navigator we were called from (e.g. the location
    // picker modal).
    useRootNavigator: true,
    builder: (dialogContext) => AlertDialog(
      backgroundColor: AppColors.surface(dialogContext),
      title: Text(
        dialogContext.l10n.locationPermissionDeniedTitle,
        style: TextStyle(color: AppColors.textPrimary(dialogContext)),
      ),
      content: Text(
        dialogContext.l10n.locationPermissionDeniedBody,
        style: TextStyle(color: AppColors.textSecondary(dialogContext)),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(dialogContext).pop(false),
          child: Text(
            dialogContext.l10n.commonCancel,
            style: TextStyle(color: AppColors.textSecondary(dialogContext)),
          ),
        ),
        TextButton(
          onPressed: () => Navigator.of(dialogContext).pop(true),
          child: Text(
            dialogContext.l10n.commonOpenSettings,
            style: TextStyle(color: AppColors.textPrimary(dialogContext)),
          ),
        ),
      ],
    ),
  );

  if (shouldOpenSettings ?? false) {
    await Geolocator.openAppSettings();
  }
  return null;
}
