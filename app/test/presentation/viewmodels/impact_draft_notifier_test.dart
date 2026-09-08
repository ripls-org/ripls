import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/services/providers.dart';

import 'impact_draft_notifier_test.mocks.dart';

@GenerateMocks([ImpactMetricsRepository])
void main() {
  group('ImpactDraftNotifier', () {
    late MockImpactMetricsRepository mockRepo;

    ProviderContainer makeContainer() {
      return ProviderContainer(
        overrides: [
          impactMetricsRepositoryProvider.overrideWithValue(mockRepo),
        ],
      );
    }

    setUp(() {
      mockRepo = MockImpactMetricsRepository();
    });

    // ── draftExperience ────────────────────────────────────────────────────

    test('draftExperience sets isLoading=true then populates draft on success', () async {
      final impact = ImpactEstimate(moneySaved: MoneySavings());
      when(mockRepo.draftImpact(experienceId: 'exp1'))
          .thenAnswer((_) async => impact);

      final container = makeContainer();
      addTearDown(container.dispose);

      final notifier = container.read(impactDraftProvider('exp1').notifier);
      final future = notifier.draftExperience();

      // isLoading should be true immediately
      expect(container.read(impactDraftProvider('exp1')).isLoading, isTrue);

      await future;

      final state = container.read(impactDraftProvider('exp1'));
      expect(state.isLoading, isFalse);
      expect(state.draft, impact);
      expect(state.error, isNull);
    });

    test('draftExperience sets errorMessage on service failure', () async {
      when(mockRepo.draftImpact(experienceId: 'exp1'))
          .thenThrow(Exception('network error'));

      final container = makeContainer();
      addTearDown(container.dispose);

      await container.read(impactDraftProvider('exp1').notifier).draftExperience();

      final state = container.read(impactDraftProvider('exp1'));
      expect(state.isLoading, isFalse);
      expect(state.draft, isNull);
      expect(state.error, isNotNull);
    });

    test('draftRequest sets isLoading=true then populates draft on success', () async {
      final impact = ImpactEstimate();
      when(mockRepo.draftImpact(requestId: 'req1'))
          .thenAnswer((_) async => impact);

      final container = makeContainer();
      addTearDown(container.dispose);

      await container.read(impactDraftProvider('req1').notifier).draftRequest();

      final state = container.read(impactDraftProvider('req1'));
      expect(state.isLoading, isFalse);
      expect(state.draft, impact);
    });

    // ── applyQualityTimeOverrides ──────────────────────────────────────────

    test('applyQualityTimeOverrides sets isRedrafting=true and merges draft on success', () async {
      final initial = ImpactEstimate(
        qualityTime: QualityTimeEstimate(),
      );
      final redrafted = ImpactEstimate(
        qualityTime: QualityTimeEstimate(),
        moneySaved: MoneySavings(),
      );
      when(mockRepo.draftImpact(experienceId: 'exp1'))
          .thenAnswer((_) async => initial);
      final overrides = QualityTimeAttributes(estimatedDurationMinutes: 30);
      when(
        mockRepo.redraftImpact(
          experienceId: 'exp1',
          qualityTimeInput: overrides,
        ),
      ).thenAnswer((_) async => redrafted);

      final container = makeContainer();
      addTearDown(container.dispose);

      await container.read(impactDraftProvider('exp1').notifier).draftExperience();

      final future = container
          .read(impactDraftProvider('exp1').notifier)
          .applyQualityTimeOverrides(overrides, isExperience: true);

      expect(container.read(impactDraftProvider('exp1')).isRedrafting, isTrue);

      await future;

      final state = container.read(impactDraftProvider('exp1'));
      expect(state.isRedrafting, isFalse);
      expect(state.draft, redrafted);
      expect(state.qualityTimeOverrides, overrides);
    });

    // ── resetOverrides ─────────────────────────────────────────────────────

    test('resetOverrides clears overrides and redrafts without LLM', () async {
      final initial = ImpactEstimate();
      final afterReset = ImpactEstimate(moneySaved: MoneySavings());
      when(mockRepo.draftImpact(experienceId: 'exp1'))
          .thenAnswer((_) async => initial);
      when(mockRepo.redraftImpact(experienceId: 'exp1'))
          .thenAnswer((_) async => afterReset);

      final container = makeContainer();
      addTearDown(container.dispose);

      await container.read(impactDraftProvider('exp1').notifier).draftExperience();
      await container
          .read(impactDraftProvider('exp1').notifier)
          .resetOverrides(isExperience: true);

      final state = container.read(impactDraftProvider('exp1'));
      expect(state.qualityTimeOverrides, isNull);
      expect(state.moneySavingsOverrides, isNull);
      expect(state.emissionsOverrides, isNull);
      expect(state.draft, afterReset);
      // LLM draft method called only once (for the initial draft, not reset)
      verify(mockRepo.draftImpact(experienceId: 'exp1')).called(1);
    });

    // ── disposal safety ────────────────────────────────────────────────────

    test('disposal mid-flight draftExperience does not throw', () async {
      final completer = Completer<ImpactEstimate>();
      when(mockRepo.draftImpact(experienceId: 'exp1'))
          .thenAnswer((_) async => completer.future);

      final container = makeContainer();
      final notifier = container.read(impactDraftProvider('exp1').notifier);
      final future = notifier.draftExperience();

      // Dispose before the async completes
      container.dispose();
      completer.complete(ImpactEstimate());

      // Should not throw
      await expectLater(future, completes);
    });

    // ── state isolation across family instances ────────────────────────────

    test('overrides on one target do not bleed into another', () async {
      when(mockRepo.draftImpact(experienceId: 'exp1'))
          .thenAnswer((_) async => ImpactEstimate());
      when(mockRepo.draftImpact(experienceId: 'exp2'))
          .thenAnswer((_) async => ImpactEstimate());
      when(
        mockRepo.redraftImpact(
          experienceId: 'exp1',
          qualityTimeInput: anyNamed('qualityTimeInput'),
        ),
      ).thenAnswer((_) async => ImpactEstimate());

      final container = makeContainer();
      addTearDown(container.dispose);

      await container.read(impactDraftProvider('exp1').notifier).draftExperience();
      await container.read(impactDraftProvider('exp2').notifier).draftExperience();

      await container
          .read(impactDraftProvider('exp1').notifier)
          .applyQualityTimeOverrides(
            QualityTimeAttributes(estimatedDurationMinutes: 60),
            isExperience: true,
          );

      expect(
        container.read(impactDraftProvider('exp1')).qualityTimeOverrides,
        isNotNull,
      );
      expect(
        container.read(impactDraftProvider('exp2')).qualityTimeOverrides,
        isNull,
      );
    });

    // ── applyExternalDraft ─────────────────────────────────────────────────

    test('applyExternalDraft writes the impact into draft when no overrides are set', () {
      final container = makeContainer();
      addTearDown(container.dispose);

      // Pre-touch to instantiate the notifier so subsequent reads see updates.
      container.read(impactDraftProvider('exp1'));

      final livePreview = ImpactEstimate(
        qualityTime: QualityTimeEstimate(
          attributes: QualityTimeAttributes(groupSize: 3),
        ),
      );

      container
          .read(impactDraftProvider('exp1').notifier)
          .applyExternalDraft(livePreview);

      final draft = container.read(impactDraftProvider('exp1')).draft;
      expect(draft, isNotNull);
      expect(draft!.qualityTime.attributes.groupSize, 3);
    });

    test('applyExternalDraft no-ops when the user has applied a QT override', () async {
      final initial = ImpactEstimate(
        qualityTime: QualityTimeEstimate(
          attributes: QualityTimeAttributes(groupSize: 5),
        ),
      );
      when(
        mockRepo.redraftImpact(
          experienceId: 'exp1',
          requestId: null,
          qualityTimeInput: anyNamed('qualityTimeInput'),
          moneySavingsInput: null,
          emissionsInput: null,
        ),
      ).thenAnswer((_) async => initial);

      final container = makeContainer();
      addTearDown(container.dispose);

      // Simulate user manually overriding group_size to 5 via the detail modal.
      await container
          .read(impactDraftProvider('exp1').notifier)
          .applyQualityTimeOverrides(
            QualityTimeAttributes(groupSize: 5),
            isExperience: true,
          );

      // A live preview from attendance toggles arrives with a different group size.
      final livePreview = ImpactEstimate(
        qualityTime: QualityTimeEstimate(
          attributes: QualityTimeAttributes(groupSize: 3),
        ),
      );
      container
          .read(impactDraftProvider('exp1').notifier)
          .applyExternalDraft(livePreview);

      // The user's override (5) must remain authoritative in the draft.
      final draft = container.read(impactDraftProvider('exp1')).draft;
      expect(draft!.qualityTime.attributes.groupSize, 5,
          reason: 'live preview must not clobber an explicit user override');
    });

    test('a slow opening draft never overwrites a preview that already landed',
        () async {
      // The opening draft and the confirmed-set preview are both in flight
      // when the modal opens; this one resolves last.
      final opening = Completer<ImpactEstimate>();
      when(mockRepo.draftImpact(requestId: 'req1'))
          .thenAnswer((_) => opening.future);

      final container = makeContainer();
      addTearDown(container.dispose);
      final notifier = container.read(impactDraftProvider('req1').notifier);

      final draftFuture = notifier.draftRequest();
      notifier.applyExternalDraft(
        ImpactEstimate(
          qualityTime: QualityTimeEstimate(
            attributes: QualityTimeAttributes(groupSize: 6),
          ),
        ),
      );
      opening.complete(
        ImpactEstimate(
          qualityTime: QualityTimeEstimate(
            attributes: QualityTimeAttributes(groupSize: 2),
          ),
        ),
      );
      await draftFuture;

      final state = container.read(impactDraftProvider('req1'));
      expect(
        state.draft!.qualityTime.attributes.groupSize,
        6,
        reason: 'the preview is what the commit persists; the generic '
            'opening draft must not win by resolving later',
      );
      expect(state.isLoading, isFalse);
    });
  });
}
