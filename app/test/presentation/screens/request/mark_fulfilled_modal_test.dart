import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show Estimate;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/screens/request/mark_fulfilled_modal.dart';
import 'package:ripls/presentation/viewmodels/fulfill_modal_view_model.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_state.dart';
import 'package:ripls/services/providers.dart';

import '../../../helpers/l10n_helpers.dart';
import '../../viewmodels/manage_members_view_model_test.mocks.dart'
    show MockCommunityRepository, MockProvisionalUserRepository;

const _requestId = 'req-1';

/// Fake that pins a fixed [FulfillModalState] and no-ops the network
/// initialization so the modal renders in isolation.
class _FakeFulfillNotifier extends FulfillModalNotifier {
  _FakeFulfillNotifier(super.requestId, this._state);
  final FulfillModalState _state;

  @override
  FulfillModalState build() => _state;

  @override
  void initialize(
    List<User> offerers, {
    List<User> contributors = const [],
  }) {}
}

/// Fake impact draft that pins a fixed state and never hits the network.
class _FakeImpactDraftNotifier extends ImpactDraftNotifier {
  _FakeImpactDraftNotifier(super.targetId, this._state);
  final ImpactDraftState _state;

  @override
  ImpactDraftState build() => _state;

  @override
  Future<void> draftRequest() async {}

  @override
  Future<void> draftExperience() async {}
}

Future<void> _pump(
  WidgetTester tester, {
  required FulfillModalState state,
  required ImpactDraftState draftState,
}) async {
  final communityRepo = MockCommunityRepository();
  final provisionalRepo = MockProvisionalUserRepository();
  when(
    communityRepo.searchMembers(
      communityId: anyNamed('communityId'),
      query: anyNamed('query'),
      limit: anyNamed('limit'),
    ),
  ).thenAnswer((_) async => []);
  when(
    provisionalRepo.searchProvisionalUsers(
      communityId: anyNamed('communityId'),
      query: anyNamed('query'),
      limit: anyNamed('limit'),
    ),
  ).thenAnswer((_) async => []);

  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        fulfillModalProvider(
          _requestId,
        ).overrideWith(() => _FakeFulfillNotifier(_requestId, state)),
        impactDraftProvider(
          _requestId,
        ).overrideWith(() => _FakeImpactDraftNotifier(_requestId, draftState)),
        communityRepositoryProvider.overrideWithValue(communityRepo),
        provisionalUserRepositoryProvider.overrideWithValue(provisionalRepo),
      ],
      child: localizedApp(
        Scaffold(
          body: MarkFulfilledModal(
            requestId: _requestId,
            communityId: 'comm-1',
            requestTitle: 'Back-to-school supplies for Room 7',
            ownerId: 'june',
            offerers: [User(id: 'theo', name: 'Theo Alvarez')],
          ),
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

FulfillModalState _state() {
  return FulfillModalState(
    offerers: [User(id: 'theo', name: 'Theo Alvarez')],
    confirmedIds: const {'theo'},
  );
}

void main() {
  group('MarkFulfilledModal ceremony copy (#2724)', () {
    testWidgets('eyebrow reads READY TO FULFILL before the action', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(),
        draftState: ImpactDraftState(
          draft: ImpactEstimate(
            moneySaved: MoneySavings(valueUsd: Estimate(mean: 80)),
          ),
        ),
      );

      // Past tense is reserved for after the CTA is tapped.
      expect(find.text('READY TO FULFILL'), findsOneWidget);
      expect(find.text('FULFILLED'), findsNothing);
      expect(find.text('Mark Fulfilled'), findsOneWidget);
    });

    testWidgets('confirm-list header names the removal affordance', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(),
        draftState: const ImpactDraftState(),
      );

      expect(
        find.text('CONFIRM HELPERS — TAP TO REMOVE'),
        findsOneWidget,
      );
      expect(find.text('TAP TO CONFIRM HELPERS'), findsNothing);
      expect(find.text('Theo Alvarez'), findsOneWidget);
    });
  });

  group('MarkFulfilledModal impact bar (#2724)', () {
    testWidgets('suppresses zero tiles and humanizes quality time', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(),
        draftState: ImpactDraftState(
          draft: ImpactEstimate(
            moneySaved: MoneySavings(valueUsd: Estimate(mean: 0)),
            qualityTime: QualityTimeEstimate(
              qualityTimeMinutes: Estimate(mean: 237),
            ),
          ),
        ),
      );

      // "$0 Saved / 0.0kg CO₂" must never headline the ceremony.
      expect(find.text('\$0'), findsNothing);
      expect(find.text('Saved'), findsNothing);
      expect(find.text('0.0kg'), findsNothing);
      expect(find.text('~4 h'), findsOneWidget);
      expect(find.text('Quality Time'), findsOneWidget);
    });
  });
}
