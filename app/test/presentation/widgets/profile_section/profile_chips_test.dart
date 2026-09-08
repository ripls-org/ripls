import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_chips.dart';

import '../../../helpers/l10n_helpers.dart';

void main() {
  const tags = [
    'Outdoor',
    'Cooking',
    'Climbing',
    'Hiking',
    'Board games',
    'Potlucks',
    'Live music',
    'Volunteering',
  ];

  Widget wrap(Widget child) => localizedApp(
        Scaffold(backgroundColor: Colors.black, body: child),
      );

  group('ProfileChips', () {
    testWidgets('clamps to five chips with a "+N more" expander',
        (tester) async {
      await tester.pumpWidget(wrap(const ProfileChips(tags: tags)));
      await tester.pumpAndSettle();

      expect(find.text('Board games'), findsOneWidget);
      expect(find.text('Potlucks'), findsNothing);
      expect(find.text('+3 more'), findsOneWidget);
    });

    testWidgets('expands in place and collapses back with "show less"',
        (tester) async {
      await tester.pumpWidget(wrap(const ProfileChips(tags: tags)));
      await tester.pumpAndSettle();

      await tester.tap(find.text('+3 more'));
      await tester.pumpAndSettle();
      expect(find.text('Potlucks'), findsOneWidget);
      expect(find.text('Volunteering'), findsOneWidget);
      expect(find.text('show less'), findsOneWidget);

      await tester.tap(find.text('show less'));
      await tester.pumpAndSettle();
      expect(find.text('Potlucks'), findsNothing);
      expect(find.text('+3 more'), findsOneWidget);
    });

    testWidgets('renders all chips with no expander at or under the clamp',
        (tester) async {
      await tester.pumpWidget(wrap(
        ProfileChips(tags: tags.take(5).toList()),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Board games'), findsOneWidget);
      expect(find.textContaining('more'), findsNothing);
    });

    testWidgets('collapses to nothing when there are no tags',
        (tester) async {
      await tester.pumpWidget(wrap(const ProfileChips(tags: [])));
      await tester.pumpAndSettle();
      expect(find.byType(ProfileChips), findsOneWidget);
      expect(tester.getSize(find.byType(ProfileChips)).height, 0);
    });
  });
}
