import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/repositories/unread_count_repository.dart';
import 'package:ripls/services/chat_service.dart';

import 'unread_count_repository_test.mocks.dart';

@GenerateMocks([CacheManager, ChatService])
void main() {
  late UnreadCountRepository repository;
  late MockCacheManager mockCacheManager;
  late MockChatService mockChatService;

  setUp(() {
    mockCacheManager = MockCacheManager();
    mockChatService = MockChatService();
    repository = UnreadCountRepository(mockCacheManager, mockChatService);
  });

  tearDown(() {
    reset(mockCacheManager);
    reset(mockChatService);
  });

  group('getTotalUnreadCount', () {
    test('returns server count from cached response', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 5,
        communityIdToUnreadCount: {'comm-1': 3, 'comm-2': 2},
        conversationIdToUnreadCount: {'conv-1': 3, 'conv-2': 2},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      final count = await repository.getTotalUnreadCount();

      expect(count, 5);
      verify(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).called(1);
    });

    test('fetches count from server on cache miss', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 8,
        communityIdToUnreadCount: {'comm-1': 5, 'comm-2': 3},
        conversationIdToUnreadCount: {'conv-1': 5, 'conv-2': 3},
      );

      when(mockChatService.getUnreadCounts())
          .thenAnswer((_) async => response);

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((invocation) async {
        final fetch = invocation.namedArguments[const Symbol('fetch')]
            as Future<GetUnreadCountsResponse> Function();
        return fetch();
      });

      final count = await repository.getTotalUnreadCount();

      expect(count, 8);
      verify(mockChatService.getUnreadCounts()).called(1);
    });

    test('applies optimistic deltas to server count', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 5,
        communityIdToUnreadCount: {'comm-1': 5},
        conversationIdToUnreadCount: {'conv-1': 5},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      // Add optimistic increments
      repository.incrementUnreadCount('comm-1', 'conv-1');
      repository.incrementUnreadCount('comm-1', 'conv-2');

      final count = await repository.getTotalUnreadCount();

      expect(count, 7); // 5 + 2
    });

    test('clamps negative total to 0', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 2,
        communityIdToUnreadCount: {'comm-1': 2},
        conversationIdToUnreadCount: {'conv-1': 2},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      // Reset count (creates negative delta)
      await repository.resetUnreadCount('comm-1', 'conv-1');

      final count = await repository.getTotalUnreadCount();

      expect(count, 0); // Should not be negative
    });
  });

  group('getUnreadCounts', () {
    test('returns map of conversation id to unread count', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 3,
        communityIdToUnreadCount: {'comm-1': 3},
        conversationIdToUnreadCount: {'conv-1': 3, 'conv-2': 0},
      );

      when(mockChatService.getUnreadCounts())
          .thenAnswer((_) async => response);

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((invocation) async {
        final fetch = invocation.namedArguments[const Symbol('fetch')]
            as Future<GetUnreadCountsResponse> Function();
        return fetch();
      });

      final counts = await repository.getUnreadCounts();

      expect(counts, {'conv-1': 3, 'conv-2': 0});
    });
  });

  group('getUnreadCountsByCommunity', () {
    test('returns map of community id to unread count', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 8,
        communityIdToUnreadCount: {'comm-1': 5, 'comm-2': 3},
        conversationIdToUnreadCount: {'conv-1': 5, 'conv-2': 3},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      final counts = await repository.getUnreadCountsByCommunity();

      expect(counts, {'comm-1': 5, 'comm-2': 3});
    });

    test('applies optimistic deltas grouped by community', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 5,
        communityIdToUnreadCount: {'comm-1': 3, 'comm-2': 2},
        conversationIdToUnreadCount: {'conv-1': 3, 'conv-2': 2},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      // Add optimistic increments for different communities
      repository.incrementUnreadCount('comm-1', 'conv-1');
      repository.incrementUnreadCount('comm-1', 'conv-3');
      repository.incrementUnreadCount('comm-2', 'conv-2');

      final counts = await repository.getUnreadCountsByCommunity();

      expect(counts['comm-1'], 5); // 3 + 2
      expect(counts['comm-2'], 3); // 2 + 1
    });
  });

  group('getUnreadCountForCommunity', () {
    test('returns count for specific community', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 8,
        communityIdToUnreadCount: {'comm-1': 5, 'comm-2': 3},
        conversationIdToUnreadCount: {},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      final count = await repository.getUnreadCountForCommunity('comm-1');

      expect(count, 5);
    });

    test('returns 0 for community with no unread messages', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 5,
        communityIdToUnreadCount: {'comm-1': 5},
        conversationIdToUnreadCount: {},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      final count = await repository.getUnreadCountForCommunity('comm-2');

      expect(count, 0);
    });
  });

  group('getUnreadCount', () {
    test('returns count for specific conversation with optimistic delta', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 5,
        communityIdToUnreadCount: {'comm-1': 5},
        conversationIdToUnreadCount: {'conv-1': 5},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      repository.incrementUnreadCount('comm-1', 'conv-1');

      final count = await repository.getUnreadCount('comm-1', 'conv-1');

      expect(count, 6); // 5 + 1
    });
  });

  group('invalidate', () {
    test('clears all unread cache patterns and optimistic deltas', () async {
      when(mockCacheManager.clear(pattern: 'unread:*'))
          .thenAnswer((_) async => 1);

      // Add some optimistic deltas
      repository.incrementUnreadCount('comm-1', 'conv-1');
      repository.incrementUnreadCount('comm-2', 'conv-2');

      await repository.invalidate();

      verify(mockCacheManager.clear(pattern: 'unread:*')).called(1);
    });
  });

  group('invalidateCommunity', () {
    test('clears cache and removes community deltas', () async {
      when(mockCacheManager.clear(pattern: 'unread:*'))
          .thenAnswer((_) async => 1);

      // Add optimistic deltas for multiple communities
      repository.incrementUnreadCount('comm-1', 'conv-1');
      repository.incrementUnreadCount('comm-1', 'conv-2');
      repository.incrementUnreadCount('comm-2', 'conv-3');

      await repository.invalidateCommunity('comm-1');

      verify(mockCacheManager.clear(pattern: 'unread:*')).called(1);
    });
  });

  group('refreshTotalUnreadCount', () {
    test('invalidates and refetches count', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 7,
        communityIdToUnreadCount: {'comm-1': 7},
        conversationIdToUnreadCount: {'conv-1': 7},
      );

      when(mockCacheManager.clear(pattern: 'unread:*'))
          .thenAnswer((_) async => 1);

      when(mockChatService.getUnreadCounts())
          .thenAnswer((_) async => response);

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((invocation) async {
        final fetch = invocation.namedArguments[const Symbol('fetch')]
            as Future<GetUnreadCountsResponse> Function();
        return fetch();
      });

      final count = await repository.refreshTotalUnreadCount();

      expect(count, 7);
      verify(mockCacheManager.clear(pattern: 'unread:*')).called(1);
      verify(mockChatService.getUnreadCounts()).called(1);
    });
  });

  group('optimistic updates', () {
    test('incrementUnreadCount adds to delta map with composite key', () async {
      repository.incrementUnreadCount('comm-1', 'conv-1');
      repository.incrementUnreadCount('comm-1', 'conv-1');

      final response = GetUnreadCountsResponse(
        totalUnreadCount: 5,
        communityIdToUnreadCount: {'comm-1': 5},
        conversationIdToUnreadCount: {'conv-1': 5},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      final count = await repository.getUnreadCount('comm-1', 'conv-1');
      expect(count, 7); // 5 + 2
    });

    test('resetUnreadCount sets negative delta to zero out server count', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 5,
        communityIdToUnreadCount: {'comm-1': 5},
        conversationIdToUnreadCount: {'conv-1': 5},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      await repository.resetUnreadCount('comm-1', 'conv-1');

      final count = await repository.getUnreadCount('comm-1', 'conv-1');
      expect(count, 0); // 5 + (-5)
    });
  });

  group('syncWithServer', () {
    test('clears delta when server count is 0', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 1,
        communityIdToUnreadCount: {'comm-1': 1},
        conversationIdToUnreadCount: {'conv-1': 1},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      // Simulate user reading a message - set negative delta
      await repository.resetUnreadCount('comm-1', 'conv-1');

      // Verify delta exists and makes count 0
      final countBeforeSync = await repository.getUnreadCount('comm-1', 'conv-1');
      expect(countBeforeSync, 0); // 1 + (-1)

      // Server has processed the read, now returns 0
      final conversations = [
        ConversationItem(
          conversationId: 'conv-1',
          communityId: 'comm-1',
          unreadCount: 0, // Server count is now 0
        ),
      ];

      await repository.syncWithServer(conversations);

      // Now mock cache to return server count of 0
      final updatedResponse = GetUnreadCountsResponse(
        totalUnreadCount: 0,
        communityIdToUnreadCount: {'comm-1': 0},
        conversationIdToUnreadCount: {'conv-1': 0},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => updatedResponse);

      // Count should still be 0 (delta should be cleared, not applied)
      final countAfterSync = await repository.getUnreadCount('comm-1', 'conv-1');
      expect(countAfterSync, 0); // Delta was cleared, so 0 + 0 = 0
    });

    test('does not clear delta when server count is non-zero', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 5,
        communityIdToUnreadCount: {'comm-1': 5},
        conversationIdToUnreadCount: {'conv-1': 5},
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      // Add optimistic delta
      repository.incrementUnreadCount('comm-1', 'conv-1');

      final countBeforeSync = await repository.getUnreadCount('comm-1', 'conv-1');
      expect(countBeforeSync, 6); // 5 + 1

      // Server still has unread count of 5
      final conversations = [
        ConversationItem(
          conversationId: 'conv-1',
          communityId: 'comm-1',
          unreadCount: 5,
        ),
      ];

      await repository.syncWithServer(conversations);

      // Delta should still be applied since server count is not 0
      final countAfterSync = await repository.getUnreadCount('comm-1', 'conv-1');
      expect(countAfterSync, 6); // Delta preserved, so 5 + 1 = 6
    });

    test('handles multiple conversations with mixed states', () async {
      final response = GetUnreadCountsResponse(
        totalUnreadCount: 5,
        communityIdToUnreadCount: {'comm-1': 5},
        conversationIdToUnreadCount: {
          'conv-1': 1,
          'conv-2': 2,
          'conv-3': 2,
        },
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => response);

      // Set negative deltas for all conversations
      await repository.resetUnreadCount('comm-1', 'conv-1');
      await repository.resetUnreadCount('comm-1', 'conv-2');
      await repository.resetUnreadCount('comm-1', 'conv-3');

      // Server has processed reads for conv-1 (0) and conv-2 (0), but not conv-3 (still 2)
      final conversations = [
        ConversationItem(
          conversationId: 'conv-1',
          communityId: 'comm-1',
          unreadCount: 0,
        ),
        ConversationItem(
          conversationId: 'conv-2',
          communityId: 'comm-1',
          unreadCount: 0,
        ),
        ConversationItem(
          conversationId: 'conv-3',
          communityId: 'comm-1',
          unreadCount: 2,
        ),
      ];

      await repository.syncWithServer(conversations);

      // Update mock to return new server state
      final updatedResponse = GetUnreadCountsResponse(
        totalUnreadCount: 2,
        communityIdToUnreadCount: {'comm-1': 2},
        conversationIdToUnreadCount: {
          'conv-1': 0,
          'conv-2': 0,
          'conv-3': 2,
        },
      );

      when(mockCacheManager.get<GetUnreadCountsResponse>(
        key: 'unread:all',
        fetch: anyNamed('fetch'),
      )).thenAnswer((_) async => updatedResponse);

      // conv-1 and conv-2 should have cleared deltas (server = 0)
      final count1 = await repository.getUnreadCount('comm-1', 'conv-1');
      expect(count1, 0);

      final count2 = await repository.getUnreadCount('comm-1', 'conv-2');
      expect(count2, 0);

      // conv-3 should still have delta applied (server != 0)
      final count3 = await repository.getUnreadCount('comm-1', 'conv-3');
      expect(count3, 0); // 2 + (-2)
    });
  });
}
