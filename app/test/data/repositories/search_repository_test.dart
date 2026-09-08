import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/services/search_service.dart';

import 'search_repository_test.mocks.dart';

@GenerateMocks([SearchService])
void main() {
  group('SearchRepository', () {
    late SearchRepository repository;
    late MockSearchService mockService;
    late StashCacheManager cacheManager;

    const query = 'tent';
    const communityIds = ['com1'];
    const lat = 47.6062;
    const lng = -122.3321;

    setUp(() async {
      mockService = MockSearchService();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = SearchRepository(cacheManager, mockService);
    });

    // ── search — result ────────────────────────────────────────────────────

    test('search returns result list from service', () async {
      final expected = [SearchResultItem(), SearchResultItem()];
      when(
        mockService.search(
          query: query,
          communityIds: communityIds,
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).thenAnswer((_) async => expected);

      final results = await repository.search(
        query: query,
        communityIds: communityIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      );

      expect(results, hasLength(2));
      verify(
        mockService.search(
          query: query,
          communityIds: communityIds,
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).called(1);
    });

    // ── search — caching ───────────────────────────────────────────────────

    test('search caches response for identical parameters', () async {
      when(
        mockService.search(
          query: query,
          communityIds: communityIds,
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).thenAnswer((_) async => []);

      await repository.search(
        query: query,
        communityIds: communityIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      );
      await repository.search(
        query: query,
        communityIds: communityIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      );

      verify(
        mockService.search(
          query: query,
          communityIds: communityIds,
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).called(1);
    });

    test('search uses different cache keys for different queries', () async {
      when(
        mockService.search(
          query: 'tent',
          communityIds: communityIds,
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).thenAnswer((_) async => []);
      when(
        mockService.search(
          query: 'kayak',
          communityIds: communityIds,
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).thenAnswer((_) async => []);

      await repository.search(
        query: 'tent',
        communityIds: communityIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      );
      await repository.search(
        query: 'kayak',
        communityIds: communityIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      );

      verify(mockService.search(
        query: 'tent',
        communityIds: communityIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      )).called(1);
      verify(mockService.search(
        query: 'kayak',
        communityIds: communityIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      )).called(1);
    });

    test('search sorts communityIds in cache key', () async {
      // The service receives the original unsorted IDs; only the cache key is
      // normalised. Stub with any communityIds to avoid argument-order mismatch.
      when(
        mockService.search(
          query: query,
          communityIds: anyNamed('communityIds'),
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).thenAnswer((_) async => []);

      await repository.search(
        query: query,
        communityIds: ['b', 'a'],
        latitudeDeg: lat,
        longitudeDeg: lng,
      );
      // Same communities in different order — should hit cache; no second service call.
      await repository.search(
        query: query,
        communityIds: ['a', 'b'],
        latitudeDeg: lat,
        longitudeDeg: lng,
      );

      verify(
        mockService.search(
          query: query,
          communityIds: anyNamed('communityIds'),
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).called(1);
    });

    // ── invalidateSearches ─────────────────────────────────────────────────

    test('invalidateSearches clears all cached searches', () async {
      when(
        mockService.search(
          query: query,
          communityIds: communityIds,
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).thenAnswer((_) async => []);

      await repository.search(
        query: query,
        communityIds: communityIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      );
      await repository.invalidateSearches();
      await repository.search(
        query: query,
        communityIds: communityIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      );

      verify(
        mockService.search(
          query: query,
          communityIds: communityIds,
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).called(2);
    });

    test('invalidateSearches fires onCacheInvalidated callback', () async {
      var callbackFired = false;
      repository = SearchRepository(
        cacheManager,
        mockService,
        () => callbackFired = true,
      );

      await repository.invalidateSearches();

      expect(callbackFired, isTrue);
    });

    test('invalidateAll delegates to invalidateSearches', () async {
      var callbackFired = false;
      repository = SearchRepository(
        cacheManager,
        mockService,
        () => callbackFired = true,
      );

      await repository.invalidateAll();

      expect(callbackFired, isTrue);
    });

    // ── invalidateSearchesForCommunity ─────────────────────────────────────

    test('invalidateSearchesForCommunity fires onCacheInvalidated callback', () async {
      var callbackFired = false;
      repository = SearchRepository(
        cacheManager,
        mockService,
        () => callbackFired = true,
      );

      await repository.invalidateSearchesForCommunity('com1');

      expect(callbackFired, isTrue);
    });

    test('invalidateSearchesForCommunity clears cache for single-community searches', () async {
      when(
        mockService.search(
          query: query,
          communityIds: ['com1'],
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).thenAnswer((_) async => []);

      await repository.search(
        query: query,
        communityIds: ['com1'],
        latitudeDeg: lat,
        longitudeDeg: lng,
      );
      await repository.invalidateSearchesForCommunity('com1');
      await repository.search(
        query: query,
        communityIds: ['com1'],
        latitudeDeg: lat,
        longitudeDeg: lng,
      );

      verify(
        mockService.search(
          query: query,
          communityIds: ['com1'],
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).called(2);
    });

    // ── error propagation ──────────────────────────────────────────────────

    test('search propagates service errors', () {
      when(
        mockService.search(
          query: query,
          communityIds: communityIds,
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
      ).thenThrow(Exception('network error'));

      expect(
        () => repository.search(
          query: query,
          communityIds: communityIds,
          latitudeDeg: lat,
          longitudeDeg: lng,
        ),
        throwsException,
      );
    });
  });
}
