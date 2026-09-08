import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';

/// AuditTrailStep renders a single row in the per-attribute provenance audit trail.
///
/// Shows the attribute label on the left, and on the right a source badge
/// (USER / AI / CALCULATED / DEFAULT) plus the formatted value.
class AuditTrailStep extends StatelessWidget {
  const AuditTrailStep({
    super.key,
    required this.label,
    required this.value,
    required this.source,
  });

  final String label;
  final String value;
  final ProvenanceSource source;

  @override
  Widget build(BuildContext context) {
    final (badgeLabel, badgeColor, badgeTextColor) = _badgeStyle(context);

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Text(
              label,
              style: TextStyle(
                fontSize: 14,
                color: AppColors.textPrimary(context),
              ),
            ),
          ),
          const SizedBox(width: 8),
          Column(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Text(
                value,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: AppColors.textPrimary(context),
                ),
              ),
              const SizedBox(height: 2),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                decoration: BoxDecoration(
                  color: badgeColor,
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Text(
                  badgeLabel,
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w600,
                    color: badgeTextColor,
                    letterSpacing: 0.3,
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  (String, Color, Color) _badgeStyle(BuildContext context) {
    switch (source) {
      case ProvenanceSource.PROVENANCE_SOURCE_USER:
        return (
          context.l10n.impactAuditTrailSourceUser,
          ImpactModalColors.coral.withValues(alpha: 0.12),
          ImpactModalColors.coral,
        );
      case ProvenanceSource.PROVENANCE_SOURCE_LLM:
        return (
          context.l10n.impactAuditTrailSourceLlm,
          PaperTokens.accentGreen.withValues(alpha: 0.12),
          PaperTokens.accentGreen,
        );
      case ProvenanceSource.PROVENANCE_SOURCE_FORMULA:
        return (
          context.l10n.impactAuditTrailSourceFormula,
          AppColors.border(context),
          AppColors.textSecondary(context),
        );
      default:
        return (
          context.l10n.impactAuditTrailSourceDefault,
          AppColors.surface(context),
          AppColors.textTertiary(context),
        );
    }
  }
}
