import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' as pb_common;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/presentation/widgets/impact/completion/co2_detail_modal.dart';
import 'package:ripls/presentation/widgets/impact/completion/impact_edit_chip.dart';
import 'package:ripls/services/providers.dart';

import '../../../../helpers/l10n_helpers.dart';
import 'co2_detail_modal_test.mocks.dart';

@GenerateMocks([ImpactMetricsRepository])
void main() {
  group('Co2DetailModal', () {
    late MockImpactMetricsRepository mockRepo;

    ImpactEstimate makeDraft() {
      return ImpactEstimate(
        emissionsPrevented: PreventedEmissions(
          manufactureAvoidedCarbon: pb_common.CarbonEstimate(
            co2eGrams: pb_common.Estimate(mean: 600),
          ),
          wasteReducedCarbon: pb_common.CarbonEstimate(
            co2eGrams: pb_common.Estimate(mean: 100),
          ),
          inputs: PreventedEmissionsInput(
            travelAvoidedCarbon: pb_common.Estimate(mean: 50),
            repairCreditCarbon: pb_common.Estimate(mean: 0),
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
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            impactMetricsRepositoryProvider.overrideWithValue(mockRepo),
          ],
          child: localizedApp(
            const Scaffold(
              body: _Co2Harness(targetId: 'exp1'),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
    }

    testWidgets('Calculation tab shows manufacture/waste rows as Text', (
      tester,
    ) async {
      await pumpModal(tester);

      // Total = 700 g formatted as "700 g".
      expect(find.text('700 g'), findsWidgets);
      // No EditChips on Calculation tab.
      expect(find.byType(ImpactEditChip), findsNothing);
    });

    testWidgets('Inputs tab exposes EditChips for travel & repair carbon', (
      tester,
    ) async {
      await pumpModal(tester);

      await tester.tap(find.text('Inputs'));
      await tester.pumpAndSettle();

      // Two editable inputs (travel avoided + repair credit). Composite row stays Text.
      expect(find.byType(ImpactEditChip), findsNWidgets(2));
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

    testWidgets('editing input auto-saves after debounce', (tester) async {
      await pumpModal(tester);

      await tester.tap(find.text('Inputs'));
      await tester.pumpAndSettle();

      // Edit the first travel-avoided EditChip.
      await tester.enterText(find.byType(TextField).first, '120');
      await tester.pump(const Duration(milliseconds: 300));
      await tester.pumpAndSettle();

      verify(
        mockRepo.redraftImpact(
          experienceId: 'exp1',
          requestId: null,
          qualityTimeInput: null,
          moneySavingsInput: null,
          emissionsInput: anyNamed('emissionsInput'),
        ),
      ).called(1);
    });
  });
}

class _Co2Harness extends ConsumerStatefulWidget {
  const _Co2Harness({required this.targetId});

  final String targetId;

  @override
  ConsumerState<_Co2Harness> createState() => _Co2HarnessState();
}

class _Co2HarnessState extends ConsumerState<_Co2Harness> {
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
    return Co2DetailModal(targetId: widget.targetId, isExperience: true);
  }
}
