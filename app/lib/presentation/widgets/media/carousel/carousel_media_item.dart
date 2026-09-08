import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;

/// MediaItem represents a single media item in the carousel.
///
/// ## URL vs. mediaId
///
/// Always pass [mediaId] when the item has a server-side identity (i.e. for
/// any media already uploaded to the server). [CarouselImageViewer] uses
/// [mediaId] to resolve a freshly-signed full-resolution URL at viewer-mount
/// time via `fullMediaUrlProvider`. Skipping [mediaId] forces the viewer to
/// reuse [url] verbatim, which can be a stale presigned URL captured into a
/// long-lived closure — see #1833.
///
/// [url] remains required because it is still used by non-image consumers:
/// video controller initialization (`createCachedVideoController`), inline
/// download/save flows, and locally-staged uploads that have no [mediaId]
/// yet. For images on server-side media, treat [url] as the fallback the
/// viewer uses only when [mediaId] is absent.
class MediaItem {
  final String url;
  final bool isVideo;
  final String? contentType;
  final String? mediaId;
  final Attribution? attribution;
  final User? uploader;
  final int? uploadedAtUnixSec;

  const MediaItem({
    required this.url,
    required this.isVideo,
    this.contentType,
    this.mediaId,
    this.attribution,
    this.uploader,
    this.uploadedAtUnixSec,
  });
}
