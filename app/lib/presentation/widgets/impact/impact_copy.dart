import 'package:intl/intl.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart'
    show ImpactMetricDimension, ImpactSourceType, RecentActivity,
        RecentActivityKind;
import 'package:ripls/l10n/app_localizations.dart';

/// Locale-aware copy for the impact metric-detail surfaces (#2827): source
/// labels, recent-activity fallbacks, dimension-value formatting, and
/// month-bucket axis labels, all derived from the typed wire fields the
/// server sends instead of pre-rendered English strings.

/// impactSourceLabel is the display label for a source-breakdown segment.
String impactSourceLabel(AppLocalizations l10n, ImpactSourceType type) {
  switch (type) {
    case ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS:
      return l10n.metricDetailSourceLoans;
    case ImpactSourceType.IMPACT_SOURCE_TYPE_GIVEAWAYS:
      return l10n.metricDetailSourceGiveaways;
    case ImpactSourceType.IMPACT_SOURCE_TYPE_REQUESTS:
      return l10n.metricDetailSourceRequests;
    case ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS:
      return l10n.metricDetailSourceEvents;
    default:
      return '';
  }
}

/// recentActivityLabel is a recent-activity row's title: the item's own name
/// when it has one, else a localized fallback derived from the row's kind.
String recentActivityLabel(AppLocalizations l10n, RecentActivity item) {
  if (item.hasItemName() && item.itemName.isNotEmpty) return item.itemName;
  switch (item.kind) {
    case RecentActivityKind.RECENT_ACTIVITY_KIND_REQUEST:
      return l10n.metricDetailFallbackRequest;
    case RecentActivityKind.RECENT_ACTIVITY_KIND_EXPERIENCE:
      return l10n.metricDetailFallbackEvent;
    default:
      return l10n.metricDetailFallbackItem;
  }
}

/// impactValueText formats a raw dimension value with its localized unit:
/// compact currency for money, hours for time, kilograms for emissions,
/// minutes for quality time. Raw values arrive in the dimension's base unit
/// (dollars, minutes, kilograms, minutes respectively).
String impactValueText(
  AppLocalizations l10n,
  ImpactMetricDimension dimension,
  double value,
) {
  switch (dimension) {
    case ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY:
      return NumberFormat.compactSimpleCurrency(locale: l10n.localeName)
          .format(value);
    case ImpactMetricDimension.IMPACT_METRIC_DIMENSION_TIME:
      return l10n.metricDetailValueHrs((value / 60).toStringAsFixed(1));
    case ImpactMetricDimension.IMPACT_METRIC_DIMENSION_EMISSIONS:
      return l10n.metricDetailValueKg(value.toStringAsFixed(1));
    case ImpactMetricDimension.IMPACT_METRIC_DIMENSION_QUALITY_TIME:
      return l10n.metricDetailValueMin(value.toStringAsFixed(0));
    default:
      return '';
  }
}

/// impactMonthLabel is the chart-axis label for a month bucket, formatted in
/// the viewer's locale. Bucket starts are UTC month boundaries, so the
/// formatting stays in UTC to avoid drifting into the neighboring month.
String impactMonthLabel(AppLocalizations l10n, int bucketStartUnixSec) {
  final month = DateTime.fromMillisecondsSinceEpoch(
    bucketStartUnixSec * 1000,
    isUtc: true,
  );
  return DateFormat.MMM(l10n.localeName).format(month);
}
