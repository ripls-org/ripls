import 'package:logging/logging.dart';
import 'package:ripls/core/utils/rpc_utils.dart';

import 'cache_manager.dart';

final _audit = Logger('CacheAudit');

/// Provides caching utilities for repositories.
///
/// Handles common caching patterns like get-with-fetch, list caching,
/// and cache invalidation with namespace support. This service wraps
/// a CacheManager and automatically prefixes all keys with a namespace
/// to prevent conflicts between different data types.
///
/// Example usage:
/// ```dart
/// final cache = CacheService(cacheManager, 'user');
/// final user = await cache.get(
///   key: userId,
///   fetch: () => api.getUser(userId),
/// );
/// ```
class CacheService {
  final CacheManager _cache;
  final String _namespace;

  /// Creates a CacheService with the given cache manager and namespace.
  ///
  /// The namespace is used as a prefix for all cache keys (e.g., 'user', 'gear').
  CacheService(this._cache, this._namespace);

  /// Gets an entity with caching.
  ///
  /// First checks the cache. If the value is not cached or expired,
  /// fetches it using the provided [fetch] function and caches the result.
  ///
  /// Uses the global TTL configured via the environment (CACHE_TTL_MINUTES).
  ///
  /// Parameters:
  /// - [key]: Cache key (will be prefixed with namespace)
  /// - [fetch]: Function to call on cache miss
  Future<T> get<T>({
    required String key,
    required Future<T> Function() fetch,
  }) async {
    final fullKey = '$_namespace:$key';
    if (!RpcUtils.auditLogging) {
      return _cache.get(key: fullKey, fetch: fetch);
    }
    var fetched = false;
    final result = await _cache.get(
      key: fullKey,
      fetch: () async {
        fetched = true;
        return fetch();
      },
    );
    _audit.info(
      'CACHE_AUDIT key=$fullKey result=${fetched ? 'miss' : 'hit'}',
    );
    return result;
  }

  /// Caches a list with a composite key.
  ///
  /// Similar to [get] but specifically designed for list operations.
  /// The listKey should identify the specific list (e.g., 'user:list',
  /// 'gear:available:filter').
  ///
  /// Uses the global TTL configured via the environment (CACHE_TTL_MINUTES).
  ///
  /// Parameters:
  /// - [listKey]: List identifier (will be prefixed with namespace)
  /// - [fetch]: Function to call on cache miss
  Future<List<T>> getList<T>({
    required String listKey,
    required Future<List<T>> Function() fetch,
  }) async {
    return _cache.get(key: '$_namespace:$listKey', fetch: fetch);
  }

  /// Stores a value in cache without fetching.
  ///
  /// Useful for optimistic updates or caching values obtained through
  /// mutations.
  ///
  /// Uses the global TTL configured via the environment (CACHE_TTL_MINUTES).
  ///
  /// Parameters:
  /// - [key]: Cache key (will be prefixed with namespace)
  /// - [value]: The value to cache
  Future<void> put<T>(String key, T value) async {
    await _cache.put('$_namespace:$key', value);
  }

  /// Invalidates a single cache entry.
  ///
  /// Removes the entry from the cache, forcing a fresh fetch on the
  /// next access.
  ///
  /// Parameters:
  /// - [key]: Cache key to remove (will be prefixed with namespace)
  Future<void> invalidate(String key) async {
    await _cache.remove('$_namespace:$key');
  }

  /// Invalidates multiple specific keys.
  ///
  /// Useful for batch invalidation of related entities (e.g., after
  /// creating a loan, invalidate user's loan list and gear availability).
  ///
  /// Parameters:
  /// - [keys]: List of cache keys to remove (will be prefixed with namespace)
  Future<void> invalidateKeys(List<String> keys) async {
    for (final key in keys) {
      await invalidate(key);
    }
  }

  /// Invalidates all entries matching a pattern.
  ///
  /// The pattern supports wildcards (e.g., 'user:*' matches all user keys).
  /// The namespace is automatically added to the pattern.
  ///
  /// Parameters:
  /// - [pattern]: Pattern to match (will be prefixed with namespace)
  Future<void> invalidatePattern(String pattern) async {
    await _cache.clear(pattern: '$_namespace:$pattern');
  }

  /// Invalidates all entries in this namespace.
  ///
  /// Removes all cache entries for this repository, forcing fresh
  /// fetches on all subsequent accesses.
  Future<void> invalidateAll() async {
    await _cache.clear(pattern: '$_namespace:*');
  }

  /// Converts filter parameters to a stable cache key.
  ///
  /// Ensures that identical filters produce identical keys regardless
  /// of the order in which filter parameters are specified.
  ///
  /// Parameters:
  /// - [filters]: Map of filter parameters
  String filtersToKey(Map<String, dynamic>? filters) {
    if (filters == null || filters.isEmpty) return 'all';
    final sorted = filters.entries.toList()
      ..sort((a, b) => a.key.compareTo(b.key));
    return sorted.map((e) => '${e.key}=${e.value}').join('&');
  }
}
