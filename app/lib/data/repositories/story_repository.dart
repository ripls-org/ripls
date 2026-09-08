import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/services/feed_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:ripls/services/user_service.dart';

/// Repository for story data with transparent caching.
///
/// Provides access to community and user stories with caching support.
/// Uses the global TTL from the environment (CACHE_TTL_MINUTES).
class StoryRepository {
  final CacheService _cache;
  final FeedService _feedService;
  final UserService _userService;
  final MediaRepository _mediaRepository;

  StoryRepository(
    CacheManager cacheManager,
    this._feedService,
    this._userService,
    this._mediaRepository,
  ) : _cache = CacheService(cacheManager, 'story');

  /// Loads media URLs for a story's media IDs.
  ///
  /// Returns a list of MediaUrl objects for all media IDs in the story.
  /// Returns an empty list if the story has no media.
  Future<List<MediaUrl>> loadMediaUrls(StoryPayload story) async {
    if (story.mediaIds.isEmpty) {
      return [];
    }

    return Future.wait(
      story.mediaIds.map((id) => _mediaRepository.getMediaUrl(id)),
    );
  }

  /// Loads a single media URL for display.
  ///
  /// Returns the first media URL if available, null otherwise.
  Future<MediaUrl?> loadPrimaryMediaUrl(StoryPayload story) async {
    if (story.mediaIds.isEmpty) {
      return null;
    }

    return _mediaRepository.getMediaUrl(story.mediaIds.first);
  }

  /// Lists stories for a community with caching.
  ///
  /// Returns up to [limit] stories ordered by creation time (newest first).
  /// Results are cached using the key 'community:$communityId'.
  Future<List<StoryPayload>> listByCommunity(
    String communityId, {
    int limit = 10,
  }) async {
    return _cache.get(
      key: 'community:$communityId',
      fetch: () => _feedService.listStories(
        communityId: communityId,
        limit: limit,
      ),
    );
  }

  /// Lists stories for a user with caching.
  ///
  /// Returns up to [limit] stories where the user is a participant,
  /// ordered by creation time (newest first).
  /// Results are cached using the key 'user:$userId'.
  Future<List<StoryPayload>> listByUser(
    String userId, {
    int limit = 10,
  }) async {
    return _cache.get(
      key: 'user:$userId',
      fetch: () => _userService.listUserStories(
        userId: userId,
        limit: limit,
      ),
    );
  }

  /// Lists stories for a specific item (gear, request, or experience) with caching.
  ///
  /// Returns up to [limit] stories for the specified item,
  /// ordered by creation time (newest first).
  /// Results are cached using the key 'item:$itemId'.
  Future<List<StoryPayload>> listByItem(
    String itemId, {
    int limit = 10,
  }) async {
    return _cache.get(
      key: 'item:$itemId',
      fetch: () => _feedService.listItemStories(
        itemId: itemId,
        limit: limit,
      ),
    );
  }

  /// Invalidates the story cache for a specific community.
  ///
  /// Call this after mutations that affect stories (e.g., completing a loan)
  /// or on pull-to-refresh to ensure fresh data.
  Future<void> invalidateCommunity(String communityId) async {
    return _cache.invalidate('community:$communityId');
  }

  /// Invalidates the story cache for a specific user.
  ///
  /// Call this after mutations that affect user stories or on pull-to-refresh.
  Future<void> invalidateUser(String userId) async {
    return _cache.invalidate('user:$userId');
  }

  /// Invalidates the story cache for a specific item.
  ///
  /// Call this after mutations that affect item stories (e.g., completing a loan,
  /// fulfilling a request) or on pull-to-refresh.
  Future<void> invalidateItem(String itemId) async {
    return _cache.invalidate('item:$itemId');
  }
}

/// Riverpod provider for StoryRepository.
final storyRepositoryProvider = Provider<StoryRepository>((ref) {
  return StoryRepository(
    ref.watch(cacheManagerProvider),
    ref.watch(feedServiceProvider),
    ref.watch(userServiceProvider),
    ref.watch(mediaRepositoryProvider),
  );
});
