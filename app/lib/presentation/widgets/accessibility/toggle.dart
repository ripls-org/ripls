import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Toggle is the codebase primitive for segmented-button / chip / RSVP-style
/// controls where the tap flips a selectable state. It wraps [Tappable] and
/// additionally requires a [selected] flag so screen readers announce the
/// current state.
///
/// Use this anywhere a tap *selects* something rather than performing a
/// transient action. Examples: filter chips, RSVP buttons, period toggles,
/// input-mode switches.
class Toggle extends StatelessWidget {
  /// Localized label announced to assistive tech. The selected state is
  /// announced separately via the `selected` semantic flag — do not embed
  /// "selected" / "not selected" into the label.
  final String semanticsLabel;

  final bool selected;

  final VoidCallback? onTap;

  /// When true, the row is announced as a radio button (one of a
  /// mutually-exclusive group). Use for single-select pickers where
  /// exactly one row is meant to be selected. Defaults to false, in
  /// which case the row is announced as a button with a selected state
  /// — appropriate for checkbox/toggle/chip semantics.
  final bool inMutuallyExclusiveGroup;

  /// Optional ink-splash radius; see [Tappable.inkBorderRadius].
  final BorderRadius? inkBorderRadius;

  /// Optional kebab-case identifier exposed on the underlying Semantics
  /// node. See [Tappable.semanticsIdentifier].
  final String? semanticsIdentifier;

  final Widget child;

  const Toggle({
    super.key,
    required this.semanticsLabel,
    required this.selected,
    required this.onTap,
    required this.child,
    this.inMutuallyExclusiveGroup = false,
    this.inkBorderRadius,
    this.semanticsIdentifier,
  });

  @override
  Widget build(BuildContext context) {
    return Semantics(
      identifier: semanticsIdentifier,
      selected: selected,
      inMutuallyExclusiveGroup: inMutuallyExclusiveGroup,
      // We delegate label/button/enabled to the inner Tappable so the
      // role is consistent with other tappable surfaces. Marking selected
      // here merges into the same semantics node. The identifier is set
      // here on the outer wrapper rather than passed through, since
      // Flutter's semantics merge prefers the outer node's identifier.
      child: Tappable(
        semanticsLabel: semanticsLabel,
        onTap: onTap,
        inkBorderRadius: inkBorderRadius,
        child: child,
      ),
    );
  }
}
