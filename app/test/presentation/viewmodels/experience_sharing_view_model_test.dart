import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/presentation/viewmodels/experience_sharing_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'experience_sharing_view_model_test.mocks.dart';

@GenerateMocks([CommunityRepository, ExperienceRepository])
void main() {
  late MockCommunityRepository mockCommunityRepo;
  late MockExperienceRepository mockExperienceRepo;
  late ProviderContainer container;

  setUp(() {
    mockCommunityRepo = MockCommunityRepository();
    mockExperienceRepo = MockExperienceRepository();
    container = ProviderContainer(
      overrides: [
        communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
        experienceRepositoryProvider.overrideWithValue(mockExperienceRepo),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockCommunityRepo);
    reset(mockExperienceRepo);
  });

  group('initial state', () {
    test('starts with empty communities and no loading', () {
      final state =
          container.read(experienceSharingNotifierProvider);
      expect(state.userCommunities, isEmpty);
      expect(state.sharedCommunityIds, isEmpty);
      expect(state.isLoadingCommunities, isFalse);
      expect(state.isSharing, isFalse);
    });
  });

  group('loadUserCommunities', () {
    test('happy path: populates userCommunities', () async {
      final communities = [
        CommunityItem(id: 'c1', name: 'Community 1'),
        CommunityItem(id: 'c2', name: 'Community 2'),
      ];
      when(mockCommunityRepo.listUserCommunities())
          .thenAnswer((_) async => communities);

      final notifier =
          container.read(experienceSharingNotifierProvider.notifier);
      await notifier.loadUserCommunities();

      final state = container.read(experienceSharingNotifierProvider);
      expect(state.userCommunities, hasLength(2));
      expect(state.isLoadingCommunities, isFalse);
    });

    test('error path: sets communitiesError', () async {
      when(mockCommunityRepo.listUserCommunities())
          .thenThrow(Exception('Load failed'));

      final notifier =
          container.read(experienceSharingNotifierProvider.notifier);
      await notifier.loadUserCommunities();

      final state = container.read(experienceSharingNotifierProvider);
      expect(state.isLoadingCommunities, isFalse);
      expect(state.communitiesError, isNotNull);
    });
  });

  group('setSharedCommunities', () {
    test('sets sharedCommunityIds and itemId', () {
      final notifier =
          container.read(experienceSharingNotifierProvider.notifier);
      notifier.setSharedCommunities('exp1', ['c1', 'c2']);

      final state = container.read(experienceSharingNotifierProvider);
      expect(state.sharedCommunityIds, ['c1', 'c2']);
      expect(state.initializedForItemId, 'exp1');
    });
  });

  group('clearInitialization', () {
    test('clears initializedForItemId', () {
      final notifier =
          container.read(experienceSharingNotifierProvider.notifier);
      notifier.setSharedCommunities('exp1', ['c1']);
      notifier.clearInitialization();

      expect(
        container
            .read(experienceSharingNotifierProvider)
            .initializedForItemId,
        isNull,
      );
    });
  });

  group('shareWithCommunity', () {
    test('adds communityId to sharedCommunityIds', () async {
      when(mockExperienceRepo.shareExperience(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async {});

      final notifier =
          container.read(experienceSharingNotifierProvider.notifier);
      await notifier.shareWithCommunity('exp1', 'c1');

      final state = container.read(experienceSharingNotifierProvider);
      expect(state.sharedCommunityIds, contains('c1'));
      expect(state.isSharing, isFalse);
    });

    test('error path: sets sharingError', () async {
      when(mockExperienceRepo.shareExperience(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
      )).thenThrow(Exception('Share failed'));

      final notifier =
          container.read(experienceSharingNotifierProvider.notifier);
      await notifier.shareWithCommunity('exp1', 'c1');

      final state = container.read(experienceSharingNotifierProvider);
      expect(state.isSharing, isFalse);
      expect(state.sharingError, isNotNull);
    });
  });

  group('unshareFromCommunity', () {
    test('removes communityId from sharedCommunityIds', () async {
      when(mockExperienceRepo.unshareExperience(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async {});

      final notifier =
          container.read(experienceSharingNotifierProvider.notifier);
      notifier.setSharedCommunities('exp1', ['c1', 'c2']);
      await notifier.unshareFromCommunity('exp1', 'c1');

      final state = container.read(experienceSharingNotifierProvider);
      expect(state.sharedCommunityIds, isNot(contains('c1')));
      expect(state.sharedCommunityIds, contains('c2'));
    });

    test('error path: sets sharingError', () async {
      when(mockExperienceRepo.unshareExperience(
        experienceId: anyNamed('experienceId'),
        communityId: anyNamed('communityId'),
      )).thenThrow(Exception('Unshare failed'));

      final notifier =
          container.read(experienceSharingNotifierProvider.notifier);
      notifier.setSharedCommunities('exp1', ['c1', 'c2']);
      await notifier.unshareFromCommunity('exp1', 'c1');

      final state = container.read(experienceSharingNotifierProvider);
      expect(state.isSharing, isFalse);
      expect(state.sharingError, isNotNull);
    });
  });
}
