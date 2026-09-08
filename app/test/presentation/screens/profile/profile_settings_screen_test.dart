import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/settings.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/profile/profile_settings_screen.dart';
import 'package:ripls/services/providers.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('ProfileSettingsScreen', () {
    Widget createTestWidget({
      ObservabilitySettings initialSettings = ObservabilitySettings.notAsked,
    }) {
      return ProviderScope(
        overrides: [
          observabilitySettingsProvider.overrideWith(
            () => _TestObservabilitySettingsNotifier(initialSettings),
          ),
        ],
        child: MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: ProfileSettingsScreen(),
        ),
      );
    }

    testWidgets('displays User Settings title in app bar', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      expect(find.text('Settings'), findsOneWidget);
    });

    testWidgets('displays Privacy & Data section header', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      expect(find.text('Privacy & Data'), findsOneWidget);
    });

    testWidgets('displays all three privacy toggles', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      expect(find.text('Crash Reporting'), findsOneWidget);
      expect(find.text('Performance Monitoring'), findsOneWidget);
      expect(find.text('Usage Analytics'), findsOneWidget);
    });

    testWidgets('displays toggle subtitles', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      expect(
        find.text('Help us fix bugs by sharing crash reports'),
        findsOneWidget,
      );
      expect(
        find.text('Share performance data to improve app speed'),
        findsOneWidget,
      );
      expect(
        find.text('Share anonymous usage data to improve Ripls'),
        findsOneWidget,
      );
    });

    testWidgets('displays More Settings section', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      expect(find.text('More Settings'), findsOneWidget);
      expect(find.text('Additional settings coming soon'), findsOneWidget);
    });

    testWidgets('shows switches as off when settings are notAsked',
        (tester) async {
      await tester.pumpWidget(createTestWidget(
        initialSettings: ObservabilitySettings.notAsked,
      ));
      await tester.pumpAndSettle();

      final switches = find.byType(Switch);
      expect(switches, findsNWidgets(3)); // 3 observability toggles

      for (int i = 0; i < 3; i++) {
        final Switch switchWidget = tester.widget<Switch>(switches.at(i));
        expect(switchWidget.value, isFalse,
            reason: 'Switch $i should be off when notAsked');
      }
    });

    testWidgets('shows switches as on when settings are granted',
        (tester) async {
      await tester.pumpWidget(createTestWidget(
        initialSettings: ObservabilitySettings.allGranted,
      ));
      await tester.pumpAndSettle();

      final switches = find.byType(Switch);
      expect(switches, findsNWidgets(3)); // 3 observability toggles

      for (int i = 0; i < 3; i++) {
        final Switch switchWidget = tester.widget<Switch>(switches.at(i));
        expect(switchWidget.value, isTrue,
            reason: 'Switch $i should be on when granted');
      }
    });

    testWidgets('shows switches as off when settings are denied',
        (tester) async {
      await tester.pumpWidget(createTestWidget(
        initialSettings: ObservabilitySettings.allDenied,
      ));
      await tester.pumpAndSettle();

      final switches = find.byType(Switch);
      expect(switches, findsNWidgets(3)); // 3 observability toggles

      for (int i = 0; i < 3; i++) {
        final Switch switchWidget = tester.widget<Switch>(switches.at(i));
        expect(switchWidget.value, isFalse,
            reason: 'Switch $i should be off when denied');
      }
    });

    testWidgets('toggles crash reporting switch', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      final switches = find.byType(Switch);
      expect(switches, findsNWidgets(3)); // 3 observability toggles

      // Toggle first switch (crash reporting)
      await tester.tap(switches.first);
      await tester.pumpAndSettle();

      final Switch switchWidget = tester.widget<Switch>(switches.first);
      expect(switchWidget.value, isTrue);
    });

    testWidgets('displays loading indicator when isLoading is true',
        (tester) async {
      await tester.pumpWidget(createTestWidget(
        initialSettings: const ObservabilitySettings(isLoading: true),
      ));
      await tester.pump();

      expect(find.byType(CircularProgressIndicator), findsOneWidget);
    });

    testWidgets('back button navigates back', (tester) async {
      bool didPop = false;

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            observabilitySettingsProvider.overrideWith(
              () =>
                  _TestObservabilitySettingsNotifier(ObservabilitySettings.notAsked),
            ),
          ],
          child: MaterialApp(
            localizationsDelegates: const [
              AppLocalizations.delegate,
              GlobalMaterialLocalizations.delegate,
              GlobalWidgetsLocalizations.delegate,
              GlobalCupertinoLocalizations.delegate,
            ],
            supportedLocales: AppLocalizations.supportedLocales,
            home: Builder(
              builder: (context) => ElevatedButton(
                onPressed: () {
                  Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) => const ProfileSettingsScreen(),
                    ),
                  );
                },
                child: const Text('Go to Settings'),
              ),
            ),
            navigatorObservers: [
              _TestNavigatorObserver(
                onDidPop: (route, previousRoute) {
                  didPop = true;
                },
              ),
            ],
          ),
        ),
      );
      await tester.pumpAndSettle();

      // Navigate to settings
      await tester.tap(find.text('Go to Settings'));
      await tester.pumpAndSettle();

      // Tap back button
      await tester.tap(find.byIcon(Icons.arrow_back_ios_new));
      await tester.pumpAndSettle();

      expect(didPop, isTrue);
    });
  });
}

/// Test implementation of ObservabilitySettingsNotifier that doesn't use SharedPreferences.
class _TestObservabilitySettingsNotifier extends ObservabilitySettingsNotifier {
  final ObservabilitySettings _initialSettings;

  _TestObservabilitySettingsNotifier(this._initialSettings);

  @override
  ObservabilitySettings build() => _initialSettings;

  @override
  Future<void> setCrashReporting(ObservabilityConsent consent) async {
    state = state.copyWith(crashReporting: consent);
  }

  @override
  Future<void> setPerformanceMonitoring(ObservabilityConsent consent) async {
    state = state.copyWith(performanceMonitoring: consent);
  }

  @override
  Future<void> setAnalytics(ObservabilityConsent consent) async {
    state = state.copyWith(analytics: consent);
  }
}

/// Test navigator observer to track navigation events.
class _TestNavigatorObserver extends NavigatorObserver {
  final void Function(Route<dynamic>?, Route<dynamic>?)? onDidPop;

  _TestNavigatorObserver({this.onDidPop});

  @override
  void didPop(Route<dynamic> route, Route<dynamic>? previousRoute) {
    onDidPop?.call(route, previousRoute);
  }
}
