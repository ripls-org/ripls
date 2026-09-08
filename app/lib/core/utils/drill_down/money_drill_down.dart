import 'package:ripls/core/utils/drill_down/drill_down_references.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/core/utils/value_helpers.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart';
import 'package:ripls/presentation/models/drill_down_data.dart';
import 'package:ripls/presentation/models/item_metric_data_base.dart';

/// MoneyDrillDown builds money-saved drill-down data for item, community,
/// and user metric contexts.
class MoneyDrillDown {
  /// buildItem constructs a money-saved drill-down for an item-level impact
  /// estimate.
  static DrillDownData? buildItem(
    ImpactEstimate? impact, {
    ItemType itemType = ItemType.gear,
    String? itemName,
    String? sourceUrl,
  }) {
    if (impact == null ||
        !impact.hasMoneySaved() ||
        !impact.moneySaved.hasValueUsd()) {
      return null;
    }

    final ms = impact.moneySaved;
    final mean = ms.valueUsd.mean;
    if (mean <= 0) return null;

    final p = ms.hasProvenance() ? ms.provenance : Provenance();
    final confidence =
        SavingsFormatter.calculateConfidence(mean, ms.valueUsd.stddev);

    final itemValueExplanation = _itemValueExplanation(p, itemType, sourceUrl);
    final pprExplanation = _preventedPurchaseRateExplanation(itemType);

    return DrillDownData(
      metricName: 'Money Saved',
      formattedValue: ValueHelpers.formatSavingsMoneyUsd(mean),
      confidence: confidence,
      formula: FormulaNode(
        label: 'Money Saved',
        formattedValue: ValueHelpers.formatSavingsMoneyUsd(mean),
        operator: '×',
        operands: [
          FormulaNode(
            label: _itemValueLabel(itemType),
            formattedValue: ValueHelpers.formatSavingsMoneyUsd(mean / 0.5),
            explanation: itemValueExplanation,
            confidence: _provenanceConfidence(p),
            referenceIds: const [],
          ),
          FormulaNode(
            label: 'Prevented Purchase Rate',
            formattedValue: '50%',
            explanation: pprExplanation,
            referenceIds: const [1, 2],
          ),
        ],
      ),
      references: moneySavedReferences,
    );
  }

  /// buildCommunity constructs a community-level money-saved drill-down.
  static DrillDownData? buildCommunity(
    CommunityImpactMetrics? metrics,
    GetCommunityMetricDetailResponse? detail,
  ) {
    if (metrics == null || !metrics.hasCostSavingsUsd()) return null;
    final mean = metrics.costSavingsUsd.mean;
    if (mean <= 0) return null;

    final confidence = SavingsFormatter.calculateConfidence(
        mean, metrics.costSavingsUsd.stddev);

    final insight = _buildAggregateInsight(metrics, detail);

    return DrillDownData(
      metricName: 'Total Savings',
      formattedValue: ValueHelpers.formatSavingsMoneyUsd(mean),
      confidence: confidence,
      formula: FormulaNode(
        label: 'Total Savings',
        formattedValue: ValueHelpers.formatSavingsMoneyUsd(mean),
        operator: '×',
        explanation:
            'Aggregate savings across ${metrics.costSavingsCount} shared items.',
        operands: [
          FormulaNode(
            label: 'Total Item Value',
            formattedValue: ValueHelpers.formatSavingsMoneyUsd(mean / 0.5),
            explanation:
                'Sum of item values across all transactions, estimated '
                'from AI analysis, user input, or web scraping.',
          ),
          const FormulaNode(
            label: 'Prevented Purchase Rate',
            formattedValue: '50%',
            explanation:
                'Fraction of transactions that prevent a new purchase.',
            referenceIds: [1, 2],
          ),
        ],
        aggregateInsight: insight,
      ),
      references: moneySavedReferences,
    );
  }

  /// buildUser constructs a user-level money-saved drill-down.
  static DrillDownData? buildUser(
    UserImpactMetrics? metrics,
  ) {
    if (metrics == null || !metrics.hasCostSavingsUsd()) return null;
    final mean = metrics.costSavingsUsd.mean;
    if (mean <= 0) return null;

    final confidence = SavingsFormatter.calculateConfidence(
        mean, metrics.costSavingsUsd.stddev);

    return DrillDownData(
      metricName: 'Money Saved',
      formattedValue: ValueHelpers.formatSavingsMoneyUsd(mean),
      confidence: confidence,
      formula: FormulaNode(
        label: 'Money Saved',
        formattedValue: ValueHelpers.formatSavingsMoneyUsd(mean),
        operator: '×',
        explanation:
            'Savings across ${metrics.itemsShared} items you\'ve shared.',
        operands: [
          FormulaNode(
            label: 'Total Item Value',
            formattedValue: ValueHelpers.formatSavingsMoneyUsd(mean / 0.5),
            explanation:
                'Sum of item values across your transactions, estimated '
                'from AI analysis, user input, or web scraping.',
          ),
          const FormulaNode(
            label: 'Prevented Purchase Rate',
            formattedValue: '50%',
            explanation:
                'Fraction of transactions that prevent a new purchase.',
            referenceIds: [1, 2],
          ),
        ],
      ),
      references: moneySavedReferences,
    );
  }

  // ─── Private Helpers ─────────────────────────────────────────────────

  static String _itemValueLabel(ItemType itemType) {
    switch (itemType) {
      case ItemType.gear:
        return 'Item Value';
      case ItemType.request:
        return 'Service Value';
      case ItemType.experience:
        return 'Event Value';
    }
  }

  static String _itemValueExplanation(
    Provenance p,
    ItemType itemType,
    String? sourceUrl,
  ) {
    final provName = SavingsFormatter.provenanceDisplayName(p.name);
    final method = provName ?? 'AI analysis';

    switch (itemType) {
      case ItemType.gear:
        if (sourceUrl != null && sourceUrl.isNotEmpty) {
          return 'Retail value sourced from product page. Method: $method.';
        }
        return 'Estimated retail value based on $method of similar items.';
      case ItemType.request:
        return 'Estimated cost to hire a professional. Method: $method.';
      case ItemType.experience:
        return 'Estimated commercial event value. Method: $method.';
    }
  }

  static String _preventedPurchaseRateExplanation(ItemType itemType) {
    switch (itemType) {
      case ItemType.gear:
        return 'Fraction of borrowers who would have bought the item new.';
      case ItemType.request:
        return 'Fraction of help-seekers who would have hired a professional.';
      case ItemType.experience:
        return 'Fraction of attendees who would have attended a paid equivalent.';
    }
  }

  static double? _provenanceConfidence(Provenance p) {
    if (p.hasConfidence()) return p.confidence;
    return null;
  }

  static AggregateInsight? _buildAggregateInsight(
    CommunityImpactMetrics metrics,
    GetCommunityMetricDetailResponse? detail,
  ) {
    // Keyed by the typed source rather than a server-rendered label, and
    // formatted here rather than on the server (#2835).
    final breakdown = <String, String>{};
    if (detail != null) {
      for (final sb in detail.sourceBreakdown) {
        if (sb.value > 0) {
          breakdown[sb.sourceType.name] =
              ValueHelpers.formatSavingsMoneyUsd(sb.value);
        }
      }
    }

    // The top item's cumulative value is no longer on the wire — TopItem
    // carries identity and counts, not rendered money (#2835).
    String? topName;
    if (detail != null && detail.topItems.isNotEmpty) {
      topName = detail.topItems.first.name;
    }

    return AggregateInsight(
      itemCount: metrics.costSavingsCount,
      topContributorName: topName,
      breakdownByType: breakdown.isNotEmpty ? breakdown : null,
    );
  }
}
