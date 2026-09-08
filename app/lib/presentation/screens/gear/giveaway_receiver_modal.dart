import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show Transfer;
import 'package:ripls/presentation/viewmodels/giveaway_receiver_state.dart';
import 'package:ripls/presentation/viewmodels/giveaway_receiver_view_model.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_modal_shell.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_utils.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// GiveawayReceiverModal shows giveaway status from the receiver's perspective.
///
/// Displays the list of interested users (read-only) and the receiver's own
/// status. All actions (mark picked up, withdraw) are on the gear detail
/// managing menu.
class GiveawayReceiverModal extends ConsumerStatefulWidget {
  final String transferId;
  final String gearId;
  final String gearName;

  const GiveawayReceiverModal({
    super.key,
    required this.transferId,
    required this.gearId,
    required this.gearName,
  });

  @override
  ConsumerState<GiveawayReceiverModal> createState() =>
      _GiveawayReceiverModalState();
}

class _GiveawayReceiverModalState extends ConsumerState<GiveawayReceiverModal> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(giveawayReceiverProvider.notifier).initialize(
            transferId: widget.transferId,
            gearId: widget.gearId,
          );
    });
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(giveawayReceiverProvider);

    return SizedBox(
      height: MediaQuery.of(context).size.height * 0.6,
      child: TransferModalShell(
        content: SingleChildScrollView(
          child: Padding(
            padding: const EdgeInsets.only(top: 16),
            child: _buildBody(context, state),
          ),
        ),
        isLoading: state.isLoading,
        errorMessage: state.error == null
            ? null
            : RpcErrorHandler.localize(state.error!, context.l10n),
      ),
    );
  }

  Widget _buildBody(BuildContext context, GiveawayReceiverState state) {
    final transfer = state.transfer;
    if (transfer == null) return const SizedBox();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Status description
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: Text(
            _statusDescription(state.currentPhase, transfer.owner.name),
            style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                  color: AppColors.modalTextSecondary,
                ),
          ),
        ),
        const SizedBox(height: 20),
        // Interest list
        if (state.interestedTransfers.isNotEmpty) ...[
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Text(
              context.l10n.giveawayReceiverInterestedCount(
                state.interestedTransfers.length,
              ),
              style: Theme.of(context).textTheme.titleSmall?.copyWith(
                    fontWeight: FontWeight.w600,
                    color: AppColors.modalTextPrimary,
                  ),
            ),
          ),
          const SizedBox(height: 8),
          for (final t in state.interestedTransfers)
            _buildInterestedUserRow(
                context, t, t.recipient.id == transfer.recipient.id),
        ],
        const SizedBox(height: 100),
      ],
    );
  }

  Widget _buildInterestedUserRow(
    BuildContext context,
    Transfer transfer,
    bool isCurrentUser,
  ) {
    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        color: isCurrentUser
            ? AppColors.transferSage.withValues(alpha: 0.12)
            : AppColors.modalInsetCardBg,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: isCurrentUser
              ? AppColors.transferSage
              : AppColors.modalInsetCardBorder,
        ),
      ),
      child: Row(
        children: [
          UserAvatar(user: transfer.recipient, radius: 18),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              isCurrentUser
                  ? '${transfer.recipient.name} (${context.l10n.commonYou})'
                  : transfer.recipient.name,
              style: TextStyle(
                fontSize: 15,
                fontWeight: isCurrentUser ? FontWeight.w600 : FontWeight.w400,
                color: AppColors.modalTextPrimary,
              ),
            ),
          ),
          Text(
            formatRelativeTimestamp(transfer.latestRequestUnixSec.toInt()),
            style: TextStyle(
              fontSize: 12,
              color: AppColors.modalTextMuted,
            ),
          ),
        ],
      ),
    );
  }

  String _statusDescription(GiveawayReceiverPhase phase, String ownerName) {
    switch (phase) {
      case GiveawayReceiverPhase.waiting:
        return context.l10n.giveawayReceiverWaitingDescription(ownerName);
      case GiveawayReceiverPhase.recipientSelected:
        return context.l10n.giveawayReceiverSelectedDescription(ownerName);
      case GiveawayReceiverPhase.complete:
        return context.l10n.giveawayReceiverCompleteDescription;
    }
  }

  // No bottom bar — withdraw interest is available in the pill menu.
}
