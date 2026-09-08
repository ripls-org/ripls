import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/giveaway_receiver_state.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('GiveawayReceiverViewModel');

/// Provider for the GiveawayReceiverNotifier.
///
/// This is an autoDispose provider that manages giveaway transfers from the receiver's perspective.
final giveawayReceiverProvider =
    NotifierProvider.autoDispose<GiveawayReceiverNotifier, GiveawayReceiverState>(
  GiveawayReceiverNotifier.new,
);

/// ViewModel for managing giveaway transfers from the receiver's perspective.
///
/// Follows the simplified giveaway phase workflow:
/// 1. Waiting: Owner reviewing requests
/// 2. Recipient Selected: Waiting for physical handoff
/// 3. Complete: Item is yours
class GiveawayReceiverNotifier extends Notifier<GiveawayReceiverState>
    with SafeNotifierMixin<GiveawayReceiverState> {
  late final TransferRepository _repository;

  @override
  GiveawayReceiverState build() {
    _repository = ref.watch(transferRepositoryProvider);
    return const GiveawayReceiverState();
  }

  /// Initializes the ViewModel by loading the transfer and interest list.
  Future<void> initialize({
    required String transferId,
    String? gearId,
  }) async {
    _log.info('Initializing giveaway receiver view for transfer: $transferId');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      final transfer = await _repository.getTransfer(transferId);

      if (!ref.mounted) return;

      if (transfer == null) {
        safeUpdateState((s) => s.copyWith(
              isLoading: false,
              error: const UserError.generic(fallback: 'Transfer not found'),
            ));
        return;
      }

      // Load all interested transfers for this gear.
      List<Transfer> interestedTransfers = [];
      final effectiveGearId = gearId ?? transfer.gearId;
      if (effectiveGearId.isNotEmpty) {
        interestedTransfers =
            await _repository.getGearTransfers(gearId: effectiveGearId);
        if (!ref.mounted) return;
        // Keep only non-cancelled, non-completed transfers.
        interestedTransfers = interestedTransfers
            .where((t) =>
                t.state != TransferState.TRANSFER_STATE_CANCELLED &&
                t.state != TransferState.TRANSFER_STATE_COMPLETED)
            .toList();
      }

      final currentPhase = _determinePhase(transfer);

      safeUpdateState((s) => s.copyWith(
            currentPhase: currentPhase,
            transfer: transfer,
            interestedTransfers: interestedTransfers,
            isLoading: false,
          ));

      _log.info(
        'Initialized: phase=$currentPhase, interested=${interestedTransfers.length}',
      );
    } catch (e, stackTrace) {
      _log.severe('Failed to initialize giveaway receiver view', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState((s) => s.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e),
          ));
    }
  }

  /// Withdraws interest in the transfer (cancels the request).
  Future<void> withdrawInterest() async {
    final transfer = state.transfer;
    if (transfer == null) return;

    _log.info('Withdrawing interest for transfer: ${transfer.id}');
    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      await _repository.withdrawInterest(transferId: transfer.id);

      if (!ref.mounted) return;

      _log.info('Interest withdrawn successfully');

      safeUpdateState((s) => s.copyWith(
            transfer: null,
            isLoading: false,
          ));
    } catch (e, stackTrace) {
      _log.severe('Failed to withdraw interest', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState((s) => s.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e, fallback: 'Could not withdraw interest'),
          ));
    }
  }

  /// Marks the item as picked up (completes the giveaway transfer).
  /// Returns the community_event_id so the caller can wire undo.
  Future<String?> markPickedUp() async {
    final transfer = state.transfer;
    if (transfer == null) return null;

    _log.info('Marking item as picked up for transfer: ${transfer.id}');
    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    String? communityEventId;
    try {
      final response =
          await _repository.completeTransfer(transferId: transfer.id);
      communityEventId = response.communityEventId;

      if (!ref.mounted) return communityEventId;

      final updatedTransfer = await _repository.getTransfer(transfer.id);
      if (!ref.mounted) return communityEventId;

      final currentPhase = _determinePhase(updatedTransfer);

      safeUpdateState((s) => s.copyWith(
            currentPhase: currentPhase,
            transfer: updatedTransfer,
            isLoading: false,
          ));

      _log.info('Item marked as picked up successfully');
      return communityEventId;
    } catch (e, stackTrace) {
      _log.severe('Failed to mark item as picked up', e, stackTrace);
      if (!ref.mounted) return null;

      safeUpdateState((s) => s.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e, fallback: 'Could not mark item as picked up'),
          ));
      return null;
    }
  }

  /// Determines the current phase based on the transfer state.
  GiveawayReceiverPhase _determinePhase(Transfer? transfer) {
    if (transfer == null) {
      return GiveawayReceiverPhase.waiting;
    }

    switch (transfer.state) {
      case TransferState.TRANSFER_STATE_INTEREST_EXPRESSED:
        return GiveawayReceiverPhase.waiting;
      case TransferState.TRANSFER_STATE_RECIPIENT_SELECTED:
        return GiveawayReceiverPhase.recipientSelected;
      case TransferState.TRANSFER_STATE_COMPLETED:
        return GiveawayReceiverPhase.complete;
      default:
        return GiveawayReceiverPhase.waiting;
    }
  }
}
