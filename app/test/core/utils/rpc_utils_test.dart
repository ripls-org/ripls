import 'dart:async';

import 'package:connectrpc/connect.dart' as connect;
import 'package:flutter_test/flutter_test.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/providers.dart';
import 'package:ripls/core/utils/request_id.dart';
import 'package:ripls/core/utils/rpc_utils.dart';

// Helper function to create a ConnectException for testing
connect.ConnectException createConnectException(
  connect.Code code,
  String message,
) {
  return connect.ConnectException(code, message);
}

void main() {
  group('RpcUtils', () {
    group('buildHeaders', () {
      test('includes Authorization header when token is provided', () {
        final headers = RpcUtils.buildHeaders(() => 'test-token-123');

        expect(headers.get('Authorization'), ['Bearer test-token-123']);
      });

      test('always includes X-Request-ID header', () {
        final headers = RpcUtils.buildHeaders(() => 'test-token');

        final requestId = headers.get(RequestIdGenerator.headerName);
        expect(requestId, isNotNull);
        expect(requestId!.length, 1);
        expect(requestId.first, isNotEmpty);
      });

      test('X-Request-ID is UUID format', () {
        final headers = RpcUtils.buildHeaders(() => 'test-token');

        final requestId = headers.get(RequestIdGenerator.headerName)!.first;
        final uuidRegex = RegExp(
          r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$',
          caseSensitive: false,
        );
        expect(uuidRegex.hasMatch(requestId), isTrue);
      });

      test('X-Request-ID is unique per call', () {
        final headers1 = RpcUtils.buildHeaders(() => 'test-token');
        final headers2 = RpcUtils.buildHeaders(() => 'test-token');

        final requestId1 = headers1.get(RequestIdGenerator.headerName)!.first;
        final requestId2 = headers2.get(RequestIdGenerator.headerName)!.first;
        expect(requestId1, isNot(equals(requestId2)));
      });

      test('reuses zone-active request_id when one is set', () {
        final headers = RequestIdGenerator.runWithRequestId<connect.Headers>(
          'zone-scoped-id',
          () => RpcUtils.buildHeaders(() => 'test-token'),
        );

        final requestId = headers.get(RequestIdGenerator.headerName)!.first;
        expect(requestId, 'zone-scoped-id');
      });

      test('includes X-Request-ID even when token is null', () async {
        final headers = RpcUtils.buildHeaders(() => null);

        final requestId = headers.get(RequestIdGenerator.headerName);
        expect(requestId, isNotNull);
        expect(requestId!.first, isNotEmpty);
      });

      test('returns headers without Authorization when token is null', () async {
        var callbackTriggered = false;

        final headers = RpcUtils.buildHeaders(
          () => null,
          onUnauthenticated: () async {
            callbackTriggered = true;
          },
        );

        // Wait for microtask to complete
        await Future.delayed(Duration.zero);

        expect(headers.get('Authorization'), isNull);
        expect(headers.get(RequestIdGenerator.headerName), isNotNull);
        expect(callbackTriggered, true);
      });

      test('returns headers without Authorization when token is empty', () async {
        var callbackTriggered = false;

        final headers = RpcUtils.buildHeaders(
          () => '',
          onUnauthenticated: () async {
            callbackTriggered = true;
          },
        );

        // Wait for microtask to complete
        await Future.delayed(Duration.zero);

        expect(headers.get('Authorization'), isNull);
        expect(headers.get(RequestIdGenerator.headerName), isNotNull);
        expect(callbackTriggered, true);
      });

      test('does not trigger callback when token is present', () async {
        var callbackTriggered = false;

        RpcUtils.buildHeaders(
          () => 'valid-token',
          onUnauthenticated: () async {
            callbackTriggered = true;
          },
        );

        // Wait for microtask to complete
        await Future.delayed(Duration.zero);

        expect(callbackTriggered, false);
      });

      test('does not fail when no callback provided and token is empty', () {
        expect(() => RpcUtils.buildHeaders(() => null), returnsNormally);
      });
    });

    group('executeRpc', () {
      final errorHandler = RpcErrorHandler();

      test('returns result when RPC call succeeds', () async {
        final result = await RpcUtils.executeRpc<String>(
          () async => 'success',
          errorHandler,
          operationName: 'TestOperation',
        );

        expect(result, 'success');
      });

      test('publishes a request_id to the active zone for the rpc closure', () async {
        String? idDuringCall;
        await RpcUtils.executeRpc<void>(
          () async {
            idDuringCall = RequestIdGenerator.current;
          },
          errorHandler,
          operationName: 'TestOperation',
        );

        expect(idDuringCall, isNotNull);
        expect(idDuringCall, isNotEmpty);
        expect(RequestIdGenerator.current, isNull,
            reason: 'zone-scoped id must not leak past the rpc call');
      });

      test('buildHeaders inside executeRpc reuses the executeRpc id', () async {
        late final String idFromZone;
        late final String idOnHeader;
        await RpcUtils.executeRpc<void>(
          () async {
            idFromZone = RequestIdGenerator.current!;
            final headers = RpcUtils.buildHeaders(() => null);
            idOnHeader = headers.get(RequestIdGenerator.headerName)!.first;
          },
          errorHandler,
          operationName: 'TestOperation',
        );

        expect(idFromZone, isNotEmpty);
        expect(idOnHeader, equals(idFromZone),
            reason: 'X-Request-ID must match the zone-scoped id so client '
                'logs and outbound headers carry the same correlation id');
      });

      test('each executeRpc call gets a distinct request_id', () async {
        final ids = <String>{};
        for (var i = 0; i < 5; i++) {
          await RpcUtils.executeRpc<void>(
            () async {
              ids.add(RequestIdGenerator.current!);
            },
            errorHandler,
            operationName: 'TestOperation',
          );
        }
        expect(ids.length, 5);
      });

      test('throws ServiceException on generic error', () async {
        expect(
          () => RpcUtils.executeRpc<String>(
            () async => throw Exception('network error'),
            errorHandler,
            operationName: 'TestOperation',
          ),
          throwsA(
            isA<ServiceException>().having(
              (e) => e.message,
              'message',
              'Something went wrong. Please try again.',
            ),
          ),
        );
      });

      test(
        'throws ServiceException with custom message for ConnectException',
        () async {
          expect(
            () => RpcUtils.executeRpc<String>(
              () async => throw createConnectException(
                connect.Code.notFound,
                'resource not found',
              ),
              errorHandler,
              operationName: 'TestOperation',
            ),
            throwsA(isA<ServiceException>()),
          );
        },
      );

      test(
        'triggers onUnauthenticated callback for unauthenticated error',
        () async {
          var callbackTriggered = false;

          try {
            await RpcUtils.executeRpc<String>(
              () async => throw createConnectException(
                connect.Code.unauthenticated,
                'not authenticated',
              ),
              errorHandler,
              onUnauthenticated: () async {
                callbackTriggered = true;
              },
              operationName: 'TestOperation',
            );
          } catch (_) {
            // Expected to throw
          }

          expect(callbackTriggered, true);
        },
      );

      test(
        'does not trigger callback for non-unauthenticated errors',
        () async {
          var callbackTriggered = false;

          try {
            await RpcUtils.executeRpc<String>(
              () async => throw createConnectException(
                connect.Code.notFound,
                'not found',
              ),
              errorHandler,
              onUnauthenticated: () async {
                callbackTriggered = true;
              },
              operationName: 'TestOperation',
            );
          } catch (_) {
            // Expected to throw
          }

          expect(callbackTriggered, false);
        },
      );

      test('handles null onUnauthenticated callback gracefully', () async {
        expect(
          () => RpcUtils.executeRpc<String>(
            () async => throw createConnectException(
              connect.Code.unauthenticated,
              'not authenticated',
            ),
            errorHandler,
            operationName: 'TestOperation',
          ),
          throwsA(isA<ServiceException>()),
        );
      });

      test('propagates ServiceException after triggering callback', () async {
        var callbackTriggered = false;

        expect(
          () => RpcUtils.executeRpc<String>(
            () async => throw createConnectException(
              connect.Code.unauthenticated,
              'not authenticated',
            ),
            errorHandler,
            onUnauthenticated: () async {
              callbackTriggered = true;
            },
            operationName: 'TestOperation',
          ),
          throwsA(isA<ServiceException>()),
        );

        // Wait for callback to execute
        await Future.delayed(Duration.zero);
        expect(callbackTriggered, true);
      });
    });

    group('executeRpc with performance tracking', () {
      late _MockPerformanceMonitor mockMonitor;
      late RpcErrorHandler errorHandler;

      setUp(() {
        mockMonitor = _MockPerformanceMonitor();
        errorHandler = RpcErrorHandler();
      });

      test('tracks successful RPC call when monitor is available', () async {
        mockMonitor.isAvailable = true;

        final result = await RpcUtils.executeRpc(
          () async => 'success',
          errorHandler,
          operationName: 'TestOperation',
          performanceMonitor: mockMonitor,
        );

        expect(result, equals('success'));
        expect(mockMonitor.startedTraces, contains('rpc_TestOperation'));

        final trace = mockMonitor.lastTrace;
        expect(trace?.started, isTrue);
        expect(trace?.stopped, isTrue);
        expect(trace?.attributes['operation'], equals('TestOperation'));
        expect(trace?.attributes['status'], equals('success'));
        expect(trace?.metrics['duration_ms'], isNotNull);
      });

      test('tracks failed RPC call with error code', () async {
        mockMonitor.isAvailable = true;

        await expectLater(
          RpcUtils.executeRpc(
            () async => throw connect.ConnectException(
              connect.Code.notFound,
              'Not found',
            ),
            errorHandler,
            operationName: 'FailedOperation',
            performanceMonitor: mockMonitor,
          ),
          throwsA(isA<ServiceException>()),
        );

        final trace = mockMonitor.lastTrace;
        expect(trace?.stopped, isTrue);
        expect(trace?.attributes['status'], equals('error'));
        expect(trace?.attributes['error_code'], equals('not_found'));
      });

      test('does not track when monitor is null', () async {
        final result = await RpcUtils.executeRpc(
          () async => 'success',
          errorHandler,
          operationName: 'TestOperation',
          performanceMonitor: null,
        );

        expect(result, equals('success'));
        expect(mockMonitor.startedTraces, isEmpty);
      });

      test('does not track when monitor is unavailable', () async {
        mockMonitor.isAvailable = false;

        final result = await RpcUtils.executeRpc(
          () async => 'success',
          errorHandler,
          operationName: 'TestOperation',
          performanceMonitor: mockMonitor,
        );

        expect(result, equals('success'));
        expect(mockMonitor.startedTraces, isEmpty);
      });

      test('tracks unexpected errors with error_code unexpected', () async {
        mockMonitor.isAvailable = true;

        await expectLater(
          RpcUtils.executeRpc(
            () async => throw Exception('Unexpected error'),
            errorHandler,
            operationName: 'UnexpectedError',
            performanceMonitor: mockMonitor,
          ),
          throwsA(isA<ServiceException>()),
        );

        final trace = mockMonitor.lastTrace;
        expect(trace?.stopped, isTrue);
        expect(trace?.attributes['status'], equals('error'));
        expect(trace?.attributes['error_code'], equals('unexpected'));
      });

      test('measures duration correctly', () async {
        mockMonitor.isAvailable = true;

        await RpcUtils.executeRpc(
          () async {
            await Future.delayed(const Duration(milliseconds: 50));
            return 'success';
          },
          errorHandler,
          operationName: 'SlowOperation',
          performanceMonitor: mockMonitor,
        );

        final trace = mockMonitor.lastTrace;
        final durationMs = trace?.metrics['duration_ms'] ?? 0;
        // Allow some tolerance for timing
        expect(durationMs, greaterThanOrEqualTo(40));
        expect(durationMs, lessThan(500)); // Shouldn't take too long
      });
    });

    group('audit logging', () {
      late RpcErrorHandler errorHandler;
      late List<LogRecord> records;
      late StreamSubscription<LogRecord> sub;

      setUp(() {
        errorHandler = RpcErrorHandler();
        records = [];
        Logger.root.level = Level.ALL;
        sub = Logger.root.onRecord.listen(records.add);
      });

      tearDown(() {
        sub.cancel();
        RpcUtils.auditLogging = false;
        RpcUtils.networkBreadcrumbSink = null;
      });

      test('emits no RPC_AUDIT line when auditLogging is false', () async {
        RpcUtils.auditLogging = false;

        await RpcUtils.executeRpc<String>(
          () async => 'ok',
          errorHandler,
          operationName: 'Quiet',
        );

        expect(
          records.any((r) => r.message.startsWith('RPC_AUDIT')),
          isFalse,
        );
      });

      test('emits RPC_AUDIT line with status=ok on success', () async {
        RpcUtils.auditLogging = true;

        await RpcUtils.executeRpc<String>(
          () async => 'ok',
          errorHandler,
          operationName: 'TestOp',
        );

        final auditLine = records
            .firstWhere((r) => r.message.startsWith('RPC_AUDIT'))
            .message;
        expect(auditLine, contains('rpc=TestOp'));
        expect(auditLine, contains('status=ok'));
        expect(auditLine, contains('duration_ms='));
      });

      test('captures request_id from buildHeaders via Zone sink', () async {
        RpcUtils.auditLogging = true;
        String? generatedRequestId;

        await RpcUtils.executeRpc<String>(
          () async {
            final headers = RpcUtils.buildHeaders(() => 'token');
            generatedRequestId =
                headers.get(RequestIdGenerator.headerName)!.first;
            return 'ok';
          },
          errorHandler,
          operationName: 'WithHeaders',
        );

        expect(generatedRequestId, isNotNull);
        final auditLine = records
            .firstWhere((r) => r.message.startsWith('RPC_AUDIT'))
            .message;
        expect(auditLine, contains('request_id=$generatedRequestId'));
      });

      test('emits status=err with code on ConnectException', () async {
        RpcUtils.auditLogging = true;

        try {
          await RpcUtils.executeRpc<String>(
            () async =>
                throw createConnectException(connect.Code.notFound, 'gone'),
            errorHandler,
            operationName: 'Failing',
          );
        } catch (_) {
          // Expected.
        }

        final auditLine = records
            .firstWhere((r) => r.message.startsWith('RPC_AUDIT'))
            .message;
        expect(auditLine, contains('rpc=Failing'));
        expect(auditLine, contains('status=err'));
        expect(auditLine, contains('code=not_found'));
      });

      test('invokes networkBreadcrumbSink when set and auditLogging on',
          () async {
        RpcUtils.auditLogging = true;
        final sinkCalls = <Map<String, dynamic>>[];
        RpcUtils.networkBreadcrumbSink = (msg, data) {
          sinkCalls.add({'message': msg, ...data});
        };

        await RpcUtils.executeRpc<String>(
          () async => 'ok',
          errorHandler,
          operationName: 'Sinky',
        );

        expect(sinkCalls, hasLength(1));
        expect(sinkCalls.first['rpc'], 'Sinky');
        expect(sinkCalls.first['status'], 'ok');
        expect(sinkCalls.first['message'], 'rpc Sinky');
      });

      test('does not invoke breadcrumb sink when auditLogging is off',
          () async {
        RpcUtils.auditLogging = false;
        var called = false;
        RpcUtils.networkBreadcrumbSink = (_, _) {
          called = true;
        };

        await RpcUtils.executeRpc<String>(
          () async => 'ok',
          errorHandler,
          operationName: 'Quiet',
        );

        expect(called, isFalse);
      });
    });
  });
}

/// Mock performance monitor for testing.
class _MockPerformanceMonitor implements PerformanceMonitor {
  @override
  bool isAvailable = false;
  final List<String> startedTraces = [];
  _MockPerformanceTrace? lastTrace;

  @override
  Future<void> initialize() async {
    isAvailable = true;
  }

  @override
  PerformanceTrace startTrace(String name) {
    startedTraces.add(name);
    lastTrace = _MockPerformanceTrace(name);
    return lastTrace!;
  }

  @override
  void recordScreenLoad(String screenName, Duration duration) {}

  @override
  void recordMetric(String name, double value, {Map<String, String>? attributes}) {}
}

/// Mock performance trace for testing.
class _MockPerformanceTrace implements PerformanceTrace {
  final String name;
  bool started = false;
  bool stopped = false;
  final Map<String, int> metrics = {};
  final Map<String, String> attributes = {};

  _MockPerformanceTrace(this.name);

  @override
  void start() {
    started = true;
  }

  @override
  void stop() {
    stopped = true;
  }

  @override
  void putMetric(String name, int value) {
    metrics[name] = value;
  }

  @override
  void putAttribute(String name, String value) {
    attributes[name] = value;
  }
}
