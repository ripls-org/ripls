import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/services/community_event_poller.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/event_router.dart';
import 'package:ripls/services/providers.dart';

@GenerateMocks([CommunityService])
import 'community_event_poller_test.mocks.dart';

void main() {
  late MockCommunityService mockService;
  late ProviderContainer container;
  late EventRouter eventRouter;
  late CommunityEventPoller poller;

  setUp(() {
    mockService = MockCommunityService();
    container = ProviderContainer();
    eventRouter = container.read(eventRouterProvider);
    poller = CommunityEventPoller(mockService, eventRouter);
  });

  tearDown(() {
    poller.reset();
    container.dispose();
  });

  CommunityEventItem makeEvent(String id, int timestamp) {
    return CommunityEventItem(
      id: id,
      communityId: 'c1',
      eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
      occurredAtUnixSec: Int64(timestamp),
    );
  }

  void stubPoll(List<CommunityEventItem> events) {
    when(mockService.listUserEvents(sinceUnixSec: anyNamed('sinceUnixSec')))
        .thenAnswer((_) async => events);
  }

  group('CommunityEventPoller', () {
    test('first poll uses seeded timestamp (not null)', () async {
      stubPoll([]);

      poller.start();
      await Future.delayed(Duration.zero);

      final captured = verify(mockService.listUserEvents(
        sinceUnixSec: captureAnyNamed('sinceUnixSec'),
      )).captured;
      expect(captured.first, isNotNull);

      poller.stop();
    });

    test('one poll covers the whole portfolio regardless of size', () async {
      // The defect this replaced (#2867): poll count scaled with membership,
      // so a large portfolio issued a request per community per interval.
      // There is no community argument to pass any more — a single call is
      // structurally all there is — so this pins that `start` issues exactly
      // one request, not one per anything.
      stubPoll([
        makeEvent('e1', 1000),
        makeEvent('e2', 1001),
        makeEvent('e3', 1002),
      ]);

      poller.start();
      await Future.delayed(Duration.zero);

      verify(mockService.listUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      )).called(1);

      poller.stop();
    });

    test('poll routes events through EventRouter', () async {
      stubPoll([makeEvent('e1', 1000)]);

      poller.start();
      await Future.delayed(Duration.zero);

      expect(container.read(contentCacheInvalidationProvider), greaterThan(0));

      poller.stop();
    });

    test('subsequent polls use last event timestamp', () async {
      // A timestamp in the future, so it beats the seeded "now".
      final futureTs = DateTime.now().millisecondsSinceEpoch ~/ 1000 + 3600;

      stubPoll([makeEvent('e1', futureTs)]);
      poller.start();
      await Future.delayed(Duration.zero);
      poller.stop();

      when(mockService.listUserEvents(sinceUnixSec: futureTs))
          .thenAnswer((_) async => []);

      poller.start();
      await Future.delayed(Duration.zero);

      verify(mockService.listUserEvents(sinceUnixSec: futureTs)).called(1);

      poller.stop();
    });

    test('stop cancels the timer', () async {
      stubPoll([]);

      poller.start();
      await Future.delayed(Duration.zero);

      poller.stop();

      clearInteractions(mockService);
      await Future.delayed(const Duration(milliseconds: 50));
      verifyNever(mockService.listUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      ));
    });

    test('a poll that resolves after stop does not route', () async {
      // docs/client/logout.md § Invariants: pollers stop before caches clear,
      // so in-flight events must not repopulate them. Without the post-await
      // guard this routes the previous user's events into the next user's
      // session.
      final gate = Completer<List<CommunityEventItem>>();
      when(mockService.listUserEvents(sinceUnixSec: anyNamed('sinceUnixSec')))
          .thenAnswer((_) => gate.future);

      poller.start();
      await Future.delayed(Duration.zero);

      final before = container.read(contentCacheInvalidationProvider);

      // Teardown lands while the request is still in flight.
      poller.stop();
      gate.complete([makeEvent('late', 9000)]);
      await Future.delayed(Duration.zero);

      expect(
        container.read(contentCacheInvalidationProvider),
        before,
        reason: 'a poll resolving after stop must not invalidate caches',
      );
    });

    test('reset clears timestamps so next start seeds fresh', () async {
      stubPoll([makeEvent('e1', 5000)]);

      poller.start();
      await Future.delayed(Duration.zero);

      poller.reset();

      stubPoll([]);

      poller.start();
      await Future.delayed(Duration.zero);

      final captured = verify(mockService.listUserEvents(
        sinceUnixSec: captureAnyNamed('sinceUnixSec'),
      )).captured;
      expect(captured.last, isNot(5000));

      poller.stop();
    });

    test('poll errors are swallowed gracefully', () async {
      when(mockService.listUserEvents(sinceUnixSec: anyNamed('sinceUnixSec')))
          .thenThrow(Exception('network error'));

      poller.start();
      await Future.delayed(Duration.zero);

      poller.stop();
    });
  });
}
