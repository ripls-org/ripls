import 'package:flutter/widgets.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';

/// Weather glyph + localized label helpers for the calendar. Weather is the
/// calendar's substrate — a glyph in every cell — so these live in one place
/// shared by the grid cells and the day detail. The server sends a coarse
/// [DayForecastCondition] and a pre-formatted temperature string; the client
/// only maps the condition to a glyph and a localized name (the raw emoji is
/// never the announced content — see [calendarConditionLabel]).

/// The emoji glyph for a coarse condition, mirroring the design mockup's
/// five-icon scheme. Empty for an unspecified condition.
String calendarConditionGlyph(DayForecastCondition condition) {
  switch (condition) {
    case DayForecastCondition.DAY_FORECAST_CONDITION_CLEAR:
      return '☀️';
    case DayForecastCondition.DAY_FORECAST_CONDITION_MOSTLY_SUNNY:
      return '🌤️';
    case DayForecastCondition.DAY_FORECAST_CONDITION_PARTLY_CLOUDY:
      return '⛅';
    case DayForecastCondition.DAY_FORECAST_CONDITION_OVERCAST:
      return '☁️';
    case DayForecastCondition.DAY_FORECAST_CONDITION_RAIN:
      return '🌧️';
    default:
      return '';
  }
}

/// The localized human label for a condition, used in accessibility
/// announcements so a screen reader says "Partly cloudy" rather than reading the
/// emoji codepoint.
String calendarConditionLabel(
    BuildContext context, DayForecastCondition condition) {
  final l10n = context.l10n;
  switch (condition) {
    case DayForecastCondition.DAY_FORECAST_CONDITION_CLEAR:
      return l10n.homeCalConditionClear;
    case DayForecastCondition.DAY_FORECAST_CONDITION_MOSTLY_SUNNY:
      return l10n.homeCalConditionMostlySunny;
    case DayForecastCondition.DAY_FORECAST_CONDITION_PARTLY_CLOUDY:
      return l10n.homeCalConditionPartlyCloudy;
    case DayForecastCondition.DAY_FORECAST_CONDITION_OVERCAST:
      return l10n.homeCalConditionOvercast;
    case DayForecastCondition.DAY_FORECAST_CONDITION_RAIN:
      return l10n.homeCalConditionRain;
    default:
      return '';
  }
}

/// A localized accessibility phrase combining condition and temperature, e.g.
/// "Clear, 72°". Returns the temperature alone when the condition is unknown,
/// and an empty string when there is nothing to announce.
String calendarWeatherSemantic(BuildContext context, DayForecast? forecast) {
  if (forecast == null) return '';
  final label = calendarConditionLabel(context, forecast.condition);
  final temp = forecast.temperatureDisplay;
  if (label.isEmpty) return temp;
  if (temp.isEmpty) return label;
  return context.l10n.homeCalWeatherSemantic(label, temp);
}
