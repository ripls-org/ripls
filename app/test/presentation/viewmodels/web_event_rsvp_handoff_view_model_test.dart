import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/share_link_repository.dart';
import 'package:ripls/presentation/viewmodels/web_event_rsvp_handoff_view_model.dart';
import 'package:ripls/services/experience_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/experience_providers.dart';

import 'web_event_rsvp_handoff_view_model_test.mocks.dart';

@GenerateMocks([ShareLinkRepository, ExperienceService])
void main() {
  late MockShareLinkRepository mockShareLinkRepo;
  late MockExperienceService mockExperienceService;
  late ProviderContainer container;

  const experienceId = 'exp-123';
  const shortCode = 'TESTCODE';
  const communityId = 'community-456';

  InvitationCheckResult okInvitation() {
    return InvitationCheckResult(
      isValid: true,
      communityId: communityId,
      communityName: 'Test Community',
      inviterName: 'Inviter',
      numMembers: 5,
      maxMembers: 32,
      targetId: experienceId,
      errorMessage: '',
    );
  }

  setUp(() {
    mockShareLinkRepo = MockShareLinkRepository();
    mockExperienceService = MockExperienceService();

    container = ProviderContainer(
      overrides: [
        shareLinkRepositoryProvider.overrideWithValue(mockShareLinkRepo),
        experienceServiceProvider.overrideWithValue(mockExperienceService),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('WebEventRsvpHandoffNotifier — happy path', () {
    test('attempt() fires RSVPToExperience and lands in succeeded', () async {
      when(mockShareLinkRepo.describe(shortCode))
          .thenAnswer((_) async => okInvitation());
      when(mockExperienceService.rsvp(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
        intention: anyNamed('intention'),
      )).thenAnswer((_) async => Future<void>.value());

      final notifier = container
          .read(webEventRsvpHandoffProvider(experienceId).notifier);
      await notifier.attempt(rsvpIntention: 'yes', shortCode: shortCode);

      final state = container.read(webEventRsvpHandoffProvider(experienceId));
      expect(state.hasSucceeded, isTrue);
      verify(mockExperienceService.rsvp(
        experienceId: experienceId,
        communityId: communityId,
        intention: RSVPIntention.RSVP_INTENTION_YES,
      )).called(1);
    });

    test('attempt() does nothing for unknown rsvpIntention', () async {
      final notifier = container
          .read(webEventRsvpHandoffProvider(experienceId).notifier);
      await notifier.attempt(rsvpIntention: 'sometimes', shortCode: shortCode);

      final state = container.read(webEventRsvpHandoffProvider(experienceId));
      // Unknown intention → stays idle, never fires the RPC.
      expect(state.status, WebRsvpHandoffStatus.idle);
      verifyNever(mockShareLinkRepo.describe(any));
      verifyNever(mockExperienceService.rsvp(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
        intention: anyNamed('intention'),
      ));
    });

    test('attempt() is no-op when already succeeded', () async {
      when(mockShareLinkRepo.describe(shortCode))
          .thenAnswer((_) async => okInvitation());
      when(mockExperienceService.rsvp(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
        intention: anyNamed('intention'),
      )).thenAnswer((_) async => Future<void>.value());

      final notifier = container
          .read(webEventRsvpHandoffProvider(experienceId).notifier);
      await notifier.attempt(rsvpIntention: 'yes', shortCode: shortCode);
      await notifier.attempt(rsvpIntention: 'yes', shortCode: shortCode);

      verify(mockExperienceService.rsvp(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
        intention: anyNamed('intention'),
      )).called(1);
    });
  });

  group('WebEventRsvpHandoffNotifier — failure path', () {
    test('exhausting retries lands in failed status', () async {
      when(mockShareLinkRepo.describe(shortCode))
          .thenAnswer((_) async => okInvitation());
      when(mockExperienceService.rsvp(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
        intention: anyNamed('intention'),
      )).thenThrow(Exception('network down'));

      // Listen to keep the autoDispose family provider mounted while
      // attempt() runs through its retry loop.
      container.listen(
        webEventRsvpHandoffProvider(experienceId),
        (_, _) {},
      );
      final notifier = container
          .read(webEventRsvpHandoffProvider(experienceId).notifier);
      await notifier.attempt(rsvpIntention: 'maybe', shortCode: shortCode);

      final state = container.read(webEventRsvpHandoffProvider(experienceId));
      expect(state.status, WebRsvpHandoffStatus.failed,
          reason: 'attempts=${state.attempts}');
      expect(state.attempts, 3);
    });

    test('retry() re-fires from failed state', () async {
      // First attempt: fail all retries.
      when(mockShareLinkRepo.describe(shortCode))
          .thenAnswer((_) async => okInvitation());
      var callCount = 0;
      when(mockExperienceService.rsvp(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
        intention: anyNamed('intention'),
      )).thenAnswer((_) async {
        callCount++;
        if (callCount <= 3) {
          throw Exception('flake');
        }
      });

      container.listen(
        webEventRsvpHandoffProvider(experienceId),
        (_, _) {},
      );
      final notifier = container
          .read(webEventRsvpHandoffProvider(experienceId).notifier);
      await notifier.attempt(rsvpIntention: 'yes', shortCode: shortCode);
      expect(
        container.read(webEventRsvpHandoffProvider(experienceId)).hasFailed,
        isTrue,
      );

      // Now retry — fourth call succeeds.
      await notifier.retry(rsvpIntention: 'yes', shortCode: shortCode);
      final state = container.read(webEventRsvpHandoffProvider(experienceId));
      expect(state.hasSucceeded, isTrue);
    });
  });

  group('WebEventRsvpHandoffNotifier — disposal safety', () {
    test('handles disposal during async operation gracefully', () async {
      when(mockShareLinkRepo.describe(shortCode))
          .thenAnswer((_) async => okInvitation());
      // Mock a slow RPC that will still be in flight when we dispose.
      when(mockExperienceService.rsvp(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
        intention: anyNamed('intention'),
      )).thenAnswer((_) async {
        await Future<void>.delayed(const Duration(milliseconds: 200));
      });

      final notifier = container
          .read(webEventRsvpHandoffProvider(experienceId).notifier);
      final future = notifier.attempt(
        rsvpIntention: 'yes',
        shortCode: shortCode,
      );

      // Dispose mid-flight. The notifier uses SafeNotifierMixin +
      // ref.mounted checks, so the in-flight write completes without
      // throwing.
      container.dispose();

      await expectLater(future, completes);
    });
  });
}
