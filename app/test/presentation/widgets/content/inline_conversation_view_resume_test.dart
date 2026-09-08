import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/conversation.pb.dart'
    show ConversationContext;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/unread_count_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/conversation_view_model.dart';
import 'package:ripls/presentation/widgets/content/inline_conversation_view.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/chat_service.dart';
import 'package:ripls/services/providers.dart';

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

/// Minimal stub for ChatService that returns empty/valid data for all methods
/// needed by InlineConversationView._initialize().
class _MinimalChatService implements ChatService {
  @override
  Future<ConversationItem> getConversation({
    required String conversationId,
  }) async {
    return ConversationItem(
      conversationId: conversationId,
      participants: [User(id: 'user-1', name: 'Test User')],
    );
  }

  @override
  Future<ConversationContext> getConversationContext({
    required String conversationId,
  }) async {
    return ConversationContext();
  }

  @override
  Future<GetConversationHistoryResponse> getConversationHistory({
    required String conversationId,
    int? maxMessages,
    int? beforeUnixSec,
  }) async {
    return GetConversationHistoryResponse(messages: []);
  }

  @override
  Stream<StreamMessagesResponse> streamMessages({
    required String conversationId,
  }) {
    return const Stream.empty();
  }

  @override
  Future<int> markMessagesRead({
    required String conversationId,
    int? upToUnixSec,
  }) async {
    return 0;
  }

  @override
  Future<GetUnreadCountsResponse> getUnreadCounts() async {
    return GetUnreadCountsResponse();
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

/// Stub that bypasses AuthStateNotifier initialization and returns a fixed user.
class _FakeAuthStateNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() {
    return const AuthStateData(
      isLoading: false,
      accessToken: 'test-token',
      user: null,
    );
  }
}

/// ConversationNotifier subclass that tracks refreshAfterResume() calls.
class _TrackingConversationNotifier extends ConversationNotifier {
  _TrackingConversationNotifier(super.conversationId);

  int refreshAfterResumeCallCount = 0;

  @override
  Future<void> initialize({
    required String currentUserId,
    required String currentUserName,
    required conversation,
    unreadCount = 0,
    conversationContext,
    onMessagesMarkedAsRead,
  }) async {
    // no-op — avoids repository dependencies in widget tests.
  }

  @override
  Future<void> refreshAfterResume() async {
    refreshAfterResumeCallCount++;
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

Widget _buildTestApp({
  required String conversationId,
  required _TrackingConversationNotifier notifier,
  required bool isActive,
}) {
  final chatService = _MinimalChatService();
  late final StashCacheManager cacheManager;

  return ProviderScope(
    overrides: [
      authStateProvider.overrideWith(() => _FakeAuthStateNotifier()),
      chatServiceProvider.overrideWith((_) => chatService),
      chatRepositoryProvider.overrideWith((ref) {
        cacheManager = StashCacheManager();
        return ChatRepository(cacheManager, chatService);
      }),
      unreadCountRepositoryProvider.overrideWith((ref) {
        return UnreadCountRepository(StashCacheManager(), chatService);
      }),
      conversationProvider(conversationId).overrideWith(
        () => notifier,
      ),
    ],
    child: MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(
        body: InlineConversationView(
          conversationId: conversationId,
          accentColor: Colors.blue,
          isActive: isActive,
        ),
      ),
    ),
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const conversationId = 'conv-test-resume';

  group('InlineConversationView lifecycle', () {
    testWidgets(
      'calls refreshAfterResume when app resumes with isActive: true',
      (tester) async {
        final notifier = _TrackingConversationNotifier(conversationId);

        await tester.pumpWidget(
          _buildTestApp(
            conversationId: conversationId,
            notifier: notifier,
            isActive: true,
          ),
        );

        // Allow _initialize() to run via addPostFrameCallback.
        await tester.pump();
        await tester.pump(const Duration(milliseconds: 100));

        // _initialized is now true. Simulate the app returning to foreground.
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.resumed,
        );
        await tester.pump();

        expect(
          notifier.refreshAfterResumeCallCount,
          1,
          reason: 'refreshAfterResume must be called when app resumes '
              'with isActive: true and after initialization',
        );
      },
    );

    testWidgets(
      'does NOT call refreshAfterResume when isActive: false',
      (tester) async {
        final notifier = _TrackingConversationNotifier(conversationId);

        await tester.pumpWidget(
          _buildTestApp(
            conversationId: conversationId,
            notifier: notifier,
            isActive: false,
          ),
        );

        await tester.pump();
        await tester.pump(const Duration(milliseconds: 100));

        // isActive is false — lifecycle callback must be a no-op.
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.resumed,
        );
        await tester.pump();

        expect(
          notifier.refreshAfterResumeCallCount,
          0,
          reason: 'refreshAfterResume must NOT be called when isActive: false',
        );
      },
    );
  });
}
