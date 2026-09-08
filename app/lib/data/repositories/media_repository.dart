import 'dart:io';

import 'package:cross_file/cross_file.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/cache/video_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/services/media_service.dart';

/// Repository for media data with transparent caching.
///
/// This repository wraps MediaService and provides caching for media
/// metadata and URLs using the global TTL from environment (CACHE_TTL_MINUTES).
/// Video file bytes are cached on disk via [VideoFileCache].
class MediaRepository {
  final CacheService _cache;
  final MediaService _service;
  final VideoFileCache _videoCache;

  MediaRepository(
    CacheManager cacheManager,
    this._service, {
    VideoFileCache? videoCache,
  })  : _cache = CacheService(cacheManager, 'media'),
        _videoCache = videoCache ?? VideoCacheManager.instance;

  /// Gets media metadata by ID with caching.
  Future<GetMediaResponse> get(String mediaId) async {
    return _cache.get(
      key: mediaId,
      fetch: () => _service.getMedia(mediaId),
    );
  }

  /// Gets the best available media URL (thumbnail if available, otherwise full).
  ///
  /// This method fetches media metadata and intelligently returns either the
  /// thumbnail URL (for better performance) or the full URL as a fallback.
  ///
  /// For video media without a thumbnail, returns an empty URL rather than the
  /// full video URL. CachedNetworkImage cannot decode video bytes as an image
  /// and would throw "Invalid image data" errors if given a video URL.
  Future<MediaUrl> getMediaUrl(String mediaId) async {
    final media = await get(mediaId);
    final isVideo =
        media.contentType.toLowerCase().startsWith('video/');
    // Videos without thumbnails must not fall back to the full video URL —
    // callers must handle empty URL by showing a placeholder.
    if (isVideo && media.thumbnailUrl.isEmpty) {
      return MediaUrl(
        url: '',
        isThumbnail: false,
        mediaId: mediaId,
        contentType: media.contentType,
      );
    }
    return MediaUrl(
      url: media.thumbnailUrl.isNotEmpty ? media.thumbnailUrl : media.url,
      isThumbnail: media.thumbnailUrl.isNotEmpty,
      mediaId: mediaId,
      contentType: media.contentType.isNotEmpty ? media.contentType : null,
    );
  }

  /// Gets the full (non-thumbnail) media URL.
  ///
  /// Use this when you specifically need the full-resolution media,
  /// not the thumbnail.
  Future<MediaUrl> getFullMediaUrl(String mediaId) async {
    final media = await get(mediaId);
    return MediaUrl(
      url: media.url,
      isThumbnail: false,
      mediaId: mediaId,
      contentType: media.contentType.isNotEmpty ? media.contentType : null,
    );
  }

  /// Returns the URL appropriate for a full-bleed hero / preview
  /// background. Images use the full-resolution URL so the hero looks
  /// sharp; videos return the thumbnail (poster JPEG) so the still
  /// renders while the MP4 downloads — the full video URL would be
  /// passed to `CachedNetworkImage` which can't decode video bytes.
  ///
  /// Videos without a thumbnail return an empty URL; the caller
  /// should show a placeholder until the video controller initializes.
  Future<MediaUrl> getHeroMediaUrl(String mediaId) async {
    final media = await get(mediaId);
    final isVideo = media.contentType.toLowerCase().startsWith('video/');
    if (isVideo) {
      if (media.thumbnailUrl.isEmpty) {
        return MediaUrl(
          url: '',
          isThumbnail: false,
          mediaId: mediaId,
          contentType: media.contentType,
        );
      }
      return MediaUrl(
        url: media.thumbnailUrl,
        isThumbnail: true,
        mediaId: mediaId,
        contentType: media.contentType,
      );
    }
    return MediaUrl(
      url: media.url,
      isThumbnail: false,
      mediaId: mediaId,
      contentType: media.contentType.isNotEmpty ? media.contentType : null,
    );
  }

  /// Adds new media by uploading a cross-platform [XFile].
  ///
  /// XFile is the common return type of `image_picker` and `camera`
  /// across iOS, Android, and web. Backing storage differs by
  /// platform (filesystem path on mobile, blob URL on web), but
  /// `XFile.readAsBytes()` works on every target — see
  /// `MediaService.addMedia` for the upload mechanics.
  ///
  /// Returns the media ID of the uploaded file.
  /// Does not cache (write operation).
  Future<String> addMedia({
    required XFile file,
    String? description,
  }) async {
    return _service.addMedia(
      file: file,
      description: description,
    );
  }

  /// Imports media by URL — the server fetches and stores the bytes,
  /// returning the new media id. Used by the Replace Media flow to
  /// swap in a streamed alternate candidate without round-tripping
  /// bytes through the device.
  ///
  /// Pass [provider] and [providerPhotoId] together when the URL came
  /// from a stock-image provider so the server can preserve attribution
  /// at import time. Does not cache (write operation).
  Future<String> addMediaFromUrl({
    required String url,
    String? description,
    StockImageProvider? provider,
    String? providerPhotoId,
  }) async {
    return _service.addMediaFromUrl(
      url: url,
      description: description,
      provider: provider,
      providerPhotoId: providerPhotoId,
    );
  }

  /// Deletes media by ID.
  ///
  /// Invalidates the cache entry for the deleted media.
  /// Does not cache (write operation).
  Future<void> deleteMedia(String mediaId) async {
    await _service.deleteMedia(mediaId);
    await invalidate(mediaId);
  }

  /// Gets media metadata by ID.
  ///
  /// This is a convenience wrapper around get() that returns
  /// the full GetMediaResponse.
  Future<GetMediaResponse> getMedia(String mediaId) async {
    return get(mediaId);
  }

  /// Downloads (or returns the cached disk file) for the video identified by [mediaId].
  ///
  /// Uses [url] only on a cache miss; subsequent calls return the local file
  /// without touching the network, even if the presigned [url] has changed.
  ///
  /// Mobile / desktop only — the underlying disk cache uses `dart:io` and
  /// `flutter_cache_manager`, neither of which has a web implementation. Web
  /// video playback bypasses this entirely via
  /// `VideoPlayerController.networkUrl` in `video_cache_helper_web.dart`.
  Future<File> getVideoFile(String mediaId, String url) {
    assert(!kIsWeb,
        'MediaRepository.getVideoFile is mobile-only; web playback uses '
        'VideoPlayerController.networkUrl directly');
    return _videoCache.getFile('video_$mediaId', url);
  }

  /// Removes the cached video file for [mediaId].
  Future<void> evictVideo(String mediaId) {
    return _videoCache.evict('video_$mediaId');
  }

  /// Invalidates a specific media entry.
  Future<void> invalidate(String mediaId) async {
    await _cache.invalidate(mediaId);
  }

  /// Invalidates all cached media.
  Future<void> invalidateAll() async {
    await _cache.invalidateAll();
  }
}
