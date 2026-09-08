import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/utils/media_helpers.dart';
import 'package:ripls/core/utils/media_picker_helper.dart';
import 'package:ripls/core/utils/media_upload_helper.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/core/utils/video_cache_helper.dart';
import 'package:ripls/core/utils/video_mute_helper.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/presentation/viewmodels/video_audio_view_model.dart';
import 'package:ripls/services/providers.dart';
import 'package:video_player/video_player.dart';

part 'community_edit_view_model.freezed.dart';

final _log = Logger('CommunityEditViewModel');

@freezed
sealed class CommunityEditState with _$CommunityEditState {
  const factory CommunityEditState({
    @Default(false) bool isEditing,
    @Default(false) bool isSaving,
    @Default(false) bool isUploadingMedia,
    String? newMediaId,
    UserError? error,
    @Default([]) List<MediaItemData> allMediaItems,
    Attribution? backgroundAttribution, // Attribution for background media (overflow menu)
    // Background media state for video support
    String? mediaPath,
    @Default(false) bool isVideo,
    VideoPlayerController? videoController,
    @Default(true) bool isMuted,
    // True while the video controller is being initialized; the view shows
    // backgroundThumbnailUrl as a static first frame during this window.
    @Default(false) bool isBackgroundMediaLoading,
    // Server-generated thumbnail URL displayed as a static background while
    // the video controller initializes. Cleared once the controller is ready.
    String? backgroundThumbnailUrl,
  }) = _CommunityEditState;

  const CommunityEditState._();

  bool get hasError => error != null;
}

/// ViewModel for managing inline editing of community details.
/// Used in CommunityContentView to enable creators to edit
/// community name and description directly in the feed.
class CommunityEditNotifier extends Notifier<CommunityEditState>
    with SafeNotifierMixin<CommunityEditState> {
  /// Constructor accepts the communityId parameter from the family modifier
  CommunityEditNotifier(this.communityId);

  /// The communityId for this specific community instance
  final String communityId;

  VideoPlayerController? _videoController;

  CommunityRepository get _repository => ref.read(communityRepositoryProvider);
  MediaRepository get _mediaRepository => ref.read(mediaRepositoryProvider);

  @override
  CommunityEditState build() {
    ref.onDispose(() {
      _videoController?.dispose();
    });
    // Seed mute state from the session-wide `videoUnmutedProvider` so a
    // user who has already chosen "audio on" hears the next community
    // background when they navigate to it (issue #1250).
    final isUnmuted = ref.read(videoUnmutedProvider);
    return CommunityEditState(isMuted: !isUnmuted);
  }

  /// Enter edit mode.
  void startEditing() {
    state = state.copyWith(isEditing: true, error: null);
  }

  /// Cancel editing without saving.
  void cancelEditing() {
    state = state.copyWith(
      isEditing: false,
      error: null,
      newMediaId: null,
    );
  }

  /// Upload media for community background.
  Future<void> uploadMedia(XFile file) async {
    state = state.copyWith(isUploadingMedia: true, error: null);

    try {
      final mediaId = await _mediaRepository.addMedia(
        file: file,
        description: 'Community background image',
      );

      _log.info('Media uploaded successfully: $mediaId');

      safeUpdateState((s) => s.copyWith(
        newMediaId: mediaId,
        isUploadingMedia: false,
      ));
    } catch (e, stackTrace) {
      _log.severe('Failed to upload media', e, stackTrace);

      safeUpdateState((s) => s.copyWith(
        isUploadingMedia: false,
        error: RpcErrorHandler.classify(e),
      ));

      rethrow;
    }
  }

  /// Upload media and add to the community's media_ids array.
  ///
  /// When [insertAtFront] is true, prepends to position 0 (for background
  /// image uploads). When false, appends to the end (for carousel uploads).
  ///
  /// Mirrors gear's [_setMediaFile] pattern: insert an optimistic placeholder,
  /// upload, fetch the real metadata, and swap the placeholder for the real
  /// MediaItemData inline. State updates immediately after the metadata fetch
  /// so carousel/strip widgets refresh in two network calls instead of N+3.
  /// Persisting the updated mediaIds to the community runs last so callers
  /// awaiting this future know the server has the new state before they
  /// dismiss any progress UI.
  Future<void> uploadMediaAndAppend(XFile file, {bool insertAtFront = false}) async {
    final tempId = MediaUploadHelper.generateTempId();
    final placeholderItem = MediaUploadHelper.createPlaceholder(
      tempId: tempId,
      file: file,
    );

    state = state.copyWith(
      isUploadingMedia: true,
      error: null,
      allMediaItems: MediaUploadHelper.insertPlaceholder(
        placeholder: placeholderItem,
        currentItems: state.allMediaItems,
        insertAtFront: insertAtFront,
      ),
    );

    try {
      final mediaId = await _mediaRepository.addMedia(
        file: file,
        description: 'Community media',
      );
      _log.info('✅ Media uploaded with ID: $mediaId');

      final mediaResponse = await _mediaRepository.get(mediaId);
      if (!ref.mounted) return;
      final isVideoFromServer = mediaResponse.contentType.startsWith('video/');
      final url = mediaResponse.thumbnailUrl.isNotEmpty
          ? mediaResponse.thumbnailUrl
          : mediaResponse.url;
      final newMediaItem = MediaItemData(
        id: mediaId,
        url: url,
        contentType: mediaResponse.contentType,
        isVideo: isVideoFromServer,
        attribution:
            mediaResponse.hasAttribution() ? mediaResponse.attribution : null,
      );

      // Swap placeholder → real item inline. The carousel's getMediaItems
      // closure reads from state.allMediaItems on its next refresh, so this
      // is what eventually surfaces in the gallery thumbnail strip.
      final updatedAllMedia = state.allMediaItems
          .map((item) => item.id == tempId ? newMediaItem : item)
          .toList();
      safeUpdateState((s) => s.copyWith(
            allMediaItems: updatedAllMedia,
            isUploadingMedia: false,
          ));

      // Persist the updated mediaIds to the server. Local state is the
      // source of truth here — _loadCommunityMedia in CommunityContentView
      // initializes allMediaItems from the community's authoritative list,
      // so mapping its IDs preserves order and reflects any prior edits.
      final updatedMediaIds = updatedAllMedia.map((m) => m.id).toList();
      await _repository.updateCommunity(
        id: communityId,
        mediaIds: updatedMediaIds,
      );
      _log.info('✅ Community updated with new media');
    } catch (e, stackTrace) {
      _log.severe('❌ Error uploading/appending media: $e', e, stackTrace);
      final mediaWithoutPlaceholder = MediaUploadHelper.removePlaceholder(
        tempId: tempId,
        currentItems: state.allMediaItems,
      );
      safeUpdateState((s) => s.copyWith(
            isUploadingMedia: false,
            error: const UserError.generic(fallback: 'Could not upload media'),
            allMediaItems: mediaWithoutPlaceholder,
          ));
      rethrow;
    }
  }

  /// Picks an image from the gallery and appends it to the community's media
  /// list (or inserts at front when [insertAtFront] is true). Mirrors the gear
  /// view-model's pickImageFromGallery so callers don't need to chain the
  /// picker and upload steps themselves.
  Future<void> pickImageFromGallery({bool insertAtFront = false}) async {
    final file = await MediaPickerHelper.pickImageFromGallery();
    if (file == null) return;
    await uploadMediaAndAppend(file, insertAtFront: insertAtFront);
  }

  /// Picks an image from the camera and appends it to the community's media
  /// list (or inserts at front when [insertAtFront] is true).
  Future<void> pickImageFromCamera({bool insertAtFront = false}) async {
    final file = await MediaPickerHelper.pickImageFromCamera();
    if (file == null) return;
    await uploadMediaAndAppend(file, insertAtFront: insertAtFront);
  }

  /// Picks a video from the gallery and appends it to the community's media
  /// list (or inserts at front when [insertAtFront] is true).
  Future<void> pickVideoFromGallery({bool insertAtFront = false}) async {
    final file = await MediaPickerHelper.pickVideoFromGallery();
    if (file == null) return;
    await uploadMediaAndAppend(file, insertAtFront: insertAtFront);
  }

  /// Save community changes.
  /// Returns true if save succeeded, false otherwise.
  Future<bool> saveCommunity({
    required String communityId,
    String? name,
    String? description,
  }) async {
    state = state.copyWith(isSaving: true, error: null);

    try {
      await _repository.updateCommunity(
        id: communityId,
        name: name,
        description: description,
        mediaId: state.newMediaId, // Use newMediaId from state
      );

      _log.info('Community $communityId updated successfully');

      safeUpdateState((s) => s.copyWith(
        isSaving: false,
        isEditing: false,
        error: null,
        newMediaId: null, // Clear after successful save
      ));

      return true;
    } catch (e, stackTrace) {
      _log.severe('Failed to save community', e, stackTrace);

      safeUpdateState((s) => s.copyWith(
        isSaving: false,
        error: RpcErrorHandler.classify(e),
      ));

      return false;
    }
  }

  /// Clear error message.
  void clearError() {
    state = state.copyWith(error: null);
  }

  /// Loads background media for display, initializing a video controller when needed.
  ///
  /// Uses content type from the repository for reliable video detection.
  /// Sets [isBackgroundMediaLoading] immediately with the thumbnail URL so the
  /// view has a static first frame while the video controller initializes.
  Future<void> loadBackgroundMedia(String mediaId) async {
    // Surface the thumbnail as a static frame before the full-resolution load.
    // Check allMediaItems first; fall back to fetching from the repository so
    // this works even when called concurrently with loadAllMedia.
    String? thumbnailUrl = state.allMediaItems
        .where((m) => m.id == mediaId)
        .firstOrNull
        ?.thumbnailUrl;
    if (thumbnailUrl == null) {
      try {
        final media = await _mediaRepository.get(mediaId);
        if (!ref.mounted) return;
        thumbnailUrl = media.thumbnailUrl.isNotEmpty ? media.thumbnailUrl : null;
      } catch (_) {
        // Thumbnail fetch failure is non-fatal; proceed without it.
      }
    }
    safeUpdateState((s) => s.copyWith(
      isBackgroundMediaLoading: true,
      backgroundThumbnailUrl: thumbnailUrl,
    ));

    try {
      final mediaUrl = await _mediaRepository.getFullMediaUrl(mediaId);
      if (!ref.mounted) return;

      final isVideo = mediaUrl.contentType?.startsWith('video/') ?? false;

      if (isVideo) {
        await _videoController?.dispose();

        final controller = await createCachedVideoController(
          _mediaRepository,
          mediaId,
          mediaUrl.url,
        );
        if (!ref.mounted) {
          await controller.dispose();
          return;
        }

        _videoController = controller;
        safeUpdateState((s) => s.copyWith(
          mediaPath: mediaUrl.url,
          isVideo: true,
          videoController: controller,
          isBackgroundMediaLoading: false,
          backgroundThumbnailUrl: null,
        ));
      } else {
        safeUpdateState((s) => s.copyWith(
          mediaPath: mediaUrl.url,
          isVideo: false,
          isBackgroundMediaLoading: false,
          backgroundThumbnailUrl: null,
        ));
      }
    } catch (e, stackTrace) {
      _log.warning('⚠️ Failed to load background media for $mediaId: $e', e, stackTrace);
      // Clear loading flag so the UI doesn't remain stuck on the thumbnail.
      safeUpdateState((s) => s.copyWith(isBackgroundMediaLoading: false));
    }
  }

  /// toggleMute toggles video mute state and updates the session-wide
  /// `videoUnmutedProvider` so the choice persists across navigation and
  /// drives the audio-session category swap (issue #1250).
  void toggleMute() {
    toggleVideoMute(
      controller: _videoController,
      currentlyMuted: state.isMuted,
      updateState: (muted) {
        state = state.copyWith(isMuted: muted);
        ref.read(videoUnmutedProvider.notifier).set(!muted);
      },
    );
  }

  /// Gets media URL for a given media ID via MediaHelpers utility.
  ///
  /// The utility handles caching through the repository.
  /// Returns the URL string for use in widgets.
  Future<MediaUrl> getMediaUrl(String mediaId) =>
      MediaHelpers.getMediaUrl(ref, mediaId);

  /// Loads background attribution for the specified media ID (for overflow menu display)
  Future<void> loadBackgroundAttribution(String? mediaId) async {
    if (mediaId == null || mediaId.isEmpty) {
      state = state.copyWith(backgroundAttribution: null);
      return;
    }

    try {
      final media = await _mediaRepository.get(mediaId);
      final attribution = media.hasAttribution() ? media.attribution : null;
      safeUpdateState((s) => s.copyWith(backgroundAttribution: attribution));

      if (attribution != null) {
        _log.info('✅ Loaded background attribution: ${attribution.creatorName}');
      }
    } catch (e) {
      _log.warning('⚠️ Failed to load background attribution for $mediaId: $e');
      // Don't fail - attribution is optional
      safeUpdateState((s) => s.copyWith(backgroundAttribution: null));
    }
  }

  /// Refetches the community from the repository and reloads
  /// [allMediaItems] from its `mediaIds`. Callers should invalidate the
  /// community cache first if they want guaranteed fresh data — used after
  /// chat-attached images are appended server-side so the carousel and
  /// chat-strip pick up the new IDs without a full screen reload.
  Future<void> reloadMediaFromCommunity() async {
    try {
      final community = await _repository.get(communityId);
      if (!ref.mounted) return;
      await loadAllMedia(community.mediaIds);
    } catch (e, stackTrace) {
      _log.warning('Failed to reload community media: $e', e, stackTrace);
    }
  }

  /// Loads all media items with thumbnail URLs for carousel display
  Future<void> loadAllMedia(List<String> mediaIds) async {
    if (mediaIds.isEmpty) {
      state = state.copyWith(allMediaItems: []);
      return;
    }

    try {
      final allMedia = <MediaItemData>[];

      for (final mediaId in mediaIds) {
        try {
          // Fetch media details to determine content type
          final media = await _mediaRepository.get(mediaId);

          // Use thumbnail URL for carousel performance
          final url = media.thumbnailUrl.isNotEmpty ? media.thumbnailUrl : media.url;

          allMedia.add(MediaItemData(
            id: mediaId,
            url: url,
            contentType: media.contentType,
            isVideo: media.contentType.startsWith('video/'),
            attribution: media.hasAttribution() ? media.attribution : null,
          ));
        } catch (e) {
          _log.warning('⚠️ Failed to load media item $mediaId: $e');
          // Continue loading other media items even if one fails
        }
      }

      safeUpdateState((s) => s.copyWith(allMediaItems: allMedia));
      _log.info('✅ Loaded ${allMedia.length} media items for carousel');
    } catch (e, stackTrace) {
      _log.severe('❌ Error loading all media: $e', e, stackTrace);
      // Don't fail the whole community if carousel media fails to load
    }
  }

  /// Deletes a media item from the community
  Future<void> deleteMedia(String mediaId, List<String> currentMediaIds) async {
    _log.info('🗑️ Deleting media: $mediaId from community');

    try {
      // Remove the mediaId from the list
      final updatedMediaIds = currentMediaIds
          .where((id) => id != mediaId)
          .toList();

      // Update community with reduced media_ids
      await _repository.updateCommunity(
        id: communityId,
        mediaIds: updatedMediaIds.isNotEmpty ? updatedMediaIds : null,
      );

      _log.info('✅ Media deleted successfully');

      // Reload media items
      await loadAllMedia(updatedMediaIds);
    } catch (e, stackTrace) {
      _log.severe('❌ Error deleting media: $e', e, stackTrace);
      safeUpdateState((s) => s.copyWith(
            error: const UserError.generic(fallback: 'Could not delete media'),
          ));
      rethrow;
    }
  }

  /// Reorders media items in the community
  Future<void> reorderMedia(List<String> mediaIds) async {
    _log.info('🔄 Reordering media: $mediaIds');

    try {
      // Update community with new media order
      await _repository.updateCommunity(
        id: communityId,
        mediaIds: mediaIds,
      );

      _log.info('✅ Media reordered successfully');

      // Reload media items
      await loadAllMedia(mediaIds);
    } catch (e, stackTrace) {
      _log.severe('❌ Error reordering media: $e', e, stackTrace);
      safeUpdateState((s) => s.copyWith(
            error: const UserError.generic(fallback: 'Could not reorder media'),
          ));
      rethrow;
    }
  }
}

/// Provider for community edit state.
/// Family provider allows separate state per community.
/// Uses .autoDispose to clean up state when widget is disposed.
final communityEditProvider = NotifierProvider.autoDispose.family<
    CommunityEditNotifier,
    CommunityEditState,
    String>(
  CommunityEditNotifier.new,
);
