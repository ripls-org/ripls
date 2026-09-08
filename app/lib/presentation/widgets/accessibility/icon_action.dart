import 'package:flutter/material.dart';

/// IconAction is the codebase default for icon-only buttons. It wraps
/// Flutter's [IconButton] and *requires* both a [semanticsLabel] and a
/// [tooltip] so the button is announced correctly to assistive tech and
/// surfaces a visible label on long-press for sighted users.
///
/// Naming note: this widget is deliberately not called `IconButton` to avoid
/// shadowing Flutter's stock widget in the import namespace. Use it anywhere
/// a stock [IconButton] would otherwise appear.
class IconAction extends StatelessWidget {
  final IconData icon;

  /// Localized label announced to assistive tech and used as the tooltip.
  /// Pass `context.l10n.<key>`.
  final String semanticsLabel;

  /// Tap callback. When null the button is rendered as disabled.
  final VoidCallback? onPressed;

  /// Optional explicit tooltip; defaults to [semanticsLabel] when omitted.
  /// Provide a separate value only when the visible tooltip should differ
  /// from the screen-reader label (rare).
  final String? tooltip;

  final double? iconSize;
  final Color? color;
  final EdgeInsetsGeometry? padding;
  final BoxConstraints? constraints;

  /// Optional kebab-case identifier exposed on a wrapping Semantics
  /// node. See [Tappable.semanticsIdentifier]. When set, a parent
  /// Semantics node carries the identifier above the IconButton's
  /// existing button/label semantics — Playwright finds the wrapper
  /// element via the `flt-semantics-identifier` DOM attribute on
  /// Flutter Web.
  final String? semanticsIdentifier;

  const IconAction({
    super.key,
    required this.icon,
    required this.semanticsLabel,
    required this.onPressed,
    this.tooltip,
    this.iconSize,
    this.color,
    this.padding,
    this.constraints,
    this.semanticsIdentifier,
  });

  @override
  Widget build(BuildContext context) {
    final button = IconButton(
      icon: Icon(icon, semanticLabel: semanticsLabel),
      tooltip: tooltip ?? semanticsLabel,
      onPressed: onPressed,
      iconSize: iconSize,
      color: color,
      padding: padding ?? const EdgeInsets.all(8),
      constraints: constraints,
    );
    if (semanticsIdentifier == null) {
      return button;
    }
    return Semantics(
      identifier: semanticsIdentifier,
      child: button,
    );
  }
}
