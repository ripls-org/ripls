import 'package:flutter/widgets.dart';
import 'package:ripls/core/utils/responsive.dart';

/// Centers its child at the app's reading measure (#2912).
///
/// `Center` + `ConstrainedBox(maxWidth:)` and nothing more. On any window
/// narrower than [maxWidth] the constraint never binds, so phone layouts are
/// untouched by construction; on desktop-wide windows the child holds a
/// readable centered column instead of stretching to the window.
///
/// Apply this per surface — it is a composition tool, not a shell clamp. No
/// layout may depend on it for *correctness* (fit at any viewport is the
/// #2908 height-budget work); this bounds width for readability only.
class ContentColumn extends StatelessWidget {
  final Widget child;
  final double maxWidth;

  /// Optional padding applied outside the constraint, so callers can keep an
  /// existing edge inset while adopting the measure.
  final EdgeInsetsGeometry? padding;

  const ContentColumn({
    super.key,
    required this.child,
    this.maxWidth = Responsive.contentMaxWidth,
    this.padding,
  });

  @override
  Widget build(BuildContext context) {
    Widget constrained = ConstrainedBox(
      constraints: BoxConstraints(maxWidth: maxWidth),
      child: child,
    );
    if (padding != null) {
      constrained = Padding(padding: padding!, child: constrained);
    }
    return Center(child: constrained);
  }
}
