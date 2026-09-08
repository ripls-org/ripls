import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_avatar.dart';
import 'package:ripls/presentation/widgets/media/attribution_widget.dart';
import 'package:ripls/presentation/widgets/media/carousel/carousel_media_item.dart';

/// CarouselHeader renders the top bar with the back button, item count, and
/// Save All / Done action.
class CarouselHeader extends StatelessWidget {
  final int itemCount;
  final bool isEditMode;
  final bool isDownloading;
  final bool hasItems;
  final VoidCallback onClose;
  final VoidCallback onToggleEditMode;
  final VoidCallback onDownloadAll;

  const CarouselHeader({
    super.key,
    required this.itemCount,
    required this.isEditMode,
    required this.isDownloading,
    required this.hasItems,
    required this.onClose,
    required this.onToggleEditMode,
    required this.onDownloadAll,
  });

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 56,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
        child: Row(
          children: [
            IconAction(
              icon: Icons.arrow_back_ios_new,
              iconSize: 22,
              color: Colors.white,
              semanticsLabel: context.l10n.a11yClose,
              onPressed: onClose,
            ),
            Expanded(
              child: Text(
                '$itemCount ${itemCount == 1 ? 'item' : 'items'}',
                textAlign: TextAlign.center,
                style: const TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                  letterSpacing: -0.2,
                ),
              ),
            ),
            if (isEditMode)
              TextButton(
                onPressed: onToggleEditMode,
                child: Text(
                  context.l10n.commonDone,
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 15,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              )
            else if (hasItems)
              Tappable(
                semanticsLabel: context.l10n.a11yMediaSaveAll,
                onTap: isDownloading ? null : onDownloadAll,
                child: Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                  decoration: BoxDecoration(
                    border: Border.all(
                      color: Colors.white.withValues(alpha: 0.2),
                    ),
                    borderRadius: BorderRadius.circular(10),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      if (isDownloading)
                        const SizedBox(
                          width: 14,
                          height: 14,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: Colors.white,
                          ),
                        )
                      else
                        const Icon(
                          Icons.download_outlined,
                          color: Colors.white,
                          size: 14,
                        ),
                      const SizedBox(width: 5),
                      Text(
                        context.l10n.mediaSaveAll,
                        style: const TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.w600,
                          color: Colors.white,
                        ),
                      ),
                    ],
                  ),
                ),
              )
            else
              const SizedBox(width: 48),
          ],
        ),
      ),
    );
  }
}

/// CarouselContextBar renders the attribution and action bar between the
/// preview area and the thumbnail strip.
class CarouselContextBar extends StatelessWidget {
  final MediaItem item;
  final bool isSaving;
  final bool canDelete;
  final VoidCallback onSave;
  final VoidCallback onDelete;

  /// Optional mute/unmute toggle. Rendered as the right-most action in
  /// the row (after delete and save/download) when present, e.g. when the
  /// current item is a video with audio.
  final Widget? muteButton;

  const CarouselContextBar({
    super.key,
    required this.item,
    required this.isSaving,
    required this.canDelete,
    required this.onSave,
    required this.onDelete,
    this.muteButton,
  });

  @override
  Widget build(BuildContext context) {
    final attribution = item.attribution;

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.12),
        border: Border(
          top: BorderSide(color: Colors.white.withValues(alpha: 0.06)),
          bottom: BorderSide(color: Colors.white.withValues(alpha: 0.06)),
        ),
      ),
      child: Row(
        children: [
          Expanded(
            child: attribution != null
                ? AttributionWidget(
                    attribution: attribution,
                    compact: true,
                    textStyle: const TextStyle(
                      color: Colors.white,
                      fontSize: 12,
                      decoration: TextDecoration.none,
                    ),
                  )
                : _UploaderAttribution(item: item),
          ),
          if (item.mediaId != null &&
              item.mediaId!.isNotEmpty &&
              canDelete) ...[
            const SizedBox(width: 8),
            Tappable(
              semanticsLabel: context.l10n.a11yMediaDelete,
              onTap: onDelete,
              child: Container(
                padding: const EdgeInsets.all(8),
                decoration: BoxDecoration(
                  color: AppColors.statusErrorOnDark.withValues(alpha: 0.15),
                  border: Border.all(
                    color: AppColors.statusErrorOnDark.withValues(alpha: 0.25),
                  ),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: const Icon(
                  Icons.delete_outline,
                  color: AppColors.statusErrorOnDark,
                  size: 16,
                ),
              ),
            ),
          ],
          if (item.mediaId != null) ...[
            const SizedBox(width: 8),
            Tappable(
              semanticsLabel: context.l10n.a11yMediaSave,
              onTap: isSaving ? null : onSave,
              child: Container(
                padding: const EdgeInsets.all(8),
                decoration: BoxDecoration(
                  color: Colors.white.withValues(alpha: 0.15),
                  border: Border.all(
                    color: Colors.white.withValues(alpha: 0.25),
                  ),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: isSaving
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : const Icon(
                        Icons.download_outlined,
                        color: Colors.white,
                        size: 16,
                      ),
              ),
            ),
          ],
          if (muteButton != null) ...[
            const SizedBox(width: 8),
            muteButton!,
          ],
        ],
      ),
    );
  }
}

/// _UploaderAttribution renders the avatar and display name (plus relative
/// upload time) for user-uploaded media.
class _UploaderAttribution extends StatelessWidget {
  final MediaItem item;

  const _UploaderAttribution({required this.item});

  @override
  Widget build(BuildContext context) {
    final uploader = item.uploader;
    final name = uploader?.name ?? '';
    final timeLabel = _formatUploadTime(item.uploadedAtUnixSec);

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (uploader != null && uploader.id.isNotEmpty) ...[
          ContentAvatar(user: uploader, size: 22),
          const SizedBox(width: 8),
        ],
        Flexible(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                name,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                  decoration: TextDecoration.none,
                ),
              ),
              if (timeLabel != null)
                Text(
                  timeLabel,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: 11,
                    color: Colors.white.withValues(alpha: 0.6),
                    decoration: TextDecoration.none,
                  ),
                ),
            ],
          ),
        ),
      ],
    );
  }

  /// _formatUploadTime returns a short relative time string for the given Unix
  /// timestamp in seconds, or null if the timestamp is absent.
  String? _formatUploadTime(int? uploadedAtUnixSec) {
    if (uploadedAtUnixSec == null) return null;
    final uploaded =
        DateTime.fromMillisecondsSinceEpoch(uploadedAtUnixSec * 1000);
    final diff = DateTime.now().difference(uploaded);
    if (diff.inSeconds < 60) return 'just now';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    if (diff.inDays < 7) return '${diff.inDays}d ago';
    if (diff.inDays < 30) return '${(diff.inDays / 7).floor()}w ago';
    if (diff.inDays < 365) return '${(diff.inDays / 30).floor()}mo ago';
    return '${(diff.inDays / 365).floor()}y ago';
  }
}
