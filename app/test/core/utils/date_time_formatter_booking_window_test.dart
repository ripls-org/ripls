import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';

/// formatBookingWindow renders a booking's inclusive day range compactly so
/// who's-using-it rows can distinguish multiple bookings by the same borrower
/// (#2638).
void main() {
  int unix(DateTime d) => d.millisecondsSinceEpoch ~/ 1000;

  group('DateTimeFormatter.formatBookingWindow', () {
    test('single-day booking renders one date', () {
      final day = unix(DateTime(2026, 7, 18));
      expect(DateTimeFormatter.formatBookingWindow(day, day), 'Jul 18');
    });

    test('same-month range renders a compact day range', () {
      final start = unix(DateTime(2026, 7, 18));
      final end = unix(DateTime(2026, 7, 20));
      expect(DateTimeFormatter.formatBookingWindow(start, end), 'Jul 18–20');
    });

    test('cross-month range repeats the month', () {
      final start = unix(DateTime(2026, 7, 30));
      final end = unix(DateTime(2026, 8, 2));
      expect(
        DateTimeFormatter.formatBookingWindow(start, end),
        'Jul 30 – Aug 2',
      );
    });
  });
}
