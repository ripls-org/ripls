import 'package:ripls/core/utils/drill_down/emissions_drill_down.dart';
import 'package:ripls/core/utils/drill_down/money_drill_down.dart';
import 'package:ripls/core/utils/drill_down/quality_time_drill_down.dart';
import 'package:ripls/core/utils/drill_down/time_drill_down.dart';
import 'package:ripls/data/gen/ripls/api/impact.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart';
import 'package:ripls/presentation/models/drill_down_data.dart';
import 'package:ripls/presentation/models/item_metric_data_base.dart';

/// DrillDownBuilder converts proto impact data into DrillDownData display
/// models.
///
/// Pure utility class (same pattern as SavingsFormatter). Takes proto data as
/// input and returns Freezed display models for the MetricDrillDownScreen
/// formula-based drill-down. Implementation is split by metric kind in the
/// drill_down/ subdirectory.
class DrillDownBuilder {
  // ─── Item-Level Builders ─────────────────────────────────────────────

  /// Builds a money saved drill-down for an item-level impact estimate.
  static DrillDownData? buildMoneySavedDrillDown(
    ImpactEstimate? impact, {
    ItemType itemType = ItemType.gear,
    String? itemName,
    String? sourceUrl,
  }) =>
      MoneyDrillDown.buildItem(
        impact,
        itemType: itemType,
        itemName: itemName,
        sourceUrl: sourceUrl,
      );

  /// Builds an emissions prevented drill-down for an item-level impact
  /// estimate.
  static DrillDownData? buildEmissionsPreventedDrillDown(
    ImpactEstimate? impact, {
    ItemType itemType = ItemType.gear,
    double? weightKg,
    String? material,
  }) =>
      EmissionsDrillDown.buildItem(
        impact,
        weightKg: weightKg,
        material: material,
      );

  /// Builds a time saved drill-down for an item-level impact estimate.
  static DrillDownData? buildTimeSavedDrillDown(
    ImpactEstimate? impact, {
    ItemType itemType = ItemType.gear,
  }) =>
      TimeDrillDown.buildItem(impact, itemType: itemType);

  /// Builds a quality time drill-down for an item-level impact estimate.
  static DrillDownData? buildQualityTimeDrillDown(
    ImpactEstimate? impact, {
    ItemType itemType = ItemType.gear,
  }) =>
      QualityTimeDrillDown.buildItem(impact);

  // ─── Community-Level Builders ────────────────────────────────────────

  /// Builds a community money saved drill-down.
  static DrillDownData? buildCommunityMoneySavedDrillDown(
    CommunityImpactMetrics? metrics,
    GetCommunityMetricDetailResponse? detail,
  ) =>
      MoneyDrillDown.buildCommunity(metrics, detail);

  /// Builds a community emissions prevented drill-down.
  static DrillDownData? buildCommunityEmissionsDrillDown(
    CommunityImpactMetrics? metrics,
    GetCommunityMetricDetailResponse? detail,
  ) =>
      EmissionsDrillDown.buildCommunity(metrics, detail);

  /// Builds a community time recovered drill-down.
  static DrillDownData? buildCommunityTimeDrillDown(
    CommunityImpactMetrics? metrics,
    GetCommunityMetricDetailResponse? detail,
  ) =>
      TimeDrillDown.buildCommunity(metrics, detail);

  /// Builds a community quality time drill-down.
  static DrillDownData? buildCommunityQualityTimeDrillDown(
    CommunityImpactMetrics? metrics,
    GetCommunityMetricDetailResponse? detail,
  ) =>
      QualityTimeDrillDown.buildCommunity(metrics, detail);

  // ─── User-Level Builders ─────────────────────────────────────────────
  
  
  
  }
