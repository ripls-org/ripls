import 'package:logging/logging.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/services/search_service.dart';

export 'package:ripls/services/search_service.dart'
    show SearchItemType, SearchStrategy;

final _log = Logger('SearchRepository');

/// Repository for search operations with transparent caching.
///
/// This repository wraps SearchService and provides caching for unified
/// search results using the global TTL from environment (CACHE_TTL_MINUTES).
class SearchRepository {
  final CacheManager _cache;
  final SearchService _service;
  final void Function()? _onCacheInvalidated;

  SearchRepository(this._cache, this._service, [this._onCacheInvalidated]);

  /// Searches for gear, requests, and experiences within a community.
  ///
  /// Results are cached based on the search parameters (query, community, location).
  /// Use different search parameters to get different cached results.
  ///
  /// Parameters:
  /// - [query]: The search query string (searches name/title and description)
  /// - [communityId]: The unique identifier of the community to search within
  /// - [latitudeDeg]: User's current latitude in degrees
  /// - [longitudeDeg]: User's current longitude in degrees
  /// - [maxResults]: Maximum number of results to return (default: 50 on server)
  /// - [itemTypes]: Optional list of item types to filter results. If null,
  ///   defaults to gear, requests, experiences, and users.
  /// - [strategy]: Optional search strategy. Defaults to semantic search.
  /// - [includeCompleted]: When true, includes completed/fulfilled/cancelled
  ///   items. Defaults to false.
  ///
  /// Returns a list of search results sorted by composite score (descending).
  /// Each result contains item type, scores, and the actual item data.
  Future<List<SearchResultItem>> search({
    required String query,
    required List<String> communityIds,
    required double latitudeDeg,
    required double longitudeDeg,
    int? maxResults,
    List<SearchItemType>? itemTypes,
    SearchStrategy? strategy,
    bool includeCompleted = false,
  }) async {
    // Create a unique cache key based on all search parameters.
    final sortedIds = List<String>.from(communityIds)..sort();
    final typesKey = itemTypes?.map((t) => t.value).join(',') ?? 'default';
    final strategyKey = strategy?.value.toString() ?? 'default';
    final completedKey = includeCompleted ? 'all' : 'active';
    final cacheKey = 'search:unified:${sortedIds.join(",")}:$query:'
        '${latitudeDeg.toStringAsFixed(4)},'
        '${longitudeDeg.toStringAsFixed(4)}:'
        '${maxResults ?? "default"}:$typesKey:$strategyKey:$completedKey';

    _log.info('SearchRepository.search() called with cacheKey: $cacheKey');

    final results = await _cache.get(
      key: cacheKey,
      fetch: () {
        _log.info('Cache miss - fetching from server');
        return _service.search(
          query: query,
          communityIds: communityIds,
          latitudeDeg: latitudeDeg,
          longitudeDeg: longitudeDeg,
          maxResults: maxResults,
          itemTypes: itemTypes,
          strategy: strategy,
          includeCompleted: includeCompleted,
        );
      },
    );

    _log.info('📦 SearchRepository returning ${results.length} results');
    if (results.isNotEmpty) {
      final first = results.first;
      if (first.hasExperience()) {
        _log.info('   First result: EXPERIENCE "${first.experience.name}", mediaIds=${first.experience.mediaIds}');
      } else if (first.hasGear()) {
        _log.info('   First result: GEAR "${first.gear.name}", mediaIds=${first.gear.mediaIds}');
      }
    }

    return results;
  }

  /// Universal search (#2634): grouped keyword search across everything in
  /// the caller's communities. Cached per query so back-to-back keystroke
  /// re-submissions of the same term don't refetch.
  Future<UniversalSearchResponse> universalSearch({
    required String query,
    int? maxResultsPerGroup,
  }) {
    final cacheKey =
        'search:universal:$query:${maxResultsPerGroup ?? "default"}';
    return _cache.get(
      key: cacheKey,
      fetch: () => _service.universalSearch(
        query: query,
        maxResultsPerGroup: maxResultsPerGroup,
      ),
    );
  }

  /// Invalidates all search caches.
  ///
  /// Call this after gear, requests, or experiences are added, updated, or
  /// removed to ensure the next search gets fresh results.
  Future<void> invalidateSearches() async {
    _log.info('🗑️ invalidateSearches() - clearing all search cache entries');
    await _cache.clear(pattern: 'search:unified:*');
    await _cache.clear(pattern: 'search:universal:*');
    _log.info('✅ Cache cleared, notifying listeners');
    _onCacheInvalidated?.call();
    _log.info('📢 Notification sent');
  }

  /// Invalidates search caches for a specific community.
  ///
  /// Call this when items change in a specific community.
  Future<void> invalidateSearchesForCommunity(String communityId) async {
    await _cache.clear(pattern: 'search:unified:$communityId:*');
    _onCacheInvalidated?.call();
  }

  /// Invalidates all cached search data.
  ///
  /// Alias for [invalidateSearches] to maintain consistency with other repositories.
  Future<void> invalidateAll() async {
    await invalidateSearches();
  }
}
