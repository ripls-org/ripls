import 'dart:async';

import 'package:connectrpc/connect.dart' as connect;
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/device_service.pb.dart';
import 'package:ripls/services/fcm_service.dart';

/// Fake FirebaseMessaging that stubs getInitialMessage() and getToken().
///
/// Uses Fake instead of Mock to avoid Mockito's stub cascade issues with
/// FirebaseMessaging's non-nullable Future return types. Only the methods
/// actually called by [FCMService.checkForMissedNotification] and
/// [FCMService.registerDeviceTokenForTest] are overridden.
class FakeFirebaseMessaging extends Fake implements FirebaseMessaging {
  /// Handler that produces the initial message. Set per-test.
  Future<RemoteMessage?> Function() getInitialMessageHandler =
      () => Future.value();

  /// Token to return from [getToken]. Set per-test.
  String? tokenToReturn;

  @override
  Future<RemoteMessage?> getInitialMessage() => getInitialMessageHandler();

  @override
  Future<String?> getToken({
    String? serviceWorkerScriptPath,
    String? vapidKey,
  }) async => tokenToReturn;
}

/// Minimal stub transport — FCMService only uses it for device token
/// registration which these tests don't exercise directly.
class _StubTransport extends Fake implements connect.Transport {}

/// Fake [FcmDeviceRegistrar] with a scriptable response queue.
///
/// Each call to [registerDeviceToken] pops the next response from the queue:
/// - a [RegisterDeviceTokenResponse] is returned normally,
/// - a [connect.ConnectException] is thrown.
class _FakeFcmDeviceRegistrar extends Fake implements FcmDeviceRegistrar {
  final List<Object> _registerQueue;

  _FakeFcmDeviceRegistrar({required List<Object> registerQueue})
      : _registerQueue = List.of(registerQueue);

  @override
  Future<RegisterDeviceTokenResponse> registerDeviceToken(
    RegisterDeviceTokenRequest request, {
    connect.Headers? headers,
  }) async {
    if (_registerQueue.isEmpty) {
      throw StateError(
        '_FakeFcmDeviceRegistrar: registerDeviceToken called more times '
        'than responses were queued',
      );
    }
    final next = _registerQueue.removeAt(0);
    if (next is RegisterDeviceTokenResponse) return next;
    // next is a ConnectException queued for this call.
    throw next; // ignore: only_throw_errors
  }

  @override
  Future<UnregisterDeviceTokenResponse> unregisterDeviceToken(
    UnregisterDeviceTokenRequest request, {
    connect.Headers? headers,
  }) async {
    return UnregisterDeviceTokenResponse();
  }
}

/// Creates an FCMService wired to the given fake messaging instance.
FCMService _createService(
  FakeFirebaseMessaging fakeMessaging, {
  FcmDeviceRegistrar? deviceRegistrar,
  String? Function()? getAccessToken,
}) {
  return FCMService(
    transport: _StubTransport(),
    getAccessToken: getAccessToken ?? (() => 'test-token'),
    messaging: fakeMessaging,
    deviceRegistrar: deviceRegistrar,
  );
}

void main() {
  group('checkForMissedNotification', () {
    test('routes notification from getInitialMessage on resume', () async {
      final fake = FakeFirebaseMessaging();
      fake.getInitialMessageHandler = () async => const RemoteMessage(
            messageId: 'msg-1',
            data: {
              'gear_id': 'gear-abc',
              'conversation_id': 'conv-1',
              'community_id': 'comm-xyz',
            },
          );

      final service = _createService(fake);

      String? navigatedRoute;
      String? navigatedCommunityId;
      service.onNavigate = (route, {String? communityId}) {
        navigatedRoute = route;
        navigatedCommunityId = communityId;
      };

      await service.checkForMissedNotification();

      expect(navigatedRoute, '/gear/gear-abc?tab=chat');
      expect(navigatedCommunityId, 'comm-xyz');
    });

    test('does nothing when getInitialMessage returns null', () async {
      final fake = FakeFirebaseMessaging();
      fake.getInitialMessageHandler = () async => null;

      final service = _createService(fake);

      var callCount = 0;
      service.onNavigate = (route, {String? communityId}) {
        callCount++;
      };

      await service.checkForMissedNotification();

      expect(callCount, 0);
    });

    test('deduplicates by message ID across repeated resume checks', () async {
      final fake = FakeFirebaseMessaging();
      // Every call returns the same message ID.
      fake.getInitialMessageHandler = () async => const RemoteMessage(
            messageId: 'msg-dup',
            data: {'gear_id': 'gear-1'},
          );

      final service = _createService(fake);

      var callCount = 0;
      service.onNavigate = (route, {String? communityId}) {
        callCount++;
      };

      await service.checkForMissedNotification();
      await service.checkForMissedNotification();

      expect(callCount, 1, reason: 'Same messageId should only route once');
    });

    test('routes different message IDs separately', () async {
      final fake = FakeFirebaseMessaging();
      var callNumber = 0;
      fake.getInitialMessageHandler = () async {
        callNumber++;
        return RemoteMessage(
          messageId: 'msg-$callNumber',
          data: const {'gear_id': 'gear-1'},
        );
      };

      final service = _createService(fake);

      final routes = <String>[];
      service.onNavigate = (route, {String? communityId}) {
        routes.add(route);
      };

      await service.checkForMissedNotification();
      await service.checkForMissedNotification();

      expect(routes, ['/gear/gear-1', '/gear/gear-1']);
    });

    test('does not crash when getInitialMessage throws', () async {
      final fake = FakeFirebaseMessaging();
      fake.getInitialMessageHandler = () => Future.error(Exception('fail'));

      final service = _createService(fake);

      service.onNavigate = (route, {String? communityId}) {
        fail('Should not navigate on error');
      };

      // Should not throw — errors are caught internally.
      await service.checkForMissedNotification();
    });

    test('waits for the in-flight startup consume before reading', () async {
      // Single-consume hardening (#2636): a resume check must not read the
      // terminated-launch message until the startup consume has recorded it,
      // otherwise the same tap is handled twice.
      final fake = FakeFirebaseMessaging();
      final order = <String>[];
      final startupDone = Completer<void>();

      fake.getInitialMessageHandler = () async {
        order.add('resume-read');
        return const RemoteMessage(
          messageId: 'm-1',
          data: {'gear_id': 'gear-1'},
        );
      };

      final service = _createService(fake);
      service.onNavigate = (route, {String? communityId}) =>
          order.add('navigate');
      service.setInitialMessageInFlightForTest(
        startupDone.future.then((_) => order.add('startup-consume-done')),
      );

      final resumeCheck = service.checkForMissedNotification();

      // The resume check is blocked on the startup consume: nothing yet.
      await Future<void>.delayed(Duration.zero);
      expect(order, isEmpty,
          reason: 'resume read must wait for the startup consume');

      startupDone.complete();
      await resumeCheck;

      expect(order, ['startup-consume-done', 'resume-read', 'navigate']);
    });
  });

  group('message ID deduplication', () {
    test(
      'prevents double-handling when same message arrives via '
      'two different paths',
      () async {
        // Simulates Android warm-start: onMessageOpenedApp fires first (via
        // broadcast stream), then checkForMissedNotification reads the same
        // message from getInitialMessage on resume. Both paths converge in
        // _onMessageOpenedApp which deduplicates by messageId.
        final fake = FakeFirebaseMessaging();
        fake.getInitialMessageHandler = () async => const RemoteMessage(
              messageId: 'msg-warm',
              data: {
                'experience_id': 'exp-1',
                'community_id': 'comm-1',
              },
            );

        final service = _createService(fake);

        final routes = <String>[];
        service.onNavigate = (route, {String? communityId}) {
          routes.add(route);
        };

        // First delivery via resume check.
        await service.checkForMissedNotification();
        expect(routes, hasLength(1));

        // Second delivery with same messageId — should be deduped.
        await service.checkForMissedNotification();
        expect(
          routes,
          hasLength(1),
          reason: 'Second call with same messageId should be deduped',
        );
      },
    );

    test('null messageId is not deduped (no ID to track)', () async {
      final fake = FakeFirebaseMessaging();
      fake.getInitialMessageHandler = () async => const RemoteMessage(
            // messageId is null — can't deduplicate.
            data: {'gear_id': 'gear-no-id'},
          );

      final service = _createService(fake);

      var callCount = 0;
      service.onNavigate = (route, {String? communityId}) {
        callCount++;
      };

      await service.checkForMissedNotification();
      await service.checkForMissedNotification();

      // Both fire because there's no messageId to deduplicate on.
      expect(callCount, 2);
    });
  });

  group('initialize reordering', () {
    // The critical fix — _setupMessageHandlers() running before
    // _registerDeviceToken() — cannot be unit tested because
    // _setupMessageHandlers() calls static FirebaseMessaging.onMessage and
    // FirebaseMessaging.onMessageOpenedApp which require Firebase platform
    // initialization.
    //
    // The reordering is verified by code inspection:
    //
    //   fcm_service.dart initialize() line ordering:
    //     1. _setupMessageHandlers()  ← runs first, unconditionally
    //     2. await _requestPermission()
    //     3. await _initializeLocalNotifications()
    //     4. try { await _registerDeviceToken() }  ← wrapped, can fail safely
    //
    // Full integration test requires a running Firebase environment
    // (on-device test).

    test('documents the reordering fix for code review', () {
      // This test exists to make the hypothesis explicit and discoverable.
      // The actual verification is structural: _setupMessageHandlers() is the
      // first call in initialize(), and _registerDeviceToken() is wrapped in
      // its own try/catch so its failure cannot prevent message handlers from
      // being set up.
      expect(true, isTrue);
    });
  });

  group('registerDeviceToken retry behavior', () {
    late List<LogRecord> logs;
    late StreamSubscription<LogRecord> logSub;

    setUp(() {
      logs = [];
      logSub = Logger.root.onRecord.listen(logs.add);
      // Stub a successful token refresh so executeRpc's retry path runs.
      RpcUtils.configureTokenRefresh(() async => true);
    });

    tearDown(() async {
      await logSub.cancel();
      // Reset to a no-op so subsequent tests start clean.
      RpcUtils.configureTokenRefresh(() async => false);
    });

    test(
      'no SEVERE from FCMService when unauthenticated error is recovered '
      'by refresh+retry',
      () async {
        var callCount = 0;
        final registrar = _FakeFcmDeviceRegistrar(
          registerQueue: [
            // First call: unauthenticated (stale token at cold start).
            connect.ConnectException(
              connect.Code.unauthenticated,
              'token is expired',
            ),
            // Second call (after refresh): success.
            RegisterDeviceTokenResponse(deviceId: 'device-123'),
          ],
        );

        final fake = FakeFirebaseMessaging();
        fake.tokenToReturn = 'fcm-token-abcdefghij12345';

        final service = _createService(
          fake,
          deviceRegistrar: registrar,
        );

        await service.registerDeviceTokenForTest();

        // No SEVERE should be emitted from FCMService for a recoverable
        // cold-start unauthenticated failure.
        final fcmSevere = logs.where(
          (r) =>
              r.level == Level.SEVERE && r.loggerName == 'FCMService',
        );
        expect(
          fcmSevere,
          isEmpty,
          reason: 'Recoverable unauthenticated error must not log SEVERE',
        );

        // RpcUtils logs a WARNING for the first failed attempt — expected.
        final rpcWarnings = logs.where(
          (r) => r.level >= Level.WARNING && r.loggerName == 'RpcUtils',
        );
        expect(rpcWarnings, isNotEmpty, reason: 'RpcUtils WARNING expected for initial failure');

        // The retry call actually fired (registrar queue exhausted).
        callCount = 2 - registrar._registerQueue.length;
        expect(callCount, 2, reason: 'registerDeviceToken should have been called twice');
      },
    );

    test(
      'throws ServiceException for non-unauthenticated ConnectException '
      'without logging SEVERE from FCMService',
      () async {
        final registrar = _FakeFcmDeviceRegistrar(
          registerQueue: [
            connect.ConnectException(
              connect.Code.unavailable,
              'server unavailable',
            ),
          ],
        );

        final fake = FakeFirebaseMessaging();
        fake.tokenToReturn = 'fcm-token-abcdefghij12345';

        final service = _createService(
          fake,
          deviceRegistrar: registrar,
        );

        await expectLater(
          service.registerDeviceTokenForTest(),
          throwsA(isA<ServiceException>()),
        );

        // _registerDeviceToken itself does not log SEVERE; it just throws.
        // initialize()'s catch is responsible for the SEVERE on genuine failure.
        final fcmSevere = logs.where(
          (r) =>
              r.level == Level.SEVERE && r.loggerName == 'FCMService',
        );
        expect(
          fcmSevere,
          isEmpty,
          reason:
              '_registerDeviceToken must not log SEVERE; only initialize() does',
        );
      },
    );

    test(
      'throws ServiceException when refresh fails and logs no SEVERE '
      'from FCMService',
      () async {
        // Override: token refresh returns false (can't refresh).
        RpcUtils.configureTokenRefresh(() async => false);

        final registrar = _FakeFcmDeviceRegistrar(
          registerQueue: [
            connect.ConnectException(
              connect.Code.unauthenticated,
              'token is expired',
            ),
          ],
        );

        final fake = FakeFirebaseMessaging();
        fake.tokenToReturn = 'fcm-token-abcdefghij12345';

        final service = _createService(
          fake,
          deviceRegistrar: registrar,
        );

        await expectLater(
          service.registerDeviceTokenForTest(),
          throwsA(isA<ServiceException>()),
        );

        // _registerDeviceToken itself does not log SEVERE.
        final fcmSevere = logs.where(
          (r) =>
              r.level == Level.SEVERE && r.loggerName == 'FCMService',
        );
        expect(
          fcmSevere,
          isEmpty,
          reason:
              '_registerDeviceToken must not log SEVERE; only initialize() does',
        );
      },
    );
  });
}
