import 'package:video_player/video_player.dart';

/// toggleVideoMute toggles the mute state of a video player controller.
///
/// This shared helper centralizes the mute/unmute logic used across all content
/// view ViewModels, avoiding code duplication. If the controller is null or not
/// initialized, this is a no-op.
void toggleVideoMute({
  required VideoPlayerController? controller,
  required bool currentlyMuted,
  required void Function(bool isMuted) updateState,
}) {
  if (controller == null || !controller.value.isInitialized) return;
  final newMuted = !currentlyMuted;
  controller.setVolume(newMuted ? 0.0 : 1.0);
  updateState(newMuted);
}
