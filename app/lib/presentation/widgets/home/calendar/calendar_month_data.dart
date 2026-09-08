import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';

/// CalendarMonthData buckets the server-assembled calendar streams — entries,
/// per-day weather, and open-day suggestions — by local day, so the grid and
/// the day detail can look each up in O(1). The client never invents dates: it
/// groups by the server's `time_unix_sec` / `date_unix_sec` and keys days by
/// local midnight.
class CalendarMonthData {
  final Map<DateTime, List<HomeUpNextEntry>> _entriesByDay;
  final Map<DateTime, DayForecast> _forecastByDay;
  final Map<DateTime, OpenDaySuggestion> _suggestionByDay;

  const CalendarMonthData._(
    this._entriesByDay,
    this._forecastByDay,
    this._suggestionByDay,
  );

  factory CalendarMonthData.from({
    required List<HomeUpNextEntry> entries,
    required List<DayForecast> forecast,
    required List<OpenDaySuggestion> suggestions,
  }) {
    final byDay = <DateTime, List<HomeUpNextEntry>>{};
    for (final e in entries) {
      byDay.putIfAbsent(_dayOfUnix(e.timeUnixSec.toInt()), () => []).add(e);
    }
    // Earliest-first within each day so events[0] is always the marquee.
    for (final list in byDay.values) {
      list.sort((a, b) => a.timeUnixSec.compareTo(b.timeUnixSec));
    }
    final fc = <DateTime, DayForecast>{
      for (final f in forecast) _dayOfUnix(f.dateUnixSec.toInt()): f,
    };
    final sg = <DateTime, OpenDaySuggestion>{
      for (final s in suggestions) _dayOfUnix(s.dateUnixSec.toInt()): s,
    };
    return CalendarMonthData._(byDay, fc, sg);
  }

  static DateTime _dayOfUnix(int unixSec) {
    final w = DateTime.fromMillisecondsSinceEpoch(unixSec * 1000);
    return DateTime(w.year, w.month, w.day);
  }

  /// Day-only key for [day].
  static DateTime dayKey(DateTime day) => DateTime(day.year, day.month, day.day);

  List<HomeUpNextEntry> eventsOn(DateTime day) =>
      _entriesByDay[dayKey(day)] ?? const <HomeUpNextEntry>[];

  DayForecast? forecastOn(DateTime day) => _forecastByDay[dayKey(day)];

  OpenDaySuggestion? suggestionOn(DateTime day) => _suggestionByDay[dayKey(day)];

  /// The soonest day on or after [from] that has at least one event, or null
  /// when nothing is scheduled from that day forward. Lets a calendar open on
  /// the next planned day instead of an empty "today" (#2675).
  DateTime? firstEventDayOnOrAfter(DateTime from) {
    final fromKey = dayKey(from);
    DateTime? soonest;
    for (final day in _entriesByDay.keys) {
      if (day.isBefore(fromKey)) continue;
      if (soonest == null || day.isBefore(soonest)) soonest = day;
    }
    return soonest;
  }

  /// The media ID to use as the day's photo (cell thumbnail and backdrop): the
  /// first event of the day that carries one. Empty when the day has no photo.
  String marqueePhotoOn(DateTime day) {
    for (final e in eventsOn(day)) {
      if (e.thumbnailMediaId.isNotEmpty) return e.thumbnailMediaId;
    }
    return '';
  }
}
