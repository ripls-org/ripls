import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/services/providers.dart' show mediaObjectProvider;

/// HomeMediaThumb renders a square thumbnail for a Home row from a media ID
/// via CachedMediaImage, with a neutral icon fallback. Decorative — the
/// surrounding row carries the semantic label.
class HomeMediaThumb extends ConsumerWidget {
  final String mediaId;
  final double size;
  final bool circle;

  const HomeMediaThumb({
    super.key,
    required this.mediaId,
    this.size = 40,
    this.circle = false,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final radius = BorderRadius.circular(circle ? size / 2 : 10);
    Widget fallback() => Container(
          width: size,
          height: size,
          decoration: BoxDecoration(
            color: AppColors.surface(context),
            borderRadius: radius,
          ),
          child: Icon(
            Icons.inventory_2_outlined,
            size: size * 0.45,
            color: AppColors.textTertiary(context),
          ),
        );

    if (mediaId.isEmpty) return fallback();

    final mediaAsync = ref.watch(mediaObjectProvider(mediaId));
    return ClipRRect(
      borderRadius: radius,
      child: mediaAsync.when(
        data: (mediaUrl) {
          if (mediaUrl.url.isEmpty) return fallback();
          return CachedMediaImage(
            // Decorative; the surrounding row carries the semantic label.
            semanticsLabel: null,
            imageUrl: mediaUrl.url,
            cacheKey: ImageCacheKeys.thumbnail(mediaId),
            width: size,
            height: size,
            fit: BoxFit.cover,
          );
        },
        loading: () => SizedBox(width: size, height: size),
        error: (_, _) => fallback(),
      ),
    );
  }
}
