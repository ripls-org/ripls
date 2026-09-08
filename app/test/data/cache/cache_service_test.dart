import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';

void main() {
  group('CacheService', () {
    late CacheManager cacheManager;
    late CacheService cacheService;

    setUp(() async {
      cacheManager = StashCacheManager();
      await (cacheManager as StashCacheManager).initialize();
      cacheService = CacheService(cacheManager, 'test');
    });

    group('get()', () {
      test('fetches and caches value on first call', () async {
        int fetchCount = 0;
        Future<String> fetch() async {
          fetchCount++;
          return 'test-value';
        }

        final result = await cacheService.get(
          key: 'item1',
          fetch: fetch,
        );

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
        await cacheService.get(key: 'item1', fetch: fetch);

        // Second call - should use cache
        final result = await cacheService.get(key: 'item1', fetch: fetch);

        expect(result, 'test-value');
        expect(fetchCount, 1); // Should only fetch once
      });

      test('uses namespace prefix in cache key', () async {
        int fetchCount = 0;
        await cacheService.get(
          key: 'item1',
          fetch: () async {
            fetchCount++;
            return 'value1';
          },
        );

        // Verify it's cached by fetching again - should not call fetch
        final result = await cacheService.get(
          key: 'item1',
          fetch: () async {
            fetchCount++;
            return 'value1';
          },
        );

        expect(result, 'value1');
        expect(fetchCount, 1); // Should only fetch once
      });

      test('caches values using global TTL', () async {
        int fetchCount = 0;
        await cacheService.get(
          key: 'item1',
          fetch: () async {
            fetchCount++;
            return 'value1';
          },
        );

        expect(fetchCount, 1);
      });
    });

    group('getList()', () {
      test('fetches and caches list on first call', () async {
        int fetchCount = 0;
        Future<List<String>> fetch() async {
          fetchCount++;
          return ['item1', 'item2', 'item3'];
        }

        final result = await cacheService.getList(
          listKey: 'user:list',
          fetch: fetch,
        );

        expect(result, ['item1', 'item2', 'item3']);
        expect(fetchCount, 1);
      });

      test('returns cached list on second call', () async {
        int fetchCount = 0;
        Future<List<String>> fetch() async {
          fetchCount++;
          return ['item1', 'item2'];
        }

        // First call - should fetch
        await cacheService.getList(listKey: 'user:list', fetch: fetch);

        // Second call - should use cache
        final result =
            await cacheService.getList(listKey: 'user:list', fetch: fetch);

        expect(result, ['item1', 'item2']);
        expect(fetchCount, 1);
      });

      test('uses namespace prefix with list key', () async {
        await cacheService.getList(
          listKey: 'user:list',
          fetch: () async => ['a', 'b'],
        );

        // Verify list is cached by fetching again without calling service
        int fetchCount = 0;
        final cached = await cacheService.getList(
          listKey: 'user:list',
          fetch: () async {
            fetchCount++;
            return ['a', 'b'];
          },
        );
        expect(cached, ['a', 'b']);
        expect(fetchCount, 0); // Should not fetch, using cache
      });
    });

    group('put()', () {
      test('stores value in cache', () async {
        int fetchCount = 0;
        await cacheService.put('item1', 'stored-value');

        // Verify it was cached by trying to get it (should not fetch)
        final result = await cacheService.get(
          key: 'item1',
          fetch: () async {
            fetchCount++;
            return 'fetched-value';
          },
        );

        expect(result, 'stored-value');
        expect(fetchCount, 0); // Should not have fetched
      });

      test('uses namespace prefix', () async {
        int fetchCount = 0;
        await cacheService.put('item1', 'value1');

        // Verify namespace isolation by checking another namespace doesn't have it
        final otherService = CacheService(cacheManager, 'other');
        await otherService.get(
          key: 'item1',
          fetch: () async {
            fetchCount++;
            return 'other-value';
          },
        );

        expect(fetchCount, 1); // Other namespace should fetch (not in cache)
      });

      test('stores value with global TTL', () async {
        int fetchCount = 0;
        await cacheService.put(
          'item1',
          'value1',
        );

        final result = await cacheService.get(
          key: 'item1',
          fetch: () async {
            fetchCount++;
            return 'fetched';
          },
        );

        expect(result, 'value1');
        expect(fetchCount, 0); // Should not fetch, using cached value
      });
    });

    group('invalidate()', () {
      test('removes single cache entry', () async {
        int fetch1Count = 0;
        int fetch2Count = 0;

        await cacheService.put('item1', 'value1');
        await cacheService.put('item2', 'value2');

        await cacheService.invalidate('item1');

        // item1 should be invalidated (will fetch)
        await cacheService.get(
          key: 'item1',
          fetch: () async {
            fetch1Count++;
            return 'new1';
          },
        );

        // item2 should still be cached (won't fetch)
        await cacheService.get(
          key: 'item2',
          fetch: () async {
            fetch2Count++;
            return 'new2';
          },
        );

        expect(fetch1Count, 1); // item1 was invalidated, should fetch
        expect(fetch2Count, 0); // item2 still cached, should not fetch
      });

      test('uses namespace prefix', () async {
        int fetchCount = 0;
        await cacheService.put('item1', 'value1');

        await cacheService.invalidate('item1');

        // Should fetch since it was invalidated
        await cacheService.get(
          key: 'item1',
          fetch: () async {
            fetchCount++;
            return 'fetched';
          },
        );

        expect(fetchCount, 1); // Should have fetched after invalidation
      });
    });

    group('invalidateKeys()', () {
      test('removes multiple cache entries', () async {
        int fetch1Count = 0;
        int fetch2Count = 0;
        int fetch3Count = 0;

        await cacheService.put('item1', 'value1');
        await cacheService.put('item2', 'value2');
        await cacheService.put('item3', 'value3');

        await cacheService.invalidateKeys(['item1', 'item3']);

        // item1 and item3 should fetch (invalidated)
        await cacheService.get(key: 'item1', fetch: () async { fetch1Count++; return 'new1'; });
        await cacheService.get(key: 'item3', fetch: () async { fetch3Count++; return 'new3'; });
        // item2 should not fetch (still cached)
        await cacheService.get(key: 'item2', fetch: () async { fetch2Count++; return 'new2'; });

        expect(fetch1Count, 1); // invalidated
        expect(fetch2Count, 0); // still cached
        expect(fetch3Count, 1); // invalidated
      });

      test('handles empty list', () async {
        int fetchCount = 0;
        await cacheService.put('item1', 'value1');

        await cacheService.invalidateKeys([]);

        // Should still be cached
        await cacheService.get(
          key: 'item1',
          fetch: () async {
            fetchCount++;
            return 'new';
          },
        );

        expect(fetchCount, 0); // Should not fetch
      });
    });

    group('invalidatePattern()', () {
      test('removes entries matching pattern', () async {
        int user1Count = 0;
        int user2Count = 0;
        int gear1Count = 0;

        await cacheService.put('user:1', 'user1');
        await cacheService.put('user:2', 'user2');
        await cacheService.put('gear:1', 'gear1');

        await cacheService.invalidatePattern('user:*');

        // user:1 and user:2 should fetch (invalidated)
        await cacheService.get(key: 'user:1', fetch: () async { user1Count++; return 'new1'; });
        await cacheService.get(key: 'user:2', fetch: () async { user2Count++; return 'new2'; });
        // gear:1 should not fetch (still cached)
        await cacheService.get(key: 'gear:1', fetch: () async { gear1Count++; return 'newgear'; });

        expect(user1Count, 1); // invalidated
        expect(user2Count, 1); // invalidated
        expect(gear1Count, 0); // still cached
      });

      test('uses namespace prefix in pattern', () async {
        int item1Count = 0;
        int item2Count = 0;
        int otherCount = 0;

        // Put values through service which applies namespace
        await cacheService.put('item:1', 'value1');
        await cacheService.put('item:2', 'value2');
        await cacheService.put('other', 'value3');

        await cacheService.invalidatePattern('item:*');

        // item:1 and item:2 should fetch (invalidated)
        await cacheService.get(key: 'item:1', fetch: () async { item1Count++; return 'new1'; });
        await cacheService.get(key: 'item:2', fetch: () async { item2Count++; return 'new2'; });
        // other should not fetch (still cached)
        await cacheService.get(key: 'other', fetch: () async { otherCount++; return 'newother'; });

        expect(item1Count, 1); // invalidated
        expect(item2Count, 1); // invalidated
        expect(otherCount, 0); // still cached
      });
    });

    group('invalidateAll()', () {
      test('removes all entries in namespace', () async {
        int item1Count = 0;
        int item2Count = 0;
        int listCount = 0;

        await cacheService.put('item1', 'value1');
        await cacheService.put('item2', 'value2');
        await cacheService.put('list:user', 'list-value');

        await cacheService.invalidateAll();

        // All should fetch (all invalidated)
        await cacheService.get(key: 'item1', fetch: () async { item1Count++; return 'new1'; });
        await cacheService.get(key: 'item2', fetch: () async { item2Count++; return 'new2'; });
        await cacheService.get(key: 'list:user', fetch: () async { listCount++; return 'newlist'; });

        expect(item1Count, 1); // invalidated
        expect(item2Count, 1); // invalidated
        expect(listCount, 1); // invalidated
      });

      test('only removes entries in same namespace', () async {
        int testCount = 0;
        int otherCount = 0;

        final otherService = CacheService(cacheManager, 'other');

        await cacheService.put('item1', 'value1');
        await otherService.put('item1', 'other-value');

        await cacheService.invalidateAll();

        // test namespace should fetch (invalidated)
        await cacheService.get(key: 'item1', fetch: () async { testCount++; return 'new1'; });
        // other namespace should not fetch (still cached)
        await otherService.get(key: 'item1', fetch: () async { otherCount++; return 'newother'; });

        expect(testCount, 1); // invalidated
        expect(otherCount, 0); // still cached
      });
    });

    group('filtersToKey()', () {
      test('returns "all" for null filters', () {
        expect(cacheService.filtersToKey(null), 'all');
      });

      test('returns "all" for empty filters', () {
        expect(cacheService.filtersToKey({}), 'all');
      });

      test('converts single filter to key', () {
        final key = cacheService.filtersToKey({'status': 'active'});
        expect(key, 'status=active');
      });

      test('converts multiple filters to sorted key', () {
        final key = cacheService.filtersToKey({
          'category': 'tools',
          'status': 'active',
          'available': 'true',
        });
        expect(key, 'available=true&category=tools&status=active');
      });

      test('produces same key regardless of insertion order', () {
        final key1 = cacheService.filtersToKey({
          'z': '1',
          'a': '2',
          'n': '3',
        });
        final key2 = cacheService.filtersToKey({
          'a': '2',
          'n': '3',
          'z': '1',
        });
        expect(key1, key2);
      });
    });

    group('namespace isolation', () {
      test('different namespaces do not conflict', () async {
        int userCount = 0;
        int gearCount = 0;

        final userCache = CacheService(cacheManager, 'user');
        final gearCache = CacheService(cacheManager, 'gear');

        await userCache.put('123', 'user-data');
        await gearCache.put('123', 'gear-data');

        // Verify by fetching - should not call fetch (using cache)
        final userData = await userCache.get(key: '123', fetch: () async { userCount++; return 'new'; });
        final gearData = await gearCache.get(key: '123', fetch: () async { gearCount++; return 'new'; });

        expect(userData, 'user-data');
        expect(gearData, 'gear-data');
        expect(userCount, 0); // Should not fetch
        expect(gearCount, 0); // Should not fetch
      });

      test('invalidation respects namespace boundaries', () async {
        int userCount = 0;
        int gearCount = 0;

        final userCache = CacheService(cacheManager, 'user');
        final gearCache = CacheService(cacheManager, 'gear');

        await userCache.put('item', 'user-item');
        await gearCache.put('item', 'gear-item');

        await userCache.invalidate('item');

        // user namespace should fetch (invalidated)
        await userCache.get(key: 'item', fetch: () async { userCount++; return 'new-user'; });
        // gear namespace should not fetch (still cached)
        await gearCache.get(key: 'item', fetch: () async { gearCount++; return 'new-gear'; });

        expect(userCount, 1); // invalidated
        expect(gearCount, 0); // still cached
      });
    });

    group('audit logging', () {
      late List<LogRecord> records;
      late StreamSubscription<LogRecord> sub;

      setUp(() {
        records = [];
        Logger.root.level = Level.ALL;
        sub = Logger.root.onRecord.listen(records.add);
      });

      tearDown(() {
        sub.cancel();
        RpcUtils.auditLogging = false;
      });

      test('emits no CACHE_AUDIT line when auditLogging is off', () async {
        RpcUtils.auditLogging = false;

        await cacheService.get(key: 'item', fetch: () async => 'v');
        await cacheService.get(key: 'item', fetch: () async => 'v');

        expect(
          records.any((r) => r.message.startsWith('CACHE_AUDIT')),
          isFalse,
        );
      });

      test('emits CACHE_AUDIT miss then hit when auditLogging is on',
          () async {
        RpcUtils.auditLogging = true;

        await cacheService.get(key: 'item', fetch: () async => 'v');
        await cacheService.get(key: 'item', fetch: () async => 'v');

        final auditLines = records
            .where((r) => r.message.startsWith('CACHE_AUDIT'))
            .map((r) => r.message)
            .toList();
        expect(auditLines, hasLength(2));
        expect(auditLines[0], contains('key=test:item'));
        expect(auditLines[0], contains('result=miss'));
        expect(auditLines[1], contains('result=hit'));
      });
    });
  });
}
