import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/impact/sharing_impact_card.dart';

import '../../../helpers/l10n_helpers.dart';

const _gearFraming =
    'Each time a friend borrows this instead of buying one, your circle saves:';
const _requestFraming =
    'Because neighbors pitched in instead of you buying new, you all saved:';

Widget _card({
  String framingText = _gearFraming,
  String? valueSaved,
  String? timeRecovered,
  String? co2Avoided,
  String? qualityTime,
  String? perActionLabel,
  VoidCallback? onValueTap,
  VoidCallback? onTimeTap,
  VoidCallback? onCo2Tap,
}) {
  return localizedApp(
    Scaffold(
      body: SharingImpactCard(
        framingText: framingText,
        valueSaved: valueSaved,
        timeRecovered: timeRecovered,
        co2Avoided: co2Avoided,
        qualityTime: qualityTime,
        perActionLabel: perActionLabel,
        onValueTap: onValueTap,
        onTimeTap: onTimeTap,
        onCo2Tap: onCo2Tap,
      ),
    ),
  );
}

void main() {
  group('SharingImpactCard', () {
    testWidgets('renders framing text', (tester) async {
      await tester.pumpWidget(
        _card(valueSaved: '\$120', timeRecovered: '2h', co2Avoided: '42kg'),
      );

      expect(find.text(_gearFraming), findsOneWidget);
    });

    testWidgets('renders scope-appropriate custom framing text', (
      tester,
    ) async {
      await tester.pumpWidget(
        _card(
          framingText: _requestFraming,
          valueSaved: '\$80',
          timeRecovered: '8h',
          co2Avoided: '19kg',
        ),
      );

      expect(find.text(_requestFraming), findsOneWidget);
      // The gear-loan phrasing must not leak onto a request/experience view.
      expect(find.text(_gearFraming), findsNothing);
    });

    testWidgets('renders section header with per-action qualifier', (
      tester,
    ) async {
      await tester.pumpWidget(
        _card(
          valueSaved: '\$120',
          timeRecovered: '2h',
          co2Avoided: '42kg',
          perActionLabel: 'Per fulfillment',
        ),
      );

      expect(find.text('WHAT SHARING THIS MEANS'), findsOneWidget);
      // The card's numbers are per action, unlike the hero totals — the
      // header must say so (#2724).
      expect(find.text('PER FULFILLMENT'), findsOneWidget);
    });

    testWidgets('renders all provided metric values and labels', (
      tester,
    ) async {
      await tester.pumpWidget(
        _card(
          valueSaved: '\$325',
          timeRecovered: '3h',
          co2Avoided: '84kg',
          qualityTime: '~4 h',
        ),
      );

      expect(find.text('\$325'), findsOneWidget);
      expect(find.text('3h'), findsOneWidget);
      expect(find.text('84kg'), findsOneWidget);
      expect(find.text('~4 h'), findsOneWidget);
      expect(find.text('Saved'), findsOneWidget);
      expect(find.text('Recovered'), findsOneWidget);
      expect(find.text('CO₂ Avoided'), findsOneWidget);
      expect(find.text('Quality Time'), findsOneWidget);
    });

    testWidgets('captions distinguish the two time metrics', (tester) async {
      await tester.pumpWidget(
        _card(timeRecovered: '3h', qualityTime: '~4 h'),
      );

      // #2724 — "Recovered" vs "Quality Time" side by side were
      // unexplained; each carries a one-line clarifier.
      expect(
        find.text('Time not spent buying or doing it alone'),
        findsOneWidget,
      );
      expect(find.text('Time spent together'), findsOneWidget);
    });

    testWidgets('hides tiles for null (zero) metrics', (tester) async {
      await tester.pumpWidget(
        _card(valueSaved: '\$120', co2Avoided: '42kg'),
      );

      // #2724 — zero metrics are suppressed, not rendered as dashes.
      expect(find.text('Saved'), findsOneWidget);
      expect(find.text('CO₂ Avoided'), findsOneWidget);
      expect(find.text('Recovered'), findsNothing);
      expect(find.text('Quality Time'), findsNothing);
      expect(find.text('—'), findsNothing);
    });

    testWidgets('renders nothing when every metric is null', (tester) async {
      await tester.pumpWidget(_card());

      expect(find.text('WHAT SHARING THIS MEANS'), findsNothing);
      expect(find.text(_gearFraming), findsNothing);
    });

    testWidgets('hasAnyMetric reflects tile presence', (tester) async {
      const empty = SharingImpactCard(framingText: _gearFraming);
      const withValue =
          SharingImpactCard(framingText: _gearFraming, valueSaved: '\$5');
      expect(empty.hasAnyMetric, isFalse);
      expect(withValue.hasAnyMetric, isTrue);
    });

    testWidgets('fires onValueTap when value tile is tapped', (tester) async {
      var tapped = false;
      await tester.pumpWidget(
        _card(
          valueSaved: '\$120',
          timeRecovered: '2h',
          co2Avoided: '42kg',
          onValueTap: () => tapped = true,
        ),
      );

      await tester.tap(find.text('\$120'));
      expect(tapped, isTrue);
    });

    testWidgets('fires onTimeTap when time tile is tapped', (tester) async {
      var tapped = false;
      await tester.pumpWidget(
        _card(
          valueSaved: '\$120',
          timeRecovered: '2h',
          co2Avoided: '42kg',
          onTimeTap: () => tapped = true,
        ),
      );

      await tester.tap(find.text('2h'));
      expect(tapped, isTrue);
    });

    testWidgets('fires onCo2Tap when CO2 tile is tapped', (tester) async {
      var tapped = false;
      await tester.pumpWidget(
        _card(
          valueSaved: '\$120',
          timeRecovered: '2h',
          co2Avoided: '42kg',
          onCo2Tap: () => tapped = true,
        ),
      );

      await tester.tap(find.text('42kg'));
      expect(tapped, isTrue);
    });

    testWidgets('does not crash when no onTap callbacks are provided', (
      tester,
    ) async {
      await tester.pumpWidget(
        _card(valueSaved: '\$120', timeRecovered: '2h', co2Avoided: '42kg'),
      );

      // Tap tiles with no callbacks — should not throw
      await tester.tap(find.text('\$120'));
      await tester.tap(find.text('2h'));
      await tester.tap(find.text('42kg'));
      await tester.pump();
    });
  });
}
