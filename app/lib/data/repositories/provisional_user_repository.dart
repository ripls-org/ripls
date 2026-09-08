import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/services/community_service.dart';

export 'package:ripls/services/community_service.dart'
    show
        ProvisionalUser,
        GetProvisionalUserInviteLinkResponse,
        ProvisionalUserActivityItem,
        GetProvisionalUserActivitiesResponse;

/// Repository for provisional user data with transparent caching.
///
/// Provisional users are community-scoped lightweight placeholders for non-registered
/// participants. They merge with real accounts when the person registers via an
/// invite link.
class ProvisionalUserRepository {
  final CacheService _cache;
  final CommunityService _service;

  ProvisionalUserRepository(CacheManager cacheManager, this._service)
    : _cache = CacheService(cacheManager, 'provisional_user');

  /// Lists all provisional users in a community, with caching.
  Future<List<ProvisionalUser>> listProvisionalUsers(String communityId) {
    return _cache.getList(
      listKey: 'community:$communityId:list',
      fetch: () => _service.listProvisionalUsers(communityId: communityId),
    );
  }

  /// Searches provisional users in a community by name.
  ///
  /// Search results are not cached because queries are ephemeral.
  Future<List<ProvisionalUser>> searchProvisionalUsers({
    required String communityId,
    required String query,
    int limit = 10,
  }) {
    return _service.searchProvisionalUsers(
      communityId: communityId,
      query: query,
      limit: limit,
    );
  }

  /// Creates a provisional user and invalidates the community list cache.
  Future<ProvisionalUser> createProvisionalUser({
    required String communityId,
    required String name,
  }) async {
    final provisionalUser = await _service.createProvisionalUser(
      communityId: communityId,
      name: name,
    );
    await _cache.invalidateKeys([
      'community:$communityId:list',
      provisionalUser.id,
    ]);
    return provisionalUser;
  }

  /// Gets or creates an invite link for a provisional user.
  Future<GetProvisionalUserInviteLinkResponse> getInviteLink({
    required String communityId,
    required String provisionalUserId,
  }) {
    return _service.getProvisionalUserInviteLink(
      communityId: communityId,
      provisionalUserId: provisionalUserId,
    );
  }

  /// Gets the activity history for a provisional user.
  ///
  /// Results are not cached because they change as new events are completed.
  Future<GetProvisionalUserActivitiesResponse> getActivities({
    required String communityId,
    required String provisionalUserId,
    int limit = 20,
  }) {
    return _service.getProvisionalUserActivities(
      communityId: communityId,
      provisionalUserId: provisionalUserId,
      limit: limit,
    );
  }

  /// Invalidates the provisional user list cache for a community.
  Future<void> invalidateList(String communityId) {
    return _cache.invalidate('community:$communityId:list');
  }
}
