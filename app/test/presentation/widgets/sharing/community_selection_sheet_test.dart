import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/sharing/community_selection_sheet.dart';
import 'package:ripls/services/providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../../core/observability/analytics_test_helper.dart';

void main() {
  late MockObservabilityService mockObservabilityService;

  setUp(() {
    mockObservabilityService = MockObservabilityService();
    // communitiesProvider.setCommunities writes to SharedPreferences.
    SharedPreferences.setMockInitialValues({});
  });

  Future<void> setPhoneViewport(WidgetTester tester) async {
    await tester.binding.setSurfaceSize(const Size(400, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
  }

  CommunityItem makeCommunity(String id, String name) {
    return CommunityItem(id: id, name: name);
  }

  Future<void> pumpOpener(
    WidgetTester tester, {
    required Widget Function(BuildContext) onPress,
    List<CommunityItem> communities = const [],
  }) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          observabilityServiceProvider.overrideWithValue(
            mockObservabilityService,
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
          home: Consumer(
            builder: (context, ref, _) {
              // Seed the communities provider with the test communities
              // synchronously so the sheet sees them on first build.
              WidgetsBinding.instance.addPostFrameCallback((_) {
                ref
                    .read(communitiesProvider.notifier)
                    .setCommunities(communities);
              });
              return Scaffold(
                body: Builder(
                  builder: (innerContext) => onPress(innerContext),
                ),
              );
            },
          ),
        ),
      ),
    );
    // Settle the post-frame callback that seeds communities.
    await tester.pumpAndSettle();
  }

  group('CommunitySelectionSheet — deferred mode', () {
    testWidgets('returns selected community IDs on Confirm', (tester) async {
      await setPhoneViewport(tester);
      List<String>? result;

      await pumpOpener(
        tester,
        communities: [
          makeCommunity('c1', 'Alpha'),
          makeCommunity('c2', 'Beta'),
        ],
        onPress: (context) => ElevatedButton(
          onPressed: () async {
            result = await CommunitySelectionSheet.showForDeferred(
              context,
              source: CommunitySelectionSource.gearCreate,
            );
          },
          child: const Text('open'),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      // Tap the toggle on the first community tile via its key.
      await tester.tap(find.byKey(const Key('community_selection_tile_c1')));
      await tester.pumpAndSettle();

      // Confirm.
      await tester.tap(find.text('Confirm'));
      await tester.pumpAndSettle();

      expect(result, ['c1']);
    });

    testWidgets('Cancel returns null', (tester) async {
      await setPhoneViewport(tester);
      List<String>? result = ['sentinel'];

      await pumpOpener(
        tester,
        communities: [makeCommunity('c1', 'Alpha')],
        onPress: (context) => ElevatedButton(
          onPressed: () async {
            result = await CommunitySelectionSheet.showForDeferred(
              context,
              source: CommunitySelectionSource.gearCreate,
            );
          },
          child: const Text('open'),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();

      expect(result, isNull);
    });

    testWidgets('Confirm disabled when nothing selected', (tester) async {
      await setPhoneViewport(tester);
      await pumpOpener(
        tester,
        communities: [makeCommunity('c1', 'Alpha')],
        onPress: (context) => ElevatedButton(
          onPressed: () => CommunitySelectionSheet.showForDeferred(
            context,
            source: CommunitySelectionSource.gearCreate,
          ),
          child: const Text('open'),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      // Confirm tap should be a no-op — sheet stays up.
      await tester.tap(find.text('Confirm'));
      await tester.pumpAndSettle();
      expect(find.byType(CommunitySelectionSheet), findsOneWidget);
    });
  });

  group('CommunitySelectionSheet — invite mode', () {
    testWidgets('tapping a row pops with that community ID', (tester) async {
      await setPhoneViewport(tester);
      String? picked;

      await pumpOpener(
        tester,
        communities: [
          makeCommunity('c1', 'Alpha'),
          makeCommunity('c2', 'Beta'),
        ],
        onPress: (context) => ElevatedButton(
          onPressed: () async {
            picked = await CommunitySelectionSheet.showForInvite(
              context,
              initialSelectedId: 'c1',
            );
          },
          child: const Text('open'),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      // No Confirm button in invite mode — tapping a tile pops directly.
      expect(find.text('Confirm'), findsNothing);

      await tester.tap(find.byKey(const Key('community_selection_tile_c2')));
      await tester.pumpAndSettle();

      expect(picked, 'c2');
    });

    // Regression for #2001: invite mode is single-select, so rows must
    // render the decorative radio indicator rather than a Switch. Pairs
    // with the deferred-mode test below to lock in the visual contract.
    testWidgets('rows render no Switch (radio indicator instead)',
        (tester) async {
      await setPhoneViewport(tester);
      await pumpOpener(
        tester,
        communities: [makeCommunity('c1', 'Alpha')],
        onPress: (context) => ElevatedButton(
          onPressed: () => CommunitySelectionSheet.showForInvite(context),
          child: const Text('open'),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      expect(find.byType(Switch), findsNothing);
    });
  });

  group('CommunitySelectionSheet — deferred mode visual contract', () {
    // The additive-invite redesign (#2492) switched deferred multi-select
    // from a Switch to the multiCheck (checkmark) indicator, matching the
    // invite-community picker. The Switch now belongs only to the
    // manage-access editor. Guards against a regression back to Switch.
    testWidgets('rows render no Switch (multiCheck indicator instead)',
        (tester) async {
      await setPhoneViewport(tester);
      await pumpOpener(
        tester,
        communities: [makeCommunity('c1', 'Alpha')],
        onPress: (context) => ElevatedButton(
          onPressed: () => CommunitySelectionSheet.showForDeferred(
            context,
            source: CommunitySelectionSource.gearCreate,
          ),
          child: const Text('open'),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      expect(find.byType(Switch), findsNothing);
      // The multi-select row still renders and stays tappable.
      expect(
        find.byKey(const Key('community_selection_tile_c1')),
        findsOneWidget,
      );
    });
  });

  group('CommunitySelectionSheet — empty state', () {
    testWidgets('renders empty heading when no communities exist',
        (tester) async {
      await setPhoneViewport(tester);
      await pumpOpener(
        tester,
        communities: const [],
        onPress: (context) => ElevatedButton(
          onPressed: () => CommunitySelectionSheet.showForDeferred(
            context,
            source: CommunitySelectionSource.gearCreate,
          ),
          child: const Text('open'),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      expect(find.text('No Communities Yet'), findsOneWidget);
    });
  });

  group('CommunitySelectionSheet — analytics', () {
    testWidgets('emits CommunitySelectionOpenedEvent with source on open',
        (tester) async {
      await setPhoneViewport(tester);
      await pumpOpener(
        tester,
        communities: [makeCommunity('c1', 'Alpha')],
        onPress: (context) => ElevatedButton(
          onPressed: () => CommunitySelectionSheet.showForDeferred(
            context,
            source: CommunitySelectionSource.experienceCreate,
          ),
          child: const Text('open'),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      expect(mockObservabilityService.loggedEvents, hasLength(1));
      final event = mockObservabilityService.loggedEvents.single;
      expect(event, isA<CommunitySelectionOpenedEvent>());
      expect(
        (event as CommunitySelectionOpenedEvent).source,
        CommunitySelectionSource.experienceCreate,
      );
    });

    testWidgets('invite mode emits source = invite', (tester) async {
      await setPhoneViewport(tester);
      await pumpOpener(
        tester,
        communities: [makeCommunity('c1', 'Alpha')],
        onPress: (context) => ElevatedButton(
          onPressed: () => CommunitySelectionSheet.showForInvite(context),
          child: const Text('open'),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      expect(mockObservabilityService.loggedEvents, hasLength(1));
      final event = mockObservabilityService.loggedEvents.single
          as CommunitySelectionOpenedEvent;
      expect(event.source, CommunitySelectionSource.invite);
    });
  });
}
