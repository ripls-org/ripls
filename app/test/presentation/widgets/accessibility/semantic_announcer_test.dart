import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';

void main() {
  group('LiveRegion', () {
    testWidgets('child is wrapped in Semantics(liveRegion: true) when enabled',
        (tester) async {
      await tester.pumpWidget(const MaterialApp(
        home: Scaffold(
          body: LiveRegion(child: Text('hello')),
        ),
      ));

      final node = tester.getSemantics(find.text('hello'));
      expect(node.flagsCollection.isLiveRegion, isTrue);
    });

    testWidgets('disabled LiveRegion does not flag the subtree as live',
        (tester) async {
      await tester.pumpWidget(const MaterialApp(
        home: Scaffold(
          body: LiveRegion(enabled: false, child: Text('hello')),
        ),
      ));

      final node = tester.getSemantics(find.text('hello'));
      expect(node.flagsCollection.isLiveRegion, isFalse);
    });
  });

  group('SemanticAnnouncer.announce', () {
    testWidgets('no-ops on empty message', (tester) async {
      await tester.pumpWidget(const MaterialApp(home: Scaffold(body: SizedBox())));
      final context = tester.element(find.byType(SizedBox));
      await SemanticAnnouncer.announce(context, '');
      // Reaching here without throwing is the contract; the SemanticsService
      // platform channel is mocked in widget tests, so no announcement is
      // delivered to a real reader.
    });
  });
}
