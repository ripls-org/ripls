import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';

/// RowCallToActionPill is the small toggle chip rendered in the trailing
/// slot of a [ContentInfoRow]. It carries a binary "acted vs. not acted"
/// state: coral solid CTA pre-action ("Help" / "Choose" / "Offer") and a
/// translucent sage confirmation chip post-action ("Helped" / "Chosen" /
/// "Offered"). Used by the event-content and request-content row stacks
/// to surface the next action a viewer can take without an extra modal.
///
/// Built on [Toggle] because the pill carries selectable state — the
/// `selected:` semantic flag announces the state instead of baking it
/// into the visible label.
class RowCallToActionPill extends StatelessWidget {
  final bool chosen;
  final String actionLabel;
  final String chosenLabel;
  final VoidCallback? onTap;

  const RowCallToActionPill({
    super.key,
    required this.chosen,
    required this.actionLabel,
    required this.chosenLabel,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final label = chosen ? chosenLabel : actionLabel;
    final Color bgColor;
    final Color borderColor;
    final Color textColor;
    if (chosen) {
      bgColor = AppColors.experienceSageGreen.withValues(alpha: 0.22);
      borderColor = AppColors.experienceSageGreen.withValues(alpha: 0.35);
      textColor = AppColors.experienceSageGreenSoftText;
    } else {
      bgColor = AppColors.transferCoralSoft;
      borderColor = AppColors.transferCoralSoft;
      textColor = AppColors.onContentImage;
    }
    return Toggle(
      semanticsLabel: label,
      selected: chosen,
      onTap: onTap,
      child: Container(
        height: 28,
        padding: const EdgeInsets.symmetric(horizontal: 12),
        decoration: BoxDecoration(
          color: bgColor,
          border: Border.all(color: borderColor),
          borderRadius: BorderRadius.circular(14),
        ),
        alignment: Alignment.center,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (chosen) ...[
              Icon(Icons.check_rounded, size: 14, color: textColor),
              const SizedBox(width: 4),
            ],
            Text(
              label,
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w600,
                color: textColor,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
