import 'package:connectrpc/connect.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/leave_community_view_model.dart';
import 'package:ripls/services/providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'leave_community_view_model_test.mocks.dart';

@GenerateMocks([
  CommunityRepository,
  GearRepository,
  RequestRepository,
  ExperienceRepository,
  TransferRepository,
  SearchRepository,
  FeedRepository,
  StoryRepository,
  ChatRepository,
])
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('LeaveCommunityNotifier', () {
    late MockCommunityRepository mockCommunity;
    late MockGearRepository mockGear;
    late MockRequestRepository mockRequest;
    late MockExperienceRepository mockExperience;
    late MockTransferRepository mockTransfer;
    late MockSearchRepository mockSearch;
    late MockFeedRepository mockFeed;
    late MockStoryRepository mockStory;
    late MockChatRepository mockChat;
    late ProviderContainer container;

    const communityId = 'comm-1';
    const candidateId = 'user-2';

    void stubFanOutHappyPath() {
      when(mockCommunity.refreshAll(any)).thenAnswer((_) async {});
      when(mockCommunity.listUserCommunities(
        regionFilter: anyNamed('regionFilter'),
      )).thenAnswer((_) async => []);
      when(mockGear.invalidateForCommunity(any)).thenAnswer((_) async {});
      when(mockRequest.invalidateForCommunity(any)).thenAnswer((_) async {});
      when(mockExperience.invalidateForCommunity(any))
          .thenAnswer((_) async {});
      when(mockTransfer.invalidateForCommunity(any))
          .thenAnswer((_) async {});
      when(mockSearch.invalidateSearchesForCommunity(any))
          .thenAnswer((_) async {});
      when(mockFeed.invalidateFeed()).thenAnswer((_) async {});
      when(mockStory.invalidateCommunity(any)).thenAnswer((_) async {});
      when(mockChat.invalidateAll()).thenAnswer((_) async {});
    }

    setUp(() {
      SharedPreferences.setMockInitialValues({});

      mockCommunity = MockCommunityRepository();
      mockGear = MockGearRepository();
      mockRequest = MockRequestRepository();
      mockExperience = MockExperienceRepository();
      mockTransfer = MockTransferRepository();
      mockSearch = MockSearchRepository();
      mockFeed = MockFeedRepository();
      mockStory = MockStoryRepository();
      mockChat = MockChatRepository();

      container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunity),
          gearRepositoryProvider.overrideWithValue(mockGear),
          requestRepositoryProvider.overrideWithValue(mockRequest),
          experienceRepositoryProvider.overrideWithValue(mockExperience),
          transferRepositoryProvider.overrideWithValue(mockTransfer),
          searchRepositoryProvider.overrideWithValue(mockSearch),
          feedRepositoryProvider.overrideWithValue(mockFeed),
          storyRepositoryProvider.overrideWithValue(mockStory),
          chatRepositoryProvider.overrideWithValue(mockChat),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    test('initial state is LeaveOutcomeIdle', () {
      expect(
        container.read(leaveCommunityProvider),
        isA<LeaveOutcomeIdle>(),
      );
    });

    test(
      'non-owner leave succeeds and fans out cache invalidations',
      () async {
        when(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenAnswer((_) async {});
        stubFanOutHappyPath();

        final notifier = container.read(leaveCommunityProvider.notifier);
        await notifier.leave(communityId);

        expect(
          container.read(leaveCommunityProvider),
          isA<LeaveOutcomeSuccess>(),
        );

        verify(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: null,
        )).called(1);
        verify(mockCommunity.refreshAll(communityId)).called(1);
        verify(mockGear.invalidateForCommunity(communityId)).called(1);
        verify(mockRequest.invalidateForCommunity(communityId)).called(1);
        verify(mockExperience.invalidateForCommunity(communityId)).called(1);
        verify(mockTransfer.invalidateForCommunity(communityId)).called(1);
        verify(mockSearch.invalidateSearchesForCommunity(communityId))
            .called(1);
        verify(mockFeed.invalidateFeed()).called(1);
        verify(mockStory.invalidateCommunity(communityId)).called(1);
        verify(mockChat.invalidateAll()).called(1);
      },
    );

    test(
      'owner leave threads newOwnerUserId through to the repo',
      () async {
        when(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenAnswer((_) async {});
        stubFanOutHappyPath();

        final notifier = container.read(leaveCommunityProvider.notifier);
        await notifier.leave(communityId, newOwnerUserId: candidateId);

        expect(
          container.read(leaveCommunityProvider),
          isA<LeaveOutcomeSuccess>(),
        );

        verify(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: candidateId,
        )).called(1);
      },
    );

    test(
      'InvalidArgument: missing_new_owner_user_id maps to '
      'LeaveOutcomeMissingNewOwner',
      () async {
        when(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenThrow(
          ServiceException('owner-leave requires new_owner_user_id: '
              'missing_new_owner_user_id',
              code: Code.invalidArgument),
        );

        final notifier = container.read(leaveCommunityProvider.notifier);
        await notifier.leave(communityId);

        expect(
          container.read(leaveCommunityProvider),
          isA<LeaveOutcomeMissingNewOwner>(),
        );
        // Cache fan-out must NOT run on failure.
        verifyNever(mockCommunity.refreshAll(any));
      },
    );

    test(
      'InvalidArgument: candidate_is_caller maps to '
      'LeaveOutcomeCandidateIsCaller',
      () async {
        when(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenThrow(
          ServiceException('candidate is caller: candidate_is_caller',
              code: Code.invalidArgument),
        );

        final notifier = container.read(leaveCommunityProvider.notifier);
        await notifier.leave(communityId, newOwnerUserId: 'self');

        expect(
          container.read(leaveCommunityProvider),
          isA<LeaveOutcomeCandidateIsCaller>(),
        );
      },
    );

    test(
      'FailedPrecondition: candidate_not_member maps to '
      'LeaveOutcomeCandidateNotMember',
      () async {
        when(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenThrow(
          ServiceException(
              'candidate is not an active member: candidate_not_member',
              code: Code.failedPrecondition),
        );

        final notifier = container.read(leaveCommunityProvider.notifier);
        await notifier.leave(communityId, newOwnerUserId: candidateId);

        expect(
          container.read(leaveCommunityProvider),
          isA<LeaveOutcomeCandidateNotMember>(),
        );
      },
    );

    test(
      'FailedPrecondition: ownership_changed maps to '
      'LeaveOutcomeOwnershipChanged',
      () async {
        when(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenThrow(
          ServiceException('ownership changed: ownership_changed',
              code: Code.failedPrecondition),
        );

        final notifier = container.read(leaveCommunityProvider.notifier);
        await notifier.leave(communityId, newOwnerUserId: candidateId);

        expect(
          container.read(leaveCommunityProvider),
          isA<LeaveOutcomeOwnershipChanged>(),
        );
      },
    );

    test(
      'FailedPrecondition: community_deleted maps to '
      'LeaveOutcomeCommunityDeleted',
      () async {
        when(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenThrow(
          ServiceException('community is deleted: community_deleted',
              code: Code.failedPrecondition),
        );

        final notifier = container.read(leaveCommunityProvider.notifier);
        await notifier.leave(communityId);

        expect(
          container.read(leaveCommunityProvider),
          isA<LeaveOutcomeCommunityDeleted>(),
        );
        verifyNever(mockCommunity.refreshAll(any));
      },
    );

    test(
      'Code.notFound maps to LeaveOutcomeNotFound regardless of message',
      () async {
        when(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenThrow(
          ServiceException('membership not found',
              code: Code.notFound),
        );

        final notifier = container.read(leaveCommunityProvider.notifier);
        await notifier.leave(communityId);

        expect(
          container.read(leaveCommunityProvider),
          isA<LeaveOutcomeNotFound>(),
        );
        verifyNever(mockCommunity.refreshAll(any));
      },
    );

    test(
      'FailedPrecondition with unrecognised suffix falls back to '
      'LeaveOutcomeError',
      () async {
        when(mockCommunity.leaveCommunity(
          communityId,
          newOwnerUserId: anyNamed('newOwnerUserId'),
        )).thenThrow(
          ServiceException('something unexpected went wrong',
              code: Code.failedPrecondition),
        );

        final notifier = container.read(leaveCommunityProvider.notifier);
        await notifier.leave(communityId);

        expect(
          container.read(leaveCommunityProvider),
          isA<LeaveOutcomeError>(),
        );
      },
    );

    test('other ServiceException maps to LeaveOutcomeError', () async {
      when(mockCommunity.leaveCommunity(
        communityId,
        newOwnerUserId: anyNamed('newOwnerUserId'),
      )).thenThrow(
        ServiceException('upstream blew up', code: Code.internal),
      );

      final notifier = container.read(leaveCommunityProvider.notifier);
      await notifier.leave(communityId);

      expect(
        container.read(leaveCommunityProvider),
        isA<LeaveOutcomeError>(),
      );
    });

    test('handles disposal during leave gracefully', () async {
      when(mockCommunity.leaveCommunity(
        communityId,
        newOwnerUserId: anyNamed('newOwnerUserId'),
      )).thenAnswer(
        (_) async => Future.delayed(const Duration(milliseconds: 50)),
      );
      stubFanOutHappyPath();

      final notifier = container.read(leaveCommunityProvider.notifier);
      final future = notifier.leave(communityId);
      container.dispose();

      await expectLater(future, completes);
    });
  });
}
