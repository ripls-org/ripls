import 'package:connectrpc/connect.dart' as connect;
import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/community_service.connect.client.dart'
    show CommunityServiceClient;
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show ShareItemRequest, UnshareItemRequest;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate, MoneySavings, PreventedEmissions, QualityTimeAttributes;
import 'package:ripls/data/gen/ripls/api/request.pb.dart'
    show Request, RequestState, RequestNeedResponse, RequestContributionResponse;
import 'package:ripls/data/gen/ripls/api/request_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/social.pb.dart' show SocialContext;

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;
export 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart'
    show GenStreamError, GenStreamErrorCode, MediaReady;
export 'package:ripls/data/gen/ripls/api/request.pb.dart'
    show RequestNeedResponse, RequestContributionResponse;
export 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show
        RequestMetadata,
        ListRequestNeedsAndContributionsResponse,
        RequestBatchNeedItem,
        StreamGenRequestRequest,
        StreamGenRequestResponse,
        StreamGenRequestResponse_Event;

final _log = Logger('RequestService');

/// RequestService handles request-related operations using the RequestService API.
class RequestService {
  final RequestServiceClient _client;
  // Community-service client used for the converged sharing RPCs (ShareItem /
  // UnshareItem, #2526): request audience management lives on CommunityService.
  final CommunityServiceClient _communityClient;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  RequestService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  }) : _client = RequestServiceClient(transport),
       _communityClient = CommunityServiceClient(transport),
       _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// GenRequest generates AI suggestions for a request from a user prompt.
  /// StreamGenRequest is the streaming variant. Provide either [prompt] (text
  /// mode) or [mediaId] (image mode), not both. Emits title / geocoded /
  /// media_ready events as each resolves, then a terminal `final` or
  /// `error`. Errors surface through the returned stream's `onError`.
  Stream<StreamGenRequestResponse> streamGenRequest({
    String? prompt,
    String? mediaId,
    String? communityId,
    String? locationId,
    double? latitudeDeg,
    double? longitudeDeg,
  }) {
    final request = StreamGenRequestRequest(
      prompt: prompt ?? '',
      mediaId: mediaId ?? '',
      communityId: communityId ?? '',
      locationId: locationId ?? '',
    );
    if (latitudeDeg != null && longitudeDeg != null) {
      request.latitudeDeg = latitudeDeg;
      request.longitudeDeg = longitudeDeg;
    }
    return _client.streamGenRequest(request, headers: _buildHeaders());
  }

  /// SubmitRequest creates a new request. The request is always created in its
  /// own per-item community (#2492). To also share it into existing
  /// communities, call [shareRequest] (CommunityService.ShareItem) with the
  /// returned request ID.
  ///
  /// Returns the request ID.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<String> submitRequest({
    required String title,
    required String description,
    List<String>? mediaIds,
    String? locationId,
    RequestMetadata? metadata,
    int? neededByUnixSec,
    List<String>? seedNeedNames,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Submitting request');

        final request = SubmitRequestRequest(
          title: title,
          description: description,
          mediaIds: mediaIds ?? [],
          locationId: locationId ?? '',
        );

        if (metadata != null) {
          request.metadata = metadata;
        }

        if (neededByUnixSec != null) {
          request.neededByUnixSec = $fixnum.Int64(neededByUnixSec);
        }

        if (seedNeedNames != null) {
          request.seedNeedNames.addAll(
            seedNeedNames.where((n) => n.trim().isNotEmpty),
          );
        }

        _log.info('🌐 Sending SubmitRequest RPC...');
        final response = await _client.submitRequest(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Request submitted! Request ID: ${response.requestId}');
        return response.requestId;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SubmitRequest',
    );
  }

  /// AcceptRequestOffer marks a gear-backed offer as the one the requester wants
  /// ("this one works"). Requester only; toggle semantics — accepting an
  /// already-accepted offer clears it. Returns the updated Request.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<Request> acceptRequestOffer({
    required String requestId,
    required String contributionId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Toggling gear-offer acceptance on request: $requestId');
        final response = await _client.acceptRequestOffer(
          AcceptRequestOfferRequest(
            requestId: requestId,
            contributionId: contributionId,
          ),
          headers: _buildHeaders(),
        );
        return response.request;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AcceptRequestOffer',
    );
  }

  /// OfferToFulfill expresses interest in fulfilling a request.
  ///
  /// Returns the full Request object with all context including conversation ID.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<Request> offerToFulfill({
    required String requestId,
    required String communityId,
  }) async {
    // Validate required parameters with clear error messages
    if (communityId.isEmpty) {
      _log.severe('offerToFulfill called with empty communityId');
      throw ServiceException(
        'Unable to offer help: no community context. Please try again.',
      );
    }

    if (requestId.isEmpty) {
      _log.severe('offerToFulfill called with empty requestId');
      throw ServiceException(
        'Unable to offer help: invalid request.',
      );
    }

    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Offering to fulfill request: $requestId in community: $communityId');

        final request = OfferToFulfillRequest(
          requestId: requestId,
          communityId: communityId,
        );

        _log.info('🌐 Sending OfferToFulfill RPC...');
        final response = await _client.offerToFulfill(
          request,
          headers: _buildHeaders(),
        );

        _log.info(
          '✅ Offer submitted! Request ID: ${response.request.id}, Conversation ID: ${response.request.conversationId}',
        );
        return response.request;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'OfferToFulfill',
    );
  }

  /// MarkRequestFulfilled marks a request as fulfilled.
  ///
  /// The requester calls this when their need has been satisfied.
  /// Optionally accepts a [resolutionSummary] (AI-generated or manually entered).
  /// Marks a request as fulfilled.
  ///
  /// Returns the response including updated request and savings metrics.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<MarkRequestFulfilledResponse> markRequestFulfilled({
    required String requestId,
    String? resolutionSummary,
    List<String> confirmedHelperIds = const [],
    int? confirmedHelperCount,
    int? fulfilledAtUnixSec,
    QualityTimeAttributes? qualityTimeOverrides,
    MoneySavings? moneySavingsOverrides,
    PreventedEmissions? emissionsOverrides,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Marking request as fulfilled: $requestId');

        final request = MarkRequestFulfilledRequest(
          requestId: requestId,
          resolutionSummary: resolutionSummary,
          confirmedHelperIds: confirmedHelperIds,
          // Same total the fulfillment modal previewed with (registered +
          // provisional helpers) so group size matches at commit (#2724).
          confirmedHelperCount: confirmedHelperCount,
          fulfilledAtUnixSec: fulfilledAtUnixSec != null
              ? $fixnum.Int64(fulfilledAtUnixSec)
              : null,
        );
        if (qualityTimeOverrides != null) {
          request.qualityTimeOverrides = qualityTimeOverrides;
        }
        if (moneySavingsOverrides != null) {
          request.moneySavingsOverrides = moneySavingsOverrides;
        }
        if (emissionsOverrides != null) {
          request.emissionsOverrides = emissionsOverrides;
        }

        _log.info('🌐 Sending MarkRequestFulfilled RPC...');
        final response = await _client.markRequestFulfilled(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Request marked as fulfilled!');
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'MarkRequestFulfilled',
    );
  }

  /// PreviewRequestImpact computes a live impact estimate for the fulfillment
  /// modal as helpers are toggled. Read-only — does not persist.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ImpactEstimate> previewRequestImpact({
    required String requestId,
    List<String> confirmedHelperIds = const [],
    int confirmedHelperCount = 0,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = PreviewRequestImpactRequest(
          requestId: requestId,
          confirmedHelperCount: confirmedHelperCount,
        );
        request.confirmedHelperIds.addAll(confirmedHelperIds);
        final response = await _client.previewRequestImpact(
          request,
          headers: _buildHeaders(),
        );
        return response.impact;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'PreviewRequestImpact',
    );
  }

  /// CancelRequest cancels a request.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  /// Returns the community_event_id of the cancel so the caller can
  /// wire an undo snackbar.
  Future<String> cancelRequest({required String requestId}) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Cancelling request: $requestId');

        final request = CancelRequestRequest(requestId: requestId);

        _log.info('🌐 Sending CancelRequest RPC...');
        final response =
            await _client.cancelRequest(request, headers: _buildHeaders());

        _log.info('✅ Request cancelled!');
        return response.communityEventId;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CancelRequest',
    );
  }

  /// Undo a prior MarkRequestFulfilled call. Throws [ServiceException]
  /// when the action cannot be reversed; the underlying
  /// ConnectException carries an UndoErrorDetail for typed handling.
  Future<void> undoMarkRequestFulfilled({
    required String communityEventId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UndoMarkRequestFulfilledRequest(
          communityEventId: communityEventId,
        );
        await _client.undoMarkRequestFulfilled(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UndoMarkRequestFulfilled',
    );
  }

  /// Undo a prior CancelRequest call.
  Future<void> undoCancelRequest({required String communityEventId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UndoCancelRequestRequest(
          communityEventId: communityEventId,
        );
        await _client.undoCancelRequest(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UndoCancelRequest',
    );
  }

  /// DeleteRequest permanently deletes a request.
  ///
  /// Only the request creator can delete their own requests.
  /// The request will no longer appear in listings or be accessible.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> deleteRequest({required String requestId}) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Deleting request: $requestId');

        final request = DeleteRequestRequest(requestId: requestId);

        _log.info('🌐 Sending DeleteRequest RPC...');
        await _client.deleteRequest(request, headers: _buildHeaders());

        _log.info('✅ Request deleted!');
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DeleteRequest',
    );
  }

  /// UpdateRequest updates the description of an active request.
  ///
  /// Pass [socialContext] to override LLM-inferred social attribute tiers used
  /// for Quality Time computation. Only [mediaIds] must be sent when saving
  /// social context (to prevent the server from clearing existing media).
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> updateRequest({
    required String requestId,
    String? title,
    String? description,
    List<String>? mediaIds,
    String? locationId,
    SocialContext? socialContext,
    int? neededByUnixSec,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Updating request: $requestId');

        final request = UpdateRequestRequest(
          requestId: requestId,
          title: title ?? '',
          description: description ?? '',
          mediaIds: mediaIds ?? [],
          locationId: locationId ?? '',
        );
        if (socialContext != null) {
          request.socialContext = socialContext;
        }
        // A positive value schedules the request; zero explicitly clears it.
        if (neededByUnixSec != null) {
          request.neededByUnixSec = $fixnum.Int64(neededByUnixSec);
        }

        _log.info('🌐 Sending UpdateRequest RPC...');
        await _client.updateRequest(request, headers: _buildHeaders());

        _log.info('✅ Request updated!');
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UpdateRequest',
    );
  }

  /// GetRequest retrieves details of a specific request.
  ///
  /// Returns the request item.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<Request> getRequest({
    required String requestId,
    String? communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Getting request: $requestId');

        final request = GetRequestRequest(
          requestId: requestId,
          communityId: communityId ?? '',
        );

        _log.info('🌐 Sending GetRequest RPC...');
        final response = await _client.getRequest(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Request retrieved!');
        return response.request;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetRequest',
    );
  }

  /// ListRequests lists requests in a community with optional filtering.
  ///
  /// Returns a list of request items.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<Request>> listRequests({
    required String communityId,
    RequestState? state,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Listing requests for community: $communityId');

        final request = ListRequestsRequest(
          communityId: communityId,
          state: state ?? RequestState.REQUEST_STATE_UNSPECIFIED,
        );

        _log.info('🌐 Sending ListRequests RPC...');
        final response = await _client.listRequests(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Retrieved ${response.requests.length} requests');
        return response.requests;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListRequests',
    );
  }

  /// ListMyRequests lists all requests created by the calling user.
  ///
  /// Returns a list of request items.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<Request>> listMyRequests({String? communityId}) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Listing my requests');

        final request = ListMyRequestsRequest(communityId: communityId ?? '');

        _log.info('🌐 Sending ListMyRequests RPC...');
        final response = await _client.listMyRequests(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Retrieved ${response.requests.length} requests');
        return response.requests;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListMyRequests',
    );
  }

  /// WithdrawOffer withdraws the user's offer to fulfill a request.
  ///
  /// Removes the user from the offerers list and the conversation.
  /// If this was the last offerer, request returns to ACTIVE state.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> withdrawOffer({
    required String requestId,
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Withdrawing offer from request: $requestId');

        final request = WithdrawOfferRequest(
          requestId: requestId,
          communityId: communityId,
        );

        _log.info('🌐 Sending WithdrawOffer RPC...');
        await _client.withdrawOffer(request, headers: _buildHeaders());

        _log.info('✅ Offer withdrawn!');
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'WithdrawOffer',
    );
  }

  /// ShareRequest shares an existing request with additional communities.
  ///
  /// The request creator can share their request with any communities they belong to.
  /// Returns the updated Request with all shared_community_ids populated.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<Request> shareRequest({
    required String requestId,
    required List<String> communityIds,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        // Converged onto CommunityService.ShareItem (#2526).
        final shareReq = ShareItemRequest(shareToCommunityIds: communityIds)
          ..requestId = requestId;
        await _communityClient.shareItem(shareReq, headers: _buildHeaders());

        // ShareItem returns audience info, not the request; refetch so callers
        // still get the updated Request (its shared_community_ids).
        final getResp = await _client.getRequest(
          GetRequestRequest(requestId: requestId),
          headers: _buildHeaders(),
        );
        return getResp.request;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ShareItem',
    );
  }

  /// UnshareRequest removes a request from a specific community.
  ///
  /// The request creator can unshare from any community except the last one.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> unshareRequest({
    required String requestId,
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        // Converged onto CommunityService.UnshareItem (#2526).
        final request = UnshareItemRequest(communityId: communityId)
          ..requestId = requestId;
        await _communityClient.unshareItem(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UnshareItem',
    );
  }

  /// GetRequestPeople retrieves people associated with a request.
  ///
  /// Returns requester, current helper, past helpers, and offerers.
  /// If [communityId] is provided, filters results for that specific community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetRequestPeopleResponse> getRequestPeople({
    required String requestId,
    String? communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetRequestPeopleRequest(
          requestId: requestId,
          communityId: communityId ?? '',
        );
        final response = await _client.getRequestPeople(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetRequestPeople',
    );
  }

  /// GetRequestStats retrieves statistics for a request.
  ///
  /// Returns offers received, times fulfilled, help value, and days open.
  /// If [communityId] is provided, returns stats specific to that community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetRequestStatsResponse> getRequestStats({
    required String requestId,
    String? communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetRequestStatsRequest(
          requestId: requestId,
          communityId: communityId ?? '',
        );
        final response = await _client.getRequestStats(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetRequestStats',
    );
  }

  /// ListRequestNeedsAndContributions returns all needs and contributions for a request.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ListRequestNeedsAndContributionsResponse> listRequestNeedsAndContributions({
    required String requestId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.listRequestNeedsAndContributions(
          ListRequestNeedsAndContributionsRequest(requestId: requestId),
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListRequestNeedsAndContributions',
    );
  }

  /// AddRequestNeed adds a planning need to a request (requester only).
  ///
  /// [slots] defaults to 1 when omitted or zero.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<RequestNeedResponse> addRequestNeed({
    required String requestId,
    required String name,
    String? note,
    int slots = 1,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final req = AddRequestNeedRequest(
          requestId: requestId,
          name: name,
          slots: slots,
        );
        if (note != null) req.note = note;
        final response = await _client.addRequestNeed(req, headers: _buildHeaders());
        return response.need;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AddRequestNeed',
    );
  }

  /// RemoveRequestNeed removes a need from a request (proposer only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> removeRequestNeed({
    required String needId,
    required String requestId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        await _client.removeRequestNeed(
          RemoveRequestNeedRequest(needId: needId, requestId: requestId),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RemoveRequestNeed',
    );
  }

  /// UpdateRequestNeed edits an existing need in place (proposer only).
  /// Reducing slots below the current claim count fails server-side
  /// with FailedPrecondition.
  Future<RequestNeedResponse> updateRequestNeed({
    required String needId,
    required String requestId,
    String? name,
    String? note,
    int? slots,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UpdateRequestNeedRequest(
          needId: needId,
          requestId: requestId,
        );
        if (name != null) request.name = name;
        if (note != null) request.note = note;
        if (slots != null) request.slots = slots;
        final response = await _client.updateRequestNeed(
          request,
          headers: _buildHeaders(),
        );
        return response.need;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UpdateRequestNeed',
    );
  }

  /// NudgeUncoveredRequestNeedClaimers pings existing RequestOffer
  /// members who haven't yet created a contribution (requester only).
  /// Returns the count of users notified.
  Future<int> nudgeUncoveredRequestNeedClaimers({
    required String requestId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.nudgeUncoveredRequestNeedClaimers(
          NudgeUncoveredRequestNeedClaimersRequest(requestId: requestId),
          headers: _buildHeaders(),
        );
        return response.nudgedCount;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'NudgeUncoveredRequestNeedClaimers',
    );
  }

  /// BatchAddRequestNeeds adds multiple needs to a request in a single call (requester only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<RequestNeedResponse>> batchAddRequestNeeds({
    required String requestId,
    required List<RequestBatchNeedItem> items,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.batchAddRequestNeeds(
          BatchAddRequestNeedsRequest(requestId: requestId, items: items),
          headers: _buildHeaders(),
        );
        return response.needs;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'BatchAddRequestNeeds',
    );
  }

  /// ClaimRequestNeed claims a need slot, creating a linked contribution.
  ///
  /// Automatically creates a RequestOffer if one does not exist.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<RequestContributionResponse> claimRequestNeed({
    required String needId,
    required String requestId,
    required String communityId,
    String? note,
    String? gearId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final req = ClaimRequestNeedRequest(
          needId: needId,
          requestId: requestId,
          communityId: communityId,
        );
        if (note != null) req.note = note;
        if (gearId != null && gearId.isNotEmpty) req.gearId = gearId;
        final response = await _client.claimRequestNeed(req, headers: _buildHeaders());
        return response.contribution;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ClaimRequestNeed',
    );
  }

  /// BatchClaimRequestNeeds claims slots on multiple needs in one call.
  ///
  /// Used by the compose flow to apply the requester's "I've got this"
  /// pre-claims after a Request is published. Returns per-claim results
  /// so the caller can render partial-success state.
  ///
  /// Throws [ServiceException] only on transport / auth failures. A
  /// per-claim failure does NOT throw — it surfaces as an entry in the
  /// returned list with `errorMessage` set.
  Future<List<BatchClaimRequestNeedResult>> batchClaimRequestNeeds({
    required String requestId,
    required List<BatchClaimRequestNeedItem> claims,
    String communityId = '',
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.batchClaimRequestNeeds(
          BatchClaimRequestNeedsRequest(
            requestId: requestId,
            communityId: communityId,
            claims: claims,
          ),
          headers: _buildHeaders(),
        );
        return response.results;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'BatchClaimRequestNeeds',
    );
  }

  /// UnclaimRequestNeed returns a claimed contribution slot (contributor only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> unclaimRequestNeed({
    required String contributionId,
    required String requestId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        await _client.unclaimRequestNeed(
          UnclaimRequestNeedRequest(
            contributionId: contributionId,
            requestId: requestId,
          ),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UnclaimRequestNeed',
    );
  }

  /// AddRequestContribution adds a free-form contribution to a request (any helper).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<RequestContributionResponse> addRequestContribution({
    required String requestId,
    required String title,
    String? description,
    String? gearId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final req = AddRequestContributionRequest(
          requestId: requestId,
          title: title,
        );
        if (description != null) req.description = description;
        if (gearId != null && gearId.isNotEmpty) req.gearId = gearId;
        final response = await _client.addRequestContribution(req, headers: _buildHeaders());
        return response.contribution;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AddRequestContribution',
    );
  }

  /// EditRequestContribution updates an existing contribution (contributor only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<RequestContributionResponse> editRequestContribution({
    required String contributionId,
    required String requestId,
    required String title,
    String? description,
    String? gearId,
    bool clearGearId = false,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final req = EditRequestContributionRequest(
          contributionId: contributionId,
          requestId: requestId,
          title: title,
        );
        if (description != null) req.description = description;
        if (clearGearId) {
          req.clearGearId();
        } else if (gearId != null && gearId.isNotEmpty) {
          req.gearId = gearId;
        }
        final response = await _client.editRequestContribution(req, headers: _buildHeaders());
        return response.contribution;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'EditRequestContribution',
    );
  }

  /// RemoveRequestContribution removes a contribution (contributor only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> removeRequestContribution({
    required String contributionId,
    required String requestId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        await _client.removeRequestContribution(
          RemoveRequestContributionRequest(
            contributionId: contributionId,
            requestId: requestId,
          ),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RemoveRequestContribution',
    );
  }
}
