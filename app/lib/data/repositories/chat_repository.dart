import 'package:flutter/foundation.dart' show VoidCallback;
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/conversation.pb.dart';
import 'package:ripls/data/gen/ripls/api/conversation_topic.pb.dart';
import 'package:ripls/services/chat_notification_manager.dart';
import 'package:ripls/services/chat_service.dart';

/// Repository for chat data with transparent caching.
///
/// This repository wraps ChatService and provides caching for conversations
/// and messages using the global TTL from environment (CACHE_TTL_MINUTES).
class ChatRepository {
  final CacheManager _cache;
  final ChatService _service;
  final ChatNotificationManager? _chatNotificationManager;
  final String _namespace = 'chat';
  final VoidCallback? _onDailyInvalidated;

  ChatRepository(
    this._cache,
    this._service, {
    VoidCallback? onDailyInvalidated,
    // onContentInvalidated intentionally removed — chat mutations
    // (send message, mark read) do not change content state. Firing
    // content invalidation caused parent content views to reload gear/
    // experience/request details, which disposed the autoDispose
    // conversationProvider and re-fetched all messages (flicker).
    VoidCallback? onContentInvalidated,
    ChatNotificationManager? chatNotificationManager,
  })  : _onDailyInvalidated = onDailyInvalidated,
        _chatNotificationManager = chatNotificationManager;

  /// Lists conversations for the current user.
  ///
  /// If [archived] is true, returns archived conversations (done items with no
  /// unread messages). If false (default), returns active conversations.
  ///
  /// Use [refreshConversations] to force a refresh.
  Future<List<ConversationItem>> listConversations({
    bool archived = false,
  }) async {
    final cacheKey = archived
        ? '$_namespace:conversations:archived'
        : '$_namespace:conversations:active';
    return _cache.get(
      key: cacheKey,
      fetch: () => _service.listConversations(archived: archived),
    );
  }

  /// Lists conversations for a specific community.
  ///
  /// Filters active conversations by community ID with client-side filtering.
  /// Results are cached per community for performance.
  ///
  /// Use [refreshConversationsByCommunity] to force a refresh.
  Future<List<ConversationItem>> listConversationsByCommunity(
    String communityId, {
    bool archived = false,
  }) async {
    final archiveSuffix = archived ? ':archived' : '';
    return _cache.get(
      key: '$_namespace:conversations:community:$communityId$archiveSuffix',
      fetch: () async {
        final allConversations = await _service.listConversations(archived: archived);
        return allConversations
            .where((c) => c.communityId == communityId)
            .toList();
      },
    );
  }

  /// Gets a specific conversation by ID with caching.
  ///
  /// Use [refreshConversation] to force a refresh.
  Future<ConversationItem> getConversation({
    required String conversationId,
  }) async {
    return _cache.get(
      key: '$_namespace:conversation:$conversationId',
      fetch: () => _service.getConversation(conversationId: conversationId),
    );
  }

  /// Gets conversation context for UI display (topic title, image, message count).
  ///
  /// This provides rich context about what the conversation is about,
  /// including the topic type, title, subtitle, and thumbnail image.
  /// Use this for conversation headers and inbox item display.
  ///
  /// Use [refreshConversationContext] to force a refresh.
  Future<ConversationContext> getConversationContext({
    required String conversationId,
  }) async {
    return _cache.get(
      key: '$_namespace:conversation:$conversationId:context',
      fetch: () => _service.getConversationContext(conversationId: conversationId),
    );
  }

  /// Refreshes conversation context by invalidating its cache.
  Future<void> refreshConversationContext(String conversationId) async {
    await _cache.remove('$_namespace:conversation:$conversationId:context');
  }

  /// Refreshes the conversations list by invalidating both active and archived caches.
  ///
  /// Call this after starting a new conversation, marking messages as read,
  /// or when you want fresh data. Both caches are cleared since marking messages
  /// as read can move conversations between active and archived views.
  Future<void> refreshConversations() async {
    await _cache.remove('$_namespace:conversations:active');
    await _cache.remove('$_namespace:conversations:archived');
  }

  /// Refreshes conversations for a specific community.
  ///
  /// Clears both active and archived caches for this community.
  Future<void> refreshConversationsByCommunity(String communityId) async {
    await _cache.remove('$_namespace:conversations:community:$communityId');
    await _cache.remove('$_namespace:conversations:community:$communityId:archived');
  }

  /// Refreshes a specific conversation by invalidating its cache.
  ///
  /// Call this when you want to ensure the conversation data is up-to-date.
  Future<void> refreshConversation(String conversationId) async {
    await _cache.remove('$_namespace:conversation:$conversationId');
  }

  /// Gets the community-wide conversation for a community.
  ///
  /// The server creates the conversation if it does not yet exist.
  /// Use [refreshConversationForCommunity] to force a refresh.
  Future<ConversationItem> getConversationForCommunity(String communityId) async {
    return _cache.get(
      key: '$_namespace:conversation:community:$communityId',
      fetch: () => _service.getConversationForCommunity(communityId: communityId),
    );
  }

  /// Refreshes the community-wide conversation cache for the given community.
  Future<void> refreshConversationForCommunity(String communityId) async {
    await _cache.remove('$_namespace:conversation:community:$communityId');
  }

  /// startGearConversation creates or retrieves the perpetual gear conversation.
  ///
  /// This is the recovery path for gear that missed the share-time conversation
  /// creation. The server is idempotent: calling this on gear that already has
  /// a conversation_id simply returns the existing id.
  ///
  /// **Cache invalidation note:** Unlike ordinary chat mutations (sendMessage,
  /// markMessagesRead), this call changes the gear's conversation_id field,
  /// which the parent GearContentView reads directly. The viewmodel — not this
  /// repository — is responsible for calling gearRepository.invalidate and
  /// reloading gear details after this call succeeds. This preserves the
  /// flicker-prevention invariant (no contentCacheInvalidationProvider fire
  /// from the chat repo) while still propagating the new conversation_id.
  Future<String> startGearConversation({
    required String gearId,
    required String communityId,
  }) async {
    final response = await _service.startConversation(
      communityId: communityId,
      topic: ConversationTopic(gearId: gearId),
    );
    // Clear the conversation namespace defensively so any stale cached entry
    // for this gear's conversation is evicted on next fetch.
    await _cache.clear(pattern: '$_namespace:conversation:*');
    return response.conversationId;
  }

  /// Gets conversation history for a specific conversation.
  ///
  /// Use [refreshConversationHistory] to force a refresh.
  Future<GetConversationHistoryResponse> getConversationHistory({
    required String conversationId,
    int? maxMessages,
    int? beforeUnixSec,
  }) async {
    // Include pagination params in cache key
    final cacheKeySuffix = _buildHistoryKeySuffix(maxMessages, beforeUnixSec);
    return _cache.get(
      key: '$_namespace:conversation:$conversationId:history$cacheKeySuffix',
      fetch: () => _service.getConversationHistory(
        conversationId: conversationId,
        maxMessages: maxMessages,
        beforeUnixSec: beforeUnixSec,
      ),
    );
  }

  /// Refreshes conversation history by invalidating the cache.
  ///
  /// Call this after sending a new message or when you want fresh messages.
  Future<void> refreshConversationHistory(String conversationId) async {
    // Clear all history variants for this conversation
    await _cache.clear(pattern: '$_namespace:conversation:$conversationId:history*');
  }

  /// Sends a message in a conversation.
  ///
  /// After sending, invalidates the conversation history cache to ensure
  /// fresh data on next fetch.
  Future<SendMessageResponse> sendMessage({
    required String conversationId,
    required String text,
    List<String>? mediaIds,
    String? replyToMessageId,
  }) async {
    final response = await _service.sendMessage(
      conversationId: conversationId,
      text: text,
      mediaIds: mediaIds ?? [],
      replyToMessageId: replyToMessageId,
    );

    // Invalidate chat caches after mutation. Do NOT call
    // _onContentInvalidated here — sending a message does not change
    // gear/experience/request content. Triggering a content reload
    // causes the parent content view to rebuild, which disposes the
    // autoDispose conversationProvider and re-fetches all messages
    // from the server (visible as a full-screen flicker).
    await refreshConversationHistory(conversationId);
    await refreshConversations(); // May update unread counts
    _onDailyInvalidated?.call();

    return response;
  }

  /// Marks messages in a conversation as read.
  ///
  /// After marking as read, invalidates relevant caches and clears any
  /// pending chat notification for [conversationId]. The notification cancel
  /// runs only on RPC success so a failed mark-read doesn't silently drop the
  /// user's lock-screen reminder.
  Future<int> markMessagesRead({
    required String conversationId,
    int? upToUnixSec,
  }) async {
    final count = await _service.markMessagesRead(
      conversationId: conversationId,
      upToUnixSec: upToUnixSec,
    );

    // Invalidate chat caches only — marking messages read does not
    // change content state. See sendMessage comment above.
    await refreshConversations(); // Updates unread counts
    _onDailyInvalidated?.call();

    // Fan out: dismiss the per-conversation notification once the read state
    // is durable on the server. Architecture: the ViewModel calls only the
    // Repository, and the Repository — not the ViewModel — is the single
    // chokepoint that calls the notification Service. Manager is optional so
    // tests that don't care about notifications can omit it.
    await _chatNotificationManager?.clearForConversation(conversationId);

    return count;
  }

  /// Gets the conversation for a specific transfer.
  Future<ConversationItem> getConversationForTransfer({
    required String transferId,
  }) async {
    return _cache.get(
      key: '$_namespace:transfer:$transferId:conversation',
      fetch: () => _service.getConversationForTransfer(transferId: transferId),
    );
  }

  /// Streams messages for a conversation in real-time.
  ///
  /// This is a direct pass-through to the service since streams cannot
  /// be cached effectively. Use this for real-time message updates.
  Stream<StreamMessagesResponse> streamMessages({
    required String conversationId,
  }) {
    return _service.streamMessages(conversationId: conversationId);
  }

  /// Updates user presence status (foreground/background).
  ///
  /// This is a direct pass-through to the service with no caching.
  Future<void> updatePresence({
    required bool isInForeground,
  }) async {
    return _service.updatePresence(isInForeground: isInForeground);
  }

  /// Adds an emoji reaction to a message.
  ///
  /// Invalidates the conversation history cache so subsequent fetches reflect
  /// the updated reactions.
  Future<AddReactionResponse> addReaction({
    required String conversationId,
    required String messageId,
    required String emoji,
  }) async {
    final response = await _service.addReaction(
      conversationId: conversationId,
      messageId: messageId,
      emoji: emoji,
    );
    await invalidateConversation(conversationId);
    return response;
  }

  /// Removes the current user's reaction from a message.
  ///
  /// Invalidates the conversation history cache so subsequent fetches reflect
  /// the updated reactions.
  Future<RemoveReactionResponse> removeReaction({
    required String conversationId,
    required String messageId,
  }) async {
    final response = await _service.removeReaction(
      conversationId: conversationId,
      messageId: messageId,
    );
    await invalidateConversation(conversationId);
    return response;
  }

  /// Edits the text of a message the caller authored.
  ///
  /// Invalidates the conversation history cache and signals the daily screen so
  /// the edited text (and last-message preview) refresh on next fetch.
  Future<EditMessageResponse> editMessage({
    required String conversationId,
    required String messageId,
    required String text,
  }) async {
    final response = await _service.editMessage(
      conversationId: conversationId,
      messageId: messageId,
      text: text,
    );
    await invalidateConversation(conversationId);
    await refreshConversations();
    _onDailyInvalidated?.call();
    return response;
  }

  /// Deletes a message the caller authored.
  ///
  /// Invalidates the conversation history cache and signals the daily screen so
  /// the removal (and last-message preview) refresh on next fetch.
  Future<void> deleteMessage({
    required String conversationId,
    required String messageId,
  }) async {
    await _service.deleteMessage(
      conversationId: conversationId,
      messageId: messageId,
    );
    await invalidateConversation(conversationId);
    await refreshConversations();
    _onDailyInvalidated?.call();
  }

  /// Invalidates a specific conversation's cache.
  Future<void> invalidateConversation(String conversationId) async {
    await _cache.clear(pattern: '$_namespace:conversation:$conversationId:*');
  }

  /// Invalidates all cached chat data.
  Future<void> invalidateAll() async {
    await _cache.clear(pattern: '$_namespace:*');
  }

  /// Builds a cache key suffix for conversation history pagination.
  String _buildHistoryKeySuffix(int? maxMessages, int? beforeUnixSec) {
    if (maxMessages == null && beforeUnixSec == null) {
      return '';
    }
    final parts = <String>[];
    if (maxMessages != null) parts.add('max=$maxMessages');
    if (beforeUnixSec != null) parts.add('before=$beforeUnixSec');
    return ':${parts.join(':')}';
  }
}
