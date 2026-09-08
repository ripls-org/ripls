import 'package:connectrpc/connect.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/delete_community_view_model.dart';
import 'package:ripls/services/providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'delete_community_view_model_test.mocks.dart';

@GenerateMocks([
  CommunityRepository,
  GearRepository,
  RequestRepository,
  ExperienceRepository,
  TransferRepository,
  SearchRepository,
  FeedRepository,
  StoryRepository,
  ImpactMetricsRepository,
])
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('DeleteCommunityNotifier', () {
    late MockCommunityRepository mockCommunity;
    late MockGearRepository mockGear;
    late MockRequestRepository mockRequest;
    late MockExperienceRepository mockExperience;
    late MockTransferRepository mockTransfer;
    late MockSearchRepository mockSearch;
    late MockFeedRepository mockFeed;
    late MockStoryRepository mockStory;
    late MockImpactMetricsRepository mockImpact;
    late ProviderContainer container;

    const communityId = 'comm-1';

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
      when(mockImpact.invalidateAll(any)).thenAnswer((_) async {});
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
      mockImpact = MockImpactMetricsRepository();

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
          impactMetricsRepositoryProvider.overrideWithValue(mockImpact),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    test('initial state is DeleteOutcomeIdle', () {
      expect(
        container.read(deleteCommunityProvider),
        isA<DeleteOutcomeIdle>(),
      );
    });

    test('successful delete fans out cache invalidations and ends in Success',
        () async {
      when(mockCommunity.deleteCommunity(communityId)).thenAnswer((_) async {});
      stubFanOutHappyPath();

      final notifier = container.read(deleteCommunityProvider.notifier);
      await notifier.delete(communityId);

      expect(
        container.read(deleteCommunityProvider),
        isA<DeleteOutcomeSuccess>(),
      );

      verify(mockCommunity.deleteCommunity(communityId)).called(1);
      verify(mockCommunity.refreshAll(communityId)).called(1);
      verify(mockGear.invalidateForCommunity(communityId)).called(1);
      verify(mockRequest.invalidateForCommunity(communityId)).called(1);
      verify(mockExperience.invalidateForCommunity(communityId)).called(1);
      verify(mockTransfer.invalidateForCommunity(communityId)).called(1);
      verify(mockSearch.invalidateSearchesForCommunity(communityId)).called(1);
      verify(mockFeed.invalidateFeed()).called(1);
      verify(mockStory.invalidateCommunity(communityId)).called(1);
      verify(mockImpact.invalidateAll(communityId)).called(1);
    });

    test('FailedPrecondition maps to DeleteOutcomeAlreadyDeleted', () async {
      when(mockCommunity.deleteCommunity(communityId)).thenThrow(
        ServiceException('already deleted', code: Code.failedPrecondition),
      );

      final notifier = container.read(deleteCommunityProvider.notifier);
      await notifier.delete(communityId);

      expect(
        container.read(deleteCommunityProvider),
        isA<DeleteOutcomeAlreadyDeleted>(),
      );
      // Cache fan-out must NOT run on failure — repos still hold their
      // pre-attempt snapshots.
      verifyNever(mockCommunity.refreshAll(any));
      verifyNever(mockFeed.invalidateFeed());
    });

    test('PermissionDenied maps to DeleteOutcomeNotOwner', () async {
      when(mockCommunity.deleteCommunity(communityId)).thenThrow(
        ServiceException('not owner', code: Code.permissionDenied),
      );

      final notifier = container.read(deleteCommunityProvider.notifier);
      await notifier.delete(communityId);

      expect(
        container.read(deleteCommunityProvider),
        isA<DeleteOutcomeNotOwner>(),
      );
      verifyNever(mockCommunity.refreshAll(any));
    });

    test('other ServiceException maps to DeleteOutcomeError', () async {
      when(mockCommunity.deleteCommunity(communityId)).thenThrow(
        ServiceException('upstream blew up', code: Code.internal),
      );

      final notifier = container.read(deleteCommunityProvider.notifier);
      await notifier.delete(communityId);

      expect(
        container.read(deleteCommunityProvider),
        isA<DeleteOutcomeError>(),
      );
    });

    test('handles disposal during delete gracefully', () async {
      when(mockCommunity.deleteCommunity(communityId)).thenAnswer(
        (_) async => Future.delayed(const Duration(milliseconds: 50)),
      );
      stubFanOutHappyPath();

      final notifier = container.read(deleteCommunityProvider.notifier);
      final future = notifier.delete(communityId);
      container.dispose();

      await expectLater(future, completes);
    });
  });
}
