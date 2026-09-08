import 'package:flutter/material.dart';

/// ContentViewPaneSwitcher animates horizontal slide transitions between
/// indexed tab panes, with an optional swipe gesture that drives [onTabChanged].
///
/// Each entry in [paneBuilders] is a builder for one tab; the active index is
/// [activeIndex]. [slideDirection] is `1` for forward (left-to-right) and `-1`
/// for backward. The caller maintains [slideDirection] alongside [activeIndex]
/// so that the slide direction stays consistent through the animation, even
/// after the active index has updated.
///
/// [swipeVelocityThreshold] is the absolute primary-velocity threshold for
/// switching tabs via horizontal drag. The default (300 logical px/s) matches
/// the gear and experience content views' historical behavior.
class ContentViewPaneSwitcher extends StatelessWidget {
  final int activeIndex;
  final int slideDirection;
  final List<Widget Function(BuildContext)> paneBuilders;
  final ValueChanged<int> onTabChanged;
  final Duration sizeDuration;
  final Duration switcherDuration;
  final double swipeVelocityThreshold;

  const ContentViewPaneSwitcher({
    super.key,
    required this.activeIndex,
    required this.slideDirection,
    required this.paneBuilders,
    required this.onTabChanged,
    this.sizeDuration = const Duration(milliseconds: 300),
    this.switcherDuration = const Duration(milliseconds: 280),
    this.swipeVelocityThreshold = 300,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.translucent,
      onHorizontalDragEnd: (details) {
        final velocity = details.primaryVelocity ?? 0;
        if (velocity < -swipeVelocityThreshold &&
            activeIndex < paneBuilders.length - 1) {
          onTabChanged(activeIndex + 1);
        } else if (velocity > swipeVelocityThreshold && activeIndex > 0) {
          onTabChanged(activeIndex - 1);
        }
      },
      child: AnimatedSize(
        duration: sizeDuration,
        curve: Curves.easeInOut,
        clipBehavior: Clip.hardEdge,
        child: AnimatedSwitcher(
          duration: switcherDuration,
          transitionBuilder: (child, animation) {
            final direction = slideDirection;
            final slideIn = Tween<Offset>(
              begin: Offset(direction.toDouble(), 0),
              end: Offset.zero,
            ).animate(
              CurvedAnimation(parent: animation, curve: Curves.easeInOut),
            );
            final slideOut = Tween<Offset>(
              begin: Offset(-direction.toDouble(), 0),
              end: Offset.zero,
            ).animate(
              CurvedAnimation(parent: animation, curve: Curves.easeInOut),
            );
            return SlideTransition(
              position: child.key == ValueKey(activeIndex) ? slideIn : slideOut,
              child: child,
            );
          },
          layoutBuilder: (current, previous) => Stack(
            alignment: Alignment.topCenter,
            children: [...previous, ?current],
          ),
          child: KeyedSubtree(
            key: ValueKey(activeIndex),
            child: paneBuilders[activeIndex](context),
          ),
        ),
      ),
    );
  }
}
