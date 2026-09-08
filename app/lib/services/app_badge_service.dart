import 'dart:io' show Platform;

import 'package:app_badge_plus/app_badge_plus.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:logging/logging.dart';

final _log = Logger('AppBadgeService');

/// AppBadgeService manages the app icon badge count.
/// Note: Badge support is only enabled on iOS and macOS.
class AppBadgeService {
  /// UpdateBadge sets the app icon badge to the specified count.
  /// If count is 0, the badge is removed.
  /// On Android and Web, this is a no-op.
  static Future<void> updateBadge(int count) async {
    // Skip badge operations on web (app_badge_plus has no web
    // implementation; dart:io Platform throws on web — kIsWeb must
    // be checked first).
    if (kIsWeb) {
      return;
    }
    if (Platform.isAndroid) {
      _log.fine('📱 App badges not supported on Android');
      return;
    }

    try {
      final isSupported = await AppBadgePlus.isSupported();

      if (!isSupported) {
        _log.info('📱 App badges not supported on this device');
        return;
      }

      if (count > 0) {
        await AppBadgePlus.updateBadge(count);
        _log.info('📱 Updated app badge count to $count');
      } else {
        await AppBadgePlus.updateBadge(0);
        _log.info('📱 Removed app badge');
      }
    } catch (e) {
      _log.warning('⚠️ Failed to update app badge: $e');
    }
  }
  }
