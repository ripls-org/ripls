import 'package:ripls/data/repositories/media_repository.dart';
import 'package:video_player/video_player.dart';

/// Web implementation of [createPlatformVideoController]. Uses
/// `VideoPlayerController.networkUrl`, which renders into an HTML5
/// `<video>` element via `video_player_web`. The on-disk cache used on
/// mobile (`flutter_cache_manager` → `VideoPlayerController.file`) has
/// no web backend, so we let the browser's HTTP cache handle repeated
/// fetches.
///
/// [repo] and [mediaId] are unused on web; they are kept so the
/// function signature matches `video_cache_helper_io.dart` for the
/// conditional import in `video_cache_helper.dart`.
Future<VideoPlayerController> createPlatformVideoController(
  // ignore: avoid_unused_constructor_parameters
  MediaRepository repo,
  // ignore: avoid_unused_constructor_parameters
  String mediaId,
  String url,
) async {
  final controller = VideoPlayerController.networkUrl(Uri.parse(url));
  await controller.initialize();
  await controller.setLooping(true);
  await controller.setVolume(0);
  await controller.play();
  return controller;
}
