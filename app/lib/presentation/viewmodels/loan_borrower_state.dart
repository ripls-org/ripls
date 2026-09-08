import 'package:flutter/material.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';

part 'loan_borrower_state.freezed.dart';

/// Phases in the loan borrower workflow.
enum LoanBorrowerPhase {
  /// Step 0: Set date, time, duration
  arrangePickup,

  /// Step 1: Summary, ready to pick up
  confirmed,

  /// Step 2: Countdown to return date
  inUse,

  /// Step 3: Confirm return to owner
  confirmReturn,

  /// Step 4: Complete with impact metrics
  returned,
}

/// State for LoanBorrowerViewModel.
///
/// Manages the state of loan transfers from the borrower's perspective,
/// tracking the current phase, pickup details, and countdown timer.
@freezed
sealed class LoanBorrowerState with _$LoanBorrowerState {
  const factory LoanBorrowerState({
    /// Current phase in the borrower workflow
    @Default(LoanBorrowerPhase.arrangePickup) LoanBorrowerPhase currentPhase,

    /// The active transfer
    Transfer? transfer,

    /// Selected pickup date
    DateTime? selectedPickupDate,

    /// Selected pickup time
    TimeOfDay? selectedPickupTime,

    /// Selected loan duration in days
    int? selectedDurationDays,

    /// Countdown: days remaining
    @Default(0) int countdownDays,

    /// Countdown: hours remaining (0-23)
    @Default(0) int countdownHours,

    /// Countdown: minutes remaining (0-59)
    @Default(0) int countdownMinutes,

    /// Whether data is currently being loaded
    @Default(false) bool isLoading,

    /// Error, if any
    UserError? error,
  }) = _LoanBorrowerState;

  const LoanBorrowerState._();

  /// Returns true if all pickup details are selected
  bool get hasCompletePickupDetails =>
      selectedPickupDate != null &&
      selectedPickupTime != null &&
      selectedDurationDays != null;

  /// Returns true if an active transfer is present
  bool get hasTransfer => transfer != null;

  /// Returns the progress steps for the progress bar
  List<String> get progressSteps => [
        'Arrange',
        'Confirmed',
        'In Use',
        'Return',
        'Complete',
      ];

  /// Returns the current step index for the progress bar
  int get currentStepIndex => currentPhase.index;
  }
