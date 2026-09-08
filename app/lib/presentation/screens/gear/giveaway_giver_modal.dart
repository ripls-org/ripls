import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/viewmodels/giveaway_giver_state.dart';
import 'package:ripls/presentation/viewmodels/giveaway_giver_view_model.dart';
import 'package:ripls/presentation/widgets/transfer/recipient_selection_card.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_modal_shell.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_phase_bottom_bar.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_utils.dart';
import 'package:ripls/services/providers.dart';

/// GiveawayGiverModal is a focused recipient picker for giveaway owners.
///
/// Shows a list of interested users and lets the owner select one.
/// All other giveaway management (pickup coordination, completion) is
/// handled by the gear detail managing menu.
class GiveawayGiverModal extends ConsumerStatefulWidget {
  final String gearId;
  final String gearName;
  final String? transferId;

  const GiveawayGiverModal({
    super.key,
    required this.gearId,
    required this.gearName,
    this.transferId,
  });

  @override
  ConsumerState<GiveawayGiverModal> createState() => _GiveawayGiverModalState();
}

class _GiveawayGiverModalState extends ConsumerState<GiveawayGiverModal> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref
          .read(giveawayGiverProvider.notifier)
          .initialize(gearId: widget.gearId, transferId: widget.transferId);
    });
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(giveawayGiverProvider);

    return SizedBox(
      height: MediaQuery.of(context).size.height * 0.6,
      child: TransferModalShell(
        isLoading: state.isLoading,
        content: SingleChildScrollView(
          child: Padding(
            padding: const EdgeInsets.only(top: 16),
            child: _buildRecipientList(context, state),
          ),
        ),
        bottomBar: Padding(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 12),
          child: _buildBottomBar(context, state),
        ),
      ),
    );
  }

  Widget _buildRecipientList(BuildContext context, GiveawayGiverState state) {
    if (state.pendingRequests.isEmpty) {
      return _buildNoRequests(context);
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: Text(
            context.l10n.gearMenuSelectRecipient,
            style: Theme.of(context).textTheme.titleMedium?.copyWith(
                  fontWeight: FontWeight.w600,
                  color: AppColors.modalTextPrimary,
                ),
          ),
        ),
        const SizedBox(height: 8),
        for (final request in state.pendingRequests)
          RecipientSelectionCard(
            key: Key('recipient_selection_card_${request.id}'),
            user: request.recipient,
            timestamp:
                formatRelativeTimestamp(request.latestRequestUnixSec.toInt()),
            isSelected: state.selectedRecipientId == request.recipient.id,
            onTap: () {
              ref
                  .read(giveawayGiverProvider.notifier)
                  .setSelectedRecipient(request.recipient.id);
            },
          ),
        const SizedBox(height: 16),
      ],
    );
  }

  Widget _buildNoRequests(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.pending_outlined,
              size: 64,
              color: AppColors.modalTextMuted,
            ),
            const SizedBox(height: 16),
            Text(
              context.l10n.gearMenuWaitingForInterest,
              style: Theme.of(context).textTheme.titleLarge?.copyWith(
                    fontWeight: FontWeight.w600,
                    color: AppColors.modalTextPrimary,
                  ),
            ),
            const SizedBox(height: 8),
            Text(
              context.l10n.giveawayNoRequestsDescription,
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                    color: AppColors.modalTextSecondary,
                  ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildBottomBar(BuildContext context, GiveawayGiverState state) {
    return TransferPhaseBottomBar(
      primaryLabel: context.l10n.gearMenuSelectRecipient,
      onPrimaryPressed: state.hasSelectedRecipient
          ? () => _selectRecipient(context, state)
          : null,
      isLoading: state.isLoading,
    );
  }

  Future<void> _selectRecipient(
    BuildContext context,
    GiveawayGiverState state,
  ) async {
    if (state.selectedRecipientId == null || state.pendingRequests.isEmpty) {
      return;
    }

    final selectedRequest = state.pendingRequests.firstWhere(
      (r) => r.recipient.id == state.selectedRecipientId,
    );

    final eventId = await ref
        .read(giveawayGiverProvider.notifier)
        .selectRecipient(selectedRequest.id, state.selectedRecipientId!);

    if (mounted && context.mounted) {
      Navigator.pop(context, true);
      if (eventId != null && eventId.isNotEmpty) {
        ToastHelper.showServerUndo(
          context: context,
          message: 'Recipient selected',
          undoLabel: 'Undo',
          onUndo: () => ref
              .read(transferServiceProvider)
              .undoSelectRecipient(communityEventId: eventId),
          onUndoSucceeded: () {
            ref.invalidate(giveawayGiverProvider);
          },
        );
      }
    }
  }
}
