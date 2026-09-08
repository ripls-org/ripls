import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show Estimate;
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' show Experience;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart' show RSVP;
import 'package:ripls/data/gen/ripls/api/experience_service.pbenum.dart'
    show RSVPIntention;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/l10n/app_localizations_en.dart';
import 'package:ripls/presentation/screens/experience/mark_completed_modal.dart';
import 'package:ripls/presentation/viewmodels/event_modal_state.dart';
import 'package:ripls/presentation/viewmodels/event_modal_view_model.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_state.dart';
import 'package:ripls/services/providers.dart';

import '../../../helpers/l10n_helpers.dart';
import '../../viewmodels/manage_members_view_model_test.mocks.dart'
    show MockCommunityRepository, MockProvisionalUserRepository;

const _experienceId = 'exp-1';

/// Fake that pins a fixed [EventModalState] and no-ops the network
/// initialization so the modal renders in isolation.
class _FakeEventModalNotifier extends EventModalNotifier {
  _FakeEventModalNotifier(super.experienceId, this._state);
  final EventModalState _state;

  @override
  EventModalState build() => _state;

  @override
  Future<void> initialize({
    String communityId = '',
    List<User> preTaggedUsers = const [],
    List<ProvisionalUser> preProvisionalUsers = const [],
  }) async {}
}

/// Fake impact draft that pins a fixed state and never hits the network.
class _FakeImpactDraftNotifier extends ImpactDraftNotifier {
  _FakeImpactDraftNotifier(super.targetId, this._state);
  final ImpactDraftState _state;

  @override
  ImpactDraftState build() => _state;

  @override
  Future<void> draftExperience() async {}

  @override
  Future<void> draftRequest() async {}
}

EventModalState _state() {
  return EventModalState(
    isLoading: false,
    experience: Experience(id: _experienceId, name: "Mother's Day Brunch"),
    rsvps: [
      RSVP(
        user: User(id: 'leslie', name: 'Leslie Ito'),
        intention: RSVPIntention.RSVP_INTENTION_YES,
      ),
    ],
    attendanceMap: const {'leslie': true},
  );
}

Future<void> _pump(
  WidgetTester tester, {
  required EventModalState state,
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
        eventModalProvider(
          _experienceId,
        ).overrideWith(() => _FakeEventModalNotifier(_experienceId, state)),
        impactDraftProvider(_experienceId).overrideWith(
          () => _FakeImpactDraftNotifier(_experienceId, draftState),
        ),
        communityRepositoryProvider.overrideWithValue(communityRepo),
        provisionalUserRepositoryProvider.overrideWithValue(provisionalRepo),
      ],
      child: localizedApp(
        const Scaffold(
          body: MarkCompletedModal(
            experienceId: _experienceId,
            communityId: 'comm-1',
          ),
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  group('MarkCompletedModal ceremony copy (#2724)', () {
    testWidgets('eyebrow reads WRAP UP before the action', (tester) async {
      await _pump(
        tester,
        state: _state(),
        draftState: ImpactDraftState(
          draft: ImpactEstimate(
            qualityTime: QualityTimeEstimate(
              qualityTimeMinutes: Estimate(mean: 237),
            ),
          ),
        ),
      );

      // Past tense is reserved for after the CTA is tapped.
      expect(find.text('WRAP UP'), findsOneWidget);
      expect(find.text('COMPLETED'), findsNothing);
      expect(find.text('Complete'), findsOneWidget);
    });

    testWidgets('confirm-list header names the removal affordance', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(),
        draftState: const ImpactDraftState(),
      );

      expect(find.text('CONFIRM WHO CAME — TAP TO REMOVE'), findsOneWidget);
      expect(find.text('TAP TO CONFIRM'), findsNothing);
      expect(find.text('Leslie Ito'), findsOneWidget);
    });
  });

  group('MarkCompletedModal impact bar (#2724)', () {
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

      // The filmed events preview headlined "$0 Saved / 0.0kg CO₂" over a
      // "237 mins" tile; none of those may render (#2724).
      expect(find.text('\$0'), findsNothing);
      expect(find.text('0.0kg'), findsNothing);
      expect(find.textContaining('237'), findsNothing);
      expect(find.text('~4 h'), findsOneWidget);
    });
  });

  group('Completion toast copy (#2724)', () {
    test('toast says "Event wrapped up" — "Experience" is banned in UI', () {
      // The toast fires deep inside the post-completion navigation flow;
      // lock the copy at the l10n layer instead.
      final l10n = AppLocalizationsEn();
      expect(l10n.eventWrappedUpToast, 'Event wrapped up');
      expect(l10n.eventWrappedUpToast.toLowerCase(), isNot(contains('experience')));
    });
  });
}
