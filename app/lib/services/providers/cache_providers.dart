import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/utils/logout_diagnostics.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';

/// Records a cache-invalidation `notify()` call. Always cheap (a single
/// ring-buffer append + optional log line). The trace is the load-bearing
/// observability hook for the #2158 logout-frame race: a `notify()` firing
/// during or just after logout is the leading hypothesis for the
/// RenderIgnorePointer crash.
void _traceNotify(String notifierName) {
  LogoutDiagnostics.trace('CACHE_INVALIDATE_NOTIFY', notifierName);
}

/// Provider for CacheManager (singleton, using Stash with memory storage)
final cacheManagerProvider = Provider<CacheManager>((ref) {
  final manager = StashCacheManager();
  // Initialize asynchronously - the manager handles auto-initialization
  // on first use if not explicitly initialized
  return manager;
});

/// Provider for search cache invalidation notifications.
/// Incremented each time search cache is invalidated to trigger listeners.
final searchCacheInvalidationProvider = NotifierProvider<SearchCacheInvalidationNotifier, int>(
  SearchCacheInvalidationNotifier.new,
);

/// Notifier for search cache invalidation events.
class SearchCacheInvalidationNotifier extends Notifier<int> {
  @override
  int build() => 0;

  void notify() {
    _traceNotify('search');
    state++;
  }
}

/// Provider for ESM (embedded story prompt and feed card) cache invalidation
/// notifications. Incremented when a vote is submitted so the Story viewer
/// can refetch the social-proof tally without invalidating the entire feed.
final esmCacheInvalidationProvider =
    NotifierProvider<EsmCacheInvalidationNotifier, int>(
  EsmCacheInvalidationNotifier.new,
);

/// Notifier for ESM cache invalidation events.
class EsmCacheInvalidationNotifier extends Notifier<int> {
  @override
  int build() => 0;

  void notify() {
    _traceNotify('esm');
    state++;
  }
}

/// Provider for feed status cache invalidation notifications.
///
/// Incremented each time FeedRepository.markItemsViewed() is called successfully.
/// FeedStatusNotifier watches this in build() to rebuild reactively when items
/// are marked viewed, ensuring the sidebar New indicator reflects fresh status.
final feedStatusCacheInvalidationProvider =
    NotifierProvider<FeedStatusCacheInvalidationNotifier, int>(
  FeedStatusCacheInvalidationNotifier.new,
);

/// Notifier for feed status cache invalidation events.
class FeedStatusCacheInvalidationNotifier extends Notifier<int> {
  @override
  int build() => 0;

  void notify() {
    _traceNotify('feedStatus');
    state++;
  }
}

/// Provider for feed listing cache invalidation notifications.
///
/// Incremented when a community event structurally changes the feed listing
/// (items added or removed: gear shared/unshared, request created/cancelled/fulfilled,
/// experience created). FeedNotifier listens to this to trigger in-place refreshes.
/// Detail-level events (RSVPs, offers) do NOT fire this provider — they fire
/// contentCacheInvalidationProvider only, so the feed is never needlessly reloaded.
final feedListingCacheInvalidationProvider =
    NotifierProvider<FeedListingCacheInvalidationNotifier, int>(
  FeedListingCacheInvalidationNotifier.new,
);

/// Notifier for feed listing cache invalidation events.
class FeedListingCacheInvalidationNotifier extends Notifier<int> {
  @override
  int build() => 0;

  void notify() {
    _traceNotify('feedListing');
    state++;
  }
}

/// Provider for portfolio screen cache invalidation notifications.
///
/// Incremented each time a mutation repository performs a change that
/// affects portfolio data (items, state, metrics, conversations).
/// PortfolioInboxNotifier listens to this provider to trigger background refreshes.
final portfolioCacheInvalidationProvider = NotifierProvider<PortfolioCacheInvalidationNotifier, int>(
  PortfolioCacheInvalidationNotifier.new,
);

/// Notifier for portfolio cache invalidation events.
class PortfolioCacheInvalidationNotifier extends Notifier<int> {
  @override
  int build() => 0;

  void notify() {
    _traceNotify('portfolio');
    state++;
  }
}

/// Provider for content view cache invalidation notifications.
///
/// Incremented after chat mutations (sendMessage, markRead) so that content
/// view ViewModels (GearViewModel, ExperienceViewModel, RequestViewModel)
/// can refresh their denormalized comment preview fields.
final contentCacheInvalidationProvider = NotifierProvider<ContentCacheInvalidationNotifier, int>(
  ContentCacheInvalidationNotifier.new,
);

/// Notifier for content view cache invalidation events.
class ContentCacheInvalidationNotifier extends Notifier<int> {
  @override
  int build() => 0;

  void notify() {
    _traceNotify('content');
    state++;
  }
}

/// Provider for transfer cache invalidation notifications.
///
/// Incremented when system chat messages indicate transfer state changes
/// (interest expressed, recipient selected, pickup proposed, completed,
/// cancelled). Gear content views listen to this to refresh transfer
/// context so the other party sees updated state in real time.
final transferCacheInvalidationProvider = NotifierProvider<TransferCacheInvalidationNotifier, int>(
  TransferCacheInvalidationNotifier.new,
);

/// Notifier for transfer cache invalidation events.
class TransferCacheInvalidationNotifier extends Notifier<int> {
  @override
  int build() => 0;

  void notify() {
    _traceNotify('transfer');
    state++;
  }
}

/// Provider for deleted-communities cache invalidation notifications.
///
/// Incremented when the `community:deleted:list` cache is cleared
/// (after `DeleteCommunity` or `RestoreCommunity`).
/// `DeletedCommunitiesNotifier` watches this in `build()` so the
/// Settings → Communities recently-deleted section refreshes
/// without waiting for the autoDispose grace period to fire.
final deletedCommunitiesCacheInvalidationProvider =
    NotifierProvider<DeletedCommunitiesCacheInvalidationNotifier, int>(
  DeletedCommunitiesCacheInvalidationNotifier.new,
);

/// Notifier for deleted-communities cache invalidation events.
class DeletedCommunitiesCacheInvalidationNotifier extends Notifier<int> {
  @override
  int build() => 0;

  void notify() {
    _traceNotify('deletedCommunities');
    state++;
  }
}

/// Provider for rejoinable-communities cache invalidation notifications.
///
/// Incremented when the `community:rejoinable:list` cache is cleared
/// (after `LeaveCommunity` adds a soft-deleted membership row, or
/// after `RejoinCommunity` removes the user from the rejoinable set).
/// `RejoinableCommunitiesNotifier` watches this in `build()` so the
/// Settings → Communities recently-left section refreshes without
/// waiting for the autoDispose grace period to fire.
final rejoinableCommunitiesCacheInvalidationProvider =
    NotifierProvider<RejoinableCommunitiesCacheInvalidationNotifier, int>(
  RejoinableCommunitiesCacheInvalidationNotifier.new,
);

/// Notifier for rejoinable-communities cache invalidation events.
class RejoinableCommunitiesCacheInvalidationNotifier extends Notifier<int> {
  @override
  int build() => 0;

  void notify() {
    _traceNotify('rejoinableCommunities');
    state++;
  }
}

/// Provider for impact metrics cache invalidation notifications.
///
/// Incremented after experience completion or request fulfillment so that
/// community impact and portfolio metrics ViewModels can refresh in place
/// without showing a full loading spinner. Follows the reactive event-driven
/// invalidation pattern (Pattern 7 from caching.md).
/// Repositories fire this signal; ViewModels listen to it in build().
final impactCacheInvalidationProvider = NotifierProvider<ImpactCacheInvalidationNotifier, int>(
  ImpactCacheInvalidationNotifier.new,
);

/// Notifier for impact metrics cache invalidation events.
class ImpactCacheInvalidationNotifier extends Notifier<int> {
  @override
  int build() => 0;

  void notify() {
    _traceNotify('impact');
    state++;
  }
}
