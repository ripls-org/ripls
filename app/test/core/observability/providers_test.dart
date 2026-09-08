import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/providers.dart';

void main() {
  group('Breadcrumb', () {
    test('creates with required fields', () {
      final breadcrumb = Breadcrumb(
        category: 'navigation',
        message: 'User navigated to home screen',
      );

      expect(breadcrumb.category, 'navigation');
      expect(breadcrumb.message, 'User navigated to home screen');
      expect(breadcrumb.level, BreadcrumbLevel.info);
      expect(breadcrumb.data, isNull);
      expect(breadcrumb.timestamp, isNotNull);
    });

    test('creates with all fields', () {
      final timestamp = DateTime(2025, 1, 15);
      final breadcrumb = Breadcrumb(
        category: 'error',
        message: 'API request failed',
        data: {'endpoint': '/api/users', 'status': 500},
        level: BreadcrumbLevel.error,
        timestamp: timestamp,
      );

      expect(breadcrumb.category, 'error');
      expect(breadcrumb.message, 'API request failed');
      expect(breadcrumb.level, BreadcrumbLevel.error);
      expect(breadcrumb.data, {'endpoint': '/api/users', 'status': 500});
      expect(breadcrumb.timestamp, timestamp);
    });
  });

  group('BreadcrumbLevel', () {
    test('has all expected values', () {
      expect(BreadcrumbLevel.values, hasLength(4));
      expect(BreadcrumbLevel.debug, isNotNull);
      expect(BreadcrumbLevel.info, isNotNull);
      expect(BreadcrumbLevel.warning, isNotNull);
      expect(BreadcrumbLevel.error, isNotNull);
    });
  });

  group('NoOpCrashReporter', () {
    late NoOpCrashReporter reporter;

    setUp(() {
      reporter = NoOpCrashReporter();
    });

    test('isAvailable returns false', () {
      expect(reporter.isAvailable, isFalse);
    });

    test('initialize completes without error', () async {
      await expectLater(reporter.initialize(), completes);
    });

    test('recordError completes without error', () async {
      await expectLater(
        reporter.recordError(Exception('test'), stackTrace: StackTrace.current),
        completes,
      );
    });

    test('addBreadcrumb does not throw', () {
      expect(
        () => reporter.addBreadcrumb(Breadcrumb(
          category: 'test',
          message: 'test',
        )),
        returnsNormally,
      );
    });

    test('setUserId does not throw', () {
      expect(() => reporter.setUserId('user123'), returnsNormally);
      expect(() => reporter.setUserId(null), returnsNormally);
    });

    test('setCustomKey does not throw', () {
      expect(() => reporter.setCustomKey('key', 'value'), returnsNormally);
    });

    test('clearUserData does not throw', () {
      expect(() => reporter.clearUserData(), returnsNormally);
    });

    test('disable completes without error', () async {
      await expectLater(reporter.disable(), completes);
    });

    test('sendUnsentReports completes without error', () async {
      await expectLater(reporter.sendUnsentReports(), completes);
    });
  });

  group('NoOpPerformanceMonitor', () {
    late NoOpPerformanceMonitor monitor;

    setUp(() {
      monitor = NoOpPerformanceMonitor();
    });

    test('isAvailable returns false', () {
      expect(monitor.isAvailable, isFalse);
    });

    test('initialize completes without error', () async {
      await expectLater(monitor.initialize(), completes);
    });

    test('startTrace returns a trace', () {
      final trace = monitor.startTrace('test_trace');
      expect(trace, isA<PerformanceTrace>());
    });

    test('trace operations do not throw', () {
      final trace = monitor.startTrace('test_trace');
      expect(() => trace.start(), returnsNormally);
      expect(() => trace.putMetric('metric', 100), returnsNormally);
      expect(() => trace.putAttribute('attr', 'value'), returnsNormally);
      expect(() => trace.stop(), returnsNormally);
    });

    test('recordScreenLoad does not throw', () {
      expect(
        () => monitor.recordScreenLoad('HomeScreen', const Duration(milliseconds: 500)),
        returnsNormally,
      );
    });

    test('recordMetric does not throw', () {
      expect(
        () => monitor.recordMetric('custom_metric', 42, attributes: {'key': 'value'}),
        returnsNormally,
      );
    });
  });

  group('NoOpAnalyticsProvider', () {
    late NoOpAnalyticsProvider provider;

    setUp(() {
      provider = NoOpAnalyticsProvider();
    });

    test('isAvailable returns false', () {
      expect(provider.isAvailable, isFalse);
    });

    test('initialize completes without error', () async {
      await expectLater(provider.initialize(), completes);
    });

    test('logEvent completes without error', () async {
      await expectLater(
        provider.logEvent('button_click', parameters: {'button_id': 'submit'}),
        completes,
      );
    });

    test('logScreenView completes without error', () async {
      await expectLater(
        provider.logScreenView(screenName: 'HomeScreen', screenClass: 'HomeScreen'),
        completes,
      );
    });

    test('setUserProperty completes without error', () async {
      await expectLater(
        provider.setUserProperty('premium_user', 'true'),
        completes,
      );
    });

    test('setUserId completes without error', () async {
      await expectLater(provider.setUserId('user123'), completes);
      await expectLater(provider.setUserId(null), completes);
    });

    test('resetAnalyticsData completes without error', () async {
      await expectLater(provider.resetAnalyticsData(), completes);
    });
  });
}
