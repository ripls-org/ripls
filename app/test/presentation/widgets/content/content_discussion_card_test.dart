import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/content/content_discussion_card.dart';

Widget _host(Widget child, {double width = 800}) => MaterialApp(
  home: Scaffold(
    body: Center(child: SizedBox(width: width, child: child)),
  ),
);

void main() {
  group('ContentDiscussionCard', () {
    testWidgets('shows the description quote attributed to the owner', (
      tester,
    ) async {
      await tester.pumpWidget(
        _host(
          const ContentDiscussionCard(
            authorName: 'Alfred',
            firstComment: 'Going for hike on the flatirons this afternoon.',
            replyCount: 3,
            replyCountLabel: '3 replies',
            startLabel: 'Start the conversation',
            lastActivityLabel: 'last 2h',
            accentColor: Colors.green,
            semanticsLabel: 'Open conversation',
          ),
        ),
      );

      expect(
        find.text('"Going for hike on the flatirons this afternoon."'),
        findsOneWidget,
      );
      expect(find.textContaining('Alfred'), findsOneWidget);
      expect(find.textContaining('3 replies'), findsOneWidget);
      expect(find.textContaining('last 2h'), findsOneWidget);
    });

    testWidgets(
      'pins the description — the latest reply never replaces the quote '
      '(#2724)',
      (tester) async {
        await tester.pumpWidget(
          _host(
            const ContentDiscussionCard(
              authorName: 'Alfred',
              firstComment: 'Going for a hike.',
              replyCount: 3,
              replyCountLabel: '3 replies',
              startLabel: 'Start the conversation',
              lastActivityLabel: 'last 2h',
              accentColor: Colors.green,
              semanticsLabel: 'Open conversation',
              latestLine: "💬 Maya: Can't wait!",
            ),
          ),
        );

        expect(find.text('"Going for a hike."'), findsOneWidget);
        // Long after the old carousel's cycle interval, the quote is still
        // the description and the reply is still its own labeled line.
        await tester.pump(const Duration(seconds: 10));
        expect(find.text('"Going for a hike."'), findsOneWidget);
        expect(find.text("💬 Maya: Can't wait!"), findsOneWidget);
        expect(find.text('"Can\'t wait!"'), findsNothing);
      },
    );

    testWidgets('shows the start prompt when there are no replies', (
      tester,
    ) async {
      await tester.pumpWidget(
        _host(
          const ContentDiscussionCard(
            authorName: 'Alfred',
            firstComment: 'Going for a hike.',
            replyCount: 0,
            replyCountLabel: '0 replies',
            startLabel: 'Start the conversation',
            accentColor: Colors.green,
            semanticsLabel: 'Open conversation',
          ),
        ),
      );

      expect(find.text('"Going for a hike."'), findsOneWidget);
      expect(find.textContaining('Start the conversation'), findsOneWidget);
      expect(find.textContaining('0 replies'), findsNothing);
    });

    testWidgets('shows only the start prompt when there is no description', (
      tester,
    ) async {
      await tester.pumpWidget(
        _host(
          const ContentDiscussionCard(
            authorName: 'Alfred',
            firstComment: '',
            replyCount: 0,
            replyCountLabel: '0 replies',
            startLabel: 'Start the conversation',
            accentColor: Colors.green,
            semanticsLabel: 'Open conversation',
          ),
        ),
      );

      expect(find.textContaining('Alfred'), findsNothing);
      expect(find.textContaining('Start the conversation'), findsOneWidget);
    });

    testWidgets(
      'drops the decorative quotes when the quote is truncated (#2724)',
      (tester) async {
        const long =
            'Going for a very long hike on the flatirons this afternoon, '
            'with an extended picnic afterwards, a stop at the creek, and a '
            'stargazing session that runs well past everyone\'s bedtime.';
        await tester.pumpWidget(
          _host(
            width: 220,
            const ContentDiscussionCard(
              authorName: 'Alfred',
              firstComment: long,
              replyCount: 0,
              replyCountLabel: '0 replies',
              startLabel: 'Start the conversation',
              accentColor: Colors.green,
              semanticsLabel: 'Open conversation',
            ),
          ),
        );

        // Overflowing text renders without the wrapping quote marks — an
        // opening quote whose closing mate was ellipsized reads as a typo.
        expect(find.text(long), findsOneWidget);
        expect(find.text('"$long"'), findsNothing);
      },
    );

    testWidgets('keeps the decorative quotes when the quote fits', (
      tester,
    ) async {
      await tester.pumpWidget(
        _host(
          const ContentDiscussionCard(
            authorName: 'Alfred',
            firstComment: 'Short and sweet.',
            replyCount: 0,
            replyCountLabel: '0 replies',
            startLabel: 'Start the conversation',
            accentColor: Colors.green,
            semanticsLabel: 'Open conversation',
          ),
        ),
      );

      expect(find.text('"Short and sweet."'), findsOneWidget);
    });

    testWidgets('invokes onTap with the card rect when tapped', (tester) async {
      Rect? tappedRect;
      await tester.pumpWidget(
        _host(
          ContentDiscussionCard(
            authorName: 'Alfred',
            firstComment: 'Going for a hike.',
            replyCount: 0,
            replyCountLabel: '0 replies',
            startLabel: 'Start the conversation',
            accentColor: Colors.green,
            semanticsLabel: 'Open conversation',
            onTap: (rect) => tappedRect = rect,
          ),
        ),
      );

      await tester.tap(find.byType(ContentDiscussionCard));
      expect(tappedRect, isNotNull);
      expect(tappedRect!.isEmpty, isFalse);
    });
  });
}
