import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/home/home_editorial_row.dart';

Future<void> _pump(WidgetTester tester, Widget child) {
  return tester.pumpWidget(MaterialApp(home: Scaffold(body: child)));
}

void main() {
  group('HomeEditorialRow', () {
    testWidgets('renders title + subtitle and fires the pill action',
        (tester) async {
      var pilled = false;
      var tapped = false;
      await _pump(
        tester,
        HomeEditorialRow(
          title: 'Wheelbarrow',
          subtitle: 'Tyler · ready to pick up',
          pillLabel: 'Pick up',
          onPill: () => pilled = true,
          onTap: () => tapped = true,
          showTopBorder: false,
        ),
      );
      expect(find.text('Wheelbarrow'), findsOneWidget);
      expect(find.text('Tyler · ready to pick up'), findsOneWidget);

      await tester.tap(find.text('Pick up'));
      expect(pilled, isTrue);
      expect(tapped, isFalse);
    });

    testWidgets('pill exposes its own semantics node (not merged into the row)',
        (tester) async {
      // The pill must be a SIBLING of the row Tappable: nested inside it, the
      // row's excluded-descendants semantics swallow the pill's node, which on
      // Flutter Web routes every pill click to the row action and hides the
      // pill from screen readers (#2638 follow-up).
      await _pump(
        tester,
        HomeEditorialRow(
          title: 'Wheelbarrow',
          subtitle: 'Tyler · due Mon',
          pillLabel: 'Mark returned',
          onPill: () {},
          onTap: () {},
          showTopBorder: false,
        ),
      );
      expect(find.bySemanticsLabel('Mark returned'), findsOneWidget);
      expect(find.bySemanticsLabel('Wheelbarrow'), findsOneWidget);
    });

    testWidgets('shows a plain "when" date when no pill', (tester) async {
      await _pump(
        tester,
        HomeEditorialRow(
          title: 'Potluck dinner',
          subtitle: 'Hosting · Tue 28',
          whenLabel: 'Tue 28',
          onTap: () {},
        ),
      );
      expect(find.text('Tue 28'), findsOneWidget);
    });
  });
}
