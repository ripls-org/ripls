import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/feedback_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/feedback_service.pb.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;

/// FeedbackService handles user feedback submission to GitHub.
class FeedbackService {
  final FeedbackServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  FeedbackService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  }) : _client = FeedbackServiceClient(transport),
       _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// SubmitFeedback submits user feedback (bug report or feature request).
  ///
  /// Returns the response with GitHub issue URL and number.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<SubmitFeedbackResponse> submitFeedback(
    SubmitFeedbackRequest request,
  ) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.submitFeedback(
          request,
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SubmitFeedback',
    );
  }
}
