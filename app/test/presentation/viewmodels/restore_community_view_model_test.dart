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
import 'package:ripls/presentation/viewmodels/restore_community_view_model.dart';
import 'package:ripls/services/providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'restore_community_view_model_test.mocks.dart';

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

  group('RestoreCommunityNotifier', () {
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
      // CommunitiesNotifier.reloadCommunities reads through the
      // repo to populate the active-communities list — needed so the
      // sidebar / "who can see" pickers update after delete + restore.
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
      // CommunitiesNotifier.setCommunities reads SharedPreferences
      // to resolve the persisted single-select state. Stub before each
      // test so the post-restore reloadCommunities call doesn't blow up.
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

    test('initial state is RestoreOutcomeIdle', () {
      expect(
        container.read(restoreCommunityProvider),
        isA<RestoreOutcomeIdle>(),
      );
    });

    test('successful restore fans out cache invalidations and ends in Success',
        () async {
      when(mockCommunity.restoreCommunity(communityId))
          .thenAnswer((_) async {});
      stubFanOutHappyPath();

      final notifier = container.read(restoreCommunityProvider.notifier);
      await notifier.restore(communityId);

      expect(
        container.read(restoreCommunityProvider),
        isA<RestoreOutcomeSuccess>(),
      );

      // Verify the §7.5 fan-out fired in full.
      verify(mockCommunity.restoreCommunity(communityId)).called(1);
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

    test('FailedPrecondition maps to RestoreOutcomeAlreadyRestored', () async {
      when(mockCommunity.restoreCommunity(communityId)).thenThrow(
        ServiceException('not in deleted state', code: Code.failedPrecondition),
      );

      final notifier = container.read(restoreCommunityProvider.notifier);
      await notifier.restore(communityId);

      expect(
        container.read(restoreCommunityProvider),
        isA<RestoreOutcomeAlreadyRestored>(),
      );
      // Cache fan-out must not run on failure — the deleted-list and
      // related caches still hold their pre-attempt snapshots.
      verifyNever(mockCommunity.refreshAll(any));
      verifyNever(mockFeed.invalidateFeed());
    });

    test('PermissionDenied maps to RestoreOutcomeNoLongerEligible', () async {
      when(mockCommunity.restoreCommunity(communityId)).thenThrow(
        ServiceException('not in restorer snapshot',
            code: Code.permissionDenied),
      );

      final notifier = container.read(restoreCommunityProvider.notifier);
      await notifier.restore(communityId);

      expect(
        container.read(restoreCommunityProvider),
        isA<RestoreOutcomeNoLongerEligible>(),
      );
      verifyNever(mockCommunity.refreshAll(any));
    });

    test('other ServiceException maps to RestoreOutcomeError', () async {
      when(mockCommunity.restoreCommunity(communityId)).thenThrow(
        ServiceException('upstream blew up', code: Code.internal),
      );

      final notifier = container.read(restoreCommunityProvider.notifier);
      await notifier.restore(communityId);

      expect(
        container.read(restoreCommunityProvider),
        isA<RestoreOutcomeError>(),
      );
    });

    test('handles disposal during restore gracefully', () async {
      when(mockCommunity.restoreCommunity(communityId)).thenAnswer(
        (_) async => Future.delayed(const Duration(milliseconds: 50)),
      );
      stubFanOutHappyPath();

      final notifier = container.read(restoreCommunityProvider.notifier);
      final future = notifier.restore(communityId);
      container.dispose();

      await expectLater(future, completes);
    });
  });
}
