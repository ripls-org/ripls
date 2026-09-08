import 'dart:async';

import 'package:cross_file/cross_file.dart';
import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/conversation.pb.dart'
    show ConversationContext;
import 'package:ripls/data/gen/ripls/api/conversation_topic.pb.dart';
import 'package:ripls/data/gen/ripls/api/gear.pbenum.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearResponse;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/models/chat_message.dart';
import 'package:ripls/presentation/viewmodels/conversation_view_model.dart';
import 'package:ripls/services/chat_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Helper function to create a MessageHistoryItem with user message
MessageHistoryItem createUserHistoryItem({
  required String messageId,
  required User sender,
  required String text,
  required Int64 sentAtUnixSec,
  bool isRead = false,
}) {
  return MessageHistoryItem(
    messageId: messageId,
    sentAtUnixSec: sentAtUnixSec,
    isRead: isRead,
  )..userMessage = UserMessage(sender: sender, text: text, mediaIds: []);
}

/// Helper function to create a StreamMessagesResponse with user message
StreamMessagesResponse createUserStreamResponse({
  required String messageId,
  required String conversationId,
  required User sender,
  required String text,
  required Int64 sentAtUnixSec,
}) {
  return StreamMessagesResponse(
    messageId: messageId,
    conversationId: conversationId,
    sentAtUnixSec: sentAtUnixSec,
  )..userMessage = UserMessage(sender: sender, text: text, mediaIds: []);
}

class FakeChatService implements ChatService {
  GetConversationHistoryResponse? historyResponse;
  Exception? getHistoryError;
  Exception? sendMessageError;
  StreamController<StreamMessagesResponse>? streamController;
  // When set, successive calls to streamMessages return controllers from this
  // list in order. Supports reconnection testing.
  List<StreamController<StreamMessagesResponse>>? streamControllers;
  int _streamCallCount = 0;
  SendMessageRequest? lastSendRequest;
  int markedAsReadCount = 0;
  int getConversationHistoryCallCount = 0;
  Exception? editMessageError;
  Exception? deleteMessageError;
  String? lastEditedMessageId;
  String? lastEditedText;
  final List<String> deletedMessageIds = [];
  ConversationItem? conversationResponse;
  ConversationContext? conversationContextResponse;
  Exception? getConversationContextError;
  int getConversationCallCount = 0;

  @override
  Future<GetConversationHistoryResponse> getConversationHistory({
    required String conversationId,
    int? maxMessages,
    int? beforeUnixSec,
  }) async {
    getConversationHistoryCallCount++;
    if (getHistoryError != null) throw getHistoryError!;
    if (historyResponse != null) return historyResponse!;
    return GetConversationHistoryResponse(messages: []);
  }

  @override
  Future<SendMessageResponse> sendMessage({
    required String conversationId,
    required String text,
    List<String> mediaIds = const [],
    String? replyToMessageId,
  }) async {
    if (sendMessageError != null) throw sendMessageError!;
    lastSendRequest = SendMessageRequest(
      conversationId: conversationId,
      text: text,
      mediaIds: mediaIds,
      replyToMessageId: replyToMessageId,
    );
    return SendMessageResponse();
  }

  @override
  Future<EditMessageResponse> editMessage({
    required String conversationId,
    required String messageId,
    required String text,
  }) async {
    if (editMessageError != null) throw editMessageError!;
    lastEditedMessageId = messageId;
    lastEditedText = text;
    return EditMessageResponse(editedAtUnixSec: Int64(12345));
  }

  @override
  Future<void> deleteMessage({
    required String conversationId,
    required String messageId,
  }) async {
    if (deleteMessageError != null) throw deleteMessageError!;
    deletedMessageIds.add(messageId);
  }

  @override
  Stream<StreamMessagesResponse> streamMessages({
    required String conversationId,
  }) {
    if (streamControllers != null &&
        _streamCallCount < streamControllers!.length) {
      return streamControllers![_streamCallCount++].stream;
    }
    streamController ??= StreamController<StreamMessagesResponse>.broadcast();
    return streamController!.stream;
  }

  @override
  Future<int> markMessagesRead({
    required String conversationId,
    int? upToUnixSec,
  }) async {
    markedAsReadCount++;
    return 1;
  }

  @override
  Future<ConversationItem> getConversation({
    required String conversationId,
  }) async {
    getConversationCallCount++;
    return conversationResponse ??
        ConversationItem(conversationId: conversationId);
  }

  @override
  Future<ConversationContext> getConversationContext({
    required String conversationId,
  }) async {
    if (getConversationContextError != null) throw getConversationContextError!;
    return conversationContextResponse ??
        ConversationContext(conversationId: conversationId);
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class FakeTransferRepository implements TransferRepository {
  Exception? selectRecipientError;
  Exception? cancelTransferError;
  List<Transfer> extendedTransfers = [];
  List<Transfer> receivedTransfers = [];
  String? lastSelectedRecipientTransferId;
  String? lastCancelledTransferId;

  @override
  Future<Transfer?> getTransfer(String transferId) async {
    // Try finding in received transfers first
    for (final transfer in receivedTransfers) {
      if (transfer.id == transferId) return transfer;
    }
    // Try finding in extended (my) transfers
    for (final transfer in extendedTransfers) {
      if (transfer.id == transferId) return transfer;
    }
    return null;
  }

  @override
  Future<({String conversationId, String communityEventId})> selectRecipient({
    required String transferId,
    required String recipientId,
  }) async {
    if (selectRecipientError != null) throw selectRecipientError!;
    lastSelectedRecipientTransferId = transferId;
    return (conversationId: 'conversation-id', communityEventId: 'event-id');
  }

  @override
  Future<String> cancelTransfer({required String transferId}) async {
    if (cancelTransferError != null) throw cancelTransferError!;
    lastCancelledTransferId = transferId;
    return 'event-id';
  }

  @override
  Future<List<Transfer>> listMyTransfers({
    TransferType? transferType,
    TransferState? state,
  }) async {
    return extendedTransfers;
  }

  @override
  Future<List<Transfer>> listReceivedTransfers({
    TransferType? transferType,
    TransferState? state,
  }) async {
    return receivedTransfers;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class FakeUserRepository implements UserRepository {
  Map<String, GetUserResponse> users = {};
  Exception? getUserError;

  @override
  Future<UserProfile> getUserProfile(String userId) async {
    if (getUserError != null) throw getUserError!;
    final user = users.containsKey(userId)
        ? users[userId]!
        : GetUserResponse(userId: userId, name: 'User $userId');
    return UserProfile(user: user, mediaUrl: null);
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class FakeGearRepository implements GearRepository {
  Map<String, GetGearResponse> gearItems = {};
  Exception? getGearError;

  @override
  Future<GetGearResponse> getGearDetails(
    String id, {
    String? communityId,
  }) async {
    if (getGearError != null) throw getGearError!;
    if (gearItems.containsKey(id)) {
      return gearItems[id]!;
    }
    return GetGearResponse(id: id, name: 'Test Gear');
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class FakeMediaRepository implements MediaRepository {
  final Map<String, MediaUrl> _mediaUrls = {};
  Exception? getMediaUrlError;
  Exception? addMediaError;
  int addMediaCallCount = 0;

  @override
  Future<MediaUrl> getMediaUrl(String mediaId) async {
    if (getMediaUrlError != null) throw getMediaUrlError!;
    return _mediaUrls[mediaId] ??
        MediaUrl(
          mediaId: mediaId,
          url: 'https://example.com/media/$mediaId',
          isThumbnail: false,
        );
  }

  
  @override
  Future<String> addMedia({
    required XFile file,
    String? description,
  }) async {
    addMediaCallCount++;
    if (addMediaError != null) throw addMediaError!;
    return 'media-$addMediaCallCount';
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

void main() {
  // Initialize SharedPreferences mock for all tests
  TestWidgetsFlutterBinding.ensureInitialized();
  SharedPreferences.setMockInitialValues({
    'access_token': 'test-token',
    'user_id': 'user-123',
    'user_name': 'Test User',
  });

  group('ConversationViewModel', () {
    late FakeChatService fakeChatService;
    late FakeTransferRepository fakeTransferRepository;
    late FakeUserRepository fakeUserRepository;
    late FakeGearRepository fakeGearRepository;
    late FakeMediaRepository fakeMediaRepository;
    late ConversationItem conversation;
    const currentUserId = 'user-123';
    const otherUserId = 'user-456';
    const conversationId = 'conv-789';

    // Helper to create a container for each test
    ProviderContainer createContainer() {
      return ProviderContainer(
        overrides: [
          chatServiceProvider.overrideWithValue(fakeChatService),
          transferRepositoryProvider.overrideWithValue(fakeTransferRepository),
          userRepositoryProvider.overrideWithValue(fakeUserRepository),
          gearRepositoryProvider.overrideWithValue(fakeGearRepository),
          mediaRepositoryProvider.overrideWithValue(fakeMediaRepository),
        ],
      );
    }

    setUp(() {
      fakeChatService = FakeChatService();
      fakeTransferRepository = FakeTransferRepository();
      fakeUserRepository = FakeUserRepository();
      fakeGearRepository = FakeGearRepository();
      fakeMediaRepository = FakeMediaRepository();

      conversation = ConversationItem(
        conversationId: conversationId,
        participants: [
          User(id: currentUserId, name: 'Test User'),
          User(id: otherUserId, name: 'Other User'),
        ],
      );
    });

    tearDown(() {
      fakeChatService.streamController?.close();
    });

    group('initialization', () {
      test('starts with loading state', () async {
        final container = createContainer();
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        final state = container.read(conversationProvider(conversationId));

        expect(state.isLoadingMessages, false);
        expect(state.messages, isEmpty);
        expect(state.isSending, false);
        expect(state.error, null);
      });
    });

    group('loadMessages', () {
      test('loads messages successfully', () async {
        final container = createContainer();
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [
            createUserHistoryItem(
              messageId: 'msg-1',
              sender: User(id: currentUserId, name: 'Me'),
              text: 'Hello',
              sentAtUnixSec: Int64(1000),
            ),
            createUserHistoryItem(
              messageId: 'msg-2',
              sender: User(id: otherUserId, name: 'Other'),
              text: 'Hi there',
              sentAtUnixSec: Int64(2000),
            ),
          ],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        final state = container.read(conversationProvider(conversationId));

        expect(state.isLoadingMessages, false);
        expect(state.messages.length, 2);
        // Messages are sorted chronologically: oldest first by sentAtUnixSec.
        expect(state.messages[0].messageId, 'msg-1');
        expect(state.messages[1].messageId, 'msg-2');
        expect(state.error, null);
      });

      test('handles error when loading messages', () async {
        final container = createContainer();
        fakeChatService.getHistoryError = Exception('Network error');

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        final state = container.read(conversationProvider(conversationId));

        expect(state.isLoadingMessages, false);
        expect(state.messages, isEmpty);
        expect(state.error, isNotNull);
      });

      test('handles missing conversation', () async {
        final container = createContainer();

        // loadMessages() before initialize(): state.conversation is still
        // null, which is the only way this guard is reachable.
        await container
            .read(conversationProvider(conversationId).notifier)
            .loadMessages();

        final state = container.read(conversationProvider(conversationId));

        expect(state.isLoadingMessages, false);
        expect(state.error, isNotNull);
      });
    });

    group('message streaming', () {
      test('handles streamed messages', () async {
        final container = createContainer();
        final streamController = StreamController<StreamMessagesResponse>();
        addTearDown(streamController.close);
        fakeChatService.streamController = streamController;
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        // Keep provider alive during async operations
        final subscription = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Add a new message to the stream
        streamController.add(
          createUserStreamResponse(
            messageId: 'msg-new',
            conversationId: conversationId,
            sender: User(id: otherUserId, name: 'Other'),
            text: 'New message',
            sentAtUnixSec: Int64(3000),
          ),
        );

        await Future.delayed(const Duration(milliseconds: 100));

        final state = container.read(conversationProvider(conversationId));
        expect(state.messages.length, 1);
        expect(state.messages[0].messageId, 'msg-new');
        expect(state.messages[0].text, 'New message');
        subscription.close();
      });

      test('ignores duplicate messages', () async {
        final container = createContainer();
        final streamController = StreamController<StreamMessagesResponse>();
        addTearDown(streamController.close);
        fakeChatService.streamController = streamController;
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [
            createUserHistoryItem(
              messageId: 'msg-1',
              sender: User(id: currentUserId, name: 'Me'),
              text: 'Hello',
              sentAtUnixSec: Int64(1000),
            ),
          ],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        // Keep provider alive during async operations
        final subscription = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Try to add the same message via stream
        streamController.add(
          createUserStreamResponse(
            messageId: 'msg-1',
            conversationId: conversationId,
            sender: User(id: currentUserId, name: 'Me'),
            text: 'Hello',
            sentAtUnixSec: Int64(1000),
          ),
        );

        await Future.delayed(const Duration(milliseconds: 100));

        final state = container.read(conversationProvider(conversationId));
        // Should still have only 1 message
        expect(state.messages.length, 1);
        subscription.close();
      });
    });

    group('sendMessage', () {
      test('sends message successfully', () async {
        final container = createContainer();
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        await container
            .read(conversationProvider(conversationId).notifier)
            .sendMessage('Test message');

        final state = container.read(conversationProvider(conversationId));
        expect(state.isSending, false);
        expect(fakeChatService.lastSendRequest?.text, 'Test message');
        expect(fakeChatService.lastSendRequest?.conversationId, conversationId);
      });

      test('handles send message error', () async {
        final container = createContainer();
        fakeChatService.sendMessageError = Exception('Send failed');
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        expect(
          () => container
              .read(conversationProvider(conversationId).notifier)
              .sendMessage('Test message'),
          throwsException,
        );
      });

      test('does not send empty messages', () async {
        final container = createContainer();
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        await container
            .read(conversationProvider(conversationId).notifier)
            .sendMessage('   ');

        expect(fakeChatService.lastSendRequest, isNull);
      });
    });

    group('refreshParticipantsFromSystemMessage', () {
      /// Drives the notifier to the point where a JOINED/LEFT system message
      /// would arrive, then triggers the refresh and drains the microtask
      /// queue — the method is fire-and-forget from the message stream, so
      /// there is no future to await.
      Future<ConversationState> refreshWith(
        ProviderContainer container, {
        required ConversationItem refetched,
        ConversationContext? context,
        Exception? contextError,
      }) async {
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
              conversationContext: ConversationContext(
                conversationId: conversationId,
                topicTitle: 'Original title',
              ),
            );

        fakeChatService.conversationResponse = refetched;
        fakeChatService.conversationContextResponse = context;
        fakeChatService.getConversationContextError = contextError;

        // Keep the auto-dispose provider alive across the awaits below.
        // Without this the notifier is rebuilt from scratch and every
        // assertion here passes vacuously against a blank state.
        final subscription = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        container
            .read(conversationProvider(conversationId).notifier)
            .refreshParticipantsFromSystemMessage();
        await Future.delayed(const Duration(milliseconds: 100));

        final state = container.read(conversationProvider(conversationId));
        subscription.close();
        return state;
      }

      test('adopts the refetched participant list', () async {
        final container = createContainer();
        const joinerId = 'user-999';

        final state = await refreshWith(
          container,
          refetched: ConversationItem(
            conversationId: conversationId,
            participants: [
              User(id: currentUserId, name: 'Test User'),
              User(id: otherUserId, name: 'Other User'),
              User(id: joinerId, name: 'Joiner'),
            ],
          ),
        );

        expect(state.conversation!.participants.length, 3);
        // participants excludes the current user.
        expect(state.participants.map((p) => p.id), [otherUserId, joinerId]);
      });

      test('drops a participant that left', () async {
        final container = createContainer();

        final state = await refreshWith(
          container,
          refetched: ConversationItem(
            conversationId: conversationId,
            participants: [User(id: currentUserId, name: 'Test User')],
          ),
        );

        expect(state.participants, isEmpty);
      });

      test('adopts the refetched unread count', () async {
        final container = createContainer();

        final state = await refreshWith(
          container,
          refetched: ConversationItem(
            conversationId: conversationId,
            participants: [User(id: currentUserId, name: 'Test User')],
            unreadCount: 7,
          ),
        );

        expect(state.unreadCount, 7);
      });

      test('adopts the refetched context and transfer status map', () async {
        final container = createContainer();

        final state = await refreshWith(
          container,
          refetched: ConversationItem(
            conversationId: conversationId,
            participants: [User(id: currentUserId, name: 'Test User')],
          ),
          context: ConversationContext(
            conversationId: conversationId,
            topicTitle: 'Refreshed title',
            gearTransferContext: GearTransferContext(
              pendingRequests: [
                TransferRequest(
                  transferId: 'transfer-1',
                  borrower: User(id: otherUserId, name: 'Other User'),
                ),
              ],
            ),
          ),
        );

        expect(state.conversationContext!.topicTitle, 'Refreshed title');
        expect(
          state.transferStatusMap[otherUserId],
          TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
        );
      });

      test(
        'keeps the previous context and still refreshes membership when the '
        'context fetch fails',
        () async {
          final container = createContainer();
          const joinerId = 'user-999';

          final state = await refreshWith(
            container,
            refetched: ConversationItem(
              conversationId: conversationId,
              participants: [
                User(id: currentUserId, name: 'Test User'),
                User(id: joinerId, name: 'Joiner'),
              ],
            ),
            contextError: Exception('context unavailable'),
          );

          expect(state.conversationContext!.topicTitle, 'Original title');
          expect(state.participants.map((p) => p.id), [joinerId]);
        },
      );
    });

    group('transfer operations', () {
      test('selects recipient successfully', () async {
        final container = createContainer();
        const transferId = 'transfer-123';
        final transfer = Transfer(
          id: transferId,
          gearName: 'Bike',
          recipient: User(id: otherUserId, name: 'Jane'),
          owner: User(id: currentUserId, name: 'John'),
        );

        // Populate the fake repository so getTransfer() can find it
        fakeTransferRepository.extendedTransfers = [transfer];

        // Create conversation with transferId
        final conversationWithTransfer = ConversationItem(
          conversationId: conversationId,
          participants: conversation.participants,
          topic: ConversationTopic(transferId: transferId),
        );

        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversationWithTransfer,
            );

        await container
            .read(conversationProvider(conversationId).notifier)
            .selectRecipient(otherUserId);

        expect(
          fakeTransferRepository.lastSelectedRecipientTransferId,
          transferId,
        );
      });

      test('cancels giveaway transfer successfully', () async {
        final container = createContainer();
        const transferId = 'transfer-123';

        // Build a giveaway conversation with GearTransferContext so
        // cancelTransfer() can resolve the transfer ID.
        final gearTransferCtx = GearTransferContext(
          isOwner: true,
          selectedRecipient: TransferRequest(
            transferId: transferId,
            borrower: User(id: otherUserId, name: 'Jane'),
          ),
        );
        final conversationContext = ConversationContext(
          topicTitle: 'Free Bike',
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          gearTransferContext: gearTransferCtx,
          topic: ConversationTopic(gearId: 'gear-1'),
        );

        final giveawayConversation = ConversationItem(
          conversationId: conversationId,
          participants: conversation.participants,
          topic: ConversationTopic(gearId: 'gear-1'),
        );

        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: giveawayConversation,
              conversationContext: conversationContext,
            );

        await container
            .read(conversationProvider(conversationId).notifier)
            .cancelTransfer();

        expect(fakeTransferRepository.lastCancelledTransferId, transferId);
      });

      test(
        'throws error when selecting recipient without transferId',
        () async {
          final container = createContainer();
          fakeChatService.historyResponse = GetConversationHistoryResponse(
            messages: [],
          );

          await container
              .read(conversationProvider(conversationId).notifier)
              .initialize(
                currentUserId: currentUserId,
                currentUserName: 'Test User',
                conversation: conversation,
              );

          expect(
            () => container
                .read(conversationProvider(conversationId).notifier)
                .selectRecipient(otherUserId),
            throwsException,
          );
        },
      );
    });

    group('participant information', () {
      test('returns cached participant name', () async {
        final container = createContainer();
        fakeUserRepository.users[otherUserId] = GetUserResponse(
          userId: otherUserId,
          name: 'Jane Doe',
        );

        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        final notifier = container.read(
          conversationProvider(conversationId).notifier,
        );

        expect(notifier.getOtherParticipantName(), 'Jane Doe');
        expect(notifier.getOtherParticipantId(), otherUserId);
      });

      test('returns participant ID when name fetch fails', () async {
        final container = createContainer();
        fakeUserRepository.getUserError = Exception('User not found');
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        final notifier = container.read(
          conversationProvider(conversationId).notifier,
        );

        expect(notifier.getOtherParticipantId(), otherUserId);
      });
    });

    group('optimistic UI updates', () {
      test('adds message optimistically with pending state', () async {
        final container = createContainer();
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        var state = container.read(conversationProvider(conversationId));
        expect(state.messages.length, 0);

        final sendFuture = container
            .read(conversationProvider(conversationId).notifier)
            .sendMessage('Optimistic message');

        state = container.read(conversationProvider(conversationId));
        expect(state.messages.length, 1);
        expect(state.messages[0].text, 'Optimistic message');
        expect(state.messages[0].state, MessageState.pending);
        expect(state.messages[0].isTemporary, true);
        expect(state.messages[0].messageId, startsWith('temp-'));

        await sendFuture;
      });

      test(
        'updates pending message to delivered when stream echo arrives',
        () async {
          final container = createContainer();
          final streamController = StreamController<StreamMessagesResponse>();
          addTearDown(streamController.close);
          fakeChatService.streamController = streamController;
          fakeChatService.historyResponse = GetConversationHistoryResponse(
            messages: [],
          );

          await container
              .read(conversationProvider(conversationId).notifier)
              .initialize(
                currentUserId: currentUserId,
                currentUserName: 'Test User',
                conversation: conversation,
              );

          // Keep provider alive during async operations
          final subscription = container.listen(
            conversationProvider(conversationId),
            (_, _) {},
          );

          final sendFuture = container
              .read(conversationProvider(conversationId).notifier)
              .sendMessage('Test message');

          var state = container.read(conversationProvider(conversationId));
          expect(state.messages.length, 1);
          expect(state.messages[0].state, MessageState.pending);
          final tempId = state.messages[0].messageId;

          await sendFuture;

          streamController.add(
            createUserStreamResponse(
              messageId: 'real-msg-id',
              conversationId: conversationId,
              sender: User(id: currentUserId, name: 'Me'),
              text: 'Test message',
              sentAtUnixSec: Int64(
                DateTime.now().millisecondsSinceEpoch ~/ 1000,
              ),
            ),
          );

          await Future.delayed(const Duration(milliseconds: 100));

          state = container.read(conversationProvider(conversationId));
          expect(state.messages.length, 1);
          expect(state.messages[0].state, MessageState.delivered);
          expect(state.messages[0].messageId, 'real-msg-id');
          expect(state.messages[0].isTemporary, false);
          expect(state.messages[0].messageId, isNot(tempId));
          subscription.close();
        },
      );

      test('marks message as failed when send throws exception', () async {
        final container = createContainer();
        fakeChatService.sendMessageError = Exception('Network error');
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        // Keep provider alive during async operations
        final subscription = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        expect(
          () => container
              .read(conversationProvider(conversationId).notifier)
              .sendMessage('Failed message'),
          throwsException,
        );

        await Future.delayed(const Duration(milliseconds: 100));

        final state = container.read(conversationProvider(conversationId));
        expect(state.messages.length, 1);
        expect(state.messages[0].text, 'Failed message');
        expect(state.messages[0].state, MessageState.failed);
        subscription.close();
      });

      test('marks message as failed on timeout', () async {
        final container = createContainer();
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        // Keep provider alive during async operations
        final subscription = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .sendMessage('Timeout message');

        var state = container.read(conversationProvider(conversationId));
        expect(state.messages.length, 1);
        expect(state.messages[0].state, MessageState.pending);

        await Future.delayed(const Duration(seconds: 11));

        state = container.read(conversationProvider(conversationId));
        expect(state.messages[0].state, MessageState.failed);
        subscription.close();
      });

      test('retryMessage removes failed message and resends', () async {
        final container = createContainer();
        fakeChatService.sendMessageError = Exception('Network error');
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        // Keep provider alive during async operations
        final subscription = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        try {
          await container
              .read(conversationProvider(conversationId).notifier)
              .sendMessage('Failed message');
        } catch (e) {
          // Expected
        }

        await Future.delayed(const Duration(milliseconds: 100));

        var state = container.read(conversationProvider(conversationId));
        expect(state.messages.length, 1);
        expect(state.messages[0].state, MessageState.failed);
        final failedMessageId = state.messages[0].messageId;

        fakeChatService.sendMessageError = null;

        await container
            .read(conversationProvider(conversationId).notifier)
            .retryMessage(failedMessageId);

        state = container.read(conversationProvider(conversationId));
        expect(state.messages.length, 1);
        expect(state.messages[0].text, 'Failed message');
        expect(state.messages[0].state, MessageState.pending);
        expect(state.messages[0].messageId, isNot(failedMessageId));
        subscription.close();
      });

      test('does not duplicate messages from stream', () async {
        final container = createContainer();
        final streamController = StreamController<StreamMessagesResponse>();
        addTearDown(streamController.close);
        fakeChatService.streamController = streamController;
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );

        // Keep provider alive during async operations
        final subscription = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        await container
            .read(conversationProvider(conversationId).notifier)
            .sendMessage('Test message');

        final now = DateTime.now().millisecondsSinceEpoch ~/ 1000;
        streamController.add(
          createUserStreamResponse(
            messageId: 'real-msg-id',
            conversationId: conversationId,
            sender: User(id: currentUserId, name: 'Me'),
            text: 'Test message',
            sentAtUnixSec: Int64(now),
          ),
        );

        await Future.delayed(const Duration(milliseconds: 100));

        var state = container.read(conversationProvider(conversationId));
        expect(state.messages.length, 1);
        expect(state.messages[0].messageId, 'real-msg-id');

        streamController.add(
          createUserStreamResponse(
            messageId: 'real-msg-id',
            conversationId: conversationId,
            sender: User(id: currentUserId, name: 'Me'),
            text: 'Test message',
            sentAtUnixSec: Int64(now),
          ),
        );

        await Future.delayed(const Duration(milliseconds: 100));

        state = container.read(conversationProvider(conversationId));
        expect(state.messages.length, 1);
        subscription.close();
      });

      test(
        'handles messages from other users alongside optimistic messages',
        () async {
          final container = createContainer();
          final streamController = StreamController<StreamMessagesResponse>();
          addTearDown(streamController.close);
          fakeChatService.streamController = streamController;
          fakeChatService.historyResponse = GetConversationHistoryResponse(
            messages: [],
          );

          await container
              .read(conversationProvider(conversationId).notifier)
              .initialize(
                currentUserId: currentUserId,
                currentUserName: 'Test User',
                conversation: conversation,
              );

          // Keep provider alive during async operations
          final subscription = container.listen(
            conversationProvider(conversationId),
            (_, _) {},
          );

          await container
              .read(conversationProvider(conversationId).notifier)
              .sendMessage('My message');

          var state = container.read(conversationProvider(conversationId));
          expect(state.messages.length, 1);
          expect(state.messages[0].senderId, currentUserId);
          expect(state.messages[0].state, MessageState.pending);

          // Pin the incoming message to the SAME unix-second as the optimistic
          // one. Using DateTime.now() here raced the optimistic message's own
          // now()-second: when the two straddled a second boundary they landed in
          // different seconds and _sortByChronology ordered by timestamp instead
          // of the messageId tiebreaker, flipping the expected order (a flake).
          final optimisticSec = state.messages[0].sentAtUnixSec;

          streamController.add(
            createUserStreamResponse(
              messageId: 'other-msg-id',
              conversationId: conversationId,
              sender: User(id: otherUserId, name: 'Other'),
              text: 'Other message',
              sentAtUnixSec: Int64(optimisticSec),
            ),
          );

          await Future.delayed(const Duration(milliseconds: 100));

          state = container.read(conversationProvider(conversationId));
          expect(state.messages.length, 2);
          // _sortByChronology sorts by (sentAtUnixSec, messageId). Both messages
          // now share the same unix-second (pinned above), so the messageId
          // tiebreak applies deterministically: 'other-msg-id' < 'temp-*',
          // putting the other user's message first.
          expect(state.messages[0].senderId, otherUserId);
          expect(state.messages[0].state, MessageState.delivered);
          expect(state.messages[1].senderId, currentUserId);
          expect(state.messages[1].state, MessageState.pending);
          subscription.close();
        },
      );
    });

    // ═══════════════════════════════════════════════════════════════════════
    // Loan getter tests
    // ═══════════════════════════════════════════════════════════════════════

    group('loan getters', () {
      /// Helper to initialize the notifier with a loan conversation context.
      Future<ConversationNotifier> initLoanNotifier(
        ProviderContainer container, {
        required GearTransferContext gearTransferContext,
      }) async {
        final ctx = ConversationContext(
          topicTitle: 'Test Drill',
          availability: Availability.AVAILABILITY_FOR_LOAN,
          gearTransferContext: gearTransferContext,
          topic: ConversationTopic(gearId: 'gear-1'),
        );

        final loanConversation = ConversationItem(
          conversationId: conversationId,
          participants: conversation.participants,
          topic: ConversationTopic(gearId: 'gear-1'),
        );

        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final notifier = container.read(
          conversationProvider(conversationId).notifier,
        );
        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: loanConversation,
          conversationContext: ctx,
        );
        return notifier;
      }

      test('isLoanConversation true for FOR_LOAN gear conversation', () async {
        final container = createContainer();
        final gearCtx = GearTransferContext(isOwner: false);
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.isLoanConversation, isTrue);
      });

      test('isLoanConversation false for non-gear conversation', () async {
        final container = createContainer();
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );
        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );
        expect(
          container
              .read(conversationProvider(conversationId).notifier)
              .isLoanConversation,
          isFalse,
        );
      });

      test('isLoanOwner true when isOwner flag set', () async {
        final container = createContainer();
        final gearCtx = GearTransferContext(isOwner: true);
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.isLoanOwner, isTrue);
      });

      test('isLoanOwner false for borrower', () async {
        final container = createContainer();
        final gearCtx = GearTransferContext(isOwner: false);
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.isLoanOwner, isFalse);
      });

      test('loanPhaseIndex returns 0 (Open) when no transfers exist', () async {
        final container = createContainer();
        final gearCtx = GearTransferContext(isOwner: false);
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.loanPhaseIndex, 0);
      });

      test(
        'loanPhaseIndex returns 1 (Scheduled) for RECIPIENT_SELECTED',
        () async {
          final container = createContainer();
          final transfer = Transfer(
            id: 't-1',
            state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
            recipient: User(id: currentUserId, name: 'Test User'),
          );
          final gearCtx = GearTransferContext(
            isOwner: false,
            userTransfer: transfer,
          );
          final notifier = await initLoanNotifier(
            container,
            gearTransferContext: gearCtx,
          );
          expect(notifier.loanPhaseIndex, 1);
        },
      );

      test('loanPhaseIndex returns 2 (On Loan) for ACTIVE', () async {
        final container = createContainer();
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_ACTIVE,
          recipient: User(id: currentUserId, name: 'Test User'),
        );
        final gearCtx = GearTransferContext(
          isOwner: false,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.loanPhaseIndex, 2);
      });

      test('loanPhaseIndex returns 3 (Returned) for COMPLETED', () async {
        final container = createContainer();
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_COMPLETED,
          recipient: User(id: currentUserId, name: 'Test User'),
        );
        final gearCtx = GearTransferContext(
          isOwner: false,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.loanPhaseIndex, 3);
      });

      test('loanPhaseIndex returns null (Cancelled) for CANCELLED', () async {
        final container = createContainer();
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_CANCELLED,
          recipient: User(id: currentUserId, name: 'Test User'),
        );
        final gearCtx = GearTransferContext(
          isOwner: false,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.loanPhaseIndex, isNull);
      });

      test('owner loanPhaseIndex reads from userTransfer (ACTIVE)', () async {
        final container = createContainer();
        // Server now populates userTransfer for owners too.
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_ACTIVE,
          recipient: User(id: otherUserId, name: 'Other User'),
        );
        final gearCtx = GearTransferContext(
          isOwner: true,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.loanPhaseIndex, 2);
        expect(notifier.isLoanActive, isTrue);
      });

      test(
        'owner with pendingRequests but no userTransfer gets phase 1',
        () async {
          final container = createContainer();
          final gearCtx = GearTransferContext(
            isOwner: true,
            pendingRequests: [
              TransferRequest(
                transferId: 't-1',
                borrower: User(id: otherUserId, name: 'Other'),
              ),
            ],
          );
          final notifier = await initLoanNotifier(
            container,
            gearTransferContext: gearCtx,
          );
          expect(notifier.loanPhaseIndex, 1);
        },
      );

      test('isLoanTerminal true for completed', () async {
        final container = createContainer();
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_COMPLETED,
          recipient: User(id: currentUserId, name: 'Test User'),
        );
        final gearCtx = GearTransferContext(
          isOwner: false,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.isLoanTerminal, isTrue);
        expect(notifier.isLoanCompleted, isTrue);
        expect(notifier.isLoanCancelled, isFalse);
      });

      test('isLoanTerminal true for cancelled', () async {
        final container = createContainer();
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_CANCELLED,
          recipient: User(id: currentUserId, name: 'Test User'),
        );
        final gearCtx = GearTransferContext(
          isOwner: false,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.isLoanTerminal, isTrue);
        expect(notifier.isLoanCancelled, isTrue);
        expect(notifier.isLoanCompleted, isFalse);
      });

      test('loanNeedsPickupDetails true when no pickup time set', () async {
        final container = createContainer();
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
          recipient: User(id: currentUserId, name: 'Test User'),
        );
        final gearCtx = GearTransferContext(
          isOwner: false,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.loanNeedsPickupDetails, isTrue);
      });

      test('loanNeedsPickupDetails false when pickup time is set', () async {
        final container = createContainer();
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
          recipient: User(id: currentUserId, name: 'Test User'),
          estimatedPickupUnixSec: Int64(1710000000),
        );
        final gearCtx = GearTransferContext(
          isOwner: false,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.loanNeedsPickupDetails, isFalse);
      });

      test('loanNeedsPickupDetails false for owner', () async {
        final container = createContainer();
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
          recipient: User(id: otherUserId, name: 'Other'),
        );
        final gearCtx = GearTransferContext(
          isOwner: true,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.loanNeedsPickupDetails, isFalse);
      });

      test('loanNeedsPickupDetails false for ACTIVE state', () async {
        final container = createContainer();
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_ACTIVE,
          recipient: User(id: currentUserId, name: 'Test User'),
        );
        final gearCtx = GearTransferContext(
          isOwner: false,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.loanNeedsPickupDetails, isFalse);
      });

      test('hasExpressedInterest true when userTransfer exists', () async {
        final container = createContainer();
        final transfer = Transfer(
          id: 't-1',
          state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
          recipient: User(id: currentUserId, name: 'Test User'),
        );
        final gearCtx = GearTransferContext(
          isOwner: false,
          userTransfer: transfer,
        );
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.hasExpressedInterest, isTrue);
      });

      test('hasExpressedInterest false when no userTransfer', () async {
        final container = createContainer();
        final gearCtx = GearTransferContext(isOwner: false);
        final notifier = await initLoanNotifier(
          container,
          gearTransferContext: gearCtx,
        );
        expect(notifier.hasExpressedInterest, isFalse);
      });
    });

    group('pending attachments', () {
      Future<ConversationNotifier> initNotifier(
        ProviderContainer container,
      ) async {
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );
        final notifier =
            container.read(conversationProvider(conversationId).notifier);
        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );
        return notifier;
      }

      test('addPendingAttachments stages files', () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        notifier.addPendingAttachments(['/path/a.jpg', '/path/b.jpg']);

        final state = container.read(conversationProvider(conversationId));
        expect(state.pendingAttachments, ['/path/a.jpg', '/path/b.jpg']);
      });

      test('addPendingAttachments caps at max', () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        // Add max items
        final paths = List.generate(
          ConversationNotifier.maxPendingAttachments,
          (i) => '/path/$i.jpg',
        );
        notifier.addPendingAttachments(paths);

        // Try adding more
        notifier.addPendingAttachments(['/path/extra.jpg']);

        final state = container.read(conversationProvider(conversationId));
        expect(
          state.pendingAttachments.length,
          ConversationNotifier.maxPendingAttachments,
        );
        expect(state.pendingAttachments.contains('/path/extra.jpg'), isFalse);
      });

      test('addPendingAttachments appends to existing', () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        notifier.addPendingAttachments(['/path/a.jpg']);
        notifier.addPendingAttachments(['/path/b.jpg']);

        final state = container.read(conversationProvider(conversationId));
        expect(state.pendingAttachments, ['/path/a.jpg', '/path/b.jpg']);
      });

      test('addPendingAttachments ignores empty list', () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        notifier.addPendingAttachments([]);

        final state = container.read(conversationProvider(conversationId));
        expect(state.pendingAttachments, isEmpty);
      });

      test('removePendingAttachment removes by index', () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        notifier.addPendingAttachments(['/a.jpg', '/b.jpg', '/c.jpg']);
        notifier.removePendingAttachment(1);

        final state = container.read(conversationProvider(conversationId));
        expect(state.pendingAttachments, ['/a.jpg', '/c.jpg']);
      });

      test('removePendingAttachment ignores invalid index', () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        notifier.addPendingAttachments(['/a.jpg']);
        notifier.removePendingAttachment(5);
        notifier.removePendingAttachment(-1);

        final state = container.read(conversationProvider(conversationId));
        expect(state.pendingAttachments, ['/a.jpg']);
      });

      test('clearPendingAttachments removes all', () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        notifier.addPendingAttachments(['/a.jpg', '/b.jpg']);
        notifier.clearPendingAttachments();

        final state = container.read(conversationProvider(conversationId));
        expect(state.pendingAttachments, isEmpty);
      });

      test('sendMessageWithPendingAttachments uploads and sends', () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        // Keep provider alive during async operations.
        final subscription = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        notifier.addPendingAttachments(['/a.jpg', '/b.jpg']);
        await notifier.sendMessageWithPendingAttachments(text: 'hello');

        // All files uploaded.
        expect(fakeMediaRepository.addMediaCallCount, 2);

        // Message sent with all media IDs.
        expect(fakeChatService.lastSendRequest, isNotNull);
        expect(fakeChatService.lastSendRequest!.text, 'hello');
        expect(fakeChatService.lastSendRequest!.mediaIds, ['media-1', 'media-2']);

        // Pending attachments cleared.
        final state = container.read(conversationProvider(conversationId));
        expect(state.pendingAttachments, isEmpty);
        expect(state.isSending, false);

        subscription.close();
      });

      test('sendMessageWithPendingAttachments without text', () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        final subscription = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        notifier.addPendingAttachments(['/a.jpg']);
        await notifier.sendMessageWithPendingAttachments();

        expect(fakeChatService.lastSendRequest, isNotNull);
        expect(fakeChatService.lastSendRequest!.text, '');
        expect(fakeChatService.lastSendRequest!.mediaIds, ['media-1']);

        subscription.close();
      });

      test('sendMessageWithPendingAttachments does nothing when empty',
          () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        await notifier.sendMessageWithPendingAttachments(text: 'hello');

        // No upload or send should happen.
        expect(fakeMediaRepository.addMediaCallCount, 0);
        expect(fakeChatService.lastSendRequest, isNull);
      });

      test('sendMessageWithPendingAttachments rethrows on upload error',
          () async {
        final container = createContainer();
        final notifier = await initNotifier(container);

        final subscription = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        fakeMediaRepository.addMediaError = Exception('upload failed');
        notifier.addPendingAttachments(['/a.jpg']);

        await expectLater(
          notifier.sendMessageWithPendingAttachments(),
          throwsA(isA<Exception>()),
        );

        final state = container.read(conversationProvider(conversationId));
        expect(state.isSending, false);

        subscription.close();
      });
    });

    group('stream reconnection', () {
      test('reconnects after stream onDone', () async {
        final firstController = StreamController<StreamMessagesResponse>();
        final secondController =
            StreamController<StreamMessagesResponse>.broadcast();
        fakeChatService.streamControllers = [firstController, secondController];
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);
        // Use zero delay so reconnect fires immediately.
        notifier.reconnectDelay = (_) => Duration.zero;

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Close the first stream — triggers onDone, which schedules a reconnect.
        await firstController.close();
        // Allow the zero-delay timer to fire and the second subscription to attach.
        await Future.delayed(const Duration(milliseconds: 50));

        // Deliver a message on the second (reconnected) stream.
        secondController.add(
          createUserStreamResponse(
            messageId: 'msg-reconnected',
            conversationId: conversationId,
            sender: User(id: otherUserId, name: 'Other'),
            text: 'After reconnect',
            sentAtUnixSec: Int64(5000),
          ),
        );
        await Future.delayed(const Duration(milliseconds: 50));

        final state = container.read(conversationProvider(conversationId));
        expect(state.messages.length, 1);
        expect(state.messages[0].messageId, 'msg-reconnected');
        expect(state.isStreamDisconnected, isFalse);

        sub.close();
        await secondController.close();
      });

      test('reconnects after stream onError', () async {
        final firstController = StreamController<StreamMessagesResponse>();
        final secondController =
            StreamController<StreamMessagesResponse>.broadcast();
        fakeChatService.streamControllers = [firstController, secondController];
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);
        notifier.reconnectDelay = (_) => Duration.zero;

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Add an error to the first stream — triggers onError reconnect.
        firstController.addError(Exception('network drop'));
        await firstController.close();
        await Future.delayed(const Duration(milliseconds: 50));

        // Deliver a message on the reconnected stream.
        secondController.add(
          createUserStreamResponse(
            messageId: 'msg-after-error',
            conversationId: conversationId,
            sender: User(id: otherUserId, name: 'Other'),
            text: 'Recovered',
            sentAtUnixSec: Int64(6000),
          ),
        );
        await Future.delayed(const Duration(milliseconds: 50));

        final state = container.read(conversationProvider(conversationId));
        expect(state.messages.any((m) => m.messageId == 'msg-after-error'),
            isTrue);
        expect(state.isStreamDisconnected, isFalse);

        sub.close();
        await secondController.close();
      });

      test('sets isStreamDisconnected after max retries', () async {
        // Create one stream controller per call (6 calls: initial + 5 retries
        // that all close immediately, exhausting max attempts).
        // 5 is _maxReconnectAttempts in ConversationNotifier.
        const totalCalls = 5 + 1;
        final controllers = List.generate(
          totalCalls,
          (_) => StreamController<StreamMessagesResponse>(),
        );
        fakeChatService.streamControllers = controllers;
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);
        notifier.reconnectDelay = (_) => Duration.zero;

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Close all streams in sequence; each close triggers a reconnect.
        for (final ctrl in controllers) {
          await ctrl.close();
          await Future.delayed(const Duration(milliseconds: 10));
        }

        final state = container.read(conversationProvider(conversationId));
        expect(state.isStreamDisconnected, isTrue);

        sub.close();
      });

      test('clears isStreamDisconnected when stream recovers', () async {
        // 5 retries (exhausts max) + 1 initial + 1 extra for recovery.
        const totalCalls = 5 + 2;
        final controllers = List.generate(
          totalCalls,
          (_) => StreamController<StreamMessagesResponse>(),
        );
        fakeChatService.streamControllers = controllers;
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);
        notifier.reconnectDelay = (_) => Duration.zero;

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Exhaust max retries by closing controllers[0..5].
        for (int i = 0; i < totalCalls - 1; i++) {
          await controllers[i].close();
          await Future.delayed(const Duration(milliseconds: 10));
        }

        expect(
          container.read(conversationProvider(conversationId)).isStreamDisconnected,
          isTrue,
        );

        // Calling startMessageStream() explicitly (e.g. from a pull-to-refresh)
        // resets the counter and re-subscribes to the next available stream.
        notifier.startMessageStream();
        await Future.delayed(const Duration(milliseconds: 10));

        // Deliver a message on the recovery stream (controllers.last).
        controllers.last.add(
          createUserStreamResponse(
            messageId: 'msg-recovery',
            conversationId: conversationId,
            sender: User(id: otherUserId, name: 'Other'),
            text: 'Back online',
            sentAtUnixSec: Int64(7000),
          ),
        );
        await Future.delayed(const Duration(milliseconds: 50));

        final state = container.read(conversationProvider(conversationId));
        expect(state.isStreamDisconnected, isFalse);
        expect(state.messages.any((m) => m.messageId == 'msg-recovery'), isTrue);

        sub.close();
        await controllers.last.close();
      });

      test('disposal while reconnect timer is pending does not crash', () async {
        final firstController = StreamController<StreamMessagesResponse>();
        fakeChatService.streamControllers = [firstController];
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        // Use a non-zero delay so the timer is pending when disposed.
        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);
        notifier.reconnectDelay = (_) => const Duration(seconds: 60);

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Close the stream — schedules a 60-second timer.
        await firstController.close();
        await Future.delayed(const Duration(milliseconds: 10));

        // Dispose the container while the timer is still pending.
        sub.close();
        container.dispose();

        // Allow any pending microtasks to run; no exception should be thrown.
        await Future.delayed(const Duration(milliseconds: 10));
      });

      test('gap-fill merges new messages after reconnect', () async {
        final firstController = StreamController<StreamMessagesResponse>();
        final secondController =
            StreamController<StreamMessagesResponse>.broadcast();
        fakeChatService.streamControllers = [firstController, secondController];

        final existingMsg = createUserHistoryItem(
          messageId: 'msg-existing',
          sender: User(id: otherUserId, name: 'Other'),
          text: 'Before disconnect',
          sentAtUnixSec: Int64(1000),
        );
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [existingMsg],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);
        notifier.reconnectDelay = (_) => Duration.zero;

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Simulate a message arriving during disconnect by updating the history
        // response before the reconnect timer fires.
        final newMsg = createUserHistoryItem(
          messageId: 'msg-gap',
          sender: User(id: otherUserId, name: 'Other'),
          text: 'Sent during disconnect',
          sentAtUnixSec: Int64(2000),
        );
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [existingMsg, newMsg],
        );

        await firstController.close();
        await Future.delayed(const Duration(milliseconds: 50));

        final state = container.read(conversationProvider(conversationId));
        expect(state.messages.any((m) => m.messageId == 'msg-gap'), isTrue,
            reason: 'Gap-fill message should appear after reconnect');
        expect(state.messages.any((m) => m.messageId == 'msg-existing'), isTrue,
            reason: 'Existing message should still be present');

        sub.close();
        await secondController.close();
      });

      test('gap-fill does not duplicate messages already in local state',
          () async {
        final firstController = StreamController<StreamMessagesResponse>();
        final secondController =
            StreamController<StreamMessagesResponse>.broadcast();
        fakeChatService.streamControllers = [firstController, secondController];

        final existingMsg = createUserHistoryItem(
          messageId: 'msg-existing',
          sender: User(id: otherUserId, name: 'Other'),
          text: 'Before disconnect',
          sentAtUnixSec: Int64(1000),
        );
        // History returns the same message on reconnect — must not duplicate.
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [existingMsg],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);
        notifier.reconnectDelay = (_) => Duration.zero;

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        await firstController.close();
        await Future.delayed(const Duration(milliseconds: 50));

        final state = container.read(conversationProvider(conversationId));
        final existingCount = state.messages
            .where((m) => m.messageId == 'msg-existing')
            .length;
        expect(existingCount, 1, reason: 'No duplicate messages after gap-fill');

        sub.close();
        await secondController.close();
      });

      test('gap-fill preserves pending optimistic entries', () async {
        final firstController = StreamController<StreamMessagesResponse>();
        final secondController =
            StreamController<StreamMessagesResponse>.broadcast();
        fakeChatService.streamControllers = [firstController, secondController];
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);
        notifier.reconnectDelay = (_) => Duration.zero;

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Optimistically add a pending message (don't await — send is in-flight).
        final sendFuture = notifier.sendMessage('Pending during disconnect');
        await Future.delayed(const Duration(milliseconds: 10));

        var state = container.read(conversationProvider(conversationId));
        expect(state.messages.any((m) => m.state == MessageState.pending),
            isTrue);

        // Close stream — gap-fill runs, must not drop the pending entry.
        await firstController.close();
        await Future.delayed(const Duration(milliseconds: 50));

        state = container.read(conversationProvider(conversationId));
        expect(
          state.messages.any((m) => m.state == MessageState.pending),
          isTrue,
          reason: 'Pending optimistic entry must survive gap-fill',
        );

        await sendFuture;
        sub.close();
        await secondController.close();
      });
    });

    group('refreshAfterResume', () {
      test('merges new server messages non-destructively', () async {
        final firstController = StreamController<StreamMessagesResponse>();
        final secondController =
            StreamController<StreamMessagesResponse>.broadcast();
        fakeChatService.streamControllers = [firstController, secondController];

        final existingMsg = createUserHistoryItem(
          messageId: 'msg-existing',
          sender: User(id: otherUserId, name: 'Other'),
          text: 'Before sleep',
          sentAtUnixSec: Int64(1000),
        );
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [existingMsg],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Simulate a message arriving while device was asleep.
        final gapMsg = createUserHistoryItem(
          messageId: 'msg-during-sleep',
          sender: User(id: otherUserId, name: 'Other'),
          text: 'Sent while you slept',
          sentAtUnixSec: Int64(2000),
        );
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [existingMsg, gapMsg],
        );

        await notifier.refreshAfterResume();

        final state = container.read(conversationProvider(conversationId));
        expect(state.messages.any((m) => m.messageId == 'msg-during-sleep'),
            isTrue,
            reason: 'Resume should merge message received during sleep');
        expect(state.messages.any((m) => m.messageId == 'msg-existing'), isTrue,
            reason: 'Previously loaded message must still be present');

        sub.close();
        await firstController.close();
        await secondController.close();
      });

      test('preserves pending optimistic entries', () async {
        final firstController = StreamController<StreamMessagesResponse>();
        final secondController =
            StreamController<StreamMessagesResponse>.broadcast();
        fakeChatService.streamControllers = [firstController, secondController];
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        // Add a pending message.
        final sendFuture = notifier.sendMessage('Optimistic message');
        await Future.delayed(const Duration(milliseconds: 10));

        var state = container.read(conversationProvider(conversationId));
        expect(state.messages.any((m) => m.state == MessageState.pending),
            isTrue);

        await notifier.refreshAfterResume();

        state = container.read(conversationProvider(conversationId));
        expect(
          state.messages.any((m) => m.state == MessageState.pending),
          isTrue,
          reason: 'Pending entry must survive refreshAfterResume',
        );

        await sendFuture;
        sub.close();
        await firstController.close();
        await secondController.close();
      });

      test('calls markMessagesRead when unreadCount > 0', () async {
        final controller = StreamController<StreamMessagesResponse>();
        final secondController =
            StreamController<StreamMessagesResponse>.broadcast();
        fakeChatService.streamControllers = [controller, secondController];
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
          unreadCount: 3,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        final markCountBefore = fakeChatService.markedAsReadCount;
        await notifier.refreshAfterResume();

        expect(
          fakeChatService.markedAsReadCount,
          greaterThan(markCountBefore),
          reason: 'markMessagesRead should be called when unreadCount > 0',
        );

        sub.close();
        await controller.close();
        await secondController.close();
      });

      test('fires onMessagesMarkedAsRead callback', () async {
        final controller = StreamController<StreamMessagesResponse>();
        final secondController =
            StreamController<StreamMessagesResponse>.broadcast();
        fakeChatService.streamControllers = [controller, secondController];
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);

        String? markedReadConversationId;
        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
          unreadCount: 2,
          onMessagesMarkedAsRead: (id) {
            markedReadConversationId = id;
          },
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        await notifier.refreshAfterResume();

        expect(markedReadConversationId, conversationId,
            reason: 'onMessagesMarkedAsRead callback must fire with the conversation ID');

        sub.close();
        await controller.close();
        await secondController.close();
      });

      test('clears isStreamDisconnected', () async {
        // Exhaust max retries to set isStreamDisconnected.
        const totalCalls = 5 + 2;
        final controllers = List.generate(
          totalCalls,
          (_) => StreamController<StreamMessagesResponse>(),
        );
        fakeChatService.streamControllers = controllers;
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);
        notifier.reconnectDelay = (_) => Duration.zero;

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        for (int i = 0; i < totalCalls - 1; i++) {
          await controllers[i].close();
          await Future.delayed(const Duration(milliseconds: 10));
        }

        expect(
          container.read(conversationProvider(conversationId)).isStreamDisconnected,
          isTrue,
        );

        await notifier.refreshAfterResume();

        expect(
          container.read(conversationProvider(conversationId)).isStreamDisconnected,
          isFalse,
          reason: 'refreshAfterResume must clear isStreamDisconnected',
        );

        sub.close();
        await controllers.last.close();
      });

      test('re-subscribes to stream after resume', () async {
        final firstController = StreamController<StreamMessagesResponse>();
        final secondController =
            StreamController<StreamMessagesResponse>.broadcast();
        fakeChatService.streamControllers = [firstController, secondController];
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );

        final container = createContainer();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);

        await notifier.initialize(
          currentUserId: currentUserId,
          currentUserName: 'Test User',
          conversation: conversation,
        );

        final sub = container.listen(
          conversationProvider(conversationId),
          (_, _) {},
        );

        await notifier.refreshAfterResume();

        // A message on the second stream (opened after refresh) should arrive.
        secondController.add(
          createUserStreamResponse(
            messageId: 'msg-after-resume',
            conversationId: conversationId,
            sender: User(id: otherUserId, name: 'Other'),
            text: 'Post-resume message',
            sentAtUnixSec: Int64(9000),
          ),
        );
        await Future.delayed(const Duration(milliseconds: 50));

        final state = container.read(conversationProvider(conversationId));
        expect(
          state.messages.any((m) => m.messageId == 'msg-after-resume'),
          isTrue,
          reason: 'refreshAfterResume must open a new stream subscription',
        );

        sub.close();
        await firstController.close();
        await secondController.close();
      });
    });

    group('editMessage', () {
      Future<ProviderContainer> setupWithOwnMessage() async {
        final container = createContainer();
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [
            createUserHistoryItem(
              messageId: 'msg-1',
              sender: User(id: currentUserId, name: 'Me'),
              text: 'original',
              sentAtUnixSec: Int64(1000),
            ),
          ],
        );
        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );
        return container;
      }

      test('optimistically replaces text and persists', () async {
        final container = await setupWithOwnMessage();

        await container
            .read(conversationProvider(conversationId).notifier)
            .editMessage('msg-1', 'edited');

        final state = container.read(conversationProvider(conversationId));
        final edited =
            state.messages.firstWhere((m) => m.messageId == 'msg-1');
        expect(edited.text, 'edited');
        expect(edited.isEdited, isTrue);
        expect(fakeChatService.lastEditedMessageId, 'msg-1');
        expect(fakeChatService.lastEditedText, 'edited');
      });

      test('reverts text on failure', () async {
        final container = await setupWithOwnMessage();
        fakeChatService.editMessageError = Exception('boom');

        await expectLater(
          container
              .read(conversationProvider(conversationId).notifier)
              .editMessage('msg-1', 'edited'),
          throwsException,
        );

        final state = container.read(conversationProvider(conversationId));
        final reverted =
            state.messages.firstWhere((m) => m.messageId == 'msg-1');
        expect(reverted.text, 'original');
        expect(reverted.isEdited, isFalse);
      });

      test('clears editing mode on success', () async {
        final container = await setupWithOwnMessage();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);

        notifier.beginEditing('msg-1');
        expect(
          container.read(conversationProvider(conversationId)).editingMessageId,
          'msg-1',
        );

        await notifier.editMessage('msg-1', 'edited');

        expect(
          container.read(conversationProvider(conversationId)).editingMessageId,
          isNull,
        );
      });
    });

    group('compose modes', () {
      Future<ProviderContainer> setup() async {
        final container = createContainer();
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [],
        );
        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );
        return container;
      }

      test('beginReplying sets reply state and clears editing', () async {
        final container = await setup();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);

        notifier.beginEditing('msg-1');
        notifier.beginReplying('msg-2', 'Alice', 'hello there');

        final state = container.read(conversationProvider(conversationId));
        expect(state.replyingToMessageId, 'msg-2');
        expect(state.replyingToSenderName, 'Alice');
        expect(state.replyingToText, 'hello there');
        expect(state.editingMessageId, isNull);
      });

      test('cancelReplying clears reply state', () async {
        final container = await setup();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);

        notifier.beginReplying('msg-2', 'Alice', 'hello');
        notifier.cancelReplying();

        final state = container.read(conversationProvider(conversationId));
        expect(state.replyingToMessageId, isNull);
      });

      test('beginEditing clears an active reply', () async {
        final container = await setup();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);

        notifier.beginReplying('msg-2', 'Alice', 'hello');
        notifier.beginEditing('msg-1');

        final state = container.read(conversationProvider(conversationId));
        expect(state.editingMessageId, 'msg-1');
        expect(state.replyingToMessageId, isNull);
      });

      test('sendMessage forwards replyToMessageId to the service', () async {
        final container = await setup();
        final notifier =
            container.read(conversationProvider(conversationId).notifier);

        await notifier.sendMessage('a reply', replyToMessageId: 'msg-2');

        expect(fakeChatService.lastSendRequest?.replyToMessageId, 'msg-2');
      });
    });

    group('deleteMessage', () {
      Future<ProviderContainer> setupWithOwnMessage() async {
        final container = createContainer();
        fakeChatService.historyResponse = GetConversationHistoryResponse(
          messages: [
            createUserHistoryItem(
              messageId: 'msg-1',
              sender: User(id: currentUserId, name: 'Me'),
              text: 'to delete',
              sentAtUnixSec: Int64(1000),
            ),
          ],
        );
        await container
            .read(conversationProvider(conversationId).notifier)
            .initialize(
              currentUserId: currentUserId,
              currentUserName: 'Test User',
              conversation: conversation,
            );
        return container;
      }

      test('optimistically removes the message and persists', () async {
        final container = await setupWithOwnMessage();

        await container
            .read(conversationProvider(conversationId).notifier)
            .deleteMessage('msg-1');

        final state = container.read(conversationProvider(conversationId));
        expect(state.messages.any((m) => m.messageId == 'msg-1'), isFalse);
        expect(fakeChatService.deletedMessageIds, contains('msg-1'));
      });

      test('re-inserts the message on failure', () async {
        final container = await setupWithOwnMessage();
        fakeChatService.deleteMessageError = Exception('boom');

        await expectLater(
          container
              .read(conversationProvider(conversationId).notifier)
              .deleteMessage('msg-1'),
          throwsException,
        );

        final state = container.read(conversationProvider(conversationId));
        expect(state.messages.any((m) => m.messageId == 'msg-1'), isTrue);
      });
    });
  });
}
