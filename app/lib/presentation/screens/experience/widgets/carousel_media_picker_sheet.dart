import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// CarouselMediaPickerSheet is a bottom sheet for picking media source from
/// within the carousel. Uses plain callbacks so the caller can await the
/// resulting upload future.
class CarouselMediaPickerSheet extends StatelessWidget {
  final bool hasMedia;
  final VoidCallback onVideoTap;
  final VoidCallback onPhotoTap;
  final VoidCallback onCameraTap;

  const CarouselMediaPickerSheet({
    super.key,
    required this.hasMedia,
    required this.onVideoTap,
    required this.onPhotoTap,
    required this.onCameraTap,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return GlassSheet(
      applyMaxHeight: false,
      // This sheet always opens over the item's full-bleed hero media; without
      // the content scrim the blurred-through photo washes the light glass out
      // to a glowing white block on the dark theme (#2724).
      scrim: AppColors.modalContentScrim,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            l10n.mediaPickerDialogTitleAdd,
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.bold,
              color: AppColors.modalTextPrimary,
            ),
          ),
          const SizedBox(height: 16),
          _row(Icons.video_library, l10n.mediaPickerDialogVideos, onVideoTap),
          _row(Icons.photo_library, l10n.mediaPickerDialogPhotos, onPhotoTap),
          _row(Icons.camera_alt, l10n.mediaPickerDialogCamera, onCameraTap),
        ],
      ),
    );
  }

  Widget _row(IconData icon, String label, VoidCallback onTap) {
    return ListTile(
      leading: Icon(icon, color: AppColors.modalTextPrimary),
      title: Text(
        label,
        style: const TextStyle(color: AppColors.modalTextPrimary),
      ),
      onTap: onTap,
    );
  }
}
