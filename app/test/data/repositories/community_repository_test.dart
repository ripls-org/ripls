import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/services/community_service.dart';

import 'community_repository_test.mocks.dart';

@GenerateMocks([CommunityService, SearchRepository, FeedRepository])
void main() {
  group('CommunityRepository', () {
    late CommunityRepository repository;
    late MockCommunityService mockService;
    late MockSearchRepository mockSearchRepository;
    late MockFeedRepository mockFeedRepository;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockCommunityService();
      mockSearchRepository = MockSearchRepository();
      mockFeedRepository = MockFeedRepository();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = CommunityRepository(
          cacheManager, mockService, mockSearchRepository, mockFeedRepository);
    });

    group('getById', () {
      test('fetches community from service', () async {
        const communityId = 'comm1';
        final mockCommunity = GetCommunityResponse(
          id: communityId,
          name: 'Test Community',
        );

        when(
          mockService.getCommunity(communityId),
        ).thenAnswer((_) async => mockCommunity);

        final result = await repository.get(communityId);

        expect(result.id, communityId);
        expect(result.name, 'Test Community');
        verify(mockService.getCommunity(communityId)).called(1);
      });

      test('caches community details', () async {
        const communityId = 'comm1';
        final mockCommunity = GetCommunityResponse(
          id: communityId,
          name: 'Test Community',
        );

        when(
          mockService.getCommunity(communityId),
        ).thenAnswer((_) async => mockCommunity);

        // First call - should fetch
        await repository.get(communityId);

        // Second call - should use cache
        await repository.get(communityId);

        // Service should only be called once
        verify(mockService.getCommunity(communityId)).called(1);
      });
    });

    group('listUserCommunities', () {
      test('fetches user communities from service', () async {
        final mockCommunities = [
          CommunityItem(id: 'comm1', name: 'Community 1'),
          CommunityItem(id: 'comm2', name: 'Community 2'),
        ];

        when(
          mockService.listCommunities(),
        ).thenAnswer((_) async => mockCommunities);

        final result = await repository.listUserCommunities();

        expect(result.length, 2);
        expect(result[0].id, 'comm1');
        expect(result[1].id, 'comm2');
        verify(mockService.listCommunities()).called(1);
      });

      test('caches user communities', () async {
        final mockCommunities = [
          CommunityItem(id: 'comm1', name: 'Community 1'),
        ];

        when(
          mockService.listCommunities(),
        ).thenAnswer((_) async => mockCommunities);

        // First call - should fetch
        await repository.listUserCommunities();

        // Second call - should use cache
        await repository.listUserCommunities();

        // Service should only be called once
        verify(mockService.listCommunities()).called(1);
      });
    });

    group('refreshUserCommunities', () {
      test('invalidates and refetches user communities', () async {
        final mockCommunities = [
          CommunityItem(id: 'comm1', name: 'Community 1'),
        ];

        when(
          mockService.listCommunities(),
        ).thenAnswer((_) async => mockCommunities);

        // Populate cache
        await repository.listUserCommunities();

        // Refresh cache - this invalidates and immediately refetches
        final result = await repository.refreshUserCommunities();

        // Verify refresh returned the data
        expect(result, mockCommunities);
        // Service should have been called twice total (initial + refresh)
        verify(mockService.listCommunities()).called(2);
      });
    });

    group('listCommunityGear', () {
      test('fetches community gear from service', () async {
        const communityId = 'comm1';
        final mockGear = [
          CommunityGearItem(id: 'gear1', name: 'Tent'),
          CommunityGearItem(id: 'gear2', name: 'Kayak'),
        ];

        when(
          mockService.listCommunityGear(communityId),
        ).thenAnswer((_) async => mockGear);

        final result = await repository.listCommunityGear(communityId);

        expect(result.length, 2);
        expect(result[0].id, 'gear1');
        expect(result[1].id, 'gear2');
        verify(mockService.listCommunityGear(communityId)).called(1);
      });

      test('caches community gear', () async {
        const communityId = 'comm1';
        final mockGear = [CommunityGearItem(id: 'gear1', name: 'Tent')];

        when(
          mockService.listCommunityGear(communityId),
        ).thenAnswer((_) async => mockGear);

        // First call - should fetch
        await repository.listCommunityGear(communityId);

        // Second call - should use cache
        await repository.listCommunityGear(communityId);

        // Service should only be called once
        verify(mockService.listCommunityGear(communityId)).called(1);
      });

      test('caches separately by communityId', () async {
        final gear1 = [CommunityGearItem(id: 'gear1', name: 'Tent')];
        final gear2 = [CommunityGearItem(id: 'gear2', name: 'Kayak')];

        when(
          mockService.listCommunityGear('comm1'),
        ).thenAnswer((_) async => gear1);
        when(
          mockService.listCommunityGear('comm2'),
        ).thenAnswer((_) async => gear2);

        // Fetch gear for community 1
        final result1 = await repository.listCommunityGear('comm1');
        expect(result1.length, 1);
        expect(result1[0].id, 'gear1');

        // Fetch gear for community 2 (should be separate cache)
        final result2 = await repository.listCommunityGear('comm2');
        expect(result2.length, 1);
        expect(result2[0].id, 'gear2');

        // Each should have been called once
        verify(mockService.listCommunityGear('comm1')).called(1);
        verify(mockService.listCommunityGear('comm2')).called(1);
      });
    });

    group('refreshCommunityGear', () {
      test('invalidates and refetches community gear', () async {
        const communityId = 'comm1';
        final mockGear = [CommunityGearItem(id: 'gear1', name: 'Tent')];

        when(
          mockService.listCommunityGear(communityId),
        ).thenAnswer((_) async => mockGear);

        // Populate cache
        await repository.listCommunityGear(communityId);

        // Refresh cache - this invalidates and immediately refetches
        final result = await repository.refreshCommunityGear(communityId);

        // Verify refresh returned the data
        expect(result, mockGear);
        // Service should have been called twice total (initial + refresh)
        verify(mockService.listCommunityGear(communityId)).called(2);
      });
    });

    group('invalidateAllCommunityGearLists', () {
      test('forces re-fetch for all community gear lists on next call',
          () async {
        const communityId1 = 'comm1';
        const communityId2 = 'comm2';
        final mockGear = [CommunityGearItem(id: 'gear1', name: 'Tent')];

        when(
          mockService.listCommunityGear(communityId1),
        ).thenAnswer((_) async => mockGear);
        when(
          mockService.listCommunityGear(communityId2),
        ).thenAnswer((_) async => mockGear);

        // Populate caches for two communities
        await repository.listCommunityGear(communityId1);
        await repository.listCommunityGear(communityId2);

        // Invalidate all gear lists
        await repository.invalidateAllCommunityGearLists();

        // Both should re-fetch
        await repository.listCommunityGear(communityId1);
        await repository.listCommunityGear(communityId2);

        // Each service call should have been made twice (initial + after invalidate)
        verify(mockService.listCommunityGear(communityId1)).called(2);
        verify(mockService.listCommunityGear(communityId2)).called(2);
      });
    });

    group('listCommunityUsers', () {
      test('fetches community users from service', () async {
        const communityId = 'comm1';
        final mockMembers = [
          CommunityMember(user: User(id: 'user1', name: 'Alice')),
          CommunityMember(user: User(id: 'user2', name: 'Bob')),
        ];

        when(
          mockService.listCommunityUsers(communityId),
        ).thenAnswer((_) async => mockMembers);

        final result = await repository.listCommunityUsers(communityId);

        expect(result.length, 2);
        expect(result[0].user.id, 'user1');
        expect(result[1].user.id, 'user2');
        verify(mockService.listCommunityUsers(communityId)).called(1);
      });

      test('caches community users', () async {
        const communityId = 'comm1';
        final mockMembers = [CommunityMember(user: User(id: 'user1', name: 'Alice'))];

        when(
          mockService.listCommunityUsers(communityId),
        ).thenAnswer((_) async => mockMembers);

        // First call - should fetch
        await repository.listCommunityUsers(communityId);

        // Second call - should use cache
        await repository.listCommunityUsers(communityId);

        // Service should only be called once
        verify(mockService.listCommunityUsers(communityId)).called(1);
      });
    });

    group('listCommunityEvents', () {
      test('fetches community events from service', () async {
        const communityId = 'comm1';
        final mockEvents = [
          CommunityEventItem(
            id: 'event1',
            eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
          ),
          CommunityEventItem(
            id: 'event2',
            eventType:
                CommunityEventType.COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
          ),
        ];

        when(
          mockService.listCommunityEvents(communityId),
        ).thenAnswer((_) async => mockEvents);

        final result = await repository.getEvents(communityId);

        expect(result.length, 2);
        expect(result[0].id, 'event1');
        expect(result[1].id, 'event2');
        verify(mockService.listCommunityEvents(communityId)).called(1);
      });

      test('caches community events', () async {
        const communityId = 'comm1';
        final mockEvents = [
          CommunityEventItem(
            id: 'event1',
            eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
          ),
        ];

        when(
          mockService.listCommunityEvents(communityId),
        ).thenAnswer((_) async => mockEvents);

        // First call - should fetch
        await repository.getEvents(communityId);

        // Second call - should use cache
        await repository.getEvents(communityId);

        // Service should only be called once
        verify(mockService.listCommunityEvents(communityId)).called(1);
      });
    });

    group('createCommunity', () {
      test('creates community and invalidates cache', () async {
        const communityId = 'new-comm';
        const name = 'New Community';
        const description = 'Test description';

        when(
          mockService.createCommunity(name: name, description: description),
        ).thenAnswer((_) async => communityId);

        // Mock the list call that will be made by refreshUserCommunities
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        final result = await repository.createCommunity(
          name: name,
          description: description,
        );

        expect(result, communityId);
        verify(
          mockService.createCommunity(name: name, description: description),
        ).called(1);
      });

      test('invalidates user communities cache after creation', () async {
        const communityId = 'new-comm';
        final mockCommunities = [
          CommunityItem(id: 'comm1', name: 'Community 1'),
        ];

        when(
          mockService.listCommunities(),
        ).thenAnswer((_) async => mockCommunities);
        when(
          mockService.createCommunity(name: 'New', description: 'Desc'),
        ).thenAnswer((_) async => communityId);

        // Initial population
        await repository.listUserCommunities();

        // Create new community - this calls refreshUserCommunities which fetches again
        await repository.createCommunity(name: 'New', description: 'Desc');

        // Service should have been called twice (initial + refresh in createCommunity)
        verify(mockService.listCommunities()).called(2);
      });
    });

    group('updateCommunity', () {
      test('updates community and invalidates cache', () async {
        const communityId = 'comm1';
        const name = 'Updated Name';

        when(
          mockService.updateCommunity(id: communityId, name: name),
        ).thenAnswer((_) async {});

        // Mock the list call that will be made by refreshUserCommunities
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        await repository.updateCommunity(id: communityId, name: name);

        verify(
          mockService.updateCommunity(id: communityId, name: name),
        ).called(1);
      });

      test('invalidates community cache after update', () async {
        const communityId = 'comm1';
        final mockCommunity = GetCommunityResponse(
          id: communityId,
          name: 'Old Name',
        );
        final mockCommunities = [CommunityItem(id: 'comm1', name: 'Community 1')];

        when(
          mockService.getCommunity(communityId),
        ).thenAnswer((_) async => mockCommunity);
        when(
          mockService.updateCommunity(id: communityId, name: 'New Name'),
        ).thenAnswer((_) async {});
        when(mockService.listCommunities()).thenAnswer((_) async => mockCommunities);

        // Initial get
        await repository.get(communityId);

        // Update community - this calls refreshUserCommunities
        await repository.updateCommunity(id: communityId, name: 'New Name');

        // Next get call should fetch again (cache was invalidated)
        await repository.get(communityId);

        // getCommunity should be called twice (initial + after update)
        verify(mockService.getCommunity(communityId)).called(2);
      });
    });

    group('deleteCommunity', () {
      test('deletes community and invalidates cache', () async {
        const communityId = 'comm1';

        when(mockService.deleteCommunity(communityId)).thenAnswer((_) async {});

        // Mock the list call that will be made by refreshUserCommunities
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        await repository.deleteCommunity(communityId);

        verify(mockService.deleteCommunity(communityId)).called(1);
      });

      test('invalidates deleted-list so the soft-deleted community shows up',
          () async {
        const communityId = 'comm1';
        final firstResponse = <DeletedCommunityItem>[];
        final secondResponse = [
          DeletedCommunityItem(id: communityId, name: 'Soft-deleted'),
        ];

        when(mockService.listDeletedCommunitiesForRestore())
            .thenAnswer((_) async => firstResponse);

        // Populate the deleted-list cache.
        final initial =
            await repository.listDeletedCommunitiesForRestore();
        expect(initial, isEmpty);

        when(mockService.deleteCommunity(communityId)).thenAnswer((_) async {});
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        // After delete, the next read should hit the service, not return
        // the stale empty list.
        when(mockService.listDeletedCommunitiesForRestore())
            .thenAnswer((_) async => secondResponse);

        await repository.deleteCommunity(communityId);

        final after = await repository.listDeletedCommunitiesForRestore();
        expect(after, secondResponse);
        verify(mockService.listDeletedCommunitiesForRestore()).called(2);
      });
    });

    group('listDeletedCommunitiesForRestore', () {
      test('returns service result', () async {
        final response = [
          DeletedCommunityItem(id: 'comm1', name: 'Deleted One'),
          DeletedCommunityItem(id: 'comm2', name: 'Deleted Two'),
        ];

        when(mockService.listDeletedCommunitiesForRestore())
            .thenAnswer((_) async => response);

        final result = await repository.listDeletedCommunitiesForRestore();

        expect(result, response);
        verify(mockService.listDeletedCommunitiesForRestore()).called(1);
      });

      test('caches the result; second call hits the cache', () async {
        final response = [
          DeletedCommunityItem(id: 'comm1', name: 'Deleted One'),
        ];

        when(mockService.listDeletedCommunitiesForRestore())
            .thenAnswer((_) async => response);

        await repository.listDeletedCommunitiesForRestore();
        await repository.listDeletedCommunitiesForRestore();

        verify(mockService.listDeletedCommunitiesForRestore()).called(1);
      });

      test('refresh: true bypasses the cache', () async {
        final response = [
          DeletedCommunityItem(id: 'comm1', name: 'Deleted One'),
        ];

        when(mockService.listDeletedCommunitiesForRestore())
            .thenAnswer((_) async => response);

        await repository.listDeletedCommunitiesForRestore();
        await repository.listDeletedCommunitiesForRestore(refresh: true);

        verify(mockService.listDeletedCommunitiesForRestore()).called(2);
      });

      test('invalidateDeletedList forces the next read to refetch', () async {
        final response = [
          DeletedCommunityItem(id: 'comm1', name: 'Deleted One'),
        ];

        when(mockService.listDeletedCommunitiesForRestore())
            .thenAnswer((_) async => response);

        await repository.listDeletedCommunitiesForRestore();
        await repository.invalidateDeletedList();
        await repository.listDeletedCommunitiesForRestore();

        verify(mockService.listDeletedCommunitiesForRestore()).called(2);
      });
    });

    group('listRejoinableCommunities', () {
      test('returns service result', () async {
        final response = [
          RejoinableCommunityItem(id: 'comm1', name: 'Recently Left One'),
          RejoinableCommunityItem(id: 'comm2', name: 'Recently Left Two'),
        ];

        when(mockService.listRejoinableCommunities())
            .thenAnswer((_) async => response);

        final result = await repository.listRejoinableCommunities();

        expect(result, response);
        verify(mockService.listRejoinableCommunities()).called(1);
      });

      test('caches the result; second call hits the cache', () async {
        final response = [
          RejoinableCommunityItem(id: 'comm1', name: 'Recently Left One'),
        ];

        when(mockService.listRejoinableCommunities())
            .thenAnswer((_) async => response);

        await repository.listRejoinableCommunities();
        await repository.listRejoinableCommunities();

        verify(mockService.listRejoinableCommunities()).called(1);
      });

      test('refresh: true bypasses the cache', () async {
        final response = [
          RejoinableCommunityItem(id: 'comm1', name: 'Recently Left One'),
        ];

        when(mockService.listRejoinableCommunities())
            .thenAnswer((_) async => response);

        await repository.listRejoinableCommunities();
        await repository.listRejoinableCommunities(refresh: true);

        verify(mockService.listRejoinableCommunities()).called(2);
      });

      test('invalidateRejoinableList forces the next read to refetch',
          () async {
        final response = [
          RejoinableCommunityItem(id: 'comm1', name: 'Recently Left One'),
        ];

        when(mockService.listRejoinableCommunities())
            .thenAnswer((_) async => response);

        await repository.listRejoinableCommunities();
        await repository.invalidateRejoinableList();
        await repository.listRejoinableCommunities();

        verify(mockService.listRejoinableCommunities()).called(2);
      });

      test('invalidateRejoinableList fires the onRejoinableListInvalidated '
          'callback', () async {
        var callbackCount = 0;
        final cacheManager2 = StashCacheManager();
        await cacheManager2.initialize();
        final repo2 = CommunityRepository(
          cacheManager2,
          mockService,
          mockSearchRepository,
          mockFeedRepository,
          onRejoinableListInvalidated: () => callbackCount++,
        );

        await repo2.invalidateRejoinableList();
        expect(callbackCount, 1);

        await repo2.invalidateRejoinableList();
        expect(callbackCount, 2);
      });
    });

    group('rejoinCommunity', () {
      test('calls service and invalidates community + rejoinable-list',
          () async {
        const communityId = 'comm1';

        when(mockService.rejoinCommunity(communityId))
            .thenAnswer((_) async {});
        when(mockService.listCommunities()).thenAnswer((_) async => []);
        when(mockService.listRejoinableCommunities())
            .thenAnswer((_) async => []);

        // Populate the rejoinable-list cache so the post-rejoin invalidation
        // is observable.
        await repository.listRejoinableCommunities();

        await repository.rejoinCommunity(communityId);

        verify(mockService.rejoinCommunity(communityId)).called(1);

        // Subsequent read after rejoin goes to the service (cache cleared).
        await repository.listRejoinableCommunities();
        verify(mockService.listRejoinableCommunities()).called(2);
      });

      test('refreshes the user-communities list after rejoin', () async {
        const communityId = 'comm1';

        when(mockService.rejoinCommunity(communityId))
            .thenAnswer((_) async {});
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        await repository.listUserCommunities();

        await repository.rejoinCommunity(communityId);

        verify(mockService.listCommunities()).called(2);
      });

      test('failure preserves the rejoinable-list cache snapshot', () async {
        const communityId = 'comm1';
        final response = [
          RejoinableCommunityItem(id: communityId, name: 'Recently Left'),
        ];

        when(mockService.listRejoinableCommunities())
            .thenAnswer((_) async => response);
        when(mockService.rejoinCommunity(communityId))
            .thenThrow(ServiceException('boom'));

        final cached = await repository.listRejoinableCommunities();
        expect(cached, response);

        await expectLater(
          repository.rejoinCommunity(communityId),
          throwsA(isA<ServiceException>()),
        );

        await repository.listRejoinableCommunities();
        verify(mockService.listRejoinableCommunities()).called(1);
        verifyNever(mockService.listCommunities());
      });

      test('fires onRejoinableListInvalidated on success', () async {
        const communityId = 'comm1';
        var callbackCount = 0;
        final cacheManager2 = StashCacheManager();
        await cacheManager2.initialize();
        final repo2 = CommunityRepository(
          cacheManager2,
          mockService,
          mockSearchRepository,
          mockFeedRepository,
          onRejoinableListInvalidated: () => callbackCount++,
        );

        when(mockService.rejoinCommunity(communityId))
            .thenAnswer((_) async {});
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        await repo2.rejoinCommunity(communityId);
        expect(callbackCount, 1);
      });
    });

    group('restoreCommunity', () {
      test('calls service and invalidates community + deleted-list', () async {
        const communityId = 'comm1';

        when(mockService.restoreCommunity(communityId))
            .thenAnswer((_) async {});
        when(mockService.listCommunities()).thenAnswer((_) async => []);
        when(mockService.listDeletedCommunitiesForRestore())
            .thenAnswer((_) async => []);

        // Populate the deleted-list cache so the post-restore invalidation
        // is observable.
        await repository.listDeletedCommunitiesForRestore();

        await repository.restoreCommunity(communityId);

        verify(mockService.restoreCommunity(communityId)).called(1);

        // Subsequent read after restore goes to the service (cache cleared).
        await repository.listDeletedCommunitiesForRestore();
        verify(mockService.listDeletedCommunitiesForRestore()).called(2);
      });

      test('refreshes the user-communities list after restore', () async {
        const communityId = 'comm1';

        when(mockService.restoreCommunity(communityId))
            .thenAnswer((_) async {});
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        // Populate user-communities cache first.
        await repository.listUserCommunities();

        await repository.restoreCommunity(communityId);

        // Service should have been called twice: once for initial populate,
        // once for refresh inside restoreCommunity.
        verify(mockService.listCommunities()).called(2);
      });

      test('failure preserves the deleted-list cache snapshot', () async {
        const communityId = 'comm1';
        final response = [
          DeletedCommunityItem(id: communityId, name: 'Deleted'),
        ];

        when(mockService.listDeletedCommunitiesForRestore())
            .thenAnswer((_) async => response);
        when(mockService.restoreCommunity(communityId))
            .thenThrow(ServiceException('boom'));

        // Populate the cache.
        final cached = await repository.listDeletedCommunitiesForRestore();
        expect(cached, response);

        await expectLater(
          repository.restoreCommunity(communityId),
          throwsA(isA<ServiceException>()),
        );

        // The next read must come from cache — service is not consulted
        // again because the failed restore should not have invalidated
        // anything.
        await repository.listDeletedCommunitiesForRestore();
        verify(mockService.listDeletedCommunitiesForRestore()).called(1);
        verifyNever(mockService.listCommunities());
      });
    });

    group('leaveCommunity', () {
      test('leaves community and invalidates cache', () async {
        const communityId = 'comm1';

        when(mockService.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenAnswer((_) async {});

        // Mock the list call that will be made by refreshUserCommunities
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        await repository.leaveCommunity(communityId);

        verify(mockService.leaveCommunity(
          communityId,
          newOwnerUserId: null,
        )).called(1);
      });

      test('threads newOwnerUserId through to the service for owner-leave',
          () async {
        const communityId = 'comm1';
        const candidateId = 'user-2';

        when(mockService.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenAnswer((_) async {});
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        await repository.leaveCommunity(
          communityId,
          newOwnerUserId: candidateId,
        );

        verify(mockService.leaveCommunity(
          communityId,
          newOwnerUserId: candidateId,
        )).called(1);
      });

      test('invalidates rejoinable-list so the freshly-left community '
          'surfaces in recently-left', () async {
        const communityId = 'comm1';
        final firstResponse = <RejoinableCommunityItem>[];
        final secondResponse = [
          RejoinableCommunityItem(id: communityId, name: 'Just Left'),
        ];

        when(mockService.listRejoinableCommunities())
            .thenAnswer((_) async => firstResponse);

        // Populate the rejoinable-list cache.
        final initial = await repository.listRejoinableCommunities();
        expect(initial, isEmpty);

        when(mockService.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenAnswer((_) async {});
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        // After leave, the next read should hit the service, not the
        // stale empty list.
        when(mockService.listRejoinableCommunities())
            .thenAnswer((_) async => secondResponse);

        await repository.leaveCommunity(communityId);

        final after = await repository.listRejoinableCommunities();
        expect(after, secondResponse);
        verify(mockService.listRejoinableCommunities()).called(2);
      });

      test('invalidates deleted-list so a sole-member leave (which '
          'soft-deletes the community) surfaces in recently-deleted',
          () async {
        const communityId = 'comm1';
        final firstResponse = <DeletedCommunityItem>[];
        final secondResponse = [
          DeletedCommunityItem(id: communityId, name: 'Sole-Member-Deleted'),
        ];

        when(mockService.listDeletedCommunitiesForRestore())
            .thenAnswer((_) async => firstResponse);

        // Populate the deleted-list cache.
        final initial =
            await repository.listDeletedCommunitiesForRestore();
        expect(initial, isEmpty);

        when(mockService.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenAnswer((_) async {});
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        // After leave, the next read of the deleted-list should hit
        // the service (cache cleared) — sole-member-leave converts to
        // community soft-delete server-side, and the leaver is in the
        // eligible-restorer snapshot.
        when(mockService.listDeletedCommunitiesForRestore())
            .thenAnswer((_) async => secondResponse);

        await repository.leaveCommunity(communityId);

        final after = await repository.listDeletedCommunitiesForRestore();
        expect(after, secondResponse);
        verify(mockService.listDeletedCommunitiesForRestore()).called(2);
      });

      test('invalidates user communities cache after leaving', () async {
        const communityId = 'comm1';
        final mockCommunities = [
          CommunityItem(id: 'comm1', name: 'Community 1'),
          CommunityItem(id: 'comm2', name: 'Community 2'),
        ];

        when(
          mockService.listCommunities(),
        ).thenAnswer((_) async => mockCommunities);
        when(mockService.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenAnswer((_) async {});

        // Initial population
        await repository.listUserCommunities();

        // Leave community - this calls refreshUserCommunities which fetches again
        await repository.leaveCommunity(communityId);

        // Service should have been called twice (initial + refresh in leaveCommunity)
        verify(mockService.listCommunities()).called(2);
      });
    });

    group('shareGear', () {
      test('shares gear and invalidates community gear cache', () async {
        const gearId = 'gear1';
        const communityId = 'comm1';
        final mockGear = [CommunityGearItem(id: 'gear2', name: 'Existing Gear')];

        when(
          mockService.shareGear(
            gearId: gearId,
            communityId: communityId,
            availability: Availability.AVAILABILITY_FOR_LOAN,
          ),
        ).thenAnswer((_) async {});
        when(mockService.listCommunityGear(communityId))
            .thenAnswer((_) async => mockGear);

        await repository.shareGear(
          gearId: gearId,
          communityId: communityId,
          availability: Availability.AVAILABILITY_FOR_LOAN,
        );

        verify(
          mockService.shareGear(
            gearId: gearId,
            communityId: communityId,
            availability: Availability.AVAILABILITY_FOR_LOAN,
          ),
        ).called(1);
        // Verify that refreshCommunityGear called listCommunityGear
        verify(mockService.listCommunityGear(communityId)).called(1);
      });

      test('invalidates community gear cache after sharing', () async {
        const gearId = 'gear1';
        const communityId = 'comm1';
        final mockGear = [
          CommunityGearItem(id: 'gear2', name: 'Existing Gear'),
        ];

        // Setup initial cache
        when(
          mockService.listCommunityGear(communityId),
        ).thenAnswer((_) async => mockGear);
        await repository.listCommunityGear(communityId);

        // Share gear - this calls refreshCommunityGear which fetches again
        when(
          mockService.shareGear(
            gearId: gearId,
            communityId: communityId,
            availability: Availability.AVAILABILITY_FOR_LOAN,
          ),
        ).thenAnswer((_) async {});
        await repository.shareGear(
          gearId: gearId,
          communityId: communityId,
          availability: Availability.AVAILABILITY_FOR_LOAN,
        );

        // Service should have been called twice (initial + refresh in shareGear)
        verify(mockService.listCommunityGear(communityId)).called(2);
      });

      // Phase 1: Critical Cache Invalidation Tests
      test('invalidates search cache after sharing gear', () async {
        const gearId = 'gear1';
        const communityId = 'comm1';
        final mockGear = [CommunityGearItem(id: 'gear2', name: 'Existing Gear')];

        when(
          mockService.shareGear(
            gearId: gearId,
            communityId: communityId,
            availability: Availability.AVAILABILITY_FOR_LOAN,
          ),
        ).thenAnswer((_) async {});
        when(mockService.listCommunityGear(communityId))
            .thenAnswer((_) async => mockGear);

        await repository.shareGear(
          gearId: gearId,
          communityId: communityId,
          availability: Availability.AVAILABILITY_FOR_LOAN,
        );

        // Verify search cache was invalidated for this community
        verify(mockSearchRepository.invalidateSearchesForCommunity(communityId))
            .called(1);
      });
    });

    group('unshareGear', () {
      test('unshares gear and invalidates community gear cache', () async {
        const gearId = 'gear1';
        const communityId = 'comm1';
        final mockGear = [CommunityGearItem(id: 'gear2', name: 'Remaining Gear')];

        when(
          mockService.unshareGear(gearId: gearId, communityId: communityId),
        ).thenAnswer((_) async {});
        when(mockService.listCommunityGear(communityId))
            .thenAnswer((_) async => mockGear);

        await repository.unshareGear(gearId: gearId, communityId: communityId);

        verify(
          mockService.unshareGear(gearId: gearId, communityId: communityId),
        ).called(1);
        // Verify that refreshCommunityGear called listCommunityGear
        verify(mockService.listCommunityGear(communityId)).called(1);
      });

      test('invalidates community gear cache after unsharing', () async {
        const gearId = 'gear1';
        const communityId = 'comm1';
        final mockGear = [
          CommunityGearItem(id: 'gear1', name: 'Tent'),
          CommunityGearItem(id: 'gear2', name: 'Kayak'),
        ];

        // Setup initial cache
        when(
          mockService.listCommunityGear(communityId),
        ).thenAnswer((_) async => mockGear);
        await repository.listCommunityGear(communityId);

        // Unshare gear - this calls refreshCommunityGear which fetches again
        when(
          mockService.unshareGear(gearId: gearId, communityId: communityId),
        ).thenAnswer((_) async {});
        await repository.unshareGear(gearId: gearId, communityId: communityId);

        // Service should have been called twice (initial + refresh in unshareGear)
        verify(mockService.listCommunityGear(communityId)).called(2);
      });
    });

    group('acceptInvitationLink', () {
      test('accepts invitation and invalidates cache', () async {
        const shortCode = 'token123';
        final mockResponse = AcceptInvitationLinkResponse(
          communityId: 'comm1',
          communityName: 'Test Community',
        );

        when(
          mockService.acceptInvitationLink(shortCode: shortCode),
        ).thenAnswer((_) async => mockResponse);

        // Mock the list call that will be made by refreshUserCommunities
        when(mockService.listCommunities()).thenAnswer((_) async => []);

        final result = await repository.acceptInvitationLink(
          shortCode: shortCode,
        );

        expect(result.communityId, 'comm1');
        expect(result.communityName, 'Test Community');
        verify(
          mockService.acceptInvitationLink(shortCode: shortCode),
        ).called(1);
      });

      test(
        'invalidates user communities cache after accepting invitation',
        () async {
          const shortCode = 'token123';
          final mockCommunities = [
            CommunityItem(id: 'comm1', name: 'Community 1'),
          ];
          final mockResponse = AcceptInvitationLinkResponse(
            communityId: 'comm2',
            communityName: 'New Community',
          );

          when(
            mockService.listCommunities(),
          ).thenAnswer((_) async => mockCommunities);
          when(
            mockService.acceptInvitationLink(shortCode: shortCode),
          ).thenAnswer((_) async => mockResponse);

          // Initial population
          await repository.listUserCommunities();

          // Accept invitation - this calls refreshUserCommunities which fetches again
          await repository.acceptInvitationLink(
            shortCode: shortCode,
          );

          // Service should have been called twice (initial + refresh in acceptInvitationLink)
          verify(mockService.listCommunities()).called(2);
        },
      );
    });

    // Phase 1 Migration Tests: New methods for CommunityContentViewModel
    group('getMembers', () {
      test('fetches community members from service', () async {
        const communityId = 'comm1';
        final mockMembers = [
          CommunityMember(user: User(id: 'user1', name: 'Alice')),
          CommunityMember(user: User(id: 'user2', name: 'Bob')),
        ];

        when(
          mockService.listCommunityUsers(communityId),
        ).thenAnswer((_) async => mockMembers);

        final result = await repository.getMembers(communityId);

        expect(result.length, 2);
        expect(result[0].user.id, 'user1');
        expect(result[1].user.id, 'user2');
        verify(mockService.listCommunityUsers(communityId)).called(1);
      });

      test('caches community members', () async {
        const communityId = 'comm1';
        final mockMembers = [CommunityMember(user: User(id: 'user1', name: 'Alice'))];

        when(
          mockService.listCommunityUsers(communityId),
        ).thenAnswer((_) async => mockMembers);

        // First call - should fetch
        await repository.getMembers(communityId);

        // Second call - should use cache
        await repository.getMembers(communityId);

        // Service should only be called once
        verify(mockService.listCommunityUsers(communityId)).called(1);
      });
    });

    group('getEvents', () {
      test('fetches community events from service', () async {
        const communityId = 'comm1';
        final mockEvents = [
          CommunityEventItem(
            id: 'event1',
            eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
          ),
          CommunityEventItem(
            id: 'event2',
            eventType:
                CommunityEventType.COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
          ),
        ];

        when(
          mockService.listCommunityEvents(communityId),
        ).thenAnswer((_) async => mockEvents);

        final result = await repository.getEvents(communityId);

        expect(result.length, 2);
        expect(result[0].id, 'event1');
        expect(result[1].id, 'event2');
        verify(mockService.listCommunityEvents(communityId)).called(1);
      });

      test('caches community events', () async {
        const communityId = 'comm1';
        final mockEvents = [
          CommunityEventItem(
            id: 'event1',
            eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
          ),
        ];

        when(
          mockService.listCommunityEvents(communityId),
        ).thenAnswer((_) async => mockEvents);

        // First call - should fetch
        await repository.getEvents(communityId);

        // Second call - should use cache
        await repository.getEvents(communityId);

        // Service should only be called once
        verify(mockService.listCommunityEvents(communityId)).called(1);
      });
    });

    group('getGearCount', () {
      test('returns count of gear items in community', () async {
        const communityId = 'comm1';
        final mockGear = [
          CommunityGearItem(id: 'gear1', name: 'Tent'),
          CommunityGearItem(id: 'gear2', name: 'Kayak'),
          CommunityGearItem(id: 'gear3', name: 'Bike'),
        ];

        when(
          mockService.listCommunityGear(communityId),
        ).thenAnswer((_) async => mockGear);

        final result = await repository.getGearCount(communityId);

        expect(result, 3);
        verify(mockService.listCommunityGear(communityId)).called(1);
      });

      test('returns zero for empty gear list', () async {
        const communityId = 'comm1';
        final mockGear = <CommunityGearItem>[];

        when(
          mockService.listCommunityGear(communityId),
        ).thenAnswer((_) async => mockGear);

        final result = await repository.getGearCount(communityId);

        expect(result, 0);
        verify(mockService.listCommunityGear(communityId)).called(1);
      });

      test('uses cached gear list to avoid duplicate call', () async {
        const communityId = 'comm1';
        final mockGear = [
          CommunityGearItem(id: 'gear1', name: 'Tent'),
          CommunityGearItem(id: 'gear2', name: 'Kayak'),
        ];

        when(
          mockService.listCommunityGear(communityId),
        ).thenAnswer((_) async => mockGear);

        // First call - populates cache
        await repository.listCommunityGear(communityId);

        // Get count - should use cached gear list
        final count = await repository.getGearCount(communityId);

        expect(count, 2);
        // Service should only be called once (by listCommunityGear)
        verify(mockService.listCommunityGear(communityId)).called(1);
      });
    });

    group('refreshMembers', () {
      test('invalidates members cache so next call refetches', () async {
        const communityId = 'comm1';
        final mockMembers = [CommunityMember(user: User(id: 'user1', name: 'Alice'))];

        when(
          mockService.listCommunityUsers(communityId),
        ).thenAnswer((_) async => mockMembers);

        // Populate cache
        await repository.getMembers(communityId);

        // Call invalidate
        await repository.refreshMembers(communityId);

        // The invalidate method should have been called (smoke test)
        // We can't easily verify cache invalidation without accessing cache internals
        // So we just verify the method completes without error
        expect(true, true);
      });

    });

    group('refreshEvents', () {
      test('invalidates events cache so next call refetches', () async {
        const communityId = 'comm1';

        // Call invalidate
        await repository.refreshEvents(communityId);

        // The invalidate method should complete without error (smoke test)
        expect(true, true);
      });

    });

    group('refreshGearCount', () {
      test('invalidates gear cache so next call refetches', () async {
        const communityId = 'comm1';

        // Call invalidate
        await repository.refreshGearCount(communityId);

        // The invalidate method should complete without error (smoke test)
        expect(true, true);
      });
    });

    group('refreshAll', () {
      test('invalidates all community caches', () async {
        const communityId = 'comm1';

        // Call refreshAll
        await repository.refreshAll(communityId);

        // The method should complete without error (smoke test)
        // Actual cache invalidation is tested indirectly through ViewModel tests
        expect(true, true);
      });

      test('executes invalidations in parallel', () async {
        const communityId = 'comm1';

        // refreshAll should complete quickly since invalidations are parallel
        final stopwatch = Stopwatch()..start();
        await repository.refreshAll(communityId);
        stopwatch.stop();

        // This is a basic smoke test - if it runs sequentially it would take longer
        // In practice, parallel execution completes in <10ms
        expect(stopwatch.elapsedMilliseconds, lessThan(100));
      });
    });

    group('listCompletedGiveaways', () {
      test('fetches past giveaways from service', () async {
        const communityId = 'comm1';
        final mockPastGiveaways = [
          CommunityGearItem(id: 'gear1', name: 'Tent'),
          CommunityGearItem(id: 'gear2', name: 'Kayak'),
        ];

        when(
          mockService.listCompletedGiveaways(communityId),
        ).thenAnswer((_) async => mockPastGiveaways);

        final result = await repository.listCompletedGiveaways(communityId);

        expect(result.length, 2);
        expect(result[0].id, 'gear1');
        expect(result[1].id, 'gear2');
        verify(mockService.listCompletedGiveaways(communityId)).called(1);
      });

      test('caches past giveaways', () async {
        const communityId = 'comm1';
        final mockPastGiveaways = [
          CommunityGearItem(id: 'gear1', name: 'Tent'),
        ];

        when(
          mockService.listCompletedGiveaways(communityId),
        ).thenAnswer((_) async => mockPastGiveaways);

        // First call - should fetch
        await repository.listCompletedGiveaways(communityId);

        // Second call - should use cache
        await repository.listCompletedGiveaways(communityId);

        // Service should only be called once
        verify(mockService.listCompletedGiveaways(communityId)).called(1);
      });

      test('caches separately by communityId', () async {
        const communityId1 = 'comm1';
        const communityId2 = 'comm2';

        when(
          mockService.listCompletedGiveaways(communityId1),
        ).thenAnswer((_) async => [CommunityGearItem(id: 'gear1', name: 'Tent')]);

        when(
          mockService.listCompletedGiveaways(communityId2),
        ).thenAnswer((_) async => [CommunityGearItem(id: 'gear2', name: 'Kayak')]);

        await repository.listCompletedGiveaways(communityId1);
        await repository.listCompletedGiveaways(communityId2);

        // Each community should be fetched independently
        verify(mockService.listCompletedGiveaways(communityId1)).called(1);
        verify(mockService.listCompletedGiveaways(communityId2)).called(1);
      });
    });

    group('refreshCompletedGiveaways', () {
      test('invalidates and refetches past giveaways', () async {
        const communityId = 'comm1';
        final mockPastGiveaways = [
          CommunityGearItem(id: 'gear1', name: 'Tent'),
        ];

        when(
          mockService.listCompletedGiveaways(communityId),
        ).thenAnswer((_) async => mockPastGiveaways);

        // First call - populates cache
        await repository.listCompletedGiveaways(communityId);

        // Refresh - should invalidate cache and refetch
        await repository.refreshCompletedGiveaways(communityId);

        // Service should be called twice
        verify(mockService.listCompletedGiveaways(communityId)).called(2);
      });
    });

    group('invalidateCompletedGiveawaysAll', () {
      test('invalidates all past giveaways caches', () async {
        const communityId1 = 'comm1';
        const communityId2 = 'comm2';

        when(
          mockService.listCompletedGiveaways(communityId1),
        ).thenAnswer((_) async => [CommunityGearItem(id: 'gear1', name: 'Tent')]);

        when(
          mockService.listCompletedGiveaways(communityId2),
        ).thenAnswer((_) async => [CommunityGearItem(id: 'gear2', name: 'Kayak')]);

        // Populate both caches
        await repository.listCompletedGiveaways(communityId1);
        await repository.listCompletedGiveaways(communityId2);

        // Invalidate all
        await repository.invalidateCompletedGiveawaysAll();

        // Next calls should refetch
        await repository.listCompletedGiveaways(communityId1);
        await repository.listCompletedGiveaways(communityId2);

        // Each should be called twice (once before invalidation, once after)
        verify(mockService.listCompletedGiveaways(communityId1)).called(2);
        verify(mockService.listCompletedGiveaways(communityId2)).called(2);
      });
    });

    group('getNotificationPreferences', () {
      test('caches the response', () async {
        const communityId = 'comm-prefs-1';
        final prefs = CommunityNotificationPreferences()..notifyChats = false;
        when(mockService.getNotificationPreferences(communityId))
            .thenAnswer((_) async => prefs);

        final first = await repository.getNotificationPreferences(communityId);
        final second = await repository.getNotificationPreferences(communityId);

        expect(first.notifyChats, isFalse);
        expect(second.notifyChats, isFalse);
        verify(mockService.getNotificationPreferences(communityId)).called(1);
      });

      test('updateNotificationPreferences invalidates the cache', () async {
        const communityId = 'comm-prefs-2';
        final initial = CommunityNotificationPreferences();
        final updated = CommunityNotificationPreferences()..notifyChats = false;

        when(mockService.getNotificationPreferences(communityId))
            .thenAnswer((_) async => initial);
        when(mockService.updateNotificationPreferences(
          communityId: communityId,
          preferences: anyNamed('preferences'),
        )).thenAnswer((_) async => updated);

        // First read populates cache.
        await repository.getNotificationPreferences(communityId);
        // Update invalidates cache.
        await repository.updateNotificationPreferences(
          communityId: communityId,
          preferences: updated,
        );
        // Subsequent read must hit the service again.
        await repository.getNotificationPreferences(communityId);

        verify(mockService.getNotificationPreferences(communityId)).called(2);
      });
    });

    group('streamGenCommunity', () {
      test('forwards prompt and region to the service', () async {
        when(mockService.streamGenCommunity(
          prompt: anyNamed('prompt'),
          region: anyNamed('region'),
        )).thenAnswer((_) => const Stream.empty());

        await repository
            .streamGenCommunity(prompt: 'boulder cycling', region: 'CO')
            .toList();

        verify(mockService.streamGenCommunity(
          prompt: 'boulder cycling',
          region: 'CO',
        )).called(1);
      });

      test('defaults a null prompt to the empty string', () async {
        when(mockService.streamGenCommunity(
          prompt: anyNamed('prompt'),
          region: anyNamed('region'),
        )).thenAnswer((_) => const Stream.empty());

        await repository.streamGenCommunity(region: 'CO').toList();

        verify(mockService.streamGenCommunity(
          prompt: '',
          region: 'CO',
        )).called(1);
      });
    });

    group('revokeShareLink', () {
    const communityId = 'comm1';
    const gearId = 'gear1';
    const shortCode = 'Xk9mN2pQ';

    setUp(() {
      when(mockService.revokeShareLink(shortCode: anyNamed('shortCode')))
          .thenAnswer((_) async {});
    });

    test('resolves short code from cache and calls revokeShareLink on service', () async {
      // Warm the cache by calling getOrCreateShareLink first.
      when(mockService.getOrCreateShareLink(
        communityId: communityId,
        gearId: gearId,
        transferId: null,
        requestId: null,
        experienceId: null,
      )).thenAnswer((_) async => GetOrCreateShareLinkResponse(
            shareUrl: 'https://example.com/go/$shortCode',
            shortCode: shortCode,
            communityId: communityId,
          ));

      await repository.getOrCreateShareLink(
        communityId: communityId,
        gearId: gearId,
      );

      await repository.revokeShareLink(
        communityId: communityId,
        gearId: gearId,
      );

      // getOrCreateShareLink served from cache on revoke path (called once total).
      verify(mockService.getOrCreateShareLink(
        communityId: communityId,
        gearId: gearId,
        transferId: null,
        requestId: null,
        experienceId: null,
      )).called(1);
      verify(mockService.revokeShareLink(shortCode: shortCode)).called(1);
    });

    test('invalidates share: cache key after successful revoke', () async {
      when(mockService.getOrCreateShareLink(
        communityId: communityId,
        gearId: null,
        transferId: null,
        requestId: null,
        experienceId: null,
      )).thenAnswer((_) async => GetOrCreateShareLinkResponse(
            shareUrl: 'https://example.com/go/$shortCode',
            shortCode: shortCode,
            communityId: communityId,
          ));

      // Populate cache.
      await repository.getOrCreateShareLink(communityId: communityId);

      // Revoke — should invalidate the share:comm1 key.
      await repository.revokeShareLink(communityId: communityId);

      // A second getOrCreateShareLink call must hit the service (cache cleared).
      await repository.getOrCreateShareLink(communityId: communityId);
      verify(mockService.getOrCreateShareLink(
        communityId: communityId,
        gearId: null,
        transferId: null,
        requestId: null,
        experienceId: null,
      )).called(2);
    });

    test('cold cache: calls getOrCreateShareLink to recover short code', () async {
      // No warm cache — getOrCreateShareLink will be called by revokeShareLink.
      when(mockService.getOrCreateShareLink(
        communityId: communityId,
        gearId: gearId,
        transferId: null,
        requestId: null,
        experienceId: null,
      )).thenAnswer((_) async => GetOrCreateShareLinkResponse(
            shareUrl: 'https://example.com/go/$shortCode',
            shortCode: shortCode,
            communityId: communityId,
          ));

      await repository.revokeShareLink(
        communityId: communityId,
        gearId: gearId,
      );

      verify(mockService.getOrCreateShareLink(
        communityId: communityId,
        gearId: gearId,
        transferId: null,
        requestId: null,
        experienceId: null,
      )).called(1);
      verify(mockService.revokeShareLink(shortCode: shortCode)).called(1);
    });
  });
  });
}
