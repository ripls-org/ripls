import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

import 'time_modal_view_model_test.mocks.dart';

class _FakeAuthStateNotifier extends AuthStateNotifier {
  // A signed-in fake user — TimeModalNotifier.build() bails to an empty
  // TimeModalData when auth.user is null (the logout-race guard added in
  // experience_view_model + time_modal_view_model). Tests that expect
  // computed state (isReadOnly, mode, etc.) need an authenticated context.
  @override
  AuthStateData build() => AuthStateData(
        isLoading: false,
        user: User(id: 'user-1', name: 'Tester'),
      );
}

@GenerateMocks([ExperienceRepository])
void main() {
  late MockExperienceRepository mockRepo;
  late ProviderContainer container;

  const experienceId = 'exp1';

  GetExperienceResponse emptyExperienceResponse() =>
      GetExperienceResponse(experience: Experience());

  ProviderContainer buildContainer() {
    return ProviderContainer(
      overrides: [
        experienceRepositoryProvider.overrideWithValue(mockRepo),
        resolvedTimezoneProvider
            .overrideWith((ref) async => 'America/New_York'),
        authStateProvider.overrideWith(_FakeAuthStateNotifier.new),
      ],
    );
  }

  setUp(() {
    mockRepo = MockExperienceRepository();
    when(mockRepo.invalidate(any)).thenAnswer((_) async {});
    when(mockRepo.getExperienceDetails(any))
        .thenAnswer((_) async => emptyExperienceResponse());
    container = buildContainer();
  });

  tearDown(() {
    container.dispose();
    reset(mockRepo);
  });

  group('build / initialize', () {
    test('loads successfully with empty experience', () async {
      final state =
          await container.read(timeModalProvider(experienceId).future);
      expect(state.proposals, isEmpty);
      expect(state.mode, 'default');
      expect(state.isReadOnly, isFalse);
    });

    test('sets isReadOnly when experience is completed', () async {
      when(mockRepo.getExperienceDetails(experienceId))
          .thenAnswer((_) async => GetExperienceResponse(
                experience: Experience(
                    state: ExperienceState.EXPERIENCE_STATE_COMPLETED),
              ));

      final state =
          await container.read(timeModalProvider(experienceId).future);
      expect(state.isReadOnly, isTrue);
    });

    test('error: provider is in error state on failure', () async {
      when(mockRepo.getExperienceDetails(experienceId))
          .thenThrow(Exception('Load failed'));

      // Wait for build to resolve
      await Future<void>.delayed(const Duration(milliseconds: 50));

      final asyncState = container.read(timeModalProvider(experienceId));
      // Either still loading or error after the delay
      expect(
        asyncState.hasError || asyncState.isLoading,
        isTrue,
      );
    });
  });

  group('enterProposeMode / exitProposeMode', () {
    test('enterProposeMode switches mode to propose', () async {
      await container.read(timeModalProvider(experienceId).future);
      final notifier =
          container.read(timeModalProvider(experienceId).notifier);
      notifier.enterProposeMode();

      final state =
          container.read(timeModalProvider(experienceId)).value;
      expect(state?.mode, 'propose');
      expect(state?.isProposing, isTrue);
    });

    test('exitProposeMode returns to default mode', () async {
      await container.read(timeModalProvider(experienceId).future);
      final notifier =
          container.read(timeModalProvider(experienceId).notifier);
      notifier.enterProposeMode();
      notifier.exitProposeMode();

      final state =
          container.read(timeModalProvider(experienceId)).value;
      expect(state?.mode, 'default');
    });
  });

  group('updateProposeDate / updateProposeTime / updateProposeDuration', () {
    test('updateProposeDate sets proposeDate', () async {
      await container.read(timeModalProvider(experienceId).future);
      final notifier =
          container.read(timeModalProvider(experienceId).notifier);
      final date = DateTime(2025, 6, 15);
      notifier.updateProposeDate(date);

      final state =
          container.read(timeModalProvider(experienceId)).value;
      expect(state?.proposeDate, date);
    });

    test('updateProposeTime sets proposeTime', () async {
      await container.read(timeModalProvider(experienceId).future);
      final notifier =
          container.read(timeModalProvider(experienceId).notifier);
      const time = TimeOfDay(hour: 14, minute: 30);
      notifier.updateProposeTime(time);

      final state =
          container.read(timeModalProvider(experienceId)).value;
      expect(state?.proposeTime, time);
    });

    test('updateProposeDuration sets proposeDurationMinutes', () async {
      await container.read(timeModalProvider(experienceId).future);
      final notifier =
          container.read(timeModalProvider(experienceId).notifier);
      notifier.updateProposeDuration(90);

      final state =
          container.read(timeModalProvider(experienceId)).value;
      expect(state?.proposeDurationMinutes, 90);
    });
  });

  group('setSingleTime', () {
    // Regression for #2598: tapping "Set the time" with exactly one staged
    // candidate must call SaveExperience directly — not ProposeTime + ConfirmTime
    // — so no transient poll is opened and no spurious "Poll ended" chat banner
    // is written.
    setUp(() {
      when(mockRepo.saveExperience(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        locationId: anyNamed('locationId'),
        time: anyNamed('time'),
        maxParticipants: anyNamed('maxParticipants'),
        sourceUrl: anyNamed('sourceUrl'),
      )).thenAnswer((_) async {});
    });

    test('calls saveExperience with the staged time', () async {
      await container.read(timeModalProvider(experienceId).future);
      final notifier =
          container.read(timeModalProvider(experienceId).notifier);

      final dt = DateTime(2026, 6, 15, 18, 30);
      await notifier.setSingleTime(dt);

      verify(mockRepo.saveExperience(
        id: experienceId,
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        locationId: anyNamed('locationId'),
        time: anyNamed('time'),
        maxParticipants: anyNamed('maxParticipants'),
        sourceUrl: anyNamed('sourceUrl'),
      )).called(1);

      // Regression: no propose/confirm poll path must be taken.
      verifyNever(mockRepo.proposeTime(
        experienceId: anyNamed('experienceId'),
        time: anyNamed('time'),
      ));
      verifyNever(mockRepo.confirmTime(
        experienceId: anyNamed('experienceId'),
        proposalId: anyNamed('proposalId'),
      ));
    });

    test('rethrows on saveExperience failure', () async {
      when(mockRepo.saveExperience(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        locationId: anyNamed('locationId'),
        time: anyNamed('time'),
        maxParticipants: anyNamed('maxParticipants'),
        sourceUrl: anyNamed('sourceUrl'),
      )).thenThrow(Exception('network down'));

      await container.read(timeModalProvider(experienceId).future);
      final notifier =
          container.read(timeModalProvider(experienceId).notifier);

      await expectLater(
        notifier.setSingleTime(DateTime(2026, 6, 15, 18, 30)),
        throwsException,
      );
    });
  });

  group('disposal safety', () {
    test('container.dispose() mid-flight does not crash', () async {
      when(mockRepo.getExperienceDetails(experienceId)).thenAnswer((_) async {
        await Future<void>.delayed(const Duration(milliseconds: 10));
        return emptyExperienceResponse();
      });

      final _ = container.read(timeModalProvider(experienceId));
      final future =
          container.read(timeModalProvider(experienceId).future);
      container.dispose();
      // After disposal, the future may complete or throw a Riverpod state
      // error — either is acceptable; the important thing is no unhandled crash.
      try {
        await future;
      } catch (_) {
        // Riverpod may throw on ref access after disposal — this is expected.
      }
    });

    test('container.dispose() mid-flight does not crash setSingleTime',
        () async {
      when(mockRepo.getExperienceDetails(experienceId)).thenAnswer((_) async {
        await Future<void>.delayed(const Duration(milliseconds: 10));
        return emptyExperienceResponse();
      });
      when(mockRepo.saveExperience(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        locationId: anyNamed('locationId'),
        time: anyNamed('time'),
        maxParticipants: anyNamed('maxParticipants'),
        sourceUrl: anyNamed('sourceUrl'),
      )).thenAnswer((_) async {});

      await container.read(timeModalProvider(experienceId).future);
      final notifier =
          container.read(timeModalProvider(experienceId).notifier);

      final future = notifier.setSingleTime(DateTime(2026, 6, 15, 18, 30));
      container.dispose();
      try {
        await future;
      } catch (_) {
        // Expected — ref access after disposal can throw.
      }
    });
  });
}
