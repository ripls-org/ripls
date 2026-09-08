import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/media/media_carousel.dart';
import 'package:ripls/services/providers.dart';

/// MessageMediaAttachments renders the inline media thumbnails for a chat
/// message bubble.
///
/// Each media ID is resolved to a URL via the media repository. Tapping an
/// image invokes [onMediaTap] when provided; otherwise a single-item
/// [MediaCarousel] is shown inline.
class MessageMediaAttachments extends ConsumerWidget {
  const MessageMediaAttachments({
    super.key,
    required this.mediaIds,
    this.onMediaTap,
  });

  final List<String> mediaIds;

  /// Called when the user taps a media thumbnail.
  ///
  /// When provided, the caller is responsible for opening the viewer.
  /// When null, tapping opens a single-item [MediaCarousel].
  final void Function(String mediaId)? onMediaTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: mediaIds.map((mediaId) {
        return FutureBuilder(
          future: ref.read(mediaRepositoryProvider).getMediaUrl(mediaId),
          builder: (context, snapshot) {
            if (snapshot.connectionState == ConnectionState.waiting) {
              return const SizedBox(
                height: 200,
                child: Center(child: CircularProgressIndicator()),
              );
            }

            if (snapshot.hasError || !snapshot.hasData) {
              return Container(
                height: 200,
                color: Colors.grey.shade200,
                child: const Center(
                  child: Icon(Icons.error_outline, color: Colors.grey),
                ),
              );
            }

            final mediaUrl = snapshot.data!;
            final isVideo =
                mediaUrl.contentType?.startsWith('video/') ?? false;
            return Tappable(
              semanticsLabel: context.l10n.a11yChatViewMedia,
              onTap: () {
                if (onMediaTap != null) {
                  onMediaTap!(mediaId);
                } else {
                  MediaCarousel.show(
                    context: context,
                    getMediaItems: () => [
                      MediaItem(
                        url: mediaUrl.url,
                        isVideo: isVideo,
                        mediaId: mediaId,
                      ),
                    ],
                    initialIndex: 0,
                  );
                }
              },
              child: Stack(
                alignment: Alignment.center,
                children: [
                  CachedNetworkImage(
                    imageUrl: mediaUrl.url,
                    cacheKey: ImageCacheKeys.thumbnail(mediaId),
                    height: 200,
                    width: double.infinity,
                    fit: BoxFit.cover,
                    placeholder: (context, url) => const SizedBox(
                      height: 200,
                      child: Center(child: CircularProgressIndicator()),
                    ),
                    errorWidget: (context, url, error) => SizedBox(
                      height: 200,
                      child: Center(
                        child: Icon(Icons.error_outline, color: Colors.grey),
                      ),
                    ),
                  ),
                  if (isVideo)
                    Container(
                      width: 48,
                      height: 48,
                      decoration: BoxDecoration(
                        color: Colors.black.withValues(alpha: 0.55),
                        shape: BoxShape.circle,
                      ),
                      child: const Icon(
                        Icons.play_arrow,
                        color: Colors.white,
                        size: 30,
                      ),
                    ),
                ],
              ),
            );
          },
        );
      }).toList(),
    );
  }
}
