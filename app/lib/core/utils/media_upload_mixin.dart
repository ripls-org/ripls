import 'package:cross_file/cross_file.dart';
import 'package:flutter/foundation.dart' show visibleForTesting;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/utils/media_picker_helper.dart';
import 'package:ripls/data/repositories/media_repository.dart';

/// Mixin providing media upload functionality for ViewModels.
///
/// This mixin handles the common pattern of:
/// 1. Picking media from gallery/camera using MediaPickerHelper
/// 2. Uploading to server via MediaRepository
/// 3. Updating state with upload progress and results
///
/// ViewModels using this mixin must:
/// - Provide a MediaRepository via [mediaRepository] getter
/// - Provide a Logger via [log] getter
/// - Implement [onMediaUploaded] to update their specific state with the mediaId
/// - Implement [onMediaUploadStarted] to set upload state
/// - Implement [onMediaUploadFailed] to handle upload errors
///
/// Example usage:
/// ```dart
/// class GenGearNotifier extends Notifier<GenGearState> with MediaUploadMixin<GenGearState> {
///   @override
///   MediaRepository get mediaRepository => ref.read(mediaRepositoryProvider);
///
///   @override
///   Logger get log => _log;
///
///   @override
///   void onMediaUploaded(String mediaId) {
///     state = state.copyWith(
///       selectedMediaIds: [mediaId],
///       isUploadingMedia: false,
///     );
///   }
///
///   @override
///   void onMediaUploadStarted() {
///     state = state.copyWith(isUploadingMedia: true, uploadError: null);
///   }
///
///   @override
///   void onMediaUploadFailed(String error) {
///     state = state.copyWith(
///       uploadError: error,
///       isUploadingMedia: false,
///     );
///   }
/// }
/// ```
mixin MediaUploadMixin<T> on Notifier<T> {
  /// MediaRepository for uploading media files.
  /// Must be implemented by the ViewModel.
  MediaRepository get mediaRepository;

  /// Logger for logging upload operations.
  /// Must be implemented by the ViewModel.
  Logger get log;

  /// Description to use when uploading media.
  /// Override to customize per content type.
  String get mediaUploadDescription => 'Content media';

  /// Called when media upload starts.
  /// Update state to show loading indicator.
  void onMediaUploadStarted();

  /// Called when media upload succeeds.
  /// Update state with the uploaded mediaId.
  void onMediaUploaded(String mediaId);

  /// Called when media upload fails.
  /// Update state with error message.
  void onMediaUploadFailed(String error);

  /// Called when batch upload progress updates.
  /// Override to update state with progress (e.g., completed 2 of 5).
  /// Default implementation is a no-op for backwards compatibility.
  void onBatchUploadProgress(int completed, int total) {}

  /// Called when a batch upload completes with all successfully uploaded media IDs.
  /// Override to update state with the full list of uploaded IDs.
  /// Default implementation calls [onMediaUploaded] with the last ID.
  void onBatchUploadCompleted(List<String> mediaIds) {
    if (mediaIds.isNotEmpty) {
      onMediaUploaded(mediaIds.last);
    }
  }

  /// Called when a batch upload finishes with partial failures.
  /// [uploadedIds] contains the IDs that succeeded.
  /// [failedCount] is the number that failed.
  /// Default implementation calls [onBatchUploadCompleted] with the successful IDs.
  void onBatchUploadPartialFailure(List<String> uploadedIds, int failedCount) {
    onBatchUploadCompleted(uploadedIds);
  }

  
  /// Pick video from gallery and upload to server.
  Future<void> pickAndUploadVideoFromGallery() async {
    try {
      final file = await MediaPickerHelper.pickVideoFromGallery();
      if (file != null) {
        await _setMediaFile(file);
      }
    } catch (e) {
      log.severe('❌ Failed to pick video: $e');
      rethrow;
    }
  }

  /// Pick image from camera and upload to server.
  Future<void> pickAndUploadImageFromCamera() async {
    try {
      final file = await MediaPickerHelper.pickImageFromCamera();
      if (file != null) {
        await _setMediaFile(file);
      }
    } catch (e) {
      log.severe('❌ Failed to take photo: $e');
      rethrow;
    }
  }

  /// Pick multiple images from gallery and upload sequentially.
  ///
  /// Uploads each image one at a time, calling [onBatchUploadProgress] after
  /// each upload. On partial failure, keeps successful uploads and calls
  /// [onBatchUploadPartialFailure]. Checks [ref.mounted] between uploads to
  /// handle early disposal in autoDispose notifiers.
  Future<void> pickAndUploadMultipleImagesFromGallery() async {
    try {
      final files =
          await MediaPickerHelper.pickMultipleImagesFromGallery();
      if (files.isEmpty) return;

      await _uploadMultipleMediaFiles(files);
    } catch (e) {
      log.severe('❌ Failed to pick multiple images: $e');
      rethrow;
    }
  }

  /// Internal helper: Upload media file and update state.
  Future<void> _setMediaFile(XFile file) async {
    onMediaUploadStarted();

    try {
      final mediaId = await mediaRepository.addMedia(
        file: file,
        description: mediaUploadDescription,
      );

      onMediaUploaded(mediaId);
      log.info('✅ Media uploaded successfully: $mediaId');
    } catch (e, stackTrace) {
      log.severe('❌ Media upload failed', e, stackTrace);
      onMediaUploadFailed('Upload failed: $e');
      rethrow;
    }
  }

  /// Uploads multiple media files sequentially from pre-selected files.
  ///
  /// Exposed for testing — production code should use
  /// [pickAndUploadMultipleImagesFromGallery] which handles picking.
  @visibleForTesting
  Future<void> uploadMultipleMediaFiles(List<XFile> files) =>
      _uploadMultipleMediaFiles(files);

  /// Internal helper: Upload multiple media files sequentially.
  Future<void> _uploadMultipleMediaFiles(List<XFile> files) async {
    onMediaUploadStarted();

    final uploadedIds = <String>[];
    var failedCount = 0;
    final total = files.length;

    for (var i = 0; i < files.length; i++) {
      try {
        if (!ref.mounted) {
          log.info('Provider disposed during batch upload, stopping.');
          break;
        }
      } catch (_) {
        // In Riverpod 3, accessing ref after full disposal throws.
        log.info('Provider disposed during batch upload, stopping.');
        break;
      }

      try {
        final mediaId = await mediaRepository.addMedia(
          file: files[i],
          description: mediaUploadDescription,
        );
        uploadedIds.add(mediaId);
        log.info(
          '✅ Batch upload ${i + 1}/$total succeeded: $mediaId',
        );
      } catch (e, stackTrace) {
        failedCount++;
        log.severe(
          '❌ Batch upload ${i + 1}/$total failed',
          e,
          stackTrace,
        );
      }

      onBatchUploadProgress(uploadedIds.length + failedCount, total);
    }

    if (uploadedIds.isEmpty && failedCount > 0) {
      onMediaUploadFailed('All $failedCount uploads failed');
      return;
    }

    if (failedCount > 0) {
      onBatchUploadPartialFailure(uploadedIds, failedCount);
    } else {
      onBatchUploadCompleted(uploadedIds);
    }
  }
}
