import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/creation/seed_needs_field.dart';

Widget _host({
  required List<String> needs,
  required ValueChanged<List<String>> onChanged,
}) {
  return MaterialApp(
    localizationsDelegates: const [
      AppLocalizations.delegate,
      GlobalMaterialLocalizations.delegate,
      GlobalWidgetsLocalizations.delegate,
      GlobalCupertinoLocalizations.delegate,
    ],
    supportedLocales: AppLocalizations.supportedLocales,
    home: Scaffold(
      body: SeedNeedsField(needs: needs, onChanged: onChanged),
    ),
  );
}

void main() {
  group('SeedNeedsField (#2731)', () {
    testWidgets('renders a chip per seeded need', (tester) async {
      await tester.pumpWidget(_host(
        needs: const ['Picture books', 'Whiteboard', 'Storage bins'],
        onChanged: (_) {},
      ));
      expect(find.text('Picture books'), findsOneWidget);
      expect(find.text('Whiteboard'), findsOneWidget);
      expect(find.text('Storage bins'), findsOneWidget);
    });

    testWidgets('typing a need and submitting appends it', (tester) async {
      List<String>? next;
      await tester.pumpWidget(_host(
        needs: const ['Picture books'],
        onChanged: (v) => next = v,
      ));
      await tester.enterText(find.byType(TextField), 'Whiteboard');
      await tester.testTextInput.receiveAction(TextInputAction.done);
      await tester.pump();
      expect(next, ['Picture books', 'Whiteboard']);
    });

    testWidgets('pasting a comma list adds each item', (tester) async {
      List<String>? next;
      await tester.pumpWidget(_host(
        needs: const [],
        onChanged: (v) => next = v,
      ));
      await tester.enterText(
        find.byType(TextField),
        'Art supplies, construction paper; glue',
      );
      await tester.testTextInput.receiveAction(TextInputAction.done);
      await tester.pump();
      expect(next, ['Art supplies', 'construction paper', 'glue']);
    });

    testWidgets('removing a chip drops that need', (tester) async {
      List<String>? next;
      await tester.pumpWidget(_host(
        needs: const ['Picture books', 'Whiteboard'],
        onChanged: (v) => next = v,
      ));
      // Remove buttons carry the localized "Remove need: {label}" semantics.
      await tester.tap(find.bySemanticsLabel('Remove need: Whiteboard'));
      await tester.pump();
      expect(next, ['Picture books']);
    });

    testWidgets('blank input is a no-op', (tester) async {
      var called = false;
      await tester.pumpWidget(_host(
        needs: const ['Picture books'],
        onChanged: (_) => called = true,
      ));
      await tester.enterText(find.byType(TextField), '   ');
      await tester.testTextInput.receiveAction(TextInputAction.done);
      await tester.pump();
      expect(called, isFalse);
    });
  });
}
