import 'package:connectrpc/connect.dart' as connect;
import 'package:fixnum/fixnum.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/unified_create_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart';

final _log = Logger('UnifiedCreateService');

/// Thin client for the UnifiedCreateService RPC. Mirrors the pattern in
/// GearService / ExperienceService: build headers, dispatch the
/// streaming call, surface events to the caller.
class UnifiedCreateService {
  UnifiedCreateService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    Future<void> Function()? onUnauthenticated,
  })  : _client = UnifiedCreateServiceClient(transport),
        _getAccessToken = getAccessToken,
        _onUnauthenticated = onUnauthenticated;

  final UnifiedCreateServiceClient _client;
  final String? Function() _getAccessToken;
  final Future<void> Function()? _onUnauthenticated;

  // Unused for now but mirrors the gear/experience services so future
  // unary-with-error-handling RPCs can reuse the same shape.
  // ignore: unused_field
  final RpcErrorHandler _errorHandler = RpcErrorHandler();

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// Streams a unified-create generation. Errors from the transport
  /// surface through the returned stream; callers must handle onError on
  /// the subscription. When [forceType] is set, the server skips the
  /// classifier and runs the per-type generator directly.
  Stream<StreamGenUnifiedCreateResponse> streamGenUnifiedCreate({
    String? text,
    String? mediaId,
    String? websiteUrl,
    DetectedContentType? forceType,
    String? locationId,
    double? latitudeDeg,
    double? longitudeDeg,
    int? currentTimeUnixSec,
    String? timezone,
  }) {
    final request = StreamGenUnifiedCreateRequest();
    if (text != null && text.isNotEmpty) {
      request.text = text;
    } else if (mediaId != null && mediaId.isNotEmpty) {
      request.mediaId = mediaId;
    } else if (websiteUrl != null && websiteUrl.isNotEmpty) {
      request.websiteUrl = websiteUrl;
    }
    if (forceType != null) {
      request.forceType = forceType;
    }
    if (locationId != null && locationId.isNotEmpty) {
      request.locationId = locationId;
    }
    if (latitudeDeg != null) {
      request.latitudeDeg = latitudeDeg;
    }
    if (longitudeDeg != null) {
      request.longitudeDeg = longitudeDeg;
    }
    request.currentTimeUnixSec = Int64(
      currentTimeUnixSec ?? DateTime.now().millisecondsSinceEpoch ~/ 1000,
    );
    if (timezone != null && timezone.isNotEmpty) {
      request.timezone = timezone;
    }
    _log.info(
      'streamGenUnifiedCreate dispatch '
      'has_text=${text?.isNotEmpty ?? false} '
      'has_media=${mediaId?.isNotEmpty ?? false} '
      'has_url=${websiteUrl?.isNotEmpty ?? false} '
      'force_type=${forceType?.name} '
      'location_id=$locationId '
      'tz=$timezone',
    );
    return _client.streamGenUnifiedCreate(request, headers: _buildHeaders());
  }
}
