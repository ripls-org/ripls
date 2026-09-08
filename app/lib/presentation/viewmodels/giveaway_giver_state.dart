import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';

part 'giveaway_giver_state.freezed.dart';

/// Phases in the giveaway giver (owner) workflow.
enum GiveawayGiverPhase {
  /// Step 0: Choose from interested users
  selectRecipient,

  /// Step 1: Recipient has been selected, waiting for handoff
  recipientSelected,

  /// Step 2: Done with impact metrics
  complete,
}

/// State for GiveawayGiverViewModel.
///
/// Manages the state of giveaway transfers from the giver's perspective,
/// tracking the current phase and pending requests.
@freezed
sealed class GiveawayGiverState with _$GiveawayGiverState {
  const factory GiveawayGiverState({
    /// Current phase in the giver workflow
    @Default(GiveawayGiverPhase.selectRecipient) GiveawayGiverPhase currentPhase,

    /// The active transfer
    Transfer? activeTransfer,

    /// List of pending requests (users who want the item)
    @Default([]) List<Transfer> pendingRequests,

    /// ID of the selected recipient (for selection phase)
    String? selectedRecipientId,

    /// Whether data is currently being loaded
    @Default(false) bool isLoading,

    /// Error, if any
    UserError? error,
  }) = _GiveawayGiverState;

  const GiveawayGiverState._();

  /// Returns true if there are pending requests to choose from
  bool get hasPendingRequests => pendingRequests.isNotEmpty;

  /// Returns true if a recipient has been selected
  bool get hasSelectedRecipient => selectedRecipientId != null;

  /// Returns true if an active transfer is present
  bool get hasActiveTransfer => activeTransfer != null;

  /// Returns the progress steps for the progress bar
  List<String> get progressSteps => [
        'Select',
        'Complete',
      ];

  /// Returns the current step index for the progress bar
  int get currentStepIndex => currentPhase.index;
}
