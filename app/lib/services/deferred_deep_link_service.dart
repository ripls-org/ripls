import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:play_install_referrer/play_install_referrer.dart';
import 'package:shared_preferences/shared_preferences.dart';

final _log = Logger('DeferredDeepLinkService');

/// Key used to persist whether the deferred deep link has already been consumed.
const _deferredConsumedKey = 'deferred_deep_link_consumed';

/// Context extracted from a deferred deep link (e.g., Play Install Referrer).
///
/// Contains the invitation short code parsed from the referrer string.
/// Resolution (looking up invitation details and the deep-link target)
/// happens in the ViewModel layer, not here.
class DeferredDeepLinkContext {
  final String? shortCode;

  const DeferredDeepLinkContext({
    this.shortCode,
  });

  /// Whether any actionable context was extracted.
  bool get hasContext => shortCode != null;
}

/// Extracts deferred deep link context from platform-specific APIs.
///
/// On Android, reads the Play Install Referrer to recover invite codes and
/// item IDs that were encoded in the Play Store URL by the landing page.
/// On iOS, reads the clipboard for a `ripls://invite?token=...` URL that the
/// landing page copied before redirecting to the App Store. iOS 16+ shows a
/// system paste permission banner automatically.
///
/// This service only handles raw extraction from platform APIs. It does NOT
/// resolve codes via RPCs or repositories — that happens in ViewModels.
class DeferredDeepLinkService {
  /// Returns true if this is the first launch (deferred context has never
  /// been checked). Used to route new users to registration instead of login.
  Future<bool> isFirstLaunch() async {
    final prefs = await SharedPreferences.getInstance();
    return prefs.getBool(_deferredConsumedKey) != true;
  }

  /// Retrieves deferred deep link context if available and not yet consumed.
  ///
  /// Returns null if:
  /// - The platform is not Android or iOS
  /// - The context has already been consumed
  /// - No referrer/clipboard data is available
  /// - The data does not contain Ripls context
  Future<DeferredDeepLinkContext?> getContext() async {
    // Only read once per install.
    final prefs = await SharedPreferences.getInstance();
    if (prefs.getBool(_deferredConsumedKey) ?? false) {
      _log.fine('Deferred deep link already consumed, skipping');
      return null;
    }

    // Mark consumed in a finally block to guarantee it runs even if the
    // platform API throws unexpectedly. Without this, a crash in the
    // Install Referrer or clipboard read would cause isFirstLaunch() to
    // return true on every subsequent launch.
    try {
      DeferredDeepLinkContext? context;
      if (Platform.isAndroid) {
        context = await _getAndroidContext(prefs);
      } else if (Platform.isIOS) {
        context = await _getIOSContext(prefs);
      }
      return context;
    } finally {
      await _markConsumed(prefs);
    }
  }

  /// Reads the Play Install Referrer on Android.
  Future<DeferredDeepLinkContext?> _getAndroidContext(
      SharedPreferences prefs) async {
    try {
      final details = await PlayInstallReferrer.installReferrer;
      final referrer = details.installReferrer;

      if (referrer == null || referrer.isEmpty) {
        _log.info('No install referrer found');
        return null;
      }

      _log.info('Install referrer found, parsing context');
      final context = parseReferrer(referrer);

      if (!context.hasContext) {
        _log.info('Install referrer did not contain Ripls context');
        return null;
      }

      _log.info('Extracted deferred deep link context from install referrer');
      return context;
    } catch (e) {
      _log.warning('Failed to read install referrer: $e');
      return null;
    }
  }

  /// Reads the clipboard on iOS for a `ripls://` deep link URL.
  ///
  /// The landing page copies `ripls://invite?token=CODE` to the clipboard
  /// before redirecting to the App Store. On iOS 16+, reading the
  /// clipboard triggers a system paste permission banner ("Ripls would like
  /// to paste from Safari").
  Future<DeferredDeepLinkContext?> _getIOSContext(
      SharedPreferences prefs) async {
    try {
      final clipData = await Clipboard.getData(Clipboard.kTextPlain);
      final text = clipData?.text;

      if (text == null || text.isEmpty) {
        _log.info('Clipboard is empty');
        return null;
      }

      final context = parseClipboardUrl(text);

      if (context == null || !context.hasContext) {
        _log.fine('Clipboard did not contain a Ripls deep link');
        return null;
      }

      _log.info('Extracted deferred deep link context from clipboard');

      // Clear the clipboard after successful read (good hygiene — the deep
      // link URL shouldn't linger in the user's pasteboard).
      await Clipboard.setData(const ClipboardData(text: ''));

      return context;
    } catch (e) {
      _log.warning('Failed to read clipboard: $e');
      return null;
    }
  }

  Future<void> _markConsumed(SharedPreferences prefs) async {
    await prefs.setBool(_deferredConsumedKey, true);
  }

  /// Parses a Play Store referrer string into deferred deep link context.
  ///
  /// The landing page encodes context as URL-encoded query parameters:
  /// `utm_source=ripls&utm_content={code}`
  ///
  /// The referrer string from the Play Store is URL-decoded by the API.
  static DeferredDeepLinkContext parseReferrer(String referrer) {
    // The referrer string uses & as separator and = for key-value pairs,
    // which is standard URI query format.
    final params = Uri.splitQueryString(referrer);

    // Only process referrers from our landing page.
    if (params['utm_source'] != 'ripls') {
      return const DeferredDeepLinkContext();
    }

    return DeferredDeepLinkContext(
      shortCode: _nonEmpty(params['utm_content']),
    );
  }

  /// Parses a clipboard URL into deferred deep link context.
  ///
  /// The landing page copies `ripls://invite?token=CODE` to the clipboard on
  /// iOS. Returns null if the text is not a valid `ripls://` URL.
  static DeferredDeepLinkContext? parseClipboardUrl(String text) {
    final trimmed = text.trim();

    final uri = Uri.tryParse(trimmed);
    if (uri == null || uri.scheme != 'ripls' || uri.host != 'invite') {
      return null;
    }

    final token = _nonEmpty(uri.queryParameters['token']);
    if (token == null) return null;

    return DeferredDeepLinkContext(
      shortCode: token,
    );
  }

  static String? _nonEmpty(String? value) =>
      (value != null && value.isNotEmpty) ? value : null;
}

/// Provider for the deferred deep link service.
final deferredDeepLinkServiceProvider =
    Provider<DeferredDeepLinkService>((ref) {
  return DeferredDeepLinkService();
});

/// Holds the deferred deep link context after it has been read during app
/// initialization. The router reads this synchronously in its redirect logic
/// to route new users to the invite flow.
///
/// Uses a static field (not a Riverpod provider) because the router redirect
/// runs during the widget build phase, where modifying Riverpod state throws.
/// Set once during startup, then consumed after the first redirect.
class DeferredDeepLinkContextHolder {
  static DeferredDeepLinkContext? value;

  /// Consumes and returns the current context, resetting it to null.
  static DeferredDeepLinkContext? consume() {
    final current = value;
    value = null;
    return current;
  }
}

/// Tracks whether the current app launch is the first launch after install.
/// Uses a static field (not a provider) because the router redirect reads
/// it synchronously and must see it before any provider refresh cycle.
class FirstLaunchFlag {
  static bool value = false;
}
