import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// PreviewTimeSelector renders a tappable time row used in the experience
/// preview modal.
///
/// [displayText] is the formatted string (e.g. "Saturday, May 3 at 4:00 PM"
/// or "TBD") supplied by the caller. [isPastDate] controls the amber warning
/// border and hint label shown when the extracted date is in the past. [isLoading]
/// disables the tap target while the experience is being submitted.
class PreviewTimeSelector extends StatelessWidget {
  const PreviewTimeSelector({
    super.key,
    required this.displayText,
    required this.isPastDate,
    required this.isLoading,
    required this.onTap,
  });

  final String displayText;
  final bool isPastDate;
  final bool isLoading;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Tappable(
          semanticsLabel: context.l10n.a11yExpSelectTime,
          onTap: isLoading ? null : onTap,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            decoration: BoxDecoration(
              color: GlassTokens.fillSubtle,
              borderRadius: BorderRadius.circular(12),
              border: Border.all(
                color: isPastDate
                    ? AppColors.statusWarningOnDark.withValues(alpha: 0.5)
                    : GlassTokens.borderSoft,
              ),
            ),
            child: Row(
              children: [
                const Icon(
                  Icons.access_time,
                  size: 20,
                  color: GlassTokens.textSecondary,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    displayText,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 14,
                      color: GlassTokens.textSecondary,
                    ),
                  ),
                ),
                const Icon(
                  Icons.chevron_right,
                  size: 20,
                  color: GlassTokens.textFaint,
                ),
              ],
            ),
          ),
        ),
        if (isPastDate) ...[
          const SizedBox(height: 4),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 4),
            child: Row(
              children: [
                Icon(
                  Icons.warning_amber_rounded,
                  size: 14,
                  color: AppColors.statusWarningOnDark.withValues(alpha: 0.9),
                ),
                const SizedBox(width: 4),
                Text(
                  'This date is in the past',
                  style: TextStyle(
                    fontSize: 12,
                    color: AppColors.statusWarningOnDark.withValues(alpha: 0.9),
                  ),
                ),
              ],
            ),
          ),
        ],
      ],
    );
  }
}
