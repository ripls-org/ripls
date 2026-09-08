import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/workshop_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/workshop_service.pb.dart';

export 'package:ripls/data/gen/ripls/api/workshop_service.pb.dart'
    show GetWorkshopBriefResponse, BriefPayload;

/// WorkshopService handles Workshop tab read calls against WorkshopService RPC.
class WorkshopService {
  final WorkshopServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  WorkshopService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  })  : _client = WorkshopServiceClient(transport),
        _getAccessToken = getAccessToken,
        _errorHandler = errorHandler ?? RpcErrorHandler(),
        _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// Generate an experience draft prefilled from a prior-instance
  /// experience referenced by a Workshop action card.
  Future<GenerateWorkshopDraftResponse> generateWorkshopDraft({
    required String experienceId,
  }) {
    return RpcUtils.executeRpc(
      () async {
        final request =
            GenerateWorkshopDraftRequest(experienceId: experienceId);
        return _client.generateWorkshopDraft(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      operationName: 'GenerateWorkshopDraft',
    );
  }

  /// Get the "Together this season" synthesis panels for the host's circles.
  Future<GetWorkshopSynthesisResponse> getWorkshopSynthesis({
    required List<String> communityIds,
  }) {
    return RpcUtils.executeRpc(
      () async {
        final request = GetWorkshopSynthesisRequest(communityIds: communityIds);
        return _client.getWorkshopSynthesis(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      operationName: 'GetWorkshopSynthesis',
    );
  }

  /// Get the AI-generated daily brief for the host's circles.
  Future<GetWorkshopBriefResponse> getWorkshopBrief({
    required List<String> communityIds,
  }) {
    return RpcUtils.executeRpc(
      () async {
        final request = GetWorkshopBriefRequest(communityIds: communityIds);
        return _client.getWorkshopBrief(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      operationName: 'GetWorkshopBrief',
    );
  }

  /// Get the per-category detail payload for a "known for" chip. Both
  /// the workshop community surface and the user-profile surface call
  /// this; the [mode] discriminator picks which scope.
  Future<GetCategoryDetailResponse> getCategoryDetail({
    required KnownForMode mode,
    String? ownerId,
    required List<String> communityIds,
    required String category,
  }) {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCategoryDetailRequest(
          mode: mode,
          ownerId: ownerId,
          communityIds: communityIds,
          category: category,
        );
        return _client.getCategoryDetail(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      operationName: 'GetCategoryDetail',
    );
  }

  /// Hide a "known for" category for the given scope. Reversible
  /// via [undoHideKnownForCategory] within the snackbar undo window.
  Future<HideKnownForCategoryResponse> hideKnownForCategory({
    required SuppressionScope scopeKind,
    required String scopeId,
    required String category,
  }) {
    return RpcUtils.executeRpc(
      () async {
        final request = HideKnownForCategoryRequest(
          scopeKind: scopeKind,
          scopeId: scopeId,
          category: category,
        );
        return _client.hideKnownForCategory(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      operationName: 'HideKnownForCategory',
    );
  }

  /// Undo a recent hide by suppression-row id.
  Future<UndoHideKnownForCategoryResponse> undoHideKnownForCategory({
    required String suppressionId,
  }) {
    return RpcUtils.executeRpc(
      () async {
        final request =
            UndoHideKnownForCategoryRequest(suppressionId: suppressionId);
        return _client.undoHideKnownForCategory(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      operationName: 'UndoHideKnownForCategory',
    );
  }

  /// Permanently remove a "known for" category for the given scope.
  /// Irreversible; the client should confirm at action time.
  Future<PermanentlyRemoveKnownForCategoryResponse>
      permanentlyRemoveKnownForCategory({
    required SuppressionScope scopeKind,
    required String scopeId,
    required String category,
  }) {
    return RpcUtils.executeRpc(
      () async {
        final request = PermanentlyRemoveKnownForCategoryRequest(
          scopeKind: scopeKind,
          scopeId: scopeId,
          category: category,
        );
        return _client.permanentlyRemoveKnownForCategory(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      operationName: 'PermanentlyRemoveKnownForCategory',
    );
  }
}
