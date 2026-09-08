import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/profile_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/gear_service.dart';

import 'gear_repository_test.mocks.dart';

@GenerateMocks([GearService, CommunityService, SearchRepository, FeedRepository, ChatRepository, CommunityRepository, ProfileRepository])
void main() {
  group('GearRepository', () {
    late GearRepository repository;
    late MockGearService mockService;
    late MockCommunityService mockCommunityService;
    late MockSearchRepository mockSearchRepository;
    late MockFeedRepository mockFeedRepository;
    late MockChatRepository mockChatRepository;
    late MockCommunityRepository mockCommunityRepository;
    late MockProfileRepository mockProfileRepository;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockGearService();
      mockCommunityService = MockCommunityService();
      mockSearchRepository = MockSearchRepository();
      mockFeedRepository = MockFeedRepository();
      mockChatRepository = MockChatRepository();
      mockCommunityRepository = MockCommunityRepository();
      mockProfileRepository = MockProfileRepository();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      when(mockCommunityRepository.invalidateAllCommunityGearLists())
          .thenAnswer((_) async {});
      when(mockProfileRepository.invalidateAll()).thenAnswer((_) async {});
      repository = GearRepository(
        cacheManager,
        mockService,
        mockCommunityService,
        mockSearchRepository,
        mockFeedRepository,
        mockChatRepository,
        mockCommunityRepository,
        onProfileInvalidated: () => mockProfileRepository.invalidateAll(),
      );
    });

    test('fetchById calls gear service', () async {
      const gearId = 'gear123';
      final mockGear = GetGearResponse(
        id: gearId,
        name: 'Test Gear',
        description: 'Test Description',
      );

      when(mockService.getGear(gearId)).thenAnswer((_) async => mockGear);

      final result = await repository.get(gearId);

      expect(result.id, gearId);
      expect(result.name, 'Test Gear');
      verify(mockService.getGear(gearId)).called(1);
    });

    test('getById caches gear data', () async {
      const gearId = 'gear123';
      final mockGear = GetGearResponse(
        id: gearId,
        name: 'Test Gear',
      );

      when(mockService.getGear(gearId)).thenAnswer((_) async => mockGear);

      // First call - should fetch
      await repository.get(gearId);

      // Second call - should use cache
      await repository.get(gearId);

      // Service should only be called once
      verify(mockService.getGear(gearId)).called(1);
    });

    test('getGearDetails wraps getById', () async {
      const gearId = 'gear123';
      final mockGear = GetGearResponse(
        id: gearId,
        name: 'Test Gear',
      );

      when(mockService.getGear(gearId)).thenAnswer((_) async => mockGear);

      final result = await repository.getGearDetails(gearId);

      expect(result.id, gearId);
      expect(result.name, 'Test Gear');
      verify(mockService.getGear(gearId)).called(1);
    });

    test('listUserGear fetches gear list', () async {
      final mockItems = [
        GearItem(id: 'gear1', name: 'Gear 1'),
        GearItem(id: 'gear2', name: 'Gear 2'),
        GearItem(id: 'gear3', name: 'Gear 3'),
      ];

      when(mockService.listGear()).thenAnswer((_) async => mockItems);

      final result = await repository.listUserGear();

      expect(result.length, 3);
      expect(result[0].id, 'gear1');
      expect(result[1].id, 'gear2');
      expect(result[2].id, 'gear3');
      verify(mockService.listGear()).called(1);
    });

    test('listUserGear caches result', () async {
      final mockItems = [
        GearItem(id: 'gear1', name: 'Gear 1'),
      ];

      when(mockService.listGear()).thenAnswer((_) async => mockItems);

      // First call - should fetch
      await repository.listUserGear();

      // Second call - should use cache
      await repository.listUserGear();

      // Service should only be called once
      verify(mockService.listGear()).called(1);
    });

    test('refreshUserGear forces re-fetch on next call', () async {
      final mockItems = [
        GearItem(id: 'gear1', name: 'Gear 1'),
      ];

      when(mockService.listGear()).thenAnswer((_) async => mockItems);

      // First call
      await repository.listUserGear();

      // Refresh
      await repository.refreshUserGear();

      // Second call should fetch again
      await repository.listUserGear();

      // Service should be called twice (once before refresh, once after)
      verify(mockService.listGear()).called(2);
    });

    test('handles service errors', () {
      const gearId = 'gear123';

      when(mockService.getGear(gearId)).thenThrow(Exception('Network error'));

      expect(
        () => repository.get(gearId),
        throwsException,
      );
    });

    test('handles list service errors', () {
      when(mockService.listGear()).thenThrow(Exception('Network error'));

      expect(
        () => repository.listUserGear(),
        throwsException,
      );
    });

    // Phase 1.1: Update Operation Cache Invalidation Tests
    group('saveGear', () {
      test('invalidates search cache after saving gear', () async {
        const gearId = 'gear123';
        const name = 'Updated Gear Name';
        final mockItems = [GearItem(id: gearId, name: name)];

        when(
          mockService.saveGear(
            id: gearId,
            name: name,
          ),
        ).thenAnswer((_) async => gearId);
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.saveGear(id: gearId, name: name);

        // Verify search cache was invalidated
        verify(mockSearchRepository.invalidateSearches()).called(1);
      });

      test('invalidates gear cache and refreshes user gear list', () async {
        const gearId = 'gear123';
        final mockGear = GetGearResponse(id: gearId, name: 'Original Name');
        final mockItems = [GearItem(id: gearId, name: 'Updated Name')];

        when(mockService.getGear(gearId)).thenAnswer((_) async => mockGear);
        when(mockService.listGear()).thenAnswer((_) async => mockItems);
        when(
          mockService.saveGear(id: gearId, name: 'Updated Name'),
        ).thenAnswer((_) async => gearId);

        // Populate cache
        await repository.get(gearId);
        await repository.listUserGear();

        // Save gear
        await repository.saveGear(id: gearId, name: 'Updated Name');

        // Next get should re-fetch (cache was invalidated)
        await repository.get(gearId);

        // getGear should be called twice (initial + after save)
        verify(mockService.getGear(gearId)).called(2);
        // listGear should be called twice (initial + refreshUserGear in saveGear)
        verify(mockService.listGear()).called(2);
      });
    });

    // Phase 4: Media Reordering Tests
    group('updateMediaOrder', () {
      test('fetches current gear and calls saveGear with reordered mediaIds', () async {
        const gearId = 'gear123';
        final mockGear = GetGearResponse(
          id: gearId,
          name: 'Test Gear',
          description: 'Test Description',
          mediaIds: ['media1', 'media2', 'media3'],
          locationId: 'loc123',
        );
        final reorderedMediaIds = ['media3', 'media1', 'media2'];
        final mockItems = [GearItem(id: gearId, name: 'Test Gear')];

        when(mockService.getGear(gearId)).thenAnswer((_) async => mockGear);
        when(
          mockService.saveGear(
            id: gearId,
            name: 'Test Gear',
            description: 'Test Description',
            mediaIds: reorderedMediaIds,
            locationId: 'loc123',
          ),
        ).thenAnswer((_) async => gearId);
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.updateMediaOrder(gearId, reorderedMediaIds);

        // Verify we fetched the current gear
        verify(mockService.getGear(gearId)).called(1);
        // Verify we called saveGear with the reordered media IDs
        verify(
          mockService.saveGear(
            id: gearId,
            name: 'Test Gear',
            description: 'Test Description',
            mediaIds: reorderedMediaIds,
            locationId: 'loc123',
          ),
        ).called(1);
      });

      test('preserves all gear fields when reordering media', () async {
        const gearId = 'gear123';
        final mockGear = GetGearResponse(
          id: gearId,
          name: 'Special Gear',
          description: 'Important Description',
          mediaIds: ['media1', 'media2'],
          locationId: 'special-location',
        );
        final reorderedMediaIds = ['media2', 'media1'];
        final mockItems = [GearItem(id: gearId, name: 'Special Gear')];

        when(mockService.getGear(gearId)).thenAnswer((_) async => mockGear);
        when(
          mockService.saveGear(
            id: gearId,
            name: 'Special Gear',
            description: 'Important Description',
            mediaIds: reorderedMediaIds,
            locationId: 'special-location',
          ),
        ).thenAnswer((_) async => gearId);
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.updateMediaOrder(gearId, reorderedMediaIds);

        // Verify all fields were preserved
        verify(
          mockService.saveGear(
            id: gearId,
            name: 'Special Gear',
            description: 'Important Description',
            mediaIds: reorderedMediaIds,
            locationId: 'special-location',
          ),
        ).called(1);
      });

      test('handles empty locationId correctly', () async {
        const gearId = 'gear123';
        final mockGear = GetGearResponse(
          id: gearId,
          name: 'Test Gear',
          description: 'Test Description',
          mediaIds: ['media1', 'media2'],
          locationId: '', // Empty location
        );
        final reorderedMediaIds = ['media2', 'media1'];
        final mockItems = [GearItem(id: gearId, name: 'Test Gear')];

        when(mockService.getGear(gearId)).thenAnswer((_) async => mockGear);
        when(
          mockService.saveGear(
            id: gearId,
            name: 'Test Gear',
            description: 'Test Description',
            mediaIds: reorderedMediaIds,
            locationId: null,
          ),
        ).thenAnswer((_) async => gearId);
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.updateMediaOrder(gearId, reorderedMediaIds);

        // Verify locationId is passed as null when empty
        verify(
          mockService.saveGear(
            id: gearId,
            name: 'Test Gear',
            description: 'Test Description',
            mediaIds: reorderedMediaIds,
            locationId: null,
          ),
        ).called(1);
      });

      test('invalidates search cache after reordering media', () async {
        const gearId = 'gear123';
        final mockGear = GetGearResponse(
          id: gearId,
          name: 'Test Gear',
          description: 'Test Description',
          mediaIds: ['media1', 'media2'],
        );
        final reorderedMediaIds = ['media2', 'media1'];
        final mockItems = [GearItem(id: gearId, name: 'Test Gear')];

        when(mockService.getGear(gearId)).thenAnswer((_) async => mockGear);
        when(
          mockService.saveGear(
            id: gearId,
            name: 'Test Gear',
            description: 'Test Description',
            mediaIds: reorderedMediaIds,
            locationId: null,
          ),
        ).thenAnswer((_) async => gearId);
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.updateMediaOrder(gearId, reorderedMediaIds);

        // Verify search cache was invalidated (happens in saveGear)
        verify(mockSearchRepository.invalidateSearches()).called(1);
      });
    });

    // Delete Operation Cache Invalidation Tests
    group('deleteGear', () {
      test('deletes gear', () async {
        const gearId = 'gear123';
        final mockItems = <GearItem>[];

        when(mockService.deleteGear(gearId)).thenAnswer((_) async {});
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.deleteGear(gearId);

        verify(mockService.deleteGear(gearId)).called(1);
      });

      test('invalidates feed cache after deleting gear', () async {
        const gearId = 'gear123';
        final mockItems = <GearItem>[];

        when(mockService.deleteGear(gearId)).thenAnswer((_) async {});
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.deleteGear(gearId);

        // Verify all feed caches were invalidated
        verify(mockFeedRepository.invalidateAllFeeds()).called(1);
      });

      test('invalidates search cache after deleting gear', () async {
        const gearId = 'gear123';
        final mockItems = <GearItem>[];

        when(mockService.deleteGear(gearId)).thenAnswer((_) async {});
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.deleteGear(gearId);

        // Verify search cache was invalidated
        verify(mockSearchRepository.invalidateSearches()).called(1);
      });

      test('invalidates gear cache after deleting', () async {
        const gearId = 'gear123';
        final mockGear = GetGearResponse(
          id: gearId,
          name: 'Test Gear',
          description: 'Test description',
        );
        final mockItems = <GearItem>[];

        when(mockService.getGear(gearId)).thenAnswer((_) async => mockGear);
        when(mockService.deleteGear(gearId)).thenAnswer((_) async {});
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        // Populate cache
        await repository.get(gearId);

        // Delete gear
        await repository.deleteGear(gearId);

        // Next get should re-fetch (cache was invalidated)
        await repository.get(gearId);

        // getGear should be called twice (initial + after delete)
        verify(mockService.getGear(gearId)).called(2);
      });

      test('invalidates conversation cache after deleting (cascade deletion)',
          () async {
        const gearId = 'gear123';
        final mockItems = <GearItem>[];

        when(mockService.deleteGear(gearId)).thenAnswer((_) async {});
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.deleteGear(gearId);

        // Verify conversations cache was refreshed (cascade deletion removes conversation)
        verify(mockChatRepository.refreshConversations()).called(1);
      });

      test('invalidates all community gear lists after deleting', () async {
        const gearId = 'gear123';
        final mockItems = <GearItem>[];

        when(mockService.deleteGear(gearId)).thenAnswer((_) async {});
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.deleteGear(gearId);

        verify(mockCommunityRepository.invalidateAllCommunityGearLists())
            .called(1);
      });

      test('invalidates profile cache after deleting', () async {
        const gearId = 'gear123';
        final mockItems = <GearItem>[];

        when(mockService.deleteGear(gearId)).thenAnswer((_) async {});
        when(mockService.listGear()).thenAnswer((_) async => mockItems);

        await repository.deleteGear(gearId);

        verify(mockProfileRepository.invalidateAll()).called(1);
      });
    });

  });
}
