import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/services/notification_route_replayer.dart';

/// Tests the catch + recover half of #2636: a resolved notification route is
/// held, navigated, verified, and replayed once if it doesn't stick.
void main() {
  /// Builds a router with `/` and `/experience/:id`. While [block] is true, any
  /// navigation to an experience route is redirected home — the stand-in for
  /// "something overrode the deep link" (auth churn / not-yet-settled router).
  GoRouter buildRouter({required bool Function() block}) {
    return GoRouter(
      initialLocation: '/',
      routes: [
        GoRoute(path: '/', builder: (_, _) => const Text('home')),
        GoRoute(
          path: '/experience/:id',
          builder: (_, _) => const Text('experience'),
        ),
      ],
      redirect: (context, state) {
        final loc = state.uri.toString();
        if (block() && loc.startsWith('/experience')) return '/';
        return null;
      },
    );
  }

  String location(GoRouter router) =>
      router.routerDelegate.currentConfiguration.uri.toString();

  group('PendingNotificationRouteHolder', () {
    setUp(() => PendingNotificationRouteHolder.value = null);

    test('consume returns and clears the pending route', () {
      PendingNotificationRouteHolder.value = const PendingNotificationRoute(
        route: '/experience/exp-1',
        source: 'initial',
      );

      final consumed = PendingNotificationRouteHolder.consume();

      expect(consumed?.route, '/experience/exp-1');
      expect(consumed?.source, 'initial');
      expect(PendingNotificationRouteHolder.value, isNull,
          reason: 'consume must clear the slot so it is not re-navigated');
    });

    test('consume on an empty slot returns null', () {
      expect(PendingNotificationRouteHolder.consume(), isNull);
    });
  });

  group('NotificationRouteReplayer', () {
    late List<NotificationDeepLinkEvent> events;
    late List<PendingNotificationRoute> drops;

    setUp(() {
      events = [];
      drops = [];
      PendingNotificationRouteHolder.value = null;
    });

    NotificationRouteReplayer replayerFor(GoRouter router) {
      return NotificationRouteReplayer(
        router: () => router,
        logAnalyticsEvent: (e) => events.add(e as NotificationDeepLinkEvent),
        onUnrecoverableDrop: (route, {String actualLocation = ''}) =>
            drops.add(route),
      );
    }

    testWidgets('drain with nothing pending is a no-op', (tester) async {
      final router = buildRouter(block: () => false);
      await tester.pumpWidget(MaterialApp.router(routerConfig: router));

      replayerFor(router).drain('ready');
      await tester.pump();

      expect(events, isEmpty);
      expect(location(router), '/');
    });

    testWidgets('navigates to the pending route and reports a clean landing',
        (tester) async {
      final router = buildRouter(block: () => false);
      await tester.pumpWidget(MaterialApp.router(routerConfig: router));

      PendingNotificationRouteHolder.value = const PendingNotificationRoute(
        route: '/experience/exp-1',
        communityId: 'comm-1',
        source: 'initial',
      );
      replayerFor(router).drain('ready');
      await tester.pumpAndSettle();

      expect(location(router), '/experience/exp-1');
      expect(events, hasLength(1));
      final p = events.single.parameters;
      expect(p['phase'], 'nav_result');
      expect(p['route'], '/experience/exp-1');
      expect(p['landed'], true);
      expect(p['trigger'], 'ready');
      expect(p.containsKey('recovered'), isFalse,
          reason: 'a first-attempt landing is not a recovery');
      expect(drops, isEmpty, reason: 'a clean landing is not a drop');
    });

    testWidgets('recovers via one replay when the first attempt is overridden',
        (tester) async {
      var block = true;
      final router = buildRouter(block: () => block);
      await tester.pumpWidget(MaterialApp.router(routerConfig: router));

      PendingNotificationRouteHolder.value = const PendingNotificationRoute(
        route: '/experience/exp-1',
        source: 'opened',
      );
      replayerFor(router).drain('tap');

      // Frame 1: first attempt bounced home, miss recorded, replay scheduled.
      await tester.pump();
      expect(location(router), '/');
      expect(events, hasLength(1));
      expect(events[0].parameters['landed'], false);
      expect(events[0].parameters['trigger'], 'tap');

      // The override clears before the replay timer fires.
      block = false;
      await tester.pump(const Duration(milliseconds: 60)); // replay go()
      await tester.pump(); // replay verify

      expect(location(router), '/experience/exp-1');
      expect(events, hasLength(2));
      final replay = events[1].parameters;
      expect(replay['landed'], true);
      expect(replay['trigger'], 'replay');
      expect(replay['recovered'], true);
      expect(drops, isEmpty,
          reason: 'a recovered replay is not an unrecoverable drop');
    });

    testWidgets('gives up after the replay also fails, flagging recovered:false',
        (tester) async {
      final router = buildRouter(block: () => true); // never lands
      await tester.pumpWidget(MaterialApp.router(routerConfig: router));

      PendingNotificationRouteHolder.value = const PendingNotificationRoute(
        route: '/experience/exp-1',
        source: 'missed',
      );
      replayerFor(router).drain('resume');
      await tester.pump(); // verify0 miss, replay scheduled
      await tester.pump(const Duration(milliseconds: 60)); // replay go()
      await tester.pump(); // replay verify

      expect(location(router), '/');
      expect(events, hasLength(2));
      // First miss on the resume drain.
      expect(events[0].parameters['landed'], false);
      expect(events[0].parameters['trigger'], 'resume');
      // Replay also missed → permanent client-side drop.
      expect(events[1].parameters['landed'], false);
      expect(events[1].parameters['trigger'], 'replay');
      expect(events[1].parameters['recovered'], false);
      // ...and the unrecoverable-drop sink fired exactly once with the route,
      // so main.dart can report it as a Crashlytics non-fatal.
      expect(drops, hasLength(1));
      expect(drops.single.route, '/experience/exp-1');
      expect(drops.single.source, 'missed');
    });
  });
}
