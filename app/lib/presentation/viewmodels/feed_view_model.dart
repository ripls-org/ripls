import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/services/providers.dart';

part 'feed_view_model.freezed.dart';

final _log = Logger('FeedViewModel');

@freezed
sealed class FeedState with _$FeedState {
  const factory FeedState({
    @Default([]) List<FeedItem> items,
    @Default(true) bool isLoading,
    @Default(false) bool isLoadingMore,
    @Default(false) bool hasMore,
    UserError? error,
    String? nextPageToken,

    /// The community IDs currently being shown in the feed.
    @Default(<String>[]) List<String> currentCommunityIds,
    @Default(0) int currentPageIndex,
    @Default(<String>{}) Set<String> consumedNudgeIds,
  }) = _FeedState;

  const FeedState._();

  bool get hasError => error != null;
  bool get isEmpty => items.isEmpty && !isLoading;
}

class FeedNotifier extends Notifier<FeedState> {
  FeedRepository get _repository => ref.read(feedRepositoryProvider);

  Timer? _debounceTimer;
  static const _debounceDelay = Duration(milliseconds: 300);

  @override
  FeedState build() {
    ref.onDispose(() {
      _debounceTimer?.cancel();
    });

    // Refresh feed in-place when listing-level community events arrive
    // (gear shared, request created/cancelled/fulfilled, experience created).
    // Detail-level events (RSVPs, offers) fire only contentCacheInvalidationProvider
    // and do NOT trigger this path — they update individual content views silently.
    ref.listen(feedListingCacheInvalidationProvider, (_, _) {
      if (state.currentCommunityIds.isNotEmpty && !state.isLoading) {
        _debounceTimer?.cancel();
        _debounceTimer = Timer(_debounceDelay, _refreshInPlace);
      }
    });

    return const FeedState();
  }

  /// Initialize feed for one or more communities.
  ///
  /// Always invalidates the feed cache and fetches fresh data from the server.
  /// The feed must reflect the latest activity (new comments, events) every
  /// time the user opens it — stale cached data causes items with recent
  /// activity to appear in the wrong position.
  Future<void> initialize(List<String> communityIds) async {
    // Logout fires `ref.invalidate(feedProvider)` inside `_clearUserCaches()`.
    // If FeedScreen is still mounted (router redirect to /login hasn't fully
    // torn down the IndexedStack), the invalidation triggers a fresh build
    // which calls initialize() with no access token in scope. The RPC would
    // throw `unauthenticated` and surface as a severe "Failed to load feed"
    // log line. Bail early when the user is gone — there's no one to show
    // a feed to.
    if (ref.read(authStateProvider).user == null) {
      _log.fine('Skipping feed initialize: no authenticated user');
      return;
    }

    _log.info('Initializing feed for ${communityIds.length} communities');

    state = state.copyWith(
      isLoading: true,
      error: null,
      currentCommunityIds: communityIds,
    );

    try {
      await _repository.invalidateFeed();
      final response = await _repository.getFeed(communityIds);
      _log.info('Feed loaded: ${response.items.length} items');
      state = state.copyWith(
        items: response.items,
        isLoading: false,
        hasMore: response.nextPageToken.isNotEmpty,
        nextPageToken: response.nextPageToken,
      );
      // Marking item 0 viewed is the swipe feed's business, not this
      // notifier's: it exists only because setPageIndex(0) is a no-op when the
      // page controller settles on the first page. FeedScreen does it after
      // initialize() returns. Doing it here marked item 0 for every consumer,
      // including the Home pulse — a scrolling list that marks nothing else
      // viewed — so the newest post was the only one that ever expired while
      // older ones stayed forever (#2799).
    } catch (e, stackTrace) {
      _log.severe('Failed to load feed', e, stackTrace);
      state = state.copyWith(
        error: RpcErrorHandler.classify(e),
        isLoading: false,
      );
    }
  }

  /// Load next page.
  Future<void> loadMore() async {
    if (!state.hasMore ||
        state.isLoadingMore ||
        state.currentCommunityIds.isEmpty) {
      return;
    }

    state = state.copyWith(isLoadingMore: true);

    try {
      final response = await _repository.getFeed(
        state.currentCommunityIds,
        pageToken: state.nextPageToken,
      );

      state = state.copyWith(
        items: [...state.items, ...response.items],
        isLoadingMore: false,
        hasMore: response.nextPageToken.isNotEmpty,
        nextPageToken: response.nextPageToken,
      );
    } catch (e) {
      state = state.copyWith(isLoadingMore: false);
    }
  }

  /// Mark current item as viewed.
  Future<void> markCurrentItemViewed() async {
    if (state.currentCommunityIds.isEmpty || state.items.isEmpty) return;
    if (state.currentPageIndex >= state.items.length) return;

    final currentItem = state.items[state.currentPageIndex];

    try {
      // Use the first enabled community for the view record. The server uses
      // this to scope the last_seen_at timestamp per community.
      await _repository.markItemsViewed(state.currentCommunityIds.first, [
        currentItem.id,
      ]);

      // Optimistically clear the unread indicator so it disappears immediately
      // without waiting for a full feed reload.
      if (currentItem.isUnread) {
        final updatedItem = FeedItem()
          ..mergeFromMessage(currentItem)
          ..isUnread = false;

        final updatedItems = List<FeedItem>.from(state.items);
        updatedItems[state.currentPageIndex] = updatedItem;

        state = state.copyWith(items: updatedItems);
      }
    } catch (e) {
      _log.warning('Failed to mark item as viewed: $e');
      // Don't show error to user, this is a background operation
    }
  }

  /// Update current page index.
  void setPageIndex(int index) {
    if (index == state.currentPageIndex) return;

    state = state.copyWith(currentPageIndex: index);

    // Mark as viewed when user scrolls to it
    markCurrentItemViewed();

    // Prefetch next page if approaching end
    if (state.hasMore && index >= state.items.length - 3) {
      loadMore();
    }
  }

  /// Refresh feed data in-place without showing a loading spinner.
  ///
  /// Called when community event polling detects content changes. Preserves
  /// the user's current scroll position by keeping currentPageIndex.
  Future<void> _refreshInPlace() async {
    final communityIds = state.currentCommunityIds;
    if (communityIds.isEmpty) return;

    _log.info(
      'refreshing feed in-place for ${communityIds.length} communities',
    );

    try {
      await _repository.invalidateFeed();
      final response = await _repository.getFeed(communityIds);
      state = state.copyWith(
        items: response.items,
        hasMore: response.nextPageToken.isNotEmpty,
        nextPageToken: response.nextPageToken,
      );
    } catch (e) {
      _log.warning('in-place feed refresh failed: $e');
      // Swallow — user still sees their existing feed.
    }
  }

  /// Consume a nudge: optimistically dismiss it from the current feed,
  /// then fire the ConsumeNudge RPC asynchronously.
  ///
  /// Per the design doc: "fire-and-forget" — if the RPC fails the nudge may
  /// reappear on the next feed load, which is an acceptable trade-off for
  /// smooth UX.
  void consumeNudge(String nudgeId, String action) {
    // Optimistic dismiss: add to consumed set so the feed hides it immediately.
    state = state.copyWith(
      consumedNudgeIds: {...state.consumedNudgeIds, nudgeId},
    );
    // Fire-and-forget RPC + cache invalidation.
    _repository.consumeNudge(nudgeId, action).catchError((Object e) {
      _log.warning('ConsumeNudge RPC failed (nudge may reappear): $e');
    });
  }

  /// Refresh feed and all related caches.
  ///
  /// This performs a comprehensive refresh by:
  /// 1. Invalidating the feed cache for the current community
  /// 2. Clearing all related content caches (gear, requests, users, media)
  /// 3. Reloading fresh data from the server
  ///
  /// Server-side filtering ensures completed/
  /// cancelled items are automatically excluded from the feed:
  /// - Giveaways filtered by CommunityGear.archived
  /// - Requests filtered by CommunityRequest.archived
  /// - Experiences filtered by CommunityExperience.archived
  ///
  /// This ensures that all nested data (user profiles, media, gear details, etc.)
  /// displayed in feed items is refreshed, not just the feed list itself.
  Future<void> refresh() async {
    // Always invalidate all feed caches so the next initialize() call fetches
    // fresh data, even if the feed has never been loaded (currentCommunityId == null).
    await _repository.invalidateAllFeeds();

    final communityIds = state.currentCommunityIds;
    if (communityIds.isEmpty) return;

    _log.info('Refreshing feed for ${communityIds.length} communities');

    // Clear all related content caches to ensure fresh nested data
    // This delegates to repositories to clear:
    // - All gear items (shown in FEED_ITEM_TYPE_GEAR_SHARED)
    // - All requests (shown in FEED_ITEM_TYPE_REQUEST_CREATED)
    // - All user profiles (owners of gear/requests)
    // - All media (thumbnails and full images)
    await Future.wait([
      ref.read(gearRepositoryProvider).invalidateAll(),
      ref.read(requestRepositoryProvider).invalidateAll(),
      ref.read(userRepositoryProvider).invalidateAll(),
      ref.read(mediaRepositoryProvider).invalidateAll(),
    ]);

    // Invalidate gear providers for items in current feed to force refetch
    // This ensures GearContentView gets fresh data when rebuilt
    for (final item in state.items) {
      if (item.itemType == FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED) {
        ref.invalidate(gearProvider(item.gearShared.gearId));
      }
    }

    // Reset items and loading state while preserving currentCommunityIds.
    // Keeping currentCommunityIds non-empty prevents FeedScreen.build() from
    // scheduling a second concurrent initialize() call via addPostFrameCallback,
    // which would race with the one below and cause the feed to not refresh.
    state = FeedState(currentCommunityIds: communityIds);
    await initialize(communityIds);
  }

  /// Refreshes feed data in-place without showing a loading spinner.
  ///
  /// Invalidates all caches and fetches fresh data, then swaps items directly
  /// into state without transitioning through isLoading: true. This keeps the
  /// PageView mounted throughout, ensuring the PageController always has clients
  /// when navigateToTab scrolls to position 0 after the refresh completes.
  ///
  /// Use this for programmatic post-action refreshes (e.g., after completing an
  /// experience or creating new content). Use [refresh] for explicit
  /// user-initiated pull-to-refresh, which intentionally shows a loading state.
  Future<void> refreshInPlace() async {
    final communityIds = state.currentCommunityIds;
    if (communityIds.isEmpty) return;

    _log.info(
      'Refreshing feed in place for ${communityIds.length} communities',
    );

    await _repository.invalidateAllFeeds();

    await Future.wait([
      ref.read(gearRepositoryProvider).invalidateAll(),
      ref.read(requestRepositoryProvider).invalidateAll(),
      ref.read(userRepositoryProvider).invalidateAll(),
      ref.read(mediaRepositoryProvider).invalidateAll(),
    ]);

    // Intentionally do NOT invalidate per-item providers here. The
    // PageView preserves existing ContentView widgets across an in-place
    // refresh via KeyedSubtree(ValueKey(item.id)) — they keep their State
    // object and only re-render when ref.watch sees a state change. If we
    // ref.invalidate(gearProvider/...) the notifier resets to its initial
    // (isLoading=true, *Details=null) state but the widget's one-shot
    // initState post-frame load never re-fires, so the existing
    // ContentView stays stuck on a loading spinner forever. The
    // repository invalidations above are enough to ensure any newly
    // mounted ContentView (e.g. for a newly-arrived feed item) fetches
    // fresh data on its first load.

    try {
      final response = await _repository.getFeed(communityIds);
      _log.info('Feed refreshed in place: ${response.items.length} items');
      state = state.copyWith(
        items: response.items,
        hasMore: response.nextPageToken.isNotEmpty,
        nextPageToken: response.nextPageToken,
      );
      unawaited(markCurrentItemViewed());
    } catch (e, stackTrace) {
      _log.severe('Failed to refresh feed in place', e, stackTrace);
      // On error keep existing items visible rather than showing an error state.
    }
  }
}

final feedProvider = NotifierProvider<FeedNotifier, FeedState>(
  FeedNotifier.new,
);

/// FeedStatusNotifier tracks whether any community has unseen feed items.
///
/// The status is fetched once on first access and re-fetched whenever
/// [refresh] is called (e.g., after the user views items in the feed).
/// The map value is `true` when the community has items newer than the
/// user's last_seen_at.
class FeedStatusNotifier extends AsyncNotifier<Map<String, bool>> {
  FeedRepository get _repository => ref.read(feedRepositoryProvider);

  @override
  Future<Map<String, bool>> build() async {
    // Watch the invalidation counter so Riverpod re-runs build() whenever
    // FeedRepository.markItemsViewed() fires _onFeedStatusInvalidated.
    // This is the canonical Pattern 7 from docs/client/caching.md.
    ref.watch(feedStatusCacheInvalidationProvider);

    final response = await _repository.getFeedStatus();
    return response.communityIdToHasNew;
  }

  /// Refresh re-fetches feed status from the server.
  Future<void> refresh() async {
    await _repository.invalidateFeedStatus();
    ref.invalidateSelf();
  }
}

final feedStatusProvider =
    AsyncNotifierProvider<FeedStatusNotifier, Map<String, bool>>(
      FeedStatusNotifier.new,
    );

/// communityHasNewFeedProvider returns true when the given community has
/// unseen feed items.  Falls back to false on loading or error.
final communityHasNewFeedProvider = Provider.family<bool, String>((
  ref,
  communityId,
) {
  final status = ref.watch(feedStatusProvider);
  return status.asData?.value[communityId] ?? false;
});
