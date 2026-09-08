import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// One tile in the [MetricTileGrid] — a serif value with an optional
/// trailing unit and an uppercase label below.
class MetricTileData {
  const MetricTileData({
    required this.value,
    required this.label,
    this.subtitle,
    this.unit,
    this.onTap,
    this.onTapRect,
    this.semanticsLabel,
  });

  /// The headline figure ("6", "207", "$8,693").
  final String value;

  /// Optional small trailing unit rendered next to [value] ("hrs", "kg").
  final String? unit;

  /// Uppercase caption ("Problems solved", "Time together").
  final String label;

  /// Optional plain-language line under the label on the ledger rows
  /// ("asks answered by a friend") — the v11 privacy-contract subtext.
  /// Ignored by the tile grid.
  final String? subtitle;

  /// Optional tap target → the matching metric drill-down. Null renders
  /// the tile inert (e.g. when viewing another user's profile).
  final VoidCallback? onTap;

  /// When set, preferred over [onTap]: receives the row's own on-screen
  /// footprint — the source rect a morph-reveal panel grows from (see
  /// `openContentMorphPanel`), so the drill-down expands in place the
  /// same way the profile conversation does.
  final ValueChanged<Rect>? onTapRect;

  /// Spoken label for assistive tech; falls back to "[value] [unit] [label]".
  final String? semanticsLabel;
}

/// The 2×2 metric tile grid shared by the person and community profile
/// surfaces (issue #2568, `profile-final.html`): Problems solved · Time
/// together · Money saved · CO₂ avoided. Person tiles carry
/// target-aggregate totals; community tiles carry collective all-member
/// totals. Each tile is its own tap target into the matching drill-down.
class MetricTileGrid extends StatelessWidget {
  const MetricTileGrid({super.key, required this.tiles, this.onPhoto = false});

  /// The tiles, rendered row-major into a 2-column grid. Designed for
  /// four entries but renders any count.
  final List<MetricTileData> tiles;

  /// When true the tiles render as glass over the profile's full-bleed
  /// background photo (translucent fill, light text). When false they use
  /// the opaque card surface.
  final bool onPhoto;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 13, 20, 2),
      child: GridView.count(
        crossAxisCount: 2,
        mainAxisSpacing: 9,
        crossAxisSpacing: 9,
        childAspectRatio: 2.3,
        shrinkWrap: true,
        physics: const NeverScrollableScrollPhysics(),
        children: tiles
            .map((t) => _Tile(data: t, onPhoto: onPhoto))
            .toList(growable: false),
      ),
    );
  }
}

class _Tile extends StatelessWidget {
  const _Tile({required this.data, required this.onPhoto});

  final MetricTileData data;
  final bool onPhoto;

  @override
  Widget build(BuildContext context) {
    final valueColor =
        onPhoto ? OverlayTokens.textPrimary : AppColors.textPrimary(context);
    final labelColor = onPhoto
        ? OverlayTokens.textFaint
        : AppColors.textTertiary(context);
    final tile = Container(
      padding: const EdgeInsets.fromLTRB(13, 12, 13, 13),
      decoration: BoxDecoration(
        color: onPhoto
            ? GlassTokens.fillSubtle
            : AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(15),
        border: Border.all(
          color: onPhoto
              ? GlassTokens.borderSoft
              : AppColors.divider(context),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.baseline,
            textBaseline: TextBaseline.alphabetic,
            children: [
              Flexible(
                child: Text(
                  data.value,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontFamily: AppTheme.headingFont,
                    fontSize: 21,
                    fontWeight: FontWeight.w600,
                    height: 1,
                    color: valueColor,
                  ),
                ),
              ),
              if (data.unit != null) ...[
                const SizedBox(width: 3),
                Text(
                  data.unit!,
                  style: TextStyle(
                    fontFamily: AppTheme.headingFont,
                    fontSize: 12,
                    fontWeight: FontWeight.w500,
                    color: valueColor,
                  ),
                ),
              ],
            ],
          ),
          const SizedBox(height: 6),
          Text(
            data.label.toUpperCase(),
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(
              fontSize: 9,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.7,
              color: labelColor,
            ),
          ),
        ],
      ),
    );
    if (data.onTap == null) return tile;
    return Tappable(
      semanticsLabel: data.semanticsLabel ??
          '${data.value} ${data.unit ?? ''} ${data.label}'.trim(),
      onTap: data.onTap!,
      child: tile,
    );
  }
}
