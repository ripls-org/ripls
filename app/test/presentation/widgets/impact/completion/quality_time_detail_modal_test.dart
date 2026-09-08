import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' as pb_common;
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/presentation/widgets/impact/completion/impact_edit_chip.dart';
import 'package:ripls/presentation/widgets/impact/completion/impact_select_chip.dart';
import 'package:ripls/presentation/widgets/impact/completion/quality_time_detail_modal.dart';
import 'package:ripls/services/providers.dart';

import '../../../../helpers/l10n_helpers.dart';
import 'quality_time_detail_modal_test.mocks.dart';

@GenerateMocks([ImpactMetricsRepository])
void main() {
  group('QualityTimeDetailModal', () {
    late MockImpactMetricsRepository mockRepo;

    ImpactEstimate makeDraft() {
      return ImpactEstimate(
        qualityTime: QualityTimeEstimate(
          qualityTimeMinutes: pb_common.Estimate(mean: 45),
          attributes: QualityTimeAttributes(
            estimatedDurationMinutes: 30,
            groupSize: 4,
            modality: SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED,
            tieStrength: SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE,
            reciprocity: SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL,
            novelty: SocialNovelty.SOCIAL_NOVELTY_INFREQUENT,
            vulnerability:
                SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM,
          ),
        ),
      );
    }

    setUp(() {
      mockRepo = MockImpactMetricsRepository();
      when(mockRepo.draftImpact(experienceId: anyNamed('experienceId')))
          .thenAnswer((_) async => makeDraft());
      when(
        mockRepo.redraftImpact(
          experienceId: anyNamed('experienceId'),
          requestId: anyNamed('requestId'),
          qualityTimeInput: anyNamed('qualityTimeInput'),
          moneySavingsInput: anyNamed('moneySavingsInput'),
          emissionsInput: anyNamed('emissionsInput'),
        ),
      ).thenAnswer((_) async => makeDraft());
    });

    Future<void> pumpModal(WidgetTester tester) async {
      tester.view.physicalSize = const Size(1080, 2400);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            impactMetricsRepositoryProvider.overrideWithValue(mockRepo),
          ],
          child: localizedApp(
            const Scaffold(
              body: _QtHarness(targetId: 'exp1'),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
    }

    testWidgets('Calculation tab renders attribute rows as text only', (
      tester,
    ) async {
      await pumpModal(tester);

      expect(find.byType(ImpactEditChip), findsNothing);
      expect(find.byType(ImpactSelectChip), findsNothing);
    });

    testWidgets(
        'Inputs tab exposes EditChips (duration, group size) and SelectChips for the tier attributes',
        (tester) async {
      await pumpModal(tester);

      await tester.tap(find.text('Inputs'));
      await tester.pumpAndSettle();

      // Two EditChips (duration + group size) and five SelectChips
      // (modality, tie strength, reciprocity, novelty, vulnerability).
      expect(find.byType(ImpactEditChip), findsNWidgets(2));
      expect(
        find.byWidgetPredicate((w) => w.runtimeType.toString().startsWith('ImpactSelectChip<')),
        findsNWidgets(5),
      );
    });

    testWidgets('Inputs tab has no Save button — saves are automatic', (
      tester,
    ) async {
      await pumpModal(tester);

      await tester.tap(find.text('Inputs'));
      await tester.pumpAndSettle();

      expect(find.text('Save'), findsNothing);
      expect(find.text('Reset'), findsOneWidget);
    });

    testWidgets('Calculation tab renders multiplier weights, not labels', (
      tester,
    ) async {
      await pumpModal(tester);

      // Modality IN_PERSON_SHARED → 1.0× ; tie strength ACTIVE → 1.1× ;
      // novelty INFREQUENT → 1.0× ; vulnerability MEDIUM → 1.0× .
      // We expect at least one "× 1.1" rendered for tie strength.
      expect(find.textContaining('×'), findsWidgets);
      // The previous label-based render (e.g., "Friend") should no longer appear.
      expect(find.text('Friend'), findsNothing);
      expect(find.text('Side-by-side'), findsNothing);
    });

    testWidgets('editing duration input auto-saves after debounce', (
      tester,
    ) async {
      await pumpModal(tester);

      await tester.tap(find.text('Inputs'));
      await tester.pumpAndSettle();

      // Edit the first EditChip (base duration in minutes).
      await tester.enterText(find.byType(TextField).first, '90');
      await tester.pump(const Duration(milliseconds: 300));
      await tester.pumpAndSettle();

      verify(
        mockRepo.redraftImpact(
          experienceId: 'exp1',
          requestId: null,
          qualityTimeInput: anyNamed('qualityTimeInput'),
          moneySavingsInput: null,
          emissionsInput: null,
        ),
      ).called(1);
    });
  });
}

class _QtHarness extends ConsumerStatefulWidget {
  const _QtHarness({required this.targetId});

  final String targetId;

  @override
  ConsumerState<_QtHarness> createState() => _QtHarnessState();
}

class _QtHarnessState extends ConsumerState<_QtHarness> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref
          .read(impactDraftProvider(widget.targetId).notifier)
          .draftExperience();
    });
  }

  @override
  Widget build(BuildContext context) {
    return QualityTimeDetailModal(
      targetId: widget.targetId,
      isExperience: true,
    );
  }
}
