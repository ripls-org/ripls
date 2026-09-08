import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart' show ConversationItem;
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/unread_count_repository.dart';
import 'package:ripls/presentation/viewmodels/unread_count_view_model.dart'
    show unreadCountProvider;
import 'package:ripls/services/chat_notification_manager.dart';
import 'package:ripls/services/chat_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';

// Re-export unreadCountProvider for convenience
export 'package:ripls/presentation/viewmodels/unread_count_view_model.dart'
    show unreadCountProvider, UnreadCountState, UnreadCountNotifier;

/// Provider for ChatService
final chatServiceProvider = Provider<ChatService>((ref) {
  return ChatService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    errorHandler: ref.watch(rpcErrorHandlerProvider),
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for the chat-notification renderer used by [ChatRepository] for
/// cancel-on-read. Constructed independently of [FCMService] — both managers
/// own their own `FlutterLocalNotificationsPlugin` instance but invoke the
/// same underlying platform channel, so notifications posted by FCMService's
/// internal manager are cancellable by this one (and vice versa).
///
/// This independence keeps tests that override `chatRepositoryProvider` from
/// having to override `fcmServiceProvider` transitively.
final chatNotificationManagerProvider = Provider<ChatNotificationManager>((ref) {
  return ChatNotificationManager(
    localNotifications: FlutterLocalNotificationsPlugin(),
  );
});

/// Provider for ChatRepository
final chatRepositoryProvider = Provider<ChatRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(chatServiceProvider);
  final notificationManager = ref.watch(chatNotificationManagerProvider);

  void onDailyInvalidated() {
    ref.read(portfolioCacheInvalidationProvider.notifier).notify();
  }

  void onContentInvalidated() {
    ref.read(contentCacheInvalidationProvider.notifier).notify();
  }

  return ChatRepository(
    cache,
    service,
    onDailyInvalidated: onDailyInvalidated,
    onContentInvalidated: onContentInvalidated,
    chatNotificationManager: notificationManager,
  );
});

/// Provider for UnreadCountRepository
final unreadCountRepositoryProvider = Provider<UnreadCountRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(chatServiceProvider);
  return UnreadCountRepository(cache, service);
});

/// Provider for global total unread message count across all communities.
///
/// Watches the UnreadCountNotifier for reactive updates.
/// Use this for the community avatar badge (upper left).
final unreadMessageCountTotalProvider = Provider<int>((ref) {
  final unreadState = ref.watch(unreadCountProvider);
  return unreadState.totalUnreadCount;
});

/// Family provider for per-community unread message count.
///
/// Returns the unread count for a specific community.
/// Use this for sidebar community chips.
final unreadMessageCountForCommunityProvider = Provider.family<int, String>((ref, communityId) {
  final unreadState = ref.watch(unreadCountProvider);
  return unreadState.communityIdToUnreadCount[communityId] ?? 0;
});

/// Family provider for the community-wide ConversationItem.
///
/// Fetches the conversation metadata (including last message preview) for a
/// given community. Used by the inbox pinned community card to show a
/// consistent last-message preview and time ago.
final communityConversationProvider =
    FutureProvider.autoDispose.family<ConversationItem?, String>((ref, communityId) async {
  if (communityId.isEmpty) return null;
  try {
    final repo = ref.watch(chatRepositoryProvider);
    final result = await repo.getConversationForCommunity(communityId);
    return result;
  } catch (_) {
    return null;
  }
});

/// resetUnreadForConversation clears the unread count for a conversation in
/// the given community. Used by content-view chat panes' tab-pane
/// `onMessagesMarkedAsRead` callback (gear, experience, request).
void resetUnreadForConversation(
  WidgetRef ref,
  String conversationId, {
  String communityId = '',
}) {
  ref
      .read(unreadCountProvider.notifier)
      .resetUnreadCount(conversationId, communityId);
}
