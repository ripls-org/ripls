import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/services/fcm_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Provider for FCM service
final fcmServiceProvider = Provider<FCMService>((ref) {
  // Keep the single FCM service alive for the app's lifetime. It holds the
  // imperatively-wired onNavigate callback and message-dedup state; letting it
  // auto-dispose (and rebuild) would strand a null callback and silently drop
  // notification deep links. The holder-based recovery (#2636) still covers a
  // rebuild if one ever happens, but keeping the instance stable is the first
  // line of defense.
  ref.keepAlive();
  return FCMService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    errorHandler: ref.watch(rpcErrorHandlerProvider),
    // Deep-link diagnostics (#2636): make a dropped notification tap queryable
    // in prod. Read (not watch) the observability service at emit time so this
    // sink never re-creates the FCM service. No-ops without analytics consent.
    logAnalyticsEvent: (event) =>
        ref.read(observabilityServiceProvider).logAnalyticsEvent(event),
  );
});

/// Provider for theme mode (system/light/dark).
/// Persists the user's preference to SharedPreferences and defaults to
/// ThemeMode.system so the app follows the OS setting on first launch.
class ThemeModeNotifier extends Notifier<ThemeMode> {
  static const _prefKey = 'theme_mode';

  @override
  ThemeMode build() {
    _loadSavedTheme();
    return ThemeMode.system;
  }

  Future<void> _loadSavedTheme() async {
    final prefs = await SharedPreferences.getInstance();
    final saved = prefs.getString(_prefKey);
    if (saved != null) {
      state = ThemeMode.values.firstWhere(
        (m) => m.name == saved,
        orElse: () => ThemeMode.system,
      );
    }
  }

  Future<void> setThemeMode(ThemeMode mode) async {
    state = mode;
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_prefKey, mode.name);
  }
}

final themeModeProvider = NotifierProvider<ThemeModeNotifier, ThemeMode>(() {
  return ThemeModeNotifier();
});
