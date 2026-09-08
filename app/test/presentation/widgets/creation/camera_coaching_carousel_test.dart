import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/creation/camera_coaching_carousel.dart';

import '../../../helpers/l10n_helpers.dart';

void main() {
  group('CameraCoachingCarousel', () {
    testWidgets('renders first nudge on initial mount', (tester) async {
      await tester.pumpWidget(localizedApp(
        const Scaffold(body: Center(child: CameraCoachingCarousel())),
      ));

      // First nudge corresponds to _CoachingExample.gear.
      expect(find.text("Try gear, games, or anything you'd lend"),
          findsOneWidget);
    });

    testWidgets('renders exactly four thumbnails as Tappable widgets',
        (tester) async {
      await tester.pumpWidget(localizedApp(
        const Scaffold(body: Center(child: CameraCoachingCarousel())),
      ));

      // Each thumbnail carries a stable key.
      expect(find.byKey(const Key('camera-coaching-thumb-gear')), findsOneWidget);
      expect(find.byKey(const Key('camera-coaching-thumb-clothes')),
          findsOneWidget);
      expect(find.byKey(const Key('camera-coaching-thumb-food')), findsOneWidget);
      expect(find.byKey(const Key('camera-coaching-thumb-flyer')),
          findsOneWidget);

      // Each thumbnail's keyed widget is a Tappable (not a raw
      // GestureDetector / InkWell). The key is set on the Tappable
      // itself, so the keyed widget's runtime type IS Tappable.
      final thumbs = [
        const Key('camera-coaching-thumb-gear'),
        const Key('camera-coaching-thumb-flyer'),
        const Key('camera-coaching-thumb-clothes'),
        const Key('camera-coaching-thumb-food'),
      ];
      for (final key in thumbs) {
        final widget = tester.widget(find.byKey(key));
        expect(widget, isA<Tappable>(),
            reason: '$key should be a Tappable widget');
      }
    });

    testWidgets('cycles to the next nudge after 3 seconds', (tester) async {
      await tester.pumpWidget(localizedApp(
        const Scaffold(body: Center(child: CameraCoachingCarousel())),
      ));
      expect(find.text("Try gear, games, or anything you'd lend"),
          findsOneWidget);

      // Cross the 3-second tick boundary plus the AnimatedSwitcher fade.
      await tester.pump(const Duration(seconds: 3));
      await tester.pump(const Duration(milliseconds: 400));

      // Second example (flyer) nudge is visible.
      expect(find.text('Get the whole event flyer in the frame'),
          findsOneWidget);
    });

    testWidgets('tapping a thumbnail locks the active example and cancels '
        'the auto-cycle', (tester) async {
      await tester.pumpWidget(localizedApp(
        const Scaffold(body: Center(child: CameraCoachingCarousel())),
      ));

      // Tap the food thumbnail.
      await tester.tap(find.byKey(const Key('camera-coaching-thumb-food')));
      await tester.pump(const Duration(milliseconds: 400));
      expect(find.text("Snap what you can't finish on your own"),
          findsOneWidget);

      // Pump well past the 3 s cycle interval; the locked example
      // should still be visible.
      await tester.pump(const Duration(seconds: 6));
      await tester.pump(const Duration(milliseconds: 400));
      expect(find.text("Snap what you can't finish on your own"),
          findsOneWidget);
    });
  });
}
