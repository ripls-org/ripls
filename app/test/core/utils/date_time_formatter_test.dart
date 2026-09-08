import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:timezone/data/latest.dart' as tz;

void main() {
  // Initialize timezone database once for all tests
  setUpAll(() {
    tz.initializeTimeZones();
  });

  group('DateTimeFormatter', () {
    group('formatReturnDate', () {
      test('returns "Return today" for same day', () {
        final now = DateTime.now();
        final todayUnixSec = now.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatReturnDate(todayUnixSec);

        expect(result, 'Return today');
      });

      test('returns "Return tomorrow" for next day', () {
        final tomorrow = DateTime.now().add(const Duration(days: 1));
        final tomorrowUnixSec = tomorrow.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatReturnDate(tomorrowUnixSec);

        expect(result, 'Return tomorrow');
      });

      test('returns "Return in X days" for future dates', () {
        final futureDate = DateTime.now().add(const Duration(days: 5));
        final futureUnixSec = futureDate.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatReturnDate(futureUnixSec);

        expect(result, 'Return in 5 days');
      });

      test('returns "Return X days ago" for past dates', () {
        final pastDate = DateTime.now().subtract(const Duration(days: 3));
        final pastUnixSec = pastDate.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatReturnDate(pastUnixSec);

        expect(result, 'Return 3 days ago');
      });

      test('handles date at midnight correctly', () {
        final now = DateTime.now();
        final today = DateTime(now.year, now.month, now.day);
        final todayUnixSec = today.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatReturnDate(todayUnixSec);

        expect(result, 'Return today');
      });

      test('handles date near midnight transition', () {
        final now = DateTime.now();
        // Use calendar date arithmetic (DST-safe) to get tomorrow at 23:59.
        final tomorrow = DateTime(now.year, now.month, now.day + 1, 23, 59);
        final tomorrowUnixSec = tomorrow.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatReturnDate(tomorrowUnixSec);

        expect(result, 'Return tomorrow');
      });
    });

    group('formatDuration', () {
      test('returns "Same day" for zero duration', () {
        final start = DateTime(2024, 1, 15, 10, 0);
        final end = DateTime(2024, 1, 15, 18, 0);
        final startUnixSec = start.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = end.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDuration(startUnixSec, endUnixSec);

        expect(result, 'Same day');
      });

      test('returns "1 day" for single day duration', () {
        final start = DateTime(2024, 1, 15);
        final end = DateTime(2024, 1, 16);
        final startUnixSec = start.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = end.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDuration(startUnixSec, endUnixSec);

        expect(result, '1 day');
      });

      test('returns "X days" for multiple days less than a week', () {
        final start = DateTime(2024, 1, 15);
        final end = DateTime(2024, 1, 20);
        final startUnixSec = start.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = end.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDuration(startUnixSec, endUnixSec);

        expect(result, '5 days');
      });

      test('returns "1 week" for 7 days', () {
        final start = DateTime(2024, 1, 15);
        final end = DateTime(2024, 1, 22);
        final startUnixSec = start.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = end.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDuration(startUnixSec, endUnixSec);

        expect(result, '1 week');
      });

      test('returns "X weeks" for multiple weeks', () {
        final start = DateTime(2024, 1, 15);
        final end = DateTime(2024, 1, 29);
        final startUnixSec = start.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = end.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDuration(startUnixSec, endUnixSec);

        expect(result, '2 weeks');
      });

      test('returns "1 month" for 30 days', () {
        final start = DateTime(2024, 1, 15);
        final end = DateTime(2024, 2, 14);
        final startUnixSec = start.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = end.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDuration(startUnixSec, endUnixSec);

        expect(result, '1 month');
      });

      test('returns "X months" for multiple months', () {
        final start = DateTime(2024, 1, 15);
        final end = DateTime(2024, 4, 15);
        final startUnixSec = start.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = end.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDuration(startUnixSec, endUnixSec);

        expect(result, '3 months');
      });

      test('handles exact week boundary (7 days)', () {
        final start = DateTime(2024, 1, 1);
        final end = DateTime(2024, 1, 8);
        final startUnixSec = start.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = end.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDuration(startUnixSec, endUnixSec);

        expect(result, '1 week');
      });

      test('handles exact month boundary (30 days)', () {
        final start = DateTime(2024, 1, 1);
        final end = DateTime(2024, 1, 31);
        final startUnixSec = start.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = end.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDuration(startUnixSec, endUnixSec);

        expect(result, '1 month');
      });
    });

    group('formatLoanDuration', () {
      test('is an alias for formatDuration', () {
        final start = DateTime(2024, 1, 15);
        final end = DateTime(2024, 1, 20);
        final startUnixSec = start.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = end.millisecondsSinceEpoch ~/ 1000;

        final resultDuration = DateTimeFormatter.formatDuration(
          startUnixSec,
          endUnixSec,
        );
        final resultLoan = DateTimeFormatter.formatLoanDuration(
          startUnixSec,
          endUnixSec,
        );

        expect(resultLoan, resultDuration);
        expect(resultLoan, '5 days');
      });
    });

    group('formatDayAndDate', () {
      test('formats Monday correctly', () {
        // Jan 15, 2024 is a Monday
        final date = DateTime(2024, 1, 15, 10, 30);
        final unixSec = date.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDayAndDate(unixSec);

        expect(result, 'Monday, Jan 15');
      });

      test('formats Friday correctly', () {
        // Jan 19, 2024 is a Friday
        final date = DateTime(2024, 1, 19, 14, 0);
        final unixSec = date.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDayAndDate(unixSec);

        expect(result, 'Friday, Jan 19');
      });

      test('formats Sunday correctly', () {
        // Jan 21, 2024 is a Sunday
        final date = DateTime(2024, 1, 21, 9, 0);
        final unixSec = date.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDayAndDate(unixSec);

        expect(result, 'Sunday, Jan 21');
      });

      test('formats December correctly', () {
        // Dec 25, 2024 is a Wednesday
        final date = DateTime(2024, 12, 25, 12, 0);
        final unixSec = date.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDayAndDate(unixSec);

        expect(result, 'Wednesday, Dec 25');
      });

      test('handles single digit day', () {
        // Jan 1, 2024 is a Monday
        final date = DateTime(2024, 1, 1, 8, 0);
        final unixSec = date.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatDayAndDate(unixSec);

        expect(result, 'Monday, Jan 1');
      });
    });

    group('formatWallClockTimeRange', () {
      test('omits the start meridiem when it matches the end', () {
        // 9:00 AM + 60 min -> 10:00 AM (both AM).
        final start = DateTime(2024, 6, 13, 9, 0);
        final unixSec = start.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatWallClockTimeRange(unixSec, 60);

        expect(result, '9:00 – 10:00 AM');
      });

      test('keeps both meridiems when start and end differ', () {
        // 11:00 AM + 120 min -> 1:00 PM (AM -> PM).
        final start = DateTime(2024, 6, 13, 11, 0);
        final unixSec = start.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatWallClockTimeRange(unixSec, 120);

        expect(result, '11:00 AM – 1:00 PM');
      });

      test('returns only the start time for a non-positive duration', () {
        final start = DateTime(2024, 6, 13, 9, 30);
        final unixSec = start.millisecondsSinceEpoch ~/ 1000;

        expect(
          DateTimeFormatter.formatWallClockTimeRange(unixSec, 0),
          '9:30 AM',
        );
      });

      test('returns empty string for a zero timestamp', () {
        expect(DateTimeFormatter.formatWallClockTimeRange(0, 60), '');
      });
    });

    group('formatRelativeEventTime', () {
      test('formats events in minutes (< 1 hour)', () {
        final now = DateTime.now();
        final fiveMinutesLater = now.add(const Duration(minutes: 5, seconds: 5));
        final result = DateTimeFormatter.formatRelativeEventTime(
          fiveMinutesLater.millisecondsSinceEpoch ~/ 1000,
        );
        expect(result, 'In 5min');
      });

      test('formats events in hours (< 24 hours)', () {
        final now = DateTime.now();
        final threeHoursLater = now.add(const Duration(hours: 3, seconds: 5));
        final result = DateTimeFormatter.formatRelativeEventTime(
          threeHoursLater.millisecondsSinceEpoch ~/ 1000,
        );
        expect(result, 'In 3hrs');
      });

      test('formats event exactly 1 hour away with minutes', () {
        final now = DateTime.now();
        final oneHourLater = now.add(const Duration(hours: 1, minutes: 1));
        final result = DateTimeFormatter.formatRelativeEventTime(
          oneHourLater.millisecondsSinceEpoch ~/ 1000,
        );
        // Unix timestamp rounding may lose precision, allow 0 or 1 minute
        expect(result, matches(r'^In 1hr [01]min$'));
      });

      test('formats event 23 hours away as hours', () {
        final now = DateTime.now();
        final twentyThreeHoursLater = now.add(const Duration(hours: 23));
        final result = DateTimeFormatter.formatRelativeEventTime(
          twentyThreeHoursLater.millisecondsSinceEpoch ~/ 1000,
        );
        // Could be 22, 23 hrs, or tomorrow depending on timing
        expect(result, matches(r'^(In 22hrs|In 23hrs|tomorrow)$'));
      });

      test('formats event exactly 1 day away as "tomorrow"', () {
        final now = DateTime.now();
        final oneDayLater = now.add(const Duration(days: 1, minutes: 1));
        final result = DateTimeFormatter.formatRelativeEventTime(
          oneDayLater.millisecondsSinceEpoch ~/ 1000,
        );
        expect(result, 'tomorrow');
      });

      test('formats events in days (< 7 days)', () {
        final now = DateTime.now();
        final fiveDaysLater = now.add(const Duration(days: 5));
        final result = DateTimeFormatter.formatRelativeEventTime(
          fiveDaysLater.millisecondsSinceEpoch ~/ 1000,
        );
        // Could be 4 or 5 days depending on timing
        expect(result, matches(r'^In [45] days$'));
      });

      test('formats event exactly 6 days away as days', () {
        final now = DateTime.now();
        final sixDaysLater = now.add(const Duration(days: 6));
        final result = DateTimeFormatter.formatRelativeEventTime(
          sixDaysLater.millisecondsSinceEpoch ~/ 1000,
        );
        // Could be 5, 6 days or month format depending on timing
        expect(result, matches(r'^(In 5 days|In 6 days|[A-Z][a-z]{2} \d{1,2})$'));
      });

      test('formats events >= 7 days away as month and day', () {
        final now = DateTime.now();
        final sevenDaysLater = now.add(const Duration(days: 7, hours: 1));
        final result = DateTimeFormatter.formatRelativeEventTime(
          sevenDaysLater.millisecondsSinceEpoch ~/ 1000,
        );
        // Check format is "Mon D" (e.g., "Jan 12")
        expect(result, matches(r'^[A-Z][a-z]{2} \d{1,2}$'));
      });

      test('formats events weeks away as month and day', () {
        final now = DateTime.now();
        final twoWeeksLater = now.add(const Duration(days: 14));
        final result = DateTimeFormatter.formatRelativeEventTime(
          twoWeeksLater.millisecondsSinceEpoch ~/ 1000,
        );
        // Check format is "Mon D" (e.g., "Jan 19")
        expect(result, matches(r'^[A-Z][a-z]{2} \d{1,2}$'));
      });

      test('formats events months away as month and day', () {
        final now = DateTime.now();
        final thirtyDaysLater = now.add(const Duration(days: 30));
        final result = DateTimeFormatter.formatRelativeEventTime(
          thirtyDaysLater.millisecondsSinceEpoch ~/ 1000,
        );
        // Check format is "Mon D" (e.g., "Feb 4")
        expect(result, matches(r'^[A-Z][a-z]{2} \d{1,2}$'));
      });

      test('returns "Completed" for past events by default', () {
        final now = DateTime.now();
        final fiveMinutesAgo = now.subtract(const Duration(minutes: 5));
        final result = DateTimeFormatter.formatRelativeEventTime(
          fiveMinutesAgo.millisecondsSinceEpoch ~/ 1000,
        );
        expect(result, 'Completed');
      });

      test('returns custom status for past events when provided', () {
        final now = DateTime.now();
        final oneHourAgo = now.subtract(const Duration(hours: 1));
        final result = DateTimeFormatter.formatRelativeEventTime(
          oneHourAgo.millisecondsSinceEpoch ~/ 1000,
          status: 'Cancelled',
        );
        expect(result, 'Cancelled');
      });

      test('returns custom status for past events (one day ago)', () {
        final now = DateTime.now();
        final oneDayAgo = now.subtract(const Duration(days: 1));
        final result = DateTimeFormatter.formatRelativeEventTime(
          oneDayAgo.millisecondsSinceEpoch ~/ 1000,
          status: 'Completed',
        );
        expect(result, 'Completed');
      });

      test('returns custom status for past events (one week ago)', () {
        final now = DateTime.now();
        final oneWeekAgo = now.subtract(const Duration(days: 7));
        final result = DateTimeFormatter.formatRelativeEventTime(
          oneWeekAgo.millisecondsSinceEpoch ~/ 1000,
          status: 'Cancelled',
        );
        expect(result, 'Cancelled');
      });

      test('handles edge case: very near future (< 1 minute)', () {
        final now = DateTime.now();
        final veryNearFuture = now.add(const Duration(seconds: 10));
        final result = DateTimeFormatter.formatRelativeEventTime(
          veryNearFuture.millisecondsSinceEpoch ~/ 1000,
        );
        // Should be "In 0min" or "In 1min" depending on milliseconds
        expect(result, matches(r'^In [01]min$'));
      });

      test('handles edge case: ~59 minutes away', () {
        final now = DateTime.now();
        final fiftyNineMinutesLater = now.add(const Duration(minutes: 59, seconds: 30));
        final result = DateTimeFormatter.formatRelativeEventTime(
          fiftyNineMinutesLater.millisecondsSinceEpoch ~/ 1000,
        );
        // Should be "In 59min" (might be 58 or 59 depending on milliseconds)
        expect(result, matches(r'^In (58|59)min$'));
      });

      test('handles edge case: ~24 hours away', () {
        final now = DateTime.now();
        final twentyFourHoursLater = now.add(const Duration(hours: 24, minutes: 2));
        final result = DateTimeFormatter.formatRelativeEventTime(
          twentyFourHoursLater.millisecondsSinceEpoch ~/ 1000,
        );
        // Could be "In 23hrs", "In 24hrs", "tomorrow", or "In 1 days" depending on timing
        expect(result, matches(r'^(In 23hrs|In 24hrs|tomorrow|In 1 days|In 2 days)$'));
      });
    });

    group('formatRelativeEventTimeWithTimezone', () {
      test('formats events with timezone conversion (PST to EST)', () {
        // Event at 10 AM PST (1 PM EST)
        final now = DateTime.now();
        final futureTime = now.add(const Duration(hours: 3));
        final unixSec = futureTime.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          eventTimezone: 'America/Los_Angeles',
          userTimezone: 'America/New_York',
        );

        // Should show relative time in hours
        expect(result, matches(r'^In [23]hrs$'));
      });

      test('handles null timezones (falls back to local)', () {
        final now = DateTime.now();
        final futureTime = now.add(const Duration(hours: 1, minutes: 59));
        final unixSec = futureTime.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
        );

        // Should show relative time with hours and minutes (< 2 hours)
        expect(result, matches(r'^In 1hr \d+min$'));
      });

      test('formats all-day events without time component', () {
        final now = DateTime.now();
        final tomorrow = now.add(const Duration(days: 1, hours: 12));
        final unixSec = tomorrow.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          isAllDay: true,
        );

        expect(result, 'tomorrow');
      });

      test('formats all-day event as "today"', () {
        // A few minutes in the future keeps difference.inDays == 0 regardless
        // of wall-clock time of day (avoids flakiness near midnight).
        final soon = DateTime.now().add(const Duration(minutes: 5));
        final unixSec = soon.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          isAllDay: true,
        );

        expect(result, 'today');
      });

      test('formats all-day events in days', () {
        final now = DateTime.now();
        final futureTime = now.add(const Duration(days: 3));
        final unixSec = futureTime.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          isAllDay: true,
        );

        expect(result, matches(r'^In [23] days$'));
      });

      test('formats all-day events > 7 days as date', () {
        final now = DateTime.now();
        final futureTime = now.add(const Duration(days: 10));
        final unixSec = futureTime.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          isAllDay: true,
        );

        // Should be formatted as "Mon D"
        expect(result, matches(r'^[A-Z][a-z]{2} \d{1,2}$'));
      });

      test('returns status for past events', () {
        final now = DateTime.now();
        final pastTime = now.subtract(const Duration(hours: 2));
        final unixSec = pastTime.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          status: 'Completed',
        );

        expect(result, 'Completed');
      });

      test('returns custom status for past events', () {
        final now = DateTime.now();
        final pastTime = now.subtract(const Duration(days: 1));
        final unixSec = pastTime.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          status: 'Cancelled',
        );

        expect(result, 'Cancelled');
      });

      test('handles UTC timezone', () {
        final now = DateTime.now();
        final futureTime = now.add(const Duration(hours: 4));
        final unixSec = futureTime.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          eventTimezone: 'UTC',
          userTimezone: 'UTC',
        );

        // Should show relative time
        expect(result, matches(r'^In [34]hrs$'));
      });

      test('formats minutes with timezone conversion', () {
        final now = DateTime.now();
        final futureTime = now.add(const Duration(minutes: 30));
        final unixSec = futureTime.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          eventTimezone: 'America/Los_Angeles',
          userTimezone: 'America/Los_Angeles',
        );

        expect(result, matches(r'^In (29|30)min$'));
      });

      test('formats tomorrow with timezone', () {
        final now = DateTime.now();
        final tomorrow = now.add(const Duration(days: 1, hours: 1));
        final unixSec = tomorrow.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          eventTimezone: 'America/New_York',
          userTimezone: 'America/New_York',
        );

        expect(result, 'tomorrow');
      });

      test('formats multi-day events with timezone', () {
        final now = DateTime.now();
        final futureTime = now.add(const Duration(days: 4));
        final unixSec = futureTime.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          eventTimezone: 'Europe/London',
          userTimezone: 'America/New_York',
        );

        // Could be 3 or 4 days depending on timing
        expect(result, matches(r'^In [34] days$'));
      });

      test('formats events > 7 days with timezone', () {
        final now = DateTime.now();
        final futureTime = now.add(const Duration(days: 14));
        final unixSec = futureTime.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          eventTimezone: 'Asia/Tokyo',
          userTimezone: 'America/Los_Angeles',
        );

        // Should be formatted as "Mon D"
        expect(result, matches(r'^[A-Z][a-z]{2} \d{1,2}$'));
      });

      test('handles event at exact hour boundary', () {
        final now = DateTime.now();
        final exactHour = now.add(const Duration(hours: 1));
        final unixSec = exactHour.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
        );

        // Could be minutes or hours+minutes depending on exact timing
        expect(result, matches(r'^In (59|60)min$|^In 1hr 0min$'));
      });

      test('handles event at exact day boundary', () {
        final now = DateTime.now();
        final exactDay = now.add(const Duration(days: 1, minutes: 1));
        final unixSec = exactDay.millisecondsSinceEpoch ~/ 1000;

        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
        );

        expect(result, 'tomorrow');
      });

      test('handles invalid event timezone gracefully (falls back to local)', () {
        final now = DateTime.now();
        final futureTime = now.add(const Duration(hours: 3));
        final unixSec = futureTime.millisecondsSinceEpoch ~/ 1000;

        // Use invalid timezone abbreviation instead of IANA format
        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          eventTimezone: 'MST', // Invalid - should be "America/Denver"
        );

        // Should still work, falling back to local timezone
        expect(result, matches(r'^In [23]hrs$'));
      });

      test('handles invalid user timezone gracefully (falls back to local)', () {
        final now = DateTime.now();
        final futureTime = now.add(const Duration(hours: 5));
        final unixSec = futureTime.millisecondsSinceEpoch ~/ 1000;

        // Use invalid timezone abbreviation instead of IANA format
        final result = DateTimeFormatter.formatRelativeEventTimeWithTimezone(
          unixSec,
          userTimezone: 'PST', // Invalid - should be "America/Los_Angeles"
        );

        // Should still work, falling back to local timezone
        expect(result, matches(r'^In [45]hrs$'));
      });
    });

    group('formatTimeAgo', () {
      test('returns null for zero (uninitialized timestamp)', () {
        expect(DateTimeFormatter.formatTimeAgo(0), isNull);
      });

      test('returns null for negative timestamp', () {
        expect(DateTimeFormatter.formatTimeAgo(-1), isNull);
      });

      test('returns "Xm ago" for timestamps under one hour', () {
        final fiveMinAgo = DateTime.now()
            .subtract(const Duration(minutes: 5))
            .millisecondsSinceEpoch ~/
            1000;
        expect(DateTimeFormatter.formatTimeAgo(fiveMinAgo), '5m ago');
      });

      test('returns "Xh ago" for timestamps under one day', () {
        final threeHoursAgo = DateTime.now()
            .subtract(const Duration(hours: 3))
            .millisecondsSinceEpoch ~/
            1000;
        expect(DateTimeFormatter.formatTimeAgo(threeHoursAgo), '3h ago');
      });

      test('returns "Xd ago" for timestamps a day or more old', () {
        final tenDaysAgo = DateTime.now()
            .subtract(const Duration(days: 10))
            .millisecondsSinceEpoch ~/
            1000;
        expect(DateTimeFormatter.formatTimeAgo(tenDaysAgo), '10d ago');
      });

      test('caps the unit ladder at months past 30 days (#2724)', () {
        final sixtyFourDaysAgo = DateTime.now()
            .subtract(const Duration(days: 64, hours: 4))
            .millisecondsSinceEpoch ~/
            1000;
        expect(DateTimeFormatter.formatTimeAgo(sixtyFourDaysAgo), '2mo ago');
      });

      test('caps the unit ladder at years past 365 days', () {
        final twoYearsAgo = DateTime.now()
            .subtract(const Duration(days: 740))
            .millisecondsSinceEpoch ~/
            1000;
        expect(DateTimeFormatter.formatTimeAgo(twoYearsAgo), '2y ago');
      });

      test('reads "just now" under a minute — never "0m ago" (#2724)', () {
        final now = DateTime.now().millisecondsSinceEpoch ~/ 1000;
        expect(DateTimeFormatter.formatTimeAgo(now), 'just now');
      });
    });

    group('formatRelativeFromNow', () {
      int secsFromNow(Duration d) =>
          DateTime.now().add(d).millisecondsSinceEpoch ~/ 1000;

      test('returns null for non-positive timestamps', () {
        expect(DateTimeFormatter.formatRelativeFromNow(0), isNull);
        expect(DateTimeFormatter.formatRelativeFromNow(-5), isNull);
      });

      test('shows whole days for multi-day futures — no trailing hours', () {
        final result = DateTimeFormatter.formatRelativeFromNow(
          secsFromNow(const Duration(days: 3, hours: 2, minutes: 30)),
        );
        expect(result, '3d from now');
      });

      test('caps at months past 30 days (#2724)', () {
        expect(
          DateTimeFormatter.formatRelativeFromNow(
            secsFromNow(const Duration(days: -64, hours: -4)),
          ),
          '2 months ago',
        );
        expect(
          DateTimeFormatter.formatRelativeFromNow(
            secsFromNow(const Duration(days: 45)),
          ),
          '1 month from now',
        );
      });

      test('shows hours and minutes for same-day futures', () {
        final result = DateTimeFormatter.formatRelativeFromNow(
          secsFromNow(const Duration(hours: 2, minutes: 15, seconds: 5)),
        );
        expect(result, '2h 15m from now');
      });

      test('shows minutes only when under an hour', () {
        final result = DateTimeFormatter.formatRelativeFromNow(
          secsFromNow(const Duration(minutes: 5, seconds: 5)),
        );
        expect(result, '5m from now');
      });

      test('floors at 1m so an imminent time never reads 0m', () {
        final result = DateTimeFormatter.formatRelativeFromNow(
          secsFromNow(const Duration(seconds: 10)),
        );
        expect(result, '1m from now');
      });

      test('reads "just now" for a past time under a minute (#2724)', () {
        final result = DateTimeFormatter.formatRelativeFromNow(
          secsFromNow(const Duration(seconds: -10)),
        );
        expect(result, 'just now');
      });

      test('uses an "ago" suffix for past timestamps', () {
        final result = DateTimeFormatter.formatRelativeFromNow(
          secsFromNow(const Duration(days: -3, hours: -2)),
        );
        expect(result, '3d ago');
      });
    });

    group('normalizeCompactTimeAgo', () {
      test('nulls sub-minute tokens so callers substitute "just now"', () {
        expect(DateTimeFormatter.normalizeCompactTimeAgo('0m'), isNull);
        expect(DateTimeFormatter.normalizeCompactTimeAgo(''), isNull);
      });

      test('passes ordinary tokens through unchanged', () {
        expect(DateTimeFormatter.normalizeCompactTimeAgo('5m'), '5m');
        expect(DateTimeFormatter.normalizeCompactTimeAgo('2h'), '2h');
        expect(DateTimeFormatter.normalizeCompactTimeAgo('29d'), '29d');
      });

      test('collapses large day counts to months and years (#2724)', () {
        expect(DateTimeFormatter.normalizeCompactTimeAgo('64d'), '2mo');
        expect(DateTimeFormatter.normalizeCompactTimeAgo('400d'), '1y');
      });

      test('passes unrecognized tokens through', () {
        expect(
          DateTimeFormatter.normalizeCompactTimeAgo('yesterday'),
          'yesterday',
        );
      });
    });
  });
}
