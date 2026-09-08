import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:ripls/data/gen/ripls/api/feedback_service.pb.dart';
import 'package:ripls/data/repositories/feedback_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/feedback_sheet_view_model.dart';
import 'package:ripls/presentation/widgets/feedback/feedback_sheet.dart';
import 'package:ripls/services/providers.dart';

import 'feedback_sheet_test.mocks.dart';

/// _SubmittedNotifier starts with isSubmitted=true for success-state tests.
class _SubmittedNotifier extends FeedbackSheetNotifier {
  @override
  FeedbackSheetState build() => const FeedbackSheetState(isSubmitted: true);
}

@GenerateMocks([FeedbackRepository])
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

  Widget buildSheet({bool successState = false}) {
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
        home: Scaffold(body: FeedbackSheet()),
      ),
    );
  }

  group('FeedbackSheet - form state', () {
    testWidgets('renders header title and subtitle', (tester) async {
      await tester.pumpWidget(buildSheet());

      expect(find.text("We'd love to hear from you!"), findsOneWidget);
      expect(
        find.text(
          'Share your ideas, report issues, or let us know how we can improve.',
        ),
        findsOneWidget,
      );
    });

    testWidgets('renders all three type selector pills', (tester) async {
      await tester.pumpWidget(buildSheet());

      expect(find.text('Bug'), findsOneWidget);
      expect(find.text('Feature'), findsOneWidget);
      expect(find.text('Other'), findsOneWidget);
    });

    testWidgets('renders title and description fields', (tester) async {
      await tester.pumpWidget(buildSheet());

      // Two TextFields: title and description
      expect(find.byType(TextField), findsNWidgets(2));
    });

    testWidgets('renders Submit Feedback button', (tester) async {
      await tester.pumpWidget(buildSheet());

      expect(find.text('Submit Feedback'), findsOneWidget);
    });

    testWidgets('renders privacy footer text', (tester) async {
      await tester.pumpWidget(buildSheet());

      expect(find.textContaining('GitHub issue'), findsOneWidget);
    });

    testWidgets('submit button is disabled when title and description are empty',
        (tester) async {
      await tester.pumpWidget(buildSheet());

      // The canonical GlassFooterButtons primitive presents the action via a
      // Semantics-labelled Tappable. With the form empty, onTap is null.
      final tappable = tester.widget<Semantics>(
        find.ancestor(
          of: find.text('Submit Feedback'),
          matching: find.bySemanticsLabel('Submit Feedback'),
        ),
      );
      expect(tappable.properties.enabled, isNot(true));
    });

    testWidgets('submit button is enabled after entering title and description',
        (tester) async {
      await tester.pumpWidget(buildSheet());

      await tester.enterText(find.byType(TextField).at(0), 'Bug title');
      await tester.pump();
      await tester.enterText(find.byType(TextField).at(1), 'Bug description');
      await tester.pump();

      final tappable = tester.widget<Semantics>(
        find.ancestor(
          of: find.text('Submit Feedback'),
          matching: find.bySemanticsLabel('Submit Feedback'),
        ),
      );
      expect(tappable.properties.enabled, true);
    });

    testWidgets('title character counter updates when typing', (tester) async {
      await tester.pumpWidget(buildSheet());

      await tester.enterText(find.byType(TextField).at(0), 'Hello');
      await tester.pump();

      expect(find.text('5/100'), findsOneWidget);
    });

    testWidgets('description character counter updates when typing',
        (tester) async {
      await tester.pumpWidget(buildSheet());

      await tester.enterText(find.byType(TextField).at(1), 'Some description');
      await tester.pump();

      expect(find.text('16/5000'), findsOneWidget);
    });

    testWidgets('type selector pill updates when tapped', (tester) async {
      await tester.pumpWidget(buildSheet());

      // Default is Bug; tap Feature
      await tester.tap(find.text('Feature'));
      await tester.pump();

      // Verify the form still renders (state updated without error)
      expect(find.text('Bug'), findsOneWidget);
      expect(find.text('Feature'), findsOneWidget);
      expect(find.text('Other'), findsOneWidget);
    });
  });

  group('FeedbackSheet - success state', () {
    testWidgets('shows thank-you message when isSubmitted is true',
        (tester) async {
      await tester.pumpWidget(buildSheet(successState: true));

      expect(find.text('Thanks for your feedback!'), findsOneWidget);
      expect(
        find.textContaining("We'll take a look"),
        findsOneWidget,
      );
    });

    testWidgets('shows Done button in success state', (tester) async {
      await tester.pumpWidget(buildSheet(successState: true));

      expect(find.text('Done'), findsOneWidget);
    });

    testWidgets('hides form fields in success state', (tester) async {
      await tester.pumpWidget(buildSheet(successState: true));

      expect(find.text('Submit Feedback'), findsNothing);
      expect(find.byType(TextField), findsNothing);
    });
  });

  group('FeedbackSheet - screenshot button', () {
    testWidgets('renders add screenshots button when no screenshots added',
        (tester) async {
      await tester.pumpWidget(buildSheet());

      expect(find.text('Add Screenshots (optional)'), findsOneWidget);
    });
  });

  group('FeedbackSheet - submission flow', () {
    testWidgets('submit button is disabled while submitting', (tester) async {
      // Arrange: repository never completes during the test (Completer avoids pending timers)
      final completer = Completer<SubmitFeedbackResponse>();
      when(mockRepository.submitFeedback(any))
          .thenAnswer((_) => completer.future);

      await tester.pumpWidget(buildSheet());

      await tester.enterText(find.byType(TextField).at(0), 'Bug title');
      await tester.pump();
      await tester.enterText(find.byType(TextField).at(1), 'Bug description text');
      await tester.pump();

      await tester.ensureVisible(find.text('Submit Feedback'));
      await tester.pump();
      await tester.tap(find.text('Submit Feedback'));
      await tester.pump();

      final tappable = tester.widget<Semantics>(
        find.ancestor(
          of: find.text('Submit Feedback'),
          matching: find.bySemanticsLabel('Submit Feedback'),
        ),
      );
      expect(tappable.properties.enabled, isNot(true));
    });
  });
}
