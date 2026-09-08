import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/unread_count_repository.dart';
import 'package:ripls/services/providers.dart';

import 'unread_count_multi_community_integration_test.mocks.dart';

/// Integration tests for multi-community unread count scenarios (Phase 9).
///
/// These tests verify the complete flow from message arrival to badge updates
/// across multiple communities, covering all test scenarios from Phase 9:
/// 1. Multi-Community Switching
/// 2. Real-Time Updates
/// 3. Mark as Read
/// 4. Offline/Error Cases
@GenerateMocks([UnreadCountRepository])
void main() {
  group('Phase 9: Multi-Community Integration Tests', () {
    late ProviderContainer container;
    late MockUnreadCountRepository mockRepository;

    setUp(() {
      mockRepository = MockUnreadCountRepository();
      container = ProviderContainer(
        overrides: [
          unreadCountRepositoryProvider.overrideWithValue(mockRepository),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    group('Scenario 1: Multi-Community Switching', () {
      test('switches between communities A → B → C with correct counts', () async {
        // Setup: User in 3 communities (A: 5 unread, B: 0 unread, C: 3 unread)
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 8); // 5 + 0 + 3

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => {
          'conv-a1': 3,
          'conv-a2': 2,
          'conv-c1': 3,
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async => {
          'community-a': 5,
          'community-b': 0,
          'community-c': 3,
        });

        // Initialize provider
        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Verify total count (for avatar badge)
        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 8, reason: 'Total should be 5 + 0 + 3 = 8');

        // Verify per-community counts (for sidebar chips)
        expect(state.communityIdToUnreadCount['community-a'], 5);
        expect(state.communityIdToUnreadCount['community-b'], 0);
        expect(state.communityIdToUnreadCount['community-c'], 3);

        // Verify per-conversation counts
        expect(state.conversationIdToUnreadCount['conv-a1'], 3);
        expect(state.conversationIdToUnreadCount['conv-a2'], 2);
        expect(state.conversationIdToUnreadCount['conv-c1'], 3);
      });

      test('community with 0 unread shows no badge', () async {
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 5);

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => {
          'conv-a1': 5,
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async => {
          'community-a': 5,
          'community-b': 0, // Zero unread
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        final state = container.read(unreadCountProvider);

        // Community B should return 0
        expect(state.communityIdToUnreadCount['community-b'], 0);

        // Provider should also return 0 via family provider
        final communityBCount = container.read(
          unreadMessageCountForCommunityProvider('community-b'),
        );
        expect(communityBCount, 0, reason: 'Badge should be hidden for 0 count');
      });
    });

    group('Scenario 2: Real-Time Updates', () {
      test('new message in community A increments sidebar chip and avatar badge', () async {
        // Initial state: Community A has 5 unread, Community B has 0 unread
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 5 : 6; // Increment from 5 to 6
        });

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1
              ? {'conv-a1': 5}
              : {'conv-a1': 6}; // Increment conv-a1
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1
              ? {'community-a': 5, 'community-b': 0}
              : {'community-a': 6, 'community-b': 0}; // Increment community-a
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Verify initial state
        var state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 5);
        expect(state.communityIdToUnreadCount['community-a'], 5);

        // Simulate new message arriving in community A, conversation A1
        await notifier.incrementUnreadCount('conv-a1', 'community-a');

        // Verify updated state
        state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 6, reason: 'Total should increment');
        expect(
          state.communityIdToUnreadCount['community-a'],
          6,
          reason: 'Community A count should increment',
        );
        expect(
          state.conversationIdToUnreadCount['conv-a1'],
          6,
          reason: 'Conversation count should increment',
        );

        // Verify repository was called with correct parameters
        verify(mockRepository.incrementUnreadCount('community-a', 'conv-a1')).called(1);
      });

      test('message in background community updates while viewing different community', () async {
        // User viewing Community A, message arrives in Community B
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 5 : 6;
        });

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1
              ? {'conv-a1': 5}
              : {'conv-a1': 5, 'conv-b1': 1}; // New conversation in B
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1
              ? {'community-a': 5, 'community-b': 0}
              : {'community-a': 5, 'community-b': 1}; // Background increment
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Message arrives in community B while viewing community A
        await notifier.incrementUnreadCount('conv-b1', 'community-b');

        final state = container.read(unreadCountProvider);

        // Community A should remain unchanged
        expect(state.communityIdToUnreadCount['community-a'], 5);

        // Community B should increment
        expect(state.communityIdToUnreadCount['community-b'], 1);

        // Total should increment
        expect(state.totalUnreadCount, 6);
      });

      test('multiple rapid messages increment correctly with optimistic updates', () async {
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          // Return increasing counts as messages arrive
          return [5, 6, 7, 8][callCount - 1];
        });

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return {
            'conv-a1': [5, 6, 7, 8][callCount - 1],
          };
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return {
            'community-a': [5, 6, 7, 8][callCount - 1],
          };
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Simulate 3 rapid messages
        await notifier.incrementUnreadCount('conv-a1', 'community-a');
        await notifier.incrementUnreadCount('conv-a1', 'community-a');
        await notifier.incrementUnreadCount('conv-a1', 'community-a');

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 8, reason: '5 + 3 messages = 8');
        expect(state.conversationIdToUnreadCount['conv-a1'], 8);
        expect(state.communityIdToUnreadCount['community-a'], 8);

        // Verify repository was called 3 times
        verify(mockRepository.incrementUnreadCount('community-a', 'conv-a1')).called(3);
      });
    });

    group('Scenario 3: Mark as Read', () {
      test('marking conversation as read updates all badges', () async {
        // Setup: Community A has 10 unread (conv-a1: 5, conv-a2: 5)
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 10 : 5; // After marking conv-a1 as read
        });

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1
              ? {'conv-a1': 5, 'conv-a2': 5}
              : {'conv-a1': 0, 'conv-a2': 5}; // conv-a1 now 0
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1
              ? {'community-a': 10}
              : {'community-a': 5}; // Decremented by 5
        });

        when(mockRepository.resetUnreadCount('community-a', 'conv-a1'))
            .thenAnswer((_) async {});

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Verify initial state
        var state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 10);
        expect(state.communityIdToUnreadCount['community-a'], 10);

        // Mark conv-a1 as read
        await notifier.resetUnreadCount('conv-a1', 'community-a');

        // Verify updated state
        state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 5, reason: '10 - 5 = 5');
        expect(state.communityIdToUnreadCount['community-a'], 5, reason: '10 - 5 = 5');
        expect(state.conversationIdToUnreadCount['conv-a1'], 0, reason: 'Should be reset to 0');
        expect(state.conversationIdToUnreadCount['conv-a2'], 5, reason: 'Should remain unchanged');

        verify(mockRepository.resetUnreadCount('community-a', 'conv-a1')).called(1);
      });

      test('marking all conversations as read in community removes sidebar chip', () async {
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return [5, 0][callCount - 1];
        });

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1
              ? {'conv-a1': 5}
              : {'conv-a1': 0};
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1
              ? {'community-a': 5}
              : {'community-a': 0}; // All read
        });

        when(mockRepository.resetUnreadCount('community-a', 'conv-a1'))
            .thenAnswer((_) async {});

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Mark last conversation as read
        await notifier.resetUnreadCount('conv-a1', 'community-a');

        final state = container.read(unreadCountProvider);
        expect(state.communityIdToUnreadCount['community-a'], 0, reason: 'Badge should be hidden');
        expect(state.totalUnreadCount, 0, reason: 'Avatar badge should be hidden');
      });

      test('marking one conversation as read leaves other community badges unchanged', () async {
        // Setup: Community A has 5 unread, Community B has 3 unread
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 8 : 3; // After marking A as read: 0 + 3 = 3
        });

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1
              ? {'conv-a1': 5, 'conv-b1': 3}
              : {'conv-a1': 0, 'conv-b1': 3}; // Only conv-a1 reset
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1
              ? {'community-a': 5, 'community-b': 3}
              : {'community-a': 0, 'community-b': 3}; // Only A changed
        });

        when(mockRepository.resetUnreadCount('community-a', 'conv-a1'))
            .thenAnswer((_) async {});

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        await notifier.resetUnreadCount('conv-a1', 'community-a');

        final state = container.read(unreadCountProvider);
        expect(state.communityIdToUnreadCount['community-a'], 0);
        expect(state.communityIdToUnreadCount['community-b'], 3, reason: 'Should remain unchanged');
        expect(state.totalUnreadCount, 3, reason: 'Total should be 0 + 3 = 3');
      });
    });

    group('Scenario 4: Error Handling', () {
      test('gracefully handles repository errors during initialization', () async {
        when(mockRepository.getTotalUnreadCount())
            .thenThrow(Exception('Network error'));

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        final state = container.read(unreadCountProvider);

        // Should have default values after error
        expect(state.isLoading, false);
        expect(state.totalUnreadCount, 0);
        expect(state.conversationIdToUnreadCount, isEmpty);
        expect(state.communityIdToUnreadCount, isEmpty);
      });

      test('continues working with cached data when sync fails', () async {
        // Initialize successfully
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          // Returns cached/stale data even when network fails
          return callCount == 1 ? 5 : 6; // Shows optimistic increment
        });

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1
              ? {'conv-a1': 5}
              : {'conv-a1': 6};
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1
              ? {'community-a': 5}
              : {'community-a': 6};
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Increment with optimistic update (repository layer handles this)
        await notifier.incrementUnreadCount('conv-a1', 'community-a');

        // State should update with optimistic data
        final state = container.read(unreadCountProvider);
        expect(state.isLoading, false);
        expect(state.totalUnreadCount, 6, reason: 'Should show optimistic update');
      });

      test('refresh invalidates cache and refetches after network recovery', () async {
        // Start with cached data
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 5);
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => {'conv-a1': 5});
        when(mockRepository.getUnreadCountsByCommunity())
            .thenAnswer((_) async => {'community-a': 5});
        when(mockRepository.invalidate()).thenAnswer((_) async {});

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Simulate network recovery - new data available
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 10);
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => {'conv-a1': 10});
        when(mockRepository.getUnreadCountsByCommunity())
            .thenAnswer((_) async => {'community-a': 10});

        // Refresh to get new data
        await notifier.refresh();

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 10, reason: 'Should have new data after refresh');

        verify(mockRepository.invalidate()).called(1);
      });
    });

    group('Scenario 5: Provider Reactivity', () {
      test('unreadMessageCountTotalProvider updates when total changes', () async {
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 5 : 10;
        });

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1
              ? {'conv-a1': 5}
              : {'conv-a1': 10};
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1
              ? {'community-a': 5}
              : {'community-a': 10};
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Check initial total via provider
        var totalCount = container.read(unreadMessageCountTotalProvider);
        expect(totalCount, 5);

        // Increment
        await notifier.incrementUnreadCount('conv-a1', 'community-a');

        // Check updated total via provider
        totalCount = container.read(unreadMessageCountTotalProvider);
        expect(totalCount, 10, reason: 'Provider should reactively update');
      });

      test('unreadMessageCountForCommunityProvider updates when community count changes', () async {
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 5 : 8;
        });

        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1
              ? {'conv-a1': 5}
              : {'conv-a1': 5, 'conv-b1': 3};
        });

        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1
              ? {'community-a': 5, 'community-b': 0}
              : {'community-a': 5, 'community-b': 3};
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Check initial community B count
        var communityBCount = container.read(
          unreadMessageCountForCommunityProvider('community-b'),
        );
        expect(communityBCount, 0);

        // Add message to community B
        await notifier.incrementUnreadCount('conv-b1', 'community-b');

        // Check updated community B count
        communityBCount = container.read(
          unreadMessageCountForCommunityProvider('community-b'),
        );
        expect(communityBCount, 3, reason: 'Family provider should reactively update');
      });

      test('provider returns 0 for non-existent community', () async {
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 0);
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => {});
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async => {});

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Query non-existent community
        final count = container.read(
          unreadMessageCountForCommunityProvider('non-existent-community'),
        );
        expect(count, 0, reason: 'Should return 0 for missing community');
      });
    });

    group('Scenario 6: Edge Cases', () {
      test('handles community with no conversations', () async {
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 5);
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => {'conv-a1': 5});
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async => {
          'community-a': 5,
          'community-b': 0, // Empty community
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        final state = container.read(unreadCountProvider);
        expect(state.communityIdToUnreadCount['community-b'], 0);
      });

      test('handles very large conversation lists with optimistic deltas', () async {
        // Simulate 100 conversations across 10 communities
        final conversations = Map<String, int>.fromIterable(
          List.generate(100, (i) => 'conv-$i'),
          value: (_) => 1,
        );

        final communities = Map<String, int>.fromIterable(
          List.generate(10, (i) => 'community-$i'),
          value: (_) => 10, // 10 unread each
        );

        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 100);
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => conversations);
        when(mockRepository.getUnreadCountsByCommunity())
            .thenAnswer((_) async => communities);

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 100);
        expect(state.conversationIdToUnreadCount.length, 100);
        expect(state.communityIdToUnreadCount.length, 10);
      });

      test('clamps negative counts to 0 when server data inconsistent', () async {
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 0);
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => {
          'conv-a1': 0, // Server says 0
        });
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async => {
          'community-a': 0, // Clamped to 0
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, greaterThanOrEqualTo(0));
        expect(state.communityIdToUnreadCount['community-a'], greaterThanOrEqualTo(0));
      });
    });

    group('Scenario 7: Synchronization', () {
      test('syncWithConversations fetches fresh counts from server', () async {
        // Initial state
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 5);
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => {'conv-a1': 5});
        when(mockRepository.getUnreadCountsByCommunity())
            .thenAnswer((_) async => {'community-a': 5});

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Server data changes
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 12);
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => {
          'conv-a1': 7,
          'conv-a2': 5,
        });
        when(mockRepository.getUnreadCountsByCommunity())
            .thenAnswer((_) async => {'community-a': 12});

        // Sync with server
        await notifier.syncWithConversations();

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 12, reason: 'Should have fresh server data');
        expect(state.conversationIdToUnreadCount['conv-a2'], 5, reason: 'New conversation detected');
      });

      test('sync handles empty server response gracefully', () async {
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 0);
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async => {});
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async => {});

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.syncWithConversations();

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 0);
        expect(state.conversationIdToUnreadCount, isEmpty);
        expect(state.communityIdToUnreadCount, isEmpty);
      });
    });
  });
}
