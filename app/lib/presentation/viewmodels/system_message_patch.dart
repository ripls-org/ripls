import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart'
    show MessageHistoryItem, SystemMessageUpdate;

/// patchedSystemMessage returns a copy of [original] with [update] applied to
/// its system-message payload.
///
/// The structured template payload travels with the patch: a coalesced update
/// can change which template applies, and the stale key would otherwise keep
/// rendering the pre-update text. When the update carries no key, the key is
/// cleared so the resolver falls back to the fresh description.
MessageHistoryItem patchedSystemMessage(
  MessageHistoryItem original,
  SystemMessageUpdate update,
) {
  final patched = MessageHistoryItem()..mergeFromMessage(original);
  patched.systemMessage
    ..action = update.action
    ..description = update.description;
  if (update.hasTemplateKey() && update.templateKey.isNotEmpty) {
    patched.systemMessage
      ..templateKey = update.templateKey
      ..templateParams.clear()
      ..templateParams.addAll(update.templateParams);
  } else {
    patched.systemMessage
      ..clearTemplateKey()
      ..templateParams.clear();
  }
  if (update.hasActor()) {
    patched.systemMessage.actor = update.actor;
  }
  patched.sentAtUnixSec = update.sentAtUnixSec;
  return patched;
}
