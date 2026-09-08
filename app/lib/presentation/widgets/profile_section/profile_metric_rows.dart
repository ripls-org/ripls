import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/profile_section/metric_tile_grid.dart'
    show MetricTileData;

/// The profile's quiet ledger (v11 synthesis): one hairline-separated
/// row per metric — bold name over a plain-language subtext on the
/// left, the serif number on the right, a chevron when the row drills
/// into a metric detail. The hero stat above it carries the editorial
/// moment; these rows stay tabular.
///
/// Renders over the profile's dark page, so all text is light.
/// `SizedBox.shrink()` when [metrics] is empty.
class ProfileMetricRows extends StatelessWidget {
  const ProfileMetricRows({super.key, required this.metrics});

  /// The metrics, rendered top-to-bottom. Rows with an `onTap` are
  /// their own ≥48px-tall tap targets; rows without one are inert.
  final List<MetricTileData> metrics;

  @override
  Widget build(BuildContext context) {
    if (metrics.isEmpty) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.fromLTRB(22, 0, 22, 0),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (var i = 0; i < metrics.length; i++)
            _MetricRow(data: metrics[i], isLast: i == metrics.length - 1),
        ],
      ),
    );
  }
}

class _MetricRow extends StatelessWidget {
  const _MetricRow({required this.data, required this.isLast});

  final MetricTileData data;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    final interactive = data.onTap != null || data.onTapRect != null;
    final row = Container(
      constraints: const BoxConstraints(minHeight: 56),
      padding: const EdgeInsets.symmetric(vertical: 12),
      decoration: BoxDecoration(
        border: isLast
            ? null
            : Border(
                bottom: BorderSide(
                  color: Colors.white.withValues(alpha: 0.16),
                ),
              ),
      ),
      child: Row(
        children: [
          Expanded(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  data.label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 14.5,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
                if (data.subtitle != null && data.subtitle!.isNotEmpty) ...[
                  const SizedBox(height: 3),
                  Text(
                    data.subtitle!,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 11.5,
                      height: 1.35,
                      color: Colors.white.withValues(alpha: 0.52),
                    ),
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(width: 12),
          Text.rich(
            TextSpan(
              text: data.value,
              style: const TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 22,
                fontWeight: FontWeight.w600,
                letterSpacing: -0.2,
                color: Colors.white,
              ),
              children: [
                if (data.unit != null)
                  TextSpan(
                    text: ' ${data.unit!}',
                    style: TextStyle(
                      fontFamily: AppTheme.bodyFont,
                      fontSize: 13,
                      fontWeight: FontWeight.w500,
                      color: Colors.white.withValues(alpha: 0.72),
                    ),
                  ),
              ],
            ),
          ),
          if (interactive) ...[
            const SizedBox(width: 8),
            Icon(
              Icons.chevron_right,
              size: 16,
              color: Colors.white.withValues(alpha: 0.52),
            ),
          ],
        ],
      ),
    );
    if (!interactive) return row;
    return Tappable(
      semanticsLabel: data.semanticsLabel ??
          '${data.value} ${data.unit ?? ''} ${data.label}'.trim(),
      // Rect-aware rows grow a morph-reveal drill-down from their own
      // footprint (the in-place expansion the profile conversation uses).
      onTap: data.onTapRect != null
          ? () => data.onTapRect!(_boundsOf(context))
          : data.onTap,
      child: row,
    );
  }
}

/// The tapped row's own on-screen footprint — the source rect a
/// morph-reveal panel grows from (`openContentMorphPanel`). Mirrors the
/// content views' per-file `_rectOf` helpers; [context] must belong to
/// an already-laid-out element (safe inside a tap handler).
Rect _boundsOf(BuildContext context) {
  final box = context.findRenderObject();
  return box is RenderBox && box.hasSize
      ? box.localToGlobal(Offset.zero) & box.size
      : Rect.zero;
}
