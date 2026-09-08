import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/services/providers/media_providers.dart';

/// CarouselThumbnailStrip renders the horizontal row of thumbnail images /
/// videos at the bottom of the carousel, including the optional "add" slot.
///
/// The strip supports reorder-mode (long-press to drag) when [canReorder] is
/// true and [isEditMode] is true.
class CarouselThumbnailStrip extends StatelessWidget {
  final List<MediaItemData> items;
  final int currentIndex;
  final bool isEditMode;
  final bool isAddingMedia;
  final bool canReorder;
  final bool showAddButton;
  final void Function(int index) onThumbnailTap;
  // Receives indices from ReorderableListView.onReorderItem: newIndex is already
  // adjusted for the removed item, so callers must not decrement it.
  final void Function(int oldIndex, int newIndex) onReorderItem;
  final void Function(String mediaId) onDeleteThumbnail;
  final VoidCallback onAddTap;
  final VoidCallback onLongPress;

  const CarouselThumbnailStrip({
    super.key,
    required this.items,
    required this.currentIndex,
    required this.isEditMode,
    required this.isAddingMedia,
    required this.canReorder,
    required this.showAddButton,
    required this.onThumbnailTap,
    required this.onReorderItem,
    required this.onDeleteThumbnail,
    required this.onAddTap,
    required this.onLongPress,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(0, 12, 0, 16),
      child: SizedBox(
        height: 80,
        child: _buildReorderableRow(context),
      ),
    );
  }

  Widget _buildReorderableRow(BuildContext context) {
    final inEditMode = isEditMode && canReorder;
    final children = List.generate(items.length, (index) {
      final thumb = _CarouselThumbnail(
        item: items[index],
        index: index,
        isSelected: currentIndex == index,
        isEditMode: inEditMode,
        onTap: () => onThumbnailTap(index),
        onLongPress: canReorder ? onLongPress : null,
        onDelete: () => onDeleteThumbnail(items[index].id),
      );
      if (inEditMode) {
        return ReorderableDragStartListener(
          key: ValueKey(items[index].id),
          index: index,
          child: thumb,
        );
      }
      return SizedBox(key: ValueKey(items[index].id), child: thumb);
    });

    return ReorderableListView(
      scrollDirection: Axis.horizontal,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      buildDefaultDragHandles: false,
      onReorderItem: inEditMode ? onReorderItem : (_, _) {},
      anchor: 0,
      footer: showAddButton
          ? _AddThumbnail(
              isAddingMedia: isAddingMedia,
              onTap: onAddTap,
            )
          : null,
      proxyDecorator: (child, index, animation) => child,
      children: children,
    );
  }
}

/// _CarouselThumbnail renders a single thumbnail in the strip.
///
/// For items with a server-side `mediaId`, the strip resolves a fresh
/// thumbnail-preferred URL via `mediaObjectProvider` rather than reusing
/// the captured `item.url` — which for video items is the video URL
/// itself, and which CachedNetworkImage cannot decode as an image. This
/// keeps the strip rendering for both photos and videos (#1833 follow-up).
class _CarouselThumbnail extends ConsumerWidget {
  final MediaItemData item;
  final int index;
  final bool isSelected;
  final bool isEditMode;
  final VoidCallback onTap;
  final VoidCallback? onLongPress;
  final VoidCallback onDelete;

  const _CarouselThumbnail({
    required this.item,
    required this.index,
    required this.isSelected,
    required this.isEditMode,
    required this.onTap,
    required this.onLongPress,
    required this.onDelete,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final label = (item.attribution != null &&
            item.attribution!.creatorName.isNotEmpty)
        ? item.attribution!.creatorName
        : (item.uploader?.name ?? '');

    return Tappable(
      semanticsLabel: context.l10n.a11yMediaThumbnail,
      onTap: onTap,
      onLongPress: onLongPress,
      excludeChildSemantics: false,
      child: Padding(
        padding: const EdgeInsets.only(right: 8),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Opacity(
              opacity: isSelected ? 1.0 : 0.55,
              child: Container(
                width: 60,
                height: 60,
                decoration: BoxDecoration(
                  border: Border.all(
                    color: isSelected
                        ? AppColors.primary(context)
                        : Colors.transparent,
                    width: 2.5,
                  ),
                  borderRadius: BorderRadius.circular(12),
                  boxShadow: isSelected
                      ? [
                          BoxShadow(
                            color: AppColors.primary(context)
                                .withValues(alpha: 0.25),
                            blurRadius: 12,
                          ),
                        ]
                      : null,
                ),
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(9.5),
                  child: Stack(
                    fit: StackFit.expand,
                    children: [
                      // Image uploads: render the local file as the optimistic
                      // preview. Videos: Image.file can't decode video bytes
                      // ("Invalid image data") and would leave the tile blank,
                      // so we fall back to a dark placeholder until the server
                      // returns the extracted-frame thumbnail. The play-icon
                      // overlay below makes the tile read as a video either
                      // way.
                      if (item.isUploading &&
                          item.localPath != null &&
                          !item.isVideo)
                        Image.file(
                          File(item.localPath!),
                          fit: BoxFit.cover,
                        )
                      else if (item.isUploading && item.isVideo)
                        Container(color: Colors.black)
                      else
                        _ThumbnailNetworkImage(item: item, ref: ref),
                      if (item.isVideo)
                        Center(
                          child: Container(
                            width: 20,
                            height: 20,
                            decoration: const BoxDecoration(
                              shape: BoxShape.circle,
                              color: Color(0xD9FFFFFF),
                            ),
                            child: const Icon(
                              Icons.play_arrow,
                              color: Colors.black87,
                              size: 13,
                            ),
                          ),
                        ),
                      if (item.isUploading)
                        Container(
                          color: Colors.black54,
                          child: const Center(
                            child: CircularProgressIndicator(
                              strokeWidth: 2,
                              color: Colors.white,
                            ),
                          ),
                        ),
                      if (isEditMode && !item.isUploading)
                        Positioned(
                          top: 3,
                          right: 3,
                          child: Tappable(
                            semanticsLabel:
                                context.l10n.a11yMediaDeleteThumbnail,
                            onTap: onDelete,
                            child: Container(
                              padding: const EdgeInsets.all(3),
                              decoration: const BoxDecoration(
                                color: Colors.red,
                                shape: BoxShape.circle,
                              ),
                              child: const Icon(
                                Icons.close,
                                color: Colors.white,
                                size: 12,
                              ),
                            ),
                          ),
                        ),
                    ],
                  ),
                ),
              ),
            ),
            const SizedBox(height: 4),
            SizedBox(
              width: 60,
              child: Text(
                label,
                textAlign: TextAlign.center,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 9,
                  color: Colors.white,
                  decoration: TextDecoration.none,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// _ThumbnailNetworkImage resolves a thumbnail-preferred URL through
/// [mediaObjectProvider] for items with a [MediaItemData.id], so video
/// items render their thumbnail bytes (not the un-decodable video bytes)
/// and image items get a freshly-signed URL on each carousel open. Falls
/// back to [item.url] when no `mediaId` is available (locally-staged
/// uploads that haven't reached the server yet).
class _ThumbnailNetworkImage extends StatelessWidget {
  final MediaItemData item;
  final WidgetRef ref;

  const _ThumbnailNetworkImage({required this.item, required this.ref});

  @override
  Widget build(BuildContext context) {
    final mediaId = item.id;
    if (mediaId.isEmpty) {
      return CachedMediaImage(
        imageUrl: item.url,
        fit: BoxFit.cover,
        semanticsLabel: context.l10n.a11yMediaThumbnail,
        placeholder: _buildPlaceholder(),
        errorWidget: _buildError(),
      );
    }
    final async = ref.watch(mediaObjectProvider(mediaId));
    return async.when(
      loading: () => _buildPlaceholder(),
      error: (_, _) => _buildError(),
      data: (mediaUrl) => CachedMediaImage(
        imageUrl: mediaUrl.url,
        cacheKey: mediaUrl.cacheKey,
        fit: BoxFit.cover,
        semanticsLabel: context.l10n.a11yMediaThumbnail,
        placeholder: _buildPlaceholder(),
        errorWidget: _buildError(),
      ),
    );
  }

  Widget _buildPlaceholder() => Container(
        color: Colors.grey[800],
        child: const Center(
          child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
        ),
      );

  Widget _buildError() => Container(
        color: Colors.grey[800],
        child: const Icon(Icons.error_outline, color: Colors.white, size: 20),
      );
}

/// _AddThumbnail renders the "add media" slot at the end of the thumbnail
/// strip.
class _AddThumbnail extends StatelessWidget {
  final bool isAddingMedia;
  final VoidCallback onTap;

  const _AddThumbnail({required this.isAddingMedia, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yMediaAddItem,
      onTap: isAddingMedia ? null : onTap,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 60,
            height: 60,
            decoration: BoxDecoration(
              border: Border.all(
                color: Colors.white.withValues(alpha: 0.2),
                style: BorderStyle.solid,
                width: 1.5,
              ),
              borderRadius: BorderRadius.circular(12),
            ),
            child: Center(
              child: isAddingMedia
                  ? const SizedBox(
                      width: 20,
                      height: 20,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: Colors.white,
                      ),
                    )
                  : Icon(
                      Icons.add,
                      color: Colors.white.withValues(alpha: 0.35),
                      size: 22,
                    ),
            ),
          ),
          // Spacer to match the label height of adjacent thumbnails.
          const SizedBox(height: 4 + 12),
        ],
      ),
    );
  }
}
