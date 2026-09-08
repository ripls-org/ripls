import 'package:freezed_annotation/freezed_annotation.dart';

part 'drill_down_data.freezed.dart';

/// DrillDownData represents the complete data for a metric drill-down screen.
@freezed
sealed class DrillDownData with _$DrillDownData {
  const factory DrillDownData({
    required String metricName, // "Money Saved", "CO₂ Avoided", etc.
    required String formattedValue, // "$75", "4.1 kg", "2 hrs", "842 QT"
    required double confidence,
    required FormulaNode formula, // Top-level formula
    required List<ResearchReference> references,
  }) = _DrillDownData;
}

/// FormulaNode represents a node in the formula tree (recursive).
@freezed
sealed class FormulaNode with _$FormulaNode {
  const factory FormulaNode({
    required String label, // "Item Value", "Prevented Purchase Rate"
    required String formattedValue, // "$150", "50%"
    String? operator, // "×", "+", "Σ", null for leaf
    String? prefix, // "Σ 30 items" — rendered before operands
    List<FormulaNode>? operands, // Sub-components (recursive)
    String? explanation, // "AI analyzed photos to estimate..."
    double? confidence, // Optional per-component confidence
    List<int>? referenceIds, // [1, 2] — forward references
    AggregateInsight? aggregateInsight, // Community-level insights
  }) = _FormulaNode;
}

/// AggregateInsight holds community-level aggregate information.
@freezed
sealed class AggregateInsight with _$AggregateInsight {
  const factory AggregateInsight({
    int? itemCount,
    String? medianValue,
    String? topContributorName,
    String? topContributorValue,
    Map<String, String>? breakdownByType,
    String? distributionNote,
  }) = _AggregateInsight;
}

/// ResearchReference is a numbered citation at the bottom of the drill-down.
@freezed
sealed class ResearchReference with _$ResearchReference {
  const factory ResearchReference({
    required int id, // [1], [2], ...
    required String citation, // "Library of Things Impact Methodology"
    required String supportLevel, // "Direct", "Directional", "Conceptual"
    required String usedFor, // "Prevented purchase rate"
  }) = _ResearchReference;
}
