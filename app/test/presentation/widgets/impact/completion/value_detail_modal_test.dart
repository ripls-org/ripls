import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' as pb_common;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/presentation/widgets/impact/completion/impact_edit_chip.dart';
import 'package:ripls/presentation/widgets/impact/completion/value_detail_modal.dart';
import 'package:ripls/services/providers.dart';

import '../../../../helpers/l10n_helpers.dart';
import 'value_detail_modal_test.mocks.dart';

@GenerateMocks([ImpactMetricsRepository])
void main() {
  group('ValueDetailModal', () {
    late MockImpactMetricsRepository mockRepo;

    ImpactEstimate makeDraft({double hireValue = 50}) {
      return ImpactEstimate(
        moneySaved: MoneySavings(
          valueUsd: pb_common.Estimate(mean: hireValue),
          inputs: MoneySavingsInput(
            hireEquivalentValue: pb_common.Estimate(mean: hireValue),
          ),
        ),
      );
    }

    setUp(() {
      mockRepo = MockImpactMetricsRepository();
      when(mockRepo.draftImpact(experienceId: anyNamed('experienceId')))
          .thenAnswer((_) async => makeDraft(hireValue: 50));
      when(
        mockRepo.redraftImpact(
          experienceId: anyNamed('experienceId'),
          requestId: anyNamed('requestId'),
          qualityTimeInput: anyNamed('qualityTimeInput'),
          moneySavingsInput: anyNamed('moneySavingsInput'),
          emissionsInput: anyNamed('emissionsInput'),
        ),
      ).thenAnswer((_) async => makeDraft(hireValue: 50));
    });

    Future<void> pumpModal(WidgetTester tester) async {
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            impactMetricsRepositoryProvider.overrideWithValue(mockRepo),
          ],
          child: localizedApp(
            const Scaffold(
              body: _DraftHarness(targetId: 'exp1'),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
    }

    testWidgets('Calculation tab renders composite as Text, not as input', (
      tester,
    ) async {
      await pumpModal(tester);

      // Calculation tab is the default — composite shows as text.
      expect(find.text('\$50'), findsWidgets);
      expect(find.byType(ImpactEditChip), findsNothing);
    });

    testWidgets('Inputs tab exposes exactly one EditChip for the input', (
      tester,
    ) async {
      await pumpModal(tester);

      await tester.tap(find.text('Inputs'));
      await tester.pumpAndSettle();

      // Exactly one editable input — the hire-equivalent value.
      // The composite Money Saved row stays as Text (INV-COMPOSITE-DERIVED).
      expect(find.byType(ImpactEditChip), findsOneWidget);
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

      // Enter a new value into the hire-equivalent EditChip's TextField.
      await tester.enterText(find.byType(TextField), '99');
      // Wait past the 250ms auto-save debounce.
      await tester.pump(const Duration(milliseconds: 300));
      await tester.pumpAndSettle();

      verify(
        mockRepo.redraftImpact(
          experienceId: 'exp1',
          requestId: null,
          qualityTimeInput: null,
          moneySavingsInput: anyNamed('moneySavingsInput'),
          emissionsInput: null,
        ),
      ).called(1);
    });

    testWidgets('Reset clears overrides via redraftImpact', (tester) async {
      await pumpModal(tester);

      await tester.tap(find.text('Inputs'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Reset'));
      await tester.pumpAndSettle();

      // Reset triggers redraft with all overrides nulled.
      verify(
        mockRepo.redraftImpact(
          experienceId: 'exp1',
          requestId: null,
          qualityTimeInput: null,
          moneySavingsInput: null,
          emissionsInput: null,
        ),
      ).called(1);
    });

    testWidgets('Reset re-seeds the EditChip from the AI-drafted value',
        (tester) async {
      // Initial draft has hireValue=50 (the AI suggestion). Redraft (Reset)
      // returns the same AI-drafted value — the override is gone.
      when(mockRepo.draftImpact(experienceId: 'exp1'))
          .thenAnswer((_) async => makeDraft(hireValue: 50));
      when(
        mockRepo.redraftImpact(
          experienceId: anyNamed('experienceId'),
          requestId: anyNamed('requestId'),
          qualityTimeInput: anyNamed('qualityTimeInput'),
          moneySavingsInput: anyNamed('moneySavingsInput'),
          emissionsInput: anyNamed('emissionsInput'),
        ),
      ).thenAnswer((inv) async {
        final money = inv.namedArguments[#moneySavingsInput] as MoneySavings?;
        // When called with overrides, return the override value. When
        // overrides are null (Reset), return the AI-drafted 50.
        final override = money?.inputs.hireEquivalentValue.mean;
        return makeDraft(hireValue: override ?? 50);
      });

      await pumpModal(tester);

      // Switch to Inputs tab and override the value to 250.
      await tester.tap(find.text('Inputs'));
      await tester.pumpAndSettle();

      await tester.enterText(find.byType(TextField), '250');
      await tester.pump(const Duration(milliseconds: 320));
      await tester.pumpAndSettle();

      // The chip now shows 250 (override applied).
      expect(find.text('250'), findsOneWidget);

      // Reset: chip must drop back to the AI-drafted 50.
      await tester.tap(find.text('Reset'));
      await tester.pumpAndSettle();

      expect(
        find.text('50'),
        findsOneWidget,
        reason: 'Reset must re-seed the input chip from the AI-drafted value, '
            'not just the composite at the top',
      );
      expect(find.text('250'), findsNothing,
          reason: 'the overridden value must be cleared from the input chip');
    });
  });
}

/// _DraftHarness pumps the modal after triggering an initial draft load.
/// This avoids autoDispose family teardown that would happen if we read
/// the notifier outside a widget subscription window.
class _DraftHarness extends ConsumerStatefulWidget {
  const _DraftHarness({required this.targetId});

  final String targetId;

  @override
  ConsumerState<_DraftHarness> createState() => _DraftHarnessState();
}

class _DraftHarnessState extends ConsumerState<_DraftHarness> {
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
    return ValueDetailModal(targetId: widget.targetId, isExperience: true);
  }
}
