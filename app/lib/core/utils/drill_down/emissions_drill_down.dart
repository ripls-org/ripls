import 'package:ripls/core/utils/drill_down/drill_down_references.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/core/utils/value_helpers.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart';
import 'package:ripls/presentation/models/drill_down_data.dart';

/// EmissionsDrillDown builds emissions-prevented drill-down data for item,
/// community, and user metric contexts.
class EmissionsDrillDown {
  /// buildItem constructs an emissions-prevented drill-down for an item-level
  /// impact estimate.
  static DrillDownData? buildItem(
    ImpactEstimate? impact, {
    double? weightKg,
    String? material,
  }) {
    if (impact == null || !impact.hasEmissionsPrevented()) return null;

    final ep = impact.emissionsPrevented;
    final mfgGrams = ep.hasManufactureAvoidedCarbon()
        ? ep.manufactureAvoidedCarbon.co2eGrams.mean
        : 0.0;
    final wasteGrams = ep.hasWasteReducedCarbon()
        ? ep.wasteReducedCarbon.co2eGrams.mean
        : 0.0;
    final totalGrams = mfgGrams + wasteGrams;
    if (totalGrams <= 0) return null;

    final p = ep.hasProvenance() ? ep.provenance : Provenance();
    final mfgStddev = ep.hasManufactureAvoidedCarbon()
        ? ep.manufactureAvoidedCarbon.co2eGrams.stddev
        : 0.0;
    final confidence =
        SavingsFormatter.calculateConfidence(totalGrams, mfgStddev);

    final operands = <FormulaNode>[];

    if (mfgGrams > 0) {
      operands.add(_buildEmbodiedCarbonNode(
        mfgGrams,
        p,
        weightKg: weightKg,
        material: material,
      ));
    }

    if (wasteGrams > 0) {
      operands.add(_buildWasteDivertedNode(
        wasteGrams,
        weightKg: weightKg,
      ));
    }

    return DrillDownData(
      metricName: 'CO₂ Avoided',
      formattedValue: ValueHelpers.formatCO2Grams(totalGrams),
      confidence: confidence,
      formula: FormulaNode(
        label: 'CO₂ Avoided',
        formattedValue: ValueHelpers.formatCO2Grams(totalGrams),
        operator: operands.length > 1 ? '+' : null,
        operands: operands.length > 1 ? operands : null,
        explanation: operands.length == 1 ? operands.first.explanation : null,
        referenceIds:
            operands.length == 1 ? operands.first.referenceIds : null,
      ),
      references: emissionsReferences,
    );
  }

  /// buildCommunity constructs a community-level emissions drill-down.
  static DrillDownData? buildCommunity(
    CommunityImpactMetrics? metrics,
    GetCommunityMetricDetailResponse? detail,
  ) {
    if (metrics == null || !metrics.hasCarbonSavingsGrams()) return null;
    final mean = metrics.carbonSavingsGrams.mean;
    if (mean <= 0) return null;

    final confidence = SavingsFormatter.calculateConfidence(
        mean, metrics.carbonSavingsGrams.stddev);

    final insight = _buildAggregateInsight(metrics, detail);
    final formattedTotal = ValueHelpers.formatCO2GramsAsKg(mean);

    return DrillDownData(
      metricName: 'CO₂ Avoided',
      formattedValue: formattedTotal,
      confidence: confidence,
      formula: FormulaNode(
        label: 'CO₂ Avoided',
        formattedValue: formattedTotal,
        prefix: 'Σ ${metrics.carbonSavingsCount} items',
        operator: '+',
        explanation:
            'Sum across ${metrics.carbonSavingsCount} items, each contributing '
            'embodied carbon + waste diversion savings.',
        operands: [
          FormulaNode(
            label: 'Embodied Carbon',
            formattedValue: '(per item)',
            explanation:
                'Manufacturing emissions estimated from weight × material '
                'emission factor, or from spend-based EEIO factors as fallback.',
            referenceIds: const [1, 2, 3],
          ),
          const FormulaNode(
            label: 'Waste Diverted',
            formattedValue: '(per item)',
            explanation:
                'Estimated weight × 1.0 kg CO₂e/kg landfill disposal factor.',
            referenceIds: [4],
          ),
        ],
        aggregateInsight: insight,
      ),
      references: emissionsReferences,
    );
  }

  /// buildUser constructs a user-level emissions drill-down.
  static DrillDownData? buildUser(
    UserImpactMetrics? metrics,
  ) {
    if (metrics == null || !metrics.hasCarbonSavingsGrams()) return null;
    final mean = metrics.carbonSavingsGrams.mean;
    if (mean <= 0) return null;

    final confidence = SavingsFormatter.calculateConfidence(
        mean, metrics.carbonSavingsGrams.stddev);

    final formattedTotal = ValueHelpers.formatCO2GramsAsKg(mean);

    return DrillDownData(
      metricName: 'CO₂ Avoided',
      formattedValue: formattedTotal,
      confidence: confidence,
      formula: FormulaNode(
        label: 'CO₂ Avoided',
        formattedValue: formattedTotal,
        prefix: 'Σ your items',
        operator: '+',
        explanation:
            'Sum of emissions avoided across items you\'ve shared.',
        operands: const [
          FormulaNode(
            label: 'Embodied Carbon',
            formattedValue: '(per item)',
            explanation:
                'Manufacturing emissions estimated from weight × material '
                'emission factor, or from spend-based EEIO factors as fallback.',
            referenceIds: [1, 2, 3],
          ),
          FormulaNode(
            label: 'Waste Diverted',
            formattedValue: '(per item)',
            explanation:
                'Estimated weight × 1.0 kg CO₂e/kg landfill disposal factor.',
            referenceIds: [4],
          ),
        ],
      ),
      references: emissionsReferences,
    );
  }

  // ─── Private Helpers ─────────────────────────────────────────────────

  static FormulaNode _buildEmbodiedCarbonNode(
    double mfgGrams,
    Provenance p, {
    double? weightKg,
    String? material,
  }) {
    final provName = p.name;
    final isWeightBased = provName == 'weight_material_carbon';
    final isSpendBased = provName == 'spend_based_carbon';

    if (isWeightBased && weightKg != null) {
      final materialStr = material ?? 'estimated material';
      final factorPerKg = weightKg > 0 ? mfgGrams / 1000.0 / weightKg : 0.0;
      return FormulaNode(
        label: 'Embodied Carbon',
        formattedValue: ValueHelpers.formatCO2Grams(mfgGrams),
        explanation: 'Carbon emissions from manufacturing this item.',
        operator: '×',
        operands: [
          FormulaNode(
            label: 'Estimated Weight',
            formattedValue: '${weightKg.toStringAsFixed(1)} kg',
            explanation: 'Item weight estimated from AI analysis.',
          ),
          FormulaNode(
            label: 'Material Factor',
            formattedValue: '${factorPerKg.toStringAsFixed(2)} kg CO₂e/kg',
            explanation: 'Emission factor for $materialStr.',
            referenceIds: const [1, 2],
          ),
        ],
        referenceIds: const [1, 2],
      );
    } else if (isSpendBased) {
      return FormulaNode(
        label: 'Embodied Carbon',
        formattedValue: ValueHelpers.formatCO2Grams(mfgGrams),
        explanation:
            'Embodied carbon estimated from item value × spend-based '
            'emission factor (EEIO method).',
        referenceIds: const [3],
      );
    } else {
      return FormulaNode(
        label: 'Embodied Carbon',
        formattedValue: ValueHelpers.formatCO2Grams(mfgGrams),
        explanation: 'Embodied carbon estimated from item characteristics.',
        referenceIds: const [1, 2, 3],
      );
    }
  }

  static FormulaNode _buildWasteDivertedNode(
    double wasteGrams, {
    double? weightKg,
  }) {
    if (weightKg != null && weightKg > 0) {
      return FormulaNode(
        label: 'Waste Diverted',
        formattedValue: ValueHelpers.formatCO2Grams(wasteGrams),
        explanation: 'Emissions avoided by diverting the item from landfill.',
        operator: '×',
        operands: [
          FormulaNode(
            label: 'Estimated Weight',
            formattedValue: '${weightKg.toStringAsFixed(1)} kg',
            explanation: 'Item weight estimated from AI analysis.',
          ),
          const FormulaNode(
            label: 'Waste Factor',
            formattedValue: '1.0 kg CO₂e/kg',
            explanation: 'Carbon intensity of landfill disposal.',
            referenceIds: [4],
          ),
        ],
        referenceIds: const [4],
      );
    }
    return FormulaNode(
      label: 'Waste Diverted',
      formattedValue: ValueHelpers.formatCO2Grams(wasteGrams),
      explanation: 'Emissions avoided by diverting the item from landfill.',
      referenceIds: const [4],
    );
  }

  static AggregateInsight? _buildAggregateInsight(
    CommunityImpactMetrics metrics,
    GetCommunityMetricDetailResponse? detail,
  ) {
    // The top item's cumulative value is no longer on the wire — TopItem
    // carries identity and counts, not rendered emissions (#2835).
    String? topName;
    if (detail != null && detail.topItems.isNotEmpty) {
      topName = detail.topItems.first.name;
    }

    return AggregateInsight(
      itemCount: metrics.carbonSavingsCount,
      topContributorName: topName,
    );
  }
}
