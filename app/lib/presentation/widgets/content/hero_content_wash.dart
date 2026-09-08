import 'package:flutter/material.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';

/// HeroContentWash darkens a hero photo behind the bottom-anchored editorial
/// sheet, and only there.
///
/// The wash is sized to the content rather than to the frame. Item, event,
/// request and community heroes all put a variable-height sheet at the bottom
/// of a full-bleed photo — a title alone on one, a title plus four metadata
/// rows plus a discussion card on another — so no fixed fraction of the screen
/// describes where text begins. Wrapping the sheet means the ramp lands wherever
/// the sheet's top edge happens to be.
///
/// ## Why the run-up exists
///
/// All six sites previously put `Colors.transparent` at stop 0.0 of the sheet's
/// own box and reached 65% black a fifth of the way down it. Text starts ~16px
/// below that edge, which on a typical sheet is ~3% — about 10% black under the
/// title. On a bright photo that measured **3.07:1**, and on a sunset **1.56:1**.
/// The gradient was doing its job over the metadata and missing the headline.
///
/// So the ramp runs OUTSIDE the sheet: [runUp] logical pixels of transparent →
/// [OverlayTokens.washTop] above it, leaving the sheet itself at full wash from
/// its first pixel. The photo above the run-up is untouched.
///
/// Fixing it by holding a floor across the whole frame is what this replaced;
/// it cost every hero photo 26-37% of its mean brightness to protect a band of
/// text near the bottom.
class HeroContentWash extends StatelessWidget {
  const HeroContentWash({super.key, required this.child});

  /// The bottom-anchored sheet. Should size itself to its content
  /// (`MainAxisSize.min`) — the wash inherits whatever height it reports.
  final Widget child;

  /// How far above the sheet the wash starts ramping in.
  ///
  /// Enough that the ramp is a gradient rather than an edge, and short enough
  /// that it reads as the sheet's own shadow rather than as a band across the
  /// photo. Tuned against the gear hero, whose title sits closest to the top of
  /// its sheet of the four.
  static const double runUp = 72;

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SizedBox(
          height: runUp,
          child: DecoratedBox(
            decoration: BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment.bottomCenter,
                colors: [Colors.transparent, OverlayTokens.washTop],
              ),
            ),
          ),
        ),
        DecoratedBox(
          decoration: const BoxDecoration(
            gradient: LinearGradient(
              begin: Alignment.topCenter,
              end: Alignment.bottomCenter,
              colors: [OverlayTokens.washTop, OverlayTokens.washBottom],
            ),
          ),
          child: child,
        ),
      ],
    );
  }
}
