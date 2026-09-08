import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/gen/ripls/api/item.pb.dart';
import 'package:ripls/data/gen/ripls/api/workshop_service.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/workshop/workshop_screen.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/workshop_providers.dart';

import 'workshop_screen_test.mocks.dart';

/// Seeds [communitiesProvider] so the Workshop pager renders a page.
class _SeededCommunitiesNotifier extends CommunitiesNotifier {
  _SeededCommunitiesNotifier(this._communities);
  final List<CommunityItem> _communities;

  @override
  CommunitiesState build() => CommunitiesState(communities: _communities);
}

void main() {
  // Regression guard for the all-black Workshop body: the metrics / Where-When
  // rows used `crossAxisAlignment: stretch` without an intrinsic-height bound,
  // forcing an infinite height that failed layout and spammed semantics
  // parentDataDirty assertions. Rendering the populated body with semantics on
  // catches a recurrence.
  testWidgets('populated workshop body builds without layout/semantics errors',
      (tester) async {
    final cacheManager = StashCacheManager();
    await cacheManager.initialize();
    final mockService = MockWorkshopService();

    final brief = BriefPayload()
      ..hoursTogether = 349
      ..replacedCostUsd = 12995
      ..co2AvoidedPounds = 1764
      ..specialties.addAll(['Cooking', 'Outdoor', 'Hiking'])
      ..availableNowItems.add(Item()
        ..title = "Alfred's chainsaw"
        ..kind = ItemKind.ITEM_KIND_GEAR
        ..miniLabel = 'LOAN')
      ..ctaRows.add(BriefCTARow()
        ..headline = 'Bring back the Morning Fishing Trip'
        ..atmosphereLine = 'Ran 6 times, then quiet'
        ..item = (Item()
          ..contextId = 'exp-1'
          ..kind = ItemKind.ITEM_KIND_EXPERIENCE
          ..title = 'Bring back the Morning Fishing Trip'));

    when(mockService.getWorkshopBrief(
      communityIds: anyNamed('communityIds'),
    )).thenAnswer((_) async => GetWorkshopBriefResponse()..brief = brief);
    when(mockService.getWorkshopSynthesis(
      communityIds: anyNamed('communityIds'),
    )).thenAnswer((_) async => GetWorkshopSynthesisResponse());

    final handle = tester.ensureSemantics();

    // Tall viewport so the lazy ListView builds every section (including the
    // bottom Where/When + newest cards) — those are WorkshopGlassCards with
    // onExpand, and a closure-capture bug there recursed infinitely at layout.
    tester.view.physicalSize = const Size(1170, 4200);
    tester.view.devicePixelRatio = 3.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(ProviderScope(
      overrides: [
        cacheManagerProvider.overrideWithValue(cacheManager),
        workshopServiceProvider.overrideWithValue(mockService),
        // One community so the pager renders a page (each page scopes the
        // workshop providers to its own community).
        communitiesProvider.overrideWith(
          () => _SeededCommunitiesNotifier([
            CommunityItem()
              ..id = 'comm-1'
              ..name = 'Test Circle',
          ]),
        ),
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
    ));
    await tester.pumpAndSettle();

    expect(tester.takeException(), isNull);
    handle.dispose();
  });

  // Zero-state: before any impact data exists, the demoralizing "0 of 0
  // problems, $0 saved" sentence is replaced with a crew invitation + teaser.
  testWidgets('empty brief shows the crew invitation, not the metrics sentence',
      (tester) async {
    final cacheManager = StashCacheManager();
    await cacheManager.initialize();
    final mockService = MockWorkshopService();

    // No impact numbers set anywhere on the brief.
    when(mockService.getWorkshopBrief(
      communityIds: anyNamed('communityIds'),
    )).thenAnswer(
        (_) async => GetWorkshopBriefResponse()..brief = BriefPayload());
    when(mockService.getWorkshopSynthesis(
      communityIds: anyNamed('communityIds'),
    )).thenAnswer((_) async => GetWorkshopSynthesisResponse());

    tester.view.physicalSize = const Size(1170, 4200);
    tester.view.devicePixelRatio = 3.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(ProviderScope(
      overrides: [
        cacheManagerProvider.overrideWithValue(cacheManager),
        workshopServiceProvider.overrideWithValue(mockService),
        communitiesProvider.overrideWith(
          () => _SeededCommunitiesNotifier([
            CommunityItem()
              ..id = 'comm-1'
              ..name = 'Test Circle',
          ]),
        ),
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
    ));
    await tester.pumpAndSettle();

    // Invitation, named invite CTA, and the teaser are present...
    expect(
      find.textContaining('A crew is better with more hands'),
      findsOneWidget,
    );
    expect(find.text('Invite to Test Circle'), findsOneWidget);
    expect(
      find.textContaining("you'll see what you've built together"),
      findsOneWidget,
    );
    // ...and the metrics sentence (with its "saved $" connector) is gone.
    expect(find.textContaining('saved \$'), findsNothing);
  });

  // The metrics sentence is assembled from only the metrics above zero, joined
  // grammatically — a lone $0 / 0 kg never renders inline.
  group('metrics sentence assembly', () {
    Finder richContaining(String s) =>
        find.textContaining(s, findRichText: true);

    testWidgets('one metric: just the problems clause', (tester) async {
      await _pumpWorkshopWithBrief(
        tester,
        BriefPayload()
          ..problemsSolvedCount = 3
          ..problemsPotentialCount = 8,
      );
      expect(richContaining('solved 3 of 8 problems'), findsWidgets);
      expect(richContaining('saved'), findsNothing);
      expect(richContaining('shared'), findsNothing);
      expect(richContaining('kept'), findsNothing);
    });

    testWidgets('two metrics: joined with "and", no comma', (tester) async {
      await _pumpWorkshopWithBrief(
        tester,
        BriefPayload()
          ..problemsSolvedCount = 3
          ..problemsPotentialCount = 8
          ..replacedCostUsd = 120,
      );
      expect(
        richContaining('solved 3 of 8 problems and saved \$120'),
        findsWidgets,
      );
    });

    testWidgets('three metrics: Oxford comma before "and"', (tester) async {
      await _pumpWorkshopWithBrief(
        tester,
        BriefPayload()
          ..problemsSolvedCount = 3
          ..problemsPotentialCount = 8
          ..replacedCostUsd = 120
          ..hoursTogether = 6,
      );
      expect(
        richContaining('solved 3 of 8 problems, saved \$120, and shared 6 hours'),
        findsWidgets,
      );
    });

    testWidgets('all asks handled: plain "X problems", no denominator',
        (tester) async {
      await _pumpWorkshopWithBrief(
        tester,
        BriefPayload()
          ..problemsSolvedCount = 3
          ..problemsPotentialCount = 3,
      );
      expect(richContaining('solved 3 problems'), findsWidgets);
      expect(richContaining('3 of 3'), findsNothing);
    });

    testWidgets('singular grammar: "1 problem"', (tester) async {
      await _pumpWorkshopWithBrief(
        tester,
        BriefPayload()
          ..problemsSolvedCount = 1
          ..problemsPotentialCount = 1,
      );
      expect(richContaining('solved 1 problem'), findsWidgets);
      expect(richContaining('1 problems'), findsNothing);
    });

    testWidgets('nothing solved but asks open: reframed as an invitation',
        (tester) async {
      await _pumpWorkshopWithBrief(
        tester,
        BriefPayload()..problemsPotentialCount = 8,
      );
      expect(
        richContaining('8 open requests waiting on a hand'),
        findsWidgets,
      );
      expect(richContaining('0 of'), findsNothing);
      expect(richContaining("Together they've"), findsNothing);
    });

    testWidgets('one lagging metric (money): soft nudge in its place',
        (tester) async {
      await _pumpWorkshopWithBrief(
        tester,
        BriefPayload()
          ..problemsSolvedCount = 3
          ..problemsPotentialCount = 8
          ..hoursTogether = 6
          ..co2AvoidedPounds = 1764,
      );
      expect(richContaining('No tools shared yet'), findsWidgets);
      // The $0 clause itself is never rendered.
      expect(richContaining('saved \$'), findsNothing);
    });
  });
}

/// Pumps the Workshop screen with a single community whose brief is [brief],
/// for asserting on the assembled metrics sentence.
Future<void> _pumpWorkshopWithBrief(
  WidgetTester tester,
  BriefPayload brief,
) async {
  final cacheManager = StashCacheManager();
  await cacheManager.initialize();
  final mockService = MockWorkshopService();
  when(mockService.getWorkshopBrief(
    communityIds: anyNamed('communityIds'),
  )).thenAnswer((_) async => GetWorkshopBriefResponse()..brief = brief);
  when(mockService.getWorkshopSynthesis(
    communityIds: anyNamed('communityIds'),
  )).thenAnswer((_) async => GetWorkshopSynthesisResponse());

  tester.view.physicalSize = const Size(1170, 4200);
  tester.view.devicePixelRatio = 3.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);

  await tester.pumpWidget(ProviderScope(
    overrides: [
      cacheManagerProvider.overrideWithValue(cacheManager),
      workshopServiceProvider.overrideWithValue(mockService),
      communitiesProvider.overrideWith(
        () => _SeededCommunitiesNotifier([
          CommunityItem()
            ..id = 'comm-1'
            ..name = 'Test Circle',
        ]),
      ),
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
  ));
  await tester.pumpAndSettle();
}
