import 'dart:io';

import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/presentation/viewmodels/conversation_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/media/media_carousel.dart'
    show MediaCarousel, MediaItem;

/// Horizontal scrollable row of thumbnails for staged (not yet sent) files.
Widget buildPendingAttachments(
  BuildContext context,
  WidgetRef ref,
  ConversationState convState,
  String conversationId,
) {
  final attachments = convState.pendingAttachments;
  final notifier = ref.read(conversationProvider(conversationId).notifier);
  return SizedBox(
    height: 72,
    child: ListView.separated(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 4),
      scrollDirection: Axis.horizontal,
      itemCount: attachments.length,
      separatorBuilder: (_, _) => const SizedBox(width: 8),
      itemBuilder: (ctx, i) {
        return Stack(
          clipBehavior: Clip.none,
          children: [
            ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: Image.file(
                File(attachments[i]),
                width: 60,
                height: 60,
                fit: BoxFit.cover,
                errorBuilder: (_, _, _) => Container(
                  width: 60,
                  height: 60,
                  color: Colors.grey[800],
                  child: const Icon(
                    Icons.broken_image,
                    color: Colors.white54,
                    size: 20,
                  ),
                ),
              ),
            ),
            // Remove button
            if (!convState.isSending)
              Positioned(
                top: -6,
                right: -6,
                child: Tappable(
                  semanticsLabel: context.l10n.a11yContentRemoveAttachment,
                  onTap: () => notifier.removePendingAttachment(i),
                  child: Container(
                    width: 20,
                    height: 20,
                    decoration: const BoxDecoration(
                      shape: BoxShape.circle,
                      color: Colors.black87,
                    ),
                    child: const Icon(
                      Icons.close,
                      size: 12,
                      color: Colors.white,
                    ),
                  ),
                ),
              ),
          ],
        );
      },
    ),
  );
}

/// Upload progress bar shown while sending attachments.
Widget buildUploadProgress(
  ConversationState convState,
  Color accentColor,
) {
  final progress = convState.uploadTotal > 0
      ? convState.uploadProgress / convState.uploadTotal
      : 0.0;
  return Padding(
    padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(2),
          child: LinearProgressIndicator(
            value: progress,
            minHeight: 3,
            backgroundColor: Colors.white.withValues(alpha: 0.15),
            valueColor: AlwaysStoppedAnimation<Color>(accentColor),
          ),
        ),
        const SizedBox(height: 2),
        Text(
          'Uploading ${convState.uploadProgress} of ${convState.uploadTotal}...',
          style: TextStyle(
            color: Colors.white.withValues(alpha: 0.55),
            fontSize: 11,
          ),
        ),
      ],
    ),
  );
}

/// buildMediaStrip renders a horizontal row of compact thumbnails below
/// the input bar. Tapping a thumbnail opens the full-screen media viewer.
///
/// If [getCarouselMediaItems] is provided, it is forwarded to the
/// [MediaCarousel] in place of capturing [mediaItems]. Use this when callers
/// need the carousel to refresh against live state (e.g. after an upload from
/// the carousel's own add button) — capturing [mediaItems] freezes the list
/// at strip-build time, so the carousel's `_refreshMediaItems()` would keep
/// reading a stale snapshot. A stable method tear-off on the parent State
/// rereads the latest `widget.mediaItems` on every invocation.
Widget buildMediaStrip(
  BuildContext context, {
  required List<MediaItemData> mediaItems,
  required Future<void> Function()? onAddMedia,
  required Future<void> Function(String)? onDeleteMedia,
  required Future<void> Function(List<String>)? onReorderMedia,
  List<MediaItem> Function()? getCarouselMediaItems,
}) {
  List<MediaItem> defaultGetItems() => mediaItems
      .map(
        (m) => MediaItem(
          url: m.url,
          isVideo: m.isVideo,
          contentType: m.contentType,
          mediaId: m.id,
          attribution: m.attribution,
          uploader: m.uploader,
          uploadedAtUnixSec: m.uploadedAtUnixSec,
        ),
      )
      .toList();
  final getItems = getCarouselMediaItems ?? defaultGetItems;

  return Tappable(
    semanticsLabel: context.l10n.a11yContentMediaStrip,
    excludeChildSemantics: false,
    onTap: () => MediaCarousel.show(
      context: context,
      initialIndex: 0,
      getMediaItems: getItems,
      onAddMedia: onAddMedia,
      onDeleteMedia: onDeleteMedia,
      onReorderMedia: onReorderMedia,
    ),
    child: SizedBox(
      height: 76,
      child: ListView.separated(
        padding: const EdgeInsets.fromLTRB(12, 0, 12, 8),
        scrollDirection: Axis.horizontal,
        itemCount: mediaItems.length,
        separatorBuilder: (_, _) => const SizedBox(width: 8),
        itemBuilder: (ctx, i) {
          final media = mediaItems[i];
          final label = (media.attribution != null &&
                  media.attribution!.creatorName.isNotEmpty)
              ? media.attribution!.creatorName
              : (media.uploader?.name ?? '');
          return Tappable(
            semanticsLabel: context.l10n.a11yContentMediaThumbnail(label),
            onTap: () => MediaCarousel.show(
              context: context,
              initialIndex: i,
              getMediaItems: getItems,
              onAddMedia: onAddMedia,
              onDeleteMedia: onDeleteMedia,
              onReorderMedia: onReorderMedia,
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 48,
                  height: 48,
                  decoration: BoxDecoration(
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: i == 0
                          ? AppColors.primary(ctx)
                          : Colors.white.withValues(alpha: 0.15),
                      width: i == 0 ? 2.0 : 1.0,
                    ),
                  ),
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(5),
                    child: Stack(
                      fit: StackFit.expand,
                      children: [
                        CachedNetworkImage(
                          imageUrl: media.url,
                          cacheKey: ImageCacheKeys.thumbnail(media.id),
                          fit: BoxFit.cover,
                          fadeInDuration: Duration.zero,
                          placeholder: (_, _) =>
                              Container(color: Colors.grey[800]),
                          errorWidget: (_, _, _) =>
                              Container(color: Colors.grey[800]),
                        ),
                        if (media.isVideo)
                          const Center(
                            child: Icon(
                              Icons.play_circle_outline,
                              color: Colors.white,
                              size: 22,
                            ),
                          ),
                      ],
                    ),
                  ),
                ),
                const SizedBox(height: 4),
                SizedBox(
                  width: 48,
                  child: Text(
                    label,
                    textAlign: TextAlign.center,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 9,
                      color: Colors.white,
                    ),
                  ),
                ),
              ],
            ),
          );
        },
      ),
    ),
  );
}
