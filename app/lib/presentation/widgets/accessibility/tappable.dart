import 'package:flutter/material.dart';

/// Tappable is the codebase default for any non-text widget that responds to
/// `onTap`. It replaces raw `GestureDetector` and `InkWell` with a thin wrapper
/// that requires a [semanticsLabel] so screen readers always announce the
/// element's purpose, and so the `require_l10n_for_semantic_labels` lint rule
/// can verify the label comes from `context.l10n`.
///
/// Use [Toggle] instead when the tap toggles a selectable state, and
/// [IconAction] for icon-only buttons that wrap [IconButton].
class Tappable extends StatelessWidget {
  /// Localized label announced to assistive tech. Required and non-empty.
  /// Pass `context.l10n.<key>` — the lint rule rejects raw string literals.
  final String semanticsLabel;

  /// Tap callback. When null the widget is rendered as disabled (greyed out
  /// hit testing remains active for visual layouts but the gesture is a noop).
  final VoidCallback? onTap;

  /// Long-press callback, optional.
  final VoidCallback? onLongPress;

  /// Whether the underlying element should be exposed as a button (default)
  /// or a link. Affects the role announced by VoiceOver/TalkBack.
  final bool isLink;

  /// Optional ink-splash material; when non-null, an [InkWell] is used so the
  /// material splash renders. When null, a [GestureDetector] wraps the child.
  final BorderRadius? inkBorderRadius;

  /// Optional `Material` color for the ink response. Only used when
  /// [inkBorderRadius] is non-null.
  final Color? inkSplashColor;

  /// Excludes descendants from the semantics tree so the wrapper's label is
  /// the only thing announced. Defaults to true — most tappable surfaces are
  /// composite widgets whose internal text is redundant with [semanticsLabel].
  /// Set to false when descendants carry information not in the label.
  final bool excludeChildSemantics;

  /// Optional kebab-case identifier exposed on the underlying Semantics
  /// node, queryable by Playwright via the `flt-semantics-identifier`
  /// DOM attribute on Flutter Web. Pass a stable identifier (e.g.
  /// `'event-hero-rsvp-yes'`) only when an e2e spec needs to target
  /// this widget — not speculatively. Defaults to null; not announced
  /// by screen readers. See docs/client/testing/semantics_identifiers.md.
  final String? semanticsIdentifier;

  final Widget child;

  const Tappable({
    super.key,
    required this.semanticsLabel,
    required this.onTap,
    required this.child,
    this.onLongPress,
    this.isLink = false,
    this.inkBorderRadius,
    this.inkSplashColor,
    this.excludeChildSemantics = true,
    this.semanticsIdentifier,
  });

  @override
  Widget build(BuildContext context) {
    final enabled = onTap != null || onLongPress != null;
    final body = excludeChildSemantics
        ? ExcludeSemantics(child: child)
        : child;

    final Widget gesture;
    if (inkBorderRadius != null) {
      gesture = Material(
        color: Colors.transparent,
        child: InkWell(
          onTap: onTap,
          onLongPress: onLongPress,
          borderRadius: inkBorderRadius,
          splashColor: inkSplashColor,
          child: body,
        ),
      );
    } else {
      gesture = GestureDetector(
        onTap: onTap,
        onLongPress: onLongPress,
        behavior: HitTestBehavior.opaque,
        child: body,
      );
    }

    return Semantics(
      identifier: semanticsIdentifier,
      label: semanticsLabel,
      button: !isLink,
      link: isLink,
      enabled: enabled,
      excludeSemantics: false,
      child: gesture,
    );
  }
}
