import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_month_data.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_weather.dart';

void main() {
  // 2026-06-20 08:00 and 14:00 local-ish (use a fixed day).
  final day = DateTime(2026, 6, 20);
  int unix(DateTime d) => d.millisecondsSinceEpoch ~/ 1000;

  HomeUpNextEntry entry(DateTime when, {String photo = '', String title = 't'}) =>
      HomeUpNextEntry()
        ..timeUnixSec = Int64(unix(when))
        ..title = title
        ..thumbnailMediaId = photo;

  group('CalendarMonthData', () {
    test('buckets entries by day and sorts earliest-first', () {
      final late = entry(DateTime(2026, 6, 20, 17, 0), title: 'late');
      final early = entry(DateTime(2026, 6, 20, 8, 0), title: 'early');
      final other = entry(DateTime(2026, 6, 21, 9, 0), title: 'other');

      final data = CalendarMonthData.from(
        entries: [late, early, other],
        forecast: const [],
        suggestions: const [],
      );

      final events = data.eventsOn(day);
      expect(events.map((e) => e.title), ['early', 'late']);
      expect(data.eventsOn(DateTime(2026, 6, 21)).single.title, 'other');
      expect(data.eventsOn(DateTime(2026, 6, 22)), isEmpty);
    });

    test('marqueePhotoOn picks the first event with a photo', () {
      final noPhoto = entry(DateTime(2026, 6, 20, 8, 0), title: 'a');
      final withPhoto =
          entry(DateTime(2026, 6, 20, 10, 0), photo: 'm1', title: 'b');

      final data = CalendarMonthData.from(
        entries: [noPhoto, withPhoto],
        forecast: const [],
        suggestions: const [],
      );

      expect(data.marqueePhotoOn(day), 'm1');
      expect(data.marqueePhotoOn(DateTime(2026, 6, 22)), '');
    });

    test('firstEventDayOnOrAfter finds the soonest upcoming event day', () {
      final soon = entry(DateTime(2026, 6, 21, 9, 0), title: 'soon');
      final later = entry(DateTime(2026, 8, 21, 15, 0), title: 'later');
      final past = entry(DateTime(2026, 6, 10, 9, 0), title: 'past');

      final data = CalendarMonthData.from(
        entries: [later, past, soon],
        forecast: const [],
        suggestions: const [],
      );

      // From today (2026-06-20), the soonest event day is 2026-06-21 (past
      // events are skipped).
      expect(data.firstEventDayOnOrAfter(day), DateTime(2026, 6, 21));
      // From after the last event, nothing remains.
      expect(data.firstEventDayOnOrAfter(DateTime(2026, 9, 1)), isNull);
      // A community whose only event is weeks out lands on that day.
      final onlyFar = CalendarMonthData.from(
        entries: [later],
        forecast: const [],
        suggestions: const [],
      );
      expect(onlyFar.firstEventDayOnOrAfter(day), DateTime(2026, 8, 21));
    });

    test('forecastOn and suggestionOn look up by local day', () {
      final fc = DayForecast()
        ..dateUnixSec = Int64(unix(DateTime(2026, 6, 20)))
        ..condition = DayForecastCondition.DAY_FORECAST_CONDITION_CLEAR
        ..temperatureDisplay = '72°';
      final sg = OpenDaySuggestion()
        ..dateUnixSec = Int64(unix(DateTime(2026, 6, 20)))
        ..title = 'A hike?';

      final data = CalendarMonthData.from(
        entries: const [],
        forecast: [fc],
        suggestions: [sg],
      );

      expect(data.forecastOn(day)?.temperatureDisplay, '72°');
      expect(data.suggestionOn(day)?.title, 'A hike?');
      expect(data.forecastOn(DateTime(2026, 6, 21)), isNull);
    });
  });

  group('calendarConditionGlyph', () {
    test('maps each condition to its glyph', () {
      expect(
          calendarConditionGlyph(
              DayForecastCondition.DAY_FORECAST_CONDITION_CLEAR),
          '☀️');
      expect(
          calendarConditionGlyph(
              DayForecastCondition.DAY_FORECAST_CONDITION_RAIN),
          '🌧️');
      expect(
          calendarConditionGlyph(
              DayForecastCondition.DAY_FORECAST_CONDITION_UNSPECIFIED),
          '');
    });
  });
}
