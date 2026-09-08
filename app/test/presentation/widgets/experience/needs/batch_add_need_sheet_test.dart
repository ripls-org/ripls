import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/experience/needs/batch_add_need_sheet.dart';

/// Mounts [BatchAddNeedSheet] directly (without the bottom-sheet route)
/// so the test framework doesn't need to drive modal-entrance animations.
Future<void> _mount(
  WidgetTester tester, {
  List<String> suggestions = const [],
  List<String> existingLabels = const [],
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
        body: BatchAddNeedSheet(
          suggestions: suggestions,
          existingLabels: existingLabels,
          onChooseTimeTap: onChooseTimeTap,
          initialText: initialText,
        ),
      ),
    ),
  ));
  // Pump once to render. Avoid pumpAndSettle — the autofocused TextField
  // can keep a focus animation alive indefinitely under test.
  await tester.pump();
}

void main() {
  group('BatchAddNeedSheet — suggestion rendering', () {
    testWidgets('renders LLM thing-suggestions as a list', (tester) async {
      await _mount(
        tester,
        suggestions: ['Plates', 'Forks', 'Cups'],
      );

      expect(find.text('Plates'), findsOneWidget);
      expect(find.text('Forks'), findsOneWidget);
      expect(find.text('Cups'), findsOneWidget);
    });

    testWidgets('hides suggestions that already exist as labels',
        (tester) async {
      await _mount(
        tester,
        suggestions: ['Plates', 'Forks', 'Cups'],
        existingLabels: ['Plates'],
      );

      expect(find.text('Plates'), findsNothing);
      expect(find.text('Forks'), findsOneWidget);
      expect(find.text('Cups'), findsOneWidget);
    });

    testWidgets('shows "Agree Upon Time" row when onChooseTimeTap is set',
        (tester) async {
      await _mount(
        tester,
        suggestions: const ['Plates'],
        onChooseTimeTap: () {},
      );

      expect(find.text('Agree Upon Time'), findsOneWidget);
      expect(find.text('Poll'), findsOneWidget);
    });

    testWidgets('hides "Agree Upon Time" row when onChooseTimeTap is null',
        (tester) async {
      await _mount(
        tester,
        suggestions: const ['Plates'],
      );

      expect(find.text('Agree Upon Time'), findsNothing);
    });

    testWidgets('shows no suggestions header when nothing to suggest',
        (tester) async {
      await _mount(tester);
      expect(find.text('Suggestions for things needed'), findsNothing);
    });

    testWidgets('shows suggestions header when at least one suggestion exists',
        (tester) async {
      await _mount(tester, suggestions: const ['Plates']);
      expect(find.text('Suggestions for things needed'), findsOneWidget);
    });

    testWidgets('shows suggestions header for "Agree Upon Time" alone',
        (tester) async {
      await _mount(tester, onChooseTimeTap: () {});
      expect(find.text('Suggestions for things needed'), findsOneWidget);
    });
  });

  group('BatchAddNeedSheet — tap a suggestion adds it directly', () {
    testWidgets('tapping a suggestion moves it into the pending list',
        (tester) async {
      await _mount(tester, suggestions: const ['Plates', 'Forks']);

      await tester.tap(find.text('Plates'));
      await tester.pump();

      // "YOUR LIST" header appears once the pending list is non-empty.
      expect(find.text('YOUR LIST'), findsOneWidget);
      // Plates now lives in the pending list; the suggestion is filtered
      // out, so only one Plates remains in the tree.
      expect(find.text('Plates'), findsOneWidget);
      // Forks remains as a suggestion.
      expect(find.text('Forks'), findsOneWidget);
    });

    testWidgets('a tapped suggestion is not duplicated in suggestions',
        (tester) async {
      await _mount(tester, suggestions: const ['Plates']);
      await tester.tap(find.text('Plates'));
      await tester.pump();
      expect(find.text('Plates'), findsOneWidget);
    });
  });

  group('BatchAddNeedSheet — duplicate prevention', () {
    testWidgets('typing an existing label keeps the "Add to list" disabled',
        (tester) async {
      await _mount(
        tester,
        existingLabels: const ['Plates'],
      );

      await tester.enterText(find.byType(TextField), 'plates');
      await tester.pump();

      await tester.tap(find.text('Add to list'), warnIfMissed: false);
      await tester.pump();

      // No pending list appears — the duplicate input was blocked.
      expect(find.text('YOUR LIST'), findsNothing);
    });

    testWidgets('typing a non-duplicate adds to the pending list',
        (tester) async {
      await _mount(tester);

      await tester.enterText(find.byType(TextField), 'Napkins');
      await tester.pump();

      await tester.tap(find.text('Add to list'));
      await tester.pump();

      expect(find.text('YOUR LIST'), findsOneWidget);
      expect(find.text('Napkins'), findsOneWidget);
    });
  });
}
