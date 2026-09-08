import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/services/feed_service.dart';

import 'feed_repository_test.mocks.dart';

@GenerateMocks([FeedService])
void main() {
  group('FeedRepository', () {
    late FeedRepository repository;
    late MockFeedService mockService;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockFeedService();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = FeedRepository(cacheManager, mockService);
    });

    group('getFeed', () {
      test('fetches feed from service', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [
            FeedItem(
              id: 'item1',
              itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,

              occurredAtUnixSec: fixnum.Int64(1234567890),
              gearShared: GearSharedPayload(
                gearId: 'gear1',
                gearName: 'Test Gear',
                actor: User(id: 'user1', name: 'Test User'),
              ),
            ),
          ],
          nextPageToken: '',
        );

        when(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: null,
          ),
        ).thenAnswer((_) async => mockResponse);

        final result = await repository.getFeed([communityId]);

        expect(result.items.length, 1);
        expect(result.items[0].id, 'item1');
        expect(
          result.items[0].itemType,
          FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,
        );
        verify(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: null,
          ),
        ).called(1);
      });

      test('caches feed response', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [],
          nextPageToken: '',
        );

        when(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: null,
          ),
        ).thenAnswer((_) async => mockResponse);

        // First call - should fetch
        await repository.getFeed([communityId]);

        // Second call - should use cache
        await repository.getFeed([communityId]);

        // Service should only be called once
        verify(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: null,
          ),
        ).called(1);
      });

      test('handles pagination with page tokens', () async {
        const communityId = 'comm1';
        const pageToken = 'token123';
        final mockResponse = GetFeedResponse(
          items: [],
          nextPageToken: 'token456',
        );

        when(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: pageToken,
          ),
        ).thenAnswer((_) async => mockResponse);

        final result = await repository.getFeed([communityId], pageToken: pageToken);

        expect(result.nextPageToken, 'token456');
        verify(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: pageToken,
          ),
        ).called(1);
      });

      test('caches different pages separately', () async {
        const communityId = 'comm1';
        final page1Response = GetFeedResponse(items: [], nextPageToken: 'token2');
        final page2Response = GetFeedResponse(items: [], nextPageToken: '');

        when(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: null,
          ),
        ).thenAnswer((_) async => page1Response);

        when(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: 'token2',
          ),
        ).thenAnswer((_) async => page2Response);

        // Fetch page 1 twice
        await repository.getFeed([communityId]);
        await repository.getFeed([communityId]);

        // Fetch page 2 twice
        await repository.getFeed([communityId], pageToken: 'token2');
        await repository.getFeed([communityId], pageToken: 'token2');

        // Each page should only be fetched once
        verify(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: null,
          ),
        ).called(1);

        verify(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: 'token2',
          ),
        ).called(1);
      });
    });

    group('markItemsViewed', () {
      test('calls service to mark items as viewed', () async {
        const communityId = 'comm1';
        final itemIds = ['item1', 'item2'];

        when(
          mockService.markFeedItemsViewed(
            communityId: communityId,
            itemIds: itemIds,
          ),
        ).thenAnswer((_) async => {});

        await repository.markItemsViewed(communityId, itemIds);

        verify(
          mockService.markFeedItemsViewed(
            communityId: communityId,
            itemIds: itemIds,
          ),
        ).called(1);
      });

      test('invalidates cache after marking items viewed', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(items: [], nextPageToken: '');

        when(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: null,
          ),
        ).thenAnswer((_) async => mockResponse);

        when(
          mockService.markFeedItemsViewed(
            communityId: communityId,
            itemIds: ['item1'],
          ),
        ).thenAnswer((_) async => {});

        // Fetch feed (fills cache)
        await repository.getFeed([communityId]);

        // Mark items viewed (should invalidate cache)
        await repository.markItemsViewed(communityId, ['item1']);

        // Fetch feed again (should hit service again, not cache)
        await repository.getFeed([communityId]);

        // Service should be called twice (once before, once after invalidation)
        verify(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: null,
          ),
        ).called(2);
      });
    });

    group('invalidateFeed', () {
      test('invalidates feed cache for community', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(items: [], nextPageToken: '');

        when(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: null,
          ),
        ).thenAnswer((_) async => mockResponse);

        // Fetch feed (fills cache)
        await repository.getFeed([communityId]);

        // Invalidate cache
        await repository.invalidateFeed();

        // Fetch feed again (should hit service again)
        await repository.getFeed([communityId]);

        // Service should be called twice
        verify(
          mockService.getFeed(
            communityIds: [communityId],
            pageSize: 20,
            pageToken: null,
          ),
        ).called(2);
      });
    });
  });
}
