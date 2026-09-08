import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

void main() {
  Widget host(Widget child) => MaterialApp(home: Scaffold(body: Center(child: child)));

  group('Tappable', () {
    testWidgets('exposes Semantics with label and button role', (tester) async {
      await tester.pumpWidget(host(
        Tappable(
          semanticsLabel: 'Open profile',
          onTap: () {},
          child: const Text('Profile'),
        ),
      ));

      final node = tester.getSemantics(find.byType(Tappable));
      expect(node.label, 'Open profile');
      expect(node.flagsCollection.isButton, isTrue);
      expect(node.flagsCollection.isLink, isFalse);
      expect(node.flagsCollection.isEnabled, Tristate.isTrue);
    });

    testWidgets('marks disabled when both onTap and onLongPress are null', (tester) async {
      await tester.pumpWidget(host(
        const Tappable(
          semanticsLabel: 'Open profile',
          onTap: null,
          child: Text('Profile'),
        ),
      ));

      final node = tester.getSemantics(find.byType(Tappable));
      expect(node.flagsCollection.isEnabled, Tristate.isFalse);
    });

    testWidgets('isLink: true exposes link role instead of button', (tester) async {
      await tester.pumpWidget(host(
        Tappable(
          semanticsLabel: 'External link',
          onTap: () {},
          isLink: true,
          child: const Text('link'),
        ),
      ));

      final node = tester.getSemantics(find.byType(Tappable));
      expect(node.flagsCollection.isLink, isTrue);
      expect(node.flagsCollection.isButton, isFalse);
    });

    testWidgets('excludes child semantics by default', (tester) async {
      await tester.pumpWidget(host(
        Tappable(
          semanticsLabel: 'wrapper',
          onTap: () {},
          child: const Text('hidden child text'),
        ),
      ));

      final node = tester.getSemantics(find.byType(Tappable));
      expect(node.label, 'wrapper');
    });

    testWidgets('invokes onTap when tapped', (tester) async {
      var taps = 0;
      await tester.pumpWidget(host(
        Tappable(
          semanticsLabel: 'tap me',
          onTap: () => taps++,
          child: const SizedBox(width: 50, height: 50),
        ),
      ));

      await tester.tap(find.byType(Tappable));
      expect(taps, 1);
    });

    testWidgets('semanticsIdentifier is null by default', (tester) async {
      await tester.pumpWidget(host(
        Tappable(
          semanticsLabel: 'Open profile',
          onTap: () {},
          child: const Text('Profile'),
        ),
      ));

      final node = tester.getSemantics(find.byType(Tappable));
      expect(node.identifier, isEmpty);
    });

    testWidgets('semanticsIdentifier flows through to the Semantics node',
        (tester) async {
      await tester.pumpWidget(host(
        Tappable(
          semanticsLabel: 'Open profile',
          semanticsIdentifier: 'open-profile-button',
          onTap: () {},
          child: const Text('Profile'),
        ),
      ));

      final node = tester.getSemantics(find.byType(Tappable));
      expect(node.identifier, 'open-profile-button');
      // identifier doesn't replace label; both ride on the same node.
      expect(node.label, 'Open profile');
    });
  });
}
