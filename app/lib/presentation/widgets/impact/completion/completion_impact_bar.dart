import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/core/utils/value_helpers.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/completion/completion_widgets.dart';

/// CompletionImpactBar renders the glassy impact strip at the top of the
/// Mark Completed / Mark Fulfilled sheets: Saved · Quality Time · CO₂.
///
/// Shared by both completion modals so the two ceremonies stay in lockstep:
/// - While [isLoading], all three metrics render spinner placeholders.
/// - Once loaded, zero metrics are suppressed and the non-zero ones fill
///   the row; if nothing is non-zero the bar disappears entirely — a
///   "$0 Saved / 0.0kg CO₂" headline reads as broken (#2724).
/// - Quality time renders humanized ("~4 h", "37 min"), never raw minutes
///   (#2724).
class CompletionImpactBar extends StatelessWidget {
  const CompletionImpactBar({
    super.key,
    required this.impact,
    required this.isLoading,
    required this.onMoneyTap,
    required this.onQualityTimeTap,
    required this.onCo2Tap,
    this.moneySemanticsLabel,
    this.qualityTimeSemanticsLabel,
    this.co2SemanticsLabel,
  });

  /// The previewed impact estimate; null while drafting or on error.
  final ImpactEstimate? impact;

  /// Whether the draft request is in flight (spinner placeholders).
  final bool isLoading;

  final VoidCallback onMoneyTap;
  final VoidCallback onQualityTimeTap;
  final VoidCallback onCo2Tap;

  /// Optional per-metric a11y labels; default announces "label: value".
  final String? moneySemanticsLabel;
  final String? qualityTimeSemanticsLabel;
  final String? co2SemanticsLabel;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;

    final money = impact != null &&
            impact!.hasMoneySaved() &&
            impact!.moneySaved.hasValueUsd()
        ? '\$${impact!.moneySaved.valueUsd.mean.toStringAsFixed(0)}'
        : null;
    final qt = impact != null &&
            impact!.hasQualityTime() &&
            impact!.qualityTime.hasQualityTimeMinutes()
        ? SavingsFormatter.formatQualityTimeCompact(
            impact!.qualityTime.qualityTimeMinutes.mean,
          )
        : null;
    final co2Grams = impact != null && impact!.hasEmissionsPrevented()
        ? _emissionsGrams(impact!.emissionsPrevented)
        : null;
    final co2 = co2Grams == null ? null : ValueHelpers.formatCO2Grams(co2Grams);

    final metrics = <_MetricData>[
      if (isLoading || (money != null && money != '\$0'))
        _MetricData(
          label: l10n.completionImpactSaved,
          value: money,
          color: kCompletionGreenLight,
          onTap: onMoneyTap,
          semanticsLabel: moneySemanticsLabel,
        ),
      if (isLoading || (qt != null && qt != '0 min'))
        _MetricData(
          label: l10n.completionImpactQualityTime,
          value: qt,
          color: kCompletionAccentLight,
          onTap: onQualityTimeTap,
          semanticsLabel: qualityTimeSemanticsLabel,
        ),
      if (isLoading || (co2Grams != null && co2Grams >= _minShownCo2Grams))
        _MetricData(
          label: l10n.completionImpactCo2,
          value: co2,
          color: PaperTokens.accentBlue,
          onTap: onCo2Tap,
          semanticsLabel: co2SemanticsLabel,
        ),
    ];
    if (metrics.isEmpty) return const SizedBox.shrink();

    return Container(
      margin: const EdgeInsets.fromLTRB(16, 0, 16, 4),
      padding: const EdgeInsets.symmetric(vertical: 18, horizontal: 20),
      decoration: BoxDecoration(
        color: CompletionColors.containerFill(context),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: CompletionColors.glassBorder(context)),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceAround,
        children: [
          for (final m in metrics)
            _Metric(data: m, isLoading: isLoading || m.value == null),
        ],
      ),
    );
  }

  /// Below this the CO₂ tile is a headline of nothing, so it is dropped
  /// rather than shown. Compared against the value, not its rendering: the
  /// string a formatter happens to produce for zero is not a contract.
  static const double _minShownCo2Grams = 50;

  double _emissionsGrams(PreventedEmissions emissions) {
    double grams = 0;
    if (emissions.hasManufactureAvoidedCarbon() &&
        emissions.manufactureAvoidedCarbon.hasCo2eGrams()) {
      grams += emissions.manufactureAvoidedCarbon.co2eGrams.mean;
    }
    if (emissions.hasWasteReducedCarbon() &&
        emissions.wasteReducedCarbon.hasCo2eGrams()) {
      grams += emissions.wasteReducedCarbon.co2eGrams.mean;
    }
    return grams;
  }
}

class _MetricData {
  const _MetricData({
    required this.label,
    required this.value,
    required this.color,
    required this.onTap,
    this.semanticsLabel,
  });

  final String label;

  /// Null while loading (spinner placeholder).
  final String? value;
  final Color color;
  final VoidCallback onTap;
  final String? semanticsLabel;
}

class _Metric extends StatelessWidget {
  const _Metric({required this.data, required this.isLoading});

  final _MetricData data;
  final bool isLoading;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: data.semanticsLabel ??
          context.l10n.a11yLabelValue(data.label, data.value ?? ''),
      onTap: data.onTap,
      child: Column(
        children: [
          isLoading
              ? SizedBox(
                  width: 32,
                  height: 22,
                  child: Center(
                    child: SizedBox(
                      width: 14,
                      height: 14,
                      child: CircularProgressIndicator(
                        strokeWidth: 1.5,
                        color: data.color.withValues(alpha: 0.5),
                      ),
                    ),
                  ),
                )
              : Text(
                  data.value!,
                  style: TextStyle(
                    fontFamily: AppTheme.headingFont,
                    fontSize: 22,
                    fontWeight: FontWeight.w700,
                    color: data.color,
                  ),
                ),
          const SizedBox(height: 3),
          Text(
            data.label,
            style: TextStyle(
              fontSize: 10,
              color: CompletionColors.textDim(context),
              letterSpacing: 0.3,
            ),
          ),
        ],
      ),
    );
  }
}
