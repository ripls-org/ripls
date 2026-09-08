import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('MediaHelpers');

/// Media utility helpers for ViewModels.
///
/// These helpers provide common media-related operations that ViewModels
/// need, eliminating code duplication across multiple ViewModels.
///
/// IMPORTANT: These utilities should NOT be called directly from widgets.
/// ViewModels must provide wrapper methods to maintain proper MVVM layering.
class MediaHelpers {
  /// Gets media URL for a given media ID via MediaRepository.
  ///
  /// The repository handles caching, so this is just a thin wrapper.
  /// Returns the MediaUrl object containing both URL and cache key.
  ///
  /// Usage in ViewModels (required for MVVM architecture):
  /// ```dart
  /// /// Gets media URL for a given media ID via MediaRepository.
  /// ///
  /// /// The repository handles caching, so this is just a thin but necessary
  /// /// wrapper to maintain MVVM architecture - widgets should call ViewModels,
  /// /// not repositories or utilities directly.
  /// Future<MediaUrl> getMediaUrl(String mediaId) =>
  ///     MediaHelpers.getMediaUrl(ref, mediaId);
  /// ```
  static Future<MediaUrl> getMediaUrl(Ref ref, String mediaId) async {
    final mediaRepository = ref.read(mediaRepositoryProvider);
    return mediaRepository.getMediaUrl(mediaId);
  }

  /// Loads all media items in parallel from the MediaRepository.
  ///
  /// Fetches each media ID concurrently using Future.wait. Items that fail
  /// to load are logged and dropped; the returned list contains only the
  /// successfully fetched items.
  /// batchUploadFiles uploads a list of files sequentially with optimistic
  /// placeholder management delegated to the caller via callbacks.
  ///
  /// For each file, uploads via [MediaRepository] and fetches the resolved
  /// [MediaItemData]. Calls [onSuccess] with the temp ID and resolved item on
  /// success, or [onFailure] with the temp ID on error. Stops early if
  /// [isMounted] returns false between iterations.
  ///
  /// Returns a record with the successfully uploaded IDs and the failure count.
  static Future<({List<String> uploadedIds, int failedCount})> batchUploadFiles({
    required Ref ref,
    required List<XFile> files,
    required List<String> tempIds,
    required bool Function() isMounted,
    required void Function(String tempId, MediaItemData item) onSuccess,
    required void Function(String tempId) onFailure,
    String? description,
    String entityLabel = 'media',
  }) async {
    final mediaRepository = ref.read(mediaRepositoryProvider);
    final uploadedIds = <String>[];
    var failedCount = 0;

    for (var i = 0; i < files.length; i++) {
      if (!isMounted()) break;
      final tempId = tempIds[i];
      final file = files[i];

      try {
        final mediaId = await mediaRepository.addMedia(
          file: file,
          description: description,
        );
        if (!isMounted()) break;
        final mediaResponse = await mediaRepository.getMedia(mediaId);
        if (!isMounted()) break;

        final item = MediaItemData(
          id: mediaId,
          url: mediaResponse.url,
          contentType: mediaResponse.contentType,
          isVideo: mediaResponse.contentType.startsWith('video/'),
        );
        onSuccess(tempId, item);
        uploadedIds.add(mediaId);
        _log.info(
          '✅ Batch $entityLabel upload ${i + 1}/${files.length}: $mediaId',
        );
      } catch (e, stackTrace) {
        failedCount++;
        _log.severe(
          '❌ Batch $entityLabel upload ${i + 1}/${files.length} failed',
          e,
          stackTrace,
        );
        if (!isMounted()) break;
        onFailure(tempId);
      }
    }

    return (uploadedIds: uploadedIds, failedCount: failedCount);
  }

  /// loadAllMedia loads all media items in parallel from the MediaRepository.
  static Future<List<MediaItemData>> loadAllMedia(
    Ref ref,
    List<String> mediaIds,
  ) async {
    if (mediaIds.isEmpty) return [];

    final mediaRepository = ref.read(mediaRepositoryProvider);

    final results = await Future.wait(
      mediaIds.map((mediaId) async {
        try {
          final media = await mediaRepository.get(mediaId);

          final isVideo = media.contentType.contains('video');

          // Videos must use the full URL for playback — thumbnail URLs
          // point to a JPEG which ExoPlayer cannot decode as video.
          // Images prefer the thumbnail URL when available.
          final url = isVideo
              ? media.url
              : (media.thumbnailUrl.isNotEmpty ? media.thumbnailUrl : media.url);
          if (!isVideo && media.thumbnailUrl.isEmpty) {
            _log.fine('No thumbnail for media $mediaId, using full URL');
          }

          return MediaItemData(
            id: mediaId,
            url: url,
            contentType: media.contentType,
            isVideo: isVideo,
            attribution: media.hasAttribution() ? media.attribution : null,
            uploader: media.hasUploader() ? media.uploader : null,
            uploadedAtUnixSec: media.createdAtUnixSec != 0
                ? media.createdAtUnixSec.toInt()
                : null,
            // Always store the thumbnail URL separately so video thumbnails
            // (JPEG frames) are available for static strip display independent
            // of the full URL used for playback.
            thumbnailUrl: media.thumbnailUrl.isNotEmpty
                ? media.thumbnailUrl
                : null,
          );
        } catch (e) {
          _log.warning('⚠️ Failed to fetch media $mediaId: $e');
          return null;
        }
      }),
    );

    return results.whereType<MediaItemData>().toList();
  }
}
