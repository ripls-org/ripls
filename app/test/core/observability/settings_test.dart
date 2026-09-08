import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/settings.dart';

void main() {
  // Initialize SharedPreferences mock for all tests
  TestWidgetsFlutterBinding.ensureInitialized();

  group('ObservabilityConsent', () {
    test('has all expected values', () {
      expect(ObservabilityConsent.values, hasLength(3));
      expect(ObservabilityConsent.notAsked, isNotNull);
      expect(ObservabilityConsent.granted, isNotNull);
      expect(ObservabilityConsent.denied, isNotNull);
    });
  });

  group('ObservabilitySettings', () {
    test('default constructor has all notAsked', () {
      const settings = ObservabilitySettings();
      expect(settings.crashReporting, ObservabilityConsent.notAsked);
      expect(settings.performanceMonitoring, ObservabilityConsent.notAsked);
      expect(settings.analytics, ObservabilityConsent.notAsked);
      expect(settings.isLoading, true);
    });

    test('notAsked constant has correct values', () {
      expect(ObservabilitySettings.notAsked.crashReporting, ObservabilityConsent.notAsked);
      expect(ObservabilitySettings.notAsked.performanceMonitoring, ObservabilityConsent.notAsked);
      expect(ObservabilitySettings.notAsked.analytics, ObservabilityConsent.notAsked);
      expect(ObservabilitySettings.notAsked.isLoading, false);
    });

    test('allGranted constant has correct values', () {
      expect(ObservabilitySettings.allGranted.crashReporting, ObservabilityConsent.granted);
      expect(ObservabilitySettings.allGranted.performanceMonitoring, ObservabilityConsent.granted);
      expect(ObservabilitySettings.allGranted.analytics, ObservabilityConsent.granted);
      expect(ObservabilitySettings.allGranted.isLoading, false);
    });

    test('allDenied constant has correct values', () {
      expect(ObservabilitySettings.allDenied.crashReporting, ObservabilityConsent.denied);
      expect(ObservabilitySettings.allDenied.performanceMonitoring, ObservabilityConsent.denied);
      expect(ObservabilitySettings.allDenied.analytics, ObservabilityConsent.denied);
      expect(ObservabilitySettings.allDenied.isLoading, false);
    });

    test('anyEnabled returns true when crash reporting is granted', () {
      const settings = ObservabilitySettings(
        crashReporting: ObservabilityConsent.granted,
        isLoading: false,
      );
      expect(settings.anyEnabled, true);
    });

    test('anyEnabled returns true when performance monitoring is granted', () {
      const settings = ObservabilitySettings(
        performanceMonitoring: ObservabilityConsent.granted,
        isLoading: false,
      );
      expect(settings.anyEnabled, true);
    });

    test('anyEnabled returns true when analytics is granted', () {
      const settings = ObservabilitySettings(
        analytics: ObservabilityConsent.granted,
        isLoading: false,
      );
      expect(settings.anyEnabled, true);
    });

    test('anyEnabled returns false when all denied', () {
      expect(ObservabilitySettings.allDenied.anyEnabled, false);
    });

    test('anyEnabled returns false when all notAsked', () {
      expect(ObservabilitySettings.notAsked.anyEnabled, false);
    });

    test('needsConsentPrompt returns true when all notAsked', () {
      expect(ObservabilitySettings.notAsked.needsConsentPrompt, true);
    });

    test('needsConsentPrompt returns false when any has been set', () {
      const settings = ObservabilitySettings(
        crashReporting: ObservabilityConsent.denied,
        isLoading: false,
      );
      expect(settings.needsConsentPrompt, false);
    });

    test('hasBeenAsked is inverse of needsConsentPrompt', () {
      expect(ObservabilitySettings.notAsked.hasBeenAsked, false);
      expect(ObservabilitySettings.allGranted.hasBeenAsked, true);
      expect(ObservabilitySettings.allDenied.hasBeenAsked, true);
    });

    test('copyWith creates new instance with updated values', () {
      const original = ObservabilitySettings(
        crashReporting: ObservabilityConsent.notAsked,
        performanceMonitoring: ObservabilityConsent.notAsked,
        analytics: ObservabilityConsent.notAsked,
        isLoading: true,
      );

      final updated = original.copyWith(
        crashReporting: ObservabilityConsent.granted,
      );

      expect(updated.crashReporting, ObservabilityConsent.granted);
      expect(updated.performanceMonitoring, ObservabilityConsent.notAsked);
      expect(updated.analytics, ObservabilityConsent.notAsked);
      expect(updated.isLoading, true);
    });

    test('copyWith preserves values when not specified', () {
      const original = ObservabilitySettings.allGranted;
      final updated = original.copyWith();

      expect(updated, original);
    });

    test('equality works correctly', () {
      const a = ObservabilitySettings(
        crashReporting: ObservabilityConsent.granted,
        performanceMonitoring: ObservabilityConsent.denied,
        analytics: ObservabilityConsent.notAsked,
        isLoading: false,
      );

      const b = ObservabilitySettings(
        crashReporting: ObservabilityConsent.granted,
        performanceMonitoring: ObservabilityConsent.denied,
        analytics: ObservabilityConsent.notAsked,
        isLoading: false,
      );

      const c = ObservabilitySettings(
        crashReporting: ObservabilityConsent.denied,
        performanceMonitoring: ObservabilityConsent.denied,
        analytics: ObservabilityConsent.notAsked,
        isLoading: false,
      );

      expect(a, equals(b));
      expect(a, isNot(equals(c)));
      expect(a.hashCode, equals(b.hashCode));
    });

    test('toString returns readable string', () {
      const settings = ObservabilitySettings.allGranted;
      final str = settings.toString();

      expect(str, contains('crash'));
      expect(str, contains('perf'));
      expect(str, contains('analytics'));
      expect(str, contains('granted'));
    });
  });

  // NOTE: ObservabilitySettingsNotifier tests are skipped because SharedPreferencesAsync
  // requires platform-specific setup that differs from the sync SharedPreferences mock.
  // The notifier follows the same pattern as AuthStateNotifier and will be tested via
  // integration tests. The data classes above have full unit test coverage.
  //
  // To run integration tests with real SharedPreferences:
  // flutter test integration_test/
}
