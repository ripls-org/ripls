/// Cache statistics for monitoring and debugging.
class CacheStats {
  /// Total number of entries across all cache layers.
  final int totalEntries;

  /// Number of entries in memory cache.
  final int memoryEntries;

  /// Number of entries in disk cache.
  final int diskEntries;

  /// Total number of cache hits.
  final int hits;

  /// Total number of cache misses.
  final int misses;

  /// Total number of evictions.
  final int evictions;

  /// Total bytes used by cache (0 if not calculated).
  final int bytesUsed;

  const CacheStats({
    required this.totalEntries,
    required this.memoryEntries,
    required this.diskEntries,
    required this.hits,
    required this.misses,
    required this.evictions,
    required this.bytesUsed,
  });
  }

/// Main cache manager interface for all caching operations.
///
/// This provides a unified interface for caching data. All cached entries
/// use a global TTL configured via the environment (CACHE_TTL_MINUTES).
/// Implementations may use memory-only, disk-only, or hybrid approaches.
abstract class CacheManager {
  /// Gets a cached value or fetches it using the provided function.
  ///
  /// If the value exists in cache and hasn't expired, returns the cached value.
  /// Otherwise, calls [fetch] to get a fresh value and stores it in the cache.
  ///
  /// All cached entries use the global TTL from the environment configuration.
  ///
  /// Parameters:
  /// - [key]: Unique identifier for this cache entry
  /// - [fetch]: Function to call if cache miss occurs
  Future<T> get<T>({
    required String key,
    required Future<T> Function() fetch,
  });

  /// Puts a value into the cache.
  ///
  /// The entry will use the global TTL from the environment configuration.
  ///
  /// Parameters:
  /// - [key]: Unique identifier for this cache entry
  /// - [value]: The value to cache
  Future<void> put<T>(String key, T value);

  /// Removes a specific entry from the cache.
  ///
  /// Parameters:
  /// - [key]: The cache key to remove
  Future<void> remove(String key);

  /// Clears all cache entries, or entries matching a pattern.
  ///
  /// If [pattern] is provided, only keys matching the pattern are cleared.
  /// Pattern supports wildcards (e.g., "user:*" clears all user keys).
  ///
  /// Parameters:
  /// - [pattern]: Optional glob pattern to match keys (null = clear all)
  Future<void> clear({String? pattern});

  /// Gets cache statistics for monitoring.
  CacheStats getStats();
}
