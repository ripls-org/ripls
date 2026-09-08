import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Bottom action bar for the gear metadata sheet.
///
/// Shows three visual states: no edits (disabled grey), has edits (primary
/// colour, tappable), and saved (sage/green, disabled). When tapped in the
/// has-edits state, [onSave] is called.
class MetadataSaveBar extends StatelessWidget {
  final bool hasEdits;
  final bool isSaved;
  final double bottomInset;
  final VoidCallback onSave;

  const MetadataSaveBar({
    super.key,
    required this.hasEdits,
    required this.isSaved,
    required this.bottomInset,
    required this.onSave,
  });

  @override
  Widget build(BuildContext context) {
    final Color bgColor;
    final Color textColor;
    final String label;
    final bool tappable;

    if (isSaved) {
      bgColor = AppColors.transferSageBackground(context);
      textColor = AppColors.statusSuccess(context);
      label = 'Saved';
      tappable = false;
    } else if (hasEdits) {
      bgColor = AppColors.modalPrimaryButtonBackground;
      textColor = AppColors.modalPrimaryButtonText;
      label = 'Save Changes';
      tappable = true;
    } else {
      bgColor = AppColors.modalSecondaryButtonBackground;
      textColor = AppColors.modalTextMuted;
      label = 'No changes yet';
      tappable = false;
    }

    return Container(
      padding: EdgeInsets.fromLTRB(20, 10, 20, 20 + bottomInset),
      decoration: BoxDecoration(
        border: Border(
          top: BorderSide(color: AppColors.modalFooterDivider),
        ),
      ),
      child: AnimatedContainer(
        duration: accessibleDuration(context, const Duration(milliseconds: 200)),
        width: double.infinity,
        height: 50,
        decoration: BoxDecoration(
          color: bgColor,
          borderRadius: BorderRadius.circular(14),
        ),
        child: Tappable(
          semanticsLabel: context.l10n.a11yGearSaveMetadata,
          inkBorderRadius: BorderRadius.circular(14),
          onTap: tappable ? onSave : null,
          child: Center(
            child: Row(
              mainAxisSize: MainAxisSize.min,
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                if (isSaved) ...[
                  Icon(Icons.check, size: 16, color: textColor),
                  const SizedBox(width: 6),
                ],
                Text(
                  label,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: textColor,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
