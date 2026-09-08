import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/unread_count_repository.dart';
import 'package:ripls/services/providers.dart';

import 'unread_count_view_model_test.mocks.dart';

@GenerateMocks([UnreadCountRepository])
void main() {
  group('UnreadCountNotifier', () {
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

    group('initialize', () {
      test('loads counts from repository', () async {
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 10);
        when(mockRepository.getUnreadCounts()).thenAnswer(
          (_) async => {'conv1': 5, 'conv2': 5},
        );
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer(
          (_) async => {'comm1': 7, 'comm2': 3},
        );

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 10);
        expect(state.conversationIdToUnreadCount, {'conv1': 5, 'conv2': 5});
        expect(state.communityIdToUnreadCount, {'comm1': 7, 'comm2': 3});
        expect(state.isLoading, false);

        verify(mockRepository.getTotalUnreadCount()).called(1);
        verify(mockRepository.getUnreadCounts()).called(1);
        verify(mockRepository.getUnreadCountsByCommunity()).called(1);
      });

      test('handles errors gracefully', () async {
        when(mockRepository.getTotalUnreadCount())
            .thenThrow(Exception('Network error'));

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        final state = container.read(unreadCountProvider);
        expect(state.isLoading, false);
        expect(state.totalUnreadCount, 0); // Should remain at default
      });
    });

    group('incrementUnreadCount', () {
      test('updates conversation and community counts', () async {
        // Set initial state (first call for initialize)
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 5 : 6; // 5 on init, 6 after increment
        });
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1 ? {'conv1': 5} : {'conv1': 6};
        });
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1 ? {'comm1': 5} : {'comm1': 6};
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        // Increment (this will call syncWithConversations which reads from repository again)
        await notifier.incrementUnreadCount('conv1', 'comm1');

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 6);
        expect(state.conversationIdToUnreadCount['conv1'], 6);
        expect(state.communityIdToUnreadCount['comm1'], 6);

        verify(mockRepository.incrementUnreadCount('comm1', 'conv1')).called(1);
      });

      test('creates new conversation count if not exists', () async {
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 0 : 1; // 0 on init, 1 after increment
        });
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1 ? {} : {'conv2': 1};
        });
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1 ? {} : {'comm2': 1};
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        await notifier.incrementUnreadCount('conv2', 'comm2');

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 1);
        expect(state.conversationIdToUnreadCount['conv2'], 1);
        expect(state.communityIdToUnreadCount['comm2'], 1);
      });
    });

    group('resetUnreadCount', () {
      test('resets conversation count and updates community count', () async {
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 10 : 5; // 10 on init, 5 after reset
        });
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1
              ? {'conv1': 5, 'conv2': 5}
              : {'conv1': 0, 'conv2': 5};
        });
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1 ? {'comm1': 7, 'comm2': 3} : {'comm1': 2, 'comm2': 3};
        });
        when(mockRepository.resetUnreadCount('comm1', 'conv1'))
            .thenAnswer((_) async {});

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        await notifier.resetUnreadCount('conv1', 'comm1');

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 5); // 10 - 5
        expect(state.conversationIdToUnreadCount['conv1'], 0);
        expect(state.communityIdToUnreadCount['comm1'], 2); // 7 - 5

        verify(mockRepository.resetUnreadCount('comm1', 'conv1')).called(1);
      });

      test('clamps negative community count to zero', () async {
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 5 : 0; // 5 on init, 0 after reset
        });
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1 ? {'conv1': 5} : {'conv1': 0};
        });
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1 ? {'comm1': 3} : {'comm1': 0}; // Repository clamps to 0
        });
        when(mockRepository.resetUnreadCount('comm1', 'conv1'))
            .thenAnswer((_) async {});

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        await notifier.resetUnreadCount('conv1', 'comm1');

        final state = container.read(unreadCountProvider);
        expect(state.communityIdToUnreadCount['comm1'], 0); // Should not be negative
      });
    });

    group('refresh', () {
      test('invalidates cache and refetches', () async {
        when(mockRepository.invalidate()).thenAnswer((_) async {});
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 15);
        when(mockRepository.getUnreadCounts()).thenAnswer(
          (_) async => {'conv1': 15},
        );
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer(
          (_) async => {'comm1': 15},
        );

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.refresh();

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 15);

        verify(mockRepository.invalidate()).called(1);
        verify(mockRepository.getTotalUnreadCount()).called(1);
      });
    });

    group('syncWithConversations', () {
      test('updates state with fresh counts', () async {
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async => 12);
        when(mockRepository.getUnreadCounts()).thenAnswer(
          (_) async => {'conv1': 7, 'conv2': 5},
        );
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer(
          (_) async => {'comm1': 8, 'comm2': 4},
        );

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.syncWithConversations();

        final state = container.read(unreadCountProvider);
        expect(state.totalUnreadCount, 12);
        expect(state.conversationIdToUnreadCount, {'conv1': 7, 'conv2': 5});
        expect(state.communityIdToUnreadCount, {'comm1': 8, 'comm2': 4});
      });
    });

    group('state immutability', () {
      test('state updates create new instances', () async {
        var callCount = 0;
        when(mockRepository.getTotalUnreadCount()).thenAnswer((_) async {
          callCount++;
          return callCount == 1 ? 5 : 6; // 5 on init, 6 after increment
        });
        when(mockRepository.getUnreadCounts()).thenAnswer((_) async {
          return callCount == 1 ? {'conv1': 5} : {'conv1': 6};
        });
        when(mockRepository.getUnreadCountsByCommunity()).thenAnswer((_) async {
          return callCount == 1 ? {'comm1': 5} : {'comm1': 6};
        });

        final notifier = container.read(unreadCountProvider.notifier);
        await notifier.initialize();

        final state1 = container.read(unreadCountProvider);

        await notifier.incrementUnreadCount('conv1', 'comm1');

        final state2 = container.read(unreadCountProvider);

        // States should be different instances
        expect(identical(state1, state2), false);
        // But original state should be unchanged
        expect(state1.totalUnreadCount, 5);
        expect(state2.totalUnreadCount, 6);
      });
    });
  });
}
