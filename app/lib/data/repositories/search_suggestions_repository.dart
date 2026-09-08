import 'package:logging/logging.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/services/search_service.dart';

export 'package:ripls/services/search_service.dart'
    show GetSearchSuggestionsResponse, SharedCommunityRef;

final _log = Logger('SearchSuggestionsRepository');

/// Cache key prefix used by [SearchSuggestionsRepository]. Exposed so
/// neighboring repositories can target their invalidation calls (e.g.
/// CommunityRepository on join/leave, LoanRepository on completion).
const _kSuggestionsKeyPrefix = 'search_suggestions:';

/// Repository for personalized search-panel suggestions.
///
/// Wraps `SearchService.getSuggestions` with transparent caching keyed
/// on `userId`. TTL follows the global `CACHE_TTL_MINUTES`; per-entry
/// TTL is not supported. Invalidation is
/// push-based: call [refreshSuggestions] from mutation paths that
/// change the underlying signals (community join/leave,
/// loan/experience/request completion).
class SearchSuggestionsRepository {
  final CacheManager _cache;
  final SearchService _service;
  final void Function()? _onCacheInvalidated;

  SearchSuggestionsRepository(
    this._cache,
    this._service, [
    this._onCacheInvalidated,
  ]);

  /// Returns the suggestions-panel payload for [userId]. The server
  /// derives every signal (top communities, top known-for categories)
  /// from the authenticated user — no client-side scope is passed in.
  Future<GetSearchSuggestionsResponse> getSuggestions({
    required String userId,
  }) async {
    final cacheKey = '$_kSuggestionsKeyPrefix$userId';
    return _cache.get(
      key: cacheKey,
      fetch: () {
        _log.info('Cache miss for $cacheKey - fetching from server');
        return _service.getSuggestions();
      },
    );
  }

  /// Drops every cached suggestions entry. Call from any mutation
  /// path that can shift the categories or community list — community
  /// join/leave, loan completion, experience host completion, request
  /// fulfillment.
  Future<void> refreshSuggestions() async {
    _log.info('refreshSuggestions(): clearing suggestions cache');
    await _cache.clear(pattern: '$_kSuggestionsKeyPrefix*');
    _onCacheInvalidated?.call();
  }
}
