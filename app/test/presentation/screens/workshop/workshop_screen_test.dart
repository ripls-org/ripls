import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/workshop_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/workshop/workshop_screen.dart';
import 'package:ripls/presentation/viewmodels/workshop_view_model.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/providers/workshop_providers.dart';
import 'package:ripls/services/workshop_service.dart';

import 'workshop_screen_test.mocks.dart';

@GenerateMocks([WorkshopService])
void main() {
  late StashCacheManager cacheManager;
  late MockWorkshopService mockService;

  setUp(() async {
    cacheManager = StashCacheManager();
    await cacheManager.initialize();
    mockService = MockWorkshopService();
  });

  void stubEmptyBrief() {
    when(mockService.getWorkshopBrief(
      communityIds: anyNamed('communityIds'),
    )).thenAnswer((_) async => GetWorkshopBriefResponse());
    when(mockService.getWorkshopSynthesis(
      communityIds: anyNamed('communityIds'),
    )).thenAnswer((_) async => GetWorkshopSynthesisResponse());
  }

  Widget buildHarness() {
    return ProviderScope(
      overrides: [
        cacheManagerProvider.overrideWithValue(cacheManager),
        workshopServiceProvider.overrideWithValue(mockService),
      ],
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: const WorkshopScreen(),
      ),
    );
  }

  group('empty brief', () {
    setUp(stubEmptyBrief);

    testWidgets('renders the empty placeholder body', (tester) async {
      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(
        find.text('Suggestions will appear here as your circles get going.'),
        findsOneWidget,
      );
    });

    testWidgets('does not render a pill carousel', (tester) async {
      await tester.pumpWidget(buildHarness());
      await tester.pumpAndSettle();

      expect(find.text('All'), findsNothing);
    });
  });

  test('WorkshopRepository.getBrief delegates to the service', () async {
    when(mockService.getWorkshopBrief(
      communityIds: anyNamed('communityIds'),
    )).thenAnswer((_) async => GetWorkshopBriefResponse());
    final repository = WorkshopRepository(cacheManager, mockService);

    final brief = await repository.getBrief(communityIds: const ['comm-1']);

    expect(brief.hasBrief(), isFalse);
    verify(mockService.getWorkshopBrief(
      communityIds: anyNamed('communityIds'),
    )).called(1);
  });

  group('WorkshopNotifier disposal safety', () {
    test('disposing the container while a fetch is in flight does not throw',
        () async {
      when(mockService.getWorkshopBrief(
        communityIds: anyNamed('communityIds'),
      )).thenAnswer((_) async {
        await Future<void>.delayed(const Duration(seconds: 1));
        return GetWorkshopBriefResponse();
      });
      when(mockService.getWorkshopSynthesis(
        communityIds: anyNamed('communityIds'),
      )).thenAnswer((_) async {
        await Future<void>.delayed(const Duration(seconds: 1));
        return GetWorkshopSynthesisResponse();
      });

      final container = ProviderContainer(overrides: [
        cacheManagerProvider.overrideWithValue(cacheManager),
        workshopServiceProvider.overrideWithValue(mockService),
      ]);

      final pending = container.read(workshopProvider.future);
      container.dispose();

      await expectLater(
        pending.catchError((_) => const WorkshopState()),
        completes,
      );
    });
  });
}
