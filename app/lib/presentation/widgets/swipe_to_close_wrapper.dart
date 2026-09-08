import 'package:flutter/material.dart';

/// SwipeToCloseWrapper adds swipe-right-to-close gesture to overlay screens.
///
/// Wraps a widget and triggers the onClose callback when the user swipes
/// right with sufficient velocity. Commonly used with full-screen overlay
/// screens that slide in from the right.
///
/// Example usage:
/// ```dart
/// SwipeToCloseWrapper(
///   onClose: () => Navigator.of(context).pop(),
///   child: SlideTransition(
///     position: _slideAnimation,
///     child: Scaffold(...),
///   ),
/// )
/// ```
class SwipeToCloseWrapper extends StatelessWidget {
  final Widget child;
  final VoidCallback onClose;
  final double velocityThreshold;

  const SwipeToCloseWrapper({
    super.key,
    required this.child,
    required this.onClose,
    this.velocityThreshold = 300.0,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onHorizontalDragEnd: (details) {
        final velocity = details.primaryVelocity ?? 0;
        if (velocity > velocityThreshold) {
          onClose();
        }
      },
      child: child,
    );
  }
}
