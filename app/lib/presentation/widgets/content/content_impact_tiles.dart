import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Horizontal row of three tappable impact metric tiles.
///
/// Shared across experience and request content views. Always renders three
/// tiles (Money Saved, CO₂ Avoided, Quality Time). Metrics with no value
/// display "–". Tapping any tile calls [onMetricTap] with the metric identifier.
class ContentImpactTiles extends StatelessWidget {
  const ContentImpactTiles({
    super.key,
    required this.impact,
    required this.onMetricTap,
  });

  /// The impact estimate to display. When null, the widget renders nothing
  /// (stats still loading).
  final ImpactEstimate? impact;

  /// Called when a tile is tapped. The argument is one of "money", "co2", "qt".
  final ValueChanged<String> onMetricTap;

  @override
  Widget build(BuildContext context) {
    if (impact == null) return const SizedBox.shrink();

    final savings = SavingsFormatter.formatSavings(impact);
    final costValue = savings?.costSaved;
    final co2Value = savings?.co2Saved;
    final qtValue = savings?.qtSaved;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        // Order matches the completion modal and story card: Money Saved →
        // Quality Time → CO₂ Avoided. Keeping the same order across surfaces
        // means a glance at any tile can be cross-checked against the others.
        Row(
          children: [
            Expanded(
              child: _MetricTile(
                icon: Icons.attach_money,
                value: _displayValue(costValue),
                label: context.l10n.impactMoneySaved,
                onTap: () => onMetricTap('money'),
              ),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: _MetricTile(
                icon: Icons.schedule,
                value: _displayValue(qtValue),
                label: context.l10n.impactQualityTime,
                onTap: () => onMetricTap('qt'),
              ),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: _MetricTile(
                icon: Icons.eco,
                value: _displayValue(co2Value),
                label: context.l10n.impactCo2Avoided,
                onTap: () => onMetricTap('co2'),
              ),
            ),
          ],
        ),
      ],
    );
  }

  /// Returns the display value, or "–" for null/zero values.
  String _displayValue(String? value) {
    if (value == null || value.isEmpty) return '–';
    // Treat zero-value formatted strings as empty. The QT formatter renders
    // zero as "0 min" (see SavingsFormatter.formatQualityTime).
    if (value == '\$0' ||
        value == '0h' ||
        value == '0kg' ||
        value == '0 min' ||
        value == '0 g') {
      return '–';
    }
    return value;
  }
}

class _MetricTile extends StatelessWidget {
  const _MetricTile({
    required this.icon,
    required this.value,
    required this.label,
    required this.onTap,
  });

  final IconData icon;
  final String value;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final isDash = value == '–';
    final valueColor = isDash ? GlassTokens.textFaint : GlassTokens.textPrimary;

    return Tappable(
      semanticsLabel: context.l10n.a11yContentMetricTile(label, value),
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 14, horizontal: 8),
        decoration: BoxDecoration(
          color: OverlayTokens.fieldFill,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: GlassTokens.hairline),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              value,
              textAlign: TextAlign.center,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w700,
                color: valueColor,
              ),
            ),
            const SizedBox(height: 2),
            Text(
              label,
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w700,
                letterSpacing: 0.5,
                color: GlassTokens.textFaint,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
