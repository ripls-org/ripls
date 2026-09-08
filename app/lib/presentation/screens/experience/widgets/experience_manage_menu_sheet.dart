import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';

/// Actions selected from the experience owner Manage menu.
enum ExperienceManageAction { editDetails, markCompleted, closeEvent }

/// Bottom-sheet listing organizer Manage actions for an active experience —
/// the affordance opened by the pencil icon next to the title. Reuses
/// [PollManageMenuSheet] so the visual chrome stays in lockstep with the
/// location and time poll Manage menus.
class ExperienceManageMenuSheet extends StatelessWidget {
  const ExperienceManageMenuSheet({super.key});

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return PollManageMenuSheet<ExperienceManageAction>(
      items: [
        PollManageMenuItem(
          icon: Icons.edit_outlined,
          label: l10n.experienceMenuUpdateDetails,
          description: l10n.experienceMenuUpdateDetailsDesc,
          action: ExperienceManageAction.editDetails,
        ),
        PollManageMenuItem(
          icon: Icons.check_circle_outline,
          label: l10n.experienceMenuMarkCompleted,
          description: l10n.experienceMenuMarkCompletedDesc,
          action: ExperienceManageAction.markCompleted,
        ),
        PollManageMenuItem(
          icon: Icons.delete_outline,
          label: l10n.experienceMenuCloseEvent,
          description: l10n.experienceMenuCloseEventDesc,
          destructive: true,
          action: ExperienceManageAction.closeEvent,
        ),
      ],
    );
  }
}
