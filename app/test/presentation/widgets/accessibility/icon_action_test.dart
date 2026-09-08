import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';

void main() {
  Widget host(Widget child) => MaterialApp(home: Scaffold(body: Center(child: child)));

  group('IconAction', () {
    testWidgets('exposes Semantics with label and button role', (tester) async {
      await tester.pumpWidget(host(
        IconAction(
          icon: Icons.close,
          semanticsLabel: 'Close',
          onPressed: () {},
        ),
      ));

      final node = tester.getSemantics(find.byType(IconAction));
      expect(node.label, contains('Close'));
      expect(node.flagsCollection.isButton, isTrue);
    });

    testWidgets('uses semanticsLabel as default tooltip', (tester) async {
      await tester.pumpWidget(host(
        IconAction(
          icon: Icons.close,
          semanticsLabel: 'Close modal',
          onPressed: () {},
        ),
      ));

      final tooltip = tester.widget<Tooltip>(
        find.descendant(
          of: find.byType(IconAction),
          matching: find.byType(Tooltip),
        ),
      );
      expect(tooltip.message, 'Close modal');
    });

    testWidgets('honors explicit tooltip override', (tester) async {
      await tester.pumpWidget(host(
        IconAction(
          icon: Icons.close,
          semanticsLabel: 'Close modal',
          tooltip: 'Tap to dismiss',
          onPressed: () {},
        ),
      ));

      final tooltip = tester.widget<Tooltip>(
        find.descendant(
          of: find.byType(IconAction),
          matching: find.byType(Tooltip),
        ),
      );
      expect(tooltip.message, 'Tap to dismiss');
    });

    testWidgets('invokes onPressed', (tester) async {
      var presses = 0;
      await tester.pumpWidget(host(
        IconAction(
          icon: Icons.close,
          semanticsLabel: 'Close',
          onPressed: () => presses++,
        ),
      ));

      await tester.tap(find.byIcon(Icons.close));
      expect(presses, 1);
    });

    testWidgets('marks disabled when onPressed is null', (tester) async {
      await tester.pumpWidget(host(
        const IconAction(
          icon: Icons.close,
          semanticsLabel: 'Close',
          onPressed: null,
        ),
      ));

      final node = tester.getSemantics(find.byType(IconAction));
      expect(node.flagsCollection.isEnabled, Tristate.isFalse);
    });

    testWidgets('semanticsIdentifier wraps in a Semantics node when set',
        (tester) async {
      await tester.pumpWidget(host(
        IconAction(
          icon: Icons.close,
          semanticsLabel: 'Close',
          semanticsIdentifier: 'modal-close-button',
          onPressed: () {},
        ),
      ));

      final node = tester.getSemantics(find.byType(IconAction));
      expect(node.identifier, 'modal-close-button');
    });

    testWidgets('semanticsIdentifier null leaves the IconButton unwrapped',
        (tester) async {
      await tester.pumpWidget(host(
        IconAction(
          icon: Icons.close,
          semanticsLabel: 'Close',
          onPressed: () {},
        ),
      ));

      final node = tester.getSemantics(find.byType(IconAction));
      expect(node.identifier, isEmpty);
    });
  });
}
