import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/content/content_action_chip.dart';

Widget _host(Widget child) => MaterialApp(home: Scaffold(body: child));

void main() {
  group('ContentActionChip', () {
    testWidgets('renders the label and a trailing chevron', (tester) async {
      await tester.pumpWidget(
        _host(
          ContentActionChip(
            label: 'Vote',
            semanticsLabel: 'Vote on the time',
            accentColor: const Color(0xFF7A9B8C),
            onTap: () {},
          ),
        ),
      );

      expect(find.text('Vote'), findsOneWidget);
      expect(find.text('›'), findsOneWidget);
    });

    testWidgets('invokes onTap via its semantics label', (tester) async {
      var taps = 0;
      await tester.pumpWidget(
        _host(
          ContentActionChip(
            label: 'RSVP — are you in?',
            semanticsLabel: 'RSVP to this event',
            accentColor: const Color(0xFF7A9B8C),
            onTap: () => taps++,
          ),
        ),
      );

      await tester.tap(find.bySemanticsLabel('RSVP to this event'));
      expect(taps, 1);
    });

    testWidgets('full-width chip stretches to its parent', (tester) async {
      await tester.pumpWidget(
        _host(
          SizedBox(
            width: 300,
            child: ContentActionChip(
              label: 'RSVP — are you in?',
              semanticsLabel: 'RSVP to this event',
              accentColor: const Color(0xFF7A9B8C),
              fullWidth: true,
              onTap: () {},
            ),
          ),
        ),
      );

      // The filled pill (the Container with a BoxDecoration) spans the width.
      final container = tester.widget<Container>(
        find
            .descendant(
              of: find.byType(ContentActionChip),
              matching: find.byType(Container),
            )
            .first,
      );
      expect(tester.getSize(find.byWidget(container)).width, 300);
    });
  });
}
