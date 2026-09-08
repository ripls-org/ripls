import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/needs/needs_copy.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';

/// Actions surfaced from the Needs organizer Manage menu (NO2 in the
/// v2 redesign). v2 trims the previous `addThing` + `markReady` rows
/// — Add lives as the inline AddOptionGhost in the list body now, and
/// the lock-in concept (`markReady`) is gone with v2's "list is alive
/// until the parent terminates" stance.
enum NeedsManageAction { editList, nudgeUnclaimed, cancelNeeds }

/// Opens the organizer Manage menu for a Needs list. Returns the picked
/// action, or `null` if dismissed.
///
/// The caller decides which actions to include — per the project
/// parity contract every row eligibility decision lives at the call
/// site, not inside this sheet.
Future<NeedsManageAction?> showNeedsManageMenu(
  BuildContext context, {
  required NeedsScopeKind scopeKind,
  bool showEditList = true,
  bool showNudgeUnclaimed = true,
  bool showCancel = true,
}) async {
  final copy = NeedsCopy(context.l10n, scopeKind);
  return showAccessibleModal<NeedsManageAction>(
    context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    barrierColor: AppColors.modalBackdrop,
    builder: (_) => PollManageMenuSheet<NeedsManageAction>(
      items: [
        if (showEditList)
          PollManageMenuItem(
            icon: Icons.edit,
            label: copy.manageEditList,
            description: copy.manageEditListDescription,
            action: NeedsManageAction.editList,
          ),
        if (showNudgeUnclaimed)
          PollManageMenuItem(
            icon: Icons.notifications_outlined,
            label: copy.manageNudgeUnclaimed,
            description: copy.manageNudgeUnclaimedDescription,
            action: NeedsManageAction.nudgeUnclaimed,
          ),
        if (showCancel)
          PollManageMenuItem(
            icon: Icons.cancel_outlined,
            label: copy.manageCancel,
            description: copy.manageCancelDescription,
            action: NeedsManageAction.cancelNeeds,
            destructive: true,
          ),
      ],
    ),
  );
}
