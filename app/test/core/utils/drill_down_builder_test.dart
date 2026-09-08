import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/drill_down_builder.dart';
import 'package:ripls/core/utils/value_helpers.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart';
import 'package:ripls/presentation/models/item_metric_data_base.dart';

void main() {
  group('DrillDownBuilder', () {
    // ─── buildMoneySavedDrillDown ────────────────────────────────────

    group('buildMoneySavedDrillDown', () {
      test('returns null when impact is null', () {
        expect(DrillDownBuilder.buildMoneySavedDrillDown(null), isNull);
      });

      test('returns null when moneySaved is empty', () {
        final impact = ImpactEstimate();
        expect(DrillDownBuilder.buildMoneySavedDrillDown(impact), isNull);
      });

      test('returns null when mean is zero', () {
        final impact = ImpactEstimate(
          moneySaved: MoneySavings(valueUsd: Estimate(mean: 0)),
        );
        expect(DrillDownBuilder.buildMoneySavedDrillDown(impact), isNull);
      });

      test('builds drill-down with formula tree', () {
        final impact = ImpactEstimate(
          moneySaved: MoneySavings(
            valueUsd: Estimate(mean: 75, stddev: 15),
            provenance: Provenance(
              name: 'genai_value_estimate',
              reasoning: 'AI estimated from product images.',
            ),
          ),
        );

        final result = DrillDownBuilder.buildMoneySavedDrillDown(impact);

        expect(result, isNotNull);
        expect(result!.metricName, 'Money Saved');
        expect(result.formattedValue, '\$75');
        expect(result.confidence, greaterThan(0));
        expect(result.formula.operator, '×');
        expect(result.formula.operands, hasLength(2));
        expect(result.formula.operands![0].label, 'Item Value');
        expect(result.formula.operands![1].label, 'Prevented Purchase Rate');
        expect(result.formula.operands![1].formattedValue, '50%');
        expect(result.references, hasLength(2));
      });

      test('does not surface raw provenance reasoning', () {
        final impact = ImpactEstimate(
          moneySaved: MoneySavings(
            valueUsd: Estimate(mean: 100),
            provenance: Provenance(reasoning: 'Loan: \$100 × 50% PPR'),
          ),
        );

        final result = DrillDownBuilder.buildMoneySavedDrillDown(impact);
        // Provenance reasoning is developer-facing; drill-down generates
        // its own user-facing explanation text.
        expect(result!.formula.operands![0].explanation,
            isNot(contains('PPR')));
      });

      test('adapts labels for request item type', () {
        final impact = ImpactEstimate(
          moneySaved: MoneySavings(
            valueUsd: Estimate(mean: 50),
          ),
        );

        final result = DrillDownBuilder.buildMoneySavedDrillDown(
          impact,
          itemType: ItemType.request,
        );
        expect(result!.formula.operands![0].label, 'Service Value');
      });

      test('adapts labels for experience item type', () {
        final impact = ImpactEstimate(
          moneySaved: MoneySavings(
            valueUsd: Estimate(mean: 50),
          ),
        );

        final result = DrillDownBuilder.buildMoneySavedDrillDown(
          impact,
          itemType: ItemType.experience,
        );
        expect(result!.formula.operands![0].label, 'Event Value');
      });

      test('includes reference IDs on prevented purchase rate node', () {
        final impact = ImpactEstimate(
          moneySaved: MoneySavings(
            valueUsd: Estimate(mean: 100),
          ),
        );

        final result = DrillDownBuilder.buildMoneySavedDrillDown(impact);
        final pprNode = result!.formula.operands![1];
        expect(pprNode.referenceIds, containsAll([1, 2]));
      });
    });

    // ─── buildEmissionsPreventedDrillDown ─────────────────────────────

    group('buildEmissionsPreventedDrillDown', () {
      test('returns null when impact is null', () {
        expect(
            DrillDownBuilder.buildEmissionsPreventedDrillDown(null), isNull);
      });

      test('returns null when emissions are zero', () {
        final impact = ImpactEstimate(
          emissionsPrevented: PreventedEmissions(
            manufactureAvoidedCarbon:
                CarbonEstimate(co2eGrams: Estimate(mean: 0)),
          ),
        );
        expect(
            DrillDownBuilder.buildEmissionsPreventedDrillDown(impact), isNull);
      });

      test('builds drill-down for weight-based carbon', () {
        final impact = ImpactEstimate(
          emissionsPrevented: PreventedEmissions(
            manufactureAvoidedCarbon:
                CarbonEstimate(co2eGrams: Estimate(mean: 5000, stddev: 1500)),
            provenance: Provenance(name: 'weight_material_carbon'),
          ),
        );

        final result = DrillDownBuilder.buildEmissionsPreventedDrillDown(
          impact,
          weightKg: 2.5,
          material: 'Steel',
        );

        expect(result, isNotNull);
        expect(result!.metricName, 'CO₂ Avoided');
        expect(result.references.length, greaterThanOrEqualTo(4));
      });

      test('builds drill-down for spend-based carbon', () {
        final impact = ImpactEstimate(
          emissionsPrevented: PreventedEmissions(
            manufactureAvoidedCarbon:
                CarbonEstimate(co2eGrams: Estimate(mean: 3000)),
            provenance: Provenance(name: 'spend_based_carbon'),
          ),
        );

        final result =
            DrillDownBuilder.buildEmissionsPreventedDrillDown(impact);
        expect(result, isNotNull);
        // Spend-based should reference Decarbon [5].
        expect(result!.formula.explanation, contains('EEIO'));
      });

      test('includes both manufacturing and waste nodes', () {
        final impact = ImpactEstimate(
          emissionsPrevented: PreventedEmissions(
            manufactureAvoidedCarbon:
                CarbonEstimate(co2eGrams: Estimate(mean: 3000)),
            wasteReducedCarbon:
                CarbonEstimate(co2eGrams: Estimate(mean: 1000)),
          ),
        );

        final result =
            DrillDownBuilder.buildEmissionsPreventedDrillDown(impact);
        expect(result!.formula.operator, '+');
        expect(result.formula.operands, hasLength(2));
        expect(result.formula.operands![0].label, 'Embodied Carbon');
        expect(result.formula.operands![1].label, 'Waste Diverted');
      });
    });

    // ─── buildTimeSavedDrillDown ──────────────────────────────────────

    group('buildTimeSavedDrillDown', () {
      test('returns null when impact is null', () {
        expect(DrillDownBuilder.buildTimeSavedDrillDown(null), isNull);
      });

      test('returns null when time is zero', () {
        final impact = ImpactEstimate(
          timeSaved: TimeSavings(minutes: Estimate(mean: 0)),
        );
        expect(DrillDownBuilder.buildTimeSavedDrillDown(impact), isNull);
      });

      test('builds drill-down with default time', () {
        final impact = ImpactEstimate(
          timeSaved: TimeSavings(
            minutes: Estimate(mean: 120, stddev: 60),
          ),
        );

        final result = DrillDownBuilder.buildTimeSavedDrillDown(impact);

        expect(result, isNotNull);
        expect(result!.metricName, 'Time Recovered');
        expect(result.formattedValue, '2h');
        expect(result.references, hasLength(1));
        expect(result.references.first.id, 1);
      });

      test('includes explanation for request item type', () {
        final impact = ImpactEstimate(
          timeSaved: TimeSavings(minutes: Estimate(mean: 120)),
        );

        final result = DrillDownBuilder.buildTimeSavedDrillDown(
          impact,
          itemType: ItemType.request,
        );
        expect(result!.formula.explanation, contains('coordinate'));
      });
    });

    // ─── buildQualityTimeDrillDown ────────────────────────────────────

    group('buildQualityTimeDrillDown', () {
      test('returns null when impact is null', () {
        expect(DrillDownBuilder.buildQualityTimeDrillDown(null), isNull);
      });

      test('returns null when QT is zero', () {
        final impact = ImpactEstimate(
          qualityTime: QualityTimeEstimate(
            qualityTimeMinutes: Estimate(mean: 0),
          ),
        );
        expect(DrillDownBuilder.buildQualityTimeDrillDown(impact), isNull);
      });

      test('builds drill-down with all 7 factors when attributes present',
          () {
        final impact = ImpactEstimate(
          qualityTime: QualityTimeEstimate(
            qualityTimeMinutes: Estimate(mean: 150, stddev: 30),
            attributes: QualityTimeAttributes(
              estimatedDurationMinutes: 15,
              modality: SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF,
              groupSize: 2,
              tieStrength: SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW,
              reciprocity: SocialReciprocity.SOCIAL_RECIPROCITY_GIVING,
              novelty: SocialNovelty.SOCIAL_NOVELTY_NOVEL,
              vulnerability:
                  SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM,
            ),
          ),
        );

        final result = DrillDownBuilder.buildQualityTimeDrillDown(impact);

        expect(result, isNotNull);
        expect(result!.metricName, 'Quality Time');
        expect(result.formula.operator, '×');
        expect(result.formula.operands, hasLength(7));

        final labels =
            result.formula.operands!.map((n) => n.label).toList();
        expect(labels, [
          'Duration',
          'Modality',
          'Group Size',
          'Tie Strength',
          'Reciprocity',
          'Novelty',
          'Vulnerability',
        ]);
      });

      test('builds fallback when attributes not present', () {
        final impact = ImpactEstimate(
          qualityTime: QualityTimeEstimate(
            qualityTimeMinutes: Estimate(mean: 50),
          ),
        );

        final result = DrillDownBuilder.buildQualityTimeDrillDown(impact);
        expect(result, isNotNull);
        expect(result!.formula.operands, hasLength(1));
        expect(result.formula.operands![0].label, 'Factors');
      });

      test('includes quality time research references', () {
        final impact = ImpactEstimate(
          qualityTime: QualityTimeEstimate(
            qualityTimeMinutes: Estimate(mean: 50),
          ),
        );

        final result = DrillDownBuilder.buildQualityTimeDrillDown(impact);
        expect(result!.references, hasLength(4));
        expect(
          result.references.map((r) => r.id),
          containsAll([1, 2, 3, 4]),
        );
      });
    });

    // ─── Community-Level Builders ─────────────────────────────────────

    group('buildCommunityMoneySavedDrillDown', () {
      test('returns null when metrics is null', () {
        expect(
          DrillDownBuilder.buildCommunityMoneySavedDrillDown(null, null),
          isNull,
        );
      });

      test('returns null when cost savings is zero', () {
        final metrics = CommunityImpactMetrics(
          costSavingsUsd: Estimate(mean: 0),
        );
        expect(
          DrillDownBuilder.buildCommunityMoneySavedDrillDown(metrics, null),
          isNull,
        );
      });

      test('builds community drill-down with aggregate insight', () {
        final metrics = CommunityImpactMetrics(
          costSavingsUsd: Estimate(mean: 4230, stddev: 800),
          costSavingsCount: 70,
        );

        // The server sends the typed source and the raw value; the client
        // keys and formats the breakdown itself (#2835).
        final detail = GetCommunityMetricDetailResponse(
          sourceBreakdown: [
            SourceBreakdown(
              sourceType: ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,
              value: 2100,
            ),
            SourceBreakdown(
              sourceType: ImpactSourceType.IMPACT_SOURCE_TYPE_GIVEAWAYS,
              value: 800,
            ),
          ],
          topItems: [
            TopItem(name: 'Power Drill'),
          ],
        );

        final result = DrillDownBuilder.buildCommunityMoneySavedDrillDown(
            metrics, detail);

        expect(result, isNotNull);
        expect(result!.metricName, 'Total Savings');
        expect(result.formula.operator, '×');
        expect(result.formula.aggregateInsight, isNotNull);
        expect(result.formula.aggregateInsight!.itemCount, 70);
        expect(
            result.formula.aggregateInsight!.topContributorName, 'Power Drill');
        expect(result.formula.aggregateInsight!.breakdownByType, isNotNull);
        expect(
            result.formula.aggregateInsight!
                .breakdownByType!['IMPACT_SOURCE_TYPE_LOANS'],
            ValueHelpers.formatSavingsMoneyUsd(2100));
      });
    });

    group('buildCommunityEmissionsDrillDown', () {
      test('builds community emissions drill-down', () {
        final metrics = CommunityImpactMetrics(
          carbonSavingsGrams: Estimate(mean: 50000, stddev: 10000),
          carbonSavingsCount: 30,
        );

        final result =
            DrillDownBuilder.buildCommunityEmissionsDrillDown(metrics, null);

        expect(result, isNotNull);
        expect(result!.metricName, 'CO₂ Avoided');
        expect(result.formula.aggregateInsight!.itemCount, 30);
      });
    });

    group('buildCommunityTimeDrillDown', () {
      test('builds community time drill-down with breakdown', () {
        final metrics = CommunityImpactMetrics(
          timeBankedMinutes: Estimate(mean: 14400, stddev: 2000),
          timeFromLoansMinutes: Estimate(mean: 7200),
          timeFromRequestsMinutes: Estimate(mean: 3600),
          timeFromSkillsMinutes: Estimate(mean: 3600),
        );

        final result =
            DrillDownBuilder.buildCommunityTimeDrillDown(metrics, null);

        expect(result, isNotNull);
        expect(result!.metricName, 'Time Recovered');
        expect(result.formula.operands, hasLength(3));
        expect(result.formula.operands![0].label, 'Loans');
        expect(result.formula.operands![1].label, 'Requests');
        expect(result.formula.operands![2].label, 'Events');
      });
    });

    group('buildCommunityQualityTimeDrillDown', () {
      test('builds community QT drill-down with activity count prefix', () {
        final metrics = CommunityImpactMetrics(
          qualityTimeMinutes: Estimate(mean: 5000, stddev: 500),
          memberCount: 25,
          completedLoans: 10,
          completedGiveaways: 5,
          fulfilledRequests: 3,
          pastEvents: 2,
        );

        final result = DrillDownBuilder.buildCommunityQualityTimeDrillDown(
            metrics, null);

        expect(result, isNotNull);
        expect(result!.metricName, 'Quality Time');
        expect(result.formula.prefix, 'Sum of 20 activities');
        expect(result.formula.aggregateInsight, isNull);
      });
    });
  });
}
