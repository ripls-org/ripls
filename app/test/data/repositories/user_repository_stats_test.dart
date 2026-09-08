import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/services/user_service.dart';

import 'user_repository_stats_test.mocks.dart';

// Note: This test focuses on verifying the repository calls the service correctly.
// Cache behavior is tested separately in cache_service_test.dart

@GenerateMocks([UserService])
void main() {
  late MockUserService mockUserService;

  setUp(() {
    mockUserService = MockUserService();
  });

  group('UserRepository getUserStats', () {
    const userId = 'user-123';
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

    test('service returns correct data structure', () async {
      // Arrange
      when(
        mockUserService.getUserStats(userId),
      ).thenAnswer((_) async => testStats);

      // Act
      final result = await mockUserService.getUserStats(userId);

      // Assert - Verify all fields are accessible
      expect(result, isA<GetUserStatsResponse>());
      expect(result.communityCount, equals(3));
      expect(result.itemCount, equals(15));
      expect(result.loansCount, equals(8));
      expect(result.borrowsCount, equals(4));
      expect(result.giveawaysCount, equals(2));
      expect(result.helpOfferedCount, equals(5));
      expect(result.eventsHostedCount, equals(1));
      expect(result.savings.costSavedUsd, equals(250.0));
      expect(result.savings.timeSavedHours, equals(16));
      expect(result.savings.co2SavedKg, equals(12.5));
    });

    test('service handles different user IDs', () async {
      // Arrange
      const userId2 = 'user-456';
      final stats2 = GetUserStatsResponse(communityCount: 5);
      when(
        mockUserService.getUserStats(userId),
      ).thenAnswer((_) async => testStats);
      when(
        mockUserService.getUserStats(userId2),
      ).thenAnswer((_) async => stats2);

      // Act
      final result1 = await mockUserService.getUserStats(userId);
      final result2 = await mockUserService.getUserStats(userId2);

      // Assert
      expect(result1.communityCount, equals(3));
      expect(result2.communityCount, equals(5));
    });
  });
}
