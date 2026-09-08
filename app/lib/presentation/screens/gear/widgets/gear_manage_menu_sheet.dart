import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';

/// Item-level owner actions selected from the gear Manage menu (the top-bar
/// `···` overflow).
///
/// These are the management actions that apply to the gear *item* rather than a
/// specific borrower. Per-borrower workflow actions (confirm pickup time /
/// pickup / return, select recipient) stay on the sticky action button's
/// dropdown next to each borrower, because a flat enum sheet can't express the
/// variable-length, per-person action list a multi-loan item needs.
enum GearManageAction {
  editDetails,
  setLocation,
  viewImpact,
  logPast,
  closeItem,
}

/// Bottom-sheet listing item-level owner Manage actions for a gear item —
/// opened from the gear content view's top-bar overflow. Reuses
/// [PollManageMenuSheet] so the chrome stays in lockstep with the request and
/// experience Manage menus (see docs/client/manage_menus.md).
class GearManageMenuSheet extends StatelessWidget {
  /// Whether the gear is a giveaway (changes the "Log past" / "Close" copy).
  final bool isGiveaway;

  /// Whether to include the "View Impact" row. Set when the item has impact to
  /// show (a completed giveaway, or loan gear with `timesLoaned > 0`).
  final bool showViewImpact;

  const GearManageMenuSheet({
    super.key,
    required this.isGiveaway,
    required this.showViewImpact,
  });

  /// Convenience builder that derives the flags from [state].
  factory GearManageMenuSheet.forState(GearState state) {
    final gear = state.gearDetails;
    final isGiveaway =
        gear?.availability == Availability.AVAILABILITY_FOR_GIVEAWAY;
    final timesLoaned = gear?.hasTimesLoaned() ?? false ? gear!.timesLoaned : 0;
    return GearManageMenuSheet(
      isGiveaway: isGiveaway,
      showViewImpact: timesLoaned > 0,
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return PollManageMenuSheet<GearManageAction>(
      items: [
        PollManageMenuItem(
          icon: Icons.edit_outlined,
          label: l10n.gearManageEditDetails,
          description: l10n.gearManageEditDetailsDesc,
          action: GearManageAction.editDetails,
        ),
        PollManageMenuItem(
          icon: Icons.location_on_outlined,
          label: l10n.gearManageSetLocation,
          description: l10n.gearManageSetLocationDesc,
          action: GearManageAction.setLocation,
        ),
        if (showViewImpact)
          PollManageMenuItem(
            icon: Icons.insights,
            label: l10n.gearMenuViewImpact,
            action: GearManageAction.viewImpact,
          ),
        PollManageMenuItem(
          icon: Icons.history,
          label: isGiveaway
              ? l10n.pastTransferMenuGiveaway
              : l10n.pastTransferMenuLoan,
          action: GearManageAction.logPast,
        ),
        PollManageMenuItem(
          icon: Icons.delete_outline,
          label: isGiveaway
              ? l10n.gearMenuCloseGiveaway
              : l10n.gearMenuCloseLending,
          destructive: true,
          action: GearManageAction.closeItem,
        ),
      ],
    );
  }
}
