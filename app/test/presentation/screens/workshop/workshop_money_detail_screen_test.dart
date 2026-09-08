import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart' as pb;
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/workshop/workshop_money_detail_screen.dart';
import 'package:ripls/services/providers/impact_providers.dart';

import 'workshop_money_detail_screen_test.mocks.dart';

@GenerateMocks([ImpactMetricsRepository])
void main() {
  late MockImpactMetricsRepository mockRepo;

  setUp(() {
    mockRepo = MockImpactMetricsRepository();
  });

  pb.GetCommunityMetricDetailResponse makeResponse({
    double currentUsd = 0,
    List<pb.SourceBreakdown> sources = const [],
    List<pb.TimeSeriesPoint> cumulativeTrend = const [],
  }) {
    return pb.GetCommunityMetricDetailResponse(
      moneyDetail: pb.MoneySavedDetail(currentUsd: currentUsd),
      sourceBreakdown: sources,
      cumulativeTrend: cumulativeTrend,
    );
  }

  Widget buildHarness({
    List<String> communityIds = const ['c1'],
  }) {
    return ProviderScope(
      overrides: [
        impactMetricsRepositoryProvider.overrideWithValue(mockRepo),
      ],
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: WorkshopMoneyDetailScreen(
          communityIds: communityIds,
        ),
      ),
    );
  }

  group('WorkshopMoneyDetailScreen', () {
    testWidgets('shows loading indicator while fetching', (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) => Completer<GetCommunityMetricDetailResponse>().future);

      await tester.pumpWidget(buildHarness());
      await tester.pump();

      expect(find.byType(CircularProgressIndicator), findsOneWidget);
    });

    testWidgets('renders equivalence headline after load', (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,value: 1241)],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      // $1241 → money-paycheck tier m_1025.
      expect(find.textContaining('A 1.2% raise for the year'), findsOneWidget);
    });

    testWidgets('title sums breakdown rows into the headline',
        (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,value: 1241)],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      // Breadcrumb includes "$1,241".
      expect(find.textContaining('\$1,241', findRichText: true), findsWidgets);
    });

    testWidgets('aggregates dollars across two communities',
        (tester) async {
      when(mockRepo.getMetricDetail('c1', any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,value: 600)],
              ));
      when(mockRepo.getMetricDetail('c2', any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS,value: 641)],
              ));

      await tester.pumpWidget(
        buildHarness(communityIds: ['c1', 'c2']),
      );
      await tester.pumpAndSettle();

      expect(find.textContaining('\$1,241', findRichText: true), findsWidgets);
    });

    testWidgets('headline dollars equal source breakdown sum',
        (tester) async {
      // The total displayed in the title must equal the sum of the
      // rendered "Where it came from" rows.
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,value: 200),
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_GIVEAWAYS,value: 300),
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS,value: 741),
                ],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      // 200 + 300 + 741 = 1241.
      expect(find.textContaining('\$1,241', findRichText: true), findsWidgets);
      expect(find.text('\$200'), findsOneWidget);
      expect(find.text('\$300'), findsOneWidget);
      expect(find.text('\$741'), findsOneWidget);
    });

    testWidgets('zero-value sources hidden except Events', (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,value: 50),
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_GIVEAWAYS,value: 0),
                ],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.text('Loans'), findsOneWidget);
      expect(find.text('Giveaways'), findsNothing);
      expect(find.text('Events'), findsOneWidget);
    });

    testWidgets('recent-items section header not rendered when list empty',
        (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,value: 10)],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.text('MOST RECENT CONTRIBUTIONS'), findsNothing);
    });

    testWidgets('recent-items section renders entries from response',
        (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,value: 10)],
              )
                ..recentItems.add(
                  pb.RecentActivity(
                    itemName: 'Ski Touring Setup',
                    personDisplayName: 'Alfred Briggs',
                    rawValue: 340,
                  ),
                ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.text('MOST RECENT CONTRIBUTIONS'), findsOneWidget);
      expect(find.text('Ski Touring Setup'), findsOneWidget);
    });
  });
}
