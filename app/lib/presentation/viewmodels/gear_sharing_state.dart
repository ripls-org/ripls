import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show Transfer, TransferType, TransferState;
import 'package:ripls/data/gen/ripls/api/transfer_service.pb.dart'
    show CompleteTransferResponse;

part 'gear_sharing_state.freezed.dart';

/// GearSharingState holds the full UI state for gear sharing screens.
///
/// Owned by [GearSharingNotifier]. All action mixins read and write this
/// state via [Notifier.state] and [Notifier.state.copyWith].
@freezed
sealed class GearSharingState with _$GearSharingState {
  const factory GearSharingState({
    GetGearResponse? gear,
    @Default(false) bool isOwner,
    String? communityId,
    // Current user ID for role determination
    String? currentUserId,
    // Submission state
    @Default(false) bool isSubmitting,
    @Default(false) bool isCanceling,
    @Default(false) bool isWithdrawing,
    UserError? error,
    // Transfer tracking
    String? submittedTransferId,
    // Track transfer state for withdrawal eligibility
    TransferState? submittedTransferState,
    // Transfer requests lists
    @Default([]) List<Transfer> receivedRequests,
    @Default([]) List<Transfer> extendedRequests,
    // All transfers for this gear (loaded for all users)
    @Default([]) List<Transfer> allGearTransfers,
    @Default(false) bool isLoadingRequests,
    UserError? requestsError,
    // Community sharing state (for owners)
    @Default([]) List<CommunityItem> userCommunities,
    @Default({}) Set<String> sharedCommunityIds,
    @Default({}) Map<String, Availability> communityAvailability,
    @Default({}) Set<String> loadingCommunityIds,
    @Default(false) bool isLoadingCommunities,
    UserError? communitiesError,
    // Transfer conversations (for owner view)
    @Default({}) Map<String, ConversationItem?> conversations,
    @Default(false) bool isLoadingConversations,
    // Track recently completed transfer to maintain view state during completion
    String? recentlyCompletedTransferId,
    // Completion savings for transfers (keyed by transfer ID)
    @Default({}) Map<String, CompleteTransferResponse> completionSavings,
  }) = _GearSharingState;

  const GearSharingState._();

  /// Returns whether there's an active transfer request from the current user.
  bool get hasActiveTransferRequest => submittedTransferId != null;

  /// Returns whether the state has an error.
  bool get hasError => error != null;

  /// Returns whether the user can withdraw their interest.
  /// Withdrawal is only possible before recipient selection (INTEREST_EXPRESSED state).
  bool get canWithdraw =>
      !isOwner &&
      submittedTransferId != null &&
      submittedTransferState == TransferState.TRANSFER_STATE_INTEREST_EXPRESSED;

  /// Returns all non-cancelled transfers for display.
  /// For owners: uses receivedRequests.
  /// For non-owners: uses allGearTransfers.
  List<Transfer> get displayTransfers {
    final transfers = isOwner ? receivedRequests : allGearTransfers;
    return transfers
        .where((t) => t.state != TransferState.TRANSFER_STATE_CANCELLED)
        .toList();
  }

  /// Returns the active transfer (one that is in progress but not completed).
  /// Excludes INTEREST_EXPRESSED (pending) and COMPLETED transfers.
  /// This represents transfers that have a selected recipient and are actively being managed.
  Transfer? get activeTransfer {
    for (final transfer in displayTransfers) {
      if (transfer.state != TransferState.TRANSFER_STATE_INTEREST_EXPRESSED &&
          transfer.state != TransferState.TRANSFER_STATE_COMPLETED) {
        return transfer;
      }
    }
    return null;
  }

  /// Returns pending requests (INTEREST_EXPRESSED state only).
  /// These are the requests that haven't been approved yet.
  List<Transfer> get pendingRequests {
    return displayTransfers
        .where(
          (t) => t.state == TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
        )
        .toList();
  }

  /// Returns whether there's an active transfer in progress (post-selection).
  bool get hasActiveTransferInProgress => activeTransfer != null;

  
  /// Returns the current user's transfer (if they have one).
  Transfer? get currentUserTransfer {
    if (currentUserId == null) return null;
    for (final transfer in displayTransfers) {
      if (transfer.hasRecipient() && transfer.recipient.id == currentUserId) {
        return transfer;
      }
    }
    return null;
  }

  /// Returns whether the current user is the selected recipient for the active transfer.
  bool get isSelectedRecipient {
    final active = activeTransfer;
    if (active == null || currentUserId == null) return false;
    return active.hasRecipient() && active.recipient.id == currentUserId;
  }

  /// Returns whether the current user has expressed interest (but may not be selected).
  bool get hasExpressedInterest => currentUserTransfer != null;

  /// Returns ALL transfers sorted by priority for display:
  /// For BOTH loans and giveaways:
  /// 1. Received (ACTIVE) - currently out
  /// 2. Approved (RECIPIENT_SELECTED) - ready to pick up
  /// 3. Interest Expressed (INTEREST_EXPRESSED) - pending requests
  /// 4. Completed (COMPLETED) - returned/transferred
  /// Within each group, sorted oldest first (FIFO) EXCEPT completed (most recent first)
  List<Transfer> get orderedTransfers {
    final transfers = displayTransfers.toList();

    if (transfers.isEmpty) return transfers;

    // Determine if this is a loan or giveaway (check first transfer type)
    final isLoan =
        transfers.first.transferType == TransferType.TRANSFER_TYPE_LOAN;

    // Define sort priority for each state
    // Both loans and giveaways: ACTIVE → RECIPIENT_SELECTED → INTEREST_EXPRESSED → COMPLETED
    int statePriority(TransferState state) {
      switch (state) {
        case TransferState.TRANSFER_STATE_ACTIVE:
          return 1; // Received - highest priority
        case TransferState.TRANSFER_STATE_RECIPIENT_SELECTED:
          return 2; // Approved
        case TransferState.TRANSFER_STATE_INTEREST_EXPRESSED:
          return 3; // Interest Expressed (for both loans and giveaways)
        case TransferState.TRANSFER_STATE_COMPLETED:
          return 4; // Completed
        default:
          return 999; // Should not appear
      }
    }

    // Get appropriate timestamp for secondary sort
    int getTimestamp(Transfer t) {
      switch (t.state) {
        case TransferState.TRANSFER_STATE_ACTIVE:
          return t.hasActualPickupUnixSec()
              ? t.actualPickupUnixSec.toInt()
              : t.latestRequestUnixSec.toInt();
        case TransferState.TRANSFER_STATE_COMPLETED:
          // Loans: use actualReturnUnixSec; Giveaways: use actualPickupUnixSec
          if (isLoan && t.hasActualReturnUnixSec()) {
            return t.actualReturnUnixSec.toInt();
          }
          return t.hasActualPickupUnixSec()
              ? t.actualPickupUnixSec.toInt()
              : t.latestRequestUnixSec.toInt();
        case TransferState.TRANSFER_STATE_RECIPIENT_SELECTED:
        case TransferState.TRANSFER_STATE_INTEREST_EXPRESSED:
        default:
          return t.latestRequestUnixSec.toInt();
      }
    }

    transfers.sort((a, b) {
      // First by state priority
      final priorityCompare = statePriority(
        a.state,
      ).compareTo(statePriority(b.state));
      if (priorityCompare != 0) return priorityCompare;

      // Then by timestamp
      final timestampA = getTimestamp(a);
      final timestampB = getTimestamp(b);

      // For COMPLETED state: most recent first (reverse order)
      // For all other states: oldest first (normal order)
      if (a.state == TransferState.TRANSFER_STATE_COMPLETED) {
        return timestampB.compareTo(timestampA); // Most recent first
      } else {
        return timestampA.compareTo(timestampB); // Oldest first
      }
    });

    return transfers;
  }
}
