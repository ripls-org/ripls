import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/services/community_event_stream.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/providers.dart';

@GenerateMocks([CommunityService])
import 'community_event_stream_test.mocks.dart';

void main() {
  late MockCommunityService mockService;
  late ProviderContainer container;
  late CommunityEventStreamService streamService;

  setUp(() {
    mockService = MockCommunityService();
    container = ProviderContainer();
    final eventRouter = container.read(eventRouterProvider);
    // Short backoff so a reconnect actually lands inside the test's window;
    // with the production two seconds the "does not reconnect" assertion
    // would hold regardless of whether the guard works.
    streamService = CommunityEventStreamService(
      mockService,
      eventRouter,
      baseDelay: const Duration(milliseconds: 5),
    );
  });

  tearDown(() {
    streamService.reset();
    container.dispose();
  });

  StreamUserEventsResponse makeResponse(
    String id,
    int timestamp, {
    String communityId = 'c1',
  }) {
    return StreamUserEventsResponse(
      event: CommunityEventItem(
        id: id,
        communityId: communityId,
        eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
        occurredAtUnixSec: Int64(timestamp),
      ),
    );
  }

  group('CommunityEventStreamService', () {
    test('connect opens the stream and routes events', () async {
      final controller = StreamController<StreamUserEventsResponse>();

      when(mockService.streamUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      )).thenAnswer((_) => controller.stream);

      streamService.connect();

      controller.add(makeResponse('e1', 1000));
      await Future.delayed(Duration.zero);

      expect(container.read(contentCacheInvalidationProvider), greaterThan(0));

      await controller.close();
      streamService.disconnect();
    });

    test('one connection covers every community', () async {
      // The defect this replaced (#2867): one stream per community meant a
      // large portfolio exhausted the browser's per-origin connection pool and
      // starved every other request. There is no community argument to pass
      // any more, so this pins that events from different communities all
      // arrive on the single subscription.
      final controller = StreamController<StreamUserEventsResponse>();

      when(mockService.streamUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      )).thenAnswer((_) => controller.stream);

      streamService.connect();

      controller.add(makeResponse('e1', 1000, communityId: 'c1'));
      controller.add(makeResponse('e2', 1001, communityId: 'c2'));
      controller.add(makeResponse('e3', 1002, communityId: 'c3'));
      await Future.delayed(Duration.zero);

      verify(mockService.streamUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      )).called(1);
      expect(container.read(contentCacheInvalidationProvider), greaterThan(0));

      await controller.close();
      streamService.disconnect();
    });

    test('disconnect cancels the subscription', () async {
      final controller = StreamController<StreamUserEventsResponse>();

      when(mockService.streamUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      )).thenAnswer((_) => controller.stream);

      streamService.connect();
      streamService.disconnect();

      final before = container.read(contentCacheInvalidationProvider);
      controller.add(makeResponse('e1', 1000));
      await Future.delayed(Duration.zero);

      expect(container.read(contentCacheInvalidationProvider), before);

      await controller.close();
    });

    test('a stream that ends after disconnect does not reconnect', () async {
      // docs/client/logout.md § Invariants: streams stop before caches clear.
      // The old per-community service reconnected from onError without
      // checking whether it had been disconnected, resurrecting a stream that
      // nothing would close and that could route the previous user's events
      // into the next user's just-cleared caches.
      final controller = StreamController<StreamUserEventsResponse>();

      when(mockService.streamUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      )).thenAnswer((_) => controller.stream);

      streamService.connect();
      await Future.delayed(Duration.zero);

      streamService.disconnect();
      clearInteractions(mockService);

      // The transport notices the cancel and completes the stream afterwards.
      await controller.close();
      await Future.delayed(const Duration(milliseconds: 50));

      verifyNever(mockService.streamUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      ));
    });

    test('a stream that ends while connected does reconnect', () async {
      // The counterpart to the test above: proves the guard is what stops the
      // reconnect, rather than the reconnect never firing in this window.
      //
      // Each call gets a FRESH controller. A reconnect re-invokes the service,
      // and handing back an already-listened single-subscription stream throws
      // "Stream has already been listened to" — a harness artifact that reads
      // like a production failure.
      final controllers = <StreamController<StreamUserEventsResponse>>[];
      when(mockService.streamUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      )).thenAnswer((_) {
        final c = StreamController<StreamUserEventsResponse>();
        controllers.add(c);
        return c.stream;
      });

      streamService.connect();
      await Future.delayed(Duration.zero);
      clearInteractions(mockService);

      // Stream drops with no disconnect() — the server's lifetime cap.
      await controllers.first.close();
      await Future.delayed(const Duration(milliseconds: 100));

      verify(mockService.streamUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      )).called(greaterThan(0));

      streamService.disconnect();
      for (final c in controllers.skip(1)) {
        await c.close();
      }
    });

    test('reset clears the timestamp so the next connect asks for no history',
        () async {
      final controller = StreamController<StreamUserEventsResponse>();

      when(mockService.streamUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      )).thenAnswer((_) => controller.stream);

      streamService.connect();
      controller.add(makeResponse('e1', 5000));
      await Future.delayed(Duration.zero);

      streamService.reset();
      clearInteractions(mockService);

      final controller2 = StreamController<StreamUserEventsResponse>();
      when(mockService.streamUserEvents(sinceUnixSec: null))
          .thenAnswer((_) => controller2.stream);

      streamService.connect();

      verify(mockService.streamUserEvents(sinceUnixSec: null)).called(1);

      await controller.close();
      await controller2.close();
      streamService.disconnect();
    });

    test('passes sinceUnixSec from last received event on reconnect', () async {
      final futureTs = DateTime.now().millisecondsSinceEpoch ~/ 1000 + 3600;

      final controller1 = StreamController<StreamUserEventsResponse>();
      when(mockService.streamUserEvents(
        sinceUnixSec: anyNamed('sinceUnixSec'),
      )).thenAnswer((_) => controller1.stream);

      streamService.connect();
      controller1.add(makeResponse('e1', futureTs));
      await Future.delayed(Duration.zero);

      streamService.disconnect();

      final controller2 = StreamController<StreamUserEventsResponse>();
      when(mockService.streamUserEvents(sinceUnixSec: futureTs))
          .thenAnswer((_) => controller2.stream);

      streamService.connect();

      verify(mockService.streamUserEvents(sinceUnixSec: futureTs)).called(1);

      await controller1.close();
      await controller2.close();
      streamService.disconnect();
    });
  });
}
