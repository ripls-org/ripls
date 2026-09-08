import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';

/// Bottom sheet for adding/replacing the gear carousel's media: videos,
/// gallery photos (multi-select), or camera. Awaits the picked upload and
/// surfaces a partial-failure toast for multi-image batches.
///
/// Extracted from `GearContentView` (its 1,000-line-gate split, #2912); the
/// caller passes the notifier so the widget layer never touches a repository.
Future<void> showGearCarouselMediaPicker({
  required BuildContext context,
  required WidgetRef ref,
  required String gearId,
  required GearNotifier notifier,
}) async {
  Future<void>? uploadFuture;

  final hasMedia = ref.read(gearProvider(gearId)).allMediaItems.isNotEmpty;
  await showAccessibleModal<void>(
    context,
    builder: (sheetContext) {
      return Container(
        padding: const EdgeInsets.all(16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              hasMedia
                  ? context.l10n.mediaPickerDialogTitleReplace
                  : context.l10n.mediaPickerDialogTitleAdd,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
            ),
            const SizedBox(height: 16),
            ListTile(
              leading: const Icon(Icons.video_library),
              title: Text(context.l10n.mediaPickerDialogVideos),
              onTap: () {
                Navigator.of(sheetContext).pop();
                uploadFuture = notifier.pickVideoFromGallery();
              },
            ),
            ListTile(
              leading: const Icon(Icons.photo_library),
              title: Text(context.l10n.mediaPickerDialogPhotos),
              onTap: () {
                Navigator.of(sheetContext).pop();
                uploadFuture = notifier.pickMultipleImagesFromGallery();
              },
            ),
            ListTile(
              leading: const Icon(Icons.camera_alt),
              title: Text(context.l10n.mediaPickerDialogCamera),
              onTap: () {
                Navigator.of(sheetContext).pop();
                uploadFuture = notifier.pickImageFromCamera();
              },
            ),
          ],
        ),
      );
    },
  );

  if (uploadFuture != null) {
    await uploadFuture;
    if (!context.mounted) return;
    final failedCount = ref.read(gearProvider(gearId)).batchUploadFailedCount;
    if (failedCount != null && failedCount > 0) {
      ToastHelper.showError(
        context,
        context.l10n.mediaBatchUploadPartialFailure(failedCount),
      );
    }
  }
}
