import 'package:cross_file/cross_file.dart';
import 'package:flutter/foundation.dart' show visibleForTesting;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/media_helpers.dart';
import 'package:ripls/core/utils/media_picker_helper.dart';
import 'package:ripls/core/utils/media_upload_helper.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart'
    show ExperienceState;
import 'package:ripls/services/providers.dart';

final _log = Logger('ExperienceMediaActions');

/// ExperienceMediaActionsMixin provides media upload, delete, reorder, and
/// background-loading methods for [ExperienceNotifier].
mixin ExperienceMediaActionsMixin on Notifier<ExperienceState> {
  /// pickMultipleImagesFromGallery picks multiple images from the gallery and
  /// uploads them sequentially, calling saveExperience() once after all
  /// uploads complete.
  Future<void> pickMultipleImagesFromGallery({bool insertAtFront = false}) async {
    final files = await MediaPickerHelper.pickMultipleImagesFromGallery();
    if (files.isEmpty) return;
    await _setMultipleMediaFiles(files, insertAtFront: insertAtFront);
  }

  /// pickImageFromGallery picks an image from the gallery and uploads it to
  /// the experience.
  Future<void> pickImageFromGallery({bool insertAtFront = false}) async {
    try {
      final file = await MediaPickerHelper.pickImageFromGallery();
      if (file != null) {
        await _setMediaFile(file, insertAtFront: insertAtFront);
      }
    } catch (e) {
      _log.severe('❌ Failed to pick image: $e');
      rethrow;
    }
  }

  /// pickImageFromCamera picks an image from the camera and uploads it to the
  /// experience.
  Future<void> pickImageFromCamera({bool insertAtFront = false}) async {
    try {
      final file = await MediaPickerHelper.pickImageFromCamera();
      if (file != null) {
        await _setMediaFile(file, insertAtFront: insertAtFront);
      }
    } catch (e) {
      _log.severe('❌ Failed to pick image from camera: $e');
      rethrow;
    }
  }

  /// pickVideoFromGallery picks a video from the gallery and uploads it to the
  /// experience.
  Future<void> pickVideoFromGallery({bool insertAtFront = false}) async {
    try {
      final file = await MediaPickerHelper.pickVideoFromGallery();
      if (file != null) {
        await _setMediaFile(file, insertAtFront: insertAtFront);
      }
    } catch (e) {
      _log.severe('❌ Failed to pick video: $e');
      rethrow;
    }
  }

  @visibleForTesting
  Future<void> setMediaFileForTesting(
    XFile file, {
    bool insertAtFront = false,
  }) =>
      _setMediaFile(file, insertAtFront: insertAtFront);

  Future<void> _setMediaFile(XFile file, {bool insertAtFront = false}) async {
    if (state.experienceDetails == null) return;

    // The new media becomes the background only when it ends up at index 0:
    // either explicitly prepended (background-replace flow) or appended to an
    // empty list. Carousel-add into a non-empty list appends to the end and
    // must leave the existing background untouched.
    final becomesPrimary = insertAtFront || state.allMediaItems.isEmpty;

    // Create optimistic placeholder.
    final tempId = MediaUploadHelper.generateTempId();
    final placeholderItem = MediaUploadHelper.createPlaceholder(
      tempId: tempId,
      file: file,
    );

    // Insert placeholder at correct position.
    final placeholderAllMedia = MediaUploadHelper.insertPlaceholder(
      placeholder: placeholderItem,
      currentItems: state.allMediaItems,
      insertAtFront: insertAtFront,
    );

    state = state.copyWith(
      isUploading: true,
      error: null,
      allMediaItems: placeholderAllMedia,
    );

    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaId = await mediaRepository.addMedia(file: file);

      // Add media ID to existing list (experiences support multiple media).
      final currentMediaIds = state.experienceDetails!.experience.mediaIds;
      final updatedMediaIds = insertAtFront
          ? [mediaId, ...currentMediaIds]
          : [...currentMediaIds, mediaId];

      // Save experience with updated media IDs. Pass existing name and
      // description so the server preserves them — the server's
      // SaveExperience update path treats empty Name/Description as "no
      // change" only if we send the current value; sending an empty string
      // is otherwise indistinguishable from "leave alone" and has caused
      // text fields to be cleared (this happens because client-side
      // serialization may drop them entirely on the wire). Time and
      // location are intentionally omitted — the server emits a system
      // chat message whenever time != nil in the request, so leaving it
      // unset suppresses a spurious "time updated" message per media
      // upload.
      final experience = state.experienceDetails!.experience;
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.saveExperience(
        id: experience.id,
        name: experience.name,
        description: experience.description,
        mediaIds: updatedMediaIds,
      );

      // Get the fresh media response for content type detection.
      final mediaResponse = await mediaRepository.getMedia(mediaId);
      final isVideoFromServer = mediaResponse.contentType.startsWith('video/');

      // Check if still mounted after async gap.
      if (!ref.mounted) return;

      // Update state. The VideoBackgroundHost widget creates the video
      // controller on its own when it sees mediaPath + isVideo.
      state = becomesPrimary
          ? state.copyWith(
              mediaId: mediaId,
              mediaPath: mediaResponse.url,
              isVideo: isVideoFromServer,
              isUploading: false,
            )
          : state.copyWith(isUploading: false);

      // Sync details without the isLoading spinner — loadExperienceDetails would
      // set isLoading:true, which unmounts InlineConversationView and orphans
      // the carousel's getMediaItems closure while the carousel is still open.
      await refreshWithoutLoadingSpinner();
    } catch (e, stackTrace) {
      // Remove placeholder on error.
      final mediaWithoutPlaceholder = MediaUploadHelper.removePlaceholder(
        tempId: tempId,
        currentItems: state.allMediaItems,
      );
      state = state.copyWith(
        isUploading: false,
        error: RpcErrorHandler.classify(e, fallback: 'Could not upload media'),
        allMediaItems: mediaWithoutPlaceholder,
      );
      _log.severe('❌ Failed to upload media', e, stackTrace);
      rethrow;
    }
  }

  Future<void> _setMultipleMediaFiles(
    List<XFile> files, {
    bool insertAtFront = false,
  }) async {
    if (state.experienceDetails == null) return;

    // Create all optimistic placeholders up front.
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
    state = state.copyWith(isUploading: true, error: null, allMediaItems: allMedia);

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
      entityLabel: 'experience',
    );

    if (!ref.mounted) return;

    // Persist with a single RPC — do not pass time so the server doesn't emit a
    // spurious "time updated" system message. Pass existing name/description
    // so the server preserves text fields rather than treating absence as
    // a clear.
    if (uploadedIds.isNotEmpty) {
      final experience = state.experienceDetails!.experience;
      final updatedMediaIds = insertAtFront
          ? [...uploadedIds, ...experience.mediaIds]
          : [...experience.mediaIds, ...uploadedIds];
      await ref.read(experienceRepositoryProvider).saveExperience(
        id: experience.id,
        name: experience.name,
        description: experience.description,
        mediaIds: updatedMediaIds,
      );
      if (!ref.mounted) return;
      await refreshWithoutLoadingSpinner();
    }

    state = state.copyWith(
      isUploading: false,
      batchUploadFailedCount: failedCount > 0 ? failedCount : null,
    );

    if (failedCount > 0) {
      _log.warning(
        '⚠️ $failedCount of ${files.length} experience media uploads failed',
      );
    }
  }

  /// deleteMedia removes a media item from the experience.
  Future<void> deleteMedia(String mediaId) async {
    if (state.experienceDetails == null) return;

    try {
      // Remove from local state immediately.
      final updatedAllMedia = state.allMediaItems
          .where((item) => item.id != mediaId)
          .toList();
      state = state.copyWith(allMediaItems: updatedAllMedia);

      // Update displayed media if we deleted the current one.
      if (state.mediaId == mediaId && updatedAllMedia.isNotEmpty) {
        final firstMedia = updatedAllMedia.first;
        state = state.copyWith(
          mediaId: firstMedia.id,
          mediaPath: firstMedia.url,
          isVideo: firstMedia.isVideo,
        );
      } else if (updatedAllMedia.isEmpty) {
        state = state.copyWith(mediaId: null, mediaPath: null, isVideo: false);
      }

      // Delete from server.
      final mediaRepository = ref.read(mediaRepositoryProvider);
      await mediaRepository.deleteMedia(mediaId);

      // Remove from experience's media IDs and save.
      final currentMediaIds = state.experienceDetails!.experience.mediaIds;
      final updatedMediaIds = currentMediaIds
          .where((id) => id != mediaId)
          .toList();

      // Pass existing name/description so the server preserves text fields.
      // Time and location are intentionally omitted — the server emits a
      // system chat message whenever time != nil in the request.
      final experience = state.experienceDetails!.experience;
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.saveExperience(
        id: experience.id,
        name: experience.name,
        description: experience.description,
        mediaIds: updatedMediaIds,
      );

      // Sync without isLoading spinner — same reason as _setMediaFile.
      await refreshWithoutLoadingSpinner();
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to delete media: $e', e, stackTrace);
      rethrow;
    }
  }

  /// reorderMedia saves a new media ordering for the experience.
  Future<void> reorderMedia(List<String> mediaIds) async {
    if (state.experienceDetails == null) return;

    try {
      // Pass existing name/description so the server preserves text fields.
      // Time is intentionally omitted — the server emits a system chat
      // message whenever time != nil in the request.
      final experience = state.experienceDetails!.experience;
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.saveExperience(
        id: experience.id,
        name: experience.name,
        description: experience.description,
        mediaIds: mediaIds,
      );

      // Sync without isLoading spinner — same reason as _setMediaFile.
      await refreshWithoutLoadingSpinner();
    } catch (e, stackTrace) {
      _log.severe('❌ Error reordering media: $e', e, stackTrace);
      // Restore correct state without triggering a loading spinner.
      await refreshWithoutLoadingSpinner();
      rethrow;
    }
  }

  /// loadAllMediaFromServer loads all media URLs for the carousel (thumbnails).
  Future<void> loadAllMediaFromServer(List<String> mediaIds) async {
    try {
      final allMedia = await MediaHelpers.loadAllMedia(ref, mediaIds);

      // Check if still mounted before updating state.
      if (!ref.mounted) return;
      // Bail if the user logged out during the fetch — writing state post-
      // logout can land in the IndexedStack rebuild frame and trigger a
      // duplicate NavigatorState GlobalKey crash.
      if (ref.read(authStateProvider).user == null) return;

      state = state.copyWith(allMediaItems: allMedia);
    } catch (e, stackTrace) {
      _log.severe('❌ Error loading all media from server: $e', e, stackTrace);
    }
  }

  /// loadMediaFromServer loads a single media item at full resolution for the
  /// background display. The widget tree owns the video controller — this just
  /// resolves the URL and sets the state so [VideoBackgroundHost] can pick it up.
  Future<void> loadMediaFromServer(String mediaId) async {
    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaUrl = await mediaRepository.getFullMediaUrl(mediaId);

      if (!ref.mounted) return;
      // Bail if the user logged out during the fetch — see logout-race
      // guard in loadAllMediaFromServer.
      if (ref.read(authStateProvider).user == null) return;

      final isVideo = mediaUrl.contentType?.startsWith('video/') ?? false;

      state = state.copyWith(
        mediaPath: mediaUrl.url,
        mediaId: mediaId,
        isVideo: isVideo,
        isBackgroundMediaLoading: false,
        // Keep [backgroundThumbnailUrl] populated — VideoPlayerController
        // initialization takes 500 ms-2 s even from local disk, and
        // MediaBackground's priority order already prefers the
        // initialized controller (priority 1) over the thumbnail
        // (priority 2). Clearing here would leave the hero black during
        // that controller-init window.
      );
      _log.info('🖼️ Background media loaded: $mediaId (isVideo: $isVideo)');
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to load background media: $e', e, stackTrace);
      // Clear the loading flag so the UI doesn't remain stuck on the thumbnail.
      if (ref.mounted) {
        state = state.copyWith(
          isBackgroundMediaLoading: false,
        );
      }
    }
  }

  // ── Cross-mixin hook ──────────────────────────────────────────────────────

  /// refreshWithoutLoadingSpinner silently reloads experience data without
  /// setting [isLoading]. Implemented by the coordinator notifier.
  Future<void> refreshWithoutLoadingSpinner();
}
