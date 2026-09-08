import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:image_picker/image_picker.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/services/providers.dart';

part 'community_view_model.freezed.dart';

final _log = Logger('CommunityViewModel');

/// Represents a single media item in the carousel
class MediaItemData {
  final String id;
  final String url;
  final String contentType;
  final bool isVideo;

  MediaItemData({
    required this.id,
    required this.url,
    required this.contentType,
    required this.isVideo,
  });
}

/// State for community edit functionality
@freezed
sealed class CommunityState with _$CommunityState {
  const factory CommunityState({
    String? communityId,
    String? communityName,
    String? communityDescription,
    String? mediaPath, // URL for image/video (first item)
    String? mediaId, // First media ID (for backward compatibility)
    @Default([]) List<MediaItemData> allMediaItems, // All media items for carousel
    @Default(0) int currentMediaIndex, // Current position in carousel
    @Default(false) bool isVideo,
    @Default(true) bool isMuted,
    // True while the video controller is being initialized; the view shows
    // backgroundThumbnailUrl as a static first frame during this window.
    @Default(false) bool isBackgroundMediaLoading,
    // Server-generated thumbnail URL displayed as a static background while
    // the video controller initializes. Cleared once the controller is ready.
    String? backgroundThumbnailUrl,
    @Default(true) bool isLoading,
    @Default(false) bool isEditing,
    @Default(false) bool isSaving,
    @Default(false) bool isUploadingMedia,
    UserError? error,
  }) = _CommunityState;

  const CommunityState._();

  /// Returns whether the state has an error
  bool get hasError => error != null;
}

/// Notifier for managing community edit state
class CommunityNotifier extends Notifier<CommunityState> {
  final ImagePicker _imagePicker = ImagePicker();

  @override
  CommunityState build() {
    return const CommunityState();
  }

  /// Initializes and loads community details
  Future<void> initialize(String communityId) async {
    state = state.copyWith(
      communityId: communityId,
      isLoading: true,
      error: null,
    );
    await loadCommunityDetails();
  }

  /// Loads community details
  Future<void> loadCommunityDetails() async {
    final communityId = state.communityId;
    if (communityId == null) return;

    _log.info('📥 Loading community details for ID: $communityId');

    state = state.copyWith(
      isLoading: true,
      error: null,
    );

    try {
      final communityRepository = ref.read(communityRepositoryProvider);
      final response = await communityRepository.get(communityId);

      final hasBgMedia = response.mediaIds.isNotEmpty;

      if (hasBgMedia) {
        await _loadAllMediaFromServer(response.mediaIds);
        if (!ref.mounted) return;
      }

      // Mark metadata loading done; surface the thumbnail immediately so
      // text content is visible while the video controller initializes.
      // allMediaItems.url already holds the thumbnail URL (see _loadAllMediaFromServer).
      state = state.copyWith(
        communityName: response.name,
        communityDescription: response.description,
        isLoading: false,
        isBackgroundMediaLoading: hasBgMedia,
        backgroundThumbnailUrl:
            hasBgMedia ? state.allMediaItems.firstOrNull?.url : null,
        error: null,
      );

      // Fire-and-forget: resolve the full media URL after the UI has already
      // rendered with textual content + thumbnail.
      if (hasBgMedia) {
        unawaited(_loadMediaFromServer(response.mediaIds.first));
      }

      _log.info('✅ Community details loaded');
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to load community details: $e', e, stackTrace);
      state = state.copyWith(
        isLoading: false,
        error: RpcErrorHandler.classify(e),
      );
    }
  }

  /// Loads media URL from server. The widget tree owns the video controller —
  /// [VideoBackgroundHost] picks up [mediaPath] + [isVideo] from state and
  /// initializes its own controller.
  Future<void> _loadMediaFromServer(String mediaId) async {
    _log.info('📥 Loading media from server: $mediaId');
    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaUrl = await mediaRepository.getMediaUrl(mediaId);

      // Use content type for reliable video detection (not URL heuristics)
      final isVideo = mediaUrl.contentType?.startsWith('video/') ?? false;

      state = state.copyWith(
        mediaPath: mediaUrl.url,
        mediaId: mediaId,
        isVideo: isVideo,
        isBackgroundMediaLoading: false,
        backgroundThumbnailUrl: null,
      );

      _log.info('✅ Media loaded successfully');
    } catch (e, stackTrace) {
      _log.warning('⚠️ Failed to load media: $e', e, stackTrace);
      // Clear loading flag so the UI doesn't remain stuck on the thumbnail.
      if (ref.mounted) {
        state = state.copyWith(isBackgroundMediaLoading: false);
      }
    }
  }

  /// Loads all media items from server for carousel support
  Future<void> _loadAllMediaFromServer(List<String> mediaIds) async {
    if (mediaIds.isEmpty) return;

    _log.info('📥 Loading ${mediaIds.length} media items from server');

    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final allMedia = <MediaItemData>[];

      for (final mediaId in mediaIds) {
        try {
          final media = await mediaRepository.get(mediaId);
          final url = media.thumbnailUrl.isNotEmpty ? media.thumbnailUrl : media.url;

          allMedia.add(MediaItemData(
            id: mediaId,
            url: url,
            contentType: media.contentType,
            isVideo: media.contentType.startsWith('video/'),
          ));
        } catch (e) {
          _log.warning('⚠️ Failed to load media item $mediaId: $e');
          // Continue loading other media items
        }
      }

      state = state.copyWith(allMediaItems: allMedia);
      _log.info('✅ Loaded ${allMedia.length} media items');
    } catch (e, stackTrace) {
      _log.warning('⚠️ Failed to load media items: $e', e, stackTrace);
      // Don't fail the whole operation
    }
  }

  /// Sets the current media index for carousel navigation
  void setCurrentMediaIndex(int index) {
    if (index >= 0 && index < state.allMediaItems.length) {
      _log.info('📍 Setting current media index to $index');
      state = state.copyWith(currentMediaIndex: index);
    }
  }

  /// Toggles edit mode
  void toggleEditMode() {
    state = state.copyWith(isEditing: !state.isEditing);
  }

  /// Cancels editing and reverts changes
  void cancelEdit() {
    state = state.copyWith(isEditing: false);
  }

  /// Toggles video mute state.
  void toggleMute() {
    state = state.copyWith(isMuted: !state.isMuted);
  }

  /// Saves community changes
  Future<void> saveChanges({
    required String name,
    required String description,
  }) async {
    final communityId = state.communityId;
    if (communityId == null) return;

    _log.info('💾 Saving community changes...');

    state = state.copyWith(isSaving: true);

    try {
      final communityRepository = ref.read(communityRepositoryProvider);
      await communityRepository.updateCommunity(
        id: communityId,
        name: name.trim(),
        description: description.trim(),
        mediaId: state.mediaId,
      );

      // Reload community details to get updated data
      await loadCommunityDetails();

      state = state.copyWith(
        isEditing: false,
        isSaving: false,
      );

      _log.info('✅ Community saved successfully');
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to save community: $e', e, stackTrace);
      state = state.copyWith(isSaving: false);
      rethrow;
    }
  }

  /// Picks image from gallery. Works on every platform via XFile —
  /// see UserProfileViewModel.pickImageFromGallery for the full
  /// rationale.
  Future<void> pickImageFromGallery() async {
    final XFile? image = await _imagePicker.pickImage(
      source: ImageSource.gallery,
      imageQuality: 85,
    );

    if (image != null) {
      await _uploadMedia(image, isVideo: false);
    }
  }

  /// Picks image from camera.
  Future<void> pickImageFromCamera() async {
    final XFile? image = await _imagePicker.pickImage(
      source: ImageSource.camera,
      imageQuality: 85,
    );

    if (image != null) {
      await _uploadMedia(image, isVideo: false);
    }
  }

  /// Picks video from gallery.
  Future<void> pickVideoFromGallery() async {
    final XFile? video = await _imagePicker.pickVideo(
      source: ImageSource.gallery,
    );

    if (video != null) {
      await _uploadMedia(video, isVideo: true);
    }
  }

  /// Uploads media and associates it with the community.
  Future<void> _uploadMedia(XFile file, {required bool isVideo}) async {
    _log.info('📤 Uploading media: ${file.name} (isVideo: $isVideo)');

    state = state.copyWith(isUploadingMedia: true);

    try {
      // Upload media to server first
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaId = await mediaRepository.addMedia(
        file: file,
        description:
            'Community media for ${state.communityName ?? "community"}',
      );

      // After upload, fetch the media to get the URL and content type
      final mediaResponse = await mediaRepository.getMedia(mediaId);
      final uploadedIsVideo = mediaResponse.contentType.startsWith('video/');

      state = state.copyWith(
        mediaPath: mediaResponse.url,
        mediaId: mediaId,
        isVideo: uploadedIsVideo,
        isUploadingMedia: false,
      );

      _log.info('✅ Media uploaded successfully: $mediaId');

      // If not in edit mode, automatically save the media association
      if (!state.isEditing) {
        final communityId = state.communityId;
        if (communityId != null) {
          final communityRepository = ref.read(communityRepositoryProvider);
          await communityRepository.updateCommunity(
            id: communityId,
            mediaId: mediaId,
          );
          _log.info('✅ Media associated with community');
        }
      }
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to upload media: $e', e, stackTrace);
      state = state.copyWith(isUploadingMedia: false);
      rethrow;
    }
  }

  /// Refreshes community data
  Future<void> refresh() async {
    await loadCommunityDetails();
  }
}

/// Provider for community state
final communityProvider = NotifierProvider<CommunityNotifier, CommunityState>(
  CommunityNotifier.new,
);
