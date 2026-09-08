import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';

/// A lightweight pulsing-box placeholder shown in the preview modals while a
/// streaming generation is still in flight. Renders a solid rounded rectangle
/// that animates its opacity between 0.4 and 1.0 so the user sees the space
/// reserved for a field before the AI populates it.
class StreamingSkeleton extends StatefulWidget {
  const StreamingSkeleton({
    super.key,
    this.height = 20,
    this.width,
    this.borderRadius = 8,
  });

  final double height;
  final double? width;
  final double borderRadius;

  @override
  State<StreamingSkeleton> createState() => _StreamingSkeletonState();
}

class _StreamingSkeletonState extends State<StreamingSkeleton>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;
  late final Animation<double> _opacity;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: Duration.zero,
    );
    _opacity = Tween<double>(begin: 0.4, end: 1).animate(
      CurvedAnimation(parent: _controller, curve: Curves.easeInOut),
    );
  }

  bool _started = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _controller.duration =
        accessibleDuration(context, const Duration(milliseconds: 900));
    if (!_started) {
      _started = true;
      _controller.repeat(reverse: true);
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final baseColor = Theme.of(context).colorScheme.surfaceContainerHighest;
    return FadeTransition(
      opacity: _opacity,
      child: Container(
        height: widget.height,
        width: widget.width,
        decoration: BoxDecoration(
          color: baseColor,
          borderRadius: BorderRadius.circular(widget.borderRadius),
        ),
      ),
    );
  }
}

/// Compact banner shown at the top of the Experience / Request / Gear preview
/// modals while a streaming generation is in flight. Combines a pulsing
/// [StreamingSkeleton] bar with a brief label so the user understands why
/// some fields are still empty. The label is passed in so callers can supply a
/// localized string (widgets, not ViewModels, own l10n lookups).
class StreamingGeneratingBanner extends StatelessWidget {
  const StreamingGeneratingBanner({super.key, required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Expanded(child: StreamingSkeleton(height: 12, borderRadius: 6)),
        const SizedBox(width: 12),
        Text(
          label,
          style: TextStyle(
            fontSize: 12,
            color: Theme.of(context).colorScheme.onSurfaceVariant,
          ),
        ),
      ],
    );
  }
}

/// Per-field "this slot is loading, hands off" wrapper for streaming Gen*
/// preview modals. Renders a non-interactive [StreamingSkeleton] of the given
/// dimensions while [ready] is false, then fades in [child] once the field's
/// data has arrived. Layout reserves [height] in both states so populating the
/// field doesn't shift the surrounding UI.
///
/// While the skeleton is showing the slot is wrapped in [IgnorePointer] and
/// hidden from semantics so screen readers don't try to interact with it.
class StreamingFieldSlot extends StatelessWidget {
  const StreamingFieldSlot({
    required this.ready,
    required this.child,
    this.height = 40,
    this.borderRadius = 8,
    super.key,
  });

  final bool ready;
  final Widget child;
  final double height;
  final double borderRadius;

  @override
  Widget build(BuildContext context) {
    return AnimatedSwitcher(
      duration: accessibleDuration(context, const Duration(milliseconds: 200)),
      child: ready
          ? KeyedSubtree(key: const ValueKey('ready'), child: child)
          : ExcludeSemantics(
              key: const ValueKey('loading'),
              child: IgnorePointer(
                child: StreamingSkeleton(
                  height: height,
                  borderRadius: borderRadius,
                ),
              ),
            ),
    );
  }
}
