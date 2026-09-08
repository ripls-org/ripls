import 'package:cross_file/cross_file.dart';
import 'package:ripls/core/models/media_item_data.dart';

/// Helper class for managing optimistic media upload placeholders.
///
/// Provides utilities for creating placeholder MediaItemData during upload
/// and inserting them at the correct position (front or back) in media lists.
class MediaUploadHelper {
  // Private constructor to prevent instantiation
  MediaUploadHelper._();

  /// Generates a unique temporary ID for upload placeholders.
  static String generateTempId() {
    return 'temp_${DateTime.now().millisecondsSinceEpoch}';
  }

  /// Determines if a file is a video based on its name / path.
  /// Accepts the XFile.name (preferred — works on every platform) or
  /// a raw path string.
  static bool isVideoFile(String pathOrName) {
    final lower = pathOrName.toLowerCase();
    return lower.endsWith('.mp4') ||
        lower.endsWith('.mov') ||
        lower.endsWith('.avi');
  }

  /// Creates an optimistic placeholder MediaItemData for an uploading file.
  ///
  /// [tempId] is the unique temporary ID for the placeholder; [file] is the
  /// cross-platform XFile being uploaded. On mobile, `file.path` is a real
  /// filesystem path the placeholder widgets can render via `Image.file`.
  /// On web, `file.path` is a `blob:` URL that renders via `Image.network`.
  /// The placeholder widgets branch on `kIsWeb` to pick the right widget.
  ///
  /// Returns a MediaItemData with isUploading=true and localPath set to
  /// `file.path` (blob URL on web).
  static MediaItemData createPlaceholder({
    required String tempId,
    required XFile file,
  }) {
    // Prefer mimeType when the picker / camera supplied it; fall back to
    // the filename's extension. On web mimeType is usually populated.
    final mime = file.mimeType ?? '';
    final isVideo = mime.startsWith('video/') || isVideoFile(file.name);

    return MediaItemData(
      id: tempId,
      url: file.path,
      contentType: mime.isNotEmpty ? mime : (isVideo ? 'video/mp4' : 'image/jpeg'),
      isVideo: isVideo,
      isUploading: true,
      localPath: file.path,
    );
  }

  /// Inserts a placeholder at the correct position in a media list.
  ///
  /// Parameters:
  /// - [placeholder]: The MediaItemData placeholder to insert
  /// - [currentItems]: Current list of media items
  /// - [insertAtFront]: If true, prepends to position 0; if false, appends to end
  ///
  /// Returns a new list with the placeholder inserted at the correct position.
  static List<MediaItemData> insertPlaceholder({
    required MediaItemData placeholder,
    required List<MediaItemData> currentItems,
    required bool insertAtFront,
  }) {
    return insertAtFront
        ? [placeholder, ...currentItems]
        : [...currentItems, placeholder];
  }

  /// Removes a placeholder from a media list by its temporary ID.
  ///
  /// Used for error handling when an upload fails.
  static List<MediaItemData> removePlaceholder({
    required String tempId,
    required List<MediaItemData> currentItems,
  }) {
    return currentItems.where((item) => item.id != tempId).toList();
  }
}
