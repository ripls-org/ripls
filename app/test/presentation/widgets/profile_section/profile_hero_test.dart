import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_hero.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../../helpers/l10n_helpers.dart';

Widget _wrap(Widget child) {
  return ProviderScope(child: localizedApp(SizedBox(width: 300, child: child)));
}

Text _sentenceText(WidgetTester tester, String sentence) {
  return tester.widget<Text>(find.text(sentence));
}

void main() {
  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
  });

  group('ProfileHero sentence', () {
    const shortSentence = 'Climbing and cooking.';
    final longSentence =
        'Climbing, cooking, and hiking in common — a season of early '
        'mornings at the crag, gear lent and rides shared, '
        'plus many more words to guarantee this description overflows '
        'three lines at a narrow width for the clamp test.';

    testWidgets('short sentence renders unclamped with no toggle',
        (tester) async {
      await tester.pumpWidget(_wrap(
        const ProfileHero(
          eyebrow: 'YOU & BETTY',
          title: 'Betty',
          sentence: shortSentence,
        ),
      ));
      await tester.pumpAndSettle();

      expect(_sentenceText(tester, shortSentence).maxLines, isNull);
      expect(find.bySemanticsLabel('Expand description'), findsNothing);
    });

    testWidgets('long sentence clamps to three lines and expands on tap',
        (tester) async {
      await tester.pumpWidget(_wrap(
        ProfileHero(
          eyebrow: 'YOU & BETTY',
          title: 'Betty',
          sentence: longSentence,
        ),
      ));
      await tester.pumpAndSettle();

      expect(_sentenceText(tester, longSentence).maxLines, 3);
      expect(
        _sentenceText(tester, longSentence).overflow,
        TextOverflow.ellipsis,
      );

      await tester.tap(find.text(longSentence));
      await tester.pumpAndSettle();
      expect(_sentenceText(tester, longSentence).maxLines, isNull);

      // Tapping again collapses back to the clamp.
      await tester.tap(find.text(longSentence));
      await tester.pumpAndSettle();
      expect(_sentenceText(tester, longSentence).maxLines, 3);
    });
  });
}
