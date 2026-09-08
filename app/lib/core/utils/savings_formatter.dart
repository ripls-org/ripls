import 'package:ripls/core/utils/value_helpers.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/widgets/item/item_metric_data.dart';

/// Utility class for formatting impact metrics from proto messages.
///
/// Delegates to ValueHelpers for consistent formatting across the app.
class SavingsFormatter {
  /// Converts ImpactEstimate proto to ItemSavingsData display model.
  ///
  /// Returns null if the impact proto is null, otherwise formats all fields
  /// using ValueHelpers for consistent display formatting. Also builds
  /// SavingsMetricDetail objects for per-metric drill-down.
  ///
  /// Unit conversions from proto:
  /// - Cost: valueUsd.mean (USD float) -> formatted dollar string
  /// - Time: minutes.mean (float minutes) -> formatted time string
  /// - Carbon: co2eGrams.mean (float grams) -> formatted CO2 string
  static ItemSavingsData? formatSavings(ImpactEstimate? impact) {
    if (impact == null) return null;

    final costUsd = impact.hasMoneySaved() && impact.moneySaved.hasValueUsd()
        ? impact.moneySaved.valueUsd.mean
        : 0.0;
    final timeMinutes = impact.hasTimeSaved() && impact.timeSaved.hasMinutes()
        ? impact.timeSaved.minutes.mean
        : 0.0;
    final ep = impact.hasEmissionsPrevented() ? impact.emissionsPrevented : null;
    final mfgGrams = ep != null && ep.hasManufactureAvoidedCarbon()
        ? ep.manufactureAvoidedCarbon.co2eGrams.mean
        : 0.0;
    final wasteGrams = ep != null && ep.hasWasteReducedCarbon()
        ? ep.wasteReducedCarbon.co2eGrams.mean
        : 0.0;
    final co2Grams = mfgGrams + wasteGrams;

    final qtMinutes = impact.hasQualityTime() && impact.qualityTime.hasQualityTimeMinutes()
        ? impact.qualityTime.qualityTimeMinutes.mean
        : 0.0;

    return ItemSavingsData(
      costSaved: ValueHelpers.formatSavingsMoneyUsd(costUsd),
      timeSaved: ValueHelpers.formatTimeMinutes(timeMinutes),
      co2Saved: ValueHelpers.formatCO2Grams(co2Grams),
      qtSaved: qtMinutes > 0 ? formatQualityTimeCompact(qtMinutes) : null,
      costDetail: _buildCostDetail(impact),
      timeDetail: _buildTimeDetail(impact),
      co2Detail: _buildCo2Detail(impact),
      qtDetail: _buildQualityTimeDetail(impact),
    );
  }

  /// Threshold at or above which a minutes value renders as hours instead
  /// of raw minutes ("90 min" reads worse than "1 h 30 m"; #2724).
  static const double _hourRolloverMinutes = 90;

  /// Formats a Quality Time value with full precision (e.g. "37 min",
  /// "3 h 57 m", "4 h").
  ///
  /// This is the canonical precise Quality Time formatter — receipts and
  /// drill-down detail surfaces (QT detail modal, metric sheets) go through
  /// this helper. Values under 90 minutes render as minutes; everything
  /// else as hours + leftover minutes so long durations stay legible
  /// ("237 mins" → "3 h 57 m"). Tiles and chips use the rounded
  /// [formatQualityTimeCompact] instead.
  static String formatQualityTime(double minutes) {
    final mins = minutes.round();
    if (mins < _hourRolloverMinutes) return '$mins min';
    final hours = mins ~/ 60;
    final rem = mins % 60;
    final formattedHours = hours.toString().replaceAllMapped(
      RegExp(r'(\d{1,3})(?=(\d{3})+(?!\d))'),
      (m) => '${m[1]},',
    );
    return rem == 0 ? '$formattedHours h' : '$formattedHours h $rem m';
  }

  /// Compact Quality Time formatter for tiles and chips (e.g. "37 min",
  /// "~4 h", "1.5 h", "~31 h").
  ///
  /// Same 90-minute rollover as [formatQualityTime], but hours are rounded
  /// for glanceability — to one decimal under 10 h, whole hours above —
  /// with a "~" prefix whenever rounding hides real minutes. Receipts and
  /// detail modals keep full precision via [formatQualityTime].
  static String formatQualityTimeCompact(double minutes) {
    final mins = minutes.round();
    if (mins < _hourRolloverMinutes) return '$mins min';
    final hours = mins / 60.0;
    if (hours < 10) {
      final tenths = (hours * 10).round() / 10;
      final exact = (tenths * 60).round() == mins;
      final display = tenths == tenths.roundToDouble()
          ? tenths.round().toString()
          : tenths.toStringAsFixed(1);
      return exact ? '$display h' : '~$display h';
    }
    final whole = hours.round();
    final exact = whole * 60 == mins;
    return exact ? '$whole h' : '~$whole h';
  }

  static SavingsMetricDetail? _buildCostDetail(ImpactEstimate impact) {
    if (!impact.hasMoneySaved() || !impact.moneySaved.hasValueUsd()) {
      return null;
    }
    final ms = impact.moneySaved;
    final mean = ms.valueUsd.mean;
    if (mean <= 0) return null;

    final p = ms.hasProvenance() ? ms.provenance : null;

    return SavingsMetricDetail(
      label: 'SAVED',
      displayValue: ValueHelpers.formatSavingsMoneyUsd(mean),
      confidence: _stddevToConfidence(mean, ms.valueUsd.stddev),
      methodName: p != null ? provenanceDisplayName(p.name) : null,
      reasoning: p != null && p.reasoning.isNotEmpty ? p.reasoning : null,
      sources: p != null && p.sources.isNotEmpty ? p.sources.toList() : null,
    );
  }

  static SavingsMetricDetail? _buildTimeDetail(ImpactEstimate impact) {
    if (!impact.hasTimeSaved() || !impact.timeSaved.hasMinutes()) {
      return null;
    }
    final ts = impact.timeSaved;
    final mean = ts.minutes.mean;
    if (mean <= 0) return null;

    final p = ts.hasProvenance() ? ts.provenance : null;

    return SavingsMetricDetail(
      label: 'TIME RECOVERED',
      displayValue: ValueHelpers.formatTimeMinutes(mean),
      confidence: _stddevToConfidence(mean, ts.minutes.stddev),
      methodName: p != null ? provenanceDisplayName(p.name) : null,
      reasoning: p != null && p.reasoning.isNotEmpty ? p.reasoning : null,
      sources: p != null && p.sources.isNotEmpty ? p.sources.toList() : null,
    );
  }

  static SavingsMetricDetail? _buildCo2Detail(ImpactEstimate impact) {
    if (!impact.hasEmissionsPrevented()) return null;
    final ep = impact.emissionsPrevented;

    final mfgGrams = ep.hasManufactureAvoidedCarbon()
        ? ep.manufactureAvoidedCarbon.co2eGrams.mean
        : 0.0;
    final wasteGrams = ep.hasWasteReducedCarbon()
        ? ep.wasteReducedCarbon.co2eGrams.mean
        : 0.0;
    final co2Grams = mfgGrams + wasteGrams;
    if (co2Grams <= 0) return null;

    final stddev = ep.hasManufactureAvoidedCarbon()
        ? ep.manufactureAvoidedCarbon.co2eGrams.stddev
        : 0.0;

    final p = ep.hasProvenance() ? ep.provenance : null;

    return SavingsMetricDetail(
      label: 'CO₂ AVOIDED',
      displayValue: ValueHelpers.formatCO2Grams(co2Grams),
      confidence: _stddevToConfidence(co2Grams, stddev),
      methodName: p != null ? provenanceDisplayName(p.name) : null,
      reasoning: p != null && p.reasoning.isNotEmpty ? p.reasoning : null,
      sources: p != null && p.sources.isNotEmpty ? p.sources.toList() : null,
    );
  }

  static SavingsMetricDetail? _buildQualityTimeDetail(ImpactEstimate impact) {
    if (!impact.hasQualityTime() || !impact.qualityTime.hasQualityTimeMinutes()) {
      return null;
    }
    final qt = impact.qualityTime;
    final mean = qt.qualityTimeMinutes.mean;
    if (mean <= 0) return null;

    final p = qt.hasProvenance() ? qt.provenance : null;

    return SavingsMetricDetail(
      label: 'QUALITY TIME',
      displayValue: formatQualityTime(mean),
      confidence: _stddevToConfidence(mean, qt.qualityTimeMinutes.stddev),
      methodName: p != null ? provenanceDisplayName(p.name) : null,
      reasoning: p != null && p.reasoning.isNotEmpty ? p.reasoning : null,
      sources: p != null && p.sources.isNotEmpty ? p.sources.toList() : null,
    );
  }

  /// Converts stddev/mean ratio to a 0.0-1.0 confidence value.
  ///
  /// confidence = 1.0 - (stddev / mean), clamped to [0.1, 0.99].
  /// Higher confidence means lower relative uncertainty.
  static double? _stddevToConfidence(double mean, double stddev) {
    if (mean <= 0) return null;
    if (stddev <= 0) return 0.99;
    final confidence = 1.0 - (stddev / mean);
    return confidence.clamp(0.1, 0.99);
  }

  /// Maps a provenance name string to a human-readable display name.
  static String? provenanceDisplayName(String name) {
    switch (name) {
      case 'product_lca_carbon':
        return 'Product Lifecycle Analysis';
      case 'weight_material_carbon':
        return 'Weight × Material Factor';
      case 'category_average_carbon':
        return 'Category Average';
      case 'spend_based_carbon':
        return 'Spend-Based Estimate';
      case 'prevented_purchase':
        return 'Prevented Purchase';
      case 'service_value':
        return 'Service Value';
      case 'commercial_value':
        return 'Commercial Value';
      case 'gear_shopping_time':
      case 'request_labor_time':
      case 'experience_duration':
        return 'Research-Based Default';
      case 'genai_time_estimate':
        return 'AI Estimated';
      case 'quality_time_v1':
        return 'Quality Time (Config Defaults)';
      case 'quality_time_llm':
        return 'Quality Time (AI Enriched)';
      default:
        return null;
    }
  }

  /// Calculates confidence score from mean and standard deviation.
  ///
  /// Returns:
  /// - 0.85 (Good) if ratio < 0.3
  /// - 0.6 (Moderate) if ratio < 0.6
  /// - 0.3 (Low) otherwise
  static double calculateConfidence(double mean, double stddev) {
    if (mean <= 0) return 0.5;
    final ratio = stddev / mean;
    if (ratio < 0.3) return 0.85; // Good
    if (ratio < 0.6) return 0.6; // Moderate
    return 0.3; // Low
  }
}
