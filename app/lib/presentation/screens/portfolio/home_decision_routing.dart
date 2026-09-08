import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/experience/mark_completed_modal.dart';
import 'package:ripls/presentation/screens/request/mark_fulfilled_modal.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/presentation/widgets/home/home_decision_copy.dart';
import 'package:ripls/presentation/widgets/transfer/confirmation_dialog.dart';
import 'package:ripls/services/providers.dart' show authStateProvider;

/// HomeDecisionRouting centralizes the Needs-you row behaviors shared by the
/// Home tab and its "See all" screen: opening the underlying item, running a
/// row's typed action, and the swipe-to-snooze background. Keeping these in
/// one place means the root preview and the full list behave identically.
mixin HomeDecisionRouting<T extends ConsumerStatefulWidget> on ConsumerState<T> {
  /// Opens the content screen for [contentId]/[itemType]. When [discussTab]
  /// is true it lands on the Discuss tab with the conversation expanded.
  void openHomeItem(String contentId, DailyItemType itemType,
      {bool discussTab = false}) {
    if (contentId.isEmpty) return;
    final routeType = switch (itemType) {
      DailyItemType.DAILY_ITEM_TYPE_TRANSFER ||
      DailyItemType.DAILY_ITEM_TYPE_GIVEAWAY =>
        'gear',
      DailyItemType.DAILY_ITEM_TYPE_EXPERIENCE => 'experience',
      DailyItemType.DAILY_ITEM_TYPE_REQUEST => 'request',
      _ => null,
    };
    if (routeType == null) return;
    unawaited(NavigationHelpers.pushToItemScreen(
      context: context,
      itemId: contentId,
      itemType: routeType,
      initialTab: discussTab ? 2 : 0,
    ));
  }

  void openDecision(HomeDecision decision, {bool discussTab = false}) {
    openHomeItem(decision.contentId, decision.itemType, discussTab: discussTab);
  }

  /// Routes the row's typed action: Lend/Give accepts in place; Say thanks
  /// acknowledges and opens the thread; Reply opens the conversation; Mark
  /// done opens the item's completion flow.
  Future<void> runDecisionAction(HomeDecision decision) async {
    switch (decision.kind) {
      case HomeDecisionKind.HOME_DECISION_KIND_ASK_CLAIM:
        unawaited(
            ref.read(homeTabProvider.notifier).resolveAskClaim(decision));
        openDecision(decision, discussTab: true);
        return;
      case HomeDecisionKind.HOME_DECISION_KIND_REPLY:
        openDecision(decision, discussTab: true);
        return;
      case HomeDecisionKind.HOME_DECISION_KIND_MARK_DONE:
        await _openMarkDoneModal(decision);
        return;
      case HomeDecisionKind.HOME_DECISION_KIND_TRANSFER_UPDATE:
        // Borrowing status update (mark picked up / returned) — confirm and
        // advance in place via a lightweight sheet instead of pushing the gear
        // screen. Falls back to opening the item if the action is unknown.
        await _confirmTransferUpdate(decision);
        return;
      default:
        final result =
            await ref.read(homeTabProvider.notifier).acceptDecision(decision);
        if (!mounted) return;
        if (result.error != null) {
          ToastHelper.showError(
              context, RpcErrorHandler.localize(result.error!, context.l10n));
          return;
        }
        // Offer an undo for the accept (un-selects the recipient and
        // re-surfaces the decision). Empty token → nothing to reverse.
        if (result.communityEventId.isNotEmpty) {
          ToastHelper.showServerUndo(
            context: context,
            message: context.l10n.homeDecisionAccepted,
            undoLabel: context.l10n.commonUndo,
            onUndo: () => ref
                .read(homeTabProvider.notifier)
                .undoAcceptDecision(decision, result.communityEventId),
          );
        }
    }
  }

  /// Opens the completion flow for a Mark-done row: the request fulfill modal
  /// for requests, or the experience completion modal for hosted events. Both are
  /// seeded from the decision's helper face stack so the user can confirm who
  /// pitched in without re-searching.
  Future<void> _openMarkDoneModal(HomeDecision decision) async {
    final helpers = decision.helpers.map(_userFromPerson).toList();
    switch (decision.itemType) {
      case DailyItemType.DAILY_ITEM_TYPE_EXPERIENCE:
        await MarkCompletedModal.show(
          context,
          decision.contentId,
          decision.communityId,
          preTaggedUsers: helpers,
        );
        return;
      case DailyItemType.DAILY_ITEM_TYPE_REQUEST:
        // Mark-done requests are the viewer's own, so the viewer is the owner.
        final ownerId = ref.read(authStateProvider).user?.id ?? '';
        await MarkFulfilledModal.show(
          context,
          requestId: decision.contentId,
          communityId: decision.communityId,
          requestTitle: decision.subjectTitle,
          ownerId: ownerId,
          offerers: helpers,
        );
        return;
      default:
        // No completion modal for this type — fall back to the item screen.
        openDecision(decision);
    }
  }

  /// Confirms and advances a borrowing status update in place: a lightweight
  /// confirm sheet, then the typed mutation, then a toast with an Undo. Replaces
  /// the old behavior of pushing the gear screen. Falls back to opening the item
  /// when the decision carries no typed action (older server payloads).
  Future<void> _confirmTransferUpdate(HomeDecision decision) async {
    if (decision.transferAction ==
        HomeTransferAction.HOME_TRANSFER_ACTION_UNSPECIFIED) {
      openDecision(decision);
      return;
    }
    final isPickup = decision.transferAction !=
        HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_LOAN;
    final item = decision.subjectTitle;
    final actionLabel = homeDecisionAcceptLabel(context.l10n, decision);

    final confirmed = await showConfirmationDialog(
      context: context,
      title: context.l10n.homeTransferConfirmTitle(actionLabel),
      message: isPickup
          ? context.l10n.homeTransferConfirmPickupMessage(item)
          : context.l10n.homeTransferConfirmReturnMessage(item),
      confirmLabel: actionLabel,
      cancelLabel: context.l10n.commonCancel,
    );
    if (confirmed != true || !mounted) return;

    final result =
        await ref.read(homeTabProvider.notifier).confirmTransferUpdate(decision);
    if (!mounted) return;
    if (result.error != null) {
      ToastHelper.showError(
          context, RpcErrorHandler.localize(result.error!, context.l10n));
      return;
    }
    if (result.communityEventId.isNotEmpty) {
      ToastHelper.showServerUndo(
        context: context,
        message: isPickup
            ? context.l10n.homeTransferPickedUp
            : context.l10n.homeTransferReturned,
        undoLabel: context.l10n.commonUndo,
        onUndo: () => ref
            .read(homeTabProvider.notifier)
            .undoTransferUpdate(decision, result.communityEventId),
      );
    }
  }

  User _userFromPerson(DailyPerson p) =>
      User(id: p.userId, name: p.displayName, mediaId: p.mediaId);
  }
