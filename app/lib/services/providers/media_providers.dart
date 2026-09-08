import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart' show Attribution;
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/services/media_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';

/// Provider for MediaService
final mediaServiceProvider = Provider<MediaService>((ref) {
  return MediaService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for MediaRepository
final mediaRepositoryProvider = Provider<MediaRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(mediaServiceProvider);
  return MediaRepository(cache, service);
});

/// Family provider for media URLs - fetches and caches media URLs by media ID
final mediaUrlProvider = FutureProvider.family<String, String>((
  ref,
  mediaId,
) async {
  final mediaRepository = ref.watch(mediaRepositoryProvider);
  final mediaUrl = await mediaRepository.getMediaUrl(mediaId);
  return mediaUrl.url;
});

/// Family provider for full media objects — fetches and caches MediaUrl (including
/// contentType) by media ID. Use this instead of [mediaUrlProvider] when you need
/// contentType (e.g., to show a play icon overlay for video items in list views).
final mediaObjectProvider = FutureProvider.family<MediaUrl, String>((
  ref,
  mediaId,
) async {
  final mediaRepository = ref.watch(mediaRepositoryProvider);
  return mediaRepository.getMediaUrl(mediaId);
});

/// Family provider for media attribution — returns [Attribution] if the media
/// is derived from stock imagery, or null for user-uploaded media.
final mediaAttributionProvider = FutureProvider.family<Attribution?, String>((
  ref,
  mediaId,
) async {
  final mediaRepository = ref.watch(mediaRepositoryProvider);
  final media = await mediaRepository.get(mediaId);
  return media.hasAttribution() ? media.attribution : null;
});

/// Family provider for full-resolution media URLs — fetches a [MediaUrl]
/// whose `.url` is the non-thumbnail (full) URL and whose `.cacheKey` is the
/// stable `media:full:{mediaId}` key.
///
/// Use this for surfaces that display the full image (fullscreen viewers,
/// carousels) and that need a freshly-resolved presigned URL on each open.
/// The autoDispose lifecycle ensures the provider drops its result when the
/// viewer is closed, so the next open re-resolves rather than reusing a
/// possibly-stale captured URL.
///
/// Retry pattern on URL expiration:
/// ```dart
/// // In an error handler:
/// await mediaRepository.invalidate(mediaId);
/// ref.invalidate(fullMediaUrlProvider(mediaId));
/// ```
final fullMediaUrlProvider =
    FutureProvider.autoDispose.family<MediaUrl, String>((ref, mediaId) async {
  final mediaRepository = ref.watch(mediaRepositoryProvider);
  return mediaRepository.getFullMediaUrl(mediaId);
});

/// Family provider for hero-shaped media URLs — fetches a [MediaUrl]
/// suitable for a full-bleed background or large preview surface.
///
/// - **Image media:** returns the full-resolution URL with the
///   `media:full:{mediaId}` cache key. Sharp at large render sizes.
/// - **Video media:** returns the thumbnail (poster) JPEG URL with the
///   `media:thumb:{mediaId}` cache key. `CachedNetworkImage` /
///   `NetworkImage` can decode the poster while the MP4 downloads in
///   the background; passing the raw video URL would render broken.
///
/// Decision matrix for the four media providers:
///
/// | Surface | Provider |
/// |---|---|
/// | Avatars, list rows, dropdown items (< 200px) | [mediaUrlProvider] |
/// | Avatars + content type lookup | [mediaObjectProvider] |
/// | Community / experience hero, full-bleed background | this provider |
/// | Fullscreen viewer, share preview (always image) | [fullMediaUrlProvider] |
///
/// Always pair the resolved URL with `mediaUrl.cacheKey` when rendering
/// through `CachedMediaImage` / `CachedNetworkImage` so the disk cache
/// keys stay distinct from the thumbnail cache. See
/// `docs/client/caching.md` § Image Cache Keys.
final heroMediaUrlProvider = FutureProvider.family<MediaUrl, String>((
  ref,
  mediaId,
) async {
  final mediaRepository = ref.watch(mediaRepositoryProvider);
  return mediaRepository.getHeroMediaUrl(mediaId);
});
