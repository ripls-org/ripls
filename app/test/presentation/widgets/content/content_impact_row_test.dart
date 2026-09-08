import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/widgets/content/content_impact_row.dart';

import '../../../helpers/l10n_helpers.dart';

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

Widget _host(Widget? row) {
  return localizedApp(
    Scaffold(
      backgroundColor: Colors.black,
      body: row ?? const SizedBox.shrink(),
    ),
  );
}

void main() {
  group('ContentImpactRow', () {
    testWidgets('labels each pill with value + unit meaning', (tester) async {
      await tester.pumpWidget(
        _host(
          ContentImpactRow.build(
            context: await _context(tester),
            impact: _impact(moneyUsd: 210, qtMinutes: 237, co2Grams: 40000),
          ),
        ),
      );

      // Bare "$210 · 37 mins · 40kg" chips were uninterpretable (#2724).
      expect(find.text('\$210 saved'), findsOneWidget);
      expect(find.text('~4 h together'), findsOneWidget);
      expect(find.text('40kg CO₂'), findsOneWidget);
    });

    testWidgets('renders every pill label in full at phone width', (
      tester,
    ) async {
      tester.view.physicalSize = const Size(390, 844);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.reset);

      await tester.pumpWidget(
        _host(
          ContentImpactRow.build(
            context: await _context(tester),
            // 37 min is the longest label case ("37 min together"); equal
            // Expanded thirds ellipsized it to "37 min to…" (#2724).
            impact: _impact(moneyUsd: 210, qtMinutes: 37, co2Grams: 40000),
          ),
        ),
      );

      for (final label in ['\$210 saved', '37 min together', '40kg CO₂']) {
        final text = tester.widget<Text>(find.text(label));
        final painter = TextPainter(
          text: TextSpan(text: text.data, style: text.style),
          maxLines: 1,
          textDirection: TextDirection.ltr,
        )..layout(maxWidth: tester.getSize(find.text(label)).width);
        expect(
          painter.didExceedMaxLines,
          isFalse,
          reason: '"$label" is truncated in its pill',
        );
      }
    });

    testWidgets('keeps unlabeled dash for empty metrics', (tester) async {
      await tester.pumpWidget(
        _host(
          ContentImpactRow.build(
            context: await _context(tester),
            impact: _impact(qtMinutes: 49),
          ),
        ),
      );

      expect(find.text('49 min together'), findsOneWidget);
      expect(find.text('–'), findsNWidgets(2));
      expect(find.textContaining('saved'), findsNothing);
      expect(find.textContaining('CO₂'), findsNothing);
    });

    testWidgets('returns null for empty impact', (tester) async {
      final row = ContentImpactRow.build(
        context: await _context(tester),
        impact: ImpactEstimate(),
      );
      expect(row, isNull);

      final noImpact = ContentImpactRow.build(
        context: await _context(tester),
        impact: null,
      );
      expect(noImpact, isNull);
    });

    testWidgets('onViewImpact makes the row tappable with a chevron', (
      tester,
    ) async {
      var opened = false;
      await tester.pumpWidget(
        _host(
          ContentImpactRow.build(
            context: await _context(tester),
            impact: _impact(moneyUsd: 210, qtMinutes: 237, co2Grams: 40000),
            onViewImpact: () => opened = true,
          ),
        ),
      );

      // Inline entry point to the receipt (#2724).
      expect(find.byIcon(Icons.chevron_right), findsOneWidget);
      await tester.tap(find.byIcon(Icons.insights));
      expect(opened, isTrue);
    });

    testWidgets('no chevron and no row tap without onViewImpact', (
      tester,
    ) async {
      await tester.pumpWidget(
        _host(
          ContentImpactRow.build(
            context: await _context(tester),
            impact: _impact(moneyUsd: 210),
          ),
        ),
      );

      expect(find.byIcon(Icons.chevron_right), findsNothing);
    });
  });
}

/// ContentImpactRow.build needs a BuildContext with localizations; pump a
/// bare localized shell first and hand its element back.
Future<BuildContext> _context(WidgetTester tester) async {
  await tester.pumpWidget(_host(null));
  return tester.element(find.byType(Scaffold));
}
