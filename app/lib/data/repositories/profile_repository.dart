import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/services/profile_service.dart';

export 'package:ripls/data/gen/ripls/api/profile_service.pb.dart'
    show GetUserProfileForViewerResponse, SharedCommunityRef;
export 'package:ripls/data/gen/ripls/api/workshop_service.pb.dart'
    show BriefCTARow, BriefPayload;

/// ProfileRepository wraps the ProfileService with transparent caching.
///
/// Cache namespace: `'profile'`. Keys are `'profile:<target_user_id>'`.
/// The viewer is implicit in the auth context, so the cache key does
/// not need to encode it — when the viewer changes, the auth flow
/// already triggers a full client reset.
///
/// Invalidation: callers that mutate the *viewer's* community
/// membership (join, leave, restore, rejoin, accept invitation) must
/// call [invalidateAll] because the viewer's membership set changes
/// the shared-community subset for every potentially-cached profile.
class ProfileRepository {
  final CacheService _cache;
  final ProfileService _service;

  ProfileRepository(CacheManager cacheManager, this._service)
      : _cache = CacheService(cacheManager, 'profile');

  /// Fetch the viewer-scoped profile of [targetUserId].
  /// Cached under `'profile:<targetUserId>'`.
  Future<GetUserProfileForViewerResponse> getProfile(String targetUserId) {
    return _cache.get(
      key: targetUserId,
      fetch: () => _service.getUserProfileForViewer(targetUserId: targetUserId),
    );
  }

  /// Fetch the pinned-sheet presence state for [targetUserId].
  ///
  /// Deliberately uncached: the sheet must reflect commits (offers,
  /// RSVPs) immediately, and a stale ask card would prompt the viewer
  /// to commit to something already resolved. The read is cheap and
  /// only fires on profile open.
  Future<GetProfilePresenceForViewerResponse> getPresence(
      String targetUserId) {
    return _service.getProfilePresenceForViewer(targetUserId: targetUserId);
  }

  /// Invalidate a specific target's cached profile entry.
  Future<void> invalidate(String targetUserId) {
    return _cache.invalidate(targetUserId);
  }

  /// Invalidate every cached profile entry. Use after the viewer's
  /// community membership changes — the shared-community subset is
  /// invalidated globally, not per-target.
  Future<void> invalidateAll() {
    return _cache.invalidateAll();
  }
}
