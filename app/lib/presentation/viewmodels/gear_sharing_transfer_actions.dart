import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show Transfer;
import 'package:ripls/data/gen/ripls/api/transfer_service.pb.dart'
    show CompleteTransferResponse;
import 'package:ripls/presentation/viewmodels/gear_sharing_state.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('GearSharingTransferActions');

/// GearSharingTransferActionsMixin provides transfer lifecycle methods for
/// [GearSharingNotifier].
///
/// Covers expressing/withdrawing interest, approving/declining requests,
/// updating transfer details, pickup/return transitions, and giveaway
/// completion.
mixin GearSharingTransferActionsMixin on Notifier<GearSharingState> {
  /// loadReceivedRequests is provided by GearSharingLoadMixin and used here
  /// to refresh the transfer list after mutations.
  Future<void> loadReceivedRequests();

  /// loadExtendedRequests is provided by GearSharingLoadMixin and used here
  /// to refresh the borrower's request list after mutations.
  Future<void> loadExtendedRequests();

  /// expressInterest submits a transfer request for the current user and sends
  /// the initial message to the conversation.
  ///
  /// Returns the full Transfer object on success. On failure it sets the
  /// classified [UserError] on state — which is what the UI renders — and
  /// rethrows so the caller can abandon its own flow. The initial message is
  /// sent to the conversation after expressing interest.
  Future<Transfer> expressInterest(String initialMessage) async {
    final gear = state.gear;
    if (gear == null) throw StateError('Gear not initialized');

    _log.info('📝 Express interest started');
    _log.info('   Gear ID: ${gear.id}');
    _log.info('   Initial message: ${initialMessage.trim()}');

    state = state.copyWith(isSubmitting: true, error: null);

    try {
      _log.info('📞 Calling transferRepository.expressInterest...');
      // Use repository method which automatically invalidates caches
      final transferRepository = ref.read(transferRepositoryProvider);
      final transfer = await transferRepository.expressInterest(
        gearId: gear.id,
      );

      _log.info(
        '✅ Interest expressed successfully! Transfer ID: ${transfer.id}, Conversation ID: ${transfer.conversationId}',
      );

      // Send the initial message to the conversation
      if (initialMessage.trim().isNotEmpty &&
          transfer.conversationId.isNotEmpty) {
        _log.info(
          '📤 Sending initial message to conversation: ${transfer.conversationId}',
        );
        try {
          final chatRepository = ref.read(chatRepositoryProvider);
          await chatRepository.sendMessage(
            conversationId: transfer.conversationId,
            text: initialMessage.trim(),
          );
          _log.info('✅ Initial message sent successfully');
        } catch (e) {
          _log.warning('⚠️ Failed to send initial message: $e');
          // Don't fail the entire operation if message send fails
          // The user can send it manually in the conversation
        }
      }

      // Store the submitted transfer request info directly from enriched response
      // No additional RPC needed!
      state = state.copyWith(
        submittedTransferId: transfer.id.isNotEmpty ? transfer.id : null,
        submittedTransferState: transfer.state,
        isSubmitting: false,
      );

      return transfer; // Return full transfer object for creating InboxItem
    } on ServiceException catch (e) {
      _log.severe('❌ ServiceException caught: ${e.message}', e);
      state = state.copyWith(
        error: RpcErrorHandler.classify(e),
        isSubmitting: false,
      );
      rethrow;
    } catch (e, stackTrace) {
      _log.severe('❌ Unexpected exception caught: $e', e, stackTrace);
      state = state.copyWith(
        error: RpcErrorHandler.classify(e, fallback: 'Could not submit request'),
        isSubmitting: false,
      );
      rethrow;
    }
  }

  /// withdrawInterest removes the user's interest from a transfer.
  ///
  /// For loans: archives the user's individual transfer request.
  /// For giveaways: removes user from group conversation.
  /// Returns null on success, error message on failure.
  Future<String?> withdrawInterest() async {
    if (state.submittedTransferId == null) {
      return 'No active transfer request to withdraw';
    }

    if (!state.canWithdraw) {
      return 'Cannot withdraw at this stage';
    }

    _log.info(
      '🚪 Withdrawing interest from transfer: ${state.submittedTransferId}',
    );

    state = state.copyWith(isWithdrawing: true, error: null);

    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.withdrawInterest(
        transferId: state.submittedTransferId!,
      );

      _log.info('✅ Interest withdrawn successfully');

      // Clear the submitted transfer request info
      state = state.copyWith(
        submittedTransferId: null,
        submittedTransferState: null,
        isWithdrawing: false,
      );

      // Reload extended requests to update the list
      await loadExtendedRequests();

      return null; // Success
    } on ServiceException catch (e) {
      _log.severe('❌ ServiceException during withdrawal: ${e.message}', e);
      state = state.copyWith(
        error: RpcErrorHandler.classify(e),
        isWithdrawing: false,
      );
      return e.message;
    } catch (e, stackTrace) {
      _log.severe(
        '❌ Unexpected exception during withdrawal: $e',
        e,
        stackTrace,
      );
      state = state.copyWith(
        error: RpcErrorHandler.classify(e, fallback: 'Could not withdraw interest'),
        isWithdrawing: false,
      );
      return 'Failed to withdraw interest: $e';
    }
  }

  /// cancelTransferRequest cancels the submitted transfer request.
  /// Returns null on success, error message on failure.
  Future<String?> cancelTransferRequest() async {
    if (state.submittedTransferId == null) {
      return 'No active transfer request to cancel';
    }

    _log.info('🚫 Canceling transfer request: ${state.submittedTransferId}');

    state = state.copyWith(isCanceling: true, error: null);

    try {
      // Use repository method which automatically invalidates caches
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.cancelTransfer(
        transferId: state.submittedTransferId!,
      );

      _log.info('✅ Transfer request canceled successfully');

      // Clear the submitted transfer request info
      state = state.copyWith(submittedTransferId: null, isCanceling: false);

      // Reload loan requests based on user role
      if (state.isOwner) {
        await loadReceivedRequests();
      } else {
        await loadExtendedRequests();
      }

      return null; // Success
    } on ServiceException catch (e) {
      _log.severe('❌ ServiceException during cancel: ${e.message}', e);
      state = state.copyWith(
        error: RpcErrorHandler.classify(e),
        isCanceling: false,
      );
      return e.message;
    } catch (e, stackTrace) {
      _log.severe('❌ Unexpected exception during cancel: $e', e, stackTrace);
      state = state.copyWith(
        error: RpcErrorHandler.classify(e, fallback: 'Could not cancel request'),
        isCanceling: false,
      );
      return 'Failed to cancel request: $e';
    }
  }

  /// selectRecipient approves a loan request by selecting the recipient.
  /// Returns null on success, error message on failure.
  Future<String?> selectRecipient(
    String transferId,
    String recipientId,
  ) async {
    if (!state.isOwner) {
      return 'Only the owner can select recipients';
    }

    _log.info('✅ Selecting recipient for transfer: $transferId');

    try {
      // Use repository method which automatically invalidates caches
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.selectRecipient(
        transferId: transferId,
        recipientId: recipientId,
      );

      _log.info('✅ Loan request approved successfully');

      // Reload received requests to update the list
      await loadReceivedRequests();

      return null; // Success
    } on ServiceException catch (e) {
      _log.severe('❌ ServiceException during approve: ${e.message}', e);
      return e.message;
    } catch (e, stackTrace) {
      _log.severe('❌ Unexpected exception during approve: $e', e, stackTrace);
      return 'Failed to approve request: $e';
    }
  }

  /// declineTransferRequest declines a loan request by cancelling it.
  /// Returns null on success, error message on failure.
  Future<String?> declineTransferRequest(String transferId) async {
    if (!state.isOwner) {
      return 'Only the owner can decline transfer requests';
    }

    _log.info('🚫 Declining transfer request: $transferId');

    try {
      // Use repository method which automatically invalidates caches
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.cancelTransfer(transferId: transferId);

      _log.info('✅ Transfer request declined successfully');

      // Reload received requests to update the list
      await loadReceivedRequests();

      return null; // Success
    } on ServiceException catch (e) {
      _log.severe('❌ ServiceException during decline: ${e.message}', e);
      return e.message;
    } catch (e, stackTrace) {
      _log.severe('❌ Unexpected exception during decline: $e', e, stackTrace);
      return 'Failed to decline request: $e';
    }
  }

  
  /// markPickedUp transitions a transfer from RECIPIENT_SELECTED to ACTIVE.
  ///
  /// Records the actual pickup time on the server.
  /// Returns null on success, error message on failure.
  Future<String?> markPickedUp(String transferId) async {
    _log.info('📦 Marking item as picked up: $transferId');

    try {
      final transferRepository = ref.read(transferRepositoryProvider);

      // Mark the transfer as picked up
      await transferRepository.startLoan(transferId: transferId);

      // Fetch the updated transfer
      final updatedTransfer = await transferRepository.getTransfer(transferId);

      if (updatedTransfer == null) {
        throw Exception('Transfer not found after marking as picked up');
      }

      _log.info('✅ Item marked as picked up successfully');

      // Update the transfer in-place without reloading entire list to avoid flickering
      _updateTransferInPlace(updatedTransfer);

      return null; // Success
    } on ServiceException catch (e) {
      _log.severe('❌ ServiceException marking picked up: ${e.message}', e);
      return e.message;
    } catch (e, stackTrace) {
      _log.severe(
        '❌ Unexpected exception marking picked up: $e',
        e,
        stackTrace,
      );
      return 'Failed to mark as picked up: $e';
    }
  }

  
  /// completeGiveaway transitions a giveaway from RECIPIENT_SELECTED to
  /// COMPLETED, recording the actual pickup time.
  ///
  /// Returns the completion response with savings, throws on failure.
  Future<CompleteTransferResponse> completeGiveaway(String transferId) async {
    _log.info('🎁 Completing giveaway: $transferId');

    try {
      final transferRepository = ref.read(transferRepositoryProvider);

      // Mark the giveaway as completed and get savings
      final response = await transferRepository.completeTransfer(
        transferId: transferId,
      );

      // Fetch the updated transfer
      final updatedTransfer = await transferRepository.getTransfer(transferId);

      if (updatedTransfer == null) {
        throw Exception('Transfer not found after completing giveaway');
      }

      _log.info('✅ Giveaway completed successfully');

      // Update the transfer in-place without reloading entire list to avoid flickering
      _updateTransferInPlace(updatedTransfer);

      // Mark as recently completed and store savings
      final updatedSavings = Map<String, CompleteTransferResponse>.from(
        state.completionSavings,
      );
      updatedSavings[transferId] = response;
      state = state.copyWith(
        recentlyCompletedTransferId: transferId,
        completionSavings: updatedSavings,
      );

      return response; // Return savings
    } on ServiceException catch (e) {
      _log.severe('❌ ServiceException completing giveaway: ${e.message}', e);
      rethrow;
    } catch (e, stackTrace) {
      _log.severe(
        '❌ Unexpected exception completing giveaway: $e',
        e,
        stackTrace,
      );
      rethrow;
    }
  }

  
  /// _updateTransferInPlace updates a single transfer without reloading the
  /// entire list.
  ///
  /// Prevents flickering by updating only the specific transfer that changed,
  /// preserving the current UI state including which cards are
  /// expanded/collapsed. Updates both receivedRequests (for owners) and
  /// allGearTransfers (for non-owners) to ensure consistency.
  void _updateTransferInPlace(Transfer updatedTransfer) {
    state = state.copyWith(
      // Update receivedRequests (used by owners)
      receivedRequests: state.receivedRequests.map((t) {
        return t.id == updatedTransfer.id ? updatedTransfer : t;
      }).toList(),
      // Update allGearTransfers (used by non-owners)
      allGearTransfers: state.allGearTransfers.map((t) {
        return t.id == updatedTransfer.id ? updatedTransfer : t;
      }).toList(),
    );
  }
}
