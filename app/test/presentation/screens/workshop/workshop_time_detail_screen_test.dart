import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' as common;
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart' as pb;
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/workshop/workshop_time_detail_screen.dart';
import 'package:ripls/services/providers/impact_providers.dart';

import 'workshop_time_detail_screen_test.mocks.dart';

@GenerateMocks([ImpactMetricsRepository])
void main() {
  late MockImpactMetricsRepository mockRepo;

  setUp(() {
    mockRepo = MockImpactMetricsRepository();
  });

  pb.GetCommunityMetricDetailResponse makeResponse({
    double totalMinutes = 0,
    List<pb.SourceBreakdown> sources = const [],
    List<pb.TimeSeriesPoint> monthlyBars = const [],
  }) {
    return pb.GetCommunityMetricDetailResponse(
      totalValue: common.Estimate(mean: totalMinutes),
      qualityTimeDetail: pb.QualityTimeDetail(monthlyBars: monthlyBars),
      sourceBreakdown: sources,
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
        home: WorkshopTimeDetailScreen(
          communityIds: communityIds,
        ),
      ),
    );
  }

  group('WorkshopTimeDetailScreen', () {
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
                sources: [
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 1860),
                ],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      // 1860 min → 31 hr → time-health tier t_25hr.
      expect(find.textContaining('Starting an exercise habit'), findsOneWidget);
    });

    testWidgets('title sums breakdown rows into the headline', (tester) async {
      // Breakdown sums to 1860 minutes → 31 hr.
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 900),
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS, value: 960),
                ],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.textContaining('31 HR', findRichText: true), findsWidgets);
    });

    testWidgets('aggregates minutes across two communities', (tester) async {
      when(mockRepo.getMetricDetail('c1', any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 900)],
              ));
      when(mockRepo.getMetricDetail('c2', any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS, value: 960)],
              ));

      await tester.pumpWidget(
        buildHarness(communityIds: ['c1', 'c2']),
      );
      await tester.pumpAndSettle();

      // 900 + 960 = 1860 min = 31 hr.
      expect(find.textContaining('31 HR', findRichText: true), findsWidgets);
    });

    testWidgets('headline minutes equal source breakdown sum', (tester) async {
      // The total displayed in the title must equal the sum of the rendered
      // "Where it came from" rows.
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 180),
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_REQUESTS, value: 240),
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS, value: 1440),
                ],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      // 180 + 240 + 1440 = 1860 min = 31 hr.
      expect(find.textContaining('31 HR', findRichText: true), findsWidgets);
      // Loans = 180 min → "3 hr"; Requests = 240 → "4 hr"; Events = 1440 → "24 hr".
      expect(find.text('3 hr'), findsOneWidget);
      expect(find.text('4 hr'), findsOneWidget);
      expect(find.text('24 hr'), findsOneWidget);
    });

    testWidgets('zero-value sources hidden except Events', (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 120),
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_REQUESTS, value: 0),
                ],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.text('Loans'), findsOneWidget);
      expect(find.text('Requests'), findsNothing);
      // Events always renders even though it was not in the response.
      expect(find.text('Events'), findsOneWidget);
    });

    testWidgets('recent-items section header not rendered when list empty',
        (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 60)],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.text('MOST RECENT CONTRIBUTIONS'), findsNothing);
    });

    testWidgets('recent-items section renders entries from response',
        (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 60)],
              )
                ..recentItems.add(
                  pb.RecentActivity(
                    itemName: 'Hike',
                    personDisplayName: 'Alfred Briggs',
                    rawValue: 90,
                  ),
                ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.text('MOST RECENT CONTRIBUTIONS'), findsOneWidget);
      expect(find.text('Hike'), findsOneWidget);
    });
  });
}
