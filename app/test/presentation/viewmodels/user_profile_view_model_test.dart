import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/viewmodels/user_profile_view_model.dart';
import 'package:ripls/services/providers.dart';

@GenerateMocks([
  UserRepository,
  CommunityRepository,
])
import 'user_profile_view_model_test.mocks.dart';

void main() {
  late MockUserRepository mockUserRepository;
  late MockCommunityRepository mockCommunityRepository;
  late ProviderContainer container;

  setUp(() {
    mockUserRepository = MockUserRepository();
    mockCommunityRepository = MockCommunityRepository();

    container = ProviderContainer(
      overrides: [
        userRepositoryProvider.overrideWithValue(mockUserRepository),
        communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('UserProfileViewModel Tab Selection', () {
    test('initial state has selectedTabIndex = 0', () {
      final state = container.read(userProfileViewModelProvider);

      expect(state.selectedTabIndex, 0);
    });

    test('selectTab updates selectedTabIndex', () {
      final notifier = container.read(userProfileViewModelProvider.notifier);

      // Select tab 1 (Skills)
      notifier.selectTab(1);
      expect(container.read(userProfileViewModelProvider).selectedTabIndex, 1);

      // Select tab 2 (Communities)
      notifier.selectTab(2);
      expect(container.read(userProfileViewModelProvider).selectedTabIndex, 2);

      // Select tab 0 (Impact)
      notifier.selectTab(0);
      expect(container.read(userProfileViewModelProvider).selectedTabIndex, 0);
    });

    test('selectTab ignores invalid indices', () {
      final notifier = container.read(userProfileViewModelProvider.notifier);

      // Set to valid tab first
      notifier.selectTab(1);
      expect(container.read(userProfileViewModelProvider).selectedTabIndex, 1);

      // Try invalid indices - should not change
      notifier.selectTab(-1);
      expect(container.read(userProfileViewModelProvider).selectedTabIndex, 1);

      notifier.selectTab(3);
      expect(container.read(userProfileViewModelProvider).selectedTabIndex, 1);

      notifier.selectTab(999);
      expect(container.read(userProfileViewModelProvider).selectedTabIndex, 1);
    });

    test('tab selection persists across state updates', () async {
      final notifier = container.read(userProfileViewModelProvider.notifier);

      // Mock responses
      when(mockUserRepository.getUserProfile(any)).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'user123',
            name: 'Test User',
            email: 'test@example.com',
          ),
          mediaUrl: null,
        ),
      );

      when(mockUserRepository.getUserMediaUrls(any))
          .thenAnswer((_) async => []);

      when(mockCommunityRepository.getEvents(any))
          .thenAnswer((_) async => []);

      // Select tab 2
      notifier.selectTab(2);
      expect(container.read(userProfileViewModelProvider).selectedTabIndex, 2);

      // Initialize (loads data)
      await notifier.initialize('user123');

      // Tab selection should persist
      expect(container.read(userProfileViewModelProvider).selectedTabIndex, 2);
    });
  });

  group('UserProfileViewModel State Management', () {
    test('initialize loads user data', () async {
      final notifier = container.read(userProfileViewModelProvider.notifier);

      when(mockUserRepository.getUserProfile(any)).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'user123',
            name: 'Test User',
            email: 'test@example.com',
          ),
          mediaUrl: null,
        ),
      );

      when(mockUserRepository.getUserMediaUrls(any))
          .thenAnswer((_) async => []);

      when(mockCommunityRepository.getEvents(any))
          .thenAnswer((_) async => []);

      await notifier.initialize('user123');

      final state = container.read(userProfileViewModelProvider);
      expect(state.user?.name, 'Test User');
      expect(state.isLoadingUser, false);
      expect(state.userError, null);
    });

    test('handles user loading error gracefully', () async {
      final notifier = container.read(userProfileViewModelProvider.notifier);

      when(mockUserRepository.getUserProfile(any))
          .thenThrow(Exception('Network error'));

      when(mockUserRepository.getUserMediaUrls(any))
          .thenThrow(Exception('Network error'));

      when(mockCommunityRepository.getEvents(any))
          .thenAnswer((_) async => []);

      await notifier.initialize('user123');

      final state = container.read(userProfileViewModelProvider);
      expect(state.userError, isNotNull);
      expect(state.isLoadingUser, false);
    });
  });
}
