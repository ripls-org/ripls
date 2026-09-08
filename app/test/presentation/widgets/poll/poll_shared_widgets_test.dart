import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/poll/poll_flexible_vote_row.dart';
import 'package:ripls/presentation/widgets/poll/poll_tbd_card.dart';

Widget _host(Widget child) => MaterialApp(
      home: Scaffold(body: Center(child: child)),
    );

void main() {
  group('PollTbdCard', () {
    testWidgets('renders title and subtitle', (tester) async {
      await tester.pumpWidget(_host(
        const PollTbdCard(title: 'Where: TBD', subtitle: 'Friends can RSVP.'),
      ));

      expect(find.text('Where: TBD'), findsOneWidget);
      expect(find.text('Friends can RSVP.'), findsOneWidget);
    });
  });

  group('PollFlexibleVoteRow', () {
    testWidgets('reports selected state and fires onTap', (tester) async {
      var taps = 0;
      await tester.pumpWidget(_host(
        PollFlexibleVoteRow(
          title: 'Honestly, anywhere works',
          subtitle: "I'm flexible",
          semanticsLabel: 'Mark anywhere works',
          selected: true,
          onTap: () => taps++,
        ),
      ));

      expect(find.text('Honestly, anywhere works'), findsOneWidget);
      // Selected → check icon is shown.
      expect(find.byIcon(Icons.check), findsOneWidget);

      await tester.tap(find.text('Honestly, anywhere works'));
      expect(taps, 1);
    });

    testWidgets('hides the check when not selected', (tester) async {
      await tester.pumpWidget(_host(
        PollFlexibleVoteRow(
          title: 'Honestly, anywhere works',
          subtitle: "I'm flexible",
          semanticsLabel: 'Mark anywhere works',
          selected: false,
          onTap: () {},
        ),
      ));

      expect(find.byIcon(Icons.check), findsNothing);
    });
  });
}
