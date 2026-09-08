import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/presentation/models/magazine_item.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/services/providers.dart' show mediaUrlProvider;

/// MagazineThumbnail is the unified item card used in the daily person panel
/// and the discover grid/gallery.
///
/// Fixed height (220px), white-background card with a 120px image section on
/// top and a text section below. Tap callbacks are passed in by the parent —
/// this widget does not handle navigation.
class MagazineThumbnail extends ConsumerWidget {
  final MagazineItem item;
  final VoidCallback? onTap;

  const MagazineThumbnail({
    super.key,
    required this.item,
    this.onTap,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Tappable(
      semanticsLabel: item.title,
      onTap: onTap,
      excludeChildSemantics: false,
      child: Container(
        height: 220,
        decoration: BoxDecoration(
          color: AppColors.cardBackground(context),
          borderRadius: BorderRadius.circular(12),
          boxShadow: const [
            BoxShadow(
              color: Color(0x0A000000),
              blurRadius: 8,
              offset: Offset(0, 1),
            ),
          ],
        ),
        clipBehavior: Clip.hardEdge,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _buildImageSection(ref),
            Expanded(child: _buildTextSection(context)),
          ],
        ),
      ),
    );
  }

  // ─── Image section (120px) ─────────────────────────────────────────────────

  Widget _buildImageSection(WidgetRef ref) {
    return SizedBox(
      height: 120,
      width: double.infinity,
      child: item.mediaId.isNotEmpty
          ? _buildCachedImage(ref)
          : _buildEmojiBackground(),
    );
  }

  Widget _buildCachedImage(WidgetRef ref) {
    final mediaAsync = ref.watch(mediaUrlProvider(item.mediaId));
    return mediaAsync.when(
      data: (url) => url.isNotEmpty
          ? CachedMediaImage(
              // Decorative; the surrounding card carries the semantic label.
              semanticsLabel: null,imageUrl: url,
              cacheKey: ImageCacheKeys.thumbnail(item.mediaId),
              width: double.infinity,
              height: 120,
              fit: BoxFit.cover,
            )
          : _buildEmojiBackground(),
      loading: _buildEmojiBackground,
      error: (_, _) => _buildEmojiBackground(),
    );
  }

  Widget _buildEmojiBackground() {
    return Container(
      color: _colorForType(),
      child: Center(
        child: Text(item.emoji, style: const TextStyle(fontSize: 36)),
      ),
    );
  }

  /// Returns the emoji fallback background color for this item type.
  ///
  /// A saturated fill behind an emoji, not a foreground, so one mid-tone per
  /// type reads on either theme's card.
  Color _colorForType() {
    switch (item.itemType) {
      case 'event':
        return AppColors.experienceColorOnDark;
      case 'help':
        return AppColors.requestColorOnDark;
      case 'giving':
        return AppColors.giveawayColorOnDark;
      default: // 'lending'
        return AppColors.loanColorOnDark;
    }
  }

  // ─── Text section (100px) ──────────────────────────────────────────────────

  Widget _buildTextSection(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(10),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildTypeLabelRow(context),
          const SizedBox(height: 3),
          Text(
            item.title,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              color: AppColors.textPrimary(context),
              height: 1.25,
            ),
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
          ),
          const Spacer(),
          _buildBottomLine(context),
        ],
      ),
    );
  }

  Widget _buildTypeLabelRow(BuildContext context) {
    return Row(
      children: [
        if (item.typeLabel.isNotEmpty)
          Text(
            item.typeLabel.toUpperCase(),
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w700,
              color: AppColors.primary(context),
              letterSpacing: 0.5,
            ),
          ),
        if (item.typeLabel.isNotEmpty && item.dateLabel != null)
          Text(
            '  ·  ${item.dateLabel}',
            style: TextStyle(
              fontSize: 10,
              color: AppColors.textSecondary(context),
            ),
          ),
      ],
    );
  }

  Widget _buildBottomLine(BuildContext context) {
    final text = item.bottomLine;
    // Fixed-height container keeps card height consistent even when empty.
    return SizedBox(
      height: 16,
      child: text.isNotEmpty
          ? Text(
              text,
              style: TextStyle(
                fontSize: 11,
                color: AppColors.textSecondary(context),
              ),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            )
          : const SizedBox.shrink(),
    );
  }
}
