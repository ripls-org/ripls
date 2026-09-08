import 'package:ripls/data/repositories/media_repository.dart';
import 'package:video_player/video_player.dart';

// Default to the web implementation; switch to the IO implementation
// when `dart.library.io` is available (mobile / desktop). Matches the
// conditional-import direction used in
// `app/lib/services/providers/auth_providers.dart`.
import 'video_cache_helper_web.dart'
    if (dart.library.io) 'video_cache_helper_io.dart' as platform;

/// createCachedVideoController returns an initialized, looping, muted
/// [VideoPlayerController] ready for playback.
///
/// On mobile and desktop, [url] values that are local file paths
/// (e.g. an `image_picker` temp path before it has been replaced with a
/// server URL — see [MediaUploadHelper.createPlaceholder] in
/// `app/lib/core/utils/media_upload_helper.dart`) play via
/// [VideoPlayerController.file], and remote URLs go through the disk
/// cache exposed by [MediaRepository.getVideoFile] keyed by [mediaId].
/// Routing a local path through `flutter_cache_manager` would throw
/// `ArgumentError: No host specified in URI` (issue #2073).
///
/// On web, every [url] plays via [VideoPlayerController.networkUrl] into
/// an HTML5 `<video>` element — `dart:io`, `flutter_cache_manager`, and
/// `VideoPlayerController.file` have no web implementation. [repo] and
/// [mediaId] are ignored on web; the browser's HTTP cache handles
/// repeated fetches.
Future<VideoPlayerController> createCachedVideoController(
  MediaRepository repo,
  String mediaId,
  String url,
) =>
    platform.createPlatformVideoController(repo, mediaId, url);
