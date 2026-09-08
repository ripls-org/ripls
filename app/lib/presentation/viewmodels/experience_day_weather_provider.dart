import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/services/experience_service.dart'
    show DayForecast, HourForecast;
import 'package:ripls/services/providers/experience_providers.dart';

/// Family key for [experienceDayWeatherProvider]: the experience plus the local
/// day (Unix seconds at local midnight) to forecast. A Dart record gives value
/// equality for free, so the same (experience, day) pair reuses one fetch.
typedef DayWeatherKey = ({String experienceId, int dayUnixSec});

/// The weather for one day on the "When" screen: the hour-by-hour forecast plus
/// a coarse daily fallback. [hours] is empty beyond the ~16-day hourly horizon;
/// [dayForecast] still fills from climate normals there, so the strip can show
/// the day's weather even when no hourly is available.
class DayWeatherResult {
  final List<HourForecast> hours;
  final DayForecast? dayForecast;
  const DayWeatherResult({required this.hours, this.dayForecast});
}

/// Weather at an experience's location for one local day — the "When" screen
/// weather strip. Lazy per viewed day and auto-cached by the family key; the
/// server also caches per place, so tapping around the calendar is cheap. Both
/// fields are empty/null only when weather is genuinely unavailable (no
/// coordinates, or the day is in the past).
final experienceDayWeatherProvider =
    FutureProvider.family<DayWeatherResult, DayWeatherKey>((ref, key) async {
  final service = ref.watch(experienceServiceProvider);
  final resp = await service.getExperienceDayWeather(
    experienceId: key.experienceId,
    dateUnixSec: Int64(key.dayUnixSec),
  );
  return DayWeatherResult(
    hours: resp.hours,
    dayForecast: resp.hasDayForecast() ? resp.dayForecast : null,
  );
});

/// Daily forecast across the "When" calendar window, all at the experience's own
/// location — so the month grid paints weather for where the event is, not the
/// viewer's home. Keyed by experience id and cached for the session; the server
/// caches per place, so this is a single cheap fetch per experience.
final experienceCalendarWeatherProvider =
    FutureProvider.family<List<DayForecast>, String>((ref, experienceId) async {
  final service = ref.watch(experienceServiceProvider);
  return service.getExperienceCalendarWeather(experienceId);
});
