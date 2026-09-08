import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:video_player/video_player.dart';

/// CarouselVideoPlayer renders a single video item in the carousel.
///
/// Displays a loading spinner while the controller is initialising. Once
/// ready it shows the video with a play/pause overlay button.
class CarouselVideoPlayer extends StatefulWidget {
  final VideoPlayerController? controller;

  const CarouselVideoPlayer({super.key, required this.controller});

  @override
  State<CarouselVideoPlayer> createState() => _CarouselVideoPlayerState();
}

class _CarouselVideoPlayerState extends State<CarouselVideoPlayer> {
  @override
  Widget build(BuildContext context) {
    final controller = widget.controller;

    if (controller == null || !controller.value.isInitialized) {
      return const Center(
        child: CircularProgressIndicator(color: Colors.white),
      );
    }

    final isPlaying = controller.value.isPlaying;
    return Center(
      child: AspectRatio(
        aspectRatio: controller.value.aspectRatio,
        child: Tappable(
          semanticsLabel: isPlaying
              ? context.l10n.a11yMediaPauseVideo
              : context.l10n.a11yMediaPlayVideo,
          onTap: () {
            setState(() {
              if (controller.value.isPlaying) {
                controller.pause();
              } else {
                controller.play();
              }
            });
          },
          child: Stack(
            alignment: Alignment.center,
            children: [
              VideoPlayer(controller),
              if (!controller.value.isPlaying)
                Container(
                  decoration: BoxDecoration(
                    color: Colors.black.withValues(alpha: 0.5),
                    shape: BoxShape.circle,
                  ),
                  padding: const EdgeInsets.all(16),
                  child: const Icon(
                    Icons.play_arrow,
                    color: Colors.white,
                    size: 48,
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
