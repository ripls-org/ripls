/// Centralized cache key generation for CachedNetworkImage.
///
/// CachedNetworkImage uses flutter_cache_manager for disk caching,
/// which is separate from the app's data cache (StashCacheManager).
/// These keys identify cached image files on disk.
///
/// IMPORTANT: Thumbnails and full-size images MUST use different keys.
/// If they share a key, CachedNetworkImage will serve the cached thumbnail
/// when a full-resolution image is requested, causing blurry backgrounds.
///
/// ## Usage
///
/// ```dart
/// // Thumbnail display (cards, avatars, lists)
/// CachedNetworkImage(
///   imageUrl: url,
///   cacheKey: ImageCacheKeys.thumbnail(mediaId),
/// )
///
/// // Full-resolution display (backgrounds, detail views)
/// CachedNetworkImage(
///   imageUrl: url,
///   cacheKey: ImageCacheKeys.full(mediaId),
/// )
/// ```
class ImageCacheKeys {
  ImageCacheKeys._();

  /// Cache key for thumbnail images.
  ///
  /// Use for small displays: cards, avatars, list items, chat thumbnails.
  /// Typically for images displayed at < 200px.
  /// Returns null if mediaId is null.
  static String? thumbnail(String? mediaId) =>
      mediaId != null ? 'media:thumb:$mediaId' : null;

  /// Cache key for full-resolution images.
  ///
  /// Use for large displays: backgrounds, detail views, fullscreen viewers.
  /// Typically for images displayed at >= 200px or full screen.
  /// Returns null if mediaId is null.
  static String? full(String? mediaId) =>
      mediaId != null ? 'media:full:$mediaId' : null;
}
