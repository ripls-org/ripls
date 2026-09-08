import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';

void main() {
  group('ContentMorphPanel', () {
    testWidgets('renders its child', (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: ContentMorphPanel(child: Text('panel body')),
        ),
      );
      expect(find.text('panel body'), findsOneWidget);
    });

    testWidgets('a horizontal flick pops the route', (tester) async {
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
        MaterialPageRoute<void>(
          builder: (_) =>
              const ContentMorphPanel(child: Center(child: Text('panel'))),
        ),
      ));
      await tester.pumpAndSettle();
      expect(find.text('panel'), findsOneWidget);

      await tester.fling(find.text('panel'), const Offset(400, 0), 1200);
      await tester.pumpAndSettle();
      expect(find.text('panel'), findsNothing);
      expect(find.text('home'), findsOneWidget);
    });
  });
}
