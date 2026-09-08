import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/services/feed_service.dart';
import 'package:ripls/services/user_service.dart';

import 'story_repository_test.mocks.dart';

@GenerateMocks([MediaRepository, FeedService, UserService, CacheManager])
void main() {
  group('StoryRepository - Media Loading', () {
    late StoryRepository repository;
    late MockMediaRepository mockMediaRepository;
    late MockFeedService mockFeedService;
    late MockUserService mockUserService;
    late MockCacheManager mockCacheManager;

    setUp(() {
      mockMediaRepository = MockMediaRepository();
      mockFeedService = MockFeedService();
      mockUserService = MockUserService();
      mockCacheManager = MockCacheManager();
      repository = StoryRepository(
        mockCacheManager,
        mockFeedService,
        mockUserService,
        mockMediaRepository,
      );
    });

    test('loadMediaUrls returns empty list when story has no media', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test Story',
        description: 'Test description',
      );

      final result = await repository.loadMediaUrls(story);

      expect(result, isEmpty);
      verifyNever(mockMediaRepository.getMediaUrl(any));
    });

    test('loadMediaUrls loads all media URLs for story', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test Story',
        description: 'Test description',
        mediaIds: ['media1', 'media2', 'media3'],
      );

      final mediaUrl1 = MediaUrl(
        url: 'https://example.com/media1.jpg',
        isThumbnail: true,
        mediaId: 'media1',
      );
      final mediaUrl2 = MediaUrl(
        url: 'https://example.com/media2.jpg',
        isThumbnail: true,
        mediaId: 'media2',
      );
      final mediaUrl3 = MediaUrl(
        url: 'https://example.com/media3.jpg',
        isThumbnail: false,
        mediaId: 'media3',
      );

      when(
        mockMediaRepository.getMediaUrl('media1'),
      ).thenAnswer((_) async => mediaUrl1);
      when(
        mockMediaRepository.getMediaUrl('media2'),
      ).thenAnswer((_) async => mediaUrl2);
      when(
        mockMediaRepository.getMediaUrl('media3'),
      ).thenAnswer((_) async => mediaUrl3);

      final result = await repository.loadMediaUrls(story);

      expect(result, hasLength(3));
      expect(result[0].mediaId, 'media1');
      expect(result[1].mediaId, 'media2');
      expect(result[2].mediaId, 'media3');

      verify(mockMediaRepository.getMediaUrl('media1')).called(1);
      verify(mockMediaRepository.getMediaUrl('media2')).called(1);
      verify(mockMediaRepository.getMediaUrl('media3')).called(1);
    });

    test('loadPrimaryMediaUrl returns null when story has no media', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test Story',
        description: 'Test description',
      );

      final result = await repository.loadPrimaryMediaUrl(story);

      expect(result, isNull);
      verifyNever(mockMediaRepository.getMediaUrl(any));
    });

    test('loadPrimaryMediaUrl returns first media URL', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test Story',
        description: 'Test description',
        mediaIds: ['media1', 'media2'],
      );

      final mediaUrl1 = MediaUrl(
        url: 'https://example.com/media1.jpg',
        isThumbnail: true,
        mediaId: 'media1',
      );

      when(
        mockMediaRepository.getMediaUrl('media1'),
      ).thenAnswer((_) async => mediaUrl1);

      final result = await repository.loadPrimaryMediaUrl(story);

      expect(result, isNotNull);
      expect(result!.mediaId, 'media1');
      expect(result.url, 'https://example.com/media1.jpg');

      verify(mockMediaRepository.getMediaUrl('media1')).called(1);
      verifyNever(mockMediaRepository.getMediaUrl('media2'));
    });
  });

  group('StoryRepository - Story Listing', () {
    late StoryRepository repository;
    late MockFeedService mockFeedService;
    late MockCacheManager mockCacheManager;
    late MockMediaRepository mockMediaRepository;

    setUp(() {
      mockFeedService = MockFeedService();
      mockCacheManager = MockCacheManager();
      mockMediaRepository = MockMediaRepository();
      final mockUserService = MockUserService();
      repository = StoryRepository(
        mockCacheManager,
        mockFeedService,
        mockUserService,
        mockMediaRepository,
      );
    });

    test('listByCommunity fetches stories from service', () async {
      final communityId = 'community123';
      final mockStories = [
        StoryPayload(
          storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
          title: 'Story 1',
          description: 'Description 1',
        ),
        StoryPayload(
          storyType: StoryType.STORY_TYPE_NEW_MEMBER_WELCOME,
          title: 'Story 2',
          description: 'Description 2',
        ),
      ];

      when(
        mockFeedService.listStories(communityId: communityId, limit: 10),
      ).thenAnswer((_) async => mockStories);

      // Mock cache get to call fetch function (simulating cache miss)
      when(
        mockCacheManager.get<List<StoryPayload>>(
          key: anyNamed('key'),
          fetch: anyNamed('fetch'),
        ),
      ).thenAnswer((invocation) async {
        final fetch =
            invocation.namedArguments[const Symbol('fetch')]
                as Future<List<StoryPayload>> Function();
        return fetch();
      });

      final result = await repository.listByCommunity(communityId);

      expect(result, hasLength(2));
      expect(result[0].title, 'Story 1');
      expect(result[1].title, 'Story 2');

      verify(
        mockFeedService.listStories(communityId: communityId, limit: 10),
      ).called(1);
    });

    test('listByCommunity respects custom limit', () async {
      final communityId = 'community123';
      final mockStories = <StoryPayload>[];

      when(
        mockFeedService.listStories(communityId: communityId, limit: 5),
      ).thenAnswer((_) async => mockStories);

      // Mock cache get to call fetch function (simulating cache miss)
      when(
        mockCacheManager.get<List<StoryPayload>>(
          key: anyNamed('key'),
          fetch: anyNamed('fetch'),
        ),
      ).thenAnswer((invocation) async {
        final fetch =
            invocation.namedArguments[const Symbol('fetch')]
                as Future<List<StoryPayload>> Function();
        return fetch();
      });

      await repository.listByCommunity(communityId, limit: 5);

      verify(
        mockFeedService.listStories(communityId: communityId, limit: 5),
      ).called(1);
    });

    test('listByCommunity returns cached data on subsequent calls', () async {
      final communityId = 'community123';
      final cachedStories = [
        StoryPayload(
          storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
          title: 'Cached Story',
          description: 'From cache',
        ),
      ];

      // Mock cache returning cached data without calling fetch
      when(
        mockCacheManager.get<List<StoryPayload>>(
          key: anyNamed('key'),
          fetch: anyNamed('fetch'),
        ),
      ).thenAnswer((_) async => cachedStories);

      final result = await repository.listByCommunity(communityId);

      expect(result, hasLength(1));
      expect(result[0].title, 'Cached Story');

      // Service should not be called if cache hit
      verifyNever(
        mockFeedService.listStories(
          communityId: anyNamed('communityId'),
          limit: anyNamed('limit'),
        ),
      );
    });

    test('invalidateCommunity removes cache entry', () async {
      final communityId = 'community123';

      // Mock the remove method that CacheService.invalidate calls
      when(mockCacheManager.remove(any)).thenAnswer((_) async => {});

      await repository.invalidateCommunity(communityId);

      // Verify remove was called with the namespaced key
      verify(mockCacheManager.remove('story:community:$communityId')).called(1);
    });
  });
}
