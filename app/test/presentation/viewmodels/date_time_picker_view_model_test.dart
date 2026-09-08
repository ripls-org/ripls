import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/date_time_picker_state.dart';
import 'package:ripls/presentation/viewmodels/date_time_picker_view_model.dart';

void main() {
  group('DateTimePickerNotifier', () {
    late ProviderContainer container;

    setUp(() {
      container = ProviderContainer();
    });

    tearDown(() {
      container.dispose();
    });

    DateTimePickerNotifier notifier() =>
        container.read(dateTimePickerProvider.notifier);

    DateTimePickerState read() => container.read(dateTimePickerProvider);

    test('initialize clamps initial inside [firstDate, lastDate]', () {
      final first = DateTime(2026, 5, 1);
      final last = DateTime(2026, 5, 31);
      notifier().initialize(
        initial: DateTime(2026, 4, 30, 10, 0),
        firstDate: first,
        lastDate: last,
      );
      expect(read().selectedDate, DateTime(2026, 5, 1));
      expect(read().visibleMonth, DateTime(2026, 5, 1));
    });

    test('initialize splits hour into 12h + isPm and snaps minutes', () {
      notifier().initialize(
        initial: DateTime(2026, 5, 16, 14, 22),
        firstDate: DateTime(2026, 1, 1),
        lastDate: DateTime(2026, 12, 31),
      );
      final s = read();
      expect(s.hour12, 2);
      expect(s.isPm, true);
      // 22 snaps to the nearest 15 = 15.
      expect(s.minute, 15);
    });

    test('initialize maps midnight to 12 AM and noon to 12 PM', () {
      notifier().initialize(
        initial: DateTime(2026, 5, 16, 0, 0),
        firstDate: DateTime(2026, 5, 1),
        lastDate: DateTime(2026, 5, 31),
      );
      expect(read().hour12, 12);
      expect(read().isPm, false);

      notifier().initialize(
        initial: DateTime(2026, 5, 16, 12, 0),
        firstDate: DateTime(2026, 5, 1),
        lastDate: DateTime(2026, 5, 31),
      );
      expect(read().hour12, 12);
      expect(read().isPm, true);
    });

    test('selectDay ignores out-of-range dates', () {
      notifier().initialize(
        initial: DateTime(2026, 5, 16, 9, 0),
        firstDate: DateTime(2026, 5, 10),
        lastDate: DateTime(2026, 5, 20),
      );
      final original = read().selectedDate;
      notifier().selectDay(DateTime(2026, 5, 5));
      expect(read().selectedDate, original);
      notifier().selectDay(DateTime(2026, 5, 25));
      expect(read().selectedDate, original);
    });

    test('selectDay accepts an in-range date', () {
      notifier().initialize(
        initial: DateTime(2026, 5, 16, 9, 0),
        firstDate: DateTime(2026, 5, 10),
        lastDate: DateTime(2026, 5, 20),
      );
      notifier().selectDay(DateTime(2026, 5, 18));
      expect(read().selectedDate, DateTime(2026, 5, 18));
    });

    test('shiftMonth respects firstDate / lastDate bounds', () {
      notifier().initialize(
        initial: DateTime(2026, 5, 16, 9, 0),
        firstDate: DateTime(2026, 5, 10),
        lastDate: DateTime(2026, 6, 5),
      );
      // Backward into April is blocked.
      expect(read().canGoToPreviousMonth, false);
      notifier().shiftMonth(-1);
      expect(read().visibleMonth, DateTime(2026, 5, 1));

      // Forward into June is allowed.
      expect(read().canGoToNextMonth, true);
      notifier().shiftMonth(1);
      expect(read().visibleMonth, DateTime(2026, 6, 1));

      // Forward into July is blocked.
      expect(read().canGoToNextMonth, false);
      notifier().shiftMonth(1);
      expect(read().visibleMonth, DateTime(2026, 6, 1));
    });

    test('setMinute snaps to nearest 15 and clamps into [0, 45]', () {
      notifier().initialize(
        initial: DateTime(2026, 5, 16, 9, 0),
        firstDate: DateTime(2026, 5, 1),
        lastDate: DateTime(2026, 5, 31),
      );
      notifier().setMinute(7);
      expect(read().minute, 0); // 7 rounds down to 0 / nearest is 0 vs 15.
      notifier().setMinute(8);
      expect(read().minute, 15);
      notifier().setMinute(60);
      expect(read().minute, 45);
      notifier().setMinute(-3);
      expect(read().minute, 0);
    });

    test('setHour12 rejects out-of-range values', () {
      notifier().initialize(
        initial: DateTime(2026, 5, 16, 9, 0),
        firstDate: DateTime(2026, 5, 1),
        lastDate: DateTime(2026, 5, 31),
      );
      notifier().setHour12(13);
      expect(read().hour12, 9);
      notifier().setHour12(0);
      expect(read().hour12, 9);
      notifier().setHour12(5);
      expect(read().hour12, 5);
    });

    test('selectedDateTime composes wheels + selected day into one DateTime',
        () {
      notifier().initialize(
        initial: DateTime(2026, 5, 16, 14, 30),
        firstDate: DateTime(2026, 5, 1),
        lastDate: DateTime(2026, 5, 31),
      );
      notifier().selectDay(DateTime(2026, 5, 20));
      notifier().setHour12(9);
      notifier().setMinute(45);
      notifier().setIsPm(false);
      final dt = read().selectedDateTime;
      expect(dt, DateTime(2026, 5, 20, 9, 45));
    });

    test('setIsPm flips period and round-trips to 24h via selectedDateTime',
        () {
      notifier().initialize(
        initial: DateTime(2026, 5, 16, 8, 0),
        firstDate: DateTime(2026, 5, 1),
        lastDate: DateTime(2026, 5, 31),
      );
      notifier().setIsPm(true);
      expect(read().selectedDateTime.hour, 20);
      notifier().setIsPm(false);
      expect(read().selectedDateTime.hour, 8);
    });
  });

  group('DateTimePickerResult', () {
    test('saved variant carries DateTime', () {
      final r = DateTimePickerResult.saved(DateTime(2026, 5, 16, 9, 0));
      expect(r is DateTimePickerResultSaved, true);
      expect((r as DateTimePickerResultSaved).value,
          DateTime(2026, 5, 16, 9, 0));
    });

    test('removed variant has no payload', () {
      const r = DateTimePickerResult.removed();
      expect(r is DateTimePickerResultRemoved, true);
    });
  });
}
