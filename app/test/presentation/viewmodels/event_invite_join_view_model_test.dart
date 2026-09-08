import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/presentation/viewmodels/event_invite_join_view_model.dart';
import 'package:ripls/services/community_service.dart' show AcceptInvitationLinkResponse;
import 'package:ripls/services/providers/community_providers.dart';

import 'event_invite_join_view_model_test.mocks.dart';

@GenerateMocks([CommunityRepository])
void main() {
  late MockCommunityRepository mockCommunityRepo;
  late ProviderContainer container;

  const shortCode = 'Ts7uj2TZ';

  AcceptInvitationLinkResponse okResponse() => AcceptInvitationLinkResponse(
        communityId: 'community-456',
        communityName: 'Test Community',
      );

  setUp(() {
    mockCommunityRepo = MockCommunityRepository();
    container = ProviderContainer(
      overrides: [
        communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockCommunityRepo);
  });

  // Keeps the autoDispose family provider mounted across async gaps.
  void keepMounted() {
    container.listen(eventInviteJoinProvider(shortCode), (_, _) {});
  }

  group('EventInviteJoinNotifier — happy path', () {
    test('join() calls acceptInvitationLink with the code and succeeds',
        () async {
      when(mockCommunityRepo.acceptInvitationLink(
        shortCode: anyNamed('shortCode'),
      )).thenAnswer((_) async => okResponse());
      keepMounted();

      final notifier =
          container.read(eventInviteJoinProvider(shortCode).notifier);
      await notifier.join();

      expect(
        container.read(eventInviteJoinProvider(shortCode)).hasSucceeded,
        isTrue,
      );
      verify(mockCommunityRepo.acceptInvitationLink(shortCode: shortCode))
          .called(1);
    });

    test('an existing member returns success (server idempotency)', () async {
      // AcceptInvitationLink returns success for an already-member; same code
      // path as a fresh join from the client's perspective.
      when(mockCommunityRepo.acceptInvitationLink(
        shortCode: anyNamed('shortCode'),
      )).thenAnswer((_) async => okResponse());
      keepMounted();

      final notifier =
          container.read(eventInviteJoinProvider(shortCode).notifier);
      await notifier.join();

      expect(
        container.read(eventInviteJoinProvider(shortCode)).hasSucceeded,
        isTrue,
      );
    });

    test('join() is a no-op when already succeeded', () async {
      when(mockCommunityRepo.acceptInvitationLink(
        shortCode: anyNamed('shortCode'),
      )).thenAnswer((_) async => okResponse());
      keepMounted();

      final notifier =
          container.read(eventInviteJoinProvider(shortCode).notifier);
      await notifier.join();
      await notifier.join();

      verify(mockCommunityRepo.acceptInvitationLink(shortCode: shortCode))
          .called(1);
    });
  });

  group('EventInviteJoinNotifier — failure path', () {
    test('join() lands in failed on error and does not throw', () async {
      when(mockCommunityRepo.acceptInvitationLink(
        shortCode: anyNamed('shortCode'),
      )).thenThrow(Exception('not authorized'));
      keepMounted();

      final notifier =
          container.read(eventInviteJoinProvider(shortCode).notifier);
      // Must not throw — join failures are surfaced via state, not exceptions.
      await notifier.join();

      expect(
        container.read(eventInviteJoinProvider(shortCode)).hasFailed,
        isTrue,
      );
    });

    test('retry() re-runs after a failure', () async {
      var callCount = 0;
      when(mockCommunityRepo.acceptInvitationLink(
        shortCode: anyNamed('shortCode'),
      )).thenAnswer((_) async {
        callCount++;
        if (callCount == 1) {
          throw Exception('first call fails');
        }
        return okResponse();
      });
      keepMounted();

      final notifier =
          container.read(eventInviteJoinProvider(shortCode).notifier);
      await notifier.join();
      expect(
        container.read(eventInviteJoinProvider(shortCode)).hasFailed,
        isTrue,
      );

      await notifier.retry();
      expect(
        container.read(eventInviteJoinProvider(shortCode)).hasSucceeded,
        isTrue,
      );
    });
  });

  group('EventInviteJoinNotifier — disposal safety', () {
    test('handles disposal during join without throwing', () async {
      final completer = Completer<AcceptInvitationLinkResponse>();
      when(mockCommunityRepo.acceptInvitationLink(
        shortCode: anyNamed('shortCode'),
      )).thenAnswer((_) => completer.future);

      final notifier =
          container.read(eventInviteJoinProvider(shortCode).notifier);
      final future = notifier.join();

      // Dispose mid-flight; SafeNotifierMixin + ref.mounted make the in-flight
      // write a no-op rather than a crash.
      container.dispose();
      completer.complete(okResponse());

      await expectLater(future, completes);
    });
  });
}
