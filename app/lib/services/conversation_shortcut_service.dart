import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:ripls/core/observability/logging/logger.dart';

final _log = ObservableLogger.named('ConversationShortcutService');

/// Dart-side wrapper for the native `ripls/conversation_shortcuts` platform
/// channel. Publishes long-lived per-conversation shortcuts so Android can
/// promote chat notifications into the Conversation Space, surface them in
/// bubbles, and rank them in system Sharesheet pickers.
///
/// All methods are no-ops on iOS. Android-side errors are caught and logged
/// at warn; this is a polish-layer side effect and should not break callers.
class ConversationShortcutService {
  ConversationShortcutService({MethodChannel? channel})
      : _channel = channel ?? const MethodChannel(channelName);

  static const String channelName = 'ripls/conversation_shortcuts';

  final MethodChannel _channel;

  /// Pushes (creates or updates) a dynamic shortcut for [spec]. Idempotent on
  /// `spec.conversationId`. Native side handles LRU eviction when the system
  /// shortcut cap is reached.
  Future<void> ensureShortcut(ConversationShortcutSpec spec) async {
    if (defaultTargetPlatform != TargetPlatform.android) return;
    if (spec.conversationId.isEmpty) {
      _log.warning('ensureShortcut: empty conversation_id, skipping');
      return;
    }
    try {
      await _channel.invokeMethod<void>('pushDynamicShortcut', {
        'conversation_id': spec.conversationId,
        'short_label': spec.shortLabel,
        'long_label': spec.longLabel ?? spec.shortLabel,
        'person_name': spec.personName,
        'person_key': spec.personKey ?? spec.conversationId,
        if (spec.personUri != null) 'person_uri': spec.personUri,
        if (spec.personIconBytes != null)
          'person_icon_bytes': spec.personIconBytes,
      });
    } catch (e) {
      _log.warning(
        'ensureShortcut failed for ${spec.conversationId}: $e',
      );
    }
  }

  /// Removes the shortcut for [conversationId]. Called when a conversation is
  /// left or deleted client-side.
  Future<void> removeShortcut(String conversationId) async {
    if (defaultTargetPlatform != TargetPlatform.android) return;
    if (conversationId.isEmpty) return;
    try {
      await _channel.invokeMethod<void>('removeShortcut', {
        'conversation_id': conversationId,
      });
    } catch (e) {
      _log.warning('removeShortcut failed for $conversationId: $e');
    }
  }
}

/// Parameters for [ConversationShortcutService.ensureShortcut]. Kept as a
/// value class so future fields (icon URI, group flag, etc.) can extend the
/// API without breaking call sites.
class ConversationShortcutSpec {
  const ConversationShortcutSpec({
    required this.conversationId,
    required this.shortLabel,
    required this.personName,
    this.longLabel,
    this.personKey,
    this.personUri,
    this.personIconBytes,
  });

  final String conversationId;
  final String shortLabel;
  final String personName;
  final String? longLabel;
  final String? personKey;
  final String? personUri;
  final Uint8List? personIconBytes;
}
