import 'package:flutter/material.dart';

/// Returns [duration] when the user has not requested reduced motion, or
/// [Duration.zero] when they have. Use this in place of bare [Duration]
/// literals on [AnimationController], [AnimatedContainer], [AnimatedOpacity],
/// and any other animation surface so the codebase respects the system
/// "Reduce Motion" setting.
///
/// Example:
/// ```dart
/// _controller = AnimationController(
///   vsync: this,
///   duration: accessibleDuration(context, const Duration(milliseconds: 280)),
/// );
/// ```
///
/// Reads [MediaQuery.maybeOf] so the helper can be called outside a Material
/// app shell (e.g. early in app boot) without throwing — when no MediaQuery
/// is available the requested [duration] is used unchanged.
Duration accessibleDuration(BuildContext context, Duration duration) {
  final media = MediaQuery.maybeOf(context);
  if (media != null && media.disableAnimations) {
    return Duration.zero;
  }
  return duration;
}
