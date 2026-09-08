import 'package:ripls/l10n/app_localizations.dart';
import 'equivalence_tier.dart';

/// ResolvedEquivalence is a localized view of an [EquivalenceTier] —
/// what the rendering layer needs. Returned by [resolveEquivalence].
class ResolvedEquivalence {
  /// Stable analytics identifier (e.g. `t_40hr`). Null when the value
  /// is below the lowest tier in the ladder.
  final String? tierId;

  /// Localized bold headline phrase (e.g. "A hug, repaid"). Falls back
  /// to "Just getting started" when no tier resolves.
  final String headline;

  /// Localized evidence paragraph. Falls back to a per-metric fallback
  /// when no tier resolves.
  final String body;

  /// Localized source name (e.g. "Psychology Today"). Empty when no
  /// tier resolves.
  final String sourceName;

  /// Source URL — opens in the system browser. Empty when no tier
  /// resolves.
  final String sourceUrl;

  const ResolvedEquivalence({
    required this.tierId,
    required this.headline,
    required this.body,
    required this.sourceName,
    required this.sourceUrl,
  });
}

/// resolveEquivalence picks the highest tier in [ladder] whose
/// threshold is `<= value` and resolves its ARB keys against [l10n].
/// When [value] is below the lowest tier, returns a fallback with
/// the per-metric [fallbackBodyKey] body — the headline is always
/// `equivalenceFallbackHeadline`.
ResolvedEquivalence resolveEquivalence({
  required List<EquivalenceTier> ladder,
  required double value,
  required AppLocalizations l10n,
  required String fallbackBody,
}) {
  final tier = selectTier(ladder, value);
  if (tier == null) {
    return ResolvedEquivalence(
      tierId: null,
      headline: l10n.equivalenceFallbackHeadline,
      body: fallbackBody,
      sourceName: '',
      sourceUrl: '',
    );
  }
  return ResolvedEquivalence(
    tierId: tier.id,
    headline: _lookup(l10n, tier.labelKey),
    body: _lookup(l10n, tier.copyKey),
    sourceName: _lookup(l10n, tier.sourceNameKey),
    sourceUrl: tier.sourceUrl,
  );
}

/// formatTimeValue turns a raw hours value into the localized "{count}
/// hr" / "{count} mins" string used inside the caption.
String formatTimeValue(double hours, AppLocalizations l10n) {
  if (hours < 1) {
    final minutes = (hours * 60).round();
    return l10n.equivalenceTimeFormattedMinutes(minutes);
  }
  return l10n.equivalenceTimeFormattedHours(hours.round());
}

/// formatCo2Value turns a raw kg-CO₂ value into the localized display
/// string used inside the caption. Values under 1 kg are shown in
/// grams; larger values use kg with thousands separators.
String formatCo2Value(double kg, AppLocalizations l10n) {
  if (kg < 1) {
    final grams = (kg * 1000).round();
    return l10n.equivalenceCo2FormattedGrams(grams);
  }
  final n = kg.round();
  if (n < 1000) {
    return l10n.equivalenceCo2FormattedKg(n.toString());
  }
  final digits = n.toString();
  final buf = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) buf.write(',');
    buf.write(digits[i]);
  }
  return l10n.equivalenceCo2FormattedKg(buf.toString());
}

/// formatMoneyValue turns a raw USD value into the localized currency
/// string used inside the caption. Adds thousands separators for
/// readability — the underlying number formatter matches the existing
/// workshop-money detail layout.
String formatMoneyValue(double usd, AppLocalizations l10n) {
  final n = usd.round();
  if (n.abs() < 1000) {
    return l10n.equivalenceMoneyFormattedUsd(n.toString());
  }
  final digits = n.abs().toString();
  final buf = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) buf.write(',');
    buf.write(digits[i]);
  }
  final formatted = '${n < 0 ? '-' : ''}${buf.toString()}';
  return l10n.equivalenceMoneyFormattedUsd(formatted);
}

// AppLocalizations is generated and exposes one getter per key. Since
// we need to look up by key string at runtime we use a small dispatch
// map. The list is exhaustive over both ladders + the shared fallback
// keys; missing keys throw to surface authoring mistakes during dev.
String _lookup(AppLocalizations l10n, String key) {
  final fn = _arbDispatch[key];
  if (fn == null) {
    throw ArgumentError('Unknown equivalence ARB key: $key');
  }
  return fn(l10n);
}

final Map<String, String Function(AppLocalizations)> _arbDispatch = {
  // Time-Health labels & copy.
  'equivalenceTimeHealthLabelT5min': (l) => l.equivalenceTimeHealthLabelT5min,
  'equivalenceTimeHealthCopyT5min': (l) => l.equivalenceTimeHealthCopyT5min,
  'equivalenceTimeHealthLabelT1hr': (l) => l.equivalenceTimeHealthLabelT1hr,
  'equivalenceTimeHealthCopyT1hr': (l) => l.equivalenceTimeHealthCopyT1hr,
  'equivalenceTimeHealthLabelT3hr': (l) => l.equivalenceTimeHealthLabelT3hr,
  'equivalenceTimeHealthCopyT3hr': (l) => l.equivalenceTimeHealthCopyT3hr,
  'equivalenceTimeHealthLabelT5hr': (l) => l.equivalenceTimeHealthLabelT5hr,
  'equivalenceTimeHealthCopyT5hr': (l) => l.equivalenceTimeHealthCopyT5hr,
  'equivalenceTimeHealthLabelT10hr': (l) => l.equivalenceTimeHealthLabelT10hr,
  'equivalenceTimeHealthCopyT10hr': (l) => l.equivalenceTimeHealthCopyT10hr,
  'equivalenceTimeHealthLabelT15hr': (l) => l.equivalenceTimeHealthLabelT15hr,
  'equivalenceTimeHealthCopyT15hr': (l) => l.equivalenceTimeHealthCopyT15hr,
  'equivalenceTimeHealthLabelT25hr': (l) => l.equivalenceTimeHealthLabelT25hr,
  'equivalenceTimeHealthCopyT25hr': (l) => l.equivalenceTimeHealthCopyT25hr,
  'equivalenceTimeHealthLabelT40hr': (l) => l.equivalenceTimeHealthLabelT40hr,
  'equivalenceTimeHealthCopyT40hr': (l) => l.equivalenceTimeHealthCopyT40hr,
  'equivalenceTimeHealthLabelT60hr': (l) => l.equivalenceTimeHealthLabelT60hr,
  'equivalenceTimeHealthCopyT60hr': (l) => l.equivalenceTimeHealthCopyT60hr,
  'equivalenceTimeHealthLabelT100hr': (l) => l.equivalenceTimeHealthLabelT100hr,
  'equivalenceTimeHealthCopyT100hr': (l) => l.equivalenceTimeHealthCopyT100hr,
  'equivalenceTimeHealthLabelT150hr': (l) => l.equivalenceTimeHealthLabelT150hr,
  'equivalenceTimeHealthCopyT150hr': (l) => l.equivalenceTimeHealthCopyT150hr,
  'equivalenceTimeHealthLabelT250hr': (l) => l.equivalenceTimeHealthLabelT250hr,
  'equivalenceTimeHealthCopyT250hr': (l) => l.equivalenceTimeHealthCopyT250hr,
  'equivalenceTimeHealthLabelT500hr': (l) => l.equivalenceTimeHealthLabelT500hr,
  'equivalenceTimeHealthCopyT500hr': (l) => l.equivalenceTimeHealthCopyT500hr,
  'equivalenceTimeHealthLabelT1000hr': (l) => l.equivalenceTimeHealthLabelT1000hr,
  'equivalenceTimeHealthCopyT1000hr': (l) => l.equivalenceTimeHealthCopyT1000hr,
  'equivalenceTimeHealthLabelT2000hr': (l) => l.equivalenceTimeHealthLabelT2000hr,
  'equivalenceTimeHealthCopyT2000hr': (l) => l.equivalenceTimeHealthCopyT2000hr,
  'equivalenceTimeHealthLabelTLifetime': (l) => l.equivalenceTimeHealthLabelTLifetime,
  'equivalenceTimeHealthCopyTLifetime': (l) => l.equivalenceTimeHealthCopyTLifetime,
  // Time-Health source names.
  'equivalenceTimeHealthSourcePsychologyToday':
      (l) => l.equivalenceTimeHealthSourcePsychologyToday,
  'equivalenceTimeHealthSourceAha': (l) => l.equivalenceTimeHealthSourceAha,
  'equivalenceTimeHealthSourceAtlanticHealth':
      (l) => l.equivalenceTimeHealthSourceAtlanticHealth,
  'equivalenceTimeHealthSourceElsa': (l) => l.equivalenceTimeHealthSourceElsa,
  'equivalenceTimeHealthSourcePmcCardio':
      (l) => l.equivalenceTimeHealthSourcePmcCardio,
  'equivalenceTimeHealthSourcePmcConnection':
      (l) => l.equivalenceTimeHealthSourcePmcConnection,
  'equivalenceTimeHealthSourceHoltLunstad':
      (l) => l.equivalenceTimeHealthSourceHoltLunstad,
  'equivalenceTimeHealthSourceUnhExtension':
      (l) => l.equivalenceTimeHealthSourceUnhExtension,
  'equivalenceTimeHealthSourceSurgeonGeneral':
      (l) => l.equivalenceTimeHealthSourceSurgeonGeneral,
  'equivalenceTimeHealthSourceCornellChronicle':
      (l) => l.equivalenceTimeHealthSourceCornellChronicle,
  'equivalenceTimeHealthSourceMahalingam':
      (l) => l.equivalenceTimeHealthSourceMahalingam,
  'equivalenceTimeHealthSourceRushUniversity':
      (l) => l.equivalenceTimeHealthSourceRushUniversity,
  'equivalenceTimeHealthSourceScientificAmerican':
      (l) => l.equivalenceTimeHealthSourceScientificAmerican,
  'equivalenceTimeHealthSourceHarvardChan':
      (l) => l.equivalenceTimeHealthSourceHarvardChan,
  'equivalenceTimeHealthSourcePsyPost':
      (l) => l.equivalenceTimeHealthSourcePsyPost,

  // Money-Paycheck labels & copy.
  'equivalenceMoneyPaycheckLabelM15': (l) => l.equivalenceMoneyPaycheckLabelM15,
  'equivalenceMoneyPaycheckCopyM15': (l) => l.equivalenceMoneyPaycheckCopyM15,
  'equivalenceMoneyPaycheckLabelM35': (l) => l.equivalenceMoneyPaycheckLabelM35,
  'equivalenceMoneyPaycheckCopyM35': (l) => l.equivalenceMoneyPaycheckCopyM35,
  'equivalenceMoneyPaycheckLabelM75': (l) => l.equivalenceMoneyPaycheckLabelM75,
  'equivalenceMoneyPaycheckCopyM75': (l) => l.equivalenceMoneyPaycheckCopyM75,
  'equivalenceMoneyPaycheckLabelM125': (l) => l.equivalenceMoneyPaycheckLabelM125,
  'equivalenceMoneyPaycheckCopyM125': (l) => l.equivalenceMoneyPaycheckCopyM125,
  'equivalenceMoneyPaycheckLabelM200': (l) => l.equivalenceMoneyPaycheckLabelM200,
  'equivalenceMoneyPaycheckCopyM200': (l) => l.equivalenceMoneyPaycheckCopyM200,
  'equivalenceMoneyPaycheckLabelM250': (l) => l.equivalenceMoneyPaycheckLabelM250,
  'equivalenceMoneyPaycheckCopyM250': (l) => l.equivalenceMoneyPaycheckCopyM250,
  'equivalenceMoneyPaycheckLabelM500': (l) => l.equivalenceMoneyPaycheckLabelM500,
  'equivalenceMoneyPaycheckCopyM500': (l) => l.equivalenceMoneyPaycheckCopyM500,
  'equivalenceMoneyPaycheckLabelM610': (l) => l.equivalenceMoneyPaycheckLabelM610,
  'equivalenceMoneyPaycheckCopyM610': (l) => l.equivalenceMoneyPaycheckCopyM610,
  'equivalenceMoneyPaycheckLabelM1025': (l) => l.equivalenceMoneyPaycheckLabelM1025,
  'equivalenceMoneyPaycheckCopyM1025': (l) => l.equivalenceMoneyPaycheckCopyM1025,
  'equivalenceMoneyPaycheckLabelM1500': (l) => l.equivalenceMoneyPaycheckLabelM1500,
  'equivalenceMoneyPaycheckCopyM1500': (l) => l.equivalenceMoneyPaycheckCopyM1500,
  'equivalenceMoneyPaycheckLabelM2000': (l) => l.equivalenceMoneyPaycheckLabelM2000,
  'equivalenceMoneyPaycheckCopyM2000': (l) => l.equivalenceMoneyPaycheckCopyM2000,
  'equivalenceMoneyPaycheckLabelM3000': (l) => l.equivalenceMoneyPaycheckLabelM3000,
  'equivalenceMoneyPaycheckCopyM3000': (l) => l.equivalenceMoneyPaycheckCopyM3000,
  'equivalenceMoneyPaycheckLabelM5000': (l) => l.equivalenceMoneyPaycheckLabelM5000,
  'equivalenceMoneyPaycheckCopyM5000': (l) => l.equivalenceMoneyPaycheckCopyM5000,
  'equivalenceMoneyPaycheckLabelM7500': (l) => l.equivalenceMoneyPaycheckLabelM7500,
  'equivalenceMoneyPaycheckCopyM7500': (l) => l.equivalenceMoneyPaycheckCopyM7500,
  'equivalenceMoneyPaycheckLabelM10000': (l) => l.equivalenceMoneyPaycheckLabelM10000,
  'equivalenceMoneyPaycheckCopyM10000': (l) => l.equivalenceMoneyPaycheckCopyM10000,
  'equivalenceMoneyPaycheckLabelM24500': (l) => l.equivalenceMoneyPaycheckLabelM24500,
  'equivalenceMoneyPaycheckCopyM24500': (l) => l.equivalenceMoneyPaycheckCopyM24500,
  'equivalenceMoneyPaycheckLabelM50000': (l) => l.equivalenceMoneyPaycheckLabelM50000,
  'equivalenceMoneyPaycheckCopyM50000': (l) => l.equivalenceMoneyPaycheckCopyM50000,
  // Money-Paycheck source names.
  'equivalenceMoneyPaycheckSourceWhataburgersMenu':
      (l) => l.equivalenceMoneyPaycheckSourceWhataburgersMenu,
  'equivalenceMoneyPaycheckSourceBlsEarnings':
      (l) => l.equivalenceMoneyPaycheckSourceBlsEarnings,
  'equivalenceMoneyPaycheckSourceUsdaFoodPlans':
      (l) => l.equivalenceMoneyPaycheckSourceUsdaFoodPlans,
  'equivalenceMoneyPaycheckSourceWalletHubGas':
      (l) => l.equivalenceMoneyPaycheckSourceWalletHubGas,
  'equivalenceMoneyPaycheckSourceRamsey':
      (l) => l.equivalenceMoneyPaycheckSourceRamsey,
  'equivalenceMoneyPaycheckSourceCrossCountry':
      (l) => l.equivalenceMoneyPaycheckSourceCrossCountry,
  'equivalenceMoneyPaycheckSourceCensusHousing':
      (l) => l.equivalenceMoneyPaycheckSourceCensusHousing,
  'equivalenceMoneyPaycheckSourcePayscale':
      (l) => l.equivalenceMoneyPaycheckSourcePayscale,
  'equivalenceMoneyPaycheckSourceBlsPaidLeave':
      (l) => l.equivalenceMoneyPaycheckSourceBlsPaidLeave,
  'equivalenceMoneyPaycheckSourceIrsLimits':
      (l) => l.equivalenceMoneyPaycheckSourceIrsLimits,

  // CO₂ daily-life labels & copy.
  'equivalenceCo2DailyLifeLabelC05': (l) => l.equivalenceCo2DailyLifeLabelC05,
  'equivalenceCo2DailyLifeCopyC05': (l) => l.equivalenceCo2DailyLifeCopyC05,
  'equivalenceCo2DailyLifeLabelC1': (l) => l.equivalenceCo2DailyLifeLabelC1,
  'equivalenceCo2DailyLifeCopyC1': (l) => l.equivalenceCo2DailyLifeCopyC1,
  'equivalenceCo2DailyLifeLabelC2': (l) => l.equivalenceCo2DailyLifeLabelC2,
  'equivalenceCo2DailyLifeCopyC2': (l) => l.equivalenceCo2DailyLifeCopyC2,
  'equivalenceCo2DailyLifeLabelC35': (l) => l.equivalenceCo2DailyLifeLabelC35,
  'equivalenceCo2DailyLifeCopyC35': (l) => l.equivalenceCo2DailyLifeCopyC35,
  'equivalenceCo2DailyLifeLabelC5': (l) => l.equivalenceCo2DailyLifeLabelC5,
  'equivalenceCo2DailyLifeCopyC5': (l) => l.equivalenceCo2DailyLifeCopyC5,
  'equivalenceCo2DailyLifeLabelC9': (l) => l.equivalenceCo2DailyLifeLabelC9,
  'equivalenceCo2DailyLifeCopyC9': (l) => l.equivalenceCo2DailyLifeCopyC9,
  'equivalenceCo2DailyLifeLabelC15': (l) => l.equivalenceCo2DailyLifeLabelC15,
  'equivalenceCo2DailyLifeCopyC15': (l) => l.equivalenceCo2DailyLifeCopyC15,
  'equivalenceCo2DailyLifeLabelC25': (l) => l.equivalenceCo2DailyLifeLabelC25,
  'equivalenceCo2DailyLifeCopyC25': (l) => l.equivalenceCo2DailyLifeCopyC25,
  'equivalenceCo2DailyLifeLabelC50': (l) => l.equivalenceCo2DailyLifeLabelC50,
  'equivalenceCo2DailyLifeCopyC50': (l) => l.equivalenceCo2DailyLifeCopyC50,
  'equivalenceCo2DailyLifeLabelC100': (l) => l.equivalenceCo2DailyLifeLabelC100,
  'equivalenceCo2DailyLifeCopyC100': (l) => l.equivalenceCo2DailyLifeCopyC100,
  'equivalenceCo2DailyLifeLabelC250': (l) => l.equivalenceCo2DailyLifeLabelC250,
  'equivalenceCo2DailyLifeCopyC250': (l) => l.equivalenceCo2DailyLifeCopyC250,
  'equivalenceCo2DailyLifeLabelC500': (l) => l.equivalenceCo2DailyLifeLabelC500,
  'equivalenceCo2DailyLifeCopyC500': (l) => l.equivalenceCo2DailyLifeCopyC500,
  'equivalenceCo2DailyLifeLabelC1000': (l) => l.equivalenceCo2DailyLifeLabelC1000,
  'equivalenceCo2DailyLifeCopyC1000': (l) => l.equivalenceCo2DailyLifeCopyC1000,
  'equivalenceCo2DailyLifeLabelC2500': (l) => l.equivalenceCo2DailyLifeLabelC2500,
  'equivalenceCo2DailyLifeCopyC2500': (l) => l.equivalenceCo2DailyLifeCopyC2500,
  'equivalenceCo2DailyLifeLabelC5000': (l) => l.equivalenceCo2DailyLifeLabelC5000,
  'equivalenceCo2DailyLifeCopyC5000': (l) => l.equivalenceCo2DailyLifeCopyC5000,
  'equivalenceCo2DailyLifeLabelC15000': (l) => l.equivalenceCo2DailyLifeLabelC15000,
  'equivalenceCo2DailyLifeCopyC15000': (l) => l.equivalenceCo2DailyLifeCopyC15000,
  'equivalenceCo2DailyLifeLabelC50000': (l) => l.equivalenceCo2DailyLifeLabelC50000,
  'equivalenceCo2DailyLifeCopyC50000': (l) => l.equivalenceCo2DailyLifeCopyC50000,
  // CO₂ daily-life source names.
  'equivalenceCo2DailyLifeSourceEpaVehicle':
      (l) => l.equivalenceCo2DailyLifeSourceEpaVehicle,
  'equivalenceCo2DailyLifeSourceCo2EverythingLaundry':
      (l) => l.equivalenceCo2DailyLifeSourceCo2EverythingLaundry,
  'equivalenceCo2DailyLifeSourcePlantBasedMinutes':
      (l) => l.equivalenceCo2DailyLifeSourcePlantBasedMinutes,
  'equivalenceCo2DailyLifeSourceEpaEquivalencies':
      (l) => l.equivalenceCo2DailyLifeSourceEpaEquivalencies,
  'equivalenceCo2DailyLifeSourceEiaKwh':
      (l) => l.equivalenceCo2DailyLifeSourceEiaKwh,
  'equivalenceCo2DailyLifeSourceOwidTravel':
      (l) => l.equivalenceCo2DailyLifeSourceOwidTravel,
  'equivalenceCo2DailyLifeSourceOwidAviation':
      (l) => l.equivalenceCo2DailyLifeSourceOwidAviation,
  'equivalenceCo2DailyLifeSourceEpaHousehold':
      (l) => l.equivalenceCo2DailyLifeSourceEpaHousehold,
  'equivalenceCo2DailyLifeSourceOwidUsProfile':
      (l) => l.equivalenceCo2DailyLifeSourceOwidUsProfile,
};
