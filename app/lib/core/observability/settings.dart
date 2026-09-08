import 'package:flutter/foundation.dart' show protected;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:shared_preferences/shared_preferences.dart';

final _log = Logger('ObservabilitySettings');

/// Consent state for each observability feature.
///
/// Users must opt-in to enable data collection (GDPR-friendly).
enum ObservabilityConsent {
  /// User has not been asked yet - will show consent dialog
  notAsked,

  /// User has explicitly opted in
  granted,

  /// User has explicitly opted out
  denied,
}

/// Holds the consent settings for all observability features.
///
/// Each feature (crash reporting, performance monitoring, analytics)
/// can be independently enabled or disabled by the user.
///
/// Follows the same pattern as AuthStateData in auth_state.dart.
class ObservabilitySettings {
  final ObservabilityConsent crashReporting;
  final ObservabilityConsent performanceMonitoring;
  final ObservabilityConsent analytics;
  final bool isLoading;

  const ObservabilitySettings({
    this.crashReporting = ObservabilityConsent.notAsked,
    this.performanceMonitoring = ObservabilityConsent.notAsked,
    this.analytics = ObservabilityConsent.notAsked,
    this.isLoading = true,
  });

  /// Returns true if any observability feature is enabled.
  bool get anyEnabled =>
      crashReporting == ObservabilityConsent.granted ||
      performanceMonitoring == ObservabilityConsent.granted ||
      analytics == ObservabilityConsent.granted;

  /// Returns true if all features are in the notAsked state.
  bool get needsConsentPrompt =>
      crashReporting == ObservabilityConsent.notAsked &&
      performanceMonitoring == ObservabilityConsent.notAsked &&
      analytics == ObservabilityConsent.notAsked;

  /// Returns true if user has made any consent decisions.
  bool get hasBeenAsked => !needsConsentPrompt;

  ObservabilitySettings copyWith({
    ObservabilityConsent? crashReporting,
    ObservabilityConsent? performanceMonitoring,
    ObservabilityConsent? analytics,
    bool? isLoading,
  }) {
    return ObservabilitySettings(
      crashReporting: crashReporting ?? this.crashReporting,
      performanceMonitoring: performanceMonitoring ?? this.performanceMonitoring,
      analytics: analytics ?? this.analytics,
      isLoading: isLoading ?? this.isLoading,
    );
  }

  /// Default settings with all features not asked.
  static const notAsked = ObservabilitySettings(isLoading: false);

  /// Settings with all features granted (for testing or when user opts in to all).
  static const allGranted = ObservabilitySettings(
    crashReporting: ObservabilityConsent.granted,
    performanceMonitoring: ObservabilityConsent.granted,
    analytics: ObservabilityConsent.granted,
    isLoading: false,
  );

  /// Settings with all features denied.
  static const allDenied = ObservabilitySettings(
    crashReporting: ObservabilityConsent.denied,
    performanceMonitoring: ObservabilityConsent.denied,
    analytics: ObservabilityConsent.denied,
    isLoading: false,
  );

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is ObservabilitySettings &&
        other.crashReporting == crashReporting &&
        other.performanceMonitoring == performanceMonitoring &&
        other.analytics == analytics &&
        other.isLoading == isLoading;
  }

  @override
  int get hashCode =>
      crashReporting.hashCode ^
      performanceMonitoring.hashCode ^
      analytics.hashCode ^
      isLoading.hashCode;

  @override
  String toString() =>
      'ObservabilitySettings(crash: $crashReporting, perf: $performanceMonitoring, analytics: $analytics, loading: $isLoading)';
}

// SharedPreferences keys
const _keyCrashReporting = 'observability_crash_reporting';
const _keyPerformanceMonitoring = 'observability_performance_monitoring';
const _keyAnalytics = 'observability_analytics';

/// Manages observability consent settings with persistence.
///
/// Uses SharedPreferencesAsync to persist user consent choices across app restarts.
/// Follows the same pattern as AuthStateNotifier in auth_state.dart.
class ObservabilitySettingsNotifier extends Notifier<ObservabilitySettings> {
  /// SharedPreferencesAsync instance. Made protected for test overrides.
  @protected
  SharedPreferencesAsync get prefs => _prefs ??= SharedPreferencesAsync();
  SharedPreferencesAsync? _prefs;

  @override
  ObservabilitySettings build() => const ObservabilitySettings(isLoading: true);

  /// Load settings from persistent storage.
  ///
  /// Call this on app startup to restore user's consent choices.
  Future<void> loadSettings() async {
    try {
      final crashStr = await prefs.getString(_keyCrashReporting);
      final perfStr = await prefs.getString(_keyPerformanceMonitoring);
      final analyticsStr = await prefs.getString(_keyAnalytics);

      state = ObservabilitySettings(
        crashReporting: _parseConsent(crashStr),
        performanceMonitoring: _parseConsent(perfStr),
        analytics: _parseConsent(analyticsStr),
        isLoading: false,
      );

      _log.info('Loaded observability settings: $state');
    } catch (e) {
      _log.warning('Failed to load observability settings: $e');
      state = ObservabilitySettings.notAsked;
    }
  }

  /// Set crash reporting consent.
  Future<void> setCrashReporting(ObservabilityConsent consent) async {
    state = state.copyWith(crashReporting: consent);
    await prefs.setString(_keyCrashReporting, consent.name);
    _log.info('Set crash reporting consent: $consent');
  }

  /// Set performance monitoring consent.
  Future<void> setPerformanceMonitoring(ObservabilityConsent consent) async {
    state = state.copyWith(performanceMonitoring: consent);
    await prefs.setString(_keyPerformanceMonitoring, consent.name);
    _log.info('Set performance monitoring consent: $consent');
  }

  /// Set analytics consent.
  Future<void> setAnalytics(ObservabilityConsent consent) async {
    state = state.copyWith(analytics: consent);
    await prefs.setString(_keyAnalytics, consent.name);
    _log.info('Set analytics consent: $consent');
  }

  /// Set all consent settings at once.
  ///
  /// Useful for the initial consent dialog where user can opt-in/out of all features.
  Future<void> setAll({
    required ObservabilityConsent crashReporting,
    required ObservabilityConsent performanceMonitoring,
    required ObservabilityConsent analytics,
  }) async {
    state = ObservabilitySettings(
      crashReporting: crashReporting,
      performanceMonitoring: performanceMonitoring,
      analytics: analytics,
      isLoading: false,
    );

    await Future.wait([
      prefs.setString(_keyCrashReporting, crashReporting.name),
      prefs.setString(_keyPerformanceMonitoring, performanceMonitoring.name),
      prefs.setString(_keyAnalytics, analytics.name),
    ]);

    _log.info('Set all observability settings: $state');
  }

  
  
  
  ObservabilityConsent _parseConsent(String? value) {
    if (value == null) return ObservabilityConsent.notAsked;

    return ObservabilityConsent.values.firstWhere(
      (c) => c.name == value,
      orElse: () => ObservabilityConsent.notAsked,
    );
  }
}
