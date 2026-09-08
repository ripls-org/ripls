import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/presentation/viewmodels/system_message_patch.dart';

void main() {
  MessageHistoryItem original() => MessageHistoryItem(
        messageId: 'm-1',
        sentAtUnixSec: Int64(100),
        systemMessage: SystemMessage(
          description: 'Ana is going',
          templateKey: 'chat.experience.rsvp_yes',
        )..templateParams['actorName'] = 'Ana',
      );

  group('patchedSystemMessage', () {
    test('carries the fresh template payload with the patch', () {
      final update = SystemMessageUpdate(
        messageId: 'm-1',
        description: 'Ana is not going',
        sentAtUnixSec: Int64(200),
        templateKey: 'chat.experience.rsvp_no',
      )..templateParams['actorName'] = 'Ana';

      final patched = patchedSystemMessage(original(), update);

      expect(patched.systemMessage.templateKey, 'chat.experience.rsvp_no');
      expect(patched.systemMessage.templateParams['actorName'], 'Ana');
      expect(patched.systemMessage.description, 'Ana is not going');
      expect(patched.sentAtUnixSec.toInt(), 200);
      // The original is untouched (patch is a copy).
      expect(original().systemMessage.templateKey, 'chat.experience.rsvp_yes');
    });

    test('clears a stale template key when the update carries none', () {
      // Without the clear, the resolver would keep rendering the
      // pre-update template while the description moved on.
      final update = SystemMessageUpdate(
        messageId: 'm-1',
        description: 'Ana changed their plans',
        sentAtUnixSec: Int64(200),
      );

      final patched = patchedSystemMessage(original(), update);

      expect(patched.systemMessage.hasTemplateKey(), isFalse);
      expect(patched.systemMessage.templateParams, isEmpty);
      expect(patched.systemMessage.description, 'Ana changed their plans');
    });
  });
}
