import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// SharingImpactCard displays the "What sharing this means" explanatory card.
///
/// Shows italic framing text followed by compact metric tiles (value saved,
/// time recovered, CO₂ avoided, and optionally quality time). Each tile is
/// tappable to open the audit trail.
///
/// The card explains the *per-action* numbers (what one loan / fulfillment /
/// session saves) while the hero strip above it shows cumulative totals, so
/// the header carries a [perActionLabel] ("Per loan", "Per fulfillment", …)
/// to make that distinction explicit (#2724). Metrics that are zero or
/// missing pass `null` and their tile is hidden — the remaining tiles fill
/// the row (#2724). All tiles get equal visual emphasis; the two time
/// tiles carry a one-line caption distinguishing time recovered from time
/// spent together (#2724).
class SharingImpactCard extends StatelessWidget {
  /// Formatted per-action metric values. Null hides the tile.
  final String? valueSaved;
  final String? timeRecovered;
  final String? co2Avoided;
  final String? qualityTime;

  /// Italic framing line above the metric tiles. A gear item is borrowed
  /// instead of bought; a request is filled by neighbors pitching in; an
  /// experience gathers people instead of everyone going it alone.
  final String framingText;

  /// Short qualifier rendered on the header row ("Per loan", "Per
  /// fulfillment", "Per session"). Null hides the qualifier.
  final String? perActionLabel;

  final VoidCallback? onValueTap;
  final VoidCallback? onTimeTap;
  final VoidCallback? onCo2Tap;
  final VoidCallback? onSocialTap;

  const SharingImpactCard({
    super.key,
    required this.framingText,
    this.valueSaved,
    this.timeRecovered,
    this.co2Avoided,
    this.qualityTime,
    this.perActionLabel,
    this.onValueTap,
    this.onTimeTap,
    this.onCo2Tap,
    this.onSocialTap,
  });

  /// Whether the card has at least one metric tile to show. Callers should
  /// skip building the card entirely when this would be false — framing
  /// text over an empty grid reads as broken.
  bool get hasAnyMetric =>
      valueSaved != null ||
      timeRecovered != null ||
      co2Avoided != null ||
      qualityTime != null;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final tiles = <_MetricTileData>[
      if (valueSaved != null)
        _MetricTileData(
          value: valueSaved!,
          label: 'Saved',
          onTap: onValueTap,
        ),
      if (timeRecovered != null)
        _MetricTileData(
          value: timeRecovered!,
          label: 'Recovered',
          caption: l10n.impactCaptionRecovered,
          onTap: onTimeTap,
        ),
      if (co2Avoided != null)
        _MetricTileData(
          value: co2Avoided!,
          label: 'CO₂ Avoided',
          onTap: onCo2Tap,
        ),
      if (qualityTime != null)
        _MetricTileData(
          value: qualityTime!,
          label: 'Quality Time',
          caption: l10n.impactCaptionQualityTime,
          onTap: onSocialTap,
        ),
    ];
    if (tiles.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.baseline,
          textBaseline: TextBaseline.alphabetic,
          children: [
            Expanded(
              child: Text(
                l10n.sharingImpactCardTitle.toUpperCase(),
                style: TextStyle(
                  color: AppColors.transferTextMuted(context),
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.6,
                ),
              ),
            ),
            if (perActionLabel != null)
              Text(
                perActionLabel!.toUpperCase(),
                style: TextStyle(
                  color: AppColors.transferTextMuted(context),
                  fontSize: 10,
                  fontWeight: FontWeight.w600,
                  letterSpacing: 0.6,
                ),
              ),
          ],
        ),
        const SizedBox(height: 12),
        Container(
          padding: const EdgeInsets.all(18),
          decoration: BoxDecoration(
            color: AppColors.transferHighlight(context),
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: AppColors.border(context), width: 1),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                framingText,
                style: TextStyle(
                  fontStyle: FontStyle.italic,
                  fontSize: 13,
                  color: AppColors.transferTextSecondary(context),
                  height: 1.5,
                ),
              ),
              const SizedBox(height: 14),
              ..._tileRows(tiles),
            ],
          ),
        ),
      ],
    );
  }

  /// Lays out the visible tiles two per row; a trailing odd tile stretches
  /// across the full width so a suppressed zero metric never leaves a gap.
  List<Widget> _tileRows(List<_MetricTileData> tiles) {
    final rows = <Widget>[];
    for (var i = 0; i < tiles.length; i += 2) {
      if (i > 0) rows.add(const SizedBox(height: 8));
      rows.add(
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _MetricTile(data: tiles[i]),
            if (i + 1 < tiles.length) ...[
              const SizedBox(width: 8),
              _MetricTile(data: tiles[i + 1]),
            ],
          ],
        ),
      );
    }
    return rows;
  }
}

/// Plain data for one metric tile.
class _MetricTileData {
  final String value;
  final String label;

  /// Optional one-line clarifier under the label (used to distinguish the
  /// two time metrics).
  final String? caption;
  final VoidCallback? onTap;

  const _MetricTileData({
    required this.value,
    required this.label,
    this.caption,
    this.onTap,
  });
}

/// _MetricTile is a compact tappable tile showing a value, label, and an
/// optional caption. All tiles share the same value color so no metric
/// reads as disabled next to another (#2724).
class _MetricTile extends StatelessWidget {
  final _MetricTileData data;

  const _MetricTile({required this.data});

  @override
  Widget build(BuildContext context) {
    final content = Container(
      padding: const EdgeInsets.symmetric(vertical: 14, horizontal: 8),
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: AppColors.transferBorderLight(context),
          width: 1,
        ),
      ),
      child: Column(
        children: [
          Text(
            data.value,
            style: const TextStyle(
              fontSize: 20,
              fontWeight: FontWeight.w700,
              color: AppColors.lightPrimary,
            ),
          ),
          const SizedBox(height: 2),
          Text(
            data.label,
            style: TextStyle(
              fontSize: 11,
              color: AppColors.transferTextMuted(context),
            ),
            textAlign: TextAlign.center,
          ),
          if (data.caption != null) ...[
            const SizedBox(height: 2),
            Text(
              data.caption!,
              style: TextStyle(
                fontSize: 9,
                height: 1.3,
                color: AppColors.transferTextMuted(context),
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ],
      ),
    );

    // Expanded must be the outermost widget so Row flex parent data is preserved.
    return Expanded(
      child: data.onTap != null
          ? Tappable(
              semanticsLabel:
                  context.l10n.a11yLabelValue(data.label, data.value),
              onTap: data.onTap,
              child: content,
            )
          : content,
    );
  }
}
