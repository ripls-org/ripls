import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' as api;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/web/web_community_screen.dart';
import 'package:ripls/presentation/viewmodels/event_invite_join_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers/auth_providers.dart';

/// `_FakeAuthStateNotifier` overrides the auth state without spinning up the
/// real Firebase + secure-storage machinery the production notifier needs.
class _FakeAuthStateNotifier extends AuthStateNotifier {
  _FakeAuthStateNotifier(this._initial);

  final AuthStateData _initial;

  @override
  AuthStateData build() => _initial;
}

/// `_StubJoinNotifier` stands in for the real join so these tests exercise the
/// screen's gating (joining → joined → content, or failed → retry) without an
/// RPC. The real notifier is covered by its own tests; what's under test here
/// is that the screen refuses to show community content until the join lands.
class _StubJoinNotifier extends EventInviteJoinNotifier {
  _StubJoinNotifier(super.shortCode, this._status);

  final EventInviteJoinStatus _status;

  @override
  EventInviteJoinState build() => EventInviteJoinState(status: _status);

  @override
  Future<void> join() async {}

  @override
  Future<void> retry() async {}
}

AuthStateData _authenticated() => AuthStateData.empty.copyWith(
  accessToken: 'token',
  user: api.User(id: 'user-1', name: 'Guest'),
);

Widget _harness({
  required Widget child,
  required AuthStateData auth,
  EventInviteJoinStatus? joinStatus,
  String shortCode = 'TESTCODE',
}) {
  return ProviderScope(
    overrides: [
      authStateProvider.overrideWith(() => _FakeAuthStateNotifier(auth)),
      if (joinStatus != null)
        eventInviteJoinProvider(
          shortCode,
        ).overrideWith(() => _StubJoinNotifier(shortCode, joinStatus)),
    ],
    child: MaterialApp(
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: child,
    ),
  );
}

void main() {
  group('WebCommunityScreen — unauthenticated guest', () {
    testWidgets('with a join intent, routes to the phone-first screen', (
      tester,
    ) async {
      // The whole point of #2875: a guest who taps the SSR landing's CTA goes
      // straight to phone verification, carrying the community id, instead of
      // being shown an install-the-app page.
      final router = GoRouter(
        initialLocation: '/group/comm-1?intent=join&code=TESTCODE',
        routes: [
          GoRoute(
            path: '/group/:communityId',
            builder: (context, state) => WebCommunityScreen(
              communityId: state.pathParameters['communityId']!,
              intent: state.uri.queryParameters['intent'],
              shortCode: state.uri.queryParameters['code'],
            ),
          ),
          GoRoute(
            path: '/verify-phone',
            builder: (context, state) => Text(
              'PHONE_FIRST_STUB:${state.uri.queryParameters['community_id']}',
            ),
          ),
        ],
      );

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            authStateProvider.overrideWith(
              () => _FakeAuthStateNotifier(AuthStateData.empty),
            ),
          ],
          child: MaterialApp.router(
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            routerConfig: router,
          ),
        ),
      );
      await tester.pumpAndSettle();

      // The community id rides along so the post-verify redirect can bring the
      // guest back here — without it they'd land on `/` having joined nothing.
      expect(find.text('PHONE_FIRST_STUB:comm-1'), findsOneWidget);
    });

    testWidgets('without a join intent, falls back to sign-in', (tester) async {
      // Someone who typed the bare URL has no invite context to honor, so the
      // screen preserves the destination and sends them to login.
      final router = GoRouter(
        initialLocation: '/group/comm-1',
        routes: [
          GoRoute(
            path: '/group/:communityId',
            builder: (context, state) => WebCommunityScreen(
              communityId: state.pathParameters['communityId']!,
            ),
          ),
          GoRoute(
            path: '/login',
            builder: (context, state) => const Text('LOGIN_STUB'),
          ),
        ],
      );

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            authStateProvider.overrideWith(
              () => _FakeAuthStateNotifier(AuthStateData.empty),
            ),
          ],
          child: MaterialApp.router(
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            routerConfig: router,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('LOGIN_STUB'), findsOneWidget);
    });
  });

  group('WebCommunityScreen — authenticated visitor', () {
    testWidgets('shows the joining status while the join is in flight', (
      tester,
    ) async {
      await tester.pumpWidget(
        _harness(
          auth: _authenticated(),
          joinStatus: EventInviteJoinStatus.running,
          child: const WebCommunityScreen(
            communityId: 'comm-1',
            intent: 'join',
            shortCode: 'TESTCODE',
          ),
        ),
      );
      await tester.pump();

      expect(find.text('Joining community…'), findsOneWidget);
    });

    testWidgets('surfaces a retry affordance when the join fails', (
      tester,
    ) async {
      await tester.pumpWidget(
        _harness(
          auth: _authenticated(),
          joinStatus: EventInviteJoinStatus.failed,
          child: const WebCommunityScreen(
            communityId: 'comm-1',
            intent: 'join',
            shortCode: 'TESTCODE',
          ),
        ),
      );
      await tester.pump();

      expect(
        find.text("We couldn't add you to this community. Tap to try again."),
        findsOneWidget,
      );
      // A failed join must not fall through to community content the
      // access gate would reject anyway.
      expect(find.text('Joining community…'), findsNothing);
    });

    testWidgets('does not render community content before the join succeeds', (
      tester,
    ) async {
      await tester.pumpWidget(
        _harness(
          auth: _authenticated(),
          joinStatus: EventInviteJoinStatus.running,
          child: const WebCommunityScreen(
            communityId: 'comm-1',
            intent: 'join',
            shortCode: 'TESTCODE',
          ),
        ),
      );
      await tester.pump();

      // Probing by name keeps this independent of CommunityPublicScreen's
      // internals — the invariant is only that it isn't mounted yet.
      expect(
        find.byWidgetPredicate(
          (w) => w.runtimeType.toString() == 'CommunityPublicScreen',
        ),
        findsNothing,
      );
    });

    testWidgets('threads the discuss tab through the phone-first detour', (
      tester,
    ) async {
      // A "say hi" link that reaches an unauthenticated reader must still land
      // them in the discussion after they verify — dropping the tab here is a
      // silent downgrade to the community's front page (#2876).
      final router = GoRouter(
        initialLocation: '/group/comm-1?intent=join&code=TESTCODE&tab=discuss',
        routes: [
          GoRoute(
            path: '/group/:communityId',
            builder: (context, state) => WebCommunityScreen(
              communityId: state.pathParameters['communityId']!,
              intent: state.uri.queryParameters['intent'],
              shortCode: state.uri.queryParameters['code'],
              tab: state.uri.queryParameters['tab'],
            ),
          ),
          GoRoute(
            path: '/verify-phone',
            builder: (context, state) =>
                Text('PHONE_FIRST_STUB:${state.uri.queryParameters['tab']}'),
          ),
        ],
      );

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            authStateProvider.overrideWith(
              () => _FakeAuthStateNotifier(AuthStateData.empty),
            ),
          ],
          child: MaterialApp.router(
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            routerConfig: router,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('PHONE_FIRST_STUB:discuss'), findsOneWidget);
    });

    testWidgets('carries the e2e semantics identifier', (tester) async {
      // The Playwright loop locates this screen through the flt-semantics DOM
      // tree; losing the identifier silently breaks that spec's locators.
      await tester.pumpWidget(
        _harness(
          auth: _authenticated(),
          joinStatus: EventInviteJoinStatus.running,
          child: const WebCommunityScreen(
            communityId: 'comm-1',
            intent: 'join',
            shortCode: 'TESTCODE',
          ),
        ),
      );
      await tester.pump();

      expect(
        find.byWidgetPredicate(
          (w) => w is Semantics && w.properties.identifier == 'web-community-screen',
        ),
        findsOneWidget,
      );
    });
  });
}
