import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';

/// ConversationOverlayChrome is the shared header treatment for the
/// full-screen conversation panels (gear / request / experience) hosted inside
/// `ContentMorphPanel` (#2724):
///
/// - the transcript is inset below the control row so the first line never
///   sits under the close button,
/// - a top fade scrim dissolves text that scrolls up toward the controls
///   instead of letting it collide with them,
/// - a single dismiss affordance — the close (✕) in the upper-right. The
///   hosting screen hides its own back chevron while a panel is open (via the
///   content-expanded provider), so the panel never shows two stacked dismiss
///   controls.
class ConversationOverlayChrome extends StatelessWidget {
  /// The conversation body (an `InlineConversationView` or a `*ChatPane`).
  final Widget child;

  const ConversationOverlayChrome({super.key, required this.child});

  /// Height reserved above the transcript for the close-control row.
  static const double topInset = 52;

  /// Extra fade run-out below the inset so scrolled-up text dissolves
  /// gradually rather than clipping at the inset edge.
  static const double _fadeRunOut = 28;

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Stack(
        children: [
          Padding(
            padding: const EdgeInsets.only(top: topInset),
            child: child,
          ),
          // Top fade: opaque enough under the controls for legibility, then
          // dissolving over the run-out so the transcript never reads as
          // jammed under the ✕.
          Positioned(
            top: 0,
            left: 0,
            right: 0,
            height: topInset + _fadeRunOut,
            child: IgnorePointer(
              child: DecoratedBox(
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topCenter,
                    end: Alignment.bottomCenter,
                    colors: [
                      GlassTokens.scrimHeavy,
                      OverlayTokens.scrimTop,
                      Colors.transparent,
                    ],
                    stops: const [0.0, 0.55, 1.0],
                  ),
                ),
              ),
            ),
          ),
          Positioned(
            top: 4,
            right: 8,
            child: IconAction(
              icon: Icons.close_rounded,
              semanticsLabel: context.l10n.a11yClose,
              color: AppColors.onContentImage,
              onPressed: () => Navigator.of(context).pop(),
            ),
          ),
        ],
      ),
    );
  }
}
