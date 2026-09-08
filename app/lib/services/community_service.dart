import 'package:connectrpc/connect.dart' as connect;
import 'package:fixnum/fixnum.dart';
import 'package:flutter/foundation.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show HostInviteConsent, Invitee;
import 'package:ripls/data/gen/ripls/api/community_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/provisional_user.pb.dart'
    show ProvisionalUser;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;
export 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show HostInviteConsent, Invitee;
export 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show
        CommunityItem,
        GetCommunityResponse,
        CommunityGearItem,
        CommunityEventItem,
        GetOrCreateShareLinkResponse,
        AcceptInvitationLinkResponse,
        GenCommunityResponse,
        StreamGenCommunityRequest,
        StreamGenCommunityResponse,
        StreamGenCommunityResponse_Event,
        CommunityRegionItem,
        RegionFilter,
        GetProvisionalUserInviteLinkResponse,
        CommunityMember,
        ProvisionalUserActivityItem,
        GetProvisionalUserActivitiesResponse,
        StreamUserEventsResponse,
        CommunityEventType,
        CommunityNotificationPreferences,
        DeletedCommunityItem,
        GetCommunityPresenceForViewerResponse,
        RejoinableCommunityItem;
export 'package:ripls/data/gen/ripls/api/community_service.pbenum.dart'
    show CommunityEventType;
export 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
export 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart'
    show GenStreamError, GenStreamErrorCode, MediaReady;
export 'package:ripls/data/gen/ripls/api/provisional_user.pb.dart'
    show ProvisionalUser;
export 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;

part 'community/event_methods.dart';

final _log = ObservableLogger.named('CommunityService');

/// Outcome of a [CommunityService.shareItem] call: the provisioned ad-hoc
/// audience community (when individuals were invited) and its shareable open
/// link (used to host-relay phone invites). Both are null when only existing
/// communities were shared to. [canManageAudience] is the server's verdict on
/// whether the caller may manage the item's audience (invite people, add
/// communities) — false for members who may only reshare the link (#2630).
class ShareItemResult {
  final String? adhocCommunityId;
  final String? shareUrl;
  final bool canManageAudience;

  const ShareItemResult({
    this.adhocCommunityId,
    this.shareUrl,
    this.canManageAudience = false,
  });
}

/// CommunityServiceBase declares the RPC client and request-building helpers
/// the method-group mixins depend on. Mirrors ExperienceServiceBase.
///
/// The method groups have to be mixins rather than extensions: Mockito
/// generates a mock from a class's members, and extension methods are not
/// members — mocking one silently dispatches to the real implementation.
abstract class CommunityServiceBase {
  CommunityServiceClient get _client;
  RpcErrorHandler get _errorHandler;
  Future<void> Function()? get _onUnauthenticated;

  connect.Headers _buildHeaders();
}

class CommunityService extends CommunityServiceBase
    with CommunityEventMethods {
  @override
  final CommunityServiceClient _client;
  final String? Function() _getAccessToken;
  @override
  final RpcErrorHandler _errorHandler;
  @override
  final Future<void> Function()? _onUnauthenticated;

  CommunityService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  }) : _client = CommunityServiceClient(transport),
       _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _onUnauthenticated = onUnauthenticated;

  @override
  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// ListCommunities retrieves all communities the user is a member of.
  ///
  /// Optional [regionFilter] filters communities by region.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<CommunityItem>> listCommunities({
    RegionFilter? regionFilter,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListCommunitiesRequest(regionFilter: regionFilter);
        final response = await _client.listCommunities(
          request,
          headers: _buildHeaders(),
        );

        return response.communities;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListCommunities',
    );
  }

  /// CreateCommunity creates a new community.
  ///
  /// Returns the ID of the created community.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<String> createCommunity({
    required String name,
    required String description,
    List<String>? mediaIds,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = CreateCommunityRequest(
          name: name,
          description: description,
          mediaIds: mediaIds ?? [],
        );
        final response = await _client.createCommunity(
          request,
          headers: _buildHeaders(),
        );

        return response.id;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CreateCommunity',
    );
  }

  /// StreamGenCommunity emits a `media_ready` event when a background
  /// image has been found, then a terminal `final` or `error`. Errors
  /// surface through the returned stream's `onError`.
  Stream<StreamGenCommunityResponse> streamGenCommunity({
    String? prompt,
    String? region,
  }) {
    final request = StreamGenCommunityRequest(
      prompt: prompt ?? '',
      region: region ?? '',
    );
    return _client.streamGenCommunity(request, headers: _buildHeaders());
  }

  /// GetCommunity retrieves a specific community by ID.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetCommunityResponse> getCommunity(String id) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCommunityRequest(id: id);
        final response = await _client.getCommunity(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunity',
    );
  }

  /// GetCommunityPresenceForViewer fetches the server-selected
  /// pinned-sheet state for the community profile (winning ask +
  /// queue, else next event, else quiet).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetCommunityPresenceForViewerResponse> getCommunityPresenceForViewer(
      String communityId) async {
    return RpcUtils.executeRpc(
      () async {
        final request =
            GetCommunityPresenceForViewerRequest(communityId: communityId);
        return _client.getCommunityPresenceForViewer(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunityPresenceForViewer',
    );
  }

  /// UpdateCommunity updates an existing community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> updateCommunity({
    required String id,
    String? name,
    String? description,
    List<String>? mediaIds,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UpdateCommunityRequest(
          id: id,
          name: name ?? '',
          description: description ?? '',
          mediaIds: mediaIds ?? [],
        );
        await _client.updateCommunity(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UpdateCommunity',
    );
  }

  /// DeleteCommunity deletes a community by ID.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> deleteCommunity(String id) async {
    return RpcUtils.executeRpc(
      () async {
        final request = DeleteCommunityRequest(id: id);
        await _client.deleteCommunity(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DeleteCommunity',
    );
  }

  /// RestoreCommunity restores a previously soft-deleted community. The
  /// caller becomes the new owner. Returns `FAILED_PRECONDITION` if the
  /// community is no longer in deleted state, and `PERMISSION_DENIED` if
  /// the caller is not in the eligible-restorer snapshot.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> restoreCommunity(String communityId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = RestoreCommunityRequest(communityId: communityId);
        await _client.restoreCommunity(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RestoreCommunity',
    );
  }

  /// ListDeletedCommunitiesForRestore returns the soft-deleted communities
  /// the caller is eligible to restore. Server returns items ordered
  /// ascending by deleted_at_unix_sec — oldest deletion first, so the
  /// most-urgent entries appear at the top.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<DeletedCommunityItem>> listDeletedCommunitiesForRestore() async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.listDeletedCommunitiesForRestore(
          ListDeletedCommunitiesForRestoreRequest(),
          headers: _buildHeaders(),
        );
        return response.communities;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListDeletedCommunitiesForRestore',
    );
  }

  /// RejoinCommunity un-soft-deletes the caller's CommunityUser row in
  /// the given community. Allowed when the caller has a soft-deleted
  /// membership less than 30 days old AND the community itself is
  /// active.
  ///
  /// Returns `FAILED_PRECONDITION` for one of four documented reasons,
  /// distinguished by a suffix on the error message:
  /// `: no_membership_to_rejoin`, `: already_member`,
  /// `: rejoin_window_expired`, `: community_deleted`.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> rejoinCommunity(String communityId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = RejoinCommunityRequest(communityId: communityId);
        await _client.rejoinCommunity(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RejoinCommunity',
    );
  }

  /// ListRejoinableCommunities returns the active communities the caller
  /// can rejoin without a fresh invite — one entry per soft-deleted
  /// CommunityUser row less than 30 days old whose community is still
  /// active. Server returns items ordered descending by left_at_unix_sec
  /// (most-recently-left first).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<RejoinableCommunityItem>> listRejoinableCommunities() async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.listRejoinableCommunities(
          ListRejoinableCommunitiesRequest(),
          headers: _buildHeaders(),
        );
        return response.communities;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListRejoinableCommunities',
    );
  }

  /// GetOrCreateShareLink gets or creates a shareable link for a community
  /// or a specific item within it. The target oneof identifies what the
  /// link points at; at most one of [gearId], [transferId], [requestId],
  /// or [experienceId] should be set. When all are null, the link points
  /// at the community itself (plain community-invite).
  ///
  /// Returns the typed share URL, short code, member counts, and the
  /// community the link is scoped to.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetOrCreateShareLinkResponse> getOrCreateShareLink({
    required String communityId,
    String? gearId,
    String? transferId,
    String? requestId,
    String? experienceId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetOrCreateShareLinkRequest(communityId: communityId);
        if (gearId != null && gearId.isNotEmpty) {
          request.gearId = gearId;
        } else if (transferId != null && transferId.isNotEmpty) {
          request.transferId = transferId;
        } else if (requestId != null && requestId.isNotEmpty) {
          request.requestId = requestId;
        } else if (experienceId != null && experienceId.isNotEmpty) {
          request.experienceId = experienceId;
        } else {
          request.communityInvite = communityId;
        }
        final response = await _client.getOrCreateShareLink(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetOrCreateShareLink',
    );
  }

  /// Revokes a shareable link by its short code.
  ///
  /// The caller must be the link's inviter. Idempotent — revoking an
  /// already-revoked link returns success.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> revokeShareLink({required String shortCode}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = RevokeShareLinkRequest(shortCode: shortCode);
        await _client.revokeShareLink(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RevokeShareLink',
    );
  }

  /// AcceptInvitationLink accepts an invitation link to join a community.
  ///
  /// Returns the community ID and name.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<AcceptInvitationLinkResponse> acceptInvitationLink({
    required String shortCode,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = AcceptInvitationLinkRequest(
          shortCode: shortCode,
        );
        final response = await _client.acceptInvitationLink(
          request,
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AcceptInvitationLink',
    );
  }

  /// LeaveCommunity allows a user to leave a community.
  ///
  /// When the caller is the community owner, [newOwnerUserId] must
  /// identify another active member who will become the new owner —
  /// the server's race-safe handoff path is exercised. For non-owners
  /// the parameter is ignored server-side; pass null.
  ///
  /// Negative paths surface as `connect.CodeFailedPrecondition` /
  /// `connect.CodeInvalidArgument` with reason discriminators in the
  /// error message: `: missing_new_owner_user_id`,
  /// `: candidate_is_caller`, `: candidate_not_member`,
  /// `: ownership_changed`. See `server/services/community/membership.go`.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> leaveCommunity(
    String communityId, {
    String? newOwnerUserId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = LeaveCommunityRequest(
          communityId: communityId,
          newOwnerUserId: newOwnerUserId,
        );
        await _client.leaveCommunity(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'LeaveCommunity',
    );
  }

  /// ListCommunityGear retrieves gear shared with a community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<CommunityGearItem>> listCommunityGear(String communityId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListCommunityGearRequest(communityId: communityId);
        final response = await _client.listCommunityGear(
          request,
          headers: _buildHeaders(),
        );

        return response.gearItems;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListCommunityGear',
    );
  }

  /// ListCompletedGiveaways retrieves completed giveaways in a community.
  ///
  /// Returns gear that was given away, with original owner information.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<CommunityGearItem>> listCompletedGiveaways(
    String communityId,
  ) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListCompletedGiveawaysRequest(communityId: communityId);
        final response = await _client.listCompletedGiveaways(
          request,
          headers: _buildHeaders(),
        );

        return response.gearItems;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListCompletedGiveaways',
    );
  }

  /// ShareGear shares gear with a community.
  ///
  /// Converged onto [shareItem] (CommunityService.ShareItem, #2526). Availability
  /// is item-wide (#2492/#2687) and inherited server-side, so the per-community
  /// [availability] is no longer sent; the parameter is retained for source
  /// compatibility with existing callers and drops out when they do.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> shareGear({
    required String gearId,
    required String communityId,
    required Availability availability,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ShareItemRequest(shareToCommunityIds: [communityId])
          ..gearId = gearId;
        await _client.shareItem(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ShareItem',
    );
  }

  /// SetGearAvailability sets an item's availability (loan vs giveaway) across
  /// every community it is shared with, in one call. Where [shareGear] /
  /// UpdateGearSharing change one community's setting, this changes the item's
  /// mode everywhere at once. Rejected server-side while the item has an
  /// in-progress loan or giveaway.
  ///
  /// Throws [ServiceException] with a user-friendly message on failure.
  Future<void> setGearAvailability({
    required String gearId,
    required Availability availability,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SetGearAvailabilityRequest(
          gearId: gearId,
          availability: availability,
        );
        await _client.setGearAvailability(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SetGearAvailability',
    );
  }

  /// ShareItem adds to an item's per-item community: it invites individuals
  /// (members/phone/email via [invitees]) and/or shares the item to existing
  /// communities (via [shareToCommunityIds]), and always returns the item's
  /// per-item community ([ShareItemResult.adhocCommunityId]) + its open link
  /// ([ShareItemResult.shareUrl]). The item's per-item community is provisioned
  /// at creation for events/requests, and on first ShareItem for gear (#2492).
  /// Provide exactly one of [experienceId] / [gearId] / [requestId]. [consent] is
  /// required when any phone invitee is present.
  Future<ShareItemResult> shareItem({
    String? experienceId,
    String? gearId,
    String? requestId,
    List<Invitee> invitees = const [],
    HostInviteConsent? consent,
    List<String> shareToCommunityIds = const [],
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ShareItemRequest(
          invitees: invitees,
          shareToCommunityIds: shareToCommunityIds,
        );
        if (experienceId != null && experienceId.isNotEmpty) {
          request.experienceId = experienceId;
        } else if (gearId != null && gearId.isNotEmpty) {
          request.gearId = gearId;
        } else if (requestId != null && requestId.isNotEmpty) {
          request.requestId = requestId;
        } else {
          throw ArgumentError(
              'shareItem requires one of experienceId, gearId, or requestId');
        }
        if (consent != null) {
          request.inviteConsent = consent;
        }
        final response = await _client.shareItem(
          request,
          headers: _buildHeaders(),
        );
        return ShareItemResult(
          adhocCommunityId: response.hasAdhocCommunityId()
              ? response.adhocCommunityId
              : null,
          shareUrl: response.hasShareUrl() ? response.shareUrl : null,
          canManageAudience: response.hasCanManageAudience() &&
              response.canManageAudience,
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ShareItem',
    );
  }

  /// UnshareGear unshares gear from a community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> unshareGear({
    required String gearId,
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UnshareItemRequest(communityId: communityId)
          ..gearId = gearId;
        await _client.unshareItem(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UnshareItem',
    );
  }

  /// ListCommunityUsers retrieves users in a community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<CommunityMember>> listCommunityUsers(String communityId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListCommunityUsersRequest(communityId: communityId);
        final response = await _client.listCommunityUsers(
          request,
          headers: _buildHeaders(),
        );

        return response.members;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListCommunityUsers',
    );
  }

  /// SearchCommunityUsers searches community members by display name using fuzzy matching.
  ///
  /// Returns up to [limit] users whose names match [query] (default 10, max 50).
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<User>> searchCommunityUsers({
    required String communityId,
    required String query,
    int limit = 10,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SearchCommunityUsersRequest(
          communityId: communityId,
          query: query,
          limit: limit,
        );
        final response = await _client.searchCommunityUsers(
          request,
          headers: _buildHeaders(),
        );

        return response.users;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SearchCommunityUsers',
    );
  }

  /// Returns true if [e] represents a clean server-side close of an event
  /// stream surfaced as an HTTP/2 INTERNAL_ERROR frame.
  ///
  /// The server-side stream handler returns nil at its self-imposed lifetime
  /// cap. Connect-Dart sometimes surfaces that as a `ConnectException` with
  /// `Code.internal` and a message matching the specific HTTP/2 "stream closed
  /// with error code INTERNAL_ERROR" pattern instead of completing the
  /// underlying Stream cleanly.
  @visibleForTesting
  static bool isBenignStreamClose(connect.ConnectException e) {
    return e.code == connect.Code.internal &&
        e.message.contains(
          'http/2 stream closed with error code INTERNAL_ERROR',
        );
  }

  /// ListCommunityRegions retrieves all regions associated with a community.
  ///
  /// Returns a list of regions with their types, names, codes, member percentages,
  /// and whether they are manual overrides or auto-computed.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<CommunityRegionItem>> listCommunityRegions(
    String communityId,
  ) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListCommunityRegionsRequest(communityId: communityId);
        final response = await _client.listCommunityRegions(
          request,
          headers: _buildHeaders(),
        );

        return response.regions;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListCommunityRegions',
    );
  }

  /// SetCommunityRegionOverride manually sets a region override for a community.
  ///
  /// This allows community creators to manually specify a region instead of
  /// relying on automatic computation from member locations.
  ///
  /// [regionId] references a normalized region (see LocationService.searchRegions).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> setCommunityRegionOverride({
    required String communityId,
    required String regionId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SetCommunityRegionOverrideRequest(
          communityId: communityId,
          regionId: regionId,
        );
        await _client.setCommunityRegionOverride(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SetCommunityRegionOverride',
    );
  }

  /// CreateProvisionalUser creates a lightweight placeholder account for a non-registered participant.
  ///
  /// Provisional users are community-scoped and can be referenced in activities.
  /// They merge with real accounts when the person later registers.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ProvisionalUser> createProvisionalUser({
    required String communityId,
    required String name,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = CreateProvisionalUserRequest(
          communityId: communityId,
          name: name,
        );
        final response = await _client.createProvisionalUser(
          request,
          headers: _buildHeaders(),
        );

        return response.provisionalUser;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CreateProvisionalUser',
    );
  }

  /// ListProvisionalUsers lists all provisional users in a community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<ProvisionalUser>> listProvisionalUsers({
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListProvisionalUsersRequest(communityId: communityId);
        final response = await _client.listProvisionalUsers(
          request,
          headers: _buildHeaders(),
        );

        return response.provisionalUsers;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListProvisionalUsers',
    );
  }

  /// SearchProvisionalUsers searches provisional users in a community by name.
  ///
  /// Returns up to [limit] provisional users whose names match [query] (default 10, max 50).
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<ProvisionalUser>> searchProvisionalUsers({
    required String communityId,
    required String query,
    int limit = 10,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SearchProvisionalUsersRequest(
          communityId: communityId,
          query: query,
          limit: limit,
        );
        final response = await _client.searchProvisionalUsers(
          request,
          headers: _buildHeaders(),
        );

        return response.provisionalUsers;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SearchProvisionalUsers',
    );
  }

  /// GetProvisionalUserActivities returns attended experiences for a provisional user.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetProvisionalUserActivitiesResponse> getProvisionalUserActivities({
    required String communityId,
    required String provisionalUserId,
    int limit = 20,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetProvisionalUserActivitiesRequest(
          communityId: communityId,
          provisionalUserId: provisionalUserId,
          limit: limit,
        );
        final response = await _client.getProvisionalUserActivities(
          request,
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetProvisionalUserActivities',
    );
  }

  /// GetProvisionalUserInviteLink gets or creates an invite link tied to a provisional user.
  ///
  /// When the person registers using this link, the provisional account is merged
  /// into their new real account.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetProvisionalUserInviteLinkResponse> getProvisionalUserInviteLink({
    required String communityId,
    required String provisionalUserId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetProvisionalUserInviteLinkRequest(
          communityId: communityId,
          provisionalUserId: provisionalUserId,
        );
        final response = await _client.getProvisionalUserInviteLink(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetProvisionalUserInviteLink',
    );
  }

  /// Reads the calling user's per-category notification preferences for a
  /// community. Unset toggles in the response mean the category is enabled —
  /// callers must treat unset as "on".
  Future<CommunityNotificationPreferences> getNotificationPreferences(
    String communityId,
  ) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCommunityNotificationPreferencesRequest(
          communityId: communityId,
        );
        final response = await _client.getCommunityNotificationPreferences(
          request,
          headers: _buildHeaders(),
        );
        return response.preferences;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunityNotificationPreferences',
    );
  }

  /// Persists the calling user's per-category notification preferences for a
  /// community. Returns the persisted message.
  Future<CommunityNotificationPreferences> updateNotificationPreferences({
    required String communityId,
    required CommunityNotificationPreferences preferences,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UpdateCommunityNotificationPreferencesRequest(
          communityId: communityId,
          preferences: preferences,
        );
        final response = await _client.updateCommunityNotificationPreferences(
          request,
          headers: _buildHeaders(),
        );
        return response.preferences;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UpdateCommunityNotificationPreferences',
    );
  }
}
