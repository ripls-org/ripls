import 'dart:async';

import 'package:flutter/material.dart';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show TransferType;
import 'package:ripls/presentation/models/item_metric_data_base.dart';
import 'package:ripls/presentation/screens/gear/past_transfer_modal.dart';
import 'package:ripls/presentation/screens/gear/transfer_modal_router.dart';
import 'package:ripls/presentation/screens/item/item_metrics_screen.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/services/providers.dart';

/// GearTransferHandlersMixin provides the transfer and gear deletion action
/// handlers for [_GearContentViewState].
///
/// Extracted from the main composer file to keep it under the project's
/// 1,000-line threshold. The mixin has full access to [BuildContext],
/// [WidgetRef], and [State.mounted] via the [ConsumerState] supertype.
mixin GearTransferHandlersMixin<T extends ConsumerStatefulWidget>
    on ConsumerState<T> {
  /// gearId must be overridden by the mixing-in class.
  String get gearId;

  /// onDeleted is called after the gear item has been successfully deleted.
  VoidCallback? get onDeleted;

  // ── Transfer action handlers ─────────────────────────────────────────────

  /// refreshAfterTransferAction reloads gear state after a transfer mutation.
  /// Repository methods already invalidate their own caches during the mutation,
  /// so we just need to reload the gear details (which re-fetches transfer
  /// context) and signal other listeners.
  Future<void> refreshAfterTransferAction() async {
    final communityId = ref.read(gearProvider(gearId)).communityId;
    if (communityId == null || !mounted) return;
    await ref
        .read(gearRepositoryProvider)
        .invalidate(gearId, communityId: communityId);
    if (!mounted) return;
    await ref
        .read(gearProvider(gearId).notifier)
        .loadGearDetails(communityId: communityId);
    ref.read(transferCacheInvalidationProvider.notifier).notify();
  }

  /// handleMarkGiveawayPickedUp completes a giveaway transfer (marks item as
  /// picked up by the selected recipient).
  Future<void> handleMarkGiveawayPickedUp(String transferId) async {
    try {
      await ref
          .read(transferRepositoryProvider)
          .completeTransfer(transferId: transferId);
      if (!mounted) return;
      await refreshAfterTransferAction();
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to mark as picked up: $e');
      }
    }
  }

  /// handleStartLoan marks a loan as picked up (RECIPIENT_SELECTED → ACTIVE).
  Future<void> handleStartLoan(String transferId) async {
    try {
      await ref
          .read(transferRepositoryProvider)
          .startLoan(transferId: transferId);
      if (!mounted) return;
      await refreshAfterTransferAction();
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to confirm pickup: $e');
      }
    }
  }

  /// handleCompleteLoan marks a loan as returned (ACTIVE → COMPLETED).
  Future<void> handleCompleteLoan(String transferId) async {
    try {
      await ref
          .read(transferRepositoryProvider)
          .completeTransfer(transferId: transferId);
      if (!mounted) return;
      await refreshAfterTransferAction();
      if (!mounted) return;
      showGearEquityModal();
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to confirm return: $e');
      }
    }
  }

  /// handleCancelLoan cancels a loan after confirming with the user.
  Future<void> handleCancelLoan(String transferId) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppColors.surface(context),
        title: Text(
          context.l10n.loanCancelDialogTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        content: Text(
          context.l10n.loanCancelDialogBody,
          style: TextStyle(color: AppColors.textSecondary(context)),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              context.l10n.commonCancel,
              style: TextStyle(color: AppColors.textSecondary(context)),
            ),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: Text(
              context.l10n.loanCancelConfirm,
              style: const TextStyle(color: Colors.red),
            ),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    try {
      await ref
          .read(transferRepositoryProvider)
          .cancelTransfer(transferId: transferId);
      if (!mounted) return;
      await refreshAfterTransferAction();
      if (!mounted) return;
      ToastHelper.showSuccess(context, context.l10n.loanCancelSuccess);
    } catch (e) {
      if (!mounted) return;
      ToastHelper.showError(context, 'Failed to cancel loan: $e');
    }
  }

  /// handleCancelGiveaway cancels a giveaway transfer after confirming with the
  /// user.
  Future<void> handleCancelGiveaway(String transferId) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppColors.surface(context),
        title: Text(
          context.l10n.giveawayCancelDialogTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        content: Text(
          context.l10n.giveawayCancelDialogBody,
          style: TextStyle(color: AppColors.textSecondary(context)),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              context.l10n.commonCancel,
              style: TextStyle(color: AppColors.textSecondary(context)),
            ),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: Text(
              context.l10n.giveawayCancelConfirm,
              style: const TextStyle(color: Colors.red),
            ),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    try {
      await ref
          .read(transferRepositoryProvider)
          .cancelTransfer(transferId: transferId);
      if (!mounted) return;
      await refreshAfterTransferAction();
      if (!mounted) return;
      ToastHelper.showSuccess(context, context.l10n.giveawayCancelSuccess);
    } catch (e) {
      if (!mounted) return;
      ToastHelper.showError(context, 'Failed to cancel giveaway: $e');
    }
  }

  /// handleUnshareGearFromCommunity removes the gear from the current community
  /// after confirming with the user.
  Future<void> handleUnshareGearFromCommunity() async {
    final state = ref.read(gearProvider(gearId));
    final communityId = state.communityId;
    if (communityId == null) return;

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppColors.surface(context),
        title: Text(
          context.l10n.commonUnshareDialogTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        content: Text(
          context.l10n.commonUnshareDialogBody,
          style: TextStyle(color: AppColors.textSecondary(context)),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              context.l10n.commonCancel,
              style: TextStyle(color: AppColors.textSecondary(context)),
            ),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: Text(
              context.l10n.commonUnshareConfirm,
              style: const TextStyle(color: Colors.red),
            ),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    try {
      await ref
          .read(communityRepositoryProvider)
          .unshareGear(gearId: gearId, communityId: communityId);
      if (!mounted) return;
      await refreshAfterTransferAction();
      if (!mounted) return;
      ToastHelper.showSuccess(context, context.l10n.commonUnshareSuccess);
    } catch (e) {
      if (!mounted) return;
      ToastHelper.showError(context, 'Failed to remove from community: $e');
    }
  }

  /// handleDeleteGear confirms and deletes the gear item, then calls [onDeleted].
  Future<void> handleDeleteGear() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppColors.surface(context),
        title: Text(
          'Delete Item?',
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        content: Text(
          'Are you sure you want to delete this item? It will be permanently removed from all communities.',
          style: TextStyle(color: AppColors.textSecondary(context)),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              context.l10n.commonCancel,
              style: TextStyle(color: AppColors.textSecondary(context)),
            ),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: Text(
              context.l10n.commonDelete,
              style: const TextStyle(color: Colors.red),
            ),
          ),
        ],
      ),
    );

    if (confirmed != true || !mounted) return;

    try {
      await ref.read(gearProvider(gearId).notifier).deleteGear();
      if (!mounted) return;
      ToastHelper.showSuccess(context, 'Item deleted');
      onDeleted?.call();
    } catch (error) {
      if (!mounted) return;
      ToastHelper.showError(context, error.toString());
    }
  }

  /// openPastTransferModal shows the past transfer modal for the given
  /// [transferType] and refreshes gear details on success.
  Future<void> openPastTransferModal(
    GearState state,
    TransferType transferType,
  ) async {
    final gear = state.gearDetails;
    if (gear == null) return;
    final communityId = state.communityId;
    if (communityId == null) {
      ToastHelper.showError(context, 'No community selected');
      return;
    }
    if (!context.mounted) return;
    final result = await PastTransferModal.show(
      context,
      gearId: gear.id,
      gearName: gear.name,
      communityId: communityId,
      transferType: transferType,
      ownerId: gear.owner.id,
      sharedCommunityIds:
          state.sharedCommunities.map((c) => c.communityId).toSet(),
    );
    if ((result ?? false) && mounted) {
      unawaited(ref.read(gearProvider(gearId).notifier).refreshGearDetails(
            communityId: communityId,
          ));
    }
  }

  /// openTransferModal opens the transfer modal router for the gear item.
  Future<void> openTransferModal() async {
    final state = ref.read(gearProvider(gearId));
    final gear = state.gearDetails;
    if (gear == null) return;

    final communityId = state.communityId;
    if (communityId == null) {
      ToastHelper.showError(context, 'No community selected');
      return;
    }

    Availability? availability = gear.availability;
    if (availability == Availability.AVAILABILITY_UNSPECIFIED) {
      try {
        final communityService = ref.read(communityServiceProvider);
        final communityGear =
            await communityService.listCommunityGear(communityId);
        final matchingGear = communityGear.where((g) => g.id == gearId);
        if (matchingGear.isNotEmpty) {
          availability = matchingGear.first.availability;
        }
      } catch (e) {
        debugPrint('Error fetching availability: $e');
      }
    }

    if (!mounted) return;

    final authState = ref.read(authStateProvider);
    final currentUserId = authState.user?.id;
    final isOwner = currentUserId != null && gear.owner.id == currentUserId;

    await TransferModalRouter.show(
      context,
      gear: gear,
      isOwner: isOwner,
      communityId: communityId,
      availability: availability,
    );

    if (mounted) {
      await ref
          .read(transferRepositoryProvider)
          .invalidateGearTransfers(gearId);
      await ref
          .read(transferRepositoryProvider)
          .invalidateUserTransferStatus(gearId);
      await ref
          .read(gearRepositoryProvider)
          .invalidate(gearId, communityId: communityId);
      await ref
          .read(gearProvider(gearId).notifier)
          .loadGearDetails(communityId: communityId);
      if (mounted) setState(() {});
    }
  }

  /// showGearEquityModal opens the item metrics screen for the gear.
  void showGearEquityModal() {
    final state = ref.read(gearProvider(gearId));
    final gear = state.gearDetails;
    final communityId = state.communityId;
    if (communityId == null || gear == null) return;

    NavigationHelpers.pushScreen(
      context: context,
      screen: ItemMetricsScreen(
        itemType: ItemType.gear,
        itemId: gear.id,
        communityId: communityId,
      ),
      routeName: 'item_metrics',
    );
  }
}
