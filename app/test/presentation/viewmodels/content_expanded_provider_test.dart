import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';

void main() {
  group('experienceContentExpandedProvider', () {
    test('defaults to false and toggles per experience', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);

      expect(container.read(experienceContentExpandedProvider('a')), isFalse);

      container
          .read(experienceContentExpandedProvider('a').notifier)
          .set(true);
      expect(container.read(experienceContentExpandedProvider('a')), isTrue);
      // A different experience is unaffected (family isolation).
      expect(container.read(experienceContentExpandedProvider('b')), isFalse);

      container
          .read(experienceContentExpandedProvider('a').notifier)
          .set(false);
      expect(container.read(experienceContentExpandedProvider('a')), isFalse);
    });
  });

  group('requestContentExpandedProvider', () {
    test('defaults to false and toggles per request, isolated from experience',
        () {
      final container = ProviderContainer();
      addTearDown(container.dispose);

      expect(container.read(requestContentExpandedProvider('r1')), isFalse);

      container.read(requestContentExpandedProvider('r1').notifier).set(true);
      expect(container.read(requestContentExpandedProvider('r1')), isTrue);
      // A different request is unaffected (family isolation).
      expect(container.read(requestContentExpandedProvider('r2')), isFalse);
      // The experience flag with the same key is a distinct provider.
      expect(container.read(experienceContentExpandedProvider('r1')), isFalse);

      container.read(requestContentExpandedProvider('r1').notifier).set(false);
      expect(container.read(requestContentExpandedProvider('r1')), isFalse);
    });
  });
}
