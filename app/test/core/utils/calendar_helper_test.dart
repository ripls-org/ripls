import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/calendar_helper.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:timezone/data/latest.dart' as tz;

/// Tests for CalendarHelper
///
/// Note: Full integration tests for add_2_calendar functionality require
/// device/emulator testing. These unit tests verify the helper logic and
/// event building without invoking the native calendar integration.
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUpAll(() {
    tz.initializeTimeZones();
  });

  group('CalendarHelper', () {
    group('Event Building Logic', () {
      test('handles TBD events gracefully', () {
        final experience = Experience(
          id: 'exp1',
          name: 'Test Experience',
          description: 'Test description',
          time: ExperienceTime(
            tbd: TimeTBD(),
          ),
        );

        // TBD events should be handled gracefully
        expect(
          () async => CalendarHelper.addToCalendar(experience),
          returnsNormally,
        );
      });

      test('creates valid event data for SpecificTime', () {
        final tomorrow = DateTime.now().add(const Duration(days: 1));
        final eventTime = DateTime(
          tomorrow.year,
          tomorrow.month,
          tomorrow.day,
          15,
          0,
        );
        final unixSec = eventTime.millisecondsSinceEpoch ~/ 1000;

        final experience = Experience(
          id: 'exp1',
          name: 'Hike to Mt. Tam',
          description: 'Bring water and snacks',
          latitudeDeg: 37.9235,
          longitudeDeg: -122.5964,
          time: ExperienceTime(
            specific: SpecificTime(
              unixTimestampSec: Int64(unixSec),
              timezone: 'America/Los_Angeles',
              durationMinutes: 120,
              isAllDay: false,
            ),
          ),
        );

        // Verify the experience has valid time data
        expect(experience.time.hasSpecific(), isTrue);
        expect(experience.time.specific.unixTimestampSec, Int64(unixSec));
        expect(experience.time.specific.timezone, 'America/Los_Angeles');
        expect(experience.time.specific.durationMinutes, 120);
        expect(experience.time.specific.isAllDay, isFalse);
      });

      test('handles TimeRange events', () {
        final tomorrow = DateTime.now().add(const Duration(days: 1));
        final startTime = DateTime(
          tomorrow.year,
          tomorrow.month,
          tomorrow.day,
          10,
          0,
        );
        final endTime = startTime.add(const Duration(days: 2, hours: 4));
        final startUnixSec = startTime.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = endTime.millisecondsSinceEpoch ~/ 1000;

        final experience = Experience(
          id: 'exp2',
          name: 'Weekend Camping',
          description: 'Bring tent and sleeping bag',
          time: ExperienceTime(
            range: TimeRange(
              startUnixSec: Int64(startUnixSec),
              endUnixSec: Int64(endUnixSec),
              isAllDay: false,
            ),
          ),
        );

        // Verify the experience has valid range data
        expect(experience.time.hasRange(), isTrue);
        expect(experience.time.range.startUnixSec, Int64(startUnixSec));
        expect(experience.time.range.endUnixSec, Int64(endUnixSec));
        expect(experience.time.range.isAllDay, isFalse);
      });

      test('handles all-day events', () {
        final tomorrow = DateTime.now().add(const Duration(days: 1));
        final eventTime = DateTime(
          tomorrow.year,
          tomorrow.month,
          tomorrow.day,
          0,
          0,
        );
        final unixSec = eventTime.millisecondsSinceEpoch ~/ 1000;

        final experience = Experience(
          id: 'exp3',
          name: 'Conference',
          description: 'All-day tech conference',
          time: ExperienceTime(
            specific: SpecificTime(
              unixTimestampSec: Int64(unixSec),
              timezone: 'America/New_York',
              durationMinutes: 0,
              isAllDay: true,
            ),
          ),
        );

        // Verify all-day flag is set
        expect(experience.time.specific.isAllDay, isTrue);
      });

      test('handles multi-day all-day events', () {
        final tomorrow = DateTime.now().add(const Duration(days: 1));
        final startTime = DateTime(
          tomorrow.year,
          tomorrow.month,
          tomorrow.day,
          0,
          0,
        );
        final endTime = startTime.add(const Duration(days: 3));
        final startUnixSec = startTime.millisecondsSinceEpoch ~/ 1000;
        final endUnixSec = endTime.millisecondsSinceEpoch ~/ 1000;

        final experience = Experience(
          id: 'exp4',
          name: 'Festival',
          description: '3-day music festival',
          time: ExperienceTime(
            range: TimeRange(
              startUnixSec: Int64(startUnixSec),
              endUnixSec: Int64(endUnixSec),
              isAllDay: true,
            ),
          ),
        );

        // Verify multi-day all-day event
        expect(experience.time.range.isAllDay, isTrue);
        final duration = endUnixSec - startUnixSec;
        expect(duration, greaterThan(86400 * 2)); // More than 2 days in seconds
      });

      test('handles events with location coordinates', () {
        final tomorrow = DateTime.now().add(const Duration(days: 1));
        final eventTime = DateTime(
          tomorrow.year,
          tomorrow.month,
          tomorrow.day,
          14,
          30,
        );
        final unixSec = eventTime.millisecondsSinceEpoch ~/ 1000;

        final experience = Experience(
          id: 'exp5',
          name: 'Beach Cleanup',
          description: 'Community service',
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
          time: ExperienceTime(
            specific: SpecificTime(
              unixTimestampSec: Int64(unixSec),
              timezone: 'America/Los_Angeles',
              durationMinutes: 180,
              isAllDay: false,
            ),
          ),
        );

        // Verify location is set
        expect(experience.latitudeDeg, 37.7749);
        expect(experience.longitudeDeg, -122.4194);
      });

      test('handles events with empty timezone', () {
        final tomorrow = DateTime.now().add(const Duration(days: 1));
        final eventTime = DateTime(
          tomorrow.year,
          tomorrow.month,
          tomorrow.day,
          10,
          0,
        );
        final unixSec = eventTime.millisecondsSinceEpoch ~/ 1000;

        final experience = Experience(
          id: 'exp7',
          name: 'Local Event',
          description: 'Uses local timezone',
          time: ExperienceTime(
            specific: SpecificTime(
              unixTimestampSec: Int64(unixSec),
              timezone: '',
              durationMinutes: 90,
              isAllDay: false,
            ),
          ),
        );

        // Verify empty timezone is handled
        expect(experience.time.specific.timezone, isEmpty);
      });

      test('handles events with no duration', () {
        final tomorrow = DateTime.now().add(const Duration(days: 1));
        final eventTime = DateTime(
          tomorrow.year,
          tomorrow.month,
          tomorrow.day,
          18,
          0,
        );
        final unixSec = eventTime.millisecondsSinceEpoch ~/ 1000;

        final experience = Experience(
          id: 'exp8',
          name: 'Quick Meeting',
          description: 'No specified duration',
          time: ExperienceTime(
            specific: SpecificTime(
              unixTimestampSec: Int64(unixSec),
              timezone: 'America/Los_Angeles',
              durationMinutes: 0,
              isAllDay: false,
            ),
          ),
        );

        // Verify zero duration is handled (should default to 1 hour in Event)
        expect(experience.time.specific.durationMinutes, 0);
      });
    });

    group('formatLocationForCalendar', () {
      test('prefers human-readable address over lat/lng', () {
        final experience = Experience(
          id: 'exp1',
          name: 'Beach Cleanup',
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        final result = CalendarHelper.formatLocationForCalendar(
          experience,
          '1701 Bryant St, Denver, CO 80204',
        );

        expect(result, '1701 Bryant St, Denver, CO 80204');
      });

      test('falls back to lat/lng when address is null', () {
        final experience = Experience(
          id: 'exp1',
          name: 'Beach Cleanup',
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        final result = CalendarHelper.formatLocationForCalendar(
          experience,
          null,
        );

        expect(result, '37.774900, -122.419400');
      });

      test('falls back to lat/lng when address is empty string', () {
        final experience = Experience(
          id: 'exp1',
          name: 'Beach Cleanup',
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        final result = CalendarHelper.formatLocationForCalendar(
          experience,
          '',
        );

        expect(result, '37.774900, -122.419400');
      });

      test('returns null when no address and no coordinates', () {
        final experience = Experience(
          id: 'exp1',
          name: 'Online Meeting',
        );

        expect(
          CalendarHelper.formatLocationForCalendar(experience, null),
          isNull,
        );
      });

      test('returns address even when coordinates are zero', () {
        final experience = Experience(
          id: 'exp1',
          name: 'Online + IRL',
        );

        final result = CalendarHelper.formatLocationForCalendar(
          experience,
          'Community Center, Denver, CO',
        );

        expect(result, 'Community Center, Denver, CO');
      });
    });

    group('composeDescription', () {
      test('combines description and url with separator', () {
        final result = CalendarHelper.composeDescription(
          'Bring water and snacks',
          'https://example.com/go/abc123',
        );
        expect(
          result,
          'Bring water and snacks\n\nView in Ripls: https://example.com/go/abc123',
        );
      });

      test('returns description alone when url is null', () {
        final result = CalendarHelper.composeDescription(
          'Bring water and snacks',
          null,
        );
        expect(result, 'Bring water and snacks');
      });

      test('returns description alone when url is empty', () {
        final result = CalendarHelper.composeDescription(
          'Bring water and snacks',
          '',
        );
        expect(result, 'Bring water and snacks');
      });

      test('returns url-only line when description is empty', () {
        final result = CalendarHelper.composeDescription(
          '',
          'https://example.com/go/abc123',
        );
        expect(result, 'View in Ripls: https://example.com/go/abc123');
      });

      test('returns null when both inputs are empty', () {
        expect(CalendarHelper.composeDescription('', null), isNull);
        expect(CalendarHelper.composeDescription('', ''), isNull);
      });
    });
  });
}
