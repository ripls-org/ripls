import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_weather.dart';

/// WeatherChip is the small pill showing a day's weather — a glyph plus a short
/// summary (or temperature). The clear-sky variant uses the warm accent; every
/// other condition uses a translucent "glass" pill, so it reads over a photo
/// backdrop. Shared by the Home calendar day detail and the event view. The
/// raw emoji is decorative; the chip carries a localized weather accessibility
/// label.
class WeatherChip extends StatelessWidget {
  final DayForecast forecast;

  const WeatherChip({super.key, required this.forecast});

  @override
  Widget build(BuildContext context) {
    final f = forecast;
    final isClear =
        f.condition == DayForecastCondition.DAY_FORECAST_CONDITION_CLEAR;
    final glyph = calendarConditionGlyph(f.condition);
    final text = f.summary.isNotEmpty ? f.summary : f.temperatureDisplay;
    return Semantics(
      label: calendarWeatherSemantic(context, f),
      container: true,
      child: ExcludeSemantics(
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 11, vertical: 6),
          decoration: BoxDecoration(
            color: isClear
                ? AppColors.statusWarningOnDark
                : OverlayTokens.chipFill,
            borderRadius: BorderRadius.circular(999),
            border:
                isClear ? null : Border.all(color: OverlayTokens.outline),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (glyph.isNotEmpty)
                Text(glyph, style: const TextStyle(fontSize: 20)),
              if (glyph.isNotEmpty) const SizedBox(width: 6),
              Text(
                f.isTypical ? '$text · ${context.l10n.homeCalTypical}' : text,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  color: isClear
                      ? const Color(0xFF3A2C08)
                      : OverlayTokens.textPrimary,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
