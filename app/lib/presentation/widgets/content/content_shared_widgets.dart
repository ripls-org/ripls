import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/content/linkified_text.dart';

/// ContentOverflowButton is a frosted-glass (···) pill pinned to the top-right
/// corner of a content view Stack. Rendered as the last Stack child so it is
/// always above every other layer, including the feed header and chat overlay.
class ContentOverflowButton extends StatelessWidget {
  final VoidCallback onTap;

  const ContentOverflowButton({super.key, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final topPadding = MediaQuery.of(context).padding.top;
    return Positioned(
      top: topPadding + 12,
      right: 16,
      child: Tappable(
        semanticsLabel: context.l10n.a11yContentOverflowMenu,
        onTap: onTap,
        child: ClipOval(
          child: BackdropFilter(
            filter: ImageFilter.blur(sigmaX: 10, sigmaY: 10),
            child: Container(
              width: 32,
              height: 32,
              decoration: BoxDecoration(
                color: GlassTokens.scrimTint,
                shape: BoxShape.circle,
                border: Border.all(
                  color: GlassTokens.borderSoft,
                ),
              ),
              child: const Center(
                child: Text(
                  '···',
                  style: TextStyle(
                    color: GlassTokens.textPrimary,
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 1,
                    height: 1,
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// ConversationPaneEmptyState renders the "No conversation yet" placeholder
/// shown in the chat tab of a content view (gear, experience, request) when
/// the underlying entity has no conversation thread yet.
class ConversationPaneEmptyState extends StatelessWidget {
  final String message;

  const ConversationPaneEmptyState({super.key, this.message = 'No conversation yet'});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Text(
        message,
        style: TextStyle(
          color: GlassTokens.textFaint,
          fontSize: 13,
        ),
      ),
    );
  }
}

/// ContentExpandableDescription shows body text truncated to [maxLines] lines
/// (default 4). Tapping toggles between collapsed and fully expanded.
///
/// [style] overrides the default `bodyLarge` + opaque white styling. Pass a
/// custom style for surfaces with different contrast (e.g. media-backed panes
/// where text needs a softer alpha).
class ContentExpandableDescription extends StatefulWidget {
  final String description;
  final int maxLines;
  final TextStyle? style;

  const ContentExpandableDescription({
    super.key,
    required this.description,
    this.maxLines = 4,
    this.style,
  });

  @override
  State<ContentExpandableDescription> createState() =>
      _ContentExpandableDescriptionState();
}

class _ContentExpandableDescriptionState
    extends State<ContentExpandableDescription> {
  bool _expanded = false;

  @override
  Widget build(BuildContext context) {
    if (widget.description.isEmpty) return const SizedBox.shrink();

    final defaultStyle = Theme.of(context).textTheme.bodyLarge!.copyWith(
          color: GlassTokens.textPrimary,
          height: 1.55,
        );

    return Tappable(
      semanticsLabel: _expanded
          ? context.l10n.a11yHideDetails
          : context.l10n.a11yShowDetails,
      excludeChildSemantics: false,
      onTap: () => setState(() => _expanded = !_expanded),
      child: LinkifiedText(
        text: widget.description,
        maxLines: _expanded ? null : widget.maxLines,
        overflow: _expanded ? TextOverflow.visible : TextOverflow.ellipsis,
        style: widget.style ?? defaultStyle,
      ),
    );
  }
}

/// ContentLocationCard displays a pickup location with a coral circle icon,
/// location name, optional city/state subtitle, and a trailing chevron.
/// Always tappable (calls [onTap] in both view and edit modes).
class ContentLocationCard extends StatelessWidget {
  final String locationName;

  /// Pre-formatted city/state string, e.g. "San Francisco, CA". Empty = hidden.
  final String cityState;
  final VoidCallback onTap;

  const ContentLocationCard({
    super.key,
    required this.locationName,
    required this.cityState,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: locationName,
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 4),
        child: Row(
          children: [
            Container(
              width: 32,
              height: 32,
              decoration: BoxDecoration(
                color: AppColors.transferCoral.withValues(alpha: 0.15),
                shape: BoxShape.circle,
              ),
              child: const Icon(
                Icons.location_on,
                size: 17,
                color: AppColors.transferCoral,
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    'PICKUP LOCATION',
                    style: TextStyle(
                      fontSize: 9,
                      fontWeight: FontWeight.w600,
                      color: GlassTokens.textFaint,
                      letterSpacing: 1,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    locationName,
                    style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                      color: GlassTokens.textPrimary,
                      height: 1.25,
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                  if (cityState.isNotEmpty)
                    Text(
                      cityState,
                      style: TextStyle(
                        fontSize: 11,
                        color: GlassTokens.textFaint,
                      ),
                    ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            Icon(
              Icons.chevron_right,
              size: 18,
              color: GlassTokens.textFaint,
            ),
          ],
        ),
      ),
    );
  }
}

/// ContentDetailRow displays a single label/value pair with a white label on
/// the left and white bold value on the right, separated by a Spacer.
class ContentDetailRow extends StatelessWidget {
  final String label;
  final String value;

  const ContentDetailRow({super.key, required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        children: [
          Text(
            label,
            style: const TextStyle(fontSize: 13, color: GlassTokens.textPrimary),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              value,
              textAlign: TextAlign.right,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w700,
                color: GlassTokens.textPrimary,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// ContentDetailRows renders a column of [ContentDetailRow]s separated by
/// thin white dividers. An optional [title] is rendered above the rows as a
/// small all-caps label (same treatment used by gear metadata cards).
class ContentDetailRows extends StatelessWidget {
  /// Each tuple is (label, value).
  final List<(String, String)> rows;

  /// Optional section header rendered above the rows in small all-caps style.
  final String? title;

  const ContentDetailRows({super.key, required this.rows, this.title});

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (title != null) ...[
          Padding(
            padding: const EdgeInsets.only(bottom: 4),
            child: Text(
              title!,
              style: TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w600,
                letterSpacing: 1.2,
                color: GlassTokens.textFaint,
              ),
            ),
          ),
        ],
        for (int i = 0; i < rows.length; i++) ...[
          ContentDetailRow(label: rows[i].$1, value: rows[i].$2),
          if (i < rows.length - 1)
            Divider(height: 1, color: GlassTokens.hairline),
        ],
      ],
    );
  }
}

/// ContentDetailCard wraps [ContentDetailRows] in the standard frosted dark
/// card used by gear metadata and experience QT attribute sections.
///
/// Pass [title] to render an all-caps section header above the card.
class ContentDetailCard extends StatelessWidget {
  final List<(String, String)> rows;
  final String? title;

  const ContentDetailCard({super.key, required this.rows, this.title});

  @override
  Widget build(BuildContext context) {
    if (rows.isEmpty) return const SizedBox.shrink();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (title != null) ...[
          Text(
            title!,
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w600,
              letterSpacing: 1.2,
              color: GlassTokens.textFaint,
            ),
          ),
          const SizedBox(height: 8),
        ],
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 14),
          decoration: BoxDecoration(
            color: OverlayTokens.fieldFill,
            borderRadius: BorderRadius.circular(16),
            border: Border.all(color: GlassTokens.hairline),
          ),
          child: ContentDetailRows(rows: rows),
        ),
      ],
    );
  }
}

/// ContentMediaCarousel displays a horizontal 94px-tall strip of 72×72
/// thumbnails with numbered labels. In edit mode an add-cell appears at the
/// end and each thumbnail shows a delete badge.
///
/// When [showAddButton] is true (e.g. for the content owner in read mode), the
/// add-cell is shown after the last thumbnail without requiring edit mode.
///
/// When [onReorderItems] is provided and [isEditing] is true, thumbnails
/// support long-press drag-to-reorder via [ReorderableListView].
class ContentMediaCarousel extends ConsumerWidget {
  final List<MediaItemData> items;
  final int selectedIndex;
  final bool isEditing;

  /// When true, an add (+) cell is shown after the last thumbnail even outside
  /// of edit mode. Intended for content owners who want quick access to add
  /// media without entering edit mode.
  final bool showAddButton;

  final void Function(int index) onTapItem;
  final void Function(String mediaId)? onDeleteItem;
  final VoidCallback? onAddItem;

  /// Called with the new ordered list of media IDs when the user drops a
  /// thumbnail in a new position. Only active when [isEditing] is true.
  final Future<void> Function(List<String> mediaIds)? onReorderItems;

  const ContentMediaCarousel({
    super.key,
    required this.items,
    required this.selectedIndex,
    required this.isEditing,
    required this.onTapItem,
    this.showAddButton = false,
    this.onDeleteItem,
    this.onAddItem,
    this.onReorderItems,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    if (isEditing && onReorderItems != null) {
      return _buildReorderableList();
    }

    final shouldShowAdd = isEditing || showAddButton;
    final itemCount = items.length + (shouldShowAdd ? 1 : 0);
    return SizedBox(
      // Thumbnail (72) + gap (4) + label text (~16 at default line height)
      height: 94,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemCount: itemCount,
        separatorBuilder: (_, _) => const SizedBox(width: 8),
        itemBuilder: (ctx, i) {
          if (shouldShowAdd && i == items.length) {
            return _AddMediaCell(onTap: onAddItem ?? () {});
          }
          final media = items[i];
          return _MediaThumbnailCell(
            mediaUrl: media.url,
            mediaId: media.id,
            label: '${i + 1}',
            isVideo: media.isVideo,
            isSelected: i == selectedIndex,
            isEditing: isEditing,
            onTap: () => onTapItem(i),
            onDelete: isEditing ? () => onDeleteItem?.call(media.id) : null,
          );
        },
      ),
    );
  }

  Widget _buildReorderableList() {
    final hasAdd = onAddItem != null;
    final itemCount = items.length + (hasAdd ? 1 : 0);

    return SizedBox(
      height: 94,
      child: ReorderableListView.builder(
        scrollDirection: Axis.horizontal,
        buildDefaultDragHandles: false,
        proxyDecorator: (child, index, animation) => Material(
          elevation: 6,
          color: Colors.transparent,
          child: child,
        ),
        itemCount: itemCount,
        itemBuilder: (ctx, i) {
          // Add cell — not draggable, always last
          if (hasAdd && i == items.length) {
            return Padding(
              key: const ValueKey('__add__'),
              padding: const EdgeInsets.only(right: 8),
              child: _AddMediaCell(onTap: onAddItem!),
            );
          }
          final media = items[i];
          return ReorderableDelayedDragStartListener(
            key: ValueKey(media.id),
            index: i,
            child: Padding(
              padding: const EdgeInsets.only(right: 8),
              child: _MediaThumbnailCell(
                mediaUrl: media.url,
                mediaId: media.id,
                label: '${i + 1}',
                isVideo: media.isVideo,
                isSelected: false,
                isEditing: true,
                onTap: () {},
                onDelete: () => onDeleteItem?.call(media.id),
              ),
            ),
          );
        },
        onReorderItem: (oldIndex, newIndex) {
          // Ignore drags involving the add cell. onReorderItem already adjusts
          // newIndex for the removed item, so no manual decrement is needed.
          if (oldIndex >= items.length) return;
          if (newIndex >= items.length) newIndex = items.length - 1;
          final reordered = [...items];
          final moved = reordered.removeAt(oldIndex);
          reordered.insert(newIndex, moved);
          onReorderItems?.call(reordered.map((m) => m.id).toList());
        },
      ),
    );
  }
}

/// ContentMetricTile is a dark card showing a large numeric value and a
/// small label below it. Used in experience and request details panes.
class ContentMetricTile extends StatelessWidget {
  final String value;
  final String label;

  const ContentMetricTile({super.key, required this.value, required this.label});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 10),
      decoration: BoxDecoration(
        color: GlassTokens.scrimTint,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: GlassTokens.hairline),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            value,
            style: const TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w700,
              color: GlassTokens.textPrimary,
            ),
          ),
          const SizedBox(height: 2),
          Text(
            label,
            style: TextStyle(
              fontSize: 10,
              color: GlassTokens.textFaint,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Private helpers ───────────────────────────────────────────────────────────

class _MediaThumbnailCell extends ConsumerWidget {
  final String mediaUrl;
  final String mediaId;
  final String label;
  final bool isVideo;
  final bool isSelected;
  final bool isEditing;
  final VoidCallback onTap;
  final VoidCallback? onDelete;

  const _MediaThumbnailCell({
    required this.mediaUrl,
    required this.mediaId,
    required this.label,
    required this.isVideo,
    required this.isSelected,
    required this.isEditing,
    required this.onTap,
    this.onDelete,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Tappable(
      semanticsLabel: context.l10n.a11yContentMediaThumbnail(label),
      onTap: onTap,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Stack(
            children: [
              Container(
                width: 72,
                height: 72,
                decoration: BoxDecoration(
                  borderRadius: BorderRadius.circular(10),
                  border: Border.all(
                    color: isSelected && !isEditing
                        ? GlassTokens.borderActive
                        : GlassTokens.borderSoft,
                    width: isSelected && !isEditing ? 2 : 1,
                  ),
                ),
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(9),
                  child: CachedMediaImage(
                    // Decorative; the surrounding card carries the semantic label.
                    semanticsLabel: null,imageUrl: mediaUrl,
                    // Intentionally separate disk cache entry from the full-image key
                    // ('media_$mediaId') used by MediaBackground — thumbnail and full
                    // resolution images are different sizes and must not share a cache slot.
                    cacheKey: 'thumbnail_$mediaId',
                    fit: BoxFit.cover,
                    width: 72,
                    height: 72,
                  ),
                ),
              ),
              // Video play icon overlay
              if (isVideo)
                Positioned.fill(
                  child: Center(
                    child: Container(
                      width: 28,
                      height: 28,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: OverlayTokens.scrimFloor,
                      ),
                      child: const Icon(
                        Icons.play_arrow,
                        color: GlassTokens.textPrimary,
                        size: 18,
                      ),
                    ),
                  ),
                ),
              if (isEditing && onDelete != null)
                Positioned(
                  top: 2,
                  right: 2,
                  child: Tappable(
                    semanticsLabel: context.l10n.a11yContentMediaDelete,
                    onTap: onDelete,
                    child: Container(
                      width: 18,
                      height: 18,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: Colors.red.withValues(alpha: 0.85),
                      ),
                      child: const Icon(
                        Icons.close,
                        color: GlassTokens.textPrimary,
                        size: 11,
                      ),
                    ),
                  ),
                ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            label,
            style: TextStyle(
              fontSize: 10,
              color: GlassTokens.textFaint,
            ),
          ),
        ],
      ),
    );
  }
}

class _AddMediaCell extends StatelessWidget {
  final VoidCallback onTap;

  const _AddMediaCell({required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yContentMediaAdd,
      onTap: onTap,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 72,
            height: 72,
            decoration: BoxDecoration(
              color: GlassTokens.fillFaint,
              borderRadius: BorderRadius.circular(10),
              border: Border.all(
                color: GlassTokens.borderSoft,
                width: 1.5,
              ),
            ),
            child: Center(
              child: Icon(
                Icons.add,
                color: GlassTokens.textFaint,
                size: 22,
              ),
            ),
          ),
          const SizedBox(height: 4),
          Text(
            'Add',
            style: TextStyle(
              fontSize: 10,
              color: GlassTokens.textFaint,
            ),
          ),
        ],
      ),
    );
  }
}
