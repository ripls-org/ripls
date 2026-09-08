import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/feedback_sheet_view_model.dart';
import 'package:ripls/presentation/widgets/feedback/feedback_sheet.dart';
import 'package:ripls/services/providers.dart';

import '../../../helpers/contrast_helpers.dart';
import 'feedback_sheet_test.mocks.dart';

/// Contrast regression tests for the feedback sheet (#2798).
///
/// The reported defect: "Add Screenshots (optional)" and the selected type chip
/// painted `accentButtonBackground` (`DesignTokens.lightSurface`, #F2F2EE — a
/// light-THEME surface) as their own fill inside the always-dark glass sheet,
/// then drew on-glass foregrounds on top. The label measured **1.12:1** and the
/// chip **1.79:1**.
///
/// These pump the real sheet rather than asserting token constants, because the
/// constants were individually fine — it was the *pairing invented at the call
/// site* that failed, and only the tree shows that.
class _SubmittedNotifier extends FeedbackSheetNotifier {
  @override
  FeedbackSheetState build() => const FeedbackSheetState(isSubmitted: true);
}

void main() {
  late MockFeedbackRepository mockRepository;

  setUp(() {
    PackageInfo.setMockInitialValues(
      appName: 'Ripls Test',
      packageName: 'com.test.ripls',
      version: '1.0.0',
      buildNumber: '42',
      buildSignature: '',
    );
    mockRepository = MockFeedbackRepository();
  });

  Widget buildSheet({bool successState = false, double textScale = 1.0}) {
    return ProviderScope(
      overrides: [
        feedbackRepositoryProvider.overrideWithValue(mockRepository),
        if (successState)
          feedbackSheetProvider.overrideWith(() => _SubmittedNotifier()),
      ],
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: MediaQuery(
          data: MediaQueryData(textScaler: TextScaler.linear(textScale)),
          child: const Scaffold(body: FeedbackSheet()),
        ),
      ),
    );
  }

  group('FeedbackSheet contrast (#2798)', () {
    testWidgets('every opaque foreground/fill pair clears WCAG AA',
        (tester) async {
      await tester.pumpWidget(buildSheet());
      await tester.pumpAndSettle();

      expectOpaqueContrast(tester);
    });

    testWidgets('the success state clears WCAG AA', (tester) async {
      await tester.pumpWidget(buildSheet(successState: true));
      await tester.pumpAndSettle();

      expectOpaqueContrast(tester);
    });

    testWidgets('the selected type chip is legible on its own fill',
        (tester) async {
      await tester.pumpWidget(buildSheet());
      await tester.pumpAndSettle();

      // The selected chip is the sheet's only opaque fill, so it is the one
      // control this layer can measure directly. "Bug" is selected by default.
      final pairs = opaqueContrastPairs(tester);
      final chip = pairs.where((p) => p.label == 'Bug');

      expect(
        chip,
        isNotEmpty,
        reason: 'The selected chip should paint an opaque fill. If this fails '
            'the chip went translucent and its contrast now belongs to the '
            'composited gate, not here.',
      );
      expect(chip.first.ratio, greaterThanOrEqualTo(4.5));
    });

    testWidgets('contrast holds at 200% text scale', (tester) async {
      // Accessibility review §4. Large text does not change colour, but it does
      // change which widgets lay out and paint at all.
      tester.view.physicalSize = const Size(1200, 3200);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.reset);

      await tester.pumpWidget(buildSheet(textScale: 2));
      await tester.pumpAndSettle();

      expect(tester.takeException(), isNull);
      expectOpaqueContrast(tester);
    });
  });
}
