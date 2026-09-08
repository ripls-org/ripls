import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_day_detail.dart';
import 'package:ripls/presentation/widgets/weather/weather_chip.dart';

// The open-day nudge card and its weather-weave are gone (#2936): an open day
// is already the whole message, so it falls through to the deterministic
// "plan an event" prompt. The weather×history suggestion panel stays — it is
// built from real forecast and activity data, not written by a model.
void main() {
  final today = DateTime(2026, 7, 10);

  DayForecast forecast() => DayForecast(
        condition: DayForecastCondition.DAY_FORECAST_CONDITION_CLEAR,
        temperatureDisplay: '72°',
        summary: 'Sunny',
      );

  Future<void> pump(
    WidgetTester tester, {
    OpenDaySuggestion? withSuggestion,
  }) {
    return tester.pumpWidget(
      ProviderScope(
        child: MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: Scaffold(
            // Mirror the real host: the detail sits bottom-aligned in a
            // scroll view under the month grid.
            body: SingleChildScrollView(
              child: CalendarDayDetail(
                day: today,
                today: today,
                events: const [],
                forecast: forecast(),
                suggestion: withSuggestion,
                onEntryTap: (_) {},
                onSuggest: () {},
                onGeneratePlan: (_, _) {},
              ),
            ),
          ),
        ),
      ),
    );
  }

  testWidgets(
      'suggestion day shows the server-woven reason with no separate '
      'weather chip or kicker weather', (tester) async {
    const reason =
        "Clear and 72° — you've done trail run 3× before, mostly on days like this.";
    await pump(
      tester,
      withSuggestion: OpenDaySuggestion(
        dateUnixSec: Int64(today.millisecondsSinceEpoch ~/ 1000),
        title: 'Trail run?',
        reason: reason,
      ),
    );

    expect(find.byType(WeatherChip), findsNothing);
    expect(find.text(reason), findsOneWidget);
    // The kicker carries no weather — it would otherwise repeat the 72°.
    expect(find.textContaining('72°'), findsOneWidget);
    expect(find.text('SUGGESTED · FRI 10'), findsOneWidget);
  });

  testWidgets(
      'an open day with no suggestion shows the weather pill above the '
      'plan prompt', (tester) async {
    await pump(tester);
    expect(find.byType(WeatherChip), findsOneWidget);
    expect(find.text('Nothing planned yet'), findsOneWidget);
  });
}
