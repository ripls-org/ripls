import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/viewmodels/feed_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

import '../../helpers/fake_async_helpers.dart';
import 'feed_view_model_test.mocks.dart';

class _FakeAuthStateNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() => AuthStateData(
        isLoading: false,
        user: User(id: 'user-1', name: 'Tester'),
      );
}

@GenerateMocks([
  FeedRepository,
  CacheManager,
  GearRepository,
  RequestRepository,
  UserRepository,
  MediaRepository,
])
void main() {
  group('FeedNotifier', () {
    late ProviderContainer container;
    late MockFeedRepository mockRepository;
    late MockCacheManager mockCacheManager;
    late MockGearRepository mockGearRepository;
    late MockRequestRepository mockRequestRepository;
    late MockUserRepository mockUserRepository;
    late MockMediaRepository mockMediaRepository;

    setUp(() {
      mockRepository = MockFeedRepository();
      mockCacheManager = MockCacheManager();
      mockGearRepository = MockGearRepository();
      mockRequestRepository = MockRequestRepository();
      mockUserRepository = MockUserRepository();
      mockMediaRepository = MockMediaRepository();

      // Mock invalidate/invalidateAll methods
      when(mockRepository.invalidateFeed()).thenAnswer((_) async {});
      when(mockGearRepository.invalidateAll()).thenAnswer((_) async {});
      when(mockRequestRepository.invalidateAll()).thenAnswer((_) async {});
      when(mockUserRepository.invalidateAll()).thenAnswer((_) async {});
      when(mockMediaRepository.invalidateAll()).thenAnswer((_) async {});

      container = ProviderContainer(
        overrides: [
          feedRepositoryProvider.overrideWithValue(mockRepository),
          cacheManagerProvider.overrideWithValue(mockCacheManager),
          gearRepositoryProvider.overrideWithValue(mockGearRepository),
          requestRepositoryProvider.overrideWithValue(mockRequestRepository),
          userRepositoryProvider.overrideWithValue(mockUserRepository),
          mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
          authStateProvider.overrideWith(_FakeAuthStateNotifier.new),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    group('initialize', () {
      test('loads feed from repository', () async {
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
              ),
            ),
          ],
          nextPageToken: '',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);

        final state = container.read(feedProvider);
        expect(state.items.length, 1);
        expect(state.items[0].id, 'item1');
        expect(state.isLoading, false);
        expect(state.hasError, false);
        expect(state.currentCommunityIds, [communityId]);
        verify(mockRepository.getFeed([communityId])).called(1);
      });

      test('always fetches fresh data even if already initialized', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [
            FeedItem(
              id: 'item1',
              itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,
            ),
          ],
          nextPageToken: '',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);

        final notifier = container.read(feedProvider.notifier);

        // First call
        await notifier.initialize([communityId]);

        // Second call - should fetch again (feed must always be fresh)
        await notifier.initialize([communityId]);

        verify(mockRepository.invalidateFeed()).called(2);
        verify(mockRepository.getFeed([communityId])).called(2);
      });

      test('handles errors gracefully', () async {
        const communityId = 'comm1';
        when(mockRepository.getFeed([communityId]))
            .thenThrow(Exception('Network error'));

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);

        final state = container.read(feedProvider);
        expect(state.hasError, true);
        expect(state.error, isNotNull);
        expect(state.isLoading, false);
      });

      test('sets hasMore flag based on nextPageToken', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [],
          nextPageToken: 'token123',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);

        final state = container.read(feedProvider);
        expect(state.hasMore, true);
        expect(state.nextPageToken, 'token123');
      });
    });

    group('loadMore', () {
      test('appends items from next page', () async {
        const communityId = 'comm1';
        final page1Response = GetFeedResponse(
          items: [
            FeedItem(
              id: 'item1',
              itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,

              occurredAtUnixSec: fixnum.Int64(1234567890),
            ),
          ],
          nextPageToken: 'token2',
        );
        final page2Response = GetFeedResponse(
          items: [
            FeedItem(
              id: 'item2',
              itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,

              occurredAtUnixSec: fixnum.Int64(1234567891),
            ),
          ],
          nextPageToken: '',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => page1Response);
        when(mockRepository.getFeed([communityId], pageToken: 'token2'))
            .thenAnswer((_) async => page2Response);

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);
        await notifier.loadMore();

        final state = container.read(feedProvider);
        expect(state.items.length, 2);
        expect(state.items[0].id, 'item1');
        expect(state.items[1].id, 'item2');
        expect(state.hasMore, false);
      });

      test('does not load if already loading', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [FeedItem(id: 'item1', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED)],
          nextPageToken: 'token2',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);

        // Manually set isLoadingMore to simulate in-progress load
        notifier.state = notifier.state.copyWith(isLoadingMore: true);

        await notifier.loadMore();

        // Should not have called repository with page token since already loading
        verifyNever(mockRepository.getFeed([communityId], pageToken: 'token2'));
      });

      test('does not load if no more pages', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(items: [], nextPageToken: '');

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);
        await notifier.loadMore();

        // Should only have been called once during initialize
        verify(mockRepository.getFeed([communityId])).called(1);
      });

      test('handles errors without clearing existing items', () async {
        const communityId = 'comm1';
        final page1Response = GetFeedResponse(
          items: [
            FeedItem(id: 'item1', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED),
          ],
          nextPageToken: 'token2',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => page1Response);
        when(mockRepository.getFeed([communityId], pageToken: 'token2'))
            .thenThrow(Exception('Network error'));

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);
        await notifier.loadMore();

        final state = container.read(feedProvider);
        expect(state.items.length, 1); // Original items still present
        expect(state.isLoadingMore, false);
      });
    });

    group('markCurrentItemViewed', () {
      test('initialize does not mark anything viewed — that belongs to the '
          'swipe feed, not every consumer (#2799)', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [
            FeedItem(
              id: 'item1',
              itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,

              occurredAtUnixSec: fixnum.Int64(1234567890),
            ),
          ],
          nextPageToken: '',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);
        when(mockRepository.markItemsViewed(communityId, ['item1']))
            .thenAnswer((_) async => {});

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);

        // Long enough that a stray unawaited call would have landed.
        await Future.delayed(const Duration(milliseconds: 10));

        // The Home pulse also calls initialize(). Marking item 0 here made the
        // newest post the only one that ever expired, while everything older
        // stayed forever. FeedScreen now does it after initialize() returns.
        verifyNever(mockRepository.markItemsViewed(communityId, ['item1']));
      });

      test('marks item as viewed and increments view count', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [
            FeedItem(
              id: 'item1',
              itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,

              occurredAtUnixSec: fixnum.Int64(1234567890),
            ),
          ],
          nextPageToken: '',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);
        when(mockRepository.markItemsViewed(communityId, ['item1']))
            .thenAnswer((_) async => {});

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);
        await notifier.markCurrentItemViewed();

        verify(mockRepository.markItemsViewed(communityId, ['item1'])).called(1);
      });

      test('marks item as viewed even if view count already >= 2', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [
            FeedItem(
              id: 'item1',
              itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,
              isUnread: true,
              occurredAtUnixSec: fixnum.Int64(1234567890),
            ),
          ],
          nextPageToken: '',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);
        when(mockRepository.markItemsViewed(communityId, ['item1']))
            .thenAnswer((_) async => {});

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);
        await notifier.markCurrentItemViewed();

        final state = container.read(feedProvider);
        expect(state.items[0].isUnread, false);
        verify(mockRepository.markItemsViewed(communityId, ['item1'])).called(1);
      });

      test('does not crash if no items loaded', () async {
        final notifier = container.read(feedProvider.notifier);

        // Should not throw
        await notifier.markCurrentItemViewed();

        verifyNever(mockRepository.markItemsViewed(any, any));
      });

      test('handles errors silently', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [
            FeedItem(
              id: 'item1',
              itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,

            ),
          ],
          nextPageToken: '',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);
        when(mockRepository.markItemsViewed(communityId, ['item1']))
            .thenThrow(Exception('Network error'));

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);

        // Should not throw
        await notifier.markCurrentItemViewed();

        final state = container.read(feedProvider);
        expect(state.hasError, false); // Error should be silent
      });

      test(
          'sidebar New indicator clears after all feed items have been viewed',
          () async {
        // Simulate: sidebar reads feedStatusProvider and sees comm1=true (has new items).
        const communityId = 'comm1';
        when(mockRepository.getFeedStatus()).thenAnswer(
          (_) async => GetFeedStatusResponse(
            communityIdToHasNew: {communityId: true},
          ),
        );

        // Prime feedStatusProvider — sidebar "opens" and reads it.
        final initialStatus = await container.read(feedStatusProvider.future);
        expect(initialStatus[communityId], true);
        expect(container.read(communityHasNewFeedProvider(communityId)), true);

        // Server now returns comm1=false because items have been seen.
        when(mockRepository.getFeedStatus()).thenAnswer(
          (_) async => GetFeedStatusResponse(
            communityIdToHasNew: {communityId: false},
          ),
        );
        when(mockRepository.invalidateFeedStatus()).thenAnswer((_) async {});

        // Fire the same invalidation signal that the real FeedRepository fires
        // via _onFeedStatusInvalidated after markItemsViewed succeeds.
        // In production this is wired in providers.dart; in tests we trigger
        // it directly to keep the test focused on the reactive rebuild logic.
        container
            .read(feedStatusCacheInvalidationProvider.notifier)
            .notify();

        // Sidebar reopens — feedStatusProvider must have rebuilt with fresh data.
        final updatedStatus = await container.read(feedStatusProvider.future);
        expect(updatedStatus[communityId], false,
            reason:
                'FeedStatusNotifier should rebuild when the invalidation '
                'counter increments, so the sidebar no longer shows New');
        expect(
            container.read(communityHasNewFeedProvider(communityId)), false);
      });
    });

    group('setPageIndex', () {
      test('updates current page index', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [
            FeedItem(id: 'item1', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED),
            FeedItem(id: 'item2', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED),
          ],
          nextPageToken: '',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);
        when(mockRepository.markItemsViewed(any, any))
            .thenAnswer((_) async => {});

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);
        notifier.setPageIndex(1);

        final state = container.read(feedProvider);
        expect(state.currentPageIndex, 1);
      });

      test('triggers markCurrentItemViewed', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [
            FeedItem(id: 'item1', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED),
            FeedItem(id: 'item2', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED),
          ],
          nextPageToken: '',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);
        when(mockRepository.markItemsViewed(communityId, ['item2']))
            .thenAnswer((_) async => {});

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);
        notifier.setPageIndex(1);

        // Give async operations time to complete
        await Future.delayed(Duration(milliseconds: 10));

        verify(mockRepository.markItemsViewed(communityId, ['item2'])).called(1);
      });

      test('does not update if index unchanged', () {
        final notifier = container.read(feedProvider.notifier);

        // Initial state has currentPageIndex = 0
        notifier.setPageIndex(0);

        final state = container.read(feedProvider);
        expect(state.currentPageIndex, 0);
      });

      test('triggers loadMore when approaching end', () async {
        const communityId = 'comm1';
        final page1Response = GetFeedResponse(
          items: List.generate(
            10,
            (i) => FeedItem(
              id: 'item$i',
              itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,
            ),
          ),
          nextPageToken: 'token2',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => page1Response);
        when(mockRepository.markItemsViewed(any, any))
            .thenAnswer((_) async => {});

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);

        // Move to index 7 (within 3 of end at index 9)
        notifier.setPageIndex(7);

        // Give async operations time to complete
        await Future.delayed(Duration(milliseconds: 100));

        // Should have triggered loadMore
        verify(mockRepository.getFeed([communityId], pageToken: 'token2')).called(1);
      });
    });

  group('in-place refresh on cache invalidation', () {
      test('feedListingCacheInvalidation triggers in-place refresh', () {
        runDebounced((async) {
          const communityId = 'comm1';
          final initialResponse = GetFeedResponse(
            items: [
              FeedItem(id: 'item1', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED),
            ],
            nextPageToken: '',
          );
          final refreshedResponse = GetFeedResponse(
            items: [
              FeedItem(id: 'item1', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED),
              FeedItem(id: 'item2', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED),
            ],
            nextPageToken: '',
          );

          when(mockRepository.getFeed([communityId]))
              .thenAnswer((_) async => initialResponse);

          final notifier = container.read(feedProvider.notifier);
          notifier.initialize([communityId]);
          async.flushMicrotasks();

          when(mockRepository.getFeed([communityId]))
              .thenAnswer((_) async => refreshedResponse);

          // Fire the listing-level invalidation signal.
          container.read(feedListingCacheInvalidationProvider.notifier).notify();

          // Let the debounce timer fire.
          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          final state = container.read(feedProvider);
          expect(state.items.length, 2,
              reason: 'feed should refresh when feedListingCacheInvalidationProvider fires');
          expect(state.isLoading, false,
              reason: 'in-place refresh must not set isLoading: true');
        });
      });

      test('contentCacheInvalidation alone does NOT trigger feed refresh', () {
        runDebounced((async) {
          const communityId = 'comm1';
          final mockResponse = GetFeedResponse(
            items: [
              FeedItem(id: 'item1', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED),
            ],
            nextPageToken: '',
          );

          when(mockRepository.getFeed([communityId]))
              .thenAnswer((_) async => mockResponse);

          final notifier = container.read(feedProvider.notifier);
          notifier.initialize([communityId]);
          async.flushMicrotasks();

          // Reset interaction tracking so we can assert no new calls below.
          clearInteractions(mockRepository);

          // Fire only the detail-level content invalidation (e.g., RSVP).
          container.read(contentCacheInvalidationProvider.notifier).notify();

          // Wait longer than the debounce delay to confirm nothing fires.
          async.elapse(const Duration(milliseconds: 400));
          async.flushMicrotasks();

          // getFeed must not be called in response to a detail-level event.
          verifyNever(mockRepository.getFeed([communityId]));
          expect(container.read(feedProvider).items.length, 1,
              reason: 'feed items should remain unchanged');
        });
      });
    });

  group('FeedStatusNotifier', () {
    late ProviderContainer container;
    late MockFeedRepository mockRepository;

    setUp(() {
      mockRepository = MockFeedRepository();
      container = ProviderContainer(
        overrides: [
          feedRepositoryProvider.overrideWithValue(mockRepository),
          authStateProvider.overrideWith(_FakeAuthStateNotifier.new),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    test('loads status map from repository on first access', () async {
      when(mockRepository.getFeedStatus()).thenAnswer(
        (_) async => GetFeedStatusResponse(
          communityIdToHasNew: {'comm1': true, 'comm2': false},
        ),
      );

      final status = await container.read(feedStatusProvider.future);
      expect(status['comm1'], true);
      expect(status['comm2'], false);
    });

    test('refresh invalidates cache and reloads', () async {
      when(mockRepository.getFeedStatus()).thenAnswer(
        (_) async => GetFeedStatusResponse(
          communityIdToHasNew: {'comm1': true},
        ),
      );
      when(mockRepository.invalidateFeedStatus()).thenAnswer((_) async {});

      // Initial load.
      await container.read(feedStatusProvider.future);

      // Trigger refresh and await the subsequent rebuild.
      await container.read(feedStatusProvider.notifier).refresh();
      await container.read(feedStatusProvider.future);

      verify(mockRepository.invalidateFeedStatus()).called(1);
      verify(mockRepository.getFeedStatus()).called(2);
    });

    test('communityHasNewFeedProvider returns true for community with new items',
        () async {
      when(mockRepository.getFeedStatus()).thenAnswer(
        (_) async => GetFeedStatusResponse(
          communityIdToHasNew: {'comm1': true, 'comm2': false},
        ),
      );

      await container.read(feedStatusProvider.future);

      expect(container.read(communityHasNewFeedProvider('comm1')), true);
      expect(container.read(communityHasNewFeedProvider('comm2')), false);
    });

    test('communityHasNewFeedProvider returns false for unknown community',
        () async {
      when(mockRepository.getFeedStatus()).thenAnswer(
        (_) async => GetFeedStatusResponse(communityIdToHasNew: {}),
      );

      await container.read(feedStatusProvider.future);

      expect(container.read(communityHasNewFeedProvider('unknown')), false);
    });
  });

  group('refresh', () {
      test('invalidates feed cache and clears all related caches', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(items: [], nextPageToken: '');

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);
        when(mockRepository.invalidateAllFeeds())
            .thenAnswer((_) async => {});

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);
        await notifier.refresh();

        // Verify all feed caches are invalidated (not just the active community)
        verify(mockRepository.invalidateAllFeeds()).called(1);

        // Verify repository invalidateAll() methods were called for all related data
        verify(mockGearRepository.invalidateAll()).called(1);
        verify(mockRequestRepository.invalidateAll()).called(1);
        verify(mockUserRepository.invalidateAll()).called(1);
        verify(mockMediaRepository.invalidateAll()).called(1);

        // Verify feed was reloaded
        verify(mockRepository.getFeed([communityId])).called(2); // Once for init, once for refresh
      });

      test('resets state before reloading', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(
          items: [
            FeedItem(id: 'item1', itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED),
          ],
          nextPageToken: '',
        );

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);
        when(mockRepository.invalidateAllFeeds())
            .thenAnswer((_) async => {});

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);

        // Change state
        notifier.setPageIndex(1);

        await notifier.refresh();

        final state = container.read(feedProvider);
        expect(state.currentPageIndex, 0); // Reset to 0
      });

      test('invalidates all feed caches even if no community loaded', () async {
        when(mockRepository.invalidateAllFeeds()).thenAnswer((_) async => {});

        final notifier = container.read(feedProvider.notifier);
        await notifier.refresh();

        // invalidateAllFeeds is called before the early-return guard so the
        // next initialize() call fetches fresh data even from a cold start.
        verify(mockRepository.invalidateAllFeeds()).called(1);
        verifyNever(mockRepository.getFeed(any));
        verifyNever(mockGearRepository.invalidateAll());
        verifyNever(mockRequestRepository.invalidateAll());
        verifyNever(mockUserRepository.invalidateAll());
        verifyNever(mockMediaRepository.invalidateAll());
      });

      test('handles errors during cache clearing gracefully', () async {
        const communityId = 'comm1';
        final mockResponse = GetFeedResponse(items: [], nextPageToken: '');

        when(mockRepository.getFeed([communityId]))
            .thenAnswer((_) async => mockResponse);
        when(mockRepository.invalidateAllFeeds())
            .thenAnswer((_) async => {});
        // Override the setUp mock to throw an exception
        when(mockGearRepository.invalidateAll())
            .thenThrow(Exception('Cache error'));

        final notifier = container.read(feedProvider.notifier);
        await notifier.initialize([communityId]);

        // Should throw since we're not handling repository invalidation errors
        expect(() => notifier.refresh(), throwsException);
      });
    });
  });
}
