import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// GlassInlineAction is the 44px row pattern used for fall-through actions
/// inside a glass bottom-sheet (e.g. "Pick another date…", "Custom time…").
///
/// Renders an optional leading icon, the action text, and a trailing chevron
/// against a `white/8` translucent fill.
class GlassInlineAction extends StatelessWidget {
  /// Action text (already localized).
  final String text;

  /// Localized accessibility label. Often equal to [text]; pass a more
  /// descriptive variant when the visible text is ambiguous.
  final String semanticsLabel;

  /// Optional leading icon (e.g. `Icons.calendar_today`).
  final IconData? icon;

  /// Tap callback. When null the row is rendered disabled.
  final VoidCallback? onTap;

  const GlassInlineAction({
    super.key,
    required this.text,
    required this.semanticsLabel,
    this.icon,
    this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(ModalTheme.inlineActionRadius),
      child: Container(
        height: ModalTheme.inlineActionHeight,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        decoration: BoxDecoration(
          color: AppColors.modalInlineActionBackground,
          borderRadius: BorderRadius.circular(ModalTheme.inlineActionRadius),
          border: Border.all(color: AppColors.modalInlineActionBorder),
        ),
        child: Row(
          children: [
            if (icon != null) ...[
              Icon(icon, size: 16, color: AppColors.modalInlineActionText),
              const SizedBox(width: 8),
            ],
            Expanded(
              child: Text(
                text,
                style: ModalTheme.inlineActionTextStyle.copyWith(
                  color: AppColors.modalInlineActionText,
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
            Icon(
              Icons.chevron_right,
              size: 18,
              color: AppColors.modalInlineActionChevron,
            ),
          ],
        ),
      ),
    );
  }
}
