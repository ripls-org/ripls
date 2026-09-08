import 'package:ripls/core/utils/drill_down/drill_down_references.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/core/utils/value_helpers.dart';
import 'package:ripls/data/gen/ripls/api/impact.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart';
import 'package:ripls/presentation/models/drill_down_data.dart';
import 'package:ripls/presentation/models/item_metric_data_base.dart';

/// TimeDrillDown builds time-saved drill-down data for item, community,
/// and user metric contexts.
class TimeDrillDown {
  /// buildItem constructs a time-saved drill-down for an item-level impact
  /// estimate.
  static DrillDownData? buildItem(
    ImpactEstimate? impact, {
    ItemType itemType = ItemType.gear,
  }) {
    if (impact == null ||
        !impact.hasTimeSaved() ||
        !impact.timeSaved.hasMinutes()) {
      return null;
    }

    final ts = impact.timeSaved;
    final mean = ts.minutes.mean;
    if (mean <= 0) return null;

    final confidence =
        SavingsFormatter.calculateConfidence(mean, ts.minutes.stddev);

    return DrillDownData(
      metricName: 'Time Recovered',
      formattedValue: ValueHelpers.formatTimeMinutes(mean),
      confidence: confidence,
      formula: FormulaNode(
        label: 'Time Recovered',
        formattedValue: ValueHelpers.formatTimeMinutes(mean),
        explanation: _defaultExplanation(itemType),
        referenceIds: const [1],
      ),
      references: timeSavedReferences,
    );
  }

  /// buildCommunity constructs a community-level time-saved drill-down.
  static DrillDownData? buildCommunity(
    CommunityImpactMetrics? metrics,
    GetCommunityMetricDetailResponse? detail,
  ) {
    if (metrics == null || !metrics.hasTimeBankedMinutes()) return null;
    final mean = metrics.timeBankedMinutes.mean;
    if (mean <= 0) return null;

    final confidence = SavingsFormatter.calculateConfidence(
        mean, metrics.timeBankedMinutes.stddev);

    final insight = _buildAggregateInsight(metrics, detail);

    return DrillDownData(
      metricName: 'Time Recovered',
      formattedValue: ValueHelpers.formatTimeMinutes(mean),
      confidence: confidence,
      formula: FormulaNode(
        label: 'Time Recovered',
        formattedValue: ValueHelpers.formatTimeMinutes(mean),
        operator: '+',
        explanation:
            'Total time saved across all transactions. '
            'Gear: shopping time avoided. '
            'Requests: labor time saved. '
            'Events: scaled by attendee count.',
        operands: [
          if (metrics.hasTimeFromLoansMinutes() &&
              metrics.timeFromLoansMinutes.mean > 0)
            FormulaNode(
              label: 'Loans',
              formattedValue: ValueHelpers.formatTimeMinutes(
                  metrics.timeFromLoansMinutes.mean),
              explanation:
                  'Default assumed shopping and research time avoided '
                  'by borrowing gear.',
              referenceIds: const [1],
            ),
          if (metrics.hasTimeFromRequestsMinutes() &&
              metrics.timeFromRequestsMinutes.mean > 0)
            FormulaNode(
              label: 'Requests',
              formattedValue: ValueHelpers.formatTimeMinutes(
                  metrics.timeFromRequestsMinutes.mean),
              explanation:
                  'Default assumed time saved by getting help from community.',
              referenceIds: const [1],
            ),
          if (metrics.hasTimeFromSkillsMinutes() &&
              metrics.timeFromSkillsMinutes.mean > 0)
            FormulaNode(
              label: 'Events',
              formattedValue: ValueHelpers.formatTimeMinutes(
                  metrics.timeFromSkillsMinutes.mean),
              explanation: 'Time from shared events.',
              referenceIds: const [1],
            ),
        ],
        aggregateInsight: insight,
      ),
      references: timeSavedReferences,
    );
  }

  /// buildUser constructs a user-level time-saved drill-down.
  static DrillDownData? buildUser(
    UserImpactMetrics? metrics,
  ) {
    if (metrics == null || !metrics.hasTimeBankedMinutes()) return null;
    final mean = metrics.timeBankedMinutes.mean;
    if (mean <= 0) return null;

    final confidence = SavingsFormatter.calculateConfidence(
        mean, metrics.timeBankedMinutes.stddev);

    return DrillDownData(
      metricName: 'Time Recovered',
      formattedValue: ValueHelpers.formatTimeMinutes(mean),
      confidence: confidence,
      formula: FormulaNode(
        label: 'Time Recovered',
        formattedValue: ValueHelpers.formatTimeMinutes(mean),
        explanation:
            'Total time saved across all your sharing transactions. '
            'Default assumed 120 minutes per transaction: time to research, '
            'shop for, and acquire similar items.',
        referenceIds: const [1],
      ),
      references: timeSavedReferences,
    );
  }

  // ─── Private Helpers ─────────────────────────────────────────────────

  static String _defaultExplanation(ItemType itemType) {
    switch (itemType) {
      case ItemType.gear:
        return 'Default assumed shopping and research time to find, '
            'travel to, and acquire this type of item.';
      case ItemType.request:
        return 'Default assumed time to find, contact, and coordinate '
            'professional help for this type of task.';
      case ItemType.experience:
        return 'Default assumed duration of a comparable experience or '
            'event. Scaled by attendee count for community impact.';
    }
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
              ValueHelpers.formatTimeMinutes(sb.value);
        }
      }
    }

    return AggregateInsight(
      breakdownByType: breakdown.isNotEmpty ? breakdown : null,
    );
  }
}
