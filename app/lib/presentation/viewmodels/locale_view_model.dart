import 'dart:async';
import 'dart:ui';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/user_providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

const _kLocaleKey = 'app_locale_language_code';

final _log = Logger('LocalePreferenceNotifier');

/// localePreferenceProvider manages the user's preferred locale.
///
/// A null value means "use system locale". Backed by SharedPreferences
/// for offline persistence, and mirrored to the server via
/// [UserRepository.updatePreferredLanguage] on every change so the
/// server can render push notifications and emails in the right
/// language without waiting for a client request.
final localePreferenceProvider =
    AsyncNotifierProvider<LocalePreferenceNotifier, Locale?>(
  LocalePreferenceNotifier.new,
);

/// LocalePreferenceNotifier persists and restores the user's preferred locale.
class LocalePreferenceNotifier extends AsyncNotifier<Locale?> {
  @override
  Future<Locale?> build() async {
    final prefs = await SharedPreferences.getInstance();
    final code = prefs.getString(_kLocaleKey);
    if (code == null) return null;
    return Locale(code);
  }

  /// setLocale updates the preferred locale and persists it both
  /// locally (SharedPreferences) and to the server (UserRepository,
  /// fire-and-forget).
  ///
  /// Pass null to reset to the system locale; this clears both the
  /// local preference and the server-side `preferred_language` so
  /// the server falls back to header-based resolution.
  Future<void> setLocale(Locale? locale) async {
    final prefs = await SharedPreferences.getInstance();
    if (locale == null) {
      await prefs.remove(_kLocaleKey);
    } else {
      await prefs.setString(_kLocaleKey, locale.languageCode);
    }

    // The shared-preferences write completes on a background isolate
    // and may resolve after the user navigates away from the picker.
    // Guard the state assignment so we do not write into a disposed
    // notifier — locale_view_model is not autoDispose, but tests
    // dispose containers mid-flight and the contract is the same.
    if (!ref.mounted) return;
    state = AsyncValue.data(locale);

    _persistToServer(locale);
  }

  /// _persistToServer mirrors the new locale to the user record on
  /// the server. Fire-and-forget by design: a failed server write
  /// must not block or undo the local preference change, and the
  /// next RPC's Accept-Language header still carries the new locale
  /// even if the persisted value lags.
  ///
  /// Wrapped in try/catch because the dependency providers
  /// (authStateProvider, userRepositoryProvider) may themselves
  /// throw during early app boot or in tightly-scoped tests; either
  /// case is harmless for the locale flow and must not blow up the
  /// UI write that just succeeded.
  void _persistToServer(Locale? locale) {
    try {
      final auth = ref.read(authStateProvider);
      final userId = auth.user?.id;
      if (userId == null || userId.isEmpty) {
        // Not yet signed in — nothing to persist. The locale will
        // be captured into preferred_language on the next
        // registration / login from the Accept-Language header.
        return;
      }

      final repo = ref.read(userRepositoryProvider);
      unawaited(
        repo
            .updatePreferredLanguage(userId, locale?.languageCode ?? '')
            .catchError((Object e, StackTrace st) {
          _log.warning(
              'Failed to persist preferred_language to server', e, st);
        }),
      );
    } catch (e, st) {
      _log.warning(
          'Failed to schedule preferred_language persistence', e, st);
    }
  }
}
