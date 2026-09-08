import 'package:connectrpc/connect.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/presentation/viewmodels/rejoin_community_view_model.dart';
import 'package:ripls/services/providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'rejoin_community_view_model_test.mocks.dart';

@GenerateMocks([
  CommunityRepository,
  ChatRepository,
  FeedRepository,
  StoryRepository,
])
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('RejoinCommunityNotifier', () {
    late MockCommunityRepository mockCommunity;
    late MockChatRepository mockChat;
    late MockFeedRepository mockFeed;
    late MockStoryRepository mockStory;
    late ProviderContainer container;

    const communityId = 'comm-1';

    void stubFanOutHappyPath() {
      when(mockChat.invalidateAll()).thenAnswer((_) async {});
      when(mockFeed.invalidateFeed()).thenAnswer((_) async {});
      when(mockStory.invalidateCommunity(any)).thenAnswer((_) async {});
      // CommunitiesNotifier.reloadCommunities reads through the
      // repo to populate the active-communities list.
      when(mockCommunity.listUserCommunities(
        regionFilter: anyNamed('regionFilter'),
      )).thenAnswer((_) async => []);
    }

    setUp(() {
      SharedPreferences.setMockInitialValues({});

      mockCommunity = MockCommunityRepository();
      mockChat = MockChatRepository();
      mockFeed = MockFeedRepository();
      mockStory = MockStoryRepository();

      container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunity),
          chatRepositoryProvider.overrideWithValue(mockChat),
          feedRepositoryProvider.overrideWithValue(mockFeed),
          storyRepositoryProvider.overrideWithValue(mockStory),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    test('initial state is RejoinOutcomeIdle', () {
      expect(
        container.read(rejoinCommunityProvider),
        isA<RejoinOutcomeIdle>(),
      );
    });

    test('successful rejoin fans out cache invalidations and ends in Success',
        () async {
      when(mockCommunity.rejoinCommunity(communityId))
          .thenAnswer((_) async {});
      stubFanOutHappyPath();

      final notifier = container.read(rejoinCommunityProvider.notifier);
      await notifier.rejoin(communityId);

      expect(
        container.read(rejoinCommunityProvider),
        isA<RejoinOutcomeSuccess>(),
      );

      verify(mockCommunity.rejoinCommunity(communityId)).called(1);
      verify(mockChat.invalidateAll()).called(1);
      verify(mockFeed.invalidateFeed()).called(1);
      verify(mockStory.invalidateCommunity(communityId)).called(1);
    });

    test(
      'FailedPrecondition: rejoin_window_expired maps to '
      'RejoinOutcomeWindowExpired',
      () async {
        when(mockCommunity.rejoinCommunity(communityId)).thenThrow(
          ServiceException('rejoin window expired: rejoin_window_expired',
              code: Code.failedPrecondition),
        );

        final notifier = container.read(rejoinCommunityProvider.notifier);
        await notifier.rejoin(communityId);

        expect(
          container.read(rejoinCommunityProvider),
          isA<RejoinOutcomeWindowExpired>(),
        );
        // Cache fan-out must NOT run on failure.
        verifyNever(mockChat.invalidateAll());
        verifyNever(mockFeed.invalidateFeed());
      },
    );

    test(
      'FailedPrecondition: community_deleted maps to '
      'RejoinOutcomeCommunityDeleted',
      () async {
        when(mockCommunity.rejoinCommunity(communityId)).thenThrow(
          ServiceException('community is deleted: community_deleted',
              code: Code.failedPrecondition),
        );

        final notifier = container.read(rejoinCommunityProvider.notifier);
        await notifier.rejoin(communityId);

        expect(
          container.read(rejoinCommunityProvider),
          isA<RejoinOutcomeCommunityDeleted>(),
        );
      },
    );

    test(
      'FailedPrecondition: already_member maps to RejoinOutcomeAlreadyMember',
      () async {
        when(mockCommunity.rejoinCommunity(communityId)).thenThrow(
          ServiceException('caller is already an active member: already_member',
              code: Code.failedPrecondition),
        );

        final notifier = container.read(rejoinCommunityProvider.notifier);
        await notifier.rejoin(communityId);

        expect(
          container.read(rejoinCommunityProvider),
          isA<RejoinOutcomeAlreadyMember>(),
        );
      },
    );

    test(
      'FailedPrecondition: no_membership_to_rejoin maps to '
      'RejoinOutcomeNoMembership',
      () async {
        when(mockCommunity.rejoinCommunity(communityId)).thenThrow(
          ServiceException(
              'no prior membership in this community: no_membership_to_rejoin',
              code: Code.failedPrecondition),
        );

        final notifier = container.read(rejoinCommunityProvider.notifier);
        await notifier.rejoin(communityId);

        expect(
          container.read(rejoinCommunityProvider),
          isA<RejoinOutcomeNoMembership>(),
        );
      },
    );

    test(
      'FailedPrecondition with unrecognised suffix falls back to '
      'RejoinOutcomeError',
      () async {
        when(mockCommunity.rejoinCommunity(communityId)).thenThrow(
          ServiceException('something unexpected went wrong',
              code: Code.failedPrecondition),
        );

        final notifier = container.read(rejoinCommunityProvider.notifier);
        await notifier.rejoin(communityId);

        expect(
          container.read(rejoinCommunityProvider),
          isA<RejoinOutcomeError>(),
        );
      },
    );

    test('other ServiceException maps to RejoinOutcomeError', () async {
      when(mockCommunity.rejoinCommunity(communityId)).thenThrow(
        ServiceException('upstream blew up', code: Code.internal),
      );

      final notifier = container.read(rejoinCommunityProvider.notifier);
      await notifier.rejoin(communityId);

      expect(
        container.read(rejoinCommunityProvider),
        isA<RejoinOutcomeError>(),
      );
    });

    test('handles disposal during rejoin gracefully', () async {
      when(mockCommunity.rejoinCommunity(communityId)).thenAnswer(
        (_) async => Future.delayed(const Duration(milliseconds: 50)),
      );
      stubFanOutHappyPath();

      final notifier = container.read(rejoinCommunityProvider.notifier);
      final future = notifier.rejoin(communityId);
      container.dispose();

      await expectLater(future, completes);
    });
  });
}
