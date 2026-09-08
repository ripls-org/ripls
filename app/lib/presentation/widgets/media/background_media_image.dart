import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';

/// Displays a full-screen background image from a media ID using proper caching.
///
/// This widget encapsulates the correct pattern for displaying user-uploaded media
/// with presigned URLs. It uses [CachedMediaImage] with stable cache keys based on
/// media ID to prevent caching bugs where different media items could show the wrong
/// cached image.
///
/// **Why this widget exists:**
/// Previously, different preview modals (gear, request, experience) implemented this
/// pattern independently using `Image.network`, which caused caching bugs with presigned
/// URLs. This shared widget ensures consistency and correctness across the codebase.
///
/// **Usage:**
/// ```dart
/// BackgroundMediaImage(
///   mediaId: mediaId,
///   getMediaUrl: () => viewModel.getMediaUrl(mediaId),
/// )
/// ```
///
/// Or directly from repository:
/// ```dart
/// BackgroundMediaImage(
///   mediaId: mediaId,
///   getMediaUrl: () => ref.read(mediaRepositoryProvider).getMediaUrl(mediaId),
/// )
/// ```
///
/// See docs/ai/experience_image.md for full context on the caching bug this solves.
class BackgroundMediaImage extends ConsumerWidget {
  /// The media ID for the background image.
  ///
  /// Used as a stable key for the internal [FutureBuilder] so the URL is
  /// re-fetched exactly when the id changes, not on every unrelated parent
  /// rebuild.
  final String mediaId;

  /// Function that returns a `Future<MediaUrl>` for the background image.
  ///
  /// This can be a call to a ViewModel's getMediaUrl method or
  /// MediaRepository's getMediaUrl method.
  final Future<MediaUrl> Function() getMediaUrl;

  /// Optional BoxFit for the image. Defaults to BoxFit.cover.
  final BoxFit fit;

  const BackgroundMediaImage({
    super.key,
    required this.mediaId,
    required this.getMediaUrl,
    this.fit = BoxFit.cover,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return FutureBuilder<MediaUrl>(
      key: ValueKey('bg-$mediaId'),
      future: getMediaUrl(),
      builder: (context, snapshot) {
        if (snapshot.hasData) {
          final mediaUrl = snapshot.data!;
          return Positioned.fill(
            child: CachedMediaImage(
              // Decorative; the surrounding card carries the semantic label.
              semanticsLabel: null,
              key: ValueKey(mediaUrl.mediaId),
              imageUrl: mediaUrl.url,
              cacheKey: mediaUrl.cacheKey,
              fit: fit,
            ),
          );
        }
        return const SizedBox.shrink();
      },
    );
  }
}
