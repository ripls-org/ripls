import 'dart:async';

import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show Estimate;
import 'package:ripls/data/gen/ripls/api/experience.pb.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart' show RSVP;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/presentation/viewmodels/event_modal_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

import '../../helpers/fake_async_helpers.dart';
import 'event_modal_view_model_test.mocks.dart';

@GenerateMocks([ExperienceRepository])
void main() {
  late MockExperienceRepository mockExperienceRepository;
  late ProviderContainer container;

  const testExperienceId = 'exp123';
  const testUserId = 'user789';
  const testOwnerId = 'owner456';
  const testCommunityId = 'community123';

  // Helper to create a mock AuthStateNotifier that returns specific data
  AuthStateNotifier createMockAuthNotifier(User? user) {
    return _MockAuthStateNotifier(user);
  }

  setUp(() {
    mockExperienceRepository = MockExperienceRepository();

    container = ProviderContainer(
      overrides: [
        experienceRepositoryProvider.overrideWithValue(
          mockExperienceRepository,
        ),
        authStateProvider.overrideWith(
          () => createMockAuthNotifier(User(id: testUserId, name: 'Test User')),
        ),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('EventModalViewModel - Initialization', () {
    test('initialize loads experience and determines organizer role', () async {
      final mockExperience = GetExperienceResponse(
        experience: Experience(
          id: testExperienceId,
          name: 'Test Experience',
          description: 'Test Description',
          owner: User(id: testOwnerId, name: 'Owner'),
          communityId: testCommunityId,
          state: ExperienceState.EXPERIENCE_STATE_ACTIVE,
          timeProposals: [],
        ),
        rsvps: [],
      );

      when(
        mockExperienceRepository.getExperienceDetails(testExperienceId),
      ).thenAnswer((_) async => mockExperience);

      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );
      await notifier.initialize();

      final state = container.read(eventModalProvider(testExperienceId));

      expect(state.experience, isNotNull);
      expect(state.experience!.id, testExperienceId);
      expect(state.isOrganizer, false); // testUserId != testOwnerId
      expect(state.currentPhase, 0); // Planning/RSVP phase
      expect(state.isLoading, false);
      expect(state.error, isNull);
    });

    test('initialize determines user is organizer when IDs match', () async {
      final mockExperience = GetExperienceResponse(
        experience: Experience(
          id: testExperienceId,
          name: 'Test Experience',
          description: 'Test Description',
          owner: User(id: testUserId, name: 'Test User'), // Same as auth user
          communityId: testCommunityId,
          state: ExperienceState.EXPERIENCE_STATE_ACTIVE,
          timeProposals: [],
        ),
        rsvps: [],
      );

      when(
        mockExperienceRepository.getExperienceDetails(testExperienceId),
      ).thenAnswer((_) async => mockExperience);

      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );
      await notifier.initialize();

      final state = container.read(eventModalProvider(testExperienceId));

      expect(state.isOrganizer, true); // testUserId == testUserId
    });

    test('initialize handles error and sets error message', () async {
      when(
        mockExperienceRepository.getExperienceDetails(testExperienceId),
      ).thenThrow(Exception('Network error'));

      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );
      await notifier.initialize();

      final state = container.read(eventModalProvider(testExperienceId));

      expect(state.isLoading, false);
      expect(state.error, isNotNull);
    });
  });

  group('EventModalViewModel - Phase Determination', () {
    test('determines completed phase for completed experience', () async {
      final mockExperience = GetExperienceResponse(
        experience: Experience(
          id: testExperienceId,
          name: 'Test Experience',
          owner: User(id: testUserId, name: 'Test User'),
          communityId: testCommunityId,
          state: ExperienceState.EXPERIENCE_STATE_COMPLETED,
          timeProposals: [],
        ),
        rsvps: [],
      );

      when(
        mockExperienceRepository.getExperienceDetails(testExperienceId),
      ).thenAnswer((_) async => mockExperience);

      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );
      await notifier.initialize();

      final state = container.read(eventModalProvider(testExperienceId));

      // Organizer: phase 2 (complete)
      expect(state.currentPhase, 2);
    });

    test('determines confirmed phase when time is confirmed', () async {
      final mockExperience = GetExperienceResponse(
        experience: Experience(
          id: testExperienceId,
          name: 'Test Experience',
          owner: User(id: testUserId, name: 'Test User'),
          communityId: testCommunityId,
          state: ExperienceState.EXPERIENCE_STATE_ACTIVE,
          timeProposals: [
            TimeProposal(
              id: 'prop1',
              isConfirmed: true,
              time: ExperienceTime(),
            ),
          ],
        ),
        rsvps: [],
      );

      when(
        mockExperienceRepository.getExperienceDetails(testExperienceId),
      ).thenAnswer((_) async => mockExperience);

      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );
      await notifier.initialize();

      final state = container.read(eventModalProvider(testExperienceId));

      // Phase 0 (Planning/RSVP) - confirmed time shown via UI, not separate phase
      expect(state.currentPhase, 0);
    });
  });

  group('EventModalViewModel - Time Proposals', () {
    test('proposeTime calls repository and refreshes experience', () async {
      final mockExperience = GetExperienceResponse(
        experience: Experience(
          id: testExperienceId,
          name: 'Test Experience',
          owner: User(id: testUserId, name: 'Test User'),
          communityId: testCommunityId,
          state: ExperienceState.EXPERIENCE_STATE_ACTIVE,
          timeProposals: [],
        ),
        rsvps: [],
      );

      when(
        mockExperienceRepository.getExperienceDetails(testExperienceId),
      ).thenAnswer((_) async => mockExperience);

      when(
        mockExperienceRepository.proposeTime(
          experienceId: testExperienceId,
          time: anyNamed('time'),
        ),
      ).thenAnswer(
        (_) async => TimeProposal(id: 'prop1', time: ExperienceTime()),
      );

      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );
      await notifier.initialize();

      final proposedTime = ExperienceTime(
        specific: SpecificTime(
          unixTimestampSec: fixnum.Int64(1234567890),
          timezone: 'America/Los_Angeles',
        ),
      );

      await notifier.proposeTime(proposedTime);

      verify(
        mockExperienceRepository.proposeTime(
          experienceId: testExperienceId,
          time: proposedTime,
        ),
      ).called(1);

      // Should refresh experience after proposing
      verify(
        mockExperienceRepository.getExperienceDetails(testExperienceId),
      ).called(2); // Once in initialize, once in refresh
    });
  });

  group('EventModalViewModel - RSVP', () {
    test('submitRsvp calls repository and updates state', () async {
      final mockExperience = GetExperienceResponse(
        experience: Experience(
          id: testExperienceId,
          name: 'Test Experience',
          owner: User(id: testOwnerId, name: 'Owner'),
          communityId: testCommunityId,
          state: ExperienceState.EXPERIENCE_STATE_ACTIVE,
          timeProposals: [],
        ),
        rsvps: [],
      );

      when(
        mockExperienceRepository.getExperienceDetails(testExperienceId),
      ).thenAnswer((_) async => mockExperience);

      when(
        mockExperienceRepository.rsvp(
          experienceId: testExperienceId,
          communityId: testCommunityId,
          intention: RSVPIntention.RSVP_INTENTION_YES,
        ),
      ).thenAnswer((_) async {});

      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );
      await notifier.initialize();

      await notifier.submitRsvp(
        RSVPIntention.RSVP_INTENTION_YES,
        testCommunityId,
      );

      final state = container.read(eventModalProvider(testExperienceId));

      expect(state.currentUserIntention, RSVPIntention.RSVP_INTENTION_YES);

      verify(
        mockExperienceRepository.rsvp(
          experienceId: testExperienceId,
          communityId: testCommunityId,
          intention: RSVPIntention.RSVP_INTENTION_YES,
        ),
      ).called(1);
    });

    test(
      'submitRsvp inserts optimistic entry for first-time RSVP before server call',
      () async {
        // Completer lets us pause the mock mid-call and inspect optimistic state.
        final rsvpCompleter = Completer<void>();

        final mockExperience = GetExperienceResponse(
          experience: Experience(
            id: testExperienceId,
            name: 'Test Experience',
            owner: User(id: testOwnerId, name: 'Owner'),
            communityId: testCommunityId,
            state: ExperienceState.EXPERIENCE_STATE_ACTIVE,
            timeProposals: [],
          ),
          rsvps: [],
        );

        when(
          mockExperienceRepository.getExperienceDetails(testExperienceId),
        ).thenAnswer((_) async => mockExperience);

        when(
          mockExperienceRepository.rsvp(
            experienceId: testExperienceId,
            communityId: testCommunityId,
            intention: RSVPIntention.RSVP_INTENTION_YES,
          ),
        ).thenAnswer((_) => rsvpCompleter.future);

        final notifier = container.read(
          eventModalProvider(testExperienceId).notifier,
        );
        await notifier.initialize();

        // Verify no rsvps before tapping.
        expect(
          container.read(eventModalProvider(testExperienceId)).rsvps,
          isEmpty,
        );

        // Start submit without awaiting — this lets us inspect state mid-flight.
        final submitFuture = notifier.submitRsvp(
          RSVPIntention.RSVP_INTENTION_YES,
          testCommunityId,
        );

        // Optimistic entry should be present immediately, before server responds.
        final stateBeforeServer = container.read(
          eventModalProvider(testExperienceId),
        );
        expect(stateBeforeServer.rsvps.length, 1);
        expect(stateBeforeServer.rsvps.first.user.id, testUserId);
        expect(
          stateBeforeServer.rsvps.first.intention,
          RSVPIntention.RSVP_INTENTION_YES,
        );
        expect(
          stateBeforeServer.currentUserIntention,
          RSVPIntention.RSVP_INTENTION_YES,
        );

        // Unblock the server call so the test can complete cleanly.
        rsvpCompleter.complete();
        await submitFuture;
      },
    );

    test(
      'submitRsvp rewrites existing entry rather than inserting duplicate',
      () async {
        final rsvpCompleter = Completer<void>();

        final existingRsvp = RSVP(
          user: User(id: testUserId, name: 'Test User'),
          intention: RSVPIntention.RSVP_INTENTION_MAYBE,
        );

        final mockExperience = GetExperienceResponse(
          experience: Experience(
            id: testExperienceId,
            name: 'Test Experience',
            owner: User(id: testOwnerId, name: 'Owner'),
            communityId: testCommunityId,
            state: ExperienceState.EXPERIENCE_STATE_ACTIVE,
            timeProposals: [],
          ),
          rsvps: [existingRsvp],
        );

        when(
          mockExperienceRepository.getExperienceDetails(testExperienceId),
        ).thenAnswer((_) async => mockExperience);

        when(
          mockExperienceRepository.rsvp(
            experienceId: testExperienceId,
            communityId: testCommunityId,
            intention: RSVPIntention.RSVP_INTENTION_YES,
          ),
        ).thenAnswer((_) => rsvpCompleter.future);

        final notifier = container.read(
          eventModalProvider(testExperienceId).notifier,
        );
        await notifier.initialize();

        expect(
          container.read(eventModalProvider(testExperienceId)).rsvps.length,
          1,
        );

        final submitFuture = notifier.submitRsvp(
          RSVPIntention.RSVP_INTENTION_YES,
          testCommunityId,
        );

        // Check optimistic state before refresh — should rewrite, not insert.
        final optimisticState = container.read(
          eventModalProvider(testExperienceId),
        );
        expect(optimisticState.rsvps.length, 1);
        expect(
          optimisticState.rsvps.first.intention,
          RSVPIntention.RSVP_INTENTION_YES,
        );

        rsvpCompleter.complete();
        await submitFuture;
      },
    );

    test('submitRsvp rolls back rsvps and intention on RPC failure', () async {
      final existingRsvp = RSVP(
        user: User(id: testUserId, name: 'Test User'),
        intention: RSVPIntention.RSVP_INTENTION_MAYBE,
      );

      final mockExperience = GetExperienceResponse(
        experience: Experience(
          id: testExperienceId,
          name: 'Test Experience',
          owner: User(id: testOwnerId, name: 'Owner'),
          communityId: testCommunityId,
          state: ExperienceState.EXPERIENCE_STATE_ACTIVE,
          timeProposals: [],
        ),
        rsvps: [existingRsvp],
      );

      when(
        mockExperienceRepository.getExperienceDetails(testExperienceId),
      ).thenAnswer((_) async => mockExperience);

      when(
        mockExperienceRepository.rsvp(
          experienceId: anyNamed('experienceId'),
          communityId: anyNamed('communityId'),
          intention: anyNamed('intention'),
        ),
      ).thenThrow(Exception('network error'));

      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );
      await notifier.initialize();

      final stateBeforeRsvp = container.read(
        eventModalProvider(testExperienceId),
      );
      final intentionBefore = stateBeforeRsvp.currentUserIntention;
      final rsvpsBefore = stateBeforeRsvp.rsvps;

      await notifier.submitRsvp(
        RSVPIntention.RSVP_INTENTION_YES,
        testCommunityId,
      );

      final stateAfterFailure = container.read(
        eventModalProvider(testExperienceId),
      );
      expect(stateAfterFailure.currentUserIntention, intentionBefore);
      expect(stateAfterFailure.rsvps.length, rsvpsBefore.length);
      expect(stateAfterFailure.error, isNotNull);
      expect(stateAfterFailure.isSubmittingRsvp, false);
    });

    test(
      'submitRsvp completes without throwing when disposed mid-flight',
      () async {
        final rsvpCompleter = Completer<void>();

        final mockExperience = GetExperienceResponse(
          experience: Experience(
            id: testExperienceId,
            name: 'Test Experience',
            owner: User(id: testOwnerId, name: 'Owner'),
            communityId: testCommunityId,
            state: ExperienceState.EXPERIENCE_STATE_ACTIVE,
            timeProposals: [],
          ),
          rsvps: [],
        );

        when(
          mockExperienceRepository.getExperienceDetails(testExperienceId),
        ).thenAnswer((_) async => mockExperience);

        when(
          mockExperienceRepository.rsvp(
            experienceId: testExperienceId,
            communityId: testCommunityId,
            intention: RSVPIntention.RSVP_INTENTION_YES,
          ),
        ).thenAnswer((_) => rsvpCompleter.future);

        final notifier = container.read(
          eventModalProvider(testExperienceId).notifier,
        );
        await notifier.initialize();

        final submitFuture = notifier.submitRsvp(
          RSVPIntention.RSVP_INTENTION_YES,
          testCommunityId,
        );

        // Dispose while the RPC is still in-flight.
        container.dispose();

        // Unblock the RPC — should complete without throwing.
        rsvpCompleter.complete();
        await expectLater(submitFuture, completes);
      },
    );
  });

  group('EventModalViewModel - Live QT Preview', () {
    late GetExperienceResponse mockActiveExperience;

    setUp(() {
      mockActiveExperience = GetExperienceResponse(
        experience: Experience(
          id: testExperienceId,
          name: 'Test Experience',
          owner: User(id: testOwnerId, name: 'Owner'),
          communityId: testCommunityId,
          state: ExperienceState.EXPERIENCE_STATE_ACTIVE,
          timeProposals: [],
        ),
        rsvps: [],
      );
    });

    test(
      'previewImpact calls repository with confirmed attendee IDs and updates state',
      () async {
        when(
          mockExperienceRepository.getExperienceDetails(testExperienceId),
        ).thenAnswer((_) async => mockActiveExperience);

        final fakeImpact = ImpactEstimate(
          qualityTime: QualityTimeEstimate(
            qualityTimeMinutes: Estimate(mean: 42),
            attributes: QualityTimeAttributes(
              tieStrength: SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW,
            ),
          ),
        );

        when(
          mockExperienceRepository.previewExperienceImpact(
            experienceId: testExperienceId,
            confirmedAttendeeIds: anyNamed('confirmedAttendeeIds'),
            confirmedAttendeeCount: anyNamed('confirmedAttendeeCount'),
          ),
        ).thenAnswer((_) async => fakeImpact);

        final notifier = container.read(
          eventModalProvider(testExperienceId).notifier,
        );
        await notifier.initialize(communityId: testCommunityId);

        // Toggle two attendees on.
        notifier.toggleAttendance('user1');
        notifier.toggleAttendance('user2');

        // Call previewImpact directly (bypassing debounce timer).
        await notifier.previewImpact();

        final state = container.read(eventModalProvider(testExperienceId));

        expect(state.impactEstimate, isNotNull);
        expect(
          state.impactEstimate!.qualityTime.qualityTimeMinutes.mean,
          closeTo(42, 0.01),
        );

        verify(
          mockExperienceRepository.previewExperienceImpact(
            experienceId: testExperienceId,
            confirmedAttendeeIds: argThat(
              containsAll(['user1', 'user2']),
              named: 'confirmedAttendeeIds',
            ),
            confirmedAttendeeCount: anyNamed('confirmedAttendeeCount'),
          ),
        ).called(greaterThanOrEqualTo(1));
      },
    );

    test(
      'previewImpact silently ignores repository errors and keeps prior state',
      () async {
        when(
          mockExperienceRepository.getExperienceDetails(testExperienceId),
        ).thenAnswer((_) async => mockActiveExperience);

        when(
          mockExperienceRepository.previewExperienceImpact(
            experienceId: anyNamed('experienceId'),
            confirmedAttendeeIds: anyNamed('confirmedAttendeeIds'),
            confirmedAttendeeCount: anyNamed('confirmedAttendeeCount'),
          ),
        ).thenThrow(Exception('network error'));

        final notifier = container.read(
          eventModalProvider(testExperienceId).notifier,
        );
        await notifier.initialize();

        // Should not throw.
        await notifier.previewImpact();

        final state = container.read(eventModalProvider(testExperienceId));
        expect(state.error, isNull);
        expect(state.isLoading, false);
      },
    );

    test('previewImpact count includes provisional attendees', () async {
      when(
        mockExperienceRepository.getExperienceDetails(testExperienceId),
      ).thenAnswer((_) async => mockActiveExperience);

      when(
        mockExperienceRepository.previewExperienceImpact(
          experienceId: anyNamed('experienceId'),
          confirmedAttendeeIds: anyNamed('confirmedAttendeeIds'),
          confirmedAttendeeCount: anyNamed('confirmedAttendeeCount'),
        ),
      ).thenAnswer((_) async => ImpactEstimate());

      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );
      await notifier.initialize();

      // 1 registered + 2 provisional attendees confirmed
      notifier.toggleAttendance('user1');
      notifier.toggleProvisionalAttendance('prov1');
      notifier.toggleProvisionalAttendance('prov2');

      await notifier.previewImpact();

      verify(
        mockExperienceRepository.previewExperienceImpact(
          experienceId: testExperienceId,
          confirmedAttendeeIds: argThat(
            equals(['user1']),
            named: 'confirmedAttendeeIds',
          ),
          confirmedAttendeeCount: argThat(
            equals(3),
            named: 'confirmedAttendeeCount',
          ),
        ),
      ).called(greaterThanOrEqualTo(1));
    });

    test(
      'debounced attendance toggle fires previewImpact exactly once after 250ms',
      () async {
        when(
          mockExperienceRepository.getExperienceDetails(testExperienceId),
        ).thenAnswer((_) async => mockActiveExperience);

        when(
          mockExperienceRepository.previewExperienceImpact(
            experienceId: anyNamed('experienceId'),
            confirmedAttendeeIds: anyNamed('confirmedAttendeeIds'),
            confirmedAttendeeCount: anyNamed('confirmedAttendeeCount'),
          ),
        ).thenAnswer((_) async => ImpactEstimate());

        final notifier = container.read(
          eventModalProvider(testExperienceId).notifier,
        );
        await notifier.initialize();
        // initialize() calls previewImpact once — reset before testing toggle debounce.
        clearInteractions(mockExperienceRepository);

        runDebounced((async) {
          // Rapid-fire three toggles within the debounce window.
          notifier.toggleAttendance('user1');
          notifier.toggleAttendance('user2');
          notifier.toggleAttendance('user3');

          // Advance past the 250ms debounce window and let the preview complete.
          async.elapse(const Duration(milliseconds: 250));
          async.flushMicrotasks();
        });

        // Despite three toggles, the server should be called exactly once.
        verify(
          mockExperienceRepository.previewExperienceImpact(
            experienceId: testExperienceId,
            confirmedAttendeeIds: anyNamed('confirmedAttendeeIds'),
            confirmedAttendeeCount: anyNamed('confirmedAttendeeCount'),
          ),
        ).called(1);
      },
    );
  });

  group('EventModalViewModel - Wrap Up', () {
    test('setCompletionSummary updates state', () {
      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );

      notifier.setCompletionSummary('Great event!');

      final state = container.read(eventModalProvider(testExperienceId));

      expect(state.completionSummary, 'Great event!');
    });

    test('toggleAttendance updates attendance map', () {
      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );

      notifier.toggleAttendance('user1');

      var state = container.read(eventModalProvider(testExperienceId));
      expect(state.attendanceMap['user1'], true);

      notifier.toggleAttendance('user1');

      state = container.read(eventModalProvider(testExperienceId));
      expect(state.attendanceMap['user1'], false);
    });

    test('wrapUp completes without summary', () async {
      final notifier = container.read(
        eventModalProvider(testExperienceId).notifier,
      );

      when(
        mockExperienceRepository.completeExperience(
          testExperienceId,
          summary: null,
          // The confirmed set the modal previewed with travels to the
          // commit so the impact numbers match (#2724); no attendance
          // was toggled in this test, so it is empty.
          confirmedAttendeeIds: [],
          confirmedAttendeeCount: 0,
        ),
      ).thenAnswer((_) async => CompleteExperienceResponse());

      // Wrap up without summary — should proceed to completion.
      await notifier.wrapUp();

      final state = container.read(eventModalProvider(testExperienceId));

      expect(state.error, isNull);
      expect(state.currentPhase, 2);
      verify(
        mockExperienceRepository.completeExperience(
          testExperienceId,
          summary: null,
          // The confirmed set the modal previewed with travels to the
          // commit so the impact numbers match (#2724); no attendance
          // was toggled in this test, so it is empty.
          confirmedAttendeeIds: [],
          confirmedAttendeeCount: 0,
        ),
      ).called(1);
    });
  });
}

// Mock AuthStateNotifier for testing
class _MockAuthStateNotifier extends AuthStateNotifier {
  final User? _user;

  _MockAuthStateNotifier(this._user);

  @override
  AuthStateData build() {
    return AuthStateData(
      user: _user,
      accessToken: _user != null ? 'mock-token' : null,
      isLoading: false,
    );
  }
}
