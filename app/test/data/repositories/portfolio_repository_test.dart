import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/portfolio_repository.dart';
import 'package:ripls/services/portfolio_service.dart';

import 'portfolio_repository_test.mocks.dart';

@GenerateMocks([PortfolioService])
void main() {
  group('PortfolioRepository', () {
    late PortfolioRepository repository;
    late MockPortfolioService mockService;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockPortfolioService();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = PortfolioRepository(cacheManager, mockService);
    });

    // ── getDirectoryPeople ─────────────────────────────────────────────────

    test('getDirectoryPeople caches response', () async {
      final expected = GetDirectoryPeopleResponse();
      when(mockService.getDirectoryPeople())
          .thenAnswer((_) async => expected);

      await repository.getDirectoryPeople();
      await repository.getDirectoryPeople();

      verify(mockService.getDirectoryPeople()).called(1);
    });

    // ── getPortfolioMetrics ────────────────────────────────────────────────

    test('getPortfolioMetrics caches response', () async {
      when(mockService.getPortfolioMetrics())
          .thenAnswer((_) async => GetPortfolioMetricsResponse());

      await repository.getPortfolioMetrics();
      await repository.getPortfolioMetrics();

      verify(mockService.getPortfolioMetrics()).called(1);
    });

    test('getPortfolioMetrics sorts communityIds in cache key', () async {
      // The service receives the original unsorted IDs; only the cache key is
      // normalised. Stub with anyNamed to avoid argument-order mismatch.
      when(mockService.getPortfolioMetrics(communityIds: anyNamed('communityIds')))
          .thenAnswer((_) async => GetPortfolioMetricsResponse());

      // Call with ['b', 'a'] first — should hit same cache as ['a', 'b'] on second call.
      await repository.getPortfolioMetrics(communityIds: ['b', 'a']);
      await repository.getPortfolioMetrics(communityIds: ['a', 'b']);

      verify(mockService.getPortfolioMetrics(communityIds: anyNamed('communityIds')))
          .called(1);
    });

    test('refreshMetrics invalidates cache so next call re-fetches', () async {
      when(mockService.getPortfolioMetrics())
          .thenAnswer((_) async => GetPortfolioMetricsResponse());

      await repository.getPortfolioMetrics();
      await repository.refreshMetrics();
      await repository.getPortfolioMetrics();

      verify(mockService.getPortfolioMetrics()).called(2);
    });

    // ── setWeeklyGoals ─────────────────────────────────────────────────────

    test('setWeeklyGoals calls service and invalidates the Home view', () async {
      when(mockService.setWeeklyGoals(60, 1000, 500))
          .thenAnswer((_) async => SetWeeklyGoalsResponse());
      when(mockService.getHomeView('UTC'))
          .thenAnswer((_) async => GetHomeViewResponse());

      await repository.getHomeView('UTC');
      await repository.setWeeklyGoals(60, 1000, 500);
      await repository.getHomeView('UTC');

      verify(mockService.getHomeView('UTC')).called(2);
    });

    // ── markInboxItemRead ──────────────────────────────────────────────────

    test('markInboxItemRead calls service and invalidates the Home view', () async {
      when(
        mockService.markInboxItemRead(
          itemType: DailyItemType.DAILY_ITEM_TYPE_TRANSFER,
          itemId: 'item1',
        ),
      ).thenAnswer((_) async => MarkInboxItemReadResponse());
      when(mockService.getHomeView('UTC'))
          .thenAnswer((_) async => GetHomeViewResponse());

      await repository.getHomeView('UTC');
      await repository.markInboxItemRead(
        itemType: DailyItemType.DAILY_ITEM_TYPE_TRANSFER,
        itemId: 'item1',
      );
      await repository.getHomeView('UTC');

      verify(mockService.getHomeView('UTC')).called(2);
    });

    // ── watchItem ──────────────────────────────────────────────────────────

    test('watchItem calls service, invalidates the Home view, and fires onWatchMutated', () async {
      var callbackFired = false;
      repository = PortfolioRepository(
        cacheManager,
        mockService,
        onWatchMutated: () => callbackFired = true,
      );

      when(
        mockService.watchItem(
          itemType: DailyItemType.DAILY_ITEM_TYPE_TRANSFER,
          itemId: 'item1',
        ),
      ).thenAnswer((_) async => WatchItemResponse());
      when(mockService.getHomeView('UTC'))
          .thenAnswer((_) async => GetHomeViewResponse());

      await repository.getHomeView('UTC');
      await repository.watchItem(
        itemType: DailyItemType.DAILY_ITEM_TYPE_TRANSFER,
        itemId: 'item1',
      );
      await repository.getHomeView('UTC');

      verify(mockService.getHomeView('UTC')).called(2);
      expect(callbackFired, isTrue);
    });

    // ── dismissInboxItem ───────────────────────────────────────────────────

    test('dismissInboxItem calls service, invalidates the Home view, and fires onWatchMutated', () async {
      var callbackFired = false;
      repository = PortfolioRepository(
        cacheManager,
        mockService,
        onWatchMutated: () => callbackFired = true,
      );

      when(
        mockService.dismissInboxItem(
          itemType: DailyItemType.DAILY_ITEM_TYPE_TRANSFER,
          itemId: 'item1',
        ),
      ).thenAnswer((_) async => DismissInboxItemResponse());
      when(mockService.getHomeView('UTC'))
          .thenAnswer((_) async => GetHomeViewResponse());

      await repository.getHomeView('UTC');
      await repository.dismissInboxItem(
        itemType: DailyItemType.DAILY_ITEM_TYPE_TRANSFER,
        itemId: 'item1',
      );
      await repository.getHomeView('UTC');

      verify(mockService.getHomeView('UTC')).called(2);
      expect(callbackFired, isTrue);
    });

    // ── error propagation ──────────────────────────────────────────────────

    test('getDirectoryPeople propagates service errors', () {
      when(mockService.getDirectoryPeople())
          .thenThrow(Exception('network down'));

      expect(repository.getDirectoryPeople(), throwsException);
    });
  });
}
