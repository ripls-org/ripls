import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/content/content_need_chip.dart';

import '../../../helpers/l10n_helpers.dart';

Widget _host(Widget child) => MaterialApp(home: Scaffold(body: child));

void main() {
  group('ContentNeedChip', () {
    testWidgets('open need is an empty slot: + and no check', (tester) async {
      await tester.pumpWidget(
        _host(
          ContentNeedChip(
            label: 'Camp stove',
            style: ContentNeedChipStyle.open,
            accentColor: Colors.green,
            semanticsLabel: 'Camp stove',
            onTap: () {},
          ),
        ),
      );
      expect(find.text('Camp stove'), findsOneWidget);
      expect(find.byIcon(Icons.add_rounded), findsOneWidget);
      expect(find.byIcon(Icons.check_rounded), findsNothing);
    });

    testWidgets('claimed need shows a check and optional attribution', (
      tester,
    ) async {
      await tester.pumpWidget(
        _host(
          ContentNeedChip(
            label: 'Sunscreen',
            style: ContentNeedChipStyle.claimed,
            accentColor: Colors.green,
            semanticsLabel: 'Sunscreen',
            attribution: '· you',
            onTap: () {},
          ),
        ),
      );
      expect(find.text('Sunscreen'), findsOneWidget);
      expect(find.byIcon(Icons.check_rounded), findsOneWidget);
      expect(find.text('· you'), findsOneWidget);
    });

    testWidgets('quantity > 1 renders a ×N badge', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: ContentNeedChip(
              label: 'Chairs',
              style: ContentNeedChipStyle.open,
              accentColor: Colors.green,
              semanticsLabel: 'Chairs',
              quantity: 3,
              onTap: () {},
            ),
          ),
        ),
      );
      expect(find.text('Chairs'), findsOneWidget);
      expect(find.text('×3'), findsOneWidget);
    });

    testWidgets('quantity of 1 renders no badge', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: ContentNeedChip(
              label: 'Tongs',
              style: ContentNeedChipStyle.open,
              accentColor: Colors.green,
              semanticsLabel: 'Tongs',
              onTap: () {},
            ),
          ),
        ),
      );
      expect(find.text('Tongs'), findsOneWidget);
      expect(find.textContaining('×'), findsNothing);
    });

    testWidgets('add chip shows a + and fires onTap', (tester) async {
      var taps = 0;
      await tester.pumpWidget(
        _host(
          ContentNeedChip(
            label: 'Add a need',
            style: ContentNeedChipStyle.add,
            accentColor: Colors.green,
            semanticsLabel: 'Add a need',
            onTap: () => taps++,
          ),
        ),
      );
      expect(find.byIcon(Icons.add_rounded), findsOneWidget);
      await tester.tap(find.text('Add a need'));
      expect(taps, 1);
    });
  });
}
