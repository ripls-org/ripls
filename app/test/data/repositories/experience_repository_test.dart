import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/profile_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/services/experience_service.dart';

import 'experience_repository_test.mocks.dart';

@GenerateMocks([ExperienceService, FeedRepository, ChatRepository, SearchRepository, ProfileRepository])
void main() {
  group('ExperienceRepository', () {
    late ExperienceRepository repository;
    late MockExperienceService mockService;
    late MockFeedRepository mockFeedRepository;
    late MockChatRepository mockChatRepository;
    late MockSearchRepository mockSearchRepository;
    late MockProfileRepository mockProfileRepository;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockExperienceService();
      mockFeedRepository = MockFeedRepository();
      mockChatRepository = MockChatRepository();
      mockSearchRepository = MockSearchRepository();
      mockProfileRepository = MockProfileRepository();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      when(mockProfileRepository.invalidateAll()).thenAnswer((_) async {});
      repository = ExperienceRepository(
        cacheManager,
        mockService,
        mockFeedRepository,
        mockChatRepository,
        mockSearchRepository,
        onProfileInvalidated: () => mockProfileRepository.invalidateAll(),
      );
    });

    test('get fetches experience details', () async {
      const experienceId = 'exp123';
      final mockExperience = GetExperienceResponse(
        experience: Experience(
          id: experienceId,
          name: 'Test Experience',
          description: 'Test Description',
        ),
      );

      when(mockService.getExperience(experienceId))
          .thenAnswer((_) async => mockExperience);

      final result = await repository.get(experienceId);

      expect(result.experience.id, experienceId);
      expect(result.experience.name, 'Test Experience');
      verify(mockService.getExperience(experienceId)).called(1);
    });

    test('get caches experience data', () async {
      const experienceId = 'exp123';
      final mockExperience = GetExperienceResponse(
        experience: Experience(
          id: experienceId,
          name: 'Test Experience',
        ),
      );

      when(mockService.getExperience(experienceId))
          .thenAnswer((_) async => mockExperience);

      // First call - should fetch
      await repository.get(experienceId);

      // Second call - should use cache
      await repository.get(experienceId);

      // Service should only be called once
      verify(mockService.getExperience(experienceId)).called(1);
    });

    test('listCommunityExperiences fetches experience list', () async {
      const communityId = 'comm123';
      final mockItems = [
        Experience(id: 'exp1', name: 'Experience 1'),
        Experience(id: 'exp2', name: 'Experience 2'),
        Experience(id: 'exp3', name: 'Experience 3'),
      ];

      when(mockService.listExperiences(communityId))
          .thenAnswer((_) async => mockItems);

      final result = await repository.listCommunityExperiences(communityId);

      expect(result.length, 3);
      expect(result[0].id, 'exp1');
      expect(result[1].id, 'exp2');
      expect(result[2].id, 'exp3');
      verify(mockService.listExperiences(communityId)).called(1);
    });

    test('listCommunityExperiences caches result', () async {
      const communityId = 'comm123';
      final mockItems = [
        Experience(id: 'exp1', name: 'Experience 1'),
      ];

      when(mockService.listExperiences(communityId))
          .thenAnswer((_) async => mockItems);

      // First call - should fetch
      await repository.listCommunityExperiences(communityId);

      // Second call - should use cache
      await repository.listCommunityExperiences(communityId);

      // Service should only be called once
      verify(mockService.listExperiences(communityId)).called(1);
    });

    test('refreshCommunityExperiences forces re-fetch on next call', () async {
      const communityId = 'comm123';
      final mockItems = [
        Experience(id: 'exp1', name: 'Experience 1'),
      ];

      when(mockService.listExperiences(communityId))
          .thenAnswer((_) async => mockItems);

      // First call
      await repository.listCommunityExperiences(communityId);

      // Refresh
      await repository.refreshCommunityExperiences(communityId);

      // Second call should fetch again
      await repository.listCommunityExperiences(communityId);

      // Service should be called twice (once before refresh, once after)
      verify(mockService.listExperiences(communityId)).called(2);
    });

    test('listMyExperiences fetches user experience list', () async {
      final mockItems = [
        Experience(id: 'exp1', name: 'My Experience 1'),
        Experience(id: 'exp2', name: 'My Experience 2'),
      ];

      when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

      final result = await repository.listMyExperiences();

      expect(result.length, 2);
      expect(result[0].id, 'exp1');
      expect(result[1].id, 'exp2');
      verify(mockService.listMyExperiences()).called(1);
    });

    test('listMyExperiences caches result', () async {
      final mockItems = [
        Experience(id: 'exp1', name: 'My Experience 1'),
      ];

      when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

      // First call - should fetch
      await repository.listMyExperiences();

      // Second call - should use cache
      await repository.listMyExperiences();

      // Service should only be called once
      verify(mockService.listMyExperiences()).called(1);
    });

    test('refreshMyExperiences forces re-fetch on next call', () async {
      final mockItems = [
        Experience(id: 'exp1', name: 'My Experience 1'),
      ];

      when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

      // First call
      await repository.listMyExperiences();

      // Refresh
      await repository.refreshMyExperiences();

      // Second call should fetch again
      await repository.listMyExperiences();

      // Service should be called twice (once before refresh, once after)
      verify(mockService.listMyExperiences()).called(2);
    });

    test('createExperience calls service and invalidates cache', () async {
      const experienceId = 'exp123';
      final mockItems = [
        Experience(id: 'exp1', name: 'Old Experience'),
      ];

      when(mockService.saveExperience(
        name: 'New Experience',
        description: 'New Description',
      )).thenAnswer((_) async => experienceId);
      when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

      final result = await repository.createExperience(
        name: 'New Experience',
        description: 'New Description',
      );

      expect(result, experienceId);
      verify(mockService.saveExperience(
        name: 'New Experience',
        description: 'New Description',
      )).called(1);
    });

    test('rsvp calls service and invalidates experience cache', () async {
      const experienceId = 'exp123';
      const communityId = 'comm123';

      when(mockService.rsvp(
        experienceId: experienceId,
        communityId: communityId,
        intention: RSVPIntention.RSVP_INTENTION_YES,
      )).thenAnswer((_) async => Future.value());

      await repository.rsvp(
        experienceId: experienceId,
        communityId: communityId,
        intention: RSVPIntention.RSVP_INTENTION_YES,
      );

      verify(mockService.rsvp(
        experienceId: experienceId,
        communityId: communityId,
        intention: RSVPIntention.RSVP_INTENTION_YES,
      )).called(1);
    });

    // Delete Operation Cache Invalidation Tests
    group('deleteExperience', () {
      test('deletes experience', () async {
        const experienceId = 'exp123';
        final mockItems = [
          Experience(id: 'exp1', name: 'Remaining Experience'),
        ];

        when(mockService.deleteExperience(experienceId))
            .thenAnswer((_) async => Future.value());
        when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

        await repository.deleteExperience(experienceId);

        verify(mockService.deleteExperience(experienceId)).called(1);
      });

      test('invalidates feed cache after deleting experience', () async {
        const experienceId = 'exp123';
        final mockItems = <Experience>[];

        when(mockService.deleteExperience(experienceId))
            .thenAnswer((_) async => Future.value());
        when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

        await repository.deleteExperience(experienceId);

        // Verify all feed caches were invalidated
        verify(mockFeedRepository.invalidateAllFeeds()).called(1);
      });

      test('invalidates search cache after deleting experience', () async {
        const experienceId = 'exp123';
        final mockItems = <Experience>[];

        when(mockService.deleteExperience(experienceId))
            .thenAnswer((_) async => Future.value());
        when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

        await repository.deleteExperience(experienceId);

        // Verify search cache was invalidated
        verify(mockSearchRepository.invalidateSearches()).called(1);
      });

      test('invalidates experience cache after deleting', () async {
        const experienceId = 'exp123';
        final mockExperience = GetExperienceResponse(
          experience: Experience(
            id: experienceId,
            name: 'Test Experience',
            description: 'Test description',
          ),
        );
        final mockItems = <Experience>[];

        when(mockService.getExperience(experienceId))
            .thenAnswer((_) async => mockExperience);
        when(mockService.deleteExperience(experienceId))
            .thenAnswer((_) async => Future.value());
        when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

        // Populate cache
        await repository.get(experienceId);

        // Delete experience
        await repository.deleteExperience(experienceId);

        // Next get should re-fetch (cache was invalidated)
        await repository.get(experienceId);

        // getExperience should be called twice (initial + after delete)
        verify(mockService.getExperience(experienceId)).called(2);
      });

      test('invalidates conversation cache after deleting (cascade deletion)',
          () async {
        const experienceId = 'exp123';
        final mockItems = <Experience>[];

        when(mockService.deleteExperience(experienceId))
            .thenAnswer((_) async => Future.value());
        when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

        await repository.deleteExperience(experienceId);

        // Verify conversations cache was refreshed (cascade deletion removes conversation)
        verify(mockChatRepository.refreshConversations()).called(1);
      });

      test('invalidates profile cache after deleting experience', () async {
        const experienceId = 'exp123';
        final mockItems = <Experience>[];

        when(mockService.deleteExperience(experienceId))
            .thenAnswer((_) async => Future.value());
        when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

        await repository.deleteExperience(experienceId);

        verify(mockProfileRepository.invalidateAll()).called(1);
      });
    });

    // Phase 1: Critical Cache Invalidation Tests
    group('shareExperience', () {
      test('invalidates search cache after sharing experience', () async {
        const experienceId = 'exp123';
        const communityId = 'comm123';
        final mockItems = [Experience(id: experienceId, name: 'Shared Experience')];

        when(mockService.shareExperience(
          experienceId: experienceId,
          communityId: communityId,
        )).thenAnswer((_) async => Future.value());
        when(mockService.listExperiences(communityId))
            .thenAnswer((_) async => mockItems);

        await repository.shareExperience(
          experienceId: experienceId,
          communityId: communityId,
        );

        // Verify search cache was invalidated for this community
        verify(mockSearchRepository.invalidateSearchesForCommunity(communityId))
            .called(1);
      });

      test('invalidates feed cache after sharing experience', () async {
        const experienceId = 'exp123';
        const communityId = 'comm123';
        final mockItems = [Experience(id: experienceId, name: 'Shared Experience')];

        when(mockService.shareExperience(
          experienceId: experienceId,
          communityId: communityId,
        )).thenAnswer((_) async => Future.value());
        when(mockService.listExperiences(communityId))
            .thenAnswer((_) async => mockItems);

        await repository.shareExperience(
          experienceId: experienceId,
          communityId: communityId,
        );

        // Verify feed cache was invalidated
        verify(mockFeedRepository.invalidateFeed()).called(1);
      });
    });

    // Phase 1.1: Update Operation Cache Invalidation Tests
    group('saveExperience', () {
      test('invalidates search cache after saving experience', () async {
        const experienceId = 'exp123';
        const name = 'Updated Experience Name';
        final mockItems = [Experience(id: experienceId, name: name)];

        when(
          mockService.saveExperience(
            id: experienceId,
            name: name,
          ),
        ).thenAnswer((_) async => experienceId);
        when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);

        await repository.saveExperience(id: experienceId, name: name);

        // Verify search cache was invalidated
        verify(mockSearchRepository.invalidateSearches()).called(1);
      });

      test('invalidates experience cache and refreshes user experience list', () async {
        const experienceId = 'exp123';
        final mockExperience = GetExperienceResponse(
          experience: Experience(id: experienceId, name: 'Original Name'),
        );
        final mockItems = [Experience(id: experienceId, name: 'Updated Name')];

        when(mockService.getExperience(experienceId))
            .thenAnswer((_) async => mockExperience);
        when(mockService.listMyExperiences()).thenAnswer((_) async => mockItems);
        when(
          mockService.saveExperience(id: experienceId, name: 'Updated Name'),
        ).thenAnswer((_) async => experienceId);

        // Populate cache
        await repository.get(experienceId);
        await repository.listMyExperiences();

        // Save experience
        await repository.saveExperience(id: experienceId, name: 'Updated Name');

        // Next get should re-fetch (cache was invalidated)
        await repository.get(experienceId);

        // getExperience should be called twice (initial + after save)
        verify(mockService.getExperience(experienceId)).called(2);
        // listMyExperiences should be called twice (initial + refreshMyExperiences in saveExperience)
        verify(mockService.listMyExperiences()).called(2);
      });
    });

    test('handles service errors', () {
      const experienceId = 'exp123';

      when(mockService.getExperience(experienceId))
          .thenThrow(Exception('Network error'));

      expect(
        () => repository.get(experienceId),
        throwsException,
      );
    });

    test('handles list service errors', () {
      when(mockService.listMyExperiences())
          .thenThrow(Exception('Network error'));

      expect(
        () => repository.listMyExperiences(),
        throwsException,
      );
    });
  });
}
