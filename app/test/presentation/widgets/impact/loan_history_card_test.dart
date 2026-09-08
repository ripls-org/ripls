import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/gear_metric_view_model.dart'
    show LoanSocialAttributeDisplay;
import 'package:ripls/presentation/widgets/impact/loan_history_card.dart';
import 'package:ripls/services/providers.dart' show mediaUrlProvider;

// Override mediaUrlProvider to return an empty URL (triggers initials fallback).
final _mediaUrlOverride = mediaUrlProvider.overrideWith(
  (ref, mediaId) async => '',
);

Widget _wrapInApp(Widget child) {
  return ProviderScope(
    overrides: [_mediaUrlOverride],
    child: MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(body: SingleChildScrollView(child: child)),
    ),
  );
}

User _makeUser(String name, {String id = 'u1'}) {
  return User(id: id, name: name);
}

const _testAttributes = LoanSocialAttributeDisplay(
  duration: '15 min',
  modality: 'In-person (brief)',
  groupSize: '2 people',
  connection: 'Acquaintance',
  reciprocity: 'Giving',
  novelty: 'Novel',
  vulnerability: 'Medium',
);

void main() {
  group('LoanHistoryCard', () {
    group('collapsed state', () {
      testWidgets('renders borrower name', (tester) async {
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Betty Ripley'),
          startDate: DateTime(2025, 6, 12),
          endDate: DateTime(2025, 6, 19),
          isActive: false,
          rmValue: '9 RM',
          attributes: _testAttributes,
        )));
        await tester.pumpAndSettle();

        expect(find.text('Betty Ripley'), findsOneWidget);
      });

      testWidgets('renders RM value', (tester) async {
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Betty Ripley'),
          startDate: DateTime(2025, 6, 12),
          endDate: DateTime(2025, 6, 19),
          isActive: false,
          rmValue: '9 RM',
          attributes: _testAttributes,
        )));
        await tester.pumpAndSettle();

        expect(find.text('9 RM'), findsOneWidget);
      });

      testWidgets('does not show attribute rows when collapsed', (tester) async {
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Betty Ripley'),
          startDate: DateTime(2025, 6, 12),
          endDate: DateTime(2025, 6, 19),
          isActive: false,
          rmValue: '9 RM',
          attributes: _testAttributes,
        )));
        await tester.pumpAndSettle();

        expect(find.text('Duration'), findsNothing);
        expect(find.text('Modality'), findsNothing);
        expect(find.text('Vulnerability'), findsNothing);
      });

      testWidgets('renders date range for completed loan', (tester) async {
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Betty Ripley'),
          startDate: DateTime(2025, 6, 12),
          endDate: DateTime(2025, 6, 19),
          isActive: false,
          rmValue: '9 RM',
          attributes: _testAttributes,
        )));
        await tester.pumpAndSettle();

        expect(find.textContaining('Jun 12'), findsOneWidget);
        expect(find.textContaining('Jun 19'), findsOneWidget);
      });

      testWidgets('renders "Since" prefix for active loan', (tester) async {
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Active Borrower'),
          startDate: DateTime(2025, 6, 12),
          isActive: true,
          rmValue: '9 RM',
          attributes: _testAttributes,
        )));
        await tester.pumpAndSettle();

        expect(find.textContaining('Since'), findsOneWidget);
      });

      testWidgets('renders without RM when rmValue is null', (tester) async {
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Betty Ripley'),
          startDate: DateTime(2025, 6, 12),
          isActive: false,
        )));
        await tester.pumpAndSettle();

        expect(find.text('Betty Ripley'), findsOneWidget);
        // No bullet, no RM value, no chevron
        expect(find.text('•'), findsNothing);
        expect(find.byIcon(Icons.keyboard_arrow_down), findsNothing);
      });
    });

    group('expanded state', () {
      testWidgets('tapping RM value expands all 7 attribute rows',
          (tester) async {
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Betty Ripley'),
          startDate: DateTime(2025, 6, 12),
          endDate: DateTime(2025, 6, 19),
          isActive: false,
          rmValue: '9 RM',
          attributes: _testAttributes,
        )));
        await tester.pumpAndSettle();

        // Initially collapsed
        expect(find.text('Duration'), findsNothing);

        // Tap to expand
        await tester.tap(find.text('9 RM'));
        await tester.pumpAndSettle();

        expect(find.text('Duration'), findsOneWidget);
        expect(find.text('Modality'), findsOneWidget);
        expect(find.text('Group size'), findsOneWidget);
        expect(find.text('Connection'), findsOneWidget);
        expect(find.text('Reciprocity'), findsOneWidget);
        expect(find.text('Novelty'), findsOneWidget);
        expect(find.text('Vulnerability'), findsOneWidget);
      });

      testWidgets('expanded state shows attribute values', (tester) async {
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Betty Ripley'),
          startDate: DateTime(2025, 6, 12),
          endDate: DateTime(2025, 6, 19),
          isActive: false,
          rmValue: '9 RM',
          attributes: _testAttributes,
        )));
        await tester.pumpAndSettle();

        await tester.tap(find.text('9 RM'));
        await tester.pumpAndSettle();

        expect(find.text('15 min'), findsOneWidget);
        expect(find.text('In-person (brief)'), findsOneWidget);
        expect(find.text('2 people'), findsOneWidget);
        expect(find.text('Acquaintance'), findsOneWidget);
        expect(find.text('Giving'), findsOneWidget);
        expect(find.text('Novel'), findsOneWidget);
        expect(find.text('Medium'), findsOneWidget);
      });

      testWidgets('tapping again collapses the section', (tester) async {
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Betty Ripley'),
          startDate: DateTime(2025, 6, 12),
          endDate: DateTime(2025, 6, 19),
          isActive: false,
          rmValue: '9 RM',
          attributes: _testAttributes,
        )));
        await tester.pumpAndSettle();

        // Expand
        await tester.tap(find.text('9 RM'));
        await tester.pumpAndSettle();
        expect(find.text('Duration'), findsOneWidget);

        // Collapse
        await tester.tap(find.text('9 RM'));
        await tester.pumpAndSettle();
        expect(find.text('Duration'), findsNothing);
      });

      testWidgets('shows drill-down link when onDrillDownTap is provided',
          (tester) async {
        var tapped = false;
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Betty Ripley'),
          startDate: DateTime(2025, 6, 12),
          endDate: DateTime(2025, 6, 19),
          isActive: false,
          rmValue: '9 RM',
          attributes: _testAttributes,
          onDrillDownTap: () => tapped = true,
        )));
        await tester.pumpAndSettle();

        // Expand
        await tester.tap(find.text('9 RM'));
        await tester.pumpAndSettle();

        expect(find.text('How we calculated this →'), findsOneWidget);

        await tester.tap(find.text('How we calculated this →'));
        expect(tapped, isTrue);
      });

      testWidgets('does not show drill-down link when callback is null',
          (tester) async {
        await tester.pumpWidget(_wrapInApp(LoanHistoryCard(
          borrower: _makeUser('Betty Ripley'),
          startDate: DateTime(2025, 6, 12),
          endDate: DateTime(2025, 6, 19),
          isActive: false,
          rmValue: '9 RM',
          attributes: _testAttributes,
        )));
        await tester.pumpAndSettle();

        await tester.tap(find.text('9 RM'));
        await tester.pumpAndSettle();

        expect(find.text('How we calculated this →'), findsNothing);
      });
    });
  });
}
