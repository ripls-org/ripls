import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/screens/experience/mark_completed_modal.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_manage_menu_sheet.dart';
import 'package:ripls/presentation/viewmodels/experience_sharing_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/content/close_item_modal.dart';

/// ExperienceCloseHandlersMixin provides the experience's destructive and
/// owner-management action handlers (cancel, unshare, delete, and the Manage
/// sheet) for [_ExperienceContentViewState].
///
/// Extracted to keep [experience_content_view.dart] under the project's
/// 1,000-line threshold.
mixin ExperienceCloseHandlersMixin<T extends ConsumerStatefulWidget>
    on ConsumerState<T> {
  /// experienceId must be provided by the mixing-in class.
  String get experienceId;

  /// Callback fired after a successful delete (provided by the mixing-in
  /// class, typically `widget.onDeleted`).
  VoidCallback? get onDeleted;

  /// handleCancelExperience confirms with the user and cancels the experience.
  Future<void> handleCancelExperience() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppColors.surface(context),
        title: Text(
          context.l10n.experienceCancelDialogTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        content: Text(
          context.l10n.experienceCancelDialogBody,
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
              context.l10n.experienceCancelConfirm,
              style: const TextStyle(color: Colors.red),
            ),
          ),
        ],
      ),
    );

    if (confirmed != true || !mounted) return;

    try {
      await ref
          .read(experienceProvider(experienceId).notifier)
          .cancelExperience();

      if (!mounted) return;
      ToastHelper.showSuccess(context, context.l10n.experienceCancelSuccess);
    } catch (e) {
      if (!mounted) return;
      ToastHelper.showError(
        context,
        RpcErrorHandler.localize(RpcErrorHandler.classify(e), context.l10n),
      );
    }
  }

  /// handleUnshareExperience confirms with the user and removes the experience
  /// from the current community feed.
  Future<void> handleUnshareExperience() async {
    final communityId = ref.read(experienceProvider(experienceId)).communityId;
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
          .read(experienceSharingNotifierProvider.notifier)
          .unshareFromCommunity(experienceId, communityId);

      if (!mounted) return;
      ToastHelper.showSuccess(context, context.l10n.commonUnshareSuccess);
      ref
          .read(experienceProvider(experienceId).notifier)
          .refreshExperienceDetails(communityId: communityId)
          .ignore();
    } catch (e) {
      if (!mounted) return;
      ToastHelper.showError(
        context,
        RpcErrorHandler.localize(RpcErrorHandler.classify(e), context.l10n),
      );
    }
  }

  /// handleDeleteExperience confirms with the user and permanently deletes the
  /// experience, firing [onDeleted] on success.
  Future<void> handleDeleteExperience() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppColors.surface(context),
        title: Text(
          'Delete Experience?',
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        content: Text(
          'Are you sure you want to delete this experience? It will be permanently removed.',
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
      await ref
          .read(experienceProvider(experienceId).notifier)
          .deleteExperience();

      if (!mounted) return;
      ToastHelper.showSuccess(context, 'Experience deleted');
      onDeleted?.call();
    } catch (error) {
      if (!mounted) return;
      ToastHelper.showError(context, error.toString());
    }
  }

  /// showManageSheet opens the owner Manage menu and dispatches the chosen
  /// action (edit details, mark completed, or close the event).
  Future<void> showManageSheet() async {
    final exp =
        ref.read(experienceProvider(experienceId)).experienceDetails?.experience;
    if (exp == null) return;

    final result = await showAccessibleModal<ExperienceManageAction>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => const ExperienceManageMenuSheet(),
    );
    if (result == null || !mounted) return;

    switch (result) {
      case ExperienceManageAction.editDetails:
        ref.read(experienceProvider(experienceId).notifier).toggleEditMode();
      case ExperienceManageAction.markCompleted:
        await markCompleted();
      case ExperienceManageAction.closeEvent:
        await closeEvent();
    }
  }

  /// markCompleted opens the "mark this event completed" flow. Exposed
  /// separately so the time/location panels can surface it in their overflow
  /// menus without re-opening the whole Manage sheet.
  Future<void> markCompleted() async {
    final state = ref.read(experienceProvider(experienceId));
    final exp = state.experienceDetails?.experience;
    if (exp == null) return;
    await MarkCompletedModal.show(
      context,
      exp.id,
      state.communityId ?? '',
      sharedCommunityIds: exp.sharedCommunityIds.toSet(),
    );
  }

  /// closeEvent opens the destructive close/cancel/delete flow for the event.
  Future<void> closeEvent() async {
    final state = ref.read(experienceProvider(experienceId));
    if (state.experienceDetails?.experience == null) return;
    await CloseItemModal.show(
      context,
      contentType: CloseItemContentType.event,
      onUnshare: state.communityId == null ? null : handleUnshareExperience,
      onCancel: handleCancelExperience,
      onDelete: handleDeleteExperience,
    );
  }
}
