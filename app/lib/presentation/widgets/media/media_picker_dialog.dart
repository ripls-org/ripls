import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart'
    show MediaCandidate;
import 'package:ripls/presentation/viewmodels/replace_media_slot.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/services/providers/media_providers.dart';

/// MediaPickerDialog shows a bottom sheet for selecting media source.
/// Offers options for video, photos, or camera.
///
/// When [candidates] is non-empty, renders a row of squircle thumbnails
/// above the action buttons — tap to swap that candidate into the
/// current media slot via [onCandidateTap]. Used by the unified-create
/// Replace Media flow; other call sites pass null and see the original
/// dialog shape unchanged.
class MediaPickerDialog {
  static void show({
    required BuildContext context,
    required bool hasMedia,
    required VoidCallback onVideoTap,
    required VoidCallback onPhotoTap,
    required VoidCallback onCameraTap,
    List<ReplaceMediaSlot>? candidates,
    void Function(int index)? onCandidateTap,
    int? candidateImportingIndex,
  }) {
    showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (BuildContext context) {
        return GlassSheet(
          applyMaxHeight: false,
          // Shown over full-bleed hero/preview media; the content scrim keeps
          // the light glass from washing out to a glowing white block on the
          // dark theme (#2724).
          scrim: AppColors.modalContentScrim,
          child: _MediaPickerContent(
            hasMedia: hasMedia,
            onVideoTap: onVideoTap,
            onPhotoTap: onPhotoTap,
            onCameraTap: onCameraTap,
            candidates: candidates,
            onCandidateTap: onCandidateTap,
            candidateImportingIndex: candidateImportingIndex,
          ),
        );
      },
    );
  }
}

class _MediaPickerContent extends StatelessWidget {
  final bool hasMedia;
  final VoidCallback onVideoTap;
  final VoidCallback onPhotoTap;
  final VoidCallback onCameraTap;
  final List<ReplaceMediaSlot>? candidates;
  final void Function(int index)? onCandidateTap;
  final int? candidateImportingIndex;

  const _MediaPickerContent({
    required this.hasMedia,
    required this.onVideoTap,
    required this.onPhotoTap,
    required this.onCameraTap,
    this.candidates,
    this.onCandidateTap,
    this.candidateImportingIndex,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final title = hasMedia
        ? l10n.mediaPickerDialogTitleReplace
        : l10n.mediaPickerDialogTitleAdd;
    final cs = candidates ?? const <ReplaceMediaSlot>[];

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 4),
          child: Semantics(
            header: true,
            child: Text(
              title,
              style: ModalTheme.headerValueStyle.copyWith(
                color: AppColors.modalTextPrimary,
                fontSize: 17,
              ),
            ),
          ),
        ),
        if (cs.isNotEmpty) ...[
          const SizedBox(height: 16),
          _CandidateThumbnailRow(
            candidates: cs,
            onTap: onCandidateTap,
            importingIndex: candidateImportingIndex,
          ),
        ],
        const SizedBox(height: 16),
        GlassInlineAction(
          icon: Icons.video_library,
          text: l10n.mediaPickerDialogVideos,
          semanticsLabel: l10n.mediaPickerDialogVideos,
          onTap: () {
            Navigator.pop(context);
            onVideoTap();
          },
        ),
        const SizedBox(height: 8),
        GlassInlineAction(
          icon: Icons.photo_library,
          text: l10n.mediaPickerDialogPhotos,
          semanticsLabel: l10n.mediaPickerDialogPhotos,
          onTap: () {
            Navigator.pop(context);
            onPhotoTap();
          },
        ),
        const SizedBox(height: 8),
        GlassInlineAction(
          icon: Icons.camera_alt,
          text: l10n.mediaPickerDialogCamera,
          semanticsLabel: l10n.mediaPickerDialogCamera,
          onTap: () {
            Navigator.pop(context);
            onCameraTap();
          },
        ),
      ],
    );
  }
}

/// Horizontal row of squircle thumbnails for media candidates. Renders
/// up to 4 entries with a play-icon overlay on video candidates. Tap
/// fires [onTap] with the entry index; the importing entry shows an
/// inline spinner via [importingIndex].
class _CandidateThumbnailRow extends StatelessWidget {
  const _CandidateThumbnailRow({
    required this.candidates,
    required this.onTap,
    required this.importingIndex,
  });

  final List<ReplaceMediaSlot> candidates;
  final void Function(int index)? onTap;
  final int? importingIndex;

  static const int _maxRendered = 4;
  static const double _thumbSize = 72;

  @override
  Widget build(BuildContext context) {
    final shown = candidates.length > _maxRendered
        ? candidates.sublist(0, _maxRendered)
        : candidates;
    return SizedBox(
      height: _thumbSize,
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: List.generate(shown.length, (i) {
          final slot = shown[i];
          final isLoading = importingIndex == i;
          return _SlotThumbnail(
            slot: slot,
            index: i,
            isLoading: isLoading,
            enabled: onTap != null && importingIndex == null,
            onTap: onTap,
          );
        }),
      ),
    );
  }

}

/// Renders one squircle thumbnail for a [ReplaceMediaSlot]. Proto slots
/// fetch a thumbnail from the carried URL; media-id slots resolve via
/// [mediaUrlProvider] so a previously-active ripls media (the
/// swap-back target) shows its own thumbnail.
class _SlotThumbnail extends ConsumerWidget {
  const _SlotThumbnail({
    required this.slot,
    required this.index,
    required this.isLoading,
    required this.enabled,
    required this.onTap,
  });

  final ReplaceMediaSlot slot;
  final int index;
  final bool isLoading;
  final bool enabled;
  final void Function(int)? onTap;

  static const double _thumbSize = 72;
  static const double _radius = 16;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final isVideo = switch (slot) {
      ProtoSlot(:final candidate) => candidate.contentType.startsWith('video/'),
      // For ripls media, read contentType from the async media lookup
      // so a swap-back video shows the play overlay just like a proto
      // video candidate. Falls back to false until the async resolves.
      MediaIdSlot(:final mediaId) => ref
              .watch(mediaObjectProvider(mediaId))
              .value
              ?.contentType
              ?.toLowerCase()
              .startsWith('video/') ??
          false,
    };
    final semantics = isVideo
        ? l10n.replaceMediaCandidateVideo(index + 1)
        : l10n.replaceMediaCandidatePhoto(index + 1);

    return Tappable(
      onTap: enabled
          ? () {
              Navigator.pop(context);
              onTap?.call(index);
            }
          : null,
      semanticsLabel: semantics,
      child: SizedBox(
        width: _thumbSize,
        height: _thumbSize,
        child: ClipRRect(
          borderRadius: BorderRadius.circular(_radius),
          child: Stack(
            fit: StackFit.expand,
            children: [
              _buildImage(context, ref),
              if (isVideo)
                const Center(
                  child: Icon(
                    Icons.play_circle_fill,
                    color: OverlayTokens.textPrimary,
                    size: 28,
                  ),
                ),
              if (isLoading)
                Container(
                  color: OverlayTokens.fieldFill,
                  alignment: Alignment.center,
                  child: const SizedBox(
                    width: 24,
                    height: 24,
                    child: CircularProgressIndicator(
                      strokeWidth: 2,
                      color: Colors.white,
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildImage(BuildContext context, WidgetRef ref) {
    switch (slot) {
      case ProtoSlot(:final candidate):
        final thumbUrl =
            candidate.hasThumbnailUrl() ? candidate.thumbnailUrl : candidate.url;
        return CachedNetworkImage(
          imageUrl: thumbUrl,
          cacheKey: _protoCacheKey(candidate),
          fit: BoxFit.cover,
          placeholder: (_, _) => Container(color: AppColors.surface(context)),
          errorWidget: (_, _, _) => Container(color: AppColors.surface(context)),
        );
      case MediaIdSlot(:final mediaId):
        final mediaAsync = ref.watch(mediaObjectProvider(mediaId));
        final semantics = context.l10n.replaceMediaCandidatePhoto(index + 1);
        return mediaAsync.when(
          data: (media) => CachedMediaImage(
            imageUrl: media.url,
            cacheKey: media.cacheKey,
            fit: BoxFit.cover,
            semanticsLabel: semantics,
          ),
          loading: () => Container(color: AppColors.surface(context)),
          error: (_, _) => Container(color: AppColors.surface(context)),
        );
    }
  }

  /// Stable cache key for a proto-slot thumbnail. External CDN URLs
  /// have no ripls media id, so we key on the provider + provider
  /// photo id when present (lets the same Pexels photo across two
  /// unified-create runs hit the disk cache). Falls back to URL.
  static String _protoCacheKey(MediaCandidate c) {
    if (c.hasProvider() && c.hasProviderPhotoId()) {
      return 'candidate:${c.provider.name}:${c.providerPhotoId}';
    }
    return 'candidate:url:${c.url}';
  }
}
