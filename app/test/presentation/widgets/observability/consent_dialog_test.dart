import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/settings.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/observability/consent_dialog.dart';
import 'package:ripls/services/providers.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('ObservabilityConsentDialog', () {
    Widget createTestWidget({
      bool isInitialPrompt = true,
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
          home: Scaffold(
            body: ObservabilityConsentDialog(
              isInitialPrompt: isInitialPrompt,
            ),
          ),
        ),
      );
    }

    testWidgets('displays initial prompt title', (tester) async {
      await tester.pumpWidget(createTestWidget(isInitialPrompt: true));
      await tester.pumpAndSettle();

      expect(find.text('Data Consent'), findsOneWidget);
    });

    testWidgets('displays settings title when not initial prompt',
        (tester) async {
      await tester.pumpWidget(createTestWidget(isInitialPrompt: false));
      await tester.pumpAndSettle();

      expect(find.text('Privacy & Data'), findsOneWidget);
    });

    testWidgets('displays all three toggle sections', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      expect(find.text('Crash Reporting'), findsOneWidget);
      expect(find.text('Performance Monitoring'), findsOneWidget);
      expect(find.text('Usage Analytics'), findsOneWidget);
    });

    testWidgets('displays footer buttons in initial prompt mode',
        (tester) async {
      await tester.pumpWidget(createTestWidget(isInitialPrompt: true));
      await tester.pumpAndSettle();

      expect(find.text('Accept All'), findsOneWidget);
      expect(find.widgetWithText(OutlinedButton, 'Save'), findsOneWidget);
    });

    testWidgets('shows save button in settings mode without accept all',
        (tester) async {
      await tester.pumpWidget(createTestWidget(isInitialPrompt: false));
      await tester.pumpAndSettle();

      expect(find.text('Accept All'), findsNothing);
      expect(find.widgetWithText(OutlinedButton, 'Save'), findsNothing);
      expect(find.widgetWithText(ElevatedButton, 'Save'), findsOneWidget);
    });

    testWidgets('toggles crash reporting switch', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      final switches = find.byType(Switch);
      expect(switches, findsNWidgets(3));

      await tester.tap(switches.first);
      await tester.pumpAndSettle();

      final Switch switchWidget = tester.widget<Switch>(switches.first);
      expect(switchWidget.value, isTrue);
    });

    testWidgets('toggles performance monitoring switch', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      final switches = find.byType(Switch);

      await tester.tap(switches.at(1));
      await tester.pumpAndSettle();

      final Switch switchWidget = tester.widget<Switch>(switches.at(1));
      expect(switchWidget.value, isTrue);
    });

    testWidgets('toggles analytics switch', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      final switches = find.byType(Switch);

      await tester.tap(switches.at(2));
      await tester.pumpAndSettle();

      final Switch switchWidget = tester.widget<Switch>(switches.at(2));
      expect(switchWidget.value, isTrue);
    });

    testWidgets('shows granted settings as enabled switches', (tester) async {
      await tester.pumpWidget(createTestWidget(
        initialSettings: ObservabilitySettings.allGranted,
      ));
      await tester.pumpAndSettle();

      final switches = find.byType(Switch);
      for (int i = 0; i < 3; i++) {
        final Switch switchWidget = tester.widget<Switch>(switches.at(i));
        expect(switchWidget.value, isTrue,
            reason: 'Switch $i should be enabled when granted');
      }
    });

    testWidgets('shows denied settings as disabled switches', (tester) async {
      await tester.pumpWidget(createTestWidget(
        initialSettings: ObservabilitySettings.allDenied,
      ));
      await tester.pumpAndSettle();

      final switches = find.byType(Switch);
      for (int i = 0; i < 3; i++) {
        final Switch switchWidget = tester.widget<Switch>(switches.at(i));
        expect(switchWidget.value, isFalse,
            reason: 'Switch $i should be disabled when denied');
      }
    });

    testWidgets('displays subtitle text for each section', (tester) async {
      await tester.pumpWidget(createTestWidget());
      await tester.pumpAndSettle();

      expect(find.text('Help us fix bugs by sharing crash reports'), findsOneWidget);
      expect(find.text('Share performance data to improve app speed'), findsOneWidget);
      expect(find.text('Share anonymous usage data to improve Ripls'), findsOneWidget);
    });
  });
}

/// Test implementation of ObservabilitySettingsNotifier that doesn't use SharedPreferences.
class _TestObservabilitySettingsNotifier extends ObservabilitySettingsNotifier {
  final ObservabilitySettings _initialSettings;

  _TestObservabilitySettingsNotifier(this._initialSettings);

  @override
  ObservabilitySettings build() => _initialSettings;
}
