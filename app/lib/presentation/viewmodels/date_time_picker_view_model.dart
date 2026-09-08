import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/presentation/viewmodels/date_time_picker_state.dart';

/// Provider for the [DateTimePickerNotifier].
///
/// Deliberately **not** `autoDispose`. The widget's post-frame `initialize(...)`
/// runs after the first build via `ref.read(...).notifier`, which doesn't
/// register a listener. With `autoDispose` the provider is then collected
/// before the next `ref.watch` fires, so the watch would create a fresh
/// notifier (placeholder state) and discard whatever the user picked. The
/// state object is tiny and the picker is opened rarely, so holding it for
/// the session is the right tradeoff.
final dateTimePickerProvider =
    NotifierProvider<DateTimePickerNotifier, DateTimePickerState>(
  DateTimePickerNotifier.new,
);

/// ViewModel for the combined date+time modal.
///
/// Holds the wheel + calendar selection state. Has no repository or service
/// dependencies — the modal is a pure picker and never persists anything;
/// callers receive the result via the Navigator result pattern.
class DateTimePickerNotifier extends Notifier<DateTimePickerState> {
  @override
  DateTimePickerState build() {
    // Placeholder state. The modal widget gates UI on its own `_initialized`
    // flag and reads only after [initialize] has run, so this default is
    // never observed by users. Returning a throwing build() puts the provider
    // in a permanent error state that `.notifier` access cannot recover from.
    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    return DateTimePickerState(
      selectedDate: today,
      hour12: 12,
      minute: 0,
      isPm: false,
      visibleMonth: DateTime(now.year, now.month, 1),
      firstDate: today,
      lastDate: today.add(const Duration(days: 365)),
    );
  }

  /// Seeds the notifier with the caller's initial datetime and clamp range.
  /// Clamps [initial] into `[firstDate, lastDate]` so out-of-range callers
  /// never end up with a selected day outside the addressable window.
  void initialize({
    required DateTime initial,
    required DateTime firstDate,
    required DateTime lastDate,
  }) {
    final clamped = _clamp(initial, firstDate, lastDate);
    final hour24 = clamped.hour;
    final isPm = hour24 >= 12;
    final hour12 = _toHour12(hour24);
    final snappedMinute = _snapMinute(clamped.minute);
    state = DateTimePickerState(
      selectedDate: DateTime(clamped.year, clamped.month, clamped.day),
      hour12: hour12,
      minute: snappedMinute,
      isPm: isPm,
      visibleMonth: DateTime(clamped.year, clamped.month, 1),
      firstDate: firstDate,
      lastDate: lastDate,
    );
  }

  /// Selects a day on the visible calendar. No-op if the date is outside the
  /// allowed `[firstDate, lastDate]` window.
  void selectDay(DateTime day) {
    final normalized = DateTime(day.year, day.month, day.day);
    final firstDay = DateTime(state.firstDate.year, state.firstDate.month,
        state.firstDate.day);
    final lastDay =
        DateTime(state.lastDate.year, state.lastDate.month, state.lastDate.day);
    if (normalized.isBefore(firstDay) || normalized.isAfter(lastDay)) {
      return;
    }
    state = state.copyWith(selectedDate: normalized);
  }

  /// Advances the visible month by [delta] (positive = forward). Clamps to
  /// `[firstDate, lastDate]`.
  void shiftMonth(int delta) {
    final next =
        DateTime(state.visibleMonth.year, state.visibleMonth.month + delta, 1);
    final firstMonth =
        DateTime(state.firstDate.year, state.firstDate.month, 1);
    final lastMonth = DateTime(state.lastDate.year, state.lastDate.month, 1);
    if (next.isBefore(firstMonth) || next.isAfter(lastMonth)) return;
    state = state.copyWith(visibleMonth: next);
  }

  void setHour12(int hour12) {
    if (hour12 < 1 || hour12 > 12) return;
    state = state.copyWith(hour12: hour12);
  }

  void setMinute(int minute) {
    final snapped = _snapMinute(minute);
    state = state.copyWith(minute: snapped);
  }

  void setIsPm(bool isPm) {
    state = state.copyWith(isPm: isPm);
  }
}

DateTime _clamp(DateTime value, DateTime first, DateTime last) {
  if (value.isBefore(first)) return first;
  if (value.isAfter(last)) return last;
  return value;
}

int _toHour12(int hour24) {
  final mod = hour24 % 12;
  return mod == 0 ? 12 : mod;
}

int _snapMinute(int minute) {
  if (minute < 0) return 0;
  if (minute >= 60) return 45;
  // Snap to nearest 15.
  return ((minute / 15).round() * 15).clamp(0, 45);
}
