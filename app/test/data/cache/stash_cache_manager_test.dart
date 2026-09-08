import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';

void main() {
  group('StashCacheManager', () {
    late StashCacheManager cacheManager;

    setUp(() async {
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
    });

    tearDown(() async {
      await cacheManager.dispose();
    });

    group('get()', () {
      test('caches and returns value on first call', () async {
        int fetchCount = 0;
        Future<String> fetch() async {
          fetchCount++;
          return 'test-value';
        }

        final result = await cacheManager.get(key: 'test-key', fetch: fetch);

        expect(result, 'test-value');
        expect(fetchCount, 1);
      });

      test('returns cached value on second call', () async {
        int fetchCount = 0;
        Future<String> fetch() async {
          fetchCount++;
          return 'test-value';
        }

        // First call - should fetch
        await cacheManager.get(key: 'test-key', fetch: fetch);

        // Second call - should use cache
        final result = await cacheManager.get(key: 'test-key', fetch: fetch);

        expect(result, 'test-value');
        expect(fetchCount, 1); // Should only fetch once
      });

      test('works with different value types', () async {
        // String
        final str = await cacheManager.get(
          key: 'string-key',
          fetch: () async => 'string-value',
        );
        expect(str, 'string-value');

        // int
        final num = await cacheManager.get(
          key: 'int-key',
          fetch: () async => 42,
        );
        expect(num, 42);

        // List
        final list = await cacheManager.get(
          key: 'list-key',
          fetch: () async => ['a', 'b', 'c'],
        );
        expect(list, ['a', 'b', 'c']);

        // Map
        final map = await cacheManager.get(
          key: 'map-key',
          fetch: () async => {'foo': 'bar'},
        );
        expect(map, {'foo': 'bar'});
      });

      test('initializes automatically on first call', () async {
        // Create new instance without manual initialization
        final manager = StashCacheManager();

        final result = await manager.get(
          key: 'test',
          fetch: () async => 'value',
        );

        expect(result, 'value');
        await manager.dispose();
      });
    });

    group('put()', () {
      test('stores value in cache', () async {
        await cacheManager.put('test-key', 'test-value');

        // Verify by getting without fetch
        int fetchCount = 0;
        final result = await cacheManager.get(
          key: 'test-key',
          fetch: () async {
            fetchCount++;
            return 'new-value';
          },
        );

        expect(result, 'test-value');
        expect(fetchCount, 0); // Shouldn't fetch
      });
    });

    group('remove()', () {
      test('deletes cached entry', () async {
        await cacheManager.put('test-key', 'test-value');

        await cacheManager.remove('test-key');

        // Verify it's gone
        int fetchCount = 0;
        await cacheManager.get(
          key: 'test-key',
          fetch: () async {
            fetchCount++;
            return 'new-value';
          },
        );

        expect(fetchCount, 1); // Had to fetch (cache miss)
      });
    });

    group('clear()', () {
      test('removes all entries when no pattern', () async {
        await cacheManager.put('key1', 'value1');
        await cacheManager.put('key2', 'value2');

        await cacheManager.clear();

        // Verify both are gone
        int fetch1Count = 0;
        await cacheManager.get(
          key: 'key1',
          fetch: () async {
            fetch1Count++;
            return 'new';
          },
        );

        int fetch2Count = 0;
        await cacheManager.get(
          key: 'key2',
          fetch: () async {
            fetch2Count++;
            return 'new';
          },
        );

        expect(fetch1Count, 1);
        expect(fetch2Count, 1);
      });

      test('removes only matching entries with pattern', () async {
        await cacheManager.put('user:1', 'user1');
        await cacheManager.put('user:2', 'user2');
        await cacheManager.put('gear:1', 'gear1');

        await cacheManager.clear(pattern: 'user:*');

        // User entries should be gone
        int userFetchCount = 0;
        await cacheManager.get(
          key: 'user:1',
          fetch: () async {
            userFetchCount++;
            return 'new';
          },
        );
        expect(userFetchCount, 1); // Had to fetch

        // Gear entry should still exist
        int gearFetchCount = 0;
        final gear = await cacheManager.get(
          key: 'gear:1',
          fetch: () async {
            gearFetchCount++;
            return 'new';
          },
        );
        expect(gear, 'gear1'); // From cache
        expect(gearFetchCount, 0); // Didn't fetch
      });

      test('handles complex patterns', () async {
        await cacheManager.put('loan:gear:123:received', 'value1');
        await cacheManager.put('loan:gear:456:received', 'value2');
        await cacheManager.put('loan:extended:list', 'value3');

        await cacheManager.clear(pattern: 'loan:gear:*');

        // First two should be gone
        int fetch1Count = 0;
        await cacheManager.get(
          key: 'loan:gear:123:received',
          fetch: () async {
            fetch1Count++;
            return 'new';
          },
        );
        expect(fetch1Count, 1);

        // Last one should still exist
        int fetch3Count = 0;
        final value = await cacheManager.get(
          key: 'loan:extended:list',
          fetch: () async {
            fetch3Count++;
            return 'new';
          },
        );
        expect(value, 'value3');
        expect(fetch3Count, 0);
      });

      test('clears bare prefix key when pattern uses colon-star', () async {
        // Reproduces issue #1064: 'request:my' not matched by 'request:my:*'
        await cacheManager.put('request:my', 'bare');
        await cacheManager.put('request:my:communityA', 'scoped');
        await cacheManager.put('request:other', 'unrelated');

        await cacheManager.clear(pattern: 'request:my:*');

        // Both request:my keys should be gone
        int bareFetchCount = 0;
        await cacheManager.get(
          key: 'request:my',
          fetch: () async {
            bareFetchCount++;
            return 'new';
          },
        );
        expect(bareFetchCount, 1, reason: 'bare key should have been cleared');

        int scopedFetchCount = 0;
        await cacheManager.get(
          key: 'request:my:communityA',
          fetch: () async {
            scopedFetchCount++;
            return 'new';
          },
        );
        expect(scopedFetchCount, 1, reason: 'scoped key should have been cleared');

        // Unrelated key should survive
        int otherFetchCount = 0;
        final other = await cacheManager.get(
          key: 'request:other',
          fetch: () async {
            otherFetchCount++;
            return 'new';
          },
        );
        expect(other, 'unrelated');
        expect(otherFetchCount, 0, reason: 'unrelated key should be untouched');
      });

      test('clears bare prefix for transfer patterns', () async {
        // Verifies the fix generalizes to transfer_repository bare keys
        await cacheManager.put('transfer:my', 'bare');
        await cacheManager.put('transfer:my:LOAN:ACTIVE', 'filtered');
        await cacheManager.put('transfer:received', 'other-bare');

        await cacheManager.clear(pattern: 'transfer:my:*');

        int bareFetch = 0;
        await cacheManager.get(
          key: 'transfer:my',
          fetch: () async {
            bareFetch++;
            return 'new';
          },
        );
        expect(bareFetch, 1, reason: 'bare transfer:my should be cleared');

        int filteredFetch = 0;
        await cacheManager.get(
          key: 'transfer:my:LOAN:ACTIVE',
          fetch: () async {
            filteredFetch++;
            return 'new';
          },
        );
        expect(filteredFetch, 1, reason: 'filtered key should be cleared');

        // received should survive
        int receivedFetch = 0;
        final received = await cacheManager.get(
          key: 'transfer:received',
          fetch: () async {
            receivedFetch++;
            return 'new';
          },
        );
        expect(received, 'other-bare');
        expect(receivedFetch, 0, reason: 'received key should be untouched');
      });
    });

    group('getStats()', () {
      test('returns stats with hit/miss counts', () async {
        // Initial state
        var stats = cacheManager.getStats();
        expect(stats.hits, 0);
        expect(stats.misses, 0);

        // Cache miss
        await cacheManager.get(key: 'test', fetch: () async => 'value');

        stats = cacheManager.getStats();
        expect(stats.misses, 1);

        // Cache hit
        await cacheManager.get(key: 'test', fetch: () async => 'value');

        stats = cacheManager.getStats();
        expect(stats.hits, 1);
        expect(stats.misses, 1);
      });

      test('entry counts are 0 (Stash size is async)', () {
        final stats = cacheManager.getStats();
        expect(stats.totalEntries, 0);
        expect(stats.memoryEntries, 0);
        expect(stats.diskEntries, 0);
      });
    });

    group('TTL behavior', () {
      test('uses global TTL from environment', () async {
        // Note: StashCacheManager uses a global 30-minute AccessedExpiryPolicy
        // configured via environment variable CACHE_TTL_MINUTES
        // This test documents that entries use the global TTL

        await cacheManager.put('test-key', 'test-value');

        // Immediately should be cached
        int fetchCount = 0;
        final result1 = await cacheManager.get(
          key: 'test-key',
          fetch: () async {
            fetchCount++;
            return 'new-value';
          },
        );
        expect(result1, 'test-value');
        expect(fetchCount, 0);

        // Note: Entries use the global TTL configured in StashCacheManager
        // Default is 30 minutes via AccessedExpiryPolicy
      });
    });

    group('initialization', () {
      test('can be called multiple times safely', () async {
        await cacheManager.initialize();
        await cacheManager.initialize(); // Should be no-op
        await cacheManager.initialize(); // Should be no-op

        // Should still work
        final result = await cacheManager.get(
          key: 'test',
          fetch: () async => 'value',
        );
        expect(result, 'value');
      });

      test('handles concurrent initialization safely', () async {
        // Create a fresh cache manager that hasn't been initialized yet
        final freshCache = StashCacheManager();

        // Simulate concurrent initialization from multiple parallel operations
        // This is what happens in the inbox when Future.wait runs parallel loads
        await Future.wait([
          freshCache.initialize(),
          freshCache.initialize(),
          freshCache.initialize(),
        ]);

        // Verify that initialization only happened once, despite 3 concurrent calls
        // If the race condition exists, this count could be 2 or 3
        expect(
          freshCache.initializationCount,
          1,
          reason:
              'Cache should only be initialized once despite concurrent calls',
        );

        // Now run concurrent get operations (which also call _ensureInitialized)
        final results = await Future.wait([
          freshCache.get(key: 'key1', fetch: () async => 'value1'),
          freshCache.get(key: 'key2', fetch: () async => 'value2'),
          freshCache.get(key: 'key3', fetch: () async => 'value3'),
        ]);

        // Verify the get operations returned correct values
        expect(results[0], 'value1');
        expect(results[1], 'value2');
        expect(results[2], 'value3');

        // Verify initialization count is still 1 (get operations shouldn't re-initialize)
        expect(freshCache.initializationCount, 1);

        await freshCache.dispose();
      });

      test('stress test: highly concurrent initialization attempts', () async {
        // This test attempts to expose race conditions by:
        // 1. Creating many concurrent initialization calls
        // 2. Testing that all operations complete successfully
        // 3. Verifying initialization only happened once

        final freshCache = StashCacheManager();

        // Launch 20 concurrent initialization attempts
        final initFutures = List.generate(20, (_) => freshCache.initialize());

        await Future.wait(initFutures);

        // Verify initialization only happened once despite 20 concurrent calls
        expect(
          freshCache.initializationCount,
          1,
          reason:
              'Cache should only be initialized once despite many concurrent calls',
        );

        // Now verify that the cache is usable and consistent
        final testFutures = List.generate(
          20,
          (i) => freshCache.get(key: 'key$i', fetch: () async => 'value$i'),
        );

        final results = await Future.wait(testFutures);

        // Verify all results are correct
        for (var i = 0; i < 20; i++) {
          expect(results[i], 'value$i');
        }

        // Verify initialization count hasn't changed
        expect(freshCache.initializationCount, 1);

        await freshCache.dispose();
      });
    });

    group('single-flight (#1808)', () {
      test('coalesces parallel misses for the same key into one fetch',
          () async {
        var fetchCount = 0;
        Future<String> fetch() async {
          fetchCount++;
          // Hold the fetch open long enough for concurrent callers to
          // hit the in-flight branch.
          await Future<void>.delayed(const Duration(milliseconds: 20));
          return 'value';
        }

        final results = await Future.wait(
          List.generate(5, (_) => cacheManager.get(key: 'k', fetch: fetch)),
        );

        expect(fetchCount, 1, reason: 'fetch must run exactly once');
        expect(results, List.filled(5, 'value'));
        expect(cacheManager.coalesced, 4,
            reason: '4 of 5 callers should have coalesced onto the first');
      });

      test('failing in-flight fetch rejects all coalesced callers and clears state',
          () async {
        var attempts = 0;
        Future<String> failingFetch() async {
          attempts++;
          await Future<void>.delayed(const Duration(milliseconds: 10));
          throw StateError('boom');
        }

        final futures = List.generate(
          3,
          (_) => cacheManager.get(key: 'fail-key', fetch: failingFetch),
        );

        for (final f in futures) {
          await expectLater(f, throwsA(isA<StateError>()));
        }
        expect(attempts, 1, reason: 'only the first caller should fetch');

        // After the failure clears the in-flight slot, a subsequent get()
        // should be free to retry from scratch.
        var retryAttempts = 0;
        final retry = await cacheManager.get(
          key: 'fail-key',
          fetch: () async {
            retryAttempts++;
            return 'recovered';
          },
        );
        expect(retry, 'recovered');
        expect(retryAttempts, 1);
      });

      test('does not coalesce different keys', () async {
        var aFetched = 0;
        var bFetched = 0;
        await Future.wait<dynamic>([
          cacheManager.get(
            key: 'a',
            fetch: () async {
              aFetched++;
              return 'a-val';
            },
          ),
          cacheManager.get(
            key: 'b',
            fetch: () async {
              bFetched++;
              return 'b-val';
            },
          ),
        ]);
        expect(aFetched, 1);
        expect(bFetched, 1);
      });
    });

    // Regression: invalidation must evict the in-flight fetch, not just the
    // stored value. Otherwise an `invalidate then re-fetch` that races a fetch
    // already in flight (e.g. the experience RSVP refresh racing the initial
    // load — both keyed by the bare experience id) coalesces onto the STALE
    // in-flight result and the caller sees pre-mutation data. Before the fix
    // these tests fail: the post-invalidation caller coalesces onto the stale
    // fetch ('stale') instead of issuing a fresh one ('fresh').
    group('invalidation evicts in-flight fetches', () {
      test('remove() during an in-flight fetch forces the next get to refetch',
          () async {
        final firstStarted = Completer<void>();
        final releaseFirst = Completer<void>();
        var staleFetches = 0;
        var freshFetches = 0;

        // A slow fetch held open until we release it — stands in for the
        // initial load's GetExperience that started before the mutation.
        final first = cacheManager.get<String>(
          key: 'k',
          fetch: () async {
            staleFetches++;
            firstStarted.complete();
            await releaseFirst.future;
            return 'stale';
          },
        );
        await firstStarted.future; // ensure it's genuinely in flight

        // Invalidate while the fetch is still running.
        await cacheManager.remove('k');

        // A new caller must issue its OWN fetch, not join the stale one.
        final secondFuture = cacheManager.get<String>(
          key: 'k',
          fetch: () async {
            freshFetches++;
            return 'fresh';
          },
        );

        // Let the stale fetch finish; it must not poison the cache.
        releaseFirst.complete();
        await first;

        final second = await secondFuture;
        expect(second, 'fresh',
            reason: 'post-invalidation get must not coalesce onto the stale '
                'in-flight fetch');
        expect(freshFetches, 1, reason: 'the new caller must fetch fresh');
        expect(staleFetches, 1);

        // The stale fetch completing after the invalidation must not have
        // written its value back into the cache.
        var thirdFetches = 0;
        final third = await cacheManager.get<String>(
          key: 'k',
          fetch: () async {
            thirdFetches++;
            return 'unused';
          },
        );
        expect(third, 'fresh',
            reason: 'the stale in-flight result must not overwrite the cache');
        expect(thirdFetches, 0, reason: 'value should be served from cache');
      });

      test('clear(pattern) during an in-flight fetch forces a refetch',
          () async {
        final firstStarted = Completer<void>();
        final releaseFirst = Completer<void>();

        final first = cacheManager.get<String>(
          key: 'experience:e1:community:c1',
          fetch: () async {
            firstStarted.complete();
            await releaseFirst.future;
            return 'stale';
          },
        );
        await firstStarted.future;

        await cacheManager.clear(pattern: 'experience:e1:*');

        var freshFetches = 0;
        final secondFuture = cacheManager.get<String>(
          key: 'experience:e1:community:c1',
          fetch: () async {
            freshFetches++;
            return 'fresh';
          },
        );

        releaseFirst.complete();
        await first;

        expect(await secondFuture, 'fresh');
        expect(freshFetches, 1);
      });
    });
  });
}
