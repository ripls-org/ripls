import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_conversation_carousel.dart';

void main() {
  group('resolveCarouselDragTarget', () {
    test('a fast leftward fling opens regardless of position', () {
      expect(
        resolveCarouselDragTarget(velocity: -900, position: 0.1),
        isTrue,
      );
    });

    test('a fast rightward fling closes regardless of position', () {
      expect(
        resolveCarouselDragTarget(velocity: 900, position: 0.9),
        isFalse,
      );
    });

    test('a slow drag settles to the nearer end', () {
      expect(resolveCarouselDragTarget(velocity: 0, position: 0.6), isTrue);
      expect(resolveCarouselDragTarget(velocity: 0, position: 0.4), isFalse);
    });

    test('exactly half-open settles open', () {
      expect(resolveCarouselDragTarget(velocity: 0, position: 0.5), isTrue);
    });

    test('velocity just under the fling threshold falls back to position', () {
      // Rightward but below threshold, dragged most of the way open → stays open.
      expect(
        resolveCarouselDragTarget(velocity: 399, position: 0.8),
        isTrue,
      );
    });
  });
}
