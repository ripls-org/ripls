import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_utils.dart';

void main() {
  group('formatRelativeTimestamp', () {
    test('returns "Just now" for times less than 60 seconds ago', () {
      final now = DateTime.now();
      final unixSec = (now.millisecondsSinceEpoch / 1000).floor();
      expect(formatRelativeTimestamp(unixSec), 'Just now');
      expect(formatRelativeTimestamp(unixSec - 15), 'Just now');
      expect(formatRelativeTimestamp(unixSec - 45), 'Just now');
    });

    test('returns minutes for times less than 60 minutes ago', () {
      final now = DateTime.now();
      final baseUnixSec = (now.millisecondsSinceEpoch / 1000).floor();
      expect(formatRelativeTimestamp(baseUnixSec - 60), '1m ago');
      expect(formatRelativeTimestamp(baseUnixSec - 120), '2m ago');
      expect(formatRelativeTimestamp(baseUnixSec - 1800), '30m ago');
      expect(formatRelativeTimestamp(baseUnixSec - 3540), '59m ago');
    });

    test('returns hours for times less than 24 hours ago', () {
      final now = DateTime.now();
      final baseUnixSec = (now.millisecondsSinceEpoch / 1000).floor();
      expect(formatRelativeTimestamp(baseUnixSec - 3600), '1h ago');
      expect(formatRelativeTimestamp(baseUnixSec - 7200), '2h ago');
      expect(formatRelativeTimestamp(baseUnixSec - 43200), '12h ago');
      expect(formatRelativeTimestamp(baseUnixSec - 86340), '23h ago');
    });

    test('returns days for times less than 7 days ago', () {
      final now = DateTime.now();
      final baseUnixSec = (now.millisecondsSinceEpoch / 1000).floor();
      expect(formatRelativeTimestamp(baseUnixSec - 86400), '1d ago');
      expect(formatRelativeTimestamp(baseUnixSec - 172800), '2d ago');
      expect(formatRelativeTimestamp(baseUnixSec - 518400), '6d ago');
    });

    test('returns weeks for times less than 30 days ago', () {
      final now = DateTime.now();
      final baseUnixSec = (now.millisecondsSinceEpoch / 1000).floor();
      expect(formatRelativeTimestamp(baseUnixSec - 604800), '1w ago');
      expect(formatRelativeTimestamp(baseUnixSec - 1209600), '2w ago');
      expect(formatRelativeTimestamp(baseUnixSec - 1814400), '3w ago');
    });

    test('returns months for times less than 365 days ago', () {
      final now = DateTime.now();
      final baseUnixSec = (now.millisecondsSinceEpoch / 1000).floor();
      expect(formatRelativeTimestamp(baseUnixSec - 2592000), '1mo ago');
      expect(formatRelativeTimestamp(baseUnixSec - 5184000), '2mo ago');
      expect(formatRelativeTimestamp(baseUnixSec - 25920000), '10mo ago');
    });

    test('returns years for times 365+ days ago', () {
      final now = DateTime.now();
      final baseUnixSec = (now.millisecondsSinceEpoch / 1000).floor();
      expect(formatRelativeTimestamp(baseUnixSec - 31536000), '1y ago');
      expect(formatRelativeTimestamp(baseUnixSec - 63072000), '2y ago');
    });
  });

  group('formatPickupDate', () {
    test('formats date correctly', () {
      final date = DateTime(2026, 2, 15);
      expect(formatPickupDate(date), 'Sun, Feb 15');
    });

    test('formats different dates correctly', () {
      expect(formatPickupDate(DateTime(2026, 1, 1)), 'Thu, Jan 1');
      expect(formatPickupDate(DateTime(2026, 12, 31)), 'Thu, Dec 31');
    });
  });

  group('formatPickupTime', () {
    test('formats time correctly', () {
      final time = DateTime(2026, 2, 15, 9, 0);
      expect(formatPickupTime(time), '9:00 AM');
    });

    test('formats different times correctly', () {
      expect(formatPickupTime(DateTime(2026, 2, 15, 13, 30)), '1:30 PM');
      expect(formatPickupTime(DateTime(2026, 2, 15, 0, 0)), '12:00 AM');
      expect(formatPickupTime(DateTime(2026, 2, 15, 12, 0)), '12:00 PM');
    });
  });

  group('formatDuration', () {
    test('formats 1 day correctly', () {
      expect(formatDuration(1), '1 day');
    });

    test('formats multiple days correctly', () {
      expect(formatDuration(2), '2 days');
      expect(formatDuration(3), '3 days');
      expect(formatDuration(6), '6 days');
    });

    test('formats 1 week correctly', () {
      expect(formatDuration(7), '1 week');
    });

    test('formats 2 weeks correctly', () {
      expect(formatDuration(14), '2 weeks');
    });

    test('formats weeks correctly', () {
      expect(formatDuration(21), '3 weeks');
      expect(formatDuration(28), '4 weeks');
    });

    test('formats months correctly', () {
      expect(formatDuration(30), '1 month');
      expect(formatDuration(60), '2 months');
      expect(formatDuration(90), '3 months');
    });

    test('formats years correctly', () {
      expect(formatDuration(365), '1 year');
      expect(formatDuration(730), '2 years');
    });
  });
}
