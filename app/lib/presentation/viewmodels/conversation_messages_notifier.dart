import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:flutter/foundation.dart' show visibleForTesting;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/models/chat_message.dart';
import 'package:ripls/presentation/viewmodels/conversation_state.dart';
import 'package:ripls/presentation/viewmodels/system_message_patch.dart';
import 'package:ripls/services/providers.dart';
import 'package:uuid/uuid.dart';

final _log = Logger('ConversationMessages');
final _uuid = Uuid();

/// ConversationMessagesMixin provides message-stream management and
/// send/retry/reaction methods for [ConversationNotifier].
///
/// Coordination pattern: this mixin is used by the single [ConversationNotifier]
/// coordinator class. All state reads and writes go through [state] and
/// [state.copyWith], which are the coordinator's own [Notifier.state] fields.
/// This avoids cross-provider dependencies entirely.
///
/// Ownership: [_messageStreamSubscription], [_messageIds], and
/// [_pendingMessageTimers] live here. The coordinator's [build] method must
/// call [disposeMessageResources] inside [ref.onDispose].
mixin ConversationMessagesMixin on Notifier<ConversationState> {
  // Stream subscription for real-time messages.
  StreamSubscription<StreamMessagesResponse>? _messageStreamSubscription;

  // Track message IDs to avoid duplicates.
  final Set<String> _messageIds = {};

  // Track timeouts for pending messages.
  final Map<String, Timer> _pendingMessageTimers = {};

  // Callback to notify inbox when messages are marked as read.
  Function(String conversationId)? _onMessagesMarkedAsRead;

  // Stream reconnection state.
  int _reconnectAttempts = 0;
  static const int _maxReconnectAttempts = 5;
  Timer? _reconnectTimer;

  /// reconnectDelay returns the backoff duration for each attempt (1-indexed).
  ///
  /// Defaults to exponential backoff starting at 1 s. Override in tests to
  /// make reconnect happen immediately without real-time delays.
  @visibleForTesting
  Duration Function(int attempt) reconnectDelay =
      (attempt) => Duration(seconds: 1 << (attempt - 1));

  /// disposeMessageResources cancels the stream subscription, reconnect timer,
  /// and pending message timers.
  ///
  /// Must be called from the coordinator's [ref.onDispose] in [build].
  void disposeMessageResources() {
    _reconnectTimer?.cancel();
    unawaited(_messageStreamSubscription?.cancel());
    for (final timer in _pendingMessageTimers.values) {
      timer.cancel();
    }
    _pendingMessageTimers.clear();
    _messageIds.clear();
  }

  /// setOnMessagesMarkedAsRead stores the callback for inbox unread-count resets.
  void setOnMessagesMarkedAsRead(
    Function(String conversationId)? callback,
  ) {
    _onMessagesMarkedAsRead = callback;
  }

  /// loadMessages loads conversation history from the server.
  Future<void> loadMessages() async {
    if (state.conversation == null) {
      state = state.copyWith(
        isLoadingMessages: false,
        error: const UserError.generic(fallback: 'No conversation found'),
      );
      return;
    }

    state = state.copyWith(isLoadingMessages: true, error: null);

    try {
      final conversationId = state.conversation!.conversationId;
      final unreadCount = state.unreadCount;
      _log.info(
        '📥 Loading messages for conversation: $conversationId '
        '(unread: $unreadCount)',
      );

      final chatRepository = ref.read(chatRepositoryProvider);
      await chatRepository.refreshConversationHistory(conversationId);
      final response = await chatRepository.getConversationHistory(
        conversationId: conversationId,
      );

      // Clear existing messages.
      _messageIds.clear();
      final newMessages = <ChatMessage>[];

      for (final msg in response.messages.reversed) {
        final sender = msg.hasUserMessage()
            ? msg.userMessage.sender
            : (msg.hasSystemMessage() && msg.systemMessage.hasActor()
                ? msg.systemMessage.actor
                : User());

        newMessages.add(
          ChatMessage(
            message: msg,
            sender: sender,
            state: MessageState.delivered,
            isTemporary: false,
          ),
        );
        _messageIds.add(msg.messageId);
      }

      _log.info(
        '✅ Loaded ${newMessages.length} messages for conversation: $conversationId',
      );

      state = state.copyWith(
        messages: _sortByChronology(newMessages),
        isLoadingMessages: false,
      );

      // Mark messages as read after successfully loading them.
      if (unreadCount > 0) {
        _log.info(
          '📝 Marking $unreadCount messages as read for conversation: $conversationId',
        );
        try {
          final markedCount = await chatRepository.markMessagesRead(
            conversationId: conversationId,
          );
          _log.info('✅ Marked $markedCount messages as read');
          _onMessagesMarkedAsRead?.call(conversationId);
        } catch (e) {
          _log.warning('⚠️  Failed to mark messages as read: $e');
        }
      }
    } catch (e) {
      _log.severe('❌ Failed to load messages: $e');
      state = state.copyWith(
        isLoadingMessages: false,
        error: RpcErrorHandler.classify(e),
      );
    }
  }

  /// startMessageStream opens the real-time message stream with reconnection.
  ///
  /// Resets the reconnect counter and clears any disconnected state, then
  /// attaches onError and onDone handlers that schedule exponential-backoff
  /// reconnection. After [_maxReconnectAttempts] consecutive failures,
  /// [ConversationState.isStreamDisconnected] is set so the UI can show a
  /// banner. Calling this method again clears the banner and starts fresh.
  void startMessageStream() {
    _reconnectAttempts = 0;
    _reconnectTimer?.cancel();
    if (state.isStreamDisconnected) {
      state = state.copyWith(isStreamDisconnected: false);
    }
    _doStartStream();
  }

  /// _doStartStream attaches a subscription to the message stream.
  ///
  /// Used by both [startMessageStream] (external) and the reconnect timer
  /// (internal). Does not reset the reconnect counter.
  void _doStartStream() {
    if (state.conversation == null) {
      _log.warning('Cannot start message stream: no conversation');
      return;
    }

    final conversationId = state.conversation!.conversationId;

    try {
      final chatRepository = ref.read(chatRepositoryProvider);
      final stream = chatRepository.streamMessages(
        conversationId: conversationId,
      );

      unawaited(_messageStreamSubscription?.cancel());
      _messageStreamSubscription = stream.listen(
        (response) {
          _onStreamMessage(response);
        },
        onError: (error) {
          if (!ref.mounted) return;
          _log.severe('Message stream error: $error');
          _scheduleReconnect(conversationId);
        },
        onDone: () {
          if (!ref.mounted) return;
          _log.info('Message stream ended for $conversationId');
          _scheduleReconnect(conversationId);
        },
      );
    } catch (e) {
      _log.severe('Failed to start message stream: $e');
    }
  }

  /// _scheduleReconnect schedules an exponential-backoff reconnect attempt.
  ///
  /// After [_maxReconnectAttempts] consecutive failures, sets
  /// [ConversationState.isStreamDisconnected] and stops retrying.
  void _scheduleReconnect(String conversationId) {
    if (!ref.mounted) return;
    _reconnectAttempts++;
    _reconnectTimer?.cancel();

    if (_reconnectAttempts > _maxReconnectAttempts) {
      _log.severe(
        'Max reconnect attempts reached for $conversationId — marking stream disconnected',
      );
      state = state.copyWith(isStreamDisconnected: true);
      return;
    }

    final delay = reconnectDelay(_reconnectAttempts);
    _log.info(
      'Scheduling reconnect attempt $_reconnectAttempts/$_maxReconnectAttempts '
      'in ${delay.inMilliseconds}ms for $conversationId',
    );
    _reconnectTimer = Timer(delay, () async {
      if (!ref.mounted) return;
      unawaited(_messageStreamSubscription?.cancel());
      _messageStreamSubscription = null;
      try {
        await _mergeServerHistory(conversationId);
      } catch (e) {
        _log.warning('Reconnect history gap-fill failed: $e');
      }
      if (!ref.mounted) return;
      _doStartStream();
    });
  }

  /// _onStreamMessage resets reconnect state on first message after a gap,
  /// then delegates to [_handleStreamedMessage].
  void _onStreamMessage(StreamMessagesResponse response) {
    if (_reconnectAttempts > 0) {
      _reconnectAttempts = 0;
    }
    if (state.isStreamDisconnected) {
      state = state.copyWith(isStreamDisconnected: false);
    }
    _handleStreamedMessage(response);
  }

  /// _handleStreamedMessage processes a new message from the real-time stream.
  void _handleStreamedMessage(StreamMessagesResponse response) {
    if (!ref.mounted) {
      _log.info('⚠️  Provider unmounted, ignoring stream message');
      return;
    }

    if (response.hasReactionUpdate()) {
      _handleReactionUpdate(response.reactionUpdate);
      return;
    }

    if (response.hasSystemMessageUpdate()) {
      _handleSystemMessageUpdate(response.systemMessageUpdate);
      return;
    }

    if (response.hasUserMessageUpdate()) {
      _handleUserMessageUpdate(response.userMessageUpdate);
      return;
    }

    if (response.hasMessageDelete()) {
      _handleMessageDelete(response.messageDelete);
      return;
    }

    final messageId = response.messageId;
    final sender = response.hasUserMessage()
        ? response.userMessage.sender
        : (response.hasSystemMessage() && response.systemMessage.hasActor()
            ? response.systemMessage.actor
            : User());
    final senderId = sender.id;
    final sentAt = response.sentAtUnixSec.toInt();

    final text = response.hasUserMessage()
        ? response.userMessage.text
        : (response.hasSystemMessage()
            ? response.systemMessage.description
            : '');

    _log.info(
      '📨 Stream message received: messageId=$messageId, senderId=$senderId, sentAt=$sentAt',
    );

    // Check if this matches a pending optimistic message.
    final pendingIndex = state.messages.indexWhere(
      (chatMsg) =>
          chatMsg.isTemporary &&
          chatMsg.senderId == senderId &&
          chatMsg.state == MessageState.pending &&
          chatMsg.text == text,
    );

    _log.info(
      '🔍 Pending message match check: pendingIndex=$pendingIndex, matchedByText=$text',
    );

    if (pendingIndex != -1) {
      final oldChatMsg = state.messages[pendingIndex];
      final oldTempId = oldChatMsg.messageId;

      _pendingMessageTimers[oldTempId]?.cancel();
      _pendingMessageTimers.remove(oldTempId);

      _messageIds.remove(oldTempId);
      _messageIds.add(messageId);

      final updatedMessage = MessageHistoryItem(
        messageId: messageId,
        sentAtUnixSec: response.sentAtUnixSec,
        isRead: false,
      );

      if (response.hasUserMessage()) {
        updatedMessage.userMessage = response.userMessage;
      } else if (response.hasSystemMessage()) {
        updatedMessage.systemMessage = response.systemMessage;
      }

      final updatedMessages = List<ChatMessage>.from(state.messages);
      updatedMessages[pendingIndex] = ChatMessage(
        message: updatedMessage,
        sender: sender,
        state: MessageState.delivered,
        isTemporary: false,
      );

      state = state.copyWith(messages: _sortByChronology(updatedMessages));

      _log.info(
        '✅ Updated pending message to delivered: $oldTempId -> $messageId',
      );

      if (response.hasUserMessage() &&
          response.userMessage.mediaIds.isNotEmpty) {
        invalidateParentEntityCacheIfNeeded();
      }

      return;
    }

    // Prevent duplicates.
    if (_messageIds.contains(messageId)) {
      return;
    }

    final message = MessageHistoryItem(
      messageId: messageId,
      sentAtUnixSec: response.sentAtUnixSec,
      isRead: false,
    );

    if (response.hasUserMessage()) {
      message.userMessage = response.userMessage;
    } else if (response.hasSystemMessage()) {
      message.systemMessage = response.systemMessage;
    }

    final chatMessage = ChatMessage(
      message: message,
      sender: sender,
      state: MessageState.delivered,
      isTemporary: false,
    );

    final updatedMessages = List<ChatMessage>.from(state.messages)
      ..add(chatMessage);
    _messageIds.add(messageId);

    state = state.copyWith(messages: _sortByChronology(updatedMessages));

    if (response.hasUserMessage() && response.userMessage.mediaIds.isNotEmpty) {
      invalidateParentEntityCacheIfNeeded();
    }

    if (response.hasSystemMessage()) {
      final action = response.systemMessage.action;
      if (action == ChatSystemAction.CHAT_SYSTEM_ACTION_JOINED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_LEFT) {
        refreshParticipantsFromSystemMessage();
      }
      if (action == ChatSystemAction.CHAT_SYSTEM_ACTION_RSVP_YES ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_RSVP_MAYBE ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_RSVP_NO ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_TIME_PROPOSED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_DETAIL_CHANGED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_CANCELLED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_STARTED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_COMPLETED) {
        invalidateAndRefreshExperience();
      }
      if (action == ChatSystemAction.CHAT_SYSTEM_ACTION_OFFERED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_LEFT ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_FULFILLED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_CANCELLED) {
        fetchRequestDetails();
      }
      if (action == ChatSystemAction.CHAT_SYSTEM_ACTION_FULFILLED &&
          cachedImpact == null) {
        fetchRequestImpact();
      }
      if (action == ChatSystemAction.CHAT_SYSTEM_ACTION_STARTED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_COMPLETED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_CANCELLED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_APPROVED ||
          action == ChatSystemAction.CHAT_SYSTEM_ACTION_JOINED) {
        final gearId = state.conversationContext?.topic.gearId ?? '';
        if (gearId.isNotEmpty) {
          final transferRepo = ref.read(transferRepositoryProvider);
          transferRepo.invalidateGearTransfers(gearId);
          transferRepo.invalidateUserTransferStatus(gearId);
          transferRepo.refreshMyTransfers();
          ref.read(gearRepositoryProvider).invalidate(gearId);
        }
        ref.read(transferCacheInvalidationProvider.notifier).notify();
        fetchConversationContext();
      }
    }
  }

  /// _handleSystemMessageUpdate patches an existing system message card in place.
  void _handleSystemMessageUpdate(SystemMessageUpdate update) {
    final idx =
        state.messages.indexWhere((m) => m.messageId == update.messageId);
    if (idx == -1) return;

    final existing = state.messages[idx];
    final patched = patchedSystemMessage(existing.message, update);

    final updatedMessages = List<ChatMessage>.from(state.messages);
    updatedMessages[idx] = existing.copyWith(message: patched);
    state = state.copyWith(messages: updatedMessages);

    final conversationId = state.conversation?.conversationId;
    if (conversationId != null) {
      ref
          .read(chatRepositoryProvider)
          .refreshConversationHistory(conversationId);
    }
  }

  /// _handleReactionUpdate patches the matching message's reactions in local state.
  void _handleReactionUpdate(ReactionUpdate update) {
    final targetId = update.messageId;
    final idx = state.messages.indexWhere((m) => m.messageId == targetId);
    if (idx == -1) return;

    final existing = state.messages[idx];
    final patched = MessageHistoryItem()..mergeFromMessage(existing.message);
    patched.reactions
      ..clear()
      ..addAll(update.reactions);

    final updatedMessages = List<ChatMessage>.from(state.messages);
    updatedMessages[idx] = existing.copyWith(message: patched);
    state = state.copyWith(messages: updatedMessages);
  }

  /// _handleUserMessageUpdate patches an edited user message's text in place.
  void _handleUserMessageUpdate(UserMessageUpdate update) {
    final idx =
        state.messages.indexWhere((m) => m.messageId == update.messageId);
    if (idx == -1) return;

    final existing = state.messages[idx];
    if (!existing.message.hasUserMessage()) return;

    final patched = MessageHistoryItem()..mergeFromMessage(existing.message);
    patched.userMessage
      ..text = update.text
      ..editedAtUnixSec = update.editedAtUnixSec;

    final updatedMessages = List<ChatMessage>.from(state.messages);
    updatedMessages[idx] = existing.copyWith(message: patched);
    state = state.copyWith(messages: updatedMessages);
  }

  /// _handleMessageDelete removes a deleted message from local state.
  void _handleMessageDelete(MessageDelete delete) {
    final idx =
        state.messages.indexWhere((m) => m.messageId == delete.messageId);
    if (idx == -1) return;

    final updatedMessages = List<ChatMessage>.from(state.messages)
      ..removeAt(idx);
    _messageIds.remove(delete.messageId);
    state = state.copyWith(messages: updatedMessages);
  }

  /// sendMessage sends a message in the conversation with optimistic UI update.
  ///
  /// When [replyToMessageId] is set, the message is sent as a reply quoting that
  /// message. The optimistic entry does not render the quote block — the server
  /// echo carries the denormalized [ReplyContext].
  Future<void> sendMessage(
    String text, {
    List<String>? mediaIds,
    String? replyToMessageId,
  }) async {
    if (text.trim().isEmpty && (mediaIds == null || mediaIds.isEmpty)) {
      return;
    }

    if (state.conversation == null) {
      throw Exception('No conversation found');
    }

    final conversationId = state.conversation!.conversationId;

    final tempId = 'temp-${_uuid.v4()}';
    final now = DateTime.now();
    final sentAtUnixSec = Int64(now.millisecondsSinceEpoch ~/ 1000);

    final currentUser =
        User(id: state.currentUserId, name: state.currentUserName);

    final optimisticMessage = MessageHistoryItem(
      messageId: tempId,
      sentAtUnixSec: sentAtUnixSec,
      isRead: false,
    );

    optimisticMessage.userMessage = UserMessage(
      sender: currentUser,
      text: text,
      mediaIds: mediaIds ?? [],
    );

    final chatMessage = ChatMessage(
      message: optimisticMessage,
      sender: currentUser,
      state: MessageState.pending,
      isTemporary: true,
    );

    final updatedMessages = List<ChatMessage>.from(state.messages)
      ..add(chatMessage);
    _messageIds.add(tempId);

    state = state.copyWith(messages: updatedMessages, isSending: true);

    _log.info(
      '📤 Optimistically added message: $tempId '
      '(media: ${mediaIds?.length ?? 0})',
    );

    _pendingMessageTimers[tempId] = Timer(const Duration(seconds: 10), () {
      _onMessageTimeout(tempId);
    });

    try {
      final chatRepository = ref.read(chatRepositoryProvider);
      await chatRepository.sendMessage(
        conversationId: conversationId,
        text: text,
        mediaIds: mediaIds,
        replyToMessageId: replyToMessageId,
      );

      state = state.copyWith(isSending: false);

      _log.info('✅ Message sent to server: $tempId');
    } catch (e) {
      final index =
          state.messages.indexWhere((msg) => msg.messageId == tempId);
      if (index != -1) {
        final updatedMessages = List<ChatMessage>.from(state.messages);
        updatedMessages[index] = updatedMessages[index].copyWith(
          state: MessageState.failed,
        );

        state = state.copyWith(messages: updatedMessages, isSending: false);
        _log.severe('❌ Failed to send message: $tempId - $e');
      } else {
        state = state.copyWith(isSending: false);
      }

      _pendingMessageTimers[tempId]?.cancel();
      _pendingMessageTimers.remove(tempId);

      rethrow;
    }
  }

  /// _onMessageTimeout marks a pending message as failed after the timeout expires.
  void _onMessageTimeout(String tempId) {
    _log.warning('⏱️  Message timeout: $tempId');

    final index =
        state.messages.indexWhere((msg) => msg.messageId == tempId);
    if (index != -1 && state.messages[index].state == MessageState.pending) {
      final updatedMessages = List<ChatMessage>.from(state.messages);
      updatedMessages[index] = updatedMessages[index].copyWith(
        state: MessageState.failed,
      );
      _pendingMessageTimers.remove(tempId);

      state = state.copyWith(messages: updatedMessages);
    }
  }

  /// retryMessage removes a failed message and resends it.
  Future<void> retryMessage(String failedMessageId) async {
    final index = state.messages.indexWhere(
      (msg) => msg.messageId == failedMessageId,
    );
    if (index == -1) {
      _log.warning('Cannot retry: message not found: $failedMessageId');
      return;
    }

    final failedMessage = state.messages[index];
    if (failedMessage.state != MessageState.failed) {
      _log.warning(
        'Cannot retry: message is not in failed state: $failedMessageId',
      );
      return;
    }

    final updatedMessages = List<ChatMessage>.from(state.messages)
      ..removeAt(index);
    _messageIds.remove(failedMessageId);

    state = state.copyWith(messages: updatedMessages);

    _log.info('🔄 Retrying failed message: $failedMessageId');

    await sendMessage(failedMessage.text);
  }

  /// addReaction optimistically adds an emoji reaction to a message.
  ///
  /// Reverts on failure.
  Future<void> addReaction(String messageId, String emoji) async {
    final conversationId = state.conversation?.conversationId;
    if (conversationId == null) return;

    final idx = state.messages.indexWhere((m) => m.messageId == messageId);
    if (idx == -1) return;

    final existing = state.messages[idx];
    final optimistic = MessageHistoryItem()..mergeFromMessage(existing.message);

    optimistic.reactions
        .removeWhere((r) => r.sender.id == state.currentUserId);
    optimistic.reactions.add(Reaction(
      sender: User(id: state.currentUserId, name: state.currentUserName),
      emoji: emoji,
    ));

    final updatedMessages = List<ChatMessage>.from(state.messages);
    updatedMessages[idx] = existing.copyWith(message: optimistic);
    state = state.copyWith(messages: updatedMessages);

    try {
      final chatRepository = ref.read(chatRepositoryProvider);
      await chatRepository.addReaction(
        conversationId: conversationId,
        messageId: messageId,
        emoji: emoji,
      );
    } catch (e) {
      _log.warning('Failed to add reaction: $e');
      final revertMessages = List<ChatMessage>.from(state.messages);
      revertMessages[idx] = existing;
      state = state.copyWith(messages: revertMessages);
    }
  }

  /// removeReaction optimistically removes the current user's reaction.
  ///
  /// Reverts on failure.
  Future<void> removeReaction(String messageId) async {
    final conversationId = state.conversation?.conversationId;
    if (conversationId == null) return;

    final idx = state.messages.indexWhere((m) => m.messageId == messageId);
    if (idx == -1) return;

    final existing = state.messages[idx];
    final optimistic = MessageHistoryItem()..mergeFromMessage(existing.message);
    optimistic.reactions
        .removeWhere((r) => r.sender.id == state.currentUserId);

    final updatedMessages = List<ChatMessage>.from(state.messages);
    updatedMessages[idx] = existing.copyWith(message: optimistic);
    state = state.copyWith(messages: updatedMessages);

    try {
      final chatRepository = ref.read(chatRepositoryProvider);
      await chatRepository.removeReaction(
        conversationId: conversationId,
        messageId: messageId,
      );
    } catch (e) {
      _log.warning('Failed to remove reaction: $e');
      final revertMessages = List<ChatMessage>.from(state.messages);
      revertMessages[idx] = existing;
      state = state.copyWith(messages: revertMessages);
    }
  }

  /// beginEditing puts the compose bar into edit mode for [messageId],
  /// clearing any active reply. The widget prefills the input with the current
  /// text and relabels the send button to "Save".
  void beginEditing(String messageId) {
    state = state.copyWith(
      editingMessageId: messageId,
      replyingToMessageId: null,
      replyingToSenderName: null,
      replyingToText: null,
    );
  }

  /// cancelEditing exits edit mode without persisting.
  void cancelEditing() {
    if (state.editingMessageId == null) return;
    state = state.copyWith(editingMessageId: null);
  }

  /// beginReplying puts the compose bar into reply mode quoting [messageId],
  /// clearing any active edit. [senderName] and [text] populate the banner.
  void beginReplying(String messageId, String senderName, String text) {
    state = state.copyWith(
      replyingToMessageId: messageId,
      replyingToSenderName: senderName,
      replyingToText: text,
      editingMessageId: null,
    );
  }

  /// cancelReplying exits reply mode.
  void cancelReplying() {
    if (state.replyingToMessageId == null) return;
    state = state.copyWith(
      replyingToMessageId: null,
      replyingToSenderName: null,
      replyingToText: null,
    );
  }

  /// editMessage optimistically replaces a message's text, then persists.
  ///
  /// Only user messages the current user authored can be edited. Reverts on
  /// failure. Guards state writes with [ref.mounted] for autoDispose safety.
  Future<void> editMessage(String messageId, String newText) async {
    final conversationId = state.conversation?.conversationId;
    if (conversationId == null) return;
    if (newText.trim().isEmpty) return;

    final idx = state.messages.indexWhere((m) => m.messageId == messageId);
    if (idx == -1) return;

    final existing = state.messages[idx];
    if (!existing.message.hasUserMessage()) return;

    final optimistic = MessageHistoryItem()..mergeFromMessage(existing.message);
    optimistic.userMessage.text = newText;
    optimistic.userMessage.editedAtUnixSec =
        Int64(DateTime.now().millisecondsSinceEpoch ~/ 1000);

    final updatedMessages = List<ChatMessage>.from(state.messages);
    updatedMessages[idx] = existing.copyWith(message: optimistic);
    state = state.copyWith(messages: updatedMessages);

    try {
      final chatRepository = ref.read(chatRepositoryProvider);
      await chatRepository.editMessage(
        conversationId: conversationId,
        messageId: messageId,
        text: newText,
      );
      if (!ref.mounted) return;
      if (state.editingMessageId == messageId) {
        state = state.copyWith(editingMessageId: null);
      }
    } catch (e) {
      _log.warning('Failed to edit message: $e');
      if (!ref.mounted) return;
      final revertIdx =
          state.messages.indexWhere((m) => m.messageId == messageId);
      if (revertIdx == -1) return;
      final revertMessages = List<ChatMessage>.from(state.messages);
      revertMessages[revertIdx] = existing;
      state = state.copyWith(messages: revertMessages);
      rethrow;
    }
  }

  /// deleteMessage optimistically removes a message, then persists.
  ///
  /// Only user messages the current user authored can be deleted. Re-inserts
  /// the message on failure. Guards state writes with [ref.mounted].
  Future<void> deleteMessage(String messageId) async {
    final conversationId = state.conversation?.conversationId;
    if (conversationId == null) return;

    final idx = state.messages.indexWhere((m) => m.messageId == messageId);
    if (idx == -1) return;

    final removed = state.messages[idx];

    final updatedMessages = List<ChatMessage>.from(state.messages)
      ..removeAt(idx);
    _messageIds.remove(messageId);
    state = state.copyWith(messages: updatedMessages);

    try {
      final chatRepository = ref.read(chatRepositoryProvider);
      await chatRepository.deleteMessage(
        conversationId: conversationId,
        messageId: messageId,
      );
    } catch (e) {
      _log.warning('Failed to delete message: $e');
      if (!ref.mounted) return;
      _messageIds.add(messageId);
      final revertMessages =
          _sortByChronology([...state.messages, removed]);
      state = state.copyWith(messages: revertMessages);
      rethrow;
    }
  }

  /// _mergeServerHistory fetches server history and merges any messages not
  /// already in local state, leaving pending/failed optimistic entries untouched.
  ///
  /// Invalidates the cache first so the fetch returns fresh data. Emits a
  /// state update only when at least one new message is added (avoids spurious
  /// rebuilds). Guards every state write with [ref.mounted].
  Future<void> _mergeServerHistory(String conversationId) async {
    final chatRepository = ref.read(chatRepositoryProvider);
    await chatRepository.refreshConversationHistory(conversationId);
    final response = await chatRepository.getConversationHistory(
      conversationId: conversationId,
    );

    if (!ref.mounted) return;

    final newMessages = response.messages
        .where((msg) => !_messageIds.contains(msg.messageId))
        .map((msg) {
          final sender = msg.hasUserMessage()
              ? msg.userMessage.sender
              : (msg.hasSystemMessage() && msg.systemMessage.hasActor()
                  ? msg.systemMessage.actor
                  : User());

          return ChatMessage(
            message: msg,
            sender: sender,
            state: MessageState.delivered,
            isTemporary: false,
          );
        })
        .toList();

    if (newMessages.isNotEmpty) {
      for (final msg in newMessages) {
        _messageIds.add(msg.messageId);
      }

      if (!ref.mounted) return;
      state = state.copyWith(
        messages: _sortByChronology([...state.messages, ...newMessages]),
      );
      _log.info(
        '✅ Merged ${newMessages.length} new messages from server history',
      );
    }
  }

  /// refreshMessagesAfterMutation re-fetches conversation history and merges
  /// new messages using message IDs to avoid duplicates.
  ///
  /// Called after mutations that generate system messages.
  Future<void> refreshMessagesAfterMutation() async {
    final conversationId = state.conversation?.conversationId;
    if (conversationId == null) return;

    try {
      await _mergeServerHistory(conversationId);
    } catch (e) {
      _log.warning('⚠️  Failed to refresh messages after mutation: $e');
    }
  }

  /// refreshAfterResume cancels the current stream subscription, merges any
  /// messages received during the gap, marks them read, and re-subscribes.
  ///
  /// Called when the app returns to the foreground while this conversation is
  /// active. Uses the non-destructive merge helper so pending optimistic
  /// messages survive the refresh.
  Future<void> refreshAfterResume() async {
    final conversationId = state.conversation?.conversationId;
    if (conversationId == null) return;

    _log.info('🔄 refreshAfterResume: starting for $conversationId');

    unawaited(_messageStreamSubscription?.cancel());
    _messageStreamSubscription = null;

    try {
      await _mergeServerHistory(conversationId);
    } catch (e) {
      _log.warning('⚠️  refreshAfterResume: history merge failed: $e');
    }

    if (!ref.mounted) return;

    final unreadCount = state.unreadCount;
    if (unreadCount > 0) {
      try {
        final chatRepository = ref.read(chatRepositoryProvider);
        await chatRepository.markMessagesRead(
          conversationId: conversationId,
        );
        if (!ref.mounted) return;
        _onMessagesMarkedAsRead?.call(conversationId);
      } catch (e) {
        _log.warning('⚠️  refreshAfterResume: markMessagesRead failed: $e');
      }
    }

    if (!ref.mounted) return;

    _reconnectAttempts = 0;
    if (state.isStreamDisconnected) {
      state = state.copyWith(isStreamDisconnected: false);
    }

    startMessageStream();
    _log.info('✅ refreshAfterResume: complete for $conversationId');
  }

  /// _sortByChronology returns a new list sorted by (sentAtUnixSec, messageId)
  /// ascending so display order always matches conversation order. Returns a
  /// copy and never mutates the input.
  List<ChatMessage> _sortByChronology(List<ChatMessage> msgs) {
    final sorted = [...msgs];
    sorted.sort((a, b) {
      final cmp = a.sentAtUnixSec.compareTo(b.sentAtUnixSec);
      if (cmp != 0) return cmp;
      return a.messageId.compareTo(b.messageId);
    });
    return sorted;
  }

  // ── Cross-mixin hooks ─────────────────────────────────────────────────────
  // These abstract methods are provided by other mixins or the coordinator.
  // Non-private names are required so they can be overridden across files.

  /// cachedImpact returns the non-Freezed impact estimate set by an action
  /// mixin after a terminal action (loan complete, request fulfilled,
  /// experience completed). Implemented by the coordinator.
  ImpactEstimate? get cachedImpact;

  void invalidateParentEntityCacheIfNeeded();
  void invalidateAndRefreshExperience();
  Future<void> fetchRequestDetails();
  Future<void> fetchRequestImpact();
  void refreshParticipantsFromSystemMessage();
  Future<void> fetchConversationContext();
}
