import 'dart:ui' show lerpDouble;

import 'package:flutter/material.dart';

/// morphRevealRoute builds a transparent page route whose destination is
/// revealed by a rounded rectangle that grows from [sourceRect] (a card's
/// on-screen footprint) to fill the screen — the "tap a widget, it expands in
/// place into the full content" morph from
/// docs/issues/2280-pitching-in-expand.md.
///
/// The route is non-opaque so the previous screen keeps painting behind the
/// growing panel; a scrim fades in to dim it (mirroring the mockup's dimmed
/// background). Popping reverses the morph back into [sourceRect]. Pass a
/// zero [duration] (via `accessibleDuration`) to honor reduce-motion.
///
/// [scrimMaxOpacity] is the peak opacity of the black scrim painted behind the
/// growing panel. Set it to 0 when the destination supplies its own
/// semi-transparent surface (so the backdrop isn't dimmed twice).
PageRouteBuilder<T> morphRevealRoute<T>({
  required Widget screen,
  required Rect sourceRect,
  required Duration duration,
  String? routeName,
  double scrimMaxOpacity = 0.5,
}) {
  return PageRouteBuilder<T>(
    opaque: false,
    transitionDuration: duration,
    reverseTransitionDuration: duration,
    settings: routeName != null ? RouteSettings(name: routeName) : null,
    pageBuilder: (context, animation, secondaryAnimation) => screen,
    transitionsBuilder: (context, animation, secondaryAnimation, child) {
      return _MorphReveal(
        progress: CurvedAnimation(
          parent: animation,
          curve: Curves.easeOutCubic,
          reverseCurve: Curves.easeInCubic,
        ),
        sourceRect: sourceRect,
        scrimMaxOpacity: scrimMaxOpacity,
        child: child,
      );
    },
  );
}

/// Clips [child] to a rounded rect interpolated from [sourceRect] to the full
/// screen as [progress] runs 0→1, dimming whatever is behind it.
class _MorphReveal extends StatelessWidget {
  final Animation<double> progress;
  final Rect sourceRect;
  final double scrimMaxOpacity;
  final Widget child;

  const _MorphReveal({
    required this.progress,
    required this.sourceRect,
    required this.scrimMaxOpacity,
    required this.child,
  });

  @override
  Widget build(BuildContext context) {
    final fullScreen = Offset.zero & MediaQuery.sizeOf(context);
    return AnimatedBuilder(
      animation: progress,
      child: child,
      builder: (context, child) {
        final t = progress.value;
        final rect = Rect.lerp(sourceRect, fullScreen, t)!;
        final radius = lerpDouble(18, 0, t)!;
        return Stack(
          children: [
            // Dim the content behind the growing panel, fading in with the
            // morph. Skipped (max opacity 0) when the destination supplies its
            // own semi-transparent surface.
            if (scrimMaxOpacity > 0)
              Positioned.fill(
                child: IgnorePointer(
                  child: ColoredBox(
                    color: Colors.black.withValues(alpha: scrimMaxOpacity * t),
                  ),
                ),
              ),
            ClipPath(
              clipper: _RevealClipper(rect: rect, radius: radius),
              child: child,
            ),
          ],
        );
      },
    );
  }
}

class _RevealClipper extends CustomClipper<Path> {
  final Rect rect;
  final double radius;

  const _RevealClipper({required this.rect, required this.radius});

  @override
  Path getClip(Size size) =>
      Path()..addRRect(RRect.fromRectAndRadius(rect, Radius.circular(radius)));

  @override
  bool shouldReclip(covariant _RevealClipper oldClipper) =>
      oldClipper.rect != rect || oldClipper.radius != radius;
}
