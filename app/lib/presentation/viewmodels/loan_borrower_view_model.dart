import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/loan_borrower_state.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('LoanBorrowerViewModel');

/// Provider for the LoanBorrowerNotifier.
///
/// This is an autoDispose provider that manages loan transfers from the borrower's perspective.
final loanBorrowerProvider =
    NotifierProvider.autoDispose<LoanBorrowerNotifier, LoanBorrowerState>(
  LoanBorrowerNotifier.new,
);

/// ViewModel for managing loan transfers from the borrower's perspective.
///
/// Follows the phase-based workflow:
/// 1. Arrange Pickup: Set date, time, duration
/// 2. Confirmed: Summary, ready to pick up
/// 3. In Use: Countdown to return date
/// 4. Confirm Return: Confirm return to owner
/// 5. Returned: Complete with impact metrics
///
/// Manages countdown timer with proper cleanup via ref.onDispose().
class LoanBorrowerNotifier extends Notifier<LoanBorrowerState>
    with SafeNotifierMixin<LoanBorrowerState> {
  late final TransferRepository _repository;
  Timer? _countdownTimer;

  @override
  LoanBorrowerState build() {
    _repository = ref.watch(transferRepositoryProvider);

    // Register cleanup for countdown timer
    ref.onDispose(() {
      _countdownTimer?.cancel();
      _log.info('Countdown timer disposed');
    });

    return const LoanBorrowerState();
  }

  /// Initializes the ViewModel by loading the transfer.
  Future<void> initialize({required String transferId}) async {
    _log.info('Initializing loan borrower view for transfer: $transferId');

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

      // Determine current phase
      final currentPhase = _determinePhase(transfer);

      // Extract pickup details if they exist
      // Note: Int64 requires explicit .toInt() for comparison with int literals
      DateTime? pickupDate;
      TimeOfDay? pickupTime;
      int? durationDays;

      if (transfer.hasEstimatedPickupUnixSec()) {
        final estimatedPickup = DateTime.fromMillisecondsSinceEpoch(
          transfer.estimatedPickupUnixSec.toInt() * 1000,
        );
        pickupDate = DateTime(
          estimatedPickup.year,
          estimatedPickup.month,
          estimatedPickup.day,
        );
        pickupTime = TimeOfDay.fromDateTime(estimatedPickup);
      }

      if (transfer.hasLoanDurationDays()) {
        durationDays = transfer.loanDurationDays;
      }

      safeUpdateState((s) => s.copyWith(
            currentPhase: currentPhase,
            transfer: transfer,
            selectedPickupDate: pickupDate,
            selectedPickupTime: pickupTime,
            selectedDurationDays: durationDays,
            isLoading: false,
          ));

      // Start countdown if in inUse phase
      if (currentPhase == LoanBorrowerPhase.inUse && transfer.hasActualPickupUnixSec()) {
        _startCountdown(transfer);
      }

      _log.info('Initialized: phase=$currentPhase');
    } catch (e, stackTrace) {
      _log.severe('Failed to initialize loan borrower view', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState((s) => s.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e),
          ));
    }
  }

  /// Sets the selected pickup date.
  void setPickupDate(DateTime date) {
    safeUpdateState((s) => s.copyWith(selectedPickupDate: date));
  }

  /// Sets the selected pickup time.
  void setPickupTime(TimeOfDay time) {
    safeUpdateState((s) => s.copyWith(selectedPickupTime: time));
  }

  /// Sets the selected loan duration in days.
  void setDuration(int days) {
    safeUpdateState((s) => s.copyWith(selectedDurationDays: days));
  }

  /// Submits the pickup details to the server.
  ///
  /// This updates the transfer with estimated pickup time and loan duration.
  Future<void> submitPickupDetails() async {
    final currentState = state;
    final transfer = currentState.transfer;

    if (transfer == null) {
      _log.warning('Cannot submit pickup details without a transfer');
      return;
    }

    if (currentState.selectedPickupDate == null ||
        currentState.selectedPickupTime == null) {
      _log.warning('Cannot submit pickup details without date and time');
      return;
    }

    _log.info('Submitting pickup details');
    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      // Combine date and time into a DateTime in UTC
      // Important: Unix timestamps are always UTC, so we create a UTC DateTime
      final pickupDateTime = DateTime.utc(
        currentState.selectedPickupDate!.year,
        currentState.selectedPickupDate!.month,
        currentState.selectedPickupDate!.day,
        currentState.selectedPickupTime!.hour,
        currentState.selectedPickupTime!.minute,
      );

      await _repository.updateTransfer(
        transferId: transfer.id,
        estimatedPickupUnixSec: pickupDateTime.millisecondsSinceEpoch ~/ 1000,
        loanDurationDays: currentState.selectedDurationDays,
      );

      if (!ref.mounted) return;

      // Reload transfer to get updated state
      final updatedTransfer = await _repository.getTransfer(transfer.id);
      if (!ref.mounted) return;

      if (updatedTransfer == null) {
        safeUpdateState((s) => s.copyWith(
              isLoading: false,
              error: const UserError.generic(
                  fallback: 'Could not reload transfer after update'),
            ));
        return;
      }

      final newPhase = _determinePhase(updatedTransfer);

      safeUpdateState((s) => s.copyWith(
            currentPhase: newPhase,
            transfer: updatedTransfer,
            isLoading: false,
          ));

      _log.info('Pickup details submitted successfully, phase is now: $newPhase');
    } catch (e, stackTrace) {
      _log.severe('Failed to submit pickup details', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState((s) => s.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e, fallback: 'Could not submit pickup details'),
          ));
    }
  }

  /// Marks the item as picked up (starts the loan). Returns the
  /// community_event_id so the caller can wire an undo snackbar.
  Future<String?> markPickedUp() async {
    final transfer = state.transfer;
    if (transfer == null) return null;

    _log.info('Marking item as picked up for transfer: ${transfer.id}');
    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    String? communityEventId;
    try {
      communityEventId =
          await _repository.startLoan(transferId: transfer.id);

      if (!ref.mounted) return communityEventId;

      // Reload transfer to get updated state
      final updatedTransfer = await _repository.getTransfer(transfer.id);
      if (!ref.mounted) return communityEventId;

      final currentPhase = _determinePhase(updatedTransfer);

      safeUpdateState((s) => s.copyWith(
            currentPhase: currentPhase,
            transfer: updatedTransfer,
            isLoading: false,
          ));

      // Start countdown if transitioned to inUse phase
      if (currentPhase == LoanBorrowerPhase.inUse && updatedTransfer != null) {
        _startCountdown(updatedTransfer);
      }

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

  /// Marks the loan as returned (completed). Returns the
  /// CommunityEvent id of the completion action so the caller can wire
  /// an undo snackbar; null on error.
  Future<String?> confirmReturn() async {
    final transfer = state.transfer;
    if (transfer == null) return null;

    _log.info('Confirming return for transfer: ${transfer.id}');
    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    String? communityEventId;
    try {
      final response = await _repository.completeTransfer(transferId: transfer.id);
      communityEventId = response.communityEventId;

      if (!ref.mounted) return communityEventId;

      // Stop countdown timer
      _countdownTimer?.cancel();

      // Reload transfer to get updated state
      final updatedTransfer = await _repository.getTransfer(transfer.id);
      if (!ref.mounted) return communityEventId;

      final currentPhase = _determinePhase(updatedTransfer);

      safeUpdateState((s) => s.copyWith(
            currentPhase: currentPhase,
            transfer: updatedTransfer,
            isLoading: false,
          ));

      _log.info('Return confirmed successfully');
      return communityEventId;
    } catch (e, stackTrace) {
      _log.severe('Failed to confirm return', e, stackTrace);
      if (!ref.mounted) return null;

      safeUpdateState((s) => s.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e, fallback: 'Could not confirm return'),
          ));
      return null;
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

      // Stop countdown timer
      _countdownTimer?.cancel();

      _log.info('Interest withdrawn successfully');

      // Clear the transfer
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

  /// Starts the countdown timer for the inUse phase.
  ///
  /// Updates countdown every minute (not every second for battery efficiency).
  void _startCountdown(Transfer transfer) {
    if (!transfer.hasActualPickupUnixSec() || !transfer.hasLoanDurationDays()) {
      _log.warning('Cannot start countdown without pickup time and duration');
      return;
    }

    _countdownTimer?.cancel(); // Cancel any existing timer

    final actualPickup = DateTime.fromMillisecondsSinceEpoch(
      transfer.actualPickupUnixSec.toInt() * 1000,
    );
    final returnDate = actualPickup.add(Duration(days: transfer.loanDurationDays));

    _log.info('Starting countdown to: $returnDate');

    // Update immediately
    _updateCountdown(returnDate);

    // Then update every minute
    _countdownTimer = Timer.periodic(const Duration(minutes: 1), (_) {
      if (!ref.mounted) {
        _countdownTimer?.cancel();
        return;
      }
      _updateCountdown(returnDate);
    });
  }

  /// Updates the countdown state based on the remaining time.
  void _updateCountdown(DateTime returnDate) {
    final now = DateTime.now();
    final remaining = returnDate.difference(now);

    if (remaining.isNegative) {
      // Loan is overdue
      safeUpdateState((s) => s.copyWith(
            countdownDays: 0,
            countdownHours: 0,
            countdownMinutes: 0,
          ));
    } else {
      safeUpdateState((s) => s.copyWith(
            countdownDays: remaining.inDays,
            countdownHours: remaining.inHours % 24,
            countdownMinutes: remaining.inMinutes % 60,
          ));
    }
  }

  /// Determines the current phase based on the transfer state.
  LoanBorrowerPhase _determinePhase(Transfer? transfer) {
    if (transfer == null) {
      return LoanBorrowerPhase.arrangePickup;
    }

    switch (transfer.state) {
      case TransferState.TRANSFER_STATE_INTEREST_EXPRESSED:
        return LoanBorrowerPhase.arrangePickup;
      case TransferState.TRANSFER_STATE_RECIPIENT_SELECTED:
        // If pickup details are set, we're confirmed. Otherwise still arranging.
        if (transfer.hasEstimatedPickupUnixSec() && transfer.hasLoanDurationDays()) {
          return LoanBorrowerPhase.confirmed;
        }
        return LoanBorrowerPhase.arrangePickup;
      case TransferState.TRANSFER_STATE_ACTIVE:
        return LoanBorrowerPhase.inUse;
      case TransferState.TRANSFER_STATE_COMPLETED:
        return LoanBorrowerPhase.returned;
      default:
        return LoanBorrowerPhase.arrangePickup;
    }
  }
}
