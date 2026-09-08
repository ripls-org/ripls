import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';

/// ContentMorphPanel is the shared chrome for a full-screen morph-reveal
/// content panel (docs/client/modals.md — Morph-reveal content panels): a
/// transparent scaffold over a strong media scrim, with horizontal
/// swipe-to-close.
///
/// It is pushed via `morphRevealRoute`, which grows it from the source card's
/// footprint over the content view's existing hero; this widget supplies the
/// surface treatment shared by every such panel:
/// - a transparent `Scaffold` so the (still-playing) hero shows through,
/// - a 75%-black scrim ([AppColors.modalContentScrimStrong]) dimming the hero
///   for legibility (the route's own backdrop scrim is disabled),
/// - a horizontal flick (either direction) that pops the route — letting the
///   morph play its reverse (the panel shrinks back into the card).
///
/// Wrap your panel body in it; the body owns its own header/close control
/// (an `IconAction` or `BackButtonWidget` that calls `Navigator.pop`) and any
/// `SafeArea` it needs.
class ContentMorphPanel extends StatelessWidget {
  /// The panel body (header + content).
  final Widget child;

  /// Scrim painted between the hero and [child]. Defaults to the strong 75%
  /// media scrim.
  final Color scrimColor;

  /// Minimum horizontal flick velocity (px/s) that dismisses the panel.
  final double dismissVelocityThreshold;

  const ContentMorphPanel({
    super.key,
    required this.child,
    this.scrimColor = AppColors.modalContentScrimStrong,
    this.dismissVelocityThreshold = 300.0,
  });

  void _onHorizontalDragEnd(BuildContext context, DragEndDetails drag) {
    final velocity = drag.primaryVelocity ?? 0;
    if (velocity.abs() > dismissVelocityThreshold) {
      final navigator = Navigator.of(context);
      // Pop plays morphRevealRoute's reverse transition (shrink into the card).
      if (navigator.canPop()) navigator.pop();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.transparent,
      body: GestureDetector(
        behavior: HitTestBehavior.translucent,
        onHorizontalDragEnd: (drag) => _onHorizontalDragEnd(context, drag),
        // The scrim stays full-bleed (it dims the whole hero); the panel BODY
        // holds the reading measure on desktop-wide windows (#2912) — a no-op
        // at phone widths.
        child: ColoredBox(
          color: scrimColor,
          child: ContentColumn(child: child),
        ),
      ),
    );
  }
}
