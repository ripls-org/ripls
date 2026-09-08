import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/presentation/models/drill_down_data.dart';

/// FormulaComponentCard renders a single FormulaNode as an expandable card.
///
/// Shows the component label, value, explanation, confidence indicator,
/// and forward-referenced citation IDs. Sub-components are rendered as
/// nested cards with indentation.
class FormulaComponentCard extends StatelessWidget {
  final FormulaNode node;
  final int depth;

  const FormulaComponentCard({
    super.key,
    required this.node,
    this.depth = 0,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: EdgeInsets.only(
        left: depth * 12.0,
        bottom: 8,
      ),
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: depth == 0 ? Colors.white : PaperTokens.surface,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(
          color: PaperTokens.borderSubtle,
          width: 1,
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildHeader(context),
          if (node.explanation != null) ...[
            const SizedBox(height: 8),
            _buildExplanation(),
          ],
          if (node.aggregateInsight != null) ...[
            const SizedBox(height: 8),
            _buildAggregateInsight(node.aggregateInsight!),
          ],
          if (node.operands != null && node.operands!.isNotEmpty) ...[
            const SizedBox(height: 10),
            ...node.operands!.map(
              (child) => FormulaComponentCard(
                node: child,
                depth: depth + 1,
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildHeader(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.baseline,
      textBaseline: TextBaseline.alphabetic,
      children: [
        Expanded(
          child: Text(
            node.label,
            style: const TextStyle(
              color: PaperTokens.textPrimary,
              fontSize: 14,
              fontWeight: FontWeight.w600,
            ),
          ),
        ),
        const SizedBox(width: 8),
        Text(
          node.formattedValue,
          style: const TextStyle(
            color: PaperTokens.textPrimary,
            fontSize: 16,
            fontWeight: FontWeight.w600,
                      ),
        ),
        if (node.confidence != null) ...[
          const SizedBox(width: 6),
          _buildConfidenceDot(context, node.confidence!),
        ],
      ],
    );
  }

  Widget _buildExplanation() {
    final hasRefs =
        node.referenceIds != null && node.referenceIds!.isNotEmpty;
    if (!hasRefs) {
      return Text(
        node.explanation!,
        style: const TextStyle(
          color: PaperTokens.textMuted,
          fontSize: 13,
          height: 1.4,
        ),
      );
    }

    // Inline reference IDs at end of explanation text.
    final refStr = node.referenceIds!.map((id) => '[$id]').join('');
    return Text.rich(
      TextSpan(
        children: [
          TextSpan(
            text: '${node.explanation!} ',
            style: const TextStyle(
              color: PaperTokens.textMuted,
              fontSize: 13,
              height: 1.4,
            ),
          ),
          TextSpan(
            text: refStr,
            style: const TextStyle(
              color: PaperTokens.accentGreen,
              fontSize: 12,
              fontWeight: FontWeight.w500,
              height: 1.4,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildAggregateInsight(AggregateInsight insight) {
    final items = <Widget>[];

    if (insight.itemCount != null) {
      items.add(_insightRow('Items', '${insight.itemCount}'));
    }
    if (insight.medianValue != null) {
      items.add(_insightRow('Median', insight.medianValue!));
    }
    if (insight.topContributorName != null) {
      final value = insight.topContributorValue ?? '';
      items.add(
          _insightRow('Top contributor', '${insight.topContributorName} $value')
              );
    }
    if (insight.distributionNote != null) {
      items.add(Padding(
        padding: const EdgeInsets.only(top: 2),
        child: Text(
          insight.distributionNote!,
          style: const TextStyle(
            color: PaperTokens.textMuted,
            fontSize: 12,
            fontStyle: FontStyle.italic,
          ),
        ),
      ));
    }
    if (insight.breakdownByType != null) {
      for (final entry in insight.breakdownByType!.entries) {
        items.add(_insightRow(entry.key, entry.value));
      }
    }

    if (items.isEmpty) return const SizedBox.shrink();

    return Container(
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: PaperTokens.surfaceRaised,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'AGGREGATE INSIGHTS',
            style: TextStyle(
              color: PaperTokens.textMuted,
              fontSize: 10,
              fontWeight: FontWeight.w600,
              letterSpacing: 0.5,
            ),
          ),
          const SizedBox(height: 6),
          ...items,
        ],
      ),
    );
  }

  Widget _insightRow(String label, String value) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 2),
      child: Row(
        children: [
          Text(
            '$label: ',
            style: const TextStyle(
              color: PaperTokens.textMuted,
              fontSize: 12,
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: const TextStyle(
                color: PaperTokens.textPrimary,
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildConfidenceDot(BuildContext context, double confidence) {
    // On-LIGHT, not theme-aware: this card is `PaperTokens`, which is opaque
    // and light-only — the impact report has no dark treatment. A theme-aware
    // getter would put the dark status colours on a cream card whenever the app
    // theme is dark, which is the #2445 bug with the themes swapped.
    Color color;
    if (confidence >= 0.8) {
      color = AppColors.statusSuccessOnLight;
    } else if (confidence >= 0.5) {
      color = AppColors.statusWarningOnLight;
    } else {
      color = AppColors.statusErrorOnLight;
    }

    return Container(
      width: 8,
      height: 8,
      decoration: BoxDecoration(
        color: color,
        shape: BoxShape.circle,
      ),
    );
  }
}
