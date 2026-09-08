import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/services/user_service.dart';

import 'user_repository_test.mocks.dart';

@GenerateMocks([UserService, MediaRepository])
void main() {
  group('UserRepository', () {
    late UserRepository repository;
    late MockUserService mockUserService;
    late MockMediaRepository mockMediaRepository;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockUserService = MockUserService();
      mockMediaRepository = MockMediaRepository();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = UserRepository(
        cacheManager,
        mockUserService,
        mockMediaRepository,
      );
    });

    test('fetchById calls user service', () async {
      const userId = 'user123';
      final mockUser = GetUserResponse(
        userId: userId,
        name: 'John Doe',
        email: 'john@example.com',
      );

      when(mockUserService.getUser(userId))
          .thenAnswer((_) async => mockUser);

      final result = await repository.get(userId);

      expect(result.userId, userId);
      expect(result.name, 'John Doe');
      verify(mockUserService.getUser(userId)).called(1);
    });

    test('getById caches user data', () async {
      const userId = 'user123';
      final mockUser = GetUserResponse(
        userId: userId,
        name: 'John Doe',
      );

      when(mockUserService.getUser(userId))
          .thenAnswer((_) async => mockUser);

      // First call - should fetch
      await repository.get(userId);

      // Second call - should use cache
      await repository.get(userId);

      // Service should only be called once
      verify(mockUserService.getUser(userId)).called(1);
    });

    test('getUserProfile fetches user and media URL', () async {
      const userId = 'user123';
      const mediaId = 'media456';
      final mockUser = GetUserResponse(
        userId: userId,
        name: 'John Doe',
        mediaId: mediaId,
        mediaIds: [mediaId], // Updated to use mediaIds
      );
      final mockMedia = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/photo.jpg',
        thumbnailUrl: 'https://example.com/thumb.jpg',
      );

      when(mockUserService.getUser(userId))
          .thenAnswer((_) async => mockUser);
      when(mockMediaRepository.get(mediaId))
          .thenAnswer((_) async => mockMedia);
      when(mockMediaRepository.getMediaUrl(mediaId))
          .thenAnswer((_) async => const MediaUrl(
                url: 'https://example.com/thumb.jpg',
                isThumbnail: true,
                mediaId: mediaId,
              ));

      final result = await repository.getUserProfile(userId);

      expect(result.user.userId, userId);
      expect(result.user.name, 'John Doe');
      expect(result.mediaUrl, 'https://example.com/thumb.jpg');
      verify(mockUserService.getUser(userId)).called(1);
      verify(mockMediaRepository.getMediaUrl(mediaId)).called(1);
    });

    test('getUserProfile handles user without media', () async {
      const userId = 'user123';
      final mockUser = GetUserResponse(
        userId: userId,
        name: 'John Doe',
        mediaId: '', // No media
      );

      when(mockUserService.getUser(userId))
          .thenAnswer((_) async => mockUser);

      final result = await repository.getUserProfile(userId);

      expect(result.user.userId, userId);
      expect(result.mediaUrl, isNull);
      verify(mockUserService.getUser(userId)).called(1);
      verifyNever(mockMediaRepository.getMediaUrl(any));
    });

    test('getUserProfile handles media fetch error gracefully', () async {
      const userId = 'user123';
      const mediaId = 'media456';
      final mockUser = GetUserResponse(
        userId: userId,
        name: 'John Doe',
        mediaId: mediaId,
        mediaIds: [mediaId], // Updated to use mediaIds
      );

      when(mockUserService.getUser(userId))
          .thenAnswer((_) async => mockUser);
      when(mockMediaRepository.getMediaUrl(mediaId))
          .thenThrow(Exception('Media not found'));

      final result = await repository.getUserProfile(userId);

      expect(result.user.userId, userId);
      expect(result.mediaUrl, isNull); // Gracefully handled
      verify(mockUserService.getUser(userId)).called(1);
      verify(mockMediaRepository.getMediaUrl(mediaId)).called(1);
    });

    test('handles service errors', () {
      const userId = 'user123';

      when(mockUserService.getUser(userId))
          .thenThrow(Exception('Network error'));

      expect(
        () => repository.get(userId),
        throwsException,
      );
    });

    test('getNotificationPreferences caches the row', () async {
      final stored = UserNotificationPreferences()
        ..notifyLoanReturnReminders = false;
      when(mockUserService.getUserNotificationPreferences())
          .thenAnswer((_) async => stored);

      final first = await repository.getNotificationPreferences();
      final second = await repository.getNotificationPreferences();

      expect(first.notifyLoanReturnReminders, isFalse);
      expect(second.notifyLoanReturnReminders, isFalse);
      // Cache hit on the second call — service is only called once.
      verify(mockUserService.getUserNotificationPreferences()).called(1);
    });

    test('updateNotificationPreferences invalidates the cache', () async {
      final initial = UserNotificationPreferences()
        ..notifyLoanReturnReminders = true;
      final updated = UserNotificationPreferences()
        ..notifyLoanReturnReminders = false;

      when(mockUserService.getUserNotificationPreferences())
          .thenAnswer((_) async => initial);
      when(mockUserService.updateUserNotificationPreferences(
        preferences: anyNamed('preferences'),
      )).thenAnswer((_) async => updated);

      // Seed the cache with the initial row.
      await repository.getNotificationPreferences();

      // Update — should invalidate the cache so the next read goes
      // back to the service.
      final fromUpdate = await repository.updateNotificationPreferences(
        preferences: UserNotificationPreferences()
          ..notifyLoanReturnReminders = false,
      );
      expect(fromUpdate.notifyLoanReturnReminders, isFalse);

      // Now arrange the service to return the new value so the
      // cache-miss path picks it up.
      when(mockUserService.getUserNotificationPreferences())
          .thenAnswer((_) async => updated);

      final afterInvalidate = await repository.getNotificationPreferences();
      expect(afterInvalidate.notifyLoanReturnReminders, isFalse);

      // Service called twice: once before update, once after invalidation.
      verify(mockUserService.getUserNotificationPreferences()).called(2);
    });
  });
}
