import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show DayForecast, HourForecast;
import 'package:ripls/presentation/widgets/home/calendar/calendar_weather.dart';

/// ExperienceWhenWeatherStrip is the hour-by-hour weather row on the event
/// "When" screen (event-when-agenda design). It paints a short window of hours
/// — the cells inside the event's time window are accented so the host can read
/// the weather "around your window" at a glance. When [hours] is empty (the day
/// is beyond the forecast horizon) it shows [unavailableText] instead.
///
/// The caller windows [hours] to a handful of entries and supplies the
/// highlight bounds; this widget is presentation-only.
class ExperienceWhenWeatherStrip extends StatelessWidget {
  /// Uppercase-styled section label, e.g. "Around your window · 2–3 PM".
  final String label;

  /// The hours to render (already sliced to a short window by the caller).
  final List<HourForecast> hours;

  /// Inclusive-start / exclusive-end of the event window (Unix seconds); cells
  /// whose hour falls inside are accented. Null when exploring another day.
  final int? windowStartUnixSec;
  final int? windowEndUnixSec;

  /// True while the forecast for the viewed day is still loading.
  final bool loading;

  /// Shown when there are no hours and we're not loading.
  final String unavailableText;

  /// Coarse daily forecast for the day — rendered as a single summary when no
  /// hourly entries exist (e.g. a day beyond the ~16-day hourly horizon, where
  /// the daily climate-normal still has a value). Null falls through to
  /// [unavailableText].
  final DayForecast? fallback;

  const ExperienceWhenWeatherStrip({
    super.key,
    required this.label,
    required this.hours,
    required this.unavailableText,
    this.windowStartUnixSec,
    this.windowEndUnixSec,
    this.loading = false,
    this.fallback,
  });

  bool _inWindow(int hourUnixSec) {
    final start = windowStartUnixSec;
    final end = windowEndUnixSec;
    if (start == null || end == null) return false;
    return hourUnixSec >= start && hourUnixSec < end;
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label.toUpperCase(),
          style: const TextStyle(
            fontSize: 10,
            fontWeight: FontWeight.w800,
            letterSpacing: 0.8,
            color: AppColors.darkTextTertiary,
          ),
        ),
        const SizedBox(height: 8),
        if (loading)
          const SizedBox(
            height: 56,
            child: Center(
              child: SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2),
              ),
            ),
          )
        else if (hours.isEmpty && fallback != null)
          _dailyFallback(context, fallback!)
        else if (hours.isEmpty)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 4),
            child: Text(
              unavailableText,
              style: const TextStyle(
                fontSize: 12.5,
                fontStyle: FontStyle.italic,
                color: AppColors.darkTextSecondary,
              ),
            ),
          )
        else
          Row(
            children: [
              for (final h in hours) ...[
                Expanded(child: _hourCell(h)),
                if (h != hours.last) const SizedBox(width: 5),
              ],
            ],
          ),
      ],
    );
  }

  // A single coarse daily summary, shown when no hourly forecast is available
  // for the day (typically beyond the hourly horizon). The glyph is decorative;
  // the summary text carries the meaning.
  Widget _dailyFallback(BuildContext context, DayForecast f) {
    final glyph = calendarConditionGlyph(f.condition);
    final summary = f.summary.isNotEmpty ? f.summary : f.temperatureDisplay;
    final text = f.isTypical ? '$summary · ${context.l10n.homeCalTypical}' : summary;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: GlassTokens.fillFaint,
        borderRadius: BorderRadius.circular(11),
      ),
      child: Row(
        children: [
          if (glyph.isNotEmpty) ...[
            Text(glyph, style: const TextStyle(fontSize: 18)),
            const SizedBox(width: 8),
          ],
          Expanded(
            child: Text(
              text,
              style: const TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w600,
                color: AppColors.onContentImage,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _hourCell(HourForecast h) {
    final win = _inWindow(h.timeUnixSec.toInt());
    final dt =
        DateTime.fromMillisecondsSinceEpoch(h.timeUnixSec.toInt() * 1000);
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 7),
      decoration: BoxDecoration(
        color: win
            ? AppColors.experienceSageGreen.withValues(alpha: 0.22)
            : GlassTokens.fillFaint,
        borderRadius: BorderRadius.circular(11),
        border: win
            ? Border.all(
                color: AppColors.experienceSageGreen.withValues(alpha: 0.55))
            : null,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            _hourLabel(dt),
            style: TextStyle(
              fontSize: 9.5,
              fontWeight: FontWeight.w700,
              color: win
                  ? AppColors.experienceSageGreen
                  : AppColors.darkTextSecondary,
            ),
          ),
          const SizedBox(height: 4),
          Text(calendarConditionGlyph(h.condition),
              style: const TextStyle(fontSize: 15, height: 1)),
          const SizedBox(height: 4),
          Text(
            h.temperatureDisplay,
            style: const TextStyle(
              fontSize: 11.5,
              fontWeight: FontWeight.w800,
              color: AppColors.onContentImage,
            ),
          ),
        ],
      ),
    );
  }

  // Compact hour label like "12p", "1p" — matches the agenda design.
  String _hourLabel(DateTime dt) {
    final h = int.parse(DateFormat('h').format(dt));
    return '$h${dt.hour < 12 ? 'a' : 'p'}';
  }
}
