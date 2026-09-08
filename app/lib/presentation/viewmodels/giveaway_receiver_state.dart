import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';

part 'giveaway_receiver_state.freezed.dart';

/// Phases in the giveaway receiver workflow.
enum GiveawayReceiverPhase {
  /// Step 0: Owner reviewing requests
  waiting,

  /// Step 1: Selected as recipient, waiting for physical handoff
  recipientSelected,

  /// Step 2: Item is yours
  complete,
}

/// State for GiveawayReceiverViewModel.
///
/// Manages the state of giveaway transfers from the receiver's perspective,
/// tracking the current phase.
@freezed
sealed class GiveawayReceiverState with _$GiveawayReceiverState {
  const factory GiveawayReceiverState({
    /// Current phase in the receiver workflow
    @Default(GiveawayReceiverPhase.waiting) GiveawayReceiverPhase currentPhase,

    /// The active transfer
    Transfer? transfer,

    /// All interested users' transfers for this gear (read-only list)
    @Default([]) List<Transfer> interestedTransfers,

    /// Whether data is currently being loaded
    @Default(false) bool isLoading,

    /// Error, if any
    UserError? error,
  }) = _GiveawayReceiverState;

  const GiveawayReceiverState._();

  /// Returns true if an active transfer is present
  bool get hasTransfer => transfer != null;
}
