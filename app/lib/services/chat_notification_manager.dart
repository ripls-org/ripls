import 'dart:convert';

import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/services/conversation_shortcut_service.dart';
import 'package:shared_preferences/shared_preferences.dart';

final _log = ObservableLogger.named('ChatNotificationManager');

/// Renders chat-message push notifications using Android `MessagingStyle` and
/// iOS thread grouping, with a per-conversation message cache so multiple
/// messages from the same conversation appear as one updating notification.
///
/// Designed to work from both the main isolate (foreground FCM path) and the
/// Android background isolate (data-only FCM delivery). Constructor takes
/// plugin instances directly; no Riverpod / no `BuildContext` needed.
///
/// See `docs/issues/1892-chat-notification-batching.md` for the broader design.
class ChatNotificationManager {
  /// Notification channel ID used on Android for chat messages. Must match the
  /// channel registered at app startup (see `FCMService._initializeLocalNotifications`).
  static const String channelId = 'chats';

  /// Group key applied to every chat notification so the Android shade groups
  /// them under one app-level summary card.
  static const String groupKey = 'chats';

  /// SharedPreferences key prefix for the per-conversation recent-messages
  /// cache. The full key is `chat_notif_cache:<conversation_id>`.
  static const String cacheKeyPrefix = 'chat_notif_cache:';

  /// SharedPreferences keys for localized strings the manager needs to render.
  /// Seeded from the main isolate at app startup via [persistLocalizedStrings];
  /// the manager reads them from any isolate without needing l10n context.
  static const String _stringSelfNameKey = 'chat_notif_strings:self_name';
  static const String _stringChannelNameKey = 'chat_notif_strings:channel_name';
  static const String _stringChannelDescriptionKey =
      'chat_notif_strings:channel_description';

  /// Maximum number of cached messages per conversation. Picked to fill the
  /// Android conversation card without being noisy on the lock screen.
  static const int maxCachedMessagesPerConversation = 6;

  /// Cached messages older than this are dropped on the next render so a
  /// single new message tomorrow doesn't re-surface yesterday's transcript.
  static const Duration cacheTtl = Duration(minutes: 60);

  final FlutterLocalNotificationsPlugin _localNotifications;
  final ConversationShortcutService _shortcutService;
  final SharedPreferencesAsync? _injectedPrefs;
  SharedPreferencesAsync? _lazyPrefs;

  ChatNotificationManager({
    required FlutterLocalNotificationsPlugin localNotifications,
    SharedPreferencesAsync? prefs,
    ConversationShortcutService? shortcutService,
  })  : _localNotifications = localNotifications,
        _shortcutService = shortcutService ?? ConversationShortcutService(),
        _injectedPrefs = prefs;

  /// Constructs [SharedPreferencesAsync] lazily on first use so the manager
  /// (and the Riverpod provider that builds it) doesn't crash at construction
  /// time in environments where the platform interface hasn't been registered
  /// — most notably unit tests that don't exercise the notification path.
  SharedPreferencesAsync get _prefs =>
      _injectedPrefs ?? (_lazyPrefs ??= SharedPreferencesAsync());

  /// Seeds the localized strings used by the manager into SharedPreferences.
  ///
  /// Call once from the main isolate after `AppLocalizations` is available.
  /// The renderer reads from SharedPreferences so it works from the background
  /// isolate too, where l10n context isn't available.
  static Future<void> persistLocalizedStrings({
    required String selfName,
    required String chatsChannelName,
    required String chatsChannelDescription,
    SharedPreferencesAsync? prefs,
  }) async {
    final p = prefs ?? SharedPreferencesAsync();
    await p.setString(_stringSelfNameKey, selfName);
    await p.setString(_stringChannelNameKey, chatsChannelName);
    await p.setString(_stringChannelDescriptionKey, chatsChannelDescription);
  }

  /// Reads the localized channel name + description from SharedPreferences,
  /// falling back to English defaults if [persistLocalizedStrings] hasn't run
  /// yet. Used at app startup when registering the Android notification
  /// channel — the renderer itself also reads these values per push.
  static Future<ChatNotificationChannelStrings> readChannelStrings({
    SharedPreferencesAsync? prefs,
  }) async {
    final p = prefs ?? SharedPreferencesAsync();
    final name = (await p.getString(_stringChannelNameKey)) ?? 'Chats';
    final description = (await p.getString(_stringChannelDescriptionKey)) ??
        'Messages from your communities';
    return ChatNotificationChannelStrings(
      name: name,
      description: description,
    );
  }

  /// Appends [data] (an FCM data payload with `type=chat_message`) to the
  /// per-conversation cache and posts / updates the notification.
  ///
  /// The notification ID is `conversationId.hashCode`, stable per conversation,
  /// so subsequent calls replace the prior notification in place.
  Future<void> renderForChatMessage(Map<String, dynamic> data) async {
    final conversationId = (data['conversation_id'] as String?) ?? '';
    if (conversationId.isEmpty) {
      _log.warning('renderForChatMessage: missing conversation_id, skipping');
      return;
    }

    final senderName = (data['sender_name'] as String?) ?? '';
    final senderUserId = (data['sender_user_id'] as String?) ?? '';
    final body = (data['preview_text'] as String?) ?? '';
    final conversationTitle = (data['conversation_title'] as String?) ?? '';
    final now = DateTime.now();

    final cacheKey = '$cacheKeyPrefix$conversationId';
    final existing = await _loadCache(cacheKey, now);

    final entry = _CachedMessage(
      senderName: senderName,
      senderUserId: senderUserId,
      body: body,
      timestampMs: now.millisecondsSinceEpoch,
    );
    final updated = [...existing, entry];
    final trimmed = updated.length > maxCachedMessagesPerConversation
        ? updated.sublist(updated.length - maxCachedMessagesPerConversation)
        : updated;

    await _prefs.setString(
      cacheKey,
      jsonEncode({
        'messages': trimmed.map((m) => m.toJson()).toList(),
      }),
    );

    final selfName = (await _prefs.getString(_stringSelfNameKey)) ?? 'You';
    final channelName =
        (await _prefs.getString(_stringChannelNameKey)) ?? 'Chats';
    final channelDescription =
        (await _prefs.getString(_stringChannelDescriptionKey)) ??
            'Messages from your communities';

    final self = Person(name: selfName, key: '__self__');
    final messages = trimmed.map((m) {
      final person = Person(
        name: m.senderName.isEmpty ? null : m.senderName,
        key: m.senderUserId.isEmpty ? null : m.senderUserId,
      );
      return Message(
        m.body,
        DateTime.fromMillisecondsSinceEpoch(m.timestampMs),
        person,
      );
    }).toList();

    // When the server ships a `conversation_title` (gear name, event name,
    // request title, or community name), render in group-conversation mode:
    // the title becomes the notification heading and each cached message
    // keeps its per-sender Person prefix. When the title is absent fall back
    // to single-sender style so the sender's name isn't duplicated into
    // every message bubble.
    final MessagingStyleInformation styleInformation;
    if (conversationTitle.isNotEmpty) {
      styleInformation = MessagingStyleInformation(
        self,
        conversationTitle: conversationTitle,
        groupConversation: true,
        messages: messages,
      );
    } else {
      styleInformation = MessagingStyleInformation(
        self,
        groupConversation: false,
        messages: messages,
      );
    }

    // Push (or update) the long-lived dynamic shortcut that backs the
    // Conversation Space surface on Android 11+. Idempotent on
    // conversation_id; on iOS this is a no-op. Best-effort: a failure to
    // register the shortcut shouldn't block the notification itself.
    await _shortcutService.ensureShortcut(
      ConversationShortcutSpec(
        conversationId: conversationId,
        shortLabel: senderName.isEmpty ? conversationId : senderName,
        personName: senderName.isEmpty ? conversationId : senderName,
        personKey: senderUserId.isEmpty ? conversationId : senderUserId,
      ),
    );

    final androidDetails = AndroidNotificationDetails(
      channelId,
      channelName,
      channelDescription: channelDescription,
      importance: Importance.high,
      priority: Priority.high,
      styleInformation: styleInformation,
      groupKey: groupKey,
      tag: conversationId,
      category: AndroidNotificationCategory.message,
      shortcutId: conversationId,
    );

    final iosDetails = DarwinNotificationDetails(
      presentAlert: true,
      presentBadge: true,
      presentSound: true,
      threadIdentifier: conversationId,
    );

    final details = NotificationDetails(android: androidDetails, iOS: iosDetails);

    await _localNotifications.show(
      id: notificationIdFor(conversationId),
      title: senderName,
      body: body,
      notificationDetails: details,
      payload: jsonEncode(data),
    );

    _log.debug(
      'rendered chat notification: '
      'conversation=$conversationId message_count=${trimmed.length}',
    );
  }

  /// Cancels the notification for [conversationId], if any. Called by
  /// `ChatRepository.markMessagesRead` after the server-side mark-read succeeds.
  ///
  /// Best-effort: a `MissingPluginException` (e.g. in unit tests without the
  /// plugin mocked) or any other plugin error is logged at warn and swallowed.
  /// The user's mark-read intent has already succeeded on the server; failing
  /// loudly here would force every caller to wrap in try/catch.
  Future<void> clearForConversation(String conversationId) async {
    if (conversationId.isEmpty) return;
    try {
      await _localNotifications.cancel(id: notificationIdFor(conversationId));
      _log.debug(
        'cancelled chat notification for conversation=$conversationId',
      );
    } catch (e) {
      _log.warning(
        'failed to cancel chat notification for $conversationId: $e',
      );
    }
  }

  /// Stable per-conversation notification ID used on both platforms. Public so
  /// tests can assert exact-id replacement.
  static int notificationIdFor(String conversationId) =>
      conversationId.hashCode;

  Future<List<_CachedMessage>> _loadCache(String key, DateTime now) async {
    final raw = await _prefs.getString(key);
    if (raw == null || raw.isEmpty) return [];
    try {
      final decoded = jsonDecode(raw) as Map<String, dynamic>;
      final rawMessages = (decoded['messages'] as List?) ?? const [];
      final cutoffMs = now.subtract(cacheTtl).millisecondsSinceEpoch;
      return rawMessages
          .whereType<Map<String, dynamic>>()
          .map(_CachedMessage.fromJson)
          .where((m) => m.timestampMs >= cutoffMs)
          .toList();
    } catch (e) {
      _log.warning('failed to decode chat notif cache at $key: $e');
      return [];
    }
  }
}

/// User-visible Android channel strings seeded into SharedPreferences by the
/// main isolate so [FCMService] can register the channel during init without
/// touching `BuildContext`.
class ChatNotificationChannelStrings {
  const ChatNotificationChannelStrings({
    required this.name,
    required this.description,
  });

  final String name;
  final String description;
}

/// Internal representation of a cached message; persisted to SharedPreferences
/// so the background isolate can re-render the full transcript.
class _CachedMessage {
  _CachedMessage({
    required this.senderName,
    required this.senderUserId,
    required this.body,
    required this.timestampMs,
  });

  factory _CachedMessage.fromJson(Map<String, dynamic> json) => _CachedMessage(
        senderName: (json['senderName'] as String?) ?? '',
        senderUserId: (json['senderUserId'] as String?) ?? '',
        body: (json['body'] as String?) ?? '',
        timestampMs: (json['timestampMs'] as int?) ?? 0,
      );

  final String senderName;
  final String senderUserId;
  final String body;
  final int timestampMs;

  Map<String, dynamic> toJson() => {
        'senderName': senderName,
        'senderUserId': senderUserId,
        'body': body,
        'timestampMs': timestampMs,
      };
}
