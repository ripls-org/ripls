import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/widgets/impact/completion/completion_impact_bar.dart';

import '../../../../helpers/l10n_helpers.dart';

ImpactEstimate _impact({
  double moneyUsd = 0,
  double qtMinutes = 0,
  double co2Grams = 0,
}) {
  return ImpactEstimate(
    moneySaved: moneyUsd > 0
        ? MoneySavings(valueUsd: Estimate(mean: moneyUsd))
        : null,
    qualityTime: qtMinutes > 0
        ? QualityTimeEstimate(qualityTimeMinutes: Estimate(mean: qtMinutes))
        : null,
    emissionsPrevented: co2Grams > 0
        ? PreventedEmissions(
            manufactureAvoidedCarbon: CarbonEstimate(
              co2eGrams: Estimate(mean: co2Grams),
            ),
          )
        : null,
  );
}

Widget _bar({ImpactEstimate? impact, bool isLoading = false}) {
  return localizedApp(
    Scaffold(
      body: CompletionImpactBar(
        impact: impact,
        isLoading: isLoading,
        onMoneyTap: () {},
        onQualityTimeTap: () {},
        onCo2Tap: () {},
      ),
    ),
  );
}

void main() {
  group('CompletionImpactBar', () {
    testWidgets('renders all non-zero metrics with humanized quality time', (
      tester,
    ) async {
      await tester.pumpWidget(
        _bar(impact: _impact(moneyUsd: 80, qtMinutes: 237, co2Grams: 19000)),
      );

      expect(find.text('\$80'), findsOneWidget);
      // 237 minutes must never render as raw minutes (#2724).
      expect(find.text('~4 h'), findsOneWidget);
      // The same quantity reads the same here as on the item page it commits
      // to — both go through the canonical compact formatter (#2724).
      expect(find.text('19kg'), findsOneWidget);
      expect(find.text('Saved'), findsOneWidget);
      expect(find.text('Quality Time'), findsOneWidget);
      expect(find.text('CO₂'), findsOneWidget);
    });

    testWidgets('suppresses zero metrics and keeps the non-zero ones', (
      tester,
    ) async {
      await tester.pumpWidget(_bar(impact: _impact(qtMinutes: 49)));

      // #2724 — never headline "$0 Saved" or a zero CO₂ tile.
      expect(find.text('\$0'), findsNothing);
      expect(find.text('Saved'), findsNothing);
      expect(find.text('CO₂'), findsNothing);
      expect(find.text('49 min'), findsOneWidget);
      expect(find.text('Quality Time'), findsOneWidget);
    });

    testWidgets('hides entirely when every metric is zero', (tester) async {
      await tester.pumpWidget(_bar(impact: _impact()));

      expect(find.text('Saved'), findsNothing);
      expect(find.text('Quality Time'), findsNothing);
      expect(find.text('CO₂'), findsNothing);
      expect(find.byType(CircularProgressIndicator), findsNothing);
    });

    testWidgets('drops a CO₂ tile too small to be worth a headline', (
      tester,
    ) async {
      await tester.pumpWidget(
        _bar(impact: _impact(moneyUsd: 12, co2Grams: 30)),
      );

      expect(find.text('\$12'), findsOneWidget);
      expect(find.text('30g'), findsNothing);
      expect(find.text('CO₂'), findsNothing);
    });

    testWidgets('shows three spinner placeholders while loading', (
      tester,
    ) async {
      await tester.pumpWidget(_bar(impact: null, isLoading: true));

      expect(find.byType(CircularProgressIndicator), findsNWidgets(3));
      expect(find.text('Saved'), findsOneWidget);
      expect(find.text('Quality Time'), findsOneWidget);
      expect(find.text('CO₂'), findsOneWidget);
    });

    testWidgets('hides when not loading and impact is missing', (
      tester,
    ) async {
      await tester.pumpWidget(_bar(impact: null, isLoading: false));

      expect(find.text('Saved'), findsNothing);
      expect(find.byType(CircularProgressIndicator), findsNothing);
    });
  });
}
