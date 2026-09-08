import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:image_picker/image_picker.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/media_helpers.dart';
import 'package:ripls/core/utils/media_picker_helper.dart';
import 'package:ripls/core/utils/media_upload_helper.dart';
import 'package:ripls/presentation/viewmodels/request_state.dart';
import 'package:ripls/presentation/viewmodels/video_audio_view_model.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('RequestMediaActions');

/// RequestMediaActionsMixin provides media loading, picking, uploading, and
/// playback methods for [RequestNotifier].
///
/// Coordination pattern: this mixin is used by the single [RequestNotifier]
/// coordinator class. All state reads and writes go through [state] and
/// [state.copyWith], which are the coordinator's own [Notifier.state] fields.
mixin RequestMediaActionsMixin on Notifier<RequestState> {
  ImagePicker get imagePicker;

  // ---------------------------------------------------------------------------
  // Internal helpers
  // ---------------------------------------------------------------------------

  /// loadMediaFromServer resolves the full media URL for background display
  /// and updates state. The widget tree owns the video controller —
  /// [VideoBackgroundHost] picks up [mediaPath] + [isVideo] from state and
  /// initializes its own controller.
  Future<void> loadMediaFromServer(String mediaId) async {
    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaUrl = await mediaRepository.getFullMediaUrl(mediaId);

      final media = await mediaRepository.get(mediaId);
      final backgroundAttribution =
          media.hasAttribution() ? media.attribution : null;

      if (!ref.mounted) return;

      final isVideo = mediaUrl.contentType?.startsWith('video/') ?? false;

      state = state.copyWith(
        mediaPath: mediaUrl.url,
        mediaId: mediaId,
        isVideo: isVideo,
        backgroundAttribution: backgroundAttribution,
        isBackgroundMediaLoading: false,
        backgroundThumbnailUrl: null,
      );
    } catch (e, stackTrace) {
      _log.severe('❌ Error loading media from server: $e', e, stackTrace);
      // Clear loading flag so the UI doesn't remain stuck on the thumbnail.
      if (ref.mounted) {
        state = state.copyWith(isBackgroundMediaLoading: false);
      }
    }
  }

  /// loadAllMediaFromServer loads all media items with thumbnail URLs for
  /// carousel display.
  Future<void> loadAllMediaFromServer(List<String> mediaIds) async {
    try {
      final allMedia = await MediaHelpers.loadAllMedia(ref, mediaIds);
      state = state.copyWith(allMediaItems: allMedia);
      _log.info('✅ Loaded ${allMedia.length} media items for carousel');
    } catch (e, stackTrace) {
      _log.severe('❌ Error loading all media: $e', e, stackTrace);
    }
  }

  // ---------------------------------------------------------------------------
  // Playback controls
  // ---------------------------------------------------------------------------

  /// setSelectedMedia sets the selected media index for the media pane.
  void setSelectedMedia(int index) {
    state = state.copyWith(selectedMediaIndex: index);
  }

  /// toggleMute toggles video mute state and updates the session-wide
  /// `videoUnmutedProvider` so the choice persists across navigation and
  /// drives the audio-session category swap (issue #1250).
  void toggleMute() {
    final newMuted = !state.isMuted;
    state = state.copyWith(isMuted: newMuted);
    ref.read(videoUnmutedProvider.notifier).set(!newMuted);
  }

  /// setCurrentMediaIndex sets the current media index for carousel navigation.
  void setCurrentMediaIndex(int index) {
    if (index >= 0 && index < state.allMediaItems.length) {
      state = state.copyWith(currentMediaIndex: index);
      _log.info('📸 Switched to media index: $index');
    }
  }

  // ---------------------------------------------------------------------------
  // Media picking
  // ---------------------------------------------------------------------------

  /// pickImageFromGallery opens the gallery to pick a single image.
  Future<void> pickImageFromGallery({bool insertAtFront = false}) async {
    await _pickMedia(
      ImageSource.gallery,
      isVideo: false,
      insertAtFront: insertAtFront,
    );
  }

  /// pickImageFromCamera opens the camera to capture an image.
  Future<void> pickImageFromCamera({bool insertAtFront = false}) async {
    await _pickMedia(
      ImageSource.camera,
      isVideo: false,
      insertAtFront: insertAtFront,
    );
  }

  /// pickVideoFromGallery opens the gallery to pick a video.
  Future<void> pickVideoFromGallery({bool insertAtFront = false}) async {
    await _pickMedia(
      ImageSource.gallery,
      isVideo: true,
      insertAtFront: insertAtFront,
    );
  }

  /// pickMultipleImagesFromGallery picks multiple images from the gallery and
  /// uploads them sequentially, calling updateRequest() once after all uploads
  /// complete.
  Future<void> pickMultipleImagesFromGallery({bool insertAtFront = false}) async {
    final files = await MediaPickerHelper.pickMultipleImagesFromGallery();
    if (files.isEmpty) return;

    final tempIds = [for (final _ in files) MediaUploadHelper.generateTempId()];
    var allMedia = state.allMediaItems;
    for (var i = 0; i < files.length; i++) {
      allMedia = MediaUploadHelper.insertPlaceholder(
        placeholder: MediaUploadHelper.createPlaceholder(
          tempId: tempIds[i],
          file: files[i],
        ),
        currentItems: allMedia,
        insertAtFront: insertAtFront,
      );
    }
    state = state.copyWith(isUploadingMedia: true, allMediaItems: allMedia);

    final (:uploadedIds, :failedCount) = await MediaHelpers.batchUploadFiles(
      ref: ref,
      files: files,
      tempIds: tempIds,
      isMounted: () => ref.mounted,
      onSuccess: (tempId, item) => state = state.copyWith(
        allMediaItems:
            state.allMediaItems.map((m) => m.id == tempId ? item : m).toList(),
      ),
      onFailure: (tempId) => state = state.copyWith(
        allMediaItems: MediaUploadHelper.removePlaceholder(
          tempId: tempId,
          currentItems: state.allMediaItems,
        ),
      ),
      entityLabel: 'request',
    );

    if (!ref.mounted) return;
    state = state.copyWith(
      isUploadingMedia: false,
      batchUploadFailedCount: failedCount > 0 ? failedCount : null,
    );

    if (uploadedIds.isNotEmpty) {
      final currentRequest = state.requestDetails;
      if (currentRequest != null) {
        final existingIds = currentRequest.mediaIds
            .where((id) => !id.startsWith('temp_'))
            .toList();
        final updatedMediaIds = insertAtFront
            ? [...uploadedIds, ...existingIds]
            : [...existingIds, ...uploadedIds];
        await ref.read(requestRepositoryProvider).updateRequest(
          requestId: currentRequest.id,
          title: currentRequest.title,
          description: currentRequest.description,
          mediaIds: updatedMediaIds,
          locationId: currentRequest.locationId.isNotEmpty
              ? currentRequest.locationId
              : null,
        );
        // Use refresh() instead of loadRequestDetails() to avoid setting
        // isLoading: true mid-flight, which causes rapid double state changes
        // that trigger a Flutter semantics assertion when the carousel and
        // picker sheet are both active in the overlay stack.
        if (ref.mounted) await refreshRequest();
      }
    }

    if (failedCount > 0) {
      _log.warning(
        '⚠️ $failedCount of ${files.length} request media uploads failed',
      );
    }
  }

  /// _pickMedia handles picking and uploading a single media item from the
  /// given source.
  Future<void> _pickMedia(
    ImageSource source, {
    required bool isVideo,
    bool insertAtFront = false,
  }) async {
    String? tempId;
    try {
      final XFile? pickedFile = isVideo
          ? await imagePicker.pickVideo(source: source)
          : await imagePicker.pickImage(source: source);

      if (pickedFile == null) {
        return;
      }

      _log.info('📸 Media picked: ${pickedFile.path}');

      tempId = MediaUploadHelper.generateTempId();
      final placeholderItem = MediaUploadHelper.createPlaceholder(
        tempId: tempId,
        file: pickedFile,
      );

      final placeholderAllMedia = MediaUploadHelper.insertPlaceholder(
        placeholder: placeholderItem,
        currentItems: state.allMediaItems,
        insertAtFront: insertAtFront,
      );

      state = state.copyWith(
        isUploadingMedia: true,
        allMediaItems: placeholderAllMedia,
      );

      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaId = await mediaRepository.addMedia(file: pickedFile);

      _log.info('✅ Media uploaded with ID: $mediaId');

      final currentRequest = state.requestDetails;
      if (currentRequest != null) {
        final updatedMediaIds = insertAtFront
            ? [mediaId, ...currentRequest.mediaIds]
            : [...currentRequest.mediaIds, mediaId];

        final requestRepository = ref.read(requestRepositoryProvider);
        await requestRepository.updateRequest(
          requestId: currentRequest.id,
          title: currentRequest.title,
          description: currentRequest.description,
          mediaIds: updatedMediaIds,
          locationId: currentRequest.locationId.isNotEmpty
              ? currentRequest.locationId
              : null,
        );

        await loadRequestDetails();
      } else {
        await loadMediaFromServer(mediaId);
        state = state.copyWith(mediaId: mediaId);
      }

      state = state.copyWith(isUploadingMedia: false);
    } catch (e, stackTrace) {
      _log.severe('❌ Error picking/uploading media: $e', e, stackTrace);
      final mediaWithoutPlaceholder = tempId != null
          ? MediaUploadHelper.removePlaceholder(
              tempId: tempId,
              currentItems: state.allMediaItems,
            )
          : state.allMediaItems;
      state = state.copyWith(
        isUploadingMedia: false,
        error: const UserError.generic(fallback: 'Could not upload media'),
        allMediaItems: mediaWithoutPlaceholder,
      );
      rethrow;
    }
  }

  // ---------------------------------------------------------------------------
  // Media mutations
  // ---------------------------------------------------------------------------

  /// deleteMedia removes a media item from the request.
  Future<void> deleteMedia(String mediaId) async {
    if (state.requestDetails == null) return;

    _log.info('🗑️ Deleting media: $mediaId from request');

    try {
      final currentRequest = state.requestDetails!;

      final updatedMediaIds = currentRequest.mediaIds
          .where((id) => id != mediaId)
          .toList();

      final requestRepository = ref.read(requestRepositoryProvider);
      await requestRepository.updateRequest(
        requestId: currentRequest.id,
        title: currentRequest.title,
        description: currentRequest.description,
        mediaIds: updatedMediaIds.isNotEmpty ? updatedMediaIds : null,
        locationId: currentRequest.locationId.isNotEmpty
            ? currentRequest.locationId
            : null,
      );

      _log.info('✅ Media deleted successfully');

      await loadRequestDetails();
    } catch (e, stackTrace) {
      _log.severe('❌ Error deleting media: $e', e, stackTrace);
      state = state.copyWith(
        error: const UserError.generic(fallback: 'Could not delete media'),
      );
      rethrow;
    }
  }

  /// reorderMedia updates the media item order for the request.
  Future<void> reorderMedia(List<String> mediaIds) async {
    if (state.requestDetails == null) return;

    _log.info('🔄 Reordering media: $mediaIds');

    try {
      final currentRequest = state.requestDetails!;

      final requestRepository = ref.read(requestRepositoryProvider);
      await requestRepository.updateRequest(
        requestId: currentRequest.id,
        title: currentRequest.title,
        description: currentRequest.description,
        mediaIds: mediaIds,
        locationId: currentRequest.locationId.isNotEmpty
            ? currentRequest.locationId
            : null,
      );

      _log.info('✅ Media reordered successfully');

      await loadRequestDetails();
    } catch (e, stackTrace) {
      _log.severe('❌ Error reordering media: $e', e, stackTrace);
      state = state.copyWith(
        error: const UserError.generic(fallback: 'Could not reorder media'),
      );
      rethrow;
    }
  }

  // ---------------------------------------------------------------------------
  // Abstract methods that the coordinator must implement
  // ---------------------------------------------------------------------------

  /// loadRequestDetails reloads request details from the repository.
  Future<void> loadRequestDetails();

  /// refreshRequest silently reloads request details without a loading spinner.
  Future<void> refreshRequest();
}
