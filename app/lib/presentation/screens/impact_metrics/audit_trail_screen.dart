import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/widgets/impact/audit_trail_step.dart';

/// AuditTrailScreen shows the per-attribute provenance breakdown for an ImpactEstimate.
///
/// Each attribute that contributed to the final impact value is displayed with
/// its source badge (USER / AI / CALCULATED / DEFAULT), making the calculation
/// transparent to the user.
class AuditTrailScreen extends StatelessWidget {
  const AuditTrailScreen({super.key, required this.impact});

  final ImpactEstimate impact;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        title: Text(
          context.l10n.impactAuditTrailTitle,
          style: TextStyle(
            color: AppColors.textPrimary(context),
            fontSize: 17,
            fontWeight: FontWeight.w600,
          ),
        ),
        iconTheme: IconThemeData(color: AppColors.textPrimary(context)),
      ),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          if (impact.hasQualityTime() && impact.qualityTime.hasAttributes())
            _Section(
              title: context.l10n.impactAuditTrailQualityTimeSection,
              children: _qtSteps(context, impact.qualityTime.attributes),
            ),
          if (impact.hasMoneySaved())
            _Section(
              title: context.l10n.impactAuditTrailMoneySection,
              children: _moneySteps(context, impact.moneySaved),
            ),
          if (impact.hasEmissionsPrevented())
            _Section(
              title: context.l10n.impactAuditTrailEmissionsSection,
              children: _emissionsSteps(context, impact.emissionsPrevented),
            ),
        ],
      ),
    );
  }

  List<Widget> _qtSteps(BuildContext context, QualityTimeAttributes attrs) {
    final l10n = context.l10n;
    return [
      AuditTrailStep(
        label: l10n.experienceQtBaseDuration,
        value: '${attrs.estimatedDurationMinutes.toStringAsFixed(0)} min',
        source: attrs.hasEstimatedDurationMinutesProvenance()
            ? attrs.estimatedDurationMinutesProvenance.source
            : ProvenanceSource.PROVENANCE_SOURCE_CONFIG_DEFAULT,
      ),
      AuditTrailStep(
        label: l10n.experienceQtGroupSize,
        value: '${attrs.groupSize}',
        source: attrs.hasGroupSizeProvenance()
            ? attrs.groupSizeProvenance.source
            : ProvenanceSource.PROVENANCE_SOURCE_CONFIG_DEFAULT,
      ),
      AuditTrailStep(
        label: l10n.experienceQtTieStrength,
        value: _tieStrengthLabel(context, attrs.tieStrength),
        source: attrs.hasTieStrengthProvenance()
            ? attrs.tieStrengthProvenance.source
            : ProvenanceSource.PROVENANCE_SOURCE_CONFIG_DEFAULT,
      ),
      AuditTrailStep(
        label: l10n.experienceQtReciprocity,
        value: _reciprocityLabel(context, attrs.reciprocity),
        source: attrs.hasReciprocityProvenance()
            ? attrs.reciprocityProvenance.source
            : ProvenanceSource.PROVENANCE_SOURCE_CONFIG_DEFAULT,
      ),
      AuditTrailStep(
        label: l10n.experienceQtNovelty,
        value: _noveltyLabel(context, attrs.novelty),
        source: attrs.hasNoveltyProvenance()
            ? attrs.noveltyProvenance.source
            : ProvenanceSource.PROVENANCE_SOURCE_CONFIG_DEFAULT,
      ),
      AuditTrailStep(
        label: l10n.experienceQtVulnerability,
        value: _vulnerabilityLabel(context, attrs.vulnerability),
        source: attrs.hasVulnerabilityProvenance()
            ? attrs.vulnerabilityProvenance.source
            : ProvenanceSource.PROVENANCE_SOURCE_CONFIG_DEFAULT,
      ),
      AuditTrailStep(
        label: l10n.experienceQtModality,
        value: _modalityLabel(context, attrs.modality),
        source: attrs.hasModalityProvenance()
            ? attrs.modalityProvenance.source
            : ProvenanceSource.PROVENANCE_SOURCE_CONFIG_DEFAULT,
      ),
    ];
  }

  List<Widget> _moneySteps(BuildContext context, MoneySavings ms) {
    final mean = ms.valueUsd.mean;
    return [
      AuditTrailStep(
        label: context.l10n.impactMetricMoneySaved,
        value: '\$${mean.toStringAsFixed(2)}',
        source: ms.provenance.source,
      ),
    ];
  }

  List<Widget> _emissionsSteps(BuildContext context, PreventedEmissions ep) {
    final co2 = ep.manufactureAvoidedCarbon.co2eGrams.mean;
    return [
      AuditTrailStep(
        label: context.l10n.impactMetricCo2Avoided,
        value: '${co2.toStringAsFixed(0)} g',
        source: ep.provenance.source,
      ),
    ];
  }

  static String _tieStrengthLabel(BuildContext context, SocialTieStrength v) {
    final l10n = context.l10n;
    return switch (v) {
      SocialTieStrength.SOCIAL_TIE_STRENGTH_CLOSE => l10n.experienceQtTieStrengthClose,
      SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE => l10n.experienceQtTieStrengthActive,
      SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE => l10n.experienceQtTieStrengthAcquaintance,
      SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW => l10n.experienceQtTieStrengthNew,
      _ => '–',
    };
  }

  static String _reciprocityLabel(BuildContext context, SocialReciprocity v) {
    final l10n = context.l10n;
    return switch (v) {
      SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL => l10n.experienceQtReciprocityMutual,
      SocialReciprocity.SOCIAL_RECIPROCITY_GIVING => l10n.experienceQtReciprocityGiving,
      SocialReciprocity.SOCIAL_RECIPROCITY_RECEIVING => l10n.experienceQtReciprocityReceiving,
      _ => '–',
    };
  }

  static String _noveltyLabel(BuildContext context, SocialNovelty v) {
    final l10n = context.l10n;
    return switch (v) {
      SocialNovelty.SOCIAL_NOVELTY_NOVEL => l10n.experienceQtNoveltyNovel,
      SocialNovelty.SOCIAL_NOVELTY_INFREQUENT => l10n.experienceQtNoveltyInfrequent,
      SocialNovelty.SOCIAL_NOVELTY_ROUTINE => l10n.experienceQtNoveltyRoutine,
      _ => '–',
    };
  }

  static String _vulnerabilityLabel(BuildContext context, SocialVulnerabilityLevel v) {
    final l10n = context.l10n;
    return switch (v) {
      SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH => l10n.experienceQtVulnerabilityHigh,
      SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM => l10n.experienceQtVulnerabilityMedium,
      SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_LOW => l10n.experienceQtVulnerabilityLow,
      _ => '–',
    };
  }

  static String _modalityLabel(BuildContext context, SocialModality v) {
    final l10n = context.l10n;
    return switch (v) {
      SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED => l10n.experienceQtModalityInPerson,
      SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF => l10n.experienceQtModalityInPersonBrief,
      SocialModality.SOCIAL_MODALITY_VIDEO => l10n.experienceQtModalityVideo,
      SocialModality.SOCIAL_MODALITY_PHONE => l10n.experienceQtModalityPhone,
      SocialModality.SOCIAL_MODALITY_TEXT => l10n.experienceQtModalityText,
      _ => '–',
    };
  }
}

class _Section extends StatelessWidget {
  const _Section({required this.title, required this.children});

  final String title;
  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    if (children.isEmpty) return const SizedBox.shrink();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(bottom: 10, top: 4),
          child: Text(
            title.toUpperCase(),
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.6,
              color: AppColors.textTertiary(context),
            ),
          ),
        ),
        Container(
          decoration: BoxDecoration(
            color: AppColors.cardBackground(context),
            borderRadius: BorderRadius.circular(12),
            border: Border.all(color: AppColors.border(context)),
          ),
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          child: Column(
            children: [
              for (int i = 0; i < children.length; i++) ...[
                children[i],
                if (i < children.length - 1)
                  Divider(color: AppColors.divider(context), height: 1),
              ],
            ],
          ),
        ),
        const SizedBox(height: 20),
      ],
    );
  }
}
