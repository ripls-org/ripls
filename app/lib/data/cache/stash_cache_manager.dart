import 'dart:async';

import 'package:stash/stash_api.dart' as stash;
import 'package:stash_memory/stash_memory.dart';

import '../../core/config/environment.dart';
import 'cache_manager.dart' as app;

/// Cache manager implementation using Stash with memory storage.
///
/// Uses a global TTL configured via Environment.cacheTtl (CACHE_TTL_MINUTES).
/// All cached entries share the same TTL, refreshed on access.
///
/// Thread-safe initialization: Multiple concurrent calls to initialize() will
/// complete safely, with only one actually performing the initialization.
class StashCacheManager implements app.CacheManager {
  stash.Cache<dynamic>? _cache;
  int _hits = 0;
  int _misses = 0;
  // Number of get() calls that piggy-backed on an in-flight fetch
  // started by a prior caller — i.e. stampedes that were absorbed.
  // Read by tests; not yet plumbed into CacheStats.
  int _coalesced = 0;
  int get coalesced => _coalesced;
  Future<void>? _initializationFuture;
  // Per-key in-flight fetches. When a miss is in progress, subsequent
  // callers for the same key await the same Future instead of starting
  // a parallel fetch (#1808).
  final Map<String, Future<dynamic>> _inflight = {};
  // Per-key invalidation epoch. Bumped by remove()/clear() so that (a) a fetch
  // already in flight when an invalidation lands does NOT write its now-stale
  // result back into the cache, and (b) the in-flight marker is dropped so a
  // post-invalidation fetch issues a fresh request instead of coalescing onto
  // the stale one. Without this, `invalidate then re-fetch` (e.g. the RSVP
  // refresh racing the initial experience load) returns pre-mutation data.
  final Map<String, int> _epoch = {};
  final Duration _cacheTtl;

  // Test-only counter for tracking initialization calls
  int _initializationCount = 0;

  StashCacheManager({Duration? cacheTtl}) : _cacheTtl = cacheTtl ?? Environment.cacheTtl;

  /// Initializes the cache store asynchronously.
  /// Must be called before using any cache methods.
  ///
  /// This method is idempotent and safe to call multiple times concurrently.
  /// All concurrent callers will await the same initialization Future.
  Future<void> initialize() async {
    // If already initialized, return immediately
    if (_cache != null) return;

    // If initialization is in progress, await it
    if (_initializationFuture != null) {
      await _initializationFuture;
      return;
    }

    // Start initialization and store the future
    _initializationFuture = _performInitialization();
    await _initializationFuture;
  }

  /// Performs the actual cache initialization.
  /// Only called once by the first caller to initialize().
  Future<void> _performInitialization() async {
    _initializationCount++;
    final store = await newMemoryCacheStore();
    _cache = await store.cache<dynamic>(
      name: 'app_cache',
      maxEntries: 1000,
      evictionPolicy: const stash.LruEvictionPolicy(),
      // Use AccessedExpiryPolicy to refresh TTL on access
      // Use environment-configured TTL
      expiryPolicy: stash.AccessedExpiryPolicy(_cacheTtl),
    );
  }

  Future<void> _ensureInitialized() async {
    if (_cache == null) {
      await initialize();
    }
  }

  @override
  Future<T> get<T>({
    required String key,
    required Future<T> Function() fetch,
  }) async {
    // If TTL is zero, skip caching entirely
    if (_cacheTtl == Duration.zero) {
      return fetch();
    }

    await _ensureInitialized();

    // Try to get from cache
    final cached = await _cache!.get(key);
    if (cached != null) {
      _hits++;
      return cached as T;
    }

    // Stampede dedup: if another caller is already fetching this key,
    // await their Future instead of issuing a parallel fetch.
    final existing = _inflight[key];
    if (existing != null) {
      _coalesced++;
      return await existing as T;
    }

    // Cache miss - fetch, store, and remember the in-flight Future
    // so concurrent callers can join it. Capture the key's epoch at start: if
    // an invalidation bumps it before the fetch resolves, the result is stale
    // and must not be written or coalesced onto.
    _misses++;
    final startEpoch = _epoch[key] ?? 0;
    final pending = _fetchAndStore<T>(key, fetch, startEpoch);
    _inflight[key] = pending;
    return pending;
  }

  Future<T> _fetchAndStore<T>(
    String key,
    Future<T> Function() fetch,
    int startEpoch,
  ) async {
    try {
      final value = await fetch();
      // Skip the write if an invalidation landed while we were fetching — our
      // result is now stale and a newer fetch owns the key.
      if ((_epoch[key] ?? 0) == startEpoch) {
        await _cache!.put(key, value);
      }
      return value;
    } finally {
      // Only clear the in-flight marker if no invalidation replaced it. An
      // invalidation bumps the epoch and drops our entry; a subsequent fetch
      // (different epoch) may own the slot now — don't evict that one. (Using
      // the epoch rather than the Future's identity also avoids referencing a
      // not-yet-assigned local when fetch() throws synchronously.)
      if ((_epoch[key] ?? 0) == startEpoch) {
        unawaited(_inflight.remove(key));
      }
    }
  }

  @override
  Future<void> put<T>(String key, T value) async {
    // If TTL is zero, skip caching entirely
    if (_cacheTtl == Duration.zero) {
      return;
    }

    await _ensureInitialized();
    await _cache!.put(key, value);
  }

  @override
  Future<void> remove(String key) async {
    await _ensureInitialized();
    _invalidateInflight(key);
    await _cache!.remove(key);
  }

  @override
  Future<void> clear({String? pattern}) async {
    await _ensureInitialized();

    if (pattern == null) {
      // Clear all — invalidate every in-flight fetch too.
      for (final key in _inflight.keys.toList()) {
        _invalidateInflight(key);
      }
      await _cache!.clear();
      return;
    }

    // Convert pattern to regex so that ':*' at the end is optional, e.g.:
    //   'my:*'  -> '^my(:.*)?$'  (matches 'my', 'my:communityId', etc.)
    //   '*'     -> '^.*$'        (matches everything)
    final regexPattern = pattern.replaceAll(':*', '(:.*)?').replaceAll('*', '.*');
    final regex = RegExp('^$regexPattern\$');

    // Match both stored keys and in-flight keys so a pattern invalidation also
    // evicts a fetch that's mid-flight under a matching key.
    final cacheKeys = await _cache!.keys;
    final matchingKeys = <String>{...cacheKeys, ..._inflight.keys}
        .where((key) => regex.hasMatch(key))
        .toList();

    for (final key in matchingKeys) {
      _invalidateInflight(key);
      await _cache!.remove(key);
    }
  }

  /// Bumps a key's epoch and drops its in-flight marker so a fetch already in
  /// flight won't write a now-stale result and subsequent callers issue a fresh
  /// fetch instead of coalescing onto the stale one.
  void _invalidateInflight(String key) {
    _epoch[key] = (_epoch[key] ?? 0) + 1;
    _inflight.remove(key);
  }

  @override
  app.CacheStats getStats() {
    // Note: Stash's cache.size is async, but CacheStats needs sync values
    // We return 0 for entry counts since they're not critical for functionality
    return app.CacheStats(
      totalEntries: 0, // Stash size is async, not sync-accessible
      memoryEntries: 0,
      diskEntries: 0,
      hits: _hits,
      misses: _misses,
      evictions: 0, // Stash doesn't expose eviction count
      bytesUsed: 0, // Stash doesn't calculate byte size
    );
  }

  /// Returns the number of times initialization has been performed.
  /// For testing purposes only.
  int get initializationCount => _initializationCount;

  /// Disposes cache resources.
  Future<void> dispose() async {
    if (_cache != null) {
      await _cache!.clear();
    }
  }
}
