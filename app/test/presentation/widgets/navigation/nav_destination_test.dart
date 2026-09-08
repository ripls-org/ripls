import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/navigation/nav_destination.dart';

void main() {
  group('RiplsNavDestination', () {
    test('stack indices are unique', () {
      final indices =
          RiplsNavDestination.values.map((d) => d.stackIndex).toSet();
      expect(indices.length, RiplsNavDestination.values.length);
    });

    test('fromStackIndex round-trips every destination', () {
      for (final destination in RiplsNavDestination.values) {
        expect(
          RiplsNavDestination.fromStackIndex(destination.stackIndex),
          destination,
        );
      }
    });

    test('orphaned stack entries map to no destination', () {
      // 0 = the retired Feed; it must never render as a selected tab.
      expect(RiplsNavDestination.fromStackIndex(0), isNull);
      expect(RiplsNavDestination.fromStackIndex(99), isNull);
    });

    test('destination indices match the home shell IndexedStack layout', () {
      // These are load-bearing: pre-dock navigateToTab callers use raw
      // stack indices (1 = Home, 2 = discover/Library, 3 = People).
      expect(RiplsNavDestination.home.stackIndex, 1);
      expect(RiplsNavDestination.library.stackIndex, 2);
      expect(RiplsNavDestination.people.stackIndex, 3);
      expect(RiplsNavDestination.plans.stackIndex, 4);
    });
  });
}
