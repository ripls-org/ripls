import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/content/morph_reveal_route.dart';

void main() {
  group('morphRevealRoute', () {
    testWidgets('reveals the destination and reverses on pop', (tester) async {
      late BuildContext ctx;
      await tester.pumpWidget(
        MaterialApp(
          home: Builder(
            builder: (context) {
              ctx = context;
              return const Scaffold(body: Text('home'));
            },
          ),
        ),
      );

      unawaited(Navigator.of(ctx).push(
        morphRevealRoute<void>(
          screen: const Scaffold(body: Center(child: Text('expanded'))),
          sourceRect: const Rect.fromLTWH(20, 400, 300, 120),
          duration: const Duration(milliseconds: 200),
          routeName: 'morph',
        ),
      ));
      await tester.pumpAndSettle();
      expect(find.text('expanded'), findsOneWidget);
      // Non-opaque route: the page behind is still in the tree.
      expect(find.text('home'), findsOneWidget);

      // Pop plays the reverse morph and removes the destination.
      Navigator.of(ctx).pop();
      await tester.pumpAndSettle();
      expect(find.text('expanded'), findsNothing);
      expect(find.text('home'), findsOneWidget);
    });

    testWidgets('shows the destination immediately with zero duration', (
      tester,
    ) async {
      late BuildContext ctx;
      await tester.pumpWidget(
        MaterialApp(
          home: Builder(
            builder: (context) {
              ctx = context;
              return const Scaffold(body: Text('home'));
            },
          ),
        ),
      );

      unawaited(Navigator.of(ctx).push(
        morphRevealRoute<void>(
          screen: const Scaffold(body: Center(child: Text('expanded'))),
          sourceRect: const Rect.fromLTWH(0, 0, 100, 100),
          // Reduce-motion: accessibleDuration would return zero.
          duration: Duration.zero,
        ),
      ));
      await tester.pump();
      expect(find.text('expanded'), findsOneWidget);
    });
  });
}
