import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// ContentInfoRow renders one row of the first-tab info stack
/// (time / location / owner) used by all four content views.
///
/// Layout matches the event-item-2 design: leading icon + value text,
/// optional trailing action (e.g. add-to-calendar, get directions),
/// and an optional thin bottom divider.
///
/// The row sits over a darkened image, so text colors are intentionally
/// `Colors.white` with alpha — they are not theme-aware. The accent
/// affordance is the caller's responsibility.
class ContentInfoRow extends StatelessWidget {
  /// Leading icon shown at the row's left edge. Pass `null` to render a
  /// non-icon leading slot (e.g. a small avatar) via [leading] instead.
  final IconData? icon;

  /// Optional custom leading widget (used by the owner row to drop in an
  /// avatar). Either [icon] or [leading] should be set, not both.
  final Widget? leading;

  /// Primary value shown next to the icon. Ignored when [valueBuilder] is set.
  final String value;

  /// Optional builder for a richer primary content area (e.g. attendee stack
  /// + count). When set it replaces the [value] Text in the row body.
  final Widget Function(BuildContext context)? valueBuilder;

  /// Localized semantic label for the row tap target. Pass `context.l10n.*`.
  final String rowSemanticsLabel;

  /// Optional secondary text rendered right-aligned (used for the owner
  /// row's "shared {relative} ago" timestamp).
  final String? trailingText;

  /// Tap on the row body (e.g. opens a picker). Null = row is non-tappable.
  final VoidCallback? onTap;

  /// Optional trailing icon-only action (calendar add, directions, etc.).
  final IconData? actionIcon;

  /// Localized semantic label for the trailing action. Required when
  /// [actionIcon] is set.
  final String? actionSemanticsLabel;

  /// Tap on the trailing action. Required when [actionIcon] is set.
  final VoidCallback? onAction;

  /// Optional custom trailing widget. When non-null, it replaces the
  /// [actionIcon] slot — useful when the trailing affordance is more
  /// complex than a single icon button (e.g. a small pill with a label).
  final Widget? trailing;

  /// Whether to draw the bottom hairline divider. False for the last row
  /// in the stack.
  final bool showDivider;

  const ContentInfoRow({
    super.key,
    this.icon,
    this.leading,
    required this.value,
    this.valueBuilder,
    required this.rowSemanticsLabel,
    this.trailingText,
    this.onTap,
    this.actionIcon,
    this.actionSemanticsLabel,
    this.onAction,
    this.trailing,
    this.showDivider = true,
  })  : assert(icon != null || leading != null,
            'ContentInfoRow needs either icon or leading'),
        assert(actionIcon == null || actionSemanticsLabel != null,
            'actionIcon requires actionSemanticsLabel'),
        assert(trailing == null || actionIcon == null,
            'trailing and actionIcon are mutually exclusive');

  /// Fixed visual height for every row in the info stack. Trailing
  /// icons and pills sit inside this envelope so a row with a calendar+
  /// action lines up perfectly with a row that has only an owner avatar
  /// + timestamp text.
  static const double _rowHeight = 44;

  @override
  Widget build(BuildContext context) {
    final body = SizedBox(
      height: _rowHeight,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          SizedBox(
            width: 18,
            height: 18,
            child: Center(
              child: icon != null
                  ? Icon(icon,
                      size: 16, color: Colors.white.withValues(alpha: 0.65))
                  : leading,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: valueBuilder != null
                ? valueBuilder!(context)
                : Text(
                    value,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      color: Colors.white,
                      fontSize: 14,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
          ),
          if (trailingText != null)
            Padding(
              padding: const EdgeInsets.only(left: 8),
              child: Text(
                trailingText!,
                style: TextStyle(
                  fontSize: 12.5,
                  color: Colors.white.withValues(alpha: 0.65),
                ),
              ),
            ),
          if (actionIcon != null)
            Padding(
              padding: const EdgeInsets.only(left: 4),
              child: IconAction(
                icon: actionIcon!,
                semanticsLabel: actionSemanticsLabel!,
                onPressed: onAction,
                iconSize: 18,
                color: Colors.white.withValues(alpha: 0.85),
                padding: const EdgeInsets.all(2),
                constraints:
                    const BoxConstraints.tightFor(width: 28, height: 28),
              ),
            ),
          if (trailing != null)
            Padding(
              padding: const EdgeInsets.only(left: 4),
              child: trailing!,
            ),
        ],
      ),
    );

    final tappableBody = onTap != null
        ? Tappable(
            semanticsLabel: rowSemanticsLabel,
            onTap: onTap,
            child: body,
          )
        : body;

    return Container(
      decoration: BoxDecoration(
        border: Border(
          bottom: BorderSide(
            color: showDivider
                ? Colors.white.withValues(alpha: 0.15)
                : Colors.transparent,
            width: 1,
          ),
        ),
      ),
      child: tappableBody,
    );
  }
}
