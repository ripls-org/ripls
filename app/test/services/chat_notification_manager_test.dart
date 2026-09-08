import 'dart:convert';

import 'package:flutter/services.dart';
import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/services/chat_notification_manager.dart';
import 'package:ripls/services/conversation_shortcut_service.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:shared_preferences_platform_interface/in_memory_shared_preferences_async.dart';
import 'package:shared_preferences_platform_interface/shared_preferences_async_platform_interface.dart';

import 'chat_notification_manager_test.mocks.dart';

@GenerateMocks([FlutterLocalNotificationsPlugin])
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late MockFlutterLocalNotificationsPlugin mockPlugin;
  late SharedPreferencesAsync prefs;
  late ChatNotificationManager manager;
  late MethodChannel shortcutChannel;
  late List<MethodCall> shortcutCalls;

  setUp(() {
    SharedPreferencesAsyncPlatform.instance =
        InMemorySharedPreferencesAsync.empty();
    mockPlugin = MockFlutterLocalNotificationsPlugin();
    prefs = SharedPreferencesAsync();
    shortcutChannel =
        const MethodChannel(ConversationShortcutService.channelName);
    shortcutCalls = [];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(shortcutChannel, (call) async {
      shortcutCalls.add(call);
      return null;
    });
    manager = ChatNotificationManager(
      localNotifications: mockPlugin,
      prefs: prefs,
    );
  });

  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(shortcutChannel, null);
  });

  group('renderForChatMessage', () {
    test('skips render when conversation_id is missing', () async {
      await manager.renderForChatMessage({
        'sender_name': 'Alice',
        'preview_text': 'Hello',
      });
      verifyNever(mockPlugin.show(
        id: anyNamed('id'),
        title: anyNamed('title'),
        body: anyNamed('body'),
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      ));
    });

    test('skips render when conversation_id is empty string', () async {
      await manager.renderForChatMessage({
        'conversation_id': '',
        'sender_name': 'Alice',
      });
      verifyNever(mockPlugin.show(
        id: anyNamed('id'),
        title: anyNamed('title'),
        body: anyNamed('body'),
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      ));
    });

    test('posts notification with id = conversationId.hashCode', () async {
      await manager.renderForChatMessage({
        'conversation_id': 'conv-1',
        'sender_name': 'Alice',
        'sender_user_id': 'user-a',
        'preview_text': 'Hello',
      });

      verify(mockPlugin.show(
        id: 'conv-1'.hashCode,
        title: 'Alice',
        body: 'Hello',
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      )).called(1);
    });

    test('subsequent messages reuse the same notification id (replacement)',
        () async {
      await manager.renderForChatMessage({
        'conversation_id': 'conv-1',
        'sender_name': 'Alice',
        'preview_text': 'Hello',
      });
      await manager.renderForChatMessage({
        'conversation_id': 'conv-1',
        'sender_name': 'Alice',
        'preview_text': 'Are you there?',
      });

      // Both show calls use the same id.
      verify(mockPlugin.show(
        id: 'conv-1'.hashCode,
        title: anyNamed('title'),
        body: anyNamed('body'),
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      )).called(2);
    });

    test('appends to cache and trims past maxCachedMessagesPerConversation',
        () async {
      const conv = 'conv-1';
      for (var i = 0; i < 8; i++) {
        await manager.renderForChatMessage({
          'conversation_id': conv,
          'sender_name': 'Sender $i',
          'sender_user_id': 'user-$i',
          'preview_text': 'msg $i',
        });
      }

      final raw = await prefs.getString('${ChatNotificationManager.cacheKeyPrefix}$conv');
      expect(raw, isNotNull);
      final decoded = jsonDecode(raw!) as Map<String, dynamic>;
      final messages = decoded['messages'] as List;
      expect(messages.length, ChatNotificationManager.maxCachedMessagesPerConversation);
      expect((messages.first as Map)['body'], 'msg 2');
      expect((messages.last as Map)['body'], 'msg 7');
    });

    test('drops cached messages older than cacheTtl on next render', () async {
      const conv = 'conv-1';
      final stalePayload = {
        'messages': [
          {
            'senderName': 'Old Alice',
            'senderUserId': 'user-a',
            'body': 'ancient',
            'timestampMs': DateTime.now()
                .subtract(const Duration(hours: 2))
                .millisecondsSinceEpoch,
          },
          {
            'senderName': 'Recent Alice',
            'senderUserId': 'user-a',
            'body': 'recent',
            'timestampMs': DateTime.now()
                .subtract(const Duration(minutes: 5))
                .millisecondsSinceEpoch,
          },
        ],
      };
      await prefs.setString(
        '${ChatNotificationManager.cacheKeyPrefix}$conv',
        jsonEncode(stalePayload),
      );

      await manager.renderForChatMessage({
        'conversation_id': conv,
        'sender_name': 'Alice',
        'preview_text': 'fresh',
      });

      final raw = await prefs.getString('${ChatNotificationManager.cacheKeyPrefix}$conv');
      final decoded = jsonDecode(raw!) as Map<String, dynamic>;
      final bodies = (decoded['messages'] as List)
          .map((m) => (m as Map)['body'] as String)
          .toList();
      expect(bodies, isNot(contains('ancient')));
      expect(bodies, contains('recent'));
      expect(bodies, contains('fresh'));
    });

    test('separate conversations cache and post independently', () async {
      await manager.renderForChatMessage({
        'conversation_id': 'conv-a',
        'sender_name': 'Alice',
        'preview_text': 'hi from a',
      });
      await manager.renderForChatMessage({
        'conversation_id': 'conv-b',
        'sender_name': 'Bob',
        'preview_text': 'hi from b',
      });

      final cacheA =
          await prefs.getString('${ChatNotificationManager.cacheKeyPrefix}conv-a');
      final cacheB =
          await prefs.getString('${ChatNotificationManager.cacheKeyPrefix}conv-b');
      expect(cacheA, contains('hi from a'));
      expect(cacheA, isNot(contains('hi from b')));
      expect(cacheB, contains('hi from b'));
      expect(cacheB, isNot(contains('hi from a')));

      verify(mockPlugin.show(
        id: 'conv-a'.hashCode,
        title: anyNamed('title'),
        body: anyNamed('body'),
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      )).called(1);
      verify(mockPlugin.show(
        id: 'conv-b'.hashCode,
        title: anyNamed('title'),
        body: anyNamed('body'),
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      )).called(1);
    });

    test(
        'corrupt cache JSON is recovered (treated as empty) so render still '
        'succeeds', () async {
      const conv = 'conv-1';
      await prefs.setString(
        '${ChatNotificationManager.cacheKeyPrefix}$conv',
        'not valid json {{{',
      );
      await manager.renderForChatMessage({
        'conversation_id': conv,
        'sender_name': 'Alice',
        'preview_text': 'hello',
      });

      verify(mockPlugin.show(
        id: 'conv-1'.hashCode,
        title: 'Alice',
        body: 'hello',
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      )).called(1);
      // Cache was rewritten with just the new message.
      final raw = await prefs.getString('${ChatNotificationManager.cacheKeyPrefix}$conv');
      expect(raw, contains('hello'));
    });

    test('reads localized strings from SharedPreferences when present',
        () async {
      await ChatNotificationManager.persistLocalizedStrings(
        selfName: 'Tú',
        chatsChannelName: 'Chats',
        chatsChannelDescription: 'Mensajes de tus comunidades',
        prefs: prefs,
      );

      await manager.renderForChatMessage({
        'conversation_id': 'conv-1',
        'sender_name': 'Alice',
        'preview_text': 'Hola',
      });

      // We can't directly inspect MessagingStyleInformation through Mockito
      // verify, but the call must have succeeded. The localized strings
      // round-trip via readChannelStrings is asserted separately.
      verify(mockPlugin.show(
        id: 'conv-1'.hashCode,
        title: anyNamed('title'),
        body: anyNamed('body'),
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      )).called(1);
    });
  });

  group('clearForConversation', () {
    test('cancels by conversationId.hashCode', () async {
      await manager.clearForConversation('conv-42');
      verify(mockPlugin.cancel(id: 'conv-42'.hashCode)).called(1);
    });

    test('is a no-op when conversationId is empty', () async {
      await manager.clearForConversation('');
      verifyNever(mockPlugin.cancel(id: anyNamed('id')));
    });
  });

  group('localized channel strings', () {
    test('readChannelStrings returns English fallback when unseeded', () async {
      final strings = await ChatNotificationManager.readChannelStrings(
        prefs: prefs,
      );
      expect(strings.name, 'Chats');
      expect(strings.description, 'Messages from your communities');
    });

    test('readChannelStrings returns seeded values when persisted', () async {
      await ChatNotificationManager.persistLocalizedStrings(
        selfName: 'Tú',
        chatsChannelName: 'Chats',
        chatsChannelDescription: 'Mensajes de tus comunidades',
        prefs: prefs,
      );
      final strings = await ChatNotificationManager.readChannelStrings(
        prefs: prefs,
      );
      expect(strings.name, 'Chats');
      expect(strings.description, 'Mensajes de tus comunidades');
    });
  });

  group('Android Conversation Space shortcut integration', () {
    test('renderForChatMessage pushes a dynamic shortcut for the conversation',
        () async {
      await manager.renderForChatMessage({
        'conversation_id': 'conv-shortcut-1',
        'sender_name': 'Alice',
        'sender_user_id': 'user-a',
        'preview_text': 'Hello',
      });

      final pushCalls = shortcutCalls
          .where((c) => c.method == 'pushDynamicShortcut')
          .toList();
      expect(pushCalls.length, 1, reason: 'expected one shortcut push');
      final args = pushCalls.single.arguments as Map<dynamic, dynamic>;
      expect(args['conversation_id'], 'conv-shortcut-1');
      expect(args['short_label'], 'Alice');
      expect(args['person_name'], 'Alice');
      expect(args['person_key'], 'user-a');
    });

    test('subsequent renders re-push the shortcut (idempotent on conv_id)',
        () async {
      for (var i = 0; i < 3; i++) {
        await manager.renderForChatMessage({
          'conversation_id': 'conv-shortcut-2',
          'sender_name': 'Alice',
          'sender_user_id': 'user-a',
          'preview_text': 'msg $i',
        });
      }
      final pushCalls = shortcutCalls
          .where((c) => c.method == 'pushDynamicShortcut')
          .toList();
      expect(pushCalls.length, 3);
      for (final call in pushCalls) {
        final args = call.arguments as Map<dynamic, dynamic>;
        expect(args['conversation_id'], 'conv-shortcut-2');
      }
    });

    test('falls back to conversation_id when sender_name is empty', () async {
      await manager.renderForChatMessage({
        'conversation_id': 'conv-shortcut-3',
        'sender_name': '',
        'sender_user_id': 'user-c',
        'preview_text': 'hi',
      });
      final args = shortcutCalls.single.arguments as Map<dynamic, dynamic>;
      expect(args['short_label'], 'conv-shortcut-3');
      expect(args['person_name'], 'conv-shortcut-3');
    });

    test(
        'shortcut failure does not block the notification render — the user '
        'still sees the message', () async {
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(shortcutChannel, (call) async {
        throw PlatformException(code: 'boom');
      });

      await manager.renderForChatMessage({
        'conversation_id': 'conv-shortcut-4',
        'sender_name': 'Dave',
        'sender_user_id': 'user-d',
        'preview_text': 'still posts',
      });

      verify(mockPlugin.show(
        id: 'conv-shortcut-4'.hashCode,
        title: 'Dave',
        body: 'still posts',
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      )).called(1);
    });
  });

  group('background-isolate handoff', () {
    // The Android background isolate constructs its own
    // FlutterLocalNotificationsPlugin and ChatNotificationManager — there is
    // no shared in-memory state with the main-isolate manager. The only
    // cross-isolate state is SharedPreferences. These tests assert that a
    // fresh manager instance picks up the existing transcript and posts a
    // notification with the same stable id (so the system updates in place).

    test(
        'second manager (simulated background isolate) sees first manager\'s '
        'cache and posts with the same id', () async {
      // First manager (simulates foreground) appends a message.
      final firstPlugin = MockFlutterLocalNotificationsPlugin();
      final firstManager = ChatNotificationManager(
        localNotifications: firstPlugin,
        prefs: prefs,
      );
      await firstManager.renderForChatMessage({
        'conversation_id': 'conv-bg',
        'sender_name': 'Alice',
        'preview_text': 'first',
      });

      // Second manager (simulates background isolate) — fresh plugin, fresh
      // manager instance, but sharing the same SharedPreferences-backed
      // cache. It appends a second message.
      final secondPlugin = MockFlutterLocalNotificationsPlugin();
      final secondManager = ChatNotificationManager(
        localNotifications: secondPlugin,
        prefs: prefs,
      );
      await secondManager.renderForChatMessage({
        'conversation_id': 'conv-bg',
        'sender_name': 'Alice',
        'preview_text': 'second',
      });

      // Both manager instances posted with the same stable id — the system
      // NotificationManager replaces the prior notification in place.
      verify(firstPlugin.show(
        id: 'conv-bg'.hashCode,
        title: anyNamed('title'),
        body: anyNamed('body'),
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      )).called(1);
      verify(secondPlugin.show(
        id: 'conv-bg'.hashCode,
        title: anyNamed('title'),
        body: anyNamed('body'),
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      )).called(1);

      // The cache holds both messages — proves the background path picked
      // up state written by the foreground path.
      final raw = await prefs
          .getString('${ChatNotificationManager.cacheKeyPrefix}conv-bg');
      expect(raw, contains('first'));
      expect(raw, contains('second'));
    });

    test(
        'second manager renders correctly when cache is empty (fresh install '
        'background-only render)', () async {
      // No prior foreground render; only a single background render.
      final backgroundPlugin = MockFlutterLocalNotificationsPlugin();
      final backgroundManager = ChatNotificationManager(
        localNotifications: backgroundPlugin,
        prefs: prefs,
      );

      await backgroundManager.renderForChatMessage({
        'conversation_id': 'conv-bg-only',
        'sender_name': 'Bob',
        'preview_text': 'hello',
      });

      verify(backgroundPlugin.show(
        id: 'conv-bg-only'.hashCode,
        title: 'Bob',
        body: 'hello',
        notificationDetails: anyNamed('notificationDetails'),
        payload: anyNamed('payload'),
      )).called(1);
    });
  });

  group('notificationIdFor', () {
    test('is stable per conversationId', () {
      expect(
        ChatNotificationManager.notificationIdFor('conv-1'),
        ChatNotificationManager.notificationIdFor('conv-1'),
      );
    });

    test('differs for distinct conversationIds', () {
      expect(
        ChatNotificationManager.notificationIdFor('conv-1'),
        isNot(ChatNotificationManager.notificationIdFor('conv-2')),
      );
    });
  });
}
