import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/presentation/viewmodels/workshop_glimpse_view_model.dart';
import 'package:ripls/services/providers/chat_providers.dart';

/// Minimal ChatRepository test double — only the two reads the glimpse uses are
/// implemented; everything else routes through noSuchMethod and is unused.
class _FakeChatRepository implements ChatRepository {
  _FakeChatRepository({required this.conversation, required this.history});

  final ConversationItem conversation;
  final GetConversationHistoryResponse history;

  @override
  Future<ConversationItem> getConversationForCommunity(String communityId) async =>
      conversation;

  @override
  Future<GetConversationHistoryResponse> getConversationHistory({
    required String conversationId,
    int? maxMessages,
    int? beforeUnixSec,
  }) async =>
      history;

  @override
  dynamic noSuchMethod(Invocation invocation) =>
      super.noSuchMethod(invocation);
}

MessageHistoryItem _userMsg(String author, String text, int atSec) {
  return MessageHistoryItem()
    ..sentAtUnixSec = Int64(atSec)
    ..userMessage = (UserMessage()
      ..sender = (User()..name = author)
      ..text = text);
}

MessageHistoryItem _systemMsg(int atSec) {
  return MessageHistoryItem()
    ..sentAtUnixSec = Int64(atSec)
    ..systemMessage = SystemMessage();
}

ProviderContainer _containerWith(_FakeChatRepository repo) {
  return ProviderContainer(overrides: [
    chatRepositoryProvider.overrideWithValue(repo),
  ]);
}

void main() {
  test('maps the latest user messages newest-first, capped at 3', () async {
    final repo = _FakeChatRepository(
      conversation: ConversationItem()..conversationId = 'conv-1',
      history: GetConversationHistoryResponse()
        ..messages.addAll([
          _userMsg('Alfred', 'oldest', 100),
          _userMsg('Betty', 'second', 200),
          _userMsg('Tom', 'third', 300),
          _userMsg('Mara', 'newest', 400),
        ]),
    );
    final container = _containerWith(repo);
    addTearDown(container.dispose);

    final quotes =
        await container.read(workshopGlimpseProvider('comm-1').future);

    expect(quotes.map((q) => q.text), ['newest', 'third', 'second']);
    expect(quotes.first.author, 'Mara');
    expect(quotes.first.atUnixSec, 400);
  });

  test('skips system messages and blank text', () async {
    final repo = _FakeChatRepository(
      conversation: ConversationItem()..conversationId = 'conv-1',
      history: GetConversationHistoryResponse()
        ..messages.addAll([
          _userMsg('Alfred', 'real one', 100),
          _systemMsg(150),
          _userMsg('Betty', '   ', 200),
        ]),
    );
    final container = _containerWith(repo);
    addTearDown(container.dispose);

    final quotes =
        await container.read(workshopGlimpseProvider('comm-1').future);

    expect(quotes, hasLength(1));
    expect(quotes.single.text, 'real one');
  });

  test('returns empty when the community has no conversation yet', () async {
    final repo = _FakeChatRepository(
      conversation: ConversationItem(), // empty conversationId
      history: GetConversationHistoryResponse(),
    );
    final container = _containerWith(repo);
    addTearDown(container.dispose);

    final quotes =
        await container.read(workshopGlimpseProvider('comm-1').future);

    expect(quotes, isEmpty);
  });
}
