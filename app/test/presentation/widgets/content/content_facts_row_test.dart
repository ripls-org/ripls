import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/content/content_facts_row.dart';

Widget _host(Widget child) => MaterialApp(home: Scaffold(body: child));

void main() {
  group('ContentFactsRow', () {
    testWidgets('renders uppercased label, value and detail', (tester) async {
      await tester.pumpWidget(
        _host(
          const ContentFactsRow(
            facts: [
              ContentFactData(
                label: 'When',
                value: 'Thu Jun 4',
                detail: '3d 2h from now',
              ),
              ContentFactData(
                label: 'Where',
                value: 'Vail Village',
                detail: '3 mi away',
              ),
            ],
          ),
        ),
      );

      expect(find.text('WHEN'), findsOneWidget);
      expect(find.text('WHERE'), findsOneWidget);
      // The value and its detail render as separate lines.
      expect(find.text('Thu Jun 4'), findsOneWidget);
      expect(find.text('3d 2h from now'), findsOneWidget);
      expect(find.text('Vail Village'), findsOneWidget);
      expect(find.text('3 mi away'), findsOneWidget);
    });

    testWidgets('a tappable fact invokes its handler with the card rect', (
      tester,
    ) async {
      var taps = 0;
      Rect? rect;
      await tester.pumpWidget(
        _host(
          ContentFactsRow(
            facts: [
              ContentFactData(
                label: 'When',
                value: 'Thu Jun 4',
                onTap: (r) {
                  taps++;
                  rect = r;
                },
                semanticsLabel: 'Edit time',
              ),
            ],
          ),
        ),
      );

      await tester.tap(find.bySemanticsLabel('Edit time'));
      expect(taps, 1);
      // The card's own footprint is captured so a caller can morph from it.
      expect(rect, isNotNull);
      expect(rect!.isEmpty, isFalse);
    });

    testWidgets('renders a single fact without a sibling', (tester) async {
      await tester.pumpWidget(
        _host(
          const ContentFactsRow(
            facts: [ContentFactData(label: 'When', value: 'Thu Jun 4')],
          ),
        ),
      );

      expect(find.text('Thu Jun 4', findRichText: true), findsOneWidget);
    });

    testWidgets(
      'with a CTA, the chip fires its handler and the card body fires onTap',
      (tester) async {
        var ctaTaps = 0;
        var cardTaps = 0;
        await tester.pumpWidget(
          _host(
            ContentFactsRow(
              facts: [
                ContentFactData(
                  label: 'When',
                  value: '3 options',
                  onTap: (_) => cardTaps++,
                  semanticsLabel: 'Change time',
                  cta: ContentFactCta(
                    label: 'Vote',
                    semanticsLabel: 'Vote on the time',
                    onTap: (_) => ctaTaps++,
                  ),
                ),
              ],
            ),
          ),
        );

        expect(find.text('Vote'), findsOneWidget);

        // Tapping the chip fires only the CTA — the nested gesture wins.
        await tester.tap(find.bySemanticsLabel('Vote on the time'));
        expect(ctaTaps, 1);
        expect(cardTaps, 0);

        // Tapping the card body (outside the chip) fires the card's onTap, so
        // the whole widget opens the panel — not just the Vote button.
        await tester.tap(find.text('3 options'));
        expect(cardTaps, 1);
        expect(ctaTaps, 1);
      },
    );
  });
}
