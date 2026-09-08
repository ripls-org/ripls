import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';

/// Action selected from the location-poll Manage menu (LO1 in
/// `docs/cowork/App Design/location-redesign.html`).
enum LocationPollManageAction { addSpot, setFinal, nudge, toggleLock, cancel }

/// Bottom-sheet listing organizer Manage actions for an active location
/// poll. Rendered as its own modal so the underlying vote modal stays
/// interactive after dismissal. Pop with a [LocationPollManageAction] —
/// or null if the user dismisses without selecting. Composes
/// [PollManageMenuSheet] for the row chrome so the time and location flows
/// stay in lockstep.
class LocationPollManageMenuSheet extends StatelessWidget {
  final bool hasProposals;
  final bool proposalsLocked;
  const LocationPollManageMenuSheet({
    super.key,
    required this.hasProposals,
    required this.proposalsLocked,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return PollManageMenuSheet<LocationPollManageAction>(
      items: [
        if (!proposalsLocked)
          PollManageMenuItem(
            icon: Icons.place_outlined,
            label: l10n.locationPollManageAddSpot,
            description: l10n.locationPollManageAddSpotDesc,
            action: LocationPollManageAction.addSpot,
          ),
        PollManageMenuItem(
          icon: proposalsLocked ? Icons.lock_open : Icons.lock_outline,
          label: proposalsLocked
              ? l10n.locationPollManageUnlock
              : l10n.locationPollManageLock,
          description: proposalsLocked
              ? l10n.locationPollManageUnlockDesc
              : l10n.locationPollManageLockDesc,
          action: LocationPollManageAction.toggleLock,
        ),
        PollManageMenuItem(
          icon: Icons.notifications_outlined,
          label: l10n.locationPollManageNudge,
          description: l10n.locationPollManageNudgeDesc,
          action: LocationPollManageAction.nudge,
        ),
        if (hasProposals)
          PollManageMenuItem(
            icon: Icons.check,
            label: l10n.locationPollManageSetFinal,
            description: l10n.locationPollManageSetFinalDesc,
            action: LocationPollManageAction.setFinal,
          ),
        PollManageMenuItem(
          icon: Icons.close,
          label: l10n.locationPollManageCancelPoll,
          description: l10n.locationPollManageCancelDesc,
          destructive: true,
          action: LocationPollManageAction.cancel,
        ),
      ],
    );
  }
}
