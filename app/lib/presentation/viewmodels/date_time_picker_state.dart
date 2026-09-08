import 'package:freezed_annotation/freezed_annotation.dart';

part 'date_time_picker_state.freezed.dart';

/// State for [DateTimePickerNotifier].
///
/// Tracks the currently-selected day, hour (1-12), minute (0/15/30/45), and
/// AM/PM, plus the month the calendar is currently showing and the
/// `firstDate` / `lastDate` clamp range.
@freezed
sealed class DateTimePickerState with _$DateTimePickerState {
  const factory DateTimePickerState({
    required DateTime selectedDate,
    required int hour12,
    required int minute,
    required bool isPm,
    required DateTime visibleMonth,
    required DateTime firstDate,
    required DateTime lastDate,
  }) = _DateTimePickerState;

  const DateTimePickerState._();

  /// Resolves the selected wheel + calendar state into a single [DateTime].
  DateTime get selectedDateTime {
    final hour24 = _toHour24(hour12, isPm);
    return DateTime(
      selectedDate.year,
      selectedDate.month,
      selectedDate.day,
      hour24,
      minute,
    );
  }

  bool get canGoToPreviousMonth {
    final prev = DateTime(visibleMonth.year, visibleMonth.month - 1, 1);
    final firstMonth = DateTime(firstDate.year, firstDate.month, 1);
    return !prev.isBefore(firstMonth);
  }

  bool get canGoToNextMonth {
    final next = DateTime(visibleMonth.year, visibleMonth.month + 1, 1);
    final lastMonth = DateTime(lastDate.year, lastDate.month, 1);
    return !next.isAfter(lastMonth);
  }
}

int _toHour24(int hour12, bool isPm) {
  final base = hour12 % 12;
  return isPm ? base + 12 : base;
}

/// Result returned from the date-time picker modal via the Navigator result
/// pattern (see [docs/client/modals.md]).
///
/// `null` from `Navigator.pop` represents a cancel; callers should treat it
/// as no-op.
@freezed
sealed class DateTimePickerResult with _$DateTimePickerResult {
  /// User confirmed a new date+time.
  const factory DateTimePickerResult.saved(DateTime value) =
      DateTimePickerResultSaved;

  /// User pressed the destructive Remove action (only available when the
  /// caller passed `allowRemove: true`).
  const factory DateTimePickerResult.removed() = DateTimePickerResultRemoved;
}
