import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/viewmodels/experience_metric_view_model.dart';
import 'package:ripls/presentation/viewmodels/gear_metric_view_model.dart'
    show GearMetricData, LoanSocialAttributeDisplay;
import 'package:ripls/presentation/viewmodels/request_metric_view_model.dart';
import 'package:ripls/presentation/widgets/item/item_metric_data.dart';

/// ItemType enum for distinguishing between gear, request, and experience items.
enum ItemType {
  gear,
  request,
  experience,
}

/// Sealed class serving as a common interface for all item metric data types.
///
/// This abstraction allows ItemMetricsScreen to work with gear, requests,
/// and experiences using a single implementation. The sealed class pattern
/// ensures exhaustive handling of all three types.
sealed class ItemMetricDataBase {
  /// Unique identifier for the item
  String get itemId;

  /// Item type (gear, request, or experience)
  ItemType get itemType;

  /// Display name of the item
  String get itemName;

  /// Description text
  String get description;

  /// Owner's display name
  String get ownerName;

  /// Display data (media, metrics, savings)
  ItemMetricDisplayData get displayData;

  /// People associated with the item (owners, borrowers, helpers, etc.)
  List<PersonWithRole> get people;

  /// AI-detected metadata metrics (category, brand, material, etc.)
  List<ItemMetricValue> get metadataMetrics;

  /// Social context attributes for the baseline interaction (duration, modality, etc.)
  LoanSocialAttributeDisplay? get socialAttributes;

  /// Whether the item has any transactions (loans > 0, fulfillments > 0, sessions > 0)
  bool get hasTransactions;

  /// Count of transactions (timesLoaned, timesFulfilled, sessionsHeld)
  int get transactionCount;

  /// Label for transaction type ("LOANS", "FULFILLMENTS", "SESSIONS")
  String get transactionLabel;

  /// Label for transaction type in singular form ("LOAN", "FULFILLMENT", "SESSION")
  String get transactionLabelSingular;

  /// Per-action impact estimate (what a single transaction saves).
  ImpactEstimate? get perActionImpact;

  /// Cumulative impact estimate across all transactions.
  ImpactEstimate? get cumulativeImpact;
}

/// GearItemMetricData wraps GearMetricData to conform to ItemMetricDataBase.
class GearItemMetricData implements ItemMetricDataBase {
  final GearMetricData data;

  const GearItemMetricData(this.data);

  @override
  String get itemId => data.gearId;

  @override
  ItemType get itemType => ItemType.gear;

  @override
  String get itemName => data.itemName;

  @override
  String get description => data.description;

  @override
  String get ownerName => data.ownerName;

  @override
  ItemMetricDisplayData get displayData => data.displayData;

  @override
  List<PersonWithRole> get people => data.people;

  @override
  List<ItemMetricValue> get metadataMetrics => data.metadataMetrics;

  @override
  LoanSocialAttributeDisplay? get socialAttributes => data.loanSocialAttributes;

  @override
  bool get hasTransactions => data.stats.timesLoaned > 0;

  @override
  int get transactionCount => data.stats.timesLoaned;

  @override
  String get transactionLabel => 'LOANS';

  @override
  String get transactionLabelSingular => 'LOAN';

  @override
  ImpactEstimate? get perActionImpact =>
      data.stats.hasPotentialImpact() ? data.stats.potentialImpact : null;

  @override
  ImpactEstimate? get cumulativeImpact =>
      data.stats.hasImpact() ? data.stats.impact : null;

  /// Weight in kg for drill-down sub-formula display.
  double? get weightKg => data.weightKg;

  /// Material display name for drill-down sub-formula display.
  String? get materialName => data.materialName;
}

/// RequestItemMetricData wraps RequestMetricData to conform to ItemMetricDataBase.
class RequestItemMetricData implements ItemMetricDataBase {
  final RequestMetricData data;

  const RequestItemMetricData(this.data);

  @override
  String get itemId => data.requestId;

  @override
  ItemType get itemType => ItemType.request;

  @override
  String get itemName => data.itemName;

  @override
  String get description => data.description;

  @override
  String get ownerName => data.ownerName;

  @override
  ItemMetricDisplayData get displayData => data.displayData;

  @override
  List<PersonWithRole> get people => data.people;

  @override
  List<ItemMetricValue> get metadataMetrics => data.metadataMetrics;

  @override
  LoanSocialAttributeDisplay? get socialAttributes => data.socialAttributes;

  @override
  bool get hasTransactions => data.stats.timesFulfilled > 0;

  @override
  int get transactionCount => data.stats.timesFulfilled;

  @override
  String get transactionLabel => 'FULFILLMENTS';

  @override
  String get transactionLabelSingular => 'FULFILLMENT';

  @override
  ImpactEstimate? get perActionImpact =>
      data.stats.hasPotentialImpact() ? data.stats.potentialImpact : null;

  @override
  ImpactEstimate? get cumulativeImpact =>
      data.stats.hasImpact() ? data.stats.impact : null;
}

/// ExperienceItemMetricData wraps ExperienceMetricData to conform to ItemMetricDataBase.
class ExperienceItemMetricData implements ItemMetricDataBase {
  final ExperienceMetricData data;

  const ExperienceItemMetricData(this.data);

  @override
  String get itemId => data.experienceId;

  @override
  ItemType get itemType => ItemType.experience;

  @override
  String get itemName => data.itemName;

  @override
  String get description => data.description;

  @override
  String get ownerName => data.ownerName;

  @override
  ItemMetricDisplayData get displayData => data.displayData;

  @override
  List<PersonWithRole> get people => data.people;

  @override
  List<ItemMetricValue> get metadataMetrics => data.metadataMetrics;

  @override
  LoanSocialAttributeDisplay? get socialAttributes => data.socialAttributes;

  @override
  bool get hasTransactions => data.stats.sessionsHeld > 0;

  @override
  int get transactionCount => data.stats.sessionsHeld;

  @override
  String get transactionLabel => 'SESSIONS';

  @override
  String get transactionLabelSingular => 'SESSION';

  @override
  ImpactEstimate? get perActionImpact =>
      data.stats.hasPotentialImpact() ? data.stats.potentialImpact : null;

  @override
  ImpactEstimate? get cumulativeImpact =>
      data.stats.hasImpact() ? data.stats.impact : null;
}
