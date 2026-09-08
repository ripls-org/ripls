import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart' as pb;
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/workshop/workshop_problems_solved_detail_screen.dart';
import 'package:ripls/presentation/widgets/impact/community/impact_chart_card.dart';
import 'package:ripls/services/providers/impact_providers.dart';

import 'workshop_problems_solved_detail_screen_test.mocks.dart';

@GenerateMocks([ImpactMetricsRepository])
void main() {
  late MockImpactMetricsRepository mockRepo;

  setUp(() {
    mockRepo = MockImpactMetricsRepository();
  });

  pb.GetCommunityProblemsSolvedDetailResponse makeResponse({
    int handledLoans = 0,
    int potentialLoans = 0,
    int handledRequests = 0,
    int potentialRequests = 0,
    int handledNeeds = 0,
    int potentialNeeds = 0,
    List<pb.ProblemSolvedItem> items = const [],
  }) {
    return pb.GetCommunityProblemsSolvedDetailResponse(
      handledCount: handledLoans + handledRequests + handledNeeds,
      handledLoans: handledLoans,
      handledRequests: handledRequests,
      handledNeeds: handledNeeds,
      potentialCount: potentialLoans + potentialRequests + potentialNeeds,
      potentialLoans: potentialLoans,
      potentialRequests: potentialRequests,
      potentialNeeds: potentialNeeds,
      items: items,
    );
  }

  Widget harness({List<String> communityIds = const ['c1']}) {
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
        home: WorkshopProblemsSolvedDetailScreen(communityIds: communityIds),
      ),
    );
  }

  group('WorkshopProblemsSolvedDetailScreen', () {
    testWidgets('sums handled-over-potential across communities and renders '
        'breakdown', (tester) async {
      when(mockRepo.getProblemsSolvedDetail('c1')).thenAnswer(
        (_) async => makeResponse(
          handledLoans: 1,
          potentialLoans: 2,
          handledRequests: 1,
          potentialRequests: 1,
        ),
      );
      when(mockRepo.getProblemsSolvedDetail('c2')).thenAnswer(
        (_) async => makeResponse(handledNeeds: 1, potentialNeeds: 3),
      );

      await tester.pumpWidget(harness(communityIds: ['c1', 'c2']));
      await tester.pumpAndSettle();

      // 3 handled of 6 potential in the title.
      expect(find.textContaining('3 / 6', findRichText: true), findsWidgets);
      expect(find.text('Requests fulfilled'), findsOneWidget);
      expect(find.text('Loans completed'), findsOneWidget);
      expect(find.text('Needs met'), findsOneWidget);
      // Per-bucket "handled / potential" appears in the breakdown rows.
      expect(find.text('1 / 2'), findsOneWidget); // loans
      expect(find.text('1 / 3'), findsOneWidget); // needs
    });

    testWidgets('renders completed entries', (tester) async {
      when(mockRepo.getProblemsSolvedDetail(any)).thenAnswer(
        (_) async => makeResponse(
          handledRequests: 1,
          potentialRequests: 1,
          items: [
            pb.ProblemSolvedItem(
              kind: pb.ProblemSolvedKind.PROBLEM_SOLVED_KIND_REQUEST,
              title: 'Fix bike',
              personName: 'Alfred',
            ),
          ],
        ),
      );

      await tester.pumpWidget(harness());
      await tester.pumpAndSettle();

      expect(find.text('Completed'), findsOneWidget);
      expect(find.text('Fix bike'), findsOneWidget);
    });

    testWidgets('renders the monthly chart for timestamped entries',
        (tester) async {
      when(mockRepo.getProblemsSolvedDetail(any)).thenAnswer(
        (_) async => makeResponse(
          handledRequests: 1,
          potentialRequests: 1,
          items: [
            pb.ProblemSolvedItem(
              kind: pb.ProblemSolvedKind.PROBLEM_SOLVED_KIND_REQUEST,
              title: 'Fix bike',
              completedAtUnixSec: Int64(1700000000),
            ),
          ],
        ),
      );

      await tester.pumpWidget(harness());
      await tester.pumpAndSettle();

      expect(find.text('BY MONTH · PROBLEMS HANDLED'), findsOneWidget);
      expect(find.byType(ImpactChartCard), findsOneWidget);
    });

    testWidgets('shows empty state when nothing solved', (tester) async {
      when(mockRepo.getProblemsSolvedDetail(any))
          .thenAnswer((_) async => makeResponse());

      await tester.pumpWidget(harness());
      await tester.pumpAndSettle();

      expect(find.text('Nothing solved yet.'), findsOneWidget);
    });
  });
}
