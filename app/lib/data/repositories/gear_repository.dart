import 'package:flutter/foundation.dart' show VoidCallback;
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/gear_service.dart';

export 'package:ripls/data/gen/ripls/api/gear_booking.pb.dart'
    show GearBooking, GearBookingState;
export 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show
        GetTransferRequestCountResponse,
        DetectedGearItem,
        GetGearPeopleResponse,
        GetGearStatsResponse,
        GearMetadata,
        StreamGenGearRequest,
        StreamGenGearResponse,
        StreamGenGearResponse_Event;
export 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart'
    show GenStreamError, GenStreamErrorCode, MediaReady;

/// Repository for gear data with transparent caching.
///
/// This repository wraps GearService and provides caching for gear
/// items using the global TTL from environment (CACHE_TTL_MINUTES).
class GearRepository {
  final CacheService _cache;
  final GearService _service;
  final CommunityService _communityService;
  final SearchRepository _searchRepository;
  final FeedRepository _feedRepository;
  final ChatRepository _chatRepository;
  final CommunityRepository _communityRepository;
  final VoidCallback? _onDailyInvalidated;
  final VoidCallback? _onProfileInvalidated;

  GearRepository(
    CacheManager cacheManager,
    this._service,
    this._communityService,
    this._searchRepository,
    this._feedRepository,
    this._chatRepository,
    this._communityRepository, {
    VoidCallback? onDailyInvalidated,
    VoidCallback? onProfileInvalidated,
  })  : _cache = CacheService(cacheManager, 'gear'),
        _onDailyInvalidated = onDailyInvalidated,
        _onProfileInvalidated = onProfileInvalidated;

  /// Gets a gear item by ID with caching.
  ///
  /// If [communityId] is provided, also returns community-specific fields
  /// (conversation_id, active_loan, availability).
  Future<GetGearResponse> get(String gearId, {String? communityId}) async {
    final cacheKey = communityId != null ? '$gearId:$communityId' : gearId;
    return _cache.get(
      key: cacheKey,
      fetch: () => _service.getGear(gearId, communityId: communityId),
    );
  }

  /// Lists all gear items owned by the current user.
  ///
  /// Use [refreshUserGear] to force a refresh.
  Future<List<GearItem>> listUserGear() async {
    return _cache.getList(
      listKey: 'user:list',
      fetch: () => _service.listGear(),
    );
  }

  /// Refreshes the user gear list by invalidating the cache.
  ///
  /// Call this after adding, updating, or deleting gear to ensure
  /// the next fetch gets fresh data.
  Future<List<GearItem>> refreshUserGear() async {
    await _cache.invalidate('user:list');
    return listUserGear();
  }

  /// Gets a gear item with full details.
  ///
  /// If [communityId] is provided, also returns community-specific fields
  /// (conversation_id, active_loan, availability).
  /// This wraps [get] with a more descriptive name.
  Future<GetGearResponse> getGearDetails(String gearId, {String? communityId}) async {
    return get(gearId, communityId: communityId);
  }

  /// Invalidates a specific gear entry.
  ///
  /// If [communityId] is provided, invalidates the community-specific cache entry.
  /// Otherwise, invalidates both the base entry and uses a pattern to clear
  /// all community-specific entries for this gear.
  Future<void> invalidate(String gearId, {String? communityId}) async {
    if (communityId != null) {
      // Invalidate specific community cache
      await _cache.invalidate('$gearId:$communityId');
    } else {
      // Invalidate base cache and all community-specific caches for this gear
      await _cache.invalidate(gearId);
      await _cache.invalidatePattern('$gearId:*');
    }
  }

  /// Invalidates every cached entry scoped to a single community.
  ///
  /// Use this after a community-level lifecycle change (e.g. restoring
  /// a previously soft-deleted community) so the next read fetches the
  /// freshly un-soft-deleted gear list rather than the cached
  /// "no gear visible" snapshot from the deleted period.
  Future<void> invalidateForCommunity(String communityId) async {
    await _cache.invalidatePattern('*:$communityId');
  }

  /// Invalidates all cached gear.
  Future<void> invalidateAll() async {
    await _cache.invalidateAll();
  }

  /// Saves gear details (update operation).
  ///
  /// Supports partial updates - only provided fields are updated.
  /// Invalidates the gear item cache, user gear list, and search cache after save.
  Future<void> saveGear({
    required String id,
    String? name,
    String? description,
    List<String>? mediaIds,
    String? locationId,
    GearMetadata? metadata,
    String? sourceUrl,
  }) async {
    await _service.saveGear(
      id: id,
      name: name,
      description: description,
      mediaIds: mediaIds,
      locationId: locationId,
      metadata: metadata,
      sourceUrl: sourceUrl,
    );

    // Invalidate caches after mutation
    await invalidate(id);
    await refreshUserGear();

    // Invalidate search cache so updates appear in discover screen
    await _searchRepository.invalidateSearches();
    _onDailyInvalidated?.call();
  }

  /// Deletes a gear item.
  ///
  /// Invalidates the gear item cache, user gear list, search cache, feed cache,
  /// conversations cache (cascade deletion), all community gear lists, and
  /// the profile cache after deletion.
  Future<void> deleteGear(String gearId) async {
    await _service.deleteGear(gearId);

    // Invalidate caches after mutation
    await invalidate(gearId);
    await refreshUserGear();
    // Invalidate all feed caches since deleted gear should disappear from feed
    await _feedRepository.invalidateAllFeeds();
    // Invalidate search cache so deleted gear disappears from discover
    await _searchRepository.invalidateSearches();
    // Invalidate conversations cache since associated conversation was cascade-deleted
    await _chatRepository.refreshConversations();
    // Invalidate all community gear lists so the item disappears from community screens
    await _communityRepository.invalidateAllCommunityGearLists();
    // Invalidate profile caches so Available Now rails drop the deleted item
    _onProfileInvalidated?.call();
    _onDailyInvalidated?.call();
  }

  /// Creates a new gear item.
  ///
  /// Returns the ID of the created gear item.
  /// Invalidates the user gear list cache after creation.
  /// Does not cache (write operation).
  Future<String> createGear({
    required String name,
    required String description,
    List<String>? mediaIds,
    String? locationId,
    GearMetadata? metadata,
    String? generationMode,
    String? sourceUrl,
  }) async {
    final gearId = await _service.saveGear(
      name: name,
      description: description,
      mediaIds: mediaIds,
      locationId: locationId,
      metadata: metadata,
      generationMode: generationMode,
      sourceUrl: sourceUrl,
    );

    // Invalidate user gear list to ensure fresh data on next fetch
    await refreshUserGear();
    _onDailyInvalidated?.call();

    return gearId;
  }

  /// StreamGenGear emits title / media_ready events as they resolve, then a
  /// terminal `final` or `error`. Generation is one-shot and unique per
  /// prompt/URL/image; this is a cache-bypass pass-through.
  ///
  /// Provide [prompt] for text mode, [websiteUrl] for webpage extraction,
  /// or [mediaId] for image mode.
  Stream<StreamGenGearResponse> streamGenGear({
    String? prompt,
    String? websiteUrl,
    String? mediaId,
    String? locationId,
    double? latitudeDeg,
    double? longitudeDeg,
  }) {
    return _service.streamGenGear(
      prompt: prompt,
      websiteUrl: websiteUrl,
      mediaId: mediaId,
      locationId: locationId,
      latitudeDeg: latitudeDeg,
      longitudeDeg: longitudeDeg,
    );
  }

  /// Extracts product metadata (brand, category, material, weight, value) from a product page URL.
  ///
  /// Fetches the webpage and uses AI to extract structured metadata fields only.
  /// Returns null if the page was fetched but contained no detectable gear.
  /// Throws on network or AI errors — no silent fallback.
  Future<DetectedGearItem?> detectGearFromURL(String url) async {
    return _service.detectGearFromURL(url);
  }

  /// Gets transfer request count for a gear item with caching.
  ///
  /// Returns information about transfer requests:
  /// - For owners: count of non-archived requests received across all communities
  /// - For borrowers: whether they have an active request and its conversation ID
  ///
  /// Uses short-lived cache (global TTL). Call [invalidateRequestCount] after
  /// any transfer mutations to ensure fresh data.
  Future<GetTransferRequestCountResponse> getRequestCount(String gearId) async {
    return _cache.get(
      key: 'request_count:$gearId',
      fetch: () => _service.getTransferRequestCount(gearId: gearId),
    );
  }

  /// Invalidates the request count cache for a specific gear item.
  ///
  /// Call this after any transfer mutations (express interest, approve,
  /// decline, cancel) to ensure the next fetch gets fresh data.
  Future<void> invalidateRequestCount(String gearId) async {
    await _cache.invalidate('request_count:$gearId');
  }

  /// Updates the media order for a gear item.
  ///
  /// Invalidates the gear item cache and user gear list after update.
  Future<void> updateMediaOrder(String gearId, List<String> mediaIds) async {
    // Get current gear details to preserve name, description, etc.
    final gear = await get(gearId);

    // Call saveGear with reordered mediaIds
    await saveGear(
      id: gearId,
      name: gear.name,
      description: gear.description,
      mediaIds: mediaIds,
      locationId: gear.locationId.isNotEmpty ? gear.locationId : null,
    );
  }

  /// Shares multiple gear items with a community.
  ///
  /// Executes sequential API calls and handles partial failures gracefully.
  /// Returns a list of gear IDs that failed to share.
  /// Invalidates cache after completion for all items (successful and failed).
  Future<List<String>> shareMultipleWithCommunity(
    List<String> gearIds,
    String communityId,
    Availability availability,
  ) async {
    final failedIds = <String>[];
    for (final gearId in gearIds) {
      try {
        await _communityService.shareGear(
          gearId: gearId,
          communityId: communityId,
          availability: availability,
        );
      } catch (e) {
        failedIds.add(gearId);
      }
    }

    // Invalidate cache after bulk operation
    await _cache.invalidatePattern('*');
    await _searchRepository.invalidateSearches();
    await _feedRepository.invalidateAllFeeds();
    _onDailyInvalidated?.call();

    return failedIds;
  }

  /// Unshares multiple gear items from a community.
  ///
  /// Executes sequential API calls and handles partial failures gracefully.
  /// Returns a list of gear IDs that failed to unshare.
  /// Invalidates cache after completion for all items (successful and failed).
  Future<List<String>> unshareMultipleFromCommunity(
    List<String> gearIds,
    String communityId,
  ) async {
    final failedIds = <String>[];
    for (final gearId in gearIds) {
      try {
        await _communityService.unshareGear(
          gearId: gearId,
          communityId: communityId,
        );
      } catch (e) {
        failedIds.add(gearId);
      }
    }

    // Invalidate cache after bulk operation
    await _cache.invalidatePattern('*');
    await _searchRepository.invalidateSearches();
    await _feedRepository.invalidateAllFeeds();
    _onDailyInvalidated?.call();

    return failedIds;
  }

  /// Sets an item's availability (loan vs giveaway) across every community it
  /// is shared with, via the gear-wide SetGearAvailability RPC. Invalidates the
  /// gear (all community variants), search, and feed caches so the new mode is
  /// reflected everywhere it appears.
  ///
  /// Throws [ServiceException] when the change is rejected — notably a
  /// FailedPrecondition while the item has an in-progress loan or giveaway;
  /// callers surface the message to the user.
  Future<void> setGearAvailability(
    String gearId,
    Availability availability,
  ) async {
    await _communityService.setGearAvailability(
      gearId: gearId,
      availability: availability,
    );
    await invalidate(gearId);
    await _searchRepository.invalidateSearches();
    await _feedRepository.invalidateAllFeeds();
    _onDailyInvalidated?.call();
  }

  /// Updates location for multiple gear items.
  ///
  /// Executes sequential API calls and handles partial failures gracefully.
  /// Fetches each item first to preserve all existing fields (name, description, mediaIds).
  /// Returns a list of gear IDs that failed to update.
  /// Invalidates cache after completion for all items (successful and failed).
  Future<List<String>> updateMultipleLocations(
    List<String> gearIds,
    String locationId,
  ) async {
    final failedIds = <String>[];
    for (final gearId in gearIds) {
      try {
        // Fetch current gear to preserve all existing fields
        final gear = await get(gearId);

        // Call saveGear with all fields preserved, only updating location
        await saveGear(
          id: gearId,
          name: gear.name,
          description: gear.description,
          mediaIds: gear.mediaIds,
          locationId: locationId,
        );
      } catch (e) {
        failedIds.add(gearId);
      }
    }

    // Invalidate cache after bulk operation
    await _cache.invalidatePattern('*');
    await _searchRepository.invalidateSearches();
    await _feedRepository.invalidateAllFeeds();
    _onDailyInvalidated?.call();

    return failedIds;
  }

  /// Gets people associated with a gear item (owner, borrowers, interested parties).
  ///
  /// If [communityId] is provided, filters results for that specific community.
  /// Results are cached with a key based on gear ID and optional community ID.
  ///
  /// Use [invalidatePeople] to force a refresh after transfers or other mutations.
  Future<GetGearPeopleResponse> getPeople(String gearId, {String? communityId}) async {
    final cacheKey = communityId != null ? 'people:$gearId:$communityId' : 'people:$gearId';
    return _cache.get(
      key: cacheKey,
      fetch: () => _service.getGearPeople(gearId: gearId, communityId: communityId),
    );
  }

  /// Invalidates the people cache for a specific gear item.
  ///
  /// If [communityId] is provided, only invalidates that community's cache.
  /// Otherwise, invalidates all people caches for this gear.
  Future<void> invalidatePeople(String gearId, {String? communityId}) async {
    if (communityId != null) {
      await _cache.invalidate('people:$gearId:$communityId');
    } else {
      await _cache.invalidatePattern('people:$gearId*');
    }
  }

  /// Gets statistics for a gear item (loans, people helped, value shared).
  ///
  /// If [communityId] is provided, returns stats specific to that community.
  /// Results are cached with a key based on gear ID and optional community ID.
  ///
  /// Use [invalidateStats] to force a refresh after transfers or other mutations.
  Future<GetGearStatsResponse> getStats(String gearId, {String? communityId}) async {
    final cacheKey = communityId != null ? 'stats:$gearId:$communityId' : 'stats:$gearId';
    return _cache.get(
      key: cacheKey,
      fetch: () => _service.getGearStats(gearId: gearId, communityId: communityId),
    );
  }

  /// Invalidates the stats cache for a specific gear item.
  ///
  /// If [communityId] is provided, only invalidates that community's cache.
  /// Otherwise, invalidates all stats caches for this gear.
  Future<void> invalidateStats(String gearId, {String? communityId}) async {
    if (communityId != null) {
      await _cache.invalidate('stats:$gearId:$communityId');
    } else {
      await _cache.invalidatePattern('stats:$gearId*');
    }
  }

  // ── Bookings (who's-using calendar) ────────────────────────────────────────
  // Bookings are a live schedule, so they are fetched fresh (not cached).

  /// Lists the who-has-it-which-days schedule for a gear item.
  Future<List<GearBooking>> listBookings({
    required String gearId,
    required String communityId,
  }) {
    return _service.listGearBookings(gearId: gearId, communityId: communityId);
  }

  /// Claims an inclusive day range. By default the caller is the recipient; the
  /// owner may reserve for [recipientId] (or themselves to block), or hold the
  /// days behind an accept link with [pending].
  Future<GearBooking> claimDays({
    required String gearId,
    required String communityId,
    required int startDateUnixSec,
    required int endDateUnixSec,
    String? recipientId,
    bool pending = false,
  }) {
    return _service.claimGearDays(
      gearId: gearId,
      communityId: communityId,
      startDateUnixSec: startDateUnixSec,
      endDateUnixSec: endDateUnixSec,
      recipientId: recipientId,
      pending: pending,
    );
  }

  /// Accepts a pending owner-created reservation via its link token.
  Future<GearBooking> acceptBooking({
    required String bookingId,
    required String acceptToken,
  }) {
    return _service.acceptGearBooking(
      bookingId: bookingId,
      acceptToken: acceptToken,
    );
  }

  /// Releases (drops) a booking the authenticated user owns.
  Future<void> releaseBooking(String bookingId) {
    return _service.releaseGearBooking(bookingId: bookingId);
  }

  /// Sets/clears the pickup + drop-off hand-off on a booking.
  Future<GearBooking> updateBookingHandoff({
    required String bookingId,
    String? pickupLocationId,
    int? pickupTimeUnixSec,
    String? dropoffLocationId,
    int? dropoffTimeUnixSec,
  }) {
    return _service.updateGearBookingHandoff(
      bookingId: bookingId,
      pickupLocationId: pickupLocationId,
      pickupTimeUnixSec: pickupTimeUnixSec,
      dropoffLocationId: dropoffLocationId,
      dropoffTimeUnixSec: dropoffTimeUnixSec,
    );
  }
}
