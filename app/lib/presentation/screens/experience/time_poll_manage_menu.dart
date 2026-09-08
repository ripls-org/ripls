import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';

/// Action selected from the time-poll Manage menu. Mirrors
/// [LocationPollManageAction] one-for-one so the two flows look and feel the
/// same to organizers.
enum TimePollManageAction { addOption, setFinal, nudge, toggleLock, cancel }

/// Bottom-sheet listing organizer Manage actions for an active time poll.
/// Rendered as its own modal so the underlying vote modal stays interactive
/// after dismissal. Pop with a [TimePollManageAction] — or null if the user
/// dismisses without selecting. Composes [PollManageMenuSheet] for the row
/// chrome so both poll flows stay in lockstep.
class TimePollManageMenuSheet extends StatelessWidget {
  final bool hasProposals;
  final bool proposalsLocked;
  const TimePollManageMenuSheet({
    super.key,
    required this.hasProposals,
    required this.proposalsLocked,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return PollManageMenuSheet<TimePollManageAction>(
      items: [
        if (!proposalsLocked)
          PollManageMenuItem(
            icon: Icons.access_time_rounded,
            label: l10n.timePollManageAddOption,
            description: l10n.timePollManageAddOptionDesc,
            action: TimePollManageAction.addOption,
          ),
        PollManageMenuItem(
          icon: proposalsLocked ? Icons.lock_open : Icons.lock_outline,
          label: proposalsLocked
              ? l10n.timePollManageUnlock
              : l10n.timePollManageLock,
          description: proposalsLocked
              ? l10n.timePollManageUnlockDesc
              : l10n.timePollManageLockDesc,
          action: TimePollManageAction.toggleLock,
        ),
        PollManageMenuItem(
          icon: Icons.notifications_outlined,
          label: l10n.timePollManageNudge,
          description: l10n.timePollManageNudgeDesc,
          action: TimePollManageAction.nudge,
        ),
        if (hasProposals)
          PollManageMenuItem(
            icon: Icons.check,
            label: l10n.timePollManageSetFinal,
            description: l10n.timePollManageSetFinalDesc,
            action: TimePollManageAction.setFinal,
          ),
        PollManageMenuItem(
          icon: Icons.close,
          label: l10n.timePollManageCancelPoll,
          description: l10n.timePollManageCancelDesc,
          destructive: true,
          action: TimePollManageAction.cancel,
        ),
      ],
    );
  }
}
