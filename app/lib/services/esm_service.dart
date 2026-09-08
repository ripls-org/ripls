import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/esm_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/esm_service.pb.dart';

export 'package:ripls/data/gen/ripls/api/esm_service.pb.dart'
    show RespondToESMPromptResponse;

/// EsmService handles client-side calls to the ESMService RPC.
class EsmService {
  final ESMServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  EsmService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  })  : _client = ESMServiceClient(transport),
        _getAccessToken = getAccessToken,
        _errorHandler = errorHandler ?? RpcErrorHandler(),
        _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// Submit a structured response to an ESM prompt.
  Future<RespondToESMPromptResponse> respondToESMPrompt({
    required String promptId,
    required String responseOptionKey,
  }) {
    return RpcUtils.executeRpc(
      () async {
        final request = RespondToESMPromptRequest(
          promptId: promptId,
          responseOptionKey: responseOptionKey,
        );
        return _client.respondToESMPrompt(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      operationName: 'RespondToESMPrompt',
    );
  }
}
