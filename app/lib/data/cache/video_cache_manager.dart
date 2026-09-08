import 'dart:io';

import 'package:flutter_cache_manager/flutter_cache_manager.dart';

/// VideoFileCache is the abstraction used by MediaRepository for video caching.
///
/// Extracted as an interface so tests can substitute a fake without depending
/// on flutter_cache_manager internals.
abstract class VideoFileCache {
  /// Downloads (or serves from disk cache) the video at [url], keyed by [cacheKey].
  Future<File> getFile(String cacheKey, String url);

  /// Removes the cached file for [cacheKey].
  Future<void> evict(String cacheKey);
}

/// VideoCacheManager is a disk-based file cache for video assets.
///
/// Videos are cached by stable media ID (not the expiring presigned URL),
/// preventing repeated HTTP range requests on each play or ViewModel recreation.
/// Backed by flutter_cache_manager with a 200 MB cap and 7-day stale period.
class VideoCacheManager implements VideoFileCache {
  static const _cacheKey = 'videoCache';
  static const _stalePeriod = Duration(days: 7);

  static final VideoCacheManager _instance = VideoCacheManager._();

  final CacheManager _cacheManager;

  VideoCacheManager._()
      : _cacheManager = CacheManager(
          Config(
            _cacheKey,
            maxNrOfCacheObjects: 100,
            stalePeriod: _stalePeriod,
            fileService: HttpFileService(),
          ),
        );

  /// Returns the singleton instance.
  static VideoCacheManager get instance => _instance;

  /// Downloads (or serves from disk cache) the video at [url], keyed by [cacheKey].
  ///
  /// Uses a stable [cacheKey] (e.g. `video_$mediaId`) so presigned URL
  /// rotations do not cause duplicate downloads.
  @override
  Future<File> getFile(String cacheKey, String url) async {
    final info = await _cacheManager.downloadFile(url, key: cacheKey);
    // flutter_cache_manager returns file.File; convert to dart:io.File via path.
    return File(info.file.path);
  }

  /// Removes the cached file for [cacheKey].
  @override
  Future<void> evict(String cacheKey) {
    return _cacheManager.removeFile(cacheKey);
  }
  }
