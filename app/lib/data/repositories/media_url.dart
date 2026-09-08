import 'package:ripls/core/utils/image_cache_keys.dart';

/// Media URL wrapper with metadata for cache management.
///
/// This class wraps a media URL along with information about whether
/// it's a thumbnail and provides a stable cache key for use with
/// CachedNetworkImage.
class MediaUrl {
  /// The actual URL to the media (thumbnail or full).
  final String url;

  /// Whether this URL points to a thumbnail (true) or full media (false).
  final bool isThumbnail;

  /// The media ID this URL is associated with.
  final String mediaId;

  /// The MIME content type of the media (e.g., "video/mp4", "image/jpeg").
  ///
  /// Populated from [GetMediaResponse.contentType] in the repository layer.
  /// Use this for reliable video detection instead of URL heuristics.
  final String? contentType;

  const MediaUrl({
    required this.url,
    required this.isThumbnail,
    required this.mediaId,
    this.contentType,
  });

  /// Generates a stable cache key for CachedNetworkImage.
  ///
  /// This key remains constant even as presigned URLs change,
  /// allowing CachedNetworkImage to maintain its file cache
  /// across URL refreshes.
  ///
  /// Uses different keys for thumbnails vs full images to prevent
  /// cache collisions.
  String get cacheKey => isThumbnail
      ? ImageCacheKeys.thumbnail(mediaId)!
      : ImageCacheKeys.full(mediaId)!;

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is MediaUrl &&
          runtimeType == other.runtimeType &&
          url == other.url &&
          isThumbnail == other.isThumbnail &&
          mediaId == other.mediaId &&
          contentType == other.contentType;

  @override
  int get hashCode => Object.hash(url, isThumbnail, mediaId, contentType);

  @override
  String toString() =>
      'MediaUrl(mediaId: $mediaId, isThumbnail: $isThumbnail, url: ${url.substring(0, url.length > 50 ? 50 : url.length)}...)';
}
