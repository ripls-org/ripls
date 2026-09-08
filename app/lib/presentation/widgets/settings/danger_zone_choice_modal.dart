import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// One of three outcomes the Danger Zone choice modal can produce.
///
/// Powers the §2.1 / §2.3 fork — owner taps Delete Community in the
/// Danger Zone, sees this modal, picks Delete-for-everyone (destructive,
/// 30-day soft delete), Leave-with-handoff (transfer ownership and
/// leave), or Cancel (do nothing). See
/// `docs/community_delete_and_leave.md`.
enum DangerZoneChoice {
  /// Soft-delete the community for all members. Goes to the
  /// DeleteCommunityScreen confirmation.
  delete,

  /// Owner-leave path: transfer ownership to another member, then
  /// leave. Goes to the member-picker (#1719).
  leaveWithHandoff,

  /// Cancel and dismiss the modal — no side effect.
  cancel,
}

/// Presents the §2.1 step 2 choice modal for owners. Returns the
/// selected [DangerZoneChoice], or `null` if the user dismissed via
/// drag handle / barrier tap (which is equivalent to Cancel).
Future<DangerZoneChoice?> showDangerZoneChoiceModal({
  required BuildContext context,
  required String communityName,
}) {
  return showAccessibleModal<DangerZoneChoice>(
    context,
    backgroundColor: Colors.transparent,
    isScrollControlled: true,
    barrierColor: AppColors.modalBackdrop,
    builder: (sheetContext) => GlassSheet(
      applyMaxHeight: false,
      child: _DangerZoneChoiceSheet(communityName: communityName),
    ),
  );
}

class _DangerZoneChoiceSheet extends StatelessWidget {
  final String communityName;

  const _DangerZoneChoiceSheet({required this.communityName});

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(4, 0, 4, 16),
          child: Semantics(
            header: true,
            child: Text(
              l10n.dangerZoneChoiceTitle,
              style: ModalTheme.headerValueStyle.copyWith(
                color: AppColors.modalTextPrimary,
                fontSize: 17,
              ),
            ),
          ),
        ),
        _ChoiceRow(
          icon: Icons.delete_outline,
          iconColor: AppColors.statusWarningOnDark,
          label: l10n.dangerZoneDeleteForEveryone(communityName),
          semanticsLabel:
              l10n.a11yDangerZoneDeleteForEveryone(communityName),
          onTap: () =>
              Navigator.of(context).pop(DangerZoneChoice.delete),
        ),
        const SizedBox(height: 4),
        _ChoiceRow(
          icon: Icons.exit_to_app,
          iconColor: AppColors.modalTextPrimary,
          label: l10n.dangerZoneJustLeave,
          semanticsLabel: l10n.a11yDangerZoneJustLeave,
          onTap: () => Navigator.of(context)
              .pop(DangerZoneChoice.leaveWithHandoff),
        ),
        const SizedBox(height: 4),
        _ChoiceRow(
          icon: Icons.close,
          iconColor: AppColors.modalTextMuted,
          label: l10n.commonCancel,
          semanticsLabel: l10n.commonCancel,
          onTap: () =>
              Navigator.of(context).pop(DangerZoneChoice.cancel),
        ),
      ],
    );
  }
}

class _ChoiceRow extends StatelessWidget {
  final IconData icon;
  final Color iconColor;
  final String label;
  final String semanticsLabel;
  final VoidCallback onTap;

  const _ChoiceRow({
    required this.icon,
    required this.iconColor,
    required this.label,
    required this.semanticsLabel,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(12),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 14),
        child: Row(
          children: [
            Icon(icon, color: iconColor),
            const SizedBox(width: 16),
            Expanded(
              child: Text(
                label,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                  color: AppColors.modalTextPrimary,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
