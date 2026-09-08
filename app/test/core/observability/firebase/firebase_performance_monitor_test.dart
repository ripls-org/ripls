import 'package:firebase_performance/firebase_performance.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/observability/firebase/firebase_performance_monitor.dart';
import 'package:ripls/core/observability/providers.dart';

@GenerateMocks([FirebasePerformance, Trace, HttpMetric])
import 'firebase_performance_monitor_test.mocks.dart';

void main() {
  late MockFirebasePerformance mockPerformance;
  late FirebasePerformanceMonitor monitor;

  setUp(() {
    mockPerformance = MockFirebasePerformance();
    monitor = FirebasePerformanceMonitor(performance: mockPerformance);

    // Setup default stubs
    when(mockPerformance.setPerformanceCollectionEnabled(any)).thenAnswer((
      _,
    ) async {});
  });

  group('FirebasePerformanceMonitor', () {
    group('initialization', () {
      test('isAvailable returns false before initialization', () {
        expect(monitor.isAvailable, isFalse);
      });

      test('initialize enables performance collection', () async {
        await monitor.initialize();

        verify(mockPerformance.setPerformanceCollectionEnabled(true)).called(1);
        expect(monitor.isAvailable, isTrue);
      });

      test('initialize does nothing if already initialized', () async {
        await monitor.initialize();
        await monitor.initialize();

        // Should only be called once
        verify(mockPerformance.setPerformanceCollectionEnabled(true)).called(1);
      });
    });

    group('startTrace', () {
      late MockTrace mockTrace;

      setUp(() async {
        mockTrace = MockTrace();
        when(mockPerformance.newTrace(any)).thenReturn(mockTrace);
        await monitor.initialize();
      });

      test('creates a new trace with the given name', () {
        final trace = monitor.startTrace('test_operation');

        verify(mockPerformance.newTrace('test_operation')).called(1);
        expect(trace, isA<PerformanceTrace>());
      });

      test('returns no-op trace before initialization', () {
        final uninitializedMonitor = FirebasePerformanceMonitor(
          performance: mockPerformance,
        );

        final trace = uninitializedMonitor.startTrace('test');

        verifyNever(mockPerformance.newTrace(any));
        // Should not throw when using the no-op trace
        trace.start();
        trace.putAttribute('key', 'value');
        trace.putMetric('metric', 42);
        trace.stop();
      });
    });

    group('PerformanceTrace wrapper', () {
      late MockTrace mockTrace;

      setUp(() async {
        mockTrace = MockTrace();
        when(mockPerformance.newTrace(any)).thenReturn(mockTrace);
        await monitor.initialize();
      });

      test('start calls underlying trace start', () {
        final trace = monitor.startTrace('test');
        trace.start();

        verify(mockTrace.start()).called(1);
      });

      test('stop calls underlying trace stop', () {
        final trace = monitor.startTrace('test');
        trace.start();
        trace.stop();

        verify(mockTrace.stop()).called(1);
      });

      test('putMetric calls underlying trace setMetric', () {
        final trace = monitor.startTrace('test');
        trace.putMetric('count', 42);

        verify(mockTrace.setMetric('count', 42)).called(1);
      });

      test('putAttribute calls underlying trace putAttribute', () {
        final trace = monitor.startTrace('test');
        trace.putAttribute('key', 'value');

        verify(mockTrace.putAttribute('key', 'value')).called(1);
      });

      test('putAttribute truncates values longer than 100 chars', () {
        final trace = monitor.startTrace('test');
        final longValue = 'a' * 150;

        trace.putAttribute('key', longValue);

        verify(mockTrace.putAttribute('key', 'a' * 100)).called(1);
      });
    });

    group('recordScreenLoad', () {
      late MockTrace mockTrace;

      setUp(() async {
        mockTrace = MockTrace();
        when(mockPerformance.newTrace(any)).thenReturn(mockTrace);
        await monitor.initialize();
      });

      test('creates and completes a trace for screen load', () {
        monitor.recordScreenLoad(
          'HomeScreen',
          const Duration(milliseconds: 250),
        );

        verify(mockPerformance.newTrace('screen_load_HomeScreen')).called(1);
        verify(mockTrace.start()).called(1);
        verify(mockTrace.putAttribute('screen_name', 'HomeScreen')).called(1);
        verify(mockTrace.setMetric('duration_ms', 250)).called(1);
        verify(mockTrace.stop()).called(1);
      });

      test('does nothing before initialization', () {
        final uninitializedMonitor = FirebasePerformanceMonitor(
          performance: mockPerformance,
        );

        uninitializedMonitor.recordScreenLoad(
          'HomeScreen',
          const Duration(milliseconds: 250),
        );

        verifyNever(mockPerformance.newTrace(any));
      });
    });

    group('recordMetric', () {
      late MockTrace mockTrace;

      setUp(() async {
        mockTrace = MockTrace();
        when(mockPerformance.newTrace(any)).thenReturn(mockTrace);
        await monitor.initialize();
      });

      test('creates and completes a trace for metric', () {
        monitor.recordMetric('api_latency', 150.5);

        verify(mockPerformance.newTrace('metric_api_latency')).called(1);
        verify(mockTrace.start()).called(1);
        verify(mockTrace.setMetric('api_latency', 151)).called(1); // rounded
        verify(mockTrace.stop()).called(1);
      });

      test('includes attributes when provided', () {
        monitor.recordMetric(
          'api_latency',
          150,
          attributes: {'endpoint': '/users', 'method': 'GET'},
        );

        verify(mockTrace.putAttribute('endpoint', '/users')).called(1);
        verify(mockTrace.putAttribute('method', 'GET')).called(1);
      });

      test('truncates long attribute values', () {
        final longValue = 'a' * 150;
        monitor.recordMetric('test', 100, attributes: {'key': longValue});

        verify(mockTrace.putAttribute('key', 'a' * 100)).called(1);
      });
    });

    group('startHttpMetric', () {
      late MockHttpMetric mockHttpMetric;

      setUp(() async {
        mockHttpMetric = MockHttpMetric();
        when(
          mockPerformance.newHttpMetric(any, any),
        ).thenReturn(mockHttpMetric);
        await monitor.initialize();
      });

      test('creates a new HTTP metric', () {
        final metric = monitor.startHttpMetric(
          'https://api.example.com/users',
          HttpMethod.Get,
        );

        verify(
          mockPerformance.newHttpMetric(
            'https://api.example.com/users',
            HttpMethod.Get,
          ),
        ).called(1);
        expect(metric, isA<HttpMetric>());
      });

      test('returns no-op metric before initialization', () {
        final uninitializedMonitor = FirebasePerformanceMonitor(
          performance: mockPerformance,
        );

        final metric = uninitializedMonitor.startHttpMetric(
          'https://api.example.com',
          HttpMethod.Post,
        );

        verifyNever(mockPerformance.newHttpMetric(any, any));
        // Should not throw when using the no-op metric
        metric.httpResponseCode = 200;
        metric.responsePayloadSize = 1024;
      });
    });

    group('disable', () {
      test('disables performance collection', () async {
        await monitor.initialize();
        await monitor.disable();

        verify(
          mockPerformance.setPerformanceCollectionEnabled(false),
        ).called(1);
        expect(monitor.isAvailable, isFalse);
      });
    });
  });
}
