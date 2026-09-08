import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/giveaway_giver_state.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('GiveawayGiverViewModel');

/// Provider for the GiveawayGiverNotifier.
///
/// This is an autoDispose provider that manages giveaway transfers from the giver's perspective.
final giveawayGiverProvider =
    NotifierProvider.autoDispose<GiveawayGiverNotifier, GiveawayGiverState>(
  GiveawayGiverNotifier.new,
);

/// ViewModel for managing giveaway transfers from the giver's perspective.
///
/// Follows the simplified giveaway phase workflow:
/// 1. Select Recipient: Choose from interested users
/// 2. Recipient Selected: Waiting for physical handoff
/// 3. Complete: Done with impact metrics
class GiveawayGiverNotifier extends Notifier<GiveawayGiverState>
    with SafeNotifierMixin<GiveawayGiverState> {
  late final TransferRepository _repository;

  @override
  GiveawayGiverState build() {
    _repository = ref.watch(transferRepositoryProvider);
    return const GiveawayGiverState();
  }

  /// Initializes the ViewModel by loading transfers for the specified gear.
  ///
  /// Loads all pending requests and determines the current phase based on
  /// the active transfer state.
  Future<void> initialize({required String gearId, String? transferId}) async {
    _log.info('Initializing giveaway giver view for gear: $gearId');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      // Get all transfers for this gear
      final transfers = await _repository.listMyTransfers(
        transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      );

      if (!ref.mounted) return;

      // Filter to transfers for this gear
      final gearTransfers = transfers
          .where((t) => t.gearId == gearId)
          .toList();

      // Separate active transfer from pending requests
      Transfer? activeTransfer;
      final List<Transfer> pendingRequests = [];

      // If a specific transferId was provided, use that as the active transfer first
      if (transferId != null) {
        final specificTransfer = await _repository.getTransfer(transferId);
        if (!ref.mounted) return;
        if (specificTransfer != null) {
          activeTransfer = specificTransfer;
          _log.info('Using specific transfer: ${specificTransfer.id}, state: ${specificTransfer.state}');
        }
      }

      // Build pending requests list and find active transfer if not already set
      for (final transfer in gearTransfers) {
        if (transfer.state == TransferState.TRANSFER_STATE_INTEREST_EXPRESSED) {
          // Only add to pending requests if no active transfer exists yet
          // Once a recipient is selected, INTEREST_EXPRESSED transfers are no longer relevant
          if (activeTransfer == null) {
            pendingRequests.add(transfer);
          }
        } else if (activeTransfer == null &&
                   transfer.state != TransferState.TRANSFER_STATE_CANCELLED) {
          // Only set activeTransfer if not already set by transferId
          // Include COMPLETED transfers so we can show the completion screen
          activeTransfer = transfer;
          _log.info('Using first active transfer: ${transfer.id}, state: ${transfer.state}');
        }
      }

      // Determine current phase
      final currentPhase = _determinePhase(activeTransfer);

      safeUpdateState((s) => s.copyWith(
            currentPhase: currentPhase,
            activeTransfer: activeTransfer,
            pendingRequests: pendingRequests,
            isLoading: false,
          ));

      _log.info(
        'Initialized: phase=$currentPhase, active=${activeTransfer != null}, pending=${pendingRequests.length}',
      );
    } catch (e, stackTrace) {
      _log.severe('Failed to initialize giveaway giver view', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState((s) => s.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e),
          ));
    }
  }

  /// Selects a recipient from the pending requests.
  ///
  /// This transitions the transfer from INTEREST_EXPRESSED to RECIPIENT_SELECTED.
  /// For giveaways, non-selected users are automatically notified.
  /// Returns community_event_id for undo wiring.
  Future<String?> selectRecipient(String transferId, String recipientId) async {
    _log.info('Selecting recipient: $recipientId for transfer: $transferId');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    String? communityEventId;
    try {
      final result = await _repository.selectRecipient(
        transferId: transferId,
        recipientId: recipientId,
      );
      communityEventId = result.communityEventId;

      if (!ref.mounted) return communityEventId;

      // Reload transfers to get updated state
      final updatedTransfer = await _repository.getTransfer(transferId);
      if (!ref.mounted) return communityEventId;

      final currentPhase = _determinePhase(updatedTransfer);

      safeUpdateState((s) => s.copyWith(
            currentPhase: currentPhase,
            activeTransfer: updatedTransfer,
            pendingRequests: [], // Clear pending requests after selection
            selectedRecipientId: null, // Clear selection
            isLoading: false,
          ));

      // Notify gear view to refresh transfer context so the pill updates.
      ref.read(transferCacheInvalidationProvider.notifier).notify();

      _log.info('Recipient selected successfully');
      return communityEventId;
    } catch (e, stackTrace) {
      _log.severe('Failed to select recipient', e, stackTrace);
      if (!ref.mounted) return null;

      safeUpdateState((s) => s.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e, fallback: 'Could not select recipient'),
          ));
      return null;
    }
  }

  /// Sets the selected recipient ID (for UI selection state).
  void setSelectedRecipient(String recipientId) {
    safeUpdateState((s) => s.copyWith(selectedRecipientId: recipientId));
  }

  /// Completes the giveaway (item picked up).
  ///
  /// This transitions the transfer from RECIPIENT_SELECTED to COMPLETED.
  /// Returns the community_event_id for undo wiring.
  Future<String?> completeGiveaway(String transferId) async {
    _log.info('Completing giveaway: $transferId');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    String? communityEventId;
    try {
      final response = await _repository.completeTransfer(
        transferId: transferId,
      );
      communityEventId = response.communityEventId;

      if (!ref.mounted) return communityEventId;

      // Reload transfer to get updated state
      final updatedTransfer = await _repository.getTransfer(transferId);
      if (!ref.mounted) return communityEventId;

      final currentPhase = _determinePhase(updatedTransfer);

      safeUpdateState((s) => s.copyWith(
            currentPhase: currentPhase,
            activeTransfer: updatedTransfer,
            isLoading: false,
          ));

      _log.info('Giveaway completed successfully');
      return communityEventId;
    } catch (e, stackTrace) {
      _log.severe('Failed to complete giveaway', e, stackTrace);
      if (!ref.mounted) return null;

      safeUpdateState((s) => s.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e, fallback: 'Could not complete giveaway'),
          ));
      return null;
    }
  }

  /// Cancels the transfer. Returns community_event_id for undo wiring.
  Future<String?> cancelTransfer(String transferId) async {
    _log.info('Canceling transfer: $transferId');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    String? communityEventId;
    try {
      communityEventId =
          await _repository.cancelTransfer(transferId: transferId);

      if (!ref.mounted) return communityEventId;

      _log.info('Transfer canceled successfully');

      // Clear the active transfer
      safeUpdateState((s) => s.copyWith(
            activeTransfer: null,
            isLoading: false,
          ));
      return communityEventId;
    } catch (e, stackTrace) {
      _log.severe('Failed to cancel transfer', e, stackTrace);
      if (!ref.mounted) return null;

      safeUpdateState((s) => s.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e, fallback: 'Could not cancel transfer'),
          ));
      return null;
    }
  }

  /// Determines the current phase based on the transfer state.
  GiveawayGiverPhase _determinePhase(Transfer? transfer) {
    if (transfer == null) {
      return GiveawayGiverPhase.selectRecipient;
    }

    switch (transfer.state) {
      case TransferState.TRANSFER_STATE_INTEREST_EXPRESSED:
        return GiveawayGiverPhase.selectRecipient;
      case TransferState.TRANSFER_STATE_RECIPIENT_SELECTED:
        return GiveawayGiverPhase.recipientSelected;
      case TransferState.TRANSFER_STATE_COMPLETED:
        return GiveawayGiverPhase.complete;
      default:
        return GiveawayGiverPhase.selectRecipient;
    }
  }
}
