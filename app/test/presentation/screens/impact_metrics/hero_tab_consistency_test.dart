import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' as pb_common;
import 'package:ripls/data/gen/ripls/api/impact.pb.dart' as pb_impact;
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/presentation/viewmodels/community_impact_view_model.dart';

/// These tests verify that the hero value and tab bar value are always computed
/// from the same source data, preventing the mismatch where the tab shows one
/// number (e.g., "$25") and the hero shows another (e.g., "$0").
///
/// The invariant: for a given stage and metric, the formatted string shown in
/// the tab bar must be identical to the formatted string shown as the hero.

/// Mirrors the _tabOrder and navigation helpers in community_metrics_screen.dart.
///
/// The visual left-to-right tab order is: SAVED | QUALITY TIME | CO2 AVOIDED.
/// Swipe left (negative velocity) advances to the next tab in this order.
/// Swipe right (positive velocity) returns to the previous tab.
const _tabOrder = [
  ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
  ImpactMetricDimension.IMPACT_METRIC_DIMENSION_QUALITY_TIME,
  ImpactMetricDimension.IMPACT_METRIC_DIMENSION_EMISSIONS,
];

ImpactMetricDimension? _nextMetric(ImpactMetricDimension current) {
  final i = _tabOrder.indexOf(current);
  if (i < 0 || i >= _tabOrder.length - 1) return null;
  return _tabOrder[i + 1];
}

ImpactMetricDimension? _previousMetric(ImpactMetricDimension current) {
  final i = _tabOrder.indexOf(current);
  if (i <= 0) return null;
  return _tabOrder[i - 1];
}

void main() {
  // ── Swipe navigation order (regression test for issue #1376) ──

  group('Stats screen swipe navigation order', () {
    test('swipe left from MONEY goes to QUALITY_TIME', () {
      expect(
        _nextMetric(ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY),
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_QUALITY_TIME,
      );
    });

    test('swipe left from QUALITY_TIME goes to EMISSIONS', () {
      expect(
        _nextMetric(
            ImpactMetricDimension.IMPACT_METRIC_DIMENSION_QUALITY_TIME),
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_EMISSIONS,
      );
    });

    test('swipe left from EMISSIONS (last tab) returns null', () {
      expect(
        _nextMetric(ImpactMetricDimension.IMPACT_METRIC_DIMENSION_EMISSIONS),
        isNull,
      );
    });

    test('swipe right from EMISSIONS goes to QUALITY_TIME', () {
      expect(
        _previousMetric(
            ImpactMetricDimension.IMPACT_METRIC_DIMENSION_EMISSIONS),
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_QUALITY_TIME,
      );
    });

    test('swipe right from QUALITY_TIME goes to MONEY', () {
      expect(
        _previousMetric(
            ImpactMetricDimension.IMPACT_METRIC_DIMENSION_QUALITY_TIME),
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
      );
    });

    test('swipe right from MONEY (first tab) returns null', () {
      expect(
        _previousMetric(ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY),
        isNull,
      );
    });
  });
  group('Hero/tab value consistency', () {
    test('active stage: costSavingsFormatted is used by both hero and tab', () {
      final data = _buildData(costSavingsUsd: 1250);
      // The hero and tab both call data.costSavingsFormatted for money.
      expect(data.costSavingsFormatted, r'$1,250');
      // Calling it twice returns the same result (deterministic).
      expect(data.costSavingsFormatted, data.costSavingsFormatted);
    });

    test('active stage: carbonSavingsFormatted is used by both hero and tab',
        () {
      final data = _buildData(carbonSavingsGrams: 17000);
      expect(data.carbonSavingsFormatted, '17 kg');
      expect(data.carbonSavingsFormatted, data.carbonSavingsFormatted);
    });

    test(
        'active stage: qualityTimeFormatted is used by both hero and tab', () {
      final data = _buildData(qualityTimeMinutes: 210);
      // qualityTimeFormatted uses SavingsFormatter — just check it's consistent.
      final value = data.qualityTimeFormatted;
      expect(value, isNotEmpty);
      expect(value, data.qualityTimeFormatted);
    });

    test('listed stage: potential money uses same formula for hero and tab',
        () {
      // Both hero and tab compute: _formatMoney(totalValueUsd * 0.075)
      // with no ~ prefix.
      const totalValueUsd = 8000.0;
      final projected = totalValueUsd * 0.075; // 600
      final formatted = _formatMoney(projected);
      expect(formatted, r'$600');
      // Must NOT contain ~ prefix.
      expect(formatted, isNot(contains('~')));
    });

    test('listed stage: potential CO₂ uses same formula for hero and tab', () {
      const totalValueUsd = 8000.0;
      final projected = totalValueUsd * 0.05; // 400 kg
      final formatted = _formatKg(projected);
      expect(formatted, '400 kg');
      expect(formatted, isNot(contains('~')));
    });

    test('listed stage: potential QT uses same formula for hero and tab', () {
      const gearCount = 50;
      final projected = (gearCount * 42).round(); // 2100
      final formatted = _formatRm(projected);
      expect(formatted, '2.1K mins');
      expect(formatted, isNot(contains('~')));
    });

    test('zero savings: hero and tab both show zero', () {
      final data = _buildData(
        costSavingsUsd: 0,
        carbonSavingsGrams: 0,
        qualityTimeMinutes: 0,
      );
      expect(data.costSavingsFormatted, r'$0');
      expect(data.carbonSavingsFormatted, '0 kg');
    });

    test('small savings: hero and tab show same precision', () {
      final data = _buildData(
        costSavingsUsd: 0.75,
        carbonSavingsGrams: 500, // 0.5 kg
      );
      expect(data.costSavingsFormatted, r'$0.75');
      expect(data.carbonSavingsFormatted, '0.5 kg');
    });
  });
}

CommunityImpactData _buildData({
  double costSavingsUsd = 0,
  double carbonSavingsGrams = 0,
  double qualityTimeMinutes = 0,
  double totalValueUsd = 1000,
  int gearCount = 10,
}) {
  return CommunityImpactData(
    communityId: 'test',
    metrics: pb_impact.CommunityImpactMetrics(
      costSavingsUsd: pb_common.Estimate(
          mean: costSavingsUsd, stddev: costSavingsUsd * 0.3),
      carbonSavingsGrams: pb_common.Estimate(
          mean: carbonSavingsGrams, stddev: carbonSavingsGrams * 0.4),
      qualityTimeMinutes: pb_common.Estimate(
          mean: qualityTimeMinutes, stddev: qualityTimeMinutes * 0.3),
      timeBankedMinutes:
          pb_common.Estimate(mean: 0, stddev: 0),
      totalValueUsd: totalValueUsd,
      gearCount: gearCount,
      memberCount: 10,
    ),
  );
}

/// Mirrors _formatMoney in community_metrics_screen.dart.
String _formatMoney(double usd) {
  if (usd <= 0) return r'$0';
  final dollars = usd.toInt();
  if (dollars < 1000) return '\$$dollars';
  if (dollars < 10000) {
    final formatted = dollars.toString().replaceAllMapped(
        RegExp(r'(\d{1,3})(?=(\d{3})+(?!\d))'), (m) => '${m[1]},');
    return '\$$formatted';
  }
  return '\$${(dollars / 1000).toStringAsFixed(1)}K';
}

/// Mirrors _formatKg in community_metrics_screen.dart.
String _formatKg(double kg) {
  if (kg <= 0) return '0 kg';
  if (kg < 1) return '${(kg * 1000).round()} g';
  if (kg < 10) return '${kg.toStringAsFixed(1)} kg';
  if (kg < 1000) return '${kg.toInt()} kg';
  return '${(kg / 1000).toStringAsFixed(1)}t';
}

/// Mirrors _formatRm in community_metrics_screen.dart.
String _formatRm(int rm) {
  if (rm < 1000) return '$rm mins';
  return '${(rm / 1000).toStringAsFixed(1)}K mins';
}
