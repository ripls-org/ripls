import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart';

export 'package:ripls/data/gen/ripls/api/feed_service.pb.dart'
    show
        GetFeedResponse,
        GetFeedStatusResponse,
        FeedItem,
        MarkFeedItemsViewedRequest,
        GearSharedPayload,
        UnreadNotificationsPayload,
        InviterCelebrationPayload,
        FirstContributionPayload,
        LoanMilestonePayload,
        CommunityMilestonePayload,
        NudgePayload,
        NudgeStat,
        StoryPayload,
        ListStoriesResponse,
        ListItemStoriesResponse;
export 'package:ripls/data/gen/ripls/api/feed_service.pbenum.dart'
    show FeedItemType, StoryType;

/// FeedService handles feed-related operations using the FeedService API.
class FeedService {
  final FeedServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  FeedService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  })  : _client = FeedServiceClient(transport),
        _getAccessToken = getAccessToken,
        _errorHandler = errorHandler ?? RpcErrorHandler(),
        _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// GetFeed retrieves the personalized feed for one or more communities.
  ///
  /// Returns feed response with items and pagination token.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetFeedResponse> getFeed({
    required List<String> communityIds,
    int pageSize = 20,
    String? pageToken,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetFeedRequest(
          communityIds: communityIds,
          pageSize: pageSize,
          pageToken: pageToken ?? '',
        );
        final response = await _client.getFeed(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetFeed',
    );
  }

  /// MarkFeedItemsViewed marks feed items as viewed, incrementing their view counts.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> markFeedItemsViewed({
    required String communityId,
    required List<String> itemIds,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = MarkFeedItemsViewedRequest(
          communityId: communityId,
          itemIds: itemIds,
        );
        await _client.markFeedItemsViewed(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'MarkFeedItemsViewed',
    );
  }

  /// ListStories retrieves stories for a community, ordered by creation time (newest first).
  ///
  /// Returns list of story payloads with enriched participant data.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<StoryPayload>> listStories({
    required String communityId,
    int limit = 10,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListStoriesRequest(
          communityId: communityId,
          limit: limit,
        );
        final response = await _client.listStories(
          request,
          headers: _buildHeaders(),
        );

        return response.stories;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListStories',
    );
  }

  /// ListItemStories retrieves stories for a specific item (gear, request, or experience), ordered by creation time (newest first).
  ///
  /// Returns list of story payloads with enriched participant data.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<StoryPayload>> listItemStories({
    required String itemId,
    int limit = 10,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListItemStoriesRequest(
          itemId: itemId,
          limit: limit,
        );
        final response = await _client.listItemStories(
          request,
          headers: _buildHeaders(),
        );

        return response.stories;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListItemStories',
    );
  }

  /// ConsumeNudge marks a nudge as consumed when the user taps its CTA.
  ///
  /// Fire-and-forget: callers should not await this before navigating.
  /// If the call fails the nudge may reappear on the next feed load —
  /// an acceptable trade-off for smooth UX.
  Future<void> consumeNudge({
    required String nudgeId,
    required String action,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ConsumeNudgeRequest(
          nudgeId: nudgeId,
          action: action,
        );
        await _client.consumeNudge(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ConsumeNudge',
    );
  }

  /// GetFeedStatus returns whether each of the user's communities has unseen feed items.
  ///
  /// Returns a map of community_id -> true when there are new items.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetFeedStatusResponse> getFeedStatus() async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.getFeedStatus(
          GetFeedStatusRequest(),
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetFeedStatus',
    );
  }
}
