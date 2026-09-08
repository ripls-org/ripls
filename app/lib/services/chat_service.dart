import 'package:connectrpc/connect.dart' as connect;
import 'package:fixnum/fixnum.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/conversation.pb.dart';
import 'package:ripls/data/gen/ripls/api/conversation_topic.pb.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;

final _log = ObservableLogger.named('ChatService');

/// ChatService handles chat-related operations using the ChatService API.
class ChatService {
  final ChatServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  ChatService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  }) : _client = ChatServiceClient(transport),
       _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// SendMessage sends a message in a conversation.
  ///
  /// Returns the message ID and timestamp.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<SendMessageResponse> sendMessage({
    required String conversationId,
    required String text,
    List<String> mediaIds = const [],
    String? replyToMessageId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SendMessageRequest(
          conversationId: conversationId,
          text: text,
          mediaIds: mediaIds,
          replyToMessageId: replyToMessageId,
        );

        final response = await _client.sendMessage(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SendMessage',
    );
  }

  /// GetConversationHistory retrieves message history for a conversation.
  ///
  /// Returns the message history with pagination support.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetConversationHistoryResponse> getConversationHistory({
    required String conversationId,
    int? maxMessages,
    int? beforeUnixSec,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetConversationHistoryRequest(
          conversationId: conversationId,
          maxMessages: maxMessages ?? 50,
          beforeUnixSec: beforeUnixSec != null
              ? Int64(beforeUnixSec)
              : Int64.ZERO,
        );

        final response = await _client.getConversationHistory(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetConversationHistory',
    );
  }

  /// MarkMessagesRead marks messages in a conversation as read.
  ///
  /// Returns the number of messages marked as read.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<int> markMessagesRead({
    required String conversationId,
    int? upToUnixSec,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = MarkMessagesReadRequest(
          conversationId: conversationId,
          upToUnixSec: upToUnixSec != null ? Int64(upToUnixSec) : Int64.ZERO,
        );

        final response = await _client.markMessagesRead(
          request,
          headers: _buildHeaders(),
        );

        return response.messagesMarked;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'MarkMessagesRead',
    );
  }

  /// ListConversations retrieves conversations for the current user.
  ///
  /// If [archived] is true, returns archived conversations (done items with no
  /// unread messages). If false (default), returns active conversations.
  ///
  /// Returns the list of conversations.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<ConversationItem>> listConversations({
    bool archived = false,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListConversationsRequest(archived: archived);

        final response = await _client.listConversations(
          request,
          headers: _buildHeaders(),
        );

        return response.conversations;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListConversations',
    );
  }

  /// GetConversation retrieves a specific conversation by ID.
  ///
  /// Returns the conversation item.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ConversationItem> getConversation({
    required String conversationId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetConversationRequest(conversationId: conversationId);

        final response = await _client.getConversation(
          request,
          headers: _buildHeaders(),
        );

        return response.conversation;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetConversation',
    );
  }

  /// GetConversationContext retrieves context for UI display (topic title, image, etc.).
  ///
  /// This provides rich context about what the conversation is about,
  /// including the topic type, title, subtitle, and thumbnail image.
  /// Use this for conversation headers and inbox item display.
  ///
  /// Returns the conversation context.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ConversationContext> getConversationContext({
    required String conversationId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetConversationContextRequest(
          conversationId: conversationId,
        );

        final response = await _client.getConversationContext(
          request,
          headers: _buildHeaders(),
        );

        return response.context;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetConversationContext',
    );
  }

  /// StreamMessages streams messages for a conversation in real-time.
  ///
  /// Returns a stream of messages. The stream handles protocol errors gracefully
  /// by logging the error and completing normally rather than throwing.
  /// This prevents crashes from HTTP/2 connection termination or missing
  /// end-stream responses.
  ///
  /// Callers should handle stream completion and re-subscribe if needed.
  Stream<StreamMessagesResponse> streamMessages({
    required String conversationId,
  }) async* {
    final request = StreamMessagesRequest(conversationId: conversationId);

    try {
      final stream = _client.streamMessages(request, headers: _buildHeaders());

      await for (final message in stream) {
        yield message;
      }
    } on connect.ConnectException catch (e) {
      // Handle Connect-specific errors (includes protocol errors)
      _log.warning(
        'Stream connection error for conversation $conversationId: ${e.code.name} - ${e.message}',
      );
      // Don't rethrow - let the stream complete gracefully
      // Callers can re-subscribe if they detect the stream ended unexpectedly
    } catch (e) {
      // Handle unexpected errors (HTTP/2 termination, protocol errors, etc.)
      _log.warning(
        'Stream error for conversation $conversationId: $e',
      );
      // Don't rethrow - let the stream complete gracefully
    }
  }

  /// GetConversationForTransfer retrieves the full conversation item for a transfer.
  ///
  /// Returns the ConversationItem.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ConversationItem> getConversationForTransfer({
    required String transferId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetConversationForTransferRequest(
          transferId: transferId,
        );

        final response = await _client.getConversationForTransfer(
          request,
          headers: _buildHeaders(),
        );

        return response.conversation;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetConversationForTransfer',
    );
  }

  /// UpdatePresence updates the user's app presence status (foreground/background).
  ///
  /// This is used by the server to determine when to send push notifications.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> updatePresence({required bool isInForeground}) async {
    return RpcUtils.executeRpc(
      () async {
        final status = isInForeground
            ? PresenceStatus.PRESENCE_STATUS_FOREGROUND
            : PresenceStatus.PRESENCE_STATUS_BACKGROUND;

        final request = UpdatePresenceRequest(status: status);

        await _client.updatePresence(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UpdatePresence',
    );
  }

  /// StartConversation creates or retrieves a conversation for a given topic.
  ///
  /// Returns the conversation_id and initial participants.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<StartConversationResponse> startConversation({
    required String communityId,
    required ConversationTopic topic,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = StartConversationRequest(
          communityId: communityId,
          topic: topic,
        );
        final response = await _client.startConversation(
          request,
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'StartConversation',
    );
  }

  /// GetConversationForCommunity retrieves (or lazily creates) the community-wide conversation.
  ///
  /// Returns the ConversationItem for the community-wide chat.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ConversationItem> getConversationForCommunity({
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetConversationForCommunityRequest(
          communityId: communityId,
        );
        final response = await _client.getConversationForCommunity(
          request,
          headers: _buildHeaders(),
        );
        return response.conversation;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetConversationForCommunity',
    );
  }

  /// GetUnreadCounts retrieves unread message counts aggregated by community and conversation.
  ///
  /// AddReaction adds an emoji reaction to a message.
  ///
  /// One reaction per user per message; adding a new one replaces the previous.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<AddReactionResponse> addReaction({
    required String conversationId,
    required String messageId,
    required String emoji,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = AddReactionRequest(
          conversationId: conversationId,
          messageId: messageId,
          emoji: emoji,
        );

        final response = await _client.addReaction(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AddReaction',
    );
  }

  /// RemoveReaction removes the current user's reaction from a message.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<RemoveReactionResponse> removeReaction({
    required String conversationId,
    required String messageId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = RemoveReactionRequest(
          conversationId: conversationId,
          messageId: messageId,
        );

        final response = await _client.removeReaction(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RemoveReaction',
    );
  }

  /// EditMessage edits the text of a message the caller authored.
  ///
  /// Returns the edit timestamp.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<EditMessageResponse> editMessage({
    required String conversationId,
    required String messageId,
    required String text,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = EditMessageRequest(
          conversationId: conversationId,
          messageId: messageId,
          text: text,
        );

        final response = await _client.editMessage(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'EditMessage',
    );
  }

  /// DeleteMessage soft-deletes a message the caller authored.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> deleteMessage({
    required String conversationId,
    required String messageId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = DeleteMessageRequest(
          conversationId: conversationId,
          messageId: messageId,
        );

        await _client.deleteMessage(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DeleteMessage',
    );
  }

  /// Returns counts at three levels:
  /// - total_unread_count: Global total across all communities
  /// - community_id_to_unread_count: Per-community unread counts
  /// - conversation_id_to_unread_count: Per-conversation unread counts
  ///
  /// Excludes archived conversations (where is_item_done = true).
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetUnreadCountsResponse> getUnreadCounts() async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetUnreadCountsRequest();

        final response = await _client.getUnreadCounts(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetUnreadCounts',
    );
  }
}
