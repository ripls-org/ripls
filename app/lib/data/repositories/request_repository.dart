import 'package:flutter/foundation.dart' show VoidCallback;
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate, MoneySavings, PreventedEmissions, QualityTimeAttributes;
import 'package:ripls/data/gen/ripls/api/request.pb.dart'
    show Request, RequestState, RequestNeedResponse, RequestContributionResponse;
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show
        BatchClaimRequestNeedItem,
        BatchClaimRequestNeedResult,
        GetRequestPeopleResponse,
        GetRequestStatsResponse,
        ListRequestNeedsAndContributionsResponse,
        MarkRequestFulfilledResponse,
        RequestBatchNeedItem,
        RequestMetadata;
import 'package:ripls/data/gen/ripls/api/social.pb.dart' show SocialContext;
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/services/request_service.dart';

export 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart'
    show GenStreamError, GenStreamErrorCode, MediaReady;
// Export types that ViewModels and widgets need.
export 'package:ripls/data/gen/ripls/api/request.pb.dart'
    show RequestNeedResponse, RequestContributionResponse;
export 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show
        ListRequestNeedsAndContributionsResponse,
        RequestBatchNeedItem,
        StreamGenRequestRequest,
        StreamGenRequestResponse,
        StreamGenRequestResponse_Event;

/// Repository for request data with transparent caching.
///
/// This repository wraps RequestService and provides caching for request
/// data using the global TTL from environment (CACHE_TTL_MINUTES).
class RequestRepository {
  final CacheService _cache;
  final RequestService _service;
  final ChatRepository _chatRepository;
  final FeedRepository _feedRepository;
  final SearchRepository _searchRepository;
  final VoidCallback? _onDailyInvalidated;
  final VoidCallback? _onProfileInvalidated;

  RequestRepository(
    CacheManager cacheManager,
    this._service,
    this._chatRepository,
    this._feedRepository,
    this._searchRepository, {
    VoidCallback? onDailyInvalidated,
    VoidCallback? onProfileInvalidated,
  })  : _cache = CacheService(cacheManager, 'request'),
        _onDailyInvalidated = onDailyInvalidated,
        _onProfileInvalidated = onProfileInvalidated;

  /// Lists requests in a community with optional filtering.
  ///
  /// Optionally filter by [state].
  /// Use [refreshRequests] to force a refresh.
  Future<List<Request>> listRequests({
    required String communityId,
    RequestState? state,
  }) async {
    final listKey = _buildListKey('community:$communityId', state);

    return _cache.getList(
      listKey: listKey,
      fetch: () => _service.listRequests(
        communityId: communityId,
        state: state,
      ),
    );
  }

  /// Lists all requests created by the calling user.
  ///
  /// Optionally filter by [communityId].
  /// Use [refreshMyRequests] to force a refresh.
  Future<List<Request>> listMyRequests({
    String? communityId,
  }) async {
    final listKey = communityId != null ? 'my:$communityId' : 'my';

    return _cache.getList(
      listKey: listKey,
      fetch: () => _service.listMyRequests(
        communityId: communityId,
      ),
    );
  }

  /// Gets a specific request by ID.
  ///
  /// Use [refreshRequest] to force a refresh.
  Future<Request> getRequest({
    required String requestId,
    String? communityId,
  }) async {
    final cacheKey = communityId != null ? '$requestId:$communityId' : requestId;
    return _cache.get(
      key: cacheKey,
      fetch: () => _service.getRequest(
        requestId: requestId,
        communityId: communityId,
      ),
    );
  }

  /// Refreshes the requests cache for a community by invalidating it.
  ///
  /// Call this after any request modifications to ensure the next fetch gets fresh data.
  Future<List<Request>> refreshRequests({
    required String communityId,
    RequestState? state,
  }) async {
    await _cache.invalidatePattern('community:$communityId:*');
    return listRequests(communityId: communityId, state: state);
  }

  /// Refreshes the my requests cache by invalidating it.
  ///
  /// Call this after creating, updating, or canceling requests.
  Future<List<Request>> refreshMyRequests({
    String? communityId,
  }) async {
    await _cache.invalidatePattern('my:*');
    return listMyRequests(communityId: communityId);
  }

  /// Refreshes a specific request by invalidating its cache.
  ///
  /// Call this after any mutations to ensure the next fetch gets fresh data.
  Future<Request> refreshRequest({
    required String requestId,
  }) async {
    await invalidate(requestId);
    return getRequest(requestId: requestId);
  }

  /// Invalidates the cache for a specific request without refetching.
  ///
  /// Use this when you know the request has been modified externally
  /// (e.g., media uploaded in a conversation) and want to invalidate
  /// the cache without immediately fetching fresh data.
  Future<void> invalidate(String requestId) async {
    await _cache.invalidate(requestId);
    await _cache.invalidatePattern('$requestId:*');
  }

  /// Invalidates every cached entry scoped to a single community.
  ///
  /// Use this after a community-level lifecycle change (e.g. restoring
  /// a previously soft-deleted community) so the next read fetches
  /// the freshly un-soft-deleted requests rather than the cached
  /// "no requests visible" snapshot from the deleted period.
  Future<void> invalidateForCommunity(String communityId) async {
    await _cache.invalidatePattern('community:$communityId:*');
  }

  /// Generates AI suggestions for a request from a user prompt.
  ///
  /// This does NOT save to the database - it only returns AI-generated
  /// suggestions for preview. Call [submitRequest] to persist.
  ///
  /// StreamGenRequest is the streaming variant. Provide either [prompt] (text
  /// mode) or [mediaId] (image mode), not both. Emits title / geocoded /
  /// media_ready events as they resolve, then a terminal `final` or `error`.
  /// Generation is one-shot and unique per prompt/media; cache-bypass.
  Stream<StreamGenRequestResponse> streamGenRequest({
    String? prompt,
    String? mediaId,
    String? communityId,
    String? locationId,
    double? latitudeDeg,
    double? longitudeDeg,
  }) {
    return _service.streamGenRequest(
      prompt: prompt,
      mediaId: mediaId,
      communityId: communityId,
      locationId: locationId,
      latitudeDeg: latitudeDeg,
      longitudeDeg: longitudeDeg,
    );
  }


  /// Submits a new request for an item in a community.
  ///
  /// Returns the request ID.
  /// Automatically invalidates relevant caches including feed cache and search cache.
  Future<String> submitRequest({
    required String title,
    required String description,
    List<String>? mediaIds,
    String? locationId,
    RequestMetadata? metadata,
    int? neededByUnixSec,
    List<String>? seedNeedNames,
  }) async {
    final requestId = await _service.submitRequest(
      title: title,
      description: description,
      mediaIds: mediaIds,
      locationId: locationId,
      metadata: metadata,
      neededByUnixSec: neededByUnixSec,
      seedNeedNames: seedNeedNames,
    );

    // Invalidate my requests cache since we just created a request. The
    // request lands in its own freshly-created per-item community, so there is
    // no existing community list/search cache to invalidate here — sharing into
    // existing communities goes through shareRequest, which invalidates theirs.
    await _cache.invalidatePattern('my:*');
    // Invalidate feed cache since new request will appear in feed
    await _feedRepository.invalidateFeed();
    _onDailyInvalidated?.call();

    return requestId;
  }

  /// Offers to fulfill a request.
  ///
  /// Returns the full Request object with all context including conversation ID.
  /// Automatically invalidates relevant caches.
  Future<Request> offerToFulfill({
    required String requestId,
    required String communityId,
  }) async {
    final request = await _service.offerToFulfill(
      requestId: requestId,
      communityId: communityId,
    );

    // Invalidate request item cache since offer count changed
    await invalidate(requestId);
    // Invalidate community lists since state may have changed
    await _cache.invalidatePattern('community:*');
    // Invalidate conversations cache since a new conversation was created or joined
    await _chatRepository.refreshConversations();
    _onDailyInvalidated?.call();

    return request;
  }

  /// Accepts (or un-accepts — toggle semantics) a gear-backed offer on the
  /// caller's request ("this one works", #2702). Requester only.
  ///
  /// Returns the updated Request including the new acceptance state on its
  /// gear offers. Automatically invalidates the request item cache.
  Future<Request> acceptRequestOffer({
    required String requestId,
    required String contributionId,
  }) async {
    final request = await _service.acceptRequestOffer(
      requestId: requestId,
      contributionId: contributionId,
    );

    // Invalidate the request item cache so the acceptance state refreshes.
    await invalidate(requestId);
    _onDailyInvalidated?.call();

    return request;
  }

  /// Marks a request as fulfilled.
  ///
  /// The requester calls this when their need has been satisfied.
  /// Marks a request as fulfilled.
  ///
  /// Optionally accepts a [resolutionSummary] (AI-generated or manually entered)
  /// and [confirmedHelperIds] (list of user IDs who were main helpers).
  /// Returns the response including updated request and savings metrics.
  /// Automatically invalidates relevant caches including feed cache and stats cache.
  Future<MarkRequestFulfilledResponse> markRequestFulfilled({
    required String requestId,
    String? resolutionSummary,
    List<String>? confirmedHelperIds,
    int? confirmedHelperCount,
    int? fulfilledAtUnixSec,
    QualityTimeAttributes? qualityTimeOverrides,
    MoneySavings? moneySavingsOverrides,
    PreventedEmissions? emissionsOverrides,
  }) async {
    final response = await _service.markRequestFulfilled(
      requestId: requestId,
      resolutionSummary: resolutionSummary,
      confirmedHelperIds: confirmedHelperIds ?? [],
      confirmedHelperCount: confirmedHelperCount,
      fulfilledAtUnixSec: fulfilledAtUnixSec,
      qualityTimeOverrides: qualityTimeOverrides,
      moneySavingsOverrides: moneySavingsOverrides,
      emissionsOverrides: emissionsOverrides,
    );

    // Invalidate all caches since request is now fulfilled
    await invalidate(requestId);
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('community:*');
    // Invalidate all feed caches since fulfilled status should update in feed
    await _feedRepository.invalidateAllFeeds();
    // Invalidate stats cache since fulfillment calculates new savings
    await invalidateStats(requestId);
    _onDailyInvalidated?.call();

    return response;
  }

  /// Reverses a prior [markRequestFulfilled] using the community_event_id it
  /// returned. Mirrors the fulfillment's cache invalidations (including the
  /// daily/home and feed refreshes) so the reverted state reflects everywhere —
  /// the Needs-you decision re-surfaces rather than staying dismissed on the
  /// home view.
  Future<void> undoMarkRequestFulfilled({
    required String communityEventId,
  }) async {
    await _service.undoMarkRequestFulfilled(communityEventId: communityEventId);
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('community:*');
    await _feedRepository.invalidateAllFeeds();
    _onDailyInvalidated?.call();
  }

  /// PreviewRequestImpact computes a live impact estimate for the fulfillment
  /// modal as helpers are toggled. Read-only — does not persist.
  ///
  /// Bypasses cache: helper-toggle previews must reflect the live count, not
  /// a snapshot. Mirrors `previewExperienceImpact` on the experience side.
  Future<ImpactEstimate> previewRequestImpact({
    required String requestId,
    required List<String> confirmedHelperIds,
    required int confirmedHelperCount,
  }) async {
    return _service.previewRequestImpact(
      requestId: requestId,
      confirmedHelperIds: confirmedHelperIds,
      confirmedHelperCount: confirmedHelperCount,
    );
  }

  /// Cancels a request. Returns the community_event_id for undo wiring.
  ///
  /// Automatically invalidates relevant caches.
  Future<String> cancelRequest({
    required String requestId,
  }) async {
    final communityEventId =
        await _service.cancelRequest(requestId: requestId);

    // Invalidate all caches since request is now cancelled
    await invalidate(requestId);
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('community:*');
    return communityEventId;
  }

  /// Withdraws the user's offer to fulfill a request.
  ///
  /// Removes the user from the offerers list and the conversation.
  /// If this was the last offerer, request returns to ACTIVE state.
  /// Automatically invalidates relevant caches.
  Future<void> withdrawOffer({
    required String requestId,
    required String communityId,
  }) async {
    await _service.withdrawOffer(
      requestId: requestId,
      communityId: communityId,
    );

    // Invalidate request cache since offerer list changed
    await invalidate(requestId);
    // Invalidate community lists since state may have changed
    await _cache.invalidatePattern('community:*');
    // Invalidate conversations cache since user was removed from conversation
    await _chatRepository.refreshConversations();
  }

  /// Permanently deletes a request.
  ///
  /// Only the request creator can delete their own requests.
  /// The request will no longer appear in listings or be accessible.
  /// The associated conversation is also deleted on the server.
  /// Automatically invalidates relevant caches including feed, search,
  /// conversation, and profile caches.
  Future<void> deleteRequest({
    required String requestId,
  }) async {
    await _service.deleteRequest(requestId: requestId);

    // Invalidate all caches since request is now deleted
    await invalidate(requestId);
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('community:*');
    // Invalidate all feed caches since deleted request should disappear from feed
    await _feedRepository.invalidateAllFeeds();
    // Invalidate search cache so deleted request disappears from discover
    await _searchRepository.invalidateSearches();
    // Invalidate conversations cache since associated conversation was cascade-deleted
    await _chatRepository.refreshConversations();
    // Invalidate profile caches so profile surfaces drop the deleted request
    _onProfileInvalidated?.call();
  }

  /// Updates the description of an active request.
  ///
  /// Pass [socialContext] to override LLM-inferred social attribute tiers.
  /// When only social context is being saved, pass [mediaIds] to prevent
  /// the server from clearing existing media.
  /// Automatically invalidates relevant caches including search cache.
  Future<void> updateRequest({
    required String requestId,
    String? title,
    String? description,
    List<String>? mediaIds,
    String? locationId,
    SocialContext? socialContext,
    int? neededByUnixSec,
  }) async {
    await _service.updateRequest(
      requestId: requestId,
      title: title,
      description: description,
      mediaIds: mediaIds,
      locationId: locationId,
      socialContext: socialContext,
      neededByUnixSec: neededByUnixSec,
    );

    // Invalidate all caches since request was updated.
    await invalidate(requestId);
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('community:*');

    // Invalidate search cache so updates appear in discover screen
    await _searchRepository.invalidateSearches();
  }

  /// Shares a request with additional communities.
  ///
  /// Returns the updated Request with all shared_community_ids populated.
  /// Automatically invalidates relevant caches.
  Future<Request> shareRequest({
    required String requestId,
    required List<String> communityIds,
  }) async {
    final request = await _service.shareRequest(
      requestId: requestId,
      communityIds: communityIds,
    );

    // Invalidate request item cache
    await invalidate(requestId);
    // Invalidate list caches for all affected communities
    for (final communityId in communityIds) {
      await _cache.invalidatePattern('community:$communityId:*');
    }
    // Invalidate "my requests" cache
    await _cache.invalidatePattern('my:*');
    // Invalidate feed caches for affected communities
    await _feedRepository.invalidateFeed();
    // Invalidate search caches for affected communities
    for (final communityId in communityIds) {
      await _searchRepository.invalidateSearchesForCommunity(communityId);
    }

    return request;
  }

  /// Unshares a request from a specific community.
  ///
  /// Removes the request from that community's feed.
  /// Automatically invalidates relevant caches.
  Future<void> unshareRequest({
    required String requestId,
    required String communityId,
  }) async {
    await _service.unshareRequest(
      requestId: requestId,
      communityId: communityId,
    );

    // Invalidate caches
    await invalidate(requestId);
    await _cache.invalidatePattern('community:$communityId:*');
    await _cache.invalidatePattern('my:*');
    // Invalidate feed cache for the community
    await _feedRepository.invalidateFeed();
    // Invalidate search cache for the community
    await _searchRepository.invalidateSearchesForCommunity(communityId);
  }

  /// Shares multiple requests with a community.
  ///
  /// Executes sequential API calls and handles partial failures gracefully.
  /// Returns a list of request IDs that failed to share.
  /// Invalidates cache after completion for all items (successful and failed).
  Future<List<String>> shareMultipleWithCommunity(
    List<String> requestIds,
    String communityId,
  ) async {
    final failedIds = <String>[];
    for (final requestId in requestIds) {
      try {
        await _service.shareRequest(
          requestId: requestId,
          communityIds: [communityId],
        );
      } catch (e) {
        failedIds.add(requestId);
      }
    }

    // Invalidate cache after bulk operation
    await _cache.invalidatePattern('*');
    await _searchRepository.invalidateSearches();
    await _feedRepository.invalidateAllFeeds();

    return failedIds;
  }

  /// Unshares multiple requests from a community.
  ///
  /// Executes sequential API calls and handles partial failures gracefully.
  /// Returns a list of request IDs that failed to unshare.
  /// Invalidates cache after completion for all items (successful and failed).
  Future<List<String>> unshareMultipleFromCommunity(
    List<String> requestIds,
    String communityId,
  ) async {
    final failedIds = <String>[];
    for (final requestId in requestIds) {
      try {
        await _service.unshareRequest(
          requestId: requestId,
          communityId: communityId,
        );
      } catch (e) {
        failedIds.add(requestId);
      }
    }

    // Invalidate cache after bulk operation
    await _cache.invalidatePattern('*');
    await _searchRepository.invalidateSearches();
    await _feedRepository.invalidateAllFeeds();

    return failedIds;
  }

  /// Updates location for multiple requests.
  ///
  /// Executes sequential API calls and handles partial failures gracefully.
  /// Fetches each item first to preserve all existing fields (title, description, mediaIds).
  /// Returns a list of request IDs that failed to update.
  /// Invalidates cache after completion for all items (successful and failed).
  Future<List<String>> updateMultipleLocations(
    List<String> requestIds,
    String locationId,
  ) async {
    final failedIds = <String>[];
    for (final requestId in requestIds) {
      try {
        // Fetch current request to preserve all existing fields
        final request = await getRequest(requestId: requestId);

        // Call updateRequest with all fields preserved, only updating location
        await updateRequest(
          requestId: requestId,
          title: request.title,
          description: request.description,
          mediaIds: request.mediaIds,
          locationId: locationId,
        );
      } catch (e) {
        failedIds.add(requestId);
      }
    }

    // Invalidate cache after bulk operation
    await _cache.invalidatePattern('*');
    await _searchRepository.invalidateSearches();
    await _feedRepository.invalidateAllFeeds();

    return failedIds;
  }

  /// Invalidates all cached request data.
  Future<void> invalidateAll() async {
    await _cache.invalidateAll();
  }

  /// Gets people associated with a request (requester, helpers, offerers).
  ///
  /// If [communityId] is provided, filters results for that specific community.
  /// Results are cached with a key based on request ID and optional community ID.
  ///
  /// Use [invalidatePeople] to force a refresh after offers or other mutations.
  Future<GetRequestPeopleResponse> getPeople(String requestId, {String? communityId}) async {
    final cacheKey = communityId != null ? 'people:$requestId:$communityId' : 'people:$requestId';
    return _cache.get(
      key: cacheKey,
      fetch: () => _service.getRequestPeople(requestId: requestId, communityId: communityId),
    );
  }

  /// Invalidates the people cache for a specific request.
  ///
  /// If [communityId] is provided, only invalidates that community's cache.
  /// Otherwise, invalidates all people caches for this request.
  Future<void> invalidatePeople(String requestId, {String? communityId}) async {
    if (communityId != null) {
      await _cache.invalidate('people:$requestId:$communityId');
    } else {
      await _cache.invalidatePattern('people:$requestId*');
    }
  }

  /// Gets statistics for a request (offers, fulfillments, help value, days open).
  ///
  /// If [communityId] is provided, returns stats specific to that community.
  /// Results are cached with a key based on request ID and optional community ID.
  ///
  /// Use [invalidateStats] to force a refresh after offers or fulfillments.
  Future<GetRequestStatsResponse> getStats(String requestId, {String? communityId}) async {
    final cacheKey = communityId != null ? 'stats:$requestId:$communityId' : 'stats:$requestId';
    return _cache.get(
      key: cacheKey,
      fetch: () => _service.getRequestStats(requestId: requestId, communityId: communityId),
    );
  }

  /// Invalidates the stats cache for a specific request.
  ///
  /// If [communityId] is provided, only invalidates that community's cache.
  /// Otherwise, invalidates all stats caches for this request.
  Future<void> invalidateStats(String requestId, {String? communityId}) async {
    if (communityId != null) {
      await _cache.invalidate('stats:$requestId:$communityId');
    } else {
      await _cache.invalidatePattern('stats:$requestId*');
    }
  }

  /// Lists all needs and contributions for a request.
  ///
  /// Results are cached under [needs_and_contributions:<requestId>].
  /// Call [refreshNeedsAndContributions] after any mutation.
  Future<ListRequestNeedsAndContributionsResponse> listNeedsAndContributions(
    String requestId,
  ) async {
    return _cache.get(
      key: 'needs_and_contributions:$requestId',
      fetch: () => _service.listRequestNeedsAndContributions(requestId: requestId),
    );
  }

  /// Invalidates and re-fetches needs and contributions for a request.
  Future<ListRequestNeedsAndContributionsResponse> refreshNeedsAndContributions(
    String requestId,
  ) async {
    await _cache.invalidate('needs_and_contributions:$requestId');
    return listNeedsAndContributions(requestId);
  }

  /// Adds a planning need to a request (requester only).
  ///
  /// Invalidates the needs/contributions cache.
  Future<RequestNeedResponse> addNeed({
    required String requestId,
    required String name,
    String? note,
    int slots = 1,
  }) async {
    final need = await _service.addRequestNeed(
      requestId: requestId,
      name: name,
      note: note,
      slots: slots,
    );
    await _cache.invalidate('needs_and_contributions:$requestId');
    return need;
  }

  /// Removes a need from a request (proposer only).
  ///
  /// Invalidates the needs/contributions cache.
  Future<void> removeNeed({
    required String needId,
    required String requestId,
  }) async {
    await _service.removeRequestNeed(needId: needId, requestId: requestId);
    await _cache.invalidate('needs_and_contributions:$requestId');
  }

  /// Updates an existing need's name, note, or slot count in place
  /// (proposer only).
  Future<RequestNeedResponse> updateNeed({
    required String needId,
    required String requestId,
    String? name,
    String? note,
    int? slots,
  }) async {
    final need = await _service.updateRequestNeed(
      needId: needId,
      requestId: requestId,
      name: name,
      note: note,
      slots: slots,
    );
    await _cache.invalidate('needs_and_contributions:$requestId');
    return need;
  }

  /// Nudges existing RequestOffer members who haven't created a
  /// contribution yet (requester only). Returns the number of users
  /// notified.
  Future<int> nudgeUncoveredNeedClaimers({
    required String requestId,
  }) async {
    return _service.nudgeUncoveredRequestNeedClaimers(requestId: requestId);
  }

  /// Adds multiple needs to a request in a single call (requester only).
  ///
  /// Invalidates the needs/contributions cache.
  Future<List<RequestNeedResponse>> addNeedsBatch({
    required String requestId,
    required List<RequestBatchNeedItem> items,
  }) async {
    final needs = await _service.batchAddRequestNeeds(requestId: requestId, items: items);
    await _cache.invalidate('needs_and_contributions:$requestId');
    return needs;
  }

  /// Claims a need slot, creating a linked contribution.
  ///
  /// Automatically creates a RequestOffer if one does not exist.
  /// Invalidates the needs/contributions cache.
  Future<RequestContributionResponse> claimNeed({
    required String needId,
    required String requestId,
    required String communityId,
    String? note,
    String? gearId,
  }) async {
    final contribution = await _service.claimRequestNeed(
      needId: needId,
      requestId: requestId,
      communityId: communityId,
      note: note,
      gearId: gearId,
    );
    await _cache.invalidate('needs_and_contributions:$requestId');
    return contribution;
  }

  /// Claims slots on multiple needs in one call. Used by the compose
  /// flow to apply the requester's pre-claims after publishing.
  ///
  /// Returns per-claim results so the caller can render partial-success
  /// state. A per-claim failure does NOT throw — it surfaces as an
  /// entry with `errorMessage` set.
  ///
  /// Invalidates the needs/contributions cache when at least one claim
  /// succeeded.
  Future<List<BatchClaimRequestNeedResult>> batchClaimNeeds({
    required String requestId,
    required List<BatchClaimRequestNeedItem> claims,
    String communityId = '',
  }) async {
    final results = await _service.batchClaimRequestNeeds(
      requestId: requestId,
      claims: claims,
      communityId: communityId,
    );
    final anySuccess = results.any((r) => !r.hasErrorMessage());
    if (anySuccess) {
      await _cache.invalidate('needs_and_contributions:$requestId');
    }
    return results;
  }

  /// Returns a claimed contribution slot (contributor only).
  ///
  /// Invalidates the needs/contributions cache.
  Future<void> unclaimNeed({
    required String contributionId,
    required String requestId,
  }) async {
    await _service.unclaimRequestNeed(contributionId: contributionId, requestId: requestId);
    await _cache.invalidate('needs_and_contributions:$requestId');
  }

  /// Adds a free-form contribution to a request (any helper).
  ///
  /// Invalidates the needs/contributions cache.
  Future<RequestContributionResponse> addContribution({
    required String requestId,
    required String title,
    String? description,
    String? gearId,
  }) async {
    final contribution = await _service.addRequestContribution(
      requestId: requestId,
      title: title,
      description: description,
      gearId: gearId,
    );
    await _cache.invalidate('needs_and_contributions:$requestId');
    return contribution;
  }

  /// Updates an existing contribution (contributor only).
  ///
  /// Invalidates the needs/contributions cache.
  Future<RequestContributionResponse> editContribution({
    required String contributionId,
    required String requestId,
    required String title,
    String? description,
    String? gearId,
    bool clearGearId = false,
  }) async {
    final contribution = await _service.editRequestContribution(
      contributionId: contributionId,
      requestId: requestId,
      title: title,
      description: description,
      gearId: gearId,
      clearGearId: clearGearId,
    );
    await _cache.invalidate('needs_and_contributions:$requestId');
    return contribution;
  }

  /// Removes a contribution (contributor only).
  ///
  /// Invalidates the needs/contributions cache.
  Future<void> removeContribution({
    required String contributionId,
    required String requestId,
  }) async {
    await _service.removeRequestContribution(
      contributionId: contributionId,
      requestId: requestId,
    );
    await _cache.invalidate('needs_and_contributions:$requestId');
  }

  // Helper method to build consistent cache keys
  String _buildListKey(String prefix, RequestState? state) {
    final parts = [prefix];

    if (state != null && state != RequestState.REQUEST_STATE_UNSPECIFIED) {
      parts.add(state.name);
    }

    return parts.join(':');
  }
}
