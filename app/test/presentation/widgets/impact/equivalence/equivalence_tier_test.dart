import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/equivalence_tier.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/money_paycheck_ladder.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/time_health_ladder.dart';

void main() {
  group('selectTier', () {
    final ladder = [
      EquivalenceTier(
        id: 'a',
        threshold: 10,
        unit: EquivalenceUnit.hours,
        labelKey: 'k1',
        copyKey: 'k2',
        sourceNameKey: 'k3',
        sourceUrl: 'https://example.com/a',
      ),
      EquivalenceTier(
        id: 'b',
        threshold: 50,
        unit: EquivalenceUnit.hours,
        labelKey: 'k4',
        copyKey: 'k5',
        sourceNameKey: 'k6',
        sourceUrl: 'https://example.com/b',
      ),
      EquivalenceTier(
        id: 'c',
        threshold: 100,
        unit: EquivalenceUnit.hours,
        labelKey: 'k7',
        copyKey: 'k8',
        sourceNameKey: 'k9',
        sourceUrl: 'https://example.com/c',
      ),
    ];

    test('returns null on empty ladder', () {
      expect(selectTier(const [], 5), isNull);
    });

    test('returns null when value below lowest threshold', () {
      expect(selectTier(ladder, 5)?.id, isNull);
    });

    test('returns the matching tier when value is exactly at the threshold',
        () {
      expect(selectTier(ladder, 10)?.id, 'a');
      expect(selectTier(ladder, 50)?.id, 'b');
      expect(selectTier(ladder, 100)?.id, 'c');
    });

    test('returns the highest tier whose threshold <= value', () {
      expect(selectTier(ladder, 49)?.id, 'a');
      expect(selectTier(ladder, 75)?.id, 'b');
      expect(selectTier(ladder, 200)?.id, 'c');
    });

    test('returns the top tier for very large values', () {
      expect(selectTier(ladder, 1000000000)?.id, 'c');
    });
  });

  group('time-health ladder shape', () {
    test('thresholds are strictly increasing', () {
      for (var i = 1; i < kTimeHealthLadder.length; i++) {
        expect(kTimeHealthLadder[i].threshold,
            greaterThan(kTimeHealthLadder[i - 1].threshold));
      }
    });

    test('all tiers are hours unit', () {
      for (final t in kTimeHealthLadder) {
        expect(t.unit, EquivalenceUnit.hours);
      }
    });

    test('tier ids are unique', () {
      final ids = kTimeHealthLadder.map((t) => t.id).toSet();
      expect(ids.length, kTimeHealthLadder.length);
    });
  });

  group('money-paycheck ladder shape', () {
    test('thresholds are strictly increasing', () {
      for (var i = 1; i < kMoneyPaycheckLadder.length; i++) {
        expect(kMoneyPaycheckLadder[i].threshold,
            greaterThan(kMoneyPaycheckLadder[i - 1].threshold));
      }
    });

    test('all tiers are USD unit', () {
      for (final t in kMoneyPaycheckLadder) {
        expect(t.unit, EquivalenceUnit.usd);
      }
    });

    test('tier ids are unique', () {
      final ids = kMoneyPaycheckLadder.map((t) => t.id).toSet();
      expect(ids.length, kMoneyPaycheckLadder.length);
    });
  });
}
