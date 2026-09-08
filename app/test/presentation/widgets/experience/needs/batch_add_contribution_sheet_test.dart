import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/experience/needs/batch_add_contribution_sheet.dart';

Future<void> _mount(
  WidgetTester tester, {
  List<String> suggestions = const [],
  List<String> existingLabels = const [],
  bool isRsvped = true,
  bool isRsvpedMaybe = false,
  String experienceName = 'Picnic',
  VoidCallback? onChooseTimeTap,
  String? initialText,
}) async {
  await tester.pumpWidget(ProviderScope(
    child: MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(
        body: BatchAddContributionSheet(
          suggestions: suggestions,
          existingLabels: existingLabels,
          isRsvped: isRsvped,
          isRsvpedMaybe: isRsvpedMaybe,
          experienceName: experienceName,
          onChooseTimeTap: onChooseTimeTap,
          initialText: initialText,
        ),
      ),
    ),
  ));
  await tester.pump();
}

void main() {
  group('BatchAddContributionSheet — suggestion rendering', () {
    testWidgets('renders LLM thing-suggestions', (tester) async {
      await _mount(tester, suggestions: const ['Chips', 'Salsa']);
      expect(find.text('Chips'), findsOneWidget);
      expect(find.text('Salsa'), findsOneWidget);
    });

    testWidgets('hides suggestions that already exist as labels',
        (tester) async {
      await _mount(
        tester,
        suggestions: const ['Chips', 'Salsa'],
        existingLabels: const ['chips'],
      );
      expect(find.text('Chips'), findsNothing);
      expect(find.text('Salsa'), findsOneWidget);
    });

    testWidgets('shows header when at least one suggestion exists',
        (tester) async {
      await _mount(tester, suggestions: const ['Chips']);
      expect(find.text('Suggestions for things needed'), findsOneWidget);
    });

    testWidgets('shows no header when nothing to suggest', (tester) async {
      await _mount(tester);
      expect(find.text('Suggestions for things needed'), findsNothing);
    });
  });

  group('BatchAddContributionSheet — tap suggestion adds it directly', () {
    testWidgets('tapping a suggestion moves it into the pending list',
        (tester) async {
      await _mount(tester, suggestions: const ['Chips', 'Salsa']);

      await tester.tap(find.text('Chips'));
      await tester.pump();

      expect(find.text('YOUR LIST'), findsOneWidget);
      // Chips lives in the pending list; suggestion filtered out.
      expect(find.text('Chips'), findsOneWidget);
      expect(find.text('Salsa'), findsOneWidget);
    });
  });

  group('BatchAddContributionSheet — duplicate prevention', () {
    testWidgets('typing an existing label keeps "Add to list" disabled',
        (tester) async {
      await _mount(tester, existingLabels: const ['Chips']);

      await tester.enterText(find.byType(TextField), 'chips');
      await tester.pump();

      await tester.tap(find.text('Add to list'), warnIfMissed: false);
      await tester.pump();

      expect(find.text('YOUR LIST'), findsNothing);
    });

    testWidgets('typing a non-duplicate adds to the pending list',
        (tester) async {
      await _mount(tester);
      await tester.enterText(find.byType(TextField), 'Plates');
      await tester.pump();
      await tester.tap(find.text('Add to list'));
      await tester.pump();
      expect(find.text('Plates'), findsOneWidget);
      expect(find.text('YOUR LIST'), findsOneWidget);
    });
  });

  group('BatchAddContributionSheet — RSVP disclosure', () {
    // The disclosure's "Make it a maybe instead" toggle label is the
    // most stable probe for its presence — the body text is
    // case-lowered, which makes textContaining brittle.
    testWidgets('shows the RSVP disclosure when user is not RSVPed',
        (tester) async {
      await _mount(tester, isRsvped: false);
      expect(find.text('Make it a maybe instead'), findsOneWidget);
    });

    testWidgets('hides the RSVP disclosure when user is already RSVPed',
        (tester) async {
      await _mount(tester, isRsvped: true);
      expect(find.text('Make it a maybe instead'), findsNothing);
    });
  });
}
