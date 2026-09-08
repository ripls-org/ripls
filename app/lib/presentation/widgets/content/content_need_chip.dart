import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/dashed_border.dart';
import 'package:ripls/presentation/widgets/content/need_quantity_badge.dart';

/// Visual style of a [ContentNeedChip].
enum ContentNeedChipStyle {
  /// An open need nobody has filled yet — a dashed "empty slot": neutral dashed
  /// border, a bright label, and a leading accent "+".
  open,

  /// A need someone has claimed — accent outline with a check.
  claimed,

  /// An "add" affordance — neutral dashed with a leading "+".
  add,
}

/// ContentNeedChip is a chip in the "Needed" section of the pitching-in surface
/// (docs/issues/2280-pitching-in-expand.md). It renders on the dark content
/// surface — a dashed empty-slot with an accent "+" for an open need, an accent
/// outline + check for a claimed one, and a neutral dashed "+ …" for the add
/// affordances. Pure; the caller resolves copy and wires [onTap].
class ContentNeedChip extends StatelessWidget {
  final String label;
  final ContentNeedChipStyle style;

  /// The view's accent (the "claimed" / check / open-"+" color).
  final Color accentColor;

  /// Tap action. Null renders the chip inert (no gesture, and an open chip
  /// drops its leading "+" claim affordance) — the wrapped-event treatment
  /// where the list is a record, not an invitation (#2724).
  final VoidCallback? onTap;
  final String semanticsLabel;

  /// How many of this need are wanted (its slot count). A `×N` badge renders
  /// after the label when this is greater than 1; 1 shows nothing.
  final int quantity;

  /// Optional faint suffix after the label (e.g. "· you" attributing a claimed
  /// need to the viewer). Shown only on [ContentNeedChipStyle.claimed].
  final String? attribution;

  const ContentNeedChip({
    super.key,
    required this.label,
    required this.style,
    required this.accentColor,
    required this.onTap,
    required this.semanticsLabel,
    this.quantity = 1,
    this.attribution,
  });

  @override
  Widget build(BuildContext context) {
    // (label color, leading-icon color, leading icon, decoration) per style.
    final (Color textColor, Color leadingColor, IconData leading, Decoration decoration) =
        switch (style) {
          ContentNeedChipStyle.open => (
            AppColors.onContentImage,
            accentColor,
            Icons.add_rounded,
            ShapeDecoration(
              color: AppColors.onContentImage.withValues(alpha: 0.12),
              shape: const DashedBorder(color: AppColors.modalChipBorder),
            ),
          ),
          ContentNeedChipStyle.claimed => (
            AppColors.darkTextSecondary,
            accentColor,
            Icons.check_rounded,
            BoxDecoration(
              color: accentColor.withValues(alpha: 0.22),
              borderRadius: BorderRadius.circular(10),
              border: Border.all(color: accentColor),
            ),
          ),
          ContentNeedChipStyle.add => (
            AppColors.darkTextTertiary,
            AppColors.darkTextTertiary,
            Icons.add_rounded,
            ShapeDecoration(
              color: AppColors.onContentImage.withValues(alpha: 0.08),
              shape: const DashedBorder(color: AppColors.darkTextTertiary),
            ),
          ),
        };

    // An inert open chip drops its "+" — the claimed check stays (it reports
    // status, not an action).
    final showLeading =
        onTap != null || style == ContentNeedChipStyle.claimed;

    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 13, vertical: 9),
        decoration: decoration,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (showLeading) ...[
              Icon(leading, size: 15, color: leadingColor),
              const SizedBox(width: 6),
            ],
            Text(
              label,
              style: TextStyle(
                color: textColor,
                fontSize: 13.5,
                fontWeight: FontWeight.w600,
              ),
            ),
            if (quantity > 1) ...[
              const SizedBox(width: 4),
              NeedQuantityBadge(
                quantity: quantity,
                style: TextStyle(
                  color: textColor,
                  fontSize: 13.5,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ],
            if (style == ContentNeedChipStyle.claimed && attribution != null) ...[
              const SizedBox(width: 5),
              Text(
                attribution!,
                style: const TextStyle(
                  color: AppColors.darkTextTertiary,
                  fontSize: 12,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
