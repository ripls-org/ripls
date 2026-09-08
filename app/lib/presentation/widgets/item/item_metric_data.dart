import 'package:flutter/material.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';

part 'item_metric_data.freezed.dart';

/// Displayed in the score card (metrics over background image).
///
/// Contains all data needed to render the item metrics card.
/// Title and description are NOT included - they're shown in the AppBar header
/// via [ItemHeaderTitle] instead.
///
/// The card displays the item's background image with a dark gradient overlay,
/// and metrics rendered on top.
@freezed
sealed class ItemMetricDisplayData with _$ItemMetricDisplayData {
  const factory ItemMetricDisplayData({
    required String mediaId, // For cache key
    String? mediaUrl, // For background image display
    required List<ItemMetricValue> metrics, // Tappable metrics (value, CO2, stats)
    ItemSavingsData? savings, // Optional savings section
  }) = _ItemMetricDisplayData;
}

/// A single tappable metric with optional detail modal.
///
/// Represents a metric that can be tapped to show more details (e.g., value estimate).
/// If [confidence], [reasoning], or [sources] are provided, tapping the metric
/// will open a detail modal showing this additional information.
@freezed
sealed class ItemMetricValue with _$ItemMetricValue {
  const factory ItemMetricValue({
    required String label, // "ESTIMATED VALUE", "CO2 SAVED"
    required String displayValue, // "$149", "N/A"
    double? confidence, // For detail modal (0.0-1.0)
    String? reasoning, // For detail modal
    List<String>? sources, // For detail modal
    String? actionUrl, // Optional "View product page" link
    String? methodName, // e.g., "Prevented Purchase", "Weight × Material Factor"
    String? methodologyDocPath, // e.g., "money_saved.md" for "Learn more" link
  }) = _ItemMetricValue;
}

/// A single stat in the 2x2 grid.
///
/// Represents a non-tappable statistic displayed in the score card grid
/// (e.g., "TIMES LOANED: 7", "PEOPLE HELPED: 5").
@freezed
sealed class ItemStat with _$ItemStat {
  const factory ItemStat({
    required String label, // "TIMES LOANED", "PEOPLE HELPED"
    required String value, // "7", "5"
  }) = _ItemStat;
}

/// A person associated with an item, with their role and display styling.
///
/// Used in the people section to show users with their relationship to the item
/// (e.g., "Owner", "Borrowing", "RSVP'd"). The [sortOrder] determines display order,
/// with lower values appearing first.
@freezed
sealed class PersonWithRole with _$PersonWithRole {
  const factory PersonWithRole({
    required User user,
    required String roleLabel,
    required Color roleColor,
    required int sortOrder,
  }) = _PersonWithRole;
}

/// Savings metrics for an item (cost, time, CO2, quality time).
///
/// Displayed in a savings section below the main metrics grid, similar to
/// RipplesScoreCard on CommunityMetricsView. Shows the aggregate environmental,
/// economic, and social impact of sharing the item.
@freezed
sealed class ItemSavingsData with _$ItemSavingsData {
  const factory ItemSavingsData({
    required String costSaved, // "$1,043"
    required String timeSaved, // "14 hrs"
    required String co2Saved, // "5.2 kg"
    SavingsMetricDetail? costDetail, // For tappable drill-down
    SavingsMetricDetail? timeDetail, // For tappable drill-down
    SavingsMetricDetail? co2Detail, // For tappable drill-down
    String? qtSaved, // "~31 h" / "37 min" — null if no quality time data
    SavingsMetricDetail? qtDetail, // For tappable drill-down
  }) = _ItemSavingsData;
}

/// Detail data for a single savings metric dimension.
///
/// Contains all information needed to display a detail screen when the user
/// taps on an individual savings metric (money saved, time saved, CO2 avoided).
@freezed
sealed class SavingsMetricDetail with _$SavingsMetricDetail {
  const factory SavingsMetricDetail({
    required String label, // "SAVED", "RECOVERED", "CO₂ AVOIDED"
    required String displayValue, // "$50", "2h", "24kg"
    double? confidence, // 0.0-1.0 derived from stddev/mean
    String? methodName, // "Prevented Purchase", "Weight × Material Factor"
    String? reasoning, // "Loan: $25.00 × 50% prevented purchase rate."
    List<String>? sources, // ["Library of Things UK — prevented purchase rate"]
    String? methodologyDocPath, // "money_saved.md"
  }) = _SavingsMetricDetail;
}
