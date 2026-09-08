import 'dart:io';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:video_player/video_player.dart';

final _log = Logger('MediaBackground');

/// MediaBackground displays media (image or video) as a full-screen background.
/// Supports both local file paths and video player controllers.
///
/// Rendering priority:
///   1. Initialized [VideoPlayerController] (live video).
///   2. [thumbnailUrl] — server-generated JPEG thumbnail shown while the video
///      controller is being initialized. Uses the `media:thumb:` cache key so it
///      shares the warm slot already populated by carousel display.
///   3. Non-video [mediaPath] (full-resolution image).
///   4. Black fallback (no media, or video without thumbnail yet).
class MediaBackground extends StatelessWidget {
  final String? mediaPath;
  final bool isVideo;
  final VideoPlayerController? videoController;
  final String? mediaId;
  // Thumbnail shown as static frame while the video controller initializes.
  final String? thumbnailUrl;

  const MediaBackground({
    super.key,
    this.mediaPath,
    this.isVideo = false,
    this.videoController,
    this.mediaId,
    this.thumbnailUrl,
  });

  @override
  Widget build(BuildContext context) {
    return Container(color: Colors.black, child: _buildMediaContent());
  }

  Widget _buildMediaContent() {
    // Priority 1: initialized video controller.
    if (isVideo &&
        videoController != null &&
        videoController!.value.isInitialized) {
      _log.info(
        '🎥 Showing video player - initialized: ${videoController!.value.isInitialized}',
      );
      return SizedBox.expand(
        child: FittedBox(
          fit: BoxFit.cover,
          child: SizedBox(
            width: videoController!.value.size.width,
            height: videoController!.value.size.height,
            child: VideoPlayer(videoController!),
          ),
        ),
      );
    }

    // Priority 2: thumbnail URL (static first-frame while video loads, or
    // legacy data where no thumbnail exists falls through to priority 4).
    if (thumbnailUrl != null && thumbnailUrl!.isNotEmpty) {
      return CachedNetworkImage(
        imageUrl: thumbnailUrl!,
        cacheKey: ImageCacheKeys.thumbnail(mediaId),
        fit: BoxFit.cover,
        alignment: Alignment.topCenter,
        width: double.infinity,
        height: double.infinity,
        fadeInDuration: Duration.zero,
        placeholder: (context, url) => const SizedBox.shrink(),
        errorWidget: (context, url, error) {
          _log.warning('⚠️ Error loading thumbnail: $error');
          return const SizedBox.shrink();
        },
      );
    }

    // Priority 3: full-resolution image background.
    if (mediaPath != null && !isVideo) {
      final isUrl =
          mediaPath!.startsWith('http://') || mediaPath!.startsWith('https://');

      if (isUrl) {
        return CachedNetworkImage(
          imageUrl: mediaPath!,
          cacheKey: ImageCacheKeys.full(mediaId),
          fit: BoxFit.cover,
          alignment: Alignment.topCenter,
          width: double.infinity,
          height: double.infinity,
          fadeInDuration: Duration.zero,
          placeholder: (context, url) => const SizedBox.shrink(),
          errorWidget: (context, url, error) {
            _log.severe('❌ Error loading image: $error');
            return Container(
              color: Colors.red.withValues(alpha: 0.3),
              child: const Center(
                child: Icon(Icons.error, color: Colors.white, size: 48),
              ),
            );
          },
        );
      } else {
        return Image.file(
          File(mediaPath!),
          fit: BoxFit.cover,
          alignment: Alignment.topCenter,
          width: double.infinity,
          height: double.infinity,
          errorBuilder: (context, error, stackTrace) {
            _log.severe('❌ Error loading image: $error');
            return Container(
              color: Colors.red.withValues(alpha: 0.3),
              child: const Center(
                child: Icon(Icons.error, color: Colors.white, size: 48),
              ),
            );
          },
        );
      }
    }

    if (mediaPath == null && thumbnailUrl == null) {
      _log.info('⬛ No media path, showing black background');
    }

    // Priority 4: black fallback.
    return const SizedBox.shrink();
  }
}
