import 'package:image_picker/image_picker.dart';

/// MediaPickerHelper provides shared utility functions for picking media from gallery/camera.
///
/// This helper ensures consistent media quality settings across the app.
/// All methods return [XFile] — the cross-platform handle returned by
/// `image_picker` on every target. Use `xfile.readAsBytes()` (or
/// `MediaRepository.addMedia(file: xfile)`) to read the contents;
/// `xfile.path` returns a filesystem path on mobile and a `blob:` URL
/// on web, and is not a valid argument to `dart:io File`.
class MediaPickerHelper {
  /// Standard image quality settings
  static const double imageMaxWidth = 1920;
  static const double imageMaxHeight = 1080;
  static const int imageQuality = 85;

  /// Standard video duration limit
  static const Duration videoMaxDuration = Duration(minutes: 5);

  /// Picks an image from the gallery with standard quality settings.
  ///
  /// Returns the picked [XFile] (null if the user cancelled).
  static Future<XFile?> pickImageFromGallery() async {
    final picker = ImagePicker();
    return picker.pickImage(
      source: ImageSource.gallery,
      maxWidth: imageMaxWidth,
      maxHeight: imageMaxHeight,
      imageQuality: imageQuality,
    );
  }

  /// Picks a video from the gallery with standard duration limit.
  ///
  /// Returns the picked [XFile] (null if the user cancelled).
  static Future<XFile?> pickVideoFromGallery() async {
    final picker = ImagePicker();
    return picker.pickVideo(
      source: ImageSource.gallery,
      maxDuration: videoMaxDuration,
    );
  }

  /// Picks an image from the camera with standard quality settings.
  ///
  /// On mobile, opens the device camera. On web,
  /// `image_picker.pickImage(source: camera)` triggers the browser's
  /// camera-aware file picker (mobile browsers offer the live camera;
  /// desktop browsers fall back to file chooser).
  /// Returns the captured [XFile] (null if the user cancelled).
  static Future<XFile?> pickImageFromCamera() async {
    final picker = ImagePicker();
    return picker.pickImage(
      source: ImageSource.camera,
      maxWidth: imageMaxWidth,
      maxHeight: imageMaxHeight,
      imageQuality: imageQuality,
    );
  }

  /// Maximum number of images that can be selected in a single batch pick.
  static const int maxBatchSize = 20;

  /// Picks multiple images from the gallery with standard quality settings.
  ///
  /// Returns a list of picked [XFile]s (up to [maxBatchSize]).
  /// Returns an empty list if the user cancels the picker.
  /// Non-image files (e.g. videos accidentally included by the OS picker) are
  /// filtered out defensively based on their MIME type.
  static Future<List<XFile>> pickMultipleImagesFromGallery() async {
    final picker = ImagePicker();
    final pickedFiles = await picker.pickMultiImage(
      maxWidth: imageMaxWidth,
      maxHeight: imageMaxHeight,
      imageQuality: imageQuality,
      limit: maxBatchSize,
    );
    return pickedFiles
        .where((file) {
          final mime = file.mimeType ?? '';
          // Keep files with an explicit image MIME type, or with no MIME type
          // (path-based fallback — the uploader will validate server-side).
          return mime.isEmpty || mime.startsWith('image/');
        })
        .toList();
  }
}
