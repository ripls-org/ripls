import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/cache/video_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/services/media_service.dart';

import 'media_repository_test.mocks.dart';

@GenerateMocks([MediaService, VideoFileCache])
void main() {
  group('MediaRepository', () {
    late MediaRepository repository;
    late MockMediaService mockService;
    late MockVideoFileCache mockVideoCache;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockMediaService();
      mockVideoCache = MockVideoFileCache();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = MediaRepository(cacheManager, mockService, videoCache: mockVideoCache);
    });

    group('getVideoFile', () {
      late MockVideoFileCache mockVideoCache;
      late MediaRepository videoRepository;

      setUp(() async {
        mockVideoCache = MockVideoFileCache();
        videoRepository = MediaRepository(
          cacheManager,
          mockService,
          videoCache: mockVideoCache,
        );
      });

      test('getVideoFile returns file from cache keyed by stable media id', () async {
        const mediaId = 'video123';
        const url = 'https://example.com/video.mp4?expires=999';
        final tmpFile = File('${Directory.systemTemp.path}/test_video.mp4');

        when(mockVideoCache.getFile('video_$mediaId', url))
            .thenAnswer((_) async => tmpFile);

        final result = await videoRepository.getVideoFile(mediaId, url);

        expect(result.path, tmpFile.path);
        verify(mockVideoCache.getFile('video_$mediaId', url)).called(1);
      });

      test('getVideoFile uses stable key for both original and rotated urls', () async {
        const mediaId = 'video456';
        const url1 = 'https://example.com/video.mp4?expires=111';
        const url2 = 'https://example.com/video.mp4?expires=999';
        final tmpFile = File('${Directory.systemTemp.path}/test_video2.mp4');

        when(mockVideoCache.getFile('video_$mediaId', any))
            .thenAnswer((_) async => tmpFile);

        await videoRepository.getVideoFile(mediaId, url1);
        await videoRepository.getVideoFile(mediaId, url2);

        // Same stable cache key used regardless of presigned URL rotation
        verify(mockVideoCache.getFile('video_$mediaId', url1)).called(1);
        verify(mockVideoCache.getFile('video_$mediaId', url2)).called(1);
      });

      test('evictVideo removes the stable cache key', () async {
        const mediaId = 'video789';

        when(mockVideoCache.evict('video_$mediaId'))
            .thenAnswer((_) => Future.value());

        await videoRepository.evictVideo(mediaId);

        verify(mockVideoCache.evict('video_$mediaId')).called(1);
      });
    });

    test('getMediaUrl returns thumbnail URL when available', () async {
      const mediaId = 'media123';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/full.jpg',
        thumbnailUrl: 'https://example.com/thumb.jpg',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      final result = await repository.getMediaUrl(mediaId);

      expect(result.url, 'https://example.com/thumb.jpg');
      expect(result.isThumbnail, true);
      expect(result.mediaId, mediaId);
      verify(mockService.getMedia(mediaId)).called(1);
    });

    test('getMediaUrl falls back to full URL when thumbnail is empty for image', () async {
      const mediaId = 'media123';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/full.jpg',
        thumbnailUrl: '', // No thumbnail
        contentType: 'image/jpeg',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      final result = await repository.getMediaUrl(mediaId);

      expect(result.url, 'https://example.com/full.jpg');
      expect(result.isThumbnail, false);
      expect(result.mediaId, mediaId);
    });

    test('getMediaUrl returns empty URL for video without thumbnail', () async {
      const mediaId = 'video123';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/video.mp4',
        thumbnailUrl: '', // No thumbnail
        contentType: 'video/mp4',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      final result = await repository.getMediaUrl(mediaId);

      // Must NOT fall back to the video URL — CachedNetworkImage cannot decode video bytes.
      expect(result.url, '');
      expect(result.isThumbnail, false);
      expect(result.mediaId, mediaId);
      expect(result.contentType, 'video/mp4');
    });

    test('getMediaUrl returns thumbnail URL for video that has a thumbnail', () async {
      const mediaId = 'video456';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/video.mp4',
        thumbnailUrl: 'https://example.com/video_thumb.jpg',
        contentType: 'video/mp4',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      final result = await repository.getMediaUrl(mediaId);

      expect(result.url, 'https://example.com/video_thumb.jpg');
      expect(result.isThumbnail, true);
      expect(result.contentType, 'video/mp4');
    });

    test('getMediaUrl includes contentType in returned MediaUrl', () async {
      const mediaId = 'media789';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/image.jpg',
        thumbnailUrl: 'https://example.com/thumb.jpg',
        contentType: 'image/jpeg',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      final result = await repository.getMediaUrl(mediaId);

      expect(result.contentType, 'image/jpeg');
    });

    test('getMediaUrl caches result on second call', () async {
      const mediaId = 'media123';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/full.jpg',
        thumbnailUrl: 'https://example.com/thumb.jpg',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      // First call - should fetch from service
      await repository.getMediaUrl(mediaId);

      // Second call - should use cache
      await repository.getMediaUrl(mediaId);

      // Service should only be called once
      verify(mockService.getMedia(mediaId)).called(1);
    });

    test('getFullMediaUrl always returns full URL', () async {
      const mediaId = 'media123';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/full.jpg',
        thumbnailUrl: 'https://example.com/thumb.jpg',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      final result = await repository.getFullMediaUrl(mediaId);

      expect(result.url, 'https://example.com/full.jpg');
      expect(result.isThumbnail, false);
      expect(result.mediaId, mediaId);
    });

    test('getHeroMediaUrl returns full URL for image media', () async {
      // Hero of an image community/experience should be sharp at large
      // sizes — return the full URL with isThumbnail:false so the cache
      // key resolves to media:full:{mediaId}. See #2064 / #2038.
      const mediaId = 'media123';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/full.jpg',
        thumbnailUrl: 'https://example.com/thumb.jpg',
        contentType: 'image/jpeg',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      final result = await repository.getHeroMediaUrl(mediaId);

      expect(result.url, 'https://example.com/full.jpg');
      expect(result.isThumbnail, false);
      expect(result.contentType, 'image/jpeg');
      expect(result.mediaId, mediaId);
    });

    test('getHeroMediaUrl returns poster URL for video media', () async {
      // Hero of a video community/experience must NOT return the raw
      // MP4 URL — NetworkImage / CachedNetworkImage can't decode video
      // bytes. Return the thumbnail (poster JPEG) so the still renders
      // while the MP4 downloads in the background. See #2064.
      const mediaId = 'media123';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/video.mp4',
        thumbnailUrl: 'https://example.com/poster.jpg',
        contentType: 'video/mp4',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      final result = await repository.getHeroMediaUrl(mediaId);

      expect(result.url, 'https://example.com/poster.jpg');
      expect(result.isThumbnail, true);
      expect(result.contentType, 'video/mp4');
      expect(result.mediaId, mediaId);
    });

    test('getHeroMediaUrl returns empty URL for video without poster', () async {
      // Edge case: a video that has no thumbnail. Returning the full
      // MP4 URL here would break NetworkImage downstream, so the repo
      // returns an empty URL and callers show a placeholder.
      const mediaId = 'media123';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/video.mp4',
        contentType: 'video/mp4',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      final result = await repository.getHeroMediaUrl(mediaId);

      expect(result.url, isEmpty);
      expect(result.isThumbnail, false);
      expect(result.contentType, 'video/mp4');
    });

    test('invalidate removes specific media from cache', () async {
      const mediaId = 'media123';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/full.jpg',
        thumbnailUrl: 'https://example.com/thumb.jpg',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      // Populate cache
      await repository.getMediaUrl(mediaId);

      // Invalidate
      await repository.invalidate(mediaId);

      // After invalidation, next call should fetch from service again
      await repository.getMediaUrl(mediaId);

      // Service should be called twice (once before invalidate, once after)
      verify(mockService.getMedia(mediaId)).called(2);
    });

    test('invalidateAll removes all media from cache', () async {
      final mockResponse1 = GetMediaResponse(
        id: 'media1',
        url: 'https://example.com/full1.jpg',
        thumbnailUrl: 'https://example.com/thumb1.jpg',
      );
      final mockResponse2 = GetMediaResponse(
        id: 'media2',
        url: 'https://example.com/full2.jpg',
        thumbnailUrl: 'https://example.com/thumb2.jpg',
      );

      when(mockService.getMedia('media1')).thenAnswer((_) async => mockResponse1);
      when(mockService.getMedia('media2')).thenAnswer((_) async => mockResponse2);

      // Populate cache with multiple entries
      await repository.getMediaUrl('media1');
      await repository.getMediaUrl('media2');

      // Invalidate all
      await repository.invalidateAll();

      // After invalidation, next calls should fetch from service again
      await repository.getMediaUrl('media1');
      await repository.getMediaUrl('media2');

      // Each service should be called twice (once before invalidate, once after)
      verify(mockService.getMedia('media1')).called(2);
      verify(mockService.getMedia('media2')).called(2);
    });

    test('MediaUrl has stable cache key', () async {
      const mediaId = 'media123';
      final mockResponse = GetMediaResponse(
        id: mediaId,
        url: 'https://example.com/full.jpg?expires=123',
        thumbnailUrl: 'https://example.com/thumb.jpg?expires=123',
      );

      when(mockService.getMedia(mediaId)).thenAnswer((_) async => mockResponse);

      final result = await repository.getMediaUrl(mediaId);

      // Cache key should be based on mediaId with thumbnail prefix (since thumbnailUrl is available)
      expect(result.cacheKey, ImageCacheKeys.thumbnail(mediaId));
    });

    test('handles service errors gracefully', () async {
      const mediaId = 'media123';

      when(mockService.getMedia(mediaId)).thenThrow(Exception('Network error'));

      expect(
        () => repository.getMediaUrl(mediaId),
        throwsException,
      );
    });
  });
}
