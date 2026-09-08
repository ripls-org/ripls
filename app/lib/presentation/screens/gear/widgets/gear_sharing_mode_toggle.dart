import 'package:connectrpc/connect.dart' show Code;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/transfer/confirmation_dialog.dart';

/// GearSharingModeToggle is the owner-facing Lend / Give-away segmented control
/// shown at the top of the gear edit pane. Tapping a segment flips the item's
/// availability across every community it is shared with (via
/// [GearNotifier.setAvailability]).
///
/// - Loan → giveaway asks for confirmation first (a giveaway permanently
///   transfers ownership), then shows a success toast.
/// - Giveaway → loan applies immediately and offers an undo toast.
/// - The control is locked (and a hint shown) while a loan or giveaway is in
///   progress — the server rejects a mode change mid-transfer.
class GearSharingModeToggle extends ConsumerWidget {
  const GearSharingModeToggle({super.key, required this.gearId});

  final String gearId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(gearProvider(gearId));

    // Owner-only, and only once details (hence the current mode) have loaded.
    if (!state.isOwner || state.gearDetails == null) {
      return const SizedBox.shrink();
    }

    final l10n = context.l10n;
    final isGiveaway = state.isGiveaway;
    final locked = state.hasInFlightTransfer;
    final busy = state.isSaving;
    final interactable = !locked && !busy;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          l10n.gearEditSharingModeLabel,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w600,
            color: AppColors.textSecondary(context),
          ),
        ),
        const SizedBox(height: 8),
        Opacity(
          opacity: locked ? 0.5 : 1.0,
          child: Row(
            children: [
              Expanded(
                child: _ModeSegment(
                  label: l10n.gearEditSharingLend,
                  icon: Icons.autorenew,
                  isSelected: !isGiveaway,
                  semanticsIdentifier: 'gear-availability-lend',
                  onTap: interactable && isGiveaway
                      ? () => _select(context, ref,
                          Availability.AVAILABILITY_FOR_LOAN)
                      : null,
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: _ModeSegment(
                  label: l10n.gearEditSharingGiveAway,
                  icon: Icons.card_giftcard,
                  isSelected: isGiveaway,
                  semanticsIdentifier: 'gear-availability-give-away',
                  onTap: interactable && !isGiveaway
                      ? () => _select(context, ref,
                          Availability.AVAILABILITY_FOR_GIVEAWAY)
                      : null,
                ),
              ),
            ],
          ),
        ),
        if (locked) ...[
          const SizedBox(height: 6),
          Text(
            l10n.gearEditSharingLockedHint,
            style: TextStyle(
              fontSize: 12,
              color: AppColors.textSecondary(context),
            ),
          ),
        ],
      ],
    );
  }

  /// Applies the requested mode. Confirms loan→giveaway first (permanent),
  /// shows a success/undo toast on completion, and surfaces the localized
  /// server message if the change is rejected.
  Future<void> _select(
    BuildContext context,
    WidgetRef ref,
    Availability target,
  ) async {
    final l10n = context.l10n;
    final notifier = ref.read(gearProvider(gearId).notifier);

    if (target == Availability.AVAILABILITY_FOR_GIVEAWAY) {
      final confirmed = await showConfirmationDialog(
        context: context,
        title: l10n.gearGiveAwayConfirmTitle,
        message: l10n.gearGiveAwayConfirmMessage,
        confirmLabel: l10n.gearGiveAwayConfirmConfirm,
        cancelLabel: l10n.gearGiveAwayConfirmKeepLending,
        isDestructive: true,
      );
      if (confirmed != true) return;
    }
    if (!context.mounted) return;

    try {
      await notifier.setAvailability(target);
      if (!context.mounted) return;
      if (target == Availability.AVAILABILITY_FOR_GIVEAWAY) {
        ToastHelper.showSuccess(context, l10n.gearAvailabilityNowGiveaway);
      } else {
        ToastHelper.showUndo(
          context: context,
          message: l10n.gearAvailabilityNowLending,
          undoLabel: l10n.commonUndo,
          onUndo: () =>
              notifier.setAvailability(Availability.AVAILABILITY_FOR_GIVEAWAY),
        );
      }
    } catch (e) {
      if (!context.mounted) return;
      // A FailedPrecondition here is the in-progress-transfer guard; show the
      // same specific hint the locked control uses. (The RPC wrapper flattens
      // the localized detail to a generic per-code message, so map it here.)
      final message = e is ServiceException && e.code == Code.failedPrecondition
          ? l10n.gearEditSharingLockedHint
          : RpcErrorHandler.localize(RpcErrorHandler.classify(e), l10n);
      ToastHelper.showError(context, message);
    }
  }
}

/// _ModeSegment is one segment of the Lend / Give-away control. Mode is conveyed
/// by icon + text (never colour alone); the selected state is a heavier border
/// and accent fill, and is exposed to assistive tech via [Toggle.selected].
class _ModeSegment extends StatelessWidget {
  const _ModeSegment({
    required this.label,
    required this.icon,
    required this.isSelected,
    required this.semanticsIdentifier,
    required this.onTap,
  });

  final String label;
  final IconData icon;
  final bool isSelected;
  final String semanticsIdentifier;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.primary(context);
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final selectedBg = accent.withValues(alpha: isDark ? 0.16 : 0.12);
    final fg = isSelected ? accent : AppColors.textSecondary(context);

    return Toggle(
      semanticsLabel: label,
      selected: isSelected,
      inMutuallyExclusiveGroup: true,
      semanticsIdentifier: semanticsIdentifier,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(10),
      child: Container(
        // 48dp min touch target: 14+20-icon/text+14 vertical padding.
        constraints: const BoxConstraints(minHeight: 48),
        padding: const EdgeInsets.symmetric(vertical: 12, horizontal: 8),
        decoration: BoxDecoration(
          color: isSelected ? selectedBg : AppColors.cardBackground(context),
          borderRadius: BorderRadius.circular(10),
          border: Border.all(
            color: isSelected ? accent : AppColors.border(context),
            width: isSelected ? 2 : 1,
          ),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(icon, size: 18, color: fg),
            const SizedBox(width: 6),
            Flexible(
              child: Text(
                label,
                textAlign: TextAlign.center,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                  color: fg,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
