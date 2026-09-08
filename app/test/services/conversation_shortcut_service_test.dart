import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/services/conversation_shortcut_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late MethodChannel channel;
  late List<MethodCall> calls;

  setUp(() {
    channel = const MethodChannel(ConversationShortcutService.channelName);
    calls = [];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
      calls.add(call);
      return null;
    });
  });

  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, null);
  });

  // The plugin guards Platform.isAndroid before making any platform call.
  // In the test environment defaultTargetPlatform is android by default so
  // the calls hit the channel; on iOS/desktop they would no-op.

  group('ensureShortcut', () {
    test('invokes pushDynamicShortcut with all required fields', () async {
      final service = ConversationShortcutService(channel: channel);
      await service.ensureShortcut(
        const ConversationShortcutSpec(
          conversationId: 'conv-1',
          shortLabel: 'Alice',
          personName: 'Alice',
          personKey: 'user-a',
        ),
      );

      expect(calls.length, 1);
      expect(calls.first.method, 'pushDynamicShortcut');
      final args = calls.first.arguments as Map<dynamic, dynamic>;
      expect(args['conversation_id'], 'conv-1');
      expect(args['short_label'], 'Alice');
      expect(args['long_label'], 'Alice');
      expect(args['person_name'], 'Alice');
      expect(args['person_key'], 'user-a');
      expect(args.containsKey('person_uri'), isFalse);
      expect(args.containsKey('person_icon_bytes'), isFalse);
    });

    test('passes person_uri and icon bytes when supplied', () async {
      final iconBytes = Uint8List.fromList([1, 2, 3]);
      final service = ConversationShortcutService(channel: channel);
      await service.ensureShortcut(
        ConversationShortcutSpec(
          conversationId: 'conv-2',
          shortLabel: 'Bob',
          personName: 'Bob',
          personUri: 'mailto:bob@example.com',
          personIconBytes: iconBytes,
        ),
      );

      final args = calls.single.arguments as Map<dynamic, dynamic>;
      expect(args['person_uri'], 'mailto:bob@example.com');
      expect(args['person_icon_bytes'], iconBytes);
    });

    test('skips when conversationId is empty', () async {
      final service = ConversationShortcutService(channel: channel);
      await service.ensureShortcut(
        const ConversationShortcutSpec(
          conversationId: '',
          shortLabel: 'Alice',
          personName: 'Alice',
        ),
      );
      expect(calls, isEmpty);
    });

    test('swallows platform errors so caller is not broken', () async {
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
        throw PlatformException(code: 'boom', message: 'native failure');
      });
      final service = ConversationShortcutService(channel: channel);
      // Should NOT throw.
      await service.ensureShortcut(
        const ConversationShortcutSpec(
          conversationId: 'conv-1',
          shortLabel: 'Alice',
          personName: 'Alice',
        ),
      );
    });
  });

  group('removeShortcut', () {
    test('invokes removeShortcut with conversation_id', () async {
      final service = ConversationShortcutService(channel: channel);
      await service.removeShortcut('conv-9');

      expect(calls.length, 1);
      expect(calls.first.method, 'removeShortcut');
      final args = calls.first.arguments as Map<dynamic, dynamic>;
      expect(args['conversation_id'], 'conv-9');
    });

    test('skips when conversationId is empty', () async {
      final service = ConversationShortcutService(channel: channel);
      await service.removeShortcut('');
      expect(calls, isEmpty);
    });

    test('swallows platform errors', () async {
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
        throw PlatformException(code: 'boom');
      });
      final service = ConversationShortcutService(channel: channel);
      await service.removeShortcut('conv-9'); // must not throw
    });
  });
}
