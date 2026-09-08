import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/content/hero_content_wash.dart';

/// The gradients, top to bottom, of every DecoratedBox under [finder].
List<LinearGradient> _gradients(WidgetTester tester, Finder finder) => tester
    .widgetList<DecoratedBox>(
      find.descendant(of: finder, matching: find.byType(DecoratedBox)),
    )
    .map((b) => (b.decoration as BoxDecoration).gradient! as LinearGradient)
    .toList();

Future<void> _pump(WidgetTester tester, Widget child) => tester.pumpWidget(
      MaterialApp(
        home: Stack(
          children: [
            Positioned(
              left: 0,
              right: 0,
              bottom: 0,
              child: HeroContentWash(child: child),
            ),
          ],
        ),
      ),
    );

void main() {
  group('HeroContentWash', () {
    testWidgets('sizes itself to the content, not the frame', (tester) async {
      await _pump(tester, const SizedBox(height: 200));

      // The whole wash is the sheet plus the run-up above it — and nothing
      // more. A wash sized to the frame is what darkened every hero photo by a
      // third; this is the property that stops it happening again.
      expect(
        tester.getSize(find.byType(HeroContentWash)).height,
        200 + HeroContentWash.runUp,
      );
    });

    testWidgets('follows the content height when the content grows',
        (tester) async {
      await _pump(tester, const SizedBox(height: 200));
      final short = tester.getSize(find.byType(HeroContentWash)).height;

      await _pump(tester, const SizedBox(height: 500));
      final tall = tester.getSize(find.byType(HeroContentWash)).height;

      expect(tall - short, 300);
    });

    testWidgets('reaches full wash before the content starts', (tester) async {
      await _pump(tester, const SizedBox(height: 200));

      final gradients = _gradients(tester, find.byType(HeroContentWash));
      expect(gradients, hasLength(2));

      // The run-up ramps up to washTop...
      expect(gradients.first.colors, [Colors.transparent, OverlayTokens.washTop]);
      // ...so the sheet's FIRST pixel is already at full wash. The shipped
      // gradient put transparent at that edge and only reached 65% a fifth of
      // the way down, which left the title — ~16px in — on roughly 10% black
      // and measured 3.07:1.
      expect(gradients.last.colors.first, OverlayTokens.washTop);
      expect(gradients.last.colors.last, OverlayTokens.washBottom);
    });

    testWidgets('the run-up is the only part painted over bare photo',
        (tester) async {
      await _pump(tester, const SizedBox(height: 200));

      final runUp = tester.getSize(
        find.descendant(
          of: find.byType(HeroContentWash),
          matching: find.byType(SizedBox),
        ).first,
      );
      expect(runUp.height, HeroContentWash.runUp);
    });

    testWidgets('renders its child', (tester) async {
      await _pump(tester, const Text('hero title'));
      expect(find.text('hero title'), findsOneWidget);
    });
  });
}
