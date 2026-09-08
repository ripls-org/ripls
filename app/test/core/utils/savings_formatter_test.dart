import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';

/// Helper to build a populated ImpactEstimate proto for testing.
ImpactEstimate _buildImpactEstimate({
  double costMean = 50.0,
  double costStddev = 15.0,
  double timeMean = 120.0,
  double timeStddev = 36.0,
  double co2Mean = 24000.0,
  double co2Stddev = 7200.0,
  String costProvenanceName = 'prevented_purchase',
  String timeProvenanceName = 'gear_shopping_time',
  String co2ProvenanceName = 'weight_material_carbon',
  String costReasoning = 'Loan: \$100 x 50% prevented purchase rate.',
  String timeReasoning = 'Default shopping time from config.',
  String co2Reasoning = 'Material-weight lookup: 4kg steel.',
  List<String> costSources = const ['Library of Things UK'],
  List<String> timeSources = const ['Impact estimation config'],
  List<String> co2Sources = const ['ICE Database v3.0'],
}) {
  return ImpactEstimate(
    moneySaved: MoneySavings(
      valueUsd: Estimate(mean: costMean, stddev: costStddev),
      provenance: Provenance(
        source: ProvenanceSource.PROVENANCE_SOURCE_FORMULA,
        name: costProvenanceName,
        reasoning: costReasoning,
        sources: costSources,
      ),
    ),
    timeSaved: TimeSavings(
      minutes: Estimate(mean: timeMean, stddev: timeStddev),
      provenance: Provenance(
        source: ProvenanceSource.PROVENANCE_SOURCE_CONFIG_DEFAULT,
        name: timeProvenanceName,
        reasoning: timeReasoning,
        sources: timeSources,
      ),
    ),
    emissionsPrevented: PreventedEmissions(
      manufactureAvoidedCarbon: CarbonEstimate(
        co2eGrams: Estimate(mean: co2Mean, stddev: co2Stddev),
      ),
      provenance: Provenance(
        source: ProvenanceSource.PROVENANCE_SOURCE_FORMULA,
        name: co2ProvenanceName,
        reasoning: co2Reasoning,
        sources: co2Sources,
      ),
    ),
  );
}

void main() {
  group('SavingsFormatter.formatSavings', () {
    test('returns null for null impact', () {
      expect(SavingsFormatter.formatSavings(null), isNull);
    });

    test('formats all three dimensions from populated impact', () {
      final impact = _buildImpactEstimate();
      final result = SavingsFormatter.formatSavings(impact);

      expect(result, isNotNull);
      expect(result!.costSaved, equals('\$50'));
      expect(result.timeSaved, equals('2h'));
      expect(result.co2Saved, equals('24kg'));
    });

    test('formats zero values', () {
      final impact = _buildImpactEstimate(
        costMean: 0,
        timeMean: 0,
        co2Mean: 0,
      );
      final result = SavingsFormatter.formatSavings(impact);

      expect(result, isNotNull);
      expect(result!.costSaved, equals('\$0'));
      expect(result.timeSaved, equals('0h'));
      expect(result.co2Saved, equals('0kg'));
    });

    test('handles empty ImpactEstimate', () {
      final impact = ImpactEstimate();
      final result = SavingsFormatter.formatSavings(impact);

      expect(result, isNotNull);
      expect(result!.costSaved, equals('\$0'));
      expect(result.timeSaved, equals('0h'));
      expect(result.co2Saved, equals('0kg'));
    });

    test('formats large values', () {
      final impact = _buildImpactEstimate(
        costMean: 2500,
        timeMean: 2880, // 48 hours = 2 days
        co2Mean: 1500000, // 1500 kg = 1.5 tonnes
      );
      final result = SavingsFormatter.formatSavings(impact);

      expect(result, isNotNull);
      expect(result!.costSaved, equals('\$2.5k'));
      expect(result.timeSaved, equals('2d'));
      expect(result.co2Saved, equals('1.5t'));
    });

    test('formats small values', () {
      final impact = _buildImpactEstimate(
        costMean: 0.50,
        timeMean: 15, // 15 minutes
        co2Mean: 500, // 0.5 kg
      );
      final result = SavingsFormatter.formatSavings(impact);

      expect(result, isNotNull);
      expect(result!.costSaved, equals('\$0.50'));
      expect(result.timeSaved, equals('15m'));
      expect(result.co2Saved, equals('500g'));
    });
  });

  group('SavingsFormatter cost detail', () {
    test('builds cost detail with all fields', () {
      final impact = _buildImpactEstimate();
      final result = SavingsFormatter.formatSavings(impact);

      final detail = result!.costDetail;
      expect(detail, isNotNull);
      expect(detail!.label, equals('SAVED'));
      expect(detail.displayValue, equals('\$50'));
      expect(detail.confidence, isNotNull);
      expect(detail.confidence!, greaterThan(0));
      expect(detail.confidence!, lessThanOrEqualTo(1.0));
      expect(detail.methodName, equals('Prevented Purchase'));
      expect(detail.reasoning, equals('Loan: \$100 x 50% prevented purchase rate.'));
      expect(detail.sources, equals(['Library of Things UK']));
    });

    test('returns null cost detail when cost is zero', () {
      final impact = _buildImpactEstimate(costMean: 0);
      final result = SavingsFormatter.formatSavings(impact);

      expect(result!.costDetail, isNull);
    });

    test('returns null for empty reasoning and sources', () {
      final impact = _buildImpactEstimate(
        costReasoning: '',
        costSources: [],
      );
      final result = SavingsFormatter.formatSavings(impact);

      final detail = result!.costDetail;
      expect(detail, isNotNull);
      expect(detail!.reasoning, isNull);
      expect(detail.sources, isNull);
    });
  });

  group('SavingsFormatter time detail', () {
    test('builds time detail with all fields', () {
      final impact = _buildImpactEstimate();
      final result = SavingsFormatter.formatSavings(impact);

      final detail = result!.timeDetail;
      expect(detail, isNotNull);
      expect(detail!.label, equals('TIME RECOVERED'));
      expect(detail.displayValue, equals('2h'));
      expect(detail.confidence, isNotNull);
      expect(detail.methodName, equals('Research-Based Default'));
      expect(detail.reasoning, equals('Default shopping time from config.'));
      expect(detail.sources, equals(['Impact estimation config']));
    });

    test('returns null time detail when minutes is zero', () {
      final impact = _buildImpactEstimate(timeMean: 0);
      final result = SavingsFormatter.formatSavings(impact);

      expect(result!.timeDetail, isNull);
    });

    test('uses AI Estimated method for hint-based times', () {
      final impact = _buildImpactEstimate(
        timeProvenanceName: 'genai_time_estimate',
      );
      final result = SavingsFormatter.formatSavings(impact);

      expect(result!.timeDetail!.methodName, equals('AI Estimated'));
    });
  });

  group('SavingsFormatter CO2 detail', () {
    test('builds CO2 detail with all fields', () {
      final impact = _buildImpactEstimate();
      final result = SavingsFormatter.formatSavings(impact);

      final detail = result!.co2Detail;
      expect(detail, isNotNull);
      expect(detail!.label, contains('CO'));
      expect(detail.displayValue, equals('24kg'));
      expect(detail.confidence, isNotNull);
      expect(detail.methodName, contains('Weight'));
      expect(detail.reasoning, equals('Material-weight lookup: 4kg steel.'));
      expect(detail.sources, equals(['ICE Database v3.0']));
    });

    test('returns null CO2 detail when grams is zero', () {
      final impact = _buildImpactEstimate(co2Mean: 0);
      final result = SavingsFormatter.formatSavings(impact);

      expect(result!.co2Detail, isNull);
    });

    test('handles missing manufacture_avoided_carbon', () {
      final impact = ImpactEstimate(
        emissionsPrevented: PreventedEmissions(
          provenance: Provenance(reasoning: 'Some reasoning'),
        ),
      );
      final result = SavingsFormatter.formatSavings(impact);

      expect(result!.co2Detail, isNull);
    });
  });

  group('SavingsFormatter.provenanceDisplayName', () {
    test('maps all provenance names to display names', () {
      expect(
        SavingsFormatter.provenanceDisplayName('product_lca_carbon'),
        equals('Product Lifecycle Analysis'),
      );
      expect(
        SavingsFormatter.provenanceDisplayName('weight_material_carbon'),
        equals('Weight \u00d7 Material Factor'),
      );
      expect(
        SavingsFormatter.provenanceDisplayName('category_average_carbon'),
        equals('Category Average'),
      );
      expect(
        SavingsFormatter.provenanceDisplayName('spend_based_carbon'),
        equals('Spend-Based Estimate'),
      );
      expect(
        SavingsFormatter.provenanceDisplayName('prevented_purchase'),
        equals('Prevented Purchase'),
      );
      expect(
        SavingsFormatter.provenanceDisplayName('service_value'),
        equals('Service Value'),
      );
      expect(
        SavingsFormatter.provenanceDisplayName('commercial_value'),
        equals('Commercial Value'),
      );
      expect(
        SavingsFormatter.provenanceDisplayName('gear_shopping_time'),
        equals('Research-Based Default'),
      );
      expect(
        SavingsFormatter.provenanceDisplayName('genai_time_estimate'),
        equals('AI Estimated'),
      );
    });

    test('returns null for unknown name', () {
      expect(
        SavingsFormatter.provenanceDisplayName(''),
        isNull,
      );
    });
  });

  group('SavingsFormatter.formatQualityTime', () {
    // Precise formatter used on receipts and detail modals: minutes under
    // 90, hours + leftover minutes at or above (#2724 — "237 mins"
    // must never headline a receipt).
    test('formats zero as "0 min"', () {
      expect(SavingsFormatter.formatQualityTime(0), equals('0 min'));
    });

    test('keeps sub-90-minute values in minutes', () {
      expect(SavingsFormatter.formatQualityTime(9), equals('9 min'));
      expect(SavingsFormatter.formatQualityTime(42), equals('42 min'));
      expect(SavingsFormatter.formatQualityTime(89), equals('89 min'));
    });

    test('rolls 90+ minutes over to hours with leftover minutes', () {
      expect(SavingsFormatter.formatQualityTime(90), equals('1 h 30 m'));
      expect(SavingsFormatter.formatQualityTime(237), equals('3 h 57 m'));
      expect(SavingsFormatter.formatQualityTime(1842), equals('30 h 42 m'));
    });

    test('omits the minutes part on exact hours', () {
      expect(SavingsFormatter.formatQualityTime(120), equals('2 h'));
      expect(SavingsFormatter.formatQualityTime(240), equals('4 h'));
    });

    test('rounds fractional minutes to nearest integer', () {
      expect(SavingsFormatter.formatQualityTime(9.4), equals('9 min'));
      expect(SavingsFormatter.formatQualityTime(9.6), equals('10 min'));
    });
  });

  group('SavingsFormatter.formatQualityTimeCompact', () {
    // Tile/chip formatter: same 90-minute rollover, but hours are rounded
    // for glanceability, "~"-prefixed when rounding hides real minutes.
    test('formats zero as "0 min"', () {
      expect(SavingsFormatter.formatQualityTimeCompact(0), equals('0 min'));
    });

    test('keeps sub-90-minute values in minutes', () {
      expect(SavingsFormatter.formatQualityTimeCompact(37), equals('37 min'));
      expect(SavingsFormatter.formatQualityTimeCompact(89), equals('89 min'));
    });

    test('renders exact hour and half-hour values without tilde', () {
      expect(SavingsFormatter.formatQualityTimeCompact(90), equals('1.5 h'));
      expect(SavingsFormatter.formatQualityTimeCompact(120), equals('2 h'));
      expect(SavingsFormatter.formatQualityTimeCompact(240), equals('4 h'));
    });

    test('approximates inexact values with a tilde', () {
      // The filmed events completion showed "237 mins" (#2724); the
      // narrative called it four hours.
      expect(SavingsFormatter.formatQualityTimeCompact(237), equals('~4 h'));
      expect(SavingsFormatter.formatQualityTimeCompact(100), equals('~1.7 h'));
    });

    test('rounds to whole hours at 10 h and above', () {
      expect(SavingsFormatter.formatQualityTimeCompact(600), equals('10 h'));
      expect(SavingsFormatter.formatQualityTimeCompact(1842), equals('~31 h'));
    });
  });

  group('SavingsFormatter QT in formatSavings', () {
    test('returns null qtSaved when no quality time data', () {
      final impact = _buildImpactEstimate();
      final result = SavingsFormatter.formatSavings(impact);
      expect(result!.qtSaved, isNull);
      expect(result.qtDetail, isNull);
    });

    test('returns qtSaved and qtDetail when quality time present', () {
      final impact = ImpactEstimate(
        moneySaved: MoneySavings(
          valueUsd: Estimate(mean: 50, stddev: 15),
        ),
        timeSaved: TimeSavings(
          minutes: Estimate(mean: 120, stddev: 36),
        ),
        emissionsPrevented: PreventedEmissions(
          manufactureAvoidedCarbon: CarbonEstimate(
            co2eGrams: Estimate(mean: 24000, stddev: 7200),
          ),
        ),
        qualityTime: QualityTimeEstimate(
          qualityTimeMinutes: Estimate(mean: 1842, stddev: 737),
          provenance: Provenance(
            source: ProvenanceSource.PROVENANCE_SOURCE_CONFIG_DEFAULT,
            name: 'quality_time_v1',
            reasoning: 'In-person gear handoff, acquaintance tie strength.',
            sources: ['social_connection_metrics_v1'],
          ),
        ),
      );
      final result = SavingsFormatter.formatSavings(impact);

      // Tiles/chips get the compact rounded form; the drill-down detail
      // keeps full hours + minutes precision (#2724).
      expect(result!.qtSaved, equals('~31 h'));
      expect(result.qtDetail, isNotNull);
      expect(result.qtDetail!.label, equals('QUALITY TIME'));
      expect(result.qtDetail!.displayValue, equals('30 h 42 m'));
      expect(result.qtDetail!.confidence, isNotNull);
      expect(result.qtDetail!.methodName, equals('Quality Time (Config Defaults)'));
      expect(result.qtDetail!.reasoning, equals('In-person gear handoff, acquaintance tie strength.'));
    });

    test('returns null qtSaved when qualityTimeMinutes is zero', () {
      final impact = ImpactEstimate(
        qualityTime: QualityTimeEstimate(
          qualityTimeMinutes: Estimate(mean: 0, stddev: 0),
        ),
      );
      final result = SavingsFormatter.formatSavings(impact);
      expect(result!.qtSaved, isNull);
      expect(result.qtDetail, isNull);
    });
  });

  group('SavingsFormatter.provenanceDisplayName QT names', () {
    test('maps quality_time_v1', () {
      expect(
        SavingsFormatter.provenanceDisplayName('quality_time_v1'),
        equals('Quality Time (Config Defaults)'),
      );
    });

    test('maps quality_time_llm', () {
      expect(
        SavingsFormatter.provenanceDisplayName('quality_time_llm'),
        equals('Quality Time (AI Enriched)'),
      );
    });
  });

  group('SavingsFormatter confidence calculation', () {
    test('high certainty gives high confidence', () {
      // stddev = 5, mean = 50 => confidence = 1.0 - 0.1 = 0.9
      final impact = _buildImpactEstimate(
        costMean: 50,
        costStddev: 5,
      );
      final result = SavingsFormatter.formatSavings(impact);
      expect(result!.costDetail!.confidence, closeTo(0.9, 0.01));
    });

    test('high uncertainty gives low confidence', () {
      // stddev = 45, mean = 50 => confidence = 1.0 - 0.9 = 0.1
      final impact = _buildImpactEstimate(
        costMean: 50,
        costStddev: 45,
      );
      final result = SavingsFormatter.formatSavings(impact);
      expect(result!.costDetail!.confidence, closeTo(0.1, 0.01));
    });

    test('clamps confidence to minimum 0.1', () {
      // stddev > mean => would be negative, clamped to 0.1
      final impact = _buildImpactEstimate(
        costMean: 50,
        costStddev: 100,
      );
      final result = SavingsFormatter.formatSavings(impact);
      expect(result!.costDetail!.confidence, equals(0.1));
    });

    test('zero stddev gives max confidence 0.99', () {
      final impact = _buildImpactEstimate(
        costMean: 50,
        costStddev: 0,
      );
      final result = SavingsFormatter.formatSavings(impact);
      expect(result!.costDetail!.confidence, equals(0.99));
    });
  });
}
