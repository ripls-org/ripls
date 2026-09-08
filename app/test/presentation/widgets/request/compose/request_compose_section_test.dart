import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/request_compose_view_model.dart';
import 'package:ripls/presentation/widgets/request/compose/request_compose_section.dart';

import '../../../../helpers/l10n_helpers.dart';

Widget _wrap(Widget child) => ProviderScope(child: localizedApp(child));

void main() {
  testWidgets('renders the suggestion chips passed in', (tester) async {
    await tester.pumpWidget(_wrap(const Scaffold(
      body: RequestComposeSection(
        suggestions: ['Stump grinder', 'Haul debris'],
      ),
    )));
    await tester.pumpAndSettle();

    expect(find.text('Stump grinder'), findsOneWidget);
    expect(find.text('Haul debris'), findsOneWidget);
  });

  testWidgets('tapping a chip adds a piece; tapping it again removes',
      (tester) async {
    await tester.pumpWidget(_wrap(const Scaffold(
      body: RequestComposeSection(suggestions: ['Saw']),
    )));
    final container =
        ProviderScope.containerOf(tester.element(find.byType(RequestComposeSection)));
    await tester.pumpAndSettle();

    // Tap by chip semantic label so we don't ambiguously hit the
    // piece-list label after the piece is added.
    final chipFinder = find.bySemanticsLabel('Suggestion: Saw');
    await tester.tap(chipFinder);
    await tester.pumpAndSettle();
    expect(container.read(requestComposeProvider).pieces.length, 1);

    await tester.tap(chipFinder);
    await tester.pumpAndSettle();
    expect(container.read(requestComposeProvider).pieces, isEmpty);
  });

  testWidgets('typing into inline-add and submitting appends a piece',
      (tester) async {
    await tester.pumpWidget(_wrap(const Scaffold(body: RequestComposeSection())));
    final container =
        ProviderScope.containerOf(tester.element(find.byType(RequestComposeSection)));

    await tester.enterText(find.byType(TextField).first, 'Gloves');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pumpAndSettle();

    expect(container.read(requestComposeProvider).pieces.single.label, 'Gloves');
  });

  testWidgets('AI suggestions eyebrow renders above the chip strip',
      (tester) async {
    await tester.pumpWidget(_wrap(const Scaffold(
      body: RequestComposeSection(suggestions: ['Stump grinder']),
    )));
    await tester.pumpAndSettle();
    // Rendered text is uppercased at paint time; the underlying ARB
    // value is mixed case.
    expect(find.text('FROM YOUR REQUEST · TAP TO ADD'), findsOneWidget);
  });

  testWidgets('AI suggestions eyebrow is omitted when there are no chips',
      (tester) async {
    await tester.pumpWidget(_wrap(const Scaffold(body: RequestComposeSection())));
    await tester.pumpAndSettle();
    expect(find.text('FROM YOUR REQUEST · TAP TO ADD'), findsNothing);
  });

  testWidgets('tapping the × button removes the piece', (tester) async {
    await tester.pumpWidget(_wrap(const Scaffold(body: RequestComposeSection())));
    final container =
        ProviderScope.containerOf(tester.element(find.byType(RequestComposeSection)));
    container.read(requestComposeProvider.notifier).addInlinePiece('Saw');
    await tester.pumpAndSettle();

    expect(container.read(requestComposeProvider).pieces.length, 1);
    await tester.tap(find.bySemanticsLabel('Remove need: Saw'));
    await tester.pumpAndSettle();
    expect(container.read(requestComposeProvider).pieces, isEmpty);
  });

  testWidgets('preclaim toggle flips selected state', (tester) async {
    await tester.pumpWidget(_wrap(const Scaffold(body: RequestComposeSection())));
    final container =
        ProviderScope.containerOf(tester.element(find.byType(RequestComposeSection)));
    container.read(requestComposeProvider.notifier).addInlinePiece('Lunch');
    await tester.pumpAndSettle();

    // The pre-claim toggle shows "I've got this" until tapped, then "You".
    expect(find.text("I've got this"), findsOneWidget);
    await tester.tap(find.text("I've got this"));
    await tester.pumpAndSettle();
    expect(find.text('You'), findsOneWidget);
    expect(container.read(requestComposeProvider).preclaimedCount, 1);
  });
}
