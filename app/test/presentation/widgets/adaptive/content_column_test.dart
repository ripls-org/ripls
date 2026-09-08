import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';

void main() {
  group('ContentColumn (#2912)', () {
    const childKey = Key('content');

    Future<void> pump(
      WidgetTester tester,
      Size surface, {
      required Widget body,
    }) async {
      tester.view.physicalSize = surface;
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.reset);
      await tester.pumpWidget(
        MaterialApp(home: Scaffold(body: body)),
      );
    }

    testWidgets('centers the child at the measure on a desktop-wide window',
        (tester) async {
      const surface = Size(1440, 810);
      await pump(
        tester,
        surface,
        body: ListView(children: const [
          ContentColumn(
            child: SizedBox(key: childKey, height: 40, width: double.infinity),
          ),
        ]),
      );

      final rect = tester.getRect(find.byKey(childKey));
      expect(rect.width, Responsive.contentMaxWidth);
      expect(rect.center.dx, moreOrLessEquals(surface.width / 2, epsilon: 1));
    });

    testWidgets('is a provable no-op at a phone width', (tester) async {
      const surface = Size(390, 844);

      // Same child pumped bare, then wrapped: the rects must be identical —
      // the whole phone-safety argument of the measure primitive.
      await pump(
        tester,
        surface,
        body: ListView(children: const [
          SizedBox(key: childKey, height: 40, width: double.infinity),
        ]),
      );
      final bare = tester.getRect(find.byKey(childKey));

      await pump(
        tester,
        surface,
        body: ListView(children: const [
          ContentColumn(
            child: SizedBox(key: childKey, height: 40, width: double.infinity),
          ),
        ]),
      );
      final wrapped = tester.getRect(find.byKey(childKey));

      expect(wrapped, bare);
      expect(wrapped.width, surface.width);
    });

    testWidgets('applies padding outside the constraint', (tester) async {
      const surface = Size(390, 844);
      await pump(
        tester,
        surface,
        body: ListView(children: const [
          ContentColumn(
            padding: EdgeInsets.symmetric(horizontal: 20),
            child: SizedBox(key: childKey, height: 40, width: double.infinity),
          ),
        ]),
      );

      expect(tester.getRect(find.byKey(childKey)).width, surface.width - 40);
    });

    testWidgets('honors a custom maxWidth', (tester) async {
      const surface = Size(1440, 810);
      await pump(
        tester,
        surface,
        body: ListView(children: const [
          ContentColumn(
            maxWidth: 500,
            child: SizedBox(key: childKey, height: 40, width: double.infinity),
          ),
        ]),
      );

      expect(tester.getRect(find.byKey(childKey)).width, 500);
    });
  });
}
