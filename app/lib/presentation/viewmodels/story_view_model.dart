import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart' show StoryPayload;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/services/providers.dart';

part 'story_view_model.freezed.dart';

final _log = Logger('StoryViewModel');

/// State for story display functionality.
@freezed
sealed class StoryState with _$StoryState {
  const factory StoryState({
    required StoryPayload story,
    @Default([]) List<MediaUrl> mediaUrls,
    @Default([]) List<User> participants,
    @Default(false) bool isVideo,
    String? mediaPath,
    String? mediaId,
    UserError? error,
    // True while the full media URL is being resolved; the view shows
    // backgroundThumbnailUrl as a static first frame during this window.
    @Default(false) bool isBackgroundMediaLoading,
    // Server-generated thumbnail URL displayed as a static background while
    // the video controller initializes. Cleared once the controller is ready.
    String? backgroundThumbnailUrl,
  }) = _StoryState;

  const StoryState._();

  /// Returns whether the state has an error.
  bool get hasError => error != null;

  /// Returns whether the story has media.
  bool get hasMedia => mediaUrls.isNotEmpty;

  /// Returns the primary media URL for display.
  MediaUrl? get primaryMediaUrl => mediaUrls.isNotEmpty ? mediaUrls.first : null;
}

/// Notifier for managing story state.
/// This is a family notifier parameterized by StoryPayload.
class StoryNotifier extends AsyncNotifier<StoryState> {
  /// Constructor receives the story payload from the family provider.
  StoryNotifier(this.storyPayload);

  /// The story payload for this notifier instance.
  final StoryPayload storyPayload;

  @override
  Future<StoryState> build() async {
    _log.info('🏗️ StoryViewModel build() called for story type: ${storyPayload.storyType}');
    return _loadStory(storyPayload);
  }

  /// Loads story data including media URLs.
  ///
  /// Returns an initial state with thumbnail URL set immediately. For videos,
  /// fires a background task to resolve the full media URL so
  /// [VideoBackgroundHost] can pick it up and own the controller.
  Future<StoryState> _loadStory(StoryPayload story) async {
    try {
      final storyRepository = ref.read(storyRepositoryProvider);
      final mediaUrls = await storyRepository.loadMediaUrls(story);

      _log.info(
        '✅ Story loaded successfully: ${story.title}, ${mediaUrls.length} media items',
      );

      final primaryMedia = mediaUrls.isNotEmpty ? mediaUrls.first : null;
      final isVideo = primaryMedia?.contentType?.startsWith('video/') ?? false;

      // For videos, getMediaUrl returns the thumbnail URL as mediaUrl.url when
      // a thumbnail exists (isThumbnail == true). Use that as the static frame.
      final thumbnailForBg = (isVideo && primaryMedia != null && primaryMedia.isThumbnail)
          ? primaryMedia.url
          : null;

      final initialState = StoryState(
        story: story,
        mediaUrls: mediaUrls,
        participants: story.participants.toList(),
        isVideo: isVideo,
        mediaId: primaryMedia?.mediaId,
        // For non-video, set mediaPath directly. For video, mediaPath is set
        // after _loadFullVideoUrl resolves the streaming URL.
        mediaPath: (!isVideo && primaryMedia != null) ? primaryMedia.url : null,
        isBackgroundMediaLoading: isVideo && primaryMedia != null,
        backgroundThumbnailUrl: thumbnailForBg,
      );

      // Fire-and-forget: resolve the full video URL in the background.
      if (isVideo && primaryMedia != null) {
        unawaited(_loadFullVideoUrl(primaryMedia.mediaId));
      }

      return initialState;
    } catch (e, stackTrace) {
      _log.warning('❌ Error loading story: $e', e, stackTrace);
      return StoryState(
        story: story,
        error: const UserError.generic(fallback: 'Could not load story media'),
      );
    }
  }

  /// Resolves the full video URL after the initial state has been rendered
  /// with the thumbnail. The widget tree owns the [VideoPlayerController] —
  /// [VideoBackgroundHost] picks up [mediaPath] + [isVideo] from state and
  /// initializes its own controller.
  Future<void> _loadFullVideoUrl(String mediaId) async {
    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      // Use getFullMediaUrl to ensure we get the video stream, not a thumbnail.
      final fullUrl = await mediaRepository.getFullMediaUrl(mediaId);

      if (!ref.mounted) return;

      final current = state.value;
      if (current == null) return;
      state = AsyncValue.data(current.copyWith(
        mediaPath: fullUrl.url,
        mediaId: mediaId,
        isVideo: true,
        isBackgroundMediaLoading: false,
      ));
    } catch (e) {
      _log.warning('⚠️ Failed to resolve story video URL: $e');
      // Clear loading flag; the view will fall back to the thumbnail or black.
      if (!ref.mounted) return;
      final current = state.value;
      if (current == null) return;
      state = AsyncValue.data(current.copyWith(
        isBackgroundMediaLoading: false,
      ));
    }
  }

  /// Refreshes story data (reloads media URLs).
  Future<void> refresh() async {
    final currentState = state.value;
    if (currentState == null) return;

    state = const AsyncValue.loading();
    state = await AsyncValue.guard(() => _loadStory(currentState.story));
  }
}

/// Provider for the story state.
/// This is a family provider parameterized by StoryPayload.
///
/// Returns `AsyncValue<StoryState>` to properly model async operations.
/// Use `.when()` or selectors to access the state.
final storyProvider = AsyncNotifierProvider.autoDispose
    .family<StoryNotifier, StoryState, StoryPayload>(StoryNotifier.new);
