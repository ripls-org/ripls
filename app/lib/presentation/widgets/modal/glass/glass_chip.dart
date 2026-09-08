import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/content/need_quantity_badge.dart';

/// GlassChip is the two-line selectable chip used across glass bottom-sheet
/// modals. Inactive chips render with a translucent background; active chips
/// flip to a solid white fill with near-black text.
///
/// Use [Toggle] semantics: the chip toggles a selectable state, so the
/// parent passes [selected]. The press-feedback scale animation respects
/// the system reduce-motion setting via `accessibleDuration`.
class GlassChip extends StatefulWidget {
  /// Primary text line (e.g. "Today").
  final String primary;

  /// Optional smaller secondary line below [primary] (e.g. "May 8").
  final String? secondary;

  /// Whether this chip is currently selected.
  final bool selected;

  /// Tap callback. When null the chip is rendered as disabled.
  final VoidCallback? onTap;

  /// Localized label announced to screen readers. The selected state is
  /// announced separately by [Toggle], so do not embed "selected" here.
  final String semanticsLabel;

  /// Optional quantity. When greater than 1 a "×N" badge renders after
  /// [primary]. Defaults to 1 (no badge).
  final int quantity;

  /// When true, a small comment glyph renders after [primary] to signal an
  /// attached note. Defaults to false.
  final bool hasComment;

  const GlassChip({
    super.key,
    required this.primary,
    required this.selected,
    required this.onTap,
    required this.semanticsLabel,
    this.secondary,
    this.quantity = 1,
    this.hasComment = false,
  });

  @override
  State<GlassChip> createState() => _GlassChipState();
}

class _GlassChipState extends State<GlassChip> {
  bool _pressed = false;

  void _setPressed(bool value) {
    if (!mounted || _pressed == value) return;
    setState(() => _pressed = value);
  }

  @override
  Widget build(BuildContext context) {
    final selected = widget.selected;
    final bg = selected
        ? AppColors.modalChipBackgroundActive
        : AppColors.modalChipBackground;
    final border = selected
        ? AppColors.modalChipBorderActive
        : AppColors.modalChipBorder;
    final primaryColor = selected
        ? AppColors.modalChipTextActive
        : AppColors.modalChipText;
    final secondaryColor = selected
        ? AppColors.modalChipTextSecondaryActive
        : AppColors.modalChipTextSecondary;

    final body = AnimatedScale(
      duration: accessibleDuration(context, ModalTheme.pressDuration),
      curve: Curves.easeOut,
      scale: _pressed ? ModalTheme.pressScale : 1.0,
      child: Container(
        padding: const EdgeInsets.symmetric(
          horizontal: ModalTheme.chipPaddingHorizontal,
          vertical: ModalTheme.chipPaddingVertical,
        ),
        decoration: BoxDecoration(
          color: bg,
          borderRadius: BorderRadius.circular(ModalTheme.chipRadius),
          border: Border.all(color: border),
          boxShadow: selected
              ? [
                  BoxShadow(
                    color: Colors.black.withValues(alpha: 0.06),
                    blurRadius: 6,
                    offset: const Offset(0, 1),
                  ),
                ]
              : null,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Flexible(
                  child: Text(
                    widget.primary,
                    style: ModalTheme.chipPrimaryStyle.copyWith(
                      color: primaryColor,
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                if (widget.quantity > 1) ...[
                  const SizedBox(width: 4),
                  NeedQuantityBadge(
                    quantity: widget.quantity,
                    style: ModalTheme.chipPrimaryStyle.copyWith(
                      color: primaryColor,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
                if (widget.hasComment) ...[
                  const SizedBox(width: 5),
                  Icon(
                    Icons.mode_comment_outlined,
                    size: 12,
                    color: primaryColor,
                  ),
                ],
              ],
            ),
            if (widget.secondary != null) ...[
              const SizedBox(height: 2),
              Text(
                widget.secondary!,
                style: ModalTheme.chipSecondaryStyle.copyWith(
                  color: secondaryColor,
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ],
        ),
      ),
    );

    return Toggle(
      semanticsLabel: widget.semanticsLabel,
      selected: selected,
      onTap: widget.onTap,
      child: Listener(
        onPointerDown: (_) => _setPressed(true),
        onPointerUp: (_) => _setPressed(false),
        onPointerCancel: (_) => _setPressed(false),
        child: body,
      ),
    );
  }
}
