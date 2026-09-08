import 'package:logging/logging.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/services/chat_service.dart';

final _log = Logger('UnreadCountRepository');

/// Repository for unread message counts with transparent caching.
///
/// This repository provides immediate access to unread message counts
/// without requiring the full inbox to be loaded. It can be initialized
/// on app startup to ensure the unread badge is available immediately.
///
/// Supports optimistic updates for real-time message streams and
/// mark-as-read operations.
class UnreadCountRepository {
  final CacheManager _cache;
  final ChatService _service;
  final String _namespace = 'unread';

  // In-memory optimistic deltas using composite keys (separate from cache)
  // Key format: 'community_id:conversation_id' -> delta
  // This is acceptable because it's in the repository layer, not ViewModel
  final Map<String, int> _optimisticDeltas = {};

  UnreadCountRepository(this._cache, this._service);

  /// Gets the total unread message count across all communities.
  ///
  /// Includes optimistic deltas from message streams and mark-as-read operations.
  Future<int> getTotalUnreadCount() async {
    final response = await _cache.get(
      key: '$_namespace:all',
      fetch: () => _service.getUnreadCounts(),
    );

    // Apply optimistic deltas
    final serverCount = response.totalUnreadCount;
    final totalDelta =
        _optimisticDeltas.values.fold<int>(0, (sum, d) => sum + d);
    final finalCount = (serverCount + totalDelta).clamp(0, 999999);

    // Log delta state for observability (only when deltas are active)
    if (_optimisticDeltas.isNotEmpty) {
      _log.fine(
        '📊 Total unread: server=$serverCount, delta=$totalDelta, final=$finalCount (${_optimisticDeltas.length} active deltas)',
      );
    }

    return finalCount;
  }

  /// Gets unread count for a specific conversation.
  ///
  /// Includes optimistic delta for real-time updates.
  Future<int> getUnreadCount(String communityId, String conversationId) async {
    final response = await _cache.get(
      key: '$_namespace:all',
      fetch: () => _service.getUnreadCounts(),
    );

    final serverCount = response.conversationIdToUnreadCount[conversationId] ?? 0;
    final compositeKey = '$communityId:$conversationId';
    final delta = _optimisticDeltas[compositeKey] ?? 0;
    return (serverCount + delta).clamp(0, 999);
  }

  /// Gets unread counts per conversation across all communities.
  ///
  /// Returns a map of conversation ID to unread count with optimistic deltas applied.
  /// Use [invalidate] to force a refresh.
  Future<Map<String, int>> getUnreadCounts() async {
    final response = await _cache.get(
      key: '$_namespace:all',
      fetch: () => _service.getUnreadCounts(),
    );

    // Start with server counts
    final counts = Map<String, int>.from(response.conversationIdToUnreadCount);

    // Apply optimistic deltas per conversation
    for (final entry in _optimisticDeltas.entries) {
      final parts = entry.key.split(':');
      if (parts.length == 2) {
        final conversationId = parts[1];
        final delta = entry.value;
        counts[conversationId] = ((counts[conversationId] ?? 0) + delta).clamp(0, 999);
      }
    }

    return counts;
  }

  /// Gets unread counts aggregated by community.
  ///
  /// Returns a map of community ID to unread count.
  /// Includes optimistic deltas from message streams.
  Future<Map<String, int>> getUnreadCountsByCommunity() async {
    final response = await _cache.get(
      key: '$_namespace:all',
      fetch: () => _service.getUnreadCounts(),
    );

    // Start with server counts
    final counts = Map<String, int>.from(response.communityIdToUnreadCount);

    // Apply optimistic deltas grouped by community
    for (final entry in _optimisticDeltas.entries) {
      final parts = entry.key.split(':');
      if (parts.length == 2) {
        final communityId = parts[0];
        final delta = entry.value;
        counts[communityId] = ((counts[communityId] ?? 0) + delta).clamp(0, 999999);
      }
    }

    return counts;
  }

  /// Gets unread count for a specific community.
  ///
  /// Includes optimistic deltas for real-time updates.
  Future<int> getUnreadCountForCommunity(String communityId) async {
    final counts = await getUnreadCountsByCommunity();
    return counts[communityId] ?? 0;
  }

  /// Increments unread count for a conversation (called from message stream).
  ///
  /// This is an optimistic update that will be reconciled on next server fetch.
  void incrementUnreadCount(String communityId, String conversationId) {
    final compositeKey = '$communityId:$conversationId';
    _optimisticDeltas[compositeKey] =
        (_optimisticDeltas[compositeKey] ?? 0) + 1;
    _log.info(
      '📈 Incremented unread delta for $compositeKey: ${_optimisticDeltas[compositeKey]}',
    );
  }

  /// Resets unread count for a conversation to zero (called when marking as read).
  ///
  /// This is an optimistic update that negates the server count.
  Future<void> resetUnreadCount(String communityId, String conversationId) async {
    final counts = await getUnreadCounts();
    final serverCount = counts[conversationId] ?? 0;
    final compositeKey = '$communityId:$conversationId';
    // Set delta to negate server count, bringing effective count to 0
    _optimisticDeltas[compositeKey] = -serverCount;
    _log.info(
      '📉 Reset unread delta for $compositeKey: ${_optimisticDeltas[compositeKey]} (server was $serverCount)',
    );
  }

  /// Invalidates all unread count caches and resets optimistic deltas.
  ///
  /// Call after marking messages as read or when inbox data changes.
  Future<void> invalidate() async {
    await _cache.clear(pattern: '$_namespace:*');
    _optimisticDeltas.clear();
    _log.info('🔄 Invalidated unread count cache and cleared optimistic deltas');
  }

  /// Invalidates cache for a specific community.
  ///
  /// Clears optimistic deltas for conversations in that community.
  Future<void> invalidateCommunity(String communityId) async {
    // Remove optimistic deltas for this community
    _optimisticDeltas.removeWhere((key, _) => key.startsWith('$communityId:'));

    // Clear the cache (we cache the full response, so need to clear all)
    await _cache.clear(pattern: '$_namespace:*');

    _log.info('🔄 Invalidated unread counts for community $communityId');
  }

  /// Syncs optimistic deltas with server counts.
  ///
  /// Call after fetching fresh conversation data to reset deltas
  /// for conversations whose server counts have changed.
  /// Clears deltas when server count is 0 to prevent double-counting.
  Future<void> syncWithServer(List<ConversationItem> conversations) async {
    for (final conversation in conversations) {
      final conversationId = conversation.conversationId;
      final communityId = conversation.communityId;
      final compositeKey = '$communityId:$conversationId';
      final newServerCount = conversation.unreadCount;

      // Always clear delta if server count is 0 (user has read all messages)
      // This prevents double-decrement bug when returning from a read conversation
      if (newServerCount == 0 && _optimisticDeltas.containsKey(compositeKey)) {
        _optimisticDeltas.remove(compositeKey);
        _log.info('🔄 Cleared delta for $compositeKey (server count is 0)');
      }
    }
  }

  /// Refreshes the total unread count by invalidating cache and refetching.
  ///
  /// Returns the fresh count.
  Future<int> refreshTotalUnreadCount() async {
    await invalidate();
    return getTotalUnreadCount();
  }
}
