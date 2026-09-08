import 'package:ripls/core/utils/drill_down/drill_down_references.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/impact.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart';
import 'package:ripls/presentation/models/drill_down_data.dart';

/// QualityTimeDrillDown builds quality-time drill-down data for item,
/// community, and user metric contexts.
class QualityTimeDrillDown {
  /// buildItem constructs a quality-time drill-down for an item-level impact
  /// estimate.
  static DrillDownData? buildItem(
    ImpactEstimate? impact,
  ) {
    if (impact == null ||
        !impact.hasQualityTime() ||
        !impact.qualityTime.hasQualityTimeMinutes()) {
      return null;
    }

    final qt = impact.qualityTime;
    final mean = qt.qualityTimeMinutes.mean;
    if (mean <= 0) return null;

    final confidence = SavingsFormatter.calculateConfidence(
        mean, qt.qualityTimeMinutes.stddev);

    final attrs = qt.hasAttributes() ? qt.attributes : null;
    final factorNodes = _buildFactors(attrs);

    return DrillDownData(
      metricName: 'Quality Time',
      formattedValue: SavingsFormatter.formatQualityTime(mean),
      confidence: confidence,
      formula: FormulaNode(
        label: 'Quality Time',
        formattedValue: SavingsFormatter.formatQualityTime(mean),
        operator: '×',
        explanation:
            'One QT minute approximates one minute of quality '
            'in-person social contact.',
        operands: factorNodes,
        referenceIds: const [1, 2, 3, 4],
      ),
      references: qualityTimeReferences,
    );
  }

  /// buildCommunity constructs a community-level quality-time drill-down.
  static DrillDownData? buildCommunity(
    CommunityImpactMetrics? metrics,
    GetCommunityMetricDetailResponse? detail,
  ) {
    if (metrics == null || !metrics.hasQualityTimeMinutes()) return null;
    final mean = metrics.qualityTimeMinutes.mean;
    if (mean <= 0) return null;

    final confidence = SavingsFormatter.calculateConfidence(
        mean, metrics.qualityTimeMinutes.stddev);

    final totalActivities = metrics.completedLoans +
        metrics.completedGiveaways +
        metrics.fulfilledRequests +
        metrics.pastEvents;
    final formattedValue = SavingsFormatter.formatQualityTime(mean);

    return DrillDownData(
      metricName: 'Quality Time',
      formattedValue: formattedValue,
      confidence: confidence,
      formula: FormulaNode(
        label: 'Quality Time',
        formattedValue: formattedValue,
        prefix: 'Sum of $totalActivities activities',
        explanation:
            'Sum of per-transaction quality time scores. '
            'Each score: duration × modality × group_size × tie_strength '
            '× reciprocity × novelty × vulnerability.',
        referenceIds: const [1, 2, 3, 4],
      ),
      references: qualityTimeReferences,
    );
  }

  /// buildUser constructs a user-level quality-time drill-down.
  static DrillDownData? buildUser(
    UserImpactMetrics? metrics,
  ) {
    if (metrics == null || !metrics.hasQualityTimeMinutes()) return null;
    final mean = metrics.qualityTimeMinutes.mean;
    if (mean <= 0) return null;

    final confidence = SavingsFormatter.calculateConfidence(
        mean, metrics.qualityTimeMinutes.stddev);

    final totalInteractions =
        metrics.completedLoans + metrics.completedGiveaways + metrics.fulfilledRequests;

    return DrillDownData(
      metricName: 'Quality Time',
      formattedValue: SavingsFormatter.formatQualityTime(mean),
      confidence: confidence,
      formula: FormulaNode(
        label: 'Quality Time',
        formattedValue: SavingsFormatter.formatQualityTime(mean),
        operator: 'Σ',
        explanation:
            'Sum of per-transaction quality time across '
            '$totalInteractions sharing interactions.',
        operands: const [
          FormulaNode(
            label: 'Per-Transaction QT',
            formattedValue: '(composite)',
            explanation:
                'Each transaction\'s 7-factor product measures the quality '
                'of social contact. One QT minute ≈ one minute of quality '
                'in-person interaction.',
            referenceIds: [1, 2, 3, 4],
          ),
        ],
      ),
      references: qualityTimeReferences,
    );
  }

  // ─── Private Helpers ─────────────────────────────────────────────────

  static List<FormulaNode> _buildFactors(
    QualityTimeAttributes? attrs,
  ) {
    if (attrs == null) {
      return [
        const FormulaNode(
          label: 'Factors',
          formattedValue: '(default)',
          explanation:
              'Quality time computed from assumed default factor values. '
              'Detailed breakdown not available for this transaction.',
        ),
      ];
    }

    return [
      FormulaNode(
        label: 'Duration',
        formattedValue: '${attrs.estimatedDurationMinutes.round()} min',
        explanation: 'Estimated face-to-face interaction time.',
        referenceIds: const [1],
      ),
      FormulaNode(
        label: 'Modality',
        formattedValue: _modalityDisplayValue(attrs.modality),
        explanation: _modalityExplanation(attrs.modality),
        referenceIds: const [2],
      ),
      FormulaNode(
        label: 'Group Size',
        formattedValue: '${attrs.groupSize}',
        explanation: _groupSizeExplanation(attrs.groupSize),
        referenceIds: const [3],
      ),
      FormulaNode(
        label: 'Tie Strength',
        formattedValue: _tieStrengthDisplayValue(attrs.tieStrength),
        explanation: _tieStrengthExplanation(attrs.tieStrength),
        referenceIds: const [3],
      ),
      FormulaNode(
        label: 'Reciprocity',
        formattedValue: _reciprocityDisplayValue(attrs.reciprocity),
        explanation: _reciprocityExplanation(attrs.reciprocity),
        referenceIds: const [4],
      ),
      FormulaNode(
        label: 'Novelty',
        formattedValue: _noveltyDisplayValue(attrs.novelty),
        explanation: _noveltyExplanation(attrs.novelty),
      ),
      FormulaNode(
        label: 'Vulnerability',
        formattedValue: _vulnerabilityDisplayValue(attrs.vulnerability),
        explanation: _vulnerabilityExplanation(attrs.vulnerability),
      ),
    ];
  }

  static String _modalityDisplayValue(SocialModality m) {
    switch (m) {
      case SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED:
        return '1.0';
      case SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF:
        return '0.6';
      case SocialModality.SOCIAL_MODALITY_VIDEO:
        return '0.4';
      case SocialModality.SOCIAL_MODALITY_PHONE:
        return '0.3';
      case SocialModality.SOCIAL_MODALITY_TEXT:
        return '0.1';
      default:
        return '0.6';
    }
  }

  static String _modalityExplanation(SocialModality m) {
    switch (m) {
      case SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED:
        return 'In-person shared activity: full social contact value (1.0×).';
      case SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF:
        return 'In-person transactional: brief exchange (0.6×).';
      case SocialModality.SOCIAL_MODALITY_VIDEO:
        return 'Video call: reduced social contact (0.4×).';
      case SocialModality.SOCIAL_MODALITY_PHONE:
        return 'Phone call: limited social contact (0.3×).';
      case SocialModality.SOCIAL_MODALITY_TEXT:
        return 'Text/async messaging: minimal social contact (0.1×).';
      default:
        return 'Default modality: in-person transactional (0.6×).';
    }
  }

  static String _groupSizeExplanation(int groupSize) {
    if (groupSize <= 2) return 'Dyadic interaction (1.0× multiplier).';
    if (groupSize <= 5) return 'Small group of $groupSize (1.2× multiplier).';
    if (groupSize <= 15) {
      return 'Medium group of $groupSize (1.3× multiplier).';
    }
    return 'Large group of $groupSize (1.1× multiplier).';
  }

  static String _tieStrengthDisplayValue(SocialTieStrength t) {
    switch (t) {
      case SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW:
        return 'New (0.8)';
      case SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE:
        return 'Acquaintance (1.0)';
      case SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE:
        return 'Active (1.1)';
      case SocialTieStrength.SOCIAL_TIE_STRENGTH_CLOSE:
        return 'Close (1.2)';
      default:
        return 'New (0.8)';
    }
  }

  static String _tieStrengthExplanation(SocialTieStrength t) {
    switch (t) {
      case SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW:
        return 'No prior interactions — new connection (0.8×).';
      case SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE:
        return '1–3 prior interactions — acquaintance (1.0×).';
      case SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE:
        return '4–10 prior interactions — active tie (1.1×).';
      case SocialTieStrength.SOCIAL_TIE_STRENGTH_CLOSE:
        return '11+ prior interactions — close tie (1.2×).';
      default:
        return 'New connection (0.8×).';
    }
  }

  static String _reciprocityDisplayValue(SocialReciprocity r) {
    switch (r) {
      case SocialReciprocity.SOCIAL_RECIPROCITY_GIVING:
        return 'Giving (1.0)';
      case SocialReciprocity.SOCIAL_RECIPROCITY_RECEIVING:
        return 'Receiving (0.7)';
      case SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL:
        return 'Mutual (1.0)';
      default:
        return 'Giving (1.0)';
    }
  }

  static String _reciprocityExplanation(SocialReciprocity r) {
    switch (r) {
      case SocialReciprocity.SOCIAL_RECIPROCITY_GIVING:
        return 'Lender/giver role — prosocial giving (1.0×).';
      case SocialReciprocity.SOCIAL_RECIPROCITY_RECEIVING:
        return 'Borrower/recipient role (0.7×).';
      case SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL:
        return 'Shared experience — mutual participation (1.0×).';
      default:
        return 'Default: giving role (1.0×).';
    }
  }

  static String _noveltyDisplayValue(SocialNovelty n) {
    switch (n) {
      case SocialNovelty.SOCIAL_NOVELTY_NOVEL:
        return 'Novel (1.3)';
      case SocialNovelty.SOCIAL_NOVELTY_INFREQUENT:
        return 'Infrequent (1.0)';
      case SocialNovelty.SOCIAL_NOVELTY_ROUTINE:
        return 'Routine (0.8)';
      default:
        return 'Novel (1.3)';
    }
  }

  static String _noveltyExplanation(SocialNovelty n) {
    switch (n) {
      case SocialNovelty.SOCIAL_NOVELTY_NOVEL:
        return 'First interaction of this type — novelty bonus (1.3×).';
      case SocialNovelty.SOCIAL_NOVELTY_INFREQUENT:
        return 'Same type, >30 days since last — moderate novelty (1.0×).';
      case SocialNovelty.SOCIAL_NOVELTY_ROUTINE:
        return 'Same type within last 30 days — routine (0.8×).';
      default:
        return 'Novel interaction (1.3×).';
    }
  }

  static String _vulnerabilityDisplayValue(SocialVulnerabilityLevel v) {
    switch (v) {
      case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_LOW:
        return 'Low (0.8)';
      case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM:
        return 'Medium (1.0)';
      case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH:
        return 'High (1.3)';
      default:
        return 'Medium (1.0)';
    }
  }

  static String _vulnerabilityExplanation(SocialVulnerabilityLevel v) {
    switch (v) {
      case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_LOW:
        return 'Low personal vulnerability (0.8×).';
      case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM:
        return 'Medium vulnerability — standard interaction (1.0×).';
      case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH:
        return 'High vulnerability, e.g., hosting at home (1.3×).';
      default:
        return 'Medium vulnerability (1.0×).';
    }
  }
}
