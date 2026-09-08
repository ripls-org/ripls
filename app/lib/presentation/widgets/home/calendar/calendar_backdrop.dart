import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/services/providers.dart' show mediaObjectProvider;

/// CalendarBackdrop is the full-bleed photo behind the whole calendar, sourced
/// from the selected day's marquee event. It crossfades when the day changes —
/// unless the user prefers reduced motion, in which case it swaps instantly.
/// Falls back to a sage fill when the day has no photo.
class CalendarBackdrop extends ConsumerWidget {
  /// The media ID of the selected day's photo. Empty → sage fallback.
  final String mediaId;

  const CalendarBackdrop({super.key, required this.mediaId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Honor the OS reduce-motion preference: no crossfade, instant swap.
    final reduceMotion = MediaQuery.of(context).disableAnimations;
    final duration =
        reduceMotion ? Duration.zero : const Duration(milliseconds: 450);

    return AnimatedSwitcher(
      duration: duration,
      switchInCurve: Curves.easeOut,
      switchOutCurve: Curves.easeOut,
      child: SizedBox.expand(
        // Key by media ID so a day change drives the crossfade.
        key: ValueKey(mediaId),
        child: _layer(context, ref),
      ),
    );
  }

  Widget _layer(BuildContext context, WidgetRef ref) {
    Widget fallback() => ColoredBox(color: AppColors.primary(context));
    if (mediaId.isEmpty) return fallback();
    final mediaAsync = ref.watch(mediaObjectProvider(mediaId));
    return mediaAsync.when(
      data: (media) {
        if (media.url.isEmpty) return fallback();
        return CachedMediaImage(
          semanticsLabel: null,
          imageUrl: media.url,
          cacheKey: ImageCacheKeys.thumbnail(mediaId),
          width: double.infinity,
          height: double.infinity,
          fit: BoxFit.cover,
        );
      },
      loading: () => ColoredBox(color: AppColors.surface(context)),
      error: (_, _) => fallback(),
    );
  }
}
