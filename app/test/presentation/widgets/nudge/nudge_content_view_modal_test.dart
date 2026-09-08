import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/config/feature_flags.dart';
import 'package:ripls/data/repositories/workshop_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/create/unified_create_modal.dart';
import 'package:ripls/presentation/screens/experience/experience_preview_modal.dart';
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_content_view.dart';
import 'package:ripls/services/feed_service.dart' show NudgePayload;
import 'package:ripls/services/providers/workshop_providers.dart';

class _FakeUnifiedCreateVm extends UnifiedCreateViewModel {
  @override
  UnifiedCreateState build() {
    super.build();
    return const UnifiedCreateState();
  }
}

class _FakeWorkshopRepo implements WorkshopRepository {
  final bool throwError;
  final bool emptyName;

  /// Round-trip latency. The default of zero completes within the same
  /// microtask drain as the tap, which is NOT how the real RPC behaves —
  /// tests about what happens to the tree *during* the request must set it,
  /// or the widget under test never gets a chance to rebuild first.
  final Duration latency;

  const _FakeWorkshopRepo({
    this.throwError = false,
    this.emptyName = true,
    this.latency = Duration.zero,
  });

  @override
  Future<GenerateWorkshopDraftResponse> generateDraft({
    required String experienceId,
  }) async {
    if (latency > Duration.zero) await Future<void>.delayed(latency);
    if (throwError) throw Exception('network error');
    final resp = GenerateWorkshopDraftResponse();
    if (!emptyName) resp.name = 'Pre-filled';
    return resp;
  }

  @override
  Future<GetWorkshopBriefResponse> getBrief({
    required List<String> communityIds,
  }) =>
      throw UnimplementedError();

  @override
  Future<GetWorkshopSynthesisResponse> getSynthesis({
    required List<String> communityIds,
  }) =>
      throw UnimplementedError();

  @override
  Future<void> invalidateBrief({List<String>? communityIds}) =>
      throw UnimplementedError();

  @override
  Future<void> invalidateAll() => throw UnimplementedError();

  @override
  Future<GetCategoryDetailResponse> getCategoryDetail({
    required KnownForMode mode,
    String? ownerId,
    required List<String> communityIds,
    required String category,
  }) =>
      throw UnimplementedError();

  @override
  Future<void> invalidateCategoryDetail({
    required KnownForMode mode,
    String? ownerId,
    required List<String> communityIds,
    required String category,
  }) =>
      throw UnimplementedError();

  @override
  Future<HideKnownForCategoryResponse> hideKnownForCategory({
    required SuppressionScope scopeKind,
    required String scopeId,
    required String category,
    List<String> invalidateCommunityIds = const <String>[],
    String? invalidateOwnerId,
    required KnownForMode invalidateMode,
  }) =>
      throw UnimplementedError();

  @override
  Future<UndoHideKnownForCategoryResponse> undoHideKnownForCategory({
    required String suppressionId,
  }) =>
      throw UnimplementedError();

  @override
  Future<PermanentlyRemoveKnownForCategoryResponse>
      permanentlyRemoveKnownForCategory({
    required SuppressionScope scopeKind,
    required String scopeId,
    required String category,
    List<String> invalidateCommunityIds = const <String>[],
    String? invalidateOwnerId,
    required KnownForMode invalidateMode,
  }) =>
      throw UnimplementedError();
}


/// The Home inbox dismisses its nudge optimistically the moment the CTA is
/// tapped, so by the time the draft round trip returns the card is gone from
/// the tree. This host reproduces that: [NudgeContentView.onCtaTap] removes it.
class _DismissingHost extends StatefulWidget {
  const _DismissingHost({required this.nudge});
  final NudgePayload nudge;
  @override
  State<_DismissingHost> createState() => _DismissingHostState();
}

class _DismissingHostState extends State<_DismissingHost> {
  bool _dismissed = false;
  @override
  Widget build(BuildContext context) {
    if (_dismissed) return const SizedBox.shrink();
    return NudgeContentView(
      nudge: widget.nudge,
      onCtaTap: () => setState(() => _dismissed = true),
    );
  }
}

NudgePayload _nudge({
  String ctaAction = 'plan_experience',
  String ctaLabel = 'Tap',
  String? contextId,
}) {
  final n = NudgePayload()
    ..nudgeVariant = 1
    ..headline = 'Headline'
    ..description = 'Body'
    ..ctaLabel = ctaLabel
    ..ctaAction = ctaAction;
  if (contextId != null) n.contextId = contextId;
  return n;
}

Widget _buildApp({
  required bool flagOn,
  required NudgePayload nudge,
  _FakeWorkshopRepo fakeRepo = const _FakeWorkshopRepo(),
}) {
  return ProviderScope(
    overrides: [
      unifiedCreateEnabledProvider.overrideWithValue(flagOn),
      unifiedCreateViewModelProvider.overrideWith(_FakeUnifiedCreateVm.new),
      workshopRepositoryProvider.overrideWithValue(fakeRepo),
    ],
    child: MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(body: NudgeContentView(nudge: nudge)),
    ),
  );
}

void main() {
  // Direct CTA actions (no workshop draft fetch).
  for (final ctaAction in ['list_item', 'ask_for_help', 'seed_subhost', 'plan_experience']) {
    group('NudgeContentView CTA "$ctaAction"', () {
      testWidgets('opens UnifiedCreateModal when flag is on', (tester) async {
        await tester.pumpWidget(_buildApp(
          flagOn: true,
          nudge: _nudge(ctaAction: ctaAction, ctaLabel: 'Go'),
        ));
        await tester.pump();

        await tester.tap(find.text('Go'));
        await tester.pump();

        expect(find.byType(UnifiedCreateModal), findsOneWidget);
      });

      testWidgets('does not open UnifiedCreateModal when flag is off', (tester) async {
        await tester.pumpWidget(_buildApp(
          flagOn: false,
          nudge: _nudge(ctaAction: ctaAction, ctaLabel: 'Go'),
        ));
        await tester.pump();

        await tester.tap(find.text('Go'));
        await tester.pump();

        // `list_item` routes through openBlankCreateGear, which no
        // longer respects the flag — gear capture has no legacy
        // fallback. Every other CTA still falls back when the flag is
        // off.
        if (ctaAction == 'list_item') {
          expect(find.byType(UnifiedCreateModal), findsOneWidget);
        } else {
          expect(find.byType(UnifiedCreateModal), findsNothing);
        }
      });
    });
  }

  // Workshop draft fallback: empty contextId.
  group('NudgeContentView openRepeatDraft – empty contextId', () {
    testWidgets('opens UnifiedCreateModal when flag is on', (tester) async {
      // No contextId set → experienceId is empty → fallback to openBlankCreate.
      await tester.pumpWidget(_buildApp(
        flagOn: true,
        nudge: _nudge(ctaAction: 'schedule_repeat', ctaLabel: 'Go'),
      ));
      await tester.pump();

      await tester.tap(find.text('Go'));
      await tester.pump();
      await tester.pump();

      expect(find.byType(UnifiedCreateModal), findsOneWidget);
    });

    testWidgets('does not open UnifiedCreateModal when flag is off', (tester) async {
      await tester.pumpWidget(_buildApp(
        flagOn: false,
        nudge: _nudge(ctaAction: 'schedule_repeat', ctaLabel: 'Go'),
      ));
      await tester.pump();

      await tester.tap(find.text('Go'));
      await tester.pumpAndSettle();

      expect(find.byType(UnifiedCreateModal), findsNothing);
    });
  });

  // Workshop draft fallback: generateDraft RPC error.
  group('NudgeContentView openRepeatDraft – RPC error', () {
    testWidgets('opens UnifiedCreateModal when flag is on', (tester) async {
      await tester.pumpWidget(_buildApp(
        flagOn: true,
        nudge: _nudge(ctaAction: 'schedule_repeat', ctaLabel: 'Go', contextId: 'exp-1'),
        fakeRepo: const _FakeWorkshopRepo(throwError: true),
      ));
      await tester.pump();

      await tester.tap(find.text('Go'));
      await tester.pump();
      await tester.pump();

      expect(find.byType(UnifiedCreateModal), findsOneWidget);
    });

    testWidgets('does not open UnifiedCreateModal when flag is off', (tester) async {
      await tester.pumpWidget(_buildApp(
        flagOn: false,
        nudge: _nudge(ctaAction: 'schedule_repeat', ctaLabel: 'Go', contextId: 'exp-1'),
        fakeRepo: const _FakeWorkshopRepo(throwError: true),
      ));
      await tester.pump();

      await tester.tap(find.text('Go'));
      await tester.pumpAndSettle();

      expect(find.byType(UnifiedCreateModal), findsNothing);
    });
  });

  // Workshop draft fallback: draft returned with empty name.
  group('NudgeContentView openRepeatDraft – empty draft name', () {
    testWidgets('opens UnifiedCreateModal when flag is on', (tester) async {
      await tester.pumpWidget(_buildApp(
        flagOn: true,
        nudge: _nudge(ctaAction: 'revive_experience', ctaLabel: 'Go', contextId: 'exp-2'),
        fakeRepo: const _FakeWorkshopRepo(emptyName: true),
      ));
      await tester.pump();

      await tester.tap(find.text('Go'));
      await tester.pump();
      await tester.pump();

      expect(find.byType(UnifiedCreateModal), findsOneWidget);
    });

    testWidgets('does not open UnifiedCreateModal when flag is off', (tester) async {
      await tester.pumpWidget(_buildApp(
        flagOn: false,
        nudge: _nudge(ctaAction: 'revive_experience', ctaLabel: 'Go', contextId: 'exp-2'),
        fakeRepo: const _FakeWorkshopRepo(emptyName: true),
      ));
      await tester.pump();

      await tester.tap(find.text('Go'));
      await tester.pumpAndSettle();

      expect(find.byType(UnifiedCreateModal), findsNothing);
    });
  });

  // The inbox's optimistic dismiss unmounts the nudge card while the draft
  // request is in flight. Resolving the navigator up front is what keeps the
  // tap from silently opening nothing (the runclub reel caught this live).
  group('NudgeContentView openRepeatDraft – dismissed while in flight', () {
    testWidgets('still opens the pre-filled preview', (tester) async {
      await tester.pumpWidget(ProviderScope(
        overrides: [
          unifiedCreateEnabledProvider.overrideWithValue(true),
          unifiedCreateViewModelProvider.overrideWith(_FakeUnifiedCreateVm.new),
          workshopRepositoryProvider.overrideWithValue(
            const _FakeWorkshopRepo(
              emptyName: false,
              latency: Duration(milliseconds: 50),
            ),
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
          home: Scaffold(
            body: _DismissingHost(
              nudge: _nudge(
                ctaAction: 'schedule_repeat',
                ctaLabel: 'Go',
                contextId: 'exp-1',
              ),
            ),
          ),
        ),
      ));
      await tester.pump();

      await tester.tap(find.text('Go'));
      await tester.pump(); // the dismiss lands, unmounting the card
      expect(find.text('Go'), findsNothing, reason: 'card dismissed');
      await tester.pump(const Duration(milliseconds: 100)); // draft resolves
      await tester.pump();

      expect(
        find.byType(ExperiencePreviewModal),
        findsOneWidget,
        reason: 'the draft must survive its host being dismissed',
      );
    });
  });
}
