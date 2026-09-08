import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart'
    show ExperienceState;
import 'package:ripls/data/gen/ripls/api/gear.pbenum.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show TransferAction, TransferState;
import 'package:ripls/presentation/viewmodels/conversation_state.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('ConversationTransferActions');

/// ConversationTransferActionsMixin provides transfer lifecycle methods
/// for [ConversationNotifier].
///
/// All mutations call [fetchConversationContext] and [refreshMessagesAfterMutation]
/// (provided by the coordinator and messages mixin respectively) to keep the UI
/// in sync after each state transition.
mixin ConversationTransferActionsMixin on Notifier<ConversationState> {
  /// setCachedImpact is implemented by the coordinator. Action mixins call it
  /// to store the impact estimate from a terminal action.
  void setCachedImpact(ImpactEstimate? impact);

  // ── Giveaway transfer getters ────────────────────────────────────────────

  /// _giveawayTransferIdToAct returns the transfer ID for completing or
  /// cancelling a giveaway.
  ///
  /// For the owner: uses the selected recipient's transfer ID.
  /// For the selected recipient: uses their own transfer ID from userTransfer.
  /// Returns empty string when no applicable transfer is found.
  String get _giveawayTransferIdToAct {
    final ctx = state.conversationContext;
    if (ctx == null || !ctx.hasGearTransferContext()) return '';
    final gearCtx = ctx.gearTransferContext;
    if (gearCtx.isOwner && gearCtx.hasSelectedRecipient()) {
      return gearCtx.selectedRecipient.transferId;
    }
    if (gearCtx.hasUserTransfer()) {
      return gearCtx.userTransfer.id;
    }
    return '';
  }

  /// _loanTransferIdToAct returns the transfer ID for loan actions.
  ///
  /// For borrowers: uses their own userTransfer.
  /// For owners: uses the first pending request's transfer.
  String get _loanTransferIdToAct {
    final ctx = state.conversationContext;
    if (ctx == null || !ctx.hasGearTransferContext()) return '';
    final gearCtx = ctx.gearTransferContext;
    if (gearCtx.hasUserTransfer()) return gearCtx.userTransfer.id;
    if (gearCtx.pendingRequests.isNotEmpty) {
      return gearCtx.pendingRequests.first.transferId;
    }
    return '';
  }

  // ── Transfer status getters ──────────────────────────────────────────────

  
  
  /// hasExpressedInterest returns true when the current user has an active
  /// (non-terminal) transfer for this giveaway gear.
  bool get hasExpressedInterest {
    final ctx = state.conversationContext;
    if (ctx == null || !ctx.hasGearTransferContext()) return false;
    return ctx.gearTransferContext.hasUserTransfer();
  }

  /// isSelectedRecipient returns true when the current user is the selected
  /// recipient for this giveaway.
  bool get isSelectedRecipient {
    final ctx = state.conversationContext;
    if (ctx == null || !ctx.hasGearTransferContext()) return false;
    final transferCtx = ctx.gearTransferContext;
    if (!transferCtx.hasSelectedRecipient()) return false;
    return transferCtx.selectedRecipient.borrower.id == state.currentUserId;
  }

  
  
  
  /// isLoanConversation returns true when the current conversation is a loan.
  bool get isLoanConversation {
    final ctx = state.conversationContext;
    if (ctx == null) return false;
    return ctx.topic.hasGearId() &&
        ctx.availability == Availability.AVAILABILITY_FOR_LOAN;
  }

  /// isLoanOwner returns true when the current user owns the gear being lent.
  bool get isLoanOwner {
    final ctx = state.conversationContext;
    if (ctx == null || !ctx.hasGearTransferContext()) return false;
    return ctx.gearTransferContext.isOwner;
  }

  /// loanNeedsPickupDetails returns true when the borrower is in
  /// RECIPIENT_SELECTED state but has not yet set a pickup time.
  bool get loanNeedsPickupDetails {
    if (!isLoanConversation || isLoanOwner) return false;
    final ctx = state.conversationContext;
    if (ctx == null || !ctx.hasGearTransferContext()) return false;
    final gearCtx = ctx.gearTransferContext;
    if (!gearCtx.hasUserTransfer()) return false;
    final transfer = gearCtx.userTransfer;
    return transfer.state ==
            TransferState.TRANSFER_STATE_RECIPIENT_SELECTED &&
        !transfer.hasEstimatedPickupUnixSec();
  }

  /// loanPhaseIndex derives the progress bar phase (0–3) from the user's
  /// transfer state. Returns null for cancelled.
  int? get loanPhaseIndex {
    final ctx = state.conversationContext;
    if (ctx == null || !ctx.hasGearTransferContext()) return null;
    final gearCtx = ctx.gearTransferContext;

    if (!gearCtx.hasUserTransfer()) {
      if (gearCtx.isOwner && gearCtx.pendingRequests.isNotEmpty) return 1;
      return 0;
    }

    switch (gearCtx.userTransfer.state) {
      case TransferState.TRANSFER_STATE_INTEREST_EXPRESSED:
        return 0;
      case TransferState.TRANSFER_STATE_RECIPIENT_SELECTED:
        return 1;
      case TransferState.TRANSFER_STATE_ACTIVE:
        return 2;
      case TransferState.TRANSFER_STATE_COMPLETED:
        return 3;
      case TransferState.TRANSFER_STATE_CANCELLED:
        return null;
      default:
        return null;
    }
  }

  /// isLoanActive returns true when the most active loan is in ACTIVE state.
  bool get isLoanActive => loanPhaseIndex == 2;

  /// isLoanCompleted returns true when the most active loan is completed.
  bool get isLoanCompleted => loanPhaseIndex == 3;

  /// isLoanCancelled returns true when the loan has been cancelled.
  bool get isLoanCancelled => loanPhaseIndex == null && isLoanConversation;

  /// isLoanTerminal returns true when the loan is completed or cancelled.
  bool get isLoanTerminal => isLoanCompleted || isLoanCancelled;

  /// actionButtonLabel returns the short label for the action pill button.
  ///
  /// Returns null when context is not loaded or no action applies.
  String? get actionButtonLabel {
    final ctx = state.conversationContext;
    if (ctx == null) return null;

    if (ctx.hasGearTransferContext()) {
      final actions = ctx.gearTransferContext.availableActions;
      if (actions.contains(TransferAction.TRANSFER_ACTION_EXPRESS_INTEREST)) {
        return 'Request';
      }
      if (actions.contains(TransferAction.TRANSFER_ACTION_SELECT_RECIPIENT)) {
        return 'Review';
      }
      return 'Manage';
    } else if (ctx.topic.hasGearId()) {
      return 'Review';
    } else if (ctx.topic.hasRequestId() || state.cachedRequest != null) {
      return 'Manage';
    } else if (ctx.topic.hasExperienceId() || state.cachedExperience != null) {
      final experience = state.cachedExperience;
      if (experience != null) {
        if (experience.experience.state ==
                ExperienceState.EXPERIENCE_STATE_COMPLETED ||
            experience.experience.state ==
                ExperienceState.EXPERIENCE_STATE_CANCELLED) {
          return null;
        }
        final isOrganizer =
            experience.experience.owner.id == state.currentUserId;
        return isOrganizer ? 'Manage' : 'RSVP';
      }
      return 'Manage';
    }

    return null;
  }

  
  // ── Giveaway actions ─────────────────────────────────────────────────────

  /// cancelTransfer cancels a giveaway transfer from the chat workflow.
  Future<void> cancelTransfer() async {
    final transferId = _giveawayTransferIdToAct;
    if (transferId.isEmpty) {
      throw Exception('Transfer ID not found');
    }

    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.cancelTransfer(transferId: transferId);
      if (!ref.mounted) return;
      await fetchConversationContext();
      if (!ref.mounted) return;
      await refreshMessagesAfterMutation();
    } catch (e) {
      _log.severe('Failed to cancel transfer: $e');
      rethrow;
    }
  }

  /// completeTransfer completes a giveaway from the chat workflow.
  Future<void> completeTransfer() async {
    final transferId = _giveawayTransferIdToAct;
    if (transferId.isEmpty) {
      throw Exception('Transfer ID not found');
    }

    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.completeTransfer(transferId: transferId);
      if (!ref.mounted) return;

      final gearId = state.conversationContext?.topic.gearId ?? '';
      if (gearId.isNotEmpty) {
        await ref.read(gearRepositoryProvider).invalidate(gearId);
      }
      if (!ref.mounted) return;
      await fetchConversationContext();
      if (!ref.mounted) return;
      await refreshMessagesAfterMutation();
    } catch (e) {
      _log.severe('Failed to complete transfer: $e');
      rethrow;
    }
  }

  /// expressInterest creates a transfer request for giveaway gear.
  Future<void> expressInterest(String gearId) async {
    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.expressInterest(gearId: gearId);
      await fetchConversationContext();
      await refreshMessagesAfterMutation();
    } catch (e) {
      _log.severe('Failed to express interest: $e');
      rethrow;
    }
  }

  /// withdrawInterest withdraws interest in borrowing gear.
  Future<void> withdrawInterest(String transferId) async {
    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.withdrawInterest(transferId: transferId);
      await fetchConversationContext();
      await refreshMessagesAfterMutation();
    } catch (e) {
      _log.severe('Failed to withdraw interest: $e');
      rethrow;
    }
  }

  
  /// declineTransferRequest declines a specific transfer request.
  Future<void> declineTransferRequest(String transferId) async {
    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.cancelTransfer(transferId: transferId);
      await fetchConversationContext();
      await refreshMessagesAfterMutation();
    } catch (e) {
      _log.severe('Failed to decline transfer request: $e');
      rethrow;
    }
  }

  // ── Loan actions ─────────────────────────────────────────────────────────

  /// startLoan marks the loan handoff (RECIPIENT_SELECTED → ACTIVE).
  Future<void> startLoan() async {
    final transferId = _loanTransferIdToAct;
    if (transferId.isEmpty) {
      throw Exception('Transfer ID not found');
    }

    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.startLoan(transferId: transferId);
      if (!ref.mounted) return;

      final gearId = state.conversationContext?.topic.gearId ?? '';
      if (gearId.isNotEmpty) {
        await ref.read(gearRepositoryProvider).invalidate(gearId);
      }
      if (!ref.mounted) return;
      await fetchConversationContext();
      if (!ref.mounted) return;
      await refreshMessagesAfterMutation();
    } catch (e) {
      _log.severe('Failed to start loan: $e');
      rethrow;
    }
  }

  
  
  
  
  
  // ── Cross-mixin hooks ─────────────────────────────────────────────────────

  Future<void> fetchConversationContext();
  Future<void> refreshMessagesAfterMutation();
}
