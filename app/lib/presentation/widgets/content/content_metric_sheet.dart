import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate, MoneySavings, QualityTimeEstimate, SocialModality,
         SocialNovelty, SocialReciprocity, SocialTieStrength,
         SocialVulnerabilityLevel;
import 'package:ripls/data/gen/ripls/api/social.pb.dart' show SocialContext;
import 'package:ripls/presentation/screens/impact_metrics/audit_trail_screen.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/item/item_metric_data.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// Maps a [SocialModality] enum to its numeric multiplier weight.
double _modalityWeight(SocialModality m) {
  switch (m) {
    case SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED:
      return 1;
    case SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF:
      return 0.6;
    case SocialModality.SOCIAL_MODALITY_VIDEO:
      return 0.4;
    case SocialModality.SOCIAL_MODALITY_PHONE:
      return 0.3;
    case SocialModality.SOCIAL_MODALITY_TEXT:
      return 0.1;
    default:
      return 0.6;
  }
}

/// Maps a group size count to its tier multiplier weight.
double _groupSizeWeight(int groupSize) {
  if (groupSize <= 2) return 1;
  if (groupSize <= 5) return 1.2;
  if (groupSize <= 15) return 1.3;
  return 1.1;
}

/// Maps a [SocialTieStrength] enum to its numeric multiplier weight.
double _tieStrengthWeight(SocialTieStrength t) {
  switch (t) {
    case SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW:
      return 0.8;
    case SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE:
      return 1;
    case SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE:
      return 1.1;
    case SocialTieStrength.SOCIAL_TIE_STRENGTH_CLOSE:
      return 1.2;
    default:
      return 0.8;
  }
}

/// Maps a [SocialReciprocity] enum to its numeric multiplier weight.
double _reciprocityWeight(SocialReciprocity r) {
  switch (r) {
    case SocialReciprocity.SOCIAL_RECIPROCITY_GIVING:
      return 1;
    case SocialReciprocity.SOCIAL_RECIPROCITY_RECEIVING:
      return 0.7;
    case SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL:
      return 1;
    default:
      return 1;
  }
}

/// Maps a [SocialNovelty] enum to its numeric multiplier weight.
double _noveltyWeight(SocialNovelty n) {
  switch (n) {
    case SocialNovelty.SOCIAL_NOVELTY_NOVEL:
      return 1.3;
    case SocialNovelty.SOCIAL_NOVELTY_INFREQUENT:
      return 1;
    case SocialNovelty.SOCIAL_NOVELTY_ROUTINE:
      return 0.8;
    default:
      return 1.3;
  }
}

/// Maps a [SocialVulnerabilityLevel] enum to its numeric multiplier weight.
double _vulnerabilityWeight(SocialVulnerabilityLevel v) {
  switch (v) {
    case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_LOW:
      return 0.8;
    case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM:
      return 1;
    case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH:
      return 1.3;
    default:
      return 1;
  }
}

/// Formats a multiplier weight as "× 1.0" style string.
String _formatMultiplier(double weight) {
  final formatted = weight == weight.roundToDouble()
      ? weight.toStringAsFixed(1)
      : weight.toStringAsFixed(1);
  return '× $formatted';
}

/// Bottom sheet showing a metric formula breakdown.
///
/// Shared across experience and request content views. Opened when tapping an
/// impact tile. Displays the metric header (icon + value + label), a description
/// paragraph, and a "CALCULATION" card with key-value rows from
/// [SavingsMetricDetail]. Theme-aware — adapts to light/dark mode.
///
/// When [isEditable] is true and [metricId] is 'qt', the quality time dimension
/// rows become tappable pickers and the header value updates live as dimensions
/// change. Changes are emitted via [onSocialContextChanged].
class ContentMetricSheet extends StatefulWidget {
  const ContentMetricSheet._({
    required this.metricId,
    required this.impact,
    required this.isCompleted,
    this.isEditable = false,
    this.onSocialContextChanged,
  });

  final String metricId;
  final ImpactEstimate impact;
  final bool isCompleted;
  final bool isEditable;
  final ValueChanged<SocialContext>? onSocialContextChanged;

  /// Shows the metric formula sheet as a modal bottom sheet.
  static Future<void> show(
    BuildContext context, {
    required String metricId,
    required ImpactEstimate impact,
    required bool isCompleted,
    bool isEditable = false,
    ValueChanged<SocialContext>? onSocialContextChanged,
  }) {
    return showAccessibleModal(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (context) => ContentMetricSheet._(
        metricId: metricId,
        impact: impact,
        isCompleted: isCompleted,
        isEditable: isEditable,
        onSocialContextChanged: onSocialContextChanged,
      ),
    );
  }

  @override
  State<ContentMetricSheet> createState() => _ContentMetricSheetState();
}

class _ContentMetricSheetState extends State<ContentMetricSheet> {
  // QT dimension local state — seeded from server values on init.
  late SocialTieStrength _tieStrength;
  late SocialReciprocity _reciprocity;
  late SocialNovelty _novelty;
  late SocialVulnerabilityLevel _vulnerability;
  late SocialModality _modality;

  @override
  void initState() {
    super.initState();
    _seedQtState();
  }

  void _seedQtState() {
    if (widget.impact.hasQualityTime() &&
        widget.impact.qualityTime.hasAttributes()) {
      final attrs = widget.impact.qualityTime.attributes;
      _tieStrength = attrs.tieStrength;
      _reciprocity = attrs.reciprocity;
      _novelty = attrs.novelty;
      _vulnerability = attrs.vulnerability;
      _modality = attrs.modality;
    } else {
      _tieStrength = SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE;
      _reciprocity = SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL;
      _novelty = SocialNovelty.SOCIAL_NOVELTY_INFREQUENT;
      _vulnerability = SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM;
      _modality = SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED;
    }
  }

  void _notifyChanged() {
    widget.onSocialContextChanged?.call(SocialContext(
      tieStrength: _tieStrength,
      reciprocity: _reciprocity,
      novelty: _novelty,
      vulnerability: _vulnerability,
      modality: _modality,
    ));
  }

  /// Computes the live QT value in minutes using current local dimension state.
  double _computeQtMinutes() {
    if (!widget.impact.hasQualityTime()) return 0;
    final attrs = widget.impact.qualityTime.attributes;
    return attrs.estimatedDurationMinutes *
        _modalityWeight(_modality) *
        _groupSizeWeight(attrs.groupSize) *
        _tieStrengthWeight(_tieStrength) *
        _reciprocityWeight(_reciprocity) *
        _noveltyWeight(_novelty) *
        _vulnerabilityWeight(_vulnerability);
  }

  /// Formats the QT header value via the shared minutes-only formatter so
  /// it always agrees with the completion modal, QT detail modal, story
  /// card, and results-tab tile for the same impact estimate.
  String _formatQtDisplay(double minutes) {
    if (minutes <= 0) return '–';
    return SavingsFormatter.formatQualityTime(minutes);
  }

  @override
  Widget build(BuildContext context) {
    final config = _metricConfig(context, widget.metricId);
    final savings = SavingsFormatter.formatSavings(widget.impact);
    final detail = _getDetail(savings, widget.metricId);

    // For QT in editable mode, show the live-computed value in the header.
    final displayValue = (widget.metricId == 'qt' && widget.isEditable)
        ? _formatQtDisplay(_computeQtMinutes())
        : (detail?.displayValue ?? '–');

    return GlassSheet(
      padding: const EdgeInsets.fromLTRB(20, 0, 20, 24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
            // Header: icon box + value + label
            Row(
              children: [
                Container(
                  width: 48,
                  height: 48,
                  decoration: BoxDecoration(
                    color: AppColors.modalIconBadgeBackground,
                    borderRadius: BorderRadius.circular(14),
                    border: Border.all(
                      color: AppColors.modalIconBadgeBorder,
                    ),
                  ),
                  child: Icon(config.icon, size: 22, color: AppColors.modalTextPrimary),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                        displayValue,
                        style: TextStyle(
                          fontSize: 28,
                          fontWeight: FontWeight.w800,
                          color: AppColors.modalTextPrimary,
                          letterSpacing: -0.3,
                        ),
                      ),
                      Text(
                        config.label,
                        style: TextStyle(
                          fontSize: 14,
                          fontWeight: FontWeight.w600,
                          color: AppColors.modalTextSecondary,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
            const SizedBox(height: 16),

            // Description
            Text(
              _descriptionText(context, widget.metricId, detail),
              style: TextStyle(
                fontSize: 13,
                height: 1.5,
                color: AppColors.modalTextSecondary,
              ),
            ),
            const SizedBox(height: 16),

            // Calculation card
            if (widget.metricId == 'qt' &&
                widget.impact.hasQualityTime() &&
                widget.impact.qualityTime.hasAttributes())
              _buildQualityTimeCalculationCard(
                  context, widget.impact.qualityTime, config.color)
            else if (widget.metricId == 'money' && widget.impact.hasMoneySaved())
              _buildMoneyCalculationCard(
                  context, widget.impact.moneySaved, detail, config.color)
            else if (detail != null)
              _buildCalculationCard(context, detail, config.color),

            // Methodology note
            if (detail?.methodName != null) ...[
              const SizedBox(height: 14),
              Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Icon(
                    Icons.info_outline,
                    size: 14,
                    color: AppColors.modalTextMuted,
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      context.l10n.experienceMethodPrefix(detail!.methodName!),
                      style: TextStyle(
                        fontSize: 12,
                        color: AppColors.modalTextMuted,
                      ),
                    ),
                  ),
                ],
              ),
            ],

            // Sources
            if (detail?.sources != null && detail!.sources!.isNotEmpty) ...[
              const SizedBox(height: 8),
              for (final source in detail.sources!)
                Padding(
                  padding: const EdgeInsets.only(left: 20, bottom: 2),
                  child: Text(
                    '· $source',
                    style: TextStyle(
                      fontSize: 11,
                      color: AppColors.modalTextMuted,
                    ),
                  ),
                ),
            ],

            // Audit trail link — only available after completion when provenance is stamped.
            if (widget.isCompleted) ...[
              const SizedBox(height: 16),
              Tappable(
                semanticsLabel: context.l10n.impactAuditTrailTitle,
                onTap: () => NavigationHelpers.pushWithSlide<void>(
                  context: context,
                  screen: AuditTrailScreen(impact: widget.impact),
                ),
                child: Row(
                  children: [
                    Icon(Icons.manage_search,
                        size: 14, color: AppColors.modalTextMuted),
                    const SizedBox(width: 6),
                    Text(
                      context.l10n.impactAuditTrailTitle,
                      style: TextStyle(
                        fontSize: 12,
                        color: AppColors.modalTextMuted,
                      ),
                    ),
                    const Spacer(),
                    Icon(Icons.chevron_right,
                        size: 14, color: AppColors.modalTextMuted),
                  ],
                ),
              ),
            ],
        ],
      ),
    );
  }

  /// Returns the description text for the metric sheet.
  ///
  /// The `reasoning` fallback is the estimator's own explanation, composed
  /// from the formula and inputs it used. LLM value estimates no longer write
  /// into that field (#2936), so what renders here is computed, not recalled.
  String _descriptionText(
      BuildContext context, String id, SavingsMetricDetail? detail) {
    if (id == 'qt') return context.l10n.experienceQtDescription;
    if (id == 'money') return context.l10n.impactMoneyDescription;
    if (id == 'co2') return context.l10n.impactCo2Description;
    return detail?.reasoning ?? '';
  }

  Widget _buildMoneyCalculationCard(
      BuildContext context,
      MoneySavings ms,
      SavingsMetricDetail? detail,
      Color accentColor) {
    final savedMean = ms.valueUsd.mean;
    final isPrevented = ms.hasProvenance() &&
        ms.provenance.name == 'prevented_purchase';

    if (isPrevented && savedMean > 0) {
      const rate = 0.50;
      final retailValue = savedMean / rate;

      return Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: AppColors.modalInsetCardBg,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: AppColors.modalInsetCardBorder),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              context.l10n.impactCalculation,
              style: TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w700,
                letterSpacing: 0.8,
                color: AppColors.modalTextMuted,
              ),
            ),
            const SizedBox(height: 10),
            _buildRow(
              context,
              context.l10n.impactMoneyRetailValue,
              '\$${retailValue.toStringAsFixed(0)}',
              AppColors.modalTextPrimary,
            ),
            const SizedBox(height: 4),
            _buildRow(
              context,
              context.l10n.impactMoneyPurchaseRate,
              '\u00d7 ${(rate * 100).toStringAsFixed(0)}%',
              AppColors.modalTextPrimary,
            ),
            Divider(color: AppColors.modalFooterDivider, height: 16),
            _buildRow(
              context,
              context.l10n.impactMetricMoneySaved,
              detail?.displayValue ?? '\$${savedMean.toStringAsFixed(0)}',
              accentColor,
              isBold: true,
            ),
            if (detail?.confidence != null) ...[
              const SizedBox(height: 6),
              _buildRow(
                context,
                context.l10n.impactConfidence,
                '${(detail!.confidence! * 100).round()}%',
                AppColors.modalTextSecondary,
              ),
            ],
          ],
        ),
      );
    }

    if (detail != null) {
      return _buildCalculationCard(context, detail, accentColor);
    }
    return const SizedBox.shrink();
  }

  Widget _buildCalculationCard(
      BuildContext context, SavingsMetricDetail detail, Color accentColor) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: AppColors.modalInsetCardBg,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppColors.modalInsetCardBorder),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            context.l10n.impactCalculation,
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.8,
              color: AppColors.modalTextMuted,
            ),
          ),
          const SizedBox(height: 10),
          _buildRow(context, detail.label, detail.displayValue, accentColor,
              isBold: true),
          if (detail.confidence != null) ...[
            const SizedBox(height: 6),
            _buildRow(
              context,
              context.l10n.impactConfidence,
              '${(detail.confidence! * 100).round()}%',
              AppColors.modalTextSecondary,
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildQualityTimeCalculationCard(
      BuildContext context, QualityTimeEstimate qt, Color accentColor) {
    final attrs = qt.attributes;
    final detail = _getDetail(SavingsFormatter.formatSavings(widget.impact), 'qt');
    final durationMin = attrs.estimatedDurationMinutes.round();

    // The live total: use computed value when editable, server value otherwise.
    final totalValue = widget.isEditable
        ? _formatQtDisplay(_computeQtMinutes())
        : (detail?.displayValue ?? '–');

    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: AppColors.modalInsetCardBg,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppColors.modalInsetCardBorder),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            context.l10n.impactCalculation,
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.8,
              color: AppColors.modalTextMuted,
            ),
          ),
          const SizedBox(height: 10),
          _buildRow(context, context.l10n.experienceQtBaseDuration,
              '$durationMin min', AppColors.modalTextPrimary),
          const SizedBox(height: 4),
          _buildQtDimensionRow(
            context: context,
            label: context.l10n.experienceQtModality,
            value: _formatMultiplier(_modalityWeight(_modality)),
            onTap: !widget.isEditable
                ? null
                : () => _showPickerSheet(
                      context: context,
                      title: context.l10n.experienceQtModality,
                      options: [
                        (SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED,
                            context.l10n.experienceQtModalityInPerson),
                        (SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF,
                            context.l10n.experienceQtModalityInPersonBrief),
                        (SocialModality.SOCIAL_MODALITY_VIDEO,
                            context.l10n.experienceQtModalityVideo),
                        (SocialModality.SOCIAL_MODALITY_PHONE,
                            context.l10n.experienceQtModalityPhone),
                        (SocialModality.SOCIAL_MODALITY_TEXT,
                            context.l10n.experienceQtModalityText),
                      ],
                      current: _modality,
                      onSelected: (v) {
                        setState(() => _modality = v as SocialModality);
                        _notifyChanged();
                      },
                    ),
          ),
          const SizedBox(height: 4),
          _buildRow(context, context.l10n.experienceQtGroupSize,
              _formatMultiplier(_groupSizeWeight(attrs.groupSize)),
              AppColors.modalTextPrimary),
          const SizedBox(height: 4),
          _buildQtDimensionRow(
            context: context,
            label: context.l10n.experienceQtTieStrength,
            value: _formatMultiplier(_tieStrengthWeight(_tieStrength)),
            onTap: !widget.isEditable
                ? null
                : () => _showPickerSheet(
                      context: context,
                      title: context.l10n.experienceQtTieStrength,
                      options: [
                        (SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW,
                            context.l10n.experienceQtTieStrengthNew),
                        (SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE,
                            context.l10n.experienceQtTieStrengthAcquaintance),
                        (SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE,
                            context.l10n.experienceQtTieStrengthActive),
                        (SocialTieStrength.SOCIAL_TIE_STRENGTH_CLOSE,
                            context.l10n.experienceQtTieStrengthClose),
                      ],
                      current: _tieStrength,
                      onSelected: (v) {
                        setState(() => _tieStrength = v as SocialTieStrength);
                        _notifyChanged();
                      },
                    ),
          ),
          const SizedBox(height: 4),
          _buildQtDimensionRow(
            context: context,
            label: context.l10n.experienceQtReciprocity,
            value: _formatMultiplier(_reciprocityWeight(_reciprocity)),
            onTap: !widget.isEditable
                ? null
                : () => _showPickerSheet(
                      context: context,
                      title: context.l10n.experienceQtReciprocity,
                      options: [
                        (SocialReciprocity.SOCIAL_RECIPROCITY_GIVING,
                            context.l10n.experienceQtReciprocityGiving),
                        (SocialReciprocity.SOCIAL_RECIPROCITY_RECEIVING,
                            context.l10n.experienceQtReciprocityReceiving),
                        (SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL,
                            context.l10n.experienceQtReciprocityMutual),
                      ],
                      current: _reciprocity,
                      onSelected: (v) {
                        setState(() => _reciprocity = v as SocialReciprocity);
                        _notifyChanged();
                      },
                    ),
          ),
          const SizedBox(height: 4),
          _buildQtDimensionRow(
            context: context,
            label: context.l10n.experienceQtNovelty,
            value: _formatMultiplier(_noveltyWeight(_novelty)),
            onTap: !widget.isEditable
                ? null
                : () => _showPickerSheet(
                      context: context,
                      title: context.l10n.experienceQtNovelty,
                      options: [
                        (SocialNovelty.SOCIAL_NOVELTY_NOVEL,
                            context.l10n.experienceQtNoveltyNovel),
                        (SocialNovelty.SOCIAL_NOVELTY_INFREQUENT,
                            context.l10n.experienceQtNoveltyInfrequent),
                        (SocialNovelty.SOCIAL_NOVELTY_ROUTINE,
                            context.l10n.experienceQtNoveltyRoutine),
                      ],
                      current: _novelty,
                      onSelected: (v) {
                        setState(() => _novelty = v as SocialNovelty);
                        _notifyChanged();
                      },
                    ),
          ),
          const SizedBox(height: 4),
          _buildQtDimensionRow(
            context: context,
            label: context.l10n.experienceQtVulnerability,
            value: _formatMultiplier(_vulnerabilityWeight(_vulnerability)),
            onTap: !widget.isEditable
                ? null
                : () => _showPickerSheet(
                      context: context,
                      title: context.l10n.experienceQtVulnerability,
                      options: [
                        (SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_LOW,
                            context.l10n.experienceQtVulnerabilityLow),
                        (SocialVulnerabilityLevel
                            .SOCIAL_VULNERABILITY_LEVEL_MEDIUM,
                            context.l10n.experienceQtVulnerabilityMedium),
                        (SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH,
                            context.l10n.experienceQtVulnerabilityHigh),
                      ],
                      current: _vulnerability,
                      onSelected: (v) {
                        setState(() =>
                            _vulnerability = v as SocialVulnerabilityLevel);
                        _notifyChanged();
                      },
                    ),
          ),
          Divider(color: AppColors.modalFooterDivider, height: 16),
          _buildRow(context, context.l10n.impactMetricQualityTime,
              totalValue, accentColor,
              isBold: true),
          if (!widget.isEditable && detail?.confidence != null) ...[
            const SizedBox(height: 6),
            _buildRow(
              context,
              context.l10n.impactConfidence,
              '${(detail!.confidence! * 100).round()}%',
              AppColors.modalTextSecondary,
            ),
          ],
        ],
      ),
    );
  }

  /// Builds a single row in the calculation card. When [onTap] is non-null
  /// (editable mode), the row is tappable and shows a trailing chevron.
  Widget _buildQtDimensionRow({
    required BuildContext context,
    required String label,
    required String value,
    VoidCallback? onTap,
  }) {
    final row = Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(
            label,
            style: TextStyle(
              fontSize: 13,
              color: AppColors.modalTextSecondary,
            ),
          ),
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                value,
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                  color: AppColors.modalTextPrimary,
                ),
              ),
              if (onTap != null) ...[
                const SizedBox(width: 4),
                Icon(
                  Icons.chevron_right,
                  size: 14,
                  color: AppColors.modalTextMuted,
                ),
              ],
            ],
          ),
        ],
      ),
    );

    if (onTap == null) return row;
    return Tappable(
      semanticsLabel: context.l10n.a11yLabelValue(label, value),
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(6),
      child: row,
    );
  }

  Widget _buildRow(
      BuildContext context, String label, String value, Color valueColor,
      {bool isBold = false}) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(
            label,
            style: TextStyle(
              fontSize: 13,
              fontWeight: isBold ? FontWeight.w700 : FontWeight.w400,
              color: AppColors.modalTextSecondary,
            ),
          ),
          Text(
            value,
            style: TextStyle(
              fontSize: 13,
              fontWeight: isBold ? FontWeight.w700 : FontWeight.w500,
              color: valueColor,
            ),
          ),
        ],
      ),
    );
  }

  /// Shows a bottom sheet picker for a single QT dimension.
  void _showPickerSheet<T>({
    required BuildContext context,
    required String title,
    required List<(T, String)> options,
    required T current,
    required ValueChanged<Object> onSelected,
  }) {
    showAccessibleModal<void>(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (ctx) => GlassSheet(
        padding: const EdgeInsets.fromLTRB(0, 0, 0, 8),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 12),
              child: Text(
                title,
                style: const TextStyle(
                  color: AppColors.modalTextPrimary,
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
            ...options.map((opt) {
              final (value, label) = opt;
              final isSelected = value == current;
              return ListTile(
                title: Text(
                  label,
                  style: TextStyle(
                    color: isSelected
                        ? AppColors.experienceColorOnDark
                        : AppColors.modalTextPrimary,
                    fontWeight:
                        isSelected ? FontWeight.w700 : FontWeight.w400,
                    fontSize: 15,
                  ),
                ),
                trailing: isSelected
                    ? Icon(Icons.check,
                        color: AppColors.experienceColorOnDark, size: 18)
                    : null,
                onTap: () {
                  onSelected(value as Object);
                  Navigator.pop(ctx);
                },
              );
            }),
          ],
        ),
      ),
    );
  }

  SavingsMetricDetail? _getDetail(ItemSavingsData? savings, String id) {
    if (savings == null) return null;
    return switch (id) {
      'money' => savings.costDetail,
      'co2' => savings.co2Detail,
      'qt' => savings.qtDetail,
      _ => null,
    };
  }

  _MetricConfig _metricConfig(BuildContext context, String id) {
    return switch (id) {
      'money' => _MetricConfig(
          icon: Icons.attach_money,
          label: context.l10n.impactMetricMoneySaved,
          color: AppColors.transferCoral,
        ),
      'co2' => _MetricConfig(
          icon: Icons.eco,
          label: context.l10n.impactMetricCo2Avoided,
          color: AppColors.statusSuccessOnDark,
        ),
      'qt' => _MetricConfig(
          icon: Icons.schedule,
          label: context.l10n.impactMetricQualityTime,
          color: AppColors.transferSage,
        ),
      _ => _MetricConfig(
          icon: Icons.help_outline,
          label: 'Unknown',
          color: AppColors.modalTextPrimary,
        ),
    };
  }
}

class _MetricConfig {
  const _MetricConfig({
    required this.icon,
    required this.label,
    required this.color,
  });

  final IconData icon;
  final String label;
  final Color color;
}
