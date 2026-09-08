import 'package:fixnum/fixnum.dart';
import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/conversation_topic.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/services/chat_notification_manager.dart';
import 'package:ripls/services/chat_service.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:shared_preferences_platform_interface/in_memory_shared_preferences_async.dart';
import 'package:shared_preferences_platform_interface/shared_preferences_async_platform_interface.dart';

import 'chat_repository_test.mocks.dart';

@GenerateMocks([ChatService, FlutterLocalNotificationsPlugin])
void main() {
  group('ChatRepository', () {
    late ChatRepository repository;
    late MockChatService mockService;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockChatService();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = ChatRepository(cacheManager, mockService);
    });

    tearDown(() async {
      await cacheManager.dispose();
    });

    group('listConversations', () {
      test('fetches active conversations from service', () async {
        final mockConversations = [
          ConversationItem(
            conversationId: 'conv1',
            participants: [
              User(id: 'user1', name: 'User 1'),
              User(id: 'user2', name: 'User 2'),
            ],
          ),
          ConversationItem(
            conversationId: 'conv2',
            participants: [
              User(id: 'user1', name: 'User 1'),
              User(id: 'user3', name: 'User 3'),
            ],
          ),
        ];

        when(mockService.listConversations(archived: false))
            .thenAnswer((_) async => mockConversations);

        final result = await repository.listConversations();

        expect(result.length, 2);
        expect(result[0].conversationId, 'conv1');
        expect(result[1].conversationId, 'conv2');
        verify(mockService.listConversations(archived: false)).called(1);
      });

      test('fetches archived conversations from service', () async {
        final mockConversations = [
          ConversationItem(
            conversationId: 'archived1',
            isItemDone: true,
            unreadCount: 0,
          ),
        ];

        when(mockService.listConversations(archived: true))
            .thenAnswer((_) async => mockConversations);

        final result = await repository.listConversations(archived: true);

        expect(result.length, 1);
        expect(result[0].conversationId, 'archived1');
        verify(mockService.listConversations(archived: true)).called(1);
      });

      test('caches active and archived conversations separately', () async {
        final activeConversations = [
          ConversationItem(conversationId: 'active1'),
        ];
        final archivedConversations = [
          ConversationItem(conversationId: 'archived1'),
        ];

        when(mockService.listConversations(archived: false))
            .thenAnswer((_) async => activeConversations);
        when(mockService.listConversations(archived: true))
            .thenAnswer((_) async => archivedConversations);

        // First calls - should fetch
        final active1 = await repository.listConversations();
        final archived1 = await repository.listConversations(archived: true);

        // Second calls - should use cache
        final active2 = await repository.listConversations();
        final archived2 = await repository.listConversations(archived: true);

        // Service should only be called once for each
        verify(mockService.listConversations(archived: false)).called(1);
        verify(mockService.listConversations(archived: true)).called(1);

        // Verify correct data returned
        expect(active1[0].conversationId, 'active1');
        expect(active2[0].conversationId, 'active1');
        expect(archived1[0].conversationId, 'archived1');
        expect(archived2[0].conversationId, 'archived1');
      });

      test('refreshConversations invalidates both active and archived caches', () async {
        final mockConversations = [
          ConversationItem(conversationId: 'conv1'),
        ];

        when(mockService.listConversations(archived: false))
            .thenAnswer((_) async => mockConversations);
        when(mockService.listConversations(archived: true))
            .thenAnswer((_) async => mockConversations);

        // First calls - should fetch
        await repository.listConversations();
        await repository.listConversations(archived: true);

        // Refresh cache
        await repository.refreshConversations();

        // Second calls - should fetch again
        await repository.listConversations();
        await repository.listConversations(archived: true);

        // Service should be called twice for each
        verify(mockService.listConversations(archived: false)).called(2);
        verify(mockService.listConversations(archived: true)).called(2);
      });
    });

    group('getConversationHistory', () {
      test('fetches conversation history from service', () async {
        const conversationId = 'conv1';
        final msg1 = MessageHistoryItem(
          messageId: 'msg1',
          sentAtUnixSec: Int64(1000),
          isRead: false,
        )..userMessage = UserMessage(
            sender: User(id: 'user1', name: 'User 1'),
            text: 'Hello',
            mediaIds: [],
          );

        final msg2 = MessageHistoryItem(
          messageId: 'msg2',
          sentAtUnixSec: Int64(2000),
          isRead: false,
        )..userMessage = UserMessage(
            sender: User(id: 'user2', name: 'User 2'),
            text: 'Hi there',
            mediaIds: [],
          );

        final mockHistory = GetConversationHistoryResponse(
          messages: [msg1, msg2],
        );

        when(mockService.getConversationHistory(
          conversationId: conversationId,
          maxMessages: anyNamed('maxMessages'),
          beforeUnixSec: anyNamed('beforeUnixSec'),
        )).thenAnswer((_) async => mockHistory);

        final result = await repository.getConversationHistory(
          conversationId: conversationId,
        );

        expect(result.messages.length, 2);
        expect(result.messages[0].messageId, 'msg1');
        expect(result.messages[1].messageId, 'msg2');
        verify(mockService.getConversationHistory(
          conversationId: conversationId,
          maxMessages: anyNamed('maxMessages'),
          beforeUnixSec: anyNamed('beforeUnixSec'),
        )).called(1);
      });

      test('caches conversation history', () async {
        const conversationId = 'conv1';
        final mockHistory = GetConversationHistoryResponse(
          messages: [
            MessageHistoryItem(messageId: 'msg1'),
          ],
        );

        when(mockService.getConversationHistory(
          conversationId: conversationId,
          maxMessages: anyNamed('maxMessages'),
          beforeUnixSec: anyNamed('beforeUnixSec'),
        )).thenAnswer((_) async => mockHistory);

        // First call - should fetch
        await repository.getConversationHistory(
          conversationId: conversationId,
        );

        // Second call - should use cache
        await repository.getConversationHistory(
          conversationId: conversationId,
        );

        // Service should only be called once
        verify(mockService.getConversationHistory(
          conversationId: conversationId,
          maxMessages: anyNamed('maxMessages'),
          beforeUnixSec: anyNamed('beforeUnixSec'),
        )).called(1);
      });

      test('refreshConversationHistory invalidates history cache', () async {
        const conversationId = 'conv1';
        final mockHistory = GetConversationHistoryResponse(
          messages: [MessageHistoryItem(messageId: 'msg1')],
        );

        when(mockService.getConversationHistory(
          conversationId: conversationId,
          maxMessages: anyNamed('maxMessages'),
          beforeUnixSec: anyNamed('beforeUnixSec'),
        )).thenAnswer((_) async => mockHistory);

        // First call
        await repository.getConversationHistory(
          conversationId: conversationId,
        );

        // Refresh
        await repository.refreshConversationHistory(conversationId);

        // Second call - should fetch again
        await repository.getConversationHistory(
          conversationId: conversationId,
        );

        // Service should be called twice
        verify(mockService.getConversationHistory(
          conversationId: conversationId,
          maxMessages: anyNamed('maxMessages'),
          beforeUnixSec: anyNamed('beforeUnixSec'),
        )).called(2);
      });
    });

    group('sendMessage', () {
      test('sends message and invalidates caches', () async {
        const conversationId = 'conv1';
        const text = 'Hello world';
        final mockResponse = SendMessageResponse(
          messageId: 'msg123',
          sentAtUnixSec: Int64(1000),
        );

        when(mockService.sendMessage(
          conversationId: conversationId,
          text: text,
        )).thenAnswer((_) async => mockResponse);

        final result = await repository.sendMessage(
          conversationId: conversationId,
          text: text,
        );

        expect(result.messageId, 'msg123');
        verify(mockService.sendMessage(
          conversationId: conversationId,
          text: text,
        )).called(1);
      });
    });

    group('editMessage', () {
      test('edits message and invalidates conversation caches', () async {
        const conversationId = 'conv1';
        const messageId = 'msg123';
        when(mockService.editMessage(
          conversationId: conversationId,
          messageId: messageId,
          text: 'updated',
        )).thenAnswer((_) async => EditMessageResponse(
              editedAtUnixSec: Int64(2000),
            ));

        final result = await repository.editMessage(
          conversationId: conversationId,
          messageId: messageId,
          text: 'updated',
        );

        expect(result.editedAtUnixSec, Int64(2000));
        verify(mockService.editMessage(
          conversationId: conversationId,
          messageId: messageId,
          text: 'updated',
        )).called(1);
      });
    });

    group('deleteMessage', () {
      test('deletes message and invalidates conversation caches', () async {
        const conversationId = 'conv1';
        const messageId = 'msg123';
        when(mockService.deleteMessage(
          conversationId: conversationId,
          messageId: messageId,
        )).thenAnswer((_) async {});

        await repository.deleteMessage(
          conversationId: conversationId,
          messageId: messageId,
        );

        verify(mockService.deleteMessage(
          conversationId: conversationId,
          messageId: messageId,
        )).called(1);
      });
    });

    group('markMessagesRead', () {
      test('marks messages as read and returns count', () async {
        const conversationId = 'conv1';

        when(mockService.markMessagesRead(
          conversationId: conversationId,
          upToUnixSec: anyNamed('upToUnixSec'),
        )).thenAnswer((_) async => 5);

        final count = await repository.markMessagesRead(
          conversationId: conversationId,
        );

        expect(count, 5);
        verify(mockService.markMessagesRead(
          conversationId: conversationId,
          upToUnixSec: anyNamed('upToUnixSec'),
        )).called(1);
      });
    });

    group('markMessagesRead → ChatNotificationManager fan-out (#1892)', () {
      late MockFlutterLocalNotificationsPlugin mockPlugin;
      late ChatNotificationManager notificationManager;
      late ChatRepository repoWithNotifications;

      setUp(() async {
        SharedPreferencesAsyncPlatform.instance =
            InMemorySharedPreferencesAsync.empty();
        mockPlugin = MockFlutterLocalNotificationsPlugin();
        notificationManager = ChatNotificationManager(
          localNotifications: mockPlugin,
          prefs: SharedPreferencesAsync(),
        );
        repoWithNotifications = ChatRepository(
          cacheManager,
          mockService,
          chatNotificationManager: notificationManager,
        );
      });

      test('cancels the notification on successful mark-read', () async {
        const conversationId = 'conv-fanout-1';

        when(mockService.markMessagesRead(
          conversationId: conversationId,
          upToUnixSec: anyNamed('upToUnixSec'),
        )).thenAnswer((_) async => 3);

        await repoWithNotifications.markMessagesRead(
          conversationId: conversationId,
        );

        verify(mockPlugin.cancel(id: conversationId.hashCode)).called(1);
      });

      test(
          'does NOT cancel the notification when the mark-read RPC throws — '
          'the user should still see the lock-screen reminder', () async {
        const conversationId = 'conv-fanout-2';

        when(mockService.markMessagesRead(
          conversationId: conversationId,
          upToUnixSec: anyNamed('upToUnixSec'),
        )).thenThrow(Exception('network down'));

        await expectLater(
          repoWithNotifications.markMessagesRead(
            conversationId: conversationId,
          ),
          throwsA(isA<Exception>()),
        );

        verifyNever(mockPlugin.cancel(id: anyNamed('id')));
      });
    });

    group('getConversationForTransfer', () {
      test('fetches conversation for transfer', () async {
        const transferId = 'transfer1';
        final mockConversation = ConversationItem(
          conversationId: 'conv1',
          topic: ConversationTopic(transferId: transferId),
        );

        when(mockService.getConversationForTransfer(transferId: transferId))
            .thenAnswer((_) async => mockConversation);

        final result = await repository.getConversationForTransfer(
          transferId: transferId,
        );

        expect(result.conversationId, 'conv1');
        expect(result.topic.transferId, transferId);
        verify(mockService.getConversationForTransfer(transferId: transferId)).called(1);
      });

      test('caches conversation for transfer', () async {
        const transferId = 'transfer1';
        final mockConversation = ConversationItem(
          conversationId: 'conv1',
          topic: ConversationTopic(transferId: transferId),
        );

        when(mockService.getConversationForTransfer(transferId: transferId))
            .thenAnswer((_) async => mockConversation);

        // First call - should fetch
        await repository.getConversationForTransfer(transferId: transferId);

        // Second call - should use cache
        await repository.getConversationForTransfer(transferId: transferId);

        // Service should only be called once
        verify(mockService.getConversationForTransfer(transferId: transferId)).called(1);
      });
    });

    group('streamMessages', () {
      test('returns stream from service', () async {
        const conversationId = 'conv1';
        final streamMsg = StreamMessagesResponse(
          messageId: 'msg1',
          conversationId: conversationId,
          sentAtUnixSec: Int64(1000),
        )..userMessage = UserMessage(
            sender: User(id: 'user1', name: 'User 1'),
            text: 'Hello',
            mediaIds: [],
          );

        final mockStream = Stream.fromIterable([streamMsg]);

        when(mockService.streamMessages(conversationId: conversationId))
            .thenAnswer((_) => mockStream);

        final stream = repository.streamMessages(
          conversationId: conversationId,
        );

        final messages = await stream.toList();
        expect(messages.length, 1);
        expect(messages[0].messageId, 'msg1');
      });
    });

    group('updatePresence', () {
      test('updates presence via service', () async {
        when(mockService.updatePresence(isInForeground: true))
            .thenAnswer((_) async => {});

        await repository.updatePresence(isInForeground: true);

        verify(mockService.updatePresence(isInForeground: true)).called(1);
      });
    });

    group('cache invalidation', () {
      test('invalidateConversation clears conversation cache', () async {
        const conversationId = 'conv1';

        // This test verifies the method executes without error
        await repository.invalidateConversation(conversationId);
      });

      test('invalidateAll clears all chat cache', () async {
        // This test verifies the method executes without error
        await repository.invalidateAll();
      });
    });

    group('startGearConversation', () {
      test('returns conversation_id from service', () async {
        const gearId = 'gear-1';
        const communityId = 'comm-1';
        const expectedConvId = 'conv-gear-1';

        when(mockService.startConversation(
          communityId: communityId,
          topic: anyNamed('topic'),
        )).thenAnswer((_) async =>
            StartConversationResponse(conversationId: expectedConvId));

        final result = await repository.startGearConversation(
          gearId: gearId,
          communityId: communityId,
        );

        expect(result, expectedConvId);
        verify(mockService.startConversation(
          communityId: communityId,
          topic: anyNamed('topic'),
        )).called(1);
      });

      test('clears conversation cache on success', () async {
        const gearId = 'gear-1';
        const communityId = 'comm-1';

        when(mockService.startConversation(
          communityId: communityId,
          topic: anyNamed('topic'),
        )).thenAnswer((_) async =>
            StartConversationResponse(conversationId: 'conv-1'));

        // Prime the conversation cache with a dummy entry.
        when(mockService.getConversationForCommunity(communityId: communityId))
            .thenAnswer((_) async => ConversationItem(conversationId: 'old-conv'));
        await repository.getConversationForCommunity(communityId);
        verify(mockService.getConversationForCommunity(communityId: communityId)).called(1);

        await repository.startGearConversation(
          gearId: gearId,
          communityId: communityId,
        );

        // Cache was cleared, so next read must re-fetch.
        when(mockService.getConversationForCommunity(communityId: communityId))
            .thenAnswer((_) async => ConversationItem(conversationId: 'new-conv'));
        final conv = await repository.getConversationForCommunity(communityId);
        expect(conv.conversationId, 'new-conv');
        verify(mockService.getConversationForCommunity(communityId: communityId)).called(1);
      });

      test('propagates service error without clearing cache', () async {
        const gearId = 'gear-1';
        const communityId = 'comm-1';

        when(mockService.startConversation(
          communityId: communityId,
          topic: anyNamed('topic'),
        )).thenThrow(Exception('network error'));

        expect(
          () => repository.startGearConversation(
            gearId: gearId,
            communityId: communityId,
          ),
          throwsException,
        );
      });
    });
  });
}
