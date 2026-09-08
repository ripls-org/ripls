import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/services/providers/media_providers.dart';

/// One Library shelf tile (#2634 v3,
/// `bottom-nav-liquid-glass-mock-v3.html`): a bigger 150×144 photo card
/// with the serif name on a bottom scrim. Borrowable is the default
/// and wears no pill — only the meaningful states earn one: a
/// Giveaway pill in the top-right corner. No owner/distance line; that
/// context lives on the category and map surfaces.
class LibraryShelfTile extends ConsumerWidget {
  final SearchDiscoverItem item;
  final VoidCallback onTap;

  const LibraryShelfTile({super.key, required this.item, required this.onTap});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final isGiveaway = item.searchResult.gear.availability ==
        Availability.AVAILABILITY_FOR_GIVEAWAY;
    return Tappable(
      semanticsLabel: isGiveaway
          ? '${item.name} · ${l10n.libraryTileGiveaway}'
          : item.name,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(16),
      child: SizedBox(
        width: 150,
        height: 144,
        child: ClipRRect(
          borderRadius: BorderRadius.circular(16),
          child: Stack(
            fit: StackFit.expand,
            children: [
              _photo(context, ref),
              // Bottom scrim so the name reads over any photo.
              const DecoratedBox(
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topCenter,
                    end: Alignment.bottomCenter,
                    colors: [Colors.transparent, GlassTokens.scrimHeavy],
                    stops: [0.45, 1.0],
                  ),
                ),
              ),
              if (isGiveaway)
                Positioned(
                  top: 8,
                  right: 8,
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 9, vertical: 4),
                    decoration: BoxDecoration(
                      color: AppColors.background(context)
                          .withValues(alpha: 0.95),
                      borderRadius: BorderRadius.circular(9),
                    ),
                    child: Text(
                      l10n.libraryTileGiveaway,
                      style: TextStyle(
                        fontSize: 10,
                        fontWeight: FontWeight.w600,
                        color: AppColors.primary(context),
                      ),
                    ),
                  ),
                ),
              Positioned(
                left: 11,
                right: 11,
                bottom: 10,
                child: Text(
                  item.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontFamily: AppTheme.headingFont,
                    fontSize: 15.5,
                    fontWeight: FontWeight.w500,
                    color: OverlayTokens.textPrimary,
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _photo(BuildContext context, WidgetRef ref) {
    Widget placeholder() => ColoredBox(color: AppColors.surface(context));
    final mediaId = item.thumbnailMediaId;
    if (mediaId.isEmpty) return placeholder();
    final mediaAsync = ref.watch(mediaObjectProvider(mediaId));
    return mediaAsync.when(
      data: (media) {
        if (media.url.isEmpty) return placeholder();
        return CachedMediaImage(
          // Decorative: the tile's Tappable announces name + status.
          semanticsLabel: null,
          imageUrl: media.url,
          cacheKey: ImageCacheKeys.thumbnail(mediaId),
          fit: BoxFit.cover,
        );
      },
      loading: () => placeholder(),
      error: (_, _) => placeholder(),
    );
  }
}

/// The add-in-context ghost end-cap on every shelf: a dashed-outline
/// invitation (not an item) that opens the create flow so contribution
/// is one tap from where the gap is felt.
class LibraryShelfGhostTile extends StatelessWidget {
  final VoidCallback onTap;

  const LibraryShelfGhostTile({super.key, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.libraryShelfAdd,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(16),
      child: CustomPaint(
        foregroundPainter: _DashedBorderPainter(
          color: AppColors.textTertiary(context),
          radius: 16,
        ),
        child: Container(
          width: 150,
          height: 144,
          decoration: BoxDecoration(
            color: AppColors.surface(context).withValues(alpha: 0.55),
            borderRadius: BorderRadius.circular(16),
          ),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(Icons.add,
                  size: 18, color: AppColors.textSecondary(context)),
              const SizedBox(height: 5),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 6),
                child: Text(
                  context.l10n.libraryShelfAdd,
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                    color: AppColors.textSecondary(context),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Paints the ghost tile's 1.5px dashed rounded-rect outline.
class _DashedBorderPainter extends CustomPainter {
  const _DashedBorderPainter({required this.color, required this.radius});

  final Color color;
  final double radius;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.5;
    final path = Path()
      ..addRRect(RRect.fromRectAndRadius(
        const Offset(0.75, 0.75) &
            Size(size.width - 1.5, size.height - 1.5),
        Radius.circular(radius),
      ));
    const dash = 5.0, gap = 4.0;
    for (final metric in path.computeMetrics()) {
      var distance = 0.0;
      while (distance < metric.length) {
        canvas.drawPath(
          metric.extractPath(distance, distance + dash),
          paint,
        );
        distance += dash + gap;
      }
    }
  }

  @override
  bool shouldRepaint(_DashedBorderPainter oldDelegate) =>
      oldDelegate.color != color || oldDelegate.radius != radius;
}
