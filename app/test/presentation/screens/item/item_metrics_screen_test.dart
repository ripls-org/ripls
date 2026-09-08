import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show CarbonEstimate, Estimate;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show GetRequestStatsResponse;
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/models/item_metric_data_base.dart';
import 'package:ripls/presentation/screens/item/item_metrics_screen.dart';
import 'package:ripls/presentation/viewmodels/request_metric_view_model.dart';
import 'package:ripls/presentation/widgets/item/item_metric_data.dart';

import '../../../helpers/l10n_helpers.dart';

const _requestId = 'req-1';
const _requestFraming =
    'Because neighbors pitched in instead of you buying new, you all saved:';

/// Fake that returns fixed metric data, bypassing the real notifier's
/// network loads so the screen renders in isolation.
class _FakeRequestMetricsNotifier extends RequestImpactMetricsNotifier {
  _FakeRequestMetricsNotifier(super.params, this._data);
  final RequestMetricData _data;

  @override
  Future<RequestMetricData> build() async => _data;
}

ImpactEstimate _impact({
  double moneyUsd = 0,
  double timeMinutes = 0,
  double qtMinutes = 0,
  double co2Grams = 0,
}) {
  return ImpactEstimate(
    moneySaved: moneyUsd > 0
        ? MoneySavings(valueUsd: Estimate(mean: moneyUsd))
        : null,
    timeSaved: timeMinutes > 0
        ? TimeSavings(minutes: Estimate(mean: timeMinutes))
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

RequestMetricData _data({
  required ImpactEstimate cumulative,
  required ImpactEstimate perAction,
  List<PersonWithRole> people = const [],
  int timesFulfilled = 1,
}) {
  return RequestMetricData(
    requestId: _requestId,
    itemName: 'A lawn mower for the weekend',
    description: 'Ours died halfway through the front yard.',
    displayData: ItemMetricDisplayData(
      mediaId: '',
      metrics: const [],
      savings: SavingsFormatter.formatSavings(perAction),
    ),
    people: people,
    stats: GetRequestStatsResponse(
      timesFulfilled: timesFulfilled,
      impact: cumulative,
      potentialImpact: perAction,
    ),
    stories: const [],
    metadataMetrics: const [],
  );
}

Future<void> _pump(WidgetTester tester, RequestMetricData data) async {
  const params = RequestMetricParams(requestId: _requestId);
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        requestImpactMetricsProvider(params).overrideWith(
          () => _FakeRequestMetricsNotifier(params, data),
        ),
      ],
      child: localizedApp(
        const ItemMetricsScreen(
          itemType: ItemType.request,
          itemId: _requestId,
        ),
      ),
    ),
  );
  // First pump renders the loading spinner; settle the async build.
  await tester.pump();
  await tester.pump();
}

void main() {
  group('ItemMetricsScreen request receipt (#2724)', () {
    final helper = PersonWithRole(
      user: User(id: 'theo', name: 'Theo Alvarez'),
      roleLabel: 'Helped',
      roleColor: Colors.green,
      sortOrder: 1,
    );

    testWidgets('frames the request receipt for the requester (A6)', (
      tester,
    ) async {
      await _pump(
        tester,
        _data(
          cumulative: _impact(moneyUsd: 210, qtMinutes: 237, co2Grams: 40000),
          perAction: _impact(moneyUsd: 210, qtMinutes: 237, co2Grams: 40000),
          people: [helper],
        ),
      );

      expect(find.text(_requestFraming), findsOneWidget);
      // The owner-framed gear copy must not leak onto a request receipt.
      expect(find.textContaining('a friend borrows'), findsNothing);
    });

    testWidgets('labels the explanatory card as per-fulfillment (A5)', (
      tester,
    ) async {
      await _pump(
        tester,
        _data(
          cumulative: _impact(moneyUsd: 210, co2Grams: 40000),
          perAction: _impact(moneyUsd: 105, co2Grams: 20000),
          people: [helper],
        ),
      );

      expect(find.text('WHAT SHARING THIS MEANS'), findsOneWidget);
      expect(find.text('PER FULFILLMENT'), findsOneWidget);
    });

    testWidgets('suppresses zero hero tiles and keeps non-zero ones (A4)', (
      tester,
    ) async {
      await _pump(
        tester,
        _data(
          cumulative: _impact(qtMinutes: 237, co2Grams: 40000),
          perAction: _impact(qtMinutes: 237, co2Grams: 40000),
          people: [helper],
        ),
      );

      // Money and recovered-time are zero: no "$0" headline anywhere.
      expect(find.text('\$0'), findsNothing);
      expect(find.text('SAVED'), findsNothing);
      expect(find.text('RECOVERED'), findsNothing);
      expect(find.text('CO₂ AVOIDED'), findsOneWidget);
      expect(find.text('QUALITY TIME'), findsOneWidget);
      // Humanized QT — never raw minutes (A3).
      expect(find.text('~4 h'), findsWidgets);
      expect(find.textContaining('237'), findsNothing);
    });

    testWidgets('captions the two time metrics (A2)', (tester) async {
      await _pump(
        tester,
        _data(
          cumulative: _impact(timeMinutes: 480, qtMinutes: 37),
          perAction: _impact(timeMinutes: 480, qtMinutes: 37),
          people: [helper],
        ),
      );

      expect(
        find.text(
          'Recovered: time not spent buying or doing it alone · '
          'Quality time: time spent together',
        ),
        findsOneWidget,
      );
      expect(find.text('Time spent together'), findsOneWidget);
    });

    testWidgets('hides the hero strip and card when all metrics are zero', (
      tester,
    ) async {
      await _pump(
        tester,
        _data(
          cumulative: _impact(),
          perAction: _impact(),
          people: [helper],
        ),
      );

      expect(find.text('SAVED'), findsNothing);
      expect(find.text('QUALITY TIME'), findsNothing);
      expect(find.text('WHAT SHARING THIS MEANS'), findsNothing);
      expect(find.text(_requestFraming), findsNothing);
    });

    testWidgets('shows fulfillment history from helper roles', (tester) async {
      await _pump(
        tester,
        _data(
          cumulative: _impact(moneyUsd: 210),
          perAction: _impact(moneyUsd: 210),
          people: [helper],
        ),
      );

      expect(find.text('FULFILLMENT HISTORY'), findsOneWidget);
      expect(find.text('Theo Alvarez'), findsOneWidget);
      expect(find.text('Helped fulfill this request'), findsOneWidget);
    });

    testWidgets('hides the history header when no rows are renderable', (
      tester,
    ) async {
      await _pump(
        tester,
        _data(
          cumulative: _impact(moneyUsd: 210),
          perAction: _impact(moneyUsd: 210),
          people: const [],
        ),
      );

      // A heading over blank space reads as broken (#2724 / P0.5).
      expect(find.text('FULFILLMENT HISTORY'), findsNothing);
    });
  });
}
