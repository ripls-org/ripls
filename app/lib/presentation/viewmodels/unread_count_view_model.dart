import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/services/providers.dart';

part 'unread_count_view_model.freezed.dart';

final _log = Logger('UnreadCountViewModel');

/// State for unread message counts.
///
/// Provides reactive state for unread counts that can be watched from
/// multiple screens (home screen badge, inbox screen, conversation screen).
@freezed
sealed class UnreadCountState with _$UnreadCountState {
  const factory UnreadCountState({
    @Default(0) int totalUnreadCount,
    @Default({}) Map<String, int> conversationIdToUnreadCount,
    @Default({}) Map<String, int> communityIdToUnreadCount,
    @Default(true) bool isLoading,
  }) = _UnreadCountState;
}

/// Notifier for managing unread message counts.
///
/// Encapsulates unread count business logic and provides reactive state.
/// This notifier coordinates with UnreadCountRepository for data access
/// and provides optimistic updates for real-time message streams.
class UnreadCountNotifier extends Notifier<UnreadCountState> {
  @override
  UnreadCountState build() {
    return const UnreadCountState();
  }

  /// Initializes unread counts from repository.
  ///
  /// Call on app startup to ensure the badge is available immediately.
  Future<void> initialize() async {
    _log.info('📥 Initializing unread counts');
    state = state.copyWith(isLoading: true);

    try {
      final repository = ref.read(unreadCountRepositoryProvider);
      final total = await repository.getTotalUnreadCount();
      final counts = await repository.getUnreadCounts();
      final communityCounts = await repository.getUnreadCountsByCommunity();

      state = state.copyWith(
        totalUnreadCount: total,
        conversationIdToUnreadCount: counts,
        communityIdToUnreadCount: communityCounts,
        isLoading: false,
      );

      _log.info('✅ Initialized unread counts: total=$total, communities=${communityCounts.length}');
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to initialize unread counts: $e', e, stackTrace);
      state = state.copyWith(isLoading: false);
    }
  }

  /// Increments unread count for a conversation.
  ///
  /// Called from message stream when a new message arrives.
  Future<void> incrementUnreadCount(String conversationId, String communityId) async {
    final repository = ref.read(unreadCountRepositoryProvider);
    repository.incrementUnreadCount(communityId, conversationId);

    // Sync state from repository (which includes the new optimistic delta)
    await syncWithConversations();

    _log.info(
      '📈 Incremented unread count for $conversationId in $communityId, total=${state.totalUnreadCount}',
    );
  }

  /// Resets unread count for a conversation to zero.
  ///
  /// Called when marking messages as read.
  Future<void> resetUnreadCount(String conversationId, String communityId) async {
    final repository = ref.read(unreadCountRepositoryProvider);
    await repository.resetUnreadCount(communityId, conversationId);

    // Sync state from repository (which includes the reset optimistic delta)
    await syncWithConversations();

    _log.info(
      '📉 Reset unread count for $conversationId in $communityId, total=${state.totalUnreadCount}',
    );
  }

  /// Refreshes unread counts from server.
  ///
  /// Invalidates cache and refetches fresh data.
  Future<void> refresh() async {
    _log.info('🔄 Refreshing unread counts');
    final repository = ref.read(unreadCountRepositoryProvider);
    await repository.invalidate();
    await initialize();
  }

  /// Syncs with server data after conversation list is fetched.
  ///
  /// Call this after fetching fresh conversation data to reconcile
  /// optimistic deltas with server counts.
  Future<void> syncWithConversations() async {
    final repository = ref.read(unreadCountRepositoryProvider);
    final total = await repository.getTotalUnreadCount();
    final counts = await repository.getUnreadCounts();
    final communityCounts = await repository.getUnreadCountsByCommunity();

    state = state.copyWith(
      totalUnreadCount: total,
      conversationIdToUnreadCount: counts,
      communityIdToUnreadCount: communityCounts,
    );
  }
}

/// Provider for unread count state.
///
/// Watch this provider from any screen to get reactive unread count updates.
final unreadCountProvider =
    NotifierProvider<UnreadCountNotifier, UnreadCountState>(
  UnreadCountNotifier.new,
);
