import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/content/content_gradient_overlay.dart';

/// Reads the gradient the overlay actually painted.
LinearGradient gradientOf(WidgetTester tester) {
  final box = tester.widget<DecoratedBox>(
    find.descendant(
      of: find.byType(ContentGradientOverlay),
      matching: find.byType(DecoratedBox),
    ),
  );
  return (box.decoration as BoxDecoration).gradient! as LinearGradient;
}

Widget host(Widget child) => MaterialApp(home: Scaffold(body: child));

void main() {
  group('ContentGradientOverlay', () {
    testWidgets('renders a top-to-bottom gradient', (tester) async {
      await tester.pumpWidget(host(const ContentGradientOverlay()));
      final g = gradientOf(tester);
      expect(find.byType(ContentGradientOverlay), findsOneWidget);
      expect(g.begin, Alignment.topCenter);
      expect(g.end, Alignment.bottomCenter);
      expect(g.stops, const [0.0, 0.15, 1.0]);
    });

    testWidgets('darkens only the top strip, and clears below it',
        (tester) async {
      // The counterpart to HeroContentWash's "sized to the content" test, and
      // the reason the photos are not flat. This layer is frame-anchored, so
      // anything it paints below the controls lands on whatever the photo
      // happens to be showing there. An earlier revision held a floor all the
      // way down: it made the text pass, and cost every hero photo 26-37% of
      // its mean brightness.
      await tester.pumpWidget(host(const ContentGradientOverlay()));
      final g = gradientOf(tester);
      expect(g.colors.first, OverlayTokens.scrimTop);
      for (final c in g.colors.skip(1)) {
        expect(c.a, 0, reason: 'a frame-wide wash is back — see #2770');
      }
    });

    testWidgets('paints the same gradient whatever media is behind it',
        (tester) async {
      // mediaCacheKey is vestigial: it used to select a per-photo alpha for a
      // floor that no longer exists. Passing one must not change anything.
      await tester.pumpWidget(host(const ContentGradientOverlay()));
      final without = gradientOf(tester).colors;

      await tester.pumpWidget(
        host(const ContentGradientOverlay(mediaCacheKey: 'some-photo')),
      );
      expect(gradientOf(tester).colors, without);
    });

    testWidgets('can be used in a Stack with Positioned.fill', (tester) async {
      await tester.pumpWidget(
        host(Stack(
          children: [
            ColoredBox(color: Colors.blue, child: const SizedBox.expand()),
            const Positioned.fill(child: ContentGradientOverlay()),
          ],
        )),
      );
      expect(find.byType(ContentGradientOverlay), findsOneWidget);
      expect(find.byType(Positioned), findsOneWidget);
    });
  });
}
