import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';

/// Actions selected from the request owner Manage menu.
///
/// Mirrors [ExperienceManageAction] one-for-one and gains a [viewImpact]
/// row that is only included after a request is fulfilled — the impact
/// surface is the same `ItemMetricsScreen` reached from the legacy
/// overflow menu, but the entry point now lives in this sheet.
enum RequestManageAction { editDetails, markFulfilled, closeRequest, viewImpact }

/// Bottom-sheet listing organizer Manage actions for a request — the
/// affordance opened by the pencil icon next to the title. Reuses
/// [PollManageMenuSheet] so the visual chrome stays in lockstep with the
/// location and time poll Manage menus and the event Manage menu.
class RequestManageMenuSheet extends StatelessWidget {
  /// When true the editing/settings rows (Edit Details, Mark Fulfilled, Close
  /// Request) are included. Set this for non-terminal requests; in terminal
  /// states those actions no longer apply and are hidden — analogous to the
  /// experience overflow hiding its settings while a poll is open.
  final bool showSettingsActions;

  /// When true a "View Impact" row is appended. Set this when the request is in
  /// the FULFILLED terminal state.
  final bool showViewImpact;

  const RequestManageMenuSheet({
    super.key,
    this.showSettingsActions = true,
    this.showViewImpact = false,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return PollManageMenuSheet<RequestManageAction>(
      items: [
        if (showSettingsActions) ...[
          PollManageMenuItem(
            icon: Icons.edit_outlined,
            label: l10n.requestManageEditDetails,
            description: l10n.requestManageEditDetailsDesc,
            action: RequestManageAction.editDetails,
          ),
          PollManageMenuItem(
            icon: Icons.check_circle_outline,
            label: l10n.requestManageMarkFulfilled,
            description: l10n.requestManageMarkFulfilledDesc,
            action: RequestManageAction.markFulfilled,
          ),
          PollManageMenuItem(
            icon: Icons.delete_outline,
            label: l10n.requestManageCloseRequest,
            description: l10n.requestManageCloseRequestDesc,
            destructive: true,
            action: RequestManageAction.closeRequest,
          ),
        ],
        if (showViewImpact)
          PollManageMenuItem(
            icon: Icons.insights,
            label: l10n.requestMenuViewImpact,
            action: RequestManageAction.viewImpact,
          ),
      ],
    );
  }
}
