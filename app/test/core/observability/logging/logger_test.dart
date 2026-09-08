import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/breadcrumbs.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/core/observability/providers.dart';
import 'package:ripls/core/utils/request_id.dart';

void main() {
  group('LogContext', () {
    test('creates with default values', () {
      const context = LogContext();

      expect(context.userId, isNull);
      expect(context.extra, isEmpty);
    });

    test('creates with all values', () {
      const context = LogContext(
        userId: 'user-456',
        extra: {'key': 'value'},
      );

      expect(context.userId, 'user-456');
      expect(context.extra, {'key': 'value'});
    });

    test('copyWith preserves values', () {
      const original = LogContext(userId: 'user-456');

      final copied = original.copyWith(extra: {'new': 'data'});

      expect(copied.userId, 'user-456');
      expect(copied.extra, {'new': 'data'});
    });

    test('copyWith overwrites values', () {
      const original = LogContext(userId: 'user-1');

      final copied = original.copyWith(userId: 'user-2');

      expect(copied.userId, 'user-2');
    });

    test('toMap masks userId', () {
      const context = LogContext(userId: 'user_abc123def456');

      expect(context.toMap(), {'user_id': 'user_abc...'});
    });

    test('toMap includes extra fields', () {
      const context = LogContext(
        userId: 'user_abc123def456',
        extra: {'operation': 'test'},
      );

      expect(context.toMap(), {
        'user_id': 'user_abc...',
        'operation': 'test',
      });
    });

    test('toMap returns empty map for empty context', () {
      const context = LogContext();

      expect(context.toMap(), isEmpty);
    });
  });

  group('LogContextHolder', () {
    setUp(() {
      LogContextHolder.clear();
      LogContextHolder.clearBreadcrumbBuffer();
    });

    test('current returns empty context by default', () {
      expect(LogContextHolder.current.userId, isNull);
    });

    test('setCurrent updates current context', () {
      const context = LogContext(userId: 'user-123');

      LogContextHolder.setCurrent(context);

      expect(LogContextHolder.current.userId, 'user-123');
    });

    test('clear resets context', () {
      LogContextHolder.setCurrent(const LogContext(userId: 'user-123'));

      LogContextHolder.clear();

      expect(LogContextHolder.current.userId, isNull);
    });

    test('setUserId updates user ID', () {
      LogContextHolder.setUserId('user-123');

      expect(LogContextHolder.current.userId, 'user-123');
    });

    test('setUserId can clear user ID', () {
      LogContextHolder.setUserId('user-123');
      LogContextHolder.setUserId(null);

      expect(LogContextHolder.current.userId, isNull);
    });

    test('breadcrumbBuffer returns shared buffer', () {
      final buffer1 = LogContextHolder.breadcrumbBuffer;
      final buffer2 = LogContextHolder.breadcrumbBuffer;

      expect(identical(buffer1, buffer2), isTrue);
    });

    test('setBreadcrumbBuffer replaces buffer', () {
      final customBuffer = BreadcrumbBuffer(capacity: 50);

      LogContextHolder.setBreadcrumbBuffer(customBuffer);

      expect(LogContextHolder.breadcrumbBuffer.capacity, 50);
    });

    test('clearBreadcrumbBuffer clears breadcrumbs', () {
      LogContextHolder.breadcrumbBuffer.addNavigation('test');
      expect(LogContextHolder.breadcrumbBuffer.isEmpty, isFalse);

      LogContextHolder.clearBreadcrumbBuffer();

      expect(LogContextHolder.breadcrumbBuffer.isEmpty, isTrue);
    });
  });

  group('ObservableLogger', () {
    late ObservableLogger logger;
    late BreadcrumbBuffer buffer;

    setUp(() {
      LogContextHolder.clear();
      buffer = BreadcrumbBuffer();
      LogContextHolder.setBreadcrumbBuffer(buffer);
      logger = ObservableLogger.named('TestLogger');
    });

    test('named creates logger with name', () {
      expect(logger.name, 'TestLogger');
    });

    group('debug', () {
      test('does not record breadcrumb', () {
        logger.debug('Debug message');

        expect(buffer.isEmpty, isTrue);
      });

      test('accepts data parameter', () {
        // Just verify it doesn't throw
        expect(
          () => logger.debug('Debug message', {'key': 'value'}),
          returnsNormally,
        );
      });
    });

    group('info', () {
      test('does not record breadcrumb', () {
        logger.info('Info message');

        expect(buffer.isEmpty, isTrue);
      });

      test('accepts data parameter', () {
        expect(
          () => logger.info('Info message', {'key': 'value'}),
          returnsNormally,
        );
      });
    });

    group('warning', () {
      test('records breadcrumb', () {
        logger.warning('Warning message');

        expect(buffer.length, 1);
        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].category, 'TestLogger');
        expect(breadcrumbs[0].message, 'Warning message');
        expect(breadcrumbs[0].level, BreadcrumbLevel.warning);
      });

      test('includes data in breadcrumb', () {
        logger.warning('Warning message', {'code': 123});

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['code'], 123);
      });

      test('redacts PII in data', () {
        logger.warning('Warning message', {'email': 'alice@example.com'});

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['email'], 'al***@example.com');
      });

      test('includes zone-scoped request_id in breadcrumb', () {
        RequestIdGenerator.runWithRequestId('req-123', () {
          logger.warning('Warning message');
        });

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['request_id'], 'req-123');
      });
    });

    group('error', () {
      test('records breadcrumb', () {
        logger.error('Error message');

        expect(buffer.length, 1);
        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].category, 'TestLogger');
        expect(breadcrumbs[0].message, 'Error message');
        expect(breadcrumbs[0].level, BreadcrumbLevel.error);
      });

      test('includes error in breadcrumb', () {
        logger.error('Error message', Exception('Test error'));

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['error'], contains('Test error'));
      });

      test('includes stack trace in breadcrumb', () {
        logger.error('Error message', Exception('Test'), StackTrace.current);

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['stack_trace'], isNotNull);
      });
    });

    group('network', () {
      test('records network breadcrumb', () {
        logger.network('API call');

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].category, 'network');
        expect(breadcrumbs[0].message, 'API call');
      });

      test('includes all network details', () {
        logger.network(
          'API call',
          method: 'POST',
          endpoint: '/users',
          statusCode: 200,
          durationMs: 150,
        );

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['method'], 'POST');
        expect(breadcrumbs[0].data!['endpoint'], '/users');
        expect(breadcrumbs[0].data!['status_code'], 200);
        expect(breadcrumbs[0].data!['duration_ms'], 150);
      });

      test('includes extra data', () {
        logger.network('API call', extra: {'service': 'users'});

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['service'], 'users');
      });
    });

    group('userAction', () {
      test('records user action breadcrumb', () {
        logger.userAction('Button tapped');

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].category, 'user_action');
        expect(breadcrumbs[0].message, 'Button tapped');
      });

      test('includes data', () {
        logger.userAction('Button tapped', {'button': 'submit'});

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['button'], 'submit');
      });
    });

    group('stateChange', () {
      test('records state change breadcrumb', () {
        logger.stateChange('App backgrounded');

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].category, 'state_change');
        expect(breadcrumbs[0].message, 'App backgrounded');
      });
    });

    group('context propagation', () {
      test('includes zone-scoped request_id in warnings', () {
        RequestIdGenerator.runWithRequestId('req-abc123', () {
          logger.warning('Test warning');
        });

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['request_id'], 'req-abc123');
      });

      test('omits request_id when no zone is active', () {
        logger.warning('Test warning');

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data, anyOf(isNull, isNot(contains('request_id'))));
      });

      test('includes masked user_id in warnings', () {
        LogContextHolder.setUserId('user_abc123def456');

        logger.warning('Test warning');

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['user_id'], 'user_abc...');
      });

      test('includes both request_id and user_id', () {
        LogContextHolder.setUserId('user_abc123def456');

        RequestIdGenerator.runWithRequestId('req-123', () {
          logger.warning('Test warning');
        });

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['request_id'], 'req-123');
        expect(breadcrumbs[0].data!['user_id'], 'user_abc...');
      });
    });
  });
}
