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
import 'package:ripls/presentation/screens/workshop/workshop_co2_detail_screen.dart';
import 'package:ripls/services/providers/impact_providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'workshop_co2_detail_screen_test.mocks.dart';

@GenerateMocks([ImpactMetricsRepository])
void main() {
  late MockImpactMetricsRepository mockRepo;

  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
    mockRepo = MockImpactMetricsRepository();
  });

  pb.GetCommunityMetricDetailResponse makeResponse({
    double currentKg = 0,
    List<pb.SourceBreakdown> sources = const [],
    List<pb.TimeSeriesPoint> monthlyBars = const [],
  }) {
    return pb.GetCommunityMetricDetailResponse(
      emissionsPreventedDetail: pb.EmissionsPreventedDetail(
        currentKg: currentKg,
        monthlyBars: monthlyBars,
      ),
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
        home: WorkshopCo2DetailScreen(
          communityIds: communityIds,
        ),
      ),
    );
  }

  group('WorkshopCo2DetailScreen', () {
    testWidgets('shows loading indicator while fetching', (tester) async {
      // Never completes — stays in loading state.
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) => Completer<GetCommunityMetricDetailResponse>().future);

      await tester.pumpWidget(buildHarness());
      await tester.pump();

      expect(find.byType(CircularProgressIndicator), findsOneWidget);
    });

    testWidgets('renders equivalence headline after load', (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 113000)],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      // 113000 g → 113 kg → co2 daily-life tier c_100.
      expect(
          find.textContaining('A full tank of gas, unburned'), findsOneWidget);
    });

    testWidgets('title sums breakdown rows into the headline', (tester) async {
      // Breakdown sums to 113000 g → 113 kg. The app-bar breadcrumb shows
      // the uppercase total "113 KG"; the editorial section's headline is
      // the resolved tier (c_100).
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 113000)],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.textContaining('113 KG'), findsWidgets);
    });

    testWidgets('aggregates kg across two communities', (tester) async {
      // SourceBreakdown.value is in grams; 60000g + 53000g = 113kg.
      when(mockRepo.getMetricDetail('c1', any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 60000)],
              ));
      when(mockRepo.getMetricDetail('c2', any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS, value: 53000)],
              ));

      await tester.pumpWidget(
        buildHarness(communityIds: ['c1', 'c2']),
      );
      await tester.pumpAndSettle();

      // App-bar breadcrumb shows aggregated total.
      expect(find.textContaining('113 KG'), findsWidgets);
      // Per-community contributions render in the breakdown table.
      expect(find.text('60 kg'), findsOneWidget);
      expect(find.text('53 kg'), findsOneWidget);
    });

    testWidgets('headline kg equal source breakdown sum', (tester) async {
      // The total displayed in the title must equal the sum of the
      // rendered "Where it came from" rows.
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 20000),
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_GIVEAWAYS, value: 30000),
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS, value: 63000),
                ],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      // 20 + 30 + 63 = 113 kg; app-bar shows the uppercase total.
      expect(find.textContaining('113 KG'), findsWidgets);
      expect(find.text('20 kg'), findsOneWidget);
      expect(find.text('30 kg'), findsOneWidget);
      expect(find.text('63 kg'), findsOneWidget);
    });

    testWidgets('zero-value sources hidden except Events', (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 50000),
                  pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_GIVEAWAYS, value: 0),
                ],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.text('Loans'), findsOneWidget);
      expect(find.text('Giveaways'), findsNothing);
      // Events always renders even though it was not in the response.
      expect(find.text('Events'), findsOneWidget);
    });

    testWidgets('recent-items section header not rendered when list empty',
        (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 10000)],
              ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.text('MOST RECENT CONTRIBUTIONS'), findsNothing);
    });

    testWidgets('recent-items section renders entries from response',
        (tester) async {
      when(mockRepo.getMetricDetail(any, any, any))
          .thenAnswer((_) async => makeResponse(
                sources: [pb.SourceBreakdown(sourceType: pb.ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS, value: 10000)],
              )
                ..recentItems.add(
                  pb.RecentActivity(
                    itemName: 'Cargo Bike',
                    personDisplayName: 'Alfred Briggs',
                    rawValue: 800,
                  ),
                ));

      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.text('MOST RECENT CONTRIBUTIONS'), findsOneWidget);
      expect(find.text('Cargo Bike'), findsOneWidget);
    });

  });
}
