import 'package:flutter/material.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';

/// ContentGradientOverlay darkens the top of a full-bleed hero so the back and
/// overflow controls read against whatever photo is behind them.
///
/// That is its whole job. It is anchored to the FRAME, so the only thing it can
/// express is "N% down the screen" — and where a hero's text sits is a function
/// of how much content that hero has, not of the screen. The wash that protects
/// the text is [HeroContentWash], which wraps the editorial sheet and is sized
/// to it.
///
/// This layer briefly held a floor across the whole frame instead. It did make
/// the text pass, but by washing the entire photograph to do it: measured
/// against the base branch, every hero photo lost 26-37% of its mean brightness,
/// nearly all of it over bare backdrop or over pixels the sheet already covers.
///
/// Usage:
/// ```dart
/// Stack(
///   children: [
///     MediaBackground(...),
///     Positioned.fill(child: ContentGradientOverlay()),
///     SafeArea(child: ...),
///   ],
/// )
/// ```
class ContentGradientOverlay extends StatelessWidget {
  const ContentGradientOverlay({super.key, this.mediaCacheKey});

  /// Stable key of the media behind this overlay — `MediaUrl.cacheKey`, not the
  /// URL, which is presigned and rotates.
  ///
  /// Retained so the four call sites keep compiling; unused. It fed a
  /// per-photo measurement that chose the frame-wide floor's alpha. With no
  /// frame-wide floor there is nothing to choose, and the measurement — an
  /// image-decode probe plus a cache — was deleted rather than left running
  /// with no reader.
  final String? mediaCacheKey;

  @override
  Widget build(BuildContext context) {
    return const DecoratedBox(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            OverlayTokens.scrimTop,
            Colors.transparent,
            Colors.transparent,
          ],
          stops: [0.0, 0.15, 1.0],
        ),
      ),
    );
  }
}
