import 'dart:async';

import 'package:cross_file/cross_file.dart';
import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' show Experience;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearResponse;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/data/repositories/experience_repository.dart'
    show ExperienceRepository, GetExperienceResponse;
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/viewmodels/conversation_view_model.dart';
import 'package:ripls/services/chat_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

// ── Fakes ──────────────────────────────────────────────────────────────────

class _FakeChatService implements ChatService {
  GetConversationHistoryResponse historyResponse =
      GetConversationHistoryResponse(messages: []);
  // Support replacing historyResponse mid-test for merge scenarios.
  GetConversationHistoryResponse? secondHistoryResponse;
  int _getHistoryCallCount = 0;

  // False positive: the suite's tearDown closes this through the
  // [streamController] getter, which the lint cannot follow out of the class.
  // ignore: close_sinks
  final StreamController<StreamMessagesResponse> _streamController =
      StreamController<StreamMessagesResponse>.broadcast();

  StreamController<StreamMessagesResponse> get streamController =>
      _streamController;

  @override
  Future<GetConversationHistoryResponse> getConversationHistory({
    required String conversationId,
    int? maxMessages,
    int? beforeUnixSec,
  }) async {
    _getHistoryCallCount++;
    if (secondHistoryResponse != null && _getHistoryCallCount > 1) {
      return secondHistoryResponse!;
    }
    return historyResponse;
  }

  @override
  Stream<StreamMessagesResponse> streamMessages({
    required String conversationId,
  }) =>
      _streamController.stream;

  @override
  Future<int> markMessagesRead({
    required String conversationId,
    int? upToUnixSec,
  }) async =>
      0;

  @override
  Future<SendMessageResponse> sendMessage({
    required String conversationId,
    required String text,
    List<String> mediaIds = const [],
    String? replyToMessageId,
  }) async =>
      SendMessageResponse();

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeTransferRepository implements TransferRepository {
  @override
  Future<Transfer?> getTransfer(String transferId) async => null;

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeUserRepository implements UserRepository {
  @override
  Future<UserProfile> getUserProfile(String userId) async =>
      UserProfile(user: GetUserResponse(userId: userId, name: 'User'), mediaUrl: null);

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeGearRepository implements GearRepository {
  @override
  Future<GetGearResponse> getGearDetails(String id, {String? communityId}) async =>
      GetGearResponse(id: id);

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeMediaRepository implements MediaRepository {
  @override
  Future<MediaUrl> getMediaUrl(String mediaId) async =>
      MediaUrl(mediaId: mediaId, url: 'https://example.com/$mediaId', isThumbnail: false);

  @override
  Future<String> addMedia({
    required XFile file,
    String? description,
  }) async =>
      'media-id';

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeExperienceRepository implements ExperienceRepository {
  @override
  Future<void> invalidate(String experienceId, {String? communityId}) async {}

  @override
  Future<GetExperienceResponse> getExperienceDetails(
    String experienceId, {
    String? communityId,
  }) async =>
      GetExperienceResponse(experience: Experience(id: experienceId));

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

// ── Helpers ────────────────────────────────────────────────────────────────

MessageHistoryItem _msg(String id, int ts, {String text = ''}) =>
    MessageHistoryItem(
      messageId: id,
      sentAtUnixSec: Int64(ts),
      isRead: false,
    )..userMessage = UserMessage(
        sender: User(id: 'sender', name: 'Sender'),
        text: text.isEmpty ? 'msg $id' : text,
      );

StreamMessagesResponse _streamMsg(String id, int ts, {String text = ''}) =>
    StreamMessagesResponse(
      messageId: id,
      conversationId: 'conv-order',
      sentAtUnixSec: Int64(ts),
    )..userMessage = UserMessage(
        sender: User(id: 'sender', name: 'Sender'),
        text: text.isEmpty ? 'msg $id' : text,
      );

// ── Test setup ─────────────────────────────────────────────────────────────

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const currentUserId = 'user-order-123';
  const conversationId = 'conv-order';

  late _FakeChatService fakeChat;

  setUp(() {
    SharedPreferences.setMockInitialValues({
      'access_token': 'test-token',
      'user_id': currentUserId,
      'user_name': 'Order Tester',
    });
    fakeChat = _FakeChatService();
  });

  tearDown(() {
    fakeChat.streamController.close();
  });

  ProviderContainer makeContainer() {
    return ProviderContainer(
      overrides: [
        chatServiceProvider.overrideWithValue(fakeChat),
        transferRepositoryProvider.overrideWithValue(_FakeTransferRepository()),
        userRepositoryProvider.overrideWithValue(_FakeUserRepository()),
        gearRepositoryProvider.overrideWithValue(_FakeGearRepository()),
        mediaRepositoryProvider.overrideWithValue(_FakeMediaRepository()),
        experienceRepositoryProvider.overrideWithValue(_FakeExperienceRepository()),
      ],
    );
  }

  ConversationItem makeConversation() => ConversationItem(
        conversationId: conversationId,
        participants: [User(id: currentUserId, name: 'Order Tester')],
      );

  Future<void> initContainer(ProviderContainer container) =>
      container.read(conversationProvider(conversationId).notifier).initialize(
            currentUserId: currentUserId,
            currentUserName: 'Order Tester',
            conversation: makeConversation(),
          );

  // ── Tests ────────────────────────────────────────────────────────────────

  test('loadMessages sorts deliberately-shuffled history into chronological order by sentAtUnixSec', () async {
    // Server returns messages in reverse order (newest first, as GetConversationHistory does).
    // The viewmodel reverses them on load — then _sortByChronology ensures the final
    // order is strictly ascending by sentAtUnixSec even if they arrive shuffled.
    fakeChat.historyResponse = GetConversationHistoryResponse(messages: [
      _msg('id-t30', 1030), // newest first (server pagination order)
      _msg('id-t15', 1015),
      _msg('id-t0', 1000),  // oldest last
    ]);

    final container = makeContainer();
    addTearDown(container.dispose);
    await initContainer(container);

    final msgs = container.read(conversationProvider(conversationId)).messages;
    expect(msgs.length, 3);
    expect(msgs[0].messageId, 'id-t0',  reason: 'oldest first');
    expect(msgs[1].messageId, 'id-t15');
    expect(msgs[2].messageId, 'id-t30', reason: 'newest last');
  });

  test('streamed messages arriving out of order are reordered (t+30 then t+10 renders t+10, t+30)', () async {
    fakeChat.historyResponse = GetConversationHistoryResponse(messages: []);

    final container = makeContainer();
    addTearDown(container.dispose);
    await initContainer(container);

    // Keep provider alive.
    final sub = container.listen(conversationProvider(conversationId), (_, _) {});
    addTearDown(sub.close);

    // Deliver t+30 first, then t+10 — out of order.
    fakeChat.streamController.add(_streamMsg('id-t30', 1030));
    await Future.delayed(const Duration(milliseconds: 50));
    fakeChat.streamController.add(_streamMsg('id-t10', 1010));
    await Future.delayed(const Duration(milliseconds: 50));

    final msgs = container.read(conversationProvider(conversationId)).messages;
    expect(msgs.length, 2);
    expect(msgs[0].messageId, 'id-t10', reason: 'earlier message first');
    expect(msgs[1].messageId, 'id-t30', reason: 'later message second');
  });

  test('_mergeServerHistory after a pull-to-refresh re-sorts existing local state, not just the newly-merged tail', () async {
    // Initial load: one message at t=1000.
    fakeChat.historyResponse = GetConversationHistoryResponse(messages: [
      _msg('id-t0', 1000),
    ]);

    final container = makeContainer();
    addTearDown(container.dispose);
    await initContainer(container);

    // Check initial state: one message.
    expect(container.read(conversationProvider(conversationId)).messages.length, 1);

    // Stream delivers a message with an earlier timestamp (t-10). Without re-sort
    // on _mergeServerHistory this would stay at the bottom.
    final sub = container.listen(conversationProvider(conversationId), (_, _) {});
    addTearDown(sub.close);

    fakeChat.streamController.add(_streamMsg('id-t-10', 990));
    await Future.delayed(const Duration(milliseconds: 50));

    final msgs = container.read(conversationProvider(conversationId)).messages;
    expect(msgs.length, 2);
    expect(msgs[0].messageId, 'id-t-10',
        reason: 'earlier-timestamp message sorted before later one');
    expect(msgs[1].messageId, 'id-t0');
  });

  test('optimistic message stays anchored at the bottom (sentAtUnixSec = now() is largest) and is replaced cleanly when server echo arrives', () async {
    // One old message already loaded.
    fakeChat.historyResponse = GetConversationHistoryResponse(messages: [
      _msg('id-old', 1000),
    ]);

    final container = makeContainer();
    addTearDown(container.dispose);
    await initContainer(container);

    final sub = container.listen(conversationProvider(conversationId), (_, _) {});
    addTearDown(sub.close);

    // Send a message — creates an optimistic entry with sentAtUnixSec = now(),
    // which is larger than 1000 (an epoch-second timestamp far in the past).
    await container
        .read(conversationProvider(conversationId).notifier)
        .sendMessage('hello optimistic');
    await Future.delayed(const Duration(milliseconds: 50));

    // Optimistic message is at the bottom.
    var msgs = container.read(conversationProvider(conversationId)).messages;
    expect(msgs.length, 2);
    expect(msgs[1].isTemporary, isTrue, reason: 'optimistic at bottom');

    // Server echo arrives — replaces the optimistic entry.
    final now = DateTime.now().millisecondsSinceEpoch ~/ 1000;
    fakeChat.streamController.add(
      StreamMessagesResponse(
        messageId: 'id-server',
        conversationId: conversationId,
        sentAtUnixSec: Int64(now),
      )..userMessage = UserMessage(
          sender: User(id: currentUserId, name: 'Order Tester'),
          text: 'hello optimistic',
        ),
    );
    await Future.delayed(const Duration(milliseconds: 50));

    msgs = container.read(conversationProvider(conversationId)).messages;
    expect(msgs.length, 2,
        reason: 'optimistic replaced by server echo, no duplicate');
    expect(msgs.any((m) => m.isTemporary), isFalse,
        reason: 'no temporary messages after echo');
    expect(msgs[1].messageId, 'id-server',
        reason: 'server message at bottom');
  });

  test('a server message with a future sentAtUnixSec sorts after a now-stamped server message (#1920 regression guard)', () async {
    // History contains a message stamped 7 days in the future
    // (simulator bug, clock skew, or any other producer that didn't
    // clamp sent_at_unix_sec to wall-clock now). The client display
    // layer faithfully sorts by sentAtUnixSec — it does not clamp.
    // The fix for #1920 is in the producer, not here; this test
    // documents that contract and guards against a well-meaning
    // future change that would mask producer bugs at the display
    // layer by collapsing future timestamps to "now" for sort.
    final nowSec = DateTime.now().millisecondsSinceEpoch ~/ 1000;
    final futureSec = nowSec + 7 * 24 * 60 * 60;
    fakeChat.historyResponse = GetConversationHistoryResponse(messages: [
      _msg('id-future', futureSec),
    ]);

    final container = makeContainer();
    addTearDown(container.dispose);
    await initContainer(container);

    final sub = container.listen(conversationProvider(conversationId), (_, _) {});
    addTearDown(sub.close);

    // A new message arrives via stream at "now". _handleStreamedMessage
    // re-sorts via _sortByChronology, so id-now lands ahead of id-future.
    fakeChat.streamController.add(_streamMsg('id-now', nowSec));
    await Future.delayed(const Duration(milliseconds: 50));

    final msgs = container.read(conversationProvider(conversationId)).messages;
    expect(msgs.length, 2);
    expect(msgs[0].messageId, 'id-now',
        reason: 'now-stamped message sorts before the +7d future one');
    expect(msgs[1].messageId, 'id-future',
        reason: 'future-stamped message stays at the bottom — display layer never clamps');
  });
}
