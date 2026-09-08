import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/profile_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/profile_service.pb.dart';

export 'package:ripls/data/gen/ripls/api/profile_presence.pb.dart'
    show
        ProfileAskCard,
        ProfileEventCard,
        ProfilePresenceFace,
        ProfileQueuedAsk,
        ProfileSheetKind;
export 'package:ripls/data/gen/ripls/api/profile_service.pb.dart'
    show
        GetProfilePresenceForViewerResponse,
        GetUserProfileForViewerResponse,
        SharedCommunityRef;

/// ProfileService handles RPC calls against the viewer-facing user
/// profile surface.
class ProfileService {
  final ProfileServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  ProfileService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  })  : _client = ProfileServiceClient(transport),
        _getAccessToken = getAccessToken,
        _errorHandler = errorHandler ?? RpcErrorHandler(),
        _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// Fetches the target user's profile rendered through the caller's
  /// viewer relationship.
  Future<GetUserProfileForViewerResponse> getUserProfileForViewer({
    required String targetUserId,
  }) {
    return RpcUtils.executeRpc(
      () async {
        final request =
            GetUserProfileForViewerRequest(targetUserId: targetUserId);
        return _client.getUserProfileForViewer(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      operationName: 'GetUserProfileForViewer',
    );
  }

  /// Fetches the server-selected pinned-sheet state for the target
  /// user's profile (active ask → next shared event → quiet, with
  /// cold-start gating).
  Future<GetProfilePresenceForViewerResponse> getProfilePresenceForViewer({
    required String targetUserId,
  }) {
    return RpcUtils.executeRpc(
      () async {
        final request =
            GetProfilePresenceForViewerRequest(targetUserId: targetUserId);
        return _client.getProfilePresenceForViewer(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      operationName: 'GetProfilePresenceForViewer',
    );
  }
}
