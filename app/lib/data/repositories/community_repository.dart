import 'package:logging/logging.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/services/community_service.dart';

final _log = Logger('CommunityRepository');

/// Repository for community data with transparent caching.
///
/// This repository wraps CommunityService and provides caching for communities,
/// community gear lists, users, and events.
class CommunityRepository {
  final CacheService _cache;
  final CommunityService _service;
  final SearchRepository _searchRepository;
  final FeedRepository _feedRepository;
  final void Function()? _onDeletedListInvalidated;
  final void Function()? _onRejoinableListInvalidated;
  final void Function()? _onViewerMembershipChanged;

  CommunityRepository(
    CacheManager cacheManager,
    this._service,
    this._searchRepository,
    this._feedRepository, {
    void Function()? onDeletedListInvalidated,
    void Function()? onRejoinableListInvalidated,
    void Function()? onViewerMembershipChanged,
  })  : _cache = CacheService(cacheManager, 'community'),
        _onDeletedListInvalidated = onDeletedListInvalidated,
        _onRejoinableListInvalidated = onRejoinableListInvalidated,
        _onViewerMembershipChanged = onViewerMembershipChanged;

  /// StreamGenCommunity emits a `media_ready` event when a background
  /// image has been found, then a terminal `final` or `error`. Generation
  /// is one-shot and keyed on the typed name/description; this is a
  /// cache-bypass pass-through.
  Stream<StreamGenCommunityResponse> streamGenCommunity({
    String? prompt,
    String? region,
  }) {
    return _service.streamGenCommunity(
      prompt: prompt ?? '',
      region: region,
    );
  }

  /// Fetch the pinned-sheet presence state for [communityId].
  ///
  /// Deliberately uncached: the sheet must reflect commits (offers,
  /// RSVPs) immediately, and a stale ask card would prompt the viewer
  /// to commit to something already resolved.
  Future<GetCommunityPresenceForViewerResponse> getPresence(
      String communityId) {
    return _service.getCommunityPresenceForViewer(communityId);
  }

  /// Gets a community by ID with caching.
  Future<GetCommunityResponse> get(String communityId) async {
    final response = await _cache.get(
      key: communityId,
      fetch: () => _service.getCommunity(communityId),
    );
    _log.info(
      'get community response: id=$communityId mediaIds=${response.mediaIds}',
    );
    return response;
  }

  /// Lists all communities the user is a member of.
  ///
  /// Optional [regionFilter] filters communities by region.
  /// Use [refreshUserCommunities] to force a refresh.
  Future<List<CommunityItem>> listUserCommunities({
    RegionFilter? regionFilter,
  }) async {
    // Build cache key with filters
    String cacheKey = 'user:list';
    if (regionFilter != null) {
      cacheKey = '$cacheKey:${regionFilter.regionId}';
    }

    _log.info('Fetching user communities from cache or server (cacheKey: $cacheKey)');

    final result = await _cache.getList(
      listKey: cacheKey,
      fetch: () => _service.listCommunities(
        regionFilter: regionFilter,
      ),
    );

    _log.info('Community fetch completed: ${result.length} communities returned');
    for (final item in result) {
      _log.info(
        'list communities item: id=${item.id} mediaIds=${item.mediaIds}',
      );
    }
    return result;
  }

  /// Refreshes the user communities list by invalidating the cache.
  ///
  /// Call this after creating, leaving, or joining communities to ensure
  /// the next fetch gets fresh data.
  /// Optional [regionFilter] to refresh a specific filtered list.
  Future<List<CommunityItem>> refreshUserCommunities({
    RegionFilter? regionFilter,
  }) async {
    // Build cache key with filters
    String cacheKey = 'user:list';
    if (regionFilter != null) {
      cacheKey = '$cacheKey:${regionFilter.regionId}';
    }

    await _cache.invalidate(cacheKey);
    return listUserCommunities(regionFilter: regionFilter);
  }

  /// Lists all gear in a specific community.
  ///
  /// Use [refreshCommunityGear] to force a refresh.
  Future<List<CommunityGearItem>> listCommunityGear(String communityId) async {
    return _cache.getList(
      listKey: '$communityId:gear',
      fetch: () => _service.listCommunityGear(communityId),
    );
  }

  /// Refreshes the community gear list by invalidating the cache.
  ///
  /// Call this after sharing or unsharing gear to ensure the next fetch
  /// gets fresh data.
  Future<List<CommunityGearItem>> refreshCommunityGear(
    String communityId,
  ) async {
    await _cache.invalidate('$communityId:gear');
    return listCommunityGear(communityId);
  }

  /// Invalidates the gear list for every community at once.
  ///
  /// Use this after a gear delete so that all `{communityId}:gear` lists
  /// drop the deleted item. Counterpart to [refreshCommunityGear] for the
  /// nuclear case where the affected set of communities is unknown.
  Future<void> invalidateAllCommunityGearLists() async {
    await _cache.invalidatePattern('*:gear');
  }

  /// Lists all completed giveaways in a specific community.
  ///
  /// Returns completed giveaways (archived CommunityGear) with original owner information.
  /// Use [refreshCompletedGiveaways] to force a refresh.
  Future<List<CommunityGearItem>> listCompletedGiveaways(String communityId) async {
    return _cache.getList(
      listKey: '$communityId:completed_giveaways',
      fetch: () => _service.listCompletedGiveaways(communityId),
    );
  }

  /// Refreshes the completed giveaways list by invalidating the cache.
  ///
  /// Call this after a giveaway completes to ensure the next fetch
  /// gets fresh data.
  Future<List<CommunityGearItem>> refreshCompletedGiveaways(
    String communityId,
  ) async {
    await _cache.invalidate('$communityId:completed_giveaways');
    return listCompletedGiveaways(communityId);
  }

  /// Invalidates completed giveaways cache for all communities.
  ///
  /// Call this when a giveaway completes and you don't know which
  /// communities the gear was shared in. This ensures all completed giveaways
  /// lists are refreshed on next fetch.
  Future<void> invalidateCompletedGiveawaysAll() async {
    await _cache.invalidatePattern('*:completed_giveaways');
  }

  /// Gets the list of members in a community.
  ///
  /// Use [refreshMembers] to force a refresh.
  Future<List<CommunityMember>> getMembers(String communityId) async {
    _log.info('👥 CommunityRepository.getMembers - communityId: $communityId');
    final result = await _cache.getList(
      listKey: '$communityId:users',
      fetch: () => _service.listCommunityUsers(communityId),
    );
    _log.info('👥 CommunityRepository.getMembers - returning ${result.length} members');
    return result;
  }

  /// Lists all users in a specific community.
  ///
  /// Results are cached with global TTL. Use [refreshMembers]
  /// to force a refresh.
  ///
  /// @deprecated Use [getMembers] instead for consistency with architecture.
  Future<List<CommunityMember>> listCommunityUsers(String communityId) async {
    return getMembers(communityId);
  }

  /// Refreshes the community users list by invalidating the cache.
  ///
  /// Call this after users join or leave to ensure the next fetch
  /// gets fresh data.
  Future<void> refreshMembers(String communityId) async {
    await _cache.invalidate('$communityId:users');
  }

  /// Searches community members by display name using fuzzy matching.
  ///
  /// Results are not cached — search is ephemeral and query-specific.
  /// Returns up to [limit] users whose names contain [query] (case-insensitive).
  Future<List<User>> searchMembers({
    required String communityId,
    required String query,
    int limit = 10,
  }) async {
    return _service.searchCommunityUsers(
      communityId: communityId,
      query: query,
      limit: limit,
    );
  }

  /// Gets the list of events in a community.
  ///
  /// Use [refreshEvents] to force a refresh.
  Future<List<CommunityEventItem>> getEvents(String communityId) async {
    return _cache.getList(
      listKey: '$communityId:events',
      fetch: () => _service.listCommunityEvents(communityId),
    );
  }

  /// Refreshes the community events list by invalidating the cache.
  ///
  /// Call this after events are created or modified to ensure the next
  /// fetch gets fresh data.
  Future<void> refreshEvents(String communityId) async {
    await _cache.invalidate('$communityId:events');
  }

  /// Gets the count of gear items shared in a community.
  ///
  /// This method leverages the cached gear list to avoid an extra network call.
  /// Results are cached with global TTL (same as gear list).
  Future<int> getGearCount(String communityId) async {
    final gearList = await listCommunityGear(communityId);
    return gearList.length;
  }

  /// Refreshes the gear count by invalidating the gear list cache.
  ///
  /// Call this after sharing or unsharing gear to ensure the next fetch
  /// gets fresh data.
  Future<void> refreshGearCount(String communityId) async {
    await _cache.invalidate('$communityId:gear');
  }

  /// Refreshes all community-related caches for a specific community.
  ///
  /// This is a convenience method that invalidates users, events, gear,
  /// and completed giveaways caches for a community. Use this when you want to
  /// refresh all data for a community at once (e.g., pull-to-refresh).
  Future<void> refreshAll(String communityId) async {
    await Future.wait([
      _cache.invalidate('$communityId:users'),
      _cache.invalidate('$communityId:events'),
      _cache.invalidate('$communityId:gear'),
      _cache.invalidate('$communityId:completed_giveaways'),
    ]);
  }

  /// Creates a new community.
  ///
  /// Returns the ID of the created community.
  /// Automatically invalidates the user communities cache and the new community's cache entry.
  Future<String> createCommunity({
    required String name,
    required String description,
    List<String>? mediaIds,
  }) async {
    final communityId = await _service.createCommunity(
      name: name,
      description: description,
      mediaIds: mediaIds,
    );

    // Invalidate user communities cache since we just added a new one
    await refreshUserCommunities();

    // Invalidate the new community's cache entry to ensure fresh data
    await _cache.invalidate(communityId);

    // No realtime subscribe call: the user's single stream and poll resolve
    // membership server-side, so the new community is covered already (#2867).
    return communityId;
  }

  /// Updates an existing community.
  ///
  /// Automatically invalidates the community's cache.
  Future<void> updateCommunity({
    required String id,
    String? name,
    String? description,
    List<String>? mediaIds,
    String? mediaId, // Deprecated: for backward compatibility
  }) async {
    // Use mediaIds if provided, otherwise wrap single mediaId in array
    final effectiveMediaIds = mediaIds ?? (mediaId != null ? [mediaId] : null);

    await _service.updateCommunity(
      id: id,
      name: name,
      description: description,
      mediaIds: effectiveMediaIds,
    );

    // Invalidate this community's cache
    await _cache.invalidate(id);
    await refreshUserCommunities();
  }

  /// Deletes a community by ID.
  ///
  /// Automatically invalidates relevant caches, including the
  /// deleted-list cache so the soft-deleted community can appear in
  /// Settings → Communities for the eligible-restorer surface.
  Future<void> deleteCommunity(String id) async {
    await _service.deleteCommunity(id);

    // Invalidate this community, the user communities list, and the
    // deleted-list (so the freshly-deleted community surfaces for
    // restore).
    await _cache.invalidate(id);
    await invalidateDeletedList();
    await refreshUserCommunities();
  }

  /// Lists soft-deleted communities the caller is eligible to restore.
  ///
  /// Server returns items ordered ascending by deleted_at_unix_sec
  /// (oldest deletion first). Use [refresh] to force a fresh fetch.
  Future<List<DeletedCommunityItem>> listDeletedCommunitiesForRestore({
    bool refresh = false,
  }) async {
    if (refresh) {
      await invalidateDeletedList();
    }
    return _cache.getList(
      listKey: 'deleted:list',
      fetch: () => _service.listDeletedCommunitiesForRestore(),
    );
  }

  /// Invalidates the deleted-communities-for-restore cache so the next
  /// read fetches fresh data from the server, and notifies any
  /// listeners (e.g. the Settings hub recently-deleted section) so
  /// they can refetch immediately rather than waiting for an
  /// autoDispose grace period.
  Future<void> invalidateDeletedList() async {
    await _cache.invalidate('deleted:list');
    _onDeletedListInvalidated?.call();
  }

  /// Restores a previously soft-deleted community. The caller becomes
  /// the new owner.
  ///
  /// On success, automatically invalidates the deleted-list, the
  /// community row, and the user communities list. Cross-repository
  /// invalidation (gear, requests, experiences, transfers, chat, feed,
  /// stories, leaderboard, impact) is the caller's responsibility —
  /// the ViewModel orchestrates that fan-out.
  ///
  /// May throw [ServiceException] with `code = Code.failedPrecondition`
  /// if the community is no longer deleted, or `Code.permissionDenied`
  /// if the caller is not in the eligible-restorer snapshot.
  Future<void> restoreCommunity(String communityId) async {
    await _service.restoreCommunity(communityId);

    await _cache.invalidate(communityId);
    await invalidateDeletedList();
    await refreshUserCommunities();
    _onViewerMembershipChanged?.call();
  }

  /// Lists active communities the caller can rejoin without a fresh
  /// invite — one entry per soft-deleted CommunityUser row less than
  /// 30 days old whose community is still active.
  ///
  /// Server returns items ordered descending by left_at_unix_sec
  /// (most-recently-left first). Use [refresh] to force a fresh fetch.
  Future<List<RejoinableCommunityItem>> listRejoinableCommunities({
    bool refresh = false,
  }) async {
    if (refresh) {
      await invalidateRejoinableList();
    }
    return _cache.getList(
      listKey: 'rejoinable:list',
      fetch: () => _service.listRejoinableCommunities(),
    );
  }

  /// Invalidates the rejoinable-communities cache so the next read
  /// fetches fresh data from the server, and notifies any listeners
  /// (e.g. the Settings hub recently-left section) so they refetch
  /// immediately rather than waiting for an autoDispose grace period.
  Future<void> invalidateRejoinableList() async {
    await _cache.invalidate('rejoinable:list');
    _onRejoinableListInvalidated?.call();
  }

  /// Rejoins a community the caller previously left within the 30-day
  /// rejoin window.
  ///
  /// On success, automatically invalidates the rejoinable-list, the
  /// community row, and the user-communities list. Cross-repository
  /// invalidation (chat, feed, stories) is the caller's
  /// responsibility — the ViewModel orchestrates that fan-out.
  ///
  /// May throw [ServiceException] with `code = Code.failedPrecondition`
  /// for one of four documented reasons; the discriminator is the
  /// suffix on the message string (e.g. `: rejoin_window_expired`).
  Future<void> rejoinCommunity(String communityId) async {
    await _service.rejoinCommunity(communityId);

    await _cache.invalidate(communityId);
    await invalidateRejoinableList();
    await refreshUserCommunities();
    _onViewerMembershipChanged?.call();
  }

  /// Leaves a community.
  ///
  /// When the caller is the community owner, [newOwnerUserId] must
  /// identify another active member who will become the new owner.
  /// For non-owners pass null.
  ///
  /// On success, automatically invalidates the community row, the
  /// rejoinable-list (so the freshly soft-deleted membership surfaces
  /// in the recently-left section on next read), the deleted-list
  /// (so the §2.4 sole-member-leave conversion — which soft-deletes
  /// the community itself — appears in the recently-deleted section
  /// for the leaver who is in the eligible-restorer snapshot), and
  /// refreshes the user-communities list. Cross-repository
  /// invalidation (gear, requests, experiences, transfers, chat,
  /// feed, stories) is the caller's responsibility — the ViewModel
  /// orchestrates that fan-out per §7.5.
  Future<void> leaveCommunity(
    String communityId, {
    String? newOwnerUserId,
  }) async {
    await _service.leaveCommunity(
      communityId,
      newOwnerUserId: newOwnerUserId,
    );

    await _cache.invalidate(communityId);
    await invalidateRejoinableList();
    await invalidateDeletedList();
    await refreshUserCommunities();
    _onViewerMembershipChanged?.call();
  }

  /// Shares gear with a community.
  ///
  /// [availability] specifies whether the gear is available for loan or giveaway.
  /// Automatically invalidates the community gear cache and search cache.
  Future<void> shareGear({
    required String gearId,
    required String communityId,
    required Availability availability,
  }) async {
    await _service.shareGear(
      gearId: gearId,
      communityId: communityId,
      availability: availability,
    );

    // Invalidate the community's gear list
    await refreshCommunityGear(communityId);

    // Invalidate search cache so new gear appears in discover screen
    await _searchRepository.invalidateSearchesForCommunity(communityId);

    // Invalidate feed cache so the new item appears immediately
    await _feedRepository.invalidateFeed();
  }

  /// Shares an experience with communities and/or invites individuals to it in
  /// one call (see [CommunityService.shareItem]). Invalidates the feed and the
  /// search cache for every community the item now appears in (the shared
  /// communities plus any provisioned ad-hoc audience).
  Future<ShareItemResult> shareItem({
    String? experienceId,
    String? gearId,
    String? requestId,
    List<Invitee> invitees = const [],
    HostInviteConsent? consent,
    List<String> shareToCommunityIds = const [],
  }) async {
    final result = await _service.shareItem(
      experienceId: experienceId,
      gearId: gearId,
      requestId: requestId,
      invitees: invitees,
      consent: consent,
      shareToCommunityIds: shareToCommunityIds,
    );

    final affected = <String>{
      ...shareToCommunityIds,
      if (result.adhocCommunityId != null) result.adhocCommunityId!,
    };
    for (final communityId in affected) {
      await _searchRepository.invalidateSearchesForCommunity(communityId);
    }
    await _feedRepository.invalidateFeed();

    return result;
  }

  /// Unshares gear from a community.
  ///
  /// Automatically invalidates the community gear cache.
  Future<void> unshareGear({
    required String gearId,
    required String communityId,
  }) async {
    await _service.unshareGear(gearId: gearId, communityId: communityId);

    // Invalidate the community's gear list
    await refreshCommunityGear(communityId);
  }

  /// Accepts an invitation link to join a community.
  ///
  /// Returns the community ID and name.
  /// Automatically invalidates the user communities cache.
  Future<AcceptInvitationLinkResponse> acceptInvitationLink({
    required String shortCode,
  }) async {
    final response = await _service.acceptInvitationLink(
      shortCode: shortCode,
    );

    // Invalidate user communities cache since we just joined a new one
    await refreshUserCommunities();

    // No realtime subscribe call: the user's single stream and poll resolve
    // membership server-side, so the joined community is covered already
    // (#2867).
    _onViewerMembershipChanged?.call();

    return response;
  }

  /// Gets regions associated with a community.
  ///
  /// Returns a list of regions with their types, names, and member percentages.
  /// Use [refreshCommunityRegions] to force a refresh.
  Future<List<CommunityRegionItem>> getCommunityRegions(
    String communityId,
  ) async {
    return _cache.getList(
      listKey: '$communityId:regions',
      fetch: () => _service.listCommunityRegions(communityId),
    );
  }

  /// Refreshes the community regions list by invalidating the cache.
  ///
  /// Call this after setting region overrides to ensure the next fetch
  /// gets fresh data.
  Future<List<CommunityRegionItem>> refreshCommunityRegions(
    String communityId,
  ) async {
    await _cache.invalidate('$communityId:regions');
    return getCommunityRegions(communityId);
  }

  /// Sets a manual region override for a community.
  ///
  /// This allows community creators to manually specify regions instead of
  /// relying on automatic computation from member locations.
  /// Automatically invalidates the community regions cache and the community itself.
  Future<void> setCommunityRegionOverride({
    required String communityId,
    required String regionId,
  }) async {
    await _service.setCommunityRegionOverride(
      communityId: communityId,
      regionId: regionId,
    );

    // Invalidate the community's regions cache
    await refreshCommunityRegions(communityId);

    // Invalidate the community itself and user communities list
    await _cache.invalidate(communityId);
    await refreshUserCommunities();
  }

  /// Invalidates a specific community entry.
  Future<void> invalidate(String communityId) async {
    await _cache.invalidate(communityId);
  }

  /// Invalidates all cached communities.
  Future<void> invalidateAll() async {
    await _cache.invalidateAll();
  }

  /// Gets or creates a share link for a community or a specific item.
  ///
  /// Optionally identifies a typed target:
  /// - [gearId]: gear-specific link
  /// - [transferId]: transfer-specific link
  /// - [requestId]: request-specific link
  /// - [experienceId]: event-specific link
  ///
  /// At most one item ID should be set. When all are null, the link
  /// points at the community itself.
  ///
  /// Returns the typed share URL, short code, member counts, and the
  /// community the link is scoped to. Results are cached by community
  /// and item context.
  Future<GetOrCreateShareLinkResponse> getOrCreateShareLink({
    required String communityId,
    String? gearId,
    String? transferId,
    String? requestId,
    String? experienceId,
  }) async {
    final cacheKey = _buildShareLinkCacheKey(
      communityId,
      gearId: gearId,
      transferId: transferId,
      requestId: requestId,
      experienceId: experienceId,
    );

    return _cache.get<GetOrCreateShareLinkResponse>(
      key: cacheKey,
      fetch: () => _service.getOrCreateShareLink(
        communityId: communityId,
        gearId: gearId,
        transferId: transferId,
        requestId: requestId,
        experienceId: experienceId,
      ),
    );
  }

  /// Revokes the share link for a community or item target.
  ///
  /// Resolves the short code via [getOrCreateShareLink] (cache-first), then
  /// calls [RevokeShareLink] with that code, and invalidates the [share:…]
  /// cache key on success so the next [getOrCreateShareLink] mints a fresh row.
  Future<void> revokeShareLink({
    required String communityId,
    String? gearId,
    String? transferId,
    String? requestId,
    String? experienceId,
  }) async {
    final cacheKey = _buildShareLinkCacheKey(
      communityId,
      gearId: gearId,
      transferId: transferId,
      requestId: requestId,
      experienceId: experienceId,
    );

    // getOrCreateShareLink is cache-first: the short code is served from the
    // warm cache in the normal flow (QR already rendered), or fetched on the
    // first call if the cache is cold.
    final resp = await getOrCreateShareLink(
      communityId: communityId,
      gearId: gearId,
      transferId: transferId,
      requestId: requestId,
      experienceId: experienceId,
    );

    await _service.revokeShareLink(shortCode: resp.shortCode);
    await _cache.invalidate(cacheKey);
  }

  /// Reads notification preferences for a community with caching.
  Future<CommunityNotificationPreferences> getNotificationPreferences(
    String communityId,
  ) async {
    return _cache.get(
      key: 'notification_prefs:$communityId',
      fetch: () => _service.getNotificationPreferences(communityId),
    );
  }

  /// Persists notification preferences and invalidates the cache so the
  /// next read fetches the fresh row.
  Future<CommunityNotificationPreferences> updateNotificationPreferences({
    required String communityId,
    required CommunityNotificationPreferences preferences,
  }) async {
    final updated = await _service.updateNotificationPreferences(
      communityId: communityId,
      preferences: preferences,
    );
    await _cache.invalidate('notification_prefs:$communityId');
    return updated;
  }

  /// Builds a cache key for a share link scoped to a community + target.
  String _buildShareLinkCacheKey(
    String communityId, {
    String? gearId,
    String? transferId,
    String? requestId,
    String? experienceId,
  }) {
    if (gearId != null && gearId.isNotEmpty) {
      return 'share:$communityId:gear:$gearId';
    }
    if (transferId != null && transferId.isNotEmpty) {
      return 'share:$communityId:transfer:$transferId';
    }
    if (requestId != null && requestId.isNotEmpty) {
      return 'share:$communityId:request:$requestId';
    }
    if (experienceId != null && experienceId.isNotEmpty) {
      return 'share:$communityId:experience:$experienceId';
    }
    return 'share:$communityId';
  }
}
