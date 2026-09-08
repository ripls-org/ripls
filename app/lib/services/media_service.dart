import 'package:connectrpc/connect.dart' as connect;
import 'package:cross_file/cross_file.dart';
import 'package:mime/mime.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/media_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;

final _log = ObservableLogger.named('MediaService');

/// MediaService handles media-related operations using the MediaService API.
class MediaService {
  final MediaServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  MediaService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  }) : _client = MediaServiceClient(transport),
       _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// AddMedia uploads media (image or video) to the server.
  ///
  /// Accepts a cross-platform [XFile] — the return type of every
  /// picker we use (`image_picker`, `camera`). `XFile.readAsBytes()`
  /// works on every Flutter target, including web (where
  /// `XFile.path` is a `blob:` URL and `dart:io File` is
  /// unavailable). For backward compatibility with a small set of
  /// callers that still pass a filesystem path (e.g. temp files
  /// written by `CameraViewport` on mobile), pass
  /// `XFile(filePath)` — the constructor is platform-agnostic.
  ///
  /// Returns the ID of the uploaded media.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<String> addMedia({
    required XFile file,
    String? description,
  }) async {
    _log.info('📤 Starting media upload - name: ${file.name}');

    return RpcUtils.executeRpc(
      () async {
        // XFile.readAsBytes works on every platform — on mobile it
        // reads from the filesystem, on web it reads from the
        // browser's Blob URL.
        final bytes = await file.readAsBytes();
        final filename = file.name;

        // Determine MIME content type. XFile exposes a best-guess
        // mimeType on web (from the picker), but it can be empty;
        // fall back to mime sniffing the header bytes plus the
        // filename extension.
        final mimeType = (file.mimeType?.isNotEmpty ?? false ? file.mimeType! : null) ??
            lookupMimeType(filename, headerBytes: bytes) ??
            'application/octet-stream';

        _log.info(
          '📁 File details - name: $filename, MIME: $mimeType, size: ${bytes.length} bytes',
        );

        final request = AddMediaRequest(
          encodedBytes: bytes,
          contentType: mimeType,
          filename: filename,
          description: description ?? '',
        );

        _log.info('🌐 Sending media to server...');
        final response = await _client.addMedia(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Media uploaded successfully! ID: ${response.id}');
        return response.id;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AddMedia',
    );
  }

  /// AddMediaFromURL imports a remote URL server-side and returns the
  /// id of the newly-created media row. Used by the Replace Media flow
  /// to swap in a streamed alternate candidate without round-tripping
  /// bytes through the device.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<String> addMediaFromUrl({
    required String url,
    String? description,
    StockImageProvider? provider,
    String? providerPhotoId,
  }) async {
    _log.info('📤 Importing media from URL: $url');

    return RpcUtils.executeRpc(
      () async {
        final request = AddMediaFromURLRequest(
          url: url,
          description: description,
          provider: provider,
          providerPhotoId: providerPhotoId,
        );
        final response = await _client.addMediaFromURL(
          request,
          headers: _buildHeaders(),
        );
        _log.info('✅ Media imported from URL! ID: ${response.id}');
        return response.id;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AddMediaFromURL',
    );
  }

  /// GetMedia retrieves media by ID.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetMediaResponse> getMedia(String id) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetMediaRequest(id: id);
        final response = await _client.getMedia(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetMedia',
    );
  }

  /// DeleteMedia deletes media by ID.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> deleteMedia(String id) async {
    _log.info('🗑️ Deleting media - ID: $id');

    return RpcUtils.executeRpc(
      () async {
        final request = DeleteMediaRequest(id: id);
        await _client.deleteMedia(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Media deleted successfully! ID: $id');
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DeleteMedia',
    );
  }
}
