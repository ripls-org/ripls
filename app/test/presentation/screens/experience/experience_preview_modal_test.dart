import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/data/repositories/media_url.dart';

/// Tests for the experience preview modal background image pattern.
///
/// These tests verify that the MediaUrl class provides stable cache keys
/// based on media ID, which is critical for preventing the image caching bug
/// where different media items could show the wrong cached image.
///
/// See docs/ai/experience_image.md for full context on the bug and fix.
void main() {
  group('Experience Background Image Caching Pattern', () {
    test('MediaUrl provides stable cache keys based on media ID', () {
      const testMediaId = 'test-media-123';
      const testUrl = 'https://example.com/test-image.jpg';

      final mediaUrl = MediaUrl(
        url: testUrl,
        isThumbnail: false,
        mediaId: testMediaId,
      );

      // Verify MediaUrl provides correct cache key format (full resolution since isThumbnail: false)
      expect(mediaUrl.cacheKey, equals(ImageCacheKeys.full(testMediaId)));
      expect(mediaUrl.url, equals(testUrl));
      expect(mediaUrl.mediaId, equals(testMediaId));
    });

    test('cache key uses ImageCacheKeys.full for full-resolution images', () {
      const testMediaId = 'media-abc-xyz';

      final mediaUrl = MediaUrl(
        url: 'https://example.com/image.jpg',
        isThumbnail: false,
        mediaId: testMediaId,
      );

      // Verify cache key uses ImageCacheKeys.full() for full-resolution images
      expect(mediaUrl.cacheKey, equals(ImageCacheKeys.full(testMediaId)));
    });

    test('different media IDs produce different cache keys', () {
      const mediaId1 = 'media-111';
      const mediaId2 = 'media-222';

      final mediaUrl1 = MediaUrl(
        url: 'https://example.com/image1.jpg',
        isThumbnail: false,
        mediaId: mediaId1,
      );

      final mediaUrl2 = MediaUrl(
        url: 'https://example.com/image2.jpg',
        isThumbnail: false,
        mediaId: mediaId2,
      );

      // Verify different media IDs produce different cache keys
      expect(mediaUrl1.cacheKey, isNot(equals(mediaUrl2.cacheKey)));
      expect(mediaUrl1.cacheKey, equals(ImageCacheKeys.full(mediaId1)));
      expect(mediaUrl2.cacheKey, equals(ImageCacheKeys.full(mediaId2)));
    });

    test('same media ID produces same cache key even with different URLs', () {
      const testMediaId = 'media-same';
      const url1 = 'https://example.com/presigned-url-1?expires=12345';
      const url2 = 'https://example.com/presigned-url-2?expires=67890';

      final mediaUrl1 = MediaUrl(
        url: url1,
        isThumbnail: false,
        mediaId: testMediaId,
      );

      final mediaUrl2 = MediaUrl(
        url: url2,
        isThumbnail: false,
        mediaId: testMediaId,
      );

      // Critical: Same media ID should always produce same cache key,
      // even if presigned URLs differ. This prevents the caching bug.
      expect(mediaUrl1.cacheKey, equals(mediaUrl2.cacheKey));
      expect(mediaUrl1.cacheKey, equals(ImageCacheKeys.full(testMediaId)));
      expect(mediaUrl2.cacheKey, equals(ImageCacheKeys.full(testMediaId)));

      // URLs can differ (presigned URLs expire and regenerate)
      expect(mediaUrl1.url, isNot(equals(mediaUrl2.url)));
    });
  });
}
