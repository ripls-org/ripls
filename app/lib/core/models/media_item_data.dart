import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;

part 'media_item_data.freezed.dart';

/// Data class for media items with URL and metadata.
/// Shared across all ViewModels that need media loading.
///
/// The [attribution] field contains creator information for stock images
/// from Unsplash. It is null for user-uploaded media.
///
/// The [uploader] field contains the user who uploaded this media. It is null
/// for stock imagery or when the uploader cannot be resolved.
///
/// The [isUploading] field indicates whether this is a placeholder for an
/// in-progress upload. When true, [localPath] should contain the local file
/// path for optimistic display.
@freezed
sealed class MediaItemData with _$MediaItemData {
  const factory MediaItemData({
    required String id,
    required String url,
    required String contentType,
    required bool isVideo,
    Attribution? attribution,
    User? uploader,
    // Unix timestamp (seconds) when this media was uploaded. Null for
    // optimistic upload placeholders and legacy items without the field.
    int? uploadedAtUnixSec,
    @Default(false) bool isUploading,
    String? localPath,
    // JPEG thumbnail URL — populated for both images and videos when the server
    // has generated a thumbnail frame. Use this for static strip display; use
    // [url] for full-resolution display and video playback.
    String? thumbnailUrl,
  }) = _MediaItemData;
}
