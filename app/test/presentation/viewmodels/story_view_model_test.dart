import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/presentation/viewmodels/story_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'story_view_model_test.mocks.dart';

@GenerateMocks([StoryRepository, MediaRepository])
void main() {
  late ProviderContainer container;
  late MockStoryRepository mockStoryRepository;
  late MockMediaRepository mockMediaRepository;

  setUp(() {
    mockStoryRepository = MockStoryRepository();
    mockMediaRepository = MockMediaRepository();
    container = ProviderContainer(
      overrides: [
        storyRepositoryProvider.overrideWithValue(mockStoryRepository),
        mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('StoryState', () {
    test('initial state loads media URLs', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Loan Completed',
        description: 'Great job completing the loan!',
        mediaIds: ['media1', 'media2'],
      );

      final mediaUrls = [
        MediaUrl(
          url: 'https://example.com/media1.jpg',
          isThumbnail: true,
          mediaId: 'media1',
        ),
        MediaUrl(
          url: 'https://example.com/media2.jpg',
          isThumbnail: false,
          mediaId: 'media2',
        ),
      ];

      when(
        mockStoryRepository.loadMediaUrls(story),
      ).thenAnswer((_) async => mediaUrls);

      // Wait for async build to complete
      final state = await container.read(storyProvider(story).future);

      expect(state.story, story);
      expect(state.mediaUrls, mediaUrls);
      expect(state.error, isNull);
      verify(mockStoryRepository.loadMediaUrls(story)).called(1);
    });

    test('state handles story with participants', () async {
      final participants = [
        User(id: 'user1', name: 'Alice'),
        User(id: 'user2', name: 'Bob'),
      ];

      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_NEW_MEMBER_WELCOME,
        title: 'Welcome New Member',
        description: 'Welcome to the community!',
        participants: participants,
      );

      when(
        mockStoryRepository.loadMediaUrls(story),
      ).thenAnswer((_) async => []);

      final state = await container.read(storyProvider(story).future);

      expect(state.participants, hasLength(2));
      expect(state.participants[0].name, 'Alice');
      expect(state.participants[1].name, 'Bob');
    });

    test('state handles error when loading media fails', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test Story',
        description: 'Test description',
        mediaIds: ['media1'],
      );

      when(
        mockStoryRepository.loadMediaUrls(story),
      ).thenThrow(Exception('Failed to load media'));

      final state = await container.read(storyProvider(story).future);

      expect(state.hasError, isTrue);
      expect(state.error, isNotNull);
      expect(state.story, story);
      expect(state.mediaUrls, isEmpty);
    });

    test('hasMedia returns correct value', () {
      final stateWithMedia = StoryState(
        story: StoryPayload(title: 'Test'),
        mediaUrls: [
          MediaUrl(
            url: 'https://example.com/media.jpg',
            isThumbnail: true,
            mediaId: 'media1',
          ),
        ],
      );

      final stateWithoutMedia = StoryState(story: StoryPayload(title: 'Test'));

      expect(stateWithMedia.hasMedia, isTrue);
      expect(stateWithoutMedia.hasMedia, isFalse);
    });

    test('primaryMediaUrl returns first media URL', () {
      final mediaUrl1 = MediaUrl(
        url: 'https://example.com/media1.jpg',
        isThumbnail: true,
        mediaId: 'media1',
      );
      final mediaUrl2 = MediaUrl(
        url: 'https://example.com/media2.jpg',
        isThumbnail: false,
        mediaId: 'media2',
      );

      final state = StoryState(
        story: StoryPayload(title: 'Test'),
        mediaUrls: [mediaUrl1, mediaUrl2],
      );

      expect(state.primaryMediaUrl, mediaUrl1);
    });

    test('primaryMediaUrl returns null when no media', () {
      final state = StoryState(story: StoryPayload(title: 'Test'));

      expect(state.primaryMediaUrl, isNull);
    });
  });

  group('refresh', () {
    test('reloads media URLs', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test Story',
        description: 'Test description',
        mediaIds: ['media1'],
      );

      final initialMediaUrls = [
        MediaUrl(
          url: 'https://example.com/media1.jpg',
          isThumbnail: true,
          mediaId: 'media1',
        ),
      ];

      final updatedMediaUrls = [
        MediaUrl(
          url: 'https://example.com/media1_updated.jpg',
          isThumbnail: false,
          mediaId: 'media1',
        ),
      ];

      when(
        mockStoryRepository.loadMediaUrls(story),
      ).thenAnswer((_) async => initialMediaUrls);

      // Initial load
      await container.read(storyProvider(story).future);

      // Update mock to return different URLs
      when(
        mockStoryRepository.loadMediaUrls(story),
      ).thenAnswer((_) async => updatedMediaUrls);

      // Refresh
      await container.read(storyProvider(story).notifier).refresh();

      final state = await container.read(storyProvider(story).future);

      expect(state.mediaUrls, updatedMediaUrls);
      expect(
        state.mediaUrls.first.url,
        'https://example.com/media1_updated.jpg',
      );

      // Should be called twice: once for initial load, once for refresh
      verify(mockStoryRepository.loadMediaUrls(story)).called(2);
    });
  });

  group('video support', () {
    test('isVideo is false for image media', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test Story',
        mediaIds: ['media1'],
      );

      when(mockStoryRepository.loadMediaUrls(story)).thenAnswer(
        (_) async => [
          MediaUrl(
            url: 'https://example.com/photo.jpg',
            isThumbnail: false,
            mediaId: 'media1',
            contentType: 'image/jpeg',
          ),
        ],
      );

      final state = await container.read(storyProvider(story).future);

      expect(state.isVideo, isFalse);
    });

    test('isVideo is false when media has no contentType', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test Story',
        mediaIds: ['media1'],
      );

      when(mockStoryRepository.loadMediaUrls(story)).thenAnswer(
        (_) async => [
          MediaUrl(
            url: 'https://example.com/photo.jpg',
            isThumbnail: false,
            mediaId: 'media1',
          ),
        ],
      );

      final state = await container.read(storyProvider(story).future);

      expect(state.isVideo, isFalse);
    });

    test('isVideo is false when there is no media', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test Story',
      );

      when(mockStoryRepository.loadMediaUrls(story))
          .thenAnswer((_) async => []);

      final state = await container.read(storyProvider(story).future);

      expect(state.isVideo, isFalse);
    });

    test('isVideo is true for video contentType', () async {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test Story',
        mediaIds: ['vid1'],
      );

      when(mockStoryRepository.loadMediaUrls(story)).thenAnswer(
        (_) async => [
          MediaUrl(
            url: 'https://example.com/video.mp4',
            isThumbnail: false,
            mediaId: 'vid1',
            contentType: 'video/mp4',
          ),
        ],
      );

      when(mockMediaRepository.getFullMediaUrl('vid1')).thenAnswer(
        (_) async => MediaUrl(
          url: 'https://example.com/video.mp4',
          isThumbnail: false,
          mediaId: 'vid1',
          contentType: 'video/mp4',
        ),
      );

      final state = await container.read(storyProvider(story).future);

      expect(state.isVideo, isTrue);
    });
  });
}
