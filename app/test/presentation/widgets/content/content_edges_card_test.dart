import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/content/content_edge_data.dart';
import 'package:ripls/presentation/widgets/content/content_edges_card.dart';

Widget _host(Widget child) => MaterialApp(home: Scaffold(body: child));

void main() {
  group('ContentEdgeRow', () {
    testWidgets('renders name, contribution, status and initials avatar', (
      tester,
    ) async {
      await tester.pumpWidget(
        _host(
          const ContentEdgeRow(
            accentColor: Colors.green,
            statusLabel: 'In',
            edge: EdgeViewData(
              userId: 'maya',
              name: 'Maya',
              initials: 'M',
              contribution: 'folding chairs ×2',
              status: EdgeStatus.going,
            ),
          ),
        ),
      );

      expect(find.text('Maya'), findsOneWidget);
      expect(find.text('folding chairs ×2'), findsOneWidget);
      expect(find.text('IN'), findsOneWidget); // status pill is uppercased
      expect(find.text('M'), findsOneWidget); // initials avatar
    });

    testWidgets('hides the status pill when showStatusPill is false', (
      tester,
    ) async {
      await tester.pumpWidget(
        _host(
          const ContentEdgeRow(
            accentColor: Colors.green,
            statusLabel: 'In',
            showStatusPill: false,
            edge: EdgeViewData(
              userId: 'maya',
              name: 'Maya',
              initials: 'M',
              status: EdgeStatus.going,
            ),
          ),
        ),
      );

      expect(find.text('Maya'), findsOneWidget);
      expect(find.text('IN'), findsNothing); // pill suppressed
    });

    testWidgets('marks a maybe face with an amber "?" badge', (tester) async {
      await tester.pumpWidget(
        _host(
          const ContentEdgeRow(
            accentColor: Colors.green,
            statusLabel: 'Maybe',
            showStatusPill: false,
            edge: EdgeViewData(
              userId: 'priya',
              name: 'Priya',
              initials: 'P',
              status: EdgeStatus.maybe,
            ),
          ),
        ),
      );

      expect(find.text('?'), findsOneWidget); // maybe badge marker
    });
  });

  group('ContentEdgesCard', () {
    testWidgets('renders header, trailing summary, rows and footer', (
      tester,
    ) async {
      await tester.pumpWidget(
        _host(
          const ContentEdgesCard(
            accentColor: Colors.green,
            headerLabel: "Who's pitching in?",
            headerTrailing: '3 going',
            rows: [Text('row-a'), Text('row-b')],
            footer: Text('you-gap'),
          ),
        ),
      );

      expect(find.text("WHO'S PITCHING IN?"), findsOneWidget);
      expect(find.text('3 going'), findsOneWidget);
      expect(find.text('row-a'), findsOneWidget);
      expect(find.text('row-b'), findsOneWidget);
      expect(find.text('you-gap'), findsOneWidget);
    });

    testWidgets('renders the optional header leading slot', (tester) async {
      await tester.pumpWidget(
        _host(
          const ContentEdgesCard(
            accentColor: Colors.green,
            headerLabel: "Who's pitching in?",
            headerLeading: Text('faces'),
            rows: [Text('row')],
          ),
        ),
      );

      expect(find.text('faces'), findsOneWidget);
    });

    testWidgets('fires the whole-card tap handler with the card rect', (
      tester,
    ) async {
      Rect? tappedRect;
      await tester.pumpWidget(
        _host(
          ContentEdgesCard(
            accentColor: Colors.green,
            headerLabel: 'Going',
            onTap: (rect) => tappedRect = rect,
            semanticsLabel: 'Open attendees',
            rows: const [Text('row')],
          ),
        ),
      );

      // The whole card is tappable (child semantics stay readable, so tap a
      // child rather than the merged button label).
      await tester.tap(find.text('row'));
      // The callback receives the card's on-screen footprint (non-empty).
      expect(tappedRect, isNotNull);
      expect(tappedRect!.isEmpty, isFalse);
    });
  });
}
