import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_settings_menu_items.dart';
import 'package:ripls/presentation/viewmodels/time_modal_state.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';

/// Owner overflow-menu actions for the "When" panel: poll management (while a
/// poll runs) plus the experience-settings actions carried over from the
/// content view's Manage sheet.
enum TimeManageAction {
  addOption,
  setFinal,
  toggleLock,
  nudge,
  cancelPoll,
  markCompleted,
  closeEvent,
}

/// The owner overflow menu for the "When" panel: poll-management actions
/// (while a poll is active) followed by the experience-settings actions, so
/// it's useful even before a poll is started.
///
/// Returns the chosen action, or null if the sheet was dismissed. The caller
/// owns what each action does — this only presents the choices.
Future<TimeManageAction?> showTimeManageMenu(
  BuildContext context,
  TimeModalData data,
) {
  final l10n = context.l10n;
  final pollActive = data.timePollActive;
  return showAccessibleModal<TimeManageAction>(
    context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    barrierColor: AppColors.modalBackdrop,
    builder: (_) => PollManageMenuSheet<TimeManageAction>(
      items: [
        if (pollActive) ...[
          if (!data.proposalsLocked)
            PollManageMenuItem(
              icon: Icons.access_time_rounded,
              label: l10n.timePollManageAddOption,
              description: l10n.timePollManageAddOptionDesc,
              action: TimeManageAction.addOption,
            ),
          PollManageMenuItem(
            icon: data.proposalsLocked ? Icons.lock_open : Icons.lock_outline,
            label: data.proposalsLocked
                ? l10n.timePollManageUnlock
                : l10n.timePollManageLock,
            description: data.proposalsLocked
                ? l10n.timePollManageUnlockDesc
                : l10n.timePollManageLockDesc,
            action: TimeManageAction.toggleLock,
          ),
          PollManageMenuItem(
            icon: Icons.notifications_outlined,
            label: l10n.timePollManageNudge,
            description: l10n.timePollManageNudgeDesc,
            action: TimeManageAction.nudge,
          ),
          if (data.proposals.isNotEmpty)
            PollManageMenuItem(
              icon: Icons.check,
              label: l10n.timePollManageSetFinal,
              description: l10n.timePollManageSetFinalDesc,
              action: TimeManageAction.setFinal,
            ),
          PollManageMenuItem(
            icon: Icons.close,
            label: l10n.timePollManageCancelPoll,
            description: l10n.timePollManageCancelDesc,
            destructive: true,
            action: TimeManageAction.cancelPoll,
          ),
        ]
        // No poll running → the event-level settings (hidden while a poll is
        // open; the poll must complete first).
        else
          ...experienceSettingsMenuItems(
            context,
            markCompleted: TimeManageAction.markCompleted,
            closeEvent: TimeManageAction.closeEvent,
          ),
      ],
    ),
  );
}

/// Confirmation for the destructive "cancel poll" action. True to cancel.
Future<bool?> confirmCancelTimePoll(BuildContext context) {
  final l10n = context.l10n;
  return showDialog<bool>(
    context: context,
    builder: (dialogCtx) => AlertDialog(
      title: Text(l10n.timePollManageCancelPoll),
      content: Text(l10n.timePollManageCancelDesc),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(dialogCtx).pop(false),
          child: Text(l10n.commonCancel),
        ),
        TextButton(
          onPressed: () => Navigator.of(dialogCtx).pop(true),
          child: Text(l10n.timePollManageCancelPoll),
        ),
      ],
    ),
  );
}
