import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/services/feed_service.dart';

/// FeedRepository manages feed data access with caching.
class FeedRepository {
  final FeedService _service;
  final CacheService _cache;
  final void Function()? _onFeedStatusInvalidated;

  FeedRepository(
    CacheManager cacheManager,
    this._service, {
    void Function()? onFeedStatusInvalidated,
  })  : _cache = CacheService(cacheManager, 'feed'),
        _onFeedStatusInvalidated = onFeedStatusInvalidated;

  /// Get feed with automatic caching.
  ///
  /// Uses a single cache key ('feed:list') regardless of which communities are
  /// enabled. The feed cache is invalidated when the enabled set changes.
  Future<GetFeedResponse> getFeed(
    List<String> communityIds, {
    String? pageToken,
  }) {
    final key = pageToken == null ? 'feed:list' : 'feed:list:$pageToken';

    return _cache.get(
      key: key,
      fetch: () => _service.getFeed(
        communityIds: communityIds,
        pageSize: 20,
        pageToken: pageToken,
      ),
    );
  }

  /// GetFeedStatus returns whether each of the user's communities has unseen feed items.
  ///
  /// The result is cached briefly; call [invalidateFeedStatus] after marking
  /// items as viewed to ensure the sidebar indicator reflects the latest state.
  Future<GetFeedStatusResponse> getFeedStatus() {
    return _cache.get(
      key: 'status',
      fetch: () => _service.getFeedStatus(),
    );
  }

  /// InvalidateFeedStatus clears the cached feed status so the next call
  /// fetches fresh data from the server.
  Future<void> invalidateFeedStatus() async {
    await _cache.invalidate('status');
  }

  /// ConsumeNudge marks a nudge as consumed and invalidates the feed cache.
  ///
  /// The cache invalidation ensures the consumed nudge is excluded from the
  /// next feed load. The RPC itself is fire-and-forget (no await needed at
  /// the call site).
  Future<void> consumeNudge(String nudgeId, String action) async {
    await _service.consumeNudge(nudgeId: nudgeId, action: action);
    // Invalidate all feed caches so the consumed nudge disappears on next load.
    await _cache.invalidateAll();
  }

  /// Mark items as viewed and invalidate cache.
  Future<void> markItemsViewed(String communityId, List<String> itemIds) async {
    await _service.markFeedItemsViewed(
      communityId: communityId,
      itemIds: itemIds,
    );

    // Invalidate feed list cache and status.
    await _cache.invalidate('feed:list');
    await invalidateFeedStatus();

    // Signal the invalidation provider so FeedStatusNotifier rebuilds reactively.
    _onFeedStatusInvalidated?.call();
  }

  /// Invalidate feed list cache.
  Future<void> invalidateFeed() async {
    await _cache.invalidate('feed:list');
  }

  /// Invalidate all feed caches across all communities.
  Future<void> invalidateAllFeeds() async {
    await _cache.invalidateAll();
  }
}
