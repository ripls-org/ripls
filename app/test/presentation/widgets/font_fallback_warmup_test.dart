import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show DayForecastCondition;
import 'package:ripls/presentation/widgets/font_fallback_warmup.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_weather.dart';

void main() {
  group('FontFallbackWarmup', () {
    test('warmup glyphs include the filmed tofu regressions (#2724)', () {
      // Subscript two from "CO₂" labels (event completion modal et al.).
      expect(FontFallbackWarmup.warmupGlyphs, contains('₂'));
      // The partly-cloudy glyph filmed as tofu on the event weather line.
      // Its emoji subset font also covers the other calendar glyphs.
      expect(
        FontFallbackWarmup.warmupGlyphs,
        contains(calendarConditionGlyph(
            DayForecastCondition.DAY_FORECAST_CONDITION_PARTLY_CLOUDY)),
      );
    });

    testWidgets('renders nothing on non-web platforms',
        (WidgetTester tester) async {
      await tester.pumpWidget(const FontFallbackWarmup());
      // VM widget tests run with kIsWeb == false, where the warmup is a
      // no-op: platform fonts already cover emoji and symbols.
      expect(find.byType(Text), findsNothing);
      expect(find.byType(Offstage), findsNothing);
    });
  });
}
