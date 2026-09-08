import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show Transfer;
import 'package:ripls/data/gen/ripls/api/transfer.pbenum.dart'
    show GiveawayPhase, TransferState;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/arrange_pickup_modal.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/chat/action_dropdown_menu.dart'
    show ActionDropdownItem;
import 'package:ripls/presentation/widgets/content/checklist_status_icons.dart'
    as checklist;
import 'package:ripls/presentation/widgets/content/close_item_modal.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// GearMenuItems builds the [ActionDropdownItem] lists for the gear content
/// view's action buttons.
///
/// This is a pure helper class — no widget state. Every builder method takes
/// the current [GearState] and callback functions so it can be called from
/// [_GearContentViewState] without coupling the menu construction to the
/// parent's widget tree.
class GearMenuItems {
  GearMenuItems._();

  /// buildGiveawayOwnerItems returns the checklist-style dropdown items for
  /// a giveaway owner's Managing button.
  static List<ActionDropdownItem> buildGiveawayOwnerItems({
    required BuildContext context,
    required GearState state,
    required VoidCallback onToggleEditMode,
    required VoidCallback onLocationTap,
    required VoidCallback onOpenTransferModal,
    required VoidCallback onOpenPastTransferModal,
    required VoidCallback onShowEquityModal,
    required Future<void> Function(String transferId) onMarkGiveawayPickedUp,
    required Future<void> Function() onDeleteGear,
    Future<void> Function(String transferId)? onCancelTransfer,
    Future<void> Function()? onUnshareFromCommunity,
  }) {
    final ctx = state.transferContext;
    final gear = state.gearDetails;
    final transfer = ctx?.hasUserTransfer() ?? false ? ctx!.userTransfer : null;
    final isCompleted =
        ctx?.overallPhase == GiveawayPhase.GIVEAWAY_PHASE_COMPLETED;
    final isCancelled =
        ctx?.overallPhase == GiveawayPhase.GIVEAWAY_PHASE_CANCELLED;
    final isRecipientSelected =
        transfer?.state == TransferState.TRANSFER_STATE_RECIPIENT_SELECTED;
    final recipient =
        transfer?.hasRecipient() ?? false ? transfer!.recipient : null;

    if (isCancelled) {
      return const [];
    }

    if (isCompleted && recipient != null) {
      return [
        ActionDropdownItem(
          icon: Icons.person,
          label: recipient.name,
          leadingWidget: buildUserAvatar(recipient),
        ),
        ActionDropdownItem(
          icon: Icons.insights,
          label: context.l10n.gearMenuViewImpact,
          leadingWidget: checklist.buildStatusCircle(
            context: context,
            backgroundColor: AppColors.rsvpYesBackground(context),
            iconColor: AppColors.rsvpYesText(context),
            icon: Icons.insights,
            semanticLabel: context.l10n.a11yGearViewImpact,
          ),
          onTap: onShowEquityModal,
        ),
      ];
    }

    if (isRecipientSelected && recipient != null) {
      return [
        buildSetDetailsItem(
          context: context,
          state: state,
          onTap: onToggleEditMode,
        ),
        buildConfirmLocationItem(
          context: context,
          state: state,
          onTap: onLocationTap,
        ),
        ActionDropdownItem(
          icon: Icons.person,
          label: recipient.name,
          leadingWidget: buildUserAvatar(recipient),
          children: [
            ActionDropdownItem(
              icon: Icons.calendar_today_outlined,
              label: context.l10n.gearMenuConfirmPickupTime,
              leadingWidget: checklist.buildPickupTimeStatusIcon(
                context: context,
                hasPickupTime: transfer!.hasEstimatedPickupUnixSec(),
              ),
              onTap: () => ArrangePickupModal.show(
                context,
                transferId: transfer.id,
                gearName: gear?.name ?? '',
                isOwner: true,
                isGiveaway: true,
              ),
            ),
            ActionDropdownItem(
              icon: Icons.card_giftcard,
              label: context.l10n.gearMenuConfirmReceived,
              leadingWidget: checklist.buildReceivedStatusIcon(
                context: context,
                isCompleted: false,
              ),
              onTap: () => onMarkGiveawayPickedUp(transfer.id),
            ),
          ],
        ),
        ActionDropdownItem(
          icon: Icons.delete_outline,
          label: context.l10n.gearMenuCloseGiveaway,
          isDestructive: true,
          onTap: () => CloseItemModal.show(
            context,
            contentType: CloseItemContentType.item,
            onUnshare: onUnshareFromCommunity,
            onCancel: onCancelTransfer != null
                ? () => onCancelTransfer(transfer.id)
                : null,
            onDelete: onDeleteGear,
          ),
        ),
      ];
    }

    final hasPendingRequests = ctx != null && ctx.pendingRequests.isNotEmpty;
    return [
      buildSetDetailsItem(
        context: context,
        state: state,
        onTap: onToggleEditMode,
      ),
      buildConfirmLocationItem(
        context: context,
        state: state,
        onTap: onLocationTap,
      ),
      ActionDropdownItem(
        icon: Icons.person_add_outlined,
        label: hasPendingRequests
            ? context.l10n.gearMenuSelectRecipient
            : context.l10n.gearMenuWaitingForInterest,
        onTap: hasPendingRequests ? onOpenTransferModal : null,
      ),
      ActionDropdownItem(
        icon: Icons.history,
        label: context.l10n.pastTransferMenuGiveaway,
        onTap: onOpenPastTransferModal,
      ),
      ActionDropdownItem(
        icon: Icons.delete_outline,
        label: context.l10n.gearMenuCloseGiveaway,
        isDestructive: true,
        onTap: () => CloseItemModal.show(
          context,
          contentType: CloseItemContentType.item,
          onUnshare: onUnshareFromCommunity,
          onCancel: null,
          onDelete: onDeleteGear,
        ),
      ),
    ];
  }

  /// buildGiveawayRecipientItems returns the checklist-style dropdown items
  /// for a selected giveaway recipient (non-owner).
  static List<ActionDropdownItem> buildGiveawayRecipientItems({
    required BuildContext context,
    required Transfer transfer,
    required String gearName,
    required GearState state,
    required Future<void> Function(String transferId) onMarkGiveawayPickedUp,
    required Future<void> Function(String transferId) onCancelTransfer,
  }) {
    final isCompleted =
        state.transferContext?.overallPhase == GiveawayPhase.GIVEAWAY_PHASE_COMPLETED;
    return [
      ActionDropdownItem(
        icon: Icons.calendar_today_outlined,
        label: context.l10n.gearRecipientArrangePickup,
        leadingWidget: checklist.buildPickupTimeStatusIcon(
          context: context,
          hasPickupTime: transfer.hasEstimatedPickupUnixSec(),
        ),
        onTap: () => ArrangePickupModal.show(
          context,
          transferId: transfer.id,
          gearName: gearName,
          isGiveaway: true,
        ),
      ),
      ActionDropdownItem(
        icon: Icons.card_giftcard,
        label: context.l10n.gearRecipientConfirmReceived,
        leadingWidget: checklist.buildReceivedStatusIcon(
          context: context,
          isCompleted: isCompleted,
        ),
        onTap: isCompleted ? null : () => onMarkGiveawayPickedUp(transfer.id),
      ),
      ActionDropdownItem(
        icon: Icons.cancel_outlined,
        label: context.l10n.gearRecipientWithdrawInterest,
        isDestructive: true,
        onTap: () => onCancelTransfer(transfer.id),
      ),
    ];
  }

  /// buildBorrowerLoanItems returns the checklist-style dropdown items for a
  /// non-owner borrower with an active loan request on this gear.
  static List<ActionDropdownItem> buildBorrowerLoanItems({
    required BuildContext context,
    required Transfer transfer,
    required String gearName,
    required bool canAct,
    required Future<void> Function(String transferId) onStartLoan,
    required Future<void> Function(String transferId) onCompleteLoan,
    required Future<void> Function(String transferId) onCancelTransfer,
  }) {
    final transferId = transfer.id;
    final hasPickupTime = transfer.hasEstimatedPickupUnixSec();
    final isPickedUp = transfer.hasActualPickupUnixSec();
    final isReturned = transfer.hasActualReturnUnixSec();
    return [
      ActionDropdownItem(
        icon: Icons.calendar_today_outlined,
        label: hasPickupTime
            ? context.l10n.gearMenuPickupPlanned
            : context.l10n.gearBorrowerArrangePickup,
        leadingWidget: checklist.buildPickupTimeStatusIcon(
          context: context,
          hasPickupTime: hasPickupTime,
        ),
        isCompleted: hasPickupTime,
        onTap: () => ArrangePickupModal.show(
          context,
          transferId: transferId,
          gearName: gearName,
        ),
      ),
      ActionDropdownItem(
        icon: Icons.inventory_2_outlined,
        label: isPickedUp
            ? context.l10n.gearMenuPickedUp
            : context.l10n.gearBorrowerConfirmPickedUp,
        leadingWidget: checklist.buildPickupStatusIcon(
          context: context,
          isPickedUp: isPickedUp,
        ),
        isCompleted: isPickedUp,
        onTap: !canAct || isPickedUp
            ? null
            : () => onStartLoan(transferId),
      ),
      ActionDropdownItem(
        icon: Icons.assignment_return_outlined,
        label: isReturned
            ? context.l10n.gearMenuReturned
            : context.l10n.gearBorrowerConfirmReturned,
        leadingWidget: checklist.buildReturnStatusIcon(
          context: context,
          isReturned: isReturned,
        ),
        isCompleted: isReturned,
        onTap: !canAct || isReturned
            ? null
            : () => onCompleteLoan(transferId),
      ),
      ActionDropdownItem(
        icon: Icons.cancel_outlined,
        label: context.l10n.gearBorrowerCancelRequest,
        isDestructive: true,
        onTap: () => onCancelTransfer(transferId),
      ),
    ];
  }

  /// buildLoanOwnerItems returns the checklist-style dropdown items for a
  /// loan owner's Managing button.
  static List<ActionDropdownItem> buildLoanOwnerItems({
    required BuildContext context,
    required GearState state,
    required VoidCallback onToggleEditMode,
    required VoidCallback onLocationTap,
    required VoidCallback onOpenPastTransferModal,
    required VoidCallback onShowEquityModal,
    required Future<void> Function(String transferId) onStartLoan,
    required Future<void> Function(String transferId) onCompleteLoan,
    required Future<void> Function() onDeleteGear,
    Future<void> Function(String transferId)? onCancelTransfer,
    Future<void> Function()? onUnshareFromCommunity,
  }) {
    final ctx = state.transferContext;
    final gear = state.gearDetails;
    final gearName = gear?.name ?? '';
    final transfer =
        ctx?.hasUserTransfer() ?? false ? ctx!.userTransfer : null;

    final borrowerSections = <ActionDropdownItem>[];

    if (transfer != null &&
        (transfer.state == TransferState.TRANSFER_STATE_RECIPIENT_SELECTED ||
            transfer.state == TransferState.TRANSFER_STATE_ACTIVE) &&
        transfer.hasRecipient()) {
      borrowerSections.add(buildBorrowerSection(
        context: context,
        borrower: transfer.recipient,
        transferId: transfer.id,
        gearName: gearName,
        hasPickupTime: transfer.hasEstimatedPickupUnixSec(),
        isPickedUp: transfer.hasActualPickupUnixSec(),
        isReturned: transfer.hasActualReturnUnixSec(),
        onStartLoan: onStartLoan,
        onCompleteLoan: onCompleteLoan,
      ));
    }

    if (ctx != null) {
      for (final req in ctx.pendingRequests) {
        if (transfer != null && req.transferId == transfer.id) continue;
        borrowerSections.add(buildUpcomingBorrowerSection(
          context: context,
          borrower: req.borrower,
          transferId: req.transferId,
          gearName: gearName,
          hasPickupTime: false,
        ));
      }
    }

    final items = <ActionDropdownItem>[
      buildSetDetailsItem(
        context: context,
        state: state,
        onTap: onToggleEditMode,
      ),
      buildConfirmLocationItem(
        context: context,
        state: state,
        onTap: onLocationTap,
      ),
    ];

    if (borrowerSections.isNotEmpty) {
      items.addAll(borrowerSections);
    }

    final timesLoaned =
        gear?.hasTimesLoaned() ?? false ? gear!.timesLoaned : 0;
    if (timesLoaned > 0) {
      items.add(ActionDropdownItem(
        icon: Icons.insights,
        label: context.l10n.gearMenuViewImpact,
        leadingWidget: checklist.buildStatusCircle(
          context: context,
          backgroundColor: AppColors.rsvpYesBackground(context),
          iconColor: AppColors.rsvpYesText(context),
          icon: Icons.insights,
          semanticLabel: context.l10n.a11yGearViewImpact,
        ),
        onTap: onShowEquityModal,
      ));
    }

    items.add(ActionDropdownItem(
      icon: Icons.history,
      label: context.l10n.pastTransferMenuLoan,
      onTap: onOpenPastTransferModal,
    ));

    items.add(ActionDropdownItem(
      icon: Icons.delete_outline,
      label: context.l10n.gearMenuCloseLending,
      isDestructive: true,
      onTap: () => CloseItemModal.show(
        context,
        contentType: CloseItemContentType.loan,
        onUnshare: onUnshareFromCommunity,
        onCancel: onCancelTransfer != null && transfer != null
            ? () => onCancelTransfer(transfer.id)
            : null,
        onDelete: onDeleteGear,
      ),
    ));

    return items;
  }

  /// buildSetDetailsItem returns the shared Set Details checklist item.
  static ActionDropdownItem buildSetDetailsItem({
    required BuildContext context,
    required GearState state,
    required VoidCallback onTap,
  }) {
    final gear = state.gearDetails;
    return ActionDropdownItem(
      icon: Icons.edit_outlined,
      label: context.l10n.gearMenuSetDetails,
      leadingWidget: checklist.buildDetailsStatusIcon(
        context: context,
        hasTitle: gear?.name.isNotEmpty ?? false,
        hasDescription: gear?.description.isNotEmpty ?? false,
        hasMedia: gear != null && gear.mediaIds.isNotEmpty,
      ),
      onTap: onTap,
    );
  }

  /// buildConfirmLocationItem returns the shared Confirm Location checklist item.
  static ActionDropdownItem buildConfirmLocationItem({
    required BuildContext context,
    required GearState state,
    required VoidCallback onTap,
  }) {
    return ActionDropdownItem(
      icon: Icons.location_on_outlined,
      label: context.l10n.gearMenuConfirmLocation,
      leadingWidget: checklist.buildLocationStatusIcon(
        context: context,
        locationName: state.locationName,
      ),
      onTap: onTap,
    );
  }

  /// buildUserAvatar returns a 36x36 [UserAvatar] for a menu leading widget.
  static Widget buildUserAvatar(User user) {
    return UserAvatar(user: user, radius: 18);
  }

  /// buildBorrowerSection returns a grouped borrower header with Arrange
  /// Pickup, Confirm Pickup, and Confirm Return children.
  static ActionDropdownItem buildBorrowerSection({
    required BuildContext context,
    required User borrower,
    required String transferId,
    required String gearName,
    required bool isPickedUp,
    required bool isReturned,
    required bool hasPickupTime,
    required Future<void> Function(String transferId) onStartLoan,
    required Future<void> Function(String transferId) onCompleteLoan,
  }) {
    return ActionDropdownItem(
      icon: Icons.person,
      label: borrower.name,
      leadingWidget: buildUserAvatar(borrower),
      children: [
        ActionDropdownItem(
          icon: Icons.calendar_today_outlined,
          label: hasPickupTime
              ? context.l10n.gearMenuPickupPlanned
              : context.l10n.gearMenuConfirmPickupTime,
          leadingWidget: checklist.buildPickupTimeStatusIcon(
            context: context,
            hasPickupTime: hasPickupTime,
          ),
          isCompleted: hasPickupTime,
          onTap: () => ArrangePickupModal.show(
            context,
            transferId: transferId,
            gearName: gearName,
            isOwner: true,
          ),
        ),
        ActionDropdownItem(
          icon: Icons.inventory_2_outlined,
          label: isPickedUp
              ? context.l10n.gearMenuPickedUp
              : context.l10n.gearMenuConfirmPickup,
          leadingWidget: checklist.buildPickupStatusIcon(
            context: context,
            isPickedUp: isPickedUp,
          ),
          isCompleted: isPickedUp,
          onTap: isPickedUp ? null : () => onStartLoan(transferId),
        ),
        ActionDropdownItem(
          icon: Icons.assignment_return_outlined,
          label: isReturned
              ? context.l10n.gearMenuReturned
              : context.l10n.gearMenuConfirmReturn,
          leadingWidget: checklist.buildReturnStatusIcon(
            context: context,
            isReturned: isReturned,
          ),
          isCompleted: isReturned,
          onTap: isReturned ? null : () => onCompleteLoan(transferId),
        ),
      ],
    );
  }

  /// buildUpcomingBorrowerSection returns a borrower header with only
  /// "Arrange Pickup" — no confirm steps since this borrower is not yet active.
  static ActionDropdownItem buildUpcomingBorrowerSection({
    required BuildContext context,
    required User borrower,
    required String transferId,
    required String gearName,
    required bool hasPickupTime,
  }) {
    return ActionDropdownItem(
      icon: Icons.person,
      label: borrower.name,
      leadingWidget: buildUserAvatar(borrower),
      children: [
        ActionDropdownItem(
          icon: Icons.calendar_today_outlined,
          label: context.l10n.gearRecipientArrangePickup,
          leadingWidget: checklist.buildPickupTimeStatusIcon(
            context: context,
            hasPickupTime: hasPickupTime,
          ),
          onTap: () => ArrangePickupModal.show(
            context,
            transferId: transferId,
            gearName: gearName,
            isOwner: true,
          ),
        ),
      ],
    );
  }
}
