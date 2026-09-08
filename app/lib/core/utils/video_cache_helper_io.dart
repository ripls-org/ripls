import 'dart:io';

import 'package:flutter/foundation.dart' show visibleForTesting;
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:video_player/video_player.dart';

/// Mobile / desktop implementation of [createPlatformVideoController]
/// (see `video_cache_helper.dart` for the public entry point). Reuses
/// the on-disk [VideoFileCache] in [MediaRepository] so a presigned-URL
/// rotation does not re-download the bytes. The web build uses
/// `video_cache_helper_web.dart` instead, which goes directly to
/// `VideoPlayerController.networkUrl` because `dart:io` and the
/// `flutter_cache_manager`-backed disk cache do not exist there.
Future<VideoPlayerController> createPlatformVideoController(
  MediaRepository repo,
  String mediaId,
  String url,
) async {
  final VideoPlayerController controller;
  if (isLocalPath(url)) {
    controller = VideoPlayerController.file(File(url));
  } else {
    final file = await repo.getVideoFile(mediaId, url);
    controller = VideoPlayerController.file(file);
  }
  await controller.initialize();
  await controller.setLooping(true);
  await controller.setVolume(0);
  await controller.play();
  return controller;
}

/// isLocalPath returns true when [value] denotes a file on the local
/// device rather than a remote URL. A value is local when its parsed URI
/// has no scheme, has a `file` scheme, or has a non-`file` scheme with no
/// host (e.g. malformed input). Anything that parses as `http`/`https`
/// with a host goes through the cache manager.
///
/// Exported via `@visibleForTesting` for direct unit-test coverage; the
/// branch inside [createPlatformVideoController] is exercised indirectly by
/// the ViewModel-layer tests that mock [MediaRepository].
@visibleForTesting
bool isLocalPath(String value) {
  if (value.isEmpty) return true;
  final uri = Uri.tryParse(value);
  if (uri == null) return true;
  if (uri.scheme.isEmpty) return true;
  if (uri.scheme == 'file') return true;
  if (uri.host.isEmpty) return true;
  return false;
}
