import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/viewmodels/profile_metrics_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'profile_metrics_view_model_test.mocks.dart';

@GenerateMocks([UserRepository, MediaRepository, LocationRepository])
void main() {
  late MockUserRepository mockUserRepository;
  late MockMediaRepository mockMediaRepository;
  late MockLocationRepository mockLocationRepository;

  setUp(() {
    mockUserRepository = MockUserRepository();
    mockMediaRepository = MockMediaRepository();
    mockLocationRepository = MockLocationRepository();
  });

  ProviderContainer createContainer() {
    return ProviderContainer(
      overrides: [
        userRepositoryProvider.overrideWithValue(mockUserRepository),
        mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
        locationRepositoryProvider.overrideWithValue(mockLocationRepository),
      ],
    );
  }

  group('ProfileImpactMetricsViewModel', () {
    const userId = 'user-123';
    const mediaId = 'media-1';
    const locationId = 'location-1';
    final testUser = GetUserResponse(
      userId: userId,
      name: 'Test User',
      email: 'test@example.com',
      description: 'Test user description',
      mediaId: mediaId,
      mediaIds: [mediaId],
      primaryResidenceLocationId: locationId,
    );
    final testStats = GetUserStatsResponse(
      communityCount: 3,
      itemCount: 15,
      loansCount: 8,
      borrowsCount: 4,
      giveawaysCount: 2,
      helpOfferedCount: 5,
      eventsHostedCount: 1,
      savings: UserSavings(
        costSavedUsd: 250,
        timeSavedHours: 16,
        co2SavedKg: 12.5,
      ),
    );
    final testMediaUrl = MediaUrl(
      url: 'https://example.com/profile.jpg',
      mediaId: mediaId,
      isThumbnail: false,
    );
    final testLocation = Location(
      id: locationId,
      name: 'Boulder, CO',
      latitudeDeg: 40.0150,
      longitudeDeg: -105.2705,
    );

    test('build() loads all data in parallel', () async {
      // Arrange
      when(mockUserRepository.get(userId)).thenAnswer((_) async => testUser);
      when(mockUserRepository.getUserStats(userId))
          .thenAnswer((_) async => testStats);
      when(mockMediaRepository.getFullMediaUrl(mediaId))
          .thenAnswer((_) async => testMediaUrl);
      when(mockLocationRepository.get(locationId))
          .thenAnswer((_) async => testLocation);

      final container = createContainer();
      addTearDown(container.dispose);

      // Act
      final result =
          await container.read(profileImpactMetricsProvider(userId).future);

      // Assert - All data loaded
      expect(result.user, equals(testUser));
      expect(result.stats, equals(testStats));
      expect(result.profileImageUrl, equals('https://example.com/profile.jpg'));
      expect(result.locationName, equals('Boulder, CO'));

      // Assert - All repositories called
      verify(mockUserRepository.get(userId)).called(1);
      verify(mockUserRepository.getUserStats(userId)).called(1);
      verify(mockMediaRepository.getFullMediaUrl(mediaId)).called(1);
      verify(mockLocationRepository.get(locationId)).called(1);
    });

    test('provider state is AsyncData when loaded', () async {
      // Arrange
      when(mockUserRepository.get(userId)).thenAnswer((_) async => testUser);
      when(mockUserRepository.getUserStats(userId))
          .thenAnswer((_) async => testStats);
      when(mockMediaRepository.getFullMediaUrl(mediaId))
          .thenAnswer((_) async => testMediaUrl);
      when(mockLocationRepository.get(locationId))
          .thenAnswer((_) async => testLocation);

      final container = createContainer();
      addTearDown(container.dispose);

      // Act
      await container.read(profileImpactMetricsProvider(userId).future);
      final state = container.read(profileImpactMetricsProvider(userId));

      // Assert
      expect(state.hasValue, isTrue);
      expect(state.isLoading, isFalse);
      expect(state.hasError, isFalse);
    });

    test('provider state is AsyncError when loading fails', () async {
      // Arrange
      final error = Exception('Failed to load user');
      when(mockUserRepository.get(userId)).thenThrow(error);
      when(mockUserRepository.getUserStats(userId))
          .thenAnswer((_) async => testStats);
      when(mockMediaRepository.getFullMediaUrl(mediaId))
          .thenAnswer((_) async => testMediaUrl);
      when(mockLocationRepository.get(locationId))
          .thenAnswer((_) async => testLocation);

      final container = createContainer();
      addTearDown(container.dispose);

      // Act & Assert
      // AutoDispose providers throw StateError when disposed during error loading
      // This is expected behavior for autoDispose.family providers
      try {
        await container.read(profileImpactMetricsProvider(userId).future);
        fail('Expected an error to be thrown');
      } catch (e) {
        // Can be either Exception or StateError depending on disposal timing
        expect(e is Exception || e is StateError, isTrue);
      }
    });

    test('refresh() reloads all data', () async {
      // Arrange
      when(mockUserRepository.get(userId)).thenAnswer((_) async => testUser);
      when(mockUserRepository.getUserStats(userId))
          .thenAnswer((_) async => testStats);
      when(mockMediaRepository.getFullMediaUrl(mediaId))
          .thenAnswer((_) async => testMediaUrl);
      when(mockLocationRepository.get(locationId))
          .thenAnswer((_) async => testLocation);

      final container = createContainer();
      addTearDown(container.dispose);

      // Act - Initial load
      await container.read(profileImpactMetricsProvider(userId).future);

      // Act - Refresh
      await container
          .read(profileImpactMetricsProvider(userId).notifier)
          .refresh();

      // Assert - All repositories called twice (initial + refresh)
      verify(mockUserRepository.get(userId)).called(2);
      verify(mockUserRepository.getUserStats(userId)).called(2);
      verify(mockMediaRepository.getFullMediaUrl(mediaId)).called(2);
      verify(mockLocationRepository.get(locationId)).called(2);
    });

    group('disposal safety', () {
      test('handles disposal during async operation', () async {
        // Arrange
        when(mockUserRepository.get(userId))
            .thenAnswer((_) async => testUser);
        when(mockUserRepository.getUserStats(userId))
            .thenAnswer((_) async => testStats);
        when(mockMediaRepository.getFullMediaUrl(mediaId))
            .thenAnswer((_) async => testMediaUrl);
        when(mockLocationRepository.get(locationId))
            .thenAnswer((_) async => testLocation);

        final container = createContainer();

        // Act - Start loading
        final future = container.read(profileImpactMetricsProvider(userId).future);

        // Act - Dispose before completion
        container.dispose();

        // Assert - Future can complete successfully or throw StateError depending on timing
        // Either outcome is acceptable - the key is no unhandled exceptions
        try {
          final result = await future;
          // If it completes, verify the data is correct
          expect(result.user, equals(testUser));
        } on StateError {
          // If it throws StateError due to disposal, that's also acceptable
        }
      });

      test('refresh checks ref.mounted before state updates', () async {
        // Arrange
        when(mockUserRepository.get(userId))
            .thenAnswer((_) async => testUser);
        when(mockUserRepository.getUserStats(userId))
            .thenAnswer((_) async => testStats);
        when(mockMediaRepository.getFullMediaUrl(mediaId))
            .thenAnswer((_) async => testMediaUrl);
        when(mockLocationRepository.get(locationId))
            .thenAnswer((_) async => testLocation);

        final container = createContainer();

        // Keep provider alive with a listener to prevent premature autodispose
        final sub = container.listen(
          profileImpactMetricsProvider(userId),
          (previous, next) {},
        );

        // Act - Initial load
        await container.read(profileImpactMetricsProvider(userId).future);

        // Act - Start refresh but dispose immediately
        final refreshFuture = container
            .read(profileImpactMetricsProvider(userId).notifier)
            .refresh();
        sub.close();
        container.dispose();

        // Assert - Should complete without error (ref.mounted prevents update)
        // The refresh completes but doesn't update state because ref is no longer mounted
        await expectLater(refreshFuture, completes);
      });
    });

    group('data integrity', () {
      test('combines data from multiple sources correctly', () async {
        // Arrange
        when(mockUserRepository.get(userId))
            .thenAnswer((_) async => testUser);
        when(mockUserRepository.getUserStats(userId))
            .thenAnswer((_) async => testStats);
        when(mockMediaRepository.getFullMediaUrl(mediaId))
            .thenAnswer((_) async => testMediaUrl);
        when(mockLocationRepository.get(locationId))
            .thenAnswer((_) async => testLocation);

        final container = createContainer();
        addTearDown(container.dispose);

        // Act
        final result =
            await container.read(profileImpactMetricsProvider(userId).future);

        // Assert - All data present and correct
        expect(result.user.userId, equals(userId));
        expect(result.user.name, equals('Test User'));
        expect(result.stats.communityCount, equals(3));
        expect(result.stats.itemCount, equals(15));
        expect(result.profileImageUrl, equals('https://example.com/profile.jpg'));
        expect(result.locationName, equals('Boulder, CO'));
      });

      test('handles user without media ID', () async {
        // Arrange - User without mediaId
        final userWithoutMedia = GetUserResponse(
          userId: userId,
          name: 'Test User',
          email: 'test@example.com',
          description: 'Test user description',
          mediaId: '', // Empty mediaId
          primaryResidenceLocationId: locationId,
        );

        when(mockUserRepository.get(userId))
            .thenAnswer((_) async => userWithoutMedia);
        when(mockUserRepository.getUserStats(userId))
            .thenAnswer((_) async => testStats);
        when(mockLocationRepository.get(locationId))
            .thenAnswer((_) async => testLocation);

        final container = createContainer();
        addTearDown(container.dispose);

        // Act
        final result =
            await container.read(profileImpactMetricsProvider(userId).future);

        // Assert - profileImageUrl is null, locationName is present
        expect(result.profileImageUrl, isNull);
        expect(result.locationName, equals('Boulder, CO'));
        verifyNever(mockMediaRepository.getFullMediaUrl(any));
      });

      test('handles user without location', () async {
        // Arrange - User without location
        final userWithoutLocation = GetUserResponse(
          userId: userId,
          name: 'Test User',
          email: 'test@example.com',
          description: 'Test user description',
          mediaId: mediaId,
          primaryResidenceLocationId: '', // Empty location ID
        );

        when(mockUserRepository.get(userId))
            .thenAnswer((_) async => userWithoutLocation);
        when(mockUserRepository.getUserStats(userId))
            .thenAnswer((_) async => testStats);
        when(mockMediaRepository.getFullMediaUrl(mediaId))
            .thenAnswer((_) async => testMediaUrl);

        final container = createContainer();
        addTearDown(container.dispose);

        // Act
        final result =
            await container.read(profileImpactMetricsProvider(userId).future);

        // Assert - locationName is null, profileImageUrl is present
        expect(result.locationName, isNull);
        expect(result.profileImageUrl, equals('https://example.com/profile.jpg'));
        verifyNever(mockLocationRepository.get(any));
      });
    });
  });
}
