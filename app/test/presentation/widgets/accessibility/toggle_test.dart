import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';

void main() {
  Widget host(Widget child) => MaterialApp(home: Scaffold(body: Center(child: child)));

  group('Toggle', () {
    testWidgets('selected: true exposes selected semantic flag', (tester) async {
      await tester.pumpWidget(host(
        Toggle(
          semanticsLabel: 'Going',
          selected: true,
          onTap: () {},
          child: const Text('Going'),
        ),
      ));

      final node = tester.getSemantics(find.byType(Toggle));
      expect(node.label, 'Going');
      expect(node.flagsCollection.isSelected, Tristate.isTrue);
      expect(node.flagsCollection.isButton, isTrue);
    });

    testWidgets('selected: false exposes unselected semantic flag', (tester) async {
      await tester.pumpWidget(host(
        Toggle(
          semanticsLabel: 'Going',
          selected: false,
          onTap: () {},
          child: const Text('Going'),
        ),
      ));

      final node = tester.getSemantics(find.byType(Toggle));
      expect(node.flagsCollection.isSelected, Tristate.isFalse);
    });

    testWidgets('invokes onTap', (tester) async {
      var taps = 0;
      await tester.pumpWidget(host(
        Toggle(
          semanticsLabel: 'Maybe',
          selected: false,
          onTap: () => taps++,
          child: const SizedBox(width: 50, height: 50),
        ),
      ));

      await tester.tap(find.byType(Toggle));
      expect(taps, 1);
    });

    testWidgets('semanticsIdentifier merges with selected + label',
        (tester) async {
      await tester.pumpWidget(host(
        Toggle(
          semanticsLabel: 'Going',
          semanticsIdentifier: 'rsvp-yes',
          selected: true,
          onTap: () {},
          child: const Text('Going'),
        ),
      ));

      final node = tester.getSemantics(find.byType(Toggle));
      // identifier and label both surface on the merged semantics node
      // so the e2e harness queries by identifier without colliding with
      // the toggle's selected/label semantics.
      expect(node.identifier, 'rsvp-yes');
      expect(node.label, 'Going');
      expect(node.flagsCollection.isSelected, Tristate.isTrue);
    });
  });
}
