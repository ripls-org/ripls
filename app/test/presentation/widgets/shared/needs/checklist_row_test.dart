import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/shared/needs/checklist_row.dart';

import '../../../../helpers/l10n_helpers.dart';

User _testUser(String name) => User(id: 'u_$name', name: name);

void main() {
  group('SuggestionRow', () {
    testWidgets('renders the label and action label', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: SuggestionRow(
              label: 'Water bottles',
              actionLabel: '+ Add',
              actionSemanticsLabel: 'Add Water bottles',
              onAction: () {},
            ),
          ),
        ),
      );

      expect(find.text('Water bottles'), findsOneWidget);
      expect(find.text('+ Add'), findsOneWidget);
    });

    testWidgets('invokes onAction when the action is tapped', (tester) async {
      var taps = 0;
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: SuggestionRow(
              label: 'Snacks',
              actionLabel: '+ Add',
              actionSemanticsLabel: 'Add Snacks',
              onAction: () => taps++,
            ),
          ),
        ),
      );

      await tester.tap(find.text('+ Add'));
      await tester.pump();
      expect(taps, 1);
    });
  });

  group('NeedRow', () {
    testWidgets('renders the label with no trailing button', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: NeedRow(
              label: 'Trail map',
              rowSemanticsLabel: 'Claim Trail map',
              onTap: () {},
            ),
          ),
        ),
      );

      expect(find.text('Trail map'), findsOneWidget);
      // The two-column plan tab uses tap-to-claim rows — no inline button.
      expect(find.text('Claim'), findsNothing);
    });

    testWidgets('invokes onTap when the row is tapped', (tester) async {
      var taps = 0;
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: NeedRow(
              label: 'First aid kit',
              rowSemanticsLabel: 'Claim First aid kit',
              onTap: () => taps++,
            ),
          ),
        ),
      );

      await tester.tap(find.text('First aid kit'));
      await tester.pump();
      expect(taps, 1);
    });

    testWidgets('renders both styles (need / suggestion)', (tester) async {
      // Just verifies both variants compile and render without error;
      // visual divergence is covered by golden tests if/when added.
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: Column(
              children: [
                NeedRow(
                  label: 'A',
                  rowSemanticsLabel: 'A',
                  onTap: () {},
                ),
                NeedRow(
                  label: 'B',
                  rowSemanticsLabel: 'B',
                  style: NeedRowStyle.suggestion,
                  onTap: () {},
                ),
              ],
            ),
          ),
        ),
      );
      expect(find.text('A'), findsOneWidget);
      expect(find.text('B'), findsOneWidget);
    });
  });

  group('ClaimedRow', () {
    testWidgets('renders label and claimer first name', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: ClaimedRow(
              label: 'Coffee',
              claimer: _testUser('Sarah Chen'),
              showDivider: false,
            ),
          ),
        ),
      );

      expect(find.text('Coffee'), findsOneWidget);
      expect(find.text('Sarah'), findsOneWidget);
    });

    testWidgets('asserts rowSemanticsLabel is required when onTap is set',
        (tester) async {
      expect(
        () => ClaimedRow(
          label: 'X',
          claimer: _testUser('A'),
          onTap: () {},
        ),
        throwsAssertionError,
      );
    });
  });
}
